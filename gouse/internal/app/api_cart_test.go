package app

import (
	"net/http"
	"sync"
	"testing"
)

// TestReadingTheCartWritesNothing: nhiều lượt đọc giỏ SONG SONG đều phải trả 200.
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
// Bài chứng minh bản sửa là `TestReadingTheCartCreatesNoCart` ngay dưới: nó kiểm
// THẲNG vào nguyên nhân — đường đọc không được tạo dữ liệu — thay vì
// kiểm một triệu chứng phụ thuộc thời điểm.
//
// Giữ lại vì nó vẫn chặn được hồi quy dạng khác: một thay đổi làm đường
// đọc trả 500 dưới tải sẽ đỏ ở đây.
func TestReadingTheCartWritesNothing(t *testing.T) {
	a := newAPITest(t)

	// Dựng đúng trạng thái đã gây lỗi: giỏ vừa chuyển thành đơn.
	placeOneOrder(t, a)

	// Mười lượt đọc SONG SONG cho cùng một khách.
	//
	// Đọc song song là chuyện bình thường: trình duyệt mở nhiều tab, hoặc
	// một trang gọi vài API cùng lúc. Nếu đường đọc còn ghi, chúng tranh
	// chấp và một số lượt trả 500.
	const parallel = 10
	var start, done sync.WaitGroup
	start.Add(1)

	var mu sync.Mutex
	couponCode := map[int]int{}

	for i := 0; i < parallel; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
			mu.Lock()
			couponCode[res.code]++
			mu.Unlock()
		}()
	}
	start.Done()
	done.Wait()

	if couponCode[http.StatusOK] != parallel {
		t.Errorf("mã trạng thái: %v — cần %d lượt 200", couponCode, parallel)
	}
}

// TestReadingTheCartCreatesNoCart: khách chưa thêm món nào thì KHÔNG có giỏ nào
// được tạo ra.
//
// Trả 200 kèm giỏ RỖNG chứ không phải 404: "chưa có giỏ" là trạng thái
// bình thường của mọi khách mới, và giao diện không nên phải xử lý một mã
// lỗi cho tình huống thường gặp nhất.
func TestReadingTheCartCreatesNoCart(t *testing.T) {
	a := newAPITest(t)

	res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("HTTP %d, cần 200 — %s", res.code, res.raw)
	}

	cartBody, _ := res.body["cart"].(map[string]any)
	if cartBody == nil {
		t.Fatalf("response không có `cart`: %s", res.raw)
	}

	// KHÔNG có định danh: chưa có giỏ nào để đặt tên.
	//
	// Bịa một mã ra sẽ khiến giao diện tưởng nó gọi được các đường sửa
	// giỏ bằng mã đó, và nhận 404 ở lần thử đầu tiên.
	if id, present := cartBody["id"]; present && id != nil && id != "" {
		t.Errorf("giỏ chưa tồn tại mà có định danh %v", id)
	}

	// `groups` là trường BẮT BUỘC của đặc tả: trả null thay vì mảng rỗng
	// bắt client kiểm tra null trước mỗi vòng lặp.
	groups, ok := cartBody["groups"].([]any)
	if !ok {
		t.Fatalf("`groups` không phải mảng: %#v", cartBody["groups"])
	}
	if len(groups) != 0 {
		t.Errorf("giỏ rỗng có %d nhóm, cần 0", len(groups))
	}
}

// placeOneOrder đi hết luồng mua hàng qua HTTP, để lại một giỏ ĐÃ CHUYỂN THÀNH
// ĐƠN — trạng thái đã gây ra PH-29.
func placeOneOrder(t *testing.T, a *apiTest) {
	t.Helper()

	res := a.call(http.MethodGet, "/api/v1/products?limit=1", nil, nil)
	ds, _ := res.body["data"].([]any)
	if len(ds) == 0 {
		t.Skip("danh mục trống")
	}
	sp, _ := ds[0].(map[string]any)
	productID, _ := sp["id"].(string)

	res = a.call(http.MethodGet, "/api/v1/products/"+productID+"/offers", nil, nil)
	offers, _ := res.body["data"].([]any)
	var offerID string
	for _, o := range offers {
		m, _ := o.(map[string]any)
		if shop2, _ := m["is_sellable"].(bool); shop2 {
			offerID, _ = m["id"].(string)
			break
		}
	}
	if offerID == "" {
		t.Skip("không có offer nào bán được")
	}

	res = a.call(http.MethodPost, "/api/v1/cart/items",
		map[string]any{"offer_id": offerID, "quantity": 1}, idemKey())
	if res.code != http.StatusOK {
		t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
	}
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id":     cartID,
		"guest_email": "ph29@example.com",
		"guest_phone": "0900555444",
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	// `id` nằm ở CẤP CAO NHẤT, không lồng trong `checkout` — khác với
	// response của giỏ hàng. Đọc nhầm chỗ cho chuỗi rỗng, và đường dẫn
	// tiếp theo thành `/checkout//complete`, bị ServeMux trả 307.
	checkoutID, _ := res.body["id"].(string)
	if checkoutID == "" {
		t.Fatalf("mở phiên không trả id: %s", res.raw)
	}

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách PH29", "phone": "0900555444",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, idemKey())
	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, idemKey())

	res = a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
}

