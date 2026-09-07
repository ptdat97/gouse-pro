package application

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// GanDonVangLai chuyển chủ các đơn vãng lai sang một hồ sơ khách.
func (s *Service) GanDonVangLai(
	ctx context.Context, guestEmail string, customerID ids.ID,
) (int, error) {
	return s.orders.GanDonVangLai(ctx, guestEmail, customerID)
}

// DemDonVangLai đếm đơn vãng lai của một email.
func (s *Service) DemDonVangLai(ctx context.Context, guestEmail string) (int, error) {
	return s.orders.DemDonVangLai(ctx, guestEmail)
}
