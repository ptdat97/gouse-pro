package domain

import (
	"context"
	"errors"
	"strings"

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

// SizeDaChuan và NhomMauDaChuan trả danh sách lọc ĐÃ CHUẨN HÓA.
//
// # Vì sao nằm ở MIỀN chứ không ở từng kho
//
// So sánh không phân biệt hoa thường là một QUY TẮC của bộ lọc, không
// phải chi tiết của PostgreSQL. Mỗi kho tự chuẩn hóa nghĩa là hai kho có
// thể chuẩn hóa khác nhau — và bản in-memory chạy trong test sẽ xanh cho
// hành vi mà production không có.
//
// Đó không phải lo xa: tới 24/09/2026 bản in-memory BỎ QUA hẳn hai bộ lọc
// này, nên mọi lượt chạy ở môi trường phát triển (`MODULES_STORAGE` mặc
// định là `memory`) đều trả về toàn bộ danh mục bất kể lọc màu nào.
//
// Size về CHỮ THƯỜNG: người bán gõ "m", "M", "Free size" tùy ý.
// Nhóm màu về CHỮ HOA: nhóm là hằng số hệ thống, không phải chuỗi người
// dùng nhập — xem NhomMau.
func (f Filter) SizeDaChuan() []string { return chuanHoaLoc(f.Sizes, strings.ToLower) }

func (f Filter) NhomMauDaChuan() []string {
	return chuanHoaLoc(f.ColorFamilies, strings.ToUpper)
}

// KhopBienThe trả true nếu bộ lọc biến thể KHÔNG loại sản phẩm này.
//
// Tương đương hai mệnh đề EXISTS của bản PostgreSQL: mỗi bộ lọc cần ÍT
// NHẤT MỘT biến thể chưa lưu trữ khớp; danh sách rỗng thì không lọc.
func (f Filter) KhopBienThe(bienThe []*Variant) bool {
	return khopThuocTinh(bienThe, AttrSize, f.SizeDaChuan(), strings.ToLower) &&
		khopThuocTinh(bienThe, AttrColorFamily, f.NhomMauDaChuan(), strings.ToUpper)
}

func khopThuocTinh(
	bienThe []*Variant, khoa string, can []string, chuan func(string) string,
) bool {
	if len(can) == 0 {
		return true
	}
	for _, v := range bienThe {
		if v == nil || v.Status() == StatusArchived {
			continue
		}
		giaTri := chuan(v.Attributes()[khoa])
		for _, x := range can {
			if giaTri == x {
				return true
			}
		}
	}
	return false
}

// chuanHoaLoc trả NIL khi không có gì để lọc, không phải mảng rỗng.
//
// Điều kiện SQL bên PostgreSQL dùng `$n::text[] IS NULL` để bỏ qua bộ lọc.
// Một mảng rỗng KHÔNG phải "không lọc" — nó là "không khớp gì cả", và khi
// ấy danh mục trả về trắng trơn.
func chuanHoaLoc(v []string, f func(string) string) []string {
	if len(v) == 0 {
		return nil
	}
	out := make([]string, 0, len(v))
	for _, x := range v {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, f(x))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
