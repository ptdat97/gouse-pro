package promotion

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/promotion/application"
	"github.com/fashion-commerce/platform/internal/modules/promotion/domain"
	promotionpg "github.com/fashion-commerce/platform/internal/modules/promotion/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Chống ghi trùng lượt sử dụng dựa vào UNIQUE (coupon_id, order_id),
	// và bộ đếm lượt dựa vào khóa lạc quan. Kiểm tra ở tầng ứng dụng đều
	// lọt khi hàng trăm người cùng áp một mã trong một giây.
	Storage string

	DB *database.DB

	Clock application.Clock
}

// New khởi tạo module promotion.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"promotion: chỉ hỗ trợ kho lưu trữ postgres — chống ghi trùng lượt " +
				"sử dụng cần ràng buộc UNIQUE ở tầng database")
	}
	if cfg.DB == nil {
		return nil, errors.New("promotion: bắt buộc phải có kết nối database")
	}

	pool := cfg.DB.Pool()

	return &Module{svc: application.NewService(application.Deps{
		Promotions: promotionpg.NewPromotionStore(pool),
		Coupons:    promotionpg.NewCouponStore(pool),
		Usages:     promotionpg.NewUsageStore(pool),
		Clock:      cfg.Clock,
	})}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- Áp mã

func (m *Module) ValidateCoupon(
	ctx context.Context, req ValidateRequest,
) (DiscountResult, error) {
	total, err := toMoney(req.OrderTotal, req.Currency)
	if err != nil {
		return DiscountResult{}, err
	}

	v, err := m.svc.Validate(ctx, application.ValidateInput{
		Code:       req.Code,
		CustomerID: ids.ID(req.CustomerID),
		SellerID:   ids.ID(req.SellerID),
		OrderTotal: total,
	})
	if err != nil {
		return DiscountResult{}, translateErr(err)
	}

	allocations := make([]CostAllocationView, 0, len(v.CostAllocations))
	for _, a := range v.CostAllocations {
		allocations = append(allocations, CostAllocationView{
			Bearer:   string(a.Bearer),
			SellerID: a.SellerID.String(),
			Amount:   a.Amount.Amount(),
		})
	}

	return DiscountResult{
		CouponID:        v.Coupon.ID().String(),
		PromotionID:     v.Promotion.ID().String(),
		Discount:        v.Discount.Amount(),
		Currency:        string(v.Discount.Currency()),
		FreeShipping:    v.FreeShipping,
		CostAllocations: allocations,
	}, nil
}

func (m *Module) AllocateDiscount(
	_ context.Context, req AllocateRequest,
) ([]DiscountLineView, error) {
	discount, err := toMoney(req.Discount, req.Currency)
	if err != nil {
		return nil, err
	}

	lineIDs := make([]string, 0, len(req.Lines))
	totals := make([]money.Money, 0, len(req.Lines))
	for _, l := range req.Lines {
		t, err := toMoney(l.Total, req.Currency)
		if err != nil {
			return nil, err
		}
		lineIDs = append(lineIDs, l.LineID)
		totals = append(totals, t)
	}

	parts, err := m.svc.AllocateToLines(discount, lineIDs, totals)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]DiscountLineView, 0, len(parts))
	for _, p := range parts {
		out = append(out, DiscountLineView{
			LineID:   p.LineID,
			Discount: p.Discount.Amount(),
		})
	}
	return out, nil
}

func (m *Module) RecordUsage(ctx context.Context, req RecordUsageRequest) error {
	discount, err := toMoney(req.Discount, req.Currency)
	if err != nil {
		return err
	}

	return translateErr(m.svc.RecordUsage(ctx, application.RecordInput{
		Code:       req.Code,
		CustomerID: ids.ID(req.CustomerID),
		OrderID:    ids.ID(req.OrderID),
		Discount:   discount,
	}))
}

