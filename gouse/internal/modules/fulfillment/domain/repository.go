package domain

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Repository là PORT cho kho lưu trữ đơn thực hiện.
//
// MỌI phương thức đọc dành cho seller đều CÓ THAM SỐ sellerID — không có
// phương thức nào trả về dữ liệu không lọc theo seller.
//
// Sự thiếu vắng một hàm "lấy tất cả đơn thực hiện" là điều quan trọng nhất
// của interface này: nếu có hàm đó, sớm muộn sẽ có người gọi nó rồi lọc ở
// tầng trên, và QUÊN MỘT LẦN là rò rỉ dữ liệu đối thủ.
type Repository interface {
	// SaveBatch ghi các đơn thực hiện vừa tách trong MỘT giao dịch.
	//
	// Tách đơn là thao tác toàn phần: một đơn có ba nguồn hàng mà chỉ ghi
	// được hai nghĩa là một seller không bao giờ biết mình có việc.
	SaveBatch(ctx context.Context, fos []*FulfillmentOrder) error

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

	// ListDeliveredBefore lấy các đơn đã giao trước một mốc thời gian.
	//
	// Đầu vào của tiến trình chuyển DELIVERED → COMPLETED. Mỗi đơn trong
	// danh sách này là tiền của seller đang bị giữ lại chờ hết hạn đổi trả.
	ListDeliveredBefore(ctx context.Context, before time.Time, limit int) ([]*FulfillmentOrder, error)

	// ExistsForOrder cho biết đơn hàng này đã được tách chưa.
	//
	// Dùng cho idempotency: event `checkout.completed` có thể được phát
	// lại, và tách đơn hai lần sẽ tạo hai bộ đơn thực hiện cho cùng một
	// đơn hàng — seller thấy việc trùng và có thể giao hàng hai lần.
	ExistsForOrder(ctx context.Context, orderID ids.ID) (bool, error)
}