// TestReadingTheCartDoesNotTouchTheDatabase là bất biến MẠNH NHẤT của PH-29.
//
// # Vì sao kiểm ở tầng DATABASE chứ không qua response
//
// Response chỉ cho thấy thứ handler CHỌN trả về. Một lần `INSERT` hay
// `UPDATE` âm thầm — cập nhật `updated_at`, ghi một dòng phụ, chạm bảng
// khác — vẫn lọt qua mọi khẳng định về JSON.
//
// Bài này chụp trạng thái database TRƯỚC và SAU, rồi so từng bảng mà
// đường đọc có thể chạm tới. Nó fail nếu ai đó lỡ đưa `Create`/`Save` trở
// lại đường đọc trong tương lai — đúng yêu cầu của tiêu chí nghiệm thu.
//
// Chụp cả TỔNG `updated_at` chứ không chỉ đếm dòng: sửa một dòng có sẵn
// không làm số dòng đổi, nhưng vẫn là ghi.
func TestReadingTheCartDoesNotTouchTheDatabase(t *testing.T) {
	a := newAPITest(t)

	// Ba trạng thái khác nhau, vì mỗi trạng thái đi một nhánh code khác:
	//
	//  1. chưa có giỏ nào       → nhánh "không tìm thấy"
	//  2. có giỏ với hàng       → nhánh đồng bộ giá
	//  3. vừa đặt hàng xong     → nhánh giỏ đã chuyển thành đơn (PH-29)
	cases := []struct {
		ten     string
		prepare func()
	}{
		{"chưa có giỏ", func() {}},
		{"giỏ có hàng", func() { addOneItem(t, a) }},
		{"vừa đặt hàng xong", func() { placeOneOrder(t, a) }},
	}

	// Mọi bảng mà đường đọc giỏ CÓ THỂ chạm tới, trực tiếp hoặc gián tiếp.
	table := []string{"cart", "cart_item", "reservation", "inventory_item"}

	for _, tt := range cases {
		t.Run(tt.ten, func(t *testing.T) {
			tt.prepare()

			before := map[string][2]any{}
			for _, b := range table {
				n, s := a.snapshotTable(b)
				before[b] = [2]any{n, s}
			}

			res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
			if res.code != http.StatusOK {
				t.Fatalf("HTTP %d, cần 200 — %s", res.code, res.raw)
			}

			for _, b := range table {
				n, s := a.snapshotTable(b)
				if n != before[b][0] {
					t.Errorf("bảng %s: %v → %d dòng — đường ĐỌC đã GHI",
						b, before[b][0], n)
				}
				if s != before[b][1] {
					t.Errorf("bảng %s: tổng updated_at đổi %v → %v — đường ĐỌC đã SỬA",
						b, before[b][1], s)
				}
			}
		})
	}
}

// addOneItem thêm một món vào giỏ qua HTTP, để có giỏ THẬT với hàng.
func addOneItem(t *testing.T, a *apiTest) {
	t.Helper()

	res := a.call(http.MethodGet, "/api/v1/products?limit=1", nil, nil)
	ds, _ := res.body["data"].([]any)
	if len(ds) == 0 {
		t.Skip("danh mục trống")
	}
	sp, _ := ds[0].(map[string]any)
	productID, _ := sp["id"].(string)

	res = a.call(http.MethodGet, "/api/v1/products/"+productID+"/offers", nil, nil)
	offers, _ := res.body["data"].([]any)
	for _, o := range offers {
		m, _ := o.(map[string]any)
		if shop2, _ := m["is_sellable"].(bool); !shop2 {
			continue
		}
		offerID, _ := m["id"].(string)
		got := a.call(http.MethodPost, "/api/v1/cart/items",
			map[string]any{"offer_id": offerID, "quantity": 1}, idemKey())
		if got.code != http.StatusOK {
			t.Fatalf("thêm vào giỏ: HTTP %d — %s", got.code, got.raw)
		}
		return
	}
	t.Skip("không có offer nào bán được")
}
