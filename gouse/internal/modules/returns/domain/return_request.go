// Package domain là mô hình nghiệp vụ của việc trả hàng.
//
// # Vì sao thư mục tên `returns` chứ không phải `return`
//
// `return` là từ khóa của Go, không dùng làm tên gói được. Tài liệu kiến
// trúc gọi module này là `return`; ở mã nguồn nó là `returns`.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrNotFound         = errors.New("returns: không tìm thấy yêu cầu trả hàng")
	ErrInvalidStatus    = errors.New("returns: chuyển trạng thái không hợp lệ")
	ErrNoLines          = errors.New("returns: yêu cầu trả hàng phải có ít nhất một dòng")
	ErrInvalidReason    = errors.New("returns: lý do trả hàng không hợp lệ")
	ErrMissingReason    = errors.New("returns: phải nêu lý do từ chối")
	ErrVersionConflict  = errors.New("returns: yêu cầu vừa bị thay đổi, hãy đọc lại")
	ErrDuplicateLine    = errors.New("returns: dòng hàng này đã có yêu cầu trả")
	ErrQuantityExceeded = errors.New("returns: số lượng xin trả vượt số đã mua")
	ErrChuaNhanHang     = errors.New("returns: chưa nhận được hàng, không kiểm định được")
	ErrDaKiemDinh       = errors.New("returns: dòng hàng này đã kiểm định rồi")
	ErrThieuLyDoLoai    = errors.New("returns: loại hàng phải nêu lý do")

	// ErrGiamGiaChuaPhanBo là HÀNG RÀO CHỐNG HOÀN THỪA.
	//
	// Đơn có giảm giá ở CẤP ĐƠN mà không dòng nào mang khoản điều chỉnh
	// tương ứng nghĩa là phần giảm chưa được phân bổ xuống dòng. Hoàn theo
	// giá dòng khi đó là hoàn nhiều hơn số khách đã trả.
	//
	// Thà TỪ CHỐI và bắt người vận hành xử lý tay, còn hơn âm thầm trả ra
	// số tiền sai. docs/07-workflows/return.md mục 5 gọi đây là điểm dễ
	// sai nhất của cả luồng.
	ErrGiamGiaChuaPhanBo = errors.New(
		"returns: đơn có giảm giá chưa phân bổ xuống dòng hàng — " +
			"không tính được số tiền thực trả, từ chối hoàn tự động")
)

// LyDo là lý do trả hàng, CHUẨN HÓA.
//
// Lý do quyết định DÒNG TIỀN và HÀNH ĐỘNG KHẮC PHỤC — hàng lỗi thì truy
// vết lô sản xuất, sai size thì sửa bảng size, hỏng khi vận chuyển thì
// khiếu nại đối tác. Văn bản tự do không phân tích được.
type LyDo string

const (
	LyDoSizeNho       LyDo = "SIZE_TOO_SMALL"
	LyDoSizeLon       LyDo = "SIZE_TOO_LARGE"
	LyDoKhacMoTa      LyDo = "NOT_AS_DESCRIBED"
	LyDoKhacMau       LyDo = "COLOR_DIFFERENT"
	LyDoChatLuong     LyDo = "QUALITY_ISSUE"
	LyDoHangLoi       LyDo = "DEFECTIVE"
	LyDoGiaoSaiHang   LyDo = "WRONG_ITEM_SENT"
	LyDoHongVanChuyen LyDo = "DAMAGED_IN_TRANSIT"
	LyDoDoiY          LyDo = "CHANGED_MIND"
	LyDoGiaoTre       LyDo = "LATE_DELIVERY"
)

var lyDoHopLe = map[LyDo]bool{
	LyDoSizeNho: true, LyDoSizeLon: true,
	LyDoKhacMoTa: true, LyDoKhacMau: true,
	LyDoChatLuong: true, LyDoHangLoi: true,
	LyDoGiaoSaiHang: true, LyDoHongVanChuyen: true,
	LyDoDoiY: true, LyDoGiaoTre: true,
}

func (l LyDo) HopLe() bool { return lyDoHopLe[l] }

