// Package inmemory cài đặt các port của catalog bằng bộ nhớ.
//
// Mục đích (mẫu adapter giả từ Flamingo — xem docs/11-oss/flamingo-commerce.md):
//
//	✓ Kiểm chứng mô hình domain TRƯỚC khi có database
//	✓ Test tầng application chạy nhanh, không cần hạ tầng
//	✓ Chạy demo và phát triển frontend không cần Docker
//
// Đây KHÔNG phải kho lưu trữ cho production — dữ liệu mất khi tiến trình dừng.
// Cài đặt PostgreSQL sẽ nằm ở infrastructure/postgres/ và cùng thỏa mãn
// các port trong domain.
package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

// BrandStore lưu thương hiệu trong bộ nhớ.
//
// Dùng RWMutex vì đọc nhiều hơn ghi rất nhiều — và vì test có thể chạy
// song song (t.Parallel).
type BrandStore struct {
	mu     sync.RWMutex
	byID   map[ids.ID]domain.RestoreBrandParams
	bySlug map[string]ids.ID
}

func NewBrandStore() *BrandStore {
	return &BrandStore{
		byID:   make(map[ids.ID]domain.RestoreBrandParams),
		bySlug: make(map[string]ids.ID),
	}
}

// snapshot chuyển aggregate thành dữ liệu thuần để lưu.
//
// Lưu bản chụp thay vì con trỏ là CÓ CHỦ ĐÍCH: nếu lưu con trỏ, bên gọi
// sửa aggregate sau khi Save sẽ vô tình thay đổi dữ liệu "đã lưu" — điều
// không xảy ra với database thật. Bản chụp làm in-memory hành xử giống
// database hơn, nên test bắt được lỗi thật.
func snapshot(b *domain.Brand) domain.RestoreBrandParams {
	return domain.RestoreBrandParams{
		ID:              b.ID(),
		Name:            b.Name(),
		Slug:            b.Slug(),
		Description:     b.Description(),
		LogoURL:         b.LogoURL(),
		BrandType:       b.Type(),
		ProtectionLevel: b.ProtectionLevel(),
		OwnerSellerID:   b.OwnerSellerID(),
		CountryOfOrigin: b.CountryOfOrigin(),
		Status:          b.Status(),
		CreatedAt:       b.CreatedAt(),
		UpdatedAt:       b.UpdatedAt(),
	}
}

func (s *BrandStore) Save(_ context.Context, b *domain.Brand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Kiểm tra slug trùng — mô phỏng ràng buộc UNIQUE của database.
	if existingID, ok := s.bySlug[b.Slug()]; ok && existingID != b.ID() {
		return domain.ErrSlugTaken
	}

	// Nếu là cập nhật và slug đổi, xóa ánh xạ slug cũ.
	if old, ok := s.byID[b.ID()]; ok && old.Slug != b.Slug() {
		delete(s.bySlug, old.Slug)
	}

	s.byID[b.ID()] = snapshot(b)
	s.bySlug[b.Slug()] = b.ID()
	return nil
}

func (s *BrandStore) FindByID(_ context.Context, id ids.ID) (*domain.Brand, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreBrand(p), nil
}

func (s *BrandStore) FindBySlug(_ context.Context, slug string) (*domain.Brand, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.bySlug[slug]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreBrand(s.byID[id]), nil
}

// FindByIDs trả về map để bên gọi tra cứu O(1).
//
// Id không tồn tại đơn giản là vắng mặt trong kết quả — KHÔNG trả lỗi.
// Lý do: khi hiển thị 50 sản phẩm mà một thương hiệu đã bị xóa, ta muốn
// hiển thị 49 sản phẩm còn lại, không muốn cả trang lỗi.
func (s *BrandStore) FindByIDs(_ context.Context, wanted []ids.ID) (map[ids.ID]*domain.Brand, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[ids.ID]*domain.Brand, len(wanted))
	for _, id := range wanted {
		if p, ok := s.byID[id]; ok {
			out[id] = domain.RestoreBrand(p)
		}
	}
	return out, nil
}

func (s *BrandStore) List(_ context.Context, f domain.BrandFilter) ([]*domain.Brand, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*domain.Brand, 0, len(s.byID))
	for _, p := range s.byID {
		if f.Type != "" && p.BrandType != f.Type {
			continue
		}
		if f.Status != "" && p.Status != f.Status {
			continue
		}
		out = append(out, domain.RestoreBrand(p))
	}

	// Thứ tự XÁC ĐỊNH: map trong Go duyệt ngẫu nhiên, nếu không sắp xếp
	// thì test sẽ thất bại ngẫu nhiên (flaky).
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })

	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// ---------------------------------------------------------------- Authorization

