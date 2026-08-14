// Package domain chứa mô hình nghiệp vụ của module promotion.
//
// # Vấn đề cốt lõi: ai chịu chi phí khuyến mãi
//
//	Khách dùng mã giảm 50.000đ cho đơn của Seller A. AI CHỊU 50.000đ?
//
// Không trả lời được câu này thì không tính được seller thực nhận bao
// nhiêu, và đối soát cuối tháng sẽ lệch đúng bằng tổng tiền khuyến mãi.
// Vì vậy CostBearer là trường BẮT BUỘC ngay từ MVP.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

var (
	ErrNotFound = errors.New("promotion: không tìm thấy khuyến mãi")

	ErrCouponNotFound = errors.New("promotion: mã giảm giá không tồn tại")

	// ErrCouponInactive là mã đã bị tắt.
	ErrCouponInactive = errors.New("promotion: mã giảm giá đã bị vô hiệu")

	// ErrNotStarted là khuyến mãi chưa tới ngày bắt đầu.
	ErrNotStarted = errors.New("promotion: khuyến mãi chưa bắt đầu")

	ErrExpired = errors.New("promotion: khuyến mãi đã hết hạn")

	// ErrNotActive là khuyến mãi không ở trạng thái áp dụng được.
	ErrNotActive = errors.New("promotion: khuyến mãi không còn hiệu lực")

	// ErrBelowMinimum là đơn hàng chưa đạt giá trị tối thiểu.
	ErrBelowMinimum = errors.New("promotion: đơn hàng chưa đạt giá trị tối thiểu")

	// ErrUsageLimitReached là đã hết lượt dùng toàn cục.
	ErrUsageLimitReached = errors.New("promotion: mã giảm giá đã hết lượt sử dụng")

	// ErrCustomerLimitReached là khách đã dùng hết lượt của mình.
	ErrCustomerLimitReached = errors.New("promotion: bạn đã dùng hết lượt của mã này")

	// ErrBudgetExhausted là đã tiêu hết ngân sách khuyến mãi.
	ErrBudgetExhausted = errors.New("promotion: ngân sách khuyến mãi đã hết")

	// ErrWrongCustomer là mã riêng của khách khác.
	ErrWrongCustomer = errors.New("promotion: mã giảm giá không dành cho bạn")

	// ErrWrongSeller là mã chỉ áp cho gian hàng khác.
	ErrWrongSeller = errors.New("promotion: mã giảm giá không áp dụng cho gian hàng này")

	ErrInvalidInput = errors.New("promotion: dữ liệu không hợp lệ")

	ErrVersionConflict = errors.New("promotion: dữ liệu đã bị thay đổi bởi thao tác khác")

	// ErrAlreadyUsed là mã đã được ghi nhận cho đơn này.
	ErrAlreadyUsed = errors.New("promotion: mã đã được áp cho đơn hàng này")
)

// Kind là loại khuyến mãi.
type Kind string

const (
	KindCoupon       Kind = "COUPON"
	KindFreeShipping Kind = "FREE_SHIPPING"

	// Các loại dưới đây CHƯA CÀI ĐẶT ở MVP — xem promotion.md mục 11.
	KindAuto         Kind = "AUTO"
	KindBuyXGetY     Kind = "BUY_X_GET_Y"
	KindOutfitBundle Kind = "OUTFIT_BUNDLE"
)

// DiscountType là cách tính giảm.
type DiscountType string

const (
	// DiscountPercentage giảm theo phần trăm, dùng DiscountBPS.
	DiscountPercentage DiscountType = "PERCENTAGE"

	// DiscountFixed giảm số tiền cố định, dùng DiscountAmount.
	DiscountFixed DiscountType = "FIXED"

	// DiscountFreeShip miễn phí vận chuyển.
	DiscountFreeShip DiscountType = "FREE_SHIP"
)

// CostBearer là bên chịu chi phí khuyến mãi.
type CostBearer string

const (
	// BearerPlatform: nền tảng chịu, trừ vào chi phí marketing.
	BearerPlatform CostBearer = "PLATFORM"

	// BearerSeller: seller chịu, trừ vào số tiền seller nhận.
	BearerSeller CostBearer = "SELLER"

	// BearerShared: chia theo tỷ lệ thỏa thuận (Phase 2).
	BearerShared CostBearer = "SHARED"
)

