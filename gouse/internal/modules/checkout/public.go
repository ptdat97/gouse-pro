// Package checkout quản lý phiên thanh toán — bước biến giỏ thành đơn.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// HAI VIỆC MODULE NÀY LÀM MÀ KHÔNG MODULE NÀO KHÁC LÀM:
//
//  1. GIỮ TỒN KHO — nơi duy nhất trong hệ thống khóa hàng cho khách
//  2. ĐÓNG BĂNG GIÁ — điểm giá chuyển từ động sang tĩnh
//
// Ngoài hai việc đó, nó là module ĐIỀU PHỐI: gọi cart, inventory,
// marketplace và order, nhưng gần như không sở hữu luật nghiệp vụ nào.
//
// Vì sao đóng băng (checkout.md mục 5):
//
//	14:00 — Khách bắt đầu checkout, áo giá 299.000đ
//	14:05 — Seller đổi giá thành 350.000đ
//	14:10 — Khách hoàn tất thanh toán
//
//	Không đóng băng: khách thấy 299.000đ nhưng bị trừ 350.000đ
//	Đóng băng:       khách trả đúng 299.000đ như đã thấy
package checkout

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module checkout.
type API interface {
	// StartCheckout mở phiên từ một giỏ hàng.
	//
	// GIỮ HÀNG rồi ĐÓNG BĂNG GIÁ, theo đúng thứ tự đó. Không giữ được hàng
	// thì không mở phiên — và mọi thứ đã giữ được sẽ được nhả lại.
	//
	// Gọi lại khi phiên đang chạy sẽ trả về ĐÚNG phiên đó, không mở phiên
	// thứ hai: phiên thứ hai giữ hàng lần thứ hai cho cùng một giỏ.
	StartCheckout(ctx context.Context, req StartCheckoutRequest) (*CheckoutView, error)

	GetCheckout(ctx context.Context, checkoutID string) (*CheckoutView, error)

	SetShippingAddress(ctx context.Context, checkoutID string, addr AddressInput) (*CheckoutView, error)
	SetShippingMethod(ctx context.Context, checkoutID, method string, fee Amount) (*CheckoutView, error)

	ApplyCoupon(ctx context.Context, checkoutID, code string, discount Amount) (*CheckoutView, error)
	RemoveCoupon(ctx context.Context, checkoutID string) (*CheckoutView, error)

	// MarkReadyForPayment chuyển sang chờ thanh toán.
	//
	// Yêu cầu đã có địa chỉ: không có địa chỉ thì đơn tạo ra không giao
	// được, và phát hiện lúc đó thì tiền đã thu rồi.
	MarkReadyForPayment(ctx context.Context, checkoutID string) (*CheckoutView, error)

	// ExtendCheckout gia hạn phiên VÀ gia hạn hàng đang giữ.
	//
	// Có giới hạn số lần: gia hạn vô hạn nghĩa là khóa hàng vô hạn.
	ExtendCheckout(ctx context.Context, checkoutID string) (*CheckoutView, error)

	// CancelCheckout hủy phiên và NHẢ HÀNG.
	CancelCheckout(ctx context.Context, checkoutID string) error

	// CompleteCheckout tạo đơn hàng từ phiên.
	//
	// IDEMPOTENT theo idempotencyKey: gọi lại KHÔNG tạo đơn thứ hai.
	//
	// Thanh toán thất bại KHÔNG hủy phiên — khách thử lại được phương thức
	// khác trong thời gian còn lại. Hủy ngay là trải nghiệm tệ và làm mất
	// đơn hàng.
	CompleteCheckout(ctx context.Context, checkoutID, idempotencyKey string) (*CompleteResult, error)

	// ExpireStale dọn các phiên quá hạn và nhả hàng.
	//
	// Gọi bởi tiến trình nền. Đây là hàm giữ cho lời hứa "giữ hàng có thời
	// hạn" thành sự thật.
	ExpireStale(ctx context.Context, limit int) (int, error)

	// CountExpiredPending đếm phiên quá hạn chưa dọn.
	//
	// Chỉ báo giám sát: con số tăng dần nghĩa là tiến trình dọn đã ngừng
	// chạy, và hàng đang bị khóa mà không ai biết.
	CountExpiredPending(ctx context.Context) (int, error)

	// CountHoanTatKetLai đếm phiên đã tạo đơn mà chuỗi hoàn tất chưa xong.
	//
	// Khác 0 nghĩa là có đơn hàng tồn tại với phiên chưa đóng, và hàng của
	// nó nằm giữ cho tới khi có người đối soát.
	CountHoanTatKetLai(ctx context.Context) (int, error)
}

