// Bất biến giữa THANH TOÁN và GIAO HÀNG — ADR-0018.
//
// # Vì sao chúng nằm ở đây, dưới dạng test
//
// Bốn phát biểu dưới đây là hợp đồng giữa ba module (order, payment,
// fulfillment) và không module nào một mình kiểm được: mỗi bên chỉ thấy
// phần của mình, và đúng chỗ lệch nhau là chỗ tiền đi mất. Viết thành
// tài liệu thì chúng đúng cho tới lần sửa đầu tiên; viết thành test thì
// lần sửa đó làm chúng đỏ.
//
// BB1  Hàng của đơn TRẢ TRƯỚC không rời kho khi chưa thu được tiền.
// BB2  Đơn COD KHÔNG bao giờ bị khóa — tiền COD về lúc giao.
// BB3  Đơn thực hiện đã mở khóa ⟹ đơn hàng đã PAID, hoặc là COD.
// BB4  Mở khóa là MỘT CHIỀU: đã thu tiền thì không quay lại "chưa thu".
//
// BB4 đáng nói riêng. Nó không phải quy tắc nghiệp vụ mà là hệ quả của sổ
// cái bất biến (ADR-0008): tiền đã về là sự thật đã xảy ra, và hoàn tiền
// là một bản ghi KHÁC chứ không phải một lần chuyển trạng thái ngược.
package e2e_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
)

// TestBB1_HangTraTruocKhongRoiKhoKhiChuaThuTien.
//
// Kiểm ĐỦ chuỗi bước của nhà bán, không chỉ bước đầu: một cửa chặn chỉ
// đóng ở `Confirm` vẫn để lọt đường đi thẳng tới `HandOver` nếu có ai đó
// thêm một lối tắt sau này.
func TestBB1_HangTraTruocKhongRoiKho(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop, fos := w.datDonVoiPhuongThuc(t, "CARD")
	foID := fos[0].ID
	seller := shop.String()

	for _, b := range []struct {
		ten string
		lam func() error
	}{
		{"xác nhận", func() error { return w.ful.ConfirmFulfillment(ctx, seller, foID) }},
		{"nhặt hàng", func() error { return w.ful.MarkPicking(ctx, seller, foID) }},
		{"đóng gói", func() error { return w.ful.MarkPacked(ctx, seller, foID) }},
		{"bàn giao", func() error {
			return w.ful.HandOverToCarrier(ctx, handOverReq(seller, foID))
		}},
	} {
		if err := b.lam(); err == nil {
			t.Errorf("BB1 vỡ: nhà bán %s được cho đơn TRẢ TRƯỚC chưa thu tiền", b.ten)
		}
	}
}

// TestBB2_DonCODKhongBaoGioBiKhoa.
//
// Nửa dễ làm hỏng nhất: một lần khóa quá tay làm đứng toàn bộ COD, và với
// COD thì chờ thanh toán trước khi giao là chặn chính đường thu tiền.
func TestBB2_DonCODKhongBaoGioBiKhoa(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop, fos := w.datDonVoiPhuongThuc(t, "COD")

	if err := w.ful.ConfirmFulfillment(ctx, shop.String(), fos[0].ID); err != nil {
		t.Fatalf("BB2 vỡ: đơn COD bị chặn — %v", err)
	}
}

