// Package domain chứa mô hình nghiệp vụ của module pricing: bảng giá,
// khung giá ràng buộc seller, và lịch sử giá.
//
// Tầng này KHÔNG biết gì về database, HTTP hay JSON. Quy tắc R2 của
// cmd/archcheck cưỡng chế điều đó.
package domain

import (
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrNonPositivePrice = errors.New("pricing: giá phải lớn hơn 0")
	ErrMissingSKU       = errors.New("pricing: thiếu định danh SKU")
	ErrCurrencyMismatch = errors.New("pricing: các mức giá phải cùng đơn vị tiền tệ")
	ErrCompareAtTooLow  = errors.New("pricing: giá gạch ngang phải cao hơn giá bán")
	ErrInvalidPeriod    = errors.New("pricing: khoảng thời gian không hợp lệ")
)

// PriceType là loại giá.
//
// Thứ tự ưu tiên (mục 4 của đặc tả): Flash > Campaign > Clearance > Member > Base.
// CHỈ MỘT loại được áp dụng — không cộng dồn. Giảm giá thêm (mã giảm giá)
// thuộc module promotion và áp dụng SAU khi đã chọn giá.
type PriceType string

const (
	PriceTypeBase      PriceType = "BASE"
	PriceTypeMember    PriceType = "MEMBER"
	PriceTypeClearance PriceType = "CLEARANCE"
	PriceTypeCampaign  PriceType = "CAMPAIGN"
	PriceTypeFlash     PriceType = "FLASH"
)

// priority trả về độ ưu tiên, số lớn hơn thắng.
//
// Mã hóa thứ tự Ở ĐÂY thay vì rải rác trong các so sánh: nếu rải rác, mỗi
// chỗ sẽ hiểu thứ tự một kiểu và khách sẽ thấy giá khác nhau giữa trang
// danh sách và trang chi tiết.
func (t PriceType) priority() int {
	switch t {
	case PriceTypeFlash:
		return 5
	case PriceTypeCampaign:
		return 4
	case PriceTypeClearance:
		return 3
	case PriceTypeMember:
		return 2
	case PriceTypeBase:
		return 1
	}
	return 0
}

func (t PriceType) valid() bool { return t.priority() > 0 }

// RequiresPeriod cho biết loại giá này có bắt buộc thời hạn không.
//
// Giá flash và giá chiến dịch KHÔNG được vô thời hạn: giá flash quên tắt
// sẽ bán lỗ vô hạn, và đó là loại lỗi không ai phát hiện cho tới khi đối
// soát cuối tháng.
func (t PriceType) RequiresPeriod() bool {
	return t == PriceTypeFlash || t == PriceTypeCampaign
}

// Period là khoảng thời gian hiệu lực của một mức giá.
//
// From rỗng = có hiệu lực ngay. To rỗng = vô thời hạn.
type Period struct {
	From time.Time
	To   time.Time
}

// IsOpenEnded cho biết khoảng thời gian không có điểm kết thúc.
func (p Period) IsOpenEnded() bool { return p.To.IsZero() }

// Contains cho biết thời điểm t có nằm trong khoảng không.
//
// Biên: bao gồm From, KHÔNG bao gồm To. Cùng quy ước với mọi khoảng thời
// gian khác trong hệ thống — hai giá liền kề không được cùng hiệu lực tại
// một thời điểm.
func (p Period) Contains(t time.Time) bool {
	if !p.From.IsZero() && t.Before(p.From) {
		return false
	}
	if !p.To.IsZero() && !t.Before(p.To) {
		return false
	}
	return true
}

func (p Period) valid() bool {
	if p.From.IsZero() || p.To.IsZero() {
		return true
	}
	return p.From.Before(p.To)
}

