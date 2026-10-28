package order

import (
	"context"
	"fmt"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// AttachGuestOrdersToCustomer gắn mọi đơn vãng lai của một email vào hồ sơ khách.
//
// Xem chú thích ở `API` cho lý do đây mới là "gộp lịch sử".
func (m *Module) AttachGuestOrdersToCustomer(
	ctx context.Context, guestEmail, customerID string,
) (int, error) {
	email := strings.ToLower(strings.TrimSpace(guestEmail))
	if email == "" {
		return 0, ErrInvalidInput
	}
	cid, err := ids.Parse(customerID, ids.PrefixCustomer)
	if err != nil {
		return 0, ErrInvalidID
	}

	n, err := m.svc.AttachGuestOrders(ctx, email, cid)
	if err != nil {
		return 0, fmt.Errorf("order: gắn đơn vãng lai: %w", err)
	}
	return n, nil
}

// CountGuestOrders đếm đơn vãng lai của một email.
func (m *Module) CountGuestOrders(ctx context.Context, guestEmail string) (int, error) {
	email := strings.ToLower(strings.TrimSpace(guestEmail))
	if email == "" {
		return 0, nil
	}
	return m.svc.CountGuestOrders(ctx, email)
}
