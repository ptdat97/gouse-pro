// Package inventory là module quản lý tồn kho.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// ĐẶC ĐIỂM QUAN TRỌNG: inventory KHÔNG GỌI module nghiệp vụ nào. Nó ở tầng
// thấp, được nhiều module gọi nhưng không phụ thuộc ai. Điều này làm nó dễ
// kiểm thử và là ứng viên tách service rõ ràng nhất về mặt phụ thuộc.
//
// Module này trả lời đúng một câu hỏi: "còn bao nhiêu, ở đâu, trạng thái
// gì". Nó KHÔNG biết ai đang mua, giá bao nhiêu, hay kho nào nên xuất hàng.
package inventory

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module inventory.
//
// Chữ ký khớp docs/04-modules/inventory.md mục 8.
type API interface {
	// ---- Truy vấn: LUÔN theo lô để tránh N+1 ----

	// GetAvailability tra số lượng khả dụng của nhiều SKU.
	//
	// locationID rỗng = cộng gộp mọi kho. Đây là con số khách nhìn thấy:
	// họ không quan tâm hàng nằm ở kho nào.
	GetAvailability(ctx context.Context, skuIDs []string, locationID string) (map[string]int, error)

	// CheckAvailability kiểm tra đủ hàng cho một giỏ nhiều món.
	//
	// Trả về TỪNG món thiếu chứ không chỉ true/false — khách cần biết món
	// nào không mua được để bỏ ra, không phải "giỏ hàng có vấn đề".
	CheckAvailability(ctx context.Context, items []AvailabilityRequest) (AvailabilityResult, error)

	// GetItemsBySKUs trả bản ghi tồn kho của nhiều SKU.
	//
	// Cần cho checkout: nó biết SKU khách mua, nhưng Reserve làm việc trên
	// InventoryItem — một SKU có thể nằm ở nhiều kho, và việc chọn kho nào
	// là quyết định của bên giữ hàng.
	//
	// Một SKU trả về NHIỀU item vì cùng món hàng có thể nằm ở Hà Nội và
	// TP.HCM; bên gọi chọn theo tiêu chí của mình.
	GetItemsBySKUs(ctx context.Context, skuIDs []string, locationID string) (map[string][]ItemView, error)

	// ---- Giữ hàng ----

	Reserve(ctx context.Context, req ReserveRequest) (*ReservationView, error)
	ReleaseReservation(ctx context.Context, reservationID string) error
	ExtendReservation(ctx context.Context, reservationID string, ttl time.Duration) error

	// Commit chuyển hàng đang giữ thành cam kết cho đơn đã xác nhận.
	Commit(ctx context.Context, reservationID string) error

	// CommitInEventTx chuyển Reserved → Committed bằng GIAO DỊCH của
	// dispatcher event.
	//
	// Dành riêng cho bên nhận domain event. Ngữ cảnh truyền vào phải mang
	// giao dịch do eventbus mở — nếu không, hàm trả lỗi thay vì âm thầm mở
	// giao dịch riêng.
	//
	// VÌ SAO CẦN HÀM RIÊNG: việc thay đổi tồn kho và việc đánh dấu event
	// đã xử lý phải cùng thành công hoặc cùng thất bại. Tách rời nghĩa là
	// commit thành công + đánh dấu thất bại → lần thử lại commit lần hai.
	CommitInEventTx(ctx context.Context, reservationID string) error

	// ---- Vận hành kho ----

	Ship(ctx context.Context, itemID string, quantity int, orderID string) error
	Receive(ctx context.Context, req ReceiveRequest) (*ItemView, error)
	Adjust(ctx context.Context, req AdjustRequest) error

	// ---- Hoàn hàng ----

	// ReceiveReturn nhận hàng khách trả về.
	//
	// Hàng vào trạng thái Returned, KHÔNG vào Available — phải qua kiểm
	// định trước. Bán lại hàng hỏng gây thiệt hại uy tín lớn hơn nhiều so
	// với giá trị món hàng.
	ReceiveReturn(ctx context.Context, itemID string, quantity int, returnID string) error
	ProcessReturnInspection(ctx context.Context, req InspectionRequest) error
}

// ---------------------------------------------------------------- DTO

// AvailabilityRequest là một dòng cần kiểm tra tồn kho.
type AvailabilityRequest struct {
	SKUID      string
	Quantity   int
	LocationID string
}

// AvailabilityResult là kết quả kiểm tra cả giỏ.
type AvailabilityResult struct {
	// AllAvailable đúng khi MỌI món đều đủ hàng.
	AllAvailable bool

	// Insufficient liệt kê từng món không đủ, kèm số còn lại.
	//
	// Chi tiết này cần cho trải nghiệm tốt: "chỉ còn 2 sản phẩm" hữu ích
	// hơn nhiều so với "hết hàng" — khách quyết định được giảm số lượng
	// hay bỏ món đó.
	Insufficient []InsufficientItem
}

// InsufficientItem là một món không đủ hàng.
type InsufficientItem struct {
	SKUID     string
	Requested int
	Available int
}

