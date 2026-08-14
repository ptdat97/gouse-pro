// Package postgres cài đặt kho lưu trữ identity bằng PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/identity/domain"
)

// UserStore lưu và đọc tài khoản.
type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

var _ domain.UserRepository = (*UserStore)(nil)

// Save ghi tài khoản MỚI cùng thông tin xác thực trong MỘT giao dịch.
//
// Tài khoản có mà không có mật khẩu là tài khoản không đăng nhập được, và
// người dùng sẽ thử đăng ký lại rồi gặp lỗi "email đã tồn tại" — bế tắc
// mà chỉ quản trị viên gỡ được.
func (s *UserStore) Save(
	ctx context.Context, u *domain.User, c *domain.Credential,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO "user" (
			id, email, phone, display_name, status,
			email_verified_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		u.ID().String(), u.Email(), u.Phone(), u.DisplayName(),
		string(u.Status()), nullTime(u.EmailVerifiedAt()),
		u.CreatedAt(), u.UpdatedAt())
	if err != nil {
		if isUnique(err, "user_email_key") {
			return domain.ErrDuplicateEmail
		}
		return fmt.Errorf("identity: ghi tài khoản: %w", err)
	}

	if c != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_credential (
				user_id, password_hash, password_changed_at,
				failed_attempts, locked_until, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6)`,
			c.UserID().String(), c.PasswordHash(), c.PasswordChangedAt(),
			c.FailedAttempts(), nullTime(c.LockedUntil()), c.UpdatedAt()); err != nil {
			return fmt.Errorf("identity: ghi thông tin xác thực: %w", err)
		}
	}

	if err := s.writeRoles(ctx, tx, u); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("identity: xác nhận giao dịch: %w", err)
	}
	return nil
}

// Update ghi lại thay đổi tài khoản và vai trò.
func (s *UserStore) Update(ctx context.Context, u *domain.User) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("identity: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE "user"
		   SET phone = $2, display_name = $3, status = $4,
		       email_verified_at = $5, updated_at = $6
		 WHERE id = $1`,
		u.ID().String(), u.Phone(), u.DisplayName(), string(u.Status()),
		nullTime(u.EmailVerifiedAt()), u.UpdatedAt())
	if err != nil {
		return fmt.Errorf("identity: cập nhật tài khoản: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	// Vai trò: XÓA HẾT RỒI GHI LẠI.
	//
	// Số vai trò của một người rất nhỏ (thường 1–3), và so từng dòng chỉ
	// thêm nhánh xử lý mà không cứu được gì.
	if _, err := tx.Exec(ctx,
		`DELETE FROM user_role WHERE user_id = $1`, u.ID().String()); err != nil {
		return fmt.Errorf("identity: dọn vai trò cũ: %w", err)
	}
	if err := s.writeRoles(ctx, tx, u); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("identity: xác nhận giao dịch: %w", err)
	}
	return nil
}

