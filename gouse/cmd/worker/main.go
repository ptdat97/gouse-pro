// Command worker xử lý tác vụ nền: job định kỳ, và sau này là phát event
// từ outbox.
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
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/analytics"
	"github.com/fashion-commerce/platform/internal/modules/cart"
	"github.com/fashion-commerce/platform/internal/modules/catalog"
	"github.com/fashion-commerce/platform/internal/modules/checkout"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	"github.com/fashion-commerce/platform/internal/modules/notification"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/modules/product"
	"github.com/fashion-commerce/platform/internal/modules/seller"
	"github.com/fashion-commerce/platform/internal/modules/supplychain"
	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
)

var version = "dev"

// Nhịp chạy các job định kỳ.
const (
	// expireReservationsInterval là nhịp dọn giữ hàng quá hạn.
	//
	// 30 giây theo khuyến nghị ở docs/04-modules/inventory.md mục 6.3.
	// Nhanh hơn thì tốn truy vấn vô ích; chậm hơn thì hàng nằm khóa lâu
	// sau khi khách đã bỏ checkout, và khách khác không mua được.
	expireReservationsInterval = 30 * time.Second

	// expireReservationsBatch giới hạn số bản ghi xử lý mỗi lượt.
	//
	// Xử lý theo lô để một đợt tồn đọng lớn không tạo ra giao dịch khổng
	// lồ giữ khóa lâu — lượt sau sẽ dọn tiếp phần còn lại.
	expireReservationsBatch = 200

	// expiredPendingAlert là ngưỡng cảnh báo (mục 13 của đặc tả).
	//
	// Con số vượt ngưỡng nghĩa là tốc độ dọn không theo kịp tốc độ hết
	// hạn, hoặc có bản ghi bị kẹt — cả hai đều dẫn tới hàng bị khóa dần.
	expiredPendingAlert = 100

	// expireCheckoutsInterval là nhịp dọn phiên thanh toán quá hạn.
	//
	// Chạy DÀY HƠN việc dọn reservation (30 giây) một chút không cần
	// thiết: hai job dọn hai đầu của cùng một sợi dây. Phiên hết hạn thì
	// reservation của nó cũng hết hạn, nên dù job này chậm thì job kia
	// vẫn nhả hàng đúng giờ.
	//
	// Việc dọn phiên chủ yếu để trạng thái phiên phản ánh đúng thực tế —
	// khách quay lại phải thấy "đã hết hạn", không phải "đang chờ thanh
	// toán" cho một phiên mà hàng đã bị nhả.
	expireCheckoutsInterval = 60 * time.Second

	expireCheckoutsBatch = 200

	// completeDeliveredInterval là nhịp chuyển đơn đã giao sang COMPLETED.
	//
	// THƯA (10 phút) vì nó chỉ chạy trên đơn đã giao QUÁ 7 NGÀY — không có
	// gì gấp. Chạy dày chỉ tạo truy vấn vô ích.
	completeDeliveredInterval = 10 * time.Minute

	completeDeliveredBatch = 200

	// dispatchEventsInterval là nhịp phát domain event từ outbox.
	//
	// DÀY NHẤT trong các job: mỗi event chờ ở đây là một việc chưa xảy ra
	// mà lẽ ra đã phải xảy ra — tồn kho chưa chuyển sang Committed, email
	// xác nhận chưa gửi.
	//
	// 5 giây là điểm cân bằng: đủ nhanh để khách không thấy độ trễ, đủ thưa
	// để không tạo truy vấn vô ích khi hệ thống rảnh.
	dispatchEventsInterval = 5 * time.Second

	dispatchEventsBatch = 100

	// outboxLagAlert là ngưỡng cảnh báo độ trễ.
	//
	// Vượt ngưỡng nghĩa là bộ phát đã chết hoặc không theo kịp — và hệ quả
	// nghiêm trọng nhất là hàng đã bán vẫn nằm ở Reserved, nơi tiến trình
	// dọn có thể nhả nó và bán cho khách khác.
	outboxLagAlert = 60 * time.Second

	// computeMetricsInterval là nhịp tính lại chỉ số phân tích.
	//
	// THƯA NHẤT (5 phút) có chủ ý. Chỉ số là BẢN SAO ĐỌC, không phải nguồn
	// sự thật: "GMV hôm nay" trễ vài phút không ảnh hưởng tới ai, còn tính
	// lại mỗi giây là quét bảng sự kiện liên tục cho một con số hiển thị.
	//
	// Xem docs/04-modules/analytics.md mục 4.
	computeMetricsInterval = 5 * time.Minute
)

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

	// Worker CHỈ chạy được với PostgreSQL: nó thao tác trên dữ liệu dùng
	// chung với cmd/api. Với kho in-memory, hai tiến trình có hai bản dữ
	// liệu riêng và worker sẽ dọn một kho rỗng.
	if cfg.Modules.Storage != "postgres" {
		return errors.New("worker: cần MODULES_STORAGE=postgres — " +
			"tiến trình nền phải dùng chung dữ liệu với API")
	}

	db, err := database.Open(ctx, database.Config{DSN: cfg.Database.DSN})
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("đã kết nối database")

	inventoryModule, err := inventory.New(inventory.Config{Storage: "postgres", DB: db})
	if err != nil {
		return err
	}

	// checkout cần bốn module để khởi tạo. Worker dựng chúng chỉ để có
	// checkoutModule — nó không tự chạy job nào.
	sellerModule, err := seller.New(seller.Config{Storage: "postgres", DB: db})
	if err != nil {
		return err
	}
	catalogModule, err := catalog.New(catalog.Config{Storage: "postgres", DB: db})
	if err != nil {
		return err
	}
	productModule, err := product.New(product.Config{
		Storage: "postgres",
		Catalog: catalogModule,
		DB:      db,
	})
	if err != nil {
		return err
	}
	marketplaceModule, err := marketplace.New(marketplace.Config{
		Storage:   "postgres",
		DB:        db,
		Catalog:   catalogModule,
		Product:   productModule,
		Seller:    sellerModule,
		Inventory: inventoryModule,
	})
	if err != nil {
		return err
	}
	orderModule, err := order.New(order.Config{Storage: "postgres", DB: db})
	if err != nil {
		return err
	}
	cartModule, err := cart.New(cart.Config{
		Storage:     "postgres",
		DB:          db,
		Marketplace: marketplaceModule,
		Product:     productModule,
		Seller:      sellerModule,
		Inventory:   inventoryModule,
		Events:      eventbus.NewOutbox(db.Pool()),
	})
	if err != nil {
		return err
	}

	// fulfillment là GÓC NHÌN VẬN HÀNH của đơn hàng: nó tách đơn theo nguồn
	// hàng và theo dõi tiến trình giao.
	fulfillmentModule, err := fulfillment.New(fulfillment.Config{
		Storage: "postgres",
		DB:      db,
		Events:  eventbus.NewOutbox(db.Pool()),
	})
	if err != nil {
		return err
	}

	// notification gửi email giao dịch.
	//
	// Chưa cấu hình Senders nên nó dùng bộ GHI-LOG: nội dung được ghi ra
	// nhật ký thay vì gửi đi. Nhờ vậy luồng chạy được đầu-cuối trước khi
	// nền tảng ký hợp đồng với nhà cung cấp dịch vụ email.
	notificationModule, err := notification.New(notification.Config{
		Storage: "postgres",
		DB:      db,
		Log:     log,
	})
	if err != nil {
		return err
	}

	// supply-chain ở MVP CHỈ ghi tín hiệu nhu cầu — chưa dự báo, chưa lập
	// kế hoạch. Nhưng phải có từ MVP vì dữ liệu lịch sử không tạo ngược được.
	supplyModule, err := supplychain.New(supplychain.Config{
		Storage: "postgres",
		DB:      db,
	})
	if err != nil {
		return err
	}

	// analytics ghi số liệu và tính chỉ số.
	//
	// KHÔNG PHẢI NGUỒN SỰ THẬT: "seller A được trả bao nhiêu" vẫn do
	// `payment` trả lời. Module này chỉ trả lời "GMV tháng này bao nhiêu",
	// và chấp nhận trễ vài phút.
	analyticsModule, err := analytics.New(analytics.Config{
		Storage: "postgres",
		DB:      db,
		Log:     log,
	})
	if err != nil {
		return err
	}
	checkoutModule, err := checkout.New(checkout.Config{
		Storage:     "postgres",
		DB:          db,
		Cart:        cartModule,
		Inventory:   inventoryModule,
		Marketplace: marketplaceModule,
		Order:       orderModule,
		Seller:      sellerModule,
		Events:      eventbus.NewOutbox(db.Pool()),
	})
	if err != nil {
		return err
	}

	// Bus event: bộ phát đọc outbox và đưa event tới các bên nhận.
	//
	// Đăng ký bên nhận Ở ĐÂY, tại điểm khởi chạy — không phải bên trong
	// module. Nhờ vậy module phát event không biết ai nghe, và thêm bên
	// nghe mới chỉ sửa file này.
	bus := eventbus.NewDispatcher(db.Pool(), log)
	bus.Subscribe(inventory.NewCommitHandler(inventoryModule, log))
	bus.Subscribe(inventory.NewReleaseHandler(
		inventoryModule, &sellerOwner{sellers: sellerModule}, log))
	bus.Subscribe(supplychain.NewSignalHandler(supplyModule))
	bus.Subscribe(fulfillment.NewSplitHandler(fulfillmentModule, log))
	bus.Subscribe(order.NewProgressHandler(orderModule, log))
	bus.Subscribe(notification.NewOrderNotifier(notificationModule, log))
	bus.Subscribe(analytics.NewEventRecorder(analyticsModule))

	jobs := []job{
		{
			name:     "phát domain event",
			interval: dispatchEventsInterval,
			run:      dispatchEvents(bus, log),
		},
		{
			name:     "dọn giữ hàng quá hạn",
			interval: expireReservationsInterval,
			run:      expireReservations(inventoryModule, log),
		},
		{
			name:     "dọn phiên thanh toán quá hạn",
			interval: expireCheckoutsInterval,
			run:      expireCheckouts(checkoutModule, log),
		},
		{
			name:     "hoàn tất đơn đã giao quá hạn đổi trả",
			interval: completeDeliveredInterval,
			run:      completeDelivered(fulfillmentModule, log),
		},
		{
			name:     "tính chỉ số phân tích",
			interval: computeMetricsInterval,
			run:      computeMetrics(analyticsModule, log),
		},
	}

	// Worker phơi /metrics của RIÊNG nó.
	//
	// # Vì sao không dùng chung endpoint với API
	//
	// Đây là hai TIẾN TRÌNH khác nhau, và chỉ số là của tiến trình. Các
	// gauge về outbox được ĐẶT ở đây — API không biết chúng. Gộp vào một
	// endpoint nghĩa là hoặc mất số liệu, hoặc phải đẩy qua lại giữa hai
	// tiến trình, và cả hai đều tệ hơn việc thu thập từ hai đích.
	//
	// Cổng riêng, và ở production phải chặn khỏi internet.
	go serveMetrics(ctx, metricsPort(), log)

	return runJobs(ctx, log, jobs)
}