// Status là trạng thái khuyến mãi.
type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusActive    Status = "ACTIVE"
	StatusPaused    Status = "PAUSED"
	StatusExhausted Status = "EXHAUSTED"
	StatusExpired   Status = "EXPIRED"
)

// Promotion là một chương trình khuyến mãi.
type Promotion struct {
	id ids.ID

	name        string
	description string

	kind         Kind
	discountType DiscountType

	// discountBPS là điểm cơ bản: 1000 = 10%.
	//
	// SỐ NGUYÊN, không phải số thực — xem kernel/money.
	discountBPS    types.BasisPoints
	discountAmount money.Money

	// maxDiscountAmount là CHẶN TRÊN cho giảm theo phần trăm.
	//
	// "Giảm 50%, tối đa 100.000đ" là cách viết thông thường của khuyến mãi
	// thật. Không có nó, một đơn 10 triệu được giảm 5 triệu.
	//
	// Zero nghĩa là không giới hạn.
	maxDiscountAmount money.Money

	minOrderAmount money.Money

	costBearer       CostBearer
	platformShareBPS types.BasisPoints
	sellerShareBPS   types.BasisPoints

	// sellerID khác rỗng nghĩa là khuyến mãi CHỈ áp cho gian hàng đó.
	sellerID ids.ID

	maxUses            int
	maxUsesPerCustomer int
	usedCount          int

	maxBudget  money.Money
	usedBudget money.Money

	status    Status
	startsAt  time.Time
	endsAt    time.Time
	version   int
	createdAt time.Time
	updatedAt time.Time
}

// NewPromotionParams là dữ liệu tạo khuyến mãi.
type NewPromotionParams struct {
	Name        string
	Description string

	Kind         Kind
	DiscountType DiscountType

	DiscountBPS       types.BasisPoints
	DiscountAmount    money.Money
	MaxDiscountAmount money.Money
	MinOrderAmount    money.Money

	CostBearer       CostBearer
	PlatformShareBPS types.BasisPoints
	SellerShareBPS   types.BasisPoints
	SellerID         ids.ID

	MaxUses            int
	MaxUsesPerCustomer int
	MaxBudget          money.Money

	StartsAt time.Time
	EndsAt   time.Time

	Currency money.Currency
	Now      time.Time
}

// NewPromotion tạo khuyến mãi mới.
//
// Trạng thái ban đầu là DRAFT: khuyến mãi vừa tạo KHÔNG áp được ngay.
// Một mã giảm 90% do gõ nhầm sẽ có hiệu lực tức thì nếu mặc định là
// ACTIVE — và không có bước nào để ai đó nhìn lại trước khi khách dùng.
func NewPromotion(p NewPromotionParams) (*Promotion, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, ErrInvalidInput
	}

	if p.StartsAt.IsZero() || p.EndsAt.IsZero() || !p.EndsAt.After(p.StartsAt) {
		return nil, ErrInvalidInput
	}

	currency := p.Currency
	if currency == "" {
		currency = money.VND
	}

	if err := validateDiscount(p); err != nil {
		return nil, err
	}

	bearer := p.CostBearer
	if bearer == "" {
		bearer = BearerPlatform
	}

	platform, seller, err := resolveShares(bearer, p.PlatformShareBPS, p.SellerShareBPS)
	if err != nil {
		return nil, err
	}

	if p.MaxUses < 0 || p.MaxUsesPerCustomer < 0 {
		return nil, ErrInvalidInput
	}

	return &Promotion{
		id:                ids.MustNew(ids.PrefixPromotion),
		name:              name,
		description:       strings.TrimSpace(p.Description),
		kind:              p.Kind,
		discountType:      p.DiscountType,
		discountBPS:       p.DiscountBPS,
		discountAmount:    zeroIfUnset(p.DiscountAmount, currency),
		maxDiscountAmount: zeroIfUnset(p.MaxDiscountAmount, currency),
		minOrderAmount:    zeroIfUnset(p.MinOrderAmount, currency),
		costBearer:        bearer,
		platformShareBPS:  platform,
		sellerShareBPS:    seller,
		sellerID:          p.SellerID,

		maxUses:            p.MaxUses,
		maxUsesPerCustomer: p.MaxUsesPerCustomer,
		maxBudget:          zeroIfUnset(p.MaxBudget, currency),
		usedBudget:         money.Zero(currency),

		status:    StatusDraft,
		startsAt:  p.StartsAt,
		endsAt:    p.EndsAt,
		version:   1,
		createdAt: p.Now,
		updatedAt: p.Now,
	}, nil
}

