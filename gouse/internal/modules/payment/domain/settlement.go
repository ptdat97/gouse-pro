package domain

import (
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrSettlementNotFound = errors.New("payment: không tìm thấy đợt đối soát")
	ErrInvalidStatus      = errors.New("payment: chuyển trạng thái không hợp lệ")
	ErrVersionConflict    = errors.New("payment: đợt đối soát vừa bị thay đổi, hãy đọc lại")
	ErrNoSettleableAmount = errors.New("payment: không có khoản nào để đối soát")

	// ErrSettlementNotPayable: đợt này KHÔNG chi trả được.
	//
	// Số thực chi bằng 0 hoặc âm — nhà bán đang nợ nền tảng nhiều hơn số
	// kiếm được trong kỳ. Không chuyển tiền âm; khoản nợ chuyển sang kỳ
	// sau (docs/04-modules/payment.md mục 6).
	ErrSettlementNotPayable = errors.New(
		"payment: đợt đối soát không có số dương để chi trả")

	// ErrThieuNguoiKy khi một bước của vòng đời không nêu ai ký.
	//
	// Tiền rời nền tảng thì phải có người chịu trách nhiệm, và vết kiểm
	// toán không tạo ngược được. Ràng buộc tương ứng ở tầng database là
	// `settlement_confirmed_needs_signer` — hàng rào này báo lỗi rõ
	// hơn, ràng buộc kia là chốt chặn cuối.
	ErrThieuNguoiKy = errors.New(
		"payment: bước này của đợt đối soát bắt buộc phải nêu người ký")
)

// TrangThaiDoiSoat là trạng thái một đợt đối soát.
type TrangThaiDoiSoat string

const (
	DoiSoatNhap    TrangThaiDoiSoat = "DRAFT"
	DoiSoatXacNhan TrangThaiDoiSoat = "CONFIRMED"
	DoiSoatDaTra   TrangThaiDoiSoat = "PAID"
)

// DongDoiSoat là một bút toán rút được đã gom vào đợt.
type DongDoiSoat struct {
	ID            ids.ID
	LedgerEntryID ids.ID
	Amount        money.Money

	// ReferenceType và ReferenceID: bút toán rút được này đến từ ĐÂU.
	//
	// Với `SELLER_RELEASE` thì đó là `FULFILLMENT_ORDER` — đơn thực hiện
	// mà nhà bán đã giao và đã hết hạn đổi trả.
	//
	// Thêm 23/09/2026. Không có hai trường này, một đợt đối soát chỉ là
	// một con số tổng: nhà bán không đối chiếu được với sổ sách của chính
	// họ, và đặc tả gọi đối soát không minh bạch là nguyên nhân tranh chấp
	// lớn nhất giữa nền tảng và nhà bán.
	ReferenceType string
	ReferenceID   ids.ID

	// CreatedAt là lúc khoản này chuyển sang rút được.
	CreatedAt time.Time
}

// DoiSoat là một đợt đối soát của một nhà bán.
type DoiSoat struct {
	id       ids.ID
	sellerID ids.ID

	// confirmedBy và paidBy là NGƯỜI KÝ ở mỗi bước. Rỗng = bước chưa xảy ra.
	confirmedBy ids.ID
	paidBy      ids.ID

	periodStart time.Time
	periodEnd   time.Time
	status      TrangThaiDoiSoat

	gross   money.Money
	deficit money.Money
	net     money.Money

	dong []DongDoiSoat

	createdAt   time.Time
	confirmedAt time.Time
	paidAt      time.Time
	version     int64
	updatedAt   time.Time
}

// TaoDoiSoatParams là dữ liệu tạo một đợt đối soát.
type TaoDoiSoatParams struct {
	SellerID    ids.ID
	PeriodStart time.Time
	PeriodEnd   time.Time

	// Dong là các bút toán RÚT ĐƯỢC chưa thuộc đợt nào.
	Dong []DongDoiSoat

	// Deficit là phần ÂM của tài khoản đang chờ, số DƯƠNG.
	//
	// Hoàn tiền thu hồi từ tài khoản đang chờ; khi tiền đã chuyển sang rút
	// được thì tài khoản ấy âm. Trừ phần âm ra khỏi số thực chi, nếu không
	// là trả cả khoản vừa hoàn cho khách.
	Deficit money.Money

	Now time.Time
}

// TaoDoiSoat gom các khoản rút được thành một đợt.
func TaoDoiSoat(p TaoDoiSoatParams) (*DoiSoat, error) {
	if len(p.Dong) == 0 {
		return nil, ErrNoSettleableAmount
	}

	gross := p.Dong[0].Amount
	for _, d := range p.Dong[1:] {
		cong, err := gross.Add(d.Amount)
		if err != nil {
			return nil, err
		}
		gross = cong
	}

	deficit := p.Deficit
	if deficit.IsZero() {
		var err error
		if deficit, err = money.New(0, gross.Currency()); err != nil {
			return nil, err
		}
	}

	// KẸP phần bị trừ về không quá tổng của đợt.
	//
	// Khoản nợ có thể lớn hơn tổng kỳ này — một nhà bán bán ít trong kỳ mà
	// hoàn nhiều từ kỳ trước. Không kẹp thì `net` âm, và "thực nhận
	// −50.000 ₫" là một dòng không ai biết phải làm gì với nó: nền tảng
	// không đòi tiền mặt ngược từ nhà bán.
	//
	// Phần chưa trừ hết KHÔNG mất đi: nó vẫn là số âm trên tài khoản đang
	// chờ, và kỳ sau lại được tính vào. Đây chính là cơ chế "nợ chuyển kỳ
	// sau" mà màn hình tiền của nhà bán đang hứa.
	if deficit.Amount() > gross.Amount() {
		deficit = gross
	}

	net, err := gross.Sub(deficit)
	if err != nil {
		return nil, err
	}

	return &DoiSoat{
		id: ids.MustNew(ids.PrefixSettlement), sellerID: p.SellerID,
		periodStart: p.PeriodStart, periodEnd: p.PeriodEnd,
		status: DoiSoatNhap,
		gross:  gross, deficit: deficit, net: net,
		dong:      p.Dong,
		createdAt: p.Now, updatedAt: p.Now,
	}, nil
}