// metricsPort đọc cổng chỉ số của worker.
//
// Đọc thẳng biến môi trường thay vì thêm trường vào `config.Config`: đây
// là núm vặn VẬN HÀNH của riêng tiến trình này, không phải cấu hình dùng
// chung. Nhét vào Config sẽ bắt cả API mang theo một trường nó không dùng.
func metricsPort() int {
	if raw := os.Getenv("WORKER_METRICS_PORT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n < 65536 {
			return n
		}
	}
	return 9091
}

// serveMetrics chạy máy chủ chỉ số của worker.
//
// Lỗi ở đây KHÔNG làm dừng worker: mất bảng theo dõi là chuyện đáng lo,
// còn dừng tiến trình xử lý event vì không mở được cổng theo dõi thì biến
// một sự cố nhỏ thành sự cố lớn.
func serveMetrics(ctx context.Context, port int, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(
		metrics.Registry, promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
		}))

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("máy chủ chỉ số của worker đang chạy", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("máy chủ chỉ số dừng", "error", err)
	}
}

// dispatchEvents phát domain event từ outbox tới các bên nhận.
//
// Đây là thứ biến Transactional Outbox thành sự thật: không có job này,
// event nằm mãi trong bảng và không việc gì xảy ra sau khi đặt hàng.
func dispatchEvents(bus *eventbus.Dispatcher, log *slog.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		daPhat, err := bus.DispatchBatch(ctx, dispatchEventsBatch)
		if err != nil {
			return fmt.Errorf("phát domain event: %w", err)
		}
		if daPhat > 0 {
			log.Info("đã phát domain event", "số_lượng", daPhat)
		}

		stats, err := bus.Outbox().Stats(ctx)
		if err != nil {
			return fmt.Errorf("đọc chỉ số outbox: %w", err)
		}

		// Đưa lên bảng theo dõi, không chỉ vào log.
		//
		// Log nói được "lúc 3h07 có 42 event tồn đọng"; nó KHÔNG nói được
		// "tồn đọng đã tăng đều suốt 40 phút". Cảnh báo hữu ích nằm ở ĐỘ
		// TĂNG, và độ tăng chỉ đọc được từ chuỗi thời gian.
		metrics.OutboxPending.Set(float64(stats.Pending))
		metrics.OutboxDeadLettered.Set(float64(stats.DeadLettered))
		metrics.OutboxOldestAgeSeconds.Set(stats.OldestPendingAge.Seconds())

		// Dead letter là sự cố NGHIÊM TRỌNG: có sự thật nghiệp vụ không
		// tới được bên nhận. Đơn đã đặt mà tồn kho chưa cập nhật, hoặc
		// khách không nhận được email xác nhận.
		if stats.DeadLettered > 0 {
			log.Error("CÓ EVENT KHÔNG PHÁT ĐƯỢC — cần người xem",
				"số_lượng", stats.DeadLettered,
				"gợi_ý", "xem cột last_error trong bảng event_outbox")
		}

		if stats.OldestPendingAge > outboxLagAlert {
			log.Warn("độ trễ phát event vượt ngưỡng",
				"độ_trễ", stats.OldestPendingAge.String(),
				"ngưỡng", outboxLagAlert.String(),
				"còn_chờ", stats.Pending,
				"hệ_quả", "hàng đã bán có thể vẫn ở trạng thái đang giữ")
		}
		return nil
	}
}