// ---------------------------------------------------------------- DTO

// Amount là số tiền kèm đơn vị.
type Amount struct {
	Value    int64
	Currency string
}

// AddressInput là địa chỉ giao hàng.
type AddressInput struct {
	RecipientName string
	Phone         string
	StreetAddress string
	Ward          string
	District      string
	Province      string
	CountryCode   string
}

// StartCheckoutRequest là dữ liệu mở phiên thanh toán.
type StartCheckoutRequest struct {
	CartID string

	// Khách vãng lai: cần email để liên hệ (quy tắc 6).
	GuestEmail string
	GuestPhone string
}

// CheckoutView là phiên thanh toán để hiển thị.
type CheckoutView struct {
	ID         string
	CartID     string
	CustomerID string
	GuestEmail string
	Currency   string
	Status     string

	ShippingAddress AddressInput
	ShippingMethod  string

	Lines []CheckoutLineView

	Subtotal       Amount
	ShippingFee    Amount
	DiscountAmount Amount
	TaxAmount      Amount

	// Total là CON SỐ KHÁCH NHÌN THẤY, và nó phải bằng đúng con số vào đơn.
	Total Amount

	CouponCode string

	// SellerIDs để tính phí ship theo từng nguồn và hiển thị thời gian
	// giao riêng cho từng nhóm — khách cần biết món nào đến trước.
	SellerIDs []string

	ExpiresAt string

	// SecondsLeft để client hiển thị đồng hồ đếm ngược.
	SecondsLeft int

	ExtendedTimes int
	OrderID       string
}

// CheckoutLineView là một dòng với giá ĐÃ ĐÓNG BĂNG.
type CheckoutLineView struct {
	ID       string
	OfferID  string
	SKUID    string
	SellerID string

	ProductName        string
	VariantDescription string

	// UnitPrice đã ĐÓNG BĂNG: con số này không đổi cho tới khi phiên kết
	// thúc, dù seller có sửa giá.
	UnitPrice Amount
	Quantity  int
	LineTotal Amount

	// ReservationID là bằng chứng hàng đang được giữ. Rỗng nghĩa là chưa
	// giữ được, và dòng đó không được vào đơn.
	ReservationID string
}

// CompleteResult là kết quả hoàn tất phiên thanh toán.
type CompleteResult struct {
	Checkout    CheckoutView
	OrderID     string
	OrderNumber string

	// Replayed = true nghĩa là đơn đã tồn tại từ lần gọi trước.
	//
	// Bên gọi PHẢI kiểm tra trước khi gửi email xác nhận hay trừ tiền.
	Replayed bool
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound      = errNotFound{}
	ErrInvalidID     = errInvalidID{}
	ErrInvalidInput  = errInvalidInput{}
	ErrInvalidStatus = errInvalidStatus{}

	// ErrExpired khi phiên đã hết hạn — hàng có thể đã bán cho người khác.
	ErrExpired = errExpired{}

	// ErrOutOfStock khi không giữ đủ hàng.
	ErrOutOfStock = errOutOfStock{}

	// ErrNoAddress khi thiếu địa chỉ giao hàng.
	ErrNoAddress = errNoAddress{}

	// ErrTooManyExtends khi đã hết số lần gia hạn.
	ErrTooManyExtends = errTooManyExtends{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "checkout: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "checkout: định danh không hợp lệ" }

type errInvalidInput struct{}

func (errInvalidInput) Error() string { return "checkout: dữ liệu không hợp lệ" }

type errInvalidStatus struct{}

func (errInvalidStatus) Error() string { return "checkout: chuyển trạng thái không hợp lệ" }

type errExpired struct{}

func (errExpired) Error() string {
	return "checkout: phiên thanh toán đã hết hạn, vui lòng bắt đầu lại"
}

type errOutOfStock struct{}

func (errOutOfStock) Error() string { return "checkout: không đủ hàng" }

type errNoAddress struct{}

func (errNoAddress) Error() string { return "checkout: phải có địa chỉ giao hàng" }

type errTooManyExtends struct{}

func (errTooManyExtends) Error() string { return "checkout: đã hết số lần gia hạn" }

// ---------------------------------------------------------------- Tiền tố

const (
	CheckoutIDPrefix     = string(ids.PrefixCheckout)
	CheckoutLineIDPrefix = string(ids.PrefixCheckoutLine)
)

// Trạng thái phiên thanh toán.
const (
	StatusStarted        = "STARTED"
	StatusPendingPayment = "PENDING_PAYMENT"
	StatusCompleted      = "COMPLETED"
	StatusCancelled      = "CANCELLED"
	StatusExpired        = "EXPIRED"
)
