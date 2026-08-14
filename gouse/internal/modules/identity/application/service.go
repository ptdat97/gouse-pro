// Package application chứa các use case của module identity.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/identity/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// Service là tầng application của module identity.
type Service struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	attempts domain.LoginAttemptRepository
	hasher   domain.PasswordHasher
	tokens   domain.TokenGenerator
	clock    Clock
}

type Deps struct {
	Users    domain.UserRepository
	Sessions domain.SessionRepository
	Attempts domain.LoginAttemptRepository
	Hasher   domain.PasswordHasher
	Tokens   domain.TokenGenerator
	Clock    Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		users:    d.Users,
		sessions: d.Sessions,
		attempts: d.Attempts,
		hasher:   d.Hasher,
		tokens:   d.Tokens,
		clock:    clock,
	}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Đăng ký

// RegisterInput là dữ liệu đăng ký tài khoản.
type RegisterInput struct {
	Email       string
	Password    string
	Phone       string
	DisplayName string

	// Roles là vai trò cấp ngay lúc đăng ký. Bỏ trống = CUSTOMER.
	Roles []domain.RoleGrant
}

// Register tạo tài khoản mới.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	if len(in.Password) < domain.MinPasswordLength {
		return nil, domain.ErrWeakPassword
	}

	now := s.clock.Now()

	u, err := domain.NewUser(domain.NewUserParams{
		Email:       in.Email,
		Phone:       in.Phone,
		DisplayName: in.DisplayName,
		Now:         now,
	})
	if err != nil {
		return nil, err
	}

	if len(in.Roles) == 0 {
		u.GrantRole(domain.RoleCustomer, "", now)
	} else {
		for _, g := range in.Roles {
			u.GrantRole(g.Role, g.ScopeID, now)
		}
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, err
	}

	cred := domain.NewCredential(u.ID(), hash, now)

	if err := s.users.Save(ctx, u, cred); err != nil {
		return nil, err
	}
	return u, nil
}

// ---------------------------------------------------------------- Đăng nhập

// LoginInput là dữ liệu đăng nhập.
type LoginInput struct {
	Email    string
	Password string

	UserAgent string
	IPHash    string
}

// LoginResult là kết quả đăng nhập thành công.
type LoginResult struct {
	User *domain.User

	// RefreshToken là bản NGUYÊN VĂN, trả cho client MỘT LẦN duy nhất.
	//
	// Database chỉ giữ bản băm. Client mất token thì phải đăng nhập lại —
	// không có cách nào lấy lại, và đó là thiết kế đúng.
	RefreshToken string

	SessionID ids.ID
	ExpiresAt time.Time
}