// completeDelivered chuyển đơn đã giao quá hạn đổi trả sang COMPLETED.
//
// ĐÂY LÀ RANH GIỚI TÀI CHÍNH, không phải bước vận hành:
//
//	DELIVERED  → số dư seller vẫn Pending
//	COMPLETED  → số dư chuyển Available, seller được chi trả
//
// Job này KHÔNG chạy nghĩa là tiền của seller bị giữ lại vô thời hạn —
// hàng đã giao, khách đã hài lòng, nhưng seller không được trả.
// computeMetrics tính lại chỉ số phân tích cho hôm nay và hôm qua.
//
// # Vì sao tính CẢ hôm qua
//
// Event đến muộn là chuyện bình thường: outbox phát mỗi 5 giây, và một
// đơn đặt lúc 23:59:58 có thể được ghi vào nhật ký sau nửa đêm. Nếu chỉ
// tính hôm nay, con số của hôm qua đóng băng ở thời điểm lần chạy cuối
// cùng trước nửa đêm — thiếu đúng những đơn cuối ngày.
//
// Tính lại là AN TOÀN vì Upsert GHI ĐÈ: hai lần tính cùng một ngày cho
// cùng một kết quả, không cộng dồn.
//
// # Vì sao KHÔNG tính cho từng seller ở đây
//
// Sàn có 10.000 gian hàng nghĩa là 20.000 lượt tính mỗi 5 phút. Chỉ số
// toàn sàn là thứ dashboard vận hành cần ngay; chỉ số theo gian hàng sẽ
// tính theo yêu cầu hoặc bằng một job thưa hơn khi có dashboard seller
// (Phase 2). Xem docs/04-modules/analytics.md mục 14.
func computeMetrics(m *analytics.Module, log *slog.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		now := time.Now().UTC()

		for _, ngay := range []time.Time{now, now.AddDate(0, 0, -1)} {
			if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
				PeriodStart: ngay,
				Granularity: analytics.GranularityDay,
			}); err != nil {
				return fmt.Errorf("tính chỉ số ngày %s: %w",
					ngay.Format("2006-01-02"), err)
			}
		}

		// Ghi lại GMV hôm nay để nhật ký vận hành có một con số theo dõi
		// được mà không cần mở dashboard.
		gmv, err := m.GetMetric(ctx, analytics.MetricRequest{
			Name:        analytics.MetricGMV,
			PeriodStart: now,
			Granularity: analytics.GranularityDay,
		})
		if err != nil {
			return fmt.Errorf("đọc chỉ số GMV: %w", err)
		}
		log.Info("đã tính chỉ số phân tích",
			"gmv_hôm_nay", gmv.Value,
			"số_đơn", gmv.SampleSize,
			"lưu_ý", "số liệu phân tích KHÔNG dùng cho quyết định tài chính")

		return nil
	}
}

