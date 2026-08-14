package checkout

import (
	"context"
	"errors"

	checkoutpg "github.com/fashion-commerce/platform/internal/modules/checkout/infrastructure/postgres"

	"github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// eventPublisher nối cổng ra của tầng application với outbox.
//
// Nằm ở tầng này chứ không phải trong application: tầng application chỉ
// biết interface do chính nó định nghĩa, nên nó kiểm chứng được bằng bản
// giả mà không cần database. Đây cũng là điều quy tắc R1 của archcheck
// cưỡng chế.
type eventPublisher struct {
	outbox *eventbus.Outbox
}

var _ application.EventPublisher = (*eventPublisher)(nil)

// NewEventPublisher tạo bộ phát event nối với outbox.
//
// Xuất khẩu để test tầng trên dựng được service với event thật — kiểm
// chứng "hàng chuyển sang Committed" cần cả outbox lẫn dispatcher thật,
// bản giả không chứng minh được gì.
func NewEventPublisher(outbox *eventbus.Outbox) application.EventPublisher {
	return &eventPublisher{outbox: outbox}
}

// PublishCheckoutCompleted ghi event vào outbox BẰNG giao dịch của kho lưu trữ.
//
// Ngữ cảnh phải mang giao dịch mà `SaveWithEvents` đã mở. Thiếu nó thì trả
// lỗi chứ KHÔNG âm thầm mở giao dịch riêng: ghi rời nghĩa là phiên có thể
// chuyển COMPLETED trong khi event không tồn tại, và tồn kho sẽ mãi nằm ở
// Reserved cho một đơn đã bán xong.
func (p *eventPublisher) PublishCheckoutCompleted(
	ctx context.Context, in application.CheckoutCompleted,
) error {
	tx, ok := checkoutpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"checkout: phát event ngoài giao dịch của kho lưu trữ — event và " +
				"thay đổi phiên phải cùng thành công hoặc cùng thất bại")
	}

	type reservationPayload struct {
		ReservationID   string `json:"reservation_id"`
		InventoryItemID string `json:"inventory_item_id"`
		SKUID           string `json:"sku_id"`
		SellerID        string `json:"seller_id"`
		Quantity        int    `json:"quantity"`
	}

	reservations := make([]reservationPayload, 0, len(in.Reservations))
	for _, r := range in.Reservations {
		reservations = append(reservations, reservationPayload{
			ReservationID:   r.ReservationID.String(),
			InventoryItemID: r.InventoryItemID.String(),
			SKUID:           r.SKUID.String(),
			SellerID:        r.SellerID.String(),
			Quantity:        r.Quantity,
		})
	}

	// Payload chứa ĐỦ thông tin để bên nhận xử lý mà KHÔNG phải gọi ngược
	// lại checkout. Nếu inventory phải hỏi "reservation nào thuộc phiên
	// này", event trở nên vô dụng và tạo đúng thứ ghép nối mà nó sinh ra
	// để tránh.
	e, err := eventbus.NewEvent(
		eventbus.TypeCheckoutCompleted,
		eventbus.AggregateCheckout,
		in.CheckoutID,
		struct {
			CheckoutID   string               `json:"checkout_id"`
			OrderID      string               `json:"order_id"`
			CartID       string               `json:"cart_id"`
			CustomerID   string               `json:"customer_id"`
			Reservations []reservationPayload `json:"reservations"`
		}{
			CheckoutID:   in.CheckoutID.String(),
			OrderID:      in.OrderID.String(),
			CartID:       in.CartID.String(),
			CustomerID:   in.CustomerID.String(),
			Reservations: reservations,
		})
	if err != nil {
		return err
	}

	// CorrelationID là mã đơn: mọi việc xảy ra sau khi đặt hàng đều truy
	// ngược được về một đơn cụ thể.
	e = e.WithTrace(in.OrderID.String(), "")

	return p.outbox.PublishTx(ctx, tx, e)
}
