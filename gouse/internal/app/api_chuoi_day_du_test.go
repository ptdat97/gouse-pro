package app

import (
	"context"
	"net/http"
	"testing"
)

// TestTheFullPrepaidChain là P3-3: Product → Offer → Cart → Checkout →
// Order → Payment → Fulfillment, đi HẾT trong một bài, qua HTTP thật.
//
// # Vì sao bài này trước nay KHÔNG viết được
//
// Mắt xích PAYMENT không tồn tại: `payment_intent` chỉ có từ PH-36
// (06/09), và `order.paid` — thứ nối tiền về với việc giao hàng — chỉ có
// từ ADR-0018 (07/09). Trước đó chuỗi đứt ở giữa, và mỗi nửa được kiểm
// riêng bằng bản giả của nửa kia.
//
// # Vì sao nó bắt được thứ các bài lẻ không bắt
//
// Mỗi bước dưới đây do một module khác nhau sở hữu, và chỗ hỏng nằm ở
// KHOẢNG GIỮA — đúng loại lỗi mà `internal/e2e/doc.go` mô tả. Ba lần đã
// xảy ra: `payment_method` bị vứt ở tầng HTTP (P3-9), `size_chart` khai mà
// không ai điền (P3-22), và bút toán ghi tiền mặt cho đơn chưa thu tiền
// (ADR-0018). Không bài lẻ nào thấy được chúng.
//
// Bài này khẳng định TRẠNG THÁI TIỀN ở từng chặng, không chỉ mã HTTP —
// một chuỗi trả 200 ở mọi bước vẫn có thể ghi sai sổ ở bước thứ tư.
func TestTheFullPrepaidChain(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	// ---- Product → Offer → Cart → Checkout → Order ----
	orderID, tong := a.prepaidOrder(t, newEmail("chuoi"), "0900777111")

	if tt := a.orderStatus(t, orderID); tt != "PENDING_PAYMENT" {
		t.Fatalf("sau khi đặt: trạng thái = %q, cần PENDING_PAYMENT", tt)
	}
	a.emitEvent(t)

	// Đơn thực hiện đã tạo, nhưng KHÓA — tiền chưa về (ADR-0018 A2).
	foID, sellerID := a.fulfillmentOfOrder(t, orderID)
	if !a.lockedAwaitingPayment(t, foID) {
		t.Fatal("đơn thực hiện của đơn TRẢ TRƯỚC phải bị khóa chờ thanh toán")
	}

	// Sổ cái: KHÁCH NỢ, chưa có đồng tiền mặt nào (ADR-0018 B1).
	if cash, receivable := a.ledgerBalance(t, orderID); cash != 0 || receivable <= 0 {
		t.Errorf("sau khi đặt: tiền mặt = %d (cần 0), phải thu = %d (cần > 0)",
			cash, receivable)
	}

	// Nhà bán CHƯA được giao hàng.
	tokNB := a.createSellerToken(t, sellerID)
	res := a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/ship",
		map[string]any{"shipping_provider": "GHN", "tracking_number": "GHN-CHUOI-1"},
		merge(idemKey(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code < 400 {
		t.Fatalf("nhà bán GIAO ĐƯỢC hàng khi tiền chưa về: HTTP %d — %s",
			res.code, res.raw)
	}

	// ---- Payment: webhook của cổng thanh toán ----
	whRes := a.callPaymentWebhook(t, "cong-tt", map[string]any{
		"event_id":   "evt_chuoi_day_du_1",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_chuoi_1",
			"amount":            tong,
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": orderID},
		},
	}, paymentGatewaySecret)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook thanh toán: HTTP %d — %s", whRes.code, whRes.raw)
	}
	if tt := a.orderStatus(t, orderID); tt != "PAID" {
		t.Fatalf("sau webhook: trạng thái = %q, cần PAID", tt)
	}

	// `order.paid` đi qua outbox tới hai bên nhận.
	a.emitEvent(t)

	// ---- Fulfillment: nay mới được giao ----
	if a.lockedAwaitingPayment(t, foID) {
		t.Fatal("thu tiền xong mà đơn thực hiện vẫn bị khóa")
	}

	// Sổ cái: khoản phải thu đã thành TIỀN MẶT.
	cash, receivable := a.ledgerBalance(t, orderID)
	if receivable != 0 {
		t.Errorf("sau khi thu tiền: phải thu = %d, cần 0", receivable)
	}
	if cash != int64(tong) {
		t.Errorf("tiền mặt = %d, cần %d — đúng số tiền đã thu", cash, tong)
	}

	res = a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/ship",
		map[string]any{"shipping_provider": "GHN", "tracking_number": "GHN-CHUOI-1"},
		merge(idemKey(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code != http.StatusOK {
		t.Fatalf("bàn giao vận chuyển: HTTP %d — %s", res.code, res.raw)
	}

	res = a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/deliver", nil,
		merge(idemKey(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code != http.StatusOK {
		t.Fatalf("giao hàng: HTTP %d — %s", res.code, res.raw)
	}
	a.emitEvent(t)

	// ---- Kết: đơn đã giao, tiền đã thu, sổ cân ----
	var foStatus string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT status FROM fulfillment_order WHERE id = $1`, foID,
	).Scan(&foStatus); err != nil {
		t.Fatalf("đọc trạng thái đơn thực hiện: %v", err)
	}
	if foStatus != "DELIVERED" {
		t.Errorf("đơn thực hiện = %q, cần DELIVERED", foStatus)
	}

	// Bất biến cuối cùng của mọi sổ kép: Σ NỢ = Σ CÓ.
	var no, co int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT COALESCE(SUM(l.amount) FILTER (WHERE l.direction = 'DEBIT'), 0),
		       COALESCE(SUM(l.amount) FILTER (WHERE l.direction = 'CREDIT'), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1`, orderID).Scan(&no, &co); err != nil {
		t.Fatalf("đọc sổ cái: %v", err)
	}
	if no != co {
		t.Errorf("sổ cái LỆCH: Σ nợ = %d, Σ có = %d", no, co)
	}
}

// fulfillmentOfOrder trả đơn thực hiện đầu tiên của một đơn hàng.
func (a *apiTest) fulfillmentOfOrder(t *testing.T, orderID string) (foID, sellerID string) {
	t.Helper()
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT id, seller_id FROM fulfillment_order WHERE order_id = $1
		  ORDER BY fo_number LIMIT 1`, orderID).Scan(&foID, &sellerID); err != nil {
		t.Fatalf("đọc đơn thực hiện của đơn %s: %v", orderID, err)
	}
	return foID, sellerID
}

func (a *apiTest) lockedAwaitingPayment(t *testing.T, foID string) bool {
	t.Helper()
	var key bool
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT awaiting_payment FROM fulfillment_order WHERE id = $1`,
		foID).Scan(&key); err != nil {
		t.Fatalf("đọc cờ chờ thanh toán: %v", err)
	}
	return key
}

// ledgerBalance trả số dư PLATFORM_CASH và ACCOUNTS_RECEIVABLE CỦA MỘT ĐƠN.
//
// Lọc theo `reference_id`, không cộng cả bảng: các bài khác trong gói dùng
// chung database, nên một phép cộng toàn cục xanh khi chạy riêng và đỏ khi
// chạy cả gói — đúng kiểu test chập chờn mà P3-2 đã dọn một lần.
func (a *apiTest) ledgerBalance(t *testing.T, orderID string) (cash, receivable int64) {
	t.Helper()
	rows, err := a.db.Pool().Query(context.Background(), `
		SELECT l.account_type,
		       COALESCE(SUM(CASE WHEN l.direction = 'DEBIT'
		                         THEN l.amount ELSE -l.amount END), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1
		   AND l.account_type IN ('PLATFORM_CASH', 'ACCOUNTS_RECEIVABLE')
		 GROUP BY l.account_type`, orderID)
	if err != nil {
		t.Fatalf("đọc số dư sổ cái: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var acc string
		var v int64
		if err := rows.Scan(&acc, &v); err != nil {
			t.Fatalf("đọc số dư: %v", err)
		}
		if acc == "PLATFORM_CASH" {
			cash = v
		} else {
			receivable = v
		}
	}
	return cash, receivable
}

// TestTheFullChainWithADiscount — cùng chuỗi, nhưng đơn CÓ mã giảm giá.
//
// # Vì sao tách thành bài riêng
//
// Bài chuỗi ở trên tìm ra rằng PHÍ VẬN CHUYỂN không nằm ở đâu trong sổ
// cái. Khoản GIẢM GIÁ là câu hỏi đối xứng, và nó lệch theo hướng NGƯỢC
// LẠI: bút toán doanh thu ghi tổng dòng hàng GỐC, còn khách chỉ trả phần
// đã trừ. Nếu sổ không ghi khoản giảm thì sau khi thu tiền, khoản phải thu
// còn dư đúng bằng số tiền đã giảm — khách "nợ" vĩnh viễn một khoản không
// ai đòi.
//
// Đây chính là mục "còn mở" mà bài chuỗi đầu tiên ghi lại.
func TestTheFullChainWithADiscount(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const ma = "CHUOIGIAM10"
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
		"cart_id": cartID, "guest_email": newEmail("chuoigiam"),
		"guest_phone": "0900777222",
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	if got := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/coupon",
		map[string]any{"code": ma}, idemKey()); got.code != http.StatusOK {
		t.Fatalf("áp mã: HTTP %d — %s", got.code, got.raw)
	}
	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Giảm", "phone": "0900777222",
			"street_address": "1 Đường Thử", "ward": "P1",
			"district": "Q1", "province": "TP.HCM", "country_code": "VN",
		}, idemKey())
	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, idemKey())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "CARD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ := ord["id"].(string)
	tong, _ := ord["total"].(map[string]any)
	amount := int64(tong["amount"].(float64))

	a.emitEvent(t)

	// Khoản phải thu phải bằng ĐÚNG số khách sẽ trả — không hơn.
	_, receivable := a.ledgerBalance(t, orderID)
	if receivable != amount {
		t.Errorf("khoản phải thu = %d nhưng khách chỉ trả %d — lệch %d, "+
			"đúng bằng phần giảm giá không được ghi sổ",
			receivable, amount, receivable-amount)
	}

	// Thu tiền.
	whRes := a.callPaymentWebhook(t, "cong-tt", map[string]any{
		"event_id":   "evt_chuoi_giam_1",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_chuoi_giam",
			"amount":            amount,
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": orderID},
		},
	}, paymentGatewaySecret)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook: HTTP %d — %s", whRes.code, whRes.raw)
	}
	a.emitEvent(t)

	cash, receivable := a.ledgerBalance(t, orderID)
	if receivable != 0 {
		t.Errorf("sau khi thu tiền: phải thu = %d, cần 0 — khách còn 'nợ' "+
			"một khoản không ai đòi", receivable)
	}
	if cash != amount {
		t.Errorf("tiền mặt = %d, cần %d", cash, amount)
	}

	var no, co int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT COALESCE(SUM(l.amount) FILTER (WHERE l.direction = 'DEBIT'), 0),
		       COALESCE(SUM(l.amount) FILTER (WHERE l.direction = 'CREDIT'), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1`, orderID).Scan(&no, &co); err != nil {
		t.Fatalf("đọc sổ cái: %v", err)
	}
	if no != co {
		t.Errorf("sổ cái LỆCH: Σ nợ = %d, Σ có = %d", no, co)
	}
}
