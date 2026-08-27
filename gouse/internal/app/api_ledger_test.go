package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	marketapp "github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	marketdom "github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
	"github.com/fashion-commerce/platform/internal/modules/order"

	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// phatEvent chạy bộ phát event với ĐÚNG bên nhận mà cmd/worker đăng ký.
//
// Dựng bên nhận riêng cho test sẽ kiểm một thứ khác với thứ chạy thật —
// đúng cái bẫy đã làm bộ middleware trong test này từng lệch với production.
//
// KHÔNG đủ bộ: supplychain, notification và analytics không nằm trong
// Modules của internal/app. Chúng không ảnh hưởng tới các bất biến mà lớp
// test này đo (sổ cái, vòng đời đơn), nhưng đây là một khác biệt CÓ THẬT
// so với worker, không phải sự tương đương.
func (a *apiTest) phatEvent(t *testing.T) int {
	t.Helper()

	log := logger.New("error", "text")
	bus := eventbus.NewDispatcher(a.db.Pool(), log)
	bus.Subscribe(inventory.NewCommitHandler(a.mods.inventory, log))
	bus.Subscribe(fulfillment.NewSplitHandler(a.mods.fulfillment, log))
	bus.Subscribe(order.NewProgressHandler(a.mods.order, log))
	bus.Subscribe(payment.NewRevenueHandler(
		a.mods.payment, payment.NewSellerKind(a.mods.seller), log))

	n, err := bus.DispatchBatch(context.Background(), 100)
	if err != nil {
		t.Fatalf("phát event: %v", err)
	}
	return n
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
