package affiliate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// AttributeOnOrderPaid quy công đơn cho creator khi tiền của đơn đã về.
//
// # Vì sao lúc TIỀN VỀ, không lúc đặt đơn
//
// Một đơn chưa trả tiền có thể không bao giờ trả. Quy công lúc đặt nghĩa là
// sinh ra một khoản hoa hồng cho doanh thu chưa tồn tại, rồi phải có một
// đường dọn nó về sau — và đường dọn ấy là chỗ P3-71 cho thấy dễ sai nhất
// của cả hệ thống tiền.
//
// `order.paid` là mốc DUY NHẤT nói "tiền của đơn này đã về" (ADR-0018), nên
// nó là mốc đúng.
//
// # Bên nhận này ĐỌC NGƯỢC module order, và đó là chủ ý
//
// Payload `order.paid` mang tổng tiền, không mang dòng hàng — mà hoa hồng
// tính theo TỪNG DÒNG, vì một đơn có thể có dòng đến từ nội dung creator và
// dòng khách tự tìm. Nhồi dòng hàng vào payload sẽ bắt payment và
// fulfillment khai lại `MaxEventVersion` (ADR-0016) cho một trường chỉ
// affiliate dùng.
//
// Chiều extension → kernel là chiều được phép của ADR-0021. Chiều ngược lại
// thì không, và không có ở đây: module order không biết module này tồn tại.
type AttributeOnOrderPaid struct {
	module *Module
	log    *slog.Logger
}

func NewAttributeHandler(m *Module, log *slog.Logger) *AttributeOnOrderPaid {
	if log == nil {
		log = slog.Default()
	}
	return &AttributeOnOrderPaid{module: m, log: log}
}

var _ eventbus.Handler = (*AttributeOnOrderPaid)(nil)

func (h *AttributeOnOrderPaid) Name() string {
	return "affiliate.attribute_on_order_paid"
}

func (h *AttributeOnOrderPaid) EventTypes() []string {
	return []string{eventbus.TypeOrderPaid}
}

type orderPaidPayload struct {
	OrderID string `json:"order_id"`
}

// Handle quy công mọi dòng có creator của đơn.
//
// IDEMPOTENT: `UNIQUE (order_line_id)` chặn ở tầng database, và tầng
// application coi việc trùng là THÀNH CÔNG. Lượt thứ hai đếm mọi dòng vào
// "đã bỏ qua".
func (h *AttributeOnOrderPaid) Handle(ctx context.Context, e eventbus.Event) error {
	var p orderPaidPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("affiliate: đọc dữ liệu event order.paid: %w", err)
	}

	kq, err := h.module.svc.AttributeOrder(ctx, p.OrderID)
	if err != nil {
		return fmt.Errorf("affiliate: quy công đơn %s: %w", p.OrderID, err)
	}
	if kq.Attributed > 0 {
		h.log.InfoContext(ctx, "đã quy công đơn cho creator",
			"order_id", p.OrderID,
			"so_dong_quy_cong", kq.Attributed,
			"so_dong_bo_qua", kq.Skipped)
	}
	return nil
}

// ---------------------------------------------------------------- Đảo

// ReverseOnOrderCancelled đảo hoa hồng khi đơn bị hủy.
//
// affiliate.md quy tắc 5. Không có bên nhận này thì hủy đơn để lại một khoản
// hoa hồng CHỜ ĐẾN HẠN cho doanh thu không còn tồn tại, và tới kỳ chi trả
// nền tảng trả tiền cho một đơn đã hủy.
//
// Hủy TỪNG PHẦN không đi qua đây, và hậu quả KHÔNG phải trả tiền sai: đơn
// thực hiện bị hủy không bao giờ phát `fulfillment_order.completed`, nên
// khoản hoa hồng của dòng ấy đứng ở CHỜ ĐẾN HẠN mãi mãi — không đảo, không
// mở khóa. Một dòng rác trong bảng tiền, và nó im lặng. Xem P3-85.
type ReverseOnOrderCancelled struct {
	module *Module
	log    *slog.Logger
}

func NewReverseHandler(m *Module, log *slog.Logger) *ReverseOnOrderCancelled {
	if log == nil {
		log = slog.Default()
	}
	return &ReverseOnOrderCancelled{module: m, log: log}
}

var _ eventbus.Handler = (*ReverseOnOrderCancelled)(nil)

func (h *ReverseOnOrderCancelled) Name() string { return "affiliate.reverse_on_order_cancelled" }

func (h *ReverseOnOrderCancelled) EventTypes() []string {
	return []string{eventbus.TypeOrderCancelled}
}

type orderCancelledPayload struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

