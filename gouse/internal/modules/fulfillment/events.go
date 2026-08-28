package fulfillment

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/application"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// ---------------------------------------------------------------- Phát

// eventPublisher nối cổng ra của tầng application với outbox.
type eventPublisher struct {
	outbox *eventbus.Outbox
}

var _ application.EventPublisher = (*eventPublisher)(nil)

// NewEventPublisher tạo bộ phát event nối với outbox.
func NewEventPublisher(outbox *eventbus.Outbox) application.EventPublisher {
	return &eventPublisher{outbox: outbox}
}

// PublishProgress phát tiến độ của toàn bộ đơn hàng.
//
// Module order nghe event này để tính lại trạng thái tổng hợp. Payload chứa
// tiến độ MỌI nguồn hàng, không chỉ nguồn vừa đổi — nhờ vậy order tính
// được ngay mà không phải hỏi ngược (ADR-0007).
func (p *eventPublisher) PublishProgress(
	ctx context.Context, in application.ProgressChanged,
) error {
	type lineProgress struct {
		Cancelled bool `json:"cancelled"`
		Delivered bool `json:"delivered"`
		Shipped   bool `json:"shipped"`
	}

	progress := make([]lineProgress, 0, len(in.Progress))
	for _, pr := range in.Progress {
		progress = append(progress, lineProgress{
			Cancelled: pr.Cancelled,
			Delivered: pr.Delivered,
			Shipped:   pr.Shipped,
		})
	}

	e, err := eventbus.NewEvent(
		eventbus.TypeFulfillmentProgress,
		eventbus.AggregateFulfillment,
		in.FulfillmentID,
		struct {
			OrderID        string `json:"order_id"`
			FulfillmentID  string `json:"fulfillment_id"`
			FONumber       string `json:"fo_number"`
			NewStatus      string `json:"new_status"`
			TrackingNumber string `json:"tracking_number"`

			// Địa chỉ liên hệ, để notification gửi được mà không gọi ngược.
			CustomerID string `json:"customer_id"`
			Email      string `json:"email"`
			Phone      string `json:"phone"`

			Progress []lineProgress `json:"progress"`
		}{
			OrderID:        in.OrderID.String(),
			FulfillmentID:  in.FulfillmentID.String(),
			FONumber:       in.FONumber,
			NewStatus:      in.NewStatus,
			TrackingNumber: in.TrackingNumber,
			CustomerID:     in.CustomerID.String(),
			Email:          in.Email,
			Phone:          in.Phone,
			Progress:       progress,
		})
	if err != nil {
		return err
	}

	// CorrelationID là mã đơn: mọi việc xảy ra sau khi đặt hàng đều truy
	// ngược được về một đơn cụ thể.
	e = e.WithTrace(in.OrderID.String(), "")

	// Phát bằng giao dịch riêng: bước chuyển trạng thái đã ghi xong trước
	// đó, và một event thất bại KHÔNG được làm hỏng việc seller vừa làm.
	//
	// Đánh đổi: có khoảng thời gian trạng thái đã đổi mà event chưa ghi.
	// Chấp nhận được vì hệ quả chỉ là trạng thái tổng hợp hiển thị chậm —
	// khác với trường hợp tồn kho, nơi mất event là mất hàng.
	return p.outbox.Publish(ctx, e)
}

// ---------------------------------------------------------------- Nhận

// SplitOnCheckoutCompleted tách đơn hàng thành các đơn thực hiện.
//
//	Giỏ hàng:
//	├── Áo own brand   (kho nền tảng, Hà Nội)
//	├── Giày Seller A  (kho seller A, TP.HCM)
//	└── Túi Seller B   (kho seller B, Đà Nẵng)
//
//	Ba món KHÔNG THỂ đóng chung một gói.
//
// # Vì sao tách ở đây chứ không ở module order
//
// Đơn thực hiện là dữ liệu VẬN HÀNH của seller, không thuộc hợp đồng với
// khách. Đặt việc tách trong module order sẽ buộc order phụ thuộc
// fulfillment, và tạo phụ thuộc vòng vì fulfillment đã trỏ tới order qua
// `order_id`.
//
// # Vì sao nghe checkout.completed
//
// Payload của event này chứa đủ dữ liệu để tách: SKU, seller, số lượng và
// tiền của từng dòng. Nghe `order.placed` sẽ phải gọi ngược module order
// để lấy chi tiết — đúng thứ kiến trúc event sinh ra để tránh.
type SplitOnCheckoutCompleted struct {
	module *Module
	log    *slog.Logger
}

// NewSplitHandler tạo bên nhận tách đơn.
func NewSplitHandler(m *Module, log *slog.Logger) *SplitOnCheckoutCompleted {
	return &SplitOnCheckoutCompleted{module: m, log: log}
}

