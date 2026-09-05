package app

import (
	"context"
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
	return tok, conLai
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
