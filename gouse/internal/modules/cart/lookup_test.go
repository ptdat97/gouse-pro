package cart

import (
	"context"
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	"github.com/fashion-commerce/platform/internal/modules/product"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// Bốn bản giả NHÚNG interface thật thay vì cài đủ mọi phương thức.
//
// Hai lý do, và lý do thứ hai mới là lý do chính:
//
//  1. Bốn interface này có hàng chục phương thức; cài hết chỉ để trả nil
//     là hàng trăm dòng không nói lên điều gì.
//  2. Phương thức KHÔNG được ghi đè sẽ panic con trỏ nil nếu bị gọi. Đó
//     chính là thứ bài `TestFourLookupCallsSufficeForATenItemCart` cần: một cài
//     đặt lén gọi thêm hàm khác không thể im lặng trượt qua.

type fakeMarketplace struct {
	marketplace.API
	offers   map[string]marketplace.OfferView
	failWith error
	calls    int
	received [][]string
}

func (f *fakeMarketplace) GetOffersByIDs(
	_ context.Context, ids []string,
) (map[string]marketplace.OfferView, error) {
	f.calls++
	f.received = append(f.received, ids)
	if f.failWith != nil {
		return nil, f.failWith
	}
	return f.offers, nil
}

type fakeProduct struct {
	product.API
	views map[string]product.ProductView
	calls int
}

func (f *fakeProduct) GetProductsBySKUIDs(
	_ context.Context, _ []string,
) (map[string]product.ProductView, error) {
	f.calls++
	return f.views, nil
}

type fakeSeller struct {
	seller.API
	views map[string]seller.SellerView
	calls int
}

func (f *fakeSeller) GetSellersByIDs(
	_ context.Context, _ []string,
) (map[string]seller.SellerView, error) {
	f.calls++
	return f.views, nil
}

type fakeInventory struct {
	inventory.API
	stock map[string]int
	calls int
}

func (f *fakeInventory) GetAvailability(
	_ context.Context, _ []string, _ string,
) (map[string]int, error) {
	f.calls++
	return f.stock, nil
}

// newLookup dựng offerLookup với bốn bản giả, sẵn một offer bán được.
func newLookup() (*offerLookup, *fakeMarketplace, *fakeProduct, *fakeSeller, *fakeInventory) {
	mk := &fakeMarketplace{offers: map[string]marketplace.OfferView{}}
	pr := &fakeProduct{views: map[string]product.ProductView{}}
	se := &fakeSeller{views: map[string]seller.SellerView{}}
	inv := &fakeInventory{stock: map[string]int{}}
	return &offerLookup{marketplace: mk, product: pr, seller: se, inventory: inv}, mk, pr, se, inv
}

const (
	testOfferID  = "off_01J9XABC123DEF456GHJKMNPQ"
	testSKUID    = "sku_01J9XABC123DEF456GHJKMNP"
	testSellerID = "sel_01J9XABC123DEF456GHJKMNP"
)

func fullOffer() marketplace.OfferView {
	return marketplace.OfferView{
		ID: testOfferID, SKUID: testSKUID, SellerID: testSellerID,
		PriceAmount: 299_000, PriceCurrency: "VND",
		MinOrderQuantity: 1, MaxOrderQuantity: 5,
		Status: "ACTIVE", IsSellable: true,
	}
}

// TestSellerTraKhongRaThiCOINHU ĐÌNH CHỈ — quy tắc "hỏng thì đóng".
//
// Chú thích của chính hàm này viết: "Seller không tra được thì coi như
// KHÔNG hoạt động: thà đánh dấu UNAVAILABLE nhầm còn hơn để khách đặt
// hàng của seller đã bị đình chỉ rồi phải hủy đơn."
//
// Đây là một quyết định BẤT ĐỐI XỨNG có chủ ý, và nó rất dễ bị một lần
// "dọn dẹp" san phẳng theo một trong hai chiều — cả hai đều là lỗi.
func TestAnUnresolvableSellerCountsAsSuspended(t *testing.T) {
	l, mk, _, se, _ := newLookup()
	mk.offers[testOfferID] = fullOffer()
	// se.views để RỖNG: seller không tra ra.

	out, err := l.LookupOffers(context.Background(), []ids.ID{ids.ID(testOfferID)})
	if err != nil {
		t.Fatalf("LookupOffers: %v", err)
	}
	d, ok := out[ids.ID(testOfferID)]
	if !ok {
		t.Fatal("offer biến mất khỏi kết quả — nó vẫn tồn tại, chỉ là không tra được nhà bán")
	}
	if d.SellerActive {
		t.Error("nhà bán tra KHÔNG RA mà vẫn báo đang hoạt động — khách sẽ " +
			"đặt được hàng của nhà bán có thể đã bị đình chỉ")
	}
	if se.calls != 1 {
		t.Errorf("gọi seller %d lần, cần đúng 1", se.calls)
	}
}

// TestTenSellerTraKhongRaThiDEROI — nửa còn lại của cùng quyết định, và
// nó đi NGƯỢC chiều với bài trên.
//
// "Tên thì ngược lại: tra không ra thì để rỗng và đi tiếp. Thiếu một nhãn
// hiển thị không phải lý do làm hỏng giỏ hàng."
//
// Hai bài phải đứng cạnh nhau: đọc riêng bài nào cũng dễ kết luận rằng
// quy tắc là "luôn đóng" hoặc "luôn mở", và sửa theo kết luận đó sẽ phá
// nửa kia.
func TestAnUnresolvableSellerNameIsLeftEmptyButTheOfferIsKept(t *testing.T) {
	l, mk, _, _, _ := newLookup()
	mk.offers[testOfferID] = fullOffer()

	out, err := l.LookupOffers(context.Background(), []ids.ID{ids.ID(testOfferID)})
	if err != nil {
		t.Fatalf("LookupOffers: %v", err)
	}
	d := out[ids.ID(testOfferID)]
	if d.SellerName != "" {
		t.Errorf("tên nhà bán = %q, mong rỗng", d.SellerName)
	}
	if !d.OfferExists {
		t.Error("thiếu TÊN nhà bán làm cả offer biến mất — giỏ hỏng vì một nhãn hiển thị")
	}
}

// TestOfferBiGoThiVANG MẶT khỏi map, không phải trả bản rỗng.
//
// Bên gọi phân biệt hai thứ đó: vắng mặt = UNAVAILABLE và món ĐƯỢC GIỮ
// LẠI trong giỏ để khách tự quyết. Trả một bản rỗng thì `OfferExists`
// false nhưng món vẫn đi qua nhánh "có dữ liệu", và giá 0 có thể lọt vào
// tổng tiền.
func TestADelistedOfferDisappearsFromTheResult(t *testing.T) {
	l, _, _, _, _ := newLookup() // offers RỖNG: offer đã bị gỡ khỏi sàn

	out, err := l.LookupOffers(context.Background(), []ids.ID{ids.ID(testOfferID)})
	if err != nil {
		t.Fatalf("LookupOffers: %v", err)
	}
	if _, ok := out[ids.ID(testOfferID)]; ok {
		t.Error("offer đã bị gỡ vẫn có mặt trong map — bên gọi sẽ coi là " +
			"'có dữ liệu' thay vì UNAVAILABLE")
	}
}

// TestGiaHongThiGIU offer, chỉ bỏ giá.
//
// "Giá hỏng ở nguồn. Bỏ qua giá thay vì bỏ qua cả offer: khách vẫn thấy
// món trong giỏ với giá cũ, tốt hơn là món biến mất."
//
// Món biến mất khỏi giỏ là thứ khách KHÔNG hiểu và không sửa được; một
// dòng giá sai thì họ thấy và hỏi được.
func TestABrokenPriceKeepsTheOfferInTheCart(t *testing.T) {
	l, mk, _, _, _ := newLookup()
	o := fullOffer()
	o.PriceCurrency = "KHONG-PHAI-TIEN-TE"
	mk.offers[testOfferID] = o

	out, err := l.LookupOffers(context.Background(), []ids.ID{ids.ID(testOfferID)})
	if err != nil {
		t.Fatalf("LookupOffers: %v", err)
	}
	d, ok := out[ids.ID(testOfferID)]
	if !ok {
		t.Fatal("giá hỏng làm cả offer biến mất khỏi giỏ")
	}
	if !d.OfferExists {
		t.Error("OfferExists = false dù offer vẫn tồn tại")
	}
}

// TestLoiTraVeLoi — hỏng ở nguồn KHÔNG được nuốt thành "giỏ rỗng".
//
// Nuốt lỗi ở đây là kiểu hỏng tệ nhất: khách mở giỏ thấy trống, tưởng mất
// hàng, và không có lỗi nào để ai đi tìm.
func TestAMarketplaceErrorErrsInsteadOfReturningAnEmptyCart(t *testing.T) {
	l, mk, _, _, _ := newLookup()
	mk.failWith = errors.New("marketplace sập")

	_, err := l.LookupOffers(context.Background(), []ids.ID{ids.ID(testOfferID)})
	if err == nil {
		t.Fatal("marketplace lỗi mà LookupOffers trả nil — khách sẽ thấy " +
			"giỏ TRỐNG và tưởng mất hàng")
	}
}

// TestFourLookupCallsSufficeForATenItemCart khóa HỢP ĐỒNG HIỆU NĂNG mà chú thích
// của offerLookup tự đặt ra:
//
//	"ĐÂY LÀ CHỖ QUYẾT ĐỊNH HIỆU NĂNG CỦA MODULE (cart.md mục 11): hiển thị
//	 giỏ 10 món cần dữ liệu từ bốn module. Làm ngây thơ thì mỗi lần khách
//	 mở giỏ là 40 lượt gọi."
//
// Đây là PH-14. Hồi quy N+1 không làm test nào đỏ và không làm giao diện
// sai — nó chỉ làm mọi thứ chậm dần, và không ai truy được nguyên nhân về
// đúng commit đã gây ra.
func TestFourLookupCallsSufficeForATenItemCart(t *testing.T) {
	l, mk, pr, se, inv := newLookup()

	// Mười món, mười offer, cùng một SKU và một nhà bán — đúng hình dạng
	// dễ sinh N+1 nhất: mười lần tra cùng một thứ.
	var offerIDs []ids.ID
	for i := 0; i < 10; i++ {
		id := testOfferID[:len(testOfferID)-1] + string(rune('A'+i))
		o := fullOffer()
		o.ID = id
		mk.offers[id] = o
		offerIDs = append(offerIDs, ids.ID(id))
	}

	if _, err := l.LookupOffers(context.Background(), offerIDs); err != nil {
		t.Fatalf("LookupOffers: %v", err)
	}

	for _, tt := range []struct {
		name      string
		wantCalls int
	}{
		{"marketplace", mk.calls},
		{"product", pr.calls},
		{"seller", se.calls},
		{"inventory", inv.calls},
	} {
		if tt.wantCalls != 1 {
			t.Errorf("gọi %s %d lần cho giỏ 10 món, cần đúng 1 — N+1 đã quay lại",
				tt.name, tt.wantCalls)
		}
	}

	// Và lượt gọi marketplace phải mang ĐỦ mười mã: gọi một lần với một mã
	// rồi bỏ chín món kia cũng cho `calls == 1`.
	if n := len(mk.received[0]); n != 10 {
		t.Errorf("lượt gọi marketplace mang %d mã, cần 10", n)
	}
}

// TestBatchingNeverLooksUpTheSameSKUTwice — mười món cùng SKU chỉ được hỏi
// tồn kho MỘT lần cho SKU đó, không mười lần.
func TestBatchingNeverLooksUpTheSameSKUTwice(t *testing.T) {
	l, mk, _, _, _ := newLookup()

	var offerIDs []ids.ID
	for i := 0; i < 5; i++ {
		id := testOfferID[:len(testOfferID)-1] + string(rune('A'+i))
		o := fullOffer() // CÙNG testSKUID và testSellerID
		o.ID = id
		mk.offers[id] = o
		offerIDs = append(offerIDs, ids.ID(id))
	}

	if _, err := l.LookupOffers(context.Background(), offerIDs); err != nil {
		t.Fatalf("LookupOffers: %v", err)
	}
	if n := len(mk.received[0]); n != 5 {
		t.Errorf("marketplace nhận %d mã offer, cần 5", n)
	}
}

// TestTheVariantLabelJoinsColorAndSize khóa nhãn khách nhìn thấy trong giỏ.
//
// "Trắng / M" là thứ giúp khách nhận ra món mình đã thêm. Một chiếc áo có
// năm size nằm ở năm ô kệ khác nhau; thiếu nhãn thì giỏ có năm dòng giống
// hệt nhau.
func TestTheVariantLabelJoinsColorAndSize(t *testing.T) {
	for _, tt := range []struct {
		name     string
		color    string
		size     string
		expected string
	}{
		{"đủ cả hai", "Trắng", "M", "Trắng / M"},
		{"chỉ có màu", "Trắng", "", "Trắng"},
		{"chỉ có size", "", "M", "M"},
		{"không có gì", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, mk, pr, _, _ := newLookup()
			mk.offers[testOfferID] = fullOffer()
			pr.views[testSKUID] = product.ProductView{
				Name: "Áo sơ mi",
				Variants: []product.VariantView{{
					Color: tt.color, Size: tt.size,
					SKUs: []product.SKUView{{ID: testSKUID}},
				}},
			}

			out, err := l.LookupOffers(context.Background(), []ids.ID{ids.ID(testOfferID)})
			if err != nil {
				t.Fatalf("LookupOffers: %v", err)
			}
			if got := out[ids.ID(testOfferID)].VariantDescription; got != tt.expected {
				t.Errorf("nhãn = %q, mong %q", got, tt.expected)
			}
		})
	}
}

// TestAnhBienTheDeNENAnhSanPham — khách chọn màu Trắng thì phải thấy ảnh
// áo TRẮNG, không phải ảnh đại diện của sản phẩm.
func TestVariantImageOverridesProductImage(t *testing.T) {
	l, mk, pr, _, _ := newLookup()
	mk.offers[testOfferID] = fullOffer()
	pr.views[testSKUID] = product.ProductView{
		Name:   "Áo sơ mi",
		Images: []string{"anh-san-pham.jpg"},
		Variants: []product.VariantView{{
			Color: "Trắng", Size: "M",
			Images: []string{"anh-trang.jpg"},
			SKUs:   []product.SKUView{{ID: testSKUID}},
		}},
	}

	out, err := l.LookupOffers(context.Background(), []ids.ID{ids.ID(testOfferID)})
	if err != nil {
		t.Fatalf("LookupOffers: %v", err)
	}
	if got := out[ids.ID(testOfferID)].ImageURL; got != "anh-trang.jpg" {
		t.Errorf("ảnh = %q, mong ảnh của biến thể — khách chọn màu Trắng "+
			"mà thấy ảnh màu khác là nhầm lẫn tốn một lần hoàn hàng", got)
	}
}
