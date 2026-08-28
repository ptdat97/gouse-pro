// Package app là GỐC LẮP GHÉP: nơi duy nhất biết tất cả các module và
// nối chúng lại với nhau.
//
// # Vì sao KHÔNG nằm trong internal/platform
//
// Backlog P0-7 từng ghi đích đến là `internal/platform/bootstrap`. Không
// dùng được: quy tắc R3 của archcheck cấm mọi thứ dưới `platform/` import
// module nghiệp vụ, và một gốc lắp ghép thì buộc phải import tất cả.
//
// Đó là ràng buộc ĐÚNG chứ không phải chỗ cần lách. Platform là tầng nền
// dùng chung; nó biết tới `order` hay `seller` là nó thôi trung lập và trở
// thành điểm phụ thuộc của cả hệ thống. Gốc lắp ghép thuộc về tầng TRÊN
// module, không phải tầng dưới.
//
// `internal/app` không phải platform, không phải module — archcheck không
// áp ràng buộc nào lên nó, đúng như một gốc lắp ghép cần.
//
// # Vì sao tách khỏi cmd/api
//
// Để TEST được. Trước khi tách, bộ route nằm trong `package main` nên
// không có cách nào dựng lại nó trong test — mọi test đầu-cuối phải gọi
// thẳng service Go, bỏ qua toàn bộ tầng HTTP: middleware xác thực, phân
// quyền, kiểm tra khóa idempotency, hình dạng JSON, mã trạng thái.
//
// Đó là khoảng trống có hậu quả thật: bốn lỗi lệch giữa đặc tả và cài đặt
// tìm được trong tháng 8 đều nằm ở đúng ranh giới HTTP này.
//
// P0-7 ghi rõ "chỉ làm nếu vướng thật, và phải quyết có ý thức". Việc
// dựng test tích hợp API (PH-3) là chỗ vướng thật đó.
//
// # Ba ràng buộc giữ nguyên từ P0-7
//
//	✗ KHÔNG dùng DI framework
//	✗ KHÔNG tạo trừu tượng hóa không cần thiết
//	✓ Dependency injection tường minh, giữ mẫu New(Config)
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/cart"
	"github.com/fashion-commerce/platform/internal/modules/catalog"
	"github.com/fashion-commerce/platform/internal/modules/checkout"
	"github.com/fashion-commerce/platform/internal/modules/customer"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	inventoryhttp "github.com/fashion-commerce/platform/internal/modules/inventory/interfaces/http"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	markethttp "github.com/fashion-commerce/platform/internal/modules/marketplace/interfaces/http"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/modules/pricing"
	"github.com/fashion-commerce/platform/internal/modules/product"
	"github.com/fashion-commerce/platform/internal/modules/promotion"
	"github.com/fashion-commerce/platform/internal/modules/returns"
	"github.com/fashion-commerce/platform/internal/modules/seller"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/config"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
	"github.com/fashion-commerce/platform/internal/platform/privacy"
	"github.com/fashion-commerce/platform/internal/platform/token"
	"github.com/fashion-commerce/platform/internal/platform/webhook"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Build khởi tạo mọi module nghiệp vụ và nối phụ thuộc giữa chúng.
