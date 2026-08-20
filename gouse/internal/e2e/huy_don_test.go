package e2e_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
)

// datVaGiao dựng một đơn đã đặt xong, trả về mã đơn thực hiện của seller.
func datVaGiao(
	t *testing.T, w *world, shop, skuID ids.ID, khoBanDau, muaBaoNhieu int,
) string {
	t.Helper()
	ctx := context.Background()

	// Nhập kho dưới CHỦ SỞ HỮU đã suy ra, không phải dưới mã nhà bán:
	// own brand là seller nội bộ nhưng hàng của nó thuộc nền tảng
	// (ADR-0012). Nhập nhầm chủ thì offer không tìm thấy hàng.
	w.stockFor(skuID, w.ownerOf(shop), khoBanDau)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items: []checkoutapp.CartItemSnapshot{
			line(shop, skuID, 300_000, muaBaoNhieu),
		},
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
	res, err := w.checkout.CompleteCheckout(
		ctx, c.ID(), ids.MustNew(ids.PrefixRequest).String())
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	w.drain()

	fos, err := w.ful.GetOrderFulfillments(ctx, res.OrderID.String())
	if err != nil {
		t.Fatalf("GetOrderFulfillments: %v", err)
	}
	if len(fos) != 1 {
		t.Fatalf("số đơn thực hiện = %d, cần 1", len(fos))
	}
	return fos[0].ID
}

// TestHuyDonTraHangVeKho — PH-28.
//
// # Lỗi mà nó chặn
//
// Trước bản sửa: hủy đơn KHÔNG trả hàng về kho. Đường vào có (Reserved →
// Committed khi đặt hàng), đường ra không. Mỗi đơn bị hủy ăn mất một phần
// kho VĨNH VIỄN — hàng có thật trên kệ nhưng hệ thống mãi coi là đã cam
// kết cho một đơn không còn tồn tại.
//
// Không lỗi, không log, không cảnh báo. Chỉ phát hiện được khi kiểm kê tay
// thấy số thực nhiều hơn số hệ thống — và lúc đó không còn lần ra được
// những đơn nào đã gây ra nó.
func TestHuyDonTraHangVeKho(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	foID := datVaGiao(t, w, shop, skuID, 20, 5)

	if avail, commit := w.stock(skuID, shop); avail != 15 || commit != 5 {
		t.Fatalf("sau khi đặt: %d/%d, cần 15/5", avail, commit)
	}

	if err := w.ful.CancelFulfillment(
		ctx, shop.String(), foID, "khách đổi ý"); err != nil {
		t.Fatalf("hủy đơn thực hiện: %v", err)
	}
	w.drain()

	avail, commit := w.stock(skuID, shop)
	if avail != 20 {
		t.Errorf("còn %d khả dụng, cần 20 — hàng KHÔNG quay về kho", avail)
	}
	if commit != 0 {
		t.Errorf("còn %d đang cam kết cho một đơn đã hủy, cần 0", commit)
	}
}

// TestHuyDonCuaOwnBrandTraVeKhoNenTang: chủ sở hữu tồn kho suy ra từ nhà
// bán (ADR-0012), nên đường trả hàng cũng phải suy đúng.
//
// Nếu handler dùng thẳng `seller_id` làm chủ sở hữu, nó sẽ đi tìm một bản
// ghi tồn kho không tồn tại, bỏ qua trong im lặng, và hàng của nền tảng
// vẫn kẹt — đúng lỗi P3-18 lặp lại ở một đường khác.
func TestHuyDonCuaOwnBrandTraVeKhoNenTang(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	ownBrand := ids.MustNew(ids.PrefixSeller)
	w.internal[ownBrand] = true

	skuID := ids.MustNew(ids.PrefixSKU)
	foID := datVaGiao(t, w, ownBrand, skuID, 30, 4)

	// Hàng nằm dưới own_platform, KHÔNG phải dưới mã seller nội bộ.
	owner := w.ownerOf(ownBrand)
	if avail, commit := w.stock(skuID, owner); avail != 26 || commit != 4 {
		t.Fatalf("sau khi đặt: %d/%d, cần 26/4", avail, commit)
	}

	if err := w.ful.CancelFulfillment(
		ctx, ownBrand.String(), foID, "hết hàng thật"); err != nil {
		t.Fatalf("hủy đơn thực hiện: %v", err)
	}
	w.drain()

	avail, commit := w.stock(skuID, owner)
	if avail != 30 || commit != 0 {
		t.Errorf("hàng nền tảng %d khả dụng / %d cam kết, cần 30/0", avail, commit)
	}
}

