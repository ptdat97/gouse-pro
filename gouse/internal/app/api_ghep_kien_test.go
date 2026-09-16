package app

import (
	"net/http"
	"testing"
)

// TestKienGhepDuocVoiDongHang.
//
// # Vì sao bài này tồn tại
//
// Chạy hệ thống thật trên Docker (16/09) và đọc hai response cạnh nhau:
//
//	GET /orders/{id}            → order_line_id: "oln_01M2NDZDMG…"
//	GET /orders/{id}/shipments  → order_line_ids: ["cln_01M2NDXWBT…"]
//
// Không một mã nào trùng. Trường `order_line_ids` tồn tại để trang chi
// tiết đơn biết DÒNG NÀO ĐI TRONG KIỆN NÀO — chú thích của nó nói thẳng
// là "cho phép TRANG ghép mà KHÔNG cần thêm lượt gọi nào". Phép ghép ấy
// khớp 0 dòng, trên MỌI đơn trong hệ thống.
//
// Gốc: `checkout.completed` mang mã dòng của PHIÊN (`cln_`) trong một
// trường mà chú thích của `ReservedLine.LineID` khai là "dòng hàng trong
// ĐƠN HÀNG". Hợp đồng nói một đằng, mã điền một nẻo, và không bài test
// nào đối chiếu hai đầu nên cả hai bên đều "đúng" khi xét riêng.
//
// Bài này đối chiếu ĐÚNG hai đầu đó, qua HTTP, như trang web phải làm.
func TestKienGhepDuocVoiDongHang(t *testing.T) {
	a := newAPITest(t)
	const sdt = "0900333222"
	maDon := a.datDonCOD(t, "ghepkien")

	// Kiện sinh ra từ `checkout.completed`, và event đi qua outbox — chưa
	// phát thì chưa có đơn thực hiện nào để ghép.
	a.phatEvent(t)

	h := map[string]string{"X-Guest-Phone": sdt}

	res := a.call(http.MethodGet, "/api/v1/orders/"+maDon, nil, h)
	if res.code != http.StatusOK {
		t.Fatalf("đọc đơn: HTTP %d — %s", res.code, res.raw)
	}
	dongDon := map[string]bool{}
	for _, raw := range res.body["lines"].([]any) {
		d, _ := raw.(map[string]any)
		if id, _ := d["order_line_id"].(string); id != "" {
			dongDon[id] = true
		}
	}
	if len(dongDon) == 0 {
		t.Fatalf("đơn không có dòng hàng nào: %s", res.raw)
	}

	res = a.call(http.MethodGet, "/api/v1/orders/"+maDon+"/shipments", nil, h)
	if res.code != http.StatusOK {
		t.Fatalf("đọc kiện: HTTP %d — %s", res.code, res.raw)
	}
	kien, _ := res.body["data"].([]any)
	if len(kien) == 0 {
		t.Fatalf("đơn không sinh kiện nào: %s", res.raw)
	}

	var daGhep int
	for _, raw := range kien {
		k, _ := raw.(map[string]any)
		ids, _ := k["order_line_ids"].([]any)
		if len(ids) == 0 {
			t.Errorf("kiện không khai dòng nào — trang không biết trong gói có gì")
		}
		for _, v := range ids {
			id, _ := v.(string)
			if !dongDon[id] {
				t.Errorf("kiện chứa mã %q KHÔNG có trong đơn — trang ghép không ra "+
					"dòng nào; mã của đơn là %v", id, khoaCua(dongDon))
				continue
			}
			daGhep++
		}
	}
	if daGhep == 0 {
		t.Error("không dòng hàng nào ghép được với kiện nào")
	}
}

func khoaCua(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
