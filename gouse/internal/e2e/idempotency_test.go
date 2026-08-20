package e2e_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
)

// TestBanGiaoVanChuyenDongThoiChiMotLan — idempotency của bước GIAO HÀNG.
//
// # Vì sao bước này cần chống trùng ở tầng dữ liệu
//
// Nhà bán bấm "Đã bàn giao" hai lần, hoặc mạng chập chờn khiến client gửi
// lại. Kiểm tra trạng thái trước khi ghi KHÔNG đủ khi hai request chạy
// song song: cả hai cùng đọc PACKED, cả hai cùng thấy hợp lệ, cả hai cùng
// ghi.
//
// Hậu quả không chỉ là một dòng thừa: mỗi lần ghi phát một event
// `fulfillment.progress`, nên khách nhận HAI email "đơn đã gửi", analytics
// đếm hai lần, và mã vận đơn ghi sau đè lên mã ghi trước — nếu hai request
// mang hai mã khác nhau thì mã thật bị mất.
func TestBanGiaoVanChuyenDongThoiChiMotLan(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	foID := datVaGiao(t, w, shop, skuID, 20, 3)
	seller := shop.String()

	// Đưa tới PACKED — trạng thái ngay trước khi bàn giao.
	if err := w.ful.ConfirmFulfillment(ctx, seller, foID); err != nil {
		t.Fatalf("xác nhận: %v", err)
	}
	if err := w.ful.MarkPicking(ctx, seller, foID); err != nil {
		t.Fatalf("nhặt hàng: %v", err)
	}
	if err := w.ful.MarkPacked(ctx, seller, foID); err != nil {
		t.Fatalf("đóng gói: %v", err)
	}
	w.drain()

	const songSong = 8
	var start, done sync.WaitGroup
	start.Add(1)

	var mu sync.Mutex
	var thanhCong int

	for i := 0; i < songSong; i++ {
		done.Add(1)
		go func(n int) {
			defer done.Done()
			start.Wait()
			err := w.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID:       seller,
				FulfillmentID:  foID,
				Provider:       "GHN",
				TrackingNumber: "GHN-" + string(rune('A'+n)),
			})
			if err == nil {
				mu.Lock()
				thanhCong++
				mu.Unlock()
			}
		}(i)
	}
	start.Done()
	done.Wait()

	if thanhCong != 1 {
		t.Errorf("%d lệnh bàn giao cùng thành công, cần đúng 1", thanhCong)
	}

	// Và chỉ MỘT event tiến độ được phát cho bước này.
	if n := demEventTienDo(t, w, foID); n != 1 {
		t.Errorf("%d event fulfillment.progress cho bước bàn giao, cần 1", n)
	}
}

// demEventTienDo đếm event tiến độ mang trạng thái HANDED_OVER của một FO.
func demEventTienDo(t *testing.T, w *world, foID string) int {
	t.Helper()
	var n int
	err := w.db.Pool().QueryRow(context.Background(), `
		SELECT count(*) FROM event_outbox
		 WHERE event_type = 'fulfillment.progress_changed'
		   AND payload->>'fulfillment_id' = $1
		   AND payload->>'new_status' = 'HANDED_OVER'`, foID).Scan(&n)
	if err != nil {
		t.Fatalf("đếm event: %v", err)
	}
	return n
}

// TestGiuHangTrungChoCungPhienBiChan — idempotency của bước GIỮ HÀNG.
//
// Một phiên thanh toán được giữ hàng HAI LẦN cho cùng một món nghĩa là
// khóa gấp đôi số hàng khách thật sự cần. Số thừa bị treo tới khi hết hạn,
// và với hàng bán chạy đó là cách tự tạo ra tình trạng hết hàng giả.
//
// Bảo vệ ở tầng ứng dụng đã có (một giỏ một phiên đang chạy). Bài này khóa
// bất biến ở tầng DỮ LIỆU: dù bằng đường nào đi nữa, một phiên không được
// có hai lần giữ hàng ĐANG HOẠT ĐỘNG cho cùng một bản ghi tồn kho.
func TestGiuHangTrungChoCungPhienBiChan(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, 50)

	items, err := w.inv.GetItemsBySKUs(ctx, []string{skuID.String()}, "")
	if err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	itemID := items[skuID.String()][0].ID
	checkoutID := ids.MustNew(ids.PrefixCheckout).String()

	if _, err := w.inv.Reserve(ctx, inventory.ReserveRequest{
		ItemID: itemID, CheckoutID: checkoutID, Quantity: 3,
		TTL: 15 * time.Minute,
	}); err != nil {
		t.Fatalf("giữ hàng lần 1: %v", err)
	}

	// Lần thứ hai cho CÙNG phiên, CÙNG bản ghi tồn kho.
	_, err = w.inv.Reserve(ctx, inventory.ReserveRequest{
		ItemID: itemID, CheckoutID: checkoutID, Quantity: 3,
		TTL: 15 * time.Minute,
	})
	if err == nil {
		t.Fatal("giữ hàng lần 2 THÀNH CÔNG — một phiên khóa hàng hai lần")
	}

	// Và hàng chỉ bị khóa MỘT lần.
	if giu := w.reserved(skuID, shop); giu != 3 {
		t.Errorf("đang giữ %d, cần 3 — đã khóa hai lần", giu)
	}
}