// Price là một mức giá của một SKU.
//
// Giá gắn với SKU (không phải Product hay Variant) vì size khác nhau có
// thể có giá khác nhau — quần size lớn tốn nhiều vải hơn.
//
// LƯU Ý RANH GIỚI (mục 3 của đặc tả): đây là giá của NỀN TẢNG cho own brand.
// Giá của seller marketplace nằm ở marketplace.Offer.price — seller tự đặt,
// pricing chỉ cung cấp khung ràng buộc.
type Price struct {
	id    ids.ID
	skuID ids.ID

	priceType PriceType
	amount    money.Money

	// compareAt là giá gạch ngang để hiển thị mức giảm.
	//
	// Bằng 0 nghĩa là không hiển thị. Phải CAO HƠN giá bán, nếu không thì
	// "giảm giá" hiển thị thành số âm.
	compareAt money.Money

	period Period

	// customerTier chỉ dùng cho giá thành viên.
	customerTier string

	// campaignID chỉ dùng cho giá chiến dịch và giá flash.
	campaignID ids.ID

	active bool

	createdAt time.Time
	updatedAt time.Time
}

type NewPriceParams struct {
	SKUID        ids.ID
	PriceType    PriceType
	Amount       money.Money
	CompareAt    money.Money
	Period       Period
	CustomerTier string
	CampaignID   ids.ID
	Now          time.Time
}

// NewPrice tạo mức giá mới.
func NewPrice(p NewPriceParams) (*Price, error) {
	if p.SKUID.IsZero() {
		return nil, ErrMissingSKU
	}

	// Quy tắc 1: giá > 0. Giá 0 không phải "miễn phí" mà gần như luôn là
	// lỗi nhập liệu hoặc lỗi chuyển đổi đơn vị tiền tệ.
	if !p.Amount.IsPositive() {
		return nil, ErrNonPositivePrice
	}

	priceType := p.PriceType
	if priceType == "" {
		priceType = PriceTypeBase
	}
	if !priceType.valid() {
		return nil, errors.New("pricing: loại giá không hợp lệ: " + string(priceType))
	}

	if !p.Period.valid() {
		return nil, ErrInvalidPeriod
	}

	// Giá flash/chiến dịch quên tắt sẽ bán lỗ cho tới khi có người phát hiện.
	if priceType.RequiresPeriod() && p.Period.IsOpenEnded() {
		return nil, errors.New("pricing: giá " + string(priceType) + " phải có thời hạn kết thúc")
	}

	// Quy tắc 5: giá luôn kèm đơn vị tiền tệ. So sánh giá khác đơn vị tiền
	// tệ là vô nghĩa và sẽ cho kết quả sai một cách âm thầm.
	if !p.CompareAt.IsZero() {
		if p.CompareAt.Currency() != p.Amount.Currency() {
			return nil, ErrCurrencyMismatch
		}
		// Giá gạch ngang thấp hơn giá bán sẽ hiển thị "giảm -20%".
		if !p.Amount.LessThan(p.CompareAt) {
			return nil, ErrCompareAtTooLow
		}
	}

	id, err := ids.New(ids.PrefixPrice)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Price{
		id:           id,
		skuID:        p.SKUID,
		priceType:    priceType,
		amount:       p.Amount,
		compareAt:    p.CompareAt,
		period:       p.Period,
		customerTier: p.CustomerTier,
		campaignID:   p.CampaignID,
		active:       true,
		createdAt:    now,
		updatedAt:    now,
	}, nil
}

