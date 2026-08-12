package domain

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// ErrNotFound là lỗi chung khi không tìm thấy bản ghi.
//
// Định nghĩa ở domain (không phải infrastructure) để tầng application
// xử lý được mà không cần biết kho lưu trữ là gì.
var ErrNotFound = errors.New("catalog: không tìm thấy")

// ErrSlugTaken khi slug đã được dùng — slug là định danh trong URL,
// phải duy nhất để không có hai trang cùng đường dẫn.
var ErrSlugTaken = errors.New("catalog: slug đã được sử dụng")

// ErrDuplicateAuthorization khi seller đã có ủy quyền đang hiệu lực cho
// thương hiệu này.
//
// Hai bản ghi APPROVED cùng lúc sẽ khiến việc thu hồi không dứt điểm: thu
// hồi một cái, cái còn lại vẫn cho phép bán.
var ErrDuplicateAuthorization = errors.New("catalog: seller đã có ủy quyền đang hiệu lực cho thương hiệu này")

// ErrDuplicateSizeChart khi thương hiệu đã có bảng size cho loại sản phẩm này.
//
// Hai bảng cùng (brand, product_type) thì không biết dùng bảng nào, và
// khách sẽ thấy số đo khác nhau ở các trang khác nhau.
var ErrDuplicateSizeChart = errors.New("catalog: thương hiệu đã có bảng size cho loại sản phẩm này")

// BrandRepository là PORT do domain định nghĩa.
//
// Đây là đảo ngược phụ thuộc: domain nói "tôi cần lấy brand theo id",
// infrastructure quyết định "lấy từ PostgreSQL hay từ bộ nhớ".
//
// Nhờ vậy domain kiểm thử được mà không cần database — điều kiện để
// chạy hàng nghìn test quy tắc nghiệp vụ trong vài giây.
type BrandRepository interface {
	Save(ctx context.Context, b *Brand) error
	FindByID(ctx context.Context, id ids.ID) (*Brand, error)
	FindBySlug(ctx context.Context, slug string) (*Brand, error)

	// FindByIDs nhận DANH SÁCH, không nhận một id.
	//
	// Thiết kế này bắt buộc bên gọi nghĩ theo lô và tránh vấn đề N+1:
	// hiển thị 50 sản phẩm cần 1 lời gọi, không phải 50.
	// Xem docs/03-architecture/modular-monolith.md mục 6.
	FindByIDs(ctx context.Context, ids []ids.ID) (map[ids.ID]*Brand, error)

	List(ctx context.Context, f BrandFilter) ([]*Brand, error)
}

// BrandFilter là điều kiện lọc danh sách thương hiệu.
type BrandFilter struct {
	Type   BrandType
	Status Status
	Limit  int
}

// AuthorizationRepository quản lý giấy ủy quyền thương hiệu.
type AuthorizationRepository interface {
	Save(ctx context.Context, a *BrandAuthorization) error
	FindByID(ctx context.Context, id ids.ID) (*BrandAuthorization, error)

	// FindActiveForSeller tìm giấy ủy quyền của một seller cho một brand.
	// Trả ErrNotFound nếu không có.
	FindActiveForSeller(ctx context.Context, brandID, sellerID ids.ID) (*BrandAuthorization, error)

	// FindExpiring tìm giấy sắp hết hạn — dùng cho job cảnh báo.
	FindExpiring(ctx context.Context, withinDays int) ([]*BrandAuthorization, error)
}

// CollectionRepository quản lý bộ sưu tập.
type CollectionRepository interface {
	Save(ctx context.Context, c *Collection) error
	FindByID(ctx context.Context, id ids.ID) (*Collection, error)
	FindBySlug(ctx context.Context, slug string) (*Collection, error)
	FindByBrand(ctx context.Context, brandID ids.ID) ([]*Collection, error)

	// FindByStatus dùng cho job định kỳ chuyển trạng thái theo lịch.
	FindByStatus(ctx context.Context, status CollectionStatus) ([]*Collection, error)
}

// CategoryRepository quản lý cây danh mục.
type CategoryRepository interface {
	Save(ctx context.Context, c *Category) error
	FindByID(ctx context.Context, id ids.ID) (*Category, error)
	FindBySlug(ctx context.Context, slug string) (*Category, error)
	FindChildren(ctx context.Context, parentID ids.ID) ([]*Category, error)

	// FindAll trả toàn bộ cây — danh mục ít và đổi hiếm nên tải hết
	// rẻ hơn truy vấn đệ quy.
	FindAll(ctx context.Context) ([]*Category, error)
}

// SizeChartRepository quản lý bảng size.
type SizeChartRepository interface {
	Save(ctx context.Context, s *SizeChart) error
	FindByID(ctx context.Context, id ids.ID) (*SizeChart, error)

	// FindForBrandAndType tìm bảng size áp dụng cho một loại sản phẩm
	// của một thương hiệu.
	FindForBrandAndType(ctx context.Context, brandID ids.ID, pt ProductType) (*SizeChart, error)
}
