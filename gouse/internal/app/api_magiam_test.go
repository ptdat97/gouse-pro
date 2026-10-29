package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/promotion"
)

// buildCoupon tạo một chương trình đang chạy và một mã dùng được.
func (a *apiTest) buildCoupon(t *testing.T, ma string, percentBP int32) {
	t.Helper()
	ctx := context.Background()

	pr, err := a.mods.promotion.CreatePromotion(ctx, promotion.CreatePromotionRequest{
		Name: "Thử phân bổ " + ma, Kind: "COUPON",
		DiscountType: "PERCENTAGE", DiscountBPS: percentBP,
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
}

// TestACouponWorksAndTheReturnRefundsWhatWasActuallyPaid.
//
// # Vì sao hai việc này phải kiểm CÙNG NHAU
//
// Nối module promotion vào ứng dụng mở ra đường tạo đơn CÓ giảm giá. Nếu
// phần giảm không được phân bổ xuống từng dòng, mọi đơn như thế sẽ không
// trả hàng tự động được — hoặc tệ hơn, hoàn theo giá niêm yết và trả ra
// nhiều hơn đã thu.
//
// Nên bài này đi trọn: áp mã → đặt đơn → giao → trả một món → kiểm số tiền
// hoàn bằng ĐÚNG giá thực trả.
func TestACouponWorksAndTheReturnRefundsWhatWasActuallyPaid(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const ma = "THUPHANBO10"
	a.buildCoupon(t, ma, 1000) // giảm 10%

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
		"cart_id": cartID, "guest_email": "magiam@example.com",
		"guest_phone": "0900246810",
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	// ÁP MÃ — đường trước đây trả "chưa sẵn sàng".
	got := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/coupon",
		map[string]any{"code": ma}, idemKey())
	if got.code != http.StatusOK {
		t.Fatalf("áp mã giảm giá: HTTP %d — %s", got.code, got.raw)
	}

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Mã", "phone": "0900246810",
			"street_address": "1 Đường Thử", "ward": "P1",
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
	orderID, _ := ord["id"].(string)

	// Phần giảm phải ĐÃ được đóng băng xuống dòng hàng.
	var orderDowngrade, totalAdjustments int64
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT discount_amount FROM "order" WHERE id = $1`, orderID).
		Scan(&orderDowngrade); err != nil {
		t.Fatalf("đọc giảm giá cấp đơn: %v", err)
	}
	if orderDowngrade <= 0 {
		t.Fatalf("đơn KHÔNG có giảm giá (%d) — mã không được áp", orderDowngrade)
	}
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT coalesce(sum(a.amount), 0) FROM order_line_adjustment a
		  JOIN order_line l ON l.id = a.order_line_id
		 WHERE l.order_id = $1`, orderID).Scan(&totalAdjustments); err != nil {
		t.Fatalf("đọc khoản điều chỉnh: %v", err)
	}
	if totalAdjustments != -orderDowngrade {
		t.Fatalf("giảm cấp đơn %d nhưng tổng phân bổ xuống dòng là %d — "+
			"trả hàng sẽ hoàn sai", orderDowngrade, totalAdjustments)
	}

	// --- giao hàng rồi trả một món ---
	a.emitEvent(t)
	fo := a.findFulfillment(t, orderID)
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
				Provider: "ghn", TrackingNumber: "MG-" + fo.id[4:14],
			})
		},
		func() error { return a.mods.fulfillment.MarkDelivered(ctx, fo.sellerID, fo.id) },
	} {
		if err := b(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}
	a.emitEvent(t)

	var lineID string
	var listPrice int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT id, unit_price * quantity FROM order_line
		 WHERE order_id = $1 LIMIT 1`, orderID).Scan(&lineID, &listPrice); err != nil {
		t.Fatalf("đọc dòng hàng: %v", err)
	}

	ret := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": lineID, "quantity": 1, "reason_code": "SIZE_TOO_SMALL",
		}},
	}, merge(idemKey(), map[string]string{"X-Guest-Phone": "0900246810"}))
	if ret.code != http.StatusCreated {
		t.Fatalf("xin trả hàng đơn CÓ mã giảm giá: HTTP %d — %s — "+
			"hàng rào chống hoàn thừa vẫn đang chặn", ret.code, ret.raw)
	}

	yc, _ := ret.body["return"].(map[string]any)
	cash, _ := yc["refund_amount"].(map[string]any)
	refunded, _ := cash["amount"].(float64)

	// Hoàn phải bằng giá thực trả = niêm yết − phần giảm đã phân bổ.
	var lineDiscount int64
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT coalesce(sum(amount), 0) FROM order_line_adjustment
		  WHERE order_line_id = $1`, lineID).Scan(&lineDiscount); err != nil {
		t.Fatalf("đọc giảm của dòng: %v", err)
	}
	can := listPrice + lineDiscount // lineDiscount là số ÂM

	if int64(refunded) != can {
		t.Errorf("hoàn %v, cần %d (niêm yết %d, giảm đã phân bổ %d) — "+
			"hoàn theo giá niêm yết là trả nhiều hơn khách đã đưa",
			refunded, can, listPrice, lineDiscount)
	}
	if int64(refunded) >= listPrice {
		t.Errorf("hoàn %v KHÔNG nhỏ hơn giá niêm yết %d — phần giảm bị bỏ qua",
			refunded, listPrice)
	}
}
