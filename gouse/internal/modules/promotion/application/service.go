// Package application chứa các use case của module promotion.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/promotion/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// Service là tầng application của module promotion.
type Service struct {
	promos  domain.PromotionRepository
	coupons domain.CouponRepository
	usages  domain.UsageRepository
	clock   Clock
}

type Deps struct {
	Promotions domain.PromotionRepository
	Coupons    domain.CouponRepository
	Usages     domain.UsageRepository
	Clock      Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		promos:  d.Promotions,
		coupons: d.Coupons,
		usages:  d.Usages,
		clock:   clock,
	}
}

// ---------------------------------------------------------------- Quản lý

// CreatePromotion tạo chương trình khuyến mãi.
//
// Trạng thái ban đầu là DRAFT — phải gọi Activate mới áp được.
func (s *Service) CreatePromotion(
	ctx context.Context, p domain.NewPromotionParams,
) (*domain.Promotion, error) {
	p.Now = s.clock.Now()

	promo, err := domain.NewPromotion(p)
	if err != nil {
		return nil, err
	}
	if err := s.promos.Save(ctx, promo); err != nil {
		return nil, err
	}
	return promo, nil
}

func (s *Service) GetPromotion(ctx context.Context, id ids.ID) (*domain.Promotion, error) {
	return s.promos.FindByID(ctx, id)
}

func (s *Service) Activate(ctx context.Context, id ids.ID) error {
	promo, err := s.promos.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if err := promo.Activate(s.clock.Now()); err != nil {
		return err
	}
	return s.promos.Update(ctx, promo)
}

func (s *Service) Pause(ctx context.Context, id ids.ID) error {
	promo, err := s.promos.FindByID(ctx, id)
	if err != nil {
		return err
	}
	promo.Pause(s.clock.Now())
	return s.promos.Update(ctx, promo)
}

