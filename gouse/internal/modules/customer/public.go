// Package customer quản lý hồ sơ khách hàng, sổ địa chỉ, wishlist và đồng ý.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// # Ranh giới: customer KHÔNG phải identity
//
//	identity  "ai đang gọi, họ được phép làm gì"   → hạ tầng
//	customer  "người này là ai"                    → nghiệp vụ
//
// Cầu nối là user_id. Module này GIỮ user_id nhưng KHÔNG import identity —
// nó không cần biết mật khẩu hay phiên đăng nhập tồn tại.
//
// # Khách vãng lai cũng có hồ sơ ở đây
//
// Khách chưa đăng ký vẫn đặt hàng được, và đơn hàng phải trỏ tới một
// customer_id — không thì không có chỗ nào giữ địa chỉ giao hàng và lịch
// sử mua. Với họ, UserID để trống và Status là GUEST.
//
// # Điều module này KHÔNG làm
//
// Nó KHÔNG giữ đơn hàng, giỏ hàng, hay điểm thưởng. Nó cũng KHÔNG sửa
// bảng của module khác khi gộp danh tính — việc chuyển đơn hàng thuộc
// module `order`, nghe qua event.
package customer

import (
	"context"
	"errors"
	"time"
)

// API là hợp đồng công khai của module customer.
type API interface {
	// Create tạo hồ sơ khách hàng.
	//
	// Trả ErrDuplicateEmail nếu email đã dùng. Với luồng thanh toán của
	// khách vãng lai, dùng EnsureByEmail thay vì hàm này.
	Create(ctx context.Context, req CreateRequest) (CustomerView, error)

	// EnsureByEmail trả hồ sơ có sẵn hoặc tạo mới.
	//
	// Dùng cho khách vãng lai thanh toán: email đã từng đặt hàng phải vào
	// ĐÚNG hồ sơ cũ, không tạo hồ sơ thứ hai — nếu không, lịch sử mua hàng
	// bị chia ra và không bao giờ gộp lại được.
	//
	// AN TOÀN KHI GỌI SONG SONG: hai request cùng email cùng lúc đều nhận
	// về cùng một hồ sơ.
	EnsureByEmail(ctx context.Context, req CreateRequest) (CustomerView, error)

	GetCustomer(ctx context.Context, customerID string) (CustomerView, error)
	GetCustomerByUserID(ctx context.Context, userID string) (CustomerView, error)

	// GetCustomersByIDs đọc nhiều hồ sơ trong MỘT truy vấn.
	//
	// Tồn tại để tránh N+1: hiển thị danh sách 50 đơn hàng mà gọi
	// GetCustomer 50 lần là 50 lượt đi-về database cho một trang.
	//
	// Id không tồn tại sẽ VẮNG MẶT trong map, không phải giá trị rỗng.
	GetCustomersByIDs(ctx context.Context, customerIDs []string) (map[string]CustomerView, error)

	UpdateProfile(ctx context.Context, customerID, displayName, phone string) (CustomerView, error)

	// LinkUser gắn hồ sơ khách vãng lai với tài khoản vừa đăng ký.
	//
	// GIỮ NGUYÊN customer_id nên toàn bộ đơn hàng cũ tự thuộc về tài khoản
	// mới — không phải chuyển gì cả.
	LinkUser(ctx context.Context, customerID, userID string) error

	// Anonymize gỡ dữ liệu định danh theo yêu cầu xóa tài khoản.
	//
	// GIỮ LẠI dữ liệu giao dịch (số đơn, tổng chi) ở dạng ẩn danh: chúng
	// đã dùng để tính hoa hồng trả cho seller, và nghĩa vụ lưu trữ chứng
	// từ kế toán không cho phép xóa.
	Anonymize(ctx context.Context, customerID string) error

	// ---------------------------------------------------------- Địa chỉ

	// AddAddress thêm địa chỉ vào sổ.
	//
	// Địa chỉ ĐẦU TIÊN luôn thành mặc định, kể cả khi không yêu cầu.
	AddAddress(ctx context.Context, req AddressRequest) (AddressView, error)

	GetAddresses(ctx context.Context, customerID string) ([]AddressView, error)

	// GetDefaultAddress trả địa chỉ mặc định.
	//
	// Trả ErrAddressNotFound nếu khách chưa có địa chỉ nào.
	GetDefaultAddress(ctx context.Context, customerID string) (AddressView, error)

	UpdateAddress(ctx context.Context, addressID string, req AddressRequest) (AddressView, error)

	SetDefaultAddress(ctx context.Context, customerID, addressID string) error

	// DeleteAddress xóa MỀM một địa chỉ.
	//
	// Không tự chọn địa chỉ khác thay thế khi xóa địa chỉ mặc định: chọn
	// hộ là đoán, và đoán sai nghĩa là hàng đi tới địa chỉ khách không
	// muốn.
	DeleteAddress(ctx context.Context, customerID, addressID string) error

	// ---------------------------------------------------------- Đồng ý

	// RecordConsent ghi nhận một lần đồng ý hoặc rút lại.
	//
	// CHỈ GHI THÊM: mỗi lần đổi ý là một bản ghi mới, không sửa bản ghi
	// cũ. Nghĩa vụ pháp lý là chứng minh được khách đã đồng ý VÀO LÚC NÀO
	// và Ở ĐÂU — sửa bản ghi cũ là hủy chính bằng chứng đó.
	RecordConsent(ctx context.Context, req ConsentRequest) error

	// HasConsent cho biết khách CÓ ĐANG đồng ý loại này không.
	//
	// KHÔNG có bản ghi nào nghĩa là CHƯA đồng ý — không phải "chưa từ
	// chối". Gửi thư quảng cáo cho người chưa bao giờ bấm đồng ý là vi
	// phạm pháp luật ở nhiều thị trường.
	HasConsent(ctx context.Context, customerID, consentType string) (bool, error)

	// GetConsentHistory trả toàn bộ lịch sử đồng ý, mới nhất trước.
	//
	// Dùng khi cần chứng minh với cơ quan quản lý.
	GetConsentHistory(ctx context.Context, customerID string) ([]ConsentView, error)

	// ---------------------------------------------------------- Wishlist

	// AddToWishlist thêm một món.
	//
	// IDEMPOTENT: thêm lại món đã có KHÔNG tạo bản sao và KHÔNG báo lỗi.
	// Khách bấm tim hai lần là chuyện thường.
	//
	// Trả về true nếu món thật sự được thêm mới.
	AddToWishlist(ctx context.Context, req WishlistRequest) (bool, error)

	// RemoveFromWishlist bỏ một món. IDEMPOTENT như AddToWishlist.
	RemoveFromWishlist(ctx context.Context, req WishlistRequest) (bool, error)

	// GetWishlist trả danh sách yêu thích.
	//
	// Khách chưa thích gì nhận danh sách RỖNG chứ không phải lỗi.
	GetWishlist(ctx context.Context, customerID string) (WishlistView, error)

	// CountWishlistForProduct đếm số KHÁCH đã thích một sản phẩm.
	//
	// Đếm theo người, không theo dòng: một khách thích cả size M lẫn L vẫn
	// là MỘT người quan tâm.
	//
	// TÍN HIỆU NHU CẦU: nhiều người thích mà chưa mua thường là dấu hiệu
	// giá cao hoặc hết size.
	CountWishlistForProduct(ctx context.Context, productID string) (int, error)

	// ---------------------------------------------------------- Gộp

	// MergeGuestIdentity gộp hồ sơ khách vãng lai vào hồ sơ đã đăng ký.
	//
	// BẮT BUỘC EmailVerified=true. Không xác minh thì bất kỳ ai đăng ký
	// bằng email người khác đều đọc được lịch sử mua hàng của họ, kể cả
	// địa chỉ nhà.
	//
	// Ở MVP hàm này CHỈ ghi nhật ký và ẩn danh hồ sơ nguồn. Chuyển đơn
	// hàng thuộc module `order`.
	MergeGuestIdentity(ctx context.Context, req MergeRequest) error
}

