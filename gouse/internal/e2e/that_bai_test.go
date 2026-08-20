package e2e_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
)

// TestMotMonHetHangThiNhaHetHangDaGiu — kịch bản "không đủ hàng giữa chừng".
//
// # Vì sao đây là nhánh dễ bỏ quên nhất
//
// Giỏ nhiều món được giữ hàng LẦN LƯỢT. Món đầu thành công, món sau hết
// hàng, và cả phiên thất bại. Câu hỏi là: món đầu đã giữ được thì sao?
//
// Bỏ quên việc nhả nó nghĩa là mỗi lần một món hết hàng, các món KHÁC
// trong giỏ bị khóa vô ích 15 phút — với hàng bán chạy, đó là chuỗi phản
// ứng: khách A không mua được vì khách B vừa thất bại.
//
// Lỗi này KHÔNG hiện ra ở đường thành công, và không có thông báo nào.
func TestMotMonHetHangThiNhaHetHangDaGiu(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	shop := ids.MustNew(ids.PrefixSeller)

	duHang := ids.MustNew(ids.PrefixSKU)
	w.stockFor(duHang, shop, 50)

	thieuHang := ids.MustNew(ids.PrefixSKU)
	w.stockFor(thieuHang, shop, 2)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items: []checkoutapp.CartItemSnapshot{
			// Món này giữ được…
			line(shop, duHang, 300_000, 3),
			// …món này thì không: cần 5, kho có 2.
			line(shop, thieuHang, 200_000, 5),
		},
	})

	_, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{
		CartID: cartID,
	})
	if err == nil {
		t.Fatal("mở phiên THÀNH CÔNG dù một món không đủ hàng")
	}
	if !errors.Is(err, checkoutapp.ErrOutOfStock) {
		t.Fatalf("lỗi sai loại: %v (cần ErrOutOfStock)", err)
	}

	// Món giữ được phải được NHẢ LẠI, đủ 50, không giữ chỗ nào.
	avail, _ := w.stock(duHang, shop)
	if avail != 50 {
		t.Errorf("món đủ hàng còn %d khả dụng, cần 50 — hàng bị khóa vô ích", avail)
	}
	if giu := w.reserved(duHang, shop); giu != 0 {
		t.Errorf("còn %d đang giữ chỗ sau khi phiên thất bại, cần 0", giu)
	}

	// Món thiếu hàng không được đụng tới.
	if avail, _ := w.stock(thieuHang, shop); avail != 2 {
		t.Errorf("món thiếu hàng còn %d, cần 2", avail)
	}
}

// TestHoanTatHaiLanCungKhoaChiRaMotDon — kịch bản "gửi trùng request".
//
// Khách bấm "Đặt hàng" hai lần, hoặc mạng chập chờn khiến client gửi lại.
// Hai đơn cho một giỏ nghĩa là khách bị trừ tiền hai lần và kho bị trừ
// hai lần.
//
// Khác test idempotency của riêng module order ở chỗ: bài này đi qua CẢ
// chuỗi, nên nó cũng khẳng định tồn kho chỉ chuyển sang cam kết MỘT lần
// — event phát lại không được cộng dồn.
func TestHoanTatHaiLanCungKhoaChiRaMotDon(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	shop := ids.MustNew(ids.PrefixSeller)

	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, 20)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 4)},
	})

	c, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{
		CartID: cartID,
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if _, err := w.checkout.SetShippingAddress(ctx, c.ID(), address()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := w.checkout.SetShippingMethod(ctx, c.ID(), "STANDARD"); err != nil {
		t.Fatalf("SetShippingMethod: %v", err)
	}

	// CÙNG một khóa, gửi hai lần.
	khoa := ids.MustNew(ids.PrefixRequest).String()
	lan1, err := w.checkout.CompleteCheckout(ctx, c.ID(), khoa)
	if err != nil {
		t.Fatalf("hoàn tất lần 1: %v", err)
	}
	lan2, err := w.checkout.CompleteCheckout(ctx, c.ID(), khoa)
	if err != nil {
		t.Fatalf("hoàn tất lần 2: %v", err)
	}

	if lan1.OrderID != lan2.OrderID {
		t.Fatalf("hai đơn khác nhau: %s và %s", lan1.OrderID, lan2.OrderID)
	}

	w.drain()

	// Kho trừ ĐÚNG MỘT LẦN: 20 − 4 = 16 khả dụng, 4 cam kết.
	avail, commit := w.stock(skuID, shop)
	if avail != 16 || commit != 4 {
		t.Errorf("tồn kho %d khả dụng / %d cam kết, cần 16/4 — trừ hai lần?",
			avail, commit)
	}

	// Và đúng MỘT đơn thực hiện, không phải hai.
	fos, err := w.ful.GetOrderFulfillments(ctx, lan1.OrderID.String())
	if err != nil {
		t.Fatalf("GetOrderFulfillments: %v", err)
	}
	if len(fos) != 1 {
		t.Errorf("số đơn thực hiện = %d, cần 1", len(fos))
	}
}

// TestOwnBrandVaNhaBanKhongDungChungKhoKhiHetHang khóa chặt quy tắc
// KHÔNG có đường lùi, ở mức TOÀN HỆ THỐNG.
//
// Test cùng tên trong gói checkout kiểm điều này với inventory thật nhưng
// order và event giả. Bài này đi hết chuỗi: nếu có ai đó ở tầng trên
// "chữa cháy" bằng cách đổi sang kho khác, chỗ đó sẽ lộ ra ở đây.
func TestOwnBrandVaNhaBanKhongDungChungKhoKhiHetHang(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	ownBrand := ids.MustNew(ids.PrefixSeller)
	w.internal[ownBrand] = true
	shop := ids.MustNew(ids.PrefixSeller)

	// CÙNG một SKU: nền tảng thừa hàng, nhà bán chỉ còn 1.
	shared := ids.MustNew(ids.PrefixSKU)
	w.stockFor(shared, ids.ID(inventory.PlatformOwnerID), 100)
	w.stockFor(shared, shop, 1)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		// Mua 5 cái QUA OFFER CỦA NHÀ BÁN, người chỉ có 1.
		Items: []checkoutapp.CartItemSnapshot{line(shop, shared, 300_000, 5)},
	})

	_, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{
		CartID: cartID,
	})
	if err == nil {
		t.Fatal("mở phiên thành công — đã mượn hàng của nền tảng")
	}
	if !errors.Is(err, checkoutapp.ErrOutOfStock) {
		t.Fatalf("lỗi sai loại: %v", err)
	}

	if avail, _ := w.stock(shared, ids.ID(inventory.PlatformOwnerID)); avail != 100 {
		t.Errorf("hàng nền tảng bị đụng tới: còn %d, cần 100", avail)
	}
	if avail, _ := w.stock(shared, shop); avail != 1 {
		t.Errorf("hàng nhà bán còn %d, cần 1", avail)
	}
}
