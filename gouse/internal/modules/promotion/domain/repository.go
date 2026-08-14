package domain

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// PromotionRepository là PORT cho kho lưu trữ khuyến mãi.
type PromotionRepository interface {
	Save(ctx context.Context, p *Promotion) error

	// Update ghi thay đổi bằng KHÓA LẠC QUAN.
	//
	// Trả ErrVersionConflict nếu bản ghi đã bị thao tác khác sửa. Với
	// khuyến mãi, xung đột là chuyện THƯỜNG XUYÊN chứ không hiếm: một mã
	// đang chạy quảng cáo có thể có hàng trăm người cùng áp trong một
	// giây, và mỗi lượt đều tăng bộ đếm.
	Update(ctx context.Context, p *Promotion) error

	FindByID(ctx context.Context, id ids.ID) (*Promotion, error)

	// ListActive trả các khuyến mãi đang chạy tại một thời điểm.
	ListActive(ctx context.Context, now time.Time) ([]*Promotion, error)

	// ExpireDue chuyển các khuyến mãi quá hạn sang EXPIRED.
	//
	// Chạy định kỳ bằng worker. Không có nó, khuyến mãi hết hạn vẫn mang
	// trạng thái ACTIVE và chỉ bị chặn bởi phép so sánh thời gian — một
	// lớp bảo vệ thay vì hai.
	//
	// Trả về số bản ghi đã đổi.
	ExpireDue(ctx context.Context, now time.Time) (int, error)
}

// CouponRepository là PORT cho mã giảm giá.
type CouponRepository interface {
	Save(ctx context.Context, c *Coupon) error

	// FindByCode tra mã theo chuỗi ĐÃ CHUẨN HÓA (chữ hoa).
	//
	// Bên gọi phải gọi NormalizeCode trước — nếu không, "sale10" sẽ không
	// tìm thấy mã đăng ký là "SALE10".
	FindByCode(ctx context.Context, code string) (*Coupon, error)

	FindByID(ctx context.Context, id ids.ID) (*Coupon, error)

	Update(ctx context.Context, c *Coupon) error
}

// UsageRepository là PORT cho lượt sử dụng.
type UsageRepository interface {
	// Record ghi nhận một lượt sử dụng.
	//
	// IDEMPOTENT Ở TẦNG DATABASE: ràng buộc UNIQUE (coupon_id, order_id)
	// chặn ghi trùng. Kiểm tra "đã ghi chưa" ở tầng ứng dụng KHÔNG cứu
	// được khi handler xử lý cùng một event hai lần — cả hai lần cùng đọc
	// thấy chưa có rồi cùng ghi.
	//
	// Khi đó ngân sách khuyến mãi bị trừ hai lần cho một đơn.
	//
	// Trả ErrAlreadyUsed nếu cặp (coupon, order) đã tồn tại.
	Record(ctx context.Context, u Usage) error

	// Release đánh dấu lượt sử dụng đã được giải phóng.
	//
	// KHÔNG xóa hàng. Trả về lượt vừa giải phóng để bên gọi biết cần trả
	// lại bao nhiêu ngân sách; nếu lượt đó đã giải phóng rồi, trả
	// ErrNotFound để không trừ ngân sách hai lần.
	Release(ctx context.Context, orderID ids.ID, now time.Time) ([]Usage, error)

	// CountByCustomer đếm số lượt CHƯA giải phóng của một khách với một mã.
	//
	// Dùng cho giới hạn maxUsesPerCustomer. Đếm cả lượt đã giải phóng sẽ
	// chặn nhầm khách có đơn bị hủy vì lý do của nền tảng.
	CountByCustomer(ctx context.Context, couponID, customerID ids.ID) (int, error)

	// ListByOrder trả các lượt sử dụng của một đơn.
	ListByOrder(ctx context.Context, orderID ids.ID) ([]Usage, error)
}

// DiscountLine là phần giảm giá phân bổ cho MỘT dòng hàng.
//
// Phải được ĐÓNG BĂNG vào đơn hàng: khi khách trả lại một món, số tiền
// hoàn là giá dòng TRỪ phần giảm đã phân bổ cho nó. Không lưu lại thì
// nền tảng hoàn nhiều hơn đã thu.
type DiscountLine struct {
	// LineID là định danh dòng hàng do bên gọi cung cấp.
	//
	// KHÔNG phải kiểu của module order: promotion không biết bảng
	// order_line tồn tại, nó chỉ trả về đúng chuỗi đã nhận vào.
	LineID string

	Discount money.Money
}

// CostAllocation là phần chi phí khuyến mãi mà một bên phải chịu.
//
// Đây là câu trả lời cho "ai chịu 50.000đ này". Thiếu nó, không tính được
// seller thực nhận bao nhiêu và đối soát cuối tháng sẽ lệch.
type CostAllocation struct {
	// Bearer: PLATFORM hoặc SELLER.
	Bearer CostBearer

	// SellerID chỉ có nghĩa khi Bearer là SELLER.
	SellerID ids.ID

	Amount money.Money
}