//
// db có thể nil khi chạy với kho lưu trữ in-memory. Module nào bắt buộc
// cần database sẽ tự từ chối khởi tạo — điều đó tốt hơn là khởi động rồi
// đổ vỡ ở request đầu tiên của khách.
func Build(
	ctx context.Context, cfg *config.Config, log *slog.Logger, db *database.DB,
) (Modules, error) {
	var err error
	_ = err
	// Khởi tạo các module nghiệp vụ. Mỗi module tự quản lý phụ thuộc bên
	// trong; main chỉ biết điểm vào công khai của nó.
	catalogModule, err := catalog.New(catalog.Config{
		Storage: cfg.Modules.Storage,
		DB:      db,
	})
	if err != nil {
		return Modules{}, err
	}

	// Module product PHỤ THUỘC catalog: nó gọi catalog để kiểm tra thương
	// hiệu và quyền bán (hàng rào chống hàng giả). Thứ tự khởi tạo phản ánh
	// đúng đồ thị phụ thuộc ở docs/10-roadmap/deliverables.md mục 4.
	//
	// product nhận catalogModule qua interface công khai catalog.API — nó
	// KHÔNG cầm được *application.Service của catalog.
	// Outbox cấp cho product để phát tín hiệu "tìm không ra kết quả".
	//
	// Chỉ có với PostgreSQL: outbox là một bảng. Với kho in-memory, tìm
	// kiếm vẫn chạy nhưng không ghi tín hiệu — chấp nhận được vì kho
	// in-memory chỉ dùng khi phát triển.
	var searchOutbox *eventbus.Outbox
	if db != nil {
		searchOutbox = eventbus.NewOutbox(db.Pool())
	}

	productModule, err := product.New(product.Config{
		Storage: cfg.Modules.Storage,
		Catalog: catalogModule,
		DB:      db,
		Events:  searchOutbox,
	})
	if err != nil {
		return Modules{}, err
	}

	// Module pricing KHÔNG phụ thuộc module nào khác: nó chỉ giữ giá theo
	// sku_id và không cần biết sản phẩm là gì. Đây là lý do nó nằm cùng
	// tầng "dữ liệu chính" với catalog và product trong đồ thị phụ thuộc.
	pricingModule, err := pricing.New(pricing.Config{
		Storage: cfg.Modules.Storage,
		DB:      db,
	})
	if err != nil {
		return Modules{}, err
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
		customerModule    *customer.Module
		cartModule        *cart.Module
		checkoutModule    *checkout.Module
		fulfillmentModule *fulfillment.Module
		returnsModule     *returns.Module
		promotionModule   *promotion.Module
		identityModule    *identity.Module

		// ownBrandSellerID cần ở bước nạp dữ liệu mẫu bên dưới: offer phải
		// đứng tên một nhà bán CÓ THẬT.
		ownBrandSellerID string
		sellerModule     *seller.Module
		auditRecorder    *audit.Recorder
	)
	if cfg.Modules.Storage == "postgres" {
		// Audit log là năng lực platform (ADR-0011), không phải module —
		// nên nó được khởi tạo ở đây và trao cho những nơi cần ghi vết.
		auditRecorder = audit.NewRecorder(db.Pool())

		// Bộ phát hành access token. Khóa được kiểm tra độ dài ở CẢ config
		// lẫn token — config chặn sớm với thông báo hướng dẫn được, token
		// chặn lần cuối cho mọi đường khởi tạo khác.
		issuer, err := token.NewIssuer(token.Config{Secret: cfg.Auth.JWTSecret})
		if err != nil {
			return Modules{}, err
		}

		// identity CHỈ chạy với PostgreSQL: email duy nhất và token duy
		// nhất dựa vào chỉ mục UNIQUE. Kiểm tra trước khi ghi vẫn lọt khi
		// hai request đăng ký cùng lúc, và hai tài khoản cùng email là bế
		// tắc chỉ quản trị viên gỡ được.
		identityModule, err = identity.New(identity.Config{
			Storage:      "postgres",
			DB:           db,
			Log:          log,
			Issuer:       issuer,
			SecureCookie: cfg.Auth.SecureCookie,
		})
		if err != nil {
			return Modules{}, err
		}
		if !cfg.Auth.SecureCookie {
			log.Warn("cookie refresh token KHÔNG bật cờ Secure — " +
				"chỉ chấp nhận được khi phát triển trên http://localhost")
		}

		inventoryModule, err = inventory.New(inventory.Config{
			Storage: "postgres",
			DB:      db,
		})
		if err != nil {
			return Modules{}, err
		}
		// Chỉ báo giám sát: reservation quá hạn chưa dọn.
		//
		// Con số này tăng dần nghĩa là tiến trình dọn đã ngừng chạy, và
		// hàng đang bị khóa mà không ai biết — cuối cùng không bán được gì
		// (docs/04-modules/inventory.md mục 6.3 và 13).
		pending, err := inventoryModule.Service().CountExpiredPending(ctx)
		if err != nil {
			return Modules{}, fmt.Errorf("kiểm tra giữ hàng quá hạn: %w", err)
		}
		log.Info("module inventory đã sẵn sàng (khóa lạc quan)",
			"reservation_qua_han_chua_don", pending)

		// seller và marketplace cũng cần PostgreSQL. Thứ tự khởi tạo phản
		// ánh đồ thị phụ thuộc: marketplace gọi cả bốn module kia.
		// Audit truyền vào để thao tác nhạy cảm (đình chỉ nhà bán) ghi vết
		// trong CÙNG giao dịch với việc đổi trạng thái.
		// Khóa mã hóa có thể rỗng khi phát triển: module vẫn khởi tạo,
		// nhưng mọi hồ sơ đăng ký kèm tài khoản ngân hàng sẽ bị TỪ CHỐI.
		// Thà không nhận còn hơn lưu số tài khoản ở dạng rõ.
		var boMaHoa *privacy.BoMaHoa
		if cfg.Auth.EncryptionKey != "" {
			boMaHoa, err = privacy.NewBoMaHoa(cfg.Auth.EncryptionKey)
			if err != nil {
				return Modules{}, fmt.Errorf("app: khóa mã hóa: %w", err)
			}
		}

		sellerModule, err = seller.New(seller.Config{
			Storage: "postgres",
			DB:      db,
			Audit:   auditRecorder,
			MaHoa:   boMaHoa,
		})
		if err != nil {
			return Modules{}, err
		}

		// Own brand là một seller INTERNAL, không phải đường đi riêng —
		// nhờ vậy đơn lẫn own brand và hàng seller đi CHUNG một luồng.
		ownBrand, err := sellerModule.Service().EnsureInternalSeller(
			ctx, "Lumière", seller.InternalSellerSlug)
		if err != nil {
			return Modules{}, fmt.Errorf("tạo seller nội bộ: %w", err)
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
			return Modules{}, err
		}
		ownBrandSellerID = ownBrand.ID().String()
		log.Info("module seller và marketplace đã sẵn sàng",
			"own_brand_seller_id", ownBrandSellerID)

		// payment và order cũng CHỈ chạy với PostgreSQL, vì cùng một lý do
		// nhưng ở hai chỗ khác nhau: sổ cái cần trigger chặn UPDATE/DELETE,
		// còn đơn hàng cần ràng buộc UNIQUE trên khóa idempotency. Cả hai
		// đều là thứ chỉ database cưỡng chế được dưới tải song song.
		// Audit truyền vào để điều chỉnh sổ cái ghi vết trong CÙNG giao
		// dịch với bút toán — thao tác nhạy cảm nhất hệ thống.
		paymentModule, err = payment.New(payment.Config{
			Storage: "postgres",
			DB:      db,
			Audit:   auditRecorder,
		})
		if err != nil {
			return Modules{}, err
		}

		// Rà soát sổ sách ngay lúc khởi động: Σ DEBIT phải bằng Σ CREDIT.
		// Lệch nghĩa là có bút toán không cân bằng đã lọt vào database —
		// sự cố nghiêm trọng, cần biết ngay chứ không đợi job hàng ngày.
		integrity, err := paymentModule.CheckIntegrity(ctx)
		if err != nil {
			return Modules{}, fmt.Errorf("rà soát toàn vẹn sổ sách: %w", err)
		}
		if !integrity.IsHealthy {
			log.Error("SỔ SÁCH KHÔNG CÂN BẰNG",
				"tong_ghi_no", integrity.TotalDebit,
				"tong_ghi_co", integrity.TotalCredit,
				"chenh_lech", integrity.Difference,
				"but_toan_lech", integrity.UnbalancedEntryIDs)
		}

		// Audit truyền vào để endpoint quản trị ghi vết: xem chi tiết đơn
		// (dữ liệu cá nhân khách) và hủy đơn.
		orderModule, err = order.New(order.Config{
			Storage: "postgres",
			DB:      db,
			Audit:   auditRecorder,
		})
		if err != nil {
			return Modules{}, err
		}
		log.Info("module payment và order đã sẵn sàng",
			"but_toan_da_ra_soat", integrity.CheckedEntries,
			"so_sach_can_bang", integrity.IsHealthy)

		// customer giữ HỒ SƠ KHÁCH HÀNG — khác với tài khoản đăng nhập của
		// identity. Ở đây nó có một việc: đổi user_id lấy customer_id để
		// giỏ hàng của người đã đăng nhập gắn đúng hồ sơ.
		customerModule, err = customer.New(customer.Config{
			Storage: "postgres",
			DB:      db,

			// Identity để khách ĐĂNG KÝ được: một lần đăng ký sinh ra tài
			// khoản đăng nhập (identity) VÀ hồ sơ mua hàng (customer).
			Identity: identityModule,
		})
		if err != nil {
			return Modules{}, err
		}

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
			return Modules{}, err
		}
		log.Info("module cart đã sẵn sàng (giỏ KHÔNG giữ tồn kho)")

		// checkout là module ĐIỀU PHỐI: nó gọi bốn module trên và là nơi
		// DUY NHẤT trong hệ thống khóa tồn kho cho khách.
		// API GHI event vào outbox; WORKER phát chúng đi.
		//
		// Tách hai việc là chủ ý: request của khách không phải chờ chín
		// bên nhận xử lý xong, và một bên nhận lỗi không làm hỏng việc
		// đặt hàng.
		// returns đảo ngược cả ba: đơn hàng, tồn kho, sổ cái. Thiếu bất kỳ
		// module nào thì returns.New từ chối khởi tạo.
		// promotion phục vụ checkout qua Go, không có tầng HTTP riêng.
		//
		// Nối nó vào là điều kiện để mã giảm giá hoạt động: thiếu, route
		// coupon trả "chưa sẵn sàng", và CompleteCheckout không phân bổ
		// được phần giảm xuống dòng hàng.
		promotionModule, err = promotion.New(promotion.Config{
			Storage: "postgres",
			DB:      db,
		})
		if err != nil {
			return Modules{}, err
		}

		returnsModule, err = returns.New(returns.Config{
			Storage:   "postgres",
			DB:        db,
			Order:     orderModule,
			Payment:   paymentModule,
			Inventory: inventoryModule,
			Owner:     &sellerOwner{sellers: sellerModule},
		})
		if err != nil {
			return Modules{}, err
		}

		checkoutModule, err = checkout.New(checkout.Config{
			Storage:     "postgres",
			DB:          db,
			Cart:        cartModule,
			Inventory:   inventoryModule,
			Marketplace: marketplaceModule,
			Order:       orderModule,
			Seller:      sellerModule,
			Promotion:   promotionModule,
			Events:      eventbus.NewOutbox(db.Pool()),
		})
		if err != nil {
			return Modules{}, err
		}

		// Chỉ báo giám sát: phiên quá hạn chưa dọn.
		//
		// Con số này tăng dần nghĩa là tiến trình dọn đã ngừng chạy, và
		// hàng đang bị khóa cho những phiên đã chết — cuối cùng không bán
		// được gì dù kho vẫn đầy.
		stalePending, err := checkoutModule.CountExpiredPending(ctx)
		if err != nil {
			return Modules{}, fmt.Errorf("kiểm tra phiên thanh toán quá hạn: %w", err)
		}
		// Chỉ báo giám sát: event chưa phát và event đã bỏ cuộc.
		//
		// Dead letter khác 0 nghĩa là có sự thật nghiệp vụ không tới được
		// bên nhận — đơn đã đặt mà tồn kho chưa cập nhật.
		outboxStats, err := eventbus.NewOutbox(db.Pool()).Stats(ctx)
		if err != nil {
			return Modules{}, fmt.Errorf("kiểm tra outbox: %w", err)
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
			return Modules{}, err
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
		// Thương hiệu own-brand phải CHỈ ĐỊNH nhà bán, nếu không hàng rào
		// chống hàng giả chặn mọi offer cho nó — kể cả của nền tảng.
		seeded, err := catalog.SeedDemo(ctx, catalogModule, catalog.SeedInput{
			OwnBrandSellerID: ids.ID(ownBrandSellerID),
		})
		if err != nil {
			return Modules{}, fmt.Errorf("nạp dữ liệu mẫu catalog: %w", err)
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
				return Modules{}, fmt.Errorf("nạp dữ liệu mẫu product: %w", err)
			}

			// Giá gắn với SKU CÓ THẬT vừa tạo ở trên.
			seededPrices, err := pricing.SeedDemo(ctx, pricingModule, pricing.SeedInput{
				SKUIDs: seededProducts.SKUIDs,
			})
			if err != nil {
				return Modules{}, fmt.Errorf("nạp dữ liệu mẫu pricing: %w", err)
			}

			// Offer và tồn kho — HAI MẮT XÍCH cuối để dữ liệu mẫu MUA ĐƯỢC.
			//
			// Thiếu chúng thì catalog đầy sản phẩm mà giỏ hàng vĩnh viễn
			// rỗng: khách mua OFFER chứ không mua SKU, và `startCheckout`
			// luôn thất bại "không đủ hàng" nếu không có tồn kho.
			//
			// Thứ tự bắt buộc: offer trước, tồn kho sau — cả hai đều trỏ
			// tới SKU vừa tạo, và offer còn cần seller có thật.
			seededOffers, err := marketplace.SeedDemo(ctx, marketplaceModule,
				marketplace.SeedInput{
					SellerID:    ownBrandSellerID,
					SKUIDs:      seededProducts.SKUIDs,
					PriceAmount: 490000,
				})
			if err != nil {
				return Modules{}, fmt.Errorf("nạp dữ liệu mẫu marketplace: %w", err)
			}

			seededStock, err := inventory.SeedDemo(ctx, inventoryModule,
				inventory.SeedInput{
					SKUIDs:   seededProducts.SKUIDs,
					Quantity: 100,
				})
			if err != nil {
				return Modules{}, fmt.Errorf("nạp dữ liệu mẫu inventory: %w", err)
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
				"seller_id", ownBrandSellerID,
				"offer_ids", seededOffers.OfferIDs,
				"stock_location_id", seededStock.LocationID,
				"so_sku_co_hang", len(seededStock.StockedSKUIDs),
			)
		}
	}

	return Modules{
		catalog:     catalogModule,
		product:     productModule,
		identity:    identityModule,
		marketplace: marketplaceModule,
		seller:      sellerModule,
		payment:     paymentModule,
		order:       orderModule,
		customer:    customerModule,
		cart:        cartModule,
		checkout:    checkoutModule,
		fulfillment: fulfillmentModule,
		returns:     returnsModule,
		promotion:   promotionModule,
		inventory:   inventoryModule,
		audit:       auditRecorder,
	}, nil
}

