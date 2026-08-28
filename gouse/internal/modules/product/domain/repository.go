package domain

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// ErrNotFound là lỗi chung khi không tìm thấy bản ghi.
//
// Định nghĩa ở tầng domain, không ở infrastructure: tầng application phải
// xử lý được "không tìm thấy" mà không cần biết dữ liệu đến từ PostgreSQL,
// bộ nhớ hay dịch vụ ngoài.
var ErrNotFound = errors.New("product: không tìm thấy")

// ErrSKUCodeTaken khi mã SKU đã thuộc về một sản phẩm khác.
//
// Quy tắc 1 (docs/04-modules/product.md mục 12): sku_code duy nhất TOÀN
// HỆ THỐNG. Trùng mã nghĩa là hai mặt hàng khác nhau dùng chung một định
// danh kho — tồn kho và đơn hàng sẽ trỏ nhầm chỗ.
var ErrSKUCodeTaken = errors.New("product: mã SKU đã được dùng")

// ErrSlugTaken khi slug đã thuộc về một sản phẩm khác.
//
// Slug nằm trên URL công khai; trùng slug nghĩa là hai sản phẩm tranh nhau
// một địa chỉ.
var ErrSlugTaken = errors.New("product: slug đã được dùng")

// ProductRepository là PORT — tầng domain định nghĩa cái nó cần, tầng
// infrastructure cài đặt.
//
// Hướng phụ thuộc này là điểm mấu chốt: domain không phụ thuộc database,
// database phụ thuộc domain. Nhờ vậy đổi PostgreSQL sang thứ khác không
// phải sửa một dòng nào trong domain.
type ProductRepository interface {
	Save(ctx context.Context, p *Product) error

	FindByID(ctx context.Context, id ids.ID) (*Product, error)
	FindBySlug(ctx context.Context, slug string) (*Product, error)

	// FindByIDs nhận DANH SÁCH, không nhận một id.
	//
	// Thiết kế này bắt buộc bên gọi nghĩ theo lô và tránh vấn đề N+1:
	// hiển thị 50 sản phẩm phải là 1 truy vấn, không phải 50.
	FindByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*Product, error)

	// FindByCollection lấy sản phẩm thuộc một bộ sưu tập.
	FindByCollection(ctx context.Context, collectionID ids.ID) ([]*Product, error)

	// List lọc sản phẩm theo điều kiện.
	List(ctx context.Context, f Filter) ([]*Product, error)

	// FindBySKUCode tra ngược từ mã SKU về sản phẩm chứa nó.
	//
	// Cần cho quét mã vạch ở kho và cho việc kiểm tra tính duy nhất
	// toàn hệ thống của mã SKU (quy tắc 1).
	FindBySKUCode(ctx context.Context, code string) (*Product, error)

	// FindBySKUIDs tra ngược từ danh sách SKU về sản phẩm.
	//
	// Module cart và order giữ sku_id; khi hiển thị đơn hàng chúng cần tên
	// và ảnh sản phẩm. Không có hàm này, chúng sẽ phải gọi lặp từng cái.
	FindBySKUIDs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID]*Product, error)
}

// Filter là điều kiện lọc sản phẩm.
//
// Trường rỗng nghĩa là không lọc theo tiêu chí đó.
type Filter struct {
	BrandID      ids.ID
	CategoryID   ids.ID
	CollectionID ids.ID
	ProductType  ProductType
	Gender       GenderTarget
	Status       Status

	// SellerID lọc theo người tạo.
	//
	// QUAN TRỌNG cho bảo mật: seller chỉ được thấy sản phẩm của MÌNH.
	// Lọc phải nằm trong TRUY VẤN, không phải ở tầng hiển thị — lọc ở tầng
	// hiển thị nghĩa là dữ liệu seller khác đã rời khỏi database và chỉ
	// cần một lỗi nhỏ là rò rỉ.
	SellerID ids.ID

	// OnlyVisible chỉ lấy sản phẩm khách xem được (ACTIVE).
	OnlyVisible bool

	// Query là từ khóa tìm kiếm, khớp theo tên sản phẩm.
	//
	// Tìm kiếm MVP là SQL cơ bản (mvp.md mục 4): so khớp chuỗi con, không
	// dấu, không xếp hạng liên quan. Chỉ mục tìm kiếm riêng là hạ tầng
	// thêm KHI ĐO ĐƯỢC nhu cầu — xem future-phases.md.
	Query string

	// Sizes lọc theo size của BIẾN THỂ. Khớp bất kỳ giá trị nào trong danh sách.
	//
	// Bộ lọc quan trọng nhất của một sàn thời trang: khách chỉ mặc vừa một
	// hai size, và danh mục không lọc được size là danh mục họ phải mở
	// từng sản phẩm mới biết có mua được không.
	Sizes []string

	// ColorFamilies lọc theo NHÓM màu, không phải tên màu cụ thể.
	//
	// Xem domain.SuyRaNhomMau để biết vì sao lọc theo nhóm.
	ColorFamilies []string

	Limit  int
	Offset int
}
