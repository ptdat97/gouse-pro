package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// stocktake gửi một lần điều chỉnh tồn kho với vai trò vận hành kho.
func (a *apiTest) stocktake(
	t *testing.T, tok string, than map[string]any,
) reply {
	t.Helper()
	h := idemKey()
	h["Authorization"] = "Bearer " + tok
	return a.call(http.MethodPost,
		"/api/v1/admin/inventory/adjustments", than, h)
}

// stockSample lấy một bản ghi tồn kho có thật kèm kho và chủ sở hữu của nó.
func (a *apiTest) stockSample(t *testing.T) (skuID, locID, ownerID string, kd int) {
	t.Helper()
	err := a.db.Pool().QueryRow(context.Background(),
		`SELECT sku_id, stock_location_id, inventory_owner_id, quantity_available
		   FROM inventory_item
		  ORDER BY quantity_available DESC
		  LIMIT 1`).Scan(&skuID, &locID, &ownerID, &kd)
	if err != nil {
		t.Fatalf("đọc tồn kho mẫu: %v", err)
	}
	return skuID, locID, ownerID, kd
}

// TestAStocktakeSetsAnAbsoluteNumber — đường cơ bản.
func TestAStocktakeSetsAnAbsoluteNumber(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsWarehouse)
	sku, filtered, owner, kd := a.stockSample(t)

	count := kd + 7
	res := a.stocktake(t, tok, map[string]any{
		"sku_id":             sku,
		"stock_location_id":  filtered,
		"inventory_owner_id": owner,
		"reason":             "kiểm kê định kỳ cuối tháng, đếm tay hai lượt",
		"adjustments":        map[string]any{"quantity_available": count},
	})
	if res.code != http.StatusOK {
		t.Fatalf("kiểm kê: HTTP %d — %s", res.code, res.raw)
	}

	before, _ := res.body["before"].(map[string]any)
	after, _ := res.body["after"].(map[string]any)
	if got := int(after["quantity_available"].(float64)); got != count {
		t.Errorf("sau khi kiểm kê available = %d, cần %d", got, count)
	}
	if got := int(before["quantity_available"].(float64)); got != kd {
		t.Errorf("before = %d, cần %d — ảnh chụp trước không đúng", got, kd)
	}
}

// TestAStocktakeDoesNotTouchGoodsPromisedToCustomers.
//
// Người kiểm kê đếm hàng trên kệ. Hàng đã giữ hoặc đã cam kết cho khách vẫn
// nằm trên kệ nhưng KHÔNG còn tự do — nó là một lời hứa đã đưa ra, và người
// đếm kho không có thẩm quyền xóa lời hứa đó.
//
// Nếu kiểm kê ghi đè `reserved`/`committed`, hàng đã bán bị trả về trạng
// thái khả dụng và bán lần thứ hai — đúng kiểu lỗi "sinh hàng từ không khí"
// mà PH-31 đã dựng lại được.
func TestAStocktakeDoesNotTouchGoodsPromisedToCustomers(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsWarehouse)
	sku, filtered, owner, kd := a.stockSample(t)

	ctx := context.Background()
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE inventory_item
		    SET quantity_reserved = 3, quantity_committed = 2
		  WHERE sku_id=$1 AND stock_location_id=$2 AND inventory_owner_id=$3`,
		sku, filtered, owner); err != nil {
		t.Fatalf("dựng hàng đã giữ: %v", err)
	}

	res := a.stocktake(t, tok, map[string]any{
		"sku_id":             sku,
		"stock_location_id":  filtered,
		"inventory_owner_id": owner,
		"reason":             "kiểm kê sau khi chuyển kệ, đếm lại toàn bộ",
		"adjustments":        map[string]any{"quantity_available": kd + 1},
	})
	if res.code != http.StatusOK {
		t.Fatalf("kiểm kê: HTTP %d — %s", res.code, res.raw)
	}

	after, _ := res.body["after"].(map[string]any)
	if got := int(after["quantity_reserved"].(float64)); got != 3 {
		t.Errorf("kiểm kê làm đổi quantity_reserved thành %d, phải giữ 3 — "+
			"hàng đã giữ cho khách bị trả về khả dụng và bán được lần hai", got)
	}
	if got := int(after["quantity_committed"].(float64)); got != 2 {
		t.Errorf("kiểm kê làm đổi quantity_committed thành %d, phải giữ 2", got)
	}
}

// TestAStocktakeThatDeclaresNothingChangesNothing.
//
// Nếu `quantity_damaged` dùng int thường thay vì con trỏ, mọi request không
// nhắc tới số hỏng sẽ lặng lẽ đặt nó về 0 — xóa sạch hàng hỏng đã ghi nhận
// mà không ai ra lệnh làm thế.
func TestAStocktakeThatDeclaresNothingChangesNothing(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsWarehouse)
	sku, filtered, owner, kd := a.stockSample(t)

	ctx := context.Background()
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE inventory_item SET quantity_damaged = 4
		  WHERE sku_id=$1 AND stock_location_id=$2 AND inventory_owner_id=$3`,
		sku, filtered, owner); err != nil {
		t.Fatalf("dựng hàng hỏng: %v", err)
	}

	res := a.stocktake(t, tok, map[string]any{
		"sku_id":             sku,
		"stock_location_id":  filtered,
		"inventory_owner_id": owner,
		"reason":             "chỉ đếm hàng lành, chưa soát tới khu hàng hỏng",
		"adjustments":        map[string]any{"quantity_available": kd + 2},
	})
	if res.code != http.StatusOK {
		t.Fatalf("kiểm kê: HTTP %d — %s", res.code, res.raw)
	}

	after, _ := res.body["after"].(map[string]any)
	if got := int(after["quantity_damaged"].(float64)); got != 4 {
		t.Errorf("không khai quantity_damaged mà nó đổi thành %d, phải giữ 4 — "+
			"hàng hỏng đã ghi nhận bị xóa mà không ai ra lệnh", got)
	}
}

