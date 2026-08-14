package domain

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// CustomerRepository là PORT cho kho lưu trữ hồ sơ khách hàng.
type CustomerRepository interface {
	// Save ghi hồ sơ MỚI.
	//
	// Trả ErrDuplicateEmail nếu email đã dùng bởi hồ sơ chưa ẩn danh.
	Save(ctx context.Context, c *Customer) error

	// Update ghi thay đổi bằng KHÓA LẠC QUAN.
	//
	// Trả ErrVersionConflict nếu bản ghi đã bị thao tác khác sửa. Khóa lạc
	// quan chứ không phải khóa bi quan: xung đột hiếm, và giữ khóa suốt
	// một request HTTP là cách chắc chắn nhất để làm nghẽn database.
	Update(ctx context.Context, c *Customer) error

	FindByID(ctx context.Context, id ids.ID) (*Customer, error)

	// FindByEmail tra hồ sơ theo email ĐÃ CHUẨN HÓA.
	//
	// Chỉ tìm trong hồ sơ CHƯA ẩn danh — hồ sơ đã ẩn danh có email giả.
	FindByEmail(ctx context.Context, email string) (*Customer, error)

	// FindByUserID tra hồ sơ từ tài khoản đăng nhập.
	//
	// Đây là truy vấn nóng nhất của module: chạy ở mỗi request của khách
	// đã đăng nhập.
	FindByUserID(ctx context.Context, userID ids.ID) (*Customer, error)

	// FindManyByIDs đọc nhiều hồ sơ trong MỘT truy vấn.
	//
	// Tồn tại để tránh N+1: hiển thị danh sách 50 đơn hàng mà gọi FindByID
	// 50 lần là 50 lượt đi-về database cho một trang.
	FindManyByIDs(ctx context.Context, ids []ids.ID) (map[ids.ID]*Customer, error)
}

// AddressRepository là PORT cho sổ địa chỉ.
type AddressRepository interface {
	Save(ctx context.Context, a *Address) error
	Update(ctx context.Context, a *Address) error

	FindByID(ctx context.Context, id ids.ID) (*Address, error)

	// ListByCustomer trả các địa chỉ CHƯA xóa, mặc định lên đầu.
	ListByCustomer(ctx context.Context, customerID ids.ID) ([]*Address, error)

	// FindDefault trả địa chỉ mặc định.
	//
	// Trả ErrAddressNotFound nếu khách chưa có địa chỉ nào.
	FindDefault(ctx context.Context, customerID ids.ID) (*Address, error)

	// ClearDefault gỡ cờ mặc định khỏi MỌI địa chỉ của một khách.
	//
	// Phải gọi TRƯỚC khi đặt địa chỉ mới làm mặc định, trong CÙNG một
	// giao dịch. Ngược lại sẽ đụng chỉ mục UNIQUE một phần.
	ClearDefault(ctx context.Context, customerID ids.ID, now time.Time) error

	// SetDefault đặt một địa chỉ làm mặc định, gỡ cờ khỏi các địa chỉ
	// khác trong MỘT giao dịch.
	//
	// Gộp hai thao tác vào một hàm có chủ ý: tách ra thì bên gọi phải nhớ
	// tự mở giao dịch, và chỗ nào quên sẽ để khách không có địa chỉ mặc
	// định nào giữa hai câu lệnh.
	SetDefault(ctx context.Context, customerID, addressID ids.ID, now time.Time) error
}

// ConsentRepository là PORT cho nhật ký đồng ý.
//
// CHỈ GHI THÊM: không có Update, không có Delete. Sửa nhật ký đồng ý là
// hủy giá trị pháp lý của chính nó.
type ConsentRepository interface {
	Record(ctx context.Context, c *Consent) error

	// Current trả bản ghi MỚI NHẤT của mỗi loại đồng ý.
	//
	// Loại không có bản ghi nào sẽ KHÔNG có trong map — nghĩa là khách
	// CHƯA đồng ý, không phải "đã từ chối". Bên gọi phải hiểu vắng mặt là
	// không đồng ý.
	Current(ctx context.Context, customerID ids.ID) (map[ConsentType]*Consent, error)

	// History trả toàn bộ lịch sử, mới nhất trước.
	//
	// Dùng khi cần chứng minh với cơ quan quản lý.
	History(ctx context.Context, customerID ids.ID) ([]*Consent, error)
}

// WishlistRepository là PORT cho danh sách yêu thích.
type WishlistRepository interface {
	Save(ctx context.Context, w *Wishlist) error

	// FindDefault trả danh sách mặc định của khách.
	//
	// Trả ErrNotFound nếu khách chưa có danh sách nào — bên gọi tự quyết
	// định tạo mới hay không.
	FindDefault(ctx context.Context, customerID ids.ID) (*Wishlist, error)

	// AddItem thêm một món.
	//
	// IDEMPOTENT ở tầng DATABASE: khóa chính (wishlist, product, variant)
	// chặn bản sao. Kiểm tra ở tầng ứng dụng không cứu được khi khách bấm
	// tim hai lần thật nhanh.
	//
	// Trả về true nếu món thật sự được thêm mới.
	AddItem(ctx context.Context, wishlistID ids.ID, item WishlistItem) (bool, error)

	// RemoveItem bỏ một món. Trả về true nếu thật sự có món bị bỏ.
	RemoveItem(ctx context.Context, wishlistID, productID, variantID ids.ID) (bool, error)

	// CountByProduct đếm số khách đã thích một sản phẩm.
	//
	// Đây là TÍN HIỆU NHU CẦU: nhiều người thích mà chưa mua thường là
	// dấu hiệu giá cao hoặc hết size.
	CountByProduct(ctx context.Context, productID ids.ID) (int, error)
}

// MergeLogRepository ghi nhật ký gộp danh tính.
//
// Gộp danh tính KHÔNG ĐẢO NGƯỢC ĐƯỢC và chạm tới lịch sử mua hàng. Bảng
// này để trả lời "vì sao đơn hàng này lại thuộc về tài khoản kia".
type MergeLogRepository interface {
	Record(ctx context.Context, m MergeRecord) error
	ListByTarget(ctx context.Context, targetID ids.ID) ([]MergeRecord, error)
}

// MergeRecord là một lần gộp danh tính.
type MergeRecord struct {
	SourceCustomerID ids.ID
	TargetCustomerID ids.ID

	// Reason là căn cứ được phép gộp: "email_verified" là đường duy nhất
	// ở MVP.
	Reason string

	MergedAt time.Time
}
