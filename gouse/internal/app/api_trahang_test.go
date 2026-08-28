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

// TestKiemDinhDuaHangHoanTroLaiBanDuoc.
//
// # Vì sao bước này quyết định
//
// Hàng hoàn về kho nằm ở trạng thái Returned và KHÔNG BAO GIỜ tự động vào
// Available — quy tắc bắt buộc, vì bán lại hàng hỏng gây thiệt hại uy tín
// lớn hơn nhiều giá trị món hàng.
//
// Nhưng không có bước ghi kết quả kiểm định thì hàng nằm CHẾT vĩnh viễn:
// nhà bán mất cả hàng lẫn tiền, và con số tồn kho ngày càng xa thực tế.
//
// Bài này đo cả hai chiều: hàng ĐẠT quay lại bán được, hàng LOẠI thì không.
//
// # Chống kiểm hai lần có HAI lớp, bài này không phân biệt được
//
//	domain GhiKetQuaKiemDinh  → từ chối khi dòng đã kiểm
//	bất biến của tồn kho      → không còn hàng ở Returned để chuyển
//
// Đã kiểm bằng cách phá: bỏ lớp domain thì bài này VẪN XANH, vì tồn kho
// chặn. Cả hai đều đúng và đều cần — nhưng lớp domain là thứ trả về lỗi
// đọc được cho nhà bán, còn tồn kho chỉ trả 500.
func TestKiemDinhDuaHangHoanTroLaiBanDuoc(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maDon, maDong, sellerID := a.dungDonDaGiao(t)
	tokBan := a.dangNhapNhaBan(t, sellerID)

	res := a.call(http.MethodPost, "/api/v1/orders/"+maDon+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": maDong, "quantity": 1, "reason_code": "SIZE_TOO_SMALL",
		}},
	}, hopNhat(khoaIdem(), map[string]string{"X-Guest-Phone": "0900321321"}))
	if res.code != http.StatusCreated {
		t.Fatalf("xin trả hàng: HTTP %d — %s", res.code, res.raw)
	}
	yc, _ := res.body["return"].(map[string]any)
	maYC, _ := yc["id"].(string)

	if err := a.mods.returns.Duyet(ctx, maYC, sellerID); err != nil {
		t.Fatalf("duyệt: %v", err)
	}
	if err := a.mods.returns.NhanHangVaHoanTien(ctx, maYC, sellerID); err != nil {
		t.Fatalf("nhận hàng: %v", err)
	}

	truoc := a.tonKhoTheoTrangThai(t, maDon)
	if truoc.hoan == 0 {
		t.Fatalf("sau khi nhận hàng, tồn Returned là 0 — hàng chưa vào kho")
	}

	// KIỂM ĐỊNH ĐẠT — qua HTTP, đường trước đây không tồn tại.
	got := a.call(http.MethodPost, "/api/v1/seller/returns/"+maYC+"/inspect",
		map[string]any{
			"lines": []any{map[string]any{"order_line_id": maDong, "passed": true}},
		}, hopNhat(bearer(tokBan), khoaIdem()))
	if got.code != http.StatusOK {
		t.Fatalf("kiểm định: HTTP %d — %s", got.code, got.raw)
	}

	// Đo MỨC THAY ĐỔI, không phải con số tuyệt đối: SKU dùng chung với
	// các bài khác trong cùng lượt chạy, và tổng tuyệt đối là đo cả dữ
	// liệu của người khác.
	sau := a.tonKhoTheoTrangThai(t, maDon)
	if sau.khaDung-truoc.khaDung != 1 {
		t.Errorf("hàng ĐẠT kiểm định: khả dụng đổi %d → %d, cần +1 — "+
			"hàng hoàn không quay lại bán được và nằm chết trong kho",
			truoc.khaDung, sau.khaDung)
	}
	if truoc.hoan-sau.hoan != 1 {
		t.Errorf("tồn Returned đổi %d → %d, cần −1 — hàng không rời trạng "+
			"thái chờ kiểm định", truoc.hoan, sau.hoan)
	}

	// KIỂM HAI LẦN phải bị chặn: hàng vào Available gấp đôi là tồn kho
	// tăng thêm số hàng không có thật.
	lai := a.call(http.MethodPost, "/api/v1/seller/returns/"+maYC+"/inspect",
		map[string]any{
			"lines": []any{map[string]any{"order_line_id": maDong, "passed": true}},
		}, hopNhat(bearer(tokBan), khoaIdem()))
	if lai.code == http.StatusOK {
		t.Error("kiểm định LẦN HAI vẫn thành công — tồn kho tăng thêm hàng không có thật")
	}
	if cuoi := a.tonKhoTheoTrangThai(t, maDon); cuoi.khaDung != sau.khaDung {
		t.Errorf("kiểm lần hai làm khả dụng đổi %d → %d", sau.khaDung, cuoi.khaDung)
	}
}

