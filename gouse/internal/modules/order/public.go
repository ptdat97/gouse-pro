// Package order quản lý HỢP ĐỒNG với khách và ĐƠN VỊ CÔNG VIỆC vận hành.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// HAI KHÁI NIỆM, KHÔNG PHẢI MỘT (ADR-0007):
//
//	Order            = hợp đồng với khách hàng, gần như bất biến sau khi đặt
//	FulfillmentOrder = đơn vị công việc của MỘT nguồn hàng
//
// Điểm quan trọng nhất khi dùng API này: HAI NHÓM HÀM RIÊNG BIỆT cho hai
// đối tượng khác nhau. Nhóm khách hàng trả về Order kèm mọi dòng hàng.
// Nhóm seller trả về FulfillmentOrder và LUÔN nhận sellerID làm tham số —
// không có hàm nào cho seller đọc được Order.
//
// Đó không phải bất tiện mà là RANH GIỚI BẢO MẬT: Order chứa hàng của mọi
// seller trong đơn. Nếu seller đọc được Order thì phải lọc ở tầng hiển thị,
// và quên một lần là rò rỉ dữ liệu đối thủ.
package order

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module order.
type API interface {
	// ---- Khách hàng ----

	// PlaceOrder tạo đơn từ giỏ đã chốt, đóng băng mọi con số và tách
	// thành các đơn vị công việc theo nguồn hàng.
	//
	// IDEMPOTENT theo IdempotencyKey: gọi lại KHÔNG tạo đơn thứ hai, mà
	// trả lại đơn cũ với Replayed = true. Bên gọi phải kiểm tra cờ đó
	// trước khi gửi email xác nhận hay trừ tiền.
	PlaceOrder(ctx context.Context, req PlaceOrderRequest) (*PlaceOrderResult, error)

	GetOrder(ctx context.Context, orderID string) (*OrderView, error)
	GetOrderByNumber(ctx context.Context, number string) (*OrderView, error)

	ListCustomerOrders(ctx context.Context, customerID string, limit, offset int) ([]OrderView, error)

	// GetOrderFulfillments trả mọi gói hàng của một đơn.
	//
	// Dành cho KHÁCH và quản trị viên: khách theo dõi được cả ba gói của
	// mình. KHÔNG được gọi từ API của seller.
	GetOrderFulfillments(ctx context.Context, orderID string) ([]FulfillmentView, error)

	// MarkOrderPaid ghi nhận đơn đã thanh toán.
	MarkOrderPaid(ctx context.Context, orderID string) error

	// CancelOrder hủy toàn bộ đơn theo yêu cầu của khách.
	//
	// Bị chặn nếu đã có gói nào đóng gói xong: từ đó việc hủy có chi phí
	// thật và cần quy trình riêng.
	CancelOrder(ctx context.Context, orderID, reason string) error

	// ---- Nhà bán ----
	//
	// MỌI hàm dưới đây nhận sellerID. Đó là chủ ý: không có cách nào gọi
	// chúng mà không nói mình là seller nào, nên không có cách nào vô tình
	// đọc dữ liệu của seller khác.

	ListSellerFulfillments(
		ctx context.Context, sellerID string, statuses []string, limit, offset int,
	) ([]FulfillmentView, error)

	GetSellerFulfillment(ctx context.Context, sellerID, fulfillmentID string) (*FulfillmentView, error)

	ConfirmFulfillment(ctx context.Context, sellerID, fulfillmentID string) error
	PackFulfillment(ctx context.Context, sellerID, fulfillmentID string) error
	ShipFulfillment(ctx context.Context, sellerID, fulfillmentID string) error
	DeliverFulfillment(ctx context.Context, sellerID, fulfillmentID string) error

	// CancelFulfillment hủy phần của một seller, ví dụ vì hết hàng.
	//
	// Lý do là BẮT BUỘC: khách cần lời giải thích khi nhận thông báo.
	//
	// Hủy cả các dòng hàng tương ứng trong hợp đồng — chỉ hủy một bên thì
	// khách vẫn bị tính tiền món không bao giờ được giao.
	CancelFulfillment(ctx context.Context, sellerID, fulfillmentID, reason string) error
}

