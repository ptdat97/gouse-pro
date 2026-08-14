package domain

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Repository là PORT cho kho lưu trữ đơn hàng.
//
// Save ghi Order, các Line và các Adjustment trong MỘT giao dịch: một đơn
// có dòng hàng ghi dở là đơn tính sai tiền.
//
// Đơn thực hiện KHÔNG nằm ở đây — chúng thuộc module fulfillment, được tạo
// khi module đó nghe event `checkout.completed`.
type Repository interface {
	// Save lưu đơn hàng mới.
	//
	// KHÔNG ghi đơn thực hiện: chúng thuộc module fulfillment, và module
	// này chỉ lắng nghe event để tính trạng thái tổng hợp (ADR-0007).
	//
	// Trả ErrDuplicateOrder nếu khóa idempotency đã dùng. Bên gọi nên coi
	// đó là THÀNH CÔNG và đọc lại đơn cũ (quy tắc 5) — hai đơn cho cùng
	// một lần bấm nút nghĩa là khách bị trừ tiền hai lần.
	Save(ctx context.Context, o *Order) error

	// Update ghi lại thay đổi trạng thái của đơn và các dòng hàng.
	Update(ctx context.Context, o *Order) error

	FindByID(ctx context.Context, id ids.ID) (*Order, error)

	FindByOrderNumber(ctx context.Context, number string) (*Order, error)

	// FindByIdempotencyKey tra đơn đã tạo theo khóa.
	//
	// Đây là cách PlaceOrder trả kết quả cũ khi bên gọi thử lại — nghĩa
	// đúng của idempotent: gọi nhiều lần cho cùng một kết quả.
	FindByIdempotencyKey(ctx context.Context, key string) (*Order, error)

	// ListByCustomer trả lịch sử đơn của một khách.
	ListByCustomer(ctx context.Context, customerID ids.ID, limit, offset int) ([]*Order, error)
}

// NumberGenerator sinh mã đơn hiển thị cho khách: FC-2026-08-000001.
//
// Là PORT vì mã phải liên tục và không trùng trong toàn hệ thống — việc đó
// cần một bộ đếm dùng chung, thứ chỉ tầng hạ tầng cung cấp được.
type NumberGenerator interface {
	NextOrderNumber(ctx context.Context) (string, error)
}
