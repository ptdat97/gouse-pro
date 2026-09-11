package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/promotion"
)

// TestKhuyenMaiDoNhaBanChiuPhaiGhiDungBenCHIU.
//
// # Bất biến
//
// Khoản giảm giá của một chương trình do NHÀ BÁN tự chạy phải được ghi là
// nhà bán chịu — không phải nền tảng.
//
// # Vì sao nó quan trọng hơn vẻ ngoài
//
// `order_line_adjustment.cost_bearer` là con số ĐÓNG BĂNG mà đối soát cuối
// kỳ đọc để biết trừ tiền ai. Ghi nhầm sang PLATFORM nghĩa là nền tảng
// gánh một khoản mà nhà bán đã đồng ý chịu — và sai theo hướng đó thì
// không ai khiếu nại, nên nó sống rất lâu.
//
// Module promotion ĐÃ có sẵn quy tắc chia (`AllocateCost`, ba bên chịu).
// Bài này kiểm rằng quy tắc đó thật sự tới được đơn hàng.
func TestKhuyenMaiDoNhaBanChiuPhaiGhiDungBenChiu(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const ma = "NHABANCHIU"
	pr, err := a.mods.promotion.CreatePromotion(ctx, promotion.CreatePromotionRequest{
		Name: "Nhà bán tự giảm", Kind: "COUPON",
		DiscountType: "PERCENTAGE", DiscountBPS: 1000,
		CostBearer: "SELLER",
		StartsAt:   time.Now().UTC().Add(-time.Hour),
		EndsAt:     time.Now().UTC().Add(24 * time.Hour),
		Currency:   "VND",
	})
	if err != nil {
		t.Fatalf("tạo chương trình: %v", err)
	}
	if err := a.mods.promotion.ActivatePromotion(ctx, pr.ID); err != nil {
		t.Fatalf("kích hoạt: %v", err)
	}
	if _, err := a.mods.promotion.CreateCoupon(ctx, promotion.CreateCouponRequest{
		PromotionID: pr.ID, Code: ma,
	}); err != nil {
		t.Fatalf("tạo mã: %v", err)
	}

	maDon := a.datDonCoMa(t, ma, "benchiu")

	var bearer string
	var soTien int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT adj.cost_bearer, adj.amount
		  FROM order_line_adjustment adj
		  JOIN order_line l ON l.id = adj.order_line_id
		 WHERE l.order_id = $1 AND adj.adjustment_type = 'PROMOTION'
		 LIMIT 1`, maDon).Scan(&bearer, &soTien); err != nil {
		t.Fatalf("đọc khoản điều chỉnh: %v", err)
	}

	if bearer != "SELLER" {
		t.Errorf("cost_bearer = %q, cần SELLER — chương trình do nhà bán chạy "+
			"mà nền tảng đang gánh %d đ", bearer, -soTien)
	}

	// SỔ CÁI phải trừ đúng bên, không chỉ đơn hàng.
	//
	// Hai nơi ghi cùng một sự thật: `order_line_adjustment.cost_bearer` cho
	// đối soát, và bút toán cho số dư. Lệch nhau thì báo cáo nào cũng có vẻ
	// đúng khi đọc riêng, và không ai phát hiện cho tới lúc chi tiền.
	a.phatEvent(t)

	var choNhaBan, choNenTang int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'SELLER_PAYABLE'
		             AND l.direction = 'DEBIT'), 0),
		       COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'PLATFORM_REVENUE'
		             AND l.direction = 'DEBIT'), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1
		   AND e.description = 'Giảm giá cho khách'`,
		maDon).Scan(&choNhaBan, &choNenTang); err != nil {
		t.Fatalf("đọc bút toán giảm giá: %v", err)
	}

	if choNhaBan != -soTien {
		t.Errorf("sổ cái trừ nhà bán %d đ, cần %d — khoản giảm của chương "+
			"trình do nhà bán chạy phải trừ vào tiền phải trả gian hàng đó",
			choNhaBan, -soTien)
	}
	if choNenTang != 0 {
		t.Errorf("sổ cái trừ doanh thu nền tảng %d đ, cần 0 — nền tảng đang "+
			"gánh hộ khoản nhà bán đã đồng ý chịu", choNenTang)
	}
}

