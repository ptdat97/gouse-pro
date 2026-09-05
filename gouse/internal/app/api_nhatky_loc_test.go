package app

import (
	"context"
	"math/rand/v2"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// ghiVetLuc chèn thẳng một bản ghi nhật ký với `occurred_at` ĐỊNH SẴN.
//
// Không đi qua API: mọi đường ghi nhật ký đều dùng `now()` của PostgreSQL,
// nên không có cách nào đặt được mốc thời gian mong muốn — mà mốc thời gian
// chính là thứ bài test này kiểm.
func (a *apiTest) ghiVetLuc(t *testing.T, actorID string, luc time.Time) {
	t.Helper()
	// `id` phải đúng dạng `aud_` + 26 ký tự ULID (ràng buộc của bảng).
	if _, err := a.db.Pool().Exec(context.Background(), `
		INSERT INTO audit_log (id, actor_type, actor_id, action,
		                       resource_type, resource_id, occurred_at)
		VALUES ($3, 'USER', $1, 'LOC_THOI_GIAN',
		        'CONFIG', 'cfg-loc', $2)`,
		actorID, luc, "aud_"+maULIDTest()); err != nil {
		t.Fatalf("ghi vết lúc %s: %v", luc.Format(time.RFC3339), err)
	}
}

// maULIDTest sinh 26 ký tự theo bảng chữ Crockford base32 của ULID.
func maULIDTest() string {
	const chuCai = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	b := make([]byte, 26)
	for i := range b {
		b[i] = chuCai[rand.IntN(len(chuCai))]
	}
	return string(b)
}

// demVet đếm số bản ghi endpoint trả về cho một khoảng ngày.
func (a *apiTest) demVet(t *testing.T, tok, actorID, tuNgay, denNgay string) int {
	t.Helper()
	res := a.call(http.MethodGet,
		"/api/v1/admin/audit-log?actor_id="+actorID+
			"&from="+tuNgay+"&to="+denNgay, nil, bearer(tok))
	if res.code != http.StatusOK {
		t.Fatalf("lọc %s..%s: HTTP %d — %s", tuNgay, denNgay, res.code, res.raw)
	}
	ds, _ := res.body["data"].([]any)
	return len(ds)
}

// TestLocNgayBaoGomTronNgayCuoi.
//
// "đến ngày 31/08" phải bao gồm CẢ NGÀY 31. Nếu `to` dừng ở 00:00 sáng hôm
// đó thì nhân viên lọc theo tháng mất sạch bản ghi của ngày cuối tháng —
// và mất im lặng, vì kết quả vẫn trả 200 kèm một danh sách trông hợp lý.
func TestLocNgayBaoGomTronNgayCuoi(t *testing.T) {
	a := newAPITest(t)
	tok, actorID := a.taoTaiKhoanVaiTroCoID(t, identity.RoleAdmin)
	vn := time.FixedZone("ICT", 7*60*60)

	// Sát TRONG ranh giới: 23:59:59 ngày 20/08 giờ Việt Nam.
	a.ghiVetLuc(t, actorID, time.Date(2026, 8, 20, 23, 59, 59, 0, vn))

	if n := a.demVet(t, tok, actorID, "2026-08-01", "2026-08-20"); n != 1 {
		t.Errorf("lọc đến 20/08 trả %d bản ghi, cần 1 — mốc `to` cắt ở "+
			"00:00 nên cả ngày cuối bị mất", n)
	}
}

// TestLocNgayKhongLanSangNgaySau: ranh giới trên phải ĐÓNG.
//
// Đi kèm bài trên và không tách rời được: "bao trọn ngày cuối" mà thiếu bài
// này thì một bộ lọc trả VỀ MỌI THỨ cũng xanh. Cặp hai bài mới ghim được
// đúng một khoảng.
func TestLocNgayKhongLanSangNgaySau(t *testing.T) {
	a := newAPITest(t)
	tok, actorID := a.taoTaiKhoanVaiTroCoID(t, identity.RoleAdmin)
	vn := time.FixedZone("ICT", 7*60*60)

	// Sát NGOÀI ranh giới: 30 giây sau nửa đêm, đã sang ngày 21/08.
	a.ghiVetLuc(t, actorID, time.Date(2026, 8, 21, 0, 0, 30, 0, vn))

	if n := a.demVet(t, tok, actorID, "2026-08-01", "2026-08-20"); n != 0 {
		t.Errorf("lọc đến 20/08 trả %d bản ghi, cần 0 — bản ghi của ngày "+
			"21/08 lọt vào khoảng đã đóng", n)
	}
}

// TestLocNgayTheoGioVietNam.
//
// # Vấn đề
//
// `parseDate` dùng `time.Parse` không kèm múi giờ, nên "2026-08-20" là
// 00:00 UTC. Nhưng nền tảng vận hành ở Việt Nam (UTC+7), và giao diện quản
// trị hiển thị giờ địa phương.
//
// Hệ quả: một thao tác lúc 03:00 sáng ngày 20/08 giờ Việt Nam được lưu là
// 20:00 UTC ngày 19/08. Nhân viên lọc đúng ngày 20/08 KHÔNG thấy nó, dù
// giao diện hiển thị bản ghi đó là "20/08 03:00".
//
// Với nhật ký kiểm toán — thứ tồn tại để điều tra sự cố — mất bảy giờ đầu
// mỗi ngày là khiếm khuyết thật: "cho tôi xem mọi việc đã xảy ra hôm đó"
// im lặng bỏ sót ca đêm.
func TestLocNgayTheoGioVietNam(t *testing.T) {
	a := newAPITest(t)
	tok, actorID := a.taoTaiKhoanVaiTroCoID(t, identity.RoleAdmin)

	vn := time.FixedZone("ICT", 7*60*60)

	// 03:00 sáng 20/08 giờ Việt Nam = 20:00 UTC ngày 19/08.
	a.ghiVetLuc(t, actorID, time.Date(2026, 8, 20, 3, 0, 0, 0, vn))

	if n := a.demVet(t, tok, actorID, "2026-08-20", "2026-08-20"); n != 1 {
		t.Errorf("thao tác lúc 03:00 ngày 20/08 giờ Việt Nam không nằm "+
			"trong bộ lọc ngày 20/08 (trả %d bản ghi) — mốc ngày đang "+
			"tính theo UTC trong khi nghiệp vụ chạy ở UTC+7", n)
	}
}

// TestLocNgayDonHangTheoGioVietNam.
//
// Cùng khiếm khuyết, cùng cách sửa, nhưng ở MỘT endpoint khác:
// `GET /admin/orders` cũng lọc theo ngày và cũng từng cắt mốc theo UTC.
//
// Có bài riêng vì mỗi endpoint tự nối dây tới `types.PhanTichNgay`. Đưa
// riêng handler đơn hàng về `time.Parse` mà không có bài này thì KHÔNG bài
// nào đỏ vì lý do đúng — đã thử: bài duy nhất đỏ là bài chặn trần trang, và
// nó đỏ vì `time.Parse("")` báo lỗi với chuỗi rỗng, không phải vì múi giờ.
func TestLocNgayDonHangTheoGioVietNam(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()
	vn := time.FixedZone("ICT", 7*60*60)

	// Một đơn có thật, rồi dời `placed_at` về 03:00 sáng giờ Việt Nam.
	// Ngày chọn xa các bài khác để không đếm nhầm dữ liệu của chúng.
	email := emailMoi("locdon-ngay")
	a.dangKyVaDangNhap(email)
	maPhien := a.dungPhienSanHoanTat(email, "0900444555")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("hoàn tất đơn: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	// 03:00 ngày 15/03 giờ Việt Nam = 20:00 UTC ngày 14/03.
	luc := time.Date(2026, 3, 15, 3, 0, 0, 0, vn)
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE "order" SET placed_at = $1 WHERE id = $2`, luc, maDon); err != nil {
		t.Fatalf("dời placed_at: %v", err)
	}

	tokAdmin := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)
	got := a.call(http.MethodGet,
		"/api/v1/admin/orders?from=2026-03-15&to=2026-03-15", nil, bearer(tokAdmin))
	if got.code != http.StatusOK {
		t.Fatalf("lọc đơn theo ngày: HTTP %d — %s", got.code, got.raw)
	}

	ds, _ := got.body["data"].([]any)
	thay := false
	for _, x := range ds {
		m, _ := x.(map[string]any)
		if id, _ := m["id"].(string); id == maDon {
			thay = true
		}
	}
	if !thay {
		t.Errorf("đơn đặt lúc 03:00 ngày 15/03 giờ Việt Nam không nằm trong "+
			"bộ lọc ngày 15/03 — mốc ngày đang tính theo UTC (%d đơn trả về)",
			len(ds))
	}
}
