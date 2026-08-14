package domain

import (
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// AccessTokenTTL là thời hạn access token.
//
// NGẮN có chủ ý. Access token không thu hồi được (nó tự chứa, không tra
// database mỗi request), nên cách duy nhất giới hạn thiệt hại khi bị lộ là
// để nó hết hạn nhanh.
const AccessTokenTTL = 15 * time.Minute

// RefreshTokenTTL là thời hạn refresh token.
//
// DÀI vì nó THU HỒI ĐƯỢC: mỗi refresh token là một hàng trong bảng
// session, và thu hồi chỉ là đánh dấu hàng đó. Đánh đổi ngược với access
// token — thời hạn dài chấp nhận được vì có nút tắt.
const RefreshTokenTTL = 30 * 24 * time.Hour

// Session là một phiên đăng nhập.
//
// VÌ SAO REFRESH TOKEN LÀ BẢN GHI TRONG DATABASE, không phải JWT tự chứa:
// khi tài khoản bị lộ, người dùng cần đăng xuất mọi thiết bị NGAY. Token
// tự chứa không làm được điều đó — nó có hiệu lực tới khi hết hạn, bất kể
// chuyện gì xảy ra.
type Session struct {
	id     ids.ID
	userID ids.ID

	// refreshTokenHash: BĂM chứ không lưu nguyên văn.
	//
	// Rò rỉ database mà token lưu nguyên văn nghĩa là kẻ tấn công đăng
	// nhập được vào MỌI tài khoản mà không cần mật khẩu — tệ hơn cả rò rỉ
	// hash mật khẩu, vì token dùng được ngay.
	refreshTokenHash string

	// Ngữ cảnh để người dùng nhận ra phiên nào là của thiết bị nào khi xem
	// danh sách "các phiên đang đăng nhập".
	userAgent string
	ipHash    string

	expiresAt time.Time
	revokedAt time.Time

	createdAt  time.Time
	lastUsedAt time.Time
}

type NewSessionParams struct {
	UserID           ids.ID
	RefreshTokenHash string
	UserAgent        string
	IPHash           string
	TTL              time.Duration
	Now              time.Time
}

// NewSession mở một phiên đăng nhập.
func NewSession(p NewSessionParams) (*Session, error) {
	id, err := ids.New(ids.PrefixSession)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ttl := p.TTL
	if ttl <= 0 {
		ttl = RefreshTokenTTL
	}

	return &Session{
		id:               id,
		userID:           p.UserID,
		refreshTokenHash: p.RefreshTokenHash,
		userAgent:        p.UserAgent,
		ipHash:           p.IPHash,
		expiresAt:        now.Add(ttl),
		createdAt:        now,
		lastUsedAt:       now,
	}, nil
}

// RestoreSessionParams dựng lại từ kho lưu trữ.
type RestoreSessionParams struct {
	ID               ids.ID
	UserID           ids.ID
	RefreshTokenHash string
	UserAgent        string
	IPHash           string
	ExpiresAt        time.Time
	RevokedAt        time.Time
	CreatedAt        time.Time
	LastUsedAt       time.Time
}

// RestoreSession dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreSession(p RestoreSessionParams) *Session {
	return &Session{
		id:               p.ID,
		userID:           p.UserID,
		refreshTokenHash: p.RefreshTokenHash,
		userAgent:        p.UserAgent,
		ipHash:           p.IPHash,
		expiresAt:        p.ExpiresAt,
		revokedAt:        p.RevokedAt,
		createdAt:        p.CreatedAt,
		lastUsedAt:       p.LastUsedAt,
	}
}

func (s *Session) ID() ids.ID               { return s.id }
func (s *Session) UserID() ids.ID           { return s.userID }
func (s *Session) RefreshTokenHash() string { return s.refreshTokenHash }
func (s *Session) UserAgent() string        { return s.userAgent }
func (s *Session) IPHash() string           { return s.ipHash }
func (s *Session) ExpiresAt() time.Time     { return s.expiresAt }
func (s *Session) RevokedAt() time.Time     { return s.revokedAt }
func (s *Session) CreatedAt() time.Time     { return s.createdAt }
func (s *Session) LastUsedAt() time.Time    { return s.lastUsedAt }

// IsValid cho biết phiên còn dùng được không.
//
// Kiểm tra CẢ hai điều kiện: chưa thu hồi VÀ chưa hết hạn. Chỉ kiểm tra
// một cái là để lọt một trong hai đường tấn công.
func (s *Session) IsValid(now time.Time) bool {
	return s.revokedAt.IsZero() && now.Before(s.expiresAt)
}

// Revoke thu hồi phiên.
//
// KHÔNG xóa hàng: cần biết phiên bị thu hồi lúc nào khi điều tra sự cố.
func (s *Session) Revoke(now time.Time) {
	if s.revokedAt.IsZero() {
		s.revokedAt = now
	}
}

// Touch cập nhật thời điểm dùng gần nhất.
//
// Cho phép người dùng nhận ra phiên nào đang hoạt động khi xem danh sách
// thiết bị — phiên không dùng ba tháng là phiên đáng ngờ.
func (s *Session) Touch(now time.Time) {
	s.lastUsedAt = now
}
