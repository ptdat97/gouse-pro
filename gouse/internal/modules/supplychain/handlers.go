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

// MaxEventVersion khai bên nhận này THEO KỊP phiên bản mới nhất của
// `checkout.completed` — con số nằm ngay dưới, không nhắc lại ở đây.
//
// Bên nhận này không đọc trường nào mà các phiên bản sau thêm vào. Nó vẫn
// phải khai vì ADR-0016 bắt MỌI bên nhận của một event nói rõ mình hiểu
// tới đâu: thiếu một khai báo là dispatcher HOÃN event cho TẤT CẢ. Đó là
// cái giá đã chấp nhận khi chọn cơ chế hoãn thay vì thả cho bên nhận đọc
// thiếu trường — nên mỗi lần nâng phiên bản, con số dưới đây phải đổi,
// còn đoạn văn này thì không.
func (h *RecordSignalsFromEvents) MaxEventVersion(eventType string) int {
	if eventType == eventbus.TypeCheckoutCompleted {
		return 9
	}
	if eventType == eventbus.TypeCartItemAdded {
		// v2 thêm `visit_id` — mã lượt truy cập, thay cho mã giỏ ở
		// trường phiên (ADR-0020).
		return 2
	}
	return eventbus.DefaultMaxEventVersion
}

func (h *RecordSignalsFromEvents) EventTypes() []string {
	return []string{
		eventbus.TypeCartItemAdded,
		eventbus.TypeCheckoutCompleted,
		eventbus.TypeSearchNoResult,
		eventbus.TypeSearchPerformed,

		// HẾT HÀNG — tín hiệu quý thứ hai, và trước đây KHÔNG ai phát.
		//
		// Ba tín hiệu module này tồn tại để thu là SEARCH_NO_RESULT,
		// STOCKOUT và NOTIFY_REQUEST. Chỉ cái đầu có bên phát; hai cái
		// còn lại được khai trong domain và không dòng mã nào tạo ra.
		eventbus.TypeInventoryDepleted,

		// TRẢ HÀNG kèm lý do — dữ liệu CHẤT LƯỢNG của thời trang.
		eventbus.TypeReturnRequested,

		// YÊU THÍCH — ý định mua rõ ràng, và khi kèm "báo khi có hàng" thì
		// là lời hứa "có hàng là tôi mua".
		eventbus.TypeWishlistItemAdded,
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
	case eventbus.TypeSearchNoResult:
		return h.handleSearchNoResult(ctx, e)
	case eventbus.TypeSearchPerformed:
		return h.handleSearchPerformed(ctx, e)
	case eventbus.TypeInventoryDepleted:
		return h.handleHetHang(ctx, e)
	case eventbus.TypeReturnRequested:
		return h.handleTraHang(ctx, e)
	case eventbus.TypeWishlistItemAdded:
		return h.handleYeuThich(ctx, e)
	}
	// Loại event không quan tâm: không phải lỗi.
	return nil
}

// searchNoResultPayload là dữ liệu từ event tìm kiếm không ra kết quả.
type searchNoResultPayload struct {
	Query string `json:"query"`
}

// handleSearchNoResult ghi tín hiệu SEARCH_NO_RESULT.
//
// # Tín hiệu KHÔNG có SKU, và đó chính là ý nghĩa của nó
//
// Mọi tín hiệu khác đều trỏ tới một mặt hàng có thật. Cái này thì không —
// khách tìm một thứ mà nền tảng KHÔNG CÓ. Đó là lý do nó quý: dữ liệu bán
// hàng chỉ nói được về những gì đã bày bán.
//
// Từ khóa lưu ở `SearchTerm` — trường được thiết kế sẵn cho việc này — để
// Phase 3 gom nhóm và xếp hạng nhu cầu chưa đáp ứng. "Áo khoác dạ oversize"
// xuất hiện 240 lần trong 30 ngày là một đề xuất sản phẩm, không phải một
// dòng log.
func (h *RecordSignalsFromEvents) handleSearchNoResult(
	ctx context.Context, e eventbus.Event,
) error {
	var p searchNoResultPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event tìm kiếm: %w", err)
	}
	if p.Query == "" {
		return nil
	}

	return h.module.RecordSignal(ctx, SignalRequest{
		Type:       SignalSearchNoResult,
		SearchTerm: p.Query,
		Quantity:   1,
	})
}

