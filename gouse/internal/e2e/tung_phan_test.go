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
		ctx, c.ID(), ids.MustNew(ids.PrefixRequest).String(), "COD")
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

// TestHuyMotPhanTraHangDUNGCHUSOHUU là nửa còn thiếu của PH-2 — bất biến
// ownership dưới HỦY TỪNG PHẦN.
//
// # Vì sao bài `TestHuyMotPhanDonVanConHieuLuc` chưa đủ
//
// Nó kiểm TRẠNG THÁI ĐƠN sau khi hủy một phần (`PARTIALLY_CANCELLED`, và
// phần của B vẫn đi tiếp). Đúng và cần — nhưng nó không nhìn tới tồn kho
// một lần nào, nên một cài đặt trả hàng về SAI CHỦ vẫn xanh.
//
// # Vì sao CÙNG MỘT SKU và CÙNG MỘT KHO
//
// Đó là ca duy nhất mà việc định tuyến theo chủ sở hữu thật sự bị thử.
// Truy vấn tìm dòng tồn kho theo `(sku_id, stock_location_id,
// inventory_owner_id)`; chỉ cần hai nhà bán ở hai kho khác nhau là hai
// cột đầu đã đủ, và cột thứ ba không bao giờ phải làm việc.
//
// Bản đầu của bài này dùng `stockFor`, thứ cấp cho mỗi chủ một kho RIÊNG.
// Bỏ hẳn `inventory_owner_id` khỏi mệnh đề WHERE thì nó VẪN XANH — bài
// test xanh vì dữ liệu dễ, không phải vì code đúng.
//
// Với kho DÙNG CHUNG thì cùng phép phá đó đỏ ngay, và đỏ SỚM hơn chỗ ta
// nhắm: `StartCheckout` báo "không đủ hàng" vì phần giữ của nhà bán thứ
// hai rơi vào dòng của nhà bán thứ nhất. Nó không chạy tới bước hủy — vẫn
// là bắt được, chỉ là bắt ở mắt xích trước.
//
// Và một kho hai chủ không phải ca dựng ra cho vui: hàng nhà bán gửi ở
// kho nền tảng vẫn thuộc nhà bán, nên cùng SKU cùng kho có bản ghi riêng
// cho từng chủ. Đó chính là ADR-0012.
//
// # Hỏng thì hỏng thế nào
//
// Hủy phần của A mà hàng chạy về kho của B: A vĩnh viễn thiếu 2 món, B tự
// nhiên thừa 2 món. Không lỗi nào báo, không con số nào âm, và cả hai bên
// chỉ phát hiện ở lần kiểm kê thật.
func TestHuyMotPhanTraHangDUNGCHUSOHUU(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shopA := ids.MustNew(ids.PrefixSeller)
	shopB := ids.MustNew(ids.PrefixSeller)

	// MỘT SKU, MỘT KHO, HAI chủ sở hữu.
	//
	// Cả ba điều kiện đều cần thiết. Kho riêng cho mỗi bên thì
	// `stock_location_id` một mình đã đủ tìm đúng dòng, và cột chủ sở hữu
	// không bao giờ phải làm việc — kiểm chứng: bỏ `inventory_owner_id`
	// khỏi mệnh đề WHERE với dữ liệu kho-riêng thì bài test VẪN XANH.
	skuID := ids.MustNew(ids.PrefixSKU)
	kho := w.khoChung()
	w.stockTaiKho(skuID, w.ownerOf(shopA), kho, 20)
	w.stockTaiKho(skuID, w.ownerOf(shopB), kho, 20)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items: []checkoutapp.CartItemSnapshot{
			line(shopA, skuID, 300_000, 2),
			line(shopB, skuID, 450_000, 1),
		},
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
	res, err := w.checkout.CompleteCheckout(ctx, c.ID(),
		ids.MustNew(ids.PrefixRequest).String(), "COD")
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	w.drain()

	// Sau khi đặt: mỗi bên bị trừ ĐÚNG phần của mình.
	kiemKho := func(t *testing.T, moc string, muonA, camA, muonB, camB int) {
		t.Helper()
		availA, commitA := w.stock(skuID, w.ownerOf(shopA))
		availB, commitB := w.stock(skuID, w.ownerOf(shopB))
		if availA != muonA || commitA != camA {
			t.Errorf("%s — nhà bán A: %d khả dụng / %d cam kết, cần %d/%d",
				moc, availA, commitA, muonA, camA)
		}
		if availB != muonB || commitB != camB {
			t.Errorf("%s — nhà bán B: %d khả dụng / %d cam kết, cần %d/%d",
				moc, availB, commitB, muonB, camB)
		}
	}
	kiemKho(t, "sau khi đặt đơn", 18, 2, 19, 1)

	fos, err := w.ful.GetOrderFulfillments(ctx, res.OrderID.String())
	if err != nil {
		t.Fatalf("GetOrderFulfillments: %v", err)
	}
	var foA string
	for _, fo := range fos {
		if fo.SellerID == shopA.String() {
			foA = fo.ID
		}
	}
	if foA == "" {
		t.Fatalf("không tìm thấy đơn thực hiện của nhà bán A trong %d đơn", len(fos))
	}

	// HỦY phần của A. Phần của B không được động tới.
	if err := w.ful.CancelFulfillment(ctx, shopA.String(), foA, "hết hàng thật"); err != nil {
		t.Fatalf("hủy đơn thực hiện của A: %v", err)
	}
	w.drain()

	// A nhận lại đủ 2 món; B GIỮ NGUYÊN.
	//
	// Vế thứ hai mới là vế khó: nó là thứ đỏ lên khi hàng trả về chạy
	// nhầm kho.
	kiemKho(t, "sau khi hủy phần của A", 20, 0, 19, 1)

	// Và tổng của TỪNG chủ phải bảo toàn — 20 món mỗi bên như lúc nhập.
	for _, tt := range []struct {
		ten   string
		owner ids.ID
	}{{"A", w.ownerOf(shopA)}, {"B", w.ownerOf(shopB)}} {
		avail, commit := w.stock(skuID, tt.owner)
		giu := w.reserved(skuID, tt.owner)
		if tong := avail + giu + commit; tong != 20 {
			t.Errorf("nhà bán %s: available(%d) + reserved(%d) + committed(%d) "+
				"= %d, nhưng chỉ nhập 20 — hàng đã đi lạc giữa hai chủ",
				tt.ten, avail, giu, commit, tong)
		}
	}
}

