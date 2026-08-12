package domain

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// ErrNotFound là lỗi chung khi không tìm thấy bản ghi.
//
// Định nghĩa ở tầng domain, không ở infrastructure: tầng application phải
// xử lý được "không tìm thấy" mà không cần biết dữ liệu đến từ đâu.
var ErrNotFound = errors.New("pricing: không tìm thấy")

// PriceRepository là PORT cho bảng giá.
type PriceRepository interface {
	Save(ctx context.Context, p *Price) error

	FindByID(ctx context.Context, id ids.ID) (*Price, error)

	// FindBySKU lấy MỌI mức giá của một SKU, kể cả đã ngừng áp dụng.
	//
	// Việc chọn mức giá nào áp dụng là quyết định NGHIỆP VỤ (SelectBest),
	// không phải quyết định của kho lưu trữ. Nếu kho tự lọc, quy tắc ưu
	// tiên sẽ nằm rải rác ở cả hai nơi.
	FindBySKU(ctx context.Context, skuID ids.ID) ([]*Price, error)

	// FindBySKUs nhận DANH SÁCH để tránh N+1.
	//
	// Hiển thị 50 sản phẩm cần 1 truy vấn giá, không phải 50.
	FindBySKUs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID][]*Price, error)
}

// ConstraintRepository là PORT cho khung giá ràng buộc seller.
type ConstraintRepository interface {
	Save(ctx context.Context, c *PriceConstraint) error

	// FindBySKU trả ErrNotFound nếu SKU chưa có khung giá.
	FindBySKU(ctx context.Context, skuID ids.ID) (*PriceConstraint, error)

	FindBySKUs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID]*PriceConstraint, error)
}

// HistoryRepository là PORT cho lịch sử giá.
//
// CHỈ CÓ GHI THÊM VÀ ĐỌC — không có Update, không có Delete.
//
// Thiếu vắng hai phương thức đó là CÓ CHỦ ĐÍCH: lịch sử sửa được thì không
// còn giá trị làm bằng chứng cho việc phát hiện thao túng giá hay cho nghĩa
// vụ minh bạch giá. Khi có PostgreSQL, ràng buộc này được siết thêm bằng
// RULE ở tầng database (cùng cách làm với sổ cái — xem ADR-0008).
type HistoryRepository interface {
	Append(ctx context.Context, p *PricePoint) error

	// FindBySKU lấy lịch sử giá trong một khoảng thời gian.
	FindBySKU(ctx context.Context, skuID ids.ID, r DateRange) ([]*PricePoint, error)
}
