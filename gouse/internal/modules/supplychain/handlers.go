package supplychain

import (
	"context"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// RecordSignalsFromEvents ghi tín hiệu nhu cầu từ các sự kiện nghiệp vụ.
//
// ĐÂY LÀ MẮT XÍCH HOÀN THÀNH BÁNH ĐÀ: dữ liệu hành vi của khách chảy ngược
// vào việc lập kế hoạch sản xuất.
//
//	Khách thêm giỏ / đặt hàng
//	    ↓  (domain event)
//	demand_signal
//	    ↓  (Phase 3)
//	Dự báo → Kế hoạch sản xuất → Đặt hàng nhà cung cấp
//
// # Vì sao nghe event thay vì để module kia gọi trực tiếp
//
// Nếu `cart` gọi thẳng `supplychain.RecordSignal`, thì `cart` phụ thuộc
// `supplychain` — và một lỗi khi ghi tín hiệu sẽ làm hỏng việc thêm giỏ
// hàng của khách. Ghi tín hiệu là việc PHỤ; nó không được phép làm hỏng
// việc CHÍNH.
//
// Với event, `cart` không biết module này tồn tại.
//
// # Vì sao chạy trong giao dịch của dispatcher
//
// Việc ghi tín hiệu và việc đánh dấu event đã xử lý phải cùng thành công
// hoặc cùng thất bại. Tách rời nghĩa là tín hiệu có thể bị ghi hai lần khi
// event được phát lại — và số liệu nhu cầu bị thổi phồng.
type RecordSignalsFromEvents struct {
	module *Module
}

// NewSignalHandler tạo bên nhận ghi tín hiệu nhu cầu.
func NewSignalHandler(m *Module) *RecordSignalsFromEvents {
	return &RecordSignalsFromEvents{module: m}
}

var _ eventbus.Handler = (*RecordSignalsFromEvents)(nil)

func (h *RecordSignalsFromEvents) Name() string {
	return "supplychain.record_demand_signals"
}

func (h *RecordSignalsFromEvents) EventTypes() []string {
	return []string{
		eventbus.TypeCartItemAdded,
		eventbus.TypeCheckoutCompleted,
	}
}

// cartItemAddedPayload là phần dữ liệu cần từ event thêm giỏ.
type cartItemAddedPayload struct {
	SKUID     string `json:"sku_id"`
	ProductID string `json:"product_id"`
	OfferID   string `json:"offer_id"`
	Quantity  int    `json:"quantity"`

	// Nguồn giới thiệu: nội dung nào dẫn tới việc thêm giỏ.
	//
	// Ghi vào metadata để Phase 3 trả lời được "nội dung nào tạo nhu cầu
	// thật, không chỉ tạo lượt xem".
	SourceContentID string `json:"source_content_id"`
	SourceCreatorID string `json:"source_creator_id"`
}

// checkoutCompletedPayload là phần dữ liệu cần từ event đặt hàng.
type checkoutCompletedPayload struct {
	OrderID      string `json:"order_id"`
	Reservations []struct {
		SKUID    string `json:"sku_id"`
		SellerID string `json:"seller_id"`
		Quantity int    `json:"quantity"`
	} `json:"reservations"`
}

// Handle ghi tín hiệu tương ứng với loại event.
func (h *RecordSignalsFromEvents) Handle(ctx context.Context, e eventbus.Event) error {
	switch e.Type {
	case eventbus.TypeCartItemAdded:
		return h.handleCartItemAdded(ctx, e)
	case eventbus.TypeCheckoutCompleted:
		return h.handleOrderPlaced(ctx, e)
	}
	// Loại event không quan tâm: không phải lỗi.
	return nil
}

// handleCartItemAdded ghi tín hiệu ADD_TO_CART.
//
// Đây là tín hiệu MẠNH HƠN LƯỢT XEM rất nhiều: khách đã quyết định muốn
// món này, chỉ chưa trả tiền. Tỷ lệ "thêm giỏ nhưng không mua" cũng là dữ
// liệu quý — nó chỉ ra sản phẩm có nhu cầu nhưng vướng ở giá hoặc phí ship.
func (h *RecordSignalsFromEvents) handleCartItemAdded(
	ctx context.Context, e eventbus.Event,
) error {
	var p cartItemAddedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event thêm giỏ: %w", err)
	}

	meta := map[string]string{}
	if p.OfferID != "" {
		meta["offer_id"] = p.OfferID
	}
	if p.SourceContentID != "" {
		meta["source_content_id"] = p.SourceContentID
	}
	if p.SourceCreatorID != "" {
		meta["source_creator_id"] = p.SourceCreatorID
	}

	return h.module.RecordSignal(ctx, SignalRequest{
		Type:       SignalAddToCart,
		SKUID:      p.SKUID,
		ProductID:  p.ProductID,
		Quantity:   p.Quantity,
		OccurredAt: e.OccurredAt.Format(time.RFC3339),
		SourceType: "cart",
		SourceID:   e.AggregateID.String(),
		Metadata:   meta,
	})
}

// handleOrderPlaced ghi tín hiệu ORDER cho từng dòng hàng.
//
// Đây là tín hiệu CHẮC CHẮN NHẤT — khách đã trả tiền. Nhưng nó KHÔNG đủ
// một mình: nếu chỉ nhìn đơn hàng, hệ thống sẽ liên tục sản xuất thiếu
// đúng những mặt hàng bán chạy (chúng hết hàng sớm nên số đơn thấp hơn
// nhu cầu thật). Vì vậy nó phải đi cùng STOCKOUT và SEARCH_NO_RESULT.
func (h *RecordSignalsFromEvents) handleOrderPlaced(
	ctx context.Context, e eventbus.Event,
) error {
	var p checkoutCompletedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event đặt hàng: %w", err)
	}

	occurredAt := e.OccurredAt.Format(time.RFC3339)

	// Gom thành MỘT lượt ghi: đơn ba dòng sinh ba tín hiệu, và ghi từng
	// cái là ba lượt đi database cho một sự kiện.
	reqs := make([]SignalRequest, 0, len(p.Reservations))
	for _, r := range p.Reservations {
		if r.SKUID == "" {
			continue
		}

		meta := map[string]string{}
		if r.SellerID != "" {
			meta["seller_id"] = r.SellerID
		}

		reqs = append(reqs, SignalRequest{
			Type:       SignalOrder,
			SKUID:      r.SKUID,
			Quantity:   r.Quantity,
			OccurredAt: occurredAt,
			SourceType: "order",
			SourceID:   p.OrderID,
			Metadata:   meta,
		})
	}

	return h.module.RecordSignals(ctx, reqs)
}