// ---------------------------------------------------------------- DTO

// CreateRequest là dữ liệu tạo hồ sơ.
type CreateRequest struct {
	// Email KHÔNG cần chuẩn hóa trước: module tự hạ chữ thường và cắt
	// khoảng trắng, GIỐNG HỆT module identity — hai module lưu email khác
	// định dạng thì không bao giờ gộp được danh tính.
	Email string

	Phone       string
	DisplayName string

	// UserID để trống với khách vãng lai.
	UserID string

	// Currency của tổng chi tiêu. Bỏ trống = VND.
	Currency string
}

// AddressRequest là dữ liệu thêm hoặc sửa địa chỉ.
type AddressRequest struct {
	CustomerID string

	// Người nhận có thể KHÁC chủ tài khoản: mua tặng, giao tới văn phòng.
	RecipientName  string
	RecipientPhone string

	// Line1 là dòng địa chỉ chính, BẮT BUỘC.
	//
	// Không tách thành số nhà / tên đường: định dạng địa chỉ khác nhau
	// theo từng nước, và tách sai còn tệ hơn không tách.
	Line1    string
	Line2    string
	Ward     string
	District string
	Province string
	Postcode string

	// Country là mã ISO hai chữ cái. Bỏ trống = VN.
	Country string

	Note      string
	IsDefault bool
}

// ConsentRequest là dữ liệu ghi nhận đồng ý.
type ConsentRequest struct {
	CustomerID string

	// Type: MARKETING_EMAIL, MARKETING_SMS, DATA_PROCESSING, PERSONALIZATION.
	Type string

	Granted bool

	// Source là NƠI khách đồng ý: "checkout", "signup_form", "settings".
	//
	// BẮT BUỘC không rỗng: "khách đã đồng ý" mà không nói được ở đâu thì
	// không dùng được làm bằng chứng.
	Source string

	// PolicyVersion là phiên bản điều khoản khách đã đọc.
	//
	// Điều khoản thay đổi theo thời gian. Không lưu phiên bản thì không
	// trả lời được "khách đồng ý với ĐIỀU GÌ".
	PolicyVersion string

	// IP là địa chỉ nguyên văn; module BĂM trước khi lưu.
	IP        string
	UserAgent string
}