// Login xác thực và mở phiên.
//
// # Mọi đường thất bại đều trả CÙNG MỘT LỖI
//
// Email không tồn tại, sai mật khẩu, tài khoản bị khóa — tất cả trả
// ErrInvalidLogin. Phân biệt chúng cho client là để lộ tài khoản nào có
// thật, và kẻ tấn công dùng nó để thu hẹp danh sách trước khi dò mật khẩu.
//
// Lý do thật được ghi vào NHẬT KÝ để điều tra.
//
// # Ngoại lệ có chủ ý: tài khoản bị khóa tạm
//
// ErrAccountLocked được trả về, vì người dùng thật cần biết vì sao mật
// khẩu đúng mà vẫn không vào được — không thì họ sẽ thử lại liên tục và
// kéo dài thời gian khóa.
func (s *Service) Login(ctx context.Context, in LoginInput) (*LoginResult, error) {
	now := s.clock.Now()
	email := domain.NormalizeEmail(in.Email)

	attempt := domain.LoginAttempt{
		Email:       email,
		IPHash:      in.IPHash,
		UserAgent:   in.UserAgent,
		AttemptedAt: now,
	}

	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Ghi nhật ký với lý do thật, trả lỗi chung cho client.
			attempt.FailureReason = "email không tồn tại"
			_ = s.attempts.Record(ctx, attempt)

			// TỐN THỜI GIAN NHƯ KHI CÓ TÀI KHOẢN THẬT.
			//
			// Không băm gì cả thì phản hồi nhanh hơn hẳn, và kẻ tấn công
			// đo thời gian là biết email nào tồn tại — đúng thứ việc trả
			// lỗi chung sinh ra để giấu.
			s.hasher.Verify(dummyHash, in.Password)

			return nil, domain.ErrInvalidLogin
		}
		return nil, err
	}

	attempt.UserID = u.ID()

	if !u.CanLogin() {
		attempt.FailureReason = "tài khoản " + string(u.Status())
		_ = s.attempts.Record(ctx, attempt)
		return nil, domain.ErrInvalidLogin
	}

	cred, err := s.users.FindCredential(ctx, u.ID())
	if err != nil {
		return nil, err
	}

	if cred.IsLocked(now) {
		attempt.FailureReason = "tài khoản đang bị khóa tạm"
		_ = s.attempts.Record(ctx, attempt)
		return nil, domain.ErrAccountLocked
	}

	if !s.hasher.Verify(cred.PasswordHash(), in.Password) {
		locked := cred.RecordFailure(now)
		if err := s.users.UpdateCredential(ctx, cred); err != nil {
			return nil, err
		}

		attempt.FailureReason = "sai mật khẩu"
		if locked {
			attempt.FailureReason = "sai mật khẩu — tài khoản bị khóa tạm"
		}
		_ = s.attempts.Record(ctx, attempt)

		return nil, domain.ErrInvalidLogin
	}

	cred.RecordSuccess(now)
	if err := s.users.UpdateCredential(ctx, cred); err != nil {
		return nil, err
	}

	plain, hashed, err := s.tokens.NewToken()
	if err != nil {
		return nil, err
	}

	sess, err := domain.NewSession(domain.NewSessionParams{
		UserID:           u.ID(),
		RefreshTokenHash: hashed,
		UserAgent:        in.UserAgent,
		IPHash:           in.IPHash,
		Now:              now,
	})
	if err != nil {
		return nil, err
	}
	if err := s.sessions.Save(ctx, sess); err != nil {
		return nil, err
	}

	attempt.Succeeded = true
	_ = s.attempts.Record(ctx, attempt)

	return &LoginResult{
		User:         u,
		RefreshToken: plain,
		SessionID:    sess.ID(),
		ExpiresAt:    sess.ExpiresAt(),
	}, nil
}

// dummyHash là hash của một mật khẩu không ai dùng.
//
// Dùng để tốn thời gian tương đương khi email không tồn tại — xem ghi chú
// ở Login. Đây là hash bcrypt hợp lệ, nên Verify sẽ chạy đủ số vòng.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// ---------------------------------------------------------------- Phiên

// Refresh làm mới phiên đăng nhập.
//
// XOAY TOKEN (rotation): mỗi lần làm mới sinh token MỚI và thu hồi token
// cũ. Nếu token cũ bị đánh cắp và dùng sau đó, nó đã vô hiệu — và lần dùng
// thất bại đó là dấu hiệu phát hiện được.
func (s *Service) Refresh(
	ctx context.Context, refreshToken, userAgent, ipHash string,
) (*LoginResult, error) {
	now := s.clock.Now()

	sess, err := s.sessions.FindByTokenHash(ctx, s.tokens.HashToken(refreshToken))
	if err != nil {
		return nil, err
	}
	if !sess.IsValid(now) {
		return nil, domain.ErrSessionInvalid
	}

	u, err := s.users.FindByID(ctx, sess.UserID())
	if err != nil {
		return nil, err
	}
	if !u.CanLogin() {
		return nil, domain.ErrAccountSuspended
	}

	// Thu hồi phiên cũ TRƯỚC khi tạo phiên mới.
	//
	// Ngược lại sẽ có khoảng thời gian hai token cùng hợp lệ — và nếu bước
	// thu hồi thất bại, token cũ sống tiếp tới khi hết hạn.
	sess.Revoke(now)
	if err := s.sessions.Update(ctx, sess); err != nil {
		return nil, err
	}

	plain, hashed, err := s.tokens.NewToken()
	if err != nil {
		return nil, err
	}

	fresh, err := domain.NewSession(domain.NewSessionParams{
		UserID:           u.ID(),
		RefreshTokenHash: hashed,
		UserAgent:        userAgent,
		IPHash:           ipHash,
		Now:              now,
	})
	if err != nil {
		return nil, err
	}
	if err := s.sessions.Save(ctx, fresh); err != nil {
		return nil, err
	}

	return &LoginResult{
		User:         u,
		RefreshToken: plain,
		SessionID:    fresh.ID(),
		ExpiresAt:    fresh.ExpiresAt(),
	}, nil
}

