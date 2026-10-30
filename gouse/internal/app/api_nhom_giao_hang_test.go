package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	marketapp "github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	marketdom "github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
)

// TestShippingGroupsAreShownPerSeller.
//
// # Vì sao bài này tồn tại
//
// `Checkout.shipping_groups` có trong đặc tả từ đầu và KHÔNG có trong mã —
// tìm ra khi chạy hệ thống thật trên Docker (P3-50). Dữ liệu thì đã được
// tính sẵn: `fulfillment.EstimateShipping` trả phí và số ngày cho TỪNG
// nguồn, rồi adapter của checkout vứt tất cả trừ tổng.
//
// docs/04-modules/checkout.md mục 7 quyết định rõ: "hiển thị thời gian
// giao RIÊNG cho từng nhóm hàng, không gộp thành một con số. Khách cần
// biết món nào đến trước."
//
// Bài này kiểm đúng ba lời hứa của câu đó, trên một đơn trộn hàng HAI nhà
// bán:
//
//  1. có đúng hai nhóm, mỗi nhóm một nhà bán
//  2. hai nhóm có ngày giao KHÁC nhau khi thời gian chuẩn bị khác nhau
//  3. phí các nhóm cộng lại ĐÚNG BẰNG phí ship của đơn
//
// Lời hứa 3 là thứ dễ vỡ nhất: miễn phí ship đưa tổng về 0 trong khi ước
// tính từng nguồn vẫn dương, nên một bản dựng cẩu thả sẽ hiện bảng kê thu
// tiền bên dưới một dòng tổng ghi 0đ.
func TestShippingGroupsAreShownPerSeller(t *testing.T) {
	a := newAPITest(t)

	offerA, offerB := a.twoOffersDifferentSellers(t)
	if offerA == "" || offerB == "" {
		t.Skip("không có đủ hai nhà bán bán được hàng")
	}

	for _, off := range []string{offerA, offerB} {
		if res := a.call(http.MethodPost, "/api/v1/cart/items",
			map[string]any{"offer_id": off, "quantity": 1},
			idemKey()); res.code != http.StatusOK {
			t.Fatalf("thêm %s vào giỏ: HTTP %d — %s", off, res.code, res.raw)
		}
	}

	res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	if res.code != http.StatusOK {
		t.Fatalf("đọc giỏ: HTTP %d — %s", res.code, res.raw)
	}
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": cartID, "guest_email": newEmail("nhomgiao"),
		"guest_phone": "0900888777",
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	// CHƯA chọn cách giao thì chưa ước tính được gì, nên chưa có nhóm nào.
	if _, present := res.body["shipping_groups"]; present {
		t.Error("phiên chưa chọn cách giao mà đã có shipping_groups — " +
			"một bảng kê dựng từ số ngày chưa biết là một lời hứa bịa")
	}

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Thử", "phone": "0900888777",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, idemKey())

	res = a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, idemKey())
	if res.code != http.StatusOK {
		t.Fatalf("chọn cách giao: HTTP %d — %s", res.code, res.raw)
	}

	group, _ := res.body["shipping_groups"].([]any)
	if len(group) != 2 {
		t.Fatalf("đơn trộn hàng HAI nhà bán ra %d nhóm, cần 2 — "+
			"gộp lại là giấu mất chuyện đơn đi thành hai kiện: %s",
			len(group), res.raw)
	}

	var groupFeeTotal int64
	day := map[string]bool{}
	seller := map[string]bool{}
	for _, raw := range group {
		g, _ := raw.(map[string]any)

		s, _ := g["seller"].(map[string]any)
		id, _ := s["id"].(string)
		if id == "" {
			t.Error("nhóm không có mã nhà bán")
		}
		if ten, _ := s["name"].(string); ten == "" {
			t.Errorf("nhóm %s không có TÊN nhà bán — đặc tả khai `name` là "+
				"trường bắt buộc của SellerRef, và khách cần biết ai gửi "+
				"kiện nào", id)
		}
		seller[id] = true

		d, _ := g["estimated_delivery_date"].(string)
		if _, err := time.Parse(time.DateOnly, d); err != nil {
			t.Errorf("ngày giao dự kiến %q không đúng định dạng date: %v", d, err)
		}
		day[d] = true

		shippingFee, _ := g["shipping_fee"].(map[string]any)
		number, _ := shippingFee["amount"].(float64)
		groupFeeTotal += int64(number)
	}

	if len(seller) != 2 {
		t.Errorf("hai nhóm nhưng chỉ %d nhà bán khác nhau", len(seller))
	}

	sumTotal, _ := res.body["shipping_fee"].(map[string]any)
	totalFee, _ := sumTotal["amount"].(float64)
	if groupFeeTotal != int64(totalFee) {
		t.Errorf("phí các nhóm cộng lại %d, phí của đơn %d — bảng kê KHÔNG "+
			"cộng ra đúng số khách trả", groupFeeTotal, int64(totalFee))
	}

	// Ngày phải KHÁC nhau khi thời gian chuẩn bị khác nhau: đó là lý do
	// duy nhất khiến việc tách nhóm có nghĩa. Cùng phương thức giao thì
	// số ngày vận chuyển như nhau, nên nếu ngày trùng nghĩa là thời gian
	// chuẩn bị của nhà bán không đi vào phép tính.
	if len(day) != 2 {
		t.Errorf("hai nhà bán có thời gian chuẩn bị khác nhau nhưng ngày "+
			"giao dự kiến trùng nhau (%v) — trả lời 'món nào đến trước' "+
			"bằng 'tất cả cùng lúc'", day)
	}
}