// Handle đảo mọi khoản hoa hồng của đơn, GHI LẠI lý do.
//
// Lý do lấy từ payload chứ không bịa một câu cố định: `order.cancelled` đã
// mang nó, và một khoản tiền biến mất kèm đúng lý do của nó là điều kiện để
// giải thích được lúc đối soát.
func (h *ReverseOnOrderCancelled) Handle(ctx context.Context, e eventbus.Event) error {
	var p orderCancelledPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("affiliate: đọc dữ liệu event order.cancelled: %w", err)
	}

	reason := p.Reason
	if reason == "" {
		// Miền BẮT BUỘC có lý do, nên không được để rỗng đi xuống. Câu này
		// nói đúng thứ đã biết: đơn bị hủy, còn vì sao thì payload không
		// nói.
		reason = "đơn bị hủy (event không nêu lý do)"
	}

	n, err := h.module.svc.ReverseByOrder(ctx, p.OrderID, reason)
	if err != nil {
		return fmt.Errorf("affiliate: đảo hoa hồng của đơn %s: %w", p.OrderID, err)
	}
	if n > 0 {
		h.log.InfoContext(ctx, "đã đảo hoa hồng creator vì đơn bị hủy",
			"order_id", p.OrderID, "so_khoan", n, "ly_do", reason)
	}
	return nil
}

// ---------------------------------------------------------------- Mở khóa

// UnlockOnReturnWindowPassed chuyển hoa hồng sang RÚT ĐƯỢC.
//
// # Vì sao nghe `fulfillment_order.completed`
//
// affiliate.md quy tắc 4: hoa hồng chỉ rút được sau khi hết hạn đổi trả.
// Hạn đổi trả là cấu hình nghiệp vụ mà module fulfillment sở hữu, và event
// này CHÍNH LÀ tín hiệu "đơn thực hiện đã qua hạn ấy" — cùng tín hiệu
// payment dùng để mở số dư nhà bán (ADR-0018).
//
// Tự đoán hạn ở đây là cài cùng một quy tắc lần thứ hai, và hai bản cài của
// một quy tắc tiền sớm muộn sẽ lệch — đúng lỗi P3-18.
//
// Event mang `(order_id, seller_id)` chứ không mang dòng hàng, nên bản quy
// công LƯU LẠI `seller_id`: một đơn nhiều nhà bán hết hạn theo từng nhà bán,
// và mở khóa cả đơn khi nhà bán đầu tiên giao xong là trả tiền cho hàng còn
// trong hạn trả.
type UnlockOnReturnWindowPassed struct {
	module *Module
	log    *slog.Logger
}

func NewUnlockHandler(m *Module, log *slog.Logger) *UnlockOnReturnWindowPassed {
	if log == nil {
		log = slog.Default()
	}
	return &UnlockOnReturnWindowPassed{module: m, log: log}
}

var _ eventbus.Handler = (*UnlockOnReturnWindowPassed)(nil)

func (h *UnlockOnReturnWindowPassed) Name() string {
	return "affiliate.unlock_on_return_window_passed"
}

func (h *UnlockOnReturnWindowPassed) EventTypes() []string {
	return []string{eventbus.TypeFulfillmentCompleted}
}

type fulfillmentCompletedPayload struct {
	OrderID  string `json:"order_id"`
	SellerID string `json:"seller_id"`
}

func (h *UnlockOnReturnWindowPassed) Handle(ctx context.Context, e eventbus.Event) error {
	var p fulfillmentCompletedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf(
			"affiliate: đọc dữ liệu event fulfillment_order.completed: %w", err)
	}

	n, err := h.module.svc.UnlockByOrder(ctx, p.OrderID, p.SellerID)
	if err != nil {
		return fmt.Errorf("affiliate: mở khóa hoa hồng của đơn %s: %w",
			p.OrderID, err)
	}
	if n > 0 {
		h.log.InfoContext(ctx, "hoa hồng creator chuyển sang rút được",
			"order_id", p.OrderID, "seller_id", p.SellerID, "so_khoan", n)
	}
	return nil
}

// ------------------------------------------- Đảo khi hủy TỪNG PHẦN

