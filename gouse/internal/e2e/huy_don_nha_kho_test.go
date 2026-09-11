package e2e_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
)

// TestHuyDonThiNHAKHO — mắt xích cuối của đường ra kho.
//
// # Bất biến
//
// Hủy cả ĐƠN HÀNG phải trả hàng về kho, y như hủy một đơn thực hiện.
//
// # Vì sao nó từng hở
//
// Đường VÀO kho có từ lâu: Reserved → Committed khi đặt hàng. Đường RA
// chỉ mở khi đơn THỰC HIỆN bị hủy — và không gì hủy chúng theo đơn hàng.
// Nên `CancelOrderAsAdmin` ghi audit, đổi trạng thái, rồi im lặng: hàng
// nằm mãi ở trạng thái cam kết, có thật trên kệ nhưng hệ thống coi là đã
// hứa cho một đơn không còn tồn tại.
//
// Chú thích của `inventory.ReleaseOnFulfillmentCancelled` đã mô tả đúng
// lỗi này cho đơn thực hiện, kèm lần kiểm chứng bằng đơn thật ngày 20/08:
// "đặt 5 món rồi hủy, tồn kho đứng nguyên 15 khả dụng / 5 cam kết. Không
// lỗi, không log."
func TestHuyDonThiNhaKho(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, 20)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "huydon@example.com",
		Currency:   money.VND,
		Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 5)},
	})

	c, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{CartID: cartID})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if _, err := w.checkout.SetShippingAddress(ctx, c.ID(), address()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := w.checkout.SetShippingMethod(ctx, c.ID(), "STANDARD"); err != nil {
		t.Fatalf("SetShippingMethod: %v", err)
	}
	res, err := w.checkout.CompleteCheckout(
		ctx, c.ID(), ids.MustNew(ids.PrefixRequest).String(), "COD")
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	w.drain()

	// Sau khi đặt: 15 khả dụng / 5 cam kết.
	avail, commit := w.stock(skuID, shop)
	if avail != 15 || commit != 5 {
		t.Fatalf("sau khi đặt: tồn kho %d/%d, cần 15/5", avail, commit)
	}

	// HỦY cả đơn.
	if err := w.ord.CancelOrder(ctx, res.OrderID.String(), "khách đổi ý"); err != nil {
		t.Fatalf("hủy đơn: %v", err)
	}
	w.drain()

	// Hàng phải TRỞ VỀ khả dụng.
	avail2, commit2 := w.stock(skuID, shop)
	if commit2 != 0 {
		t.Errorf("sau khi hủy: còn %d cam kết, cần 0 — hàng bị khóa cho một "+
			"đơn không còn tồn tại", commit2)
	}
	if avail2 != 20 {
		t.Errorf("sau khi hủy: %d khả dụng, cần 20 — hàng không trở về kệ",
			avail2)
	}
}

// TestHuyDonHaiLanKhongNhaKhoHaiLan.
//
// Event `order.cancelled` phát lại là đường đi bình thường. Nhả hai lần
// nghĩa là kho có nhiều hàng hơn thực tế, và nền tảng bán thứ không có.
func TestHuyDonHaiLanKhongNhaKhoHaiLan(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, 20)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "huy2@example.com",
		Currency:   money.VND,
		Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 5)},
	})

	c, _ := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{CartID: cartID})
	if _, err := w.checkout.SetShippingAddress(ctx, c.ID(), address()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := w.checkout.SetShippingMethod(ctx, c.ID(), "STANDARD"); err != nil {
		t.Fatalf("SetShippingMethod: %v", err)
	}
	res, err := w.checkout.CompleteCheckout(
		ctx, c.ID(), ids.MustNew(ids.PrefixRequest).String(), "COD")
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	w.drain()

	if err := w.ord.CancelOrder(ctx, res.OrderID.String(), "khách đổi ý"); err != nil {
		t.Fatalf("hủy đơn: %v", err)
	}
	w.drain()
	avail, _ := w.stock(skuID, shop)

	// Phát lại event hủy.
	if _, err := w.db.Pool().Exec(ctx, `
		UPDATE event_outbox SET published_at = NULL, attempts = 0
		 WHERE event_type = 'order.cancelled'`); err != nil {
		t.Fatalf("đưa event trở lại hàng đợi: %v", err)
	}
	if _, err := w.db.Pool().Exec(ctx, `
		DELETE FROM event_processed
		 WHERE event_id IN (SELECT event_id FROM event_outbox
		                     WHERE event_type = 'order.cancelled')`); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
	w.drain()

	avail2, _ := w.stock(skuID, shop)
	if avail2 != avail {
		t.Errorf("khả dụng %d → %d sau khi phát lại event hủy — nhả kho hai "+
			"lần, kho có nhiều hàng hơn thực tế", avail, avail2)
	}
}