func (s *UserStore) writeRoles(ctx context.Context, tx pgx.Tx, u *domain.User) error {
	for _, g := range u.Roles() {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_role (user_id, role, scope_id, granted_at)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT DO NOTHING`,
			u.ID().String(), string(g.Role), g.ScopeID.String(), g.GrantedAt); err != nil {
			return fmt.Errorf("identity: ghi vai trò %s: %w", g.Role, err)
		}
	}
	return nil
}

const userCols = `
	id, email, phone, display_name, status,
	email_verified_at, created_at, updated_at`

func (s *UserStore) FindByID(ctx context.Context, id ids.ID) (*domain.User, error) {
	return s.findOne(ctx, `WHERE id = $1`, id.String())
}

// FindByEmail tra tài khoản theo email ĐÃ CHUẨN HÓA.
//
// Bên gọi phải gọi NormalizeEmail trước — nếu không, "Khach@Example.com"
// sẽ không tìm thấy tài khoản đăng ký bằng "khach@example.com".
func (s *UserStore) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.findOne(ctx, `WHERE email = $1`, email)
}

func (s *UserStore) findOne(
	ctx context.Context, where string, args ...any,
) (*domain.User, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+userCols+` FROM "user" `+where, args...)

	var (
		p        domain.RestoreUserParams
		id       string
		status   string
		verified *time.Time
	)
	err := row.Scan(&id, &p.Email, &p.Phone, &p.DisplayName, &status,
		&verified, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("identity: đọc tài khoản: %w", err)
	}

	p.ID = ids.ID(id)
	p.Status = domain.Status(status)
	if verified != nil {
		p.EmailVerifiedAt = *verified
	}

	roles, err := s.loadRoles(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Roles = roles

	return domain.RestoreUser(p), nil
}

func (s *UserStore) loadRoles(ctx context.Context, userID ids.ID) ([]domain.RoleGrant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT role, scope_id, granted_at
		  FROM user_role WHERE user_id = $1 ORDER BY granted_at`,
		userID.String())
	if err != nil {
		return nil, fmt.Errorf("identity: đọc vai trò: %w", err)
	}
	defer rows.Close()

	var out []domain.RoleGrant
	for rows.Next() {
		var (
			role, scopeID string
			grantedAt     time.Time
		)
		if err := rows.Scan(&role, &scopeID, &grantedAt); err != nil {
			return nil, fmt.Errorf("identity: đọc vai trò: %w", err)
		}
		out = append(out, domain.RoleGrant{
			Role:      domain.Role(role),
			ScopeID:   ids.ID(scopeID),
			GrantedAt: grantedAt,
		})
	}
	return out, rows.Err()
}

// FindCredential đọc thông tin xác thực.
//
// Tách khỏi FindByID có chủ ý: đọc hash mật khẩu phải là hành động CÓ CHỦ
// Ý, không phải tác dụng phụ của việc đọc tên người dùng.
func (s *UserStore) FindCredential(
	ctx context.Context, userID ids.ID,
) (*domain.Credential, error) {
	var (
		p      domain.RestoreCredentialParams
		id     string
		locked *time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, password_hash, password_changed_at,
		       failed_attempts, locked_until, updated_at
		  FROM user_credential WHERE user_id = $1`, userID.String()).
		Scan(&id, &p.PasswordHash, &p.PasswordChangedAt,
			&p.FailedAttempts, &locked, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("identity: đọc thông tin xác thực: %w", err)
	}

	p.UserID = ids.ID(id)
	if locked != nil {
		p.LockedUntil = *locked
	}
	return domain.RestoreCredential(p), nil
}

func (s *UserStore) UpdateCredential(ctx context.Context, c *domain.Credential) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE user_credential
		   SET password_hash = $2, password_changed_at = $3,
		       failed_attempts = $4, locked_until = $5, updated_at = $6
		 WHERE user_id = $1`,
		c.UserID().String(), c.PasswordHash(), c.PasswordChangedAt(),
		c.FailedAttempts(), nullTime(c.LockedUntil()), c.UpdatedAt())
	if err != nil {
		return fmt.Errorf("identity: cập nhật thông tin xác thực: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- Phiên

// SessionStore lưu và đọc phiên đăng nhập.
type SessionStore struct {
	pool *pgxpool.Pool
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool}
}

var _ domain.SessionRepository = (*SessionStore)(nil)

func (s *SessionStore) Save(ctx context.Context, sess *domain.Session) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO session (
			id, user_id, refresh_token_hash, user_agent, ip_hash,
			expires_at, revoked_at, created_at, last_used_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		sess.ID().String(), sess.UserID().String(), sess.RefreshTokenHash(),
		sess.UserAgent(), sess.IPHash(), sess.ExpiresAt(),
		nullTime(sess.RevokedAt()), sess.CreatedAt(), sess.LastUsedAt())
	if err != nil {
		return fmt.Errorf("identity: ghi phiên: %w", err)
	}
	return nil
}

const sessionCols = `
	id, user_id, refresh_token_hash, user_agent, ip_hash,
	expires_at, revoked_at, created_at, last_used_at`

// FindByTokenHash tra phiên theo BĂM của refresh token.
func (s *SessionStore) FindByTokenHash(
	ctx context.Context, tokenHash string,
) (*domain.Session, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+sessionCols+`
		  FROM session WHERE refresh_token_hash = $1`, tokenHash)

	sess, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("identity: đọc phiên: %w", err)
	}
	return sess, nil
}

