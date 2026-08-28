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
}

// DoiSoat là một đợt đối soát của một nhà bán.
type DoiSoat struct {
	id       ids.ID
	sellerID ids.ID

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
func (d *DoiSoat) XacNhan(now time.Time) error {
	if d.status != DoiSoatNhap {
		return ErrInvalidStatus
	}
	// KHÔNG cho xác nhận đợt không có gì để trả: xác nhận rồi mới phát
	// hiện không trả được là bắt người vận hành làm hai lần.
	if !d.net.IsPositive() {
		return ErrSettlementNotPayable
	}
	d.status = DoiSoatXacNhan
	d.confirmedAt = now
	d.updatedAt = now
	return nil
}

// DanhDauDaTra ghi nhận đã chuyển tiền.
func (d *DoiSoat) DanhDauDaTra(now time.Time) error {
	if d.status != DoiSoatXacNhan {
		return ErrInvalidStatus
	}
	d.status = DoiSoatDaTra
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
	PaidAt      time.Time
	Version     int64
	UpdatedAt   time.Time
}

// KhoiPhucDoiSoat dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func KhoiPhucDoiSoat(p KhoiPhucDoiSoatParams) *DoiSoat {
	return &DoiSoat{
		id: p.ID, sellerID: p.SellerID,
		periodStart: p.PeriodStart, periodEnd: p.PeriodEnd,
		status: p.Status, gross: p.Gross, deficit: p.Deficit, net: p.Net,
		dong: p.Dong, createdAt: p.CreatedAt, confirmedAt: p.ConfirmedAt,
		paidAt: p.PaidAt, version: p.Version, updatedAt: p.UpdatedAt,
	}
}