// searchPerformedPayload là dữ liệu từ event tìm kiếm có kết quả.
type searchPerformedPayload struct {
	Query    string `json:"query"`
	SoKetQua int    `json:"so_ket_qua"`
}

// handleSearchPerformed ghi tín hiệu SEARCH.
//
// # Vì sao cần, khi đã có SEARCH_NO_RESULT
//
// Hai tín hiệu trả lời hai câu khác nhau, và thiếu cái nào cũng đọc sai
// cái kia. Chỉ có vế "không ra kết quả" thì "áo khoác dạ" 240 lượt trông
// như một cơ hội lớn — mà không biết "áo sơ mi" được tìm 24.000 lượt, tức
// không có gì để so.
//
// # Quantity là 1, KHÔNG phải số kết quả
//
// Tín hiệu đếm NHU CẦU, và một lượt tìm là một lần khách hỏi. Số kết quả
// đo độ phủ của danh mục, không đo nhu cầu; nhân nó vào số lượng sẽ khiến
// từ khóa nào danh mục đã phục vụ tốt nhất trông như nhu cầu lớn nhất —
// đúng ngược thứ tín hiệu này tồn tại để tìm.
func (h *RecordSignalsFromEvents) handleSearchPerformed(
	ctx context.Context, e eventbus.Event,
) error {
	var p searchPerformedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event tìm kiếm: %w", err)
	}
	if p.Query == "" {
		return nil
	}

	return h.module.RecordSignal(ctx, SignalRequest{
		Type:       SignalSearch,
		SearchTerm: p.Query,
		Quantity:   1,
	})
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

// hetHangPayload là dữ liệu từ event SKU hết sạch hàng.
type hetHangPayload struct {
	SKUID      string `json:"sku_id"`
	LocationID string `json:"stock_location_id"`
	OwnerID    string `json:"inventory_owner_id"`
}

// handleHetHang ghi tín hiệu STOCKOUT.
//
// # Vì sao tín hiệu này quý
//
// Mỗi lần hết hàng là một lần nhu cầu CÓ THẬT bị bỏ lỡ, và nó biến mất
// khỏi mọi báo cáo doanh số: báo cáo chỉ đếm được thứ đã bán.
//
//	Chỉ nhìn doanh số:  "Áo khoác bán 200 chiếc" → nhu cầu là 200
//	Thực tế:            bán 200, HẾT HÀNG từ tuần 3
//
// Không có tín hiệu này thì kế hoạch sản xuất sẽ liên tục làm thiếu đúng
// những mặt hàng bán chạy nhất — sai lầm kinh điển của ngành, và là lý do
// module này tồn tại từ MVP.
//
// # Số lượng là 0, có chủ ý
//
// Hết hàng không đo được BAO NHIÊU người muốn mua tiếp — nó chỉ đánh dấu
// THỜI ĐIỂM nguồn cung dừng. Điền một con số đoán vào đây sẽ làm mọi phép
// tổng hợp sau này cộng phải số bịa.
func (h *RecordSignalsFromEvents) handleHetHang(
	ctx context.Context, e eventbus.Event,
) error {
	var p hetHangPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event hết hàng: %w", err)
	}

	return h.module.RecordSignal(ctx, SignalRequest{
		Type:       SignalStockout,
		SKUID:      p.SKUID,
		Quantity:   0,
		OccurredAt: e.OccurredAt.Format(time.RFC3339),
		SourceType: "inventory_item",
		SourceID:   e.AggregateID.String(),
		Metadata: map[string]string{
			"stock_location_id":  p.LocationID,
			"inventory_owner_id": p.OwnerID,
		},
	})
}

// traHangPayload là dữ liệu từ event khách xin trả hàng.
type traHangPayload struct {
	Lines []struct {
		SKUID    string `json:"sku_id"`
		Quantity int    `json:"quantity"`
		LyDo     string `json:"reason_code"`
	} `json:"lines"`
}