// WishlistRequest là dữ liệu thêm hoặc bỏ món yêu thích.
type WishlistRequest struct {
	CustomerID string

	// ProductID, KHÔNG phải VariantID.
	//
	// Khách yêu thích "chiếc áo này", không phải "chiếc áo này size M".
	// Lưu theo variant thì hết size M là món đồ biến mất khỏi danh sách.
	ProductID string

	// VariantID TÙY CHỌN: khi có, đây là tín hiệu nhu cầu mạnh hơn hẳn —
	// nó nói chính xác khách chờ size nào về hàng.
	VariantID string

	Note string
}

// MergeRequest là dữ liệu gộp danh tính.
type MergeRequest struct {
	SourceCustomerID string
	TargetCustomerID string

	// EmailVerified là XÁC NHẬN đã kiểm chứng quyền sở hữu email.
	//
	// KHÔNG phải cờ tiện lợi: truyền true mà chưa gửi thư xác minh là mở
	// đường cho bất kỳ ai đọc lịch sử mua hàng của người khác.
	EmailVerified bool
}

// CustomerView là hồ sơ khách hàng trả ra ngoài.
type CustomerView struct {
	ID string

	// UserID rỗng nghĩa là khách vãng lai.
	UserID string

	Email       string
	Phone       string
	DisplayName string

	// Status: GUEST, REGISTERED, MEMBER, VIP, ANONYMIZED.
	Status string

	OrderCount int

	// TotalSpent là số nguyên đơn vị nhỏ nhất của tiền tệ.
	//
	// KHÔNG dùng số thực cho tiền: 0.1 + 0.2 = 0.30000000000000004, và
	// với hàng triệu giao dịch sai số tích lũy thành tiền thật.
	TotalSpent int64
	Currency   string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// AddressView là một mục trong sổ địa chỉ.
type AddressView struct {
	ID         string
	CustomerID string

	RecipientName  string
	RecipientPhone string

	Line1    string
	Line2    string
	Ward     string
	District string
	Province string
	Postcode string
	Country  string

	Note      string
	IsDefault bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConsentView là một bản ghi trong nhật ký đồng ý.
type ConsentView struct {
	Type          string
	Granted       bool
	Source        string
	PolicyVersion string
	RecordedAt    time.Time
}

// WishlistView là danh sách yêu thích.
type WishlistView struct {
	ID    string
	Name  string
	Items []WishlistItemView
}

// WishlistItemView là một món yêu thích.
type WishlistItemView struct {
	ProductID string
	VariantID string
	Note      string
	AddedAt   time.Time
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound = errors.New("customer: không tìm thấy khách hàng")

	ErrAddressNotFound = errors.New("customer: không tìm thấy địa chỉ")

	// ErrInvalidInput là dữ liệu không hợp lệ: email sai định dạng, địa
	// chỉ thiếu trường bắt buộc, loại đồng ý không tồn tại.
	ErrInvalidInput = errors.New("customer: dữ liệu không hợp lệ")

	ErrDuplicateEmail = errors.New("customer: email đã được dùng")

	// ErrVersionConflict là hồ sơ đã bị thao tác khác sửa.
	//
	// Bên gọi nên ĐỌC LẠI rồi thử lại, không phải thử lại nguyên văn.
	ErrVersionConflict = errors.New("customer: dữ liệu đã bị thay đổi bởi thao tác khác")

	// ErrAnonymized là hồ sơ đã ẩn danh, không sửa được.
	ErrAnonymized = errors.New("customer: hồ sơ đã ẩn danh, không sửa được")

	// ErrMergeNotVerified là gộp danh tính mà chưa xác minh email.
	ErrMergeNotVerified = errors.New("customer: gộp danh tính bắt buộc xác minh email")
)

// ---------------------------------------------------------------- Hằng

// Trạng thái khách hàng.
const (
	StatusGuest      = "GUEST"
	StatusRegistered = "REGISTERED"
	StatusMember     = "MEMBER"
	StatusVIP        = "VIP"
	StatusAnonymized = "ANONYMIZED"
)

// Loại đồng ý.
//
// PHÂN BIỆT MARKETING VỚI DATA_PROCESSING LÀ YÊU CẦU PHÁP LÝ:
//
//	MARKETING_*      bắt buộc có đồng ý trước khi gửi
//	DATA_PROCESSING  cần để xử lý đơn hàng, thu thập lúc đặt hàng
const (
	ConsentMarketingEmail  = "MARKETING_EMAIL"
	ConsentMarketingSMS    = "MARKETING_SMS"
	ConsentDataProcessing  = "DATA_PROCESSING"
	ConsentPersonalization = "PERSONALIZATION"
)