func completeDelivered(m *fulfillment.Module, log *slog.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		daHoanTat, err := m.CompleteDelivered(ctx, completeDeliveredBatch)
		if err != nil {
			return fmt.Errorf("hoàn tất đơn đã giao: %w", err)
		}
		if daHoanTat > 0 {
			log.Info("đã hoàn tất đơn quá hạn đổi trả — số dư seller chuyển Available",
				"số_lượng", daHoanTat)
		}
		return nil
	}
}

// expireCheckouts dọn phiên thanh toán quá hạn và NHẢ HÀNG.
//
// Đây là hàm biến lời hứa "giữ hàng có thời hạn" thành sự thật. Không có
// nó thì mọi phiên khách bỏ dở đều khóa hàng cho tới khi có người phát
// hiện thủ công — và không ai đi tìm cho tới lúc hết hàng bán dù kho đầy.
func expireCheckouts(m *checkout.Module, log *slog.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		daDon, err := m.ExpireStale(ctx, expireCheckoutsBatch)
		if err != nil {
			return fmt.Errorf("dọn phiên thanh toán quá hạn: %w", err)
		}
		if daDon > 0 {
			log.Info("đã dọn phiên thanh toán quá hạn và nhả hàng",
				"số_lượng", daDon)
		}

		conLai, err := m.CountExpiredPending(ctx)
		if err != nil {
			return fmt.Errorf("đếm phiên thanh toán quá hạn: %w", err)
		}
		if conLai > expiredPendingAlert {
			log.Warn("tồn đọng phiên thanh toán quá hạn vượt ngưỡng",
				"còn_lại", conLai,
				"ngưỡng", expiredPendingAlert,
				"gợi_ý", "khách quay lại sẽ thấy phiên còn sống dù hàng đã bị nhả")
		}
		return nil
	}
}

