package notification

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/modules/notification/domain"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// NotifyOnOrderEvents gửi email giao dịch khi đơn hàng có thay đổi.
//
// # Đây là bên nhận thuần túy
//
// Nó KHÔNG gọi module nghiệp vụ nào — mọi dữ liệu để soạn email đến từ
// payload event. Đó là ràng buộc quan trọng nhất của module notification:
// phụ thuộc toàn hệ thống thì một module lỗi sẽ làm hỏng việc gửi mọi loại
// thông báo, kể cả mã OTP.
//
// # Hai event, bốn loại email
//
//	checkout.completed            → xác nhận đơn hàng
//	fulfillment.progress_changed  → đã gửi / đã giao / đã hủy
//
// Không nghe `order.placed`: payload của nó không mang email và tên sản
// phẩm. `checkout.completed` mang đủ cả hai.
type NotifyOnOrderEvents struct {
	module *Module
	log    *slog.Logger
}

// NewOrderNotifier tạo bên nhận gửi email về đơn hàng.
func NewOrderNotifier(m *Module, log *slog.Logger) *NotifyOnOrderEvents {
	if log == nil {
		log = slog.Default()
	}
	return &NotifyOnOrderEvents{module: m, log: log}
}

var _ eventbus.Handler = (*NotifyOnOrderEvents)(nil)

func (h *NotifyOnOrderEvents) Name() string {
	return "notification.notify_on_order_events"
}

func (h *NotifyOnOrderEvents) EventTypes() []string {
	return []string{
		eventbus.TypeCheckoutCompleted,
		eventbus.TypeFulfillmentProgress,
	}
}

// orderPlacedPayload là phần dữ liệu cần từ event đặt hàng.
type orderPlacedPayload struct {
	OrderID     string `json:"order_id"`
	OrderNumber string `json:"order_number"`
	CustomerID  string `json:"customer_id"`
	GuestEmail  string `json:"guest_email"`
	Currency    string `json:"currency"`

	Reservations []struct {
		ProductName string `json:"product_name"`
		Quantity    int    `json:"quantity"`
		LineTotal   int64  `json:"line_total"`
	} `json:"reservations"`
}

// progressPayload là phần dữ liệu cần từ event tiến độ giao hàng.
type progressPayload struct {
	OrderID        string `json:"order_id"`
	FONumber       string `json:"fo_number"`
	NewStatus      string `json:"new_status"`
	TrackingNumber string `json:"tracking_number"`
	CustomerID     string `json:"customer_id"`
	Email          string `json:"email"`
}

func (h *NotifyOnOrderEvents) Handle(ctx context.Context, e eventbus.Event) error {
	switch e.Type {
	case eventbus.TypeCheckoutCompleted:
		return h.notifyOrderPlaced(ctx, e)
	case eventbus.TypeFulfillmentProgress:
		return h.notifyProgress(ctx, e)
	}
	return nil
}

// notifyOrderPlaced gửi email xác nhận đơn hàng.
//
// Đây là email QUAN TRỌNG NHẤT của hệ thống: nó là bằng chứng đầu tiên
// khách có rằng tiền của họ đã đổi lấy một cam kết.
func (h *NotifyOnOrderEvents) notifyOrderPlaced(
	ctx context.Context, e eventbus.Event,
) error {
	var p orderPlacedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event đặt hàng: %w", err)
	}

	items := make([]domain.OrderItem, 0, len(p.Reservations))
	var total int64
	for _, r := range p.Reservations {
		items = append(items, domain.OrderItem{
			ProductName: r.ProductName,
			Quantity:    r.Quantity,
		})
		total += r.LineTotal
	}

	msg := domain.ComposeOrderConfirmed(domain.OrderInfo{
		OrderNumber: p.OrderNumber,
		Items:       items,
		Total:       total,
		Currency:    p.Currency,
	})

	return h.module.Send(ctx, SendRequest{
		EventID:  e.ID.String(),
		Channel:  ChannelEmail,
		Category: CategoryTransactional,
		Template: TemplateOrderConfirmed,

		// Email khách vãng lai đến từ payload. Với khách đã đăng ký, hiện
		// chưa có module customer nên cũng dùng trường này — khi có module
		// đó, event sẽ mang email của tài khoản.
		Recipient: p.GuestEmail,
		UserID:    p.CustomerID,

		Subject:       msg.Subject,
		Body:          msg.Body,
		ReferenceType: "order",
		ReferenceID:   p.OrderID,
	})
}

// notifyProgress gửi email theo tiến độ giao hàng.
//
// CHỈ ba trạng thái sinh email. Gửi email cho MỌI bước (đã xác nhận, đang
// lấy hàng, đã đóng gói) là làm phiền khách — họ chỉ quan tâm ba mốc: hàng
// đã đi, hàng đã tới, hoặc hàng bị hủy.
func (h *NotifyOnOrderEvents) notifyProgress(
	ctx context.Context, e eventbus.Event,
) error {
	var p progressPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event tiến độ: %w", err)
	}

	var (
		template string
		msg      domain.Message
		info     = domain.OrderInfo{
			OrderNumber:    p.FONumber,
			FONumber:       p.FONumber,
			TrackingNumber: p.TrackingNumber,
		}
	)

	switch p.NewStatus {
	case "HANDED_OVER":
		template = TemplateOrderShipped
		msg = domain.ComposeOrderShipped(info)

	case "DELIVERED":
		template = TemplateOrderDelivered
		msg = domain.ComposeOrderDelivered(info)

	case "CANCELLED":
		template = TemplateOrderCancelled
		msg = domain.ComposeOrderCancelled(info, "")

	default:
		// Các bước còn lại là việc nội bộ của seller, khách không cần biết.
		return nil
	}

	return h.module.Send(ctx, SendRequest{
		EventID:       e.ID.String(),
		Channel:       ChannelEmail,
		Category:      CategoryTransactional,
		Template:      template,
		Recipient:     p.Email,
		UserID:        p.CustomerID,
		Subject:       msg.Subject,
		Body:          msg.Body,
		ReferenceType: "order",
		ReferenceID:   p.OrderID,
	})
}