// LoiCuaNguoiBan cho biết lý do này quy trách nhiệm cho bên bán.
//
// Dùng để quyết ai chịu phí vận chuyển chiều về. Khách đổi ý thì khách
// chịu; hàng lỗi hay giao sai thì không.
func (l LyDo) LoiCuaNguoiBan() bool {
	switch l {
	case LyDoHangLoi, LyDoGiaoSaiHang, LyDoKhacMoTa, LyDoKhacMau, LyDoChatLuong:
		return true
	}
	return false
}

// TrangThai là trạng thái một yêu cầu trả hàng.
type TrangThai string

const (
	TTYeuCau TrangThai = "REQUESTED"
	TTDuyet  TrangThai = "APPROVED"
	TTTuChoi TrangThai = "REJECTED"
	TTDaNhan TrangThai = "RECEIVED"
	TTDaHoan TrangThai = "REFUNDED"
	TTDaHuy  TrangThai = "CANCELLED"
)

// KetThuc cho biết trạng thái đã là cuối, không đổi được nữa.
func (t TrangThai) KetThuc() bool {
	return t == TTTuChoi || t == TTDaHoan || t == TTDaHuy
}

// Dong là một dòng hàng xin trả.
type Dong struct {
	ID          ids.ID
	OrderLineID ids.ID
	SKUID       ids.ID
	Quantity    int

	// TienHoan là phần tiền của dòng này, ĐÓNG BĂNG lúc tạo yêu cầu.
	//
	// Đóng băng chứ không tính lại lúc hoàn: giữa lúc khách xin trả và lúc
	// hàng về kho, giá có thể đã đổi và khuyến mãi có thể đã kết thúc.
	TienHoan money.Money

	// LyDo là lý do trả CỦA DÒNG NÀY.
	//
	// Khách trả hai món trong một đơn vì hai lý do khác nhau là chuyện
	// thường — cái áo chật, cái quần lỗi đường may. Mỗi lý do dẫn tới một
	// hành động khắc phục khác nhau, nên gộp chúng lại là mất cả hai.
	LyDo LyDo

	// ChiTiet là mô tả thêm của khách cho dòng này.
	ChiTiet string

	// KiemDinh là kết quả kiểm định hàng hoàn: PENDING, PASSED, FAILED.
	//
	// Hàng hoàn về kho nằm ở Returned và KHÔNG BAO GIỜ tự động vào
	// Available — bước này quyết định nó đi đâu.
	KiemDinh KetQuaKiemDinh

	// GhiChuKiemDinh BẮT BUỘC khi loại hàng: lý do loại là đầu vào cho
	// việc làm việc với nhà cung cấp và quyết ai chịu chi phí.
	GhiChuKiemDinh string

	InspectedAt time.Time
}

// KetQuaKiemDinh là kết quả kiểm định một dòng hàng hoàn.
type KetQuaKiemDinh string

const (
	KiemDinhChoXuLy KetQuaKiemDinh = "PENDING"
	KiemDinhDat     KetQuaKiemDinh = "PASSED"
	KiemDinhLoai    KetQuaKiemDinh = "FAILED"
)

