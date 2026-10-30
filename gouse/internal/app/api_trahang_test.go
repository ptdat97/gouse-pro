package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
)

// buildDeliveredOrder dựng một đơn đã giao xong, trả (mã đơn, mã dòng, mã nhà bán).
func (a *apiTest) buildDeliveredOrder(t *testing.T) (string, string, string) {
	t.Helper()
	ctx := context.Background()

	checkoutID := a.buildCheckoutReadyToComplete("trahang@example.com", "0900321321")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ := ord["id"].(string)

	a.emitEvent(t)
	fo := a.findFulfillment(t, orderID)
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
	a.emitEvent(t)

	var lineID string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT id FROM order_line WHERE order_id = $1 LIMIT 1`, orderID).
		Scan(&lineID); err != nil {
		t.Fatalf("đọc dòng hàng: %v", err)
	}
	return orderID, lineID, fo.sellerID
}

// TestACustomerReturnGoesEndToEnd — PH-37.
//
// Xin trả → nhà bán duyệt → nhận hàng → hoàn tiền, tất cả qua HTTP thật.
//
// Hai bất biến quan trọng nhất được kiểm ở TẦNG DATABASE, không qua
// response: hàng hoàn KHÔNG vào tồn khả dụng, và sổ cái CÓ bút toán hoàn.
func TestACustomerReturnGoesEndToEnd(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	orderID, lineID, sellerID := a.buildDeliveredOrder(t)

	res := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": lineID, "quantity": 1,
			"reason_code": "SIZE_TOO_SMALL", "reason_detail": "chật quá",
		}},
	}, merge(idemKey(), map[string]string{"X-Guest-Phone": "0900321321"}))
	if res.code != http.StatusCreated {
		t.Fatalf("xin trả hàng: HTTP %d — %s", res.code, res.raw)
	}
	request, _ := res.body["return"].(map[string]any)
	requestID, _ := request["id"].(string)
	if tt, _ := request["status"].(string); tt != "REQUESTED" {
		t.Errorf("trạng thái đầu là %q, cần REQUESTED", tt)
	}

	// Tiền hoàn phải bằng giá thực trả của dòng.
	cash, _ := request["refund_amount"].(map[string]any)
	amount, _ := cash["amount"].(float64)
	if amount <= 0 {
		t.Errorf("số tiền hoàn là %v", amount)
	}

	// --- nhà bán duyệt rồi nhận hàng ---
	if err := a.mods.returns.Approve(ctx, requestID, sellerID); err != nil {
		t.Fatalf("duyệt: %v", err)
	}

	beforeAvailable := a.availableStock(t, orderID)
	beforeEntry := a.countRows("ledger_entry", "reference_id = $1", requestID)

	if err := a.mods.returns.ReceiveAndRefund(ctx, requestID, sellerID); err != nil {
		t.Fatalf("nhận hàng và hoàn tiền: %v", err)
	}

	// BẤT BIẾN 1: hàng hoàn KHÔNG vào tồn khả dụng.
	//
	// docs/07-workflows/return.md mục 4: vi phạm quy tắc này nghĩa là bán
	// lại hàng hỏng cho khách khác, và thiệt hại uy tín lớn hơn nhiều giá
	// trị món hàng.
	afterAvailable := a.availableStock(t, orderID)
	if afterAvailable != beforeAvailable {
		t.Errorf("tồn KHẢ DỤNG đổi %d → %d khi nhận hàng hoàn — "+
			"hàng hoàn phải vào trạng thái Returned, chờ kiểm định",
			beforeAvailable, afterAvailable)
	}

	// BẤT BIẾN 2: sổ cái có bút toán hoàn.
	if n := a.countRows("ledger_entry", "reference_id = $1", requestID) - beforeEntry; n < 1 {
		t.Errorf("có %d bút toán hoàn tiền, cần ít nhất 1 — "+
			"khách được trả hàng mà tiền không được đảo ngược", n)
	}
}

// TestYouCannotReturnAnothersOrder — bất biến BẢO MẬT.
func TestYouCannotReturnAnothersOrder(t *testing.T) {
	a := newAPITest(t)
	orderID, lineID, _ := a.buildDeliveredOrder(t)

	// Không có số điện thoại của khách → không được đụng vào đơn.
	res := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": lineID, "quantity": 1, "reason_code": "CHANGED_MIND",
		}},
	}, idemKey())
	if res.code != http.StatusNotFound {
		t.Errorf("người lạ xin trả hàng: HTTP %d, cần 404 — %s", res.code, res.raw)
	}
}

// TestRequestingAReturnTwiceForOneItemIsBlocked.
//
// Không có chặn này, khách gửi hai yêu cầu cho cùng một món và được hoàn
// tiền hai lần cho một lần mua.
func TestRequestingAReturnTwiceForOneItemIsBlocked(t *testing.T) {
	a := newAPITest(t)
	orderID, lineID, _ := a.buildDeliveredOrder(t)

	than := map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": lineID, "quantity": 1, "reason_code": "CHANGED_MIND",
		}},
	}
	h := merge(idemKey(), map[string]string{"X-Guest-Phone": "0900321321"})

	if res := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/returns", than, h); res.code != http.StatusCreated {
		t.Fatalf("lần đầu: HTTP %d — %s", res.code, res.raw)
	}

	h2 := merge(idemKey(), map[string]string{"X-Guest-Phone": "0900321321"})
	again := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/returns", than, h2)
	if again.code == http.StatusCreated {
		t.Error("xin trả LẦN HAI cho cùng một món vẫn thành công — " +
			"khách được hoàn tiền hai lần cho một lần mua")
	}
}

// availableStock đọc tổng tồn khả dụng của các SKU trong đơn.
func (a *apiTest) availableStock(t *testing.T, orderID string) int {
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

// TestInspectionPutsReturnedGoodsBackOnSale.
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
//	domain RecordInspection  → từ chối khi dòng đã kiểm
//	bất biến của tồn kho      → không còn hàng ở Returned để chuyển
//
// Đã kiểm bằng cách phá: bỏ lớp domain thì bài này VẪN XANH, vì tồn kho
// chặn. Cả hai đều đúng và đều cần — nhưng lớp domain là thứ trả về lỗi
// đọc được cho nhà bán, còn tồn kho chỉ trả 500.
func TestInspectionPutsReturnedGoodsBackOnSale(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	orderID, lineID, sellerID := a.buildDeliveredOrder(t)
	tokSeller := a.logInAsSeller(t, sellerID)

	res := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": lineID, "quantity": 1, "reason_code": "SIZE_TOO_SMALL",
		}},
	}, merge(idemKey(), map[string]string{"X-Guest-Phone": "0900321321"}))
	if res.code != http.StatusCreated {
		t.Fatalf("xin trả hàng: HTTP %d — %s", res.code, res.raw)
	}
	request, _ := res.body["return"].(map[string]any)
	requestID, _ := request["id"].(string)

	if err := a.mods.returns.Approve(ctx, requestID, sellerID); err != nil {
		t.Fatalf("duyệt: %v", err)
	}
	if err := a.mods.returns.ReceiveAndRefund(ctx, requestID, sellerID); err != nil {
		t.Fatalf("nhận hàng: %v", err)
	}

	before := a.stockByStatus(t, orderID)
	if before.refunded == 0 {
		t.Fatalf("sau khi nhận hàng, tồn Returned là 0 — hàng chưa vào kho")
	}

	// KIỂM ĐỊNH ĐẠT — qua HTTP, đường trước đây không tồn tại.
	got := a.call(http.MethodPost, "/api/v1/seller/returns/"+requestID+"/inspect",
		map[string]any{
			"lines": []any{map[string]any{"order_line_id": lineID, "passed": true}},
		}, merge(bearer(tokSeller), idemKey()))
	if got.code != http.StatusOK {
		t.Fatalf("kiểm định: HTTP %d — %s", got.code, got.raw)
	}

	// Đo MỨC THAY ĐỔI, không phải con số tuyệt đối: SKU dùng chung với
	// các bài khác trong cùng lượt chạy, và tổng tuyệt đối là đo cả dữ
	// liệu của người khác.
	after := a.stockByStatus(t, orderID)
	if after.available-before.available != 1 {
		t.Errorf("hàng ĐẠT kiểm định: khả dụng đổi %d → %d, cần +1 — "+
			"hàng hoàn không quay lại bán được và nằm chết trong kho",
			before.available, after.available)
	}
	if before.refunded-after.refunded != 1 {
		t.Errorf("tồn Returned đổi %d → %d, cần −1 — hàng không rời trạng "+
			"thái chờ kiểm định", before.refunded, after.refunded)
	}

	// KIỂM HAI LẦN phải bị chặn: hàng vào Available gấp đôi là tồn kho
	// tăng thêm số hàng không có thật.
	again := a.call(http.MethodPost, "/api/v1/seller/returns/"+requestID+"/inspect",
		map[string]any{
			"lines": []any{map[string]any{"order_line_id": lineID, "passed": true}},
		}, merge(bearer(tokSeller), idemKey()))
	if again.code == http.StatusOK {
		t.Error("kiểm định LẦN HAI vẫn thành công — tồn kho tăng thêm hàng không có thật")
	}
	if last := a.stockByStatus(t, orderID); last.available != after.available {
		t.Errorf("kiểm lần hai làm khả dụng đổi %d → %d", after.available, last.available)
	}
}

// TestScrappingGoodsRequiresAReason.
//
// Lý do loại là đầu vào cho việc làm việc với nhà cung cấp và quyết ai
// chịu chi phí. "Hỏng" không có mô tả thì không làm được gì với nó.
//
// Cũng có HAI lớp: kiểm ở domain và CHECK `return_line_fail_needs_note` ở
// database. Bỏ lớp domain thì bài vẫn xanh vì database chặn — nhưng khi
// đó nhà bán nhận 500 thay vì một thông điệp nói rõ thiếu gì.
func TestScrappingGoodsRequiresAReason(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	orderID, lineID, sellerID := a.buildDeliveredOrder(t)
	tokSeller := a.logInAsSeller(t, sellerID)

	res := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/returns", map[string]any{
		"lines": []any{map[string]any{
			"order_line_id": lineID, "quantity": 1, "reason_code": "DEFECTIVE",
		}},
	}, merge(idemKey(), map[string]string{"X-Guest-Phone": "0900321321"}))
	if res.code != http.StatusCreated {
		t.Fatalf("xin trả hàng: HTTP %d — %s", res.code, res.raw)
	}
	request, _ := res.body["return"].(map[string]any)
	requestID, _ := request["id"].(string)

	if err := a.mods.returns.Approve(ctx, requestID, sellerID); err != nil {
		t.Fatalf("duyệt: %v", err)
	}
	if err := a.mods.returns.ReceiveAndRefund(ctx, requestID, sellerID); err != nil {
		t.Fatalf("nhận hàng: %v", err)
	}

	// Loại hàng mà KHÔNG nêu lý do.
	got := a.call(http.MethodPost, "/api/v1/seller/returns/"+requestID+"/inspect",
		map[string]any{
			"lines": []any{map[string]any{"order_line_id": lineID, "passed": false}},
		}, merge(bearer(tokSeller), idemKey()))
	if got.code == http.StatusOK {
		t.Error("loại hàng KHÔNG nêu lý do vẫn được chấp nhận")
	}

	// Có lý do thì được, và hàng KHÔNG quay lại bán được.
	before := a.stockByStatus(t, orderID)
	ok := a.call(http.MethodPost, "/api/v1/seller/returns/"+requestID+"/inspect",
		map[string]any{
			"lines": []any{map[string]any{
				"order_line_id": lineID, "passed": false,
				"note": "rách đường may vai trái",
			}},
		}, merge(bearer(tokSeller), idemKey()))
	if ok.code != http.StatusOK {
		t.Fatalf("loại hàng có lý do: HTTP %d — %s", ok.code, ok.raw)
	}

	after := a.stockByStatus(t, orderID)
	if after.available != before.available {
		t.Errorf("hàng LOẠI vẫn vào khả dụng: %d → %d — bán lại hàng hỏng "+
			"cho khách khác", before.available, after.available)
	}
	if after.broken-before.broken != 1 {
		t.Errorf("hàng loại vào Damaged đổi %d → %d, cần +1",
			before.broken, after.broken)
	}
}

type stockStatus struct{ available, refunded, broken int }

func (a *apiTest) stockByStatus(t *testing.T, orderID string) stockStatus {
	t.Helper()
	var tk stockStatus
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT coalesce(sum(quantity_available), 0),
		       coalesce(sum(quantity_returned), 0),
		       coalesce(sum(quantity_damaged), 0)
		  FROM inventory_item
		 WHERE sku_id IN (SELECT sku_id FROM order_line WHERE order_id = $1)`,
		orderID).Scan(&tk.available, &tk.refunded, &tk.broken); err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	return tk
}
