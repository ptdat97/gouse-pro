// Package inmemory cài đặt các port của pricing bằng bộ nhớ.
//
// Mục đích: kiểm chứng mô hình domain TRƯỚC khi có database, và cho phép
// test tầng application chạy nhanh không cần hạ tầng.
//
// Đây KHÔNG phải kho lưu trữ cho production — dữ liệu mất khi tiến trình
// dừng. Cài đặt PostgreSQL sẽ nằm ở infrastructure/postgres/ và cùng thỏa
// mãn các port trong domain.
package inmemory

import (
	"context"
	"sort"
	"sync"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
)

// PriceStore lưu bảng giá trong bộ nhớ.
type PriceStore struct {
	mu    sync.RWMutex
	byID  map[ids.ID]domain.RestorePriceParams
	bySKU map[ids.ID][]ids.ID
}

func NewPriceStore() *PriceStore {
	return &PriceStore{
		byID:  make(map[ids.ID]domain.RestorePriceParams),
		bySKU: make(map[ids.ID][]ids.ID),
	}
}

// snapshot chuyển aggregate thành dữ liệu thuần để lưu.
//
// Lưu bản chụp thay vì con trỏ là CÓ CHỦ ĐÍCH: nếu lưu con trỏ, bên gọi
// sửa aggregate sau khi Save sẽ vô tình thay đổi dữ liệu "đã lưu" — điều
// không xảy ra với database thật.
//
// Money là struct giá trị nên sao chép tự nhiên, không cần xử lý riêng.
func snapshot(p *domain.Price) domain.RestorePriceParams {
	return domain.RestorePriceParams{
		ID:           p.ID(),
		SKUID:        p.SKUID(),
		PriceType:    p.Type(),
		Amount:       p.Amount(),
		CompareAt:    p.CompareAt(),
		Period:       p.Period(),
		CustomerTier: p.CustomerTier(),
		CampaignID:   p.CampaignID(),
		Active:       p.IsActive(),
		CreatedAt:    p.CreatedAt(),
		UpdatedAt:    p.UpdatedAt(),
	}
}

func (s *PriceStore) Save(_ context.Context, p *domain.Price) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, existing := s.byID[p.ID()]; !existing {
		s.bySKU[p.SKUID()] = append(s.bySKU[p.SKUID()], p.ID())
	}
	s.byID[p.ID()] = snapshot(p)
	return nil
}

func (s *PriceStore) FindByID(_ context.Context, id ids.ID) (*domain.Price, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestorePrice(p), nil
}

// FindBySKU trả MỌI mức giá của SKU, kể cả đã ngừng áp dụng.
//
// Việc chọn mức nào áp dụng là quyết định NGHIỆP VỤ (domain.SelectBest).
// Nếu kho tự lọc, quy tắc ưu tiên sẽ nằm rải rác ở cả hai nơi.
func (s *PriceStore) FindBySKU(_ context.Context, skuID ids.ID) ([]*domain.Price, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pricesFor(skuID), nil
}

func (s *PriceStore) FindBySKUs(
	_ context.Context, skuIDs []ids.ID,
) (map[ids.ID][]*domain.Price, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[ids.ID][]*domain.Price, len(skuIDs))
	for _, skuID := range skuIDs {
		if list := s.pricesFor(skuID); len(list) > 0 {
			out[skuID] = list
		}
	}
	return out, nil
}