// TestAStocktakeChangesBothNumbersAtOnce: "90 lành, 10 hỏng" là MỘT lời khẳng định.
func TestAStocktakeChangesBothNumbersAtOnce(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsWarehouse)
	sku, filtered, owner, _ := a.stockSample(t)

	res := a.stocktake(t, tok, map[string]any{
		"sku_id":             sku,
		"stock_location_id":  filtered,
		"inventory_owner_id": owner,
		"reason":             "kiểm kê toàn bộ, tách riêng hàng lỗi đường may",
		"adjustments": map[string]any{
			"quantity_available": 90, "quantity_damaged": 10,
		},
	})
	if res.code != http.StatusOK {
		t.Fatalf("kiểm kê: HTTP %d — %s", res.code, res.raw)
	}
	after, _ := res.body["after"].(map[string]any)
	if int(after["quantity_available"].(float64)) != 90 ||
		int(after["quantity_damaged"].(float64)) != 10 {
		t.Errorf("đặt hai số trong một lần không ăn: %s", res.raw)
	}
}

// TestAStocktakeDemandsALongEnoughReason: tồn kho lệch mà không giải trình được thì mất cắp
// trông giống hệt sai sót nhập liệu.
func TestAStocktakeDemandsALongEnoughReason(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsWarehouse)
	sku, filtered, owner, kd := a.stockSample(t)

	for _, reason := range []string{"", "sai", "fix"} {
		res := a.stocktake(t, tok, map[string]any{
			"sku_id":             sku,
			"stock_location_id":  filtered,
			"inventory_owner_id": owner,
			"reason":             reason,
			"adjustments":        map[string]any{"quantity_available": kd + 1},
		})
		if res.code != http.StatusBadRequest {
			t.Errorf("lý do %q được chấp nhận: HTTP %d — %s",
				reason, res.code, res.raw)
		}
	}
}

// TestAStocktakeWithAnAmbiguousOwnerIsRejected.
//
// Cùng một SKU ở cùng một kho có thể có tồn kho của nhiều chủ — hàng seller
// gửi ở kho nền tảng vẫn thuộc seller. Đoán bừa một trong số đó là sửa nhầm
// tồn kho của người khác, và cả hai bên không biết cho tới khi bán hụt.
func TestAStocktakeWithAnAmbiguousOwnerIsRejected(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsWarehouse)
	sku, filtered, owner, _ := a.stockSample(t)

	// Dựng bản ghi thứ hai: cùng SKU, cùng kho, CHỦ KHÁC.
	ctx := context.Background()
	if _, err := a.db.Pool().Exec(ctx,
		`INSERT INTO inventory_item
		   (id, sku_id, stock_location_id, inventory_owner_id,
		    quantity_available, version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,5,0,now(),now())`,
		"inv_01M19TESTNHAPNHANG0000001", sku, filtered, owner+"x"); err != nil {
		t.Fatalf("dựng chủ sở hữu thứ hai: %v", err)
	}

	res := a.stocktake(t, tok, map[string]any{
		"sku_id":            sku,
		"stock_location_id": filtered,
		"reason":            "kiểm kê định kỳ, không nêu rõ chủ sở hữu",
		"adjustments":       map[string]any{"quantity_available": 1},
	})
	if res.code != http.StatusConflict {
		t.Fatalf("không nêu chủ sở hữu khi có nhiều chủ: HTTP %d, cần 409 — "+
			"hệ thống đang ĐOÁN xem sửa tồn kho của ai: %s", res.code, res.raw)
	}
}