// datDonCoMa đặt một đơn có áp mã giảm giá, trả về mã đơn.
func (a *apiTest) datDonCoMa(t *testing.T, ma, nhan string) string {
	t.Helper()

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": maGio, "guest_email": emailMoi(nhan), "guest_phone": "0900555111",
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	if got := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/coupon",
		map[string]any{"code": ma}, khoaIdem()); got.code != http.StatusOK {
		t.Fatalf("áp mã: HTTP %d — %s", got.code, got.raw)
	}
	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Thử", "phone": "0900555111",
			"street_address": "1 Đường Thử", "ward": "P1",
			"district": "Q1", "province": "TP.HCM", "country_code": "VN",
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
	return maDon
}

// TestKhuyenMaiCHIADOIPhaiTruDungTyLe.
//
// # Bất biến
//
// Chương trình CHIA ĐÔI: mỗi bên gánh đúng phần đã thỏa thuận, và tổng hai
// phần bằng ĐÚNG số tiền giảm.
//
// # Vì sao sai ở đây không ai khiếu nại
//
// Hôm nay `benChiuTu` rút danh sách phân bổ thành MỘT giá trị, và bút toán
// giảm giá ghi trọn về một bên. Với chương trình chia đôi, phần của nhà
// bán không bao giờ bị trừ — nền tảng gánh hộ. Sai theo hướng đó thì nhà
// bán không mất tiền nên không ai báo, và nó sống rất lâu.
func TestKhuyenMaiChiaDoiPhaiTruDungTyLe(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const ma = "CHIADOI50"
	pr, err := a.mods.promotion.CreatePromotion(ctx, promotion.CreatePromotionRequest{
		Name: "Chia đôi 50-50", Kind: "COUPON",
		DiscountType: "PERCENTAGE", DiscountBPS: 1000,
		CostBearer:       "SHARED",
		PlatformShareBPS: 5000,
		SellerShareBPS:   5000,
		StartsAt:         time.Now().UTC().Add(-time.Hour),
		EndsAt:           time.Now().UTC().Add(24 * time.Hour),
		Currency:         "VND",
	})
	if err != nil {
		t.Fatalf("tạo chương trình: %v", err)
	}
	if err := a.mods.promotion.ActivatePromotion(ctx, pr.ID); err != nil {
		t.Fatalf("kích hoạt: %v", err)
	}
	if _, err := a.mods.promotion.CreateCoupon(ctx, promotion.CreateCouponRequest{
		PromotionID: pr.ID, Code: ma,
	}); err != nil {
		t.Fatalf("tạo mã: %v", err)
	}

	maDon := a.datDonCoMa(t, ma, "chiadoi")
	a.phatEvent(t)

	var giam int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT COALESCE(SUM(-adj.amount), 0)
		  FROM order_line_adjustment adj
		  JOIN order_line l ON l.id = adj.order_line_id
		 WHERE l.order_id = $1 AND adj.adjustment_type = 'PROMOTION'`,
		maDon).Scan(&giam); err != nil {
		t.Fatalf("đọc khoản giảm: %v", err)
	}
	if giam <= 0 {
		t.Fatalf("khoản giảm = %d, phải dương", giam)
	}

	var choNhaBan, choNenTang int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'SELLER_PAYABLE'
		             AND l.direction = 'DEBIT'), 0),
		       COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'PLATFORM_REVENUE'
		             AND l.direction = 'DEBIT'), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1
		   AND e.description = 'Giảm giá cho khách'`,
		maDon).Scan(&choNhaBan, &choNenTang); err != nil {
		t.Fatalf("đọc bút toán giảm giá: %v", err)
	}

	// Bất biến quan trọng nhất: tổng hai phần bằng ĐÚNG số tiền giảm.
	if choNhaBan+choNenTang != giam {
		t.Errorf("tổng đã trừ = %d (nhà bán %d + nền tảng %d), cần %d — "+
			"lệch một đồng ở đây là khoản KHÔNG AI CHỊU",
			choNhaBan+choNenTang, choNhaBan, choNenTang, giam)
	}
	if choNhaBan == 0 {
		t.Errorf("nhà bán bị trừ 0 đ trong chương trình CHIA ĐÔI — nền tảng "+
			"đang gánh trọn %d đ", choNenTang)
	}
}