// YeuCauTraHang là một yêu cầu trả hàng.
type YeuCauTraHang struct {
	id         ids.ID
	orderID    ids.ID
	sellerID   ids.ID
	customerID ids.ID

	status     TrangThai
	lyDo       LyDo
	ghiChu     string
	lyDoTuChoi string

	dong     []Dong
	tienHoan money.Money

	requestedAt time.Time
	decidedAt   time.Time
	receivedAt  time.Time
	refundedAt  time.Time

	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// TaoParams là dữ liệu tạo một yêu cầu trả hàng.
type TaoParams struct {
	OrderID    ids.ID
	SellerID   ids.ID
	CustomerID ids.ID
	GhiChu     string
	Dong       []Dong
	Now        time.Time
}

// Tao dựng một yêu cầu trả hàng mới ở trạng thái REQUESTED.
func Tao(p TaoParams) (*YeuCauTraHang, error) {
	if len(p.Dong) == 0 {
		return nil, ErrNoLines
	}
	for _, d := range p.Dong {
		if !d.LyDo.HopLe() {
			return nil, ErrInvalidReason
		}
	}

	// Tổng tiền hoàn = tổng các dòng. Cộng ở đây, một chỗ duy nhất, thay
	// vì để bên gọi truyền vào — hai nguồn cho cùng một con số tiền sớm
	// muộn sẽ lệch nhau.
	tong := p.Dong[0].TienHoan
	daThay := map[ids.ID]bool{p.Dong[0].OrderLineID: true}
	for _, d := range p.Dong[1:] {
		if daThay[d.OrderLineID] {
			return nil, ErrDuplicateLine
		}
		daThay[d.OrderLineID] = true

		cong, err := tong.Add(d.TienHoan)
		if err != nil {
			return nil, err
		}
		tong = cong
	}

	return &YeuCauTraHang{
		id:         ids.MustNew(ids.PrefixReturnRequest),
		orderID:    p.OrderID,
		sellerID:   p.SellerID,
		customerID: p.CustomerID,
		status:     TTYeuCau,
		// Lý do CHÍNH là lý do của dòng đầu — chỉ để lọc nhanh danh sách.
		// Lý do đầy đủ nằm ở từng dòng.
		lyDo:        p.Dong[0].LyDo,
		ghiChu:      strings.TrimSpace(p.GhiChu),
		dong:        p.Dong,
		tienHoan:    tong,
		requestedAt: p.Now,
		createdAt:   p.Now,
		updatedAt:   p.Now,
	}, nil
}

// Duyet chấp nhận yêu cầu trả hàng.
func (y *YeuCauTraHang) Duyet(now time.Time) error {
	if y.status != TTYeuCau {
		return ErrInvalidStatus
	}
	y.status = TTDuyet
	y.decidedAt = now
	y.touch(now)
	return nil
}

// TuChoi từ chối yêu cầu, BẮT BUỘC nêu lý do.
//
// Khách cần biết vì sao để quyết định làm gì tiếp — khiếu nại, gửi thêm
// ảnh, hay chấp nhận. Từ chối không lý do biến mọi trường hợp thành khiếu nại.
func (y *YeuCauTraHang) TuChoi(lyDo string, now time.Time) error {
	if y.status != TTYeuCau {
		return ErrInvalidStatus
	}
	if strings.TrimSpace(lyDo) == "" {
		return ErrMissingReason
	}
	y.status = TTTuChoi
	y.lyDoTuChoi = strings.TrimSpace(lyDo)
	y.decidedAt = now
	y.touch(now)
	return nil
}

// DaNhanHang ghi nhận hàng đã về kho.
//
// KHÔNG cộng vào tồn khả dụng — đó là việc của bước kiểm định, và quy tắc
// bắt buộc là hàng hoàn không bao giờ tự động thành Available. Vi phạm nó
// nghĩa là bán lại hàng hỏng cho khách khác.
func (y *YeuCauTraHang) DaNhanHang(now time.Time) error {
	if y.status != TTDuyet {
		return ErrInvalidStatus
	}
	y.status = TTDaNhan
	y.receivedAt = now
	y.touch(now)
	return nil
}

// DaHoanTien ghi nhận đã hoàn tiền.
func (y *YeuCauTraHang) DaHoanTien(now time.Time) error {
	if y.status != TTDaNhan {
		return ErrInvalidStatus
	}
	y.status = TTDaHoan
	y.refundedAt = now
	y.touch(now)
	return nil
}

// Huy để khách rút yêu cầu khi chưa ai xử lý.
func (y *YeuCauTraHang) Huy(now time.Time) error {
	if y.status != TTYeuCau {
		return ErrInvalidStatus
	}
	y.status = TTDaHuy
	y.touch(now)
	return nil
}

func (y *YeuCauTraHang) touch(now time.Time) { y.updatedAt = now }

func (y *YeuCauTraHang) ID() ids.ID             { return y.id }
func (y *YeuCauTraHang) OrderID() ids.ID        { return y.orderID }
func (y *YeuCauTraHang) SellerID() ids.ID       { return y.sellerID }
func (y *YeuCauTraHang) CustomerID() ids.ID     { return y.customerID }
func (y *YeuCauTraHang) Status() TrangThai      { return y.status }
func (y *YeuCauTraHang) LyDo() LyDo             { return y.lyDo }
func (y *YeuCauTraHang) GhiChu() string         { return y.ghiChu }
func (y *YeuCauTraHang) LyDoTuChoi() string     { return y.lyDoTuChoi }
func (y *YeuCauTraHang) TienHoan() money.Money  { return y.tienHoan }
func (y *YeuCauTraHang) RequestedAt() time.Time { return y.requestedAt }
func (y *YeuCauTraHang) DecidedAt() time.Time   { return y.decidedAt }
func (y *YeuCauTraHang) ReceivedAt() time.Time  { return y.receivedAt }
func (y *YeuCauTraHang) RefundedAt() time.Time  { return y.refundedAt }
func (y *YeuCauTraHang) Version() int64         { return y.version }
func (y *YeuCauTraHang) CreatedAt() time.Time   { return y.createdAt }
func (y *YeuCauTraHang) UpdatedAt() time.Time   { return y.updatedAt }

// Dong trả bản sao danh sách dòng hàng.
func (y *YeuCauTraHang) Dong() []Dong {
	return append([]Dong(nil), y.dong...)
}

// KhoiPhucParams dựng lại từ kho lưu trữ.
type KhoiPhucParams struct {
	ID         ids.ID
	OrderID    ids.ID
	SellerID   ids.ID
	CustomerID ids.ID
	Status     TrangThai
	LyDo       LyDo
	GhiChu     string
	LyDoTuChoi string
	Dong       []Dong
	TienHoan   money.Money

	RequestedAt time.Time
	DecidedAt   time.Time
	ReceivedAt  time.Time
	RefundedAt  time.Time

	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// KhoiPhuc dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func KhoiPhuc(p KhoiPhucParams) *YeuCauTraHang {
	return &YeuCauTraHang{
		id: p.ID, orderID: p.OrderID, sellerID: p.SellerID,
		customerID: p.CustomerID, status: p.Status, lyDo: p.LyDo,
		ghiChu: p.GhiChu, lyDoTuChoi: p.LyDoTuChoi,
		dong: p.Dong, tienHoan: p.TienHoan,
		requestedAt: p.RequestedAt, decidedAt: p.DecidedAt,
		receivedAt: p.ReceivedAt, refundedAt: p.RefundedAt,
		version: p.Version, createdAt: p.CreatedAt, updatedAt: p.UpdatedAt,
	}
}

// GhiKetQuaKiemDinh ghi kết quả kiểm định cho một dòng hàng.
//
// # Vì sao chỉ cho kiểm sau khi ĐÃ NHẬN hàng
//
// Kiểm định là nhìn vào món hàng thật. Ghi kết quả trước khi hàng về là
// ghi một điều chưa ai biết — và nó sẽ đẩy hàng vào Available trong khi
// kho chưa có gì.
func (y *YeuCauTraHang) GhiKetQuaKiemDinh(
	orderLineID ids.ID, dat bool, ghiChu string, now time.Time,
) error {
	if y.status != TTDaNhan && y.status != TTDaHoan {
		return ErrChuaNhanHang
	}
	if !dat && strings.TrimSpace(ghiChu) == "" {
		return ErrThieuLyDoLoai
	}

	for i := range y.dong {
		if y.dong[i].OrderLineID != orderLineID {
			continue
		}
		if y.dong[i].KiemDinh != KiemDinhChoXuLy && y.dong[i].KiemDinh != "" {
			// Kiểm hai lần nghĩa là hàng vào Available hai lần — tồn kho
			// tăng thêm số hàng không có thật.
			return ErrDaKiemDinh
		}

		y.dong[i].KiemDinh = KiemDinhLoai
		if dat {
			y.dong[i].KiemDinh = KiemDinhDat
		}
		y.dong[i].GhiChuKiemDinh = strings.TrimSpace(ghiChu)
		y.dong[i].InspectedAt = now
		y.touch(now)
		return nil
	}
	return ErrNotFound
}

// ConChoKiemDinh cho biết còn dòng nào chưa kiểm không.
func (y *YeuCauTraHang) ConChoKiemDinh() bool {
	for _, d := range y.dong {
		if d.KiemDinh == KiemDinhChoXuLy || d.KiemDinh == "" {
			return true
		}
	}
	return false
}
