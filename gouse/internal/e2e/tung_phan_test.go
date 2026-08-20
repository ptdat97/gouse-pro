package e2e_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
)

// datHaiNhaBan dựng một đơn có hàng của HAI nhà bán, trả về hai mã đơn
// thực hiện theo đúng thứ tự (shopA, shopB).
func datHaiNhaBan(
	t *testing.T, w *world, shopA, shopB ids.ID,
) (string, string, string) {
	t.Helper()
	ctx := context.Background()

	skuA := ids.MustNew(ids.PrefixSKU)
	skuB := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuA, w.ownerOf(shopA), 20)
	w.stockFor(skuB, w.ownerOf(shopB), 20)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items: []checkoutapp.CartItemSnapshot{
			line(shopA, skuA, 300_000, 2),
			line(shopB, skuB, 450_000, 1),
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
	if len(fos) != 2 {
		t.Fatalf("số đơn thực hiện = %d, cần 2", len(fos))
	}

	var foA, foB string
	for _, fo := range fos {
		switch fo.SellerID {
		case shopA.String():
			foA = fo.ID
		case shopB.String():
			foB = fo.ID
		}
	}
	if foA == "" || foB == "" {
		t.Fatalf("không tìm được đơn thực hiện của cả hai nhà bán")
	}
	return res.OrderID.String(), foA, foB
}

// giaoToiBuoc đẩy một đơn thực hiện tới trạng thái mong muốn.
func giaoToiBuoc(t *testing.T, w *world, seller, foID string, denBuoc string) {
	t.Helper()
	ctx := context.Background()

	buoc := []struct {
		ten string
		lam func() error
	}{
		{"CONFIRMED", func() error { return w.ful.ConfirmFulfillment(ctx, seller, foID) }},
		{"PICKING", func() error { return w.ful.MarkPicking(ctx, seller, foID) }},
		{"PACKED", func() error { return w.ful.MarkPacked(ctx, seller, foID) }},
		{"HANDED_OVER", func() error {
			return w.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: seller, FulfillmentID: foID,
				Provider: "GHN", TrackingNumber: "GHN" + foID[len(foID)-6:],
			})
		}},
		{"IN_TRANSIT", func() error { return w.ful.MarkInTransit(ctx, seller, foID) }},
		{"DELIVERED", func() error { return w.ful.MarkDelivered(ctx, seller, foID) }},
	}

	for _, b := range buoc {
		if err := b.lam(); err != nil {
			t.Fatalf("%s: %v", b.ten, err)
		}
		w.drain()
		if b.ten == denBuoc {
			return
		}
	}
}

// trangThaiDon đọc trạng thái tổng hợp của đơn hàng.
func trangThaiDon(t *testing.T, w *world, orderID string) string {
	t.Helper()
	v, err := w.ord.GetOrder(context.Background(), orderID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	return v.Status
}

// TestGiaoTungPhan — một nhà bán đã xuất hàng, nhà bán kia chưa.
//
// # Vì sao trạng thái tổng hợp phải phân biệt được
//
// Khách mua một đơn nhưng hàng đến từ nhiều nguồn, đi bằng nhiều gói, tới
// vào nhiều ngày khác nhau. Gộp thành một trạng thái duy nhất thì hoặc là
// nói dối theo hướng lạc quan ("đã gửi" khi mới gửi một nửa), hoặc theo
// hướng bi quan ("đang xử lý" khi một nửa đã tới nơi).
//
// Cả hai đều dẫn tới cùng một hệ quả: khách gọi lên hỏi hàng của tôi đâu.
func TestGiaoTungPhan(t *testing.T) {
	w := newWorld(t)
	shopA := ids.MustNew(ids.PrefixSeller)
	shopB := ids.MustNew(ids.PrefixSeller)

	orderID, foA, _ := datHaiNhaBan(t, w, shopA, shopB)

	// Chỉ nhà bán A bàn giao vận chuyển.
	giaoToiBuoc(t, w, shopA.String(), foA, "HANDED_OVER")

	if got := trangThaiDon(t, w, orderID); got != "PARTIALLY_SHIPPED" {
		t.Errorf("trạng thái đơn = %s, cần PARTIALLY_SHIPPED", got)
	}
}

// TestGiaoDuTungPhanRoiDuHet — từ giao một phần tới giao đủ.
//
// Bài này khóa cả HAI phía của quy tắc: một nửa thì phải là "một phần",
// và đủ cả thì phải chuyển sang trạng thái cuối. Chỉ kiểm một phía thì
// một cài đặt luôn trả "PARTIALLY_DELIVERED" vẫn xanh.
func TestGiaoDuTungPhanRoiDuHet(t *testing.T) {
	w := newWorld(t)
	shopA := ids.MustNew(ids.PrefixSeller)
	shopB := ids.MustNew(ids.PrefixSeller)

	orderID, foA, foB := datHaiNhaBan(t, w, shopA, shopB)

	giaoToiBuoc(t, w, shopA.String(), foA, "DELIVERED")
	if got := trangThaiDon(t, w, orderID); got != "PARTIALLY_DELIVERED" {
		t.Fatalf("sau khi A giao xong: %s, cần PARTIALLY_DELIVERED", got)
	}

	giaoToiBuoc(t, w, shopB.String(), foB, "DELIVERED")
	if got := trangThaiDon(t, w, orderID); got != "DELIVERED" {
		t.Errorf("sau khi cả hai giao xong: %s, cần DELIVERED", got)
	}
}

// TestHuyMotPhanDonVanConHieuLuc — một nhà bán hủy, đơn KHÔNG hủy theo.
//
// Đây là chỗ dễ sai theo hướng tai hại: coi đơn là đã hủy khi mới một
// nguồn hàng hủy nghĩa là khách mất phần hàng còn lại mà không ai báo.
func TestHuyMotPhanDonVanConHieuLuc(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	shopA := ids.MustNew(ids.PrefixSeller)
	shopB := ids.MustNew(ids.PrefixSeller)

	orderID, foA, foB := datHaiNhaBan(t, w, shopA, shopB)

	if err := w.ful.CancelFulfillment(
		ctx, shopA.String(), foA, "hết hàng thật"); err != nil {
		t.Fatalf("hủy đơn thực hiện của A: %v", err)
	}
	w.drain()

	if got := trangThaiDon(t, w, orderID); got != "PARTIALLY_CANCELLED" {
		t.Errorf("trạng thái đơn = %s, cần PARTIALLY_CANCELLED", got)
	}

	// Phần của B vẫn đi tiếp bình thường.
	giaoToiBuoc(t, w, shopB.String(), foB, "DELIVERED")
	if got := trangThaiDon(t, w, orderID); got == "CANCELLED" {
		t.Error("đơn bị coi là đã hủy dù nhà bán B đã giao xong")
	}
}
