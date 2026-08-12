package application

import (
	"errors"
	"fmt"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	// ErrBrandNotFound khi thương hiệu không tồn tại hoặc không hoạt động.
	ErrBrandNotFound = errors.New("product: thương hiệu không tồn tại")

	// ErrSellerRequired khi thiếu định danh seller ở nơi bắt buộc phải có.
	//
	// Tồn tại để một lỗi lập trình ("quên truyền sellerID") thành lỗi rõ
	// ràng thay vì âm thầm trả về dữ liệu của TẤT CẢ seller.
	ErrSellerRequired = errors.New("product: bắt buộc phải có định danh seller")
)

// NotAuthorizedError khi seller không được phép bán thương hiệu.
//
// Là kiểu riêng chứ không phải errors.New vì nó mang theo LÝ DO — tầng
// giao diện cần lý do để hiển thị hành động cụ thể ("Tải lên giấy ủy quyền")
// thay vì thông báo chung chung.
type NotAuthorizedError struct {
	BrandID  ids.ID
	SellerID ids.ID
	Reason   string
}

func (e *NotAuthorizedError) Error() string {
	return fmt.Sprintf("product: seller %s không được bán thương hiệu %s: %s",
		e.SellerID, e.BrandID, e.Reason)
}

// ErrNotAuthorized là mẫu để so sánh bằng errors.Is.
var ErrNotAuthorized = &NotAuthorizedError{}

// Is cho phép errors.Is(err, ErrNotAuthorized) bắt mọi lỗi loại này bất kể
// thương hiệu hay seller nào.
func (e *NotAuthorizedError) Is(target error) bool {
	var t *NotAuthorizedError
	return errors.As(target, &t)
}
