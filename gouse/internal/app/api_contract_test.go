package app

import (
	"net/http"
	"testing"
)

// TestResponseCoDuTruongDacTaHUA — loại lỗi thứ tư của ranh giới HTTP.
//
// # Vì sao TypeScript sinh từ đặc tả KHÔNG đủ
//
// `npm run types:check` chỉ khẳng định đặc tả và kiểu TypeScript khớp
// nhau. Nó KHÔNG biết Go thật sự trả gì. Lớp lỗi này đã xảy ra bốn lần
// trong tháng 8, và ba lần lọt qua TypeScript vì trường không `required`:
//
//	availability     đặc tả khai, Go không trả → nút mua khóa vĩnh viễn
//	updated_at       đặc tả khai, Go không trả → không ai phát hiện
//	Color · Size     đặc tả khai object, Go trả chuỗi → chặn lúc biên dịch
//	updateInventory  đặc tả PATCH, Go đăng ký PUT → client sinh ra ăn 405
//
// Chỉ có gọi API THẬT rồi soi thân trả về mới bắt được ba cái đầu.
//
// # Vì sao danh sách trường viết tay
//
// Đọc `required` từ file YAML cần thư viện phân tích YAML — một phụ thuộc
// mới cho tầng test. Viết tay có cái giá là phải sửa khi đặc tả đổi, và
// đó là cái giá ĐÚNG: đổi hợp đồng công khai nên là một hành động có ý
// thức, không phải thứ trôi qua im lặng.
func TestResponseCoDuTruongDacTaHua(t *testing.T) {
	a := newAPITest(t)

	// Lấy một sản phẩm có thật từ dữ liệu mẫu.
	res := a.call(http.MethodGet, "/api/v1/products", nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("danh sách sản phẩm: HTTP %d — %s", res.code, res.raw)
	}
	ds, _ := res.body["data"].([]any)
	if len(ds) == 0 {
		t.Fatal("dữ liệu mẫu không có sản phẩm nào — bài test không kiểm được gì")
	}
	sp, _ := ds[0].(map[string]any)
	maSP, _ := sp["id"].(string)

	t.Run("ProductSummary", func(t *testing.T) {
		// schemas.yaml#/ProductSummary — required: [id, name, brand]
		canCo(t, sp, "id", "name", "brand")

		// KHÔNG có giá, có chủ ý. Giá thuộc về offer và được tra riêng
		// qua `listBuyBoxPrices` — xem 2.10 trong backlog.
		//
		// Bài này từng đỏ với "thiếu trường price_from": đặc tả khai nó
		// BẮT BUỘC trong khi API chưa bao giờ trả, và cửa hàng hiện dấu
		// gạch thay cho giá suốt nhiều tuần.
		for _, k := range []string{"price_from", "compare_at_price"} {
			if _, co := sp[k]; co {
				t.Errorf("danh mục trả %q — giá phải tra qua listBuyBoxPrices", k)
			}
		}
	})

	t.Run("BuyBoxPrice", func(t *testing.T) {
		got := a.call(http.MethodGet,
			"/api/v1/offers/buy-box?product_ids="+maSP, nil, nil)
		if got.code != http.StatusOK {
			t.Fatalf("HTTP %d — %s", got.code, got.raw)
		}
		ds, _ := got.body["data"].([]any)
		if len(ds) == 0 {
			t.Skip("sản phẩm chưa có offer bán được")
		}
		g, _ := ds[0].(map[string]any)
		canCo(t, g, "product_id", "price_from")

		pf, _ := g["price_from"].(map[string]any)
		canCo(t, pf, "amount", "currency")
	})

	t.Run("ProductDetail", func(t *testing.T) {
		got := a.call(http.MethodGet, "/api/v1/products/"+maSP, nil, nil)
		if got.code != http.StatusOK {
			t.Fatalf("HTTP %d — %s", got.code, got.raw)
		}
		canCo(t, got.body, "id", "name", "variants")

		bt, _ := got.body["variants"].([]any)
		if len(bt) == 0 {
			t.Fatal("sản phẩm không có biến thể nào")
		}
		v, _ := bt[0].(map[string]any)

		// schemas.yaml#/Variant — required: [id, color, skus]
		canCo(t, v, "id", "color", "skus")

		// `color` là CHUỖI, không phải object. Đặc tả từng khai object và
		// đã được sửa về đúng sự thật (P3-22).
		if _, ok := v["color"].(string); !ok {
			t.Errorf("variant.color kiểu %T, cần chuỗi", v["color"])
		}

		skus, _ := v["skus"].([]any)
		if len(skus) == 0 {
			t.Fatal("biến thể không có SKU nào")
		}
		sku, _ := skus[0].(map[string]any)

		// schemas.yaml#/SKUSummary — required: [id, size]
		canCo(t, sku, "id", "size")
		if _, ok := sku["size"].(string); !ok {
			t.Errorf("sku.size kiểu %T, cần chuỗi", sku["size"])
		}

		// KHÔNG có buy box ở mức SẢN PHẨM, có chủ ý (P3-20).
		//
		// Buy box quyết theo SKU: áo có size S, M, L thì mỗi size là một
		// cuộc cạnh tranh riêng và người thắng có thể khác nhau. Trường ở
		// mức sản phẩm buộc máy chủ chọn bừa winner của một size rồi trình
		// bày như winner của cả sản phẩm.
		//
		// Ghim ở đây để lần sau ai đó "cài nốt trường đặc tả còn thiếu"
		// thì bài này đỏ, kèm lý do vì sao đừng cài.
		for _, k := range []string{"buy_box_offer", "other_offers_count"} {
			if _, co := got.body[k]; co {
				t.Errorf("chi tiết sản phẩm trả %q — buy box quyết theo SKU, "+
					"phải lấy qua /products/{id}/offers", k)
			}
		}
	})

	t.Run("Offer", func(t *testing.T) {
		got := a.call(http.MethodGet, "/api/v1/products/"+maSP+"/offers", nil, nil)
		if got.code != http.StatusOK {
			t.Fatalf("HTTP %d — %s", got.code, got.raw)
		}
		ds, _ := got.body["data"].([]any)
		if len(ds) == 0 {
			t.Skip("dữ liệu mẫu chưa có offer — bỏ qua")
		}
		o, _ := ds[0].(map[string]any)

		// schemas.yaml#/Offer — required: [id, seller_id, price]
		//
		// `is_sellable` KHÔNG required trong đặc tả nhưng luôn được trả,
		// và giao diện dùng nó để bật/tắt nút mua. Kiểm ở đây vì đúng lớp
		// lỗi này từng làm cửa hàng không bán được gì.
		canCo(t, o, "id", "sku_id", "seller_id", "price", "is_sellable")

		gia, _ := o["price"].(map[string]any)
		canCo(t, gia, "amount", "currency")
	})

	t.Run("SellerRef công khai", func(t *testing.T) {
		got := a.call(http.MethodGet,
			"/api/v1/sellers?ids="+maNhaBanDauTien(t, a), nil, nil)
		if got.code != http.StatusOK {
			t.Fatalf("HTTP %d — %s", got.code, got.raw)
		}
		ds, _ := got.body["data"].([]any)
		if len(ds) == 0 {
			t.Skip("không có nhà bán nào")
		}
		s, _ := ds[0].(map[string]any)
		canCo(t, s, "id", "name")

		// Và KHÔNG có gì ngoài ba trường công khai. Endpoint này không có
		// xác thực, nên mỗi trường thừa là một rò rỉ.
		choPhep := map[string]bool{"id": true, "name": true, "is_official": true}
		for k := range s {
			if !choPhep[k] {
				t.Errorf("trường %q lọt ra endpoint công khai không xác thực", k)
			}
		}
	})
}

