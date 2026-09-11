package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/promotion"
)

// dungMaCoGioiHan tạo một mã giảm giá có TRẦN số lượt dùng toàn cục.
func (a *apiTest) dungMaCoGioiHan(
	t *testing.T, ma string, phanTramBP int32, tranLuot int,
) string {
	t.Helper()
	ctx := context.Background()

	pr, err := a.mods.promotion.CreatePromotion(ctx, promotion.CreatePromotionRequest{
		Name: "Giới hạn lượt " + ma, Kind: "COUPON",
		DiscountType: "PERCENTAGE", DiscountBPS: phanTramBP,
		MaxUses:  tranLuot,
		StartsAt: time.Now().UTC().Add(-time.Hour),
		EndsAt:   time.Now().UTC().Add(24 * time.Hour),
		Currency: "VND",
	})
	if err != nil {
		t.Fatalf("tạo chương trình: %v", err)
	}
	if err := a.mods.promotion.ActivatePromotion(ctx, pr.ID); err != nil {
		t.Fatalf("kích hoạt chương trình: %v", err)
	}
	if _, err := a.mods.promotion.CreateCoupon(ctx, promotion.CreateCouponRequest{
		PromotionID: pr.ID, Code: ma,
	}); err != nil {
		t.Fatalf("tạo mã: %v", err)
	}
	return pr.ID
}

// datDonVoiMa đi trọn một đơn COD có áp mã, trả về mã đơn.
//
// apDuoc = false nghĩa là bước áp mã BỊ TỪ CHỐI — khi đó trả về "".
func (a *apiTest) datDonVoiMa(t *testing.T, ma, sdt string) (maDon string, apDuoc bool) {
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
		"cart_id": maGio, "guest_email": "luot@example.com",
		"guest_phone": sdt,
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	maPhien, _ := res.body["id"].(string)

	got := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/coupon",
		map[string]any{"code": ma}, khoaIdem())
	if got.code != http.StatusOK {
		return "", false
	}

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Lượt", "phone": sdt,
			"street_address": "2 Đường Thử", "ward": "P1",
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
	maDon, _ = don["id"].(string)
	return maDon, true
}

