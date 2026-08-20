package e2e_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
)

// TestMuoiKhachTranhBaMonKhongAiMuaQua — bất biến QUAN TRỌNG NHẤT của cả
// hệ thống, kiểm ở mức TOÀN CHUỖI.
//
//	available >= 0   luôn đúng
//	KHÔNG oversell   dưới thanh toán đồng thời
//
// # Vì sao cần bài này khi module inventory đã có test tranh chấp
//
// Test kia chứng minh `Reserve` an toàn khi hai giao dịch PostgreSQL chạy
// song song trên cùng một dòng. Đúng, và cần thiết — nhưng nó gọi thẳng
// inventory.
//
// Bài này đi qua `StartCheckout`: đọc giỏ, tra chủ sở hữu tồn kho, chọn
// kho, giữ hàng, ghi phiên. Nhiều bước hơn nghĩa là nhiều chỗ hơn để một
// lần đọc-rồi-ghi lọt ra ngoài vòng khóa. Bán quá hàng là lỗi KHÔNG sửa
// được bằng xin lỗi: hàng không tồn tại thì không giao được.
func TestMuoiKhachTranhBaMonKhongAiMuaQua(t *testing.T) {
	w := newWorld(t)
	shop := ids.MustNew(ids.PrefixSeller)

	const kho = 3
	const khach = 10

	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, kho)

	// Mỗi khách một giỏ RIÊNG, mỗi giỏ mua 1 món.
	carts := make([]ids.ID, khach)
	for i := range carts {
		carts[i] = ids.MustNew(ids.PrefixCart)
		w.cart.put(checkoutapp.CartSnapshot{
			CartID:     carts[i],
			CustomerID: ids.MustNew(ids.PrefixCustomer),
			GuestEmail: "khach@example.com",
			Currency:   money.VND,
			Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 1)},
		})
	}

	// Thả cùng lúc: `start` giữ mọi goroutine lại tới khi tất cả sẵn sàng,
	// để chúng thật sự tranh nhau chứ không chạy nối đuôi.
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)

	var mu sync.Mutex
	var thanhCong int
	var loiLa []error

	for i := 0; i < khach; i++ {
		done.Add(1)
		go func(cartID ids.ID) {
			defer done.Done()
			start.Wait()

			_, err := w.checkout.StartCheckout(
				context.Background(),
				checkoutapp.StartCheckoutInput{CartID: cartID},
			)

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				thanhCong++
			case errors.Is(err, checkoutapp.ErrOutOfStock):
				// Đúng như mong đợi: hết hàng thì bị từ chối.
			default:
				loiLa = append(loiLa, err)
			}
		}(carts[i])
	}
	start.Done()
	done.Wait()

	for _, err := range loiLa {
		t.Errorf("lỗi ngoài dự kiến (không phải hết hàng): %v", err)
	}

	if thanhCong != kho {
		t.Errorf("%d khách giữ được hàng, kho chỉ có %d", thanhCong, kho)
	}

	avail, _ := w.stock(skuID, shop)
	giu := w.reserved(skuID, shop)

	if avail < 0 {
		t.Errorf("TỒN KHO ÂM: %d — đã bán quá hàng", avail)
	}
	if avail != 0 {
		t.Errorf("còn %d khả dụng, cần 0 (đã giữ hết)", avail)
	}
	if giu != kho {
		t.Errorf("đang giữ %d, cần đúng %d", giu, kho)
	}
}

// TestHaiTabCungGioChiGiuHangMotLan — cùng MỘT khách, hai tab.
//
// Khác bài trên ở chỗ đây không phải nhiều người tranh nhau mà là một
// người bấm hai lần. Mở phiên thứ hai cho cùng một giỏ sẽ giữ hàng LẦN
// THỨ HAI — tức khóa gấp đôi số hàng khách thật sự cần, và số hàng thừa
// đó bị treo 15 phút.
//
// Với hàng bán chạy, đó là cách tự tạo ra tình trạng hết hàng giả.
//
// # Phòng vệ HAI lớp — và vì sao phải biết điều đó khi đọc test này
//
// Bất biến "một giỏ một phiên" được cưỡng chế ở hai chỗ độc lập:
//
//	tầng ứng dụng   StartCheckout trả lại phiên đang chạy nếu đã có
//	tầng database   chỉ mục UNIQUE CÓ ĐIỀU KIỆN trên (cart_id, ACTIVE)
//
// cộng thêm một lớp thứ ba: `Save` thất bại thì hàng đã giữ được NHẢ lại.
//
// Kiểm chứng (20/08): bỏ RIÊNG chốt ở tầng ứng dụng thì test VẪN XANH —
// database bắt được, và đường nhả hàng dọn sạch. Phải bỏ CẢ chốt lẫn
// đường nhả mới thấy đỏ: "8 giữ chỗ, cần 4".
//
// Ghi lại điều này vì nó dễ dẫn tới kết luận sai theo cả hai chiều: thấy
// test xanh sau khi phá một lớp mà tưởng test vô dụng, hoặc tưởng một lớp
// là đủ nên gỡ lớp kia.
func TestHaiTabCungGioChiGiuHangMotLan(t *testing.T) {
	w := newWorld(t)
	shop := ids.MustNew(ids.PrefixSeller)

	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, 10)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 4)},
	})

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)

	var mu sync.Mutex
	phien := map[string]bool{}

	for i := 0; i < 8; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			c, err := w.checkout.StartCheckout(
				context.Background(),
				checkoutapp.StartCheckoutInput{CartID: cartID},
			)
			if err != nil {
				return
			}
			mu.Lock()
			phien[c.ID().String()] = true
			mu.Unlock()
		}()
	}
	start.Done()
	done.Wait()

	if len(phien) != 1 {
		t.Errorf("%d phiên được mở cho MỘT giỏ, cần 1", len(phien))
	}

	// Giữ đúng 4, không phải 8 hay 32.
	avail, _ := w.stock(skuID, shop)
	giu := w.reserved(skuID, shop)
	if avail != 6 || giu != 4 {
		t.Errorf("tồn kho %d khả dụng / %d giữ chỗ, cần 6/4 — giữ hàng nhiều lần",
			avail, giu)
	}
}
