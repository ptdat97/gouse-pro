// Package cart quản lý giỏ hàng — Ý ĐỊNH mua, chưa phải hợp đồng.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// ĐIỀU QUAN TRỌNG NHẤT VỀ MODULE NÀY LÀ THỨ NÓ KHÔNG LÀM: giỏ hàng KHÔNG
// GIỮ TỒN KHO. Không có hàm nào ở đây gọi inventory.Reserve, và không có
// trường nào lưu reservation.
//
//	Nếu giỏ giữ hàng: khách thêm rồi bỏ quên hai tuần → hàng khóa hai tuần.
//	                  Với hàng khan hiếm, vài trăm giỏ bỏ quên = hết hàng
//	                  ảo, không bán được cho khách thật sự muốn mua.
//
// HỆ QUẢ PHẢI CHẤP NHẬN: khách có thể thêm vào giỏ rồi tới lúc checkout
// mới biết hết hàng. Đây là đánh đổi ĐÚNG — số lượng hiển thị ở giỏ là
// THÔNG TIN THAM KHẢO, không phải cam kết. Cam kết chỉ có ở checkout.
package cart

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module cart.
type API interface {
	// GetOrCreateCart tìm giỏ đang dùng, tạo mới nếu chưa có.
	//
	// Một khách chỉ có MỘT giỏ đang dùng; gọi lại trả về đúng giỏ đó.
	GetOrCreateCart(ctx context.Context, req GetOrCreateRequest) (*CartView, error)

	// GetCart đọc giỏ, ĐÃ ĐỒNG BỘ giá và tình trạng hàng hiện tại.
	//
	// Giá trả về là giá HIỆN TẠI, có thể khác lần xem trước. Đó là hành vi
	// đúng: giỏ hiện giá cũ sau khi seller giảm giá sẽ làm khách bỏ lỡ
	// khuyến mãi, hoặc thấy giá thấp rồi bị tính giá cao ở bước thanh toán.
	GetCart(ctx context.Context, cartID string) (*CartView, error)

	AddItem(ctx context.Context, req AddItemRequest) (*CartView, error)
	UpdateItemQuantity(ctx context.Context, cartID, itemID string, quantity int) (*CartView, error)
	RemoveItem(ctx context.Context, cartID, itemID string) (*CartView, error)
	ClearCart(ctx context.Context, cartID string) error

	// MergeOnLogin gộp giỏ vãng lai vào giỏ tài khoản khi khách đăng nhập.
	//
	// Kết quả kèm CẢNH BÁO cho những món không gộp trọn vẹn được. Bên gọi
	// PHẢI hiển thị chúng — im lặng bỏ qua nghĩa là khách đăng nhập xong
	// thấy giỏ ít hàng hơn mà không hiểu vì sao.
	MergeOnLogin(ctx context.Context, customerID, sessionID string) (*MergeResult, error)

	// MarkConverted đánh dấu giỏ đã thành đơn hàng.
	//
	// Gọi bởi checkout sau khi đặt hàng thành công. Giỏ KHÔNG bị xóa: nó
	// cho biết nội dung nào dẫn tới việc mua thật.
	MarkConverted(ctx context.Context, cartID string) error
}

// ---------------------------------------------------------------- DTO

// Amount là số tiền kèm đơn vị.
type Amount struct {
	Value    int64
	Currency string
}

// GetOrCreateRequest là dữ liệu tìm hoặc tạo giỏ.
//
// Một trong hai định danh phải có. Khách vãng lai dùng SessionID; khi đăng
// nhập, giỏ theo phiên được gộp vào giỏ của tài khoản.
type GetOrCreateRequest struct {
	CustomerID string
	SessionID  string
	Currency   string
}

// AddItemRequest là dữ liệu thêm một món vào giỏ.
//
// KHÔNG có trường giá: giá được TRA từ marketplace, không nhận từ bên gọi.
// Nhận giá từ client nghĩa là khách đặt được giá của chính mình.
type AddItemRequest struct {
	CartID   string
	OfferID  string
	Quantity int

	// Nguồn giới thiệu — ghi ngay lúc THÊM GIỎ, không đợi lúc mua.
	//
	// Nhờ vậy đo được tỷ lệ "thêm giỏ" của từng nội dung (tín hiệu ý định
	// mua mạnh hơn lượt xem nhiều) và quy kết đúng khi khách mua sau vài
	// ngày.
	SourceContentID string
	SourceCreatorID string
}