type Modules struct {
	catalog     *catalog.Module
	product     *product.Module
	identity    *identity.Module
	marketplace *marketplace.Module
	seller      *seller.Module
	payment     *payment.Module
	order       *order.Module
	customer    *customer.Module
	cart        *cart.Module
	checkout    *checkout.Module
	fulfillment *fulfillment.Module
	returns     *returns.Module
	promotion   *promotion.Module
	inventory   *inventory.Module

	// audit là năng lực platform (ADR-0011), không phải module — nhưng nó
	// cũng cần nối route nên đi cùng chỗ này.
	audit *audit.Recorder
}

// registerRoutes gắn các route vào mux.
//
// Khi có module nghiệp vụ, mỗi module tự đăng ký route của mình ở đây —
// đó là điểm nối duy nhất giữa main và các module.
func RegisterRoutes(
	mux *http.ServeMux,
	cfg *config.Config,
	log *slog.Logger,
	db *database.DB,
	m Modules,

	// version chỉ dùng cho endpoint /version. Truyền vào thay vì đọc biến
	// toàn cục: biến đó do linker đặt lúc build và chỉ có ở cmd/, nên gói
	// này không được phụ thuộc vào nó.
	version string,
) {
	catalogModule := m.catalog
	productModule := m.product
	identityModule := m.identity
	marketplaceModule := m.marketplace
	sellerModule := m.seller
	paymentModule := m.payment
	orderModule := m.order
	auditRecorder := m.audit

	// Mỗi module tự đăng ký route của mình. main không biết đường dẫn hay
	// hình dạng response của module nào — nó chỉ trao mux.
	catalogModule.RegisterRoutes(mux, log)
	productModule.RegisterRoutes(mux, log)

	// Offer của sản phẩm — công khai, khách vãng lai xem được.
	if marketplaceModule != nil {
		marketplaceModule.RegisterRoutes(mux, log)
	}

	// Hồ sơ nhà bán — công khai, và đi CÙNG CẶP với offer ở trên.
	//
	// Endpoint offer chỉ trả `seller_id`; trang tự tra tên theo lô qua
	// đường này. Thiếu nó thì trang "Chọn nhà bán" liệt kê được giá nhưng
	// không nói được đang mua của ai.
	if sellerModule != nil {
		sellerModule.RegisterPublicRoutes(mux, log)

		// Đăng ký làm nhà bán: CẦN đăng nhập, KHÔNG cần vai trò nào.
		//
		// Bọc RequireRole("SELLER_OWNER") ở đây sẽ khiến chỉ nhà bán mới
		// đăng ký được làm nhà bán, và không ai vào được vòng.
		if identityModule != nil {
			applyMux := http.NewServeMux()
			sellerModule.RegisterApplyRoute(applyMux, log)
			mux.Handle("POST /api/v1/sellers", httpserver.Chain(
				applyMux,
				httpserver.Auth(identityModule),
				httpserver.RequireIdempotencyKey(),
			))
		}
	}

	registerShoppingRoutes(mux, log, m)

	if identityModule != nil {
		// Endpoint công khai: login, refresh, logout.
		identityModule.RegisterRoutes(mux, log)

		// Endpoint cần xác thực đi qua một mux RIÊNG đã bọc middleware
		// Auth, rồi mới gắn vào mux gốc.
		//
		// Vì sao không dùng chung mux: middleware bọc quanh mux gốc sẽ áp
		// cho MỌI route, kể cả login — và khi đó không ai đăng nhập được
		// vì muốn đăng nhập phải có token, mà muốn có token phải đăng nhập.
		protected := http.NewServeMux()
		identityModule.RegisterProtectedRoutes(protected, log)

		authed := httpserver.Chain(protected, httpserver.Auth(identityModule))
		mux.Handle("GET /api/v1/admin/me", authed)

		// Quản trị nhà bán: ADMIN và OPS_MERCHANDISING (admin.md mục 2).
		//
		// Đình chỉ nhà bán cần Idempotency-Key: bấm nút hai lần không được
		// tạo hai vết kiểm toán cho cùng một quyết định.
		if sellerModule != nil {
			sellerMux := http.NewServeMux()
			sellerModule.RegisterAdminRoutes(sellerMux, log)

			authed := httpserver.Chain(
				sellerMux,
				httpserver.Auth(identityModule),
				httpserver.RequireRole("ADMIN", "OPS_MERCHANDISING"),
				httpserver.RequireIdempotencyKey(),
			)
			mux.Handle("GET /api/v1/admin/sellers", authed)
			mux.Handle("GET /api/v1/admin/sellers/{seller_id}", authed)
			mux.Handle("POST /api/v1/admin/sellers/{seller_id}/approve", authed)
			mux.Handle("POST /api/v1/admin/sellers/{seller_id}/suspend", authed)
		}

		// Tài chính: ADMIN và OPS_FINANCE (admin-api.md mục 1).
		//
		// admin-api.md mục 1 còn yêu cầu XÁC THỰC HAI LỚP cho hai vai trò
		// này. Luồng 2FA chưa triển khai (P3-5) — đây là lý do Admin UI
		// chưa được phát hành, không phải lý do để nới lỏng ở đây.
		if paymentModule != nil {
			payMux := http.NewServeMux()
			paymentModule.RegisterAdminRoutes(payMux, log)

			mux.Handle("POST /api/v1/admin/ledger/adjustments", httpserver.Chain(
				payMux,
				httpserver.Auth(identityModule),
				httpserver.RequireRole("ADMIN", "OPS_FINANCE"),
				httpserver.RequireIdempotencyKey(),
			))
		}

		// Đơn hàng: ADMIN và OPS_SUPPORT (admin.md mục 2 — hỗ trợ khách).
		//
		// Hủy đơn cần Idempotency-Key; xem chi tiết thì không (GET không
		// đổi trạng thái, và middleware chỉ áp cho POST/PATCH).
		if orderModule != nil {
			orderMux := http.NewServeMux()
			orderModule.RegisterAdminRoutes(orderMux, log)

			authed := httpserver.Chain(
				orderMux,
				httpserver.Auth(identityModule),
				httpserver.RequireRole("ADMIN", "OPS_SUPPORT"),
				httpserver.RequireIdempotencyKey(),
			)
			mux.Handle("GET /api/v1/admin/orders", authed)
			mux.Handle("GET /api/v1/admin/orders/{order_id}", authed)
			mux.Handle("POST /api/v1/admin/orders/{order_id}/cancel", authed)
		}

		// Nhà bán: CHỈ vai trò SELLER_OWNER và SELLER_STAFF.
		//
		// Ranh giới bảo mật quan trọng nhất của nhóm này nằm ở TRUY VẤN —
		// mọi câu SQL lọc theo seller_id lấy từ token. Vai trò ở đây chỉ
		// chặn người không phải nhà bán; nó KHÔNG chặn nhà bán A đọc dữ
		// liệu nhà bán B, và không được nhầm hai việc đó với nhau.
		if m.fulfillment != nil {
			sellerFOMux := http.NewServeMux()
			m.fulfillment.RegisterSellerRoutes(sellerFOMux, log)

			authed := httpserver.Chain(
				sellerFOMux,
				httpserver.Auth(identityModule),
				httpserver.RequireRole("SELLER_OWNER", "SELLER_STAFF"),
				httpserver.RequireIdempotencyKey(),
			)
			mux.Handle("GET /api/v1/seller/fulfillment-orders", authed)
			mux.Handle("GET /api/v1/seller/fulfillment-orders/{fulfillment_order_id}", authed)
			mux.Handle("POST /api/v1/seller/fulfillment-orders/{fulfillment_order_id}/ship", authed)
			mux.Handle("POST /api/v1/seller/fulfillment-orders/{fulfillment_order_id}/deliver", authed)
		}

		// Trả hàng phía nhà bán: cùng ranh giới bảo mật với đơn thực hiện.
		if m.returns != nil {
			retMux := http.NewServeMux()
			m.returns.RegisterSellerRoutes(retMux, log)

			authed := httpserver.Chain(
				retMux,
				httpserver.Auth(identityModule),
				httpserver.RequireRole("SELLER_OWNER", "SELLER_STAFF"),
				httpserver.RequireIdempotencyKey(),
			)
			mux.Handle("GET /api/v1/seller/returns", authed)
			mux.Handle("POST /api/v1/seller/returns/{return_id}/approve", authed)
			mux.Handle("POST /api/v1/seller/returns/{return_id}/reject", authed)
			mux.Handle("POST /api/v1/seller/returns/{return_id}/receive", authed)
		}

		// Số dư nhà bán — cùng ranh giới bảo mật.
		if m.payment != nil {
			balMux := http.NewServeMux()
			m.payment.RegisterSellerRoutes(balMux, log)

			authed := httpserver.Chain(
				balMux,
				httpserver.Auth(identityModule),
				httpserver.RequireRole("SELLER_OWNER", "SELLER_STAFF"),
			)
			mux.Handle("GET /api/v1/seller/balance", authed)
		}

		// Webhook vận chuyển: KHÔNG có Auth, KHÔNG có Idempotency-Key.
		//
		// Bên gọi là hệ thống của hãng vận chuyển. Họ xác thực bằng CHỮ KÝ
		// HMAC trên thân request, và idempotency dựa vào `event_id` của
		// chính họ — hai thứ middleware của ta không biết cách kiểm.
		if m.fulfillment != nil {
			m.fulfillment.RegisterWebhookRoutes(mux,
				&ghiSuKien{r: webhook.NewRecorder(db.Pool())},
				biMatWebhook(cfg.Auth.WebhookSecrets), log)
		}

		// Offer và tồn kho của nhà bán — nửa còn lại của luồng 2.
		//
		// Cùng chuỗi middleware với đơn thực hiện: vai trò chặn người
		// không phải nhà bán, còn ranh giới giữa các nhà bán nằm ở truy
		// vấn và ở kiểm tra quyền sở hữu trong handler.
		if m.marketplace != nil || m.inventory != nil {
			sellerMux := http.NewServeMux()
			if m.marketplace != nil {
				// Cần CẢ HAI: inventory để nhập kho, seller để biết
				// hàng đó thuộc về ai. Thiếu seller thì thà không nhận
				// `initial_inventory` còn hơn nhập vào nhầm chủ — bản ghi
				// sai chủ không bán được và cũng không ai thấy nó sai.
				var stock markethttp.StockPort
				if m.inventory != nil && m.seller != nil {
					stock = &sellerStock{inv: m.inventory, sellers: m.seller}
				}
				m.marketplace.RegisterSellerRoutes(sellerMux, stock, log)
			}
			if m.inventory != nil {
				var owner inventoryhttp.OwnerResolver
				if m.seller != nil {
					owner = &sellerOwner{sellers: m.seller}
				}
				m.inventory.RegisterSellerRoutes(sellerMux, owner, log)
			}

			authed := httpserver.Chain(
				sellerMux,
				httpserver.Auth(identityModule),
				httpserver.RequireRole("SELLER_OWNER", "SELLER_STAFF"),
				httpserver.RequireIdempotencyKey(),
			)
			mux.Handle("GET /api/v1/seller/offers", authed)
			mux.Handle("POST /api/v1/seller/offers", authed)
			mux.Handle("PATCH /api/v1/seller/offers/{offer_id}", authed)
			mux.Handle("PUT /api/v1/seller/inventory/{sku_id}", authed)
		}

		// Nhật ký thao tác: CHỈ vai trò ADMIN (admin-api.md mục 7).
		//
		// Chuỗi middleware theo thứ tự Auth → RequireRole. Đảo lại thì
		// RequireRole chạy khi context chưa có AuthContext và mọi request
		// bị từ chối — hỏng theo hướng an toàn, nhưng vẫn là lỗi nối dây.
		if auditRecorder != nil {
			auditMux := http.NewServeMux()
			audit.NewHandler(auditRecorder, log).Register(auditMux)

			mux.Handle("GET /api/v1/admin/audit-log", httpserver.Chain(
				auditMux,
				httpserver.Auth(identityModule),
				httpserver.RequireRole("ADMIN"),
			))
		}
	} else {
		log.Warn("bỏ qua endpoint xác thực: cần MODULES_STORAGE=postgres")
	}

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

	// Chỉ số cho Prometheus.
	//
	// KHÔNG nằm dưới /api/v1: đây không phải API của sản phẩm mà là cửa
	// vận hành. Ở production nó phải được chặn khỏi internet — chỉ số phơi
	// ra số đơn, số lỗi, và hình dạng lưu lượng của cả hệ thống.
	mux.Handle("GET /metrics", promhttp.HandlerFor(
		metrics.Registry, promhttp.HandlerOpts{
			// Lỗi khi thu thập thì GHI LOG rồi trả phần còn lại, không
			// trả 500: mất một chỉ số không đáng để mất toàn bộ bảng theo
			// dõi, nhất là lúc đang có sự cố.
			ErrorHandling: promhttp.ContinueOnError,
		}))

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
