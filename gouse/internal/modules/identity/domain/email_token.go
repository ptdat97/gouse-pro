package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	// ErrTokenKhongHopLe: token không tồn tại, đã dùng, hoặc đã hết hạn.
	//
	// MỘT lỗi cho cả ba, có chủ ý: phân biệt "không tồn tại" với "đã hết
	// hạn" cho kẻ dò biết token nào TỪNG có thật, và từ đó biết email nào
	// đang chờ xác minh.
	ErrTokenKhongHopLe = errors.New("identity: token xác minh không hợp lệ hoặc đã hết hạn")

	// ErrEmailDaDoi: email của tài khoản đã đổi kể từ khi phát token.
	ErrEmailDaDoi = errors.New("identity: email đã thay đổi kể từ khi gửi liên kết xác minh")
)

// HanTokenXacMinh là thời gian sống của một token xác minh email.
//
// 24 giờ: đủ rộng cho người nhận thư ở múi giờ khác hoặc mở hộp thư hôm
// sau, đủ hẹp để một hộp thư bị chiếm sau đó không xác minh được bằng thư
// cũ. Ngắn hơn (như token đặt lại mật khẩu) là phiền mà không an toàn hơn
// đáng kể — token này không cấp quyền đăng nhập, nó chỉ chứng minh quyền
// sở hữu hộp thư.
const HanTokenXacMinh = 24 * time.Hour

// EmailToken là một lần phát liên kết xác minh email.
//
// # Vì sao lưu EMAIL tại thời điểm phát
//
// Người dùng đổi email giữa chừng thì token cũ phải chết. Không giữ email
// thì một token gửi tới địa chỉ CŨ lại xác minh được địa chỉ MỚI — tức là
// bỏ qua chính bước đang làm.
type EmailToken struct {
	id     ids.ID
	userID ids.ID

	// hash là băm của token; bản nguyên văn chỉ tồn tại trong thư gửi đi.
	hash  string
	email string

	expiresAt time.Time
	usedAt    time.Time
	createdAt time.Time
}

// NewEmailTokenParams là dữ liệu phát một token xác minh.
type NewEmailTokenParams struct {
	UserID ids.ID
	Hash   string
	Email  string
	Now    time.Time
}

// NewEmailToken phát một token xác minh email.
func NewEmailToken(p NewEmailTokenParams) (*EmailToken, error) {
	if p.UserID.IsZero() {
		return nil, errors.New("identity: token xác minh phải thuộc một tài khoản")
	}
	if strings.TrimSpace(p.Hash) == "" {
		return nil, errors.New("identity: thiếu băm token")
	}
	if strings.TrimSpace(p.Email) == "" {
		return nil, errors.New("identity: thiếu email")
	}

	id, err := ids.New(ids.PrefixEmailToken)
	if err != nil {
		return nil, err
	}
	return &EmailToken{
		id:        id,
		userID:    p.UserID,
		hash:      p.Hash,
		email:     NormalizeEmail(p.Email),
		expiresAt: p.Now.Add(HanTokenXacMinh),
		createdAt: p.Now,
	}, nil
}

// Dung đánh dấu token ĐÃ dùng.
//
// # Vì sao kiểm cả ba điều kiện ở đây
//
// Hết hạn, đã dùng, và email đã đổi — cả ba đều làm token vô hiệu, và cả
// ba đều phải chặn ở DOMAIN chứ không ở truy vấn SQL. Một câu `WHERE
// used_at IS NULL AND expires_at > now()` trông đủ, nhưng nó đặt quy tắc
// vào chỗ mà lần viết truy vấn tiếp theo không nhìn thấy.
func (t *EmailToken) Dung(emailHienTai string, now time.Time) error {
	if !t.usedAt.IsZero() {
		return ErrTokenKhongHopLe
	}
	if !now.Before(t.expiresAt) {
		return ErrTokenKhongHopLe
	}
	if NormalizeEmail(emailHienTai) != t.email {
		return ErrEmailDaDoi
	}
	t.usedAt = now
	return nil
}

func (t *EmailToken) ID() ids.ID           { return t.id }
func (t *EmailToken) UserID() ids.ID       { return t.userID }
func (t *EmailToken) Hash() string         { return t.hash }
func (t *EmailToken) Email() string        { return t.email }
func (t *EmailToken) ExpiresAt() time.Time { return t.expiresAt }
func (t *EmailToken) UsedAt() time.Time    { return t.usedAt }
func (t *EmailToken) CreatedAt() time.Time { return t.createdAt }

// RestoreEmailTokenParams dựng lại token từ database.
type RestoreEmailTokenParams struct {
	ID        ids.ID
	UserID    ids.ID
	Hash      string
	Email     string
	ExpiresAt time.Time
	UsedAt    time.Time
	CreatedAt time.Time
}

// RestoreEmailToken dựng lại từ database, KHÔNG kiểm quy tắc nghiệp vụ.
func RestoreEmailToken(p RestoreEmailTokenParams) *EmailToken {
	return &EmailToken{
		id: p.ID, userID: p.UserID, hash: p.Hash, email: p.Email,
		expiresAt: p.ExpiresAt, usedAt: p.UsedAt, createdAt: p.CreatedAt,
	}
}
