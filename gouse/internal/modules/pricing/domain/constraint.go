package domain

import (
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrMinAboveMax     = errors.New("pricing: giá tối thiểu không được cao hơn giá tối đa")
	ErrEmptyConstraint = errors.New("pricing: khung giá phải có ít nhất một giới hạn")
)

// ViolationCode là mã lý do giá seller bị từ chối.
//
// Máy đọc được và ổn định — giao diện dùng để hiển thị thông báo phù hợp,
// không parse chuỗi tiếng Việt.
type ViolationCode string

const (
	ViolationNone          ViolationCode = ""
	ViolationBelowMin      ViolationCode = "BELOW_MINIMUM"
	ViolationAboveMax      ViolationCode = "ABOVE_MAXIMUM"
	ViolationWrongCurrency ViolationCode = "CURRENCY_MISMATCH"
	ViolationNotPositive   ViolationCode = "PRICE_NOT_POSITIVE"

	// ViolationSuspicious là CẢNH BÁO, không phải từ chối.
	//
	// Giá lệch xa thị trường thường là lỗi nhập liệu (thiếu/thừa một số 0),
	// nhưng cũng có thể là hàng thanh lý thật. Chặn thẳng sẽ cản trở việc
	// bán hàng hợp lệ, nên chỉ đánh dấu để người rà soát xem.
	ViolationSuspicious ViolationCode = "SUSPICIOUS_PRICE"
)

// CheckResult là kết quả kiểm tra giá seller.
type CheckResult struct {
	// Allowed cho biết giá có được chấp nhận không.
	Allowed bool

	Code ViolationCode

	// Message giải thích cho người dùng.
	Message string

	// NeedsReview đánh dấu giá cần người rà soát dù vẫn được chấp nhận.
	NeedsReview bool

	// Min và Max là khung giá, để giao diện hiển thị "giá phải từ X đến Y".
	Min money.Money
	Max money.Money
}

// PriceConstraint là khung giá ràng buộc seller cho một SKU.
//
// VÌ SAO CẦN (mục 3 của đặc tả):
//
//	Giá tối thiểu — chống bán phá giá VÀ chống lỗi nhập liệu.
//	                Seller gõ thiếu một số 0 sẽ bán 10.000đ thay vì 100.000đ;
//	                không có khung giá thì lỗi này chỉ lộ ra sau khi đã bán.
//	Giá tối đa    — chống thổi giá làm hỏng uy tín nền tảng.
//
// Đây KHÔNG phải chỗ đặt giá. Seller tự đặt giá trong marketplace.Offer;
// khung này chỉ nói giá đó có được chấp nhận không.
type PriceConstraint struct {
	id    ids.ID
	skuID ids.ID

	// minPrice và maxPrice có thể bằng 0 nghĩa là không giới hạn phía đó.
	minPrice money.Money
	maxPrice money.Money

	// referencePrice là giá tham chiếu thị trường, dùng để phát hiện giá
	// bất thường. Bằng 0 nghĩa là chưa có dữ liệu tham chiếu.
	referencePrice money.Money

	// suspiciousBelowBP là ngưỡng cảnh báo tính theo phần vạn so với giá
	// tham chiếu. Ví dụ 5000 = thấp hơn 50% giá tham chiếu thì cảnh báo.
	suspiciousBelowBP int64

	createdAt time.Time
	updatedAt time.Time
}

type NewConstraintParams struct {
	SKUID             ids.ID
	MinPrice          money.Money
	MaxPrice          money.Money
	ReferencePrice    money.Money
	SuspiciousBelowBP int64
	Now               time.Time
}

// mặc định: thấp hơn 50% giá tham chiếu thì đánh dấu để rà soát.
const defaultSuspiciousBelowBP = 5000

// NewPriceConstraint tạo khung giá.
func NewPriceConstraint(p NewConstraintParams) (*PriceConstraint, error) {
	if p.SKUID.IsZero() {
		return nil, ErrMissingSKU
	}

	// Khung giá không có giới hạn nào là khung giá vô nghĩa — nó tạo cảm
	// giác an toàn giả rằng giá đang được kiểm soát.
	if p.MinPrice.IsZero() && p.MaxPrice.IsZero() {
		return nil, ErrEmptyConstraint
	}

	if !p.MinPrice.IsZero() && !p.MaxPrice.IsZero() {
		if p.MinPrice.Currency() != p.MaxPrice.Currency() {
			return nil, ErrCurrencyMismatch
		}
		// Min > Max thì KHÔNG giá nào hợp lệ — seller không đăng bán được
		// và sẽ không hiểu vì sao.
		if p.MaxPrice.LessThan(p.MinPrice) {
			return nil, ErrMinAboveMax
		}
	}

	if !p.ReferencePrice.IsZero() && !p.MinPrice.IsZero() &&
		p.ReferencePrice.Currency() != p.MinPrice.Currency() {
		return nil, ErrCurrencyMismatch
	}

	id, err := ids.New(ids.PrefixPriceConstraint)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	threshold := p.SuspiciousBelowBP
	if threshold <= 0 {
		threshold = defaultSuspiciousBelowBP
	}

	return &PriceConstraint{
		id:                id,
		skuID:             p.SKUID,
		minPrice:          p.MinPrice,
		maxPrice:          p.MaxPrice,
		referencePrice:    p.ReferencePrice,
		suspiciousBelowBP: threshold,
		createdAt:         now,
		updatedAt:         now,
	}, nil
}

