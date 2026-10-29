package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/promotion"
)

// buildLimitedCoupon tạo một mã giảm giá có TRẦN số lượt dùng toàn cục.
func (a *apiTest) buildLimitedCoupon(
	t *testing.T, ma string, percentBP int32, usageCapValue int,
) string {
	t.Helper()
	ctx := context.Background()

	pr, err := a.mods.promotion.CreatePromotion(ctx, promotion.CreatePromotionRequest{
		Name: "Giới hạn lượt " + ma, Kind: "COUPON",
		DiscountType: "PERCENTAGE", DiscountBPS: percentBP,
		MaxUses:  usageCapValue,
		StartsAt: types.Now().Add(-time.Hour),
		EndsAt:   types.Now().Add(24 * time.Hour),
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

// placeOrderWithCoupon đi trọn một đơn COD có áp mã, trả về mã đơn.
//
// applied = false nghĩa là bước áp mã BỊ TỪ CHỐI — khi đó trả về "".
func (a *apiTest) placeOrderWithCoupon(t *testing.T, ma, sdt string) (orderID string, applied bool) {
	t.Helper()

	maOffer := a.findSellableOffer()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, idemKey())
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": cartID, "guest_email": "luot@example.com",
		"guest_phone": sdt,
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	got := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/coupon",
		map[string]any{"code": ma}, idemKey())
	if got.code != http.StatusOK {
		return "", false
	}

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Lượt", "phone": sdt,
			"street_address": "2 Đường Thử", "ward": "P1",
			"district": "Q1", "province": "TP.HCM", "country_code": "VN",
		}, idemKey())
	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, idemKey())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ = ord["id"].(string)
	return orderID, true
}