// job là một tác vụ chạy lặp theo nhịp.
type job struct {
	name     string
	interval time.Duration
	run      func(context.Context) error
}

// runJobs chạy mọi job theo nhịp riêng cho tới khi nhận tín hiệu dừng.
//
// Mỗi job chạy TUẦN TỰ trong vòng lặp của nó: một lượt chưa xong thì lượt
// sau chờ. Chạy chồng lấn sẽ khiến hai lượt cùng dọn một bản ghi và tạo
// xung đột không cần thiết.
func runJobs(ctx context.Context, log *slog.Logger, jobs []job) error {
	done := make(chan struct{})
	var running int

	for _, j := range jobs {
		running++
		go func(j job) {
			defer func() { done <- struct{}{} }()

			ticker := time.NewTicker(j.interval)
			defer ticker.Stop()

			jobLog := log.With("job", j.name)
			jobLog.Info("job đã khởi động", "nhịp", j.interval.String())

			// Chạy NGAY một lượt lúc khởi động, không chờ hết nhịp đầu:
			// nếu worker vừa khởi động lại sau sự cố, có thể đang có tồn
			// đọng cần dọn ngay.
			runOnce(ctx, jobLog, j)

			for {
				select {
				case <-ctx.Done():
					jobLog.Info("job đã dừng")
					return
				case <-ticker.C:
					runOnce(ctx, jobLog, j)
				}
			}
		}(j)
	}

	log.Info("worker sẵn sàng", "số_job", running)

	<-ctx.Done()
	log.Info("nhận tín hiệu dừng, đang hoàn tất công việc dở")

	// Chờ mọi job hoàn tất lượt đang chạy.
	//
	// Bỏ qua bước này sẽ cắt ngang một giao dịch đang mở và để lại
	// reservation ở trạng thái nửa vời.
	for i := 0; i < running; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			log.Warn("hết thời gian chờ job hoàn tất, dừng cưỡng bức")
			return nil
		}
	}

	log.Info("worker đã dừng")
	return nil
}

