// Package postgres cài đặt kho lưu trữ thông báo bằng PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/modules/notification/domain"
)

// LogStore ghi và đọc nhật ký thông báo.
type LogStore struct {
	pool *pgxpool.Pool
}

func NewLogStore(pool *pgxpool.Pool) *LogStore {
	return &LogStore{pool: pool}
}

var _ domain.Repository = (*LogStore)(nil)

// Save ghi bản ghi thông báo.
//
// Trả ErrDuplicate khi (event_id, template, recipient) đã tồn tại.
//
// CHỈ MỤC UNIQUE Ở DATABASE LÀ THỨ CHẶN THẬT: kiểm tra trước khi ghi vẫn
// lọt khi hai worker cùng xử lý một event, và khi đó khách nhận hai email
// giống hệt nhau.
func (s *LogStore) Save(ctx context.Context, n *domain.Notification) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notification_log (
			event_id, channel, category, template, recipient, user_id,
			subject, body, status, provider_message_id, skip_reason,
			error, attempts, reference_type, reference_id, created_at, sent_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id`,
		n.EventID(), string(n.Channel()), string(n.Category()), n.Template(),
		n.Recipient(), n.UserID(), n.Subject(), n.Body(), string(n.Status()),
		n.ProviderMessageID(), n.SkipReason(), n.LastError(), n.Attempts(),
		n.ReferenceType(), n.ReferenceID(), n.CreatedAt(),
		nullTime(n.SentAt())).Scan(new(int64))

	if err != nil {
		if isUnique(err, "notification_log_dedup_idx") {
			return domain.ErrDuplicate
		}
		return fmt.Errorf("notification: ghi nhật ký: %w", err)
	}
	return nil
}

// Update ghi lại kết quả gửi.
//
// Khóa theo (event_id, template, recipient) chứ không theo id: bên gọi
// không giữ id, và bộ ba này đã là khóa duy nhất.
func (s *LogStore) Update(ctx context.Context, n *domain.Notification) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notification_log
		   SET status = $4, provider_message_id = $5, error = $6,
		       attempts = $7, sent_at = $8, subject = $9, body = $10
		 WHERE event_id = $1 AND template = $2 AND recipient = $3`,
		n.EventID(), n.Template(), n.Recipient(),
		string(n.Status()), n.ProviderMessageID(), n.LastError(),
		n.Attempts(), nullTime(n.SentAt()), n.Subject(), n.Body())
	if err != nil {
		return fmt.Errorf("notification: cập nhật nhật ký: %w", err)
	}
	return nil
}

const logCols = `
	event_id, channel, category, template, recipient, user_id,
	subject, body, status, provider_message_id, skip_reason,
	error, attempts, reference_type, reference_id, created_at, sent_at`

// ListByReference trả lịch sử thông báo của một đối tượng.
func (s *LogStore) ListByReference(
	ctx context.Context, refType, refID string,
) ([]*domain.Notification, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+logCols+`
		  FROM notification_log
		 WHERE reference_type = $1 AND reference_id = $2
		 ORDER BY created_at`, refType, refID)
	if err != nil {
		return nil, fmt.Errorf("notification: đọc nhật ký: %w", err)
	}
	defer rows.Close()

	var out []*domain.Notification
	for rows.Next() {
		n, err := scanLog(rows)
		if err != nil {
			return nil, fmt.Errorf("notification: đọc nhật ký: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CountByStatus đếm theo trạng thái, cho giám sát.
func (s *LogStore) CountByStatus(ctx context.Context) (map[domain.Status]int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT status, count(*) FROM notification_log GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("notification: đếm nhật ký: %w", err)
	}
	defer rows.Close()

	out := map[domain.Status]int{}
	for rows.Next() {
		var (
			st string
			n  int
		)
		if err := rows.Scan(&st, &n); err != nil {
			return nil, fmt.Errorf("notification: đếm nhật ký: %w", err)
		}
		out[domain.Status(st)] = n
	}
	return out, rows.Err()
}

func scanLog(row interface{ Scan(...any) error }) (*domain.Notification, error) {
	var (
		p       domain.RestoreParams
		channel string
		cat     string
		status  string
		sentAt  *time.Time
	)
	if err := row.Scan(
		&p.EventID, &channel, &cat, &p.Template, &p.Recipient, &p.UserID,
		&p.Subject, &p.Body, &status, &p.ProviderMessageID, &p.SkipReason,
		&p.LastError, &p.Attempts, &p.ReferenceType, &p.ReferenceID,
		&p.CreatedAt, &sentAt,
	); err != nil {
		return nil, err
	}

	p.Channel = domain.Channel(channel)
	p.Category = domain.Category(cat)
	p.Status = domain.Status(status)
	if sentAt != nil {
		p.SentAt = *sentAt
	}

	return domain.Restore(p), nil
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.ConstraintName == constraint || pgErr.Code == "23505"
}
