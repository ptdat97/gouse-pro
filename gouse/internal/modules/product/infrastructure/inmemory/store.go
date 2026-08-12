// Package inmemory cài đặt các port của product bằng bộ nhớ.
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
	"strings"
	"sync"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

// ProductStore lưu sản phẩm trong bộ nhớ.
//
// Dùng RWMutex vì đọc nhiều hơn ghi rất nhiều, và vì test có thể chạy
// song song.
type ProductStore struct {
	mu     sync.RWMutex
	byID   map[ids.ID]domain.RestoreProductParams
	bySlug map[string]ids.ID

	// skuCodeToProduct mô phỏng ràng buộc UNIQUE toàn hệ thống trên mã SKU
	// (quy tắc 1, mục 12) và phục vụ tra ngược khi quét mã vạch.
	skuCodeToProduct map[string]ids.ID

	// skuIDToProduct phục vụ FindBySKUIDs — module cart/order giữ sku_id
	// và cần tra về sản phẩm để hiển thị tên, ảnh.
	skuIDToProduct map[ids.ID]ids.ID
}

func NewProductStore() *ProductStore {
	return &ProductStore{
		byID:             make(map[ids.ID]domain.RestoreProductParams),
		bySlug:           make(map[string]ids.ID),
		skuCodeToProduct: make(map[string]ids.ID),
		skuIDToProduct:   make(map[ids.ID]ids.ID),
	}
}

// snapshot chuyển aggregate thành dữ liệu thuần để lưu.
//
// Lưu bản chụp thay vì con trỏ là CÓ CHỦ ĐÍCH: nếu lưu con trỏ, bên gọi
// sửa aggregate sau khi Save sẽ vô tình thay đổi dữ liệu "đã lưu" — điều
// không xảy ra với database thật.
//
// Với product, bản chụp phải đi SÂU: Variant và SKU thuộc cùng aggregate,
// chụp nông sẽ vẫn chia sẻ con trỏ và bỏ lọt đúng loại lỗi ta muốn bắt.
func snapshot(p *domain.Product) domain.RestoreProductParams {
	variants := p.Variants()
	copied := make([]*domain.Variant, 0, len(variants))
	for _, v := range variants {
		copied = append(copied, copyVariant(v))
	}

	return domain.RestoreProductParams{
		ID:                  p.ID(),
		BrandID:             p.BrandID(),
		CollectionID:        p.CollectionID(),
		CategoryID:          p.CategoryID(),
		SizeChartID:         p.SizeChartID(),
		Name:                p.Name(),
		Slug:                p.Slug(),
		Description:         p.Description(),
		CareInstructions:    p.CareInstructions(),
		MaterialComposition: p.MaterialComposition(),
		OriginCountry:       p.OriginCountry(),
		ProductType:         p.Type(),
		GenderTarget:        p.GenderTarget(),
		Status:              p.Status(),
		RejectionReason:     p.RejectionReason(),
		CreatedBySellerID:   p.CreatedBySellerID(),
		Images:              p.Images(), // đã là bản sao
		Variants:            copied,
		PublishedAt:         p.PublishedAt(),
		CreatedAt:           p.CreatedAt(),
		UpdatedAt:           p.UpdatedAt(),
	}
}

func copyVariant(v *domain.Variant) *domain.Variant {
	skus := v.SKUs()
	copiedSKUs := make([]*domain.SKU, 0, len(skus))
	for _, s := range skus {
		copiedSKUs = append(copiedSKUs, copySKU(s))
	}

	return domain.RestoreVariant(domain.RestoreVariantParams{
		ID:           v.ID(),
		ProductID:    v.ProductID(),
		Attributes:   v.Attributes(), // đã là bản sao
		Images:       v.Images(),     // đã là bản sao
		DisplayOrder: v.DisplayOrder(),
		Status:       v.Status(),
		SKUs:         copiedSKUs,
		CreatedAt:    v.CreatedAt(),
		UpdatedAt:    v.UpdatedAt(),
	})
}

func copySKU(s *domain.SKU) *domain.SKU {
	return domain.RestoreSKU(domain.RestoreSKUParams{
		ID:         s.ID(),
		VariantID:  s.VariantID(),
		Code:       s.Code(),
		Barcode:    s.Barcode(),
		WeightGram: s.WeightGram(),
		Dimensions: s.Dimensions(), // struct giá trị, sao chép tự nhiên
		Status:     s.Status(),
		CreatedAt:  s.CreatedAt(),
		UpdatedAt:  s.UpdatedAt(),
	})
}

// restore dựng lại aggregate từ bản chụp.
//
// Chụp lại LẦN NỮA khi đọc: nếu trả thẳng con trỏ đã lưu, bên gọi sửa
// aggregate vừa đọc sẽ làm hỏng dữ liệu trong kho.
func restore(p domain.RestoreProductParams) *domain.Product {
	variants := make([]*domain.Variant, 0, len(p.Variants))
	for _, v := range p.Variants {
		variants = append(variants, copyVariant(v))
	}
	p.Variants = variants
	p.Images = append([]string(nil), p.Images...)
	return domain.RestoreProduct(p)
}

