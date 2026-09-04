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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fashion-commerce/platform/internal/app"
	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
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

// opsConfigReloadInterval là nhịp nạp lại tham số vận hành.
//
// 30 giây: những tham số này là chính sách kinh doanh, đổi vài lần một
// tháng. Một khoảng lệch nửa phút giữa các bản sao không gây hại, còn nạp
// dày hơn là thêm truy vấn cho một thứ gần như không bao giờ đổi.
const opsConfigReloadInterval = 30 * time.Second

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

	// Kết nối database khi dùng kho lưu trữ postgres.
	//
	// Mở TRƯỚC khi khởi tạo module: nếu database không sẵn sàng thì thà
	// không khởi động còn hơn khởi động rồi đổ vỡ ở request đầu tiên của
	// khách hàng.
	var db *database.DB
	if cfg.Modules.Storage == "postgres" {
		tracer := database.NewQueryTracer("api")
		if err := metrics.Registry.Register(tracer); err != nil {
			return fmt.Errorf("đăng ký chỉ số truy vấn database: %w", err)
		}
		db, err = database.Open(ctx, database.Config{
			DSN:    cfg.Database.DSN,
			Tracer: tracer,
		})
		if err != nil {
			return err
		}
		defer db.Close()
		log.Info("đã kết nối database")

		// Phơi trạng thái pool cho Prometheus.
		//
		// Đăng ký ở GỐC TIẾN TRÌNH chứ không trong app.Build: Build chạy
		// lại ở mỗi bài test tích hợp, và đăng ký cùng một collector nhiều
		// lần vào một registry sẽ lỗi. Gốc tiến trình chạy đúng một lần.
		if err := metrics.Registry.Register(
			database.NewPoolCollector(db, "api")); err != nil {
			return fmt.Errorf("đăng ký chỉ số pool database: %w", err)
		}
	}

	// Dựng module và nối route qua internal/app.
	//
	// main CHỈ còn: đọc cấu hình, mở log, mở database, bắt tín hiệu dừng,
	// chạy server. Phần biết-về-nghiệp-vụ nằm ở `app` để TEST dựng lại
	// được đúng bộ route này — xem tài liệu gói đó.
	m, err := app.Build(ctx, cfg, log, db)
	if err != nil {
		return err
	}

	// Giữ bộ đệm tham số vận hành tươi khi chạy NHIỀU BẢN SAO.
	//
	// Ghi tham số chỉ nạp lại bộ đệm của tiến trình ĐANG GHI. Không có
	// vòng này, một bản sao khác sẽ dùng giá trị cũ MÃI MÃI — tính năng
	// trông như chạy được vì bản vừa đổi thấy đúng, còn những bản kia thì
	// không, và không có gì báo.
	if oc := m.OpsConfig(); oc != nil {
		go oc.ChayNapLaiDinhKy(ctx, opsConfigReloadInterval)
	}

	mux := http.NewServeMux()
	app.RegisterRoutes(mux, cfg, log, db, m, version)

	srv := httpserver.New(cfg.HTTP, log, mux, cfg.Auth.AllowedOrigins)
	return srv.Run(ctx)
}
