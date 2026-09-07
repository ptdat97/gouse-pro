package app

import (
	"context"
	"net/http"
	"testing"
)

// TestChuoiDayDuTraTruoc là P3-3: Product → Offer → Cart → Checkout →
// Order → Payment → Fulfillment, đi HẾT trong một bài, qua HTTP thật.
//
// # Vì sao bài này trước nay KHÔNG viết được
//
// Mắt xích PAYMENT không tồn tại: `payment_intent` chỉ có từ PH-36
// (06/09), và `order.paid` — thứ nối tiền về với việc giao hàng — chỉ có
// từ ADR-0018 (07/09). Trước đó chuỗi đứt ở giữa, và mỗi nửa được kiểm
// riêng bằng bản giả của nửa kia.
//
// # Vì sao nó bắt được thứ các bài lẻ không bắt
//
// Mỗi bước dưới đây do một module khác nhau sở hữu, và chỗ hỏng nằm ở
// KHOẢNG GIỮA — đúng loại lỗi mà `internal/e2e/doc.go` mô tả. Ba lần đã
// xảy ra: `payment_method` bị vứt ở tầng HTTP (P3-9), `size_chart` khai mà
// không ai điền (P3-22), và bút toán ghi tiền mặt cho đơn chưa thu tiền
// (ADR-0018). Không bài lẻ nào thấy được chúng.
//
// Bài này khẳng định TRẠNG THÁI TIỀN ở từng chặng, không chỉ mã HTTP —
// một chuỗi trả 200 ở mọi bước vẫn có thể ghi sai sổ ở bước thứ tư.
func TestChuoiDayDuTraTruoc(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	// ---- Product → Offer → Cart → Checkout → Order ----
	maDon, tong := a.donTraTruoc(t, emailMoi("chuoi"), "0900777111")

	if tt := a.trangThaiDon(t, maDon); tt != "PENDING_PAYMENT" {
		t.Fatalf("sau khi đặt: trạng thái = %q, cần PENDING_PAYMENT", tt)
	}
	a.phatEvent(t)

	// Đơn thực hiện đã tạo, nhưng KHÓA — tiền chưa về (ADR-0018 A2).
	foID, sellerID := a.donThucHienCuaDon(t, maDon)
	if !a.dangKhoaChoThanhToan(t, foID) {
		t.Fatal("đơn thực hiện của đơn TRẢ TRƯỚC phải bị khóa chờ thanh toán")
	}

	// Sổ cái: KHÁCH NỢ, chưa có đồng tiền mặt nào (ADR-0018 B1).
	if tien, phaiThu := a.soDuSoCai(t, maDon); tien != 0 || phaiThu <= 0 {
		t.Errorf("sau khi đặt: tiền mặt = %d (cần 0), phải thu = %d (cần > 0)",
			tien, phaiThu)
	}

	// Nhà bán CHƯA được giao hàng.
	tokNB := a.taoTokenNhaBan(t, sellerID)
	res := a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/ship",
		map[string]any{"shipping_provider": "GHN", "tracking_number": "GHN-CHUOI-1"},
		hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code < 400 {
		t.Fatalf("nhà bán GIAO ĐƯỢC hàng khi tiền chưa về: HTTP %d — %s",
			res.code, res.raw)
	}

	// ---- Payment: webhook của cổng thanh toán ----
	whRes := a.goiWebhookThanhToan(t, "cong-tt", map[string]any{
		"event_id":   "evt_chuoi_day_du_1",
		"event_type": "payment.succeeded",
		"data": map[string]any{
			"payment_intent_id": "pi_chuoi_1",
			"amount":            tong,
			"currency":          "VND",
			"metadata":          map[string]any{"order_id": maDon},
		},
	}, biMatCongTT)
	if whRes.code != http.StatusOK {
		t.Fatalf("webhook thanh toán: HTTP %d — %s", whRes.code, whRes.raw)
	}
	if tt := a.trangThaiDon(t, maDon); tt != "PAID" {
		t.Fatalf("sau webhook: trạng thái = %q, cần PAID", tt)
	}

	// `order.paid` đi qua outbox tới hai bên nhận.
	a.phatEvent(t)

	// ---- Fulfillment: nay mới được giao ----
	if a.dangKhoaChoThanhToan(t, foID) {
		t.Fatal("thu tiền xong mà đơn thực hiện vẫn bị khóa")
	}

	// Sổ cái: khoản phải thu đã thành TIỀN MẶT.
	tien, phaiThu := a.soDuSoCai(t, maDon)
	if phaiThu != 0 {
		t.Errorf("sau khi thu tiền: phải thu = %d, cần 0", phaiThu)
	}
	if tien != int64(tong) {
		t.Errorf("tiền mặt = %d, cần %d — đúng số tiền đã thu", tien, tong)
	}

	res = a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/ship",
		map[string]any{"shipping_provider": "GHN", "tracking_number": "GHN-CHUOI-1"},
		hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code != http.StatusOK {
		t.Fatalf("bàn giao vận chuyển: HTTP %d — %s", res.code, res.raw)
	}

	res = a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/deliver", nil,
		hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code != http.StatusOK {
		t.Fatalf("giao hàng: HTTP %d — %s", res.code, res.raw)
	}
	a.phatEvent(t)

	// ---- Kết: đơn đã giao, tiền đã thu, sổ cân ----
	var trangThaiFO string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT status FROM fulfillment_order WHERE id = $1`, foID,
	).Scan(&trangThaiFO); err != nil {
		t.Fatalf("đọc trạng thái đơn thực hiện: %v", err)
	}
	if trangThaiFO != "DELIVERED" {
		t.Errorf("đơn thực hiện = %q, cần DELIVERED", trangThaiFO)
	}

	// Bất biến cuối cùng của mọi sổ kép: Σ NỢ = Σ CÓ.
	var no, co int64
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT COALESCE(SUM(l.amount) FILTER (WHERE l.direction = 'DEBIT'), 0),
		       COALESCE(SUM(l.amount) FILTER (WHERE l.direction = 'CREDIT'), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1`, maDon).Scan(&no, &co); err != nil {
		t.Fatalf("đọc sổ cái: %v", err)
	}
	if no != co {
		t.Errorf("sổ cái LỆCH: Σ nợ = %d, Σ có = %d", no, co)
	}
}

