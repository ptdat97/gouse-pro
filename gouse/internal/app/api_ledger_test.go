package app

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/affiliate"
	"github.com/fashion-commerce/platform/internal/modules/analytics"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	marketapp "github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	marketdom "github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
	"github.com/fashion-commerce/platform/internal/modules/notification"
	"github.com/fashion-commerce/platform/internal/modules/order"

	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/modules/promotion"
	"github.com/fashion-commerce/platform/internal/modules/recommendation"
	"github.com/fashion-commerce/platform/internal/modules/supplychain"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// phatEvent chạy bộ phát event với ĐÚNG bên nhận mà cmd/worker đăng ký.
//
// Dựng bên nhận riêng cho test sẽ kiểm một thứ khác với thứ chạy thật —
// đúng cái bẫy đã làm bộ middleware trong test này từng lệch với production.
//
// ĐỦ BỘ từ 01/10/2026, trừ ĐÚNG MỘT bên nhận có lý do kỹ thuật:
// `payment.NewCarrierCostHandler` cần một nguồn GIÁ vận chuyển, nên
// `api_chi_phi_van_chuyen_test.go` tự đăng ký nó kèm giá cố định — đăng ký
// vào bộ chung sẽ phải chọn một giá cho MỌI bài, và giá ấy là thứ bài kia
// đang kiểm.
//
// `cmd/eventcheck` R4 so danh sách này với `cmd/worker/main.go` và ĐỎ khi
// lệch, theo cả hai chiều. Miễn trừ duy nhất còn lại nằm ở sổ
// `benNhanChiOWorker` kèm lý do trên.
//
// Hai khoảng trống từng ở đây và cách chúng lộ ra:
//
//	inventory.NewReleaseHandler   tìm ra bằng một phép phá KHÔNG đỏ — bỏ
//	                              `MaxEventVersion` của nó mà không bài nào
//	                              đỏ, vì nó chưa bao giờ chạy ở đây (P3-86)
//	supplychain.NewSignalHandler  chú thích cũ ĐÃ GHI nó, nhiều tháng. "Đã
//	                              ghi" không phải "đã gác" (P3-90)
//
// `analytics` và `notification` thì ĐÃ đủ từ 17/09 — xem hai dòng
// Subscribe cuối. Trước đó hai đường ấy chỉ chạy ở worker, nên đường thư
// giao dịch và đường ghi phễu chuyển đổi không có bài test tích hợp nào
// đi qua: một lần nối dây hỏng sẽ im lặng y như lỗi `size_recommendation`
// (P3-50).
// dangKyBenNhan đăng ký bộ bên nhận chuẩn — dùng chung cho mọi bài test
// cần phát event, để không có bài nào chạy với bộ bên nhận khác production.
func (a *apiTest) dangKyBenNhan(bus *eventbus.Dispatcher, log *slog.Logger) {
	bus.Subscribe(inventory.NewCommitHandler(a.mods.inventory, log))

	// ĐƯỜNG RA của kho — thiếu ở bộ này tới 01/10/2026.
	//
	// Worker đăng ký nó từ lâu; bộ bên nhận của test thì không, và không
	// chú thích nào nói vì sao. Hệ quả: hủy đơn thực hiện ở MỌI bài test
	// lớp này không bao giờ trả hàng về kho, nên lỗi "tồn kho đứng nguyên
	// 15 khả dụng / 5 cam kết" — lỗi mà `order/events.go` mô tả kèm lần
	// kiểm chứng bằng đơn thật — không có bài nào ở đây bắt được.
	//
	// Tìm ra khi phá hàng rào phiên bản ADR-0016: bỏ `MaxEventVersion` của
	// chính bên nhận này mà không bài nào đỏ, vì nó chưa từng chạy ở đây.
	bus.Subscribe(inventory.NewReleaseHandler(
		a.mods.inventory, &sellerOwner{sellers: a.mods.seller}, log))

	bus.Subscribe(fulfillment.NewSplitHandler(a.mods.fulfillment, log))
	bus.Subscribe(order.NewProgressHandler(a.mods.order, log))
	bus.Subscribe(payment.NewRevenueHandler(
		a.mods.payment, payment.NewSellerKind(a.mods.seller), log))
	bus.Subscribe(payment.NewSellerReleaseHandler(a.mods.payment, log))

	// Hai bên nhận của `order.paid` — ADR-0018. Thiếu chúng thì đơn trả
	// trước bị khóa vĩnh viễn và tiền mặt không bao giờ được ghi nhận.
	bus.Subscribe(payment.NewPaymentHandler(a.mods.payment, log))
	bus.Subscribe(fulfillment.NewUnlockHandler(a.mods.fulfillment, log))
	bus.Subscribe(fulfillment.NewCancelByOrderHandler(a.mods.fulfillment, log))

	// Lượt dùng mã giảm giá: ghi khi phiên hoàn tất, trả lại khi đơn hủy.
	// Read model gợi ý size — dựng từ mua hàng và trả hàng.
	bus.Subscribe(recommendation.NewSizeObservationHandler(a.mods.recommendation, log))
	bus.Subscribe(promotion.NewRecordUseHandler(a.mods.promotion, log))
	bus.Subscribe(promotion.NewReleaseUseHandler(a.mods.promotion, log))

	// Phễu chuyển đổi: `analytics` ghi event_log cho mỗi bước.
	if a.mods.analytics != nil {
		bus.Subscribe(analytics.NewEventRecorder(a.mods.analytics))
	}

	// Bánh đà dữ liệu: hành vi khách chảy ngược vào lập kế hoạch sản xuất.
	//
	// Thiếu ở bộ này tới 01/10/2026 — chú thích cũ đã ghi khoảng trống ấy,
	// nhưng "đã ghi" không phải "đã gác": không bài nào ở lớp này chứng
	// minh được tín hiệu nhu cầu có được ghi. Xem P3-90.
	if a.mods.supplychain != nil {
		bus.Subscribe(supplychain.NewSignalHandler(a.mods.supplychain))
	}

	// Thư giao dịch: khách phải nhận được thư báo đặt hàng và thư báo đã
	// giao. Đây là lời hứa NHÌN THẤY ĐƯỢC với khách, nên nó thuộc bộ bên
	// nhận chuẩn chứ không phải phần phụ.
	if a.mods.notification != nil {
		bus.Subscribe(notification.NewOrderNotifier(a.mods.notification, log))
	}

	// Hoa hồng creator — BỐN bên nhận, đi cùng nhau.
	//
	// Đăng ký thiếu một cái cho một hệ thống TRÔNG NHƯ chạy: quy công có,
	// nhưng hoặc nền tảng trả hoa hồng cho đơn đã hủy, hoặc khoản hoa hồng
	// đứng ở CHỜ ĐẾN HẠN vĩnh viễn. Cả hai đều im lặng, và cả hai đều là
	// tiền.
	if a.mods.affiliate != nil {
		bus.Subscribe(affiliate.NewAttributeHandler(a.mods.affiliate, log))
		bus.Subscribe(affiliate.NewReverseHandler(a.mods.affiliate, log))
		bus.Subscribe(affiliate.NewReverseOnFulfillmentHandler(a.mods.affiliate, log))
		bus.Subscribe(affiliate.NewUnlockHandler(a.mods.affiliate, log))
	}
}

