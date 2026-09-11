package app

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
)

// TestCODThuTienKhiGiaoXong.
//
// # Vì sao bài này tồn tại
//
// ADR-0018 khẳng định "với COD thì tiền về lúc giao" — nhưng không chỉ
// định AI ghi nhận, và không đường nào ở production làm việc đó:
// `MarkOrderPaid` chỉ được gọi từ webhook cổng thanh toán, thứ COD không
// có.
//
// Đo trên sổ cái thật lúc phát hiện: **0 bút toán THU TIỀN trong toàn bộ
// hệ thống**. Nghĩa là khoản phải thu của MỌI đơn COD tồn vĩnh viễn, và
// nhà bán không bao giờ được quyết toán.
//
// Bài này đi trọn đường: đặt đơn COD → giao xong → kiểm sổ cái.
func TestCODThuTienKhiGiaoXong(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maDon := a.datDonCOD(t, "codthutien")
	a.phatEvent(t)

	foID, sellerID := a.donThucHienCuaDon(t, maDon)
	if foID == "" {
		t.Skip("không tạo được đơn thực hiện")
	}

	// TRƯỚC khi giao: khoản phải thu còn nguyên, chưa có tiền mặt.
	tienMat, phaiThu := a.soDuSoCai(t, maDon)
	if phaiThu <= 0 {
		t.Fatalf("đơn chưa giao mà khoản phải thu = %d — bài kiểm không có "+
			"gì để đo", phaiThu)
	}
	if tienMat != 0 {
		t.Fatalf("đơn COD chưa giao mà đã ghi %d đ tiền mặt", tienMat)
	}

	for _, b := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, sellerID, foID) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, sellerID, foID) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, sellerID, foID) },
		func() error {
			return a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: sellerID, FulfillmentID: foID,
				Provider: "GHN", TrackingNumber: "GHN-COD-" + foID[4:14],
			})
		},
		func() error { return a.mods.fulfillment.MarkDelivered(ctx, sellerID, foID) },
	} {
		if err := b(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}
	a.phatEvent(t)

	// Mốc thu tiền phải được ghi, và trạng thái KHÔNG được lùi về PAID.
	var paidAt *string
	var trangThai string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT paid_at::text, status FROM "order" WHERE id = $1`, maDon).
		Scan(&paidAt, &trangThai); err != nil {
		t.Fatalf("đọc đơn: %v", err)
	}
	if paidAt == nil {
		t.Fatal("đơn COD đã giao xong mà KHÔNG có mốc thu tiền — khoản " +
			"phải thu sẽ tồn vĩnh viễn và nhà bán không được quyết toán")
	}
	if trangThai == "PAID" {
		t.Error("trạng thái lùi về PAID sau khi đã giao — ghi nhận thu " +
			"tiền không được xóa sự thật 'đã giao xong'")
	}

	// Và sổ cái phải chuyển phải thu thành tiền mặt.
	tienMat2, phaiThu2 := a.soDuSoCai(t, maDon)
	if tienMat2 != phaiThu {
		t.Errorf("tiền mặt ghi nhận = %d, cần %d (bằng khoản phải thu) — "+
			"bút toán THU TIỀN không được ghi", tienMat2, phaiThu)
	}
	if phaiThu2 != 0 {
		t.Errorf("khoản phải thu còn %d sau khi thu tiền, cần 0", phaiThu2)
	}
}

// TestCODChuaGiaoXongThiChuaThuTien.
//
// Đơn tách cho hai nhà bán đi thành hai kiện, và khách trả tiền cho từng
// người giao. Ghi nhận ở kiện ĐẦU TIÊN là khẳng định đã thu đủ tiền cả đơn
// trong khi kiện thứ hai còn trên đường — sổ cái sẽ ghi tiền mặt chưa cầm,
// đúng thứ ADR-0018 vừa dọn 1,13 tỷ đồng.
func TestCODChuaGiaoXongThiChuaThuTien(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maDon := a.datDonCOD(t, "codmotphan")
	a.phatEvent(t)

	foID, sellerID := a.donThucHienCuaDon(t, maDon)
	if foID == "" {
		t.Skip("không tạo được đơn thực hiện")
	}

	// Mới bàn giao cho hãng, CHƯA giao tới khách.
	for _, b := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, sellerID, foID) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, sellerID, foID) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, sellerID, foID) },
		func() error {
			return a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: sellerID, FulfillmentID: foID,
				Provider: "GHN", TrackingNumber: "GHN-CHUA-" + foID[4:14],
			})
		},
	} {
		if err := b(); err != nil {
			t.Fatalf("chuyển trạng thái: %v", err)
		}
	}
	a.phatEvent(t)

	var paidAt *string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT paid_at::text FROM "order" WHERE id = $1`, maDon).
		Scan(&paidAt); err != nil {
		t.Fatalf("đọc đơn: %v", err)
	}
	if paidAt != nil {
		t.Error("hàng mới rời kho mà đã ghi nhận thu tiền — sổ cái đang " +
			"khẳng định cầm một khoản tiền còn nằm trên đường")
	}

	if tienMat, _ := a.soDuSoCai(t, maDon); tienMat != 0 {
		t.Errorf("đã ghi %d đ tiền mặt cho đơn chưa giao", tienMat)
	}
}