// Authenticate đổi refresh token lấy người dùng và phiên.
//
// KHÔNG xoay token: đây là đường đọc, chạy ở mỗi request cần biết người
// gọi là ai. Xoay token ở đây sẽ khiến hai request song song của cùng một
// client đá nhau — request thứ hai mang token vừa bị thu hồi.
//
// Có cập nhật last_used_at để danh sách "thiết bị đang đăng nhập" phản ánh
// hoạt động thật.
func (s *Service) Authenticate(
	ctx context.Context, refreshToken string,
) (*domain.User, *domain.Session, error) {
	now := s.clock.Now()

	sess, err := s.sessions.FindByTokenHash(ctx, s.tokens.HashToken(refreshToken))
	if err != nil {
		return nil, nil, err
	}
	if !sess.IsValid(now) {
		return nil, nil, domain.ErrSessionInvalid
	}

	u, err := s.users.FindByID(ctx, sess.UserID())
	if err != nil {
		return nil, nil, err
	}

	// Tài khoản bị treo SAU khi phiên được mở vẫn còn token hợp lệ trong
	// tay. Kiểm tra ở đây là chốt chặn cuối: không có nó, treo tài khoản
	// chỉ có hiệu lực khi token hết hạn.
	if !u.CanLogin() {
		return nil, nil, domain.ErrAccountSuspended
	}

	sess.Touch(now)
	if err := s.sessions.Update(ctx, sess); err != nil {
		return nil, nil, err
	}

	return u, sess, nil
}

// Logout thu hồi một phiên.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	sess, err := s.sessions.FindByTokenHash(ctx, s.tokens.HashToken(refreshToken))
	if err != nil {
		if errors.Is(err, domain.ErrSessionInvalid) {
			// Đăng xuất một phiên không tồn tại là THÀNH CÔNG: kết quả
			// mong muốn (phiên đó không dùng được) đã đạt.
			return nil
		}
		return err
	}

	sess.Revoke(s.clock.Now())
	return s.sessions.Update(ctx, sess)
}

// ListSessions trả các phiên đang hoạt động.
//
// Cho màn hình "các thiết bị đang đăng nhập" — người dùng tự kiểm soát
// được tài khoản của mình.
func (s *Service) ListSessions(
	ctx context.Context, userID ids.ID,
) ([]*domain.Session, error) {
	return s.sessions.ListActive(ctx, userID, s.clock.Now())
}

// ---------------------------------------------------------------- Tài khoản

func (s *Service) GetUser(ctx context.Context, id ids.ID) (*domain.User, error) {
	return s.users.FindByID(ctx, id)
}

func (s *Service) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.users.FindByEmail(ctx, domain.NormalizeEmail(email))
}

// ChangePassword đổi mật khẩu và THU HỒI MỌI PHIÊN.
//
// Thu hồi phiên là phần quan trọng nhất: nếu tài khoản bị lộ, đổi mật khẩu
// mà phiên cũ vẫn sống thì kẻ tấn công vẫn vào được — và người dùng tưởng
// mình đã an toàn.
func (s *Service) ChangePassword(
	ctx context.Context, userID ids.ID, oldPassword, newPassword string,
) error {
	if len(newPassword) < domain.MinPasswordLength {
		return domain.ErrWeakPassword
	}

	cred, err := s.users.FindCredential(ctx, userID)
	if err != nil {
		return err
	}
	if !s.hasher.Verify(cred.PasswordHash(), oldPassword) {
		return domain.ErrInvalidLogin
	}

	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	cred.ChangePassword(hash, now)
	if err := s.users.UpdateCredential(ctx, cred); err != nil {
		return err
	}

	_, err = s.sessions.RevokeAllForUser(ctx, userID, now)
	return err
}

// GrantRole cấp vai trò cho người dùng.
func (s *Service) GrantRole(
	ctx context.Context, userID ids.ID, role domain.Role, scopeID ids.ID,
) error {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	u.GrantRole(role, scopeID, s.clock.Now())
	return s.users.Update(ctx, u)
}

// RevokeRole thu hồi vai trò.
func (s *Service) RevokeRole(
	ctx context.Context, userID ids.ID, role domain.Role, scopeID ids.ID,
) error {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	u.RevokeRole(role, scopeID, s.clock.Now())
	return s.users.Update(ctx, u)
}

// Suspend khóa tài khoản và thu hồi mọi phiên.
func (s *Service) Suspend(ctx context.Context, userID ids.ID) error {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	u.Suspend(now)
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}

	// Khóa tài khoản mà phiên vẫn sống nghĩa là người bị khóa vẫn dùng
	// được hệ thống tới khi token hết hạn.
	_, err = s.sessions.RevokeAllForUser(ctx, userID, now)
	return err
}
