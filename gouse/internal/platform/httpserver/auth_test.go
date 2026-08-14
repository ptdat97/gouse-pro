package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

// stubVerifier là TokenVerifier giả cho test.
type stubVerifier struct {
	ac  httpserver.AuthContext
	err error

	// gotToken ghi lại token nhận được, để kiểm tra việc tách header.
	gotToken string
}

func (s *stubVerifier) VerifyAccessToken(_ context.Context, token string) (httpserver.AuthContext, error) {
	s.gotToken = token
	if s.err != nil {
		return httpserver.AuthContext{}, s.err
	}
	return s.ac, nil
}

// errorBody đọc mã lỗi từ response.
func errorBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response không phải JSON hợp lệ: %v", err)
	}
	return body.Error.Code
}

func TestAuthRejectsMissingToken(t *testing.T) {
	v := &stubVerifier{}
	called := false
	h := httpserver.Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }),
		httpserver.Auth(v),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if called {
		t.Fatal("handler KHÔNG được chạy khi thiếu token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("thiếu token phải trả 401, nhận %d", rec.Code)
	}
	if got := errorBody(t, rec); got != "UNAUTHORIZED" {
		t.Errorf("mã lỗi phải là UNAUTHORIZED, nhận %q", got)
	}
}

func TestAuthRejectsInvalidToken(t *testing.T) {
	v := &stubVerifier{err: errors.New("token hết hạn lúc 14:23")}
	h := httpserver.Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("handler KHÔNG được chạy khi token không hợp lệ")
		}),
		httpserver.Auth(v),
	)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer some-token-value")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("token không hợp lệ phải trả 401, nhận %d", rec.Code)
	}

	// Lý do thất bại KHÔNG được ra response: phân biệt "hết hạn" với "sai
	// chữ ký" cho kẻ tấn công biết họ đã đi đúng hướng tới đâu.
	if body := rec.Body.String(); contains(body, "14:23") {
		t.Errorf("chi tiết lỗi nội bộ bị lộ ra response: %s", body)
	}
}

// TestAuthDistinguishes401From403 là bất biến quan trọng nhất của middleware
// này. Trả sai mã khiến client rơi vào vòng lặp làm mới token vô vọng: 403
// nghĩa là đăng nhập lại VÔ ÍCH, 401 nghĩa là đăng nhập lại CÓ ÍCH.
func TestAuthDistinguishes401From403(t *testing.T) {
	v := &stubVerifier{ac: httpserver.AuthContext{
		UserID: "usr_1",
		Roles:  []string{"OPS_SUPPORT"},
	}}

	h := httpserver.Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("handler KHÔNG được chạy khi thiếu vai trò")
		}),
		httpserver.Auth(v),
		httpserver.RequireRole("OPS_FINANCE"),
	)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer valid-token-here")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("đã xác thực nhưng thiếu vai trò phải trả 403, nhận %d — "+
			"401 sẽ khiến client đăng nhập lại vô ích", rec.Code)
	}
	if got := errorBody(t, rec); got != "FORBIDDEN" {
		t.Errorf("mã lỗi phải là FORBIDDEN, nhận %q", got)
	}

	// Thông báo KHÔNG được tiết lộ vai trò nào còn thiếu — đó là bản đồ mô
	// hình phân quyền, và người bị từ chối không cần bản đồ đó.
	if body := rec.Body.String(); contains(body, "OPS_FINANCE") {
		t.Errorf("response không được lộ vai trò yêu cầu: %s", body)
	}
}

func TestAuthPassesContextToHandler(t *testing.T) {
	want := httpserver.AuthContext{
		UserID:    "usr_01J9XABC",
		Roles:     []string{"ADMIN"},
		Scope:     "ALL",
		SessionID: "ses_01J9XABC",
	}
	v := &stubVerifier{ac: want}

	var got httpserver.AuthContext
	var ok bool
	h := httpserver.Chain(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got, ok = httpserver.AuthContextFrom(r.Context())
		}),
		httpserver.Auth(v),
	)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer valid-token-here")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !ok {
		t.Fatal("handler phải lấy được AuthContext từ context")
	}
	if got.UserID != want.UserID || got.Scope != want.Scope {
		t.Errorf("AuthContext sai: mong %+v, nhận %+v", want, got)
	}
	if v.gotToken != "valid-token-here" {
		t.Errorf("token phải được tách khỏi tiền tố Bearer, nhận %q", v.gotToken)
	}
}