func (m *Module) ReleaseUsage(ctx context.Context, orderID string) (int, error) {
	n, err := m.svc.ReleaseUsage(ctx, ids.ID(orderID))
	if err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

// ---------------------------------------------------------------- Quản lý

func (m *Module) CreatePromotion(
	ctx context.Context, req CreatePromotionRequest,
) (PromotionView, error) {
	currency := money.Currency(req.Currency)
	if currency == "" {
		currency = money.VND
	}

	discountBPS, err := toBPS(req.DiscountBPS)
	if err != nil {
		return PromotionView{}, err
	}
	platformBPS, err := toBPS(req.PlatformShareBPS)
	if err != nil {
		return PromotionView{}, err
	}
	sellerBPS, err := toBPS(req.SellerShareBPS)
	if err != nil {
		return PromotionView{}, err
	}

	amounts := make([]money.Money, 4)
	for i, v := range []int64{
		req.DiscountAmount, req.MaxDiscountAmount,
		req.MinOrderAmount, req.MaxBudget,
	} {
		m, err := money.New(v, currency)
		if err != nil {
			return PromotionView{}, ErrInvalidInput
		}
		amounts[i] = m
	}

	p, err := m.svc.CreatePromotion(ctx, domain.NewPromotionParams{
		Name:              req.Name,
		Description:       req.Description,
		Kind:              domain.Kind(req.Kind),
		DiscountType:      domain.DiscountType(req.DiscountType),
		DiscountBPS:       discountBPS,
		DiscountAmount:    amounts[0],
		MaxDiscountAmount: amounts[1],
		MinOrderAmount:    amounts[2],
		MaxBudget:         amounts[3],
		CostBearer:        domain.CostBearer(req.CostBearer),
		PlatformShareBPS:  platformBPS,
		SellerShareBPS:    sellerBPS,
		SellerID:          ids.ID(req.SellerID),

		MaxUses:            req.MaxUses,
		MaxUsesPerCustomer: req.MaxUsesPerCustomer,

		StartsAt: req.StartsAt,
		EndsAt:   req.EndsAt,
		Currency: currency,
	})
	if err != nil {
		return PromotionView{}, translateErr(err)
	}
	return toPromotionView(p), nil
}

func (m *Module) GetPromotion(ctx context.Context, promotionID string) (PromotionView, error) {
	p, err := m.svc.GetPromotion(ctx, ids.ID(promotionID))
	if err != nil {
		return PromotionView{}, translateErr(err)
	}
	return toPromotionView(p), nil
}

func (m *Module) ActivatePromotion(ctx context.Context, promotionID string) error {
	return translateErr(m.svc.Activate(ctx, ids.ID(promotionID)))
}

func (m *Module) PausePromotion(ctx context.Context, promotionID string) error {
	return translateErr(m.svc.Pause(ctx, ids.ID(promotionID)))
}

func (m *Module) CreateCoupon(
	ctx context.Context, req CreateCouponRequest,
) (CouponView, error) {
	c, err := m.svc.CreateCoupon(ctx,
		ids.ID(req.PromotionID), req.Code, ids.ID(req.CustomerID))
	if err != nil {
		return CouponView{}, translateErr(err)
	}

	return CouponView{
		ID:          c.ID().String(),
		PromotionID: c.PromotionID().String(),
		Code:        c.Code(),
		CustomerID:  c.CustomerID().String(),
		UsedCount:   c.UsedCount(),
		Active:      c.Active(),
	}, nil
}

func (m *Module) ExpireDuePromotions(ctx context.Context) (int, error) {
	n, err := m.svc.ExpireDue(ctx)
	if err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

// ---------------------------------------------------------------- Chuyển đổi

// toMoney đổi số nguyên + mã tiền tệ thành Money.
//
// Mã tiền tệ rỗng mặc định VND: đa số bên gọi ở thị trường này không
// truyền nó, và bắt lỗi sẽ chỉ thêm một dòng vào mọi chỗ gọi.
func toMoney(amount int64, currency string) (money.Money, error) {
	c := money.Currency(currency)
	if c == "" {
		c = money.VND
	}
	m, err := money.New(amount, c)
	if err != nil {
		return money.Money{}, ErrInvalidInput
	}
	return m, nil
}

// toBPS đổi điểm cơ bản, TỪ CHỐI giá trị ngoài khoảng.
//
// Không dùng MustNewBasisPoints: giá trị đến từ bên ngoài module, và
// panic vì dữ liệu người dùng nhập là cách chắc chắn nhất để sập tiến
// trình bằng một request.
func toBPS(v int32) (types.BasisPoints, error) {
	bps, err := types.NewBasisPoints(v)
	if err != nil {
		return types.BasisPoints{}, ErrInvalidInput
	}
	return bps, nil
}

func toPromotionView(p *domain.Promotion) PromotionView {
	return PromotionView{
		ID:                p.ID().String(),
		Name:              p.Name(),
		Description:       p.Description(),
		Kind:              string(p.Kind()),
		DiscountType:      string(p.DiscountType()),
		DiscountBPS:       p.DiscountBPS().Value(),
		DiscountAmount:    p.DiscountAmount().Amount(),
		MaxDiscountAmount: p.MaxDiscountAmount().Amount(),
		MinOrderAmount:    p.MinOrderAmount().Amount(),
		CostBearer:        string(p.CostBearer()),
		PlatformShareBPS:  p.PlatformShare().Value(),
		SellerShareBPS:    p.SellerShare().Value(),
		SellerID:          p.SellerID().String(),

		MaxUses:            p.MaxUses(),
		MaxUsesPerCustomer: p.MaxUsesPerCustomer(),
		UsedCount:          p.UsedCount(),

		MaxBudget:  p.MaxBudget().Amount(),
		UsedBudget: p.UsedBudget().Amount(),

		Status:   string(p.Status()),
		StartsAt: p.StartsAt(),
		EndsAt:   p.EndsAt(),
		Currency: string(p.DiscountAmount().Currency()),
	}
}

// translateErr đổi lỗi domain thành lỗi CÔNG KHAI.
func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrCouponNotFound):
		return ErrCouponNotFound
	case errors.Is(err, domain.ErrCouponInactive):
		return ErrCouponInactive
	case errors.Is(err, domain.ErrNotStarted):
		return ErrNotStarted
	case errors.Is(err, domain.ErrExpired):
		return ErrExpired
	case errors.Is(err, domain.ErrNotActive):
		return ErrNotActive
	case errors.Is(err, domain.ErrBelowMinimum):
		return ErrBelowMinimum
	case errors.Is(err, domain.ErrUsageLimitReached):
		return ErrUsageLimitReached
	case errors.Is(err, domain.ErrCustomerLimitReached):
		return ErrCustomerLimitReached
	case errors.Is(err, domain.ErrBudgetExhausted):
		return ErrBudgetExhausted
	case errors.Is(err, domain.ErrWrongCustomer):
		return ErrWrongCustomer
	case errors.Is(err, domain.ErrWrongSeller):
		return ErrWrongSeller
	case errors.Is(err, domain.ErrVersionConflict):
		return ErrVersionConflict
	case errors.Is(err, domain.ErrInvalidInput):
		return ErrInvalidInput
	default:
		return err
	}
}
