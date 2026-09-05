package app

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"
)

// dungNhieuDon tạo `n` đơn cho MỘT khách rồi đặt trạng thái XEN KẼ theo
// đúng thứ tự mà API trả về.
//
// Xen kẽ là điều kiện sống còn của mấy bài dưới, không phải chi tiết trang
// trí. Nếu các đơn bị loại nằm liền một khối ở CUỐI danh sách thì lọc
// trong truy vấn và lọc sau khi đọc cho ra CÙNG một trang đầu, và bài test
// không phân biệt được hai bên — xanh mà chẳng chứng minh gì.
//
// `placed_at` cũng được đặt cách nhau hẳn một phút: truy vấn thật sắp theo
// `placed_at DESC`, nên nếu các đơn trùng mốc thời gian thì thứ tự do
// PostgreSQL tuỳ ý chọn và bài test đổi kết quả giữa các lần chạy.
//
// Trả về token và trạng thái của nhóm đơn KHÔNG bị huỷ, để bài test lọc
// theo trạng thái có thật thay vì một chuỗi đoán mò.
func (a *apiTest) dungNhieuDon(t *testing.T, n int) (string, string) {
	t.Helper()
	d := a.dungNhieuDonCoEmail(t, n)
	return d.token, d.trangThaiConLai
}

// donDaDung gom những gì bài test cần sau khi dựng dữ liệu. Trả struct thay
// vì bốn giá trị trần: `(string, string, string, string)` thì đổi chỗ hai
// tham số là lỗi âm thầm, trình biên dịch không bắt được.
type donDaDung struct {
	token           string
	email           string
	maKhach         string
	trangThaiConLai string
}

