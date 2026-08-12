// Command worker xử lý tác vụ nền: phát event từ outbox, job định kỳ.
//
// Dùng CHUNG codebase, phiên bản và database với cmd/api — chỉ khác điểm
// khởi chạy. Đây KHÔNG phải microservices.
//
// Tách tiến trình vì:
//   - Tác vụ nền nặng không được làm chậm request của khách
//   - Outbox publisher cần chạy liên tục, độc lập với lưu lượng HTTP
//   - Mở rộng quy mô độc lập với API
//
// Xem docs/09-operations/deployment.md mục 3.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log.Level, cfg.Log.Format).With(
		"service", "worker",
		"version", version,
		"env", string(cfg.Env),
	)
	log.Info("đang khởi động")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Vòng lặp chính. Khi có outbox và job định kỳ, chúng được đăng ký ở đây.
	//
	// Nhịp hiện tại chỉ để tiến trình sống và phản hồi tín hiệu dừng —
	// giữ chỗ cho outbox publisher ở bước tiếp theo.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Info("worker sẵn sàng")
	for {
		select {
		case <-ctx.Done():
			log.Info("nhận tín hiệu dừng, đang hoàn tất công việc dở")
			// Khi có outbox: chờ lô đang xử lý hoàn tất ở đây, tránh
			// để event ở trạng thái nửa vời.
			log.Info("worker đã dừng")
			return nil
		case <-ticker.C:
			log.Debug("nhịp worker")
		}
	}
}
