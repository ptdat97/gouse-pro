package domain

import (
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// MaxFailedAttempts là số lần đăng nhập sai trước khi khóa tạm.
//
// Năm lần là điểm cân bằng: đủ để người dùng thật gõ nhầm vài lần, đủ chặt
// để việc thử vét cạn trở nên vô nghĩa.
const MaxFailedAttempts = 5

// LockDuration là thời gian khóa tạm sau khi vượt ngưỡng.
//
// KHÓA TẠM chứ không khóa vĩnh viễn: khóa vĩnh viễn biến cơ chế bảo vệ
// thành công cụ tấn công — kẻ xấu chỉ cần thử sai năm lần với email của
// người khác là khóa được tài khoản họ.
const LockDuration = 15 * time.Minute

// Credential là thông tin xác thực của một tài khoản.
//
// TÁCH KHỎI User CÓ CHỦ Ý: truy vấn thông tin người dùng xảy ra ở khắp
// nơi (hiển thị tên, gửi email). Nếu hash nằm cùng bảng, mỗi lần đọc là
// một lần hash đi qua tầng ứng dụng và có thể lọt vào log.
//
// Tách ra khiến việc đọc hash thành hành động CÓ CHỦ Ý.
type Credential struct {
	userID ids.ID

	// passwordHash là chuỗi bcrypt đầy đủ, đã gồm muối và chi phí.
	//
	// KHÔNG BAO GIỜ ghi trường này ra log, không đưa vào thông báo lỗi,
	// không trả về qua API.
	passwordHash string

	passwordChangedAt time.Time

	failedAttempts int
	lockedUntil    time.Time

	updatedAt time.Time
}

// NewCredential tạo thông tin xác thực mới.
func NewCredential(userID ids.ID, passwordHash string, now time.Time) *Credential {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return &Credential{
		userID:            userID,
		passwordHash:      passwordHash,
		passwordChangedAt: now,
		updatedAt:         now,
	}
}

// RestoreCredentialParams dựng lại từ kho lưu trữ.
type RestoreCredentialParams struct {
	UserID            ids.ID
	PasswordHash      string
	PasswordChangedAt time.Time
	FailedAttempts    int
	LockedUntil       time.Time
	UpdatedAt         time.Time
}

// RestoreCredential dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreCredential(p RestoreCredentialParams) *Credential {
	return &Credential{
		userID:            p.UserID,
		passwordHash:      p.PasswordHash,
		passwordChangedAt: p.PasswordChangedAt,
		failedAttempts:    p.FailedAttempts,
		lockedUntil:       p.LockedUntil,
		updatedAt:         p.UpdatedAt,
	}
}

func (c *Credential) UserID() ids.ID               { return c.userID }
func (c *Credential) PasswordHash() string         { return c.passwordHash }
func (c *Credential) PasswordChangedAt() time.Time { return c.passwordChangedAt }
func (c *Credential) FailedAttempts() int          { return c.failedAttempts }
func (c *Credential) LockedUntil() time.Time       { return c.lockedUntil }
func (c *Credential) UpdatedAt() time.Time         { return c.updatedAt }

// IsLocked cho biết tài khoản đang bị khóa tạm không.
func (c *Credential) IsLocked(now time.Time) bool {
	return !c.lockedUntil.IsZero() && now.Before(c.lockedUntil)
}

// RecordFailure ghi nhận một lần đăng nhập sai.
//
// Vượt ngưỡng thì khóa TẠM. Trả về true nếu lần này làm khóa tài khoản —
// bên gọi dùng nó để ghi cảnh báo.
func (c *Credential) RecordFailure(now time.Time) bool {
	c.failedAttempts++
	c.updatedAt = now

	if c.failedAttempts >= MaxFailedAttempts {
		c.lockedUntil = now.Add(LockDuration)
		return true
	}
	return false
}

// RecordSuccess xóa bộ đếm sau khi đăng nhập thành công.
func (c *Credential) RecordSuccess(now time.Time) {
	c.failedAttempts = 0
	c.lockedUntil = time.Time{}
	c.updatedAt = now
}

// ChangePassword đổi mật khẩu.
//
// Bên gọi PHẢI thu hồi mọi phiên đang mở sau đó: nếu tài khoản bị lộ, đổi
// mật khẩu mà phiên cũ vẫn sống thì kẻ tấn công vẫn vào được — và người
// dùng tưởng mình đã an toàn.
func (c *Credential) ChangePassword(newHash string, now time.Time) {
	c.passwordHash = newHash
	c.passwordChangedAt = now
	c.failedAttempts = 0
	c.lockedUntil = time.Time{}
	c.updatedAt = now
}
