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

	// addressPayload là địa chỉ giao, KHÔNG kèm email.
	//
	// Chỉ những trường CẦN cho việc giao hàng: người nhận, số điện thoại
	// để gọi trước khi giao, và địa chỉ. Email khách không giúp giao hàng
	// và không được đưa tới seller.
	type addressPayload struct {
		RecipientName string `json:"recipient_name"`
		Phone         string `json:"phone"`
		StreetAddress string `json:"street_address"`
		Ward          string `json:"ward"`
		District      string `json:"district"`
		Province      string `json:"province"`
		CountryCode   string `json:"country_code"`
	}

	type reservationPayload struct {
		ReservationID   string `json:"reservation_id"`
		InventoryItemID string `json:"inventory_item_id"`
		LineID          string `json:"line_id"`
		SKUID           string `json:"sku_id"`
		SellerID        string `json:"seller_id"`
		Quantity        int    `json:"quantity"`
		ProductName     string `json:"product_name"`

		// VariantDescription ("Trắng / M") là thứ SELLER cần để nhặt đúng
		// hàng. Với thời trang, tên sản phẩm KHÔNG đủ: cùng một chiếc áo có
		// năm size nằm ở năm ô kệ khác nhau.
		VariantDescription string `json:"variant_description"`

		// UnitPrice ĐÃ ĐÓNG BĂNG. Gửi kèm thay vì để bên nhận chia
		// LineTotal cho Quantity — đó là phép chia số nguyên và nó làm
		// tròn sai với giá không chia hết.
		UnitPrice int64 `json:"unit_price"`

		// Tiền ĐÃ ĐÓNG BĂNG, để fulfillment tính phần của từng seller.
		LineTotal        int64 `json:"line_total"`
		CommissionAmount int64 `json:"commission_amount"`
	}

	reservations := make([]reservationPayload, 0, len(in.Reservations))
	for _, r := range in.Reservations {
		reservations = append(reservations, reservationPayload{
			ReservationID:      r.ReservationID.String(),
			InventoryItemID:    r.InventoryItemID.String(),
			LineID:             r.LineID.String(),
			SKUID:              r.SKUID.String(),
			SellerID:           r.SellerID.String(),
			Quantity:           r.Quantity,
			ProductName:        r.ProductName,
			VariantDescription: r.VariantDescription,
			UnitPrice:          r.UnitPrice.Amount(),
			LineTotal:          r.LineTotal.Amount(),
			CommissionAmount:   r.CommissionAmount.Amount(),
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
			CheckoutID  string `json:"checkout_id"`
			OrderID     string `json:"order_id"`
			OrderNumber string `json:"order_number"`
			CartID      string `json:"cart_id"`
			CustomerID  string `json:"customer_id"`
			GuestEmail  string `json:"guest_email"`
			GuestPhone  string `json:"guest_phone"`

			// ShippingAddress để SELLER in được phiếu giao hàng.
			//
			// Không có nó thì họ biết nhặt gì mà không biết gửi đi đâu — và
			// họ KHÔNG được mở đơn hàng gốc để tra, vì ở đó có cả hàng của
			// seller khác lẫn email khách.
			ShippingAddress addressPayload `json:"shipping_address"`

			Currency     string               `json:"currency"`
			Reservations []reservationPayload `json:"reservations"`
		}{
			CheckoutID:  in.CheckoutID.String(),
			OrderID:     in.OrderID.String(),
			OrderNumber: in.OrderNumber,
			CartID:      in.CartID.String(),
			CustomerID:  in.CustomerID.String(),
			GuestEmail:  in.GuestEmail,
			GuestPhone:  in.GuestPhone,
			ShippingAddress: addressPayload{
				RecipientName: in.ShippingAddress.RecipientName,
				Phone:         in.ShippingAddress.Phone,
				StreetAddress: in.ShippingAddress.StreetAddress,
				Ward:          in.ShippingAddress.Ward,
				District:      in.ShippingAddress.District,
				Province:      in.ShippingAddress.Province,
				CountryCode:   in.ShippingAddress.CountryCode,
			},
			Currency:     string(in.Currency),
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