func (a *apiTest) phatEvent(t *testing.T) int {
	t.Helper()

	log := logger.New("error", "text")
	bus := eventbus.NewDispatcher(a.db.Pool(), log)
	a.dangKyBenNhan(bus, log)

	// PHÁT TỚI CẠN, không phải một mẻ.
	//
	// Cả gói test dùng CHUNG một database, nên outbox tích event của mọi
	// bài chạy trước. Một mẻ cố định nghĩa là bài nào chạy sau sẽ thấy
	// event của mình nằm ngoài mẻ — bài xanh khi chạy riêng, đỏ khi chạy
	// cả gói, và thông điệp lỗi chỉ vào nghiệp vụ chứ không vào hàng đợi.
	// Đã xảy ra khi thêm `checkout.started` làm số event tăng gấp đôi.
	var tong int
	for {
		n, err := bus.DispatchBatch(context.Background(), 200)
		if err != nil {
			t.Fatalf("phát event: %v", err)
		}
		tong += n
		if n == 0 {
			return tong
		}
	}
}

// TestDatHangThiGhiSoDoanhThu — PH-33.
//
// # Vì sao bài này tồn tại
//
// Trước 27/08, `ledger_entry` có ĐÚNG 0 dòng sau 76 đơn hàng và 20 lượt
// bàn giao vận chuyển. Module payment đã dựng xong, đã nối vào ứng dụng —
// nhưng không nghe event nào, nên không ai gọi nó. Hàng rời kho mà không
// có một bản ghi kế toán nào.
//
// Bài này khoá lại điều đó: đặt đơn qua HTTP thật, chạy bộ phát event
// thật, rồi kiểm SỔ CÁI chứ không kiểm response.
func TestDatHangThiGhiSoDoanhThu(t *testing.T) {
	a := newAPITest(t)

	truoc := a.demDong("ledger_entry", "")

	maPhien := a.dungPhienSanHoanTat("soCai@example.com", "0900888777")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)
	if maDon == "" {
		t.Fatalf("không lấy được mã đơn: %s", res.raw)
	}

	// Trước khi phát event: chưa có gì. Đây chính là trạng thái cũ.
	if got := a.demDong("ledger_entry", ""); got != truoc {
		t.Errorf("có %d bút toán TRƯỚC khi phát event, cần %d — "+
			"ghi sổ phải đi qua event, không phải chèn thẳng trong lúc đặt đơn",
			got-truoc, 0)
	}

	a.phatEvent(t)

	sau := a.demDong("ledger_entry", "")
	if sau <= truoc {
		t.Fatalf("phát event xong vẫn KHÔNG có bút toán nào — " +
			"payment không nghe checkout.completed (PH-33)")
	}

	// Bút toán phải trỏ đúng đơn hàng này.
	n := a.demDong("ledger_entry", "reference_id = $1 AND entry_type = 'ORDER_REVENUE'", maDon)
	if n < 1 {
		t.Errorf("đơn %s có %d bút toán doanh thu, cần ít nhất 1", maDon, n)
	}

	// Sổ KÉP: mỗi bút toán phải cân. Lệch một đồng là sổ sai.
	var lech int
	err := a.db.Pool().QueryRow(context.Background(), `
		SELECT count(*) FROM (
			SELECT l.entry_id
			  FROM ledger_line l
			  JOIN ledger_entry e ON e.id = l.entry_id
			 WHERE e.reference_id = $1
			 GROUP BY l.entry_id
			HAVING sum(CASE WHEN l.direction = 'DEBIT'  THEN l.amount ELSE 0 END)
			    <> sum(CASE WHEN l.direction = 'CREDIT' THEN l.amount ELSE 0 END)
		) x`, maDon).Scan(&lech)
	if err != nil {
		t.Fatalf("kiểm cân đối: %v", err)
	}
	if lech != 0 {
		t.Errorf("%d bút toán KHÔNG cân — sổ kép bị vi phạm", lech)
	}
}

