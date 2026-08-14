// Package identity quản lý tài khoản đăng nhập, phiên và vai trò.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// # Ranh giới: identity KHÔNG phải customer
//
//	identity  trả lời "ai đang gọi, và họ được phép làm gì"
//	customer  trả lời "người này là ai" — hồ sơ, địa chỉ, số đo, sở thích
//
// Gộp hai thứ này là sai lầm phổ biến nhất. Hệ quả khi gộp: bảng tài khoản
// phình ra theo mọi nhu cầu nghiệp vụ mới, và mọi truy vấn xác thực — thứ
// chạy ở MỌI request — phải đọc qua một bảng ngày càng nặng. Xấu hơn nữa,
// nhân viên vận hành cần xem hồ sơ khách sẽ có luôn quyền đọc bảng chứa
// hash mật khẩu.
//
// Cầu nối giữa hai module là `user_id` — customer giữ user_id, identity
// không biết customer tồn tại.
//
// # Điều module này KHÔNG quyết định
//
// Nó KHÔNG kiểm tra quyền cho module khác. Nó trả lời "người này có vai
// trò gì, trên phạm vi nào" (xem AuthContext); module SỞ HỮU DỮ LIỆU tự
// quyết định phạm vi đó nghĩa là gì trong bảng của mình.
//
// Lý do: identity không biết bảng nào tồn tại ở đâu. Nếu nó kiểm tra hộ,
// mỗi lần một module thêm bảng là phải sửa identity — và một lỗi ở đây làm
// hổng quyền của toàn hệ thống.
package identity

import (
	"context"
	"errors"
	"time"
)

// API là hợp đồng công khai của module identity.
type API interface {
	// Register tạo tài khoản mới.
	//
	// Trả ErrDuplicateEmail nếu email đã dùng — đây là NGOẠI LỆ CÓ CHỦ Ý
	// so với nguyên tắc giấu sự tồn tại tài khoản ở Login: người dùng thật
	// cần biết vì sao không đăng ký được. Bù lại, đường đăng ký phải có
	// giới hạn tần suất ở tầng interfaces, nếu không nó thành công cụ dò
	// danh sách email.
	Register(ctx context.Context, req RegisterRequest) (UserView, error)

	// Login xác thực và mở phiên.
	//
	// MỌI lý do thất bại đều trả CÙNG MỘT LỖI ErrInvalidLogin — email
	// không tồn tại, sai mật khẩu, tài khoản bị treo. Phân biệt chúng cho
	// client là để lộ tài khoản nào có thật.
	//
	// Ngoại lệ: ErrAccountLocked, vì người dùng thật cần biết vì sao mật
	// khẩu đúng mà vẫn không vào được.
	Login(ctx context.Context, req LoginRequest) (AuthResult, error)

	// Refresh xoay token và mở phiên mới.
	//
	// Token cũ bị THU HỒI. Nếu nó bị đánh cắp và dùng sau đó, lần dùng
	// thất bại chính là dấu hiệu phát hiện được.
	Refresh(ctx context.Context, req RefreshRequest) (AuthResult, error)

	// Logout thu hồi một phiên.
	//
	// IDEMPOTENT: đăng xuất phiên đã đăng xuất là thành công, vì kết quả
	// mong muốn đã đạt.
	Logout(ctx context.Context, refreshToken string) error

	// Authenticate đổi refresh token lấy ngữ cảnh phân quyền.
	//
	// Đây là hàm các module khác thật sự dùng: cho một token, trả về
	// "ai đang gọi và họ có phạm vi nào".
	Authenticate(ctx context.Context, refreshToken string) (AuthContext, error)

	// GetUser đọc thông tin tài khoản.
	GetUser(ctx context.Context, userID string) (UserView, error)

	// ListSessions trả các phiên đang hoạt động của một người dùng.
	//
	// Cho màn hình "các thiết bị đang đăng nhập" — người dùng tự phát hiện
	// được tài khoản bị chiếm mà không cần chờ nền tảng báo.
	ListSessions(ctx context.Context, userID string) ([]SessionView, error)

	// ChangePassword đổi mật khẩu và THU HỒI MỌI PHIÊN.
	//
	// Thu hồi phiên là phần quan trọng nhất: đổi mật khẩu mà phiên cũ vẫn
	// sống thì kẻ tấn công vẫn vào được, còn người dùng tưởng mình đã an
	// toàn — nguy hiểm hơn là không đổi gì cả.
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error

	// GrantRole cấp vai trò. Gọi lại với cùng tham số KHÔNG tạo bản sao.
	GrantRole(ctx context.Context, userID, role, scopeID string) error

	// RevokeRole thu hồi vai trò.
	RevokeRole(ctx context.Context, userID, role, scopeID string) error

	// Suspend treo tài khoản và thu hồi mọi phiên.
	//
	// Treo mà phiên vẫn sống nghĩa là người bị treo vẫn dùng được hệ thống
	// tới khi token hết hạn — tức là tối đa 30 ngày.
	Suspend(ctx context.Context, userID string) error
}

