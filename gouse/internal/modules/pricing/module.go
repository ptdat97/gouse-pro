package pricing

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/pricing/application"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
	pricingpg "github.com/fashion-commerce/platform/internal/modules/pricing/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Module là cài đặt của API công khai.
//
// Nó CHUYỂN ĐỔI giữa domain object (nội bộ) và DTO (công khai). Domain
// object không bao giờ rời khỏi package con.
type Module struct {
	svc *application.Service
}

// Bảo đảm lúc biên dịch rằng Module thỏa mãn API.
var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage chọn kho lưu trữ: "memory" hoặc "postgres".
	Storage string

	// DB là kết nối database. BẮT BUỘC khi Storage = "postgres".
	DB *database.DB

	// Clock cho phép test kiểm soát thời gian. Nil = đồng hồ hệ thống.
	Clock application.Clock
}

// New khởi tạo module pricing.
func New(cfg Config) (*Module, error) {
	switch cfg.Storage {
	case "", "memory":
		return &Module{svc: application.NewInMemoryService(cfg.Clock)}, nil

	case "postgres":
		if cfg.DB == nil {
			return nil, errors.New("pricing: kho lưu trữ postgres cần kết nối database")
		}
		pool := cfg.DB.Pool()
		return &Module{svc: application.NewService(application.Deps{
			Prices:      pricingpg.NewPriceStore(pool),
			Constraints: pricingpg.NewConstraintStore(pool),
			History:     pricingpg.NewHistoryStore(pool),
			Clock:       cfg.Clock,
		})}, nil

	default:
		return nil, errors.New("pricing: kho lưu trữ không hợp lệ: " + cfg.Storage)
	}
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- API

func (m *Module) GetPrice(ctx context.Context, req PriceRequest) (*PriceResult, error) {
	q, err := toQuery(req)
	if err != nil {
		return nil, err
	}

	got, err := m.svc.GetPrice(ctx, q)
	if err != nil {
		return nil, translateErr(err)
	}
	out := toPriceResult(got)
	return &out, nil
}

func (m *Module) GetPrices(ctx context.Context, reqs []PriceRequest) (map[string]PriceResult, error) {
	queries := make([]application.PriceQuery, 0, len(reqs))
	for _, r := range reqs {
		q, err := toQuery(r)
		if err != nil {
			// Bỏ qua yêu cầu sai định dạng thay vì làm hỏng cả lời gọi:
			// hiển thị 49/50 giá tốt hơn là cả trang lỗi.
			continue
		}
		queries = append(queries, q)
	}

	found, err := m.svc.GetPrices(ctx, queries)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]PriceResult, len(found))
	for skuID, res := range found {
		out[skuID.String()] = toPriceResult(res)
	}
	return out, nil
}

