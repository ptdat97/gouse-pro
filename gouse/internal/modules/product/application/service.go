// Package application chứa các use case của module product.
//
// Tầng này điều phối: gọi domain để áp dụng quy tắc nghiệp vụ, gọi
// repository để đọc/ghi, gọi module khác qua port. Nó KHÔNG chứa quy tắc
// nghiệp vụ — quy tắc nằm ở domain.
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

// Clock cho phép test kiểm soát thời gian.
//
// Không dùng time.Now() trực tiếp trong use case: test về thời điểm xuất
// bản cần thời gian xác định, không phụ thuộc lúc chạy test.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// SystemClock là đồng hồ thật, dùng ở production.
var SystemClock Clock = systemClock{}

// CatalogPort là những gì module product CẦN từ catalog.
//
// Định nghĩa Ở ĐÂY, ở phía người dùng, chứ không dùng thẳng interface của
// catalog. Đây là nguyên tắc "interface thuộc về bên gọi":
//
//   - product chỉ khai báo đúng 3 năng lực nó dùng, không phải toàn bộ API
//     của catalog
//   - test của product giả lập được catalog mà không cần dựng cả module
//   - catalog thêm phương thức mới không ảnh hưởng product
//
// Adapter nối port này với catalog.API nằm ở tầng ngoài (module.go), nên
// tầng application KHÔNG import catalog — giữ đúng quy tắc R1 và R8.
type CatalogPort interface {
	// BrandExists kiểm tra thương hiệu có tồn tại và đang hoạt động không.
	BrandExists(ctx context.Context, brandID ids.ID) (bool, error)

	// CanSellerSellBrand kiểm tra seller có được bán thương hiệu này không.
	//
	// Đây là hàng rào CHỐNG HÀNG GIẢ. Quy tắc nằm ở catalog (nơi sở hữu dữ
	// liệu ủy quyền); product chỉ hỏi và tuân theo.
	CanSellerSellBrand(ctx context.Context, brandID, sellerID ids.ID) (allowed bool, reason string, err error)

	// SizeChartExistsFor kiểm tra có bảng size cho (thương hiệu, loại sản
	// phẩm) không.
	SizeChartExistsFor(ctx context.Context, brandID ids.ID, productType string) (ids.ID, bool, error)
}

// Service là tầng application của module product.
type Service struct {
	products      domain.ProductRepository
	searchSignals SearchSignalPublisher
	catalog       CatalogPort
	clock         Clock
}

// Deps gom các phụ thuộc — dễ đọc hơn hàm nhiều tham số cùng kiểu interface.
// SearchSignalPublisher báo rằng khách tìm mà KHÔNG ra kết quả.
//
// Là PORT do tầng application định nghĩa: nó không biết event bus hay
// supply-chain tồn tại. Chiều phụ thuộc vẫn là `product` → không ai, vì
// bên nhận chỉ nghe event.
//
// # Vì sao event chứ không phải gọi đồng bộ
//
// "Khách tìm không ra kết quả" là một SỰ VIỆC ĐÃ XẢY RA, không phải câu
// hỏi cần trả lời để quyết định. Kết quả tìm kiếm trả về không phụ thuộc
// việc ghi tín hiệu có thành công hay không — xem ADR-0006 phần 3.
type SearchSignalPublisher interface {
	PublishSearchNoResult(ctx context.Context, query string) error
}

type Deps struct {
	Products domain.ProductRepository
	Catalog  CatalogPort
	Clock    Clock

	// SearchSignals có thể nil: tìm kiếm vẫn chạy, chỉ là không ghi tín
	// hiệu. Không chặn tìm kiếm vì mất một tín hiệu nhẹ hơn nhiều so với
	// mất khả năng tìm sản phẩm.
	SearchSignals SearchSignalPublisher
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		products:      d.Products,
		catalog:       d.Catalog,
		searchSignals: d.SearchSignals,
		clock:         clock,
	}
}

// Now trả về thời điểm hiện tại theo đồng hồ của service.
func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Tạo

// CreateProductInput là dữ liệu tạo sản phẩm mới.
type CreateProductInput struct {
	BrandID      ids.ID
	CollectionID ids.ID
	CategoryID   ids.ID
	SizeChartID  ids.ID

	Name        string
	Slug        string
	Description string

	CareInstructions    string
	MaterialComposition string
	OriginCountry       string

	ProductType  domain.ProductType
	GenderTarget domain.GenderTarget

	// SellerID rỗng nghĩa là nền tảng tự tạo (danh mục chuẩn).
	SellerID ids.ID
	Images   []string
}

