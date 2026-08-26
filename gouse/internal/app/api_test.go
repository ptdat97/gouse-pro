package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// apiTest dựng TOÀN BỘ ứng dụng — module thật, route thật, middleware thật,
// PostgreSQL thật — rồi gọi qua HTTP.
//
// # Vì sao cần lớp test này khi đã có test từng module và test đầu-cuối
//
// Hai lớp kia đều BỎ QUA tầng HTTP. Test module gọi thẳng service Go; test
// trong `internal/e2e` cũng vậy. Chúng không thấy được:
//
//	– route quên bọc `RequireRole` → ai cũng gọi được đường quản trị
//	– route quên bọc `RequireIdempotencyKey` → mất chống trùng
//	– handler trả đúng dữ liệu nhưng sai MÃ TRẠNG THÁI
//	– tên trường JSON lệch với đặc tả
//
// Ba trong bốn loại trên là lỗi NỐI DÂY: bản thân module đúng, chỉ chỗ ráp
// sai. Không lớp test nào khác nhìn thấy chúng, vì mỗi lớp chỉ thấy phần
// mình dựng.
//
// Đây là lý do `internal/app` được tách khỏi `package main` (P0-7).
type apiTest struct {
	t       *testing.T
	handler http.Handler
	mods    Modules

	// db cho phép test soi TRẠNG THÁI DATABASE trước/sau một request.
	//
	// Cần cho lớp bất biến mạnh nhất: "đường đọc KHÔNG ghi gì". Kiểm qua
	// response chỉ thấy được thứ handler chọn trả về — một lần INSERT âm
	// thầm vẫn lọt.
	db *database.DB

	// cookies giữ phiên giữa các request.
	//
	// `httptest` KHÔNG tự quản cookie như trình duyệt, nên thiếu chỗ này
	// thì mỗi lời gọi là một khách vãng lai KHÁC — và luồng mua hàng của
	// khách vãng lai (thêm giỏ → thanh toán) đứt ngay ở bước thứ hai với
	// "Giỏ hàng này không thuộc về bạn".
	cookies map[string]string
}

func newAPITest(t *testing.T) *apiTest {
	t.Helper()

	db := testdb.Open(t)
	log := logger.NewWithWriter(io.Discard, "error", "json")

	cfg := &config.Config{
		Env: config.EnvDevelopment,
		HTTP: config.HTTPConfig{
			MaxRequestBytes: 1 << 20,
		},
		Log:     config.LogConfig{Level: "error", Format: "json"},
		Modules: config.ModulesConfig{Storage: "postgres"},
		Auth: config.AuthConfig{
			JWTSecret: "development-only-jwt-secret-do-not-use-in-production",
		},
	}

	mods, err := Build(context.Background(), cfg, log, db)
	if err != nil {
		t.Fatalf("dựng ứng dụng: %v", err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, cfg, log, db, mods, "test")

	// CÙNG chuỗi middleware, CÙNG thứ tự với httpserver.New.
	//
	// Thiếu một cái là test một hệ thống khác với hệ thống chạy thật —
	// và điều đó đã xảy ra: bản đầu bỏ quên `Logging`, nên
	// `logger.FromContext` rơi về logger mặc định và mọi lần từ chối
	// quyền đổ ra stderr. Đầu ra nhiễu chỉ là triệu chứng; vấn đề thật là
	// chuỗi đã lệch.
	h := httpserver.Chain(mux,
		httpserver.RequestID(),
		httpserver.Recover(log),
		httpserver.Logging(log),
		httpserver.Metrics(),
		httpserver.SecurityHeaders(),
		httpserver.CORS([]string{"http://localhost:3001"}),
		httpserver.MaxBytes(cfg.HTTP.MaxRequestBytes),
	)
	return &apiTest{t: t, handler: h, mods: mods, db: db, cookies: map[string]string{}}
}

type reply struct {
	code int
	body map[string]any
	raw  string
}

// call gọi một request và trả về mã trạng thái cùng thân đã giải mã.
func (a *apiTest) call(
	method, path string, body any, headers map[string]string,
) reply {
	a.t.Helper()

	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			a.t.Fatalf("mã hóa thân request: %v", err)
		}
		r = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for name, val := range a.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: val})
	}

	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)

	// Nhớ cookie máy chủ vừa cấp, giống trình duyệt.
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 || c.Value == "" {
			delete(a.cookies, c.Name)
			continue
		}
		a.cookies[c.Name] = c.Value
	}

	out := reply{code: rec.Code, raw: rec.Body.String()}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	}
	return out
}

// dangKyVaDangNhap tạo một tài khoản mới và trả access token.
//
// Đi qua ĐÚNG hai endpoint mà giao diện dùng, không gọi tắt vào module:
// mục đích của lớp test này là kiểm chuỗi HTTP.
func (a *apiTest) dangKyVaDangNhap(email string) string {
	a.t.Helper()
	const matKhau = "MatKhauDuDai@2026"

	res := a.call(http.MethodPost, "/api/v1/auth/register",
		map[string]any{"email": email, "password": matKhau},
		map[string]string{"Idempotency-Key": ids.MustNew(ids.PrefixRequest).String()})
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		a.t.Fatalf("đăng ký %s: HTTP %d — %s", email, res.code, res.raw)
	}

	res = a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": matKhau}, nil)
	if res.code != http.StatusOK {
		a.t.Fatalf("đăng nhập %s: HTTP %d — %s", email, res.code, res.raw)
	}
	tok, _ := res.body["access_token"].(string)
	if tok == "" {
		a.t.Fatalf("đăng nhập không trả access_token: %s", res.raw)
	}
	return tok
}

// khoaIdem sinh header Idempotency-Key mới.
//
// Cần cho mọi request POST/PATCH/PUT trong test phân quyền: thiếu nó thì
// middleware trả 400 TRƯỚC khi tới bước kiểm quyền, và bài test sẽ đo
// nhầm thứ.
func khoaIdem() map[string]string {
	return map[string]string{"Idempotency-Key": ids.MustNew(ids.PrefixRequest).String()}
}

func bearer(tok string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + tok}
}

func emailMoi(prefix string) string {
	return fmt.Sprintf("%s-%s@apitest.local", prefix,
		ids.MustNew(ids.PrefixRequest).String()[4:14])
}

// demDong đếm số dòng khớp điều kiện.
func (a *apiTest) demDong(bang, dieuKien string, args ...any) int {
	a.t.Helper()
	var n int
	q := "SELECT count(*) FROM " + bang
	if dieuKien != "" {
		q += " WHERE " + dieuKien
	}
	if err := a.db.Pool().QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		a.t.Fatalf("đếm %s: %v", bang, err)
	}
	return n
}

// anhChupBang chụp lại (số dòng, tổng updated_at) của một bảng.
//
// Tổng dấu thời gian bắt được cả trường hợp SỬA mà không thêm dòng — thứ
// mà đếm dòng một mình bỏ qua.
func (a *apiTest) anhChupBang(bang string) (int, float64) {
	a.t.Helper()
	var n int
	var tong *float64
	err := a.db.Pool().QueryRow(context.Background(),
		"SELECT count(*), sum(extract(epoch FROM updated_at)) FROM "+bang,
	).Scan(&n, &tong)
	if err != nil {
		a.t.Fatalf("chụp bảng %s: %v", bang, err)
	}
	if tong == nil {
		return n, 0
	}
	return n, *tong
}
