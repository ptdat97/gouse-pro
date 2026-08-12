// Package catalog là module quản lý danh mục: thương hiệu, bộ sưu tập,
// danh mục sản phẩm, bảng size.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module. Module khác CHỈ được import package
// này, không được import catalog/domain, catalog/application hay bất kỳ
// package con nào — quy tắc R1 của cmd/archcheck cưỡng chế điều này và
// vi phạm làm CI thất bại.
//
// Vì sao: nếu module khác import sâu vào domain, mọi thay đổi nội bộ của
// catalog trở thành thay đổi phá vỡ, và catalog không tách thành service
// riêng được. Xem docs/adr/0005-module-boundaries.md.
package catalog

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module catalog.
//
// Mọi phương thức nhận và trả DTO của package này, KHÔNG trả domain object.
// Lý do: nếu trả *domain.Brand, module khác gọi được Brand.Deactivate() —
// phá vỡ tính đóng gói và cho phép sửa trạng thái ngoài tầm kiểm soát của
// module sở hữu.
type API interface {
	// ---- Thương hiệu ----

	GetBrand(ctx context.Context, brandID string) (*BrandView, error)

	// GetBrandsByIDs nhận DANH SÁCH để tránh vấn đề N+1.
	//
	// Hiển thị 50 sản phẩm cần 1 lời gọi, không phải 50. Interface được
	// thiết kế theo lô ngay từ đầu để bên gọi không có lựa chọn sai.
	GetBrandsByIDs(ctx context.Context, brandIDs []string) (map[string]BrandView, error)

	// CanSellerCreateOffer kiểm tra seller có được bán thương hiệu không.
	//
	// Đây là năng lực quan trọng nhất mà module marketplace sẽ gọi trước
	// khi cho tạo offer — cơ chế chống hàng giả.
	CanSellerCreateOffer(ctx context.Context, brandID, sellerID string) (SellPermission, error)

	// ---- Bộ sưu tập ----

	GetCollection(ctx context.Context, collectionID string) (*CollectionView, error)

	// ---- Danh mục ----

	GetCategoryTree(ctx context.Context) ([]CategoryView, error)
	GetCategory(ctx context.Context, categoryID string) (*CategoryView, error)

	// ---- Bảng size ----

	GetSizeChart(ctx context.Context, sizeChartID string) (*SizeChartView, error)

	// GetSizeChartFor tra bảng size theo thương hiệu và loại sản phẩm.
	//
	// Module product gọi khi hiển thị trang sản phẩm — bảng size có số đo
	// thực tế là một trong ba yếu tố giảm trực tiếp tỷ lệ hoàn hàng.
	GetSizeChartFor(ctx context.Context, brandID, productType string) (*SizeChartView, error)
}

// ---------------------------------------------------------------- DTO

// BrandView là dữ liệu thương hiệu cho module khác — CHỈ ĐỌC.
type BrandView struct {
	ID              string
	Name            string
	Slug            string
	Description     string
	LogoURL         string
	BrandType       string
	ProtectionLevel string
	CountryOfOrigin string
	Status          string
	IsOwnBrand      bool
}

// SellPermission là kết quả kiểm tra quyền bán.
//
// Chứa cả LÝ DO và HÀNH ĐỘNG CẦN LÀM, không chỉ true/false — module gọi
// cần thông tin này để hiển thị thông báo hữu ích cho seller.
//
// "Thương hiệu này yêu cầu giấy ủy quyền. [Tải lên giấy ủy quyền]"
// hữu ích hơn nhiều so với "Không thể tạo offer".
type SellPermission struct {
	Allowed bool

	// Reason là mã máy đọc được: OK, NO_AUTHORIZATION, AUTHORIZATION_EXPIRED,
	// BRAND_NOT_FOUND, BRAND_INACTIVE, RESTRICTED_TO_DESIGNATED_SELLER.
	Reason string

	ProtectionLevel string

	// RequiredAction gợi ý hành động: UPLOAD_AUTHORIZATION,
	// RENEW_AUTHORIZATION, CONTACT_PLATFORM.
	RequiredAction string
}

// CollectionView là dữ liệu bộ sưu tập cho module khác.
type CollectionView struct {
	ID          string
	BrandID     string
	Name        string
	Slug        string
	Season      string
	Theme       string
	LaunchDate  time.Time
	EndOfSeason time.Time
	Status      string

	// IsVisible cho biết có hiển thị cho khách không — module product
	// dùng để quyết định hiện sản phẩm thuộc bộ sưu tập này hay chưa.
	IsVisible bool

	// WeeksRemaining là số tuần còn lại của mùa.
	//
	// Đầu vào cho quyết định bổ sung hàng: nếu ít hơn lead time nhà cung
	// cấp thì KHÔNG nên đặt thêm — hàng về sẽ không kịp bán.
	WeeksRemaining int
}

// CategoryView là một nút trong cây danh mục.
type CategoryView struct {
	ID           string
	ParentID     string
	Name         string
	Slug         string
	Depth        int
	DisplayOrder int
	Status       string
	Children     []CategoryView
}

// SizeChartView là bảng size với SỐ ĐO THỰC TẾ.
type SizeChartView struct {
	ID          string
	BrandID     string
	ProductType string
	System      string
	Note        string
	Entries     []SizeEntryView
}

// SizeEntryView là một dòng trong bảng size.
//
// Measurements dùng map vì số đo khác nhau theo loại sản phẩm:
// áo có chest_cm, quần có waist_cm, giày có foot_length_cm.
type SizeEntryView struct {
	Size         string
	Measurements map[string]string
}

// ---------------------------------------------------------------- Lỗi

// Các lỗi mà module khác có thể so sánh bằng errors.Is.
var (
	// ErrNotFound khi không tìm thấy tài nguyên.
	ErrNotFound = errNotFound{}
	// ErrInvalidID khi định danh sai định dạng.
	ErrInvalidID = errInvalidID{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "catalog: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "catalog: định danh không hợp lệ" }

// ---------------------------------------------------------------- Tiền tố ID

// Các tiền tố ID mà module khác cần khi kiểm tra tham số đầu vào.
const (
	BrandIDPrefix      = string(ids.PrefixBrand)
	CollectionIDPrefix = string(ids.PrefixCollection)
	CategoryIDPrefix   = string(ids.PrefixCategory)
	SizeChartIDPrefix  = string(ids.PrefixSizeChart)
)