// soLuotConHieuLuc đếm lượt dùng CHƯA bị giải phóng của một chương trình.
func (a *apiTest) soLuotConHieuLuc(t *testing.T, maCT string) (luot int, tienDaGiam int64) {
	t.Helper()
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT count(*), coalesce(sum(discount_amount), 0)
		  FROM coupon_usage
		 WHERE promotion_id = $1 AND released_at IS NULL`, maCT).
		Scan(&luot, &tienDaGiam); err != nil {
		t.Fatalf("đọc lượt dùng: %v", err)
	}
	return luot, tienDaGiam
}

// TestLuotDungMaDuocGhiVaTranLuotChanDonSau.
//
// # Vì sao bài này tồn tại
//
// `RecordUsage` có đủ ba tầng từ lâu — kèm cả phần khó: ghi lượt rồi cộng
// dồn nguyên tử, idempotent theo đơn. Nhưng KHÔNG ai gọi nó từ ngoài
// module, nên bộ đếm đứng yên ở 0 và MỌI giới hạn của mã đều vô hiệu:
// một mã phát ra dùng được vô hạn lần bởi vô hạn người.
//
// Đo trên dữ liệu thật trước khi sửa: 1 đơn dùng mã, 0 lượt được ghi.
//
// Bài này đi qua HTTP như khách thật, và kiểm cả hai nửa: lượt được GHI,
// và trần lượt thật sự CHẶN đơn kế tiếp.
func TestLuotDungMaDuocGhiVaTranLuotChanDonSau(t *testing.T) {
	a := newAPITest(t)

	const ma = "TRANMOTLUOT"
	maCT := a.dungMaCoGioiHan(t, ma, 1000, 1) // giảm 10%, tối đa MỘT lượt

	maDon, apDuoc := a.datDonVoiMa(t, ma, "0900111222")
	if !apDuoc {
		t.Fatal("đơn ĐẦU TIÊN bị từ chối mã — trần lượt chặn nhầm")
	}

	// Trước khi phát event: chưa có gì được ghi, và đó là điều bình
	// thường — lượt được ghi bởi bên nhận, không phải trong giao dịch
	// đặt đơn.
	a.phatEvent(t)

	luot, daGiam := a.soLuotConHieuLuc(t, maCT)
	if luot != 1 {
		t.Fatalf("sau một đơn dùng mã, ghi được %d lượt — cần 1. "+
			"Bộ đếm không tăng nghĩa là mọi giới hạn của mã đều vô hiệu", luot)
	}
	if daGiam <= 0 {
		t.Errorf("lượt được ghi nhưng số tiền giảm là %d — "+
			"ngân sách khuyến mãi sẽ không bao giờ cạn", daGiam)
	}

	var ctLuot int
	var ctNganSach int64
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT used_count, used_budget FROM promotion WHERE id = $1`, maCT).
		Scan(&ctLuot, &ctNganSach); err != nil {
		t.Fatalf("đọc bộ đếm chương trình: %v", err)
	}
	if ctLuot != 1 || ctNganSach != daGiam {
		t.Errorf("bộ đếm chương trình = (%d lượt, %d đ), cần (1, %d) — "+
			"bộ đếm là thứ `max_uses` và `max_budget` đọc",
			ctLuot, ctNganSach, daGiam)
	}

	// Lượt phải gắn ĐÚNG đơn — không thì hủy đơn không trả lại được lượt.
	var luotCuaDon int
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM coupon_usage WHERE order_id = $1`, maDon).
		Scan(&luotCuaDon); err != nil {
		t.Fatalf("đọc lượt theo đơn: %v", err)
	}
	if luotCuaDon != 1 {
		t.Errorf("đơn %s có %d lượt gắn với nó, cần 1", maDon, luotCuaDon)
	}

	// Nửa thứ hai: trần đã chạm, đơn sau KHÔNG được áp mã nữa.
	if _, apDuoc := a.datDonVoiMa(t, ma, "0900333444"); apDuoc {
		t.Fatal("mã trần MỘT lượt vẫn áp được cho đơn thứ hai — " +
			"trần lượt không có tác dụng")
	}
}

// TestPhatLaiEventKhongDemHaiLuot kiểm tính idempotent.
//
// Outbox giao ít nhất một lần: cùng một `checkout.completed` được phát lại
// là chuyện bình thường. Đếm hai lần sẽ làm mã trần một lượt cạn ngay ở
// đơn đầu, và ngân sách khuyến mãi cạn gấp đôi tốc độ thật.
func TestPhatLaiEventKhongDemHaiLuot(t *testing.T) {
	a := newAPITest(t)

	const ma = "PHATLAIKHONGDEM"
	maCT := a.dungMaCoGioiHan(t, ma, 1000, 5)

	if _, apDuoc := a.datDonVoiMa(t, ma, "0900555666"); !apDuoc {
		t.Fatal("không áp được mã")
	}
	a.phatEvent(t)

	luotSauLan1, _ := a.soLuotConHieuLuc(t, maCT)
	if luotSauLan1 != 1 {
		t.Fatalf("ghi được %d lượt sau lần phát đầu, cần 1", luotSauLan1)
	}

	// Phát lại CHÍNH event đó.
	//
	// Xóa luôn dấu "đã xử lý" của riêng bên nhận này. Bộ phát có sẵn một
	// lớp chống trùng (`event_processed`), nhưng lớp đó KHÔNG phải thứ
	// đang kiểm: nó che mất câu hỏi thật là `RecordUsage` tự nó có
	// idempotent không. Bỏ dấu đi thì bên nhận chạy lại thật sự — đúng
	// tình huống tiến trình chết sau khi bên nhận ghi xong nhưng trước
	// khi dấu kịp lưu.
	ctx := context.Background()
	if _, err := a.db.Pool().Exec(ctx,
		`DELETE FROM event_processed WHERE handler = $1`,
		"promotion.ghi_luot_dung_khi_hoan_tat"); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx, `
		UPDATE event_outbox SET published_at = NULL
		 WHERE event_type = 'checkout.completed'`); err != nil {
		t.Fatalf("đặt lại outbox: %v", err)
	}
	n := a.phatEvent(t)

	// Đọc lỗi của outbox TRƯỚC khi kết luận.
	//
	// Bên nhận không chịu nổi event lặp có HAI cách hỏng: đếm hai lần,
	// hoặc báo lỗi và bị thử lại mãi mãi. Không đọc `last_error` thì cách
	// thứ hai hiện ra dưới dạng "không event nào được phát" — một thông
	// điệp chỉ vào sai chỗ.
	var loiCuoi string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT coalesce(max(last_error), '') FROM event_outbox
		 WHERE event_type = 'checkout.completed'`).Scan(&loiCuoi); err != nil {
		t.Fatalf("đọc lỗi outbox: %v", err)
	}
	if loiCuoi != "" {
		t.Fatalf("phát lại event làm bên nhận BÁO LỖI: %s — "+
			"event sẽ bị thử lại mãi mãi và kẹt hàng đợi", loiCuoi)
	}
	if n == 0 {
		t.Fatal("không event nào được phát lại — bài kiểm không chạm tới " +
			"bên nhận, nên nó không chứng minh gì")
	}

	luotSauLan2, _ := a.soLuotConHieuLuc(t, maCT)
	if luotSauLan2 != 1 {
		t.Errorf("phát lại event làm lượt dùng thành %d, cần vẫn 1 — "+
			"giao ít nhất một lần sẽ thổi phồng bộ đếm", luotSauLan2)
	}
}