// TestLoaiHangPhaiNeuLyDo.
//
// Lý do loại là đầu vào cho việc làm việc với nhà cung cấp và quyết ai
// chịu chi phí. "Hỏng" không có mô tả thì không làm được gì với nó.
//
// Cũng có HAI lớp: kiểm ở domain và CHECK `return_line_fail_needs_note` ở
// database. Bỏ lớp domain thì bài vẫn xanh vì database chặn — nhưng khi
// đó nhà bán nhận 500 thay vì một thông điệp nói rõ thiếu gì.
func TestLoaiHangPhaiNeuLyDo(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maDon, maDong, sellerID := a.dungDonDaGiao(t)
	tokBan := a.dangNhapNhaBan(t, sellerID)

	res := a.call(http.MethodPost, "/api/v1/orders/"+maDon+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": maDong, "quantity": 1, "reason_code": "DEFECTIVE",
		}},
	}, hopNhat(khoaIdem(), map[string]string{"X-Guest-Phone": "0900321321"}))
	if res.code != http.StatusCreated {
		t.Fatalf("xin trả hàng: HTTP %d — %s", res.code, res.raw)
	}
	yc, _ := res.body["return"].(map[string]any)
	maYC, _ := yc["id"].(string)

	if err := a.mods.returns.Duyet(ctx, maYC, sellerID); err != nil {
		t.Fatalf("duyệt: %v", err)
	}
	if err := a.mods.returns.NhanHangVaHoanTien(ctx, maYC, sellerID); err != nil {
		t.Fatalf("nhận hàng: %v", err)
	}

	// Loại hàng mà KHÔNG nêu lý do.
	got := a.call(http.MethodPost, "/api/v1/seller/returns/"+maYC+"/inspect",
		map[string]any{
			"lines": []any{map[string]any{"order_line_id": maDong, "passed": false}},
		}, hopNhat(bearer(tokBan), khoaIdem()))
	if got.code == http.StatusOK {
		t.Error("loại hàng KHÔNG nêu lý do vẫn được chấp nhận")
	}

	// Có lý do thì được, và hàng KHÔNG quay lại bán được.
	truoc := a.tonKhoTheoTrangThai(t, maDon)
	ok := a.call(http.MethodPost, "/api/v1/seller/returns/"+maYC+"/inspect",
		map[string]any{
			"lines": []any{map[string]any{
				"order_line_id": maDong, "passed": false,
				"note": "rách đường may vai trái",
			}},
		}, hopNhat(bearer(tokBan), khoaIdem()))
	if ok.code != http.StatusOK {
		t.Fatalf("loại hàng có lý do: HTTP %d — %s", ok.code, ok.raw)
	}

	sau := a.tonKhoTheoTrangThai(t, maDon)
	if sau.khaDung != truoc.khaDung {
		t.Errorf("hàng LOẠI vẫn vào khả dụng: %d → %d — bán lại hàng hỏng "+
			"cho khách khác", truoc.khaDung, sau.khaDung)
	}
	if sau.hong-truoc.hong != 1 {
		t.Errorf("hàng loại vào Damaged đổi %d → %d, cần +1",
			truoc.hong, sau.hong)
	}
}

type tonKhoTrangThai struct{ khaDung, hoan, hong int }

func (a *apiTest) tonKhoTheoTrangThai(t *testing.T, orderID string) tonKhoTrangThai {
	t.Helper()
	var tk tonKhoTrangThai
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT coalesce(sum(quantity_available), 0),
		       coalesce(sum(quantity_returned), 0),
		       coalesce(sum(quantity_damaged), 0)
		  FROM inventory_item
		 WHERE sku_id IN (SELECT sku_id FROM order_line WHERE order_id = $1)`,
		orderID).Scan(&tk.khaDung, &tk.hoan, &tk.hong); err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	return tk
}
