package domain

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// ErrNotFound là lỗi chung khi không tìm thấy bản ghi.
var ErrNotFound = errors.New("inventory: không tìm thấy")

// ErrDuplicateItem khi đã có bản ghi cho cùng (sku, địa điểm, chủ sở hữu).
//
// Hai bản ghi cho cùng tổ hợp làm tổng tồn kho tính ra sai mà không ai biết.
var ErrDuplicateItem = errors.New("inventory: đã có bản ghi tồn kho cho tổ hợp này")

// ItemKey là KHÓA ĐỊNH DANH NGHIỆP VỤ của một bản ghi tồn kho.
//
// Ba trường, không phải một: cùng một SKU có thể nằm ở nhiều kho và thuộc
// nhiều chủ sở hữu khác nhau (mục 3.1 của đặc tả).
type ItemKey struct {
	SKUID      ids.ID
	LocationID ids.ID
	OwnerID    ids.ID
}

// ItemRepository là PORT cho bản ghi tồn kho.
type ItemRepository interface {
	// Create tạo bản ghi mới với số lượng RỖNG.
	Create(ctx context.Context, item *InventoryItem) error

	FindByID(ctx context.Context, id ids.ID) (*InventoryItem, error)
	FindByKey(ctx context.Context, key ItemKey) (*InventoryItem, error)

	// FindBySKUs lấy tồn kho của NHIỀU SKU trong một truy vấn.
	//
	// Trang danh sách 50 sản phẩm cần biết còn hàng không — phải là 1 truy
	// vấn, không phải 50.
	FindBySKUs(ctx context.Context, skuIDs []ids.ID, locationID ids.ID) (map[ids.ID][]*InventoryItem, error)

	FindByOwner(ctx context.Context, ownerID ids.ID, limit, offset int) ([]*InventoryItem, error)

	// ApplyChange ghi thay đổi số lượng bằng KHÓA LẠC QUAN.
	//
	// Đây là phương thức QUAN TRỌNG NHẤT của cả module.
	//
	// `expectedVersion` là version đã đọc lúc lấy item ra. Nếu bản ghi
	// trong database đã có version khác, nghĩa là tiến trình khác vừa sửa
	// — thao tác bị TỪ CHỐI với ErrVersionConflict.
	//
	// Cài đặt PHẢI kiểm tra điều kiện số lượng NGUYÊN TỬ trong cùng câu
	// UPDATE, không phải đọc rồi ghi:
	//
	//	UPDATE inventory_item
	//	SET quantity_available = ..., version = version + 1
	//	WHERE id = $1 AND version = $2 AND quantity_available >= $3
	//
	// Đọc-rồi-ghi có khoảng trống giữa hai bước, và hai khách mua cùng lúc
	// sẽ cùng thấy "còn 1" rồi cùng mua — bán 2 sản phẩm khi chỉ có 1.
	//
	// Xem mục 5.2 của đặc tả.
	ApplyChange(ctx context.Context, item *InventoryItem, expectedVersion int64) error
}

// ReservationRepository là PORT cho việc giữ hàng.
type ReservationRepository interface {
	Save(ctx context.Context, r *Reservation) error
	FindByID(ctx context.Context, id ids.ID) (*Reservation, error)
	FindByCheckout(ctx context.Context, checkoutID ids.ID) ([]*Reservation, error)

	// FindExpired tìm reservation quá hạn chưa xử lý — cho tiến trình nền.
	//
	// Cơ chế này phải ĐÁNG TIN CẬY: nếu nó ngừng chạy, hàng bị khóa dần và
	// cuối cùng không bán được gì (mục 6.3 của đặc tả).
	FindExpired(ctx context.Context, before time.Time, limit int) ([]*Reservation, error)

	// CountExpiredPending đếm số reservation quá hạn chưa xử lý.
	//
	// Chỉ báo giám sát: cảnh báo khi > 100 (mục 13).
	CountExpiredPending(ctx context.Context, before time.Time) (int, error)
}

// MovementRepository là PORT cho nhật ký biến động.
//
// CHỈ CÓ GHI THÊM VÀ ĐỌC — không có Update, không có Delete.
//
// Sự thiếu vắng đó là CÓ CHỦ ĐÍCH: nhật ký sửa được thì không tái dựng
// được trạng thái quá khứ và không điều tra được sai lệch kiểm kê. Ở tầng
// database còn có trigger chặn, giống sổ cái tài chính (ADR-0008).
type MovementRepository interface {
	Append(ctx context.Context, m *InventoryMovement) error

	FindByItem(ctx context.Context, itemID ids.ID, limit int) ([]*InventoryMovement, error)
	FindBySKU(ctx context.Context, skuID ids.ID, from, to time.Time) ([]*InventoryMovement, error)
}

// UnitOfWork gom nhiều thao tác vào MỘT giao dịch.
//
// VÌ SAO CẦN: đổi số lượng và ghi nhật ký phải cùng thành công hoặc cùng
// thất bại. Nếu số lượng đổi mà nhật ký không ghi, quy tắc 4 bị vi phạm và
// sai lệch sau này không truy được nguyên nhân.
type UnitOfWork interface {
	// Do chạy fn trong một giao dịch. Trả lỗi thì giao dịch được hoàn tác.
	Do(ctx context.Context, fn func(Repos) error) error
}

// Repos là bộ repository dùng bên trong một giao dịch.
type Repos struct {
	Items        ItemRepository
	Reservations ReservationRepository
	Movements    MovementRepository
	Locations    LocationRepository
}
