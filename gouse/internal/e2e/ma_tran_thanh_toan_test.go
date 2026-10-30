// Ma trận E2E của luồng thanh toán — ADR-0018.
//
// Sáu tình huống, và mỗi cái hỏi một câu khác nhau:
//
//	trả trước THÀNH CÔNG   tiền về → hàng đi được, sổ ghi tiền mặt
//	trả trước THẤT BẠI     tiền KHÔNG về → hàng vẫn nằm im
//	COD                    không có tiền để chờ → hàng đi ngay
//	thu tiền ĐỒNG THỜI     hai webhook cùng lúc → một lần ghi nhận
//	phát lại webhook       nhà cung cấp gửi trùng → không đổi gì
//	phát lại order.paid    outbox thử lại → không mở khóa/ghi sổ hai lần
//
// Bốn cái cuối là nơi hệ thống phân phối thường hỏng, và chúng hỏng theo
// kiểu KHÔNG ai thấy: không có lỗi nào được báo, chỉ có một con số sai.
package e2e_test

import (
	"context"
	"sync"
	"testing"
)

// TestMT_TraTruocTHATBAI_HangVanNamIm.
//
// Nửa đối xứng của "thu tiền xong thì mở khóa". Không có bài này thì một
// lỗi làm mở khóa vô điều kiện vẫn xanh ở mọi bài còn lại — và hàng đi ra
// cho đơn mà nhà cung cấp đã báo THẤT BẠI.
func TestMT_AFailedPrepaymentLeavesGoodsUntouched(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop, fos := w.placeOrderWith(t, "CARD")

	// Không có `order.paid` nào được phát: thanh toán thất bại nghĩa là
	// đơn KHÔNG bao giờ tới trạng thái PAID.
	w.drain()

	if err := w.ful.ConfirmFulfillment(ctx, shop.String(), fos[0].ID); err == nil {
		t.Error("thanh toán thất bại mà nhà bán vẫn xử lý được đơn")
	}

	amount, receivable := w.balance(t)
	if amount != 0 {
		t.Errorf("tiền mặt = %d sau khi thanh toán thất bại, mong 0", amount)
	}
	if receivable <= 0 {
		t.Errorf("khoản phải thu = %d, mong dương — đơn vẫn là một khoản "+
			"khách nợ cho tới khi bị hủy", receivable)
	}
}

// TestMT_ThuTienDONGTHOI_ChiGhiMotLan.
//
// Hai tiến trình cùng xử lý một đơn: hai bản sao worker, hoặc nhà cung cấp
// gửi hai webhook trong cùng một giây. Nếu cả hai cùng ghi bút toán thu
// tiền thì sổ cái nói nền tảng nhận gấp đôi.
//
// Chạy THẬT song song bằng goroutine, không mô phỏng bằng cách gọi tuần tự
// hai lần: gọi tuần tự chỉ kiểm idempotency, không kiểm tranh chấp.
func TestMT_ConcurrentCapturePostsOnce(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	w.placeOrderWith(t, "CARD")
	orderID := w.lastOrderID.String()

	const parallel = 4
	var wg sync.WaitGroup
	failure := make([]error, parallel)
	wg.Add(parallel)
	for i := 0; i < parallel; i++ {
		go func(i int) {
			defer wg.Done()
			failure[i] = w.ord.MarkOrderPaid(ctx, orderID)
		}(i)
	}
	wg.Wait()

	// ĐÚNG MỘT lần được phép thành công: máy trạng thái của đơn chỉ cho
	// PENDING_PAYMENT → PAID một lần. Các lần còn lại phải bị TỪ CHỐI,
	// không phải âm thầm bỏ qua.
	var succeeded int
	for _, e := range failure {
		if e == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Errorf("%d/%d lời gọi MarkOrderPaid thành công, mong đúng 1",
			succeeded, parallel)
	}

	w.drain()

	amount, receivable := w.balance(t)
	if receivable != 0 {
		t.Errorf("khoản phải thu = %d sau khi thu tiền, mong 0", receivable)
	}
	if amount <= 0 {
		t.Fatalf("tiền mặt = %d, mong dương", amount)
	}

	// Số bút toán thu tiền phải đúng MỘT.
	var n int
	if err := w.db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM ledger_entry WHERE entry_type = 'PAYMENT_RECEIVED'`,
	).Scan(&n); err != nil {
		t.Fatalf("đếm bút toán thu tiền: %v", err)
	}
	if n != 1 {
		t.Errorf("có %d bút toán PAYMENT_RECEIVED, mong 1 — sổ cái ghi "+
			"nhận tiền về nhiều lần cho một lần thu", n)
	}
}

// TestMT_PhatLaiOrderPaid_KhongMoKhoaVaGhiSoHaiLan.
//
// Outbox giao ÍT NHẤT MỘT LẦN, nên event lặp là thiết kế chứ không phải
// sự cố. Bài này chạy cả hai bên nhận của `order.paid` cùng lúc — mở khóa
// và ghi sổ — vì chúng phải cùng chịu được việc lặp.
func TestMT_RepublishingOrderPaidDoesNotActTwice(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop, fos := w.placeOrderWith(t, "BANK_TRANSFER")
	if err := w.ord.MarkOrderPaid(ctx, w.lastOrderID.String()); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	w.drain()

	amount, _ := w.balance(t)

	// Phát lại ba lần liên tiếp.
	for i := 0; i < 3; i++ {
		w.republishOrderPaid(t)
		w.drain()
	}

	amount2, _ := w.balance(t)
	if amount2 != amount {
		t.Errorf("tiền mặt %d → %d sau ba lần phát lại", amount, amount2)
	}

	// Và đơn vẫn giao được: phát lại KHÔNG được khóa ngược.
	if err := w.ful.ConfirmFulfillment(ctx, shop.String(), fos[0].ID); err != nil {
		t.Errorf("sau khi phát lại, đơn lại bị chặn: %v", err)
	}
}
