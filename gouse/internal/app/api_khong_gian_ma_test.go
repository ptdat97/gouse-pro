package app

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// TestKhongGianMaKhongLanNhau.
//
// # Vì sao bài này tồn tại
//
// Ngày 16/09, chạy hệ thống thật trên Docker và đọc hai response cạnh nhau:
//
//	GET /orders/{id}            → order_line_id:  "oln_…"
//	GET /orders/{id}/shipments  → order_line_ids: ["cln_…"]
//
// Cả hai trường đều được gán, đều có dữ liệu, đều đúng kiểu. Phép ghép mà
// chúng tồn tại để phục vụ khớp 0 dòng — trên MỌI đơn trong hệ thống. Cùng
// hình dạng ấy lặp lại ở `event_log.session_id`, nơi ba không gian mã nằm
// chung một cột và không tập nào giao tập nào, khiến `conversion_rate` bằng
// 0 vĩnh viễn.
//
// Không test nào bắt được, vì mỗi test soi MỘT đầu và mỗi đầu đều đúng.
//
// # Cách bài này kiểm, và vì sao nó không cần bảo trì
//
// Nó KHÔNG mang một bảng kỳ vọng viết tay. Nó đọc luật ra từ chính dữ liệu:
//
//  1. Tiền tố trong cột `id` của bảng T ĐỊNH NGHĨA không gian mã của T.
//     `order_line.id` toàn `oln_` → không gian của `order_line` là `oln`.
//  2. Mọi cột tên `T_id` ở BẤT KỲ bảng nào phải nằm trong không gian của T.
//     `fulfillment_order_line.order_line_id` chứa `cln_` → sai.
//
// Bảng mới, cột mới đều tự động được phủ. Thứ duy nhất phải khai tay là
// ngoại lệ — cột CỐ Ý đa hình, và mỗi dòng ngoại lệ phải nói rõ vì sao.
func TestKhongGianMaKhongLanNhau(t *testing.T) {
	a := newAPITest(t)

	// Dựng dữ liệu đi hết chuỗi lõi: giỏ → phiên → đơn → đơn thực hiện.
	// Không có dữ liệu thì không có tiền tố nào để đối chiếu, và bài test
	// sẽ xanh vì rỗng — đúng kiểu xanh vô nghĩa mà nó sinh ra để chống.
	maDon := a.datDonCOD(t, "khonggianma")
	a.phatEvent(t)
	if maDon == "" {
		t.Fatal("không dựng được đơn để lấy dữ liệu")
	}

	// Đọc thêm hai đường nữa để chắc `fulfillment_order_line` có dòng.
	if res := a.call(http.MethodGet, "/api/v1/orders/"+maDon+"/shipments", nil,
		map[string]string{"X-Guest-Phone": "0900333222"}); res.code != http.StatusOK {
		t.Fatalf("đọc kiện hàng: HTTP %d — %s", res.code, res.raw)
	}

	cot := docCacCotMa(t, a)

	// Bước 1: không gian mã của từng bảng, lấy từ chính cột `id` của nó.
	khongGian := map[string]string{}
	for _, c := range cot {
		if c.cot != "id" {
			continue
		}
		tt := tienToCua(t, a, c.bang, c.cot)
		if len(tt) == 1 {
			khongGian[c.bang] = tt[0]
		}
	}
	if len(khongGian) < 10 {
		t.Fatalf("chỉ suy ra được %d không gian mã — dữ liệu quá mỏng để "+
			"kết luận gì", len(khongGian))
	}

	// Bước 2: mọi cột `T_id` phải nằm trong không gian của T.
	var daKiem int
	for _, c := range cot {
		if c.cot == "id" {
			continue
		}
		bangDich, co := bangCuaCot(c.bang, c.cot, khongGian)
		if !co {
			continue // không suy ra được bảng đích — bỏ qua, xem ghi chú dưới
		}
		muon := khongGian[bangDich]
		khoa := c.bang + "." + c.cot

		for _, tt := range tienToCua(t, a, c.bang, c.cot) {
			if !laTienTo(tt) {
				// Không phải mã có tiền tố: hằng canh (`seed-demo`,
				// `dev-tool`) hoặc dữ liệu dựng tay trong test. Phép kiểm
				// này nói về KHÔNG GIAN MÃ, không về mọi chuỗi lọt vào cột.
				t.Logf("bỏ qua %s: %q không phải mã có tiền tố", khoa, tt)
				continue
			}
			daKiem++
			if tt == muon {
				continue
			}
			if ly := noDaBiet[khoa]; ly != "" {
				t.Logf("NỢ ĐÃ BIẾT — %s chứa %q thay vì %q: %s",
					khoa, tt, muon, ly)
				continue
			}
			t.Errorf("%s.%s chứa mã %q nhưng phải là %q (không gian của "+
				"bảng %q, lấy từ %s.id).\n"+
				"    Một cột mang mã của không gian KHÁC thì mọi phép ghép "+
				"dựa vào nó khớp 0 dòng, và cả hai đầu vẫn trông đúng khi "+
				"xét riêng.\n"+
				"    Cố ý đa hình thì khai vào `ngoaiLeDaHinh` kèm lý do.",
				c.bang, c.cot, tt, muon, bangDich, bangDich)
		}
	}

	if daKiem == 0 {
		t.Fatal("không kiểm được cột nào — phép quét hỏng chứ không phải hệ " +
			"thống sạch")
	}
	t.Logf("đã đối chiếu %d cặp (cột, tiền tố) trên %d không gian mã",
		daKiem, len(khongGian))
}