// CartView là giỏ hàng để hiển thị.
type CartView struct {
	ID         string
	CustomerID string
	SessionID  string
	Currency   string
	Status     string

	Items []CartItemView

	// Subtotal tính từ các món MUA ĐƯỢC, theo GIÁ HIỆN TẠI.
	//
	// Món hết hàng không tính vào: hiện một con số bao gồm hàng đã hết sẽ
	// làm khách bất ngờ ở bước thanh toán.
	Subtotal Amount

	ItemCount     int
	TotalQuantity int

	// HasUnavailableItems cho biết có món nào cần khách xử lý không.
	//
	// Giao diện dùng nó để hiện cảnh báo trước khi khách bấm thanh toán.
	HasUnavailableItems bool

	// SellerIDs để NHÓM KHI HIỂN THỊ: khách cần hiểu hàng đến từ đâu và
	// thời gian giao khác nhau. Giỏ không chia theo seller ở tầng dữ liệu.
	SellerIDs []string

	ExpiresAt string
	UpdatedAt string
}

// CartItemView là một món trong giỏ.
type CartItemView struct {
	ID       string
	OfferID  string
	SKUID    string
	SellerID string

	ProductName        string
	VariantDescription string
	ImageURL           string

	// UnitPrice là giá HIỆN TẠI, không phải giá lúc thêm giỏ.
	UnitPrice Amount
	Quantity  int
	LineTotal Amount

	MaxOrderQuantity int

	// Availability: AVAILABLE, OUT_OF_STOCK, UNAVAILABLE, QUANTITY_REDUCED.
	//
	// Món không hợp lệ được ĐÁNH DẤU chứ không bị xóa: xóa im lặng làm
	// khách bối rối và nghi ngờ cả những món còn lại.
	Availability string

	// AvailableQuantity là số lượng còn bán tại lần đồng bộ gần nhất —
	// THÔNG TIN THAM KHẢO. Giỏ không giữ hàng nên con số này có thể sai
	// ngay khi khách đọc nó.
	AvailableQuantity int

	SourceContentID string
	SourceCreatorID string

	AddedAt string
}

// MergeResult là kết quả gộp giỏ khi đăng nhập.
type MergeResult struct {
	Cart CartView

	// Warnings PHẢI được hiển thị cho khách.
	Warnings []MergeWarningView
}

// MergeWarningView là một món không gộp trọn vẹn được.
type MergeWarningView struct {
	OfferID     string
	ProductName string

	// Reason: QUANTITY_CAPPED hoặc REJECTED.
	Reason string

	WantedQty int
	ActualQty int
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound     = errNotFound{}
	ErrInvalidID    = errInvalidID{}
	ErrInvalidInput = errInvalidInput{}

	// ErrQuantityOutOfRange khi số lượng vi phạm min/max của offer.
	ErrQuantityOutOfRange = errQuantityRange{}

	// ErrCartNotActive khi giỏ đã chuyển đổi hoặc bị bỏ quên.
	ErrCartNotActive = errCartNotActive{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "cart: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "cart: định danh không hợp lệ" }

type errInvalidInput struct{}

func (errInvalidInput) Error() string { return "cart: dữ liệu không hợp lệ" }

type errQuantityRange struct{}

func (errQuantityRange) Error() string {
	return "cart: số lượng nằm ngoài giới hạn của offer"
}

type errCartNotActive struct{}

func (errCartNotActive) Error() string { return "cart: giỏ không còn hoạt động" }

// ---------------------------------------------------------------- Tiền tố

const (
	CartIDPrefix     = string(ids.PrefixCart)
	CartItemIDPrefix = string(ids.PrefixCartItem)
)

// Tình trạng của món trong giỏ.
const (
	AvailabilityAvailable       = "AVAILABLE"
	AvailabilityOutOfStock      = "OUT_OF_STOCK"
	AvailabilityUnavailable     = "UNAVAILABLE"
	AvailabilityQuantityReduced = "QUANTITY_REDUCED"
)

// Trạng thái giỏ hàng.
const (
	StatusActive    = "ACTIVE"
	StatusConverted = "CONVERTED"
	StatusAbandoned = "ABANDONED"
	StatusMerged    = "MERGED"
)
