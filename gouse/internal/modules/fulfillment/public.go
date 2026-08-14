// Package fulfillment là GÓC NHÌN VẬN HÀNH của đơn hàng.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// HAI KHÁI NIỆM, HAI MODULE (ADR-0007):
//
//	order        = HỢP ĐỒNG với khách   — "khách mua gì, giá bao nhiêu"
//	fulfillment  = ĐƠN VỊ CÔNG VIỆC     — "ai giao, đến đâu rồi"
//
// # Ranh giới bảo mật nằm trong CẤU TRÚC DỮ LIỆU
//
// Seller làm việc với module này, KHÔNG với module order — Order chứa dòng
// hàng của mọi seller trong đơn. Ở đây, mọi truy vấn lọc theo `seller_id`
// ngay trong SQL, nên một lần quên lọc ở tầng trên vẫn không rò rỉ được dữ
// liệu đối thủ.
//
// Đó là lý do MỌI hàm dành cho seller trong API này đều nhận `sellerID`
// làm tham số bắt buộc.
//
// # Không hỏi ngược module order
//
// Module này trỏ tới order qua `order_id`. Chiều ngược lại đi bằng EVENT:
// order lắng nghe `fulfillment.progress_changed` để tính trạng thái tổng
// hợp. Hỏi ngược sẽ tạo phụ thuộc vòng.
package fulfillment

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module fulfillment.
type API interface {
	// ---- Khách hàng và quản trị ----

	// GetOrderFulfillments trả mọi gói hàng của một đơn.
	//
	// Dành cho KHÁCH và quản trị viên: khách theo dõi được cả ba gói của
	// mình. KHÔNG được gọi từ API của seller.
	GetOrderFulfillments(ctx context.Context, orderID string) ([]FulfillmentView, error)

	// ---- Nhà bán ----
	//
	// MỌI hàm dưới đây nhận sellerID. Đó là chủ ý: không có cách nào gọi
	// chúng mà không nói mình là seller nào, nên không có cách nào vô tình
	// đọc dữ liệu của seller khác.

	ListSellerFulfillments(
		ctx context.Context, sellerID string, statuses []string, limit, offset int,
	) ([]FulfillmentView, error)

	GetSellerFulfillment(ctx context.Context, sellerID, fulfillmentID string) (*FulfillmentView, error)

	// AllocateInventory chọn kho xuất hàng.
	AllocateInventory(ctx context.Context, sellerID, fulfillmentID, locationID string) error

	ConfirmFulfillment(ctx context.Context, sellerID, fulfillmentID string) error
	MarkPicking(ctx context.Context, sellerID, fulfillmentID string) error
	MarkPacked(ctx context.Context, sellerID, fulfillmentID string) error

	// HandOverToCarrier bàn giao cho đơn vị vận chuyển.
	//
	// Mã vận đơn BẮT BUỘC: từ đây hàng ra khỏi tầm kiểm soát của seller, và
	// không có mã thì không ai trả lời được "hàng của tôi đang ở đâu".
	HandOverToCarrier(ctx context.Context, req HandOverRequest) error

	MarkInTransit(ctx context.Context, sellerID, fulfillmentID string) error
	MarkDeliveryFailed(ctx context.Context, sellerID, fulfillmentID, reason string) error
	MarkDelivered(ctx context.Context, sellerID, fulfillmentID string) error

	// CancelFulfillment hủy phần của một seller, ví dụ vì hết hàng.
	//
	// Lý do BẮT BUỘC: khách cần lời giải thích khi nhận thông báo.
	CancelFulfillment(ctx context.Context, sellerID, fulfillmentID, reason string) error

	// ---- Tiến trình nền ----

	// CompleteDelivered chuyển các đơn đã giao quá hạn đổi trả sang COMPLETED.
	//
	// ĐÂY LÀ RANH GIỚI TÀI CHÍNH:
	//
	//	DELIVERED  → số dư seller vẫn Pending
	//	COMPLETED  → số dư chuyển Available, seller được chi trả
	//
	// Chạy sớm nghĩa là trả tiền cho seller trước khi biết khách có hoàn
	// hàng không — và tiền đã chi thì đòi lại rất khó.
	CompleteDelivered(ctx context.Context, limit int) (int, error)
}

