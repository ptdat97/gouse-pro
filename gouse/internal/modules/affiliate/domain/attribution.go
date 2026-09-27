// Package domain là mô hình nghiệp vụ của việc quy công cho creator.
//
// # Module này là một EXTENSION, không phải kernel
//
// Bỏ nó ra thì khách vẫn mua được hàng — chỉ mất phần ghi công cho creator.
// Đó là tiêu chí phân loại của [ADR-0021], và nó quyết định mọi thứ ở đây:
//
//	KHÔNG import module nào khác          kể cả order, payment
//	KHÔNG chạm bảng của module nào khác   R6 cưỡng chế
//	Nhận dữ liệu qua EVENT, không gọi ngược
//
// Kernel chở `order_line.attributed_creator_id` tới đây và hết phần của nó.
// Kernel không biết tỷ lệ hoa hồng, không biết ai chịu chi phí, không biết
// mô hình quy công nào đang dùng.
//
// [ADR-0021]: ../../../../docs/adr/0021-ranh-gioi-commerce-kernel.md
package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

var (
	ErrNotFound = errors.New("affiliate: không tìm thấy bản ghi")

	// ErrDaQuyCong khi một dòng đơn đã được quy công.
	//
	// KHÔNG phải lỗi hệ thống: `order.paid` giao ít nhất một lần, nên bên
	// nhận thấy cùng một đơn hai lần là chuyện thường. Bên gọi coi đây là
	// THÀNH CÔNG và bỏ qua.
	ErrDaQuyCong = errors.New("affiliate: dòng đơn này đã được quy công")

	ErrThieuCreator  = errors.New("affiliate: thiếu định danh creator")
	ErrThieuDongDon  = errors.New("affiliate: thiếu định danh dòng đơn")
	ErrTienGocAm     = errors.New("affiliate: tiền gốc tính hoa hồng không được âm")
	ErrBenChiuLa     = errors.New("affiliate: bên chịu chi phí không hợp lệ")
	ErrThieuLyDoDao  = errors.New("affiliate: đảo quy công bắt buộc phải nêu lý do")
	ErrDaDao         = errors.New("affiliate: khoản hoa hồng này đã bị đảo")
	ErrChuaHetHanTra = errors.New("affiliate: chưa hết hạn đổi trả, chưa được chuyển sang rút được")
)

// BenChiu là bên gánh chi phí hoa hồng creator.
//
// affiliate.md mục 5: *"Phải được ghi rõ trong Campaign, không mặc định"*.
// Tới khi có module `campaign`, mặc định là nền tảng chịu — xem
// `BenChiuMacDinh`.
type BenChiu string

const (
	BenChiuNenTang BenChiu = "PLATFORM"
	BenChiuNhaBan  BenChiu = "SELLER"
)

func (b BenChiu) HopLe() bool {
	return b == BenChiuNenTang || b == BenChiuNhaBan
}

// MoHinhQuyCong là cách chọn creator khi có nhiều người trong chuỗi.
type MoHinhQuyCong string

// MoHinhLastClick: creator của click GẦN NHẤT nhận toàn bộ.
//
// Quyết định khởi điểm của affiliate.md mục 3.3. Ghi vào từng bản quy công
// vì đổi mô hình sau này phải giải thích được số cũ tính theo cách nào.
//
// Ở phiên bản này việc chọn creator đã xảy ra TRƯỚC khi tới module này:
// `cart.Item.setSource` giữ nguồn của lần thêm GẦN NHẤT (cart.md mục 6), và
// chuỗi giỏ→phiên→đơn chở đúng một creator. Nên "last click" ở đây là mô tả
// thứ đã xảy ra, không phải phép tính của module này.
const MoHinhLastClick MoHinhQuyCong = "LAST_CLICK"

// Attribution là một bản quy công — BẤT BIẾN sau khi tạo.
//
// affiliate.md quy tắc 2. Sửa một bản quy công nghĩa là sửa một khoản tiền
// đã hứa với creator; "sửa" là ghi một bản mới, cùng nguyên tắc với sổ cái
// (ADR-0008).
//
// Vì bất biến, struct này KHÔNG có method nào đổi trạng thái. Trạng thái của
// khoản tiền nằm ở `CommissionRecord`.
type Attribution struct {
	id          ids.ID
	orderID     ids.ID
	orderLineID ids.ID

	// sellerID là nhà bán của dòng hàng, ĐÓNG BĂNG cùng bản quy công.
	//
	// Cần vì tín hiệu hết hạn đổi trả của hệ thống mang `(order_id,
	// seller_id)` chứ không mang dòng hàng — xem chú thích ở migration
	// 000058. Không có nó, một đơn nhiều nhà bán sẽ mở khóa hoa hồng của
	// cả đơn khi nhà bán đầu tiên giao xong.
	sellerID ids.ID

	creatorID ids.ID
	contentID ids.ID

	rate    types.BasisPoints
	base    money.Money
	benChiu BenChiu
	moHinh  MoHinhQuyCong
	taoLuc  time.Time
}