// TestBB3_DaMoKhoaThiDonPhaiDaTraTien.
//
// Bất biến này nhìn từ phía DỮ LIỆU chứ không từ phía lời gọi: quét mọi
// đơn thực hiện đang mở khóa rồi đối chiếu ngược lên đơn hàng. Nó bắt được
// cả những đường mở khóa mà bài test không biết tới — ví dụ một câu UPDATE
// viết tay, hay một bên nhận mới đặt cờ sai.
func TestBB3_DaMoKhoaThiDonPhaiDaTraTien(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	// Dựng cả ba tình huống trong CÙNG một database rồi mới quét.
	w.datDonVoiPhuongThuc(t, "COD")
	w.datDonVoiPhuongThuc(t, "CARD")

	shop, fos := w.datDonVoiPhuongThuc(t, "BANK_TRANSFER")
	if err := w.ord.MarkOrderPaid(ctx, w.orderIDCuoi.String()); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	w.drain()
	_ = shop
	_ = fos

	rows, err := w.db.Pool().Query(ctx, `
		SELECT f.id, o.status, COALESCE(NULLIF(o.payment_method, ''), '(trống)')
		  FROM fulfillment_order f
		  JOIN "order" o ON o.id = f.order_id
		 WHERE f.cho_thanh_toan = FALSE`)
	if err != nil {
		t.Fatalf("quét đơn đã mở khóa: %v", err)
	}
	defer rows.Close()

	var soDong int
	for rows.Next() {
		var foID, trangThai, phuongThuc string
		if err := rows.Scan(&foID, &trangThai, &phuongThuc); err != nil {
			t.Fatalf("đọc dòng: %v", err)
		}
		soDong++

		// Mở khóa hợp lệ khi: đơn đã trả tiền, HOẶC là COD (tiền lúc giao),
		// HOẶC không có phương thức (đường placeOrder — xem ADR-0018).
		hopLe := trangThai == "PAID" ||
			phuongThuc == "COD" || phuongThuc == "(trống)"
		if !hopLe {
			t.Errorf("BB3 vỡ: đơn thực hiện %s đã mở khóa nhưng đơn hàng "+
				"ở trạng thái %q với phương thức %q", foID, trangThai, phuongThuc)
		}
	}
	if soDong == 0 {
		t.Fatal("không có đơn thực hiện nào đã mở khóa — bài test không kiểm được gì")
	}
}

// TestBB4_MoKhoaLaMOTCHIEU.
//
// Event `order.paid` được phát lại là chuyện bình thường của giao hàng
// ít-nhất-một-lần. Lần thứ hai KHÔNG được khóa lại, và cũng không được
// biến thành lỗi làm event kẹt trong hàng đợi.
func TestBB4_MoKhoaLaMotChieu(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop, fos := w.datDonVoiPhuongThuc(t, "CARD")
	foID := fos[0].ID

	if err := w.ord.MarkOrderPaid(ctx, w.orderIDCuoi.String()); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	w.drain()

	// Phát lại: gọi thẳng đường mở khóa lần nữa, đúng như một event trùng.
	if _, err := w.ful.MoKhoaTheoDon(ctx, w.orderIDCuoi.String()); err != nil {
		t.Fatalf("BB4 vỡ: mở khóa lần hai báo lỗi — %v", err)
	}

	if err := w.ful.ConfirmFulfillment(ctx, shop.String(), foID); err != nil {
		t.Errorf("BB4 vỡ: sau lần mở khóa thứ hai, đơn lại bị chặn — %v", err)
	}
}

func handOverReq(sellerID, foID string) fulfillment.HandOverRequest {
	return fulfillment.HandOverRequest{
		SellerID: sellerID, FulfillmentID: foID,
		Provider: "GHN", TrackingNumber: "GHN-BB-" + foID[len(foID)-6:],
	}
}

// TestBB5_TienMatChiTangKhiTienTHATSUVe — ADR-0018 phần B1.
//
// # Bất biến
//
// BB5  `PLATFORM_CASH` chỉ ghi NỢ khi tiền đã về. Trước lúc đó, số tiền
//
//	của đơn nằm ở `ACCOUNTS_RECEIVABLE` — khách NỢ.
//
// Đo được lúc phát hiện: 1.132.273.000 đ ghi NỢ PLATFORM_CASH cho 3110
// đơn còn PENDING_PAYMENT. Sổ cái khẳng định nền tảng cầm 1,13 tỷ đồng
// chưa hề tới, và con số đó đi thẳng vào mọi báo cáo tài sản.
func TestBB5_TienMatChiTangKhiTienThatSuVe(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	w.datDonVoiPhuongThuc(t, "CARD")

	tien, phaiThu := w.soDu(t)
	if tien != 0 {
		t.Errorf("BB5 vỡ: tiền mặt = %d ngay sau khi đặt đơn, mong 0 — "+
			"chưa đồng nào về", tien)
	}
	if phaiThu <= 0 {
		t.Fatalf("khoản phải thu = %d, mong dương — tiền của đơn phải nằm "+
			"ở đâu đó", phaiThu)
	}

	// Tiền về.
	if err := w.ord.MarkOrderPaid(ctx, w.orderIDCuoi.String()); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	w.drain()

	tien2, phaiThu2 := w.soDu(t)
	if tien2 != phaiThu {
		t.Errorf("tiền mặt sau khi thu = %d, mong %d (đúng khoản phải thu cũ)",
			tien2, phaiThu)
	}
	if phaiThu2 != 0 {
		t.Errorf("khoản phải thu còn %d sau khi thu tiền, mong 0", phaiThu2)
	}
}

