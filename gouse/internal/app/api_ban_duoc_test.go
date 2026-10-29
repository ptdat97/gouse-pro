package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	marketapp "github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	marketdom "github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
)

// TestBothViewsAgreeOnSellability khóa một bất biến LIÊN TẦNG:
// `is_sellable` phải là CÙNG một câu trả lời ở Seller Center và ở cửa hàng.
//
// # Vì sao cần bài test đi qua HTTP
//
// Quy tắc "khách bấm mua được không" có ba đầu vào thuộc ba module khác
// nhau — trạng thái offer, tồn kho, trạng thái nhà bán — nên mỗi tầng cần
// trả lời đều bị cám dỗ tự ghép lấy phần mình có. Đã xảy ra đúng vậy:
// tầng HTTP của nhà bán tự suy `status == ACTIVE`, tức bỏ qua CẢ HAI đầu
// vào còn lại, trong khi đặc tả của chính trường đó dặn "đừng suy ra từ
// status". Test ở tầng domain không bắt được, vì chỗ sai nằm ngoài domain.
//
// # Hậu quả trước khi sửa
//
// Nhà bán thấy `is_sellable: true` cho món khách không mua được. Seller
// Center chưa bao giờ có tín hiệu "hết hàng" hay "tài khoản đang bị đình
// chỉ" — không phải vì ai đó tắt nó đi, mà vì câu trả lời chưa bao giờ
// tính tới hai điều đó.
func TestBothViewsAgreeOnSellability(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	nb := buildSeller(t, a, "nhabanbanduoc")
	skuID := buildOpenBrandSKU(t, a)

	priceObj, _ := money.New(259_000, money.VND)
	offer, err := a.mods.marketplace.Service().CreateOffer(ctx, marketapp.CreateOfferInput{
		SKUID: ids.ID(skuID), SellerID: ids.ID(nb.sellerID), Price: priceObj,
		Condition: marketdom.ConditionNew, HandlingTimeHours: 24,
		MinOrderQuantity: 1, Activate: true,
	})
	if err != nil {
		t.Fatalf("tạo offer: %v", err)
	}

	// Sản phẩm chứa SKU này, để hỏi CỬA HÀNG về cùng một offer.
	sp, err := a.mods.product.GetProductsBySKUIDs(ctx, []string{skuID})
	if err != nil || len(sp) == 0 {
		t.Fatalf("tra sản phẩm của SKU: %v", err)
	}
	productID := sp[skuID].ID

	filtered, err := a.mods.inventory.EnsureLocation(ctx, "Kho bán được", "SELLER-"+nb.sellerID, "SELLER")
	if err != nil {
		t.Fatalf("tạo kho: %v", err)
	}
	// setStock ĐẶT số khả dụng về đúng một con số, không cộng dồn: bài
	// test cần biết chính xác đang còn bao nhiêu ở mỗi bước.
	var itemID string
	setStock := func(t *testing.T, qty int) {
		t.Helper()
		if itemID == "" {
			it, err := a.mods.inventory.Receive(ctx, inventory.ReceiveRequest{
				SKUID: skuID, LocationID: filtered,
				OwnerID:  inventory.OwnerForSeller(nb.sellerID, false),
				Quantity: qty, PerformedBy: "test",
			})
			if err != nil {
				t.Fatalf("nhập kho: %v", err)
			}
			itemID = it.ID
			return
		}
		if err := a.mods.inventory.SetAvailable(ctx, itemID, qty,
			"dat ton kho cho bai kiem tra tin hieu ban duoc", "test"); err != nil {
			t.Fatalf("đặt tồn kho = %d: %v", qty, err)
		}
	}

	// fromSellerView đọc is_sellable từ GET /api/v1/seller/offers.
	fromSellerView := func(t *testing.T) bool {
		t.Helper()
		res := a.call(http.MethodGet, "/api/v1/seller/offers", nil,
			map[string]string{"Authorization": "Bearer " + nb.token})
		if res.code != http.StatusOK {
			t.Fatalf("GET seller/offers: HTTP %d — %s", res.code, res.raw)
		}
		ds, _ := res.body["data"].([]any)
		for _, x := range ds {
			m, _ := x.(map[string]any)
			if id, _ := m["id"].(string); id != offer.ID().String() {
				continue
			}
			// `status` phải VẪN là ACTIVE: hết hàng không phải một trạng
			// thái của lời chào bán (P3-23). Nếu nó đổi, tín hiệu mà giao
			// diện nhà bán đọc là cặp (status, is_sellable) đã hỏng.
			if st, _ := m["status"].(string); st != "ACTIVE" {
				t.Fatalf("status = %q, cần ACTIVE — hết hàng không phải trạng thái offer", st)
			}
			shop2, _ := m["is_sellable"].(bool)
			return shop2
		}
		t.Fatalf("không thấy offer của mình trong danh sách: %s", res.raw)
		return false
	}

	// fromCustomerView đọc is_sellable từ trang sản phẩm công khai.
	fromCustomerView := func(t *testing.T) bool {
		t.Helper()
		res := a.call(http.MethodGet, "/api/v1/products/"+productID+"/offers", nil, nil)
		if res.code != http.StatusOK {
			t.Fatalf("GET products/%s/offers: HTTP %d — %s", productID, res.code, res.raw)
		}
		ds, _ := res.body["data"].([]any)
		for _, x := range ds {
			m, _ := x.(map[string]any)
			if id, _ := m["id"].(string); id != offer.ID().String() {
				continue
			}
			shop2, _ := m["is_sellable"].(bool)
			return shop2
		}
		// Offer KHÔNG hiện là một câu trả lời khác hẳn "hiện nhưng không
		// mua được", và đặc tả đòi hết hàng thì VẪN hiện.
		t.Fatalf("offer biến mất khỏi trang sản phẩm: %s", res.raw)
		return false
	}

	identical := func(t *testing.T, cases string, mong bool) {
		t.Helper()
		shop, customer := fromSellerView(t), fromCustomerView(t)
		if shop != customer {
			t.Errorf("%s: nhà bán thấy is_sellable=%v còn khách thấy %v — "+
				"cùng một câu hỏi, hai câu trả lời", cases, shop, customer)
		}
		if shop != mong {
			t.Errorf("%s: is_sellable=%v, mong %v", cases, shop, mong)
		}
	}

	t.Run("còn hàng, nhà bán hoạt động", func(t *testing.T) {
		setStock(t, 12)
		identical(t, "còn hàng", true)
	})

	t.Run("hết hàng", func(t *testing.T) {
		setStock(t, 0)
		identical(t, "hết hàng", false)
	})

	t.Run("nhà bán bị đình chỉ", func(t *testing.T) {
		setStock(t, 12) // kho đầy lại: chỉ còn đình chỉ là lý do

		if _, err := a.mods.seller.Service().Suspend(ctx, ids.ID(nb.sellerID),
			"kiem tra tin hieu dinh chi"); err != nil {
			t.Fatalf("đình chỉ nhà bán: %v", err)
		}
		identical(t, "nhà bán bị đình chỉ", false)
	})
}