func (s *ProductStore) Save(_ context.Context, p *domain.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Mô phỏng ràng buộc UNIQUE trên slug.
	if existingID, ok := s.bySlug[p.Slug()]; ok && existingID != p.ID() {
		return domain.ErrSlugTaken
	}

	// Quy tắc 1: sku_code duy nhất TOÀN HỆ THỐNG, không chỉ trong sản phẩm.
	// Kiểm tra ở đây mô phỏng ràng buộc UNIQUE của database — nếu chỉ dựa
	// vào kiểm tra ở tầng application, hai request đồng thời vẫn lọt.
	for _, sku := range p.SKUs() {
		if owner, ok := s.skuCodeToProduct[sku.Code()]; ok && owner != p.ID() {
			return domain.ErrSKUCodeTaken
		}
	}

	// Nếu là cập nhật, dọn các ánh xạ cũ trước khi ghi lại. Không dọn thì
	// SKU đã bị xóa khỏi sản phẩm vẫn chiếm mã của nó mãi mãi.
	if old, ok := s.byID[p.ID()]; ok {
		if old.Slug != p.Slug() {
			delete(s.bySlug, old.Slug)
		}
		s.forgetSKUs(old)
	}

	s.byID[p.ID()] = snapshot(p)
	s.bySlug[p.Slug()] = p.ID()
	for _, sku := range p.SKUs() {
		s.skuCodeToProduct[sku.Code()] = p.ID()
		s.skuIDToProduct[sku.ID()] = p.ID()
	}
	return nil
}

// forgetSKUs xóa ánh xạ SKU của một bản chụp cũ. Bên gọi phải giữ khóa.
func (s *ProductStore) forgetSKUs(old domain.RestoreProductParams) {
	for _, v := range old.Variants {
		for _, sku := range v.SKUs() {
			delete(s.skuCodeToProduct, sku.Code())
			delete(s.skuIDToProduct, sku.ID())
		}
	}
}

func (s *ProductStore) FindByID(_ context.Context, id ids.ID) (*domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return restore(p), nil
}

func (s *ProductStore) FindBySlug(_ context.Context, slug string) (*domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.bySlug[slug]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return restore(s.byID[id]), nil
}

// FindByIDs trả về map chỉ chứa những id TÌM THẤY.
//
// Không trả lỗi khi thiếu: hiển thị 49/50 sản phẩm tốt hơn là cả trang lỗi
// chỉ vì một sản phẩm đã bị lưu trữ.
func (s *ProductStore) FindByIDs(_ context.Context, list []ids.ID) (map[ids.ID]*domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[ids.ID]*domain.Product, len(list))
	for _, id := range list {
		if p, ok := s.byID[id]; ok {
			out[id] = restore(p)
		}
	}
	return out, nil
}

func (s *ProductStore) FindByCollection(_ context.Context, collectionID ids.ID) ([]*domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*domain.Product
	for _, p := range s.byID {
		if p.CollectionID == collectionID {
			out = append(out, restore(p))
		}
	}
	sortProducts(out)
	return out, nil
}

func (s *ProductStore) FindBySKUCode(_ context.Context, code string) (*domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Chuẩn hóa giống domain.NewSKU, nếu không thì quét mã vạch chữ thường
	// sẽ không tìm ra hàng.
	id, ok := s.skuCodeToProduct[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return restore(s.byID[id]), nil
}

func (s *ProductStore) FindBySKUIDs(_ context.Context, skuIDs []ids.ID) (map[ids.ID]*domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[ids.ID]*domain.Product, len(skuIDs))
	for _, skuID := range skuIDs {
		if productID, ok := s.skuIDToProduct[skuID]; ok {
			if p, ok := s.byID[productID]; ok {
				out[skuID] = restore(p)
			}
		}
	}
	return out, nil
}

func (s *ProductStore) List(_ context.Context, f domain.Filter) ([]*domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []*domain.Product
	for _, p := range s.byID {
		if !matches(p, f) {
			continue
		}
		out = append(out, restore(p))
	}

	sortProducts(out)
	return paginate(out, f), nil
}

// matches áp dụng bộ lọc.
//
// LƯU Ý BẢO MẬT: lọc theo SellerID nằm ở ĐÂY, trong "truy vấn", không ở
// tầng hiển thị. Dữ liệu của seller khác không bao giờ rời khỏi kho.
func matches(p domain.RestoreProductParams, f domain.Filter) bool {
	if !f.BrandID.IsZero() && p.BrandID != f.BrandID {
		return false
	}
	if !f.CategoryID.IsZero() && p.CategoryID != f.CategoryID {
		return false
	}
	if !f.CollectionID.IsZero() && p.CollectionID != f.CollectionID {
		return false
	}
	if !f.SellerID.IsZero() && p.CreatedBySellerID != f.SellerID {
		return false
	}
	if f.ProductType != "" && p.ProductType != f.ProductType {
		return false
	}
	if f.Gender != "" && p.GenderTarget != f.Gender {
		return false
	}
	if f.Status != "" && p.Status != f.Status {
		return false
	}
	if f.OnlyVisible && p.Status != domain.StatusActive {
		return false
	}
	return true
}

// sortProducts sắp xếp theo id để kết quả ỔN ĐỊNH.
//
// Duyệt map trong Go có thứ tự ngẫu nhiên. Không sắp xếp thì phân trang
// sẽ trả kết quả khác nhau giữa các lần gọi và khách sẽ thấy sản phẩm
// nhảy qua lại giữa các trang.
func sortProducts(list []*domain.Product) {
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID() < list[j].ID()
	})
}

func paginate(list []*domain.Product, f domain.Filter) []*domain.Product {
	if f.Offset > 0 {
		if f.Offset >= len(list) {
			return nil
		}
		list = list[f.Offset:]
	}
	if f.Limit > 0 && f.Limit < len(list) {
		list = list[:f.Limit]
	}
	return list
}
