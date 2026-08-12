// Package application chứa các use case của module pricing.
//
// Tầng này điều phối: gọi domain để áp dụng quy tắc nghiệp vụ, gọi
// repository để đọc/ghi. Nó KHÔNG chứa quy tắc nghiệp vụ — quy tắc nằm ở
// domain.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
)

// Clock cho phép test kiểm soát thời gian.
//
// Không dùng time.Now() trực tiếp: test về giá có thời hạn (flash, chiến
// dịch) cần thời gian xác định, không phụ thuộc lúc chạy test.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// SystemClock là đồng hồ thật, dùng ở production.
var SystemClock Clock = systemClock{}

// ErrNoPrice khi SKU chưa có mức giá nào áp dụng được.
//
// Khác với ErrNotFound: SKU có thể có giá nhưng tất cả đã hết hạn hoặc
// không khớp ngữ cảnh khách. Phân biệt hai trường hợp giúp gỡ lỗi nhanh —
// "chưa cấu hình giá" và "giá đã hết hạn" cần hai cách xử lý khác nhau.
var ErrNoPrice = errors.New("pricing: không có mức giá nào áp dụng được")

// Service là tầng application của module pricing.
type Service struct {
	prices      domain.PriceRepository
	constraints domain.ConstraintRepository
	history     domain.HistoryRepository
	clock       Clock
}

// Deps gom các phụ thuộc.
type Deps struct {
	Prices      domain.PriceRepository
	Constraints domain.ConstraintRepository
	History     domain.HistoryRepository
	Clock       Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		prices:      d.Prices,
		constraints: d.Constraints,
		history:     d.History,
		clock:       clock,
	}
}

// Now trả về thời điểm hiện tại theo đồng hồ của service.
func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Đặt giá

// SetPriceInput là dữ liệu đặt một mức giá.
type SetPriceInput struct {
	SKUID     ids.ID
	PriceType domain.PriceType
	Amount    money.Money
	CompareAt money.Money
	Period    domain.Period

	CustomerTier string
	CampaignID   ids.ID

	// Reason là lý do thay đổi giá, ghi vào lịch sử.
	Reason domain.ChangeReason

	// ChangedBy là người thực hiện. Rỗng nghĩa là hệ thống tự động.
	ChangedBy ids.ID
}