// TestHuyDonTraLaiLuotDungMa.
//
// Không trả lại thì khách dùng mã, đơn bị hủy, và họ MẤT quyền dùng mã —
// còn ngân sách khuyến mãi bị trừ cho một đơn không còn tồn tại.
func TestHuyDonTraLaiLuotDungMa(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const ma = "HUYTRALAILUOT"
	maCT := a.dungMaCoGioiHan(t, ma, 1000, 1)

	maDon, apDuoc := a.datDonVoiMa(t, ma, "0900777888")
	if !apDuoc {
		t.Fatal("không áp được mã")
	}
	a.phatEvent(t)

	if luot, _ := a.soLuotConHieuLuc(t, maCT); luot != 1 {
		t.Fatalf("trước khi hủy có %d lượt, cần 1", luot)
	}

	if err := a.mods.order.CancelOrder(ctx, maDon, "khách đổi ý"); err != nil {
		t.Fatalf("hủy đơn: %v", err)
	}
	a.phatEvent(t)

	luot, _ := a.soLuotConHieuLuc(t, maCT)
	if luot != 0 {
		t.Fatalf("sau khi hủy đơn còn %d lượt còn hiệu lực, cần 0 — "+
			"khách mất quyền dùng mã vì một đơn không còn tồn tại", luot)
	}

	// Và mã phải dùng lại được: đó mới là bằng chứng lượt đã trả THẬT.
	if _, apDuocLan2 := a.datDonVoiMa(t, ma, "0900777888"); !apDuocLan2 {
		t.Error("sau khi hủy đơn, mã vẫn không áp được — " +
			"bộ đếm chưa được trả lại dù hàng lượt đã đánh dấu giải phóng")
	}
}

// TestGoMaGiamGiaTinhLaiPhiVaThue.
//
// # Vì sao tuyến này từng không tồn tại
//
// `RemoveDiscount` có đủ ba tầng, và tầng ứng dụng làm đúng phần khó: gỡ
// mã làm tiền hàng TĂNG lại nên phí ship và thuế phải tính lại. Chỉ thiếu
// cửa vào — khách gõ nhầm mã là mắc kẹt với nó tới khi phiên hết hạn.
//
// Bài này kiểm cả phần khó: sau khi gỡ, tổng tiền phải TRỞ VỀ đúng số
// trước lúc áp mã, không phải chỉ trừ đi phần giảm.
func TestGoMaGiamGiaTinhLaiPhiVaThue(t *testing.T) {
	a := newAPITest(t)

	const ma = "GOMARATEST"
	a.dungMaCoGioiHan(t, ma, 1000, 0)

	maOffer := a.timOfferBanDuoc()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, khoaIdem())
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": maGio, "guest_email": "goma@example.com",
		"guest_phone": "0900999888",
	}, khoaIdem())
	maPhien, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Gỡ", "phone": "0900999888",
			"street_address": "3 Đường Thử", "ward": "P1",
			"district": "Q1", "province": "TP.HCM", "country_code": "VN",
		}, khoaIdem())
	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, khoaIdem())

	truoc := a.call(http.MethodGet, "/api/v1/checkout/"+maPhien, nil, nil)
	tongTruoc := tongTien(t, truoc.body)

	ap := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/coupon",
		map[string]any{"code": ma}, khoaIdem())
	if ap.code != http.StatusOK {
		t.Fatalf("áp mã: HTTP %d — %s", ap.code, ap.raw)
	}
	tongCoMa := tongTien(t, ap.body)
	if tongCoMa >= tongTruoc {
		t.Fatalf("áp mã xong tổng là %d, không nhỏ hơn %d — "+
			"bài kiểm không có gì để gỡ", tongCoMa, tongTruoc)
	}

	go1 := a.call(http.MethodDelete, "/api/v1/checkout/"+maPhien+"/coupon",
		nil, khoaIdem())
	if go1.code != http.StatusOK {
		t.Fatalf("gỡ mã: HTTP %d — %s — khách gõ nhầm mã là mắc kẹt với nó",
			go1.code, go1.raw)
	}

	tongSauGo := tongTien(t, go1.body)
	if tongSauGo != tongTruoc {
		t.Errorf("gỡ mã xong tổng là %d, cần trở về %d — "+
			"phí ship hoặc thuế chưa được tính lại", tongSauGo, tongTruoc)
	}

	// Idempotent: gỡ lần hai không phải lỗi.
	go2 := a.call(http.MethodDelete, "/api/v1/checkout/"+maPhien+"/coupon",
		nil, khoaIdem())
	if go2.code != http.StatusOK {
		t.Errorf("gỡ mã lần hai: HTTP %d — client thử lại sau khi mất mạng "+
			"không được nhận lỗi cho một việc đã xong", go2.code)
	}
}

// tongTien đọc tổng tiền của phiên từ body trả về.
func tongTien(t *testing.T, body map[string]any) int64 {
	t.Helper()
	tong, ok := body["total"].(map[string]any)
	if !ok {
		t.Fatalf("phiên không có trường total: %v", body)
	}
	v, _ := tong["amount"].(float64)
	return int64(v)
}