// RestoreConstraintParams dựng lại khung giá từ kho lưu trữ.
type RestoreConstraintParams struct {
	ID                ids.ID
	SKUID             ids.ID
	MinPrice          money.Money
	MaxPrice          money.Money
	ReferencePrice    money.Money
	SuspiciousBelowBP int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// RestorePriceConstraint dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestorePriceConstraint(p RestoreConstraintParams) *PriceConstraint {
	return &PriceConstraint{
		id:                p.ID,
		skuID:             p.SKUID,
		minPrice:          p.MinPrice,
		maxPrice:          p.MaxPrice,
		referencePrice:    p.ReferencePrice,
		suspiciousBelowBP: p.SuspiciousBelowBP,
		createdAt:         p.CreatedAt,
		updatedAt:         p.UpdatedAt,
	}
}

func (c *PriceConstraint) ID() ids.ID                  { return c.id }
func (c *PriceConstraint) SKUID() ids.ID               { return c.skuID }
func (c *PriceConstraint) MinPrice() money.Money       { return c.minPrice }
func (c *PriceConstraint) MaxPrice() money.Money       { return c.maxPrice }
func (c *PriceConstraint) ReferencePrice() money.Money { return c.referencePrice }

// SuspiciousBelowBP là ngưỡng cảnh báo theo phần vạn.
//
// Có getter để tầng infrastructure lưu lại được giá trị đã chuẩn hóa trong
// constructor. Không có getter thì kho lưu 0, và khi đọc lên ngưỡng thành
// 0 — cảnh báo giá bất thường sẽ im lặng ngừng hoạt động.
func (c *PriceConstraint) SuspiciousBelowBP() int64 { return c.suspiciousBelowBP }
func (c *PriceConstraint) CreatedAt() time.Time     { return c.createdAt }
func (c *PriceConstraint) UpdatedAt() time.Time     { return c.updatedAt }

// Check kiểm tra giá seller có nằm trong khung không.
//
// Quy tắc 4: giá seller phải trong khung ràng buộc.
//
// Trả về CẢ lý do lẫn khung giá, không chỉ true/false — seller cần biết
// "giá phải từ 80.000đ đến 200.000đ" để sửa, chứ không phải "giá không hợp lệ".
func (c *PriceConstraint) Check(price money.Money) CheckResult {
	base := CheckResult{Min: c.minPrice, Max: c.maxPrice}

	if !price.IsPositive() {
		base.Code = ViolationNotPositive
		base.Message = "Giá phải lớn hơn 0"
		return base
	}

	// So sánh giá khác đơn vị tiền tệ cho kết quả sai một cách âm thầm,
	// nên phải chặn tường minh.
	if ref := c.currency(); ref != "" && price.Currency() != ref {
		base.Code = ViolationWrongCurrency
		base.Message = "Giá phải dùng đơn vị tiền tệ " + string(ref)
		return base
	}

	if !c.minPrice.IsZero() && price.LessThan(c.minPrice) {
		base.Code = ViolationBelowMin
		base.Message = "Giá thấp hơn mức tối thiểu " + c.minPrice.String()
		return base
	}

	if !c.maxPrice.IsZero() && c.maxPrice.LessThan(price) {
		base.Code = ViolationAboveMax
		base.Message = "Giá cao hơn mức tối đa " + c.maxPrice.String()
		return base
	}

	base.Allowed = true

	// Giá hợp lệ nhưng lệch xa giá tham chiếu: CHẤP NHẬN nhưng đánh dấu.
	// Chặn thẳng sẽ cản trở hàng thanh lý thật.
	if c.isSuspicious(price) {
		base.Code = ViolationSuspicious
		base.NeedsReview = true
		base.Message = "Giá thấp bất thường so với thị trường, cần rà soát"
	}
	return base
}

// isSuspicious cho biết giá có thấp bất thường so với giá tham chiếu không.
func (c *PriceConstraint) isSuspicious(price money.Money) bool {
	if c.referencePrice.IsZero() || !c.referencePrice.IsPositive() {
		return false
	}
	// Ngưỡng = giá tham chiếu × (10000 − suspiciousBelowBP) / 10000.
	// Tính bằng số nguyên để không có sai số dấu phẩy động.
	threshold := c.referencePrice.Amount() * (10000 - c.suspiciousBelowBP) / 10000
	return price.Amount() < threshold
}

// currency trả về đơn vị tiền tệ của khung giá.
func (c *PriceConstraint) currency() money.Currency {
	if !c.minPrice.IsZero() {
		return c.minPrice.Currency()
	}
	if !c.maxPrice.IsZero() {
		return c.maxPrice.Currency()
	}
	return ""
}

// Update cập nhật khung giá.
func (c *PriceConstraint) Update(min, max money.Money, now time.Time) error {
	if min.IsZero() && max.IsZero() {
		return ErrEmptyConstraint
	}
	if !min.IsZero() && !max.IsZero() {
		if min.Currency() != max.Currency() {
			return ErrCurrencyMismatch
		}
		if max.LessThan(min) {
			return ErrMinAboveMax
		}
	}
	c.minPrice = min
	c.maxPrice = max
	c.touch(now)
	return nil
}

func (c *PriceConstraint) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.updatedAt = now
}
