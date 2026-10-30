package app

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"
)

// buildOrders tạo `n` đơn cho MỘT khách rồi đặt trạng thái XEN KẼ theo
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
func (a *apiTest) buildOrders(t *testing.T, n int) (string, string) {
	t.Helper()
	d := a.buildOrdersWithEmail(t, n)
	return d.token, d.remainingStatuses
}

// builtOrder gom những gì bài test cần sau khi dựng dữ liệu. Trả struct thay
// vì bốn giá trị trần: `(string, string, string, string)` thì đổi chỗ hai
// tham số là lỗi âm thầm, trình biên dịch không bắt được.
type builtOrder struct {
	token             string
	email             string
	custID            string
	remainingStatuses string
}

func (a *apiTest) buildOrdersWithEmail(t *testing.T, n int) builtOrder {
	t.Helper()
	ctx := context.Background()

	email := newEmail("locdon")
	tok := a.registerAndLogIn(email)

	var customerID string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT id FROM customer WHERE email = lower($1)`, email).
		Scan(&customerID); err != nil {
		t.Fatalf("đọc hồ sơ khách: %v", err)
	}

	for i := 0; i < n; i++ {
		checkoutID := a.buildCheckoutReadyToComplete(email, "0900444555")
		res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
			map[string]any{"payment_method": "COD"}, idemKey())
		if res.code != http.StatusCreated && res.code != http.StatusOK {
			t.Fatalf("hoàn tất đơn %d: HTTP %d — %s", i, res.code, res.raw)
		}
	}

	// `guest_email` lưu NGUYÊN VĂN chuỗi khách gửi lên, còn `customer.email`
	// thì lưu chữ thường. So sánh hai cột đó phải hạ chữ CẢ HAI bên, không
	// chỉ tham số — khớp hụt ở đây thì không đơn nào được gắn vào khách và
	// mọi khẳng định bên dưới chạy trên danh sách rỗng.
	assign, err := a.db.Pool().Exec(ctx,
		`UPDATE "order" SET customer_id = $1 WHERE lower(guest_email) = lower($2)`,
		customerID, email)
	if err != nil {
		t.Fatalf("gắn đơn vào khách: %v", err)
	}
	if got := assign.RowsAffected(); got != int64(n) {
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
	var left string
	var cancelCount, remaining int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT coalesce(max(status) FILTER (WHERE status <> 'CANCELLED'), ''),
		       count(*) FILTER (WHERE status = 'CANCELLED'),
		       count(*) FILTER (WHERE status <> 'CANCELLED')
		  FROM "order" WHERE customer_id = $1`, customerID).
		Scan(&left, &cancelCount, &remaining); err != nil {
		t.Fatalf("đếm trạng thái: %v", err)
	}
	if cancelCount < 2 || remaining < 2 || left == "" {
		t.Fatalf("dữ liệu dựng sai: %d đơn CANCELLED, %d đơn %q — cần ít "+
			"nhất 2 mỗi bên thì bộ lọc mới có gì để lọc",
			cancelCount, remaining, left)
	}
	return builtOrder{
		token: tok, email: email, custID: customerID, remainingStatuses: left,
	}
}