type AuthorizationStore struct {
	mu   sync.RWMutex
	byID map[ids.ID]domain.RestoreAuthorizationParams
}

func NewAuthorizationStore() *AuthorizationStore {
	return &AuthorizationStore{byID: make(map[ids.ID]domain.RestoreAuthorizationParams)}
}

func (s *AuthorizationStore) Save(_ context.Context, a *domain.BrandAuthorization) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.byID[a.ID()] = domain.RestoreAuthorizationParams{
		ID:          a.ID(),
		BrandID:     a.BrandID(),
		SellerID:    a.SellerID(),
		DocumentURL: a.DocumentURL(),
		ValidFrom:   a.ValidFrom(),
		ValidUntil:  a.ValidUntil(),
		Status:      a.Status(),
		ApprovedBy:  a.ApprovedBy(),
		ApprovedAt:  a.ApprovedAt(),
		CreatedAt:   a.CreatedAt(),
	}
	return nil
}

func (s *AuthorizationStore) FindByID(_ context.Context, id ids.ID) (*domain.BrandAuthorization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreBrandAuthorization(p), nil
}

// FindActiveForSeller tìm giấy ủy quyền ĐÃ DUYỆT của seller cho brand.
//
// Nếu có nhiều giấy (gia hạn nhiều lần), trả giấy có hạn xa nhất — đó là
// giấy còn hiệu lực lâu nhất.
func (s *AuthorizationStore) FindActiveForSeller(
	_ context.Context, brandID, sellerID ids.ID,
) (*domain.BrandAuthorization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var best *domain.RestoreAuthorizationParams
	for i := range s.byID {
		p := s.byID[i]
		if p.BrandID != brandID || p.SellerID != sellerID {
			continue
		}
		if p.Status != domain.AuthApproved {
			continue
		}
		if best == nil || p.ValidUntil.After(best.ValidUntil) {
			cp := p
			best = &cp
		}
	}
	if best == nil {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreBrandAuthorization(*best), nil
}

func (s *AuthorizationStore) FindExpiring(
	_ context.Context, now time.Time, withinDays int,
) ([]*domain.BrandAuthorization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	window := time.Duration(withinDays) * 24 * time.Hour

	out := make([]*domain.BrandAuthorization, 0)
	for _, p := range s.byID {
		a := domain.RestoreBrandAuthorization(p)
		if a.ExpiresWithin(window, now) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ValidUntil().Before(out[j].ValidUntil()) })
	return out, nil
}

// ---------------------------------------------------------------- Collection

type CollectionStore struct {
	mu     sync.RWMutex
	byID   map[ids.ID]domain.RestoreCollectionParams
	bySlug map[string]ids.ID
}

func NewCollectionStore() *CollectionStore {
	return &CollectionStore{
		byID:   make(map[ids.ID]domain.RestoreCollectionParams),
		bySlug: make(map[string]ids.ID),
	}
}

func (s *CollectionStore) Save(_ context.Context, c *domain.Collection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.bySlug[c.Slug()]; ok && existingID != c.ID() {
		return domain.ErrSlugTaken
	}
	if old, ok := s.byID[c.ID()]; ok && old.Slug != c.Slug() {
		delete(s.bySlug, old.Slug)
	}

	s.byID[c.ID()] = domain.RestoreCollectionParams{
		ID:              c.ID(),
		BrandID:         c.BrandID(),
		Name:            c.Name(),
		Slug:            c.Slug(),
		Season:          c.Season(),
		Theme:           c.Theme(),
		LaunchDate:      c.LaunchDate(),
		EndOfSeasonDate: c.EndOfSeason(),
		Status:          c.Status(),
		CreatedAt:       c.CreatedAt(),
		UpdatedAt:       c.UpdatedAt(),
	}
	s.bySlug[c.Slug()] = c.ID()
	return nil
}

func (s *CollectionStore) FindByID(_ context.Context, id ids.ID) (*domain.Collection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreCollection(p), nil
}

func (s *CollectionStore) FindBySlug(_ context.Context, slug string) (*domain.Collection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.bySlug[slug]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreCollection(s.byID[id]), nil
}

