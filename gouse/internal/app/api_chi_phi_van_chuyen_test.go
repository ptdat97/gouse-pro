package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// fixedPrice là bảng giá hãng vận chuyển cho test.
//
// # Vì sao nó NHẠY với phương thức giao
//
// Bản cũ có chữ ký `PricePerParcel(string) int64` — VỨT BỎ tham số và luôn
// trả cùng một giá. Bài test vì thế xanh bất kể `shipping_method` của đơn
// thực hiện có giá trị hay không, và nó rỗng trên CẢ 3.207 đơn thật: không
// ai từng gán trường đó. Một bản giả bỏ qua đầu vào thì che mất đúng cái
// đầu vào đang thiếu.
//
// Nay phương thức rỗng trả 0, nên bài test đỏ ngay nếu dây bị đứt lại.
type fixedPrice int64

func (g fixedPrice) PricePerParcel(phuongThuc string) int64 {
	if phuongThuc == "" {
		return 0
	}
	return int64(g)
}

// TestCarrierCostIsPostedAtHandover.
//
// # Bất biến
//
// Bàn giao kiện hàng cho hãng vận chuyển làm phát sinh NGHĨA VỤ TRẢ TIỀN,
// và nghĩa vụ đó phải nằm trong sổ cái ngay lúc đó.
//
// # Vì sao nó quan trọng
//
// Phí vận chuyển khách trả đã được ghi là doanh thu nền tảng từ ADR-0018.
// Thiếu vế chi phí thì doanh thu bị THỔI LÊN đúng bằng khoản chưa ghi, và
// lãi/lỗ mảng vận chuyển — con số duy nhất trả lời "thu phí ship như vậy
// là lãi hay lỗ" — không tồn tại.
func TestCarrierCostIsPostedAtHandover(t *testing.T) {
	a := newAPITest(t)

	const returnPrice = 18_000

	orderID := a.placeCODOrder(t, "chiphiship")
	a.emitEventWithCost(t, returnPrice)

	foID, sellerID := a.fulfillmentOfOrder(t, orderID)
	tokNB := a.createSellerToken(t, sellerID)

	// TRƯỚC khi bàn giao: chưa có nghĩa vụ nào với hãng.
	//
	// Không có khẳng định này thì một bên nhận ghi chi phí ở MỌI bước tiến
	// độ vẫn qua bài test — khóa idempotency giữ cho tổng đúng, nên chỉ
	// THỜI ĐIỂM sai, và thời điểm sai nghĩa là sổ ghi một nghĩa vụ chưa
	// phát sinh.
	if chiPhi, owedGoods := a.shippingBalance(t, foID); chiPhi != 0 || owedGoods != 0 {
		t.Errorf("chưa bàn giao mà đã ghi chi phí %d / nợ hãng %d", chiPhi, owedGoods)
	}

	res := a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/ship",
		map[string]any{"shipping_provider": "GHN", "tracking_number": "GHN-CP-1"},
		merge(idemKey(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code != http.StatusOK {
		t.Fatalf("bàn giao: HTTP %d — %s", res.code, res.raw)
	}
	a.emitEventWithCost(t, returnPrice)

	chiPhi, owedGoods := a.shippingBalance(t, foID)
	if chiPhi != returnPrice {
		t.Errorf("SHIPPING_EXPENSE = %d, cần %d — bàn giao rồi mà chi phí "+
			"chưa vào sổ", chiPhi, returnPrice)
	}
	if owedGoods != returnPrice {
		t.Errorf("CARRIER_PAYABLE = %d, cần %d", owedGoods, returnPrice)
	}

	// Phát lại event tiến độ KHÔNG được tính chi phí lần hai.
	a.republishProgress(t)
	a.emitEventWithCost(t, returnPrice)

	cost2, _ := a.shippingBalance(t, foID)
	if cost2 != chiPhi {
		t.Errorf("chi phí %d → %d sau khi phát lại event — tính hai lần cho "+
			"một kiện hàng", chiPhi, cost2)
	}
}

// TestWithNoCarrierPriceNoEntryIsPosted.
//
// Hệ thống KHÔNG biết giá thỏa thuận với hãng: biểu phí của fulfillment là
// phí KHÁCH TRẢ. Chưa khai thì không ghi — đặt một con số mặc định sẽ làm
// lãi vận chuyển trông như một sự thật đã đo, trong khi không ai đo cả.
func TestWithNoCarrierPriceNoEntryIsPosted(t *testing.T) {
	a := newAPITest(t)

	orderID := a.placeCODOrder(t, "chuakhaigia")
	a.emitEventWithCost(t, 0) // 0 = chưa khai

	foID, sellerID := a.fulfillmentOfOrder(t, orderID)
	tokNB := a.createSellerToken(t, sellerID)

	res := a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/ship",
		map[string]any{"shipping_provider": "GHN", "tracking_number": "GHN-CP-2"},
		merge(idemKey(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code != http.StatusOK {
		t.Fatalf("bàn giao: HTTP %d — %s", res.code, res.raw)
	}
	a.emitEventWithCost(t, 0)

	chiPhi, owedGoods := a.shippingBalance(t, foID)
	if chiPhi != 0 || owedGoods != 0 {
		t.Errorf("chưa khai giá mà vẫn ghi sổ: chi phí %d, nợ hãng %d — "+
			"con số này từ đâu ra?", chiPhi, owedGoods)
	}
}

// emitEventWithCost phát event kèm bên nhận ghi chi phí vận chuyển.
func (a *apiTest) emitEventWithCost(t *testing.T, priceObj int64) {
	t.Helper()
	log := logger.New("error", "text")
	bus := eventbus.NewDispatcher(a.db.Pool(), log)
	a.registerReceivers(bus, log)
	bus.Subscribe(payment.NewCarrierCostHandler(
		a.mods.payment, fixedPrice(priceObj), log))

	if _, err := bus.DispatchBatch(context.Background(), 200); err != nil {
		t.Fatalf("phát event: %v", err)
	}
}

// shippingBalance trả số dư SHIPPING_EXPENSE và CARRIER_PAYABLE của MỘT kiện.
func (a *apiTest) shippingBalance(t *testing.T, foID string) (chiPhi, owedGoods int64) {
	t.Helper()
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'SHIPPING_EXPENSE'), 0),
		       COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'CARRIER_PAYABLE'), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1`, foID).Scan(&chiPhi, &owedGoods); err != nil {
		t.Fatalf("đọc số dư vận chuyển: %v", err)
	}
	return chiPhi, owedGoods
}

// republishProgress đưa event tiến độ trở lại hàng đợi chưa xử lý.
func (a *apiTest) republishProgress(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.db.Pool().Exec(ctx, `
		UPDATE event_outbox SET published_at = NULL, attempts = 0
		 WHERE event_type = 'fulfillment.progress_changed'`); err != nil {
		t.Fatalf("đưa event trở lại hàng đợi: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx, `
		DELETE FROM event_processed
		 WHERE event_id IN (SELECT event_id FROM event_outbox
		                     WHERE event_type = 'fulfillment.progress_changed')`,
	); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
}

// placeCODOrder đặt một đơn COD, không áp mã giảm giá.
func (a *apiTest) placeCODOrder(t *testing.T, label string) string {
	t.Helper()

	checkoutID := a.buildCheckoutReadyToComplete(newEmail(label), "0900333222")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ := ord["id"].(string)
	if orderID == "" {
		t.Fatalf("không lấy được mã đơn: %s", res.raw)
	}
	return orderID
}
