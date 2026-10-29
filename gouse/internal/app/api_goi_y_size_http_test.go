package app

import (
	"context"
	"net/http"
	"testing"
)

// TestTheSizeSuggestionShowsUpOverHTTP.
//
// # Vì sao bài này tồn tại, khi đã có TestTheSizeSuggestionComesFromReturnHistory
//
// Bài kia kiểm QUY TẮC: cho quan sát vào, đòi gợi ý đúng ra. Nó xanh, và
// nó xanh cả khi trường `size_recommendation` không bao giờ tới được
// trình duyệt — vì nó gọi thẳng module, không đi qua HTTP.
//
// Chạy hệ thống thật trên Docker (16/09) cho thấy đúng chuyện đó: khách
// đã mua size M, mở lại trang sản phẩm cùng thương hiệu, response KHÔNG
// có `size_recommendation`. Tuyến `/api/v1/products/{id}` được đăng ký
// thẳng lên mux gốc, không qua `OptionalAuth` + `ResolveShopper…`, nên
// `ShopperFrom` trả rỗng và handler bỏ qua nhánh gợi ý với MỌI người gọi.
//
// Tính năng đúng từ domain lên tới module, và chết ở dây nối cuối cùng.
// Bài này khóa đúng dây nối đó: một request HTTP mang token, và một
// trường trong JSON.
func TestTheSizeSuggestionShowsUpOverHTTP(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.recommendation == nil {
		t.Skip("module gợi ý chưa được nối")
	}

	email := newEmail("goiysizehttp")
	tok := a.registerAndLogIn(email)
	bear := map[string]string{"Authorization": "Bearer " + tok}

	// Khách phải có hồ sơ mua hàng thì mới có `customer_id` để gắn quan
	// sát — lấy nó qua đúng đường mà handler dùng.
	var custID string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT id FROM customer WHERE lower(email) = lower($1)`, email).Scan(&custID); err != nil {
		t.Fatalf("đọc mã khách: %v", err)
	}

	// Tìm một sản phẩm CÓ size và lấy thương hiệu của nó: gợi ý tra theo
	// thương hiệu, nên quan sát phải thuộc đúng thương hiệu đang xem.
	var maSP, maTH, size string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT p.id, p.brand_id, v.attributes->>'size'
		FROM product p
		JOIN variant v ON v.product_id = p.id
		WHERE p.status = 'ACTIVE' AND v.attributes->>'size' <> ''
		LIMIT 1`).Scan(&maSP, &maTH, &size); err != nil {
		t.Skipf("không có sản phẩm nào bán theo size: %v", err)
	}

	if _, err := a.db.Pool().Exec(ctx, `
		INSERT INTO size_observation (customer_id, brand_id, size, outcome,
		                           source_type, source_id, observed_at)
		VALUES ($1,$2,$3,'PURCHASED','test','ord_test_goiy_http', now())`,
		custID, maTH, size); err != nil {
		t.Fatalf("ghi quan sát: %v", err)
	}

	res := a.call(http.MethodGet, "/api/v1/products/"+maSP, nil, bear)
	if res.code != http.StatusOK {
		t.Fatalf("đọc sản phẩm: HTTP %d — %s", res.code, res.raw)
	}

	suggestion, co := res.body["size_recommendation"].(map[string]any)
	if !co {
		t.Fatalf("response KHÔNG có size_recommendation dù khách đã mua size %s "+
			"của thương hiệu này — kiểm chuỗi middleware của tuyến sản phẩm: %s",
			size, res.raw)
	}
	if got, _ := suggestion["suggested_size"].(string); got != size {
		t.Errorf("gợi ý size %q, cần %q", got, size)
	}
	if got, _ := suggestion["reason"].(string); got != "PREVIOUS_PURCHASE" {
		t.Errorf("lý do %q, cần PREVIOUS_PURCHASE", got)
	}

	// KHÔNG đăng nhập thì KHÔNG gợi ý: trường này là dữ liệu riêng của
	// một người, không phải thuộc tính của sản phẩm.
	res = a.call(http.MethodGet, "/api/v1/products/"+maSP, nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("đọc sản phẩm (ẩn danh): HTTP %d — %s", res.code, res.raw)
	}
	if _, co := res.body["size_recommendation"]; co {
		t.Errorf("khách ẩn danh cũng nhận được gợi ý size: %s", res.raw)
	}
}
