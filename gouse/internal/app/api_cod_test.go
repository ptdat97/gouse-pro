package app

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
)

// TestCODIsCapturedOnDelivery.
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
func TestCODIsCapturedOnDelivery(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	orderID := a.placeCODOrder(t, "codthutien")
	a.emitEvent(t)

	foID, sellerID := a.fulfillmentOfOrder(t, orderID)
	if foID == "" {
		t.Skip("không tạo được đơn thực hiện")
	}

	// TRƯỚC khi giao: khoản phải thu còn nguyên, chưa có tiền mặt.
	cash, receivable := a.ledgerBalance(t, orderID)
	if receivable <= 0 {
		t.Fatalf("đơn chưa giao mà khoản phải thu = %d — bài kiểm không có "+
			"gì để đo", receivable)
	}
	if cash != 0 {
		t.Fatalf("đơn COD chưa giao mà đã ghi %d đ tiền mặt", cash)
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
	a.emitEvent(t)

	// Mốc thu tiền phải được ghi, và trạng thái KHÔNG được lùi về PAID.
	var paidAt *string
	var status string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT paid_at::text, status FROM "order" WHERE id = $1`, orderID).
		Scan(&paidAt, &status); err != nil {
		t.Fatalf("đọc đơn: %v", err)
	}
	if paidAt == nil {
		t.Fatal("đơn COD đã giao xong mà KHÔNG có mốc thu tiền — khoản " +
			"phải thu sẽ tồn vĩnh viễn và nhà bán không được quyết toán")
	}
	if status == "PAID" {
		t.Error("trạng thái lùi về PAID sau khi đã giao — ghi nhận thu " +
			"tiền không được xóa sự thật 'đã giao xong'")
	}

	// Và sổ cái phải chuyển phải thu thành tiền mặt.
	cash2, receivable2 := a.ledgerBalance(t, orderID)
	if cash2 != receivable {
		t.Errorf("tiền mặt ghi nhận = %d, cần %d (bằng khoản phải thu) — "+
			"bút toán THU TIỀN không được ghi", cash2, receivable)
	}
	if receivable2 != 0 {
		t.Errorf("khoản phải thu còn %d sau khi thu tiền, cần 0", receivable2)
	}
}

// TestCODIsNotCapturedBeforeDelivery.
//
// Đơn tách cho hai nhà bán đi thành hai kiện, và khách trả tiền cho từng
// người giao. Ghi nhận ở kiện ĐẦU TIÊN là khẳng định đã thu đủ tiền cả đơn
// trong khi kiện thứ hai còn trên đường — sổ cái sẽ ghi tiền mặt chưa cầm,
// đúng thứ ADR-0018 vừa dọn 1,13 tỷ đồng.
func TestCODIsNotCapturedBeforeDelivery(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	orderID := a.placeCODOrder(t, "codmotphan")
	a.emitEvent(t)

	foID, sellerID := a.fulfillmentOfOrder(t, orderID)
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
	a.emitEvent(t)

	var paidAt *string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT paid_at::text FROM "order" WHERE id = $1`, orderID).
		Scan(&paidAt); err != nil {
		t.Fatalf("đọc đơn: %v", err)
	}
	if paidAt != nil {
		t.Error("hàng mới rời kho mà đã ghi nhận thu tiền — sổ cái đang " +
			"khẳng định cầm một khoản tiền còn nằm trên đường")
	}

	if cash, _ := a.ledgerBalance(t, orderID); cash != 0 {
		t.Errorf("đã ghi %d đ tiền mặt cho đơn chưa giao", cash)
	}
}