// ngoaiLeDaHinh là những cột CỐ Ý chứa nhiều không gian mã.
//
// Mỗi dòng phải có cột phân loại đi kèm cho biết mã thuộc loại nào — không
// có nó thì đây không phải đa hình, mà là lẫn lộn.
var ngoaiLeDaHinh = map[string]string{
	"audit_log.resource_id":           "resource_type cho biết đây là tài nguyên gì",
	"demand_signal.source_id":         "source_type cho biết tín hiệu đến từ đâu",
	"event_log.subject_id":            "subject_type cho biết đối tượng là gì",
	"event_outbox.aggregate_id":       "aggregate_type cho biết aggregate nào",
	"inventory_movement.reference_id": "reference_type cho biết lý do điều chuyển",
	"notification_log.reference_id":   "reference_type cho biết thông báo về cái gì",
}

// noDaBiet là những cột ĐANG SAI mà bản sửa còn chờ một quyết định.
//
// Khác `ngoaiLeDaHinh` ở chỗ: kia là thiết kế, đây là nợ. Bài test ghi ra
// từng dòng ở đây thay vì đỏ, vì chặn cả bộ test vì một việc chưa quyết
// được thì người ta sẽ xóa phép kiểm chứ không đi quyết.
//
// Mỗi dòng phải trỏ tới chỗ ghi quyết định đang chờ.
var noDaBiet = map[string]string{
	"event_log.session_id": "ba không gian mã nằm chung một cột — " +
		"`ses_` (phiên duyệt web), `crt_` (giỏ), `chk_` (phiên thanh toán). " +
		"Hệ quả: tử số và mẫu số của `conversion_rate` không bao giờ giao " +
		"nhau nên chỉ số ấy bằng 0 vĩnh viễn. Sửa được thì phải trả lời " +
		"trước 'một PHIÊN là gì' — xem P3-50 trong docs/10-roadmap/backlog.md.",
}

// laTienTo cho biết một chuỗi có hình dạng tiền tố mã của hệ thống không.
//
// Mã của dự án là `abc_<ULID>`: đúng ba chữ cái thường rồi gạch dưới. Giá
// trị không theo hình dạng đó là hằng canh hoặc dữ liệu test dựng tay, và
// phép kiểm này không nói gì về chúng.
func laTienTo(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// coTen là một cột định danh trong schema.
type coTen struct{ bang, cot string }

// docCacCotMa liệt kê mọi cột text tên `id` hoặc `*_id`.
func docCacCotMa(t *testing.T, a *apiTest) []coTen {
	t.Helper()

	rows, err := a.db.Pool().Query(context.Background(), `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND data_type IN ('text', 'character varying')
		  AND (column_name = 'id' OR column_name LIKE '%\_id')
		  AND table_name <> 'schema_migrations'
		ORDER BY table_name, column_name`)
	if err != nil {
		t.Fatalf("đọc schema: %v", err)
	}
	defer rows.Close()

	var out []coTen
	for rows.Next() {
		var c coTen
		if err := rows.Scan(&c.bang, &c.cot); err != nil {
			t.Fatalf("đọc schema: %v", err)
		}
		out = append(out, c)
	}
	return out
}

// tienToCua trả các tiền tố KHÁC NHAU đang có trong một cột.
//
// Bỏ qua giá trị rỗng: cột định danh trong schema này dùng ” thay cho
// NULL, và "chưa gán" không phải "gán sai".
func tienToCua(t *testing.T, a *apiTest, bang, cot string) []string {
	t.Helper()

	q := fmt.Sprintf(
		`SELECT DISTINCT split_part(%s, '_', 1) FROM %s WHERE %s <> ''`,
		trichDan(cot), trichDan(bang), trichDan(cot))

	rows, err := a.db.Pool().Query(context.Background(), q)
	if err != nil {
		t.Fatalf("quét %s.%s: %v", bang, cot, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("quét %s.%s: %v", bang, cot, err)
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// bangCuaCot suy ra bảng đích của một cột `*_id`.
//
// # Vì sao chỉ khớp CHÍNH XÁC, không đoán thêm
//
// `inventory_owner_id` bỏ tiền tố sẽ thành `owner_id` rồi `inventory_id` —
// cả hai đều sai (chủ sở hữu tồn kho là một nhà bán). Đoán rộng đổi một
// phép kiểm tin được lấy vài cảnh báo giả, và cảnh báo giả là thứ làm
// người ta tắt hẳn phép kiểm.
//
// Cột không suy ra được bảng nào thì KHÔNG kiểm — bài test này không hứa
// phủ hết, nó hứa mọi thứ nó nói đều đúng.
func bangCuaCot(bang, cot string, khongGian map[string]string) (string, bool) {
	if ngoaiLeDaHinh[bang+"."+cot] != "" {
		return "", false
	}

	ten := strings.TrimSuffix(cot, "_id")

	// `parent_id` trỏ tới chính bảng đang đứng (danh mục cha).
	if ten == "parent" {
		ten = bang
	}

	if _, co := khongGian[ten]; co {
		return ten, true
	}
	return "", false
}

// trichDan bọc tên định danh SQL.
//
// Cần vì `user` là từ khóa của PostgreSQL: không bọc thì câu truy vấn
// quét bảng đó là lỗi cú pháp, và bảng người dùng là đúng chỗ đáng quét
// nhất.
func trichDan(s string) string { return `"` + s + `"` }
