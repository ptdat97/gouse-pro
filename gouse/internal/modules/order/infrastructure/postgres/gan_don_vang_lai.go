package postgres

import (
	"context"
	"fmt"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// GanDonVangLai gắn đơn vãng lai của một email vào hồ sơ khách.
//
// `customer_id = ”` trong mệnh đề WHERE là ràng buộc quan trọng nhất: đơn
// đã thuộc về một khách khác KHÔNG được chuyển chủ, kể cả khi trùng email.
// Thiếu điều kiện đó thì một lần xác minh có thể kéo đơn của người khác về.
func (s *OrderStore) GanDonVangLai(
	ctx context.Context, guestEmail string, customerID ids.ID,
) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE "order"
		   SET customer_id = $2, updated_at = now()
		 WHERE lower(guest_email) = $1 AND customer_id = ''`,
		guestEmail, customerID.String())
	if err != nil {
		return 0, fmt.Errorf("order: gắn đơn vãng lai: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// DemDonVangLai đếm đơn vãng lai của một email.
func (s *OrderStore) DemDonVangLai(ctx context.Context, guestEmail string) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM "order"
		 WHERE lower(guest_email) = $1 AND customer_id = ''`, guestEmail).Scan(&n); err != nil {
		return 0, fmt.Errorf("order: đếm đơn vãng lai: %w", err)
	}
	return n, nil
}
