package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
)

// TestKhachTheoDoiLoHangThayNgayGiaoDuKien.
//
// # Vì sao bài này tồn tại
//
// Phép quét "tuyến nào không bài test nào gọi tới" ra đúng hai tuyến trên
// tổng 86; đây là tuyến thứ hai.
//
// Nó phục vụ `estimated_delivery_date` — trường mà đến 11/09 vẫn RỖNG trên
// mọi đơn thực hiện vì câu `UPDATE` không ghi cột đó, và không đường nào
// gán nó. Đã sửa ở P3-36, nhưng chưa bài nào đi qua tuyến mà KHÁCH thật sự
// gọi để xem nó có tới nơi không.
//
// Một trường được tính đúng ở domain mà không tới được response thì với
// khách nó vẫn không tồn tại.
func TestKhachTheoDoiLoHangThayNgayGiaoDuKien(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maDon := a.datDonCOD(t, "theodoi")
	a.phatEvent(t)

	foID, sellerID := a.donThucHienCuaDon(t, maDon)
	if foID == "" {
		t.Skip("không tạo được đơn thực hiện")
	}

	xem := func() map[string]any {
		t.Helper()
		res := a.call(http.MethodGet, "/api/v1/orders/"+maDon+"/shipments", nil,
			map[string]string{"X-Guest-Phone": "0900333222"})
		if res.code != http.StatusOK {
			t.Fatalf("theo dõi lô hàng: HTTP %d — %s", res.code, res.raw)
		}
		ds, _ := res.body["data"].([]any)
		if len(ds) == 0 {
			t.Fatal("đơn có đơn thực hiện mà danh sách lô hàng RỖNG")
		}
		m, _ := ds[0].(map[string]any)
		return m
	}

	// TRƯỚC khi bàn giao: chưa có ngày dự kiến, và đó là đúng — đồng hồ
	// của hãng vận chuyển chưa bắt đầu chạy.
	if v, có := xem()["estimated_delivery_date"]; có && v != "" {
		t.Errorf("chưa bàn giao mà đã hứa ngày giao %v — thời gian nhà bán "+
			"chuẩn bị hàng là phần của người khác", v)
	}

	for _, b := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, sellerID, foID) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, sellerID, foID) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, sellerID, foID) },
		func() error {
			return a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: sellerID, FulfillmentID: foID,
				Provider: "GHN", TrackingNumber: "GHN-TD-" + foID[4:14],
			})
		},
	} {
		if err := b(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}

	lo := xem()

	ngay, _ := lo["estimated_delivery_date"].(string)
	if ngay == "" {
		t.Error("đã bàn giao mà khách KHÔNG thấy ngày giao dự kiến — " +
			"trường tính đúng ở domain nhưng không tới được response thì " +
			"với khách nó vẫn không tồn tại")
	}

	// Mã vận đơn cũng phải tới nơi: không có nó thì khách không tra được
	// trên trang của hãng vận chuyển.
	if mv, _ := lo["tracking_number"].(string); mv == "" {
		t.Error("đã bàn giao mà khách không thấy mã vận đơn")
	}
	if tt, _ := lo["status"].(string); tt != "HANDED_OVER" {
		t.Errorf("trạng thái lô hàng = %q, cần HANDED_OVER", tt)
	}
}

// TestKhachKhongXemDuocLoHangCuaDonNguoiKhac.
//
// Tuyến này trả mã vận đơn và trạng thái giao hàng — đủ để lần ra một đơn
// không phải của mình. Chưa bài nào từng kiểm hàng rào đó.
func TestKhachKhongXemDuocLoHangCuaDonNguoiKhac(t *testing.T) {
	a := newAPITest(t)

	maDon := a.datDonCOD(t, "lohangnguoikhac")
	a.phatEvent(t)

	res := a.call(http.MethodGet, "/api/v1/orders/"+maDon+"/shipments", nil,
		map[string]string{"X-Guest-Phone": "0900999888"})
	if res.code == http.StatusOK {
		t.Errorf("số điện thoại KHÁC vẫn xem được lô hàng của đơn này — "+
			"mã vận đơn và trạng thái giao đủ để lần ra đơn người khác "+
			"(HTTP %d)", res.code)
	}
}