// validUsesLeft đếm lượt dùng CHƯA bị giải phóng của một chương trình.
func (a *apiTest) validUsesLeft(t *testing.T, maCT string) (visits int, discounted int64) {
	t.Helper()
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT count(*), coalesce(sum(discount_amount), 0)
		  FROM coupon_usage
		 WHERE promotion_id = $1 AND released_at IS NULL`, maCT).
		Scan(&visits, &discounted); err != nil {
		t.Fatalf("đọc lượt dùng: %v", err)
	}
	return visits, discounted
}

// TestCouponUsesAreRecordedAndTheCapBlocksLaterOrders.
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
func TestCouponUsesAreRecordedAndTheCapBlocksLaterOrders(t *testing.T) {
	a := newAPITest(t)

	const ma = "TRANMOTLUOT"
	maCT := a.buildLimitedCoupon(t, ma, 1000, 1) // giảm 10%, tối đa MỘT lượt

	orderID, applied := a.placeOrderWithCoupon(t, ma, "0900111222")
	if !applied {
		t.Fatal("đơn ĐẦU TIÊN bị từ chối mã — trần lượt chặn nhầm")
	}

	// Trước khi phát event: chưa có gì được ghi, và đó là điều bình
	// thường — lượt được ghi bởi bên nhận, không phải trong giao dịch
	// đặt đơn.
	a.emitEvent(t)

	visits, discountedAlready := a.validUsesLeft(t, maCT)
	if visits != 1 {
		t.Fatalf("sau một đơn dùng mã, ghi được %d lượt — cần 1. "+
			"Bộ đếm không tăng nghĩa là mọi giới hạn của mã đều vô hiệu", visits)
	}
	if discountedAlready <= 0 {
		t.Errorf("lượt được ghi nhưng số tiền giảm là %d — "+
			"ngân sách khuyến mãi sẽ không bao giờ cạn", discountedAlready)
	}

	var usageCount int
	var budgetSpent int64
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT used_count, used_budget FROM promotion WHERE id = $1`, maCT).
		Scan(&usageCount, &budgetSpent); err != nil {
		t.Fatalf("đọc bộ đếm chương trình: %v", err)
	}
	if usageCount != 1 || budgetSpent != discountedAlready {
		t.Errorf("bộ đếm chương trình = (%d lượt, %d đ), cần (1, %d) — "+
			"bộ đếm là thứ `max_uses` và `max_budget` đọc",
			usageCount, budgetSpent, discountedAlready)
	}

	// Lượt phải gắn ĐÚNG đơn — không thì hủy đơn không trả lại được lượt.
	var orderVisits int
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM coupon_usage WHERE order_id = $1`, orderID).
		Scan(&orderVisits); err != nil {
		t.Fatalf("đọc lượt theo đơn: %v", err)
	}
	if orderVisits != 1 {
		t.Errorf("đơn %s có %d lượt gắn với nó, cần 1", orderID, orderVisits)
	}

	// Nửa thứ hai: trần đã chạm, đơn sau KHÔNG được áp mã nữa.
	if _, applied := a.placeOrderWithCoupon(t, ma, "0900333444"); applied {
		t.Fatal("mã trần MỘT lượt vẫn áp được cho đơn thứ hai — " +
			"trần lượt không có tác dụng")
	}
}

// TestARedeliveredEventIsNotCountedTwice kiểm tính idempotent.
//
// Outbox giao ít nhất một lần: cùng một `checkout.completed` được phát lại
// là chuyện bình thường. Đếm hai lần sẽ làm mã trần một lượt cạn ngay ở
// đơn đầu, và ngân sách khuyến mãi cạn gấp đôi tốc độ thật.
func TestARedeliveredEventIsNotCountedTwice(t *testing.T) {
	a := newAPITest(t)

	const ma = "PHATLAIKHONGDEM"
	maCT := a.buildLimitedCoupon(t, ma, 1000, 5)

	if _, applied := a.placeOrderWithCoupon(t, ma, "0900555666"); !applied {
		t.Fatal("không áp được mã")
	}
	a.emitEvent(t)

	visitsAfter1, _ := a.validUsesLeft(t, maCT)
	if visitsAfter1 != 1 {
		t.Fatalf("ghi được %d lượt sau lần phát đầu, cần 1", visitsAfter1)
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
	n := a.emitEvent(t)

	// Đọc lỗi của outbox TRƯỚC khi kết luận.
	//
	// Bên nhận không chịu nổi event lặp có HAI cách hỏng: đếm hai lần,
	// hoặc báo lỗi và bị thử lại mãi mãi. Không đọc `last_error` thì cách
	// thứ hai hiện ra dưới dạng "không event nào được phát" — một thông
	// điệp chỉ vào sai chỗ.
	var lastErr string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT coalesce(max(last_error), '') FROM event_outbox
		 WHERE event_type = 'checkout.completed'`).Scan(&lastErr); err != nil {
		t.Fatalf("đọc lỗi outbox: %v", err)
	}
	if lastErr != "" {
		t.Fatalf("phát lại event làm bên nhận BÁO LỖI: %s — "+
			"event sẽ bị thử lại mãi mãi và kẹt hàng đợi", lastErr)
	}
	if n == 0 {
		t.Fatal("không event nào được phát lại — bài kiểm không chạm tới " +
			"bên nhận, nên nó không chứng minh gì")
	}

	visitsAfter2, _ := a.validUsesLeft(t, maCT)
	if visitsAfter2 != 1 {
		t.Errorf("phát lại event làm lượt dùng thành %d, cần vẫn 1 — "+
			"giao ít nhất một lần sẽ thổi phồng bộ đếm", visitsAfter2)
	}
}

