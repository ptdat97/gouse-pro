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

// TestHoanTatDongThoiKhongSinhHangTuKhongKhi là NỬA CÒN LẠI của bất biến
// quan trọng nhất hệ thống — phần mà backlog mục 2.12 ghi là còn thiếu.
//
// # Hai bài trên dừng ở đâu, và vì sao chưa đủ
//
// `TestMuoiKhachTranhBaMonKhongAiMuaQua` chứng minh `StartCheckout` an
// toàn: đúng ba người GIỮ được hàng. Nhưng nó dừng ở đó — hàng mới chỉ
// chuyển Available → Reserved.
//
// Đường còn lại dài hơn hẳn, và mỗi bước là một chỗ số lượng có thể lệch:
//
//	CompleteCheckout → PlaceOrder → ghi outbox
//	                 → worker vét outbox
//	                 → inventory: Reserved → Committed
//
// PH-31 đã xảy ra đúng ở khúc này: nhả giữ hàng HAI LẦN "sinh ra hàng từ
// không khí". Một lần commit đôi ở đây cũng vậy, chỉ theo chiều ngược.
//
// # Bất biến được kiểm: BẢO TOÀN, không chỉ "không âm"
//
//	available + reserved + committed  ==  số hàng đã nhập
//
// Mạnh hơn hẳn `available >= 0`. Kho âm là lỗi tự lộ ra; hàng BỐC HƠI hay
// SINH THÊM mà ba con số vẫn dương thì không có gì báo, và nó chỉ hiện ra
// ở lần kiểm kê thật — nhiều tuần sau.
//
// # Kiểm chứng bằng cách phá, và một lần phá bị NUỐT
//
// Ba lần phá, kết quả không giống nhau và cả ba đều đáng ghi:
//
//	commit HAI LẦN mỗi reservation   → VẪN XANH
//	bỏ hẳn bước commit               → đỏ: "đã cam kết 0, cần 3"
//	commit mà KHÔNG trừ reserved     → đỏ: "= 6, nhưng chỉ nhập 3 — SINH RA
//	                                   TỪ KHÔNG KHÍ"
//
// Dòng đầu KHÔNG phải lỗ hổng của bài test: lần commit thứ hai trả
// `ErrReservationNotActive`, và handler coi đó là kết quả MONG MUỐN của
// một event được phát lại. Tức là chỗ đó đã idempotent SẴN, và phá nó
// không tạo ra hành vi sai để mà bắt.
//
// Ghi lại vì nó dễ dẫn tới kết luận sai theo cả hai chiều: thấy phá mà
// test vẫn xanh rồi tưởng bài test vô dụng, hoặc tưởng cứ phá gì cũng
// phải đỏ mới là test tốt.
func TestHoanTatDongThoiKhongSinhHangTuKhongKhi(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	shop := ids.MustNew(ids.PrefixSeller)

	const kho = 3
	const khach = 10

	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, kho)

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

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)

	var mu sync.Mutex
	var datDuoc int
	var loiLa []error

	for i := 0; i < khach; i++ {
		done.Add(1)
		go func(cartID ids.ID) {
			defer done.Done()
			start.Wait()

			// ĐI TRỌN đường, không dừng ở giữ hàng: mở phiên, điền địa
			// chỉ, chọn cách giao, rồi HOÀN TẤT.
			c, err := w.checkout.StartCheckout(ctx,
				checkoutapp.StartCheckoutInput{CartID: cartID})
			if err != nil {
				mu.Lock()
				defer mu.Unlock()
				if !errors.Is(err, checkoutapp.ErrOutOfStock) {
					loiLa = append(loiLa, err)
				}
				return
			}

			if _, err := w.checkout.SetShippingAddress(ctx, c.ID(), address()); err != nil {
				mu.Lock()
				loiLa = append(loiLa, err)
				mu.Unlock()
				return
			}
			if _, err := w.checkout.SetShippingMethod(ctx, c.ID(), "STANDARD"); err != nil {
				mu.Lock()
				loiLa = append(loiLa, err)
				mu.Unlock()
				return
			}

			_, err = w.checkout.CompleteCheckout(ctx, c.ID(),
				ids.MustNew(ids.PrefixRequest).String(), "COD")

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				datDuoc++
			case errors.Is(err, checkoutapp.ErrOutOfStock):
			default:
				loiLa = append(loiLa, err)
			}
		}(carts[i])
	}
	start.Done()
	done.Wait()

	for _, err := range loiLa {
		t.Errorf("lỗi ngoài dự kiến: %v", err)
	}

	if datDuoc != kho {
		t.Errorf("%d khách đặt được đơn, kho chỉ có %d", datDuoc, kho)
	}

	// Vét outbox: đây là chỗ Reserved → Committed thật sự xảy ra. Trước
	// dòng này hàng vẫn đang ở trạng thái GIỮ, chưa trừ.
	w.drain()

	avail, committed := w.stock(skuID, shop)
	giu := w.reserved(skuID, shop)

	if avail < 0 {
		t.Errorf("TỒN KHO ÂM: %d — đã bán quá hàng", avail)
	}

	// BẢO TOÀN. Đây là khẳng định chính của bài test.
	if tong := avail + giu + committed; tong != kho {
		t.Errorf("available(%d) + reserved(%d) + committed(%d) = %d, "+
			"nhưng chỉ nhập %d món — hàng %s",
			avail, giu, committed, tong, kho,
			map[bool]string{true: "SINH RA TỪ KHÔNG KHÍ", false: "BỐC HƠI"}[tong > kho])
	}

	// Và hàng phải đi HẾT sang đã-cam-kết: mỗi đơn đặt được là một món
	// rời kho. Còn sót ở `reserved` nghĩa là có event không tới đích, tức
	// hàng bị khóa cho một đơn đã xong — không ai đi tìm nó.
	if committed != kho {
		t.Errorf("đã cam kết %d, cần %d — event Reserved→Committed chưa "+
			"tới đích cho %d món", committed, kho, kho-committed)
	}
	if giu != 0 {
		t.Errorf("còn %d món ở trạng thái GIỮ sau khi mọi đơn đã đặt và "+
			"outbox đã vét — hàng bị khóa cho đơn đã xong", giu)
	}
}

