package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	fulfillmentapp "github.com/fashion-commerce/platform/internal/modules/fulfillment/application"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	marketapp "github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	marketdom "github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
)

// TestTienNhaBanChuyenSangRutDuocKhiHetHanDoiTra.
//
// # Bất biến
//
// Tiền của nhà bán CHỈ rút được sau khi hết hạn đổi trả. Trước đó khách
// vẫn có thể trả hàng, và chi trả một khoản có thể phải đòi lại là cách
// mất tiền chắc chắn nhất — nhà bán đã rút rồi thì đòi bằng gì.
//
// Bài này đi trọn: đặt đơn của nhà bán NGOÀI → giao → đẩy quá hạn đổi trả
// → chạy job → kiểm số dư qua HTTP.
func TestTienNhaBanChuyenSangRutDuocKhiHetHanDoiTra(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	// Nhà bán NGOÀI: own brand không có ai để trả tiền.
	B := dungNhaBan(t, a, "nhabansodu"+ids.MustNew(ids.PrefixRequest).String()[24:])
	skuID := dungSkuThuongHieuMo(t, a)

	gia, _ := money.New(250_000, money.VND)
	offer, err := a.mods.marketplace.Service().CreateOffer(ctx, marketapp.CreateOfferInput{
		SKUID: ids.ID(skuID), SellerID: ids.ID(B.sellerID), Price: gia,
		Condition: marketdom.ConditionNew, HandlingTimeHours: 24,
		MinOrderQuantity: 1, Activate: true,
	})
	if err != nil {
		t.Skipf("không tạo được offer: %v", err)
	}
	loc, err := a.mods.inventory.EnsureLocation(ctx, "Kho số dư", "SELLER-"+B.sellerID, "SELLER")
	if err != nil {
		t.Fatalf("tạo kho: %v", err)
	}
	if _, err := a.mods.inventory.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: loc,
		OwnerID:  inventory.OwnerForSeller(B.sellerID, false),
		Quantity: 5, PerformedBy: "test",
	}); err != nil {
		t.Fatalf("nhập kho: %v", err)
	}

	// Đặt đơn CHỈ có hàng của nhà bán này.
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": offer.ID().String(), "quantity": 1}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)
	maDon := a.datDonTuGio(t, maGio, "sodu@example.com", "0900864209")

	a.phatEvent(t)

	// Số dư SAU khi đặt đơn: tiền ở ĐANG CHỜ, chưa rút được.
	cho, rut := a.soDuNhaBan(t, B.token)
	if cho <= 0 {
		t.Fatalf("sau khi đặt đơn, số dư đang chờ là %v — doanh thu không "+
			"được ghi cho nhà bán", cho)
	}
	if rut != 0 {
		t.Errorf("chưa hết hạn đổi trả mà đã rút được %v — nhà bán rút tiền "+
			"của một đơn khách vẫn trả lại được", rut)
	}

	// --- giao hàng ---
	fo := a.timFulfillment(t, maDon)
	if fo.id == "" {
		t.Skip("không tạo được đơn thực hiện")
	}
	for _, b := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, fo.sellerID, fo.id) },
		func() error {
			return a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: fo.sellerID, FulfillmentID: fo.id,
				Provider: "ghn", TrackingNumber: "SD-" + fo.id[4:14],
			})
		},
		func() error { return a.mods.fulfillment.MarkDelivered(ctx, fo.sellerID, fo.id) },
	} {
		if err := b(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}

	// Vừa giao xong: VẪN chưa rút được.
	a.phatEvent(t)
	if _, rut := a.soDuNhaBan(t, B.token); rut != 0 {
		t.Errorf("vừa giao xong đã rút được %v — hạn đổi trả chưa hết", rut)
	}

	// --- đẩy quá hạn đổi trả rồi chạy job ---
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE fulfillment_order SET delivered_at = now() - $1::interval WHERE id = $2`,
		(fulfillmentapp.ReturnWindow + time.Hour).String(), fo.id); err != nil {
		t.Fatalf("đẩy mốc giao: %v", err)
	}
	if n, err := a.mods.fulfillment.CompleteDelivered(ctx, 10); err != nil || n == 0 {
		t.Fatalf("hoàn tất đơn quá hạn: n=%d err=%v", n, err)
	}
	a.phatEvent(t)

	choSau, rutSau := a.soDuNhaBan(t, B.token)
	if rutSau <= 0 {
		t.Errorf("hết hạn đổi trả mà số dư rút được vẫn là %v — "+
			"nhà bán không bao giờ nhận được tiền", rutSau)
	}
	if choSau != 0 {
		t.Errorf("số dư đang chờ còn %v sau khi chuyển, cần 0", choSau)
	}
	if rutSau != cho {
		t.Errorf("chuyển %v sang rút được nhưng ban đầu đang chờ %v — "+
			"số tiền không khớp", rutSau, cho)
	}

	// PHÁT LẠI event KHÔNG được chuyển lần hai.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE event_outbox SET published_at = NULL, attempts = 0
		  WHERE event_type = 'fulfillment_order.completed'`); err != nil {
		t.Fatalf("đưa event về hàng chờ: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx,
		`DELETE FROM event_processed WHERE event_id IN (
		   SELECT event_id FROM event_outbox
		    WHERE event_type = 'fulfillment_order.completed')`); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
	a.phatEvent(t)

	if _, rutLai := a.soDuNhaBan(t, B.token); rutLai != rutSau {
		t.Errorf("phát lại event làm số dư rút được đổi %v → %v — "+
			"nhà bán rút được gấp đôi số tiền thật", rutSau, rutLai)
	}
}

