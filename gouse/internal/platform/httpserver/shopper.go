package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

// Shopper là danh tính của người đang mua hàng.
//
// # Vì sao KHÔNG dùng AuthContext
//
// `AuthContext.UserID` là định danh TÀI KHOẢN ĐĂNG NHẬP. Giỏ hàng và phiên
// thanh toán gắn với KHÁCH HÀNG — hai khái niệm khác nhau và do hai module
// khác nhau sở hữu (xem identity/public.go: "identity KHÔNG phải customer").
//
// Và quan trọng hơn: khách VÃNG LAI mua được mà không có tài khoản nào.
type Shopper struct {
	// CustomerID có giá trị khi khách đã đăng nhập VÀ có hồ sơ khách hàng.
	CustomerID string

	// SessionID là định danh phiên của khách vãng lai.
	//
	// Luôn có giá trị, kể cả khi đã đăng nhập — nhờ vậy lúc đăng nhập còn
	// biết giỏ vãng lai nào cần gộp vào giỏ tài khoản.
	SessionID string
}

// IsGuest cho biết đây có phải khách vãng lai không.
func (s Shopper) IsGuest() bool { return s.CustomerID == "" }

// CustomerResolver đổi định danh tài khoản lấy định danh khách hàng.
//
// Interface do PLATFORM khai báo, module `customer` cài đặt — cùng mẫu với
// TokenVerifier. Nhờ vậy platform không cần biết module nào tồn tại (R3),
// và không module nghiệp vụ nào phải phụ thuộc module khác chỉ để lấy danh
// tính người mua.
type CustomerResolver interface {
	// CustomerIDForUser trả định danh khách hàng của một tài khoản.
	//
	// Trả chuỗi rỗng khi tài khoản CHƯA có hồ sơ khách hàng — đó là trạng
	// thái hợp lệ, không phải lỗi: nhân viên vận hành có tài khoản nhưng
	// không phải khách hàng.
	CustomerIDForUser(ctx context.Context, userID string) (string, error)
}

// guestSessionCookie là tên cookie giữ phiên của khách vãng lai.
const guestSessionCookie = "shopper_session"

// guestSessionTTL là thời hạn phiên vãng lai.
//
// Ba mươi ngày: đủ dài để khách quay lại thấy giỏ còn nguyên, đủ ngắn để
// một máy dùng chung không giữ giỏ của người lạ vô thời hạn.
const guestSessionTTL = 30 * 24 * time.Hour

type shopperCtxKey struct{}

// WithShopper gắn danh tính người mua vào context. Xuất ra để test dùng.
func WithShopper(ctx context.Context, s Shopper) context.Context {
	return context.WithValue(ctx, shopperCtxKey{}, s)
}

// ShopperFrom lấy danh tính người mua từ context.
func ShopperFrom(ctx context.Context) (Shopper, bool) {
	s, ok := ctx.Value(shopperCtxKey{}).(Shopper)
	return s, ok
}

// ResolveShopper xác định ai đang mua hàng, kể cả khách vãng lai.
//
// PHẢI đặt SAU middleware xác thực nếu có — nó đọc AuthContext để biết
// khách đã đăng nhập chưa. Nhưng nó KHÔNG yêu cầu đăng nhập: đường mua
// hàng phải chạy được cho khách vãng lai (mvp.md mục 4).
//
// Phiên vãng lai lưu ở cookie `HttpOnly`: định danh này quyết định giỏ nào
// là của ai, nên JavaScript đọc được nó nghĩa là một lỗ hổng XSS đủ để đọc
// giỏ hàng người khác.
func ResolveShopper(resolver CustomerResolver) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := Shopper{SessionID: guestSession(w, r)}

			if ac, ok := AuthContextFrom(r.Context()); ok && resolver != nil {
				// Không tra được hồ sơ khách KHÔNG phải lỗi chặn đường:
				// coi như khách vãng lai và đi tiếp. Chặn ở đây nghĩa là
				// một sự cố của module customer làm sập cả luồng mua hàng.
				if id, err := resolver.CustomerIDForUser(r.Context(), ac.UserID); err == nil {
					s.CustomerID = id
				}
			}

			next.ServeHTTP(w, r.WithContext(WithShopper(r.Context(), s)))
		})
	}
}

// guestSession đọc phiên vãng lai từ cookie, tạo mới nếu chưa có.
func guestSession(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(guestSessionCookie); err == nil && c.Value != "" {
		return c.Value
	}

	id := newSessionID()
	http.SetCookie(w, &http.Cookie{
		Name:     guestSessionCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   int(guestSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand hỏng là sự cố hệ thống nghiêm trọng. Trả chuỗi rỗng
		// để giỏ không gắn vào một định danh đoán được — thà khách mất giỏ
		// còn hơn hai người dùng chung một giỏ.
		return ""
	}
	return "ses_" + hex.EncodeToString(b[:])
}