// donThucHienCuaDon trả đơn thực hiện đầu tiên của một đơn hàng.
func (a *apiTest) donThucHienCuaDon(t *testing.T, orderID string) (foID, sellerID string) {
	t.Helper()
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT id, seller_id FROM fulfillment_order WHERE order_id = $1
		  ORDER BY fo_number LIMIT 1`, orderID).Scan(&foID, &sellerID); err != nil {
		t.Fatalf("đọc đơn thực hiện của đơn %s: %v", orderID, err)
	}
	return foID, sellerID
}

func (a *apiTest) dangKhoaChoThanhToan(t *testing.T, foID string) bool {
	t.Helper()
	var khoa bool
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT cho_thanh_toan FROM fulfillment_order WHERE id = $1`,
		foID).Scan(&khoa); err != nil {
		t.Fatalf("đọc cờ chờ thanh toán: %v", err)
	}
	return khoa
}

// soDuSoCai trả số dư PLATFORM_CASH và ACCOUNTS_RECEIVABLE CỦA MỘT ĐƠN.
//
// Lọc theo `reference_id`, không cộng cả bảng: các bài khác trong gói dùng
// chung database, nên một phép cộng toàn cục xanh khi chạy riêng và đỏ khi
// chạy cả gói — đúng kiểu test chập chờn mà P3-2 đã dọn một lần.
func (a *apiTest) soDuSoCai(t *testing.T, orderID string) (tienMat, phaiThu int64) {
	t.Helper()
	rows, err := a.db.Pool().Query(context.Background(), `
		SELECT l.account_type,
		       COALESCE(SUM(CASE WHEN l.direction = 'DEBIT'
		                         THEN l.amount ELSE -l.amount END), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1
		   AND l.account_type IN ('PLATFORM_CASH', 'ACCOUNTS_RECEIVABLE')
		 GROUP BY l.account_type`, orderID)
	if err != nil {
		t.Fatalf("đọc số dư sổ cái: %v", err)
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
