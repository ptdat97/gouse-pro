package analytics

import (
	"context"
	"fmt"

	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// RecordEventsFromBus ghi sự kiện nghiệp vụ vào nhật ký phân tích.
//
// # Đây là mắt xích cuối của kiến trúc event
//
// `analytics` là module DUY NHẤT tồn tại để tiêu thụ event. Nó không phát
// gì, không gọi ai, và không ai gọi nó trong luồng nghiệp vụ:
//
//	checkout.completed ──→ inventory     : Reserved → Committed
//	                  ├──→ fulfillment   : tách đơn theo nguồn hàng
//	                  ├──→ supply-chain  : ghi tín hiệu nhu cầu
//	                  ├──→ notification  : email xác nhận
//	                  └──→ analytics     : GMV, số đơn        ← thêm ở đây
//
// Bên phát KHÔNG BIẾT bên nào nghe mình. Thêm module này vào không sửa một
// dòng nào của `checkout`.
//
// # Vì sao nghe event thay vì để module kia gọi trực tiếp
//
// Nếu `checkout` gọi thẳng `analytics.TrackEvent`, thì một sự cố ở
// analytics sẽ nằm trong đường đặt hàng của khách. Ghi số liệu là việc
// PHỤ; nó không được phép làm hỏng việc CHÍNH.
//
// # Vì sao chạy trong giao dịch của dispatcher
//
// Việc ghi sự kiện và việc đánh dấu event đã xử lý phải cùng thành công
// hoặc cùng thất bại. Tách rời nghĩa là GMV có thể bị cộng hai lần khi
// event được phát lại.
//
// Nhưng CHỐNG TRÙNG THẬT nằm ở chỉ mục UNIQUE (event_id, event_name) —
// giao dịch chỉ thu hẹp cửa sổ, không đóng nó.
type RecordEventsFromBus struct {
	module *Module
}

// NewEventRecorder tạo bên nhận ghi sự kiện nghiệp vụ.
func NewEventRecorder(m *Module) *RecordEventsFromBus {
	return &RecordEventsFromBus{module: m}
}

var _ eventbus.Handler = (*RecordEventsFromBus)(nil)

func (h *RecordEventsFromBus) Name() string {
	return "analytics.record_business_events"
}

func (h *RecordEventsFromBus) EventTypes() []string {
	return []string{
		eventbus.TypeCheckoutCompleted,
		eventbus.TypeCartItemAdded,
		eventbus.TypeFulfillmentProgress,
	}
}

// checkoutCompletedPayload là phần dữ liệu cần từ event đặt hàng.
//
// Chỉ khai báo những trường THẬT SỰ DÙNG: khai báo thừa tạo cảm giác
// module này phụ thuộc vào nhiều thứ hơn thực tế, và khi bên phát bỏ một
// trường thì không rõ có phá gì không.
type checkoutCompletedPayload struct {
	OrderID      string `json:"order_id"`
	CustomerID   string `json:"customer_id"`
	CheckoutID   string `json:"checkout_id"`
	Currency     string `json:"currency"`
	Reservations []struct {
		SKUID     string `json:"sku_id"`
		SellerID  string `json:"seller_id"`
		Quantity  int    `json:"quantity"`
		LineTotal int64  `json:"line_total"`
	} `json:"reservations"`
}

// cartItemAddedPayload là phần dữ liệu cần từ event thêm giỏ.
//
// KHÔNG có product_id: giỏ hàng làm việc với SKU, không với sản phẩm. Ai
// cần nhóm theo sản phẩm phải tự tra — và đó là việc của tầng đọc, không
// phải của đường ghi.
type cartItemAddedPayload struct {
	CartID   string `json:"cart_id"`
	SKUID    string `json:"sku_id"`
	SellerID string `json:"seller_id"`
	Quantity int    `json:"quantity"`
}

// fulfillmentProgressPayload là phần dữ liệu cần từ event tiến độ.
//
// LƯU Ý tên trường: bên phát dùng `new_status`, không phải `status`. Đọc
// sai tên thì JSON không lỗi — nó chỉ trả về chuỗi rỗng, và mọi mốc giao
// hàng lặng lẽ bị bỏ qua.
//
// KHÔNG có seller_id trong payload này: một đơn thực hiện thuộc về đúng
// một gian hàng, nhưng bên phát chưa mang nó theo. Hệ quả được ghi rõ ở
// handleFulfillmentProgress.
type fulfillmentProgressPayload struct {
	OrderID       string `json:"order_id"`
	FulfillmentID string `json:"fulfillment_id"`
	NewStatus     string `json:"new_status"`
}

// Handle ghi sự kiện tương ứng với loại event.
func (h *RecordEventsFromBus) Handle(ctx context.Context, e eventbus.Event) error {
	switch e.Type {
	case eventbus.TypeCheckoutCompleted:
		return h.handleCheckoutCompleted(ctx, e)
	case eventbus.TypeCartItemAdded:
		return h.handleCartItemAdded(ctx, e)
	case eventbus.TypeFulfillmentProgress:
		return h.handleFulfillmentProgress(ctx, e)
	}
	// Loại event không quan tâm: không phải lỗi.
	return nil
}

// handleCheckoutCompleted ghi MỘT sự kiện order.placed cho MỖI gian hàng.
//
// # Vì sao tách theo seller chứ không ghi một sự kiện cho cả đơn
//
// Một đơn ba dòng từ hai gian hàng phải cộng vào GMV của ĐÚNG hai gian
// hàng đó. Ghi một sự kiện mang tổng đơn thì dashboard của seller sẽ hiện
// cả doanh số của đối thủ — và không có cách nào tách ra sau.
//
// # Vì sao EventID phải KHÁC nhau cho mỗi seller
//
// Chỉ mục chống trùng là (event_id, event_name). Nếu hai sự kiện của cùng
// một đơn dùng chung event_id, sự kiện thứ hai bị coi là bản trùng và bị
// bỏ — GMV của gian hàng thứ hai biến mất.
//
// Ghép id event với id gian hàng giữ được CẢ HAI tính chất: khác nhau
// giữa các seller, và giống nhau khi event được phát lại.
func (h *RecordEventsFromBus) handleCheckoutCompleted(
	ctx context.Context, e eventbus.Event,
) error {
	var p checkoutCompletedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event đặt hàng: %w", err)
	}

	// Gom theo gian hàng TRƯỚC khi ghi: một đơn ba dòng cùng gian hàng là
	// MỘT đơn hàng của gian hàng đó, không phải ba.
	//
	// Không gom thì số đơn bị thổi lên theo số dòng, và AOV — vốn là
	// GMV chia số đơn — thấp đi đúng theo tỷ lệ đó.
	type sellerTotal struct {
		amount int64
		items  int
	}
	bySeller := make(map[string]*sellerTotal)
	order := make([]string, 0, len(p.Reservations))

	for _, r := range p.Reservations {
		got, ok := bySeller[r.SellerID]
		if !ok {
			got = &sellerTotal{}
			bySeller[r.SellerID] = got
			order = append(order, r.SellerID)
		}
		got.amount += r.LineTotal
		got.items += r.Quantity
	}

	events := make([]EventInput, 0, len(bySeller))
	for _, sellerID := range order {
		total := bySeller[sellerID]
		amount := total.amount

		events = append(events, EventInput{
			Name:     EventOrderPlaced,
			Category: CategoryBusiness,

			// Ghép id gian hàng vào để mỗi seller có một khóa riêng —
			// xem ghi chú ở đầu hàm.
			EventID: e.ID.String() + ":" + sellerID,

			CustomerID: p.CustomerID,

			// SessionID dùng CheckoutID: nó nối các sự kiện của cùng một
			// lượt mua, và đó chính là thứ tỷ lệ chuyển đổi cần.
			SessionID: p.CheckoutID,

			SubjectType: "order",
			SubjectID:   p.OrderID,
			SellerID:    sellerID,

			Amount:   &amount,
			Currency: p.Currency,

			Properties: map[string]any{
				"item_count": total.items,
			},

			OccurredAt: e.OccurredAt,
		})
	}

	if len(events) == 0 {
		return nil
	}

	_, err := h.module.TrackBatch(ctx, events)
	return err
}