// soDuNhaBan đọc số dư qua HTTP, trả (đang chờ, rút được).
func (a *apiTest) soDuNhaBan(t *testing.T, token string) (float64, float64) {
	t.Helper()
	res := a.call(http.MethodGet, "/api/v1/seller/balance", nil, bearer(token))
	if res.code != http.StatusOK {
		t.Fatalf("đọc số dư: HTTP %d — %s", res.code, res.raw)
	}
	cho, _ := res.body["pending"].(map[string]any)
	rut, _ := res.body["available"].(map[string]any)
	c, _ := cho["amount"].(float64)
	r, _ := rut["amount"].(float64)
	return c, r
}

// TestDoiSoatGomKhoanRutDuocVaKhongGomHaiLan.
//
// # Bất biến
//
// Một bút toán rút được thuộc về ĐÚNG MỘT đợt đối soát. Lọt vào hai đợt
// nghĩa là nhà bán được trả tiền hai lần cho cùng một đơn hàng.
//
// # Lớp nào thật sự gánh bất biến này
//
// Có HAI lớp, và chúng không ngang nhau:
//
//	lọc "chưa gom" ở truy vấn      → tránh làm việc thừa
//	UNIQUE (ledger_entry_id)       → CƯỠNG CHẾ bất biến
//
// Đã kiểm bằng cách phá: bỏ lớp lọc thì bài này VẪN XANH, vì ràng buộc
// database chặn. Bỏ CẢ HAI thì nó đỏ với "bút toán với khóa này đã tồn
// tại". Nên lớp lọc là tối ưu, không phải bảo đảm — và nếu ai đó gỡ ràng
// buộc UNIQUE vì thấy "đã có kiểm ở tầng ứng dụng rồi", nhà bán sẽ được
// trả tiền hai lần.
func TestDoiSoatGomKhoanRutDuocVaKhongGomHaiLan(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	B, rut := a.dungNhaBanCoTienRutDuoc(t)
	if rut <= 0 {
		t.Fatalf("không dựng được số dư rút được (%v)", rut)
	}

	den := time.Now().UTC().Add(time.Minute)
	tu := den.Add(-7 * 24 * time.Hour)

	n, err := a.mods.payment.TaoDoiSoatChoKy(ctx, tu, den, 1000)
	if err != nil {
		t.Fatalf("tạo đợt đối soát: %v", err)
	}
	if n == 0 {
		t.Fatal("không tạo được đợt nào dù có khoản rút được")
	}

	res := a.call(http.MethodGet, "/api/v1/seller/settlements", nil, bearer(B.token))
	if res.code != http.StatusOK {
		t.Fatalf("đọc danh sách đợt: HTTP %d — %s", res.code, res.raw)
	}
	ds, _ := res.body["data"].([]any)
	if len(ds) != 1 {
		t.Fatalf("nhà bán có %d đợt, cần 1", len(ds))
	}
	d0, _ := ds[0].(map[string]any)
	gross, _ := d0["gross_amount"].(map[string]any)
	g, _ := gross["amount"].(float64)
	if g != rut {
		t.Errorf("đợt gom %v, cần %v — không khớp số rút được", g, rut)
	}

	// CHẠY LẠI job: KHÔNG được gom lần hai.
	lai, err := a.mods.payment.TaoDoiSoatChoKy(ctx, tu, den, 1000)
	if err != nil {
		t.Fatalf("chạy lại job: %v", err)
	}
	if lai != 0 {
		t.Errorf("chạy lại tạo thêm %d đợt — bút toán bị gom hai lần, "+
			"nhà bán được trả tiền gấp đôi", lai)
	}

	// Đếm theo ĐÚNG nhà bán này: bảng dùng chung với các bài khác trong
	// cùng lượt chạy, và đếm toàn bảng là đo cả dữ liệu của người khác.
	var soDong int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(*) FROM settlement_line sl
		  JOIN settlement s ON s.id = sl.settlement_id
		 WHERE s.seller_id = $1`, B.sellerID).Scan(&soDong); err != nil {
		t.Fatalf("đếm dòng đối soát: %v", err)
	}
	if soDong != 1 {
		t.Errorf("nhà bán này có %d dòng đối soát, cần 1", soDong)
	}
}

// dungNhaBanCoTienRutDuoc dựng một nhà bán đã bán được và hết hạn đổi trả.
func (a *apiTest) dungNhaBanCoTienRutDuoc(t *testing.T) (nhaBanThu, float64) {
	t.Helper()
	ctx := context.Background()

	B := dungNhaBan(t, a, "nhabanstl"+ids.MustNew(ids.PrefixRequest).String()[24:])
	skuID := dungSkuThuongHieuMo(t, a)

	gia, _ := money.New(300_000, money.VND)
	offer, err := a.mods.marketplace.Service().CreateOffer(ctx, marketapp.CreateOfferInput{
		SKUID: ids.ID(skuID), SellerID: ids.ID(B.sellerID), Price: gia,
		Condition: marketdom.ConditionNew, HandlingTimeHours: 24,
		MinOrderQuantity: 1, Activate: true,
	})
	if err != nil {
		t.Skipf("không tạo được offer: %v", err)
	}
	loc, err := a.mods.inventory.EnsureLocation(ctx, "Kho đối soát", "SELLER-"+B.sellerID, "SELLER")
	if err != nil {
		t.Fatalf("tạo kho: %v", err)
	}
	if _, err := a.mods.inventory.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: loc,
		OwnerID:  inventory.OwnerForSeller(B.sellerID, false),
		Quantity: 5, PerformedBy: "test",
	}); err != nil {
		t.Fatalf("nhập kho: %v", err)
	}

	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": offer.ID().String(), "quantity": 1}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)
	maDon := a.datDonTuGio(t, maGio, "doisoat@example.com", "0900112233")

	a.phatEvent(t)
	fo := a.timFulfillment(t, maDon)
	if fo.id == "" {
		t.Skip("không tạo được đơn thực hiện")
	}
	for _, b := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, fo.sellerID, fo.id) },
		func() error {
			return a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: fo.sellerID, FulfillmentID: fo.id,
				Provider: "ghn", TrackingNumber: "DS-" + fo.id[4:14],
			})
		},
		func() error { return a.mods.fulfillment.MarkDelivered(ctx, fo.sellerID, fo.id) },
	} {
		if err := b(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}

	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE fulfillment_order SET delivered_at = now() - $1::interval WHERE id = $2`,
		(fulfillmentapp.ReturnWindow + time.Hour).String(), fo.id); err != nil {
		t.Fatalf("đẩy mốc giao: %v", err)
	}
	if _, err := a.mods.fulfillment.CompleteDelivered(ctx, 10); err != nil {
		t.Fatalf("hoàn tất đơn: %v", err)
	}
	a.phatEvent(t)

	_, rut := a.soDuNhaBan(t, B.token)
	return B, rut
}