// validateDiscount kiểm tra cấu hình giảm giá có nghĩa không.
//
// Thiếu bước này thì tạo được khuyến mãi "giảm 0đ": nó qua mọi kiểm tra,
// khách áp được, và không giảm gì cả — rồi bộ phận hỗ trợ nhận khiếu nại
// "mã của tôi không hoạt động".
func validateDiscount(p NewPromotionParams) error {
	switch p.DiscountType {
	case DiscountPercentage:
		if p.DiscountBPS.Value() <= 0 || p.DiscountBPS.Value() > 10000 {
			return ErrInvalidInput
		}
	case DiscountFixed:
		if p.DiscountAmount.IsZero() || p.DiscountAmount.IsNegative() {
			return ErrInvalidInput
		}
	case DiscountFreeShip:
		// Không cần tham số nào.
	default:
		return ErrInvalidInput
	}
	return nil
}

// resolveShares xác định tỷ lệ chia chi phí.
//
// Hai tỷ lệ PHẢI cộng đúng 100%. Lệch một điểm cơ bản là một khoản tiền
// KHÔNG AI CHỊU, và đối soát cuối tháng sẽ không khớp đúng khoản đó.
func resolveShares(
	bearer CostBearer, platform, seller types.BasisPoints,
) (types.BasisPoints, types.BasisPoints, error) {
	switch bearer {
	case BearerPlatform:
		return types.MustNewBasisPoints(10000), types.MustNewBasisPoints(0), nil
	case BearerSeller:
		return types.MustNewBasisPoints(0), types.MustNewBasisPoints(10000), nil
	case BearerShared:
		if platform.Value()+seller.Value() != 10000 {
			return types.BasisPoints{}, types.BasisPoints{}, ErrInvalidInput
		}
		return platform, seller, nil
	}
	return types.BasisPoints{}, types.BasisPoints{}, ErrInvalidInput
}

func zeroIfUnset(m money.Money, c money.Currency) money.Money {
	if m.Currency() == "" {
		return money.Zero(c)
	}
	return m
}