// ---------------------------------------------------------------- DTO

// RegisterRequest là dữ liệu đăng ký.
type RegisterRequest struct {
	// Email KHÔNG cần chuẩn hóa trước: module tự hạ chữ thường và cắt
	// khoảng trắng. "Khach@Example.com" và "khach@example.com" là MỘT tài
	// khoản.
	Email string

	// Password là mật khẩu NGUYÊN VĂN, chỉ tồn tại trong bộ nhớ.
	//
	// KHÔNG BAO GIỜ ghi trường này ra nhật ký, kể cả khi gỡ lỗi. Nhật ký
	// được chuyển tới hệ thống tập trung, giữ nhiều tháng, và nhiều người
	// đọc được.
	Password string

	Phone       string
	DisplayName string

	// Roles là vai trò cấp ngay lúc đăng ký. Bỏ trống = CUSTOMER.
	//
	// Đường đăng ký công khai PHẢI bỏ trống trường này. Cho phép client
	// tự chọn vai trò là để bất kỳ ai cũng tự cấp mình quyền ADMIN.
	Roles []RoleGrantInput
}

// RoleGrantInput là một vai trò cần cấp.
type RoleGrantInput struct {
	Role string

	// ScopeID là thực thể mà vai trò gắn vào: seller_id với SELLER_*.
	// Rỗng với vai trò không gắn thực thể nào (CUSTOMER, ADMIN).
	ScopeID string
}

// LoginRequest là dữ liệu đăng nhập.
type LoginRequest struct {
	Email    string
	Password string

	UserAgent string

	// IP là địa chỉ nguyên văn; module BĂM trước khi lưu.
	//
	// Lưu IP nguyên văn là dữ liệu cá nhân, cần cơ sở pháp lý. Băm vẫn cho
	// phép phát hiện "nhiều lần thử từ cùng một nguồn".
	IP string
}

// RefreshRequest là dữ liệu làm mới phiên.
type RefreshRequest struct {
	RefreshToken string
	UserAgent    string
	IP           string
}

// AuthResult là kết quả xác thực thành công.
type AuthResult struct {
	User UserView

	// RefreshToken là bản NGUYÊN VĂN, trả cho client MỘT LẦN duy nhất.
	//
	// Database chỉ giữ bản băm, nên rò rỉ database KHÔNG cho phép kẻ tấn
	// công đăng nhập. Đổi lại, mất token là phải đăng nhập lại — không có
	// cách nào lấy lại, và đó là thiết kế đúng.
	RefreshToken string

	SessionID string

	// ExpiresAt là hạn của refresh token (30 ngày).
	ExpiresAt time.Time

	// AccessTokenTTL là thời gian sống khuyến nghị của access token (15
	// phút) để tầng interfaces tự phát hành.
	//
	// Access token NGẮN có chủ ý: nó không tra database mỗi request, nên
	// thu hồi không có hiệu lực tức thì. Mười lăm phút là khoảng thời gian
	// tối đa một tài khoản đã bị treo còn dùng được.
	AccessTokenTTL time.Duration
}