// SetPrice đặt một mức giá và GHI LỊCH SỬ.
//
// Quy tắc 3: mọi thay đổi giá phải ghi vào lịch sử.
//
// Ghi lịch sử nằm TRONG use case này chứ không phải việc bên gọi phải nhớ
// làm: nếu để bên gọi tự ghi, sẽ có chỗ quên, và lịch sử thiếu một điểm là
// lịch sử không dùng được cho nghĩa vụ minh bạch giá.
//
// Thứ tự: ghi lịch sử TRƯỚC, lưu giá SAU. Nếu đảo lại và việc ghi lịch sử
// thất bại, giá đã đổi mà không có vết — đúng thứ tự thì trường hợp xấu
// nhất là có vết thừa, dễ đối chiếu hơn nhiều so với thiếu vết.
func (s *Service) SetPrice(ctx context.Context, in SetPriceInput) (*domain.Price, error) {
	now := s.clock.Now()

	p, err := domain.NewPrice(domain.NewPriceParams{
		SKUID:        in.SKUID,
		PriceType:    in.PriceType,
		Amount:       in.Amount,
		CompareAt:    in.CompareAt,
		Period:       in.Period,
		CustomerTier: in.CustomerTier,
		CampaignID:   in.CampaignID,
		Now:          now,
	})
	if err != nil {
		return nil, err
	}

	point, err := domain.NewPricePoint(domain.NewPricePointParams{
		SKUID:     in.SKUID,
		PriceType: p.Type(),
		Amount:    in.Amount,
		CompareAt: in.CompareAt,
		Reason:    in.Reason,
		ChangedBy: in.ChangedBy,
		Now:       now,
	})
	if err != nil {
		return nil, err
	}
	if err := s.history.Append(ctx, point); err != nil {
		return nil, err
	}

	if err := s.prices.Save(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// DeactivatePrice ngừng áp dụng một mức giá.
func (s *Service) DeactivatePrice(ctx context.Context, id ids.ID) (*domain.Price, error) {
	p, err := s.prices.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Deactivate(s.clock.Now())
	if err := s.prices.Save(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ---------------------------------------------------------------- Tra giá

// PriceQuery là ngữ cảnh tra giá.
type PriceQuery struct {
	SKUID ids.ID

	// CustomerTier để áp giá thành viên. Rỗng = khách vãng lai.
	CustomerTier string

	// CampaignID để áp giá chiến dịch. Rỗng = không đến từ chiến dịch nào.
	CampaignID ids.ID
}

// PriceResult là kết quả tra giá.
type PriceResult struct {
	SKUID ids.ID

	// Amount là giá khách phải trả.
	Amount money.Money

	// CompareAt là giá gạch ngang. Bằng 0 nghĩa là không hiển thị.
	CompareAt money.Money

	// PriceType cho biết loại giá đang áp dụng.
	PriceType domain.PriceType

	// DiscountBP là mức giảm theo phần vạn (1000 = 10%).
	DiscountBP int64
}

// GetPrice tra giá áp dụng cho một SKU trong một ngữ cảnh.
//
// Quy tắc 2: CHỈ MỘT giá được áp dụng. Việc chọn do domain.SelectBest
// quyết định — tầng này chỉ lấy dữ liệu và gọi.
func (s *Service) GetPrice(ctx context.Context, q PriceQuery) (PriceResult, error) {
	list, err := s.prices.FindBySKU(ctx, q.SKUID)
	if err != nil {
		return PriceResult{}, err
	}

	best := domain.SelectBest(list, s.clock.Now(), q.CustomerTier, q.CampaignID)
	if best == nil {
		return PriceResult{}, ErrNoPrice
	}
	return toResult(q.SKUID, best), nil
}

// GetPrices tra giá theo LÔ.
//
// Hiển thị 50 sản phẩm cần 1 lời gọi, không phải 50 — đây là khác biệt
// giữa trang tải trong 200ms và trang tải trong 3 giây.
//
// SKU không có giá áp dụng được bị BỎ QUA thay vì làm hỏng cả lời gọi:
// hiển thị 49/50 sản phẩm tốt hơn là cả trang lỗi.
func (s *Service) GetPrices(ctx context.Context, queries []PriceQuery) (map[ids.ID]PriceResult, error) {
	skuIDs := make([]ids.ID, 0, len(queries))
	for _, q := range queries {
		skuIDs = append(skuIDs, q.SKUID)
	}

	all, err := s.prices.FindBySKUs(ctx, skuIDs)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	out := make(map[ids.ID]PriceResult, len(queries))
	for _, q := range queries {
		best := domain.SelectBest(all[q.SKUID], now, q.CustomerTier, q.CampaignID)
		if best == nil {
			continue
		}
		out[q.SKUID] = toResult(q.SKUID, best)
	}
	return out, nil
}

func toResult(skuID ids.ID, p *domain.Price) PriceResult {
	return PriceResult{
		SKUID:      skuID,
		Amount:     p.Amount(),
		CompareAt:  p.CompareAt(),
		PriceType:  p.Type(),
		DiscountBP: p.DiscountBasisPoints(),
	}
}

// ---------------------------------------------------------------- Khung giá

// SetConstraintInput là dữ liệu đặt khung giá ràng buộc seller.
type SetConstraintInput struct {
	SKUID          ids.ID
	MinPrice       money.Money
	MaxPrice       money.Money
	ReferencePrice money.Money
}

// SetConstraint đặt khung giá cho một SKU.
func (s *Service) SetConstraint(
	ctx context.Context, in SetConstraintInput,
) (*domain.PriceConstraint, error) {
	c, err := domain.NewPriceConstraint(domain.NewConstraintParams{
		SKUID:          in.SKUID,
		MinPrice:       in.MinPrice,
		MaxPrice:       in.MaxPrice,
		ReferencePrice: in.ReferencePrice,
		Now:            s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.constraints.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) GetConstraint(ctx context.Context, skuID ids.ID) (*domain.PriceConstraint, error) {
	return s.constraints.FindBySKU(ctx, skuID)
}

// ValidateSellerPrice kiểm tra giá seller có được chấp nhận không.
//
// Quy tắc 4: giá seller phải trong khung ràng buộc.
//
// SKU CHƯA CÓ khung giá thì CHẤP NHẬN mọi giá dương. Đây là quyết định có
// chủ đích: chặn hết khi chưa cấu hình sẽ khiến không seller nào đăng bán
// được cho tới khi nền tảng cấu hình xong từng SKU — không khả thi.
//
// Đánh đổi: SKU chưa có khung giá không được bảo vệ khỏi lỗi nhập liệu.
// Việc bảo đảm mọi SKU đều có khung giá là trách nhiệm vận hành, và nên có
// cảnh báo giám sát cho số SKU thiếu khung.
func (s *Service) ValidateSellerPrice(
	ctx context.Context, skuID ids.ID, price money.Money,
) (domain.CheckResult, error) {
	c, err := s.constraints.FindBySKU(ctx, skuID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.CheckResult{
				Allowed: price.IsPositive(),
				Code:    noConstraintCode(price),
				Message: noConstraintMessage(price),
			}, nil
		}
		return domain.CheckResult{}, err
	}
	return c.Check(price), nil
}

func noConstraintCode(price money.Money) domain.ViolationCode {
	if price.IsPositive() {
		return domain.ViolationNone
	}
	return domain.ViolationNotPositive
}

func noConstraintMessage(price money.Money) string {
	if price.IsPositive() {
		return ""
	}
	return "Giá phải lớn hơn 0"
}

// ---------------------------------------------------------------- Lịch sử

// GetHistory lấy lịch sử giá của một SKU.
func (s *Service) GetHistory(
	ctx context.Context, skuID ids.ID, r domain.DateRange,
) ([]*domain.PricePoint, error) {
	return s.history.FindBySKU(ctx, skuID, r)
}

// LowestPriceIn trả giá THẤP NHẤT trong khoảng thời gian.
//
// Đây là con số một số thị trường bắt buộc công bố khi quảng cáo giảm giá
// ("giá thấp nhất 30 ngày qua"). Không có nó thì quảng cáo "giảm 50%" so
// với một mức giá vừa được nâng lên là hành vi bị cấm.
//
// Trả false nếu không có dữ liệu trong khoảng.
func (s *Service) LowestPriceIn(
	ctx context.Context, skuID ids.ID, r domain.DateRange,
) (money.Money, bool, error) {
	points, err := s.history.FindBySKU(ctx, skuID, r)
	if err != nil {
		return money.Money{}, false, err
	}
	lowest, ok := domain.LowestIn(points, r)
	return lowest, ok, nil
}

// LowestPriceLast30Days là trường hợp dùng thường xuyên nhất của
// LowestPriceIn — đặt riêng để bên gọi không tự tính khoảng thời gian sai.
func (s *Service) LowestPriceLast30Days(
	ctx context.Context, skuID ids.ID,
) (money.Money, bool, error) {
	now := s.clock.Now()
	return s.LowestPriceIn(ctx, skuID, domain.DateRange{
		From: now.AddDate(0, 0, -30),
		To:   now,
	})
}