// ReserveRequest là yêu cầu giữ hàng.
type ReserveRequest struct {
	ItemID     string
	CheckoutID string
	Quantity   int

	// TTL bằng 0 nghĩa là dùng mặc định (15 phút).
	TTL time.Duration
}

// ReservationView là thông tin một lần giữ hàng.
type ReservationView struct {
	ID         string
	ItemID     string
	CheckoutID string
	Quantity   int

	// ExpiresAt định dạng RFC3339. Client dùng để hiển thị đồng hồ đếm ngược.
	ExpiresAt string
	Status    string
}

// ReceiveRequest là yêu cầu nhập hàng vào kho.
type ReceiveRequest struct {
	SKUID      string
	LocationID string

	// OwnerID rỗng = hàng của nền tảng.
	OwnerID string

	Quantity    int
	ReferenceID string
	PerformedBy string
	BatchID     string
}

// AdjustRequest là yêu cầu điều chỉnh thủ công sau kiểm kê.
//
// Reason và PerformedBy là BẮT BUỘC (quy tắc 7): điều chỉnh không lý do là
// điểm mù trong kiểm toán — không phân biệt được sai sót kiểm kê với thất thoát.
type AdjustRequest struct {
	ItemID      string
	Delta       int
	Reason      string
	PerformedBy string
}

// InspectionRequest là kết quả kiểm định hàng hoàn.
type InspectionRequest struct {
	ItemID   string
	Quantity int

	// Passed = true → hàng bán lại được. false → hàng hỏng.
	Passed      bool
	Reason      string
	PerformedBy string
}

// ItemView là thông tin tồn kho cho module khác — CHỈ ĐỌC.
type ItemView struct {
	ID         string
	SKUID      string
	LocationID string
	OwnerID    string

	// Sáu trạng thái tồn kho.
	Available int
	Reserved  int
	Committed int
	InTransit int
	Damaged   int
	Returned  int

	// Total là tổng số lượng vật lý đang nắm giữ.
	Total int

	// IsPlatformOwned phân biệt hàng nền tảng với hàng seller gửi kho.
	//
	// Quan trọng khi đối soát: hàng seller nằm ở kho nền tảng KHÔNG phải
	// tài sản của nền tảng.
	IsPlatformOwned bool

	Version int64
}

// ---------------------------------------------------------------- Lỗi

var (
	// ErrNotFound khi không tìm thấy tài nguyên.
	ErrNotFound = errNotFound{}
	// ErrInvalidID khi định danh sai định dạng.
	ErrInvalidID = errInvalidID{}

	// ErrInsufficientStock khi không đủ hàng.
	//
	// KHÔNG nên thử lại: hàng không tự xuất hiện.
	ErrInsufficientStock = errInsufficient{}

	// ErrConflict khi tranh chấp đồng thời không giải quyết được sau khi
	// đã thử lại.
	//
	// NÊN thử lại ở tầng gọi hoặc báo khách thử lại — khác hoàn toàn với
	// ErrInsufficientStock.
	ErrConflict = errConflict{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "inventory: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "inventory: định danh không hợp lệ" }

type errInsufficient struct{}

func (errInsufficient) Error() string { return "inventory: không đủ hàng" }

type errConflict struct{}

func (errConflict) Error() string {
	return "inventory: hệ thống đang bận, vui lòng thử lại"
}

// ---------------------------------------------------------------- Tiền tố

const (
	ItemIDPrefix        = string(ids.PrefixInventoryItem)
	ReservationIDPrefix = string(ids.PrefixReservation)
	LocationIDPrefix    = string(ids.PrefixStockLocation)
)

// PlatformOwnerID là định danh chủ sở hữu cho hàng của nền tảng.
const PlatformOwnerID = "own_platform"

// OwnerForSeller trả CHỦ SỞ HỮU tồn kho ứng với một nhà bán.
//
// # Vì sao không phải lúc nào cũng là chính nhà bán đó
//
// Own brand của nền tảng là một seller NỘI BỘ (own-brand.md mục 7,
// seller.md mục 3): nó có bản ghi seller, có offer, đi chung một luồng đơn
// hàng với nhà bán ngoài. Nhưng hàng thì KHÔNG phải của nó — hàng là tài
// sản của NỀN TẢNG. Bản ghi seller nội bộ khai đúng điều đó bằng
// `inventory_owner: PLATFORM`.
//
// Nhầm chỗ này là ghi sai tài sản trên sổ sách theo cả hai chiều: hàng
// nền tảng biến thành hàng ký gửi của một seller không sở hữu nó, hoặc
// hàng seller bị tính thành tài sản nền tảng (fulfillment.md mục 2.3).
//
// # Vì sao là hàm chứ không phải trường trên SellerView
//
// Hằng số `own_platform` thuộc về module này. Để module seller tự sinh ra
// nó nghĩa là cùng một chuỗi được định nghĩa ở hai nơi, và hai nơi sẽ
// lệch. Bên gọi ghép hai mảnh: `IsInternal` từ seller, định danh từ đây.
//
// Hàm THUẦN, không chạm database: bên gọi thường đã có SellerView trong
// tay rồi.
func OwnerForSeller(sellerID string, isInternal bool) string {
	if isInternal {
		return PlatformOwnerID
	}
	return sellerID
}