// TestCancellingAnOrderGivesBackTheCouponUse.
//
// Không trả lại thì khách dùng mã, đơn bị hủy, và họ MẤT quyền dùng mã —
// còn ngân sách khuyến mãi bị trừ cho một đơn không còn tồn tại.
func TestCancellingAnOrderGivesBackTheCouponUse(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const ma = "HUYTRALAILUOT"
	maCT := a.buildLimitedCoupon(t, ma, 1000, 1)

	orderID, applied := a.placeOrderWithCoupon(t, ma, "0900777888")
	if !applied {
		t.Fatal("không áp được mã")
	}
	a.emitEvent(t)

	if visits, _ := a.validUsesLeft(t, maCT); visits != 1 {
		t.Fatalf("trước khi hủy có %d lượt, cần 1", visits)
	}

	if err := a.mods.order.CancelOrder(ctx, orderID, "khách đổi ý"); err != nil {
		t.Fatalf("hủy đơn: %v", err)
	}
	a.emitEvent(t)

	visits, _ := a.validUsesLeft(t, maCT)
	if visits != 0 {
		t.Fatalf("sau khi hủy đơn còn %d lượt còn hiệu lực, cần 0 — "+
			"khách mất quyền dùng mã vì một đơn không còn tồn tại", visits)
	}

	// Và mã phải dùng lại được: đó mới là bằng chứng lượt đã trả THẬT.
	if _, appliedSecondTime := a.placeOrderWithCoupon(t, ma, "0900777888"); !appliedSecondTime {
		t.Error("sau khi hủy đơn, mã vẫn không áp được — " +
			"bộ đếm chưa được trả lại dù hàng lượt đã đánh dấu giải phóng")
	}
}

// TestRemovingACouponRecomputesFeeAndTax.
//
// # Vì sao tuyến này từng không tồn tại
//
// `RemoveDiscount` có đủ ba tầng, và tầng ứng dụng làm đúng phần khó: gỡ
// mã làm tiền hàng TĂNG lại nên phí ship và thuế phải tính lại. Chỉ thiếu
// cửa vào — khách gõ nhầm mã là mắc kẹt với nó tới khi phiên hết hạn.
//
// Bài này kiểm cả phần khó: sau khi gỡ, tổng tiền phải TRỞ VỀ đúng số
// trước lúc áp mã, không phải chỉ trừ đi phần giảm.
func TestRemovingACouponRecomputesFeeAndTax(t *testing.T) {
	a := newAPITest(t)

	const ma = "GOMARATEST"
	a.buildLimitedCoupon(t, ma, 1000, 0)

	maOffer := a.findSellableOffer()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, idemKey())
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": cartID, "guest_email": "goma@example.com",
		"guest_phone": "0900999888",
	}, idemKey())
	checkoutID, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Gỡ", "phone": "0900999888",
			"street_address": "3 Đường Thử", "ward": "P1",
			"district": "Q1", "province": "TP.HCM", "country_code": "VN",
		}, idemKey())
	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, idemKey())

	before := a.call(http.MethodGet, "/api/v1/checkout/"+checkoutID, nil, nil)
	totalBefore := total(t, before.body)

	ap := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/coupon",
		map[string]any{"code": ma}, idemKey())
	if ap.code != http.StatusOK {
		t.Fatalf("áp mã: HTTP %d — %s", ap.code, ap.raw)
	}
	tongCoMa := total(t, ap.body)
	if tongCoMa >= totalBefore {
		t.Fatalf("áp mã xong tổng là %d, không nhỏ hơn %d — "+
			"bài kiểm không có gì để gỡ", tongCoMa, totalBefore)
	}

	go1 := a.call(http.MethodDelete, "/api/v1/checkout/"+checkoutID+"/coupon",
		nil, idemKey())
	if go1.code != http.StatusOK {
		t.Fatalf("gỡ mã: HTTP %d — %s — khách gõ nhầm mã là mắc kẹt với nó",
			go1.code, go1.raw)
	}

	totalAfterRemoval := total(t, go1.body)
	if totalAfterRemoval != totalBefore {
		t.Errorf("gỡ mã xong tổng là %d, cần trở về %d — "+
			"phí ship hoặc thuế chưa được tính lại", totalAfterRemoval, totalBefore)
	}

	// Idempotent: gỡ lần hai không phải lỗi.
	go2 := a.call(http.MethodDelete, "/api/v1/checkout/"+checkoutID+"/coupon",
		nil, idemKey())
	if go2.code != http.StatusOK {
		t.Errorf("gỡ mã lần hai: HTTP %d — client thử lại sau khi mất mạng "+
			"không được nhận lỗi cho một việc đã xong", go2.code)
	}
}

