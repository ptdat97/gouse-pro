package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

const uiOrigin = "http://localhost:3000"

func corsHandler(origins []string, onNext func()) http.Handler {
	return httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if onNext != nil {
				onNext()
			}
			w.WriteHeader(http.StatusOK)
		}),
		httpserver.CORS(origins),
	)
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	h := corsHandler([]string{uiOrigin}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sellers", nil)
	req.Header.Set("Origin", uiOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != uiOrigin {
		t.Errorf("Allow-Origin = %q, mong %q", got, uiOrigin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Error("phải cho phép credentials — refresh token nằm ở cookie")
	}
}

// TestCORSNeverUsesWildcardWithCredentials là bất biến bảo mật quan trọng
// nhất của middleware này.
//
// `*` kết hợp với credentials nghĩa là BẤT KỲ trang web nào cũng gọi được
// API dưới danh nghĩa người dùng đang đăng nhập. Chuẩn CORS cấm điều đó, và
// ta cũng không được tự lách bằng cách phản chiếu mọi origin.
func TestCORSNeverUsesWildcardWithCredentials(t *testing.T) {
	h := corsHandler([]string{uiOrigin}, nil)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", uiOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Fatal("KHÔNG BAO GIỜ được dùng * khi cho phép credentials")
	}
}

// Origin lạ KHÔNG được nhận header CORS.
//
// Không đặt header là cách trình duyệt biết phải chặn. Phản chiếu lại origin
// nào cũng được thì danh sách trắng thành vô nghĩa.
func TestCORSRejectsUnknownOrigin(t *testing.T) {
	h := corsHandler([]string{uiOrigin}, nil)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://ke-tan-cong.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("origin lạ KHÔNG được nhận header CORS, nhận %q", got)
	}
}

// Preflight trả 204 và KHÔNG chạy handler.
//
// Preflight chỉ hỏi "tôi có được gửi request thật không". Cho nó đi tiếp
// nghĩa là một OPTIONS có thể kích hoạt thao tác nghiệp vụ.
func TestCORSPreflightDoesNotReachHandler(t *testing.T) {
	reached := false
	h := corsHandler([]string{uiOrigin}, func() { reached = true })

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", uiOrigin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if reached {
		t.Error("preflight KHÔNG được chạy tới handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight phải trả 204, nhận %d", rec.Code)
	}

	// Client PHẢI gửi được các header mà API yêu cầu, nếu không mọi lệnh
	// ghi đều bị trình duyệt chặn trước khi rời máy.
	allow := rec.Header().Get("Access-Control-Allow-Headers")
	// X-Guest-Phone nằm trong danh sách vì khách VÃNG LAI tra đơn bằng mã
	// đơn kèm số điện thoại — thiếu nó thì cả trang tra cứu đơn không chạy.
	for _, h := range []string{
		"Authorization", "Idempotency-Key", "Content-Type", "X-Guest-Phone",
	} {
		if !contains(allow, h) {
			t.Errorf("preflight phải cho phép header %q, nhận %q", h, allow)
		}
	}
}

// Vary: Origin BẮT BUỘC có, kể cả khi origin bị từ chối.
//
// Thiếu nó, một proxy có thể trả cho origin A cái response đã cache cho
// origin B — và khi đó danh sách trắng bị vô hiệu hóa bởi tầng cache.
func TestCORSAlwaysSetsVary(t *testing.T) {
	h := corsHandler([]string{uiOrigin}, nil)

	for _, origin := range []string{uiOrigin, "https://la.example.com"} {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if !contains(rec.Header().Get("Vary"), "Origin") {
			t.Errorf("origin %q: thiếu Vary: Origin", origin)
		}
	}
}

// Request không phải từ trình duyệt (không có Origin) đi qua bình thường.
func TestCORSIgnoresNonBrowserRequests(t *testing.T) {
	reached := false
	h := corsHandler([]string{uiOrigin}, func() { reached = true })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if !reached {
		t.Error("request không có Origin phải đi tiếp bình thường")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("không có Origin thì không đặt header CORS")
	}
}

// Danh sách rỗng = không trình duyệt nào gọi được.
//
// Mặc định AN TOÀN: quên cấu hình thì giao diện hỏng ngay và người ta sửa,
// thay vì mở cho mọi origin mà không ai để ý.
func TestCORSEmptyAllowlistBlocksEveryone(t *testing.T) {
	h := corsHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", uiOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("danh sách rỗng thì KHÔNG origin nào được phép")
	}
}
