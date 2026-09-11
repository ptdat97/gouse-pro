package order

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/modules/order/application"
	orderpg "github.com/fashion-commerce/platform/internal/modules/order/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// eventPublisher nối cổng ra của tầng application với outbox.
//
// Nằm ở tầng này chứ không phải trong application: tầng application chỉ
// biết interface do chính nó định nghĩa, nên nó kiểm chứng được bằng bản
// giả mà không cần database (quy tắc R1 của archcheck).
type eventPublisher struct {
	outbox *eventbus.Outbox
}

var _ application.EventPublisher = (*eventPublisher)(nil)

// NewEventPublisher tạo bộ phát event nối với outbox.
//
// Xuất khẩu để test tầng trên dựng được service với event THẬT: kiểm chứng
// "đơn trả trước được mở khóa giao hàng" cần cả outbox lẫn dispatcher
// thật, bản giả không chứng minh được gì.
func NewEventPublisher(outbox *eventbus.Outbox) application.EventPublisher {
	return &eventPublisher{outbox: outbox}
}

// PublishOrderPaid ghi `order.paid` vào outbox BẰNG giao dịch của kho lưu trữ.
//
// Ngữ cảnh phải mang giao dịch mà `UpdateWithAudit` đã mở. Thiếu nó thì
// trả lỗi chứ KHÔNG âm thầm mở giao dịch riêng: ghi rời nghĩa là đơn có
// thể chuyển PAID trong khi event không tồn tại — và khi đó hàng của đơn
// trả trước không bao giờ được mở khóa, còn sổ cái không bao giờ biết tiền
// đã về. Không tiến trình nào đi tìm, vì nhìn từ ngoài đơn trông đã xong.
func (p *eventPublisher) PublishOrderPaid(
	ctx context.Context, in application.OrderPaid,
) error {
	tx, ok := orderpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"order: phát order.paid ngoài giao dịch của kho lưu trữ — event " +
				"và trạng thái đơn phải cùng thành công hoặc cùng thất bại")
	}

	// Payload mang sẵn PHƯƠNG THỨC và SỐ TIỀN.
	//
	// Bên nhận cần phân biệt COD với trả trước, và cần số tiền để ghi bút
	// toán chuyển khoản phải thu thành tiền mặt (ADR-0018 phần B). Cả hai
	// đều KHÔNG phải gọi ngược module order — đúng lý do event tồn tại.
	e, err := eventbus.NewEvent(
		eventbus.TypeOrderPaid,
		eventbus.AggregateOrder,
		in.OrderID,
		struct {
			OrderID       string `json:"order_id"`
			OrderNumber   string `json:"order_number"`
			CustomerID    string `json:"customer_id"`
			PaymentMethod string `json:"payment_method"`
			Amount        int64  `json:"amount"`
			Currency      string `json:"currency"`
			PaidAt        string `json:"paid_at"`
		}{
			OrderID:       in.OrderID.String(),
			OrderNumber:   in.OrderNumber,
			CustomerID:    in.CustomerID.String(),
			PaymentMethod: in.PaymentMethod,
			Amount:        in.Total.Amount(),
			Currency:      in.Currency,
			PaidAt:        in.PaidAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	if err != nil {
		return err
	}

	e = e.WithTrace(in.OrderID.String(), "")

	return p.outbox.PublishTx(ctx, tx, e)
}

// PublishOrderCancelled ghi `order.cancelled` vào outbox.
//
// # Vì sao event này là mốc MỞ ĐƯỜNG RA của kho
//
// Đường VÀO đã có từ lâu: Reserved → Committed khi đặt hàng. Đường RA chỉ
// có cho đơn THỰC HIỆN bị hủy (`inventory.ReleaseOnFulfillmentCancelled`),
// nên hủy cả ĐƠN để lại hàng ở trạng thái cam kết VĨNH VIỄN — có thật
// trên kệ nhưng hệ thống mãi coi là đã hứa cho một đơn không còn tồn tại.
//
// Chú thích của chính bên nhận kia đã mô tả đúng lỗi này, kèm lần kiểm
// chứng bằng đơn thật: "đặt 5 món rồi hủy, tồn kho đứng nguyên 15 khả
// dụng / 5 cam kết. Không lỗi, không log."
//
// Bên nhận là FULFILLMENT chứ không phải inventory: nó hủy các đơn thực
// hiện, và việc đó phát `fulfillment.cancelled` — đường nhả hàng đã có và
// đã kiểm chứng. Nối thẳng order → inventory sẽ là đường nhả THỨ HAI cho
// cùng một việc, và hai đường nhả nghĩa là sớm muộn nhả hai lần.
func (p *eventPublisher) PublishOrderCancelled(
	ctx context.Context, in application.OrderCancelled,
) error {
	tx, ok := orderpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"order: phát order.cancelled ngoài giao dịch của kho lưu trữ — " +
				"event và trạng thái đơn phải cùng thành công hoặc cùng thất bại")
	}

	e, err := eventbus.NewEvent(
		eventbus.TypeOrderCancelled,
		eventbus.AggregateOrder,
		in.OrderID,
		struct {
			OrderID     string `json:"order_id"`
			OrderNumber string `json:"order_number"`
			Reason      string `json:"reason"`
			CancelledAt string `json:"cancelled_at"`
		}{
			OrderID:     in.OrderID.String(),
			OrderNumber: in.OrderNumber,
			Reason:      in.Reason,
			CancelledAt: in.CancelledAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	if err != nil {
		return err
	}

	e = e.WithTrace(in.OrderID.String(), "")
	return p.outbox.PublishTx(ctx, tx, e)
}
