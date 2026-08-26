package app

import (
	"net/http"
	"sync"
	"testing"
)

// TestDocGioKhongGhiGi: nhiều lượt đọc giỏ SONG SONG đều phải trả 200.
//
// Đọc song song là chuyện bình thường — trình duyệt mở nhiều tab, một
// trang gọi vài API cùng lúc. Bài này chạy ngay sau khi đặt hàng, tức
// trạng thái đã gây ra PH-29.
//
// # GIỚI HẠN ĐÃ BIẾT, ghi ra để không ai đọc nhầm
//
// Bài này KHÔNG tái hiện được lỗi gốc: phá bản sửa (cho đường đọc tạo
// giỏ và ghi lại) thì nó VẪN XANH. Lỗi 500 cần một thời điểm hẹp mà
// mười goroutine trong cùng tiến trình không đánh trúng một cách đáng
// tin cậy.
//
// Bài chứng minh bản sửa là `TestDocGioKhongTaoGio` ngay dưới: nó kiểm
// THẲNG vào nguyên nhân — đường đọc không được tạo dữ liệu — thay vì
// kiểm một triệu chứng phụ thuộc thời điểm.
//
// Giữ lại vì nó vẫn chặn được hồi quy dạng khác: một thay đổi làm đường
// đọc trả 500 dưới tải sẽ đỏ ở đây.
func TestDocGioKhongGhiGi(t *testing.T) {
	a := newAPITest(t)

	// Dựng đúng trạng thái đã gây lỗi: giỏ vừa chuyển thành đơn.
	datMotDon(t, a)

	// Mười lượt đọc SONG SONG cho cùng một khách.
	//
	// Đọc song song là chuyện bình thường: trình duyệt mở nhiều tab, hoặc
	// một trang gọi vài API cùng lúc. Nếu đường đọc còn ghi, chúng tranh
	// chấp và một số lượt trả 500.
	const songSong = 10
	var start, done sync.WaitGroup
	start.Add(1)

	var mu sync.Mutex
	ma := map[int]int{}

	for i := 0; i < songSong; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
			mu.Lock()
			ma[res.code]++
			mu.Unlock()
		}()
	}
	start.Done()
	done.Wait()

	if ma[http.StatusOK] != songSong {
		t.Errorf("mã trạng thái: %v — cần %d lượt 200", ma, songSong)
	}
}

// TestDocGioKhongTaoGio: khách chưa thêm món nào thì KHÔNG có giỏ nào
// được tạo ra.
//
// Trả 200 kèm giỏ RỖNG chứ không phải 404: "chưa có giỏ" là trạng thái
// bình thường của mọi khách mới, và giao diện không nên phải xử lý một mã
// lỗi cho tình huống thường gặp nhất.
func TestDocGioKhongTaoGio(t *testing.T) {
	a := newAPITest(t)

	res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("HTTP %d, cần 200 — %s", res.code, res.raw)
	}

	gio, _ := res.body["cart"].(map[string]any)
	if gio == nil {
		t.Fatalf("response không có `cart`: %s", res.raw)
	}

	// KHÔNG có định danh: chưa có giỏ nào để đặt tên.
	//
	// Bịa một mã ra sẽ khiến giao diện tưởng nó gọi được các đường sửa
	// giỏ bằng mã đó, và nhận 404 ở lần thử đầu tiên.
	if id, co := gio["id"]; co && id != nil && id != "" {
		t.Errorf("giỏ chưa tồn tại mà có định danh %v", id)
	}

	// `groups` là trường BẮT BUỘC của đặc tả: trả null thay vì mảng rỗng
	// bắt client kiểm tra null trước mỗi vòng lặp.
	groups, ok := gio["groups"].([]any)
	if !ok {
		t.Fatalf("`groups` không phải mảng: %#v", gio["groups"])
	}
	if len(groups) != 0 {
		t.Errorf("giỏ rỗng có %d nhóm, cần 0", len(groups))
	}
}

// datMotDon đi hết luồng mua hàng qua HTTP, để lại một giỏ ĐÃ CHUYỂN THÀNH
// ĐƠN — trạng thái đã gây ra PH-29.
func datMotDon(t *testing.T, a *apiTest) {
	t.Helper()

	res := a.call(http.MethodGet, "/api/v1/products?limit=1", nil, nil)
	ds, _ := res.body["data"].([]any)
	if len(ds) == 0 {
		t.Skip("danh mục trống")
	}
	sp, _ := ds[0].(map[string]any)
	maSP, _ := sp["id"].(string)

	res = a.call(http.MethodGet, "/api/v1/products/"+maSP+"/offers", nil, nil)
	offers, _ := res.body["data"].([]any)
	var maOffer string
	for _, o := range offers {
		m, _ := o.(map[string]any)
		if ban, _ := m["is_sellable"].(bool); ban {
			maOffer, _ = m["id"].(string)
			break
		}
	}
	if maOffer == "" {
		t.Skip("không có offer nào bán được")
	}

	res = a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": maOffer, "quantity": 1}, khoaIdem())
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	gio, _ := res.body["cart"].(map[string]any)
	maGio, _ := gio["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id":     maGio,
		"guest_email": "ph29@example.com",
		"guest_phone": "0900555444",
	}, khoaIdem())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	// `id` nằm ở CẤP CAO NHẤT, không lồng trong `checkout` — khác với
	// response của giỏ hàng. Đọc nhầm chỗ cho chuỗi rỗng, và đường dẫn
	// tiếp theo thành `/checkout//complete`, bị ServeMux trả 307.
	maPhien, _ := res.body["id"].(string)
	if maPhien == "" {
		t.Fatalf("mở phiên không trả id: %s", res.raw)
	}

	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách PH29", "phone": "0900555444",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, khoaIdem())
	a.call(http.MethodPatch, "/api/v1/checkout/"+maPhien+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, khoaIdem())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
}
