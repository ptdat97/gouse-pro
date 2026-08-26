package e2e_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
)

// TestHaiJobDonHanKhongNhaHangHaiLan — tái hiện PH-31.
//
// # Hai job dọn hai đầu của CÙNG một sợi dây
//
//	inventory.ExpireReservations  mỗi 30 giây — nhả theo RESERVATION
//	checkout.ExpireStale          mỗi 60 giây — nhả theo PHIÊN
//
// Phiên hết hạn thì reservation của nó cũng hết hạn CÙNG LÚC, nên hai job
// nhắm vào đúng một bản ghi. Chúng chạy ở hai goroutine trong cùng một
// worker, và có thể chạm nhau trong vài mili giây.
//
// # Điều đã xảy ra thật (26/08)
//
//	18:04:22.826  RESERVE  1  → còn 76   ref=rsv_...GGG
//	18:19:28.497  RELEASE  1  → còn 77   ref=rsv_...GGG
//	18:19:28.499  RELEASE  1  → còn 78   ← nhả LẦN HAI, cách 1,5ms
//
// 78 cao hơn 77 trước khi giữ: hàng sinh ra từ không khí. Và một
// reservation khác kẹt ở ACTIVE vì phần giữ chỗ của nó bị lượt nhả thừa
// ăn mất — job dọn hạn hỏng vĩnh viễn sau đó.
//
// # Bài test này khẳng định gì — và KHÔNG khẳng định gì
//
// Chạy hai job SONG SONG trên cùng một phiên hết hạn. Dù đường nào thắng,
// tồn kho phải trở về đúng trạng thái trước khi giữ — không hơn.
//
// NÓ KHÔNG TÁI HIỆN được lỗi gốc. Đã thử: 8 lượt nhả song song, hai job
// dọn song song, và cả hai kịch bản đó CÓ và KHÔNG có khóa lạc quan —
// tám lần chạy đều xanh. Cửa sổ tranh chấp thật hẹp hơn thứ hai goroutine
// trong một tiến trình đánh trúng được.
//
// Giữ lại vì nó khóa đúng bất biến và chặn được hồi quy dạng khác. Nhưng
// KHÔNG được coi nó là bằng chứng rằng PH-31 đã được sửa — xem backlog
// mục 2.6e để biết phần còn chưa giải thích được.
func TestHaiJobDonHanKhongNhaHangHaiLan(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := h_stock(t, w, shop, 20)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "hethan@example.com",
		Currency:   money.VND,
		Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 3)},
	})

	// TTL cực ngắn để phiên hết hạn ngay, không phải chờ 15 phút.
	if _, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{
		CartID: cartID,
		TTL:    time.Millisecond,
	}); err != nil {
		t.Fatalf("mở phiên: %v", err)
	}

	avail, _ := w.stock(skuID, shop)
	giu := w.reserved(skuID, shop)
	if avail != 17 || giu != 3 {
		t.Fatalf("dựng sai: %d khả dụng / %d giữ chỗ, cần 17/3", avail, giu)
	}

	time.Sleep(20 * time.Millisecond)

	// HAI job dọn, chạy CÙNG LÚC — đúng cấu hình thật của worker.
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(2)

	var mu sync.Mutex
	var loi []error

	go func() {
		defer done.Done()
		start.Wait()
		if _, err := w.invMod.Service().ExpireReservations(ctx, 200); err != nil {
			mu.Lock()
			loi = append(loi, err)
			mu.Unlock()
		}
	}()
	go func() {
		defer done.Done()
		start.Wait()
		if _, err := w.checkout.ExpireStale(ctx, 200); err != nil {
			mu.Lock()
			loi = append(loi, err)
			mu.Unlock()
		}
	}()
	start.Done()
	done.Wait()

	// Tồn kho phải TRỞ VỀ ĐÚNG 20, không phải 23.
	avail, _ = w.stock(skuID, shop)
	giu = w.reserved(skuID, shop)

	if avail != 20 {
		t.Errorf("còn %d khả dụng, cần 20 — nhả hai lần đã SINH RA %d món hàng",
			avail, avail-20)
	}
	if giu != 0 {
		t.Errorf("còn %d giữ chỗ, cần 0", giu)
	}

	// KHÔNG được để lại reservation ACTIVE nào: đó là thứ làm job dọn hạn
	// hỏng vĩnh viễn sau sự cố thật.
	var conActive int
	if err := w.db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM reservation WHERE status = 'ACTIVE'`,
	).Scan(&conActive); err != nil {
		t.Fatalf("đếm reservation: %v", err)
	}
	if conActive != 0 {
		t.Errorf("còn %d reservation kẹt ở ACTIVE", conActive)
	}

	for _, e := range loi {
		t.Logf("lỗi từ job dọn (có thể chấp nhận được nếu tồn kho vẫn đúng): %v", e)
	}
}

// h_stock nhập kho cho một nhà bán và trả mã SKU.
func h_stock(t *testing.T, w *world, seller ids.ID, qty int) ids.ID {
	t.Helper()
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, w.ownerOf(seller), qty)
	return skuID
}