// runOnce chạy một lượt của job.
//
// Lỗi được GHI LOG chứ không làm dừng worker: một lượt thất bại (mất kết
// nối database chẳng hạn) không được làm chết tiến trình nền — lượt sau có
// thể thành công.
func runOnce(ctx context.Context, log *slog.Logger, j job) {
	start := time.Now()
	if err := j.run(ctx); err != nil {
		// Ngữ cảnh bị hủy là chuyện bình thường khi đang tắt.
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Error("job thất bại", "error", err, "thời_gian", time.Since(start).String())
		return
	}
	log.Debug("job hoàn tất", "thời_gian", time.Since(start).String())
}

// expireReservations dọn các reservation quá hạn.
//
// VÌ SAO JOB NÀY QUAN TRỌNG (docs/04-modules/inventory.md mục 6.3):
//
// Khách vào checkout thì hàng bị giữ. Nếu họ bỏ ngang và không có gì giải
// phóng, hàng nằm khóa vĩnh viễn. Tích lũy dần, cuối cùng không bán được
// gì — mà tồn kho trên báo cáo vẫn đầy.
//
// Đây là loại sự cố không có thông báo lỗi nào cả: hệ thống vẫn chạy, chỉ
// là doanh số tụt dần. Đó là lý do phải có cảnh báo giám sát.
func expireReservations(m *inventory.Module, log *slog.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		svc := m.Service()

		daDon, err := svc.ExpireReservations(ctx, expireReservationsBatch)
		if err != nil {
			return fmt.Errorf("dọn giữ hàng quá hạn: %w", err)
		}
		if daDon > 0 {
			log.Info("đã giải phóng hàng giữ quá hạn", "số_lượng", daDon)
		}

		// Kiểm tra tồn đọng SAU khi dọn: nếu vẫn còn nhiều, nghĩa là tốc
		// độ dọn không theo kịp hoặc có bản ghi bị kẹt.
		conLai, err := svc.CountExpiredPending(ctx)
		if err != nil {
			return fmt.Errorf("đếm giữ hàng quá hạn: %w", err)
		}
		if conLai > expiredPendingAlert {
			log.Warn("tồn đọng giữ hàng quá hạn vượt ngưỡng — hàng đang bị khóa dần",
				"còn_lại", conLai,
				"ngưỡng", expiredPendingAlert,
				"gợi_ý", "kiểm tra tốc độ dọn và bản ghi bị kẹt")
		}
		return nil
	}
}

// sellerOwner đổi định danh nhà bán lấy chủ sở hữu tồn kho.
//
// Cùng quy tắc và cùng nguồn sự thật với cmd/api: `inventory.OwnerForSeller`
// (ADR-0012). Hai tiến trình phải nhất trí — nếu không, đường ghi của API
// tạo ra bản ghi mà bên nhận event của worker không tìm thấy.
type sellerOwner struct{ sellers seller.API }

var _ inventory.OwnerResolver = (*sellerOwner)(nil)

func (s *sellerOwner) InventoryOwnerID(
	ctx context.Context, sellerID string,
) (string, error) {
	v, err := s.sellers.GetSeller(ctx, sellerID)
	if err != nil {
		return "", err
	}
	return inventory.OwnerForSeller(v.ID, v.IsInternal), nil
}