// TestPhatLaiEventKhongGhiSoHaiLan.
//
// Outbox giao hàng ÍT NHẤT MỘT LẦN. Một event được phát lại — vì tiến
// trình chết sau khi handler chạy nhưng trước khi đánh dấu, hoặc vì người
// vận hành chạy lại — không được ghi doanh thu lần thứ hai.
//
// Đây là bất biến của TIỀN, nên nó phải được cưỡng chế ở tầng dữ liệu:
// ràng buộc UNIQUE trên `ledger_entry.idempotency_key`, với khoá gồm cả
// mã đơn lẫn mã nhà bán.
func TestPhatLaiEventKhongGhiSoHaiLan(t *testing.T) {
	a := newAPITest(t)

	maPhien := a.dungPhienSanHoanTat("phatlai@example.com", "0900888666")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	a.phatEvent(t)
	lan1 := a.demDong("ledger_entry", "reference_id = $1", maDon)
	if lan1 == 0 {
		t.Fatalf("lần phát đầu không ghi sổ gì")
	}

	// Đưa event về lại hàng chờ, đúng như dispatcher gặp phải khi tiến
	// trình chết giữa chừng.
	if _, err := a.db.Pool().Exec(context.Background(),
		`UPDATE event_outbox SET published_at = NULL, attempts = 0
		  WHERE payload::text LIKE '%' || $1 || '%'`, maDon); err != nil {
		t.Fatalf("đưa event về hàng chờ: %v", err)
	}
	if _, err := a.db.Pool().Exec(context.Background(),
		`DELETE FROM event_processed WHERE event_id IN (
		   SELECT event_id FROM event_outbox WHERE payload::text LIKE '%' || $1 || '%')`,
		maDon); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}

	a.phatEvent(t)

	lan2 := a.demDong("ledger_entry", "reference_id = $1", maDon)
	if lan2 != lan1 {
		t.Errorf("phát lại làm số bút toán đổi %d → %d — doanh thu bị ghi hai lần",
			lan1, lan2)
	}
}

