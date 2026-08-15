package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

func testHTTPConfig(port int) config.HTTPConfig {
	return config.HTTPConfig{
		Port:            port,
		ReadTimeout:     2 * time.Second,
		WriteTimeout:    2 * time.Second,
		IdleTimeout:     5 * time.Second,
		ShutdownTimeout: 2 * time.Second,
		MaxRequestBytes: 1 << 20,
	}
}

// freePort xin hệ điều hành một cổng trống để test không xung đột.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("không xin được cổng trống: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestServerStartsAndServes kiểm chứng server thật sự phục vụ được request.
func TestServerStartsAndServes(t *testing.T) {
	port := freePort(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	srv := httpserver.New(testHTTPConfig(port), logger.NewWithWriter(io.Discard, "error", "json"), mux, nil)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()

	waitForServer(t, port)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
	if err != nil {
		t.Fatalf("gọi server lỗi: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("mong 'pong', nhận %q", body)
	}
	// Middleware chuẩn phải được nối sẵn.
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("server phải tự động gắn X-Request-ID")
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("server phải tự động gắn header bảo mật")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Errorf("Run trả lỗi khi tắt bình thường: %v", err)
	}
}

// TestGracefulShutdownWaitsForInflightRequest — cắt ngang giao dịch đang chạy
// có thể để lại trạng thái nửa vời trong database.
func TestGracefulShutdownWaitsForInflightRequest(t *testing.T) {
	port := freePort(t)
	started := make(chan struct{})
	finished := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond) // giả lập giao dịch đang chạy
		_, _ = w.Write([]byte("xong"))
		close(finished)
	})

	srv := httpserver.New(testHTTPConfig(port), logger.NewWithWriter(io.Discard, "error", "json"), mux, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Run(ctx) }()

	waitForServer(t, port)

	respCh := make(chan string, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/slow", port))
		if err != nil {
			respCh <- "lỗi: " + err.Error()
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		respCh <- string(b)
	}()

	<-started
	cancel() // yêu cầu tắt KHI request đang xử lý dở

	select {
	case got := <-respCh:
		if got != "xong" {
			t.Errorf("request đang xử lý bị cắt ngang: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request không hoàn tất sau khi yêu cầu tắt")
	}

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Error("handler không chạy xong")
	}
}

// TestHealthLiveDoesNotCheckDependencies là phân biệt QUAN TRỌNG.
//
// Nếu `live` kiểm tra database, một sự cố database ngắn sẽ khiến bộ điều phối
// khởi động lại TOÀN BỘ tiến trình — làm sự cố tệ hơn nhiều.
func TestHealthLiveDoesNotCheckDependencies(t *testing.T) {
	dbDown := func(context.Context) error {
		return errors.New("database không kết nối được")
	}

	live, ready := httpserver.Health(map[string]httpserver.HealthChecker{
		"database": dbDown,
	})

	// live: vẫn OK dù database hỏng
	recLive := httptest.NewRecorder()
	live.ServeHTTP(recLive, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if recLive.Code != http.StatusOK {
		t.Errorf("/health/live phải trả 200 dù phụ thuộc hỏng (nhận %d) — "+
			"nếu không, sự cố database ngắn sẽ gây khởi động lại tiến trình", recLive.Code)
	}

	// ready: báo không sẵn sàng
	recReady := httptest.NewRecorder()
	ready.ServeHTTP(recReady, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if recReady.Code != http.StatusServiceUnavailable {
		t.Errorf("/health/ready phải trả 503 khi phụ thuộc hỏng, nhận %d", recReady.Code)
	}

	var body map[string]any
	_ = json.Unmarshal(recReady.Body.Bytes(), &body)
	checks := body["checks"].(map[string]any)
	if checks["database"] == "ok" {
		t.Error("kết quả kiểm tra phải nêu rõ phụ thuộc nào hỏng")
	}
}

func TestHealthReadyWhenAllChecksPass(t *testing.T) {
	ok := func(context.Context) error { return nil }
	_, ready := httpserver.Health(map[string]httpserver.HealthChecker{
		"database": ok,
		"cache":    ok,
	})

	rec := httptest.NewRecorder()
	ready.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("mọi kiểm tra đạt phải trả 200, nhận %d", rec.Code)
	}

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("status: mong ok, nhận %v", body["status"])
	}
	checks := body["checks"].(map[string]any)
	for name, result := range checks {
		if result != "ok" {
			t.Errorf("kiểm tra %q: mong ok, nhận %v", name, result)
		}
	}
}

func TestHealthWithNoChecks(t *testing.T) {
	// Giai đoạn đầu chưa có database — ready vẫn phải hoạt động.
	_, ready := httpserver.Health(nil)

	rec := httptest.NewRecorder()
	ready.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("không có phụ thuộc nào phải trả 200, nhận %d", rec.Code)
	}
}

// waitForServer chờ server sẵn sàng nhận kết nối.
func waitForServer(t *testing.T, port int) {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < 100; i++ {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server không khởi động trong thời hạn: %s", addr)
}
