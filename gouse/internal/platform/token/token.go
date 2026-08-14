// Package token phát hành và xác minh access token dạng JWT.
//
// Đây là hạ tầng TRUNG LẬP VỚI DOMAIN: nó không biết "vai trò ADMIN" nghĩa
// là gì, chỉ mang danh sách chuỗi từ nơi phát hành tới nơi xác minh (quy
// tắc R3 của archcheck).
//
// # Vì sao access token tự chứa, còn refresh token là bản ghi database
//
// Hai loại token có đánh đổi ngược nhau, và đó là chủ ý:
//
//	Access token   tự chứa, KHÔNG thu hồi được → bù lại thời hạn NGẮN (15 phút)
//	Refresh token  bản ghi database, thu hồi ĐƯỢC → nên thời hạn dài (30 ngày)
//
// Access token không tra database mỗi request, nên một tài khoản đã bị treo
// vẫn dùng được tối đa 15 phút. Đó là cái giá đã cân nhắc để mọi request
// không phải chịu thêm một truy vấn. Xem
// internal/modules/identity/domain/session.go.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Lỗi của package.
var (
	// ErrInvalidToken là token sai chữ ký, sai định dạng, hoặc dùng thuật
	// toán không được chấp nhận.
	ErrInvalidToken = errors.New("token: token không hợp lệ")

	// ErrExpired là token đã hết hạn.
	//
	// TÁCH RIÊNG khỏi ErrInvalidToken để tầng gọi ghi log phân biệt được
	// "phiên hết hạn bình thường" với "có người thử token giả". KHÔNG được
	// phân biệt hai lỗi này cho client.
	ErrExpired = errors.New("token: token đã hết hạn")
)

// signingMethod là thuật toán DUY NHẤT được chấp nhận.
//
// Khóa cứng một thuật toán là biện pháp chống tấn công thay đổi thuật toán:
// kẻ tấn công sửa header thành {"alg":"none"} rồi bỏ chữ ký, và bộ xác minh
// nào tin vào header sẽ chấp nhận token tự chế. Ở đây header không được tin.
var signingMethod = jwt.SigningMethodHS256

// Claims là nội dung access token.
//
// KHÔNG chứa email hay tên: token đi qua nhiều tầng và nằm trong log của
// trình duyệt; dữ liệu không đặt vào là dữ liệu không rò rỉ được. Module
// cần thông tin cá nhân thì tự tra theo UserID.
type Claims struct {
	UserID string
	Roles  []string

	// Scope là phạm vi rộng nhất: OWN, SELLER, hoặc ALL.
	Scope string

	// SellerIDs là các gian hàng người dùng có vai trò SELLER_*.
	SellerIDs []string

	SessionID string
}

// jwtClaims là dạng tuần tự hóa của Claims.
//
// Tên trường ngắn có chủ ý — token đi kèm MỌI request, nên mỗi byte thừa
// nhân với số request.
type jwtClaims struct {
	Roles     []string `json:"roles"`
	Scope     string   `json:"scope,omitempty"`
	SellerIDs []string `json:"sids,omitempty"`
	SessionID string   `json:"sid"`
	jwt.RegisteredClaims
}

// Issuer phát hành và xác minh access token.
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

// Config cấu hình Issuer.
type Config struct {
	// Secret là khóa bí mật để ký. Tối thiểu 32 byte.
	//
	// Khóa ngắn làm chữ ký HMAC dò được bằng vét cạn, và khi đó bất kỳ ai
	// cũng tự phát hành được token với vai trò ADMIN.
	Secret string

	// TTL là thời hạn token. Bỏ trống dùng 15 phút.
	TTL time.Duration
}

// minSecretLen là độ dài khóa tối thiểu, theo RFC 7518 mục 3.2 cho HS256.
const minSecretLen = 32

// defaultTTL khớp identity/domain.AccessTokenTTL.
const defaultTTL = 15 * time.Minute

// NewIssuer tạo bộ phát hành token.
//
// Trả lỗi khi khóa quá ngắn — đây là lỗi cấu hình phải chặn lúc KHỞI ĐỘNG,
// không phải lúc phục vụ request. Một hệ thống chạy được với khóa yếu là hệ
// thống không ai biết là đang yếu.
func NewIssuer(cfg Config) (*Issuer, error) {
	if len(cfg.Secret) < minSecretLen {
		return nil, fmt.Errorf(
			"token: khóa bí mật phải dài tối thiểu %d ký tự, nhận %d",
			minSecretLen, len(cfg.Secret))
	}

	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = defaultTTL
	}

	return &Issuer{secret: []byte(cfg.Secret), ttl: ttl}, nil
}

// TTL trả về thời hạn token, để tầng HTTP báo `expires_in` cho client.
func (i *Issuer) TTL() time.Duration { return i.ttl }

// Issue phát hành access token.
func (i *Issuer) Issue(c Claims, now time.Time) (string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	claims := jwtClaims{
		Roles:     c.Roles,
		Scope:     c.Scope,
		SellerIDs: c.SellerIDs,
		SessionID: c.SessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   c.UserID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
	}

	signed, err := jwt.NewWithClaims(signingMethod, claims).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("token: ký token thất bại: %w", err)
	}
	return signed, nil
}

// Verify xác minh token và trả về nội dung.
//
// Trả ErrExpired khi token hết hạn, ErrInvalidToken cho mọi lý do khác.
// Bên gọi KHÔNG được chuyển sự phân biệt này ra response — nó cho kẻ tấn
// công biết họ đã đi đúng hướng tới đâu.
func (i *Issuer) Verify(tokenString string) (Claims, error) {
	var claims jwtClaims

	_, err := jwt.ParseWithClaims(tokenString, &claims,
		func(t *jwt.Token) (any, error) {
			// Chỉ chấp nhận ĐÚNG thuật toán đã chọn. Không tin header.
			if t.Method.Alg() != signingMethod.Alg() {
				return nil, fmt.Errorf("%w: thuật toán %q không được chấp nhận",
					ErrInvalidToken, t.Method.Alg())
			}
			return i.secret, nil
		},
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Claims{}, ErrExpired
		}
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if claims.Subject == "" {
		return Claims{}, fmt.Errorf("%w: thiếu định danh người dùng", ErrInvalidToken)
	}

	return Claims{
		UserID:    claims.Subject,
		Roles:     claims.Roles,
		Scope:     claims.Scope,
		SellerIDs: claims.SellerIDs,
		SessionID: claims.SessionID,
	}, nil
}