// maNhaBanDauTien lấy mã nhà bán từ offer của dữ liệu mẫu.
func maNhaBanDauTien(t *testing.T, a *apiTest) string {
	t.Helper()
	res := a.call(http.MethodGet, "/api/v1/products", nil, nil)
	ds, _ := res.body["data"].([]any)
	if len(ds) == 0 {
		t.Skip("không có sản phẩm")
	}
	sp, _ := ds[0].(map[string]any)
	maSP, _ := sp["id"].(string)

	got := a.call(http.MethodGet, "/api/v1/products/"+maSP+"/offers", nil, nil)
	offers, _ := got.body["data"].([]any)
	if len(offers) == 0 {
		t.Skip("không có offer")
	}
	o, _ := offers[0].(map[string]any)
	id, _ := o["seller_id"].(string)
	return id
}

// canCo khẳng định mọi trường liệt kê đều CÓ MẶT và khác rỗng.
//
// Kiểm cả "có mặt" lẫn "khác rỗng": một trường trả về `""` hay `null` thì
// về mặt JSON là có, nhưng với bên gọi thì không khác gì thiếu.
func canCo(t *testing.T, obj map[string]any, truong ...string) {
	t.Helper()
	for _, k := range truong {
		v, ok := obj[k]
		if !ok {
			t.Errorf("thiếu trường %q — đặc tả khai là bắt buộc", k)
			continue
		}
		if v == nil || v == "" {
			t.Errorf("trường %q rỗng (%v) — có mặt nhưng vô dụng với bên gọi", k, v)
		}
	}
}