func (s *CollectionStore) FindByBrand(_ context.Context, brandID ids.ID) ([]*domain.Collection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*domain.Collection, 0)
	for _, p := range s.byID {
		if p.BrandID == brandID {
			out = append(out, domain.RestoreCollection(p))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LaunchDate().After(out[j].LaunchDate())
	})
	return out, nil
}

func (s *CollectionStore) FindByStatus(_ context.Context, status domain.CollectionStatus) ([]*domain.Collection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*domain.Collection, 0)
	for _, p := range s.byID {
		if p.Status == status {
			out = append(out, domain.RestoreCollection(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

// ---------------------------------------------------------------- Category

type CategoryStore struct {
	mu     sync.RWMutex
	byID   map[ids.ID]domain.RestoreCategoryParams
	bySlug map[string]ids.ID
}

func NewCategoryStore() *CategoryStore {
	return &CategoryStore{
		byID:   make(map[ids.ID]domain.RestoreCategoryParams),
		bySlug: make(map[string]ids.ID),
	}
}

func (s *CategoryStore) Save(_ context.Context, c *domain.Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.bySlug[c.Slug()]; ok && existingID != c.ID() {
		return domain.ErrSlugTaken
	}
	if old, ok := s.byID[c.ID()]; ok && old.Slug != c.Slug() {
		delete(s.bySlug, old.Slug)
	}

	s.byID[c.ID()] = domain.RestoreCategoryParams{
		ID:           c.ID(),
		ParentID:     c.ParentID(),
		Name:         c.Name(),
		Slug:         c.Slug(),
		Depth:        c.Depth(),
		DisplayOrder: c.DisplayOrder(),
		Status:       c.Status(),
		CreatedAt:    c.CreatedAt(),
		UpdatedAt:    c.UpdatedAt(),
	}
	s.bySlug[c.Slug()] = c.ID()
	return nil
}

func (s *CategoryStore) FindByID(_ context.Context, id ids.ID) (*domain.Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreCategory(p), nil
}

func (s *CategoryStore) FindBySlug(_ context.Context, slug string) (*domain.Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.bySlug[slug]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreCategory(s.byID[id]), nil
}

func (s *CategoryStore) FindChildren(_ context.Context, parentID ids.ID) ([]*domain.Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*domain.Category, 0)
	for _, p := range s.byID {
		if p.ParentID == parentID {
			out = append(out, domain.RestoreCategory(p))
		}
	}
	sortCategories(out)
	return out, nil
}

func (s *CategoryStore) FindAll(_ context.Context) ([]*domain.Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*domain.Category, 0, len(s.byID))
	for _, p := range s.byID {
		out = append(out, domain.RestoreCategory(p))
	}
	sortCategories(out)
	return out, nil
}

// sortCategories sắp xếp theo độ sâu, rồi thứ tự hiển thị, rồi tên.
func sortCategories(cs []*domain.Category) {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Depth() != cs[j].Depth() {
			return cs[i].Depth() < cs[j].Depth()
		}
		if cs[i].DisplayOrder() != cs[j].DisplayOrder() {
			return cs[i].DisplayOrder() < cs[j].DisplayOrder()
		}
		return cs[i].Name() < cs[j].Name()
	})
}

// ---------------------------------------------------------------- SizeChart

type SizeChartStore struct {
	mu   sync.RWMutex
	byID map[ids.ID]domain.RestoreSizeChartParams
}

func NewSizeChartStore() *SizeChartStore {
	return &SizeChartStore{byID: make(map[ids.ID]domain.RestoreSizeChartParams)}
}

func (s *SizeChartStore) Save(_ context.Context, sc *domain.SizeChart) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.byID[sc.ID()] = domain.RestoreSizeChartParams{
		ID:          sc.ID(),
		BrandID:     sc.BrandID(),
		ProductType: sc.ProductType(),
		System:      sc.System(),
		Note:        sc.Note(),
		Entries:     sc.Entries(), // Entries() đã trả bản sao
		CreatedAt:   sc.CreatedAt(),
		UpdatedAt:   sc.UpdatedAt(),
	}
	return nil
}

func (s *SizeChartStore) FindByID(_ context.Context, id ids.ID) (*domain.SizeChart, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.RestoreSizeChart(p), nil
}

func (s *SizeChartStore) FindForBrandAndType(
	_ context.Context, brandID ids.ID, pt domain.ProductType,
) (*domain.SizeChart, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.byID {
		if p.BrandID == brandID && p.ProductType == pt {
			return domain.RestoreSizeChart(p), nil
		}
	}
	return nil, domain.ErrNotFound
}