// RestorePromotionParams dựng lại khuyến mãi từ kho lưu trữ.
type RestorePromotionParams struct {
	ID                 ids.ID
	Name               string
	Description        string
	Kind               Kind
	DiscountType       DiscountType
	DiscountBPS        types.BasisPoints
	DiscountAmount     money.Money
	MaxDiscountAmount  money.Money
	MinOrderAmount     money.Money
	CostBearer         CostBearer
	PlatformShareBPS   types.BasisPoints
	SellerShareBPS     types.BasisPoints
	SellerID           ids.ID
	MaxUses            int
	MaxUsesPerCustomer int
	UsedCount          int
	MaxBudget          money.Money
	UsedBudget         money.Money
	Status             Status
	StartsAt           time.Time
	EndsAt             time.Time
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func RestorePromotion(p RestorePromotionParams) *Promotion {
	return &Promotion{
		id:                 p.ID,
		name:               p.Name,
		description:        p.Description,
		kind:               p.Kind,
		discountType:       p.DiscountType,
		discountBPS:        p.DiscountBPS,
		discountAmount:     p.DiscountAmount,
		maxDiscountAmount:  p.MaxDiscountAmount,
		minOrderAmount:     p.MinOrderAmount,
		costBearer:         p.CostBearer,
		platformShareBPS:   p.PlatformShareBPS,
		sellerShareBPS:     p.SellerShareBPS,
		sellerID:           p.SellerID,
		maxUses:            p.MaxUses,
		maxUsesPerCustomer: p.MaxUsesPerCustomer,
		usedCount:          p.UsedCount,
		maxBudget:          p.MaxBudget,
		usedBudget:         p.UsedBudget,
		status:             p.Status,
		startsAt:           p.StartsAt,
		endsAt:             p.EndsAt,
		version:            p.Version,
		createdAt:          p.CreatedAt,
		updatedAt:          p.UpdatedAt,
	}
}

func (p *Promotion) ID() ids.ID                       { return p.id }
func (p *Promotion) Name() string                     { return p.name }
func (p *Promotion) Description() string              { return p.description }
func (p *Promotion) Kind() Kind                       { return p.kind }
func (p *Promotion) DiscountType() DiscountType       { return p.discountType }
func (p *Promotion) DiscountBPS() types.BasisPoints   { return p.discountBPS }
func (p *Promotion) DiscountAmount() money.Money      { return p.discountAmount }
func (p *Promotion) MaxDiscountAmount() money.Money   { return p.maxDiscountAmount }
func (p *Promotion) MinOrderAmount() money.Money      { return p.minOrderAmount }
func (p *Promotion) CostBearer() CostBearer           { return p.costBearer }
func (p *Promotion) PlatformShare() types.BasisPoints { return p.platformShareBPS }
func (p *Promotion) SellerShare() types.BasisPoints   { return p.sellerShareBPS }
func (p *Promotion) SellerID() ids.ID                 { return p.sellerID }
func (p *Promotion) MaxUses() int                     { return p.maxUses }
func (p *Promotion) MaxUsesPerCustomer() int          { return p.maxUsesPerCustomer }
func (p *Promotion) UsedCount() int                   { return p.usedCount }
func (p *Promotion) MaxBudget() money.Money           { return p.maxBudget }
func (p *Promotion) UsedBudget() money.Money          { return p.usedBudget }
func (p *Promotion) Status() Status                   { return p.status }
func (p *Promotion) StartsAt() time.Time              { return p.startsAt }
func (p *Promotion) EndsAt() time.Time                { return p.endsAt }
func (p *Promotion) Version() int                     { return p.version }
func (p *Promotion) CreatedAt() time.Time             { return p.createdAt }
func (p *Promotion) UpdatedAt() time.Time             { return p.updatedAt }

// Activate bật khuyến mãi.
func (p *Promotion) Activate(now time.Time) error {
	if p.status == StatusExpired || p.status == StatusExhausted {
		return ErrNotActive
	}
	p.status = StatusActive
	p.touch(now)
	return nil
}

// Pause tạm dừng khuyến mãi.
func (p *Promotion) Pause(now time.Time) {
	if p.status == StatusActive {
		p.status = StatusPaused
		p.touch(now)
	}
}

// CheckUsable kiểm tra MỌI điều kiện áp dụng.
//
// # Thứ tự kiểm tra có chủ ý
//
// Kiểm tra thời gian và trạng thái TRƯỚC giới hạn lượt dùng: một mã đã
// hết hạn phải báo "hết hạn", không phải "hết lượt". Báo sai lý do khiến
// khách thử lại vô ích và bộ phận hỗ trợ điều tra nhầm hướng.
//
// customerUsed là số lượt khách này ĐÃ dùng mã — bên gọi đếm từ kho lưu
// trữ vì entity này không biết bảng nào tồn tại.
func (p *Promotion) CheckUsable(
	orderTotal money.Money, sellerID ids.ID, customerUsed int, now time.Time,
) error {
	if p.status != StatusActive {
		switch p.status {
		case StatusExpired:
			return ErrExpired
		case StatusExhausted:
			return ErrUsageLimitReached
		default:
			return ErrNotActive
		}
	}

	if now.Before(p.startsAt) {
		return ErrNotStarted
	}
	// Dùng !After thay vì Before: thời điểm kết thúc là HẾT HẠN, không
	// phải còn dùng được. Lệch một giây ở đây là mã sống thêm một giây sau
	// khi chiến dịch đã đóng.
	if !now.Before(p.endsAt) {
		return ErrExpired
	}

	// Khuyến mãi gắn với một gian hàng chỉ áp cho đơn của gian hàng đó.
	if !p.sellerID.IsZero() && p.sellerID != sellerID {
		return ErrWrongSeller
	}

	if !p.minOrderAmount.IsZero() {
		cmp, err := orderTotal.Compare(p.minOrderAmount)
		if err != nil {
			return err
		}
		if cmp < 0 {
			return ErrBelowMinimum
		}
	}

	if p.maxUses > 0 && p.usedCount >= p.maxUses {
		return ErrUsageLimitReached
	}

	if p.maxUsesPerCustomer > 0 && customerUsed >= p.maxUsesPerCustomer {
		return ErrCustomerLimitReached
	}

	if !p.maxBudget.IsZero() {
		cmp, err := p.usedBudget.Compare(p.maxBudget)
		if err != nil {
			return err
		}
		if cmp >= 0 {
			return ErrBudgetExhausted
		}
	}

	return nil
}

// CalculateDiscount tính số tiền giảm cho một đơn hàng.
//
// # Ba chặn trên, áp dụng theo thứ tự
//
//  1. maxDiscountAmount   "giảm 50%, tối đa 100.000đ"
//  2. ngân sách còn lại   không tiêu quá ngân sách chiến dịch
//  3. giá trị đơn hàng    KHÔNG BAO GIỜ giảm nhiều hơn tiền đơn
//
// Chặn thứ ba là quan trọng nhất và KHÔNG BAO GIỜ được bỏ: giảm nhiều hơn
// giá trị đơn nghĩa là nền tảng TRẢ TIỀN cho khách để họ mua hàng. Một lỗi
// cấu hình — mã giảm 500.000đ dùng cho đơn 200.000đ — là đủ.
//
// FREE_SHIP trả về 0 ở đây: phí vận chuyển do module khác tính, và
// promotion không biết nó là bao nhiêu. Bên gọi đọc DiscountType để biết
// có miễn phí ship không.
func (p *Promotion) CalculateDiscount(orderTotal money.Money) (money.Money, error) {
	zero := money.Zero(orderTotal.Currency())

	if orderTotal.IsNegative() {
		return zero, ErrInvalidInput
	}

	var discount money.Money

	switch p.discountType {
	case DiscountFreeShip:
		return zero, nil

	case DiscountPercentage:
		discount = applyBPS(orderTotal, p.discountBPS)

	case DiscountFixed:
		if p.discountAmount.Currency() != orderTotal.Currency() {
			return zero, ErrInvalidInput
		}
		discount = p.discountAmount

	default:
		return zero, ErrInvalidInput
	}

	// Chặn 1: trần của khuyến mãi.
	if !p.maxDiscountAmount.IsZero() {
		discount = minMoney(discount, p.maxDiscountAmount)
	}

	// Chặn 2: ngân sách còn lại.
	if !p.maxBudget.IsZero() {
		remaining, err := p.maxBudget.Sub(p.usedBudget)
		if err != nil {
			return zero, err
		}
		if remaining.IsNegative() {
			return zero, nil
		}
		discount = minMoney(discount, remaining)
	}

	// Chặn 3: KHÔNG BAO GIỜ vượt giá trị đơn.
	discount = minMoney(discount, orderTotal)

	if discount.IsNegative() {
		return zero, nil
	}
	return discount, nil
}

// applyBPS tính phần trăm của một số tiền theo điểm cơ bản.
//
// Nhân TRƯỚC rồi chia SAU, toàn bộ bằng số nguyên: chia trước sẽ mất phần
// dư ngay ở bước đầu. Với 1000 bps của 999đ, chia trước ra 0 còn nhân
// trước ra 99.
//
// Phần lẻ bị CẮT XUỐNG (làm tròn xuống), tức là có lợi cho nền tảng ở mức
// dưới một đơn vị tiền nhỏ nhất. Làm tròn lên sẽ khiến tổng giảm giá của
// nhiều dòng vượt quá mức đã hứa.
func applyBPS(m money.Money, bps types.BasisPoints) money.Money {
	amount := m.Amount() * int64(bps.Value()) / 10000
	return money.MustNew(amount, m.Currency())
}

func minMoney(a, b money.Money) money.Money {
	if a.LessThan(b) {
		return a
	}
	return b
}

// RecordUse ghi nhận một lượt sử dụng.
//
// Tăng bộ đếm và cộng ngân sách đã tiêu, rồi TỰ CHUYỂN sang EXHAUSTED khi
// chạm giới hạn. Không tự chuyển thì mã đã hết lượt vẫn qua được kiểm tra
// trạng thái, và chỉ bị chặn ở lần so sánh số đếm — một lớp bảo vệ thay vì
// hai.
func (p *Promotion) RecordUse(discount money.Money, now time.Time) error {
	used, err := p.usedBudget.Add(discount)
	if err != nil {
		return err
	}

	p.usedBudget = used
	p.usedCount++

	if p.maxUses > 0 && p.usedCount >= p.maxUses {
		p.status = StatusExhausted
	}
	if !p.maxBudget.IsZero() {
		cmp, err := p.usedBudget.Compare(p.maxBudget)
		if err != nil {
			return err
		}
		if cmp >= 0 {
			p.status = StatusExhausted
		}
	}

	p.touch(now)
	return nil
}

// ReleaseUse giải phóng một lượt khi đơn hàng bị hủy.
//
// # Vì sao phải bật lại từ EXHAUSTED
//
// Mã hết lượt vì mười đơn, rồi năm đơn bị hủy — nếu không bật lại, năm
// lượt đó mất vĩnh viễn và chiến dịch kết thúc sớm hơn dự tính.
//
// KHÔNG bật lại nếu đã EXPIRED: hết hạn là do thời gian, hủy đơn không
// làm thời gian chạy ngược.
func (p *Promotion) ReleaseUse(discount money.Money, now time.Time) error {
	if p.usedCount > 0 {
		p.usedCount--
	}

	released, err := p.usedBudget.Sub(discount)
	if err != nil {
		return err
	}
	// Chặn âm: bộ đếm ngân sách âm là dấu hiệu giải phóng nhiều hơn đã
	// dùng, và nó sẽ làm mọi kiểm tra ngân sách sau đó sai.
	if released.IsNegative() {
		released = money.Zero(p.usedBudget.Currency())
	}
	p.usedBudget = released

	if p.status == StatusExhausted && p.stillHasRoom() {
		p.status = StatusActive
	}

	p.touch(now)
	return nil
}

func (p *Promotion) stillHasRoom() bool {
	if p.maxUses > 0 && p.usedCount >= p.maxUses {
		return false
	}
	if !p.maxBudget.IsZero() && !p.usedBudget.LessThan(p.maxBudget) {
		return false
	}
	return true
}

// ExpireIfDue chuyển sang EXPIRED nếu đã quá hạn.
//
// Trả về true nếu trạng thái thật sự đổi.
func (p *Promotion) ExpireIfDue(now time.Time) bool {
	if p.status == StatusExpired {
		return false
	}
	if now.Before(p.endsAt) {
		return false
	}
	p.status = StatusExpired
	p.touch(now)
	return true
}

func (p *Promotion) touch(now time.Time) {
	p.updatedAt = now
	p.version++
}

// AllocateToLines phân bổ số tiền giảm xuống từng dòng hàng THEO TỶ LỆ.
//
// # Vì sao phải phân bổ
//
//	Đơn: 3 món, tổng 500.000đ, giảm 50.000đ (10%)
//	    Món A: 200.000đ → giảm 20.000đ → thực trả 180.000đ
//	    Món B: 200.000đ → giảm 20.000đ → thực trả 180.000đ
//	    Món C: 100.000đ → giảm 10.000đ → thực trả  90.000đ
//
//	Khách trả món C → hoàn 90.000đ, KHÔNG phải 100.000đ
//
// Không phân bổ và lưu lại thì khi trả hàng từng phần, không có cách nào
// biết món đó thực trả bao nhiêu — và nền tảng hoàn nhiều hơn đã thu.
//
// Dùng money.Allocate để KHÔNG MẤT ĐỒNG NÀO: phần dư của phép chia được
// rải cho các dòng đầu thay vì biến mất.
func AllocateToLines(discount money.Money, lineTotals []money.Money) ([]money.Money, error) {
	if len(lineTotals) == 0 {
		return nil, nil
	}

	ratios := make([]int64, len(lineTotals))
	var sum int64
	for i, t := range lineTotals {
		if t.IsNegative() {
			return nil, ErrInvalidInput
		}
		ratios[i] = t.Amount()
		sum += t.Amount()
	}

	// Tổng bằng 0 thì không có tỷ lệ nào để chia — chia đều là lựa chọn
	// duy nhất còn ý nghĩa.
	if sum == 0 {
		return discount.AllocateEqual(len(lineTotals))
	}

	return discount.Allocate(ratios)
}
