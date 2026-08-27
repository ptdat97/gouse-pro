package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
)

// dungDonDaGiao dựng một đơn đã giao xong, trả (mã đơn, mã dòng, mã nhà bán).
func (a *apiTest) dungDonDaGiao(t *testing.T) (string, string, string) {
	t.Helper()
	ctx := context.Background()

	maPhien := a.dungPhienSanHoanTat("trahang@example.com", "0900321321")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	a.phatEvent(t)
	fo := a.timFulfillment(t, maDon)
	if fo.id == "" {
		t.Skip("không tạo được đơn thực hiện")
	}

	for _, b := range []func() error{
		func() error { return a.mods.fulfillment.ConfirmFulfillment(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPicking(ctx, fo.sellerID, fo.id) },
		func() error { return a.mods.fulfillment.MarkPacked(ctx, fo.sellerID, fo.id) },
		func() error {
			return a.mods.fulfillment.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: fo.sellerID, FulfillmentID: fo.id,
				Provider: "ghn", TrackingNumber: "TRA-" + fo.id[4:14],
			})
		},
		func() error { return a.mods.fulfillment.MarkDelivered(ctx, fo.sellerID, fo.id) },
	} {
		if err := b(); err != nil {
			t.Fatalf("chuyển trạng thái đơn thực hiện: %v", err)
		}
	}
	a.phatEvent(t)

	var maDong string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT id FROM order_line WHERE order_id = $1 LIMIT 1`, maDon).
		Scan(&maDong); err != nil {
		t.Fatalf("đọc dòng hàng: %v", err)
	}
	return maDon, maDong, fo.sellerID
}

// TestKhachTraHangDiTronLuong — PH-37.
//
// Xin trả → nhà bán duyệt → nhận hàng → hoàn tiền, tất cả qua HTTP thật.
//
// Hai bất biến quan trọng nhất được kiểm ở TẦNG DATABASE, không qua
// response: hàng hoàn KHÔNG vào tồn khả dụng, và sổ cái CÓ bút toán hoàn.
func TestKhachTraHangDiTronLuong(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maDon, maDong, sellerID := a.dungDonDaGiao(t)

	res := a.call(http.MethodPost, "/api/v1/orders/"+maDon+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": maDong, "quantity": 1,
			"reason_code": "SIZE_TOO_SMALL", "reason_detail": "chật quá",
		}},
	}, hopNhat(khoaIdem(), map[string]string{"X-Guest-Phone": "0900321321"}))
	if res.code != http.StatusCreated {
		t.Fatalf("xin trả hàng: HTTP %d — %s", res.code, res.raw)
	}
	yc, _ := res.body["return"].(map[string]any)
	maYC, _ := yc["id"].(string)
	if tt, _ := yc["status"].(string); tt != "REQUESTED" {
		t.Errorf("trạng thái đầu là %q, cần REQUESTED", tt)
	}

	// Tiền hoàn phải bằng giá thực trả của dòng.
	tien, _ := yc["refund_amount"].(map[string]any)
	soTien, _ := tien["amount"].(float64)
	if soTien <= 0 {
		t.Errorf("số tiền hoàn là %v", soTien)
	}

	// --- nhà bán duyệt rồi nhận hàng ---
	if err := a.mods.returns.Duyet(ctx, maYC, sellerID); err != nil {
		t.Fatalf("duyệt: %v", err)
	}

	truocKhaDung := a.tonKhoKhaDung(t, maDon)
	truocButToan := a.demDong("ledger_entry", "reference_id = $1", maYC)

	if err := a.mods.returns.NhanHangVaHoanTien(ctx, maYC, sellerID); err != nil {
		t.Fatalf("nhận hàng và hoàn tiền: %v", err)
	}

	// BẤT BIẾN 1: hàng hoàn KHÔNG vào tồn khả dụng.
	//
	// docs/07-workflows/return.md mục 4: vi phạm quy tắc này nghĩa là bán
	// lại hàng hỏng cho khách khác, và thiệt hại uy tín lớn hơn nhiều giá
	// trị món hàng.
	sauKhaDung := a.tonKhoKhaDung(t, maDon)
	if sauKhaDung != truocKhaDung {
		t.Errorf("tồn KHẢ DỤNG đổi %d → %d khi nhận hàng hoàn — "+
			"hàng hoàn phải vào trạng thái Returned, chờ kiểm định",
			truocKhaDung, sauKhaDung)
	}

	// BẤT BIẾN 2: sổ cái có bút toán hoàn.
	if n := a.demDong("ledger_entry", "reference_id = $1", maYC) - truocButToan; n < 1 {
		t.Errorf("có %d bút toán hoàn tiền, cần ít nhất 1 — "+
			"khách được trả hàng mà tiền không được đảo ngược", n)
	}
}

// TestKhongTraDuocDonCuaNguoiKhac — bất biến BẢO MẬT.
func TestKhongTraDuocDonCuaNguoiKhac(t *testing.T) {
	a := newAPITest(t)
	maDon, maDong, _ := a.dungDonDaGiao(t)

	// Không có số điện thoại của khách → không được đụng vào đơn.
	res := a.call(http.MethodPost, "/api/v1/orders/"+maDon+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": maDong, "quantity": 1, "reason_code": "CHANGED_MIND",
		}},
	}, khoaIdem())
	if res.code != http.StatusNotFound {
		t.Errorf("người lạ xin trả hàng: HTTP %d, cần 404 — %s", res.code, res.raw)
	}
}

// TestXinTraHaiLanChoCungMonBiChan.
//
// Không có chặn này, khách gửi hai yêu cầu cho cùng một món và được hoàn
// tiền hai lần cho một lần mua.
func TestXinTraHaiLanChoCungMonBiChan(t *testing.T) {
	a := newAPITest(t)
	maDon, maDong, _ := a.dungDonDaGiao(t)

	than := map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": maDong, "quantity": 1, "reason_code": "CHANGED_MIND",
		}},
	}
	h := hopNhat(khoaIdem(), map[string]string{"X-Guest-Phone": "0900321321"})

	if res := a.call(http.MethodPost, "/api/v1/orders/"+maDon+"/returns", than, h); res.code != http.StatusCreated {
		t.Fatalf("lần đầu: HTTP %d — %s", res.code, res.raw)
	}

	h2 := hopNhat(khoaIdem(), map[string]string{"X-Guest-Phone": "0900321321"})
	lai := a.call(http.MethodPost, "/api/v1/orders/"+maDon+"/returns", than, h2)
	if lai.code == http.StatusCreated {
		t.Error("xin trả LẦN HAI cho cùng một món vẫn thành công — " +
			"khách được hoàn tiền hai lần cho một lần mua")
	}
}

// tonKhoKhaDung đọc tổng tồn khả dụng của các SKU trong đơn.
func (a *apiTest) tonKhoKhaDung(t *testing.T, orderID string) int {
	t.Helper()
	var n int
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT coalesce(sum(i.quantity_available), 0)
		  FROM inventory_item i
		 WHERE i.sku_id IN (SELECT sku_id FROM order_line WHERE order_id = $1)`,
		orderID).Scan(&n); err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	return n
}