// TaoAttributionParams là dữ liệu quy công một dòng đơn.
type TaoAttributionParams struct {
	OrderID     ids.ID
	OrderLineID ids.ID
	SellerID    ids.ID
	CreatorID   ids.ID
	ContentID   ids.ID

	// Rate và Base ĐÓNG BĂNG tại đây (quy tắc 3).
	//
	// Base là tiền THỰC TRẢ của dòng, không phải giá niêm yết — xem chú
	// thích ở migration 000058.
	Rate types.BasisPoints
	Base money.Money

	BenChiu BenChiu
	Now     time.Time
}

// TaoAttribution dựng một bản quy công mới.
func TaoAttribution(p TaoAttributionParams) (*Attribution, error) {
	if p.CreatorID.IsZero() {
		return nil, ErrThieuCreator
	}
	if p.OrderLineID.IsZero() {
		return nil, ErrThieuDongDon
	}
	if p.Base.Amount() < 0 {
		return nil, fmt.Errorf("%w: %s", ErrTienGocAm, p.Base)
	}
	if !p.BenChiu.HopLe() {
		return nil, fmt.Errorf("%w: %q", ErrBenChiuLa, p.BenChiu)
	}

	id, err := ids.New(ids.PrefixAttribution)
	if err != nil {
		return nil, err
	}
	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Attribution{
		id:          id,
		orderID:     p.OrderID,
		orderLineID: p.OrderLineID,
		sellerID:    p.SellerID,
		creatorID:   p.CreatorID,
		contentID:   p.ContentID,
		rate:        p.Rate,
		base:        p.Base,
		benChiu:     p.BenChiu,
		moHinh:      MoHinhLastClick,
		taoLuc:      now,
	}, nil
}

// HoaHong là số tiền hoa hồng, TÍNH từ tỷ lệ và tiền gốc đã đóng băng.
//
// Tính ở đây chứ không nhận từ bên gọi: hai nguồn cho cùng một con số tiền
// sớm muộn sẽ lệch nhau. Và vì cả tỷ lệ lẫn tiền gốc đều đóng băng, phép
// tính này cho cùng kết quả mãi mãi — đó là điều kiện để đối soát lại được.
func (a *Attribution) HoaHong() money.Money {
	return a.base.ApplyRate(a.rate, money.RoundHalfUp)
}

// RestoreAttributionParams dựng lại từ kho lưu trữ. CHỈ dùng ở infrastructure.
type RestoreAttributionParams struct {
	ID          ids.ID
	OrderID     ids.ID
	OrderLineID ids.ID
	SellerID    ids.ID
	CreatorID   ids.ID
	ContentID   ids.ID
	Rate        types.BasisPoints
	Base        money.Money
	BenChiu     BenChiu
	MoHinh      MoHinhQuyCong
	TaoLuc      time.Time
}

func RestoreAttribution(p RestoreAttributionParams) *Attribution {
	return &Attribution{
		id:          p.ID,
		orderID:     p.OrderID,
		orderLineID: p.OrderLineID,
		sellerID:    p.SellerID,
		creatorID:   p.CreatorID,
		contentID:   p.ContentID,
		rate:        p.Rate,
		base:        p.Base,
		benChiu:     p.BenChiu,
		moHinh:      p.MoHinh,
		taoLuc:      p.TaoLuc,
	}
}

func (a *Attribution) ID() ids.ID              { return a.id }
func (a *Attribution) OrderID() ids.ID         { return a.orderID }
func (a *Attribution) OrderLineID() ids.ID     { return a.orderLineID }
func (a *Attribution) SellerID() ids.ID        { return a.sellerID }
func (a *Attribution) CreatorID() ids.ID       { return a.creatorID }
func (a *Attribution) ContentID() ids.ID       { return a.contentID }
func (a *Attribution) Rate() types.BasisPoints { return a.rate }
func (a *Attribution) Base() money.Money       { return a.base }
func (a *Attribution) BenChiu() BenChiu        { return a.benChiu }
func (a *Attribution) MoHinh() MoHinhQuyCong   { return a.moHinh }
func (a *Attribution) TaoLuc() time.Time       { return a.taoLuc }
