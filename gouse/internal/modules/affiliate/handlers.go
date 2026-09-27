package affiliate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// QuyCongKhiDaTraTien quy công đơn cho creator khi tiền của đơn đã về.
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
type QuyCongKhiDaTraTien struct {
	module *Module
	log    *slog.Logger
}

func NewQuyCongHandler(m *Module, log *slog.Logger) *QuyCongKhiDaTraTien {
	if log == nil {
		log = slog.Default()
	}
	return &QuyCongKhiDaTraTien{module: m, log: log}
}

var _ eventbus.Handler = (*QuyCongKhiDaTraTien)(nil)

func (h *QuyCongKhiDaTraTien) Name() string {
	return "affiliate.quy_cong_khi_da_tra_tien"
}

func (h *QuyCongKhiDaTraTien) EventTypes() []string {
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
func (h *QuyCongKhiDaTraTien) Handle(ctx context.Context, e eventbus.Event) error {
	var p orderPaidPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("affiliate: đọc dữ liệu event order.paid: %w", err)
	}

	kq, err := h.module.svc.QuyCongDon(ctx, p.OrderID)
	if err != nil {
		return fmt.Errorf("affiliate: quy công đơn %s: %w", p.OrderID, err)
	}
	if kq.DaQuyCong > 0 {
		h.log.InfoContext(ctx, "đã quy công đơn cho creator",
			"order_id", p.OrderID,
			"so_dong_quy_cong", kq.DaQuyCong,
			"so_dong_bo_qua", kq.DaBoQua)
	}
	return nil
}

// ---------------------------------------------------------------- Đảo

// DaoKhiHuyDon đảo hoa hồng khi đơn bị hủy.
//
// affiliate.md quy tắc 5. Không có bên nhận này thì hủy đơn để lại một khoản
// hoa hồng CHỜ ĐẾN HẠN cho doanh thu không còn tồn tại, và tới kỳ chi trả
// nền tảng trả tiền cho một đơn đã hủy.
//
// Hủy TỪNG PHẦN không đi qua đây, và hậu quả KHÔNG phải trả tiền sai: đơn
// thực hiện bị hủy không bao giờ phát `fulfillment_order.completed`, nên
// khoản hoa hồng của dòng ấy đứng ở CHỜ ĐẾN HẠN mãi mãi — không đảo, không
// mở khóa. Một dòng rác trong bảng tiền, và nó im lặng. Xem P3-85.
type DaoKhiHuyDon struct {
	module *Module
	log    *slog.Logger
}

func NewDaoHandler(m *Module, log *slog.Logger) *DaoKhiHuyDon {
	if log == nil {
		log = slog.Default()
	}
	return &DaoKhiHuyDon{module: m, log: log}
}

var _ eventbus.Handler = (*DaoKhiHuyDon)(nil)

func (h *DaoKhiHuyDon) Name() string { return "affiliate.dao_khi_huy_don" }

func (h *DaoKhiHuyDon) EventTypes() []string {
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
func (h *DaoKhiHuyDon) Handle(ctx context.Context, e eventbus.Event) error {
	var p orderCancelledPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("affiliate: đọc dữ liệu event order.cancelled: %w", err)
	}

	lyDo := p.Reason
	if lyDo == "" {
		// Miền BẮT BUỘC có lý do, nên không được để rỗng đi xuống. Câu này
		// nói đúng thứ đã biết: đơn bị hủy, còn vì sao thì payload không
		// nói.
		lyDo = "đơn bị hủy (event không nêu lý do)"
	}

	n, err := h.module.svc.DaoTheoDon(ctx, p.OrderID, lyDo)
	if err != nil {
		return fmt.Errorf("affiliate: đảo hoa hồng của đơn %s: %w", p.OrderID, err)
	}
	if n > 0 {
		h.log.InfoContext(ctx, "đã đảo hoa hồng creator vì đơn bị hủy",
			"order_id", p.OrderID, "so_khoan", n, "ly_do", lyDo)
	}
	return nil
}

// ---------------------------------------------------------------- Mở khóa

// MoKhoaKhiHetHanDoiTra chuyển hoa hồng sang RÚT ĐƯỢC.
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
type MoKhoaKhiHetHanDoiTra struct {
	module *Module
	log    *slog.Logger
}

func NewMoKhoaHandler(m *Module, log *slog.Logger) *MoKhoaKhiHetHanDoiTra {
	if log == nil {
		log = slog.Default()
	}
	return &MoKhoaKhiHetHanDoiTra{module: m, log: log}
}

var _ eventbus.Handler = (*MoKhoaKhiHetHanDoiTra)(nil)

func (h *MoKhoaKhiHetHanDoiTra) Name() string {
	return "affiliate.mo_khoa_khi_het_han_doi_tra"
}

func (h *MoKhoaKhiHetHanDoiTra) EventTypes() []string {
	return []string{eventbus.TypeFulfillmentCompleted}
}

type fulfillmentCompletedPayload struct {
	OrderID  string `json:"order_id"`
	SellerID string `json:"seller_id"`
}

func (h *MoKhoaKhiHetHanDoiTra) Handle(ctx context.Context, e eventbus.Event) error {
	var p fulfillmentCompletedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf(
			"affiliate: đọc dữ liệu event fulfillment_order.completed: %w", err)
	}

	n, err := h.module.svc.MoKhoaTheoDon(ctx, p.OrderID, p.SellerID)
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