// TestDonTronHaiNhaBanGhiHaiButToan.
//
// # Vì sao phải có bài RIÊNG cho đơn trộn
//
// Đơn một nhà bán không phân biệt được hai thiết kế khác nhau: khoá
// idempotency theo ĐƠN, và khoá theo (ĐƠN, NHÀ BÁN). Cả hai đều cho đúng
// một bút toán, nên bài test đơn lẻ vẫn xanh với cả hai — tôi đã kiểm và
// nó thật sự không bắt được.
//
// Với đơn trộn thì khác hẳn: khoá chỉ theo đơn khiến nhà bán thứ hai trở
// đi bị coi là TRÙNG LẶP, và tiền của họ không bao giờ vào sổ. Nhà bán
// giao hàng thật rồi không được ghi nhận gì.
//
// docs/07-workflows/marketplace-order.md mục 4 nói rõ: mỗi phần của mỗi
// nhà bán là MỘT bút toán riêng.
func TestDonTronHaiNhaBanGhiHaiButToan(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	// Nhà bán thứ hai, có hàng thật bán được.
	B := dungNhaBan(t, a, "nhabanso"+ids.MustNew(ids.PrefixRequest).String()[24:])
	skuID := dungSkuThuongHieuMo(t, a)

	gia, _ := money.New(199_000, money.VND)
	offerB, err := a.mods.marketplace.Service().CreateOffer(ctx, marketapp.CreateOfferInput{
		SKUID: ids.ID(skuID), SellerID: ids.ID(B.sellerID), Price: gia,
		Condition: marketdom.ConditionNew, HandlingTimeHours: 24,
		MinOrderQuantity: 1, Activate: true,
	})
	if err != nil {
		t.Skipf("không tạo được offer: %v", err)
	}
	loc, err := a.mods.inventory.EnsureLocation(ctx, "Kho B", "SELLER-"+B.sellerID, "SELLER")
	if err != nil {
		t.Fatalf("tạo kho: %v", err)
	}
	if _, err := a.mods.inventory.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: loc,
		OwnerID:  inventory.OwnerForSeller(B.sellerID, false),
		Quantity: 10, PerformedBy: "test",
	}); err != nil {
		t.Fatalf("nhập kho: %v", err)
	}

	// Giỏ có hàng của CẢ HAI nhà bán.
	offerA := a.timOfferBanDuoc()
	if offerA == "" {
		t.Skip("không có offer nào của nhà bán mặc định")
	}
	for _, id := range []string{offerA, offerB.ID().String()} {
		r := a.call(http.MethodPost, "/api/v1/cart/items",
			map[string]any{"offer_id": id, "quantity": 1}, khoaIdem())
		if r.code != http.StatusOK {
			t.Fatalf("thêm offer %s: HTTP %d — %s", id, r.code, r.raw)
		}
	}

	gio := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	c, _ := gio.body["cart"].(map[string]any)
	nhom, _ := c["groups"].([]any)
	if len(nhom) < 2 {
		t.Skipf("giỏ chỉ có %d nhà bán, cần 2", len(nhom))
	}
	maGio, _ := c["id"].(string)

	maDon := a.datDonTuGio(t, maGio, "tron@example.com", "0900222111")
	a.phatEvent(t)

	// HAI bút toán doanh thu, mỗi nhà bán một cái.
	n := a.demDong("ledger_entry", "reference_id = $1 AND entry_type = 'ORDER_REVENUE'", maDon)
	if n != 2 {
		t.Errorf("đơn trộn hai nhà bán có %d bút toán doanh thu, cần 2 — "+
			"khoá idempotency phải gồm CẢ mã nhà bán", n)
	}

	// Và chúng phải ghi cho hai chủ sở hữu KHÁC NHAU.
	var soChu int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(DISTINCT l.account_owner_id)
		  FROM ledger_line l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1
		   AND l.account_type = 'SELLER_PAYABLE'`, maDon).Scan(&soChu); err != nil {
		t.Fatalf("đếm chủ sở hữu: %v", err)
	}
	if soChu < 1 {
		t.Errorf("không có dòng PHẢI TRẢ NHÀ BÁN nào — tiền của nhà bán ngoài "+
			"không được ghi nhận (%d chủ)", soChu)
	}
}

// datDonTuGio hoàn tất một giỏ đã có sẵn và trả mã đơn.
func (a *apiTest) datDonTuGio(t *testing.T, maGio, email, dienThoai string) string {
	t.Helper()

	res := a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": maGio, "guest_email": email, "guest_phone": dienThoai,
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Trộn", "phone": dienThoai,
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, khoaIdem())
	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, khoaIdem())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)
	if maDon == "" {
		t.Fatalf("không lấy được mã đơn: %s", res.raw)
	}
	return maDon
}