// TestVetOutboxHaiLanKhongTruKhoHaiLan — cùng bất biến bảo toàn, nhưng
// dưới việc PHÁT LẠI event.
//
// Outbox là at-least-once: cùng một event SẼ được phát lại khi worker
// chết giữa chừng hoặc khi một lô bị thử lại. Nếu `Reserved → Committed`
// không idempotent thì lần phát thứ hai trừ kho lần nữa — và lần này
// không có ai mua.
//
// PH-31 là đúng lớp lỗi này, chỉ ngược chiều: nhả giữ hàng hai lần SINH
// RA hàng. Bài này khóa chiều còn lại.
func TestVetOutboxHaiLanKhongTruKhoHaiLan(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	shop := ids.MustNew(ids.PrefixSeller)

	const kho = 5
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, kho)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 2)},
	})

	c, err := w.checkout.StartCheckout(ctx,
		checkoutapp.StartCheckoutInput{CartID: cartID})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if _, err := w.checkout.SetShippingAddress(ctx, c.ID(), address()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := w.checkout.SetShippingMethod(ctx, c.ID(), "STANDARD"); err != nil {
		t.Fatalf("SetShippingMethod: %v", err)
	}
	if _, err := w.checkout.CompleteCheckout(ctx, c.ID(),
		ids.MustNew(ids.PrefixRequest).String(), "COD"); err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}

	w.drain()
	availSau1, committedSau1 := w.stock(skuID, shop)

	// Vét LẦN HAI. Event đã `published` nên vòng này không có gì để phát —
	// nhưng nếu có đường nào phát lại được, đây là chỗ nó lộ ra.
	w.drain()
	availSau2, committedSau2 := w.stock(skuID, shop)

	if availSau2 != availSau1 || committedSau2 != committedSau1 {
		t.Errorf("vét outbox lần hai làm ĐỔI tồn kho: "+
			"(%d khả dụng, %d cam kết) → (%d, %d) — Reserved→Committed "+
			"không idempotent", availSau1, committedSau1, availSau2, committedSau2)
	}

	giu := w.reserved(skuID, shop)
	if tong := availSau2 + giu + committedSau2; tong != kho {
		t.Errorf("available(%d) + reserved(%d) + committed(%d) = %d, "+
			"nhưng chỉ nhập %d món", availSau2, giu, committedSau2, tong, kho)
	}
	if committedSau2 != 2 {
		t.Errorf("đã cam kết %d, cần đúng 2 (số món khách mua)", committedSau2)
	}
}