// RestorePriceParams dựng lại mức giá TỪ KHO LƯU TRỮ.
type RestorePriceParams struct {
	ID           ids.ID
	SKUID        ids.ID
	PriceType    PriceType
	Amount       money.Money
	CompareAt    money.Money
	Period       Period
	CustomerTier string
	CampaignID   ids.ID
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RestorePrice dựng lại mà KHÔNG kiểm tra.
//
// Dữ liệu đã lưu từng hợp lệ theo luật lúc đó. Kiểm tra lại lúc đọc sẽ làm
// hỏng chức năng đọc dữ liệu cũ khi luật đổi — phải xử lý bằng migration.
//
// CHỈ tầng infrastructure được gọi hàm này.
func RestorePrice(p RestorePriceParams) *Price {
	return &Price{
		id:           p.ID,
		skuID:        p.SKUID,
		priceType:    p.PriceType,
		amount:       p.Amount,
		compareAt:    p.CompareAt,
		period:       p.Period,
		customerTier: p.CustomerTier,
		campaignID:   p.CampaignID,
		active:       p.Active,
		createdAt:    p.CreatedAt,
		updatedAt:    p.UpdatedAt,
	}
}

func (p *Price) ID() ids.ID             { return p.id }
func (p *Price) SKUID() ids.ID          { return p.skuID }
func (p *Price) Type() PriceType        { return p.priceType }
func (p *Price) Amount() money.Money    { return p.amount }
func (p *Price) CompareAt() money.Money { return p.compareAt }
func (p *Price) Period() Period         { return p.period }
func (p *Price) CustomerTier() string   { return p.customerTier }
func (p *Price) CampaignID() ids.ID     { return p.campaignID }
func (p *Price) IsActive() bool         { return p.active }
func (p *Price) CreatedAt() time.Time   { return p.createdAt }
func (p *Price) UpdatedAt() time.Time   { return p.updatedAt }

// HasDiscount cho biết có hiển thị mức giảm không.
func (p *Price) HasDiscount() bool {
	return !p.compareAt.IsZero() && p.amount.LessThan(p.compareAt)
}

// DiscountBasisPoints tính mức giảm theo phần vạn (basis points).
//
// Dùng phần vạn thay vì phần trăm số thực: 1/3 không biểu diễn chính xác
// bằng float, và làm tròn phần trăm khiến "giảm 33%" hiển thị khác nhau ở
// các trang khác nhau.
//
// Trả 0 nếu không có giảm giá.
func (p *Price) DiscountBasisPoints() int64 {
	if !p.HasDiscount() {
		return 0
	}
	saved := p.compareAt.Amount() - p.amount.Amount()
	return saved * 10000 / p.compareAt.Amount()
}

// AppliesAt cho biết mức giá này có hiệu lực tại thời điểm t không.
func (p *Price) AppliesAt(t time.Time) bool {
	return p.active && p.period.Contains(t)
}

// AppliesTo cho biết mức giá có áp dụng cho ngữ cảnh của khách không.
//
// Giá thành viên chỉ áp cho đúng hạng; giá chiến dịch chỉ áp khi khách đến
// từ chiến dịch đó. Không kiểm tra thì mọi khách đều nhận giá thành viên
// và chương trình thành viên mất hết ý nghĩa.
func (p *Price) AppliesTo(tier string, campaignID ids.ID) bool {
	if p.customerTier != "" && p.customerTier != tier {
		return false
	}
	if !p.campaignID.IsZero() && p.campaignID != campaignID {
		return false
	}
	return true
}

// Deactivate ngừng áp dụng mức giá.
//
// KHÔNG xóa: quy tắc 3 yêu cầu mọi thay đổi giá phải ghi lịch sử, và lịch
// sử cần cho việc phát hiện thao túng giá (tăng rồi giảm giả vờ khuyến mãi).
func (p *Price) Deactivate(now time.Time) {
	p.active = false
	p.touch(now)
}

func (p *Price) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	p.updatedAt = now
}

// SelectBest chọn mức giá được áp dụng trong số các mức giá cho trước.
//
// Quy tắc 2: CHỈ MỘT giá được áp dụng, không cộng dồn.
//
// Thứ tự: ưu tiên loại giá trước; cùng loại thì chọn GIÁ THẤP HƠN cho khách.
// Chọn giá thấp hơn khi hòa là quyết định có chủ đích: nếu cấu hình trùng
// lặp do lỗi vận hành, khách được lợi chứ không phải bị thiệt.
//
// Trả nil nếu không mức giá nào áp dụng được.
func SelectBest(prices []*Price, at time.Time, tier string, campaignID ids.ID) *Price {
	var best *Price
	for _, p := range prices {
		if p == nil || !p.AppliesAt(at) || !p.AppliesTo(tier, campaignID) {
			continue
		}
		if best == nil {
			best = p
			continue
		}

		switch {
		case p.priceType.priority() > best.priceType.priority():
			best = p
		case p.priceType.priority() == best.priceType.priority():
			if p.amount.LessThan(best.amount) {
				best = p
			}
		}
	}
	return best
}
