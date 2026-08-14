package domain

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// UserRepository là PORT cho kho lưu trữ tài khoản.
type UserRepository interface {
	// Save ghi tài khoản MỚI cùng thông tin xác thực trong MỘT giao dịch.
	//
	// Tài khoản có mà không có mật khẩu là tài khoản không đăng nhập được,
	// và người dùng sẽ thử đăng ký lại rồi gặp lỗi "email đã tồn tại".
	//
	// Trả ErrDuplicateEmail nếu email đã dùng.
	Save(ctx context.Context, u *User, c *Credential) error

	Update(ctx context.Context, u *User) error

	FindByID(ctx context.Context, id ids.ID) (*User, error)

	// FindByEmail tra tài khoản theo email ĐÃ CHUẨN HÓA.
	//
	// Bên gọi phải gọi NormalizeEmail trước — nếu không, "Khach@Example.com"
	// sẽ không tìm thấy tài khoản đăng ký bằng "khach@example.com".
	FindByEmail(ctx context.Context, email string) (*User, error)

	// FindCredential đọc thông tin xác thực.
	//
	// Tách khỏi FindByID có chủ ý: đọc hash mật khẩu phải là hành động CÓ
	// CHỦ Ý, không phải tác dụng phụ của việc đọc tên người dùng.
	FindCredential(ctx context.Context, userID ids.ID) (*Credential, error)

	UpdateCredential(ctx context.Context, c *Credential) error
}

// SessionRepository là PORT cho phiên đăng nhập.
type SessionRepository interface {
	Save(ctx context.Context, s *Session) error

	// FindByTokenHash tra phiên theo BĂM của refresh token.
	//
	// Nhận hash chứ không nhận token: token nguyên văn không được đi vào
	// tầng lưu trữ, nơi nó có thể lọt vào log truy vấn chậm.
	FindByTokenHash(ctx context.Context, tokenHash string) (*Session, error)

	Update(ctx context.Context, s *Session) error

	// ListActive trả các phiên còn hiệu lực của một người dùng.
	//
	// Dùng cho màn hình "các thiết bị đang đăng nhập" — người dùng tự
	// kiểm soát được tài khoản của mình.
	ListActive(ctx context.Context, userID ids.ID, now time.Time) ([]*Session, error)

	// RevokeAllForUser thu hồi MỌI phiên của một người dùng.
	//
	// Gọi khi đổi mật khẩu hoặc khi phát hiện tài khoản bị lộ. Đổi mật
	// khẩu mà phiên cũ vẫn sống nghĩa là kẻ tấn công vẫn vào được, và
	// người dùng tưởng mình đã an toàn.
	RevokeAllForUser(ctx context.Context, userID ids.ID, now time.Time) (int, error)
}

// LoginAttemptRepository ghi nhật ký đăng nhập.
//
// Ghi CẢ THÀNH CÔNG LẪN THẤT BẠI. Chỉ ghi thất bại thì không phát hiện
// được "đăng nhập thành công từ một quốc gia lạ lúc 3 giờ sáng" — loại
// bất thường nguy hiểm nhất.
type LoginAttemptRepository interface {
	Record(ctx context.Context, a LoginAttempt) error

	// CountRecentFailures đếm số lần thất bại gần đây của một email.
	//
	// Dùng cho giới hạn tần suất ở tầng trước khi chạm tới mật khẩu.
	CountRecentFailures(ctx context.Context, email string, since time.Time) (int, error)
}

// LoginAttempt là một lần thử đăng nhập.
type LoginAttempt struct {
	// Email chứ không phải UserID: lần thử với email không tồn tại cũng
	// phải ghi, vì đó là dấu hiệu dò tài khoản.
	Email  string
	UserID ids.ID

	Succeeded bool

	// FailureReason chỉ dùng cho ĐIỀU TRA, KHÔNG trả về cho client.
	//
	// Trả "sai mật khẩu" hay "email không tồn tại" cho client là để lộ tài
	// khoản nào có thật — kẻ tấn công dùng nó để thu hẹp danh sách.
	FailureReason string

	// IPHash đã băm: lưu IP nguyên văn là dữ liệu cá nhân, cần cơ sở pháp lý.
	IPHash    string
	UserAgent string

	AttemptedAt time.Time
}

// PasswordHasher là PORT cho việc băm mật khẩu.
//
// Là interface vì thuật toán băm SẼ đổi: bcrypt hôm nay, argon2 hoặc thứ
// khác trong năm năm nữa. Đổi thuật toán là viết một cài đặt mới, không
// phải sửa module này.
type PasswordHasher interface {
	// Hash băm mật khẩu, trả về chuỗi đã gồm muối và tham số.
	Hash(password string) (string, error)

	// Verify kiểm tra mật khẩu khớp với hash.
	//
	// Trả về false chứ không phải lỗi khi không khớp: sai mật khẩu là
	// đường đi bình thường, không phải sự cố.
	Verify(hash, password string) bool
}

// TokenGenerator sinh token ngẫu nhiên.
//
// Là PORT vì nó cần nguồn ngẫu nhiên mã hóa an toàn — thứ tầng domain
// không được biết đến.
type TokenGenerator interface {
	// NewToken sinh một token và trả về cả bản nguyên văn lẫn bản băm.
	//
	// Nguyên văn trả cho client MỘT LẦN duy nhất; bản băm lưu vào database.
	NewToken() (plain, hashed string, err error)

	// HashToken băm một token có sẵn, để tra cứu.
	HashToken(plain string) string
}
