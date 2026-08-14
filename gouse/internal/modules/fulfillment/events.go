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
			OrderID       string         `json:"order_id"`
			FulfillmentID string         `json:"fulfillment_id"`
			NewStatus     string         `json:"new_status"`
			Progress      []lineProgress `json:"progress"`
		}{
			OrderID:       in.OrderID.String(),
			FulfillmentID: in.FulfillmentID.String(),
			NewStatus:     in.NewStatus,
			Progress:      progress,
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
	OrderID      string `json:"order_id"`
	OrderNumber  string `json:"order_number"`
	Currency     string `json:"currency"`
	Reservations []struct {
		LineID           string `json:"line_id"`
		SKUID            string `json:"sku_id"`
		SellerID         string `json:"seller_id"`
		Quantity         int    `json:"quantity"`
		LineTotal        int64  `json:"line_total"`
		CommissionAmount int64  `json:"commission_amount"`
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

		in.Lines = append(in.Lines, domain.SplitLine{
			LineID:           ids.ID(r.LineID),
			SellerID:         ids.ID(r.SellerID),
			SKUID:            ids.ID(r.SKUID),
			Quantity:         r.Quantity,
			LineTotal:        lineTotal,
			CommissionAmount: commission,
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