// ---------------------------------------------------------------- DTO

// Amount là số tiền kèm đơn vị.
type Amount struct {
	Value    int64
	Currency string
}

// HandOverRequest là dữ liệu bàn giao vận chuyển.
type HandOverRequest struct {
	SellerID      string
	FulfillmentID string

	// Provider là tên đơn vị vận chuyển — DỮ LIỆU, không phải mã nguồn.
	//
	// Giá và chất lượng của các đối tác thay đổi thường xuyên, và nền tảng
	// cần đổi hoặc dùng đồng thời nhiều đối tác (nguyên tắc P13).
	Provider string

	// TrackingNumber là BẮT BUỘC.
	TrackingNumber string
}

// FulfillmentView là đơn vị công việc của MỘT nguồn hàng.
//
// Đây là thứ seller được xem. KHÔNG có trường nào tiết lộ seller khác
// trong cùng đơn, kể cả gián tiếp: FONumber có hậu tố -A/-B/-C nhưng
// seller không suy ra được có bao nhiêu seller khác từ chữ cái của mình.
type FulfillmentView struct {
	ID       string
	OrderID  string
	FONumber string
	SellerID string

	Status  string
	Type    string
	LineIDs []string

	Subtotal         Amount
	CommissionAmount Amount

	// SellerPayable = tiền hàng − hoa hồng, tính từ số ĐÃ ĐÓNG BĂNG.
	SellerPayable Amount

	StockLocationID   string
	ShippingMethod    string
	ShippingProvider  string
	TrackingNumber    string
	EstimatedDelivery string

	CancelReason  string
	FailureReason string

	ConfirmedAt string
	PackedAt    string
	ShippedAt   string
	DeliveredAt string
	CompletedAt string
	CancelledAt string
	CreatedAt   string
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound      = errNotFound{}
	ErrInvalidID     = errInvalidID{}
	ErrInvalidInput  = errInvalidInput{}
	ErrInvalidStatus = errInvalidStatus{}

	// ErrForbidden khi seller thao tác trên đơn không phải của mình.
	ErrForbidden = errForbidden{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "fulfillment: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "fulfillment: định danh không hợp lệ" }

type errInvalidInput struct{}

func (errInvalidInput) Error() string { return "fulfillment: dữ liệu không hợp lệ" }

type errInvalidStatus struct{}

func (errInvalidStatus) Error() string {
	return "fulfillment: chuyển trạng thái không hợp lệ"
}

type errForbidden struct{}

func (errForbidden) Error() string {
	return "fulfillment: đơn thực hiện không thuộc về nhà bán này"
}

// ---------------------------------------------------------------- Tiền tố

const FulfillmentIDPrefix = string(ids.PrefixFulfillmentOrder)

// Trạng thái đơn vị công việc vận hành.
const (
	StatusPending        = "PENDING"
	StatusAllocated      = "ALLOCATED"
	StatusConfirmed      = "CONFIRMED"
	StatusPicking        = "PICKING"
	StatusPacked         = "PACKED"
	StatusHandedOver     = "HANDED_OVER"
	StatusInTransit      = "IN_TRANSIT"
	StatusDeliveryFailed = "DELIVERY_FAILED"
	StatusDelivered      = "DELIVERED"

	// StatusCompleted có Ý NGHĨA TÀI CHÍNH: số dư seller chuyển Available.
	StatusCompleted = "COMPLETED"

	StatusCancelled = "CANCELLED"
)

// Ba mô hình thực hiện đơn.
const (
	TypePlatform        = "PLATFORM"
	TypeSeller          = "SELLER"
	TypePlatformService = "PLATFORM_SERVICE"
)