func (m *Module) GetPriceConstraint(ctx context.Context, skuID string) (*PriceConstraint, error) {
	id, err := ids.Parse(skuID, ids.PrefixSKU)
	if err != nil {
		return nil, ErrInvalidID
	}

	c, err := m.svc.GetConstraint(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return &PriceConstraint{
		SKUID:    c.SKUID().String(),
		MinPrice: toAmount(c.MinPrice()),
		MaxPrice: toAmount(c.MaxPrice()),
	}, nil
}

func (m *Module) ValidateSellerPrice(
	ctx context.Context, skuID string, amount int64, currency string,
) (PriceCheck, error) {
	id, err := ids.Parse(skuID, ids.PrefixSKU)
	if err != nil {
		return PriceCheck{}, ErrInvalidID
	}

	price, err := money.New(amount, money.Currency(currency))
	if err != nil {
		// Đơn vị tiền tệ không hợp lệ là lỗi của bên gọi, KHÔNG được coi
		// là "giá hợp lệ" — nếu bỏ qua, offer sẽ lưu với giá không kiểm
		// chứng được.
		return PriceCheck{
			Allowed: false,
			Code:    string(domain.ViolationWrongCurrency),
			Message: "Đơn vị tiền tệ không hợp lệ: " + currency,
		}, nil
	}

	res, err := m.svc.ValidateSellerPrice(ctx, id, price)
	if err != nil {
		return PriceCheck{}, translateErr(err)
	}
	return PriceCheck{
		Allowed:     res.Allowed,
		Code:        string(res.Code),
		Message:     res.Message,
		NeedsReview: res.NeedsReview,
		MinPrice:    toAmount(res.Min),
		MaxPrice:    toAmount(res.Max),
	}, nil
}

func (m *Module) GetPriceHistory(
	ctx context.Context, skuID string, period DateRange,
) ([]PricePoint, error) {
	id, err := ids.Parse(skuID, ids.PrefixSKU)
	if err != nil {
		return nil, ErrInvalidID
	}

	r, err := toDateRange(period)
	if err != nil {
		return nil, err
	}

	points, err := m.svc.GetHistory(ctx, id, r)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]PricePoint, 0, len(points))
	for _, p := range points {
		out = append(out, PricePoint{
			SKUID:      p.SKUID().String(),
			PriceType:  string(p.Type()),
			Amount:     toAmount(p.Amount()),
			CompareAt:  toAmount(p.CompareAt()),
			Reason:     string(p.Reason()),
			RecordedAt: p.RecordedAt().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (m *Module) GetLowestPriceLast30Days(ctx context.Context, skuID string) (Amount, bool, error) {
	id, err := ids.Parse(skuID, ids.PrefixSKU)
	if err != nil {
		return Amount{}, false, ErrInvalidID
	}

	lowest, ok, err := m.svc.LowestPriceLast30Days(ctx, id)
	if err != nil {
		return Amount{}, false, translateErr(err)
	}
	if !ok {
		return Amount{}, false, nil
	}
	return toAmount(lowest), true, nil
}

// ---------------------------------------------------------------- Chuyển đổi

func toQuery(r PriceRequest) (application.PriceQuery, error) {
	skuID, err := ids.Parse(r.SKUID, ids.PrefixSKU)
	if err != nil {
		return application.PriceQuery{}, ErrInvalidID
	}

	q := application.PriceQuery{SKUID: skuID, CustomerTier: r.CustomerTier}

	if r.CampaignID != "" {
		campaignID, err := ids.Parse(r.CampaignID, ids.PrefixCampaign)
		if err != nil {
			return application.PriceQuery{}, ErrInvalidID
		}
		q.CampaignID = campaignID
	}
	return q, nil
}

func toPriceResult(r application.PriceResult) PriceResult {
	return PriceResult{
		SKUID:               r.SKUID.String(),
		Amount:              toAmount(r.Amount),
		CompareAt:           toAmount(r.CompareAt),
		PriceType:           string(r.PriceType),
		DiscountBasisPoints: r.DiscountBP,
	}
}

// toAmount chuyển Money sang DTO.
//
// Money rỗng (chưa đặt) có Currency rỗng — giữ nguyên như vậy để bên gọi
// phân biệt được "giá 0 đồng" với "chưa có giá". Nếu điền đơn vị mặc định,
// hai trường hợp đó lẫn vào nhau.
func toAmount(m money.Money) Amount {
	return Amount{Value: m.Amount(), Currency: string(m.Currency())}
}

func toDateRange(d DateRange) (domain.DateRange, error) {
	var out domain.DateRange

	if d.From != "" {
		from, err := time.Parse(time.RFC3339, d.From)
		if err != nil {
			return out, errors.New("pricing: thời điểm From không đúng định dạng RFC3339")
		}
		out.From = from
	}
	if d.To != "" {
		to, err := time.Parse(time.RFC3339, d.To)
		if err != nil {
			return out, errors.New("pricing: thời điểm To không đúng định dạng RFC3339")
		}
		out.To = to
	}
	return out, nil
}

// translateErr chuyển lỗi nội bộ sang lỗi công khai.
func translateErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, application.ErrNoPrice):
		return ErrNoPrice
	}
	return err
}
