package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// newTestHandler dựng bộ định tuyến giống hệt production, kèm middleware.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	cfg := &config.Config{
		Env: config.EnvDevelopment,
		HTTP: config.HTTPConfig{
			MaxRequestBytes: 1 << 20,
		},
		Log: config.LogConfig{Level: "error", Format: "json"},
	}
	log := logger.NewWithWriter(io.Discard, "error", "json")

	mux := http.NewServeMux()
	registerRoutes(mux, cfg, log)

	// Nối cùng middleware với httpserver.New để test đúng hành vi thật.
	return httpserver.Chain(mux,
		httpserver.RequestID(),
		httpserver.Recover(log),
		httpserver.SecurityHeaders(),
		httpserver.MaxBytes(cfg.HTTP.MaxRequestBytes),
	)
}

func TestHealthLiveEndpoint(t *testing.T) {
	h := newTestHandler(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/health/live phải trả 200, nhận %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response phải là JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("mong status=ok, nhận %v", body)
	}
}

func TestHealthReadyEndpoint(t *testing.T) {
	h := newTestHandler(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/health/ready phải trả 200 khi chưa có phụ thuộc, nhận %d", rec.Code)
	}
}

func TestVersionEndpoint(t *testing.T) {
	h := newTestHandler(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response phải là JSON: %v", err)
	}
	if body["version"] == "" {
		t.Error("phải trả về version")
	}
	if body["env"] != "development" {
		t.Errorf("env: mong development, nhận %q", body["env"])
	}
}

// TestUnknownRouteReturnsSpecCompliantError — đường dẫn không tồn tại phải
// trả lỗi ĐÚNG ĐỊNH DẠNG ĐẶC TẢ, không phải trang 404 mặc định của Go.
//
// Client xử lý lỗi theo `code`; nếu một endpoint trả về HTML thay vì JSON,
// client sẽ crash khi parse.
func TestUnknownRouteReturnsSpecCompliantError(t *testing.T) {
	h := newTestHandler(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/khong-ton-tai", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("mong 404, nhận %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("lỗi 404 phải là JSON đúng đặc tả, không phải HTML: %v\nbody: %s",
			err, rec.Body.String())
	}

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("thiếu object 'error': %v", body)
	}
	if errObj["code"] != "NOT_FOUND" {
		t.Errorf("code: mong NOT_FOUND, nhận %v", errObj["code"])
	}
	if body["request_id"] == nil || body["request_id"] == "" {
		t.Error("mọi response lỗi phải có request_id")
	}
}

func TestAllResponsesHaveRequestID(t *testing.T) {
	h := newTestHandler(t)

	paths := []string{"/health/live", "/health/ready", "/version", "/khong-ton-tai"}
	for _, p := range paths {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))

		if rec.Header().Get("X-Request-ID") == "" {
			t.Errorf("%s: thiếu header X-Request-ID — cần để hỗ trợ khách hàng", p)
		}
	}
}

func TestSecurityHeadersOnAllRoutes(t *testing.T) {
	h := newTestHandler(t)

	for _, p := range []string{"/health/live", "/version", "/khong-ton-tai"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))

		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: thiếu header bảo mật", p)
		}
	}
}
