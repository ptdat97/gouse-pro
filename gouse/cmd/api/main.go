// Command api là tiến trình phục vụ HTTP request.
//
// Kiến trúc: modular monolith. Tiến trình này và cmd/worker dùng CHUNG
// codebase, chung phiên bản, chung database — chỉ khác điểm khởi chạy.
// Đây KHÔNG phải microservices.
//
// Tách tiến trình vì tác vụ nền nặng (tổng hợp tín hiệu nhu cầu, tạo báo cáo)
// không được làm chậm request của khách hàng.
//
// Xem docs/09-operations/deployment.md mục 3.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/fashion-commerce/platform/internal/modules/catalog"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// version được đặt lúc build qua -ldflags.
var version = "dev"

func main() {
	if err := run(); err != nil {
		// Ghi ra stderr vì logger có thể chưa khởi tạo được.
		fmt.Fprintf(os.Stderr, "api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log.Level, cfg.Log.Format).With(
		"service", "api",
		"version", version,
		"env", string(cfg.Env),
	)

	log.Info("đang khởi động")

	// Dừng khi nhận SIGINT/SIGTERM để tắt có kiểm soát.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Khởi tạo các module nghiệp vụ. Mỗi module tự quản lý phụ thuộc bên
	// trong; main chỉ biết điểm vào công khai của nó.
	catalogModule, err := catalog.New(catalog.Config{Storage: cfg.Modules.Storage})
	if err != nil {
		return err
	}

	if cfg.Modules.Storage == "memory" {
		// Kho in-memory rỗng lúc khởi động — không nạp dữ liệu mẫu thì mọi
		// endpoint trả 404 và không kiểm chứng được gì.
		seeded, err := catalog.SeedDemo(ctx, catalogModule)
		if err != nil {
			return fmt.Errorf("nạp dữ liệu mẫu catalog: %w", err)
		}
		// Ghi ID ra log: ULID sinh mới mỗi lần khởi động nên không gọi thử
		// được nếu không biết chúng.
		log.Warn("đang dùng kho in-memory với dữ liệu mẫu — chỉ dành cho môi trường phát triển",
			"brand_id", seeded.OwnBrandID,
			"third_party_brand_id", seeded.ThirdPartyID,
			"collection_id", seeded.LaunchedColID,
			"unlaunched_collection_id", seeded.UnlaunchedColID,
		)
	}

	mux := http.NewServeMux()
	registerRoutes(mux, cfg, log, catalogModule)

	srv := httpserver.New(cfg.HTTP, log, mux)
	return srv.Run(ctx)
}

// registerRoutes gắn các route vào mux.
//
// Khi có module nghiệp vụ, mỗi module tự đăng ký route của mình ở đây —
// đó là điểm nối duy nhất giữa main và các module.
func registerRoutes(
	mux *http.ServeMux,
	cfg *config.Config,
	log *slog.Logger,
	catalogModule *catalog.Module,
) {
	// Module catalog tự đăng ký route của mình. main không biết đường dẫn
	// hay hình dạng response của catalog — nó chỉ trao mux.
	catalogModule.RegisterRoutes(mux, log)

	// Danh sách kiểm tra sức khỏe. Hiện chưa có phụ thuộc ngoài; khi thêm
	// database, đăng ký check ở đây.
	checks := map[string]httpserver.HealthChecker{}

	live, ready := httpserver.Health(checks)
	mux.Handle("GET /health/live", live)
	mux.Handle("GET /health/ready", ready)

	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		_ = apierror.WriteJSON(w, http.StatusOK, map[string]string{
			"version": version,
			"env":     string(cfg.Env),
		})
	})

	// Mọi đường dẫn khác trả lỗi đúng định dạng đặc tả, không phải trang
	// 404 mặc định của Go.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requestID := logger.RequestIDFromContext(r.Context())
		apierror.Write(w, r,
			apierror.Newf(apierror.CodeNotFound, "Không tìm thấy %s %s", r.Method, r.URL.Path),
			requestID, log)
	})
}
