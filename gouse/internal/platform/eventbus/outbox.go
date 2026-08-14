package eventbus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// maxAttempts là số lần thử phát trước khi bỏ cuộc.
//
// CÓ GIỚI HẠN là điều quan trọng nhất ở đây: một event hỏng thử lại vô hạn
// sẽ chặn hàng đợi và làm mọi event sau nó không bao giờ được phát. Thà
// bỏ một event vào dead letter và cảnh báo người vận hành.
const maxAttempts = 5

// Tx là giao dịch database mà bên gọi đang mở.
//
// Kiểu này là lý do outbox hoạt động: event được ghi bằng CHÍNH giao dịch
// của thay đổi nghiệp vụ, nên hai thứ cùng thành công hoặc cùng thất bại.
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Outbox ghi và đọc event chờ phát.
type Outbox struct {
	pool *pgxpool.Pool
}

func NewOutbox(pool *pgxpool.Pool) *Outbox {
	return &Outbox{pool: pool}
}

// PublishTx ghi event vào outbox BẰNG giao dịch của bên gọi.
//
// ĐÂY LÀ HÀM QUAN TRỌNG NHẤT CỦA GÓI NÀY, và cách dùng đúng là:
//
//	tx, _ := pool.Begin(ctx)
//	defer tx.Rollback(ctx)
//
//	// ... ghi thay đổi nghiệp vụ bằng tx ...
//	outbox.PublishTx(ctx, tx, event)   // ← CÙNG tx
//
//	tx.Commit(ctx)
//
// Nếu ghi event bằng một kết nối khác, toàn bộ đảm bảo biến mất: giao dịch
// nghiệp vụ có thể rollback trong khi event đã nằm trong outbox, và bên
// nhận sẽ xử lý một sự thật chưa từng xảy ra.
func (o *Outbox) PublishTx(ctx context.Context, tx Tx, e Event) error {
	if e.ID.IsZero() {
		return errors.New("eventbus: event thiếu định danh")
	}

	version := e.Version
	if version <= 0 {
		version = 1
	}
	occurredAt := e.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO event_outbox (
			event_id, event_type, event_version,
			aggregate_type, aggregate_id, payload,
			correlation_id, causation_id, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (event_id) DO NOTHING`,
		e.ID.String(), e.Type, version,
		e.AggregateType, e.AggregateID.String(), []byte(e.Payload),
		e.CorrelationID, e.CausationID, occurredAt)
	if err != nil {
		return fmt.Errorf("eventbus: ghi outbox: %w", err)
	}
	return nil
}

// Publish ghi event trong một giao dịch riêng.
//
// CHỈ dùng khi KHÔNG có thay đổi nghiệp vụ đi kèm — ví dụ event thuần
// thông báo. Với mọi trường hợp khác, dùng PublishTx: ghi rời nghĩa là mất
// đảm bảo "cùng thành công hoặc cùng thất bại".
func (o *Outbox) Publish(ctx context.Context, e Event) error {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("eventbus: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := o.PublishTx(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// pending là một event chờ phát, kèm số lần đã thử.
type pending struct {
	event    Event
	rowID    int64
	attempts int
}

// fetchPending lấy các event chưa phát, theo thứ tự ghi.
//
// KHÓA HÀNG bằng FOR UPDATE SKIP LOCKED: nhiều tiến trình worker chạy song
// song sẽ lấy các phần khác nhau thay vì tranh nhau cùng một event.
//
// SKIP LOCKED chứ không phải NOWAIT: worker thứ hai bỏ qua hàng đang bị
// khóa và làm việc khác, thay vì báo lỗi rồi thử lại.
func (o *Outbox) fetchPending(ctx context.Context, tx pgx.Tx, limit int) ([]pending, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, event_id, event_type, event_version,
		       aggregate_type, aggregate_id, payload,
		       correlation_id, causation_id, occurred_at, attempts
		  FROM event_outbox
		 WHERE published_at IS NULL AND dead_lettered_at IS NULL
		 ORDER BY id
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("eventbus: đọc outbox: %w", err)
	}
	defer rows.Close()

	var out []pending
	for rows.Next() {
		var (
			p                          pending
			eventID, aggregateID       string
			payload                    []byte
			eventType, aggregateType   string
			correlationID, causationID string
			version                    int
			occurredAt                 time.Time
		)
		if err := rows.Scan(
			&p.rowID, &eventID, &eventType, &version,
			&aggregateType, &aggregateID, &payload,
			&correlationID, &causationID, &occurredAt, &p.attempts,
		); err != nil {
			return nil, fmt.Errorf("eventbus: đọc outbox: %w", err)
		}

		p.event = Event{
			ID:            ids.ID(eventID),
			Type:          eventType,
			Version:       version,
			AggregateType: aggregateType,
			AggregateID:   ids.ID(aggregateID),
			Payload:       payload,
			CorrelationID: correlationID,
			CausationID:   causationID,
			OccurredAt:    occurredAt,
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (o *Outbox) markPublished(ctx context.Context, tx pgx.Tx, rowID int64) error {
	_, err := tx.Exec(ctx,
		`UPDATE event_outbox SET published_at = now() WHERE id = $1`, rowID)
	if err != nil {
		return fmt.Errorf("eventbus: đánh dấu đã phát: %w", err)
	}
	return nil
}

// markFailed ghi lỗi và tăng số lần thử; vượt ngưỡng thì chuyển dead letter.
func (o *Outbox) markFailed(ctx context.Context, tx pgx.Tx, rowID int64, attempts int, cause error) error {
	dead := attempts+1 >= maxAttempts

	_, err := tx.Exec(ctx, `
		UPDATE event_outbox
		   SET attempts = attempts + 1,
		       last_error = $2,
		       dead_lettered_at = CASE WHEN $3 THEN now() ELSE NULL END
		 WHERE id = $1`,
		rowID, truncate(cause.Error(), 1000), dead)
	if err != nil {
		return fmt.Errorf("eventbus: ghi lỗi phát event: %w", err)
	}
	return nil
}

// Stats là chỉ số giám sát của outbox.
type Stats struct {
	// Pending là số event chờ phát.
	Pending int

	// DeadLettered là số event đã bỏ cuộc — cần người xem.
	//
	// Con số này lớn hơn 0 nghĩa là có sự thật nghiệp vụ không tới được
	// bên nhận: đơn đã đặt mà không ai gửi email, hàng đã bán mà tồn kho
	// chưa cập nhật.
	DeadLettered int

	// OldestPendingAge là độ trễ của event cũ nhất chưa phát.
	//
	// Vượt 60 giây nghĩa là bộ đọc đã chết hoặc không theo kịp.
	OldestPendingAge time.Duration
}

// Stats đọc chỉ số giám sát.
func (o *Outbox) Stats(ctx context.Context) (Stats, error) {
	var (
		s      Stats
		oldest *time.Time
	)
	err := o.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE published_at IS NULL AND dead_lettered_at IS NULL),
			count(*) FILTER (WHERE dead_lettered_at IS NOT NULL),
			min(occurred_at) FILTER (WHERE published_at IS NULL AND dead_lettered_at IS NULL)
		  FROM event_outbox`).Scan(&s.Pending, &s.DeadLettered, &oldest)
	if err != nil {
		return Stats{}, fmt.Errorf("eventbus: đọc chỉ số outbox: %w", err)
	}
	if oldest != nil {
		s.OldestPendingAge = time.Since(*oldest)
	}
	return s, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