var _ eventbus.Handler = (*SplitOnCheckoutCompleted)(nil)

func (h *SplitOnCheckoutCompleted) Name() string {
	return "fulfillment.split_on_checkout_completed"
}

func (h *SplitOnCheckoutCompleted) EventTypes() []string {
	return []string{eventbus.TypeCheckoutCompleted}
}

// splitPayload là phần dữ liệu bên nhận này cần.
type splitPayload struct {
	OrderID     string `json:"order_id"`
	OrderNumber string `json:"order_number"`
	Currency    string `json:"currency"`

	// Thông tin liên hệ, sao chép xuống đơn thực hiện để event phát từ
	// module này mang theo được — notification không gọi ngược để lấy.
	CustomerID string `json:"customer_id"`
	GuestEmail string `json:"guest_email"`
	GuestPhone string `json:"guest_phone"`

	// ShippingAddress là nơi hàng phải đến — SELLER cần để in phiếu giao.
	ShippingAddress struct {
		RecipientName string `json:"recipient_name"`
		Phone         string `json:"phone"`
		StreetAddress string `json:"street_address"`
		Ward          string `json:"ward"`
		District      string `json:"district"`
		Province      string `json:"province"`
		CountryCode   string `json:"country_code"`
	} `json:"shipping_address"`
	Reservations []struct {
		LineID   string `json:"line_id"`
		SKUID    string `json:"sku_id"`
		SellerID string `json:"seller_id"`
		Quantity int    `json:"quantity"`

		// ProductName và VariantDescription là thứ SELLER cần để NHẶT ĐÚNG
		// hàng. Với thời trang, tên sản phẩm không đủ: cùng một chiếc áo có
		// năm size nằm ở năm ô kệ khác nhau.
		//
		// Sao chép xuống đơn thực hiện chứ không tra ngược module order:
		// seller KHÔNG được phép xem đơn hàng gốc.
		ProductName        string `json:"product_name"`
		VariantDescription string `json:"variant_description"`

		UnitPrice        int64 `json:"unit_price"`
		LineTotal        int64 `json:"line_total"`
		CommissionAmount int64 `json:"commission_amount"`
	} `json:"reservations"`
}

// Handle tách đơn hàng thành các đơn thực hiện theo nguồn hàng.
//
// IDEMPOTENT: tầng application kiểm tra đơn đã tách chưa trước khi tạo.
// Event có thể được phát lại, và tách hai lần nghĩa là seller thấy việc
// trùng — có thể giao hàng hai lần cho một đơn.
func (h *SplitOnCheckoutCompleted) Handle(ctx context.Context, e eventbus.Event) error {
	var p splitPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}

	if len(p.Reservations) == 0 {
		// Đơn không có dòng hàng nào giữ được: không có gì để tách. Không
		// phải lỗi — trả nil để event không kẹt trong hàng đợi.
		h.log.Warn("đơn không có dòng hàng để tách", "order_id", p.OrderID)
		return nil
	}

	currency := money.Currency(p.Currency)
	if currency == "" {
		currency = money.VND
	}

	in := domain.SplitInput{
		OrderID:     ids.ID(p.OrderID),
		OrderNumber: p.OrderNumber,
		Currency:    currency,
		CustomerID:  ids.ID(p.CustomerID),
		NotifyEmail: p.GuestEmail,
		NotifyPhone: p.GuestPhone,
		ShippingAddress: domain.ShippingAddress{
			RecipientName: p.ShippingAddress.RecipientName,
			Phone:         p.ShippingAddress.Phone,
			StreetAddress: p.ShippingAddress.StreetAddress,
			Ward:          p.ShippingAddress.Ward,
			District:      p.ShippingAddress.District,
			Province:      p.ShippingAddress.Province,
			CountryCode:   p.ShippingAddress.CountryCode,
		},
	}

	for _, r := range p.Reservations {
		lineTotal, err := money.New(r.LineTotal, currency)
		if err != nil {
			return fmt.Errorf("tiền hàng của dòng %s: %w", r.LineID, err)
		}
		commission, err := money.New(r.CommissionAmount, currency)
		if err != nil {
			return fmt.Errorf("hoa hồng của dòng %s: %w", r.LineID, err)
		}

		// Đơn giá có thể bằng 0 với event phát TRƯỚC khi payload mang
		// trường này. Không chặn tách đơn vì chuyện đó: thiếu một con số
		// hiển thị không đáng để cả đơn kẹt lại trong hàng đợi.
		unitPrice, err := money.New(r.UnitPrice, currency)
		if err != nil {
			unitPrice = money.Zero(currency)
		}

		in.Lines = append(in.Lines, domain.SplitLine{
			LineID:             ids.ID(r.LineID),
			SellerID:           ids.ID(r.SellerID),
			SKUID:              ids.ID(r.SKUID),
			Quantity:           r.Quantity,
			ProductName:        r.ProductName,
			VariantDescription: r.VariantDescription,
			UnitPrice:          unitPrice,
			LineTotal:          lineTotal,
			CommissionAmount:   commission,
		})
	}

	fos, err := h.module.svc.SplitOrder(ctx, in)
	if err != nil {
		return fmt.Errorf("tách đơn %s: %w", p.OrderID, err)
	}

	h.log.Info("đã tách đơn thành các đơn thực hiện",
		"order_id", p.OrderID,
		"số_nguồn_hàng", len(fos))
	return nil
}

