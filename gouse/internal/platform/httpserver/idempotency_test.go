package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

func TestIdempotencyKeyRequiredForWrites(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			h := httpserver.Chain(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Error("handler KHÔNG được chạy khi thiếu Idempotency-Key")
				}),
				httpserver.RequireIdempotencyKey(),
			)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(method, "/x", nil))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("thiếu khóa phải trả 400, nhận %d", rec.Code)
			}
			if got := errorBody(t, rec); got != "VALIDATION_FAILED" {
				t.Errorf("mã lỗi phải là VALIDATION_FAILED, nhận %q", got)
			}
		})
	}
}

// TestIdempotencyKeyNotRequiredForSafeMethods: GET không đổi trạng thái;
// PUT và DELETE đã idempotent theo định nghĩa HTTP.
func TestIdempotencyKeyNotRequiredForSafeMethods(t *testing.T) {
	for _, method := range []string{
		http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead,
	} {
		t.Run(method, func(t *testing.T) {
			passed := false
			h := httpserver.Chain(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { passed = true }),
				httpserver.RequireIdempotencyKey(),
			)

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/x", nil))

			if !passed {
				t.Errorf("%s không cần Idempotency-Key", method)
			}
		})
	}
}

func TestIdempotencyKeyPassedToHandler(t *testing.T) {
	key := ids.MustNew(ids.PrefixRequest).String()

	var got string
	var ok bool
	h := httpserver.Chain(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got, ok = httpserver.IdempotencyKeyFrom(r.Context())
		}),
		httpserver.RequireIdempotencyKey(),
	)

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("Idempotency-Key", key)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !ok {
		t.Fatal("handler phải lấy được khóa từ context")
	}
	if got != key {
		t.Errorf("khóa sai: mong %q, nhận %q", key, got)
	}
}

func TestIdempotencyKeyLengthBounds(t *testing.T) {
	cases := []struct {
		name   string
		key    string
		wantOK bool
	}{
		{"quá ngắn", strings.Repeat("a", 15), false},
		{"đúng giới hạn dưới", strings.Repeat("a", 16), true},
		{"đúng giới hạn trên", strings.Repeat("a", 64), true},
		{"quá dài", strings.Repeat("a", 65), false},
		{"chỉ có khoảng trắng", "                    ", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			passed := false
			h := httpserver.Chain(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { passed = true }),
				httpserver.RequireIdempotencyKey(),
			)

			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			req.Header.Set("Idempotency-Key", tc.key)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if passed != tc.wantOK {
				t.Errorf("khóa %q: mong đi tiếp=%v, nhận %v (status %d)",
					tc.name, tc.wantOK, passed, rec.Code)
			}
		})
	}
}

// TestIdempotencyKeyMissingFromContextWhenNotApplied: handler phải phân biệt
// được "không có khóa" với "khóa rỗng". Nếu IdempotencyKeyFrom trả ok=true
// cho chuỗi rỗng, module sẽ ghi một bản ghi có idempotency_key = ” và ràng
// buộc UNIQUE khiến request hợp lệ TIẾP THEO bị từ chối.
func TestIdempotencyKeyAbsentFromContextWhenNotApplied(t *testing.T) {
	var ok bool
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, ok = httpserver.IdempotencyKeyFrom(r.Context())
	})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/x", nil))

	if ok {
		t.Error("không qua middleware thì context KHÔNG được có khóa")
	}
}
