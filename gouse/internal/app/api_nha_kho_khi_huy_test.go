package app

import (
	"context"
	"testing"
)

// Hủy đơn thực hiện phải TRẢ HÀNG VỀ KHO.
//
// # Vì sao bài này chỉ ra đời 01/10/2026
//
// Bên nhận `inventory.NewReleaseHandler` được worker đăng ký từ lâu, nhưng
// bộ bên nhận của test lớp này KHÔNG đăng ký nó — và không chú thích nào nói
// vì sao. Nên mọi bài test ở đây chạy với một hệ thống không có đường ra của
// kho, và không bài nào bắt được điều đó.
//
// Lỗi mà khoảng trống ấy che chính là lỗi `order/events.go` mô tả kèm lần
// kiểm chứng bằng đơn thật: *"đặt 5 món rồi hủy, tồn kho đứng nguyên 15 khả
// dụng / 5 cam kết. Không lỗi, không log."*
//
// # Vì sao đo CẢ HAI con số
//
// Chỉ đo `quantity_available` thì một bản cài cộng vào khả dụng mà quên trừ
// cam kết sẽ xanh — và hệ quả của nó là tồn kho tự nhân lên: cùng một món
// được đếm hai lần, nên nền tảng bán thứ không có.
func TestCancellingAFulfillmentReturnsGoodsToStock(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	_, dsFO := a.paidTwoSellerOrder(t, "nhakho")
	cancelled := dsFO[0]

	// Chụp tồn kho của ĐÚNG các món trong đơn thực hiện sắp hủy.
	//
	// Theo `fulfillment_order_line` chứ không theo cả đơn: nhà bán còn lại
	// không bị hủy, nên gộp cả đơn sẽ trộn hai nhóm số vào nhau.
	before := a.fulfillmentStock(t, cancelled.id)
	if len(before) == 0 {
		t.Skip("đơn thực hiện không có dòng nào tra ra tồn kho")
	}

	hasCommitment := false
	for _, v := range before {
		if v.committed > 0 {
			hasCommitment = true
		}
	}
	if !hasCommitment {
		t.Fatalf("trước khi hủy, không món nào ở trạng thái CAM KẾT: %v — "+
			"đơn đã thanh toán thì hàng phải đang được giữ cho nó", before)
	}

	if err := a.mods.fulfillment.CancelFulfillment(
		ctx, cancelled.sellerID, cancelled.id, "hết hàng",
	); err != nil {
		t.Fatalf("hủy đơn thực hiện: %v", err)
	}
	a.emitEvent(t)

	after := a.fulfillmentStock(t, cancelled.id)
	for sku, t0 := range before {
		t1, ok := after[sku]
		if !ok {
			t.Errorf("sku %s biến mất khỏi tồn kho sau khi hủy", sku)
			continue
		}

		if t1.committed >= t0.committed {
			t.Errorf("sku %s: cam kết %d → %d — hủy đơn mà hàng vẫn bị giữ "+
				"cho một đơn không còn tồn tại", sku, t0.committed, t1.committed)
		}
		if t1.khaDung <= t0.khaDung {
			t.Errorf("sku %s: khả dụng %d → %d — hàng có thật trên kệ nhưng "+
				"hệ thống không bán được nữa", sku, t0.khaDung, t1.khaDung)
		}

		// TỔNG phải GIỮ NGUYÊN: nhả hàng là CHUYỂN trạng thái, không phải
		// tạo thêm hàng. Thiếu khẳng định này thì một bản cài cộng vào khả
		// dụng mà quên trừ cam kết vẫn xanh ở hai điều trên.
		if t0.khaDung+t0.committed != t1.khaDung+t1.committed {
			t.Errorf("sku %s: tổng (khả dụng + cam kết) đổi %d → %d — nhả "+
				"hàng phải CHUYỂN trạng thái, không tạo thêm hàng",
				sku, t0.khaDung+t0.committed, t1.khaDung+t1.committed)
		}
	}
}

type stockLevel struct{ khaDung, committed int }

// fulfillmentStock trả tồn kho theo SKU của các món trong một đơn
// thực hiện.
//
// Gộp theo SKU trên mọi kho và mọi chủ sở hữu: bài test không quan tâm hàng
// nằm ở đâu, chỉ quan tâm nó CHUYỂN từ cam kết sang khả dụng.
func (a *apiTest) fulfillmentStock(
	t *testing.T, foID string,
) map[string]stockLevel {
	t.Helper()

	rows, err := a.db.Pool().Query(context.Background(), `
		SELECT i.sku_id,
		       COALESCE(SUM(i.quantity_available), 0),
		       COALESCE(SUM(i.quantity_committed), 0)
		  FROM inventory_item i
		 WHERE i.sku_id IN (
		           SELECT l.sku_id FROM fulfillment_order_line l
		            WHERE l.fulfillment_order_id = $1)
		 GROUP BY i.sku_id`, foID)
	if err != nil {
		t.Fatalf("đọc tồn kho của đơn thực hiện: %v", err)
	}
	defer rows.Close()

	out := map[string]stockLevel{}
	for rows.Next() {
		var sku string
		var m stockLevel
		if err := rows.Scan(&sku, &m.khaDung, &m.committed); err != nil {
			t.Fatalf("đọc tồn kho: %v", err)
		}
		out[sku] = m
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	return out
}
