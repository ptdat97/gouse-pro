package domain

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Repository là PORT cho kho lưu trữ đơn hàng.
//
// Save ghi CẢ Order, các Line, các Adjustment và các FulfillmentOrder trong
// MỘT giao dịch. Đây không phải chi tiết cài đặt mà là yêu cầu nghiệp vụ:
// một đơn có dòng hàng nhưng thiếu đơn thực hiện là đơn không ai xử lý, và
// một đơn thực hiện trỏ tới đơn hàng không tồn tại là dữ liệu rác cho
// seller. Hai thứ phải cùng có hoặc cùng không.
type Repository interface {
	// Save lưu đơn hàng mới cùng các đơn thực hiện đã tách.
	//
	// Trả ErrDuplicateOrder nếu khóa idempotency đã dùng. Bên gọi nên coi
	// đó là THÀNH CÔNG và đọc lại đơn cũ (quy tắc 5) — hai đơn cho cùng
	// một lần bấm nút nghĩa là khách bị trừ tiền hai lần.
	Save(ctx context.Context, o *Order, fos []*FulfillmentOrder) error

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

// FulfillmentRepository là PORT cho đơn vị công việc vận hành.
//
// TÁCH KHỎI Repository CÓ CHỦ ĐÍCH. Đây là kho mà tầng seller dùng, và mọi
// phương thức đọc ở đây đều CÓ THAM SỐ sellerID — không có phương thức nào
// trả về dữ liệu không lọc theo seller.
//
// Sự thiếu vắng một hàm "lấy tất cả đơn thực hiện" là điều quan trọng nhất
// của interface này: nếu có hàm đó, sớm muộn sẽ có người gọi nó rồi lọc ở
// tầng trên, và QUÊN MỘT LẦN là rò rỉ dữ liệu đối thủ.
type FulfillmentRepository interface {
	Update(ctx context.Context, fo *FulfillmentOrder) error

	// FindByID trả đơn thực hiện KÈM kiểm tra chủ sở hữu.
	//
	// sellerID là THAM SỐ BẮT BUỘC, không phải tùy chọn: bộ lọc nằm ngay
	// trong câu SQL, nên một lần gọi sai vẫn không trả được dữ liệu của
	// seller khác.
	FindByID(ctx context.Context, id, sellerID ids.ID) (*FulfillmentOrder, error)

	// ListBySeller trả danh sách việc cần xử lý của một seller.
	//
	// statuses rỗng nghĩa là mọi trạng thái.
	ListBySeller(
		ctx context.Context, sellerID ids.ID, statuses []FOStatus, limit, offset int,
	) ([]*FulfillmentOrder, error)

	// ListByOrder trả mọi đơn thực hiện của một đơn hàng.
	//
	// CHỈ dành cho khách hàng và quản trị viên — khách theo dõi được cả ba
	// gói của mình. KHÔNG được lộ ra API của seller.
	ListByOrder(ctx context.Context, orderID ids.ID) ([]*FulfillmentOrder, error)
}

// NumberGenerator sinh mã đơn hiển thị cho khách: FC-2026-08-000001.
//
// Là PORT vì mã phải liên tục và không trùng trong toàn hệ thống — việc đó
// cần một bộ đếm dùng chung, thứ chỉ tầng hạ tầng cung cấp được.
type NumberGenerator interface {
	NextOrderNumber(ctx context.Context) (string, error)
}
