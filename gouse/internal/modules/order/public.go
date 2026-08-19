// Package order quản lý HỢP ĐỒNG VỚI KHÁCH HÀNG.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// HAI KHÁI NIỆM, HAI MODULE (ADR-0007):
//
//	order        = HỢP ĐỒNG với khách, gần như bất biến sau khi đặt
//	fulfillment  = GÓC NHÌN VẬN HÀNH, thay đổi liên tục theo tiến trình
//
// Module này KHÔNG có API nào cho seller. Order chứa dòng hàng của MỌI
// seller trong đơn, nên nếu seller đọc được Order thì phải lọc ở tầng hiển
// thị — và quên một lần là rò rỉ dữ liệu đối thủ.
//
// Seller làm việc với `fulfillment`, nơi ranh giới bảo mật nằm sẵn trong
// cấu trúc dữ liệu: mọi truy vấn lọc theo seller_id ngay trong SQL.
//
// Trạng thái tổng hợp của đơn được SUY RA từ tiến độ các nguồn hàng
// (quy tắc 7). Module này LẮNG NGHE event từ fulfillment và tự tính —
// không hỏi ngược, vì hỏi ngược tạo phụ thuộc vòng.
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

	// ResolveViewableOrder phân giải mã đơn VÀ kiểm tra quyền xem.
	//
	// # Vì sao là hàm CỦA MODULE ORDER, không phải của bên hỏi
	//
	// Quyền xem đơn được hỏi từ nhiều nơi — trang chi tiết đơn, trang hủy
	// đơn, và endpoint lô giao của module fulfillment. Mỗi nơi tự cài lại
	// nghĩa là sớm muộn có một nơi cài lỏng hơn, và MỘT nơi lỏng là đủ để
	// lộ lịch sử mua hàng của người khác.
	//
	// `customerID` rỗng = khách vãng lai; khi đó `guestPhone` là thứ duy
	// nhất chứng minh đơn thuộc về họ.
	//
	// Trả false cho cả đơn KHÔNG TỒN TẠI lẫn đơn KHÔNG PHẢI của người hỏi:
	// phân biệt hai trường hợp là để lộ mã đơn nào có thật, mà mã đơn thì
	// tăng dần nên dò được số đơn nền tảng bán mỗi tháng.
	// `key` nhận CẢ mã đơn (`ord_...`) lẫn mã hiển thị
	// (`FC-2026-08-000001`): khách vãng lai chỉ có mã hiển thị trong email
	// xác nhận, không có mã nội bộ.
	//
	// Trả về mã đơn CHUẨN khi được phép — bên gọi cần nó để tra dữ liệu
	// của mình, và bắt họ tự phân giải lần nữa là làm hai lần một việc.
	ResolveViewableOrder(
		ctx context.Context, key, customerID, guestPhone string,
	) (orderID string, allowed bool, err error)

	GetOrder(ctx context.Context, orderID string) (*OrderView, error)
	GetOrderByNumber(ctx context.Context, number string) (*OrderView, error)

	ListCustomerOrders(ctx context.Context, customerID string, limit, offset int) ([]OrderView, error)

	// ApplyFulfillmentProgress tính lại trạng thái tổng hợp của đơn.
	//
	// QUY TẮC 7: trạng thái đơn được SUY RA từ tiến độ các nguồn hàng,
	// không tự đặt.
	//
	// Nhận dữ liệu THUẦN chứ không phải kiểu của module fulfillment: module
	// này KHÔNG phụ thuộc module đó, và không hỏi ngược — hỏi ngược tạo
	// phụ thuộc vòng (ADR-0007).
	ApplyFulfillmentProgress(ctx context.Context, orderID string, progress []FulfillmentProgressInput) error

	// MarkOrderPaid ghi nhận đơn đã thanh toán.
	MarkOrderPaid(ctx context.Context, orderID string) error

	// CancelOrder hủy toàn bộ đơn theo yêu cầu của khách.
	//
	// Bị chặn nếu đã có gói nào đóng gói xong: từ đó việc hủy có chi phí
	// thật và cần quy trình riêng.
	CancelOrder(ctx context.Context, orderID, reason string) error

	// ---------------------------------------------------------- Quản trị

	// ListOrders trả đơn theo bộ lọc, cho giao diện quản trị.
	//
	// KHÔNG giới hạn theo khách — việc chặn ai được gọi nằm ở tầng route.
	ListOrders(ctx context.Context, f ListFilter) ([]OrderSummary, error)

	// ViewOrderAsAdmin đọc chi tiết đơn VÀ ghi vết việc đọc.
	//
	// Response chứa dữ liệu cá nhân (tên người nhận, số điện thoại, địa
	// chỉ), nên `Reason` là BẮT BUỘC — xem admin-api.md mục 6.
	ViewOrderAsAdmin(ctx context.Context, req ViewOrderRequest) (*OrderView, error)

	// CancelOrderAsAdmin hủy đơn VÀ ghi vết kiểm toán trong cùng giao dịch.
	CancelOrderAsAdmin(ctx context.Context, req CancelOrderRequest) (*OrderView, error)
}

// ListFilter là điều kiện lọc danh sách đơn ở giao diện quản trị.
type ListFilter struct {
	// OrderNumber tra chính xác một đơn — đường tra cứu chính của nhân
	// viên hỗ trợ, vì khách đọc mã này qua điện thoại.
	OrderNumber string

	Status     string
	CustomerID string

	Limit  int
	Offset int
}

// OrderSummary là đơn ở dạng tóm tắt, KHÔNG chứa dữ liệu cá nhân.
type OrderSummary struct {
	ID          string
	OrderNumber string
	Status      string
	Total       Amount
	LineCount   int
	PlacedAt    string
}

// ViewOrderRequest là yêu cầu xem chi tiết đơn từ giao diện quản trị.
type ViewOrderRequest struct {
	OrderID string
	ActorID string

	// Reason BẮT BUỘC, tối thiểu 20 ký tự.
	Reason    string
	RequestID string
}

// CancelOrderRequest là yêu cầu hủy đơn từ giao diện quản trị.
type CancelOrderRequest struct {
	OrderID string
	ActorID string

	// Reason BẮT BUỘC, tối thiểu 20 ký tự.
	Reason    string
	RequestID string
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
	Order OrderView

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

// FulfillmentProgressInput là tiến độ của MỘT nguồn hàng.
//
// Dữ liệu thuần, không mang kiểu của module fulfillment — nhờ vậy module
// order không phụ thuộc module đó.
type FulfillmentProgressInput struct {
	// Cancelled và Delivered là hai trạng thái CUỐI có ý nghĩa với khách.
	Cancelled bool
	Delivered bool

	// Shipped đúng khi hàng đã rời kho, bao gồm cả trường hợp đã giao.
	Shipped bool
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound       = errNotFound{}
	ErrInvalidID      = errInvalidID{}
	ErrInvalidInput   = errInvalidInput{}
	ErrInvalidStatus  = errInvalidStatus{}
	ErrNotCancellable = errNotCancellable{}
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

// ---------------------------------------------------------------- Tiền tố

const (
	OrderIDPrefix     = string(ids.PrefixOrder)
	OrderLineIDPrefix = string(ids.PrefixOrderLine)
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
