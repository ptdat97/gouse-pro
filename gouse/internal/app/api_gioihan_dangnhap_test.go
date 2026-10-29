package app

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// TestPasswordSprayingAcrossAccountsIsBlocked là lý do lớp giới hạn theo IP tồn tại.
//
// Khóa theo TÀI KHOẢN (identity.MaxFailedAttempts = 5) chặn được việc dò
// mật khẩu của MỘT tài khoản. Nó mù hoàn toàn trước kiểu RẢI MẬT KHẨU: lấy
// một mật khẩu phổ biến rồi thử lên hàng nghìn email khác nhau. Mỗi tài
// khoản chỉ sai ĐÚNG MỘT LẦN — dưới ngưỡng khóa — nên không tài khoản nào
// bị khóa, trong khi kẻ tấn công vẫn mở được mọi tài khoản mật khẩu yếu.
//
// Chỉ nhìn từ phía ĐƯỜNG MẠNG mới thấy bất thường, và đó là việc của lớp
// giới hạn theo IP.
func TestPasswordSprayingAcrossAccountsIsBlocked(t *testing.T) {
	a := newAPITest(t)

	// Mỗi lượt một email KHÁC nhau: đúng hình dạng của rải mật khẩu.
	var chan429 bool
	for i := 0; i < loginLimit+2; i++ {
		res := a.call(http.MethodPost, "/api/v1/auth/login", map[string]any{
			"email":    newEmail(fmt.Sprintf("nannhan%d", i)),
			"password": "MatKhauPhoBien@123",
		}, nil)

		if res.code == http.StatusTooManyRequests {
			chan429 = true
			if i < loginLimit {
				t.Fatalf("chặn quá sớm ở lượt %d, hạn mức là %d", i+1, loginLimit)
			}
			break
		}
		if res.code != http.StatusUnauthorized {
			t.Fatalf("lượt %d: HTTP %d, cần 401 — %s", i+1, res.code, res.raw)
		}
	}

	if !chan429 {
		t.Fatalf("rải mật khẩu qua %d tài khoản khác nhau mà KHÔNG bị chặn — "+
			"khóa theo tài khoản không thấy kiểu này, và nếu lớp theo IP "+
			"cũng không thấy thì không có gì cản", loginLimit+2)
	}
}

// TestASuccessfulLoginNeverHitsTheRateLimit là nửa còn lại, và là nửa dễ mất.
//
// Đếm MỌI lượt đăng nhập cũng chặn được kẻ rải mật khẩu — nhưng một địa chỉ
// IP là một hạn mức, mà văn phòng, trường học và mạng di động đều ra
// Internet bằng MỘT địa chỉ NAT dùng chung. Khi đó người thứ N gõ ĐÚNG mật
// khẩu vẫn bị chặn, và hỏng kiểu này không ai thấy: log sạch, kẻ tấn công
// vẫn bị chặn, chỉ có khách thật lặng lẽ bỏ đi.
//
// Bài này đăng nhập ĐÚNG nhiều hơn hạn mức và đòi không lượt nào bị chặn.
func TestASuccessfulLoginNeverHitsTheRateLimit(t *testing.T) {
	a := newAPITest(t)

	email := newEmail("vanphong")
	a.registerAndLogIn(email)
	const password = "MatKhauDuDai@2026"

	for i := 0; i < loginLimit+5; i++ {
		res := a.call(http.MethodPost, "/api/v1/auth/login",
			map[string]any{"email": email, "password": password}, nil)
		if res.code != http.StatusOK {
			t.Fatalf("lượt đăng nhập ĐÚNG thứ %d bị từ chối: HTTP %d — %s\n"+
				"giới hạn đang đếm cả lượt thành công, và như thế là khóa "+
				"người dùng thật ngồi chung một địa chỉ NAT",
				i+1, res.code, res.raw)
		}
	}
}

// TestAFewFailuresThenASuccessStillGetsIn: người quên mật khẩu mò lại là chuyện
// thường. Sai vài lần — dưới ngưỡng khóa tài khoản — rồi gõ đúng thì phải
// vào được ngay, không phải chờ hết cửa sổ nào cả.
func TestAFewFailuresThenASuccessStillGetsIn(t *testing.T) {
	a := newAPITest(t)

	email := newEmail("quenmatkhau")
	a.registerAndLogIn(email)
	const password = "MatKhauDuDai@2026"

	for i := 0; i < identity.MaxFailedAttempts-1; i++ {
		a.call(http.MethodPost, "/api/v1/auth/login", map[string]any{
			"email": email, "password": "NhoNhamMatKhau@1",
		}, nil)
	}

	res := a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": password}, nil)
	if res.code != http.StatusOK {
		t.Fatalf("gõ đúng sau %d lần sai vẫn không vào được: HTTP %d — %s",
			identity.MaxFailedAttempts-1, res.code, res.raw)
	}
}