// ---------------------------------------------------------------- DTO

// Amount là số tiền kèm đơn vị. Value là số nguyên theo đơn vị nhỏ nhất.
type Amount struct {
	Value    int64
	Currency string
}

// AddressInput là địa chỉ giao hàng — sẽ được ĐÓNG BĂNG vào đơn.
type AddressInput struct {
	RecipientName string
	Phone         string
	StreetAddress string
	Ward          string
	District      string
	Province      string
	CountryCode   string
}

// PlaceOrderLineInput là một dòng hàng khách muốn mua.
//
// GIÁ VÀ TỶ LỆ HOA HỒNG do bên gọi truyền xuống, module này KHÔNG đi tra:
// chúng phải là con số khách đã NHÌN THẤY và đồng ý ở màn hình thanh toán,
// không phải con số tại thời điểm ghi database.
type PlaceOrderLineInput struct {
	OfferID  string
	SKUID    string
	SellerID string

	// ProductName và VariantDescription được ĐÓNG BĂNG: seller đổi tên sản
	// phẩm sau này thì hóa đơn cũ vẫn phải đúng.
	ProductName        string
	VariantDescription string

	UnitPrice Amount
	Quantity  int

	// CommissionRate theo ĐIỂM CƠ BẢN (1000 = 10%). Không dùng số thực:
	// sai số dấu phẩy động tích lũy thành lệch đối soát.
	CommissionRate int

	AttributedCreatorID   string
	CreatorCommissionRate int

	Adjustments []AdjustmentInput
}

// AdjustmentInput là một khoản cộng/trừ ĐÃ PHÂN BỔ về dòng hàng này.
type AdjustmentInput struct {
	Type string

	// Label là thứ KHÁCH NHÌN THẤY: "Giảm giá THUDONG20". Bắt buộc.
	Label string

	// Amount: ÂM là giảm, DƯƠNG là tăng.
	Amount Amount

	SourceType string
	SourceID   string

	// CostBearer: PLATFORM, SELLER hoặc SHARED. Không có trường này thì
	// không đối soát được "seller chịu bao nhiêu giảm giá trong kỳ".
	CostBearer string
}

// PlaceOrderRequest là dữ liệu đặt một đơn hàng.
type PlaceOrderRequest struct {
	// CustomerID rỗng nghĩa là KHÁCH VÃNG LAI — được phép, nhưng khi đó
	// GuestEmail là bắt buộc.
	CustomerID string
	GuestEmail string
	GuestPhone string

	ShippingAddress AddressInput
	BillingAddress  AddressInput

	Currency       string
	ShippingFee    Amount
	DiscountAmount Amount
	TaxAmount      Amount

	Lines []PlaceOrderLineInput

	// IdempotencyKey là BẮT BUỘC. Không có nó thì hai lần bấm nút thành
	// hai đơn, và khách bị trừ tiền hai lần.
	IdempotencyKey string
}

// PlaceOrderResult là kết quả đặt hàng.
type PlaceOrderResult struct {
	Order        OrderView
	Fulfillments []FulfillmentView

	// Replayed = true nghĩa là đơn ĐÃ tồn tại từ một lần gọi trước.
	//
	// Bên gọi PHẢI kiểm tra cờ này trước khi gửi email hay trừ tiền: không
	// tạo đơn mới là chưa đủ, tác dụng phụ cũng không được lặp.
	Replayed bool
}

// OrderView là hợp đồng với khách — dành cho KHÁCH và quản trị viên.
//
// KHÔNG trả về cho seller: nó chứa dòng hàng của mọi seller trong đơn.
type OrderView struct {
	ID          string
	OrderNumber string

	CustomerID string
	GuestEmail string
	GuestPhone string

	ShippingAddress AddressInput
	Status          string

	Lines []OrderLineView

	Subtotal       Amount
	ShippingFee    Amount
	DiscountAmount Amount
	TaxAmount      Amount

	// Total tính từ các dòng CÒN HIỆU LỰC, nên hủy từng phần tự động làm
	// giảm tổng.
	Total Amount

	PlacedAt    string
	CompletedAt string
}

