package domain

import (
	"context"
	"time"

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

	// FindBySourceCheckout tìm đơn sinh ra từ một phiên thanh toán.
	//
	// Trả ErrNotFound khi phiên chưa sinh đơn nào, và với id rỗng.
	FindBySourceCheckout(ctx context.Context, checkoutID ids.ID) (*Order, error)

	// ListByCustomer trả lịch sử đơn của một khách.
	//
	// `status` rỗng nghĩa là MỌI trạng thái.
	//
	// Lọc nằm TRONG truy vấn, không phải sau khi đọc. Lọc sau khi đọc làm
	// bản ghi bị loại vẫn tính vào trang: một trang trả ít hơn `limit`, và
	// khi cả trang bị loại thì `data` rỗng trong lúc `has_more` vẫn true —
	// client thường coi trang rỗng là hết dữ liệu và dừng phân trang.
	//
	// Đọc tiếp theo KHÓA (`moc`) chứ không theo số thứ tự bỏ qua. `moc`
	// nil nghĩa là đọc từ đầu. Xem MocPhanTrang để biết vì sao.
	ListByCustomer(
		ctx context.Context, customerID ids.ID, status Status,
		limit int, moc *MocPhanTrang,
	) ([]*Order, error)

	// List trả đơn theo bộ lọc, cho giao diện quản trị.
	//
	// KHÁC ListByCustomer ở chỗ nó KHÔNG giới hạn theo khách: nhân viên hỗ
	// trợ cần tra đơn của bất kỳ ai. Vì thế endpoint gọi nó phải chặn theo
	// vai trò ở tầng route — không có giới hạn tự nhiên nào ở đây.
	List(ctx context.Context, f Filter) ([]*Order, error)

	// UpdateWithAudit ghi thay đổi VÀ chạy fn trong CÙNG một giao dịch.
	//
	// Hủy đơn là thao tác nhạy cảm: nó dừng việc giao hàng khách đã trả
	// tiền. Trạng thái đổi mà không có vết kiểm toán nghĩa là không ai trả
	// lời được vì sao đơn của khách bị hủy.
	UpdateWithAudit(ctx context.Context, o *Order, fn TxFunc) error
}

// TxFunc chạy bên trong giao dịch mà kho lưu trữ đã mở.
type TxFunc func(ctx context.Context) error

// Filter là điều kiện lọc danh sách đơn ở giao diện quản trị.
type Filter struct {
	// OrderNumber tra chính xác một đơn. Khách đọc mã này qua điện thoại
	// khi khiếu nại, nên đây là đường tra cứu chính của nhân viên hỗ trợ.
	OrderNumber string

	Status     string
	CustomerID ids.ID

	// From, To lọc theo thời điểm đặt. Giá trị zero = không giới hạn.
	From time.Time
	To   time.Time

	Limit  int
	Offset int
}

// NumberGenerator sinh mã đơn hiển thị cho khách: FC-2026-08-000001.
//
// Là PORT vì mã phải liên tục và không trùng trong toàn hệ thống — việc đó
// cần một bộ đếm dùng chung, thứ chỉ tầng hạ tầng cung cấp được.
type NumberGenerator interface {
	NextOrderNumber(ctx context.Context) (string, error)
}