// TestAStatusFilterDoesNotShortAPage.
//
// # Lỗi trước đây
//
// Bộ lọc chạy SAU khi đọc: bản ghi bị loại vẫn tính vào trang, nên một
// trang trả ít hơn `limit`.
//
// Khi CẢ trang bị loại thì tệ hơn hẳn: `data` rỗng trong lúc `has_more`
// vẫn true. Client thường coi trang rỗng là hết dữ liệu và dừng phân
// trang — khách mất phần lịch sử còn lại mà không có lỗi nào.
func TestAStatusFilterDoesNotShortAPage(t *testing.T) {
	a := newAPITest(t)
	tok, _ := a.buildOrders(t, 6)

	res := a.call(http.MethodGet,
		"/api/v1/orders?status=CANCELLED&limit=2", nil, bearer(tok))
	if res.code != http.StatusOK {
		t.Fatalf("lọc theo trạng thái: HTTP %d — %s", res.code, res.raw)
	}

	ds, _ := res.body["data"].([]any)
	pg, _ := res.body["pagination"].(map[string]any)
	more, _ := pg["has_more"].(bool)

	// Còn trang sau thì trang này PHẢI đầy.
	if more && len(ds) != 2 {
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

// TestAStatusFilterDoesNotReturnAnEmptyPage.
//
// Trang RỖNG kèm `has_more=true` là tổ hợp không được phép: client đọc nó
// là "hết dữ liệu" và dừng, trong khi máy chủ nói còn.
func TestAStatusFilterDoesNotReturnAnEmptyPage(t *testing.T) {
	a := newAPITest(t)
	tok, left := a.buildOrders(t, 6)

	for _, tt := range []string{"CANCELLED", left} {
		res := a.call(http.MethodGet,
			"/api/v1/orders?status="+tt+"&limit=1", nil, bearer(tok))
		if res.code != http.StatusOK {
			t.Fatalf("%s: HTTP %d — %s", tt, res.code, res.raw)
		}
		ds, _ := res.body["data"].([]any)
		pg, _ := res.body["pagination"].(map[string]any)
		more, _ := pg["has_more"].(bool)

		if len(ds) == 0 && more {
			t.Errorf("%s: trang RỖNG nhưng has_more=true — client sẽ coi "+
				"là hết dữ liệu và dừng: %s", tt, res.raw)
		}
	}
}

// TestAnUnknownStatusFilterReturns400.
//
// Trạng thái gõ sai phải báo lỗi, KHÔNG trả danh sách rỗng: rỗng trông
// giống "khách chưa có đơn nào" chứ không giống "bạn gõ sai".
func TestAnUnknownStatusFilterReturns400(t *testing.T) {
	a := newAPITest(t)
	tok := a.registerAndLogIn(newEmail("locsai"))

	for _, bad := range []string{"KHONG_TON_TAI", "cancelled", "'; DROP"} {
		// Mã hoá như client thật: chuỗi có khoảng trắng mà ghép thẳng vào
		// URL thì hỏng ngay ở khâu dựng request, chưa tới được máy chủ.
		res := a.call(http.MethodGet,
			"/api/v1/orders?status="+url.QueryEscape(bad), nil, bearer(tok))
		if res.code != http.StatusBadRequest {
			t.Errorf("trạng thái %q: HTTP %d, cần 400 — %s",
				bad, res.code, res.raw)
		}
	}

	// Không lọc thì vẫn phải chạy bình thường.
	if res := a.call(http.MethodGet, "/api/v1/orders", nil, bearer(tok)); res.code != http.StatusOK {
		t.Errorf("không lọc: HTTP %d — %s", res.code, res.raw)
	}
}

// addLatestOrder tạo thêm MỘT đơn cho khách đã có, và đặt `placed_at` sau
// mọi đơn cũ — tức là đơn này đứng ĐẦU danh sách sắp theo `placed_at DESC`.
//
// Đây là tình huống thật, không phải dựng để bắt lỗi: khách mở trang lịch
// sử, xem trang 1, rồi đặt thêm một đơn trước khi bấm "xem thêm".
func (a *apiTest) addLatestOrder(t *testing.T, email, customerID string) {
	t.Helper()
	ctx := context.Background()

	checkoutID := a.buildCheckoutReadyToComplete(email, "0900444555")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("đơn mới: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ := ord["id"].(string)

	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE "order" SET customer_id = $1, placed_at = now() WHERE id = $2`,
		customerID, orderID); err != nil {
		t.Fatalf("gắn đơn mới: %v", err)
	}
}

// readPage trả danh sách id của một trang, kèm con trỏ đi tiếp.
func (a *apiTest) readPage(t *testing.T, tok, query string) ([]string, string) {
	t.Helper()
	res := a.call(http.MethodGet, "/api/v1/orders?"+query, nil, bearer(tok))
	if res.code != http.StatusOK {
		t.Fatalf("đọc trang %q: HTTP %d — %s", query, res.code, res.raw)
	}
	ds, _ := res.body["data"].([]any)
	ids := make([]string, 0, len(ds))
	for _, x := range ds {
		m, _ := x.(map[string]any)
		id, _ := m["id"].(string)
		ids = append(ids, id)
	}
	pg, _ := res.body["pagination"].(map[string]any)
	leftover, _ := pg["next_cursor"].(string)
	return ids, leftover
}

// TestPagingNeverRepeatsARecord.
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
func TestPagingNeverRepeatsARecord(t *testing.T) {
	a := newAPITest(t)
	d := a.buildOrdersWithEmail(t, 6)
	tok := d.token

	page1, leftover := a.readPage(t, tok, "limit=3")
	if len(page1) != 3 || leftover == "" {
		t.Fatalf("trang 1 có %d đơn, con trỏ %q — cần 3 đơn và còn trang sau",
			len(page1), leftover)
	}

	// Khách đặt thêm một đơn TRƯỚC khi bấm "xem thêm".
	a.addLatestOrder(t, d.email, d.custID)

	page2, _ := a.readPage(t, tok, "limit=3&cursor="+url.QueryEscape(leftover))

	seen := map[string]bool{}
	for _, id := range page1 {
		seen[id] = true
	}
	for _, id := range page2 {
		if seen[id] {
			t.Errorf("đơn %s xuất hiện ở CẢ trang 1 và trang 2 — con trỏ là "+
				"offset, nên đơn mới xen vào đầu đẩy mọi bản ghi lùi một "+
				"bậc\ntrang 1: %v\ntrang 2: %v", id, page1, page2)
		}
	}
}

// TestPagingWalksTheWholeList: đi hết mọi trang phải gặp MỖI đơn đúng MỘT lần.
//
// Lặp bản ghi thì khách còn nhìn thấy và nghi ngờ. Chiều ngược lại — bản
// ghi bị NHẢY QUA — thì im lặng hoàn toàn: đơn đó biến mất khỏi lịch sử mà
// không ai biết để đi tìm. Bài này bắt cả hai chiều.
func TestPagingWalksTheWholeList(t *testing.T) {
	a := newAPITest(t)
	d := a.buildOrdersWithEmail(t, 7)

	count := map[string]int{}
	query, pageCount := "limit=2", 0
	for {
		ids, leftover := a.readPage(t, d.token, query)
		for _, id := range ids {
			count[id]++
		}
		pageCount++
		if pageCount > 20 {
			t.Fatal("đi quá 20 trang cho 7 đơn — con trỏ không tiến")
		}
		if leftover == "" {
			break
		}
		query = "limit=2&cursor=" + url.QueryEscape(leftover)
	}

	if len(count) != 7 {
		t.Errorf("đi hết các trang gặp %d đơn khác nhau, cần 7 — có đơn bị "+
			"nhảy qua", len(count))
	}
	for id, n := range count {
		if n != 1 {
			t.Errorf("đơn %s xuất hiện %d lần khi đi hết danh sách", id, n)
		}
	}
	// Chống test rỗng ruột: 7 đơn với limit=2 phải là 4 trang.
	if pageCount != 4 {
		t.Errorf("đi %d trang cho 7 đơn với limit=2, cần 4", pageCount)
	}
}

// TestABadCursorReturns400: con trỏ bịa phải báo lỗi, không lặng lẽ đọc từ đầu.
//
// Đọc từ đầu là kiểu hỏng xấu nhất ở đây: client tưởng đang đọc trang 5,
// nhận lại trang 1, rồi lặp vô hạn nếu nó cứ đi tiếp theo con trỏ trả về.
func TestABadCursorReturns400(t *testing.T) {
	a := newAPITest(t)
	tok := a.registerAndLogIn(newEmail("contro"))

	// Con trỏ mang id của thực thể KHÁC: đúng định dạng, sai kiểu.
	wrongKind := base64.RawURLEncoding.EncodeToString(
		[]byte("1757000000000000|off_01ARZ3NDEKTSV4RRFFQ69G5FAV"))

	for _, bad := range []string{
		"0", "3", "khong-phai-base64!!", "MTIz", wrongKind,
		base64.RawURLEncoding.EncodeToString([]byte("abc|ord_01ARZ3NDEKTSV4RRFFQ69G5FAV")),
	} {
		res := a.call(http.MethodGet,
			"/api/v1/orders?cursor="+url.QueryEscape(bad), nil, bearer(tok))
		if res.code != http.StatusBadRequest {
			t.Errorf("cursor %q: HTTP %d, cần 400 — %s", bad, res.code, res.raw)
		}
	}
}

// TestPagingWhenTimestampsCollide: nhiều đơn CÙNG một `placed_at`.
//
// Đây là lý do mốc phải gồm CẢ `id`, không riêng `placed_at`. Trùng mốc
// không phải chuyện hiếm dựng ra để bắt lỗi: đơn tạo trong cùng một
// transaction dùng chung `now()` của PostgreSQL, và nhập liệu hàng loạt
// thì cả lô chung một mốc.
//
// Nếu chỉ so `placed_at`, trang sau hỏi "đơn nào CŨ HƠN mốc" và loại sạch
// những đơn cùng mốc — kể cả đơn chưa ai đọc. Danh sách đứt giữa chừng và
// không có lỗi nào báo.
func TestPagingWhenTimestampsCollide(t *testing.T) {
	a := newAPITest(t)
	d := a.buildOrdersWithEmail(t, 7)

	// Dồn MỌI đơn về đúng một mốc thời gian.
	if _, err := a.db.Pool().Exec(context.Background(),
		`UPDATE "order" SET placed_at = timestamptz '2026-09-01 10:00:00+07'
		  WHERE customer_id = $1`, d.custID); err != nil {
		t.Fatalf("dồn mốc thời gian: %v", err)
	}

	count := map[string]int{}
	query, pageCount := "limit=2", 0
	for {
		ids, leftover := a.readPage(t, d.token, query)
		for _, id := range ids {
			count[id]++
		}
		pageCount++
		if pageCount > 20 {
			t.Fatal("đi quá 20 trang cho 7 đơn — con trỏ không tiến")
		}
		if leftover == "" {
			break
		}
		query = "limit=2&cursor=" + url.QueryEscape(leftover)
	}

	if len(count) != 7 {
		t.Errorf("7 đơn cùng một `placed_at`, đi hết các trang chỉ gặp %d — "+
			"mốc bỏ qua `id` nên đơn cùng mốc bị loại sạch", len(count))
	}
	for id, n := range count {
		if n != 1 {
			t.Errorf("đơn %s xuất hiện %d lần", id, n)
		}
	}
}