// handleCartItemAdded ghi sự kiện thêm giỏ.
//
// Đây là một bước của PHỄU CHUYỂN ĐỔI. Đo tổng thể chỉ cho biết CÓ vấn
// đề; đo từng bước cho biết vấn đề Ở ĐÂU.
func (h *RecordEventsFromBus) handleCartItemAdded(
	ctx context.Context, e eventbus.Event,
) error {
	var p cartItemAddedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event thêm giỏ: %w", err)
	}

	return h.module.TrackEvent(ctx, EventInput{
		Name:     EventAddToCart,
		Category: CategoryBusiness,
		EventID:  e.ID.String(),

		// SessionID dùng id giỏ hàng: một giỏ là MỘT lượt mua sắm, và đó
		// chính là đơn vị mà tỷ lệ chuyển đổi đếm.
		SessionID: p.CartID,

		// Đối tượng là SKU, không phải sản phẩm — xem ghi chú ở
		// cartItemAddedPayload.
		SubjectType: "sku",
		SubjectID:   p.SKUID,

		SellerID: p.SellerID,

		Properties: map[string]any{
			"quantity": p.Quantity,
		},

		OccurredAt: e.OccurredAt,
	})
}

// handleFulfillmentProgress ghi mốc GIAO HÀNG THÀNH CÔNG.
//
// # Chỉ ghi DELIVERED, bỏ qua các bước còn lại
//
// Vòng đời giao hàng có chín trạng thái, phần lớn là bước nội bộ của
// seller (PICKING, PACKED). Ghi hết vào analytics là nhồi nhật ký bằng dữ
// liệu không ai hỏi tới — và khối lượng đó phải trả giá ở mọi truy vấn
// chỉ số sau này.
//
// DELIVERED là mốc DUY NHẤT có nghĩa với người đọc số liệu: nó là mẫu số
// của tỷ lệ hoàn hàng và là mốc bắt đầu đếm bảy ngày trước khi seller
// được chi trả.
func (h *RecordEventsFromBus) handleFulfillmentProgress(
	ctx context.Context, e eventbus.Event,
) error {
	var p fulfillmentProgressPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event tiến độ giao hàng: %w", err)
	}

	if p.NewStatus != statusDelivered {
		return nil
	}

	// SellerID ĐỂ TRỐNG có chủ ý.
	//
	// Payload `fulfillment.progress_changed` chưa mang seller_id. Ba lựa
	// chọn, và lý do chọn cái thứ ba:
	//
	//	1. Gọi fulfillment để tra   → vi phạm quy tắc 1, và một sự cố ở
	//	                              fulfillment làm hỏng việc ghi số liệu
	//	2. Bổ sung vào payload      → đúng nhất, nhưng sửa module khác
	//	3. Để trống, ghi rõ hệ quả  ← CHỌN
	//
	// HỆ QUẢ: mốc giao hàng chỉ đếm được ở mức TOÀN SÀN, chưa cắt lát
	// theo gian hàng. GMV và số đơn KHÔNG bị ảnh hưởng — chúng đến từ
	// `checkout.completed`, nơi seller_id có đầy đủ.
	//
	// Khi cần chỉ số giao hàng theo seller (dashboard seller, Phase 2),
	// việc đúng là bổ sung seller_id vào payload — một trường thêm vào
	// event không phá bên nhận nào đang chạy.
	return h.module.TrackEvent(ctx, EventInput{
		Name:     EventDelivered,
		Category: CategoryBusiness,
		EventID:  e.ID.String(),

		SubjectType: "order",
		SubjectID:   p.OrderID,

		Properties: map[string]any{
			"fulfillment_id": p.FulfillmentID,
		},

		OccurredAt: e.OccurredAt,
	})
}

// statusDelivered là trạng thái "đã giao" của module fulfillment.
//
// KHAI BÁO LẠI CHUỖI thay vì import fulfillment: analytics KHÔNG được phụ
// thuộc module nghiệp vụ nào (quy tắc 1). Đây là cái giá của ranh giới —
// và nó rẻ hơn việc một sự cố ở fulfillment làm hỏng việc ghi số liệu.
const statusDelivered = "DELIVERED"