// handleTraHang ghi tín hiệu RETURN cho từng dòng hàng bị trả.
//
// # Vì sao trả hàng là tín hiệu NHU CẦU
//
// Nhìn thoáng thì trả hàng là chi phí, không phải nhu cầu. Nhưng với thời
// trang, LÝ DO hoàn là đầu vào để sửa bảng size và mô tả sản phẩm:
//
//	"size nhỏ" lặp lại trên một mã   → bảng size của mã đó sai
//	"khác mô tả" lặp lại             → ảnh hoặc mô tả đang nói quá
//
// Sửa bảng size rẻ hơn nhiều so với chịu tỷ lệ hoàn cao mãi. Và con số ấy
// chỉ gom được khi lý do được CHUẨN HÓA — "áo bé quá" viết tự do thì không
// cộng được với "chật".
//
// # Một tín hiệu MỖI DÒNG, không phải mỗi yêu cầu
//
// Một yêu cầu có thể trả ba món với ba lý do khác nhau. Ghi một tín hiệu
// cho cả yêu cầu sẽ buộc phải chọn một lý do đại diện, và hai lý do kia
// mất — chính là phần dữ liệu đáng giữ nhất.
func (h *RecordSignalsFromEvents) handleTraHang(
	ctx context.Context, e eventbus.Event,
) error {
	var p traHangPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event trả hàng: %w", err)
	}
	if len(p.Lines) == 0 {
		// Yêu cầu không có dòng nào: không có gì để ghi. Không phải lỗi —
		// trả nil để event không kẹt trong hàng đợi.
		return nil
	}

	reqs := make([]SignalRequest, 0, len(p.Lines))
	for _, d := range p.Lines {
		reqs = append(reqs, SignalRequest{
			Type:       SignalReturn,
			SKUID:      d.SKUID,
			Quantity:   d.Quantity,
			OccurredAt: e.OccurredAt.Format(time.RFC3339),
			SourceType: "return_request",
			SourceID:   e.AggregateID.String(),
			Metadata:   map[string]string{"reason_code": d.LyDo},
		})
	}

	return h.module.RecordSignals(ctx, reqs)
}

// yeuThichPayload là dữ liệu từ event thêm món vào yêu thích.
type yeuThichPayload struct {
	CustomerID       string `json:"customer_id"`
	ProductID        string `json:"product_id"`
	VariantID        string `json:"variant_id"`
	MuonBaoKhiCoHang bool   `json:"notify_when_available"`
}

// handleYeuThich ghi tín hiệu WISHLIST, và NOTIFY_REQUEST nếu khách xin báo.
//
// # HAI tín hiệu từ MỘT hành động, và vì sao không gộp
//
// Thêm vào yêu thích là "tôi muốn món này" — ý định mua rõ ràng, chỉ chưa
// đúng thời điểm. Bật thêm "báo khi có hàng" là một việc KHÁC: khách nói
// "có hàng là tôi mua", và đó là cam kết mạnh hơn hẳn.
//
// Gộp thành một loại tín hiệu sẽ không phân biệt được hai mức cam kết ấy,
// mà chênh lệch giữa chúng chính là thứ quyết định nên sản xuất bao nhiêu.
// `NOTIFY_REQUEST` là một trong BA tín hiệu mà module này tồn tại để thu.
//
// # Vì sao ghi theo SẢN PHẨM, không theo SKU
//
// Khách thường thích cả sản phẩm rồi mới chọn size. `variant_id` có thể
// rỗng, và điền một SKU đoán vào đó sẽ làm mọi phép tổng hợp theo size sai.
// Cột `product_id` của bảng tín hiệu tồn tại đúng cho trường hợp này.
func (h *RecordSignalsFromEvents) handleYeuThich(
	ctx context.Context, e eventbus.Event,
) error {
	var p yeuThichPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event yêu thích: %w", err)
	}

	chung := SignalRequest{
		SKUID:      p.VariantID,
		ProductID:  p.ProductID,
		Quantity:   1,
		OccurredAt: e.OccurredAt.Format(time.RFC3339),
		SourceType: "wishlist",
		SourceID:   p.CustomerID,
	}

	reqs := make([]SignalRequest, 0, 2)

	yeuThich := chung
	yeuThich.Type = SignalWishlist
	reqs = append(reqs, yeuThich)

	if p.MuonBaoKhiCoHang {
		xinBao := chung
		xinBao.Type = SignalNotifyRequest
		reqs = append(reqs, xinBao)
	}

	return h.module.RecordSignals(ctx, reqs)
}