// XacNhan chốt đợt đối soát, sẵn sàng chi trả.
//
// `nguoiKy` BẮT BUỘC: đây là bước biến một con số tính được thành một CAM KẾT
// với nhà bán, và một cam kết không ai ký là một cam kết không ai chịu trách
// nhiệm. Xem P3-89 và migration 000061.
func (d *DoiSoat) XacNhan(nguoiKy ids.ID, now time.Time) error {
	if d.status != DoiSoatNhap {
		return ErrInvalidStatus
	}
	if nguoiKy.IsZero() {
		return ErrThieuNguoiKy
	}
	// KHÔNG cho xác nhận đợt không có gì để trả: xác nhận rồi mới phát
	// hiện không trả được là bắt người vận hành làm hai lần.
	if !d.net.IsPositive() {
		return ErrSettlementNotPayable
	}
	d.status = DoiSoatXacNhan
	d.confirmedBy = nguoiKy
	d.confirmedAt = now
	d.updatedAt = now
	return nil
}

// DanhDauDaTra ghi nhận đã chuyển tiền.
//
// `nguoiKy` BẮT BUỘC, cùng lý do với `XacNhan` — và ở bước này nặng hơn: tiền
// đã RỜI nền tảng, nên đây là bản ghi duy nhất nói ai quyết định chuyển nó.
//
// # Vì sao KHÔNG chặn người ký bước 1 ký bước 2
//
// Nguyên tắc bốn mắt — người xác nhận không được là người đánh dấu đã trả —
// là hàng rào đúng cho một tổ chức có từ hai người vận hành tài chính. Nó
// KHÔNG được cài ở đây vì nó sẽ chặn hẳn đường chi trả của một đội một
// người, và dự án này chưa có đội vận hành thật.
//
// Thêm nó là MỘT dòng ở đây khi cần:
//
//	if nguoiKy == d.confirmedBy { return ErrCanHaiNguoiKy }
//
// Ghi ra để lần sau không ai phải suy lại, và để việc KHÔNG cài nó là một
// lựa chọn đọc được chứ không phải một chỗ bị quên. Xem P3-89.
func (d *DoiSoat) DanhDauDaTra(nguoiKy ids.ID, now time.Time) error {
	if d.status != DoiSoatXacNhan {
		return ErrInvalidStatus
	}
	if nguoiKy.IsZero() {
		return ErrThieuNguoiKy
	}
	d.status = DoiSoatDaTra
	d.paidBy = nguoiKy
	d.paidAt = now
	d.updatedAt = now
	return nil
}

func (d *DoiSoat) ID() ids.ID               { return d.id }
func (d *DoiSoat) SellerID() ids.ID         { return d.sellerID }
func (d *DoiSoat) PeriodStart() time.Time   { return d.periodStart }
func (d *DoiSoat) PeriodEnd() time.Time     { return d.periodEnd }
func (d *DoiSoat) Status() TrangThaiDoiSoat { return d.status }
func (d *DoiSoat) Gross() money.Money       { return d.gross }
func (d *DoiSoat) Deficit() money.Money     { return d.deficit }
func (d *DoiSoat) Net() money.Money         { return d.net }
func (d *DoiSoat) CreatedAt() time.Time     { return d.createdAt }
func (d *DoiSoat) ConfirmedAt() time.Time   { return d.confirmedAt }
func (d *DoiSoat) ConfirmedBy() ids.ID      { return d.confirmedBy }
func (d *DoiSoat) PaidBy() ids.ID           { return d.paidBy }
func (d *DoiSoat) PaidAt() time.Time        { return d.paidAt }
func (d *DoiSoat) Version() int64           { return d.version }
func (d *DoiSoat) UpdatedAt() time.Time     { return d.updatedAt }
func (d *DoiSoat) Dong() []DongDoiSoat      { return append([]DongDoiSoat(nil), d.dong...) }

// KhoiPhucDoiSoatParams dựng lại từ kho lưu trữ.
type KhoiPhucDoiSoatParams struct {
	ID          ids.ID
	SellerID    ids.ID
	PeriodStart time.Time
	PeriodEnd   time.Time
	Status      TrangThaiDoiSoat
	Gross       money.Money
	Deficit     money.Money
	Net         money.Money
	Dong        []DongDoiSoat
	CreatedAt   time.Time
	ConfirmedAt time.Time
	ConfirmedBy ids.ID
	PaidAt      time.Time
	PaidBy      ids.ID
	Version     int64
	UpdatedAt   time.Time
}

// KhoiPhucDoiSoat dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func KhoiPhucDoiSoat(p KhoiPhucDoiSoatParams) *DoiSoat {
	return &DoiSoat{
		id: p.ID, sellerID: p.SellerID,
		periodStart: p.PeriodStart, periodEnd: p.PeriodEnd,
		status: p.Status, gross: p.Gross, deficit: p.Deficit, net: p.Net,
		dong: p.Dong, createdAt: p.CreatedAt,
		confirmedAt: p.ConfirmedAt, confirmedBy: p.ConfirmedBy,
		paidAt: p.PaidAt, paidBy: p.PaidBy,
		version: p.Version, updatedAt: p.UpdatedAt,
	}
}