// AuthContext là ngữ cảnh phân quyền của người gọi.
//
// Đây là thứ các module khác nhận được. Chú ý nó KHÔNG chứa email hay tên:
// module `order` không cần biết khách tên gì để lọc đơn của khách đó, và
// dữ liệu không truyền đi là dữ liệu không rò rỉ được.
type AuthContext struct {
	UserID string

	// Roles là danh sách vai trò, ví dụ ["CUSTOMER", "SELLER_OWNER"].
	Roles []string

	// Scope là phạm vi RỘNG NHẤT: OWN, SELLER, hoặc ALL.
	//
	// Module sở hữu dữ liệu dịch phạm vi này sang truy vấn của mình:
	//
	//	OWN     WHERE customer_id = ctx.UserID
	//	SELLER  WHERE seller_id IN ctx.SellerIDs
	//	ALL     không thêm điều kiện
	Scope string

	// SellerIDs là các gian hàng người này có vai trò SELLER_*.
	//
	// PHẢI dùng để giới hạn truy vấn — seller chỉ thấy dữ liệu gian hàng
	// mình. Danh sách rỗng với Scope=SELLER nghĩa là KHÔNG thấy gì, không
	// phải thấy tất cả.
	SellerIDs []string

	SessionID string
}

// UserView là thông tin tài khoản trả ra ngoài.
//
// KHÔNG chứa hash mật khẩu, số lần đăng nhập sai, hay thời điểm khóa. Kiểu
// dữ liệu này đi tới tầng HTTP; thứ gì có ở đây có thể lọt ra ngoài.
type UserView struct {
	ID          string
	Email       string
	Phone       string
	DisplayName string

	// Status: ACTIVE, SUSPENDED, DELETED.
	Status string

	Roles []RoleGrantView

	EmailVerified bool
	CreatedAt     time.Time
}

// RoleGrantView là một vai trò đã cấp.
type RoleGrantView struct {
	Role      string
	ScopeID   string
	GrantedAt time.Time
}

// SessionView là một phiên đang hoạt động.
//
// KHÔNG chứa refresh token hay băm của nó: màn hình "thiết bị đang đăng
// nhập" chỉ cần đủ để người dùng NHẬN RA phiên nào lạ.
type SessionView struct {
	ID         string
	UserAgent  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// ---------------------------------------------------------------- Lỗi

var (
	// ErrNotFound là không tìm thấy tài khoản.
	ErrNotFound = errors.New("identity: không tìm thấy tài khoản")

	// ErrInvalidInput là dữ liệu không hợp lệ (email sai định dạng, vai
	// trò không tồn tại).
	ErrInvalidInput = errors.New("identity: dữ liệu không hợp lệ")

	// ErrWeakPassword là mật khẩu quá ngắn.
	ErrWeakPassword = errors.New("identity: mật khẩu quá ngắn")

	// ErrDuplicateEmail là email đã được dùng.
	ErrDuplicateEmail = errors.New("identity: email đã được dùng")

	// ErrInvalidLogin là đăng nhập thất bại.
	//
	// MỘT lỗi duy nhất cho mọi lý do — xem ghi chú ở API.Login.
	ErrInvalidLogin = errors.New("identity: email hoặc mật khẩu không đúng")

	// ErrAccountLocked là tài khoản tạm khóa do sai mật khẩu nhiều lần.
	ErrAccountLocked = errors.New("identity: tài khoản tạm khóa do đăng nhập sai nhiều lần")

	// ErrAccountSuspended là tài khoản bị treo.
	ErrAccountSuspended = errors.New("identity: tài khoản đã bị khóa")

	// ErrSessionInvalid là phiên không hợp lệ, đã thu hồi, hoặc hết hạn.
	ErrSessionInvalid = errors.New("identity: phiên đăng nhập không hợp lệ hoặc đã hết hạn")
)

// ---------------------------------------------------------------- Hằng

// Vai trò hệ thống.
const (
	RoleCustomer         = "CUSTOMER"
	RoleSellerOwner      = "SELLER_OWNER"
	RoleSellerStaff      = "SELLER_STAFF"
	RoleCreator          = "CREATOR"
	RoleAdmin            = "ADMIN"
	RoleOpsWarehouse     = "OPS_WAREHOUSE"
	RoleOpsMerchandising = "OPS_MERCHANDISING"
	RoleOpsFinance       = "OPS_FINANCE"
	RoleOpsSupport       = "OPS_SUPPORT"
)

// Phạm vi quyền.
const (
	ScopeOwn    = "OWN"
	ScopeSeller = "SELLER"
	ScopeAll    = "ALL"
)

// Trạng thái tài khoản.
const (
	StatusActive    = "ACTIVE"
	StatusSuspended = "SUSPENDED"
	StatusDeleted   = "DELETED"
)