// TestBB6_ThuTienHaiLanCHIGhiMotButToan.
//
// Event `order.paid` phát lại là chuyện bình thường của giao hàng
// ít-nhất-một-lần. Ghi hai bút toán thu tiền cho một lần thu nghĩa là sổ
// cái nói nền tảng nhận gấp đôi — và sổ cái bất biến thì sửa bằng bút
// toán đảo, không xóa được.
func TestBB6_ThuTienHaiLanChiGhiMotButToan(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	w.datDonVoiPhuongThuc(t, "CARD")
	if err := w.ord.MarkOrderPaid(ctx, w.orderIDCuoi.String()); err != nil {
		t.Fatalf("MarkOrderPaid: %v", err)
	}
	w.drain()

	tien, _ := w.soDu(t)

	// Phát lại đúng event đó lần nữa.
	w.phatLaiOrderPaid(t)
	w.drain()

	tien2, _ := w.soDu(t)
	if tien2 != tien {
		t.Errorf("BB6 vỡ: tiền mặt %d → %d sau khi phát lại order.paid — "+
			"ghi nhận tiền về hai lần", tien, tien2)
	}
}

// soDu trả số dư PLATFORM_CASH và ACCOUNTS_RECEIVABLE, tính từ sổ cái.
func (w *world) soDu(t *testing.T) (tienMat, phaiThu int64) {
	t.Helper()
	rows, err := w.db.Pool().Query(context.Background(), `
		SELECT account_type,
		       COALESCE(SUM(CASE WHEN direction = 'DEBIT'
		                         THEN amount ELSE -amount END), 0)
		  FROM ledger_line
		 WHERE account_type IN ('PLATFORM_CASH', 'ACCOUNTS_RECEIVABLE')
		 GROUP BY account_type`)
	if err != nil {
		t.Fatalf("đọc số dư: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var acc string
		var v int64
		if err := rows.Scan(&acc, &v); err != nil {
			t.Fatalf("đọc số dư: %v", err)
		}
		if acc == "PLATFORM_CASH" {
			tienMat = v
		} else {
			phaiThu = v
		}
	}
	return tienMat, phaiThu
}

// phatLaiOrderPaid đưa event `order.paid` trở lại hàng đợi chưa xử lý.
//
// Mô phỏng đúng thứ xảy ra thật: nhà cung cấp gửi trùng, hoặc worker chết
// giữa lúc đánh dấu đã xử lý rồi khởi động lại.
func (w *world) phatLaiOrderPaid(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := w.db.Pool().Exec(ctx, `
		UPDATE event_outbox SET published_at = NULL, attempts = 0
		 WHERE event_type = 'order.paid'`); err != nil {
		t.Fatalf("đưa event trở lại hàng đợi: %v", err)
	}
	// CHỈ xóa dấu của event order.paid.
	//
	// Xóa sạch bảng sẽ phát lại cả `checkout.completed`, và bút toán doanh
	// thu lần hai đụng khóa idempotency rồi làm hỏng cả giao dịch — bài
	// test đỏ vì một lý do chẳng liên quan gì tới thứ nó muốn kiểm.
	if _, err := w.db.Pool().Exec(ctx, `
		DELETE FROM event_processed
		 WHERE event_id IN (
		       SELECT event_id FROM event_outbox
		        WHERE event_type = 'order.paid')`); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
}