func (s *SessionStore) Update(ctx context.Context, sess *domain.Session) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE session
		   SET revoked_at = $2, last_used_at = $3, expires_at = $4
		 WHERE id = $1`,
		sess.ID().String(), nullTime(sess.RevokedAt()),
		sess.LastUsedAt(), sess.ExpiresAt())
	if err != nil {
		return fmt.Errorf("identity: cập nhật phiên: %w", err)
	}
	return nil
}

func (s *SessionStore) ListActive(
	ctx context.Context, userID ids.ID, now time.Time,
) ([]*domain.Session, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+sessionCols+`
		  FROM session
		 WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > $2
		 ORDER BY last_used_at DESC`, userID.String(), now)
	if err != nil {
		return nil, fmt.Errorf("identity: đọc phiên: %w", err)
	}
	defer rows.Close()

	var out []*domain.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("identity: đọc phiên: %w", err)
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// RevokeAllForUser thu hồi MỌI phiên của một người dùng.
//
// Một câu UPDATE chứ không phải vòng lặp: đổi mật khẩu là lúc tài khoản có
// thể đang bị tấn công, và mỗi mili-giây trễ là một khoảng thời gian kẻ
// tấn công còn dùng được phiên cũ.
func (s *SessionStore) RevokeAllForUser(
	ctx context.Context, userID ids.ID, now time.Time,
) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE session SET revoked_at = $2
		 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID.String(), now)
	if err != nil {
		return 0, fmt.Errorf("identity: thu hồi phiên: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func scanSession(row interface{ Scan(...any) error }) (*domain.Session, error) {
	var (
		p          domain.RestoreSessionParams
		id, userID string
		revoked    *time.Time
	)
	if err := row.Scan(&id, &userID, &p.RefreshTokenHash, &p.UserAgent,
		&p.IPHash, &p.ExpiresAt, &revoked, &p.CreatedAt, &p.LastUsedAt); err != nil {
		return nil, err
	}

	p.ID = ids.ID(id)
	p.UserID = ids.ID(userID)
	if revoked != nil {
		p.RevokedAt = *revoked
	}
	return domain.RestoreSession(p), nil
}

// ---------------------------------------------------------------- Nhật ký

// LoginAttemptStore ghi nhật ký đăng nhập.
type LoginAttemptStore struct {
	pool *pgxpool.Pool
}

func NewLoginAttemptStore(pool *pgxpool.Pool) *LoginAttemptStore {
	return &LoginAttemptStore{pool: pool}
}

var _ domain.LoginAttemptRepository = (*LoginAttemptStore)(nil)

func (s *LoginAttemptStore) Record(ctx context.Context, a domain.LoginAttempt) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO login_attempt (
			email, user_id, succeeded, failure_reason,
			ip_hash, user_agent, attempted_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		a.Email, a.UserID.String(), a.Succeeded, a.FailureReason,
		a.IPHash, a.UserAgent, a.AttemptedAt)
	if err != nil {
		return fmt.Errorf("identity: ghi nhật ký đăng nhập: %w", err)
	}
	return nil
}

func (s *LoginAttemptStore) CountRecentFailures(
	ctx context.Context, email string, since time.Time,
) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM login_attempt
		 WHERE email = $1 AND succeeded = false AND attempted_at > $2`,
		email, since).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("identity: đếm lần đăng nhập sai: %w", err)
	}
	return n, nil
}

// ---------------------------------------------------------------- Tiện ích

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.ConstraintName == constraint || pgErr.Code == "23505"
}