// CreateProduct tạo sản phẩm mới ở trạng thái DRAFT.
//
// Kiểm tra quyền bán TRƯỚC khi tạo: để seller tạo xong cả sản phẩm rồi mới
// báo "bạn không được bán thương hiệu này" là lãng phí công sức của họ và
// tạo ra dữ liệu rác phải dọn.
func (s *Service) CreateProduct(ctx context.Context, in CreateProductInput) (*domain.Product, error) {
	ok, err := s.catalog.BrandExists(ctx, in.BrandID)
	if err != nil {
		return nil, fmt.Errorf("kiểm tra thương hiệu: %w", err)
	}
	if !ok {
		return nil, ErrBrandNotFound
	}

	// Hàng rào chống hàng giả: chỉ kiểm tra khi có seller. Nền tảng tự tạo
	// sản phẩm cho thương hiệu của mình thì không cần ủy quyền.
	if !in.SellerID.IsZero() {
		allowed, reason, err := s.catalog.CanSellerSellBrand(ctx, in.BrandID, in.SellerID)
		if err != nil {
			return nil, fmt.Errorf("kiểm tra quyền bán: %w", err)
		}
		if !allowed {
			return nil, &NotAuthorizedError{BrandID: in.BrandID, SellerID: in.SellerID, Reason: reason}
		}
	}

	// Tự tra bảng size nếu bên gọi không chỉ định.
	//
	// Bảng size gắn với (thương hiệu, loại sản phẩm) và catalog đã biết —
	// bắt người đăng bán tự tìm id bảng size là nguồn gốc của việc gắn sai
	// hoặc bỏ trống.
	sizeChartID := in.SizeChartID
	if sizeChartID.IsZero() && in.ProductType.NeedsSizeChart() {
		if id, found, err := s.catalog.SizeChartExistsFor(ctx, in.BrandID, string(in.ProductType)); err == nil && found {
			sizeChartID = id
		}
	}

	p, err := domain.NewProduct(domain.NewProductParams{
		BrandID:             in.BrandID,
		CollectionID:        in.CollectionID,
		CategoryID:          in.CategoryID,
		SizeChartID:         sizeChartID,
		Name:                in.Name,
		Slug:                in.Slug,
		Description:         in.Description,
		CareInstructions:    in.CareInstructions,
		MaterialComposition: in.MaterialComposition,
		OriginCountry:       in.OriginCountry,
		ProductType:         in.ProductType,
		GenderTarget:        in.GenderTarget,
		CreatedBySellerID:   in.SellerID,
		Images:              in.Images,
		Now:                 s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}

	if err := s.products.Save(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// AddVariantInput là dữ liệu thêm biến thể kèm các SKU của nó.
type AddVariantInput struct {
	ProductID  ids.ID
	Attributes map[string]string
	Images     []string
	SKUs       []NewSKUInput
}

// NewSKUInput là dữ liệu một SKU.
type NewSKUInput struct {
	Code       string
	Barcode    string
	WeightGram int
	Dimensions domain.Dimensions
}

// AddVariant thêm biến thể và SKU vào sản phẩm.
//
// Thêm cả biến thể lẫn SKU trong MỘT use case vì biến thể không có SKU là
// vô nghĩa: không đếm kho được, không bán được. Tách làm hai lời gọi sẽ tạo
// ra trạng thái trung gian hỏng nếu lời gọi thứ hai thất bại.
func (s *Service) AddVariant(ctx context.Context, in AddVariantInput) (*domain.Product, error) {
	p, err := s.products.FindByID(ctx, in.ProductID)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()

	v, err := domain.NewVariant(domain.NewVariantParams{
		Attributes: in.Attributes,
		Images:     in.Images,
		Now:        now,
	})
	if err != nil {
		return nil, err
	}

	for _, si := range in.SKUs {
		// Quy tắc 1: mã SKU duy nhất toàn hệ thống. Kiểm tra ở đây để báo
		// lỗi rõ ràng; ràng buộc ở tầng kho là chốt chặn cuối cho trường
		// hợp hai request đồng thời.
		if existing, err := s.products.FindBySKUCode(ctx, si.Code); err == nil && existing.ID() != p.ID() {
			return nil, fmt.Errorf("%w: %s", domain.ErrSKUCodeTaken, si.Code)
		} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}

		sku, err := domain.NewSKU(domain.NewSKUParams{
			Code:       si.Code,
			Barcode:    si.Barcode,
			WeightGram: si.WeightGram,
			Dimensions: si.Dimensions,
			Now:        now,
		})
		if err != nil {
			return nil, err
		}
		if err := v.AddSKU(sku, now); err != nil {
			return nil, err
		}
	}

	if err := p.AddVariant(v, now); err != nil {
		return nil, err
	}
	if err := s.products.Save(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ---------------------------------------------------------------- Xuất bản

// SubmitForReview gửi sản phẩm đi duyệt.
func (s *Service) SubmitForReview(ctx context.Context, id ids.ID) (*domain.Product, error) {
	p, err := s.products.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := p.SubmitForReview(s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.products.Save(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Approve duyệt và xuất bản sản phẩm.
//
// Kiểm tra LẠI quyền bán ngay trước khi xuất bản. Giữa lúc gửi duyệt và
// lúc duyệt có thể đã nhiều ngày — giấy ủy quyền có thể đã hết hạn hoặc bị
// thu hồi. Không kiểm tra lại nghĩa là hàng không còn được phép bán vẫn
// lên sàn.
func (s *Service) Approve(ctx context.Context, id ids.ID) (*domain.Product, error) {
	p, err := s.products.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if sellerID := p.CreatedBySellerID(); !sellerID.IsZero() {
		allowed, reason, err := s.catalog.CanSellerSellBrand(ctx, p.BrandID(), sellerID)
		if err != nil {
			return nil, fmt.Errorf("kiểm tra quyền bán: %w", err)
		}
		if !allowed {
			return nil, &NotAuthorizedError{BrandID: p.BrandID(), SellerID: sellerID, Reason: reason}
		}
	}

	if err := p.Approve(s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.products.Save(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Reject từ chối sản phẩm kèm lý do.
func (s *Service) Reject(ctx context.Context, id ids.ID, reason string) (*domain.Product, error) {
	p, err := s.products.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := p.Reject(reason, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.products.Save(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Deactivate tạm ngừng bán sản phẩm.
func (s *Service) Deactivate(ctx context.Context, id ids.ID) (*domain.Product, error) {
	return s.changeStatus(ctx, id, func(p *domain.Product, now time.Time) error {
		return p.Deactivate(now)
	})
}

// Reactivate bán lại sản phẩm.
func (s *Service) Reactivate(ctx context.Context, id ids.ID) (*domain.Product, error) {
	return s.changeStatus(ctx, id, func(p *domain.Product, now time.Time) error {
		return p.Reactivate(now)
	})
}

// Archive ngừng bán vĩnh viễn.
func (s *Service) Archive(ctx context.Context, id ids.ID) (*domain.Product, error) {
	return s.changeStatus(ctx, id, func(p *domain.Product, now time.Time) error {
		return p.Archive(now)
	})
}

func (s *Service) changeStatus(
	ctx context.Context, id ids.ID, apply func(*domain.Product, time.Time) error,
) (*domain.Product, error) {
	p, err := s.products.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := apply(p, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.products.Save(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ---------------------------------------------------------------- Đọc

func (s *Service) GetProduct(ctx context.Context, id ids.ID) (*domain.Product, error) {
	return s.products.FindByID(ctx, id)
}

func (s *Service) GetProductBySlug(ctx context.Context, slug string) (*domain.Product, error) {
	return s.products.FindBySlug(ctx, slug)
}

func (s *Service) GetProductsByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*domain.Product, error) {
	return s.products.FindByIDs(ctx, list)
}

func (s *Service) GetProductsBySKUIDs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID]*domain.Product, error) {
	return s.products.FindBySKUIDs(ctx, skuIDs)
}

func (s *Service) GetProductBySKUCode(ctx context.Context, code string) (*domain.Product, error) {
	return s.products.FindBySKUCode(ctx, code)
}

func (s *Service) ListProducts(ctx context.Context, f domain.Filter) ([]*domain.Product, error) {
	return s.products.List(ctx, f)
}

// Search tìm sản phẩm theo từ khóa, và GHI TÍN HIỆU khi không ra kết quả.
//
// # Không ra kết quả là dữ liệu quý nhất ở đây
//
// Dữ liệu bán hàng chỉ cho biết khách mua gì. Tìm kiếm không ra kết quả cho
// biết khách MUỐN gì mà nền tảng KHÔNG CÓ — thứ không suy ra được từ đơn
// hàng, và không tạo ngược được nếu hôm nay không ghi.
//
// Chỉ trả sản phẩm ACTIVE: hàng chưa duyệt không được lộ qua tìm kiếm.
func (s *Service) Search(
	ctx context.Context, query string, limit, offset int,
) ([]*domain.Product, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	found, err := s.products.List(ctx, domain.Filter{
		Query:       query,
		OnlyVisible: true,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, err
	}

	// Ghi tín hiệu SAU khi có kết quả, và KHÔNG chặn nếu ghi hỏng: khách
	// đang đợi kết quả tìm kiếm, không đợi hệ thống ghi số liệu.
	if len(found) == 0 && s.searchSignals != nil && offset == 0 {
		if err := s.searchSignals.PublishSearchNoResult(ctx, query); err != nil {
			return found, nil
		}
	}

	return found, nil
}

// ListVisibleByCollection lấy sản phẩm HIỂN THỊ ĐƯỢC của một bộ sưu tập.
//
// Dùng cho trang bộ sưu tập ở storefront. Lọc theo trạng thái nằm trong
// truy vấn, không ở tầng hiển thị — sản phẩm chưa duyệt không bao giờ rời
// khỏi kho.
func (s *Service) ListVisibleByCollection(ctx context.Context, collectionID ids.ID) ([]*domain.Product, error) {
	return s.products.List(ctx, domain.Filter{
		CollectionID: collectionID,
		OnlyVisible:  true,
	})
}

// ListSellerProducts lấy sản phẩm của MỘT seller.
//
// BẢO MẬT: sellerID là bắt buộc. Nếu rỗng, hàm trả lỗi thay vì trả tất cả
// sản phẩm — một lỗi lập trình ở tầng gọi sẽ thành rò rỉ dữ liệu toàn sàn.
func (s *Service) ListSellerProducts(
	ctx context.Context, sellerID ids.ID, status domain.Status,
) ([]*domain.Product, error) {
	if sellerID.IsZero() {
		return nil, ErrSellerRequired
	}
	return s.products.List(ctx, domain.Filter{SellerID: sellerID, Status: status})
}