// TestPhiVanChuyenTinhTHEO NGUON qua cả chuỗi thật (P3-8).
//
// # Vì sao cần bài này khi domain đã có test
//
// Test domain chứng minh `UocTinhPhiGiao` cộng đúng. Bài này chứng minh
// con số đó THẬT SỰ đi vào tổng đơn — qua `SetShippingMethod`, qua
// `CompleteCheckout`, vào `Order.ShippingFee` đã đóng băng.
//
// Đó là hai việc khác nhau: biểu phí đúng mà tầng trên vẫn dùng bảng cũ
// thì mọi test domain vẫn xanh, và khách vẫn bị thu sai.
func TestPhiVanChuyenTinhTheoNguonQuaCaChuoi(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shopA := ids.MustNew(ids.PrefixSeller)
	shopB := ids.MustNew(ids.PrefixSeller)

	skuA := ids.MustNew(ids.PrefixSKU)
	skuB := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuA, w.ownerOf(shopA), 10)
	w.stockFor(skuB, w.ownerOf(shopB), 10)

	// phiCuaDon dựng một đơn với danh sách món cho trước và trả phí ship
	// ĐÃ ĐÓNG BĂNG trên đơn.
	phiCuaDon := func(t *testing.T, items []checkoutapp.CartItemSnapshot) int64 {
		t.Helper()

		cartID := ids.MustNew(ids.PrefixCart)
		w.cart.put(checkoutapp.CartSnapshot{
			CartID:     cartID,
			CustomerID: ids.MustNew(ids.PrefixCustomer),
			GuestEmail: "khach@example.com",
			Currency:   money.VND,
			Items:      items,
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
		res, err := w.checkout.CompleteCheckout(ctx, c.ID(),
			ids.MustNew(ids.PrefixRequest).String(), "COD")
		if err != nil {
			t.Fatalf("CompleteCheckout: %v", err)
		}

		don, err := w.ord.GetOrder(ctx, res.OrderID.String())
		if err != nil {
			t.Fatalf("GetOrder: %v", err)
		}
		w.drain()
		return don.ShippingFee.Value
	}

	// Mọi đơn ở đây phải nằm DƯỚI ngưỡng miễn phí vận chuyển (mặc định
	// 499.000đ), nếu không phí về 0 và bài test không đo được gì.
	//
	// Bản đầu dùng 300.000đ + 450.000đ = 750.000đ và đỏ ngay khi ngưỡng
	// được cài — một cách hay để bài test tự nhắc rằng nó phụ thuộc vào
	// một con số chính sách. Ngưỡng có bài riêng:
	// `TestMienPhiVanChuyenKhiDatNguong`.
	motNhaBan := phiCuaDon(t, []checkoutapp.CartItemSnapshot{
		line(shopA, skuA, 100_000, 1),
	})
	haiNhaBan := phiCuaDon(t, []checkoutapp.CartItemSnapshot{
		line(shopA, skuA, 100_000, 1),
		line(shopB, skuB, 120_000, 1),
	})

	if motNhaBan <= 0 {
		t.Fatalf("phí đơn một nhà bán = %d, phải > 0", motNhaBan)
	}

	// HAI nguồn = HAI kiện = HAI lần phí.
	if haiNhaBan != motNhaBan*2 {
		t.Errorf("đơn hai nhà bán thu %d, đơn một nhà bán thu %d — "+
			"phí phải NHÂN ĐÔI vì hàng đi thành hai kiện. Thu một lần "+
			"nghĩa là nền tảng bù phần chênh trên mọi đơn nhiều nhà bán",
			haiNhaBan, motNhaBan)
	}

	// Nhiều MÓN của CÙNG một nhà bán vẫn là MỘT kiện — không nhân lên.
	haiMonMotNhaBan := phiCuaDon(t, []checkoutapp.CartItemSnapshot{
		line(shopA, skuA, 100_000, 2),
	})
	if haiMonMotNhaBan != motNhaBan {
		t.Errorf("hai món cùng một nhà bán thu %d, một món thu %d — "+
			"cùng nhà bán thì cùng một kiện, phí không được nhân theo SỐ MÓN",
			haiMonMotNhaBan, motNhaBan)
	}
}

// TestThueVaMienPhiShipDiVaoDON — hai chính sách của chủ dự án, kiểm ở
// mức số tiền ĐÃ ĐÓNG BĂNG trên đơn (PH-40 + P3-8).
//
// # Vì sao cần bài này khi domain đã có test
//
// Test domain chứng minh phép tính đúng. Bài này chứng minh con số đó đi
// hết chuỗi — qua `SetShippingMethod`, `CompleteCheckout`, vào
// `Order.TaxAmount` và `Order.ShippingFee`.
//
// Đó là hai việc khác nhau, và khoảng cách giữa chúng chính là chỗ PH-40
// đã nằm im: `SetTax` tồn tại đủ ba tầng và KHÔNG AI GỌI, nên thuế luôn
// bằng 0 trong khi domain hoàn toàn có khả năng tính đúng.
func TestThueVaMienPhiShipDiVaoDon(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, w.ownerOf(shop), 50)

	datDon := func(t *testing.T, gia int64, sl int) (phi, thue, tong int64) {
		t.Helper()
		cartID := ids.MustNew(ids.PrefixCart)
		w.cart.put(checkoutapp.CartSnapshot{
			CartID:     cartID,
			CustomerID: ids.MustNew(ids.PrefixCustomer),
			GuestEmail: "khach@example.com",
			Currency:   money.VND,
			Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, gia, sl)},
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
		res, err := w.checkout.CompleteCheckout(ctx, c.ID(),
			ids.MustNew(ids.PrefixRequest).String(), "COD")
		if err != nil {
			t.Fatalf("CompleteCheckout: %v", err)
		}
		don, err := w.ord.GetOrder(ctx, res.OrderID.String())
		if err != nil {
			t.Fatalf("GetOrder: %v", err)
		}
		w.drain()
		return don.ShippingFee.Value, don.TaxAmount.Value, don.Total.Value
	}

	t.Run("dưới ngưỡng: có phí ship, thuế trên cả phí", func(t *testing.T) {
		// tiền hàng 100.000 · phí 30.000 · thuế 8% × 130.000 = 10.400
		phi, thue, tong := datDon(t, 100_000, 1)

		if phi != 30_000 {
			t.Errorf("phí ship = %d, mong 30000 (dưới ngưỡng 499.000)", phi)
		}
		if thue != 10_400 {
			t.Errorf("thuế trên ĐƠN = %d, mong 10400 — thuế bằng 0 ở đây "+
				"đúng là PH-40: domain tính được mà không ai gọi", thue)
		}
		if tong != 140_400 {
			t.Errorf("tổng = %d, mong 140400", tong)
		}
	})

	t.Run("đạt ngưỡng: miễn phí ship, vẫn có thuế", func(t *testing.T) {
		// tiền hàng 500.000 ≥ 499.000 → phí 0 · thuế 8% × 500.000 = 40.000
		phi, thue, tong := datDon(t, 500_000, 1)

		if phi != 0 {
			t.Errorf("phí ship = %d, mong 0 — đơn 500.000đ đạt ngưỡng "+
				"499.000đ", phi)
		}
		if thue != 40_000 {
			t.Errorf("thuế = %d, mong 40000 = 8%% × 500.000 (phí ship đã "+
				"miễn nên không cộng vào)", thue)
		}
		if tong != 540_000 {
			t.Errorf("tổng = %d, mong 540000", tong)
		}
	})
}