func (a *apiTest) dungNhieuDonCoEmail(t *testing.T, n int) donDaDung {
	t.Helper()
	ctx := context.Background()

	email := emailMoi("locdon")
	tok := a.dangKyVaDangNhap(email)

	var customerID string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT id FROM customer WHERE email = lower($1)`, email).
		Scan(&customerID); err != nil {
		t.Fatalf("đọc hồ sơ khách: %v", err)
	}

	for i := 0; i < n; i++ {
		maPhien := a.dungPhienSanHoanTat(email, "0900444555")
		res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
			map[string]any{"payment_method": "COD"}, khoaIdem())
		if res.code != http.StatusCreated && res.code != http.StatusOK {
			t.Fatalf("hoàn tất đơn %d: HTTP %d — %s", i, res.code, res.raw)
		}
	}

	// `guest_email` lưu NGUYÊN VĂN chuỗi khách gửi lên, còn `customer.email`
	// thì lưu chữ thường. So sánh hai cột đó phải hạ chữ CẢ HAI bên, không
	// chỉ tham số — khớp hụt ở đây thì không đơn nào được gắn vào khách và
	// mọi khẳng định bên dưới chạy trên danh sách rỗng.
	gan, err := a.db.Pool().Exec(ctx,
		`UPDATE "order" SET customer_id = $1 WHERE lower(guest_email) = lower($2)`,
		customerID, email)
	if err != nil {
		t.Fatalf("gắn đơn vào khách: %v", err)
	}
	if got := gan.RowsAffected(); got != int64(n) {
		t.Fatalf("gắn được %d/%d đơn vào khách", got, n)
	}

	// Đơn lẻ thứ tự thành CANCELLED, đơn chẵn giữ nguyên: theo `placed_at
	// DESC` thì hai trạng thái nằm xen kẽ một-một.
	if _, err := a.db.Pool().Exec(ctx, `
		WITH danh AS (
		  SELECT id, row_number() OVER (ORDER BY created_at, id) AS n
		    FROM "order" WHERE customer_id = $1)
		UPDATE "order" o
		   SET placed_at = now() - (d.n || ' minutes')::interval,
		       status = CASE WHEN d.n % 2 = 1 THEN 'CANCELLED' ELSE o.status END
		  FROM danh d WHERE o.id = d.id`,
		customerID); err != nil {
		t.Fatalf("đặt trạng thái xen kẽ: %v", err)
	}

	// Chống test rỗng ruột: nếu dữ liệu không có ĐỦ HAI trạng thái thì mọi
	// khẳng định bên dưới đều đúng một cách vô nghĩa.
	var conLai string
	var soHuy, soConLai int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT coalesce(max(status) FILTER (WHERE status <> 'CANCELLED'), ''),
		       count(*) FILTER (WHERE status = 'CANCELLED'),
		       count(*) FILTER (WHERE status <> 'CANCELLED')
		  FROM "order" WHERE customer_id = $1`, customerID).
		Scan(&conLai, &soHuy, &soConLai); err != nil {
		t.Fatalf("đếm trạng thái: %v", err)
	}
	if soHuy < 2 || soConLai < 2 || conLai == "" {
		t.Fatalf("dữ liệu dựng sai: %d đơn CANCELLED, %d đơn %q — cần ít "+
			"nhất 2 mỗi bên thì bộ lọc mới có gì để lọc",
			soHuy, soConLai, conLai)
	}
	return donDaDung{
		token: tok, email: email, maKhach: customerID, trangThaiConLai: conLai,
	}
}

// TestLocTrangThaiKhongLamTrangTraThieu.
//
// # Lỗi trước đây
//
// Bộ lọc chạy SAU khi đọc: bản ghi bị loại vẫn tính vào trang, nên một
// trang trả ít hơn `limit`.
//
// Khi CẢ trang bị loại thì tệ hơn hẳn: `data` rỗng trong lúc `has_more`
// vẫn true. Client thường coi trang rỗng là hết dữ liệu và dừng phân
// trang — khách mất phần lịch sử còn lại mà không có lỗi nào.
func TestLocTrangThaiKhongLamTrangTraThieu(t *testing.T) {
	a := newAPITest(t)
	tok, _ := a.dungNhieuDon(t, 6)

	res := a.call(http.MethodGet,
		"/api/v1/orders?status=CANCELLED&limit=2", nil, bearer(tok))
	if res.code != http.StatusOK {
		t.Fatalf("lọc theo trạng thái: HTTP %d — %s", res.code, res.raw)
	}

	ds, _ := res.body["data"].([]any)
	pg, _ := res.body["pagination"].(map[string]any)
	conNua, _ := pg["has_more"].(bool)

	// Còn trang sau thì trang này PHẢI đầy.
	if conNua && len(ds) != 2 {
		t.Errorf("trang trả %d đơn trong khi limit=2 và has_more=true — "+
			"bộ lọc đang chạy sau khi đọc, nên bản ghi bị loại vẫn tính "+
			"vào trang: %s", len(ds), res.raw)
	}

	// Và mọi đơn trả về phải ĐÚNG trạng thái đã lọc.
	for _, x := range ds {
		m, _ := x.(map[string]any)
		if s, _ := m["status"].(string); s != "CANCELLED" {
			t.Errorf("lọc CANCELLED nhưng trả đơn trạng thái %q", s)
		}
	}
}

// TestLocTrangThaiKhongTraTrangRong.
//
// Trang RỖNG kèm `has_more=true` là tổ hợp không được phép: client đọc nó
// là "hết dữ liệu" và dừng, trong khi máy chủ nói còn.
func TestLocTrangThaiKhongTraTrangRong(t *testing.T) {
	a := newAPITest(t)
	tok, conLai := a.dungNhieuDon(t, 6)

	for _, tt := range []string{"CANCELLED", conLai} {
		res := a.call(http.MethodGet,
			"/api/v1/orders?status="+tt+"&limit=1", nil, bearer(tok))
		if res.code != http.StatusOK {
			t.Fatalf("%s: HTTP %d — %s", tt, res.code, res.raw)
		}
		ds, _ := res.body["data"].([]any)
		pg, _ := res.body["pagination"].(map[string]any)
		conNua, _ := pg["has_more"].(bool)

		if len(ds) == 0 && conNua {
			t.Errorf("%s: trang RỖNG nhưng has_more=true — client sẽ coi "+
				"là hết dữ liệu và dừng: %s", tt, res.raw)
		}
	}
}

// TestLocTrangThaiLaTra400.
//
// Trạng thái gõ sai phải báo lỗi, KHÔNG trả danh sách rỗng: rỗng trông
// giống "khách chưa có đơn nào" chứ không giống "bạn gõ sai".
func TestLocTrangThaiLaTra400(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("locsai"))

	for _, xau := range []string{"KHONG_TON_TAI", "cancelled", "'; DROP"} {
		// Mã hoá như client thật: chuỗi có khoảng trắng mà ghép thẳng vào
		// URL thì hỏng ngay ở khâu dựng request, chưa tới được máy chủ.
		res := a.call(http.MethodGet,
			"/api/v1/orders?status="+url.QueryEscape(xau), nil, bearer(tok))
		if res.code != http.StatusBadRequest {
			t.Errorf("trạng thái %q: HTTP %d, cần 400 — %s",
				xau, res.code, res.raw)
		}
	}

	// Không lọc thì vẫn phải chạy bình thường.
	if res := a.call(http.MethodGet, "/api/v1/orders", nil, bearer(tok)); res.code != http.StatusOK {
		t.Errorf("không lọc: HTTP %d — %s", res.code, res.raw)
	}
}

// themDonMoiNhat tạo thêm MỘT đơn cho khách đã có, và đặt `placed_at` sau
// mọi đơn cũ — tức là đơn này đứng ĐẦU danh sách sắp theo `placed_at DESC`.
//
// Đây là tình huống thật, không phải dựng để bắt lỗi: khách mở trang lịch
// sử, xem trang 1, rồi đặt thêm một đơn trước khi bấm "xem thêm".
func (a *apiTest) themDonMoiNhat(t *testing.T, email, customerID string) {
	t.Helper()
	ctx := context.Background()

	maPhien := a.dungPhienSanHoanTat(email, "0900444555")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("đơn mới: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE "order" SET customer_id = $1, placed_at = now() WHERE id = $2`,
		customerID, maDon); err != nil {
		t.Fatalf("gắn đơn mới: %v", err)
	}
}

