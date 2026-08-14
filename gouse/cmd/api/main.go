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

	"github.com/fashion-commerce/platform/internal/modules/cart"
	"github.com/fashion-commerce/platform/internal/modules/catalog"
	"github.com/fashion-commerce/platform/internal/modules/checkout"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/modules/pricing"
	"github.com/fashion-commerce/platform/internal/modules/product"
	"github.com/fashion-commerce/platform/internal/modules/seller"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
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

	// Kết nối database khi dùng kho lưu trữ postgres.
	//
	// Mở TRƯỚC khi khởi tạo module: nếu database không sẵn sàng thì thà
	// không khởi động còn hơn khởi động rồi đổ vỡ ở request đầu tiên của
	// khách hàng.
	var db *database.DB
	if cfg.Modules.Storage == "postgres" {
		db, err = database.Open(ctx, database.Config{DSN: cfg.Database.DSN})
		if err != nil {
			return err
		}
		defer db.Close()
		log.Info("đã kết nối database")
	}

	// Khởi tạo các module nghiệp vụ. Mỗi module tự quản lý phụ thuộc bên
	// trong; main chỉ biết điểm vào công khai của nó.
	catalogModule, err := catalog.New(catalog.Config{
		Storage: cfg.Modules.Storage,
		DB:      db,
	})
	if err != nil {
		return err
	}

	// Module product PHỤ THUỘC catalog: nó gọi catalog để kiểm tra thương
	// hiệu và quyền bán (hàng rào chống hàng giả). Thứ tự khởi tạo phản ánh
	// đúng đồ thị phụ thuộc ở docs/10-roadmap/deliverables.md mục 4.
	//
	// product nhận catalogModule qua interface công khai catalog.API — nó
	// KHÔNG cầm được *application.Service của catalog.
	productModule, err := product.New(product.Config{
		Storage: cfg.Modules.Storage,
		Catalog: catalogModule,
		DB:      db,
	})
	if err != nil {
		return err
	}

	// Module pricing KHÔNG phụ thuộc module nào khác: nó chỉ giữ giá theo
	// sku_id và không cần biết sản phẩm là gì. Đây là lý do nó nằm cùng
	// tầng "dữ liệu chính" với catalog và product trong đồ thị phụ thuộc.
	pricingModule, err := pricing.New(pricing.Config{
		Storage: cfg.Modules.Storage,
		DB:      db,
	})
	if err != nil {
		return err
	}

	// Module inventory CHỈ chạy với PostgreSQL: cơ chế cốt lõi của nó là
	// khóa lạc quan, thứ không kiểm chứng được bằng bộ nhớ. Với kho
	// in-memory, module này đơn giản là KHÔNG được khởi tạo — thà thiếu
	// tính năng còn hơn có một bản giả tạo cảm giác an toàn sai.
	var (
		inventoryModule   *inventory.Module
		marketplaceModule *marketplace.Module
		paymentModule     *payment.Module
		orderModule       *order.Module
		cartModule        *cart.Module
		checkoutModule    *checkout.Module
		fulfillmentModule *fulfillment.Module
	)
	if cfg.Modules.Storage == "postgres" {
		inventoryModule, err = inventory.New(inventory.Config{
			Storage: "postgres",
			DB:      db,
		})
		if err != nil {
			return err
		}
		// Chỉ báo giám sát: reservation quá hạn chưa dọn.
		//
		// Con số này tăng dần nghĩa là tiến trình dọn đã ngừng chạy, và
		// hàng đang bị khóa mà không ai biết — cuối cùng không bán được gì
		// (docs/04-modules/inventory.md mục 6.3 và 13).
		pending, err := inventoryModule.Service().CountExpiredPending(ctx)
		if err != nil {
			return fmt.Errorf("kiểm tra giữ hàng quá hạn: %w", err)
		}
		log.Info("module inventory đã sẵn sàng (khóa lạc quan)",
			"reservation_qua_han_chua_don", pending)

		// seller và marketplace cũng cần PostgreSQL. Thứ tự khởi tạo phản
		// ánh đồ thị phụ thuộc: marketplace gọi cả bốn module kia.
		sellerModule, err := seller.New(seller.Config{Storage: "postgres", DB: db})
		if err != nil {
			return err
		}

		// Own brand là một seller INTERNAL, không phải đường đi riêng —
		// nhờ vậy đơn lẫn own brand và hàng seller đi CHUNG một luồng.
		ownBrand, err := sellerModule.Service().EnsureInternalSeller(
			ctx, "Lumière", seller.InternalSellerSlug)
		if err != nil {
			return fmt.Errorf("tạo seller nội bộ: %w", err)
		}

		marketplaceModule, err = marketplace.New(marketplace.Config{
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
		log.Info("module seller và marketplace đã sẵn sàng",
			"own_brand_seller_id", ownBrand.ID().String())

		// payment và order cũng CHỈ chạy với PostgreSQL, vì cùng một lý do
		// nhưng ở hai chỗ khác nhau: sổ cái cần trigger chặn UPDATE/DELETE,
		// còn đơn hàng cần ràng buộc UNIQUE trên khóa idempotency. Cả hai
		// đều là thứ chỉ database cưỡng chế được dưới tải song song.
		paymentModule, err = payment.New(payment.Config{Storage: "postgres", DB: db})
		if err != nil {
			return err
		}

		// Rà soát sổ sách ngay lúc khởi động: Σ DEBIT phải bằng Σ CREDIT.
		// Lệch nghĩa là có bút toán không cân bằng đã lọt vào database —
		// sự cố nghiêm trọng, cần biết ngay chứ không đợi job hàng ngày.
		integrity, err := paymentModule.CheckIntegrity(ctx)
		if err != nil {
			return fmt.Errorf("rà soát toàn vẹn sổ sách: %w", err)
		}
		if !integrity.IsHealthy {
			log.Error("SỔ SÁCH KHÔNG CÂN BẰNG",
				"tong_ghi_no", integrity.TotalDebit,
				"tong_ghi_co", integrity.TotalCredit,
				"chenh_lech", integrity.Difference,
				"but_toan_lech", integrity.UnbalancedEntryIDs)
		}

		orderModule, err = order.New(order.Config{Storage: "postgres", DB: db})
		if err != nil {
			return err
		}
		log.Info("module payment và order đã sẵn sàng",
			"but_toan_da_ra_soat", integrity.CheckedEntries,
			"so_sach_can_bang", integrity.IsHealthy)

		// cart cần bốn module để đồng bộ giá và tình trạng hàng.
		//
		// LƯU Ý khi đọc đoạn này: KHÔNG có inventory.Reserve ở đây. Giỏ
		// hàng chỉ ĐỌC tồn kho để hiển thị, không giữ chỗ — giữ chỗ ở giỏ
		// nghĩa là khách bỏ quên hai tuần thì hàng khóa hai tuần.
		cartModule, err = cart.New(cart.Config{
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
		log.Info("module cart đã sẵn sàng (giỏ KHÔNG giữ tồn kho)")

		// checkout là module ĐIỀU PHỐI: nó gọi bốn module trên và là nơi
		// DUY NHẤT trong hệ thống khóa tồn kho cho khách.
		// API GHI event vào outbox; WORKER phát chúng đi.
		//
		// Tách hai việc là chủ ý: request của khách không phải chờ chín
		// bên nhận xử lý xong, và một bên nhận lỗi không làm hỏng việc
		// đặt hàng.
		checkoutModule, err = checkout.New(checkout.Config{
			Storage:     "postgres",
			DB:          db,
			Cart:        cartModule,
			Inventory:   inventoryModule,
			Marketplace: marketplaceModule,
			Order:       orderModule,
			Events:      eventbus.NewOutbox(db.Pool()),
		})
		if err != nil {
			return err
		}

		// Chỉ báo giám sát: phiên quá hạn chưa dọn.
		//
		// Con số này tăng dần nghĩa là tiến trình dọn đã ngừng chạy, và
		// hàng đang bị khóa cho những phiên đã chết — cuối cùng không bán
		// được gì dù kho vẫn đầy.
		stalePending, err := checkoutModule.CountExpiredPending(ctx)
		if err != nil {
			return fmt.Errorf("kiểm tra phiên thanh toán quá hạn: %w", err)
		}
		// Chỉ báo giám sát: event chưa phát và event đã bỏ cuộc.
		//
		// Dead letter khác 0 nghĩa là có sự thật nghiệp vụ không tới được
		// bên nhận — đơn đã đặt mà tồn kho chưa cập nhật.
		outboxStats, err := eventbus.NewOutbox(db.Pool()).Stats(ctx)
		if err != nil {
			return fmt.Errorf("kiểm tra outbox: %w", err)
		}
		if outboxStats.DeadLettered > 0 {
			log.Error("CÓ EVENT KHÔNG PHÁT ĐƯỢC",
				"số_lượng", outboxStats.DeadLettered,
				"gợi_ý", "xem cột last_error trong bảng event_outbox")
		}

		// fulfillment là góc nhìn vận hành: seller làm việc với module này,
		// KHÔNG với order — Order chứa dòng hàng của mọi seller trong đơn.
		fulfillmentModule, err = fulfillment.New(fulfillment.Config{
			Storage: "postgres",
			DB:      db,
			Events:  eventbus.NewOutbox(db.Pool()),
		})
		if err != nil {
			return err
		}

		log.Info("module checkout đã sẵn sàng (giữ hàng + đóng băng giá)",
			"phien_qua_han_chua_don", stalePending,
			"event_cho_phat", outboxStats.Pending,
			"event_bo_cuoc", outboxStats.DeadLettered)
	} else {
		log.Warn("bỏ qua inventory, seller, marketplace, payment, order, " +
			"cart, checkout: cần MODULES_STORAGE=postgres")
	}
	_, _, _, _ = paymentModule, cartModule, checkoutModule, fulfillmentModule

	// Nạp dữ liệu mẫu ở môi trường phát triển.
	//
	// Với in-memory: bắt buộc, vì kho rỗng lúc khởi động và không nạp thì
	// mọi endpoint trả 404.
	//
	// Với postgres: chỉ nạp LẦN ĐẦU. SeedDemo tự kiểm tra và bỏ qua nếu đã
	// có dữ liệu — nạp lại sẽ vi phạm UNIQUE và làm tiến trình chết.
	if cfg.Env == config.EnvDevelopment {
		seeded, err := catalog.SeedDemo(ctx, catalogModule)
		if err != nil {
			return fmt.Errorf("nạp dữ liệu mẫu catalog: %w", err)
		}
		if seeded.AlreadySeeded {
			log.Info("database đã có dữ liệu, bỏ qua nạp dữ liệu mẫu")
		} else {

			// Sản phẩm mẫu phải trỏ tới thương hiệu và danh mục vừa tạo ở
			// trên — nếu bịa định danh, hàng rào kiểm tra thương hiệu sẽ
			// chặn ngay.
			seededProducts, err := product.SeedDemo(ctx, productModule, product.SeedInput{
				BrandID:      seeded.OwnBrandID,
				CollectionID: seeded.LaunchedColID,
				CategoryID:   seeded.MenTopsCategoryID,
			})
			if err != nil {
				return fmt.Errorf("nạp dữ liệu mẫu product: %w", err)
			}

			// Giá gắn với SKU CÓ THẬT vừa tạo ở trên.
			seededPrices, err := pricing.SeedDemo(ctx, pricingModule, pricing.SeedInput{
				SKUIDs: seededProducts.SKUIDs,
			})
			if err != nil {
				return fmt.Errorf("nạp dữ liệu mẫu pricing: %w", err)
			}

			// Ghi ID ra log: ULID sinh mới mỗi lần nạp nên không gọi thử
			// được nếu không biết chúng.
			log.Warn("đã nạp dữ liệu mẫu — chỉ dành cho môi trường phát triển",
				"storage", cfg.Modules.Storage,
				"brand_id", seeded.OwnBrandID,
				"third_party_brand_id", seeded.ThirdPartyID,
				"collection_id", seeded.LaunchedColID,
				"unlaunched_collection_id", seeded.UnlaunchedColID,
				"category_id", seeded.MenTopsCategoryID,
				"product_id", seededProducts.ActiveProductID,
				"draft_product_id", seededProducts.DraftProductID,
				"sku_code", seededProducts.SKUCode,
				"priced_sku_id", seededPrices.PricedSKUID,
				"clearance_sku_id", seededPrices.ClearanceSKUID,
			)
		}
	}

	mux := http.NewServeMux()
	registerRoutes(mux, cfg, log, db, catalogModule, productModule)

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
	db *database.DB,
	catalogModule *catalog.Module,
	productModule *product.Module,
) {
	// Mỗi module tự đăng ký route của mình. main không biết đường dẫn hay
	// hình dạng response của module nào — nó chỉ trao mux.
	catalogModule.RegisterRoutes(mux, log)
	productModule.RegisterRoutes(mux, log)

	// Danh sách kiểm tra sức khỏe cho endpoint `ready`.
	//
	// Database CHỈ nằm ở `ready`, KHÔNG ở `live`: nếu `live` cũng kiểm tra
	// database thì một sự cố database ngắn sẽ khiến bộ điều phối khởi động
	// lại toàn bộ tiến trình — làm sự cố nặng thêm thay vì nhẹ đi.
	checks := map[string]httpserver.HealthChecker{}
	if db != nil {
		checks["database"] = db.Ping
	}

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