// pricesFor lấy giá của một SKU. Bên gọi phải giữ khóa đọc.
func (s *PriceStore) pricesFor(skuID ids.ID) []*domain.Price {
	idList := s.bySKU[skuID]
	out := make([]*domain.Price, 0, len(idList))
	for _, id := range idList {
		if p, ok := s.byID[id]; ok {
			out = append(out, domain.RestorePrice(p))
		}
	}
	// Sắp xếp theo id để kết quả ỔN ĐỊNH giữa các lần gọi.
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// ---------------------------------------------------------------- Khung giá

// ConstraintStore lưu khung giá ràng buộc seller.
type ConstraintStore struct {
	mu    sync.RWMutex
	bySKU map[ids.ID]domain.RestoreConstraintParams
}

func NewConstraintStore() *ConstraintStore {
	return &ConstraintStore{bySKU: make(map[ids.ID]domain.RestoreConstraintParams)}
}

func constraintSnapshot(c *domain.PriceConstraint) domain.RestoreConstraintParams {
	return domain.RestoreConstraintParams{
		ID:             c.ID(),
		SKUID:          c.SKUID(),
		MinPrice:       c.MinPrice(),
		MaxPrice:       c.MaxPrice(),
		ReferencePrice: c.ReferencePrice(),
		// Phải lưu ngưỡng cảnh báo: bỏ sót trường này thì khi đọc lên ngưỡng
		// thành 0 và việc phát hiện giá bất thường im lặng ngừng hoạt động —
		// loại lỗi không có thông báo nào cả.
		SuspiciousBelowBP: c.SuspiciousBelowBP(),
		CreatedAt:         c.CreatedAt(),
		UpdatedAt:         c.UpdatedAt(),
	}
}

// Save lưu khung giá. Mỗi SKU có TỐI ĐA MỘT khung giá.
//
// Nhiều khung giá cho cùng một SKU sẽ mâu thuẫn nhau và không có cách nào
// quyết định khung nào thắng — mô hình một-một loại bỏ hẳn câu hỏi đó.
func (s *ConstraintStore) Save(_ context.Context, c *domain.PriceConstraint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bySKU[c.SKUID()] = constraintSnapshot(c)
	return nil
}

func (s *ConstraintStore) FindBySKU(
	_ context.Context, skuID ids.ID,
) (*domain.PriceConstraint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.bySKU[skuID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestorePriceConstraint(c), nil
}

func (s *ConstraintStore) FindBySKUs(
	_ context.Context, skuIDs []ids.ID,
) (map[ids.ID]*domain.PriceConstraint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[ids.ID]*domain.PriceConstraint, len(skuIDs))
	for _, skuID := range skuIDs {
		if c, ok := s.bySKU[skuID]; ok {
			out[skuID] = domain.RestorePriceConstraint(c)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- Lịch sử

// HistoryStore lưu lịch sử giá — CHỈ GHI THÊM.
//
// Không có phương thức sửa hay xóa. Sự thiếu vắng đó là CÓ CHỦ ĐÍCH: lịch
// sử sửa được thì không còn giá trị làm bằng chứng cho việc phát hiện thao
// túng giá hay cho nghĩa vụ minh bạch giá.
//
// Khi có PostgreSQL, ràng buộc này được siết thêm bằng RULE ở tầng database
// (cùng cách làm với sổ cái — xem ADR-0008).
type HistoryStore struct {
	mu    sync.RWMutex
	bySKU map[ids.ID][]domain.RestorePricePointParams
}

func NewHistoryStore() *HistoryStore {
	return &HistoryStore{bySKU: make(map[ids.ID][]domain.RestorePricePointParams)}
}

func (s *HistoryStore) Append(_ context.Context, p *domain.PricePoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.bySKU[p.SKUID()] = append(s.bySKU[p.SKUID()], domain.RestorePricePointParams{
		ID:         p.ID(),
		SKUID:      p.SKUID(),
		PriceType:  p.Type(),
		Amount:     p.Amount(),
		CompareAt:  p.CompareAt(),
		Reason:     p.Reason(),
		ChangedBy:  p.ChangedBy(),
		RecordedAt: p.RecordedAt(),
	})
	return nil
}

func (s *HistoryStore) FindBySKU(
	_ context.Context, skuID ids.ID, r domain.DateRange,
) ([]*domain.PricePoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*domain.PricePoint
	for _, p := range s.bySKU[skuID] {
		if !r.Contains(p.RecordedAt) {
			continue
		}
		out = append(out, domain.RestorePricePoint(p))
	}
	// Trả theo thứ tự thời gian — lịch sử đọc ngược thời gian rất khó hiểu.
	return domain.SortByTime(out), nil
}