// CreateCoupon phát hành một mã cho chương trình khuyến mãi.
func (s *Service) CreateCoupon(
	ctx context.Context, promotionID ids.ID, code string, customerID ids.ID,
) (*domain.Coupon, error) {
	if _, err := s.promos.FindByID(ctx, promotionID); err != nil {
		return nil, err
	}

	c, err := domain.NewCoupon(domain.NewCouponParams{
		PromotionID: promotionID,
		Code:        code,
		CustomerID:  customerID,
		Now:         s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.coupons.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ExpireDue chuyển các khuyến mãi quá hạn sang EXPIRED.
//
// Gọi từ worker chạy định kỳ.
func (s *Service) ExpireDue(ctx context.Context) (int, error) {
	return s.promos.ExpireDue(ctx, s.clock.Now())
}

// ---------------------------------------------------------------- Áp mã

// ValidateInput là dữ liệu kiểm tra mã.
type ValidateInput struct {
	Code string

	// CustomerID để trống với khách vãng lai.
	CustomerID ids.ID

	// SellerID của đơn hàng, dùng để kiểm tra mã riêng của gian hàng.
	SellerID ids.ID

	OrderTotal money.Money
}

// Validation là kết quả kiểm tra mã.
type Validation struct {
	Coupon    *domain.Coupon
	Promotion *domain.Promotion

	Discount money.Money

	// FreeShipping cho biết mã này có miễn phí vận chuyển không.
	//
	// TÁCH KHỎI Discount có chủ ý: phí vận chuyển do module khác tính, và
	// promotion không biết nó là bao nhiêu. Trả cờ này để bên gọi tự trừ.
	FreeShipping bool

	CostAllocations []domain.CostAllocation
}

// Validate kiểm tra mã và tính số tiền giảm.
//
// KHÔNG ghi nhận sử dụng — đây là đường ĐỌC, chạy mỗi lần khách gõ mã vào
// giỏ hàng. Ghi nhận là việc của RecordUsage, gọi khi đơn đã đặt thật.
//
// Tách hai việc vì khách có thể gõ mã rồi bỏ giỏ hàng. Ghi nhận ngay lúc
// kiểm tra sẽ làm mã hết lượt vì những người chưa mua gì.
func (s *Service) Validate(ctx context.Context, in ValidateInput) (*Validation, error) {
	code := domain.NormalizeCode(in.Code)

	coupon, err := s.coupons.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if !coupon.Active() {
		return nil, domain.ErrCouponInactive
	}
	if err := coupon.CheckOwner(in.CustomerID); err != nil {
		return nil, err
	}

	promo, err := s.promos.FindByID(ctx, coupon.PromotionID())
	if err != nil {
		return nil, err
	}

	customerUsed, err := s.usages.CountByCustomer(ctx, coupon.ID(), in.CustomerID)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	if err := promo.CheckUsable(in.OrderTotal, in.SellerID, customerUsed, now); err != nil {
		return nil, err
	}

	discount, err := promo.CalculateDiscount(in.OrderTotal)
	if err != nil {
		return nil, err
	}

	allocations, err := promo.AllocateCostFor(discount, in.SellerID)
	if err != nil {
		return nil, err
	}

	return &Validation{
		Coupon:          coupon,
		Promotion:       promo,
		Discount:        discount,
		FreeShipping:    promo.DiscountType() == domain.DiscountFreeShip,
		CostAllocations: allocations,
	}, nil
}

// AllocateToLines phân bổ số tiền giảm xuống từng dòng hàng THEO TỶ LỆ.
//
// Kết quả phải được ĐÓNG BĂNG vào đơn hàng: khi khách trả lại một món, số
// tiền hoàn là giá dòng TRỪ phần giảm đã phân bổ cho nó.
func (s *Service) AllocateToLines(
	discount money.Money, lineIDs []string, lineTotals []money.Money,
) ([]domain.DiscountLine, error) {
	if len(lineIDs) != len(lineTotals) {
		return nil, domain.ErrInvalidInput
	}

	parts, err := domain.AllocateToLines(discount, lineTotals)
	if err != nil {
		return nil, err
	}

	out := make([]domain.DiscountLine, 0, len(parts))
	for i, part := range parts {
		out = append(out, domain.DiscountLine{
			LineID:   lineIDs[i],
			Discount: part,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------- Ghi nhận

// RecordInput là dữ liệu ghi nhận sử dụng.
type RecordInput struct {
	Code       string
	CustomerID ids.ID
	OrderID    ids.ID
	Discount   money.Money
}

// RecordUsage ghi nhận một lượt sử dụng mã.
//
// # Idempotent, và điều đó nằm ở TẦNG DATABASE
//
// Ràng buộc UNIQUE (coupon_id, order_id) là thứ chặn ghi trùng. Gọi lại
// với cùng (mã, đơn) trả về nil chứ không phải lỗi: handler event xử lý
// lại cùng một event là chuyện bình thường, và báo lỗi sẽ khiến nó thử lại
// mãi mãi.
//
// # Thứ tự có chủ ý: GHI LƯỢT TRƯỚC, TĂNG BỘ ĐẾM SAU
//
// Bảng lượt sử dụng là NGUỒN SỰ THẬT; bộ đếm chỉ là bản tóm tắt. Nếu tăng
// bộ đếm trước rồi ghi lượt thất bại, bộ đếm cao hơn thực tế và mã hết
// lượt sớm. Ngược lại — ghi lượt xong mà tăng bộ đếm thất bại — chỉ làm
// bộ đếm thấp hơn, và đếm lại từ bảng lượt là dựng lại được.
func (s *Service) RecordUsage(ctx context.Context, in RecordInput) error {
	code := domain.NormalizeCode(in.Code)

	coupon, err := s.coupons.FindByCode(ctx, code)
	if err != nil {
		return err
	}

	promo, err := s.promos.FindByID(ctx, coupon.PromotionID())
	if err != nil {
		return err
	}

	now := s.clock.Now()

	err = s.usages.Record(ctx, domain.Usage{
		CouponID:    coupon.ID(),
		PromotionID: promo.ID(),
		CustomerID:  in.CustomerID,
		OrderID:     in.OrderID,
		Discount:    in.Discount,
		UsedAt:      now,
	})
	if errors.Is(err, domain.ErrAlreadyUsed) {
		// Đã ghi rồi — kết quả mong muốn đã đạt.
		return nil
	}
	if err != nil {
		return err
	}

	// Lượt đã ghi vào bảng — thứ còn lại chỉ là cập nhật bản TÓM TẮT.
	//
	// # Vì sao phải thử lại, không được trả lỗi
	//
	// Một mã đang chạy quảng cáo có hàng trăm người cùng áp trong một
	// giây, nên khóa lạc quan xung đột là chuyện THƯỜNG XUYÊN chứ không
	// hiếm. Trả lỗi ở đây để lại đúng trạng thái tệ nhất: hàng trong bảng
	// lượt sử dụng đã có, nhưng bộ đếm và ngân sách KHÔNG tăng.
	//
	// Khi đó mã giới hạn 100 lượt sẽ được dùng vài trăm lần — bộ đếm mãi
	// không chạm tới giới hạn.
	//
	// Thử lại được vì thao tác là ĐỌC LẠI rồi cộng thêm, không phải ghi đè
	// một giá trị đã tính từ trước.
	if err := s.retryUpdate(ctx, promo.ID(), func(p *domain.Promotion) error {
		return p.RecordUse(in.Discount, now)
	}); err != nil {
		return err
	}

	coupon.IncrementUse()
	return s.coupons.Update(ctx, coupon)
}

// maxUpdateRetries là số lần thử lại khi khóa lạc quan xung đột.
//
// Mười lần là đủ cho hàng trăm request song song: mỗi lần thử lại đọc
// phiên bản mới nhất, nên xác suất trượt liên tiếp giảm rất nhanh. Không
// thử vô hạn — nếu trượt mười lần liên tiếp thì có gì đó sai hơn là tranh
// chấp thông thường, và vòng lặp vô hạn giữ kết nối database mãi mãi.
const maxUpdateRetries = 10

// retryUpdate đọc lại khuyến mãi và áp dụng thay đổi cho tới khi ghi được.
//
// ĐỌC LẠI TRONG MỖI LẦN THỬ là điều bắt buộc: thử lại với bản đã đọc từ
// trước sẽ trượt phiên bản mãi mãi.
func (s *Service) retryUpdate(
	ctx context.Context, id ids.ID, mutate func(*domain.Promotion) error,
) error {
	var lastErr error

	for i := 0; i < maxUpdateRetries; i++ {
		promo, err := s.promos.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if err := mutate(promo); err != nil {
			return err
		}

		err = s.promos.Update(ctx, promo)
		if err == nil {
			return nil
		}
		if !errors.Is(err, domain.ErrVersionConflict) {
			return err
		}
		lastErr = err
	}

	return lastErr
}

// ReleaseUsage giải phóng các lượt sử dụng của một đơn bị hủy.
//
// IDEMPOTENT: gọi lại với đơn đã giải phóng không trừ ngân sách lần nữa —
// điều kiện `released_at IS NULL` ở tầng database bảo đảm điều đó.
//
// Trả về số lượt đã giải phóng.
func (s *Service) ReleaseUsage(ctx context.Context, orderID ids.ID) (int, error) {
	now := s.clock.Now()

	released, err := s.usages.Release(ctx, orderID, now)
	if err != nil {
		return 0, err
	}
	if len(released) == 0 {
		return 0, nil
	}

	for _, u := range released {
		// Thử lại khi xung đột, cùng lý do như RecordUsage: hàng trong
		// bảng lượt sử dụng ĐÃ đánh dấu giải phóng, nên bỏ cuộc ở đây để
		// lại một lượt vĩnh viễn bị trừ khỏi ngân sách mà không ai dùng.
		if err := s.retryUpdate(ctx, u.PromotionID, func(p *domain.Promotion) error {
			return p.ReleaseUse(u.Discount, now)
		}); err != nil {
			return 0, err
		}

		coupon, err := s.coupons.FindByID(ctx, u.CouponID)
		if err != nil {
			return 0, err
		}
		coupon.DecrementUse()
		if err := s.coupons.Update(ctx, coupon); err != nil {
			return 0, err
		}
	}

	return len(released), nil
}

// ListUsageForOrder trả các lượt sử dụng của một đơn.
func (s *Service) ListUsageForOrder(
	ctx context.Context, orderID ids.ID,
) ([]domain.Usage, error) {
	return s.usages.ListByOrder(ctx, orderID)
}
