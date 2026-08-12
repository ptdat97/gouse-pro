package httpserver_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

func discardLogger() *slog.Logger {
	return logger.NewWithWriter(io.Discard, "error", "json")
}

func TestRequestIDGeneratedWhenAbsent(t *testing.T) {
	var seenInContext string
	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenInContext = logger.RequestIDFromContext(r.Context())
		}),
		httpserver.RequestID(),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	got := rec.Header().Get("X-Request-ID")
	if got == "" {
		t.Fatal("X-Request-ID phải LUÔN có trong response — dùng khi hỗ trợ khách hàng")
	}
	if !ids.IsValid(got) {
		t.Errorf("request id sinh ra phải là ULID hợp lệ, nhận %q", got)
	}
	if seenInContext != got {
		t.Errorf("request id trong context (%q) phải khớp header (%q)", seenInContext, got)
	}
}

func TestRequestIDAcceptsValidClientValue(t *testing.T) {
	// Client truyền request id để nối chuỗi truy vết xuyên hệ thống.
	clientID := ids.MustNew(ids.PrefixRequest).String()

	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		httpserver.RequestID(),
	)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-ID", clientID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != clientID {
		t.Errorf("phải giữ request id hợp lệ của client: mong %q, nhận %q", clientID, got)
	}
}

func TestRequestIDRejectsMalformedClientValue(t *testing.T) {
	// KHÔNG tin định dạng do client gửi — chống chèn giá trị rác vào log.
	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		httpserver.RequestID(),
	)

	for _, bad := range []string{"'; DROP TABLE orders--", "abc", "", strings.Repeat("x", 500)} {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("X-Request-ID", bad)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		got := rec.Header().Get("X-Request-ID")
		if got == bad {
			t.Errorf("giá trị client không hợp lệ %q phải bị thay bằng id mới", bad)
		}
		if !ids.IsValid(got) {
			t.Errorf("id thay thế phải hợp lệ, nhận %q", got)
		}
	}
}

// TestRecoverPreventsProcessCrash — panic ở MỘT request không được làm sập
// toàn bộ tiến trình.
func TestRecoverPreventsProcessCrash(t *testing.T) {
	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("lỗi giả lập trong handler")
		}),
		httpserver.RequestID(),
		httpserver.Recover(discardLogger()),
	)

	rec := httptest.NewRecorder()
	// Nếu Recover không hoạt động, dòng này làm sập test binary.
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("panic phải trả 500, nhận %d", rec.Code)
	}
}

// TestRecoverDoesNotLeakStackTrace là test BẢO MẬT.
//
// Stack trace lộ cấu trúc nội bộ, đường dẫn file, tên hàm — chỉ vào log,
// không bao giờ ra response.
func TestRecoverDoesNotLeakStackTrace(t *testing.T) {
	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("mật khẩu database là: hunter2")
		}),
		httpserver.RequestID(),
		httpserver.Recover(discardLogger()),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	body := rec.Body.String()
	for _, leak := range []string{"hunter2", "goroutine", "runtime.", ".go:"} {
		if strings.Contains(body, leak) {
			t.Errorf("RÒ RỈ %q ra response:\n%s", leak, body)
		}
	}

	// Vẫn phải trả lỗi đúng định dạng đặc tả.
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response phải là JSON hợp lệ: %v", err)
	}
	if got["request_id"] == nil || got["request_id"] == "" {
		t.Error("response lỗi phải có request_id để tra cứu")
	}
	errObj := got["error"].(map[string]any)
	if errObj["code"] != "INTERNAL_ERROR" {
		t.Errorf("mong INTERNAL_ERROR, nhận %v", errObj["code"])
	}
}

func TestMaxBytesRejectsOversizedBody(t *testing.T) {
	const limit = 100

	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := io.ReadAll(r.Body); err != nil {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		}),
		httpserver.MaxBytes(limit),
	)

	// Body nhỏ: chấp nhận
	rec := httptest.NewRecorder()
	small := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 50)))
	h.ServeHTTP(rec, small)
	if rec.Code != http.StatusOK {
		t.Errorf("body nhỏ phải được chấp nhận, nhận %d", rec.Code)
	}

	// Body lớn: từ chối — chống cạn bộ nhớ
	rec = httptest.NewRecorder()
	large := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 500)))
	h.ServeHTTP(rec, large)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("body vượt giới hạn phải bị từ chối, nhận %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		httpserver.SecurityHeaders(),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s: mong %q, nhận %q", k, v, got)
		}
	}
}

func TestLoggingIncludesRequestID(t *testing.T) {
	var buf strings.Builder
	log := logger.NewWithWriter(&buf, "info", "json")

	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}),
		httpserver.RequestID(),
		httpserver.Logging(log),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil))

	out := buf.String()
	reqID := rec.Header().Get("X-Request-ID")

	if !strings.Contains(out, reqID) {
		t.Errorf("log phải chứa request_id %q để liên kết:\n%s", reqID, out)
	}
	for _, want := range []string{`"status":201`, `"method":"POST"`, `"path":"/api/v1/orders"`, "duration_ms"} {
		if !strings.Contains(out, want) {
			t.Errorf("log thiếu %q:\n%s", want, out)
		}
	}
}

func TestChainOrder(t *testing.T) {
	// Middleware đầu tiên là lớp NGOÀI CÙNG.
	var order []string
	mw := func(name string) httpserver.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "vào:"+name)
				next.ServeHTTP(w, r)
				order = append(order, "ra:"+name)
			})
		}
	}

	h := httpserver.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		}),
		mw("ngoài"), mw("trong"),
	)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"vào:ngoài", "vào:trong", "handler", "ra:trong", "ra:ngoài"}
	if len(order) != len(want) {
		t.Fatalf("thứ tự sai: %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("bước %d: mong %q, nhận %q (đầy đủ: %v)", i, want[i], order[i], order)
		}
	}
}
