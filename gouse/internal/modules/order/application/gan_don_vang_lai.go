package application

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// AttachGuestOrders chuyển chủ các đơn vãng lai sang một hồ sơ khách.
func (s *Service) AttachGuestOrders(
	ctx context.Context, guestEmail string, customerID ids.ID,
) (int, error) {
	return s.orders.AttachGuestOrders(ctx, guestEmail, customerID)
}

// CountGuestOrders đếm đơn vãng lai của một email.
func (s *Service) CountGuestOrders(ctx context.Context, guestEmail string) (int, error) {
	return s.orders.CountGuestOrders(ctx, guestEmail)
}