// PublishCancelled phát event hủy đơn thực hiện, kèm DÒNG HÀNG.
//
// # Vì sao là event RIÊNG chứ không phải mở rộng fulfillment.progress
//
// Hai event trả lời hai câu khác nhau. `progress` nói "tiến độ đổi rồi,
// tính lại trạng thái đơn" và có ba bên nghe (order, notification,
// analytics). Event này nói "những món cụ thể này không đi nữa, trả về
// kho" và chỉ inventory cần.
//
// Nhồi dòng hàng vào `progress` sẽ bắt ba bên kia tải dữ liệu họ không
// dùng — và quan trọng hơn, ĐỔI một payload đang chạy đòi hỏi triển khai
// bên nhận trước bên phát (domain-events.md mục 8.1). Ngày 19/08 đã có sự
// cố đúng kiểu đó: worker cũ nuốt event mới rồi bỏ qua trường mới trong
// im lặng. Thêm event MỚI thì bên nhận cũ không bị ảnh hưởng gì.
func (p *eventPublisher) PublishCancelled(
	ctx context.Context, in application.FulfillmentCancelled,
) error {
	type cancelledLine struct {
		SKUID    string `json:"sku_id"`
		Quantity int    `json:"quantity"`
	}

	lines := make([]cancelledLine, 0, len(in.Lines))
	for _, l := range in.Lines {
		lines = append(lines, cancelledLine{
			SKUID:    l.SKUID.String(),
			Quantity: l.Quantity,
		})
	}

	e, err := eventbus.NewEvent(
		eventbus.TypeFulfillmentCancelled,
		eventbus.AggregateFulfillment,
		in.FulfillmentID,
		struct {
			OrderID       string `json:"order_id"`
			FulfillmentID string `json:"fulfillment_id"`
			FONumber      string `json:"fo_number"`

			// SellerID chứ KHÔNG phải inventory_owner_id.
			//
			// Chủ sở hữu tồn kho suy ra từ nhà bán, và quy tắc suy ra
			// thuộc module inventory (ADR-0012). Tự tính ở đây là cài
			// quy tắc lần thứ hai — đúng lỗi P3-18.
			SellerID string `json:"seller_id"`

			StockLocationID string `json:"stock_location_id,omitempty"`

			// ReleaseStock: hàng còn trong kho hay đang trên đường trả về.
			ReleaseStock bool `json:"release_stock"`

			Lines []cancelledLine `json:"lines"`
		}{
			OrderID:         in.OrderID.String(),
			FulfillmentID:   in.FulfillmentID.String(),
			FONumber:        in.FONumber,
			SellerID:        in.SellerID.String(),
			StockLocationID: in.StockLocationID.String(),
			ReleaseStock:    in.ReleaseStock,
			Lines:           lines,
		})
	if err != nil {
		return err
	}

	e = e.WithTrace(in.OrderID.String(), "")
	return p.outbox.Publish(ctx, e)
}

// PublishCompleted phát tín hiệu tài chính "đơn thực hiện đã qua hạn đổi trả".
//
// Payload mang SẴN số tiền phải trả nhà bán. Bên nhận (payment) không phải
// gọi ngược fulfillment để hỏi — hai module nhận event của nhau mà còn hỏi
// nhau đồng bộ thì cái lợi của event mất sạch.
func (p *eventPublisher) PublishCompleted(
	ctx context.Context, in application.FulfillmentCompleted,
) error {
	e, err := eventbus.NewEvent(
		eventbus.TypeFulfillmentCompleted,
		eventbus.AggregateFulfillment,
		in.FulfillmentID,
		struct {
			FulfillmentID string `json:"fulfillment_id"`
			OrderID       string `json:"order_id"`
			SellerID      string `json:"seller_id"`

			SellerPayable int64  `json:"seller_payable"`
			Currency      string `json:"currency"`
		}{
			FulfillmentID: in.FulfillmentID.String(),
			OrderID:       in.OrderID.String(),
			SellerID:      in.SellerID.String(),
			SellerPayable: in.SellerPayable.Amount(),
			Currency:      string(in.SellerPayable.Currency()),
		},
	)
	if err != nil {
		return err
	}
	return p.outbox.Publish(ctx, e)
}
