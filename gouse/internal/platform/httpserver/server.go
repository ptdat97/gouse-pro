package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/config"
)

// Server bọc http.Server với graceful shutdown.
type Server struct {
	httpServer *http.Server
	log        *slog.Logger
	shutdown   time.Duration
	addr       string
}

// New tạo server với middleware chuẩn đã được nối sẵn.
//
// Thứ tự middleware quan trọng:
//  1. RequestID  — phải đầu tiên, để mọi thứ sau đó có request_id
//  2. Recover    — bắt panic từ mọi tầng bên trong
//  3. Logging    — ghi log kể cả request bị panic (đã được Recover xử lý)
//  4. Metrics    — đo cả request bị panic, vì panic cũng là độ trễ
//  5. Security   — header bảo mật
//  6. CORS       — cho phép giao diện ở origin khác gọi API
//  7. MaxBytes   — giới hạn body
func New(
	cfg config.HTTPConfig, log *slog.Logger, handler http.Handler,
	allowedOrigins []string,
) *Server {
	h := Chain(handler,
		RequestID(),
		Recover(log),
		Logging(log),

		// Metrics SAU Recover: request bị panic vẫn phải vào biểu đồ. Một
		// đường đang panic 100% mà biến mất khỏi biểu đồ trông giống hệt
		// một đường không ai gọi.
		Metrics(),

		SecurityHeaders(),
		// CORS SAU SecurityHeaders và TRƯỚC MaxBytes: preflight không có
		// body nên không cần qua MaxBytes, và nó phải trả lời sớm thay vì
		// đi tiếp tới handler.
		CORS(allowedOrigins),
		MaxBytes(cfg.MaxRequestBytes),
	)

	addr := fmt.Sprintf(":%d", cfg.Port)
	return &Server{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           h,
			ReadTimeout:       cfg.ReadTimeout,
			ReadHeaderTimeout: cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
		log:      log,
		shutdown: cfg.ShutdownTimeout,
		addr:     addr,
	}
}

// Run khởi động server và chờ tới khi ctx bị hủy.
//
// Khi ctx bị hủy, server dừng nhận request mới và chờ request đang xử lý
// hoàn tất trong ShutdownTimeout — tránh cắt ngang giao dịch đang chạy dở,
// điều có thể để lại trạng thái nửa vời trong database.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("lắng nghe %s: %w", s.addr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("HTTP server đang chạy", "addr", s.addr)
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server dừng bất thường: %w", err)
	case <-ctx.Done():
		s.log.Info("nhận tín hiệu dừng, đang tắt server",
			"timeout", s.shutdown.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdown)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		// Hết thời gian chờ: buộc đóng. Request đang xử lý bị cắt ngang.
		s.log.Error("tắt server không kịp thời hạn, buộc đóng", "error", err)
		_ = s.httpServer.Close()
		return fmt.Errorf("tắt server: %w", err)
	}

	s.log.Info("server đã tắt hoàn toàn")
	return nil
}

// Addr trả về địa chỉ đang lắng nghe (hữu ích khi port = 0 trong test).
func (s *Server) Addr() string { return s.addr }

// HealthChecker là hàm kiểm tra một phụ thuộc.
type HealthChecker func(context.Context) error

// Health tạo handler cho hai endpoint kiểm tra sức khỏe.
//
// PHÂN BIỆT QUAN TRỌNG:
//
//	/health/live  — tiến trình còn sống không → KHÔNG kiểm tra phụ thuộc
//	/health/ready — sẵn sàng nhận request chưa → CÓ kiểm tra database
//
// Nếu `live` cũng kiểm tra database, một sự cố database ngắn sẽ khiến
// bộ điều phối khởi động lại TOÀN BỘ tiến trình — làm sự cố tệ hơn.
func Health(checks map[string]HealthChecker) (live, ready http.Handler) {
	live = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = apierror.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	ready = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		results := make(map[string]string, len(checks))
		healthy := true
		for name, check := range checks {
			if err := check(ctx); err != nil {
				results[name] = "lỗi: " + err.Error()
				healthy = false
				continue
			}
			results[name] = "ok"
		}

		status := http.StatusOK
		overall := "ok"
		if !healthy {
			status = http.StatusServiceUnavailable
			overall = "không sẵn sàng"
		}

		_ = apierror.WriteJSON(w, status, map[string]any{
			"status": overall,
			"checks": results,
		})
	})

	return live, ready
}
