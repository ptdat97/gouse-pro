package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/identity/domain"
)

// EmailTokenStore lưu token xác minh email.
type EmailTokenStore struct {
	pool *pgxpool.Pool
}

func NewEmailTokenStore(pool *pgxpool.Pool) *EmailTokenStore {
	return &EmailTokenStore{pool: pool}
}

var _ domain.EmailTokenRepository = (*EmailTokenStore)(nil)

const emailTokenCols = ` id, user_id, token_hash, email, expires_at, used_at, created_at`

func (s *EmailTokenStore) Create(ctx context.Context, t *domain.EmailToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO email_verification_token (
			id, user_id, token_hash, email, expires_at, used_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		t.ID().String(), t.UserID().String(), t.Hash(), t.Email(),
		t.ExpiresAt(), nullTime(t.UsedAt()), t.CreatedAt())
	if err != nil {
		return fmt.Errorf("identity: ghi token xác minh: %w", err)
	}
	return nil
}

func (s *EmailTokenStore) FindByHash(
	ctx context.Context, hash string,
) (*domain.EmailToken, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT`+emailTokenCols+`
		   FROM email_verification_token WHERE token_hash = $1`, hash)

	var (
		id, userID, h, email string
		expiresAt, createdAt time.Time
		usedAt               *time.Time
	)
	if err := row.Scan(&id, &userID, &h, &email, &expiresAt, &usedAt, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// CÙNG một lỗi với token hết hạn hay đã dùng: phân biệt chúng
			// cho kẻ dò biết token nào TỪNG có thật.
			return nil, domain.ErrTokenKhongHopLe
		}
		return nil, fmt.Errorf("identity: đọc token xác minh: %w", err)
	}

	var used time.Time
	if usedAt != nil {
		used = *usedAt
	}
	return domain.RestoreEmailToken(domain.RestoreEmailTokenParams{
		ID: ids.ID(id), UserID: ids.ID(userID), Hash: h, Email: email,
		ExpiresAt: expiresAt, UsedAt: used, CreatedAt: createdAt,
	}), nil
}

func (s *EmailTokenStore) MarkUsed(ctx context.Context, t *domain.EmailToken) error {
	// `used_at IS NULL` trong mệnh đề WHERE là thứ chặn DÙNG HAI LẦN khi
	// hai request tới cùng lúc: kiểm ở domain rồi ghi là hai bước, và giữa
	// hai bước đó có chỗ cho request thứ hai chen vào.
	tag, err := s.pool.Exec(ctx, `
		UPDATE email_verification_token
		   SET used_at = $2
		 WHERE id = $1 AND used_at IS NULL`,
		t.ID().String(), t.UsedAt())
	if err != nil {
		return fmt.Errorf("identity: đánh dấu token đã dùng: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTokenKhongHopLe
	}
	return nil
}

func (s *EmailTokenStore) InvalidateForUser(
	ctx context.Context, userID ids.ID, now time.Time,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE email_verification_token
		   SET used_at = $2
		 WHERE user_id = $1 AND used_at IS NULL`,
		userID.String(), now)
	if err != nil {
		return fmt.Errorf("identity: vô hiệu token cũ: %w", err)
	}
	return nil
}