// total đọc tổng tiền của phiên từ body trả về.
func total(t *testing.T, body map[string]any) int64 {
	t.Helper()
	tong, ok := body["total"].(map[string]any)
	if !ok {
		t.Fatalf("phiên không có trường total: %v", body)
	}
	v, _ := tong["amount"].(float64)
	return int64(v)
}

// TestTheConversionRecordHasAllThreeSteps.
//
// # Vì sao bài này tồn tại
//
// `checkout.started` và `checkout.expired` được khai trong `eventbus` và
// đặc tả trong docs/02-domain/domain-events.md từ lâu — KHÔNG ai phát,
// KHÔNG ai nghe. Hệ quả đo trên dữ liệu thật:
//
//	add_to_cart    9.668
//	checkout_start     0   ← khúc giữa TRỐNG
//	order.placed   3.207
//
// Trong khi bảng `checkout` có 8.714 phiên, 5.522 trong đó bỏ dở (63,4%).
// Không có khúc giữa thì "khách rơi ở đâu" không trả lời được: rơi giữa
// GIỎ và phiên, hay rơi TRONG phiên, là hai vấn đề khác nhau với hai cách
// chữa khác nhau.
func TestTheConversionRecordHasAllThreeSteps(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maOffer := a.findSellableOffer()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, idemKey())
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": cartID, "guest_email": "pheu@example.com",
		"guest_phone": "0900123123",
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	var n int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(*) FROM event_outbox
		 WHERE event_type = 'checkout.started' AND aggregate_id = $1`,
		checkoutID).Scan(&n); err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}
	if n != 1 {
		t.Fatalf("mở phiên phát %d event checkout.started, cần 1 — "+
			"khúc giữa của phễu vẫn trống", n)
	}

	// Event phải mang GỐC CHUỖI là mã giỏ, nếu không bước thêm-giỏ và
	// bước thanh toán nằm ở hai chuỗi rời nhau.
	var corr string
	var rowCount int
	var subtotal int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT coalesce(correlation_id, ''),
		       (payload->>'line_count')::int,
		       (payload->>'subtotal')::bigint
		  FROM event_outbox
		 WHERE event_type = 'checkout.started' AND aggregate_id = $1`,
		checkoutID).Scan(&corr, &rowCount, &subtotal); err != nil {
		t.Fatalf("đọc payload: %v", err)
	}
	if corr != cartID {
		t.Errorf("correlation_id = %q, cần mã giỏ %q — phễu đứt làm đôi",
			corr, cartID)
	}
	if rowCount != 1 || subtotal <= 0 {
		t.Errorf("payload có %d dòng và subtotal %d — phiên bỏ dở sẽ không "+
			"đo được giá trị", rowCount, subtotal)
	}
}

// TestAnExpiredCheckoutEmitsAnEventSoItCanBeMeasured.
//
// 63,4% phiên bỏ dở mà analytics không thấy cái nào. Bài này kiểm mắt xích
// còn lại: job dọn phải phát `checkout.expired`.
func TestAnExpiredCheckoutEmitsAnEventSoItCanBeMeasured(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maOffer := a.findSellableOffer()
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}
	res := a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, idemKey())
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": cartID, "guest_email": "hethan@example.com",
		"guest_phone": "0900321321",
	}, idemKey())
	checkoutID, _ := res.body["id"].(string)

	// Đẩy phiên thành quá hạn. Sửa thẳng DB vì không có đường nào cho
	// khách hay quản trị làm một phiên già đi.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE checkout SET expires_at = now() - interval '1 hour' WHERE id = $1`,
		checkoutID); err != nil {
		t.Fatalf("đẩy phiên quá hạn: %v", err)
	}

	if _, err := a.mods.checkout.ExpireStale(ctx, 50); err != nil {
		t.Fatalf("dọn phiên quá hạn: %v", err)
	}

	var n int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(*) FROM event_outbox
		 WHERE event_type = 'checkout.expired' AND aggregate_id = $1`,
		checkoutID).Scan(&n); err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}
	if n != 1 {
		t.Fatalf("dọn phiên quá hạn phát %d event checkout.expired, cần 1 — "+
			"phiên bỏ dở vẫn vô hình với analytics", n)
	}
}