// ReverseOnFulfillmentCancelled đảo hoa hồng của những dòng mà nhà bán không giao nữa.
//
// # Vì sao `ReverseOnOrderCancelled` không đủ
//
// Nó nghe `order.cancelled`, và event ấy chỉ phát khi hủy CẢ đơn. Đường hủy
// TỪNG PHẦN đi như sau, và tới 01/10/2026 không chỗ nào affiliate nghe được:
//
//	đơn thực hiện của MỘT nhà bán bị hủy
//	  → fulfillment phát `fulfillment.cancelled`
//	  → order.RecalculateStatus → PARTIALLY_CANCELLED
//	  → KHÔNG phát `order.cancelled`
//
// Hệ quả KHÔNG phải trả tiền sai, mà là một dòng rác trong bảng tiền: đơn
// thực hiện bị hủy không bao giờ phát `fulfillment_order.completed`, nên
// khoản hoa hồng của dòng ấy đứng ở CHỜ ĐẾN HẠN mãi mãi — không đảo, không
// mở khóa, không ai thấy. Xem P3-85.
//
// Đảo theo DÒNG, không theo đơn: nhà bán còn lại vẫn đang giao bình thường,
// và hoa hồng của họ không được chạm tới.
type ReverseOnFulfillmentCancelled struct {
	module *Module
	log    *slog.Logger
}

func NewReverseOnFulfillmentHandler(m *Module, log *slog.Logger) *ReverseOnFulfillmentCancelled {
	if log == nil {
		log = slog.Default()
	}
	return &ReverseOnFulfillmentCancelled{module: m, log: log}
}

var _ eventbus.Handler = (*ReverseOnFulfillmentCancelled)(nil)

func (h *ReverseOnFulfillmentCancelled) Name() string {
	return "affiliate.reverse_on_fulfillment_cancelled"
}

func (h *ReverseOnFulfillmentCancelled) EventTypes() []string {
	return []string{eventbus.TypeFulfillmentCancelled}
}

// MaxEventVersion: bên nhận này ĐÒI phiên bản 2 trở lên.
//
// v1 của `fulfillment.cancelled` mang `(sku_id, quantity)` và KHÔNG mang
// `order_line_id` — tức không đủ để đảo đúng người. Một đơn có thể có hai
// dòng cùng SKU với hai creator khác nhau, nên gộp theo SKU sẽ đảo sai.
//
// Trường ấy thêm ở v2 (01/10/2026), cùng lúc với bên nhận này.
func (h *ReverseOnFulfillmentCancelled) MaxEventVersion(eventType string) int {
	if eventType == eventbus.TypeFulfillmentCancelled {
		return 2
	}
	return eventbus.DefaultMaxEventVersion
}

type fulfillmentCancelledPayload struct {
	OrderID       string `json:"order_id"`
	FulfillmentID string `json:"fulfillment_id"`
	FONumber      string `json:"fo_number"`

	Lines []struct {
		OrderLineID string `json:"order_line_id"`
	} `json:"lines"`
}

func (h *ReverseOnFulfillmentCancelled) Handle(ctx context.Context, e eventbus.Event) error {
	var p fulfillmentCancelledPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf(
			"affiliate: đọc dữ liệu event fulfillment.cancelled: %w", err)
	}

	ma := make([]string, 0, len(p.Lines))
	for _, l := range p.Lines {
		if l.OrderLineID == "" {
			continue
		}
		ma = append(ma, l.OrderLineID)
	}

	// Đơn thực hiện CŨ không có mã dòng đơn (`FOLine.OrderLineID` chỉ được
	// điền từ migration 000024). Ghi log ở mức CẢNH BÁO thay vì trả lỗi:
	// trả lỗi sẽ làm event thử lại mãi rồi vào dead letter, mà thử lại
	// không làm dữ liệu cũ có thêm mã dòng.
	if len(ma) == 0 && len(p.Lines) > 0 {
		h.log.WarnContext(ctx,
			"hủy đơn thực hiện không mang mã dòng đơn — hoa hồng creator "+
				"của các dòng này KHÔNG đảo được",
			"order_id", p.OrderID, "fo_number", p.FONumber,
			"so_dong", len(p.Lines))
		return nil
	}

	// Lý do nêu rõ ĐƠN THỰC HIỆN nào, không chỉ "bị hủy": lúc đối soát,
	// "đơn thực hiện FC-FO-000123 bị hủy" tra ra được chuyện đã xảy ra, còn
	// "đơn bị hủy" thì không.
	reason := "đơn thực hiện " + p.FONumber + " bị hủy"
	if p.FONumber == "" {
		reason = "đơn thực hiện " + p.FulfillmentID + " bị hủy"
	}

	n, err := h.module.svc.ReverseByOrderLines(ctx, ma, reason)
	if err != nil {
		return fmt.Errorf("affiliate: đảo hoa hồng của %d dòng đơn: %w",
			len(ma), err)
	}
	if n > 0 {
		h.log.InfoContext(ctx,
			"đã đảo hoa hồng creator vì đơn thực hiện bị hủy",
			"order_id", p.OrderID, "fo_number", p.FONumber, "so_khoan", n)
	}
	return nil
}
