// Package webhook ghi nhật ký webhook đến và cưỡng chế idempotency.
//
// Không biết gì về nghiệp vụ: nó chỉ trả lời "sự kiện này đã nhận chưa"
// và giữ lại nguyên văn những gì bên ngoài đã gửi.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Recorder ghi webhook đến vào bảng webhook_event.
type Recorder struct {
	pool *pgxpool.Pool
}

func NewRecorder(pool *pgxpool.Pool) *Recorder {
	return &Recorder{pool: pool}
}

// Event là một webhook đã được ghi nhận.
type Event struct {
	ID string

	// AlreadyReceived đúng khi nhà cung cấp gửi lại một sự kiện cũ.
	//
	// Bên gọi phải trả 200 và KHÔNG xử lý lại — nếu trả lỗi, nhà cung cấp
	// sẽ gửi tiếp mãi.
	AlreadyReceived bool

	// AlreadyDone đúng khi lần nhận trước đã xử lý thành công.
	//
	// Khác AlreadyReceived: một sự kiện có thể đã nhận mà chưa xử lý xong,
	// và khi đó gửi lại là cơ hội để thử lại.
	AlreadyDone bool
}

// Log lưu webhook và cho biết đây có phải lần đầu không.
//
// # Vì sao chỉ mục UNIQUE chứ không phải kiểm tra trước khi ghi
//
// Nhà cung cấp gửi trùng là chuyện thường. Hai bản trùng tới CÙNG LÚC thì
// cả hai cùng thấy "chưa có" nếu ta kiểm bằng một câu SELECT riêng — và
// cả hai cùng xử lý. Chỉ ràng buộc ở database mới nằm cùng giao dịch với
// việc ghi.
func (r *Recorder) Log(
	ctx context.Context, provider, eventID, eventType string, than []byte,
) (Event, error) {
	if !json.Valid(than) {
		return Event{}, fmt.Errorf("webhook: thân request không phải JSON hợp lệ")
	}

	id := ids.MustNew(ids.PrefixWebhookEvent).String()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO webhook_event (
			id, provider, provider_event_id, event_type, payload, received_at
		) VALUES ($1,$2,$3,$4,$5,$6)`,
		id, provider, eventID, eventType, than, time.Now().UTC())
	if err == nil {
		return Event{ID: id}, nil
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "webhook_event_provider_provider_event_id_key" {
		return Event{}, fmt.Errorf("webhook: ghi nhật ký: %w", err)
	}

	// Đã nhận trước đó. Đọc lại để bên gọi biết lần trước có xử lý xong không.
	var cu Event
	var done *time.Time
	err = r.pool.QueryRow(ctx, `
		SELECT id, processed_at FROM webhook_event
		 WHERE provider = $1 AND provider_event_id = $2`,
		provider, eventID).Scan(&cu.ID, &done)
	if err != nil {
		return Event{}, fmt.Errorf("webhook: đọc lại sự kiện cũ: %w", err)
	}
	cu.AlreadyReceived = true
	cu.AlreadyDone = done != nil
	return cu, nil
}

// MarkDone ghi kết quả xử lý.
//
// loi rỗng nghĩa là thành công. Ngược lại giữ lại lý do và ĐỂ NGỎ
// processed_at, để job thử lại nhặt được.
func (r *Recorder) MarkDone(ctx context.Context, id string, handleErr error) error {
	if handleErr != nil {
		_, err := r.pool.Exec(ctx,
			`UPDATE webhook_event SET last_error = $2 WHERE id = $1`,
			id, handleErr.Error())
		return err
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE webhook_event SET processed_at = $2, last_error = '' WHERE id = $1`,
		id, time.Now().UTC())
	return err
}

// CountPending đếm webhook đã nhận mà chưa xử lý xong — chỉ báo giám sát.
//
// Con số này tăng dần nghĩa là có loại sự kiện ta xử lý hỏng liên tục, và
// nhà cung cấp thì đã coi như xong vì ta trả 200.
func (r *Recorder) CountPending(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM webhook_event WHERE processed_at IS NULL`).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}