// docTrang trả danh sách id của một trang, kèm con trỏ đi tiếp.
func (a *apiTest) docTrang(t *testing.T, tok, truyVan string) ([]string, string) {
	t.Helper()
	res := a.call(http.MethodGet, "/api/v1/orders?"+truyVan, nil, bearer(tok))
	if res.code != http.StatusOK {
		t.Fatalf("đọc trang %q: HTTP %d — %s", truyVan, res.code, res.raw)
	}
	ds, _ := res.body["data"].([]any)
	ids := make([]string, 0, len(ds))
	for _, x := range ds {
		m, _ := x.(map[string]any)
		id, _ := m["id"].(string)
		ids = append(ids, id)
	}
	pg, _ := res.body["pagination"].(map[string]any)
	con, _ := pg["next_cursor"].(string)
	return ids, con
}

// TestPhanTrangKhongLapBanGhi.
//
// # Lỗi
//
// `next_cursor` là SỐ THỨ TỰ bỏ qua (offset) chứ không phải khoá. Giữa hai
// lần đọc mà có đơn mới xen vào đầu danh sách, mọi bản ghi bị đẩy lùi một
// bậc, nên trang sau đọc lại bản ghi mà trang trước đã trả.
//
// Khách thấy CÙNG một đơn hai lần trong lịch sử mua hàng. Tệ hơn: nếu một
// đơn rời khỏi tập lọc giữa chừng thì chiều ngược lại xảy ra — một đơn bị
// NHẢY QUA và không bao giờ hiện ra.
func TestPhanTrangKhongLapBanGhi(t *testing.T) {
	a := newAPITest(t)
	d := a.dungNhieuDonCoEmail(t, 6)
	tok := d.token

	trang1, con := a.docTrang(t, tok, "limit=3")
	if len(trang1) != 3 || con == "" {
		t.Fatalf("trang 1 có %d đơn, con trỏ %q — cần 3 đơn và còn trang sau",
			len(trang1), con)
	}

	// Khách đặt thêm một đơn TRƯỚC khi bấm "xem thêm".
	a.themDonMoiNhat(t, d.email, d.maKhach)

	trang2, _ := a.docTrang(t, tok, "limit=3&cursor="+url.QueryEscape(con))

	da := map[string]bool{}
	for _, id := range trang1 {
		da[id] = true
	}
	for _, id := range trang2 {
		if da[id] {
			t.Errorf("đơn %s xuất hiện ở CẢ trang 1 và trang 2 — con trỏ là "+
				"offset, nên đơn mới xen vào đầu đẩy mọi bản ghi lùi một "+
				"bậc\ntrang 1: %v\ntrang 2: %v", id, trang1, trang2)
		}
	}
}