// OrderLineView là một dòng hàng với dữ liệu ĐÃ ĐÓNG BĂNG.
type OrderLineView struct {
	ID       string
	OfferID  string
	SKUID    string
	SellerID string

	ProductName        string
	VariantDescription string

	UnitPrice Amount
	Quantity  int
	LineTotal Amount

	CommissionRate   int
	CommissionAmount Amount

	// SellerPayable = tiền hàng − hoa hồng, tính từ số ĐÃ ĐÓNG BĂNG.
	SellerPayable Amount

	Status      string
	Adjustments []AdjustmentView
}

// AdjustmentView là một khoản cộng/trừ trên dòng hàng.
type AdjustmentView struct {
	ID         string
	Type       string
	Label      string
	Amount     Amount
	CostBearer string
}

// FulfillmentView là đơn vị công việc của MỘT nguồn hàng.
//
// Đây là thứ seller được xem. KHÔNG có trường nào tiết lộ seller khác
// trong cùng đơn, kể cả gián tiếp: FONumber có hậu tố -A/-B/-C nhưng seller
// không suy ra được có bao nhiêu seller khác từ chữ cái của riêng mình.
type FulfillmentView struct {
	ID       string
	OrderID  string
	FONumber string
	SellerID string

	Status  string
	LineIDs []string

	Subtotal         Amount
	CommissionAmount Amount
	SellerPayable    Amount

	CancelReason string

	ConfirmedAt string
	PackedAt    string
	ShippedAt   string
	DeliveredAt string
	CancelledAt string
	CreatedAt   string
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound       = errNotFound{}
	ErrInvalidID      = errInvalidID{}
	ErrInvalidInput   = errInvalidInput{}
	ErrInvalidStatus  = errInvalidStatus{}
	ErrNotCancellable = errNotCancellable{}

	// ErrForbidden khi seller thao tác trên đơn không phải của mình.
	ErrForbidden = errForbidden{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "order: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "order: định danh không hợp lệ" }

type errInvalidInput struct{}

func (errInvalidInput) Error() string { return "order: dữ liệu không hợp lệ" }

type errInvalidStatus struct{}

func (errInvalidStatus) Error() string { return "order: chuyển trạng thái không hợp lệ" }

type errNotCancellable struct{}

func (errNotCancellable) Error() string { return "order: không còn hủy được" }

type errForbidden struct{}

func (errForbidden) Error() string {
	return "order: đơn thực hiện không thuộc về nhà bán này"
}

// ---------------------------------------------------------------- Tiền tố

const (
	OrderIDPrefix       = string(ids.PrefixOrder)
	OrderLineIDPrefix   = string(ids.PrefixOrderLine)
	FulfillmentIDPrefix = string(ids.PrefixFulfillmentOrder)
)

// Trạng thái đơn hàng, để module khác không phải đoán chuỗi.
const (
	StatusPendingPayment     = "PENDING_PAYMENT"
	StatusPaid               = "PAID"
	StatusProcessing         = "PROCESSING"
	StatusPartiallyShipped   = "PARTIALLY_SHIPPED"
	StatusShipped            = "SHIPPED"
	StatusPartiallyDelivered = "PARTIALLY_DELIVERED"
	StatusDelivered          = "DELIVERED"
	StatusPartiallyCancelled = "PARTIALLY_CANCELLED"
	StatusCancelled          = "CANCELLED"
	StatusCompleted          = "COMPLETED"
)

// Trạng thái đơn vị công việc vận hành.
const (
	FulfillmentPending   = "PENDING"
	FulfillmentConfirmed = "CONFIRMED"
	FulfillmentPacked    = "PACKED"
	FulfillmentShipped   = "SHIPPED"
	FulfillmentDelivered = "DELIVERED"
	FulfillmentCancelled = "CANCELLED"
)