func TestAuthAcceptsAnyBearerCasing(t *testing.T) {
	// RFC 7235: tên scheme không phân biệt hoa thường.
	for _, scheme := range []string{"Bearer", "bearer", "BEARER"} {
		t.Run(scheme, func(t *testing.T) {
			v := &stubVerifier{ac: httpserver.AuthContext{UserID: "usr_1"}}
			passed := false
			h := httpserver.Chain(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { passed = true }),
				httpserver.Auth(v),
			)

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Authorization", scheme+" valid-token-here")
			h.ServeHTTP(httptest.NewRecorder(), req)

			if !passed {
				t.Errorf("scheme %q phải được chấp nhận", scheme)
			}
		})
	}
}

func TestAuthRejectsMalformedHeader(t *testing.T) {
	cases := map[string]string{
		"thiếu scheme":  "just-a-token",
		"sai scheme":    "Basic dXNlcjpwYXNz",
		"chỉ có scheme": "Bearer",
		"token rỗng":    "Bearer   ",
		"header rỗng":   "",
	}

	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			v := &stubVerifier{ac: httpserver.AuthContext{UserID: "usr_1"}}
			h := httpserver.Chain(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Error("handler KHÔNG được chạy với header sai định dạng")
				}),
				httpserver.Auth(v),
			)

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("phải trả 401, nhận %d", rec.Code)
			}
		})
	}
}

// TestRequireRoleWithoutAuthFailsClosed kiểm chứng chuỗi middleware sai thứ
// tự thất bại theo hướng AN TOÀN. Nếu RequireRole chạy trước Auth mà vẫn cho
// request đi tiếp, một lỗi nối dây sẽ mở toang endpoint quản trị.
func TestRequireRoleWithoutAuthFailsClosed(t *testing.T) {
	h := httpserver.Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("handler KHÔNG được chạy khi chưa qua Auth")
		}),
		httpserver.RequireRole("ADMIN"),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("thiếu AuthContext phải trả 401, nhận %d", rec.Code)
	}
}

func TestRequireRoleAllowsAnyMatchingRole(t *testing.T) {
	v := &stubVerifier{ac: httpserver.AuthContext{
		UserID: "usr_1",
		Roles:  []string{"OPS_SUPPORT"},
	}}

	passed := false
	h := httpserver.Chain(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { passed = true }),
		httpserver.Auth(v),
		httpserver.RequireRole("ADMIN", "OPS_SUPPORT"),
	)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer valid-token-here")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !passed {
		t.Error("có MỘT trong các vai trò yêu cầu là đủ để đi tiếp")
	}
}

// TestHasAnyRoleEmptyListDeniesAccess: danh sách rỗng phải trả false.
//
// Nếu trả true, một lời gọi RequireRole() viết thiếu tham số sẽ âm thầm cho
// mọi người đã đăng nhập đi qua — lỗi kiểu này rất khó thấy khi đọc code.
func TestHasAnyRoleEmptyListDeniesAccess(t *testing.T) {
	ac := httpserver.AuthContext{Roles: []string{"ADMIN"}}
	if ac.HasAnyRole() {
		t.Error("danh sách vai trò rỗng phải trả false, không phải true")
	}
}

func TestHasRole(t *testing.T) {
	ac := httpserver.AuthContext{Roles: []string{"OPS_FINANCE", "OPS_SUPPORT"}}

	if !ac.HasRole("OPS_FINANCE") {
		t.Error("phải tìm thấy vai trò có trong danh sách")
	}
	if ac.HasRole("ADMIN") {
		t.Error("không được báo có vai trò không nằm trong danh sách")
	}
	// Vai trò phân biệt hoa thường: "admin" không phải "ADMIN".
	if ac.HasRole("ops_finance") {
		t.Error("so khớp vai trò phải phân biệt hoa thường")
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
