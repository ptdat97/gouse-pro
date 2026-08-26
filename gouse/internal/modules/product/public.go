// Package product là module quản lý sản phẩm: Product → Variant → SKU.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module. Module khác CHỈ được import package
// này, không được import product/domain, product/application hay bất kỳ
// package con nào — quy tắc R1 của cmd/archcheck cưỡng chế điều này và
// vi phạm làm CI thất bại.
//
// Vì sao: nếu module khác import sâu vào domain, mọi thay đổi nội bộ của
// product trở thành thay đổi phá vỡ, và product không tách thành service
// riêng được. Xem docs/adr/0005-module-boundaries.md.
package product

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module product.
//
// Mọi phương thức nhận và trả DTO của package này, KHÔNG trả domain object.
// Lý do: nếu trả *domain.Product, module khác gọi được Product.Approve() —
// phá vỡ tính đóng gói và cho phép xuất bản sản phẩm ngoài tầm kiểm soát
// của module sở hữu.
//
// Chữ ký khớp docs/04-modules/product.md mục 9.
type API interface {
	// ---- Sản phẩm ----

	GetProduct(ctx context.Context, productID string) (*ProductView, error)

	// GetProductsByIDs nhận DANH SÁCH để tránh vấn đề N+1.
	//
	// Hiển thị giỏ hàng 20 món cần 1 lời gọi, không phải 20.
	GetProductsByIDs(ctx context.Context, productIDs []string) (map[string]ProductView, error)

	// ---- Biến thể và SKU ----

	GetVariantsByProduct(ctx context.Context, productID string) ([]VariantView, error)

	// GetSKUsByProduct trả mọi SKU của sản phẩm.
	//
	// Module inventory dùng để biết cần theo dõi tồn kho những mã nào.
	GetSKUsByProduct(ctx context.Context, productID string) ([]SKUView, error)

	// GetSKUsByProducts là bản THEO LÔ của hàm trên.
	//
	// Trang danh sách cần SKU của mọi sản phẩm để hỏi giá; gọi từng cái là
	// N+1. Sản phẩm không tồn tại thì vắng mặt trong kết quả.
	GetSKUsByProducts(ctx context.Context, productIDs []string) (map[string][]SKUView, error)

	// GetProductsBySKUIDs tra ngược từ SKU về sản phẩm.
	//
	// Module cart và order chỉ giữ sku_id; khi hiển thị chúng cần tên và
	// ảnh sản phẩm. Không có hàm này, chúng phải gọi lặp từng cái.
	GetProductsBySKUIDs(ctx context.Context, skuIDs []string) (map[string]ProductView, error)

	// ---- Kiểm tra cho module khác ----

	// IsSellable cho biết SKU có được phép bán không.
	//
	// Module marketplace gọi TRƯỚC khi cho tạo offer. Trả lời "mặt hàng này
	// còn được kinh doanh không", KHÔNG phải "còn hàng không" — tồn kho
	// thuộc module inventory.
	IsSellable(ctx context.Context, skuID string) (bool, error)
}

// ---------------------------------------------------------------- DTO

// ProductView là dữ liệu sản phẩm cho module khác — CHỈ ĐỌC.
type ProductView struct {
	ID           string
	BrandID      string
	CollectionID string
	CategoryID   string
	SizeChartID  string

	Name        string
	Slug        string
	Description string

	// Ba trường đặc thù thời trang, ảnh hưởng trực tiếp tỷ lệ hoàn hàng.
	CareInstructions    string
	MaterialComposition string
	OriginCountry       string

	ProductType  string
	GenderTarget string
	Status       string

	// IsVisible cho biết khách xem được không.
	//
	// LƯU Ý: hiển thị được KHÔNG có nghĩa là mua được — còn cần offer đang
	// hoạt động và còn hàng.
	IsVisible bool

	// SellerID rỗng nghĩa là danh mục chuẩn của nền tảng.
	SellerID string

	Images   []string
	Variants []VariantView
}

// VariantView là một tổ hợp thuộc tính của sản phẩm.
type VariantView struct {
	ID        string
	ProductID string

	// Attributes là tổ hợp thuộc tính, ví dụ {color: "Trắng", size: "M"}.
	Attributes map[string]string

	Color  string
	Size   string
	Images []string
	Status string
	SKUs   []SKUView
}

// SKUView là đơn vị lưu kho.
//
// SKU KHÔNG thuộc về seller nào — đây là định danh hàng hóa chung cho phép
// biết "ba seller đang bán cùng một món hàng".
type SKUView struct {
	ID        string
	VariantID string
	Code      string
	Barcode   string

	WeightGram int
	LengthMM   int
	WidthMM    int
	HeightMM   int

	Status string

	// IsSellable: mặt hàng còn được kinh doanh không (không phải còn hàng không).
	IsSellable bool

	// CanShip: đã đủ thông tin để tính phí vận chuyển chưa.
	//
	// Thiếu thông tin thì hãng vận chuyển tính theo mặc định, thường cao
	// hơn thực tế — chi phí âm thầm ăn vào biên lợi nhuận.
	CanShip bool
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

func (errNotFound) Error() string { return "product: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "product: định danh không hợp lệ" }

// ---------------------------------------------------------------- Tiền tố ID

// Các tiền tố ID mà module khác cần khi kiểm tra tham số đầu vào.
const (
	ProductIDPrefix = string(ids.PrefixProduct)
	VariantIDPrefix = string(ids.PrefixVariant)
	SKUIDPrefix     = string(ids.PrefixSKU)
)