// TestHuyLaiKhongTraHangHaiLan: event có thể được phát lại.
//
// Dispatcher đảm bảo mỗi bên nhận xử lý một event đúng một lần, nhưng bài
// này khẳng định điều đó ở mức TOÀN CHUỖI — vì hậu quả của việc trả hai
// lần là hàng không có thật xuất hiện trong kho, và nó sẽ được bán.
func TestHuyLaiKhongTraHangHaiLan(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	foID := datVaGiao(t, w, shop, skuID, 20, 5)

	if err := w.ful.CancelFulfillment(
		ctx, shop.String(), foID, "khách đổi ý"); err != nil {
		t.Fatalf("hủy đơn thực hiện: %v", err)
	}
	w.drain()
	// Phát lại: dispatcher chạy thêm vòng nữa trên cùng outbox.
	w.drain()
	w.drain()

	avail, commit := w.stock(skuID, shop)
	if avail != 20 {
		t.Errorf("còn %d khả dụng, cần 20 — trả hàng nhiều lần", avail)
	}
	if commit != 0 {
		t.Errorf("còn %d cam kết, cần 0", commit)
	}
}

// TestHangDangTrenDuongVeThiKHONGTraVeKho — mặt kia của PH-28.
//
// Domain cho phép DELIVERY_FAILED → CANCELLED: giao thất bại, hàng được
// chuyển trả về người gửi. Nhưng lúc đó hàng ĐÃ RỜI KHO và đang trên
// đường — chưa ai cầm trong tay.
//
// Trả về khả dụng ngay là bán một món chưa về tới nơi. Nếu nó hỏng, thất
// lạc, hoặc không bao giờ tới, lỗi hiện ra ở KHÁCH THỨ HAI chứ không phải
// khách đầu — và lúc đó không còn lần ra được nguyên nhân.
//
// Hàng trả về nhập lại bằng quy trình riêng có bước kiểm tra chất lượng.
func TestHangDangTrenDuongVeThiKHONGTraVeKho(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	foID := datVaGiao(t, w, shop, skuID, 20, 5)
	seller := shop.String()

	// Đi hết đường tới lúc giao thất bại.
	for _, buoc := range []struct {
		ten string
		lam func() error
	}{
		{"xác nhận", func() error { return w.ful.ConfirmFulfillment(ctx, seller, foID) }},
		{"nhặt hàng", func() error { return w.ful.MarkPicking(ctx, seller, foID) }},
		{"đóng gói", func() error { return w.ful.MarkPacked(ctx, seller, foID) }},
		{"bàn giao", func() error {
			return w.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: seller, FulfillmentID: foID,
				Provider: "GHN", TrackingNumber: "GHN123456",
			})
		}},
		{"đang giao", func() error { return w.ful.MarkInTransit(ctx, seller, foID) }},
		{"giao thất bại", func() error {
			return w.ful.MarkDeliveryFailed(ctx, seller, foID, "khách không nhận máy")
		}},
	} {
		if err := buoc.lam(); err != nil {
			t.Fatalf("%s: %v", buoc.ten, err)
		}
	}
	w.drain()

	// Hàng đã rời kho: Ship() đã trừ khỏi tồn kho vật lý, hoặc vẫn ở
	// Committed tùy luồng. Ghi lại con số TRƯỚC khi hủy để so.
	truocAvail, truocCommit := w.stock(skuID, shop)

	if err := w.ful.CancelFulfillment(
		ctx, seller, foID, "trả về người gửi"); err != nil {
		t.Fatalf("hủy sau khi giao thất bại: %v", err)
	}
	w.drain()

	sauAvail, sauCommit := w.stock(skuID, shop)
	if sauAvail != truocAvail || sauCommit != truocCommit {
		t.Errorf(
			"tồn kho ĐỔI khi hủy lúc hàng đang trên đường về: %d/%d → %d/%d",
			truocAvail, truocCommit, sauAvail, sauCommit)
	}
}