// TestKhongCoOfferBanDuocThiVANG_MAT, không phải giá 0.
//
// # Vì sao đây là bất biến chứ không phải chi tiết hiển thị
//
// `price_from = 0` hiện ra màn hình là "miễn phí". Một sản phẩm không ai
// bán được mà hiện giá 0 sẽ được bấm vào, thêm vào giỏ, và thất bại ở bước
// thanh toán — hoặc tệ hơn, bán được với giá 0 nếu có đường nào đó tin vào
// con số ấy.
//
// Trạng thái "không có ai bán" phải được nói RÕ bằng sự vắng mặt, để bên
// gọi buộc phải xử lý nó thay vì vô tình hiển thị một con số sai.
func TestKhongCoOfferBanDuocThiVangMat(t *testing.T) {
	a := newAPITest(t)

	// Sản phẩm vừa tạo, CHƯA ai chào bán.
	skuID := dungSkuThuongHieuMo(t, a)

	res := a.call(http.MethodGet, "/api/v1/products?limit=50", nil, nil)
	ds, _ := res.body["data"].([]any)

	var maSP string
	for _, x := range ds {
		p, _ := x.(map[string]any)
		id, _ := p["id"].(string)
		ct := a.call(http.MethodGet, "/api/v1/products/"+id, nil, nil)
		bt, _ := ct.body["variants"].([]any)
		for _, v := range bt {
			vm, _ := v.(map[string]any)
			skus, _ := vm["skus"].([]any)
			for _, s := range skus {
				sm, _ := s.(map[string]any)
				if sid, _ := sm["id"].(string); sid == skuID {
					maSP = id
				}
			}
		}
	}
	if maSP == "" {
		t.Skip("không tìm được sản phẩm vừa tạo trong danh mục")
	}

	got := a.call(http.MethodGet, "/api/v1/offers/buy-box?product_ids="+maSP, nil, nil)
	if got.code != http.StatusOK {
		t.Fatalf("HTTP %d — %s", got.code, got.raw)
	}

	rows, _ := got.body["data"].([]any)
	for _, r := range rows {
		m, _ := r.(map[string]any)
		if id, _ := m["product_id"].(string); id != maSP {
			continue
		}
		gia, _ := m["price_from"].(map[string]any)
		amount, _ := gia["amount"].(float64)
		t.Errorf("sản phẩm KHÔNG có offer bán được vẫn có giá %v — "+
			"phải VẮNG MẶT khỏi kết quả", amount)
	}

	// `data` là mảng RỖNG, không phải null: bên gọi không nên phải kiểm
	// null trước mỗi vòng lặp.
	if rows == nil {
		t.Error("`data` là null, cần mảng rỗng")
	}
}