// TestWithFreeShippingEveryGroupIsZero.
//
// Nửa dễ vỡ nhất của bảng kê: ngưỡng miễn phí đưa TỔNG về 0 trong khi ước
// tính từng nguồn vẫn dương. Bảng kê phải theo con số THỰC THU, không theo
// con số ước tính — nếu không, khách thấy hai dòng thu tiền nằm dưới một
// dòng tổng ghi 0đ.
func TestWithFreeShippingEveryGroupIsZero(t *testing.T) {
	a := newAPITest(t)

	offerA, offerB := a.twoOffersDifferentSellers(t)
	if offerA == "" || offerB == "" {
		t.Skip("không có đủ hai nhà bán bán được hàng")
	}

	// Số lượng lớn để tiền hàng vượt ngưỡng miễn phí ship.
	for _, off := range []string{offerA, offerB} {
		if res := a.call(http.MethodPost, "/api/v1/cart/items",
			map[string]any{"offer_id": off, "quantity": 5},
			idemKey()); res.code != http.StatusOK {
			t.Fatalf("thêm vào giỏ: HTTP %d — %s", res.code, res.raw)
		}
	}

	res := a.call(http.MethodGet, "/api/v1/cart", nil, nil)
	cartBody, _ := res.body["cart"].(map[string]any)
	cartID, _ := cartBody["id"].(string)

	res = a.call(http.MethodPost, "/api/v1/checkout", map[string]any{
		"cart_id": cartID, "guest_email": newEmail("mienphinhom"),
		"guest_phone": "0900888666",
	}, idemKey())
	if res.code != http.StatusCreated && res.code != http.StatusOK {
		t.Fatalf("mở phiên: HTTP %d — %s", res.code, res.raw)
	}
	checkoutID, _ := res.body["id"].(string)

	a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-address",
		map[string]any{
			"recipient_name": "Khách Thử", "phone": "0900888666",
			"street_address": "1 Đường Thử", "ward": "Phường 1",
			"district": "Quận 1", "province": "TP.HCM", "country_code": "VN",
		}, idemKey())

	res = a.call(http.MethodPatch, "/api/v1/checkout/"+checkoutID+"/shipping-method",
		map[string]any{"shipping_method": "STANDARD"}, idemKey())
	if res.code != http.StatusOK {
		t.Fatalf("chọn cách giao: HTTP %d — %s", res.code, res.raw)
	}

	sumTotal, _ := res.body["shipping_fee"].(map[string]any)
	totalFee, _ := sumTotal["amount"].(float64)
	if totalFee != 0 {
		t.Skipf("đơn chưa đạt ngưỡng miễn phí ship (phí %v) — "+
			"bài này không kiểm được nửa nó sinh ra để kiểm", totalFee)
	}

	group, _ := res.body["shipping_groups"].([]any)
	if len(group) == 0 {
		t.Fatalf("miễn phí ship mà mất luôn bảng kê: %s", res.raw)
	}
	for _, raw := range group {
		g, _ := raw.(map[string]any)
		shippingFee, _ := g["shipping_fee"].(map[string]any)
		if number, _ := shippingFee["amount"].(float64); number != 0 {
			t.Errorf("đơn được miễn phí ship nhưng một nhóm vẫn ghi %v đ — "+
				"bảng kê dựng từ con số ƯỚC TÍNH thay vì con số THỰC THU", number)
		}
	}
}

// twoOffersDifferentSellers dựng hai offer mua được của HAI nhà bán khác nhau,
// với thời gian chuẩn bị KHÁC nhau.
//
// Dựng chứ không đi tìm: khuôn database test chỉ có một nhà bán, nên bài
// nào chỉ `t.Skip` khi thiếu dữ liệu là bài không bao giờ chạy — xanh mà
// rỗng. Thời gian chuẩn bị phải khác nhau vì đó là thứ DUY NHẤT làm ngày
// giao của hai nhóm khác nhau: cùng một phiên thì mọi nhóm dùng chung
// phương thức giao, tức chung số ngày vận chuyển.
func (a *apiTest) twoOffersDifferentSellers(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()

	offerA := a.findSellableOffer()
	if offerA == "" {
		return "", ""
	}

	B := buildSeller(t, a, "nhomgiao"+ids.MustNew(ids.PrefixRequest).String()[24:])
	skuID := buildOpenBrandSKU(t, a)

	priceObj, _ := money.New(199_000, money.VND)
	offerB, err := a.mods.marketplace.Service().CreateOffer(ctx, marketapp.CreateOfferInput{
		SKUID: ids.ID(skuID), SellerID: ids.ID(B.sellerID), Price: priceObj,
		Condition: marketdom.ConditionNew,

		// 72 giờ, khác hẳn 24 giờ của nhà bán mặc định: ba ngày chuẩn bị
		// so với một ngày là chênh lệch KHÁCH NHÌN THẤY được.
		HandlingTimeHours: 72,
		MinOrderQuantity:  1, Activate: true,
	})
	if err != nil {
		t.Fatalf("tạo offer cho nhà bán thứ hai: %v", err)
	}

	filtered, err := a.mods.inventory.EnsureLocation(ctx, "Kho nhóm", "SELLER-"+B.sellerID, "SELLER")
	if err != nil {
		t.Fatalf("tạo kho: %v", err)
	}
	if _, err := a.mods.inventory.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: filtered,
		OwnerID:  inventory.OwnerForSeller(B.sellerID, false),
		Quantity: 20, PerformedBy: "test",
	}); err != nil {
		t.Fatalf("nhập kho: %v", err)
	}

	return offerA, offerB.ID().String()
}