// TestPhanTrangDiHetDanhSach: đi hết mọi trang phải gặp MỖI đơn đúng MỘT lần.
//
// Lặp bản ghi thì khách còn nhìn thấy và nghi ngờ. Chiều ngược lại — bản
// ghi bị NHẢY QUA — thì im lặng hoàn toàn: đơn đó biến mất khỏi lịch sử mà
// không ai biết để đi tìm. Bài này bắt cả hai chiều.
func TestPhanTrangDiHetDanhSach(t *testing.T) {
	a := newAPITest(t)
	d := a.dungNhieuDonCoEmail(t, 7)

	dem := map[string]int{}
	truyVan, soTrang := "limit=2", 0
	for {
		ids, con := a.docTrang(t, d.token, truyVan)
		for _, id := range ids {
			dem[id]++
		}
		soTrang++
		if soTrang > 20 {
			t.Fatal("đi quá 20 trang cho 7 đơn — con trỏ không tiến")
		}
		if con == "" {
			break
		}
		truyVan = "limit=2&cursor=" + url.QueryEscape(con)
	}

	if len(dem) != 7 {
		t.Errorf("đi hết các trang gặp %d đơn khác nhau, cần 7 — có đơn bị "+
			"nhảy qua", len(dem))
	}
	for id, n := range dem {
		if n != 1 {
			t.Errorf("đơn %s xuất hiện %d lần khi đi hết danh sách", id, n)
		}
	}
	// Chống test rỗng ruột: 7 đơn với limit=2 phải là 4 trang.
	if soTrang != 4 {
		t.Errorf("đi %d trang cho 7 đơn với limit=2, cần 4", soTrang)
	}
}

// TestConTroBiaTra400: con trỏ bịa phải báo lỗi, không lặng lẽ đọc từ đầu.
//
// Đọc từ đầu là kiểu hỏng xấu nhất ở đây: client tưởng đang đọc trang 5,
// nhận lại trang 1, rồi lặp vô hạn nếu nó cứ đi tiếp theo con trỏ trả về.
func TestConTroBiaTra400(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("contro"))

	// Con trỏ mang id của thực thể KHÁC: đúng định dạng, sai kiểu.
	saiKieu := base64.RawURLEncoding.EncodeToString(
		[]byte("1757000000000000|off_01ARZ3NDEKTSV4RRFFQ69G5FAV"))

	for _, xau := range []string{
		"0", "3", "khong-phai-base64!!", "MTIz", saiKieu,
		base64.RawURLEncoding.EncodeToString([]byte("abc|ord_01ARZ3NDEKTSV4RRFFQ69G5FAV")),
	} {
		res := a.call(http.MethodGet,
			"/api/v1/orders?cursor="+url.QueryEscape(xau), nil, bearer(tok))
		if res.code != http.StatusBadRequest {
			t.Errorf("cursor %q: HTTP %d, cần 400 — %s", xau, res.code, res.raw)
		}
	}
}

// TestPhanTrangKhiTrungMocThoiGian: nhiều đơn CÙNG một `placed_at`.
//
// Đây là lý do mốc phải gồm CẢ `id`, không riêng `placed_at`. Trùng mốc
// không phải chuyện hiếm dựng ra để bắt lỗi: đơn tạo trong cùng một
// transaction dùng chung `now()` của PostgreSQL, và nhập liệu hàng loạt
// thì cả lô chung một mốc.
//
// Nếu chỉ so `placed_at`, trang sau hỏi "đơn nào CŨ HƠN mốc" và loại sạch
// những đơn cùng mốc — kể cả đơn chưa ai đọc. Danh sách đứt giữa chừng và
// không có lỗi nào báo.
func TestPhanTrangKhiTrungMocThoiGian(t *testing.T) {
	a := newAPITest(t)
	d := a.dungNhieuDonCoEmail(t, 7)

	// Dồn MỌI đơn về đúng một mốc thời gian.
	if _, err := a.db.Pool().Exec(context.Background(),
		`UPDATE "order" SET placed_at = timestamptz '2026-09-01 10:00:00+07'
		  WHERE customer_id = $1`, d.maKhach); err != nil {
		t.Fatalf("dồn mốc thời gian: %v", err)
	}

	dem := map[string]int{}
	truyVan, soTrang := "limit=2", 0
	for {
		ids, con := a.docTrang(t, d.token, truyVan)
		for _, id := range ids {
			dem[id]++
		}
		soTrang++
		if soTrang > 20 {
			t.Fatal("đi quá 20 trang cho 7 đơn — con trỏ không tiến")
		}
		if con == "" {
			break
		}
		truyVan = "limit=2&cursor=" + url.QueryEscape(con)
	}

	if len(dem) != 7 {
		t.Errorf("7 đơn cùng một `placed_at`, đi hết các trang chỉ gặp %d — "+
			"mốc bỏ qua `id` nên đơn cùng mốc bị loại sạch", len(dem))
	}
	for id, n := range dem {
		if n != 1 {
			t.Errorf("đơn %s xuất hiện %d lần", id, n)
		}
	}
}
