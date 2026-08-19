package marketplace_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
	marketpg "github.com/fashion-commerce/platform/internal/modules/marketplace/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

// Bốn bản giả cho bốn module marketplace phụ thuộc.
//
// Viết tay thay vì dựng cả bốn module thật: mỗi port chỉ có 1–2 phương
// thức, và bản giả cho phép mô tả rõ TÌNH HUỐNG NGHIỆP VỤ đang thử
// ("thương hiệu được bảo vệ, seller chưa có ủy quyền").

type fakeCatalog struct {
	allowed bool
	reason  string
	calls   int
}

func (f *fakeCatalog) CanSellerSellBrand(context.Context, ids.ID, ids.ID) (bool, string, error) {
	f.calls++
	return f.allowed, f.reason, nil
}

type fakeProduct struct {
	brandID  ids.ID
	found    bool
	sellable bool

	// skus là SKU của sản phẩm, cho ListProductOffers.
	skus []ids.ID
}

func (f *fakeProduct) BrandOfSKU(context.Context, ids.ID) (ids.ID, bool, error) {
	return f.brandID, f.found, nil
}
func (f *fakeProduct) IsSKUSellable(context.Context, ids.ID) (bool, error) {
	return f.sellable, nil
}
func (f *fakeProduct) SKUsOfProduct(context.Context, ids.ID) ([]ids.ID, error) {
	return f.skus, nil
}

type fakeSeller struct {
	active bool
	rate   int32
}

func (f *fakeSeller) IsActive(context.Context, ids.ID) (bool, error) { return f.active, nil }
func (f *fakeSeller) CommissionRate(context.Context, ids.ID) (types.BasisPoints, error) {
	return types.NewBasisPoints(f.rate)
}

type fakeInventory struct{ available map[ids.ID]int }

func (f *fakeInventory) AvailableForSKUs(_ context.Context, skuIDs []ids.ID) (map[ids.ID]int, error) {
	out := make(map[ids.ID]int, len(skuIDs))
	for _, id := range skuIDs {
		out[id] = f.available[id]
	}
	return out, nil
}

type harness struct {
	svc     *application.Service
	catalog *fakeCatalog
	product *fakeProduct
	seller  *fakeSeller
	inv     *fakeInventory
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	// offer_price_history có trigger chặn DELETE — dùng TRUNCATE.
	for _, stmt := range []string{
		"TRUNCATE offer_price_history CASCADE",
		"DELETE FROM offer",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	h := &harness{
		catalog: &fakeCatalog{allowed: true, reason: "OK"},
		product: &fakeProduct{brandID: ids.MustNew(ids.PrefixBrand), found: true, sellable: true},
		seller:  &fakeSeller{active: true, rate: 1000},
		inv:     &fakeInventory{available: map[ids.ID]int{}},
	}

	pool := db.Pool()
	h.svc = application.NewService(application.Deps{
		Offers:    marketpg.NewOfferStore(pool),
		History:   marketpg.NewPriceHistoryStore(pool),
		Catalog:   h.catalog,
		Product:   h.product,
		Seller:    h.seller,
		Inventory: h.inv,
	})
	return h
}

func (h *harness) createOffer(t *testing.T, skuID, sellerID ids.ID, price int64) *domain.Offer {
	t.Helper()
	h.inv.available[skuID] = 10

	o, err := h.svc.CreateOffer(context.Background(), application.CreateOfferInput{
		SKUID: skuID, SellerID: sellerID, Price: vnd(price), Activate: true,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	return o
}

// HÀNG RÀO CHỐNG HÀNG GIẢ (mục 5 của đặc tả).
//
// Rủi ro hàng giả là rủi ro SỐNG CÒN của marketplace thời trang. Seller
// không có ủy quyền KHÔNG được tạo offer cho thương hiệu được bảo vệ.
func TestSellerKhongCoUyQuyenKhongTaoDuocOffer(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	h.catalog.allowed = false
	h.catalog.reason = "NO_AUTHORIZATION"

	_, err := h.svc.CreateOffer(ctx, application.CreateOfferInput{
		SKUID:    ids.MustNew(ids.PrefixSKU),
		SellerID: ids.MustNew(ids.PrefixSeller),
		Price:    vnd(100000),
	})
	if !errors.Is(err, application.ErrNotAuthorized) {
		t.Fatalf("lỗi = %v, mong NotAuthorizedError", err)
	}

	// Lý do phải đi kèm để giao diện hiển thị hành động cụ thể
	// ("Tải lên giấy ủy quyền") thay vì thông báo chung chung.
	var authErr *application.NotAuthorizedError
	if !errors.As(err, &authErr) {
		t.Fatal("không lấy được chi tiết lỗi")
	}
	if authErr.Reason != "NO_AUTHORIZATION" {
		t.Errorf("lý do = %q, mong NO_AUTHORIZATION", authErr.Reason)
	}
}

// Kiểm tra LẠI quyền bán khi đưa offer lên bán.
//
// Giữa lúc tạo nháp và lúc đưa lên bán có thể đã nhiều ngày, và giấy ủy
// quyền có thể đã hết hạn.
func TestKiemTraLaiQuyenBanKhiDuaOfferLenBan(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	skuID := ids.MustNew(ids.PrefixSKU)

	o, err := h.svc.CreateOffer(ctx, application.CreateOfferInput{
		SKUID: skuID, SellerID: ids.MustNew(ids.PrefixSeller), Price: vnd(100000),
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	// Giấy ủy quyền hết hạn TRONG LÚC offer còn ở trạng thái nháp.
	h.catalog.allowed = false
	h.catalog.reason = "AUTHORIZATION_EXPIRED"

	if _, err := h.svc.ActivateOffer(ctx, o.ID()); !errors.Is(err, application.ErrNotAuthorized) {
		t.Fatalf("lỗi = %v, mong NotAuthorizedError", err)
	}

	// Và offer KHÔNG được lên bán.
	got, err := h.svc.GetOffer(ctx, o.ID())
	if err != nil {
		t.Fatalf("GetOffer: %v", err)
	}
	if got.IsSellable() {
		t.Error("offer hết hạn ủy quyền vẫn lên bán được")
	}
}

// Seller không hoạt động thì không tạo được offer.
func TestSellerKhongHoatDongKhongTaoDuocOffer(t *testing.T) {
	h := newHarness(t)
	h.seller.active = false

	_, err := h.svc.CreateOffer(context.Background(), application.CreateOfferInput{
		SKUID: ids.MustNew(ids.PrefixSKU), SellerID: ids.MustNew(ids.PrefixSeller),
		Price: vnd(100000),
	})
	if !errors.Is(err, application.ErrSellerInactive) {
		t.Errorf("lỗi = %v, mong ErrSellerInactive", err)
	}
}

// SKU đã ngừng kinh doanh thì không tạo được offer.
func TestSKUNgungKinhDoanhKhongTaoDuocOffer(t *testing.T) {
	h := newHarness(t)
	h.product.sellable = false

	_, err := h.svc.CreateOffer(context.Background(), application.CreateOfferInput{
		SKUID: ids.MustNew(ids.PrefixSKU), SellerID: ids.MustNew(ids.PrefixSeller),
		Price: vnd(100000),
	})
	if !errors.Is(err, application.ErrSKUNotSellable) {
		t.Errorf("lỗi = %v, mong ErrSKUNotSellable", err)
	}
}

// QUY TẮC 1: một seller chỉ có MỘT offer ACTIVE cho một SKU.
//
// Hai offer cùng lúc thì không biết giá nào là giá thật, và buy box chọn
// nhầm sẽ bán với giá seller không định.
func TestMotSellerChiMotOfferActiveChoMotSKU(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	skuID := ids.MustNew(ids.PrefixSKU)
	sellerID := ids.MustNew(ids.PrefixSeller)

	h.createOffer(t, skuID, sellerID, 100000)

	_, err := h.svc.CreateOffer(ctx, application.CreateOfferInput{
		SKUID: skuID, SellerID: sellerID, Price: vnd(200000), Activate: true,
	})
	if !errors.Is(err, domain.ErrDuplicateActiveOffer) {
		t.Errorf("lỗi = %v, mong ErrDuplicateActiveOffer", err)
	}

	// Nhưng seller KHÁC vẫn tạo được offer cho cùng SKU — đó là bản chất
	// của marketplace.
	if _, err := h.svc.CreateOffer(ctx, application.CreateOfferInput{
		SKUID: skuID, SellerID: ids.MustNew(ids.PrefixSeller),
		Price: vnd(150000), Activate: true,
	}); err != nil {
		t.Errorf("seller khác không tạo được offer cho cùng SKU: %v", err)
	}
}

// QUY TẮC 5: lưu lịch sử MỌI lần đổi giá.
//
// Cần cho việc phát hiện thao túng giá (tăng rồi giảm để giả vờ khuyến mãi).
func TestMoiLanDoiGiaDeuGhiLichSu(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	o := h.createOffer(t, ids.MustNew(ids.PrefixSKU), ids.MustNew(ids.PrefixSeller), 100000)

	for _, price := range []int64{120000, 90000} {
		if _, err := h.svc.UpdatePrice(ctx, o.ID(), vnd(price), money.Money{}, ""); err != nil {
			t.Fatalf("UpdatePrice %d: %v", price, err)
		}
	}

	history, err := h.svc.GetPriceHistory(ctx, o.ID(), 100)
	if err != nil {
		t.Fatalf("GetPriceHistory: %v", err)
	}
	// Ba điểm: lúc tạo + hai lần đổi.
	if len(history) != 3 {
		t.Errorf("số điểm lịch sử = %d, mong 3", len(history))
	}
}

// Buy box chỉ chọn offer CÒN HÀNG.
//
// Offer KHÔNG lưu số lượng — nguồn sự thật là inventory. Hiển thị "mua
// ngay" rồi báo hết hàng ở bước thanh toán là trải nghiệm tệ nhất.
func TestBuyBoxChiChonOfferConHang(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	skuID := ids.MustNew(ids.PrefixSKU)

	h.createOffer(t, skuID, ids.MustNew(ids.PrefixSeller), 100000)

	// Còn hàng → có buy box.
	h.inv.available[skuID] = 5
	got, err := h.svc.GetBuyBox(ctx, skuID)
	if err != nil {
		t.Fatalf("GetBuyBox: %v", err)
	}
	if got.Winner == nil {
		t.Fatal("còn hàng mà không có buy box")
	}

	// Hết hàng → KHÔNG buy box, dù offer vẫn ACTIVE.
	h.inv.available[skuID] = 0
	got, err = h.svc.GetBuyBox(ctx, skuID)
	if err != nil {
		t.Fatalf("GetBuyBox: %v", err)
	}
	if got.Winner != nil {
		t.Error("hết hàng mà vẫn có buy box — khách sẽ đặt rồi mới biết không có hàng")
	}
}

// Buy box theo LÔ phải cho cùng kết quả với tra từng cái.
//
// Nếu lệch, trang danh sách và trang chi tiết sẽ hiển thị giá khác nhau.
func TestBuyBoxTheoLoKhopVoiTraTungCai(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	sku1 := ids.MustNew(ids.PrefixSKU)
	sku2 := ids.MustNew(ids.PrefixSKU)

	// sku1 có hai offer cạnh tranh.
	h.createOffer(t, sku1, ids.MustNew(ids.PrefixSeller), 100000)
	h.createOffer(t, sku1, ids.MustNew(ids.PrefixSeller), 90000)
	h.createOffer(t, sku2, ids.MustNew(ids.PrefixSeller), 200000)

	batch, err := h.svc.GetBuyBoxes(ctx, []ids.ID{sku1, sku2})
	if err != nil {
		t.Fatalf("GetBuyBoxes: %v", err)
	}

	for _, skuID := range []ids.ID{sku1, sku2} {
		single, err := h.svc.GetBuyBox(ctx, skuID)
		if err != nil {
			t.Fatalf("GetBuyBox: %v", err)
		}
		if single.Winner == nil || batch[skuID].Winner == nil {
			t.Fatalf("SKU %s: thiếu buy box", skuID)
		}
		if single.Winner.ID() != batch[skuID].Winner.ID() {
			t.Errorf("SKU %s: lô chọn %s, đơn chọn %s",
				skuID, batch[skuID].Winner.ID(), single.Winner.ID())
		}
	}
}

// Đình chỉ seller → ẩn TOÀN BỘ offer của họ (quy tắc 4).
//
// LƯU Ý: việc này KHÔNG hủy đơn đang xử lý — đó là việc của module order.
func TestDinhChiSellerAnToanBoOffer(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sellerID := ids.MustNew(ids.PrefixSeller)

	for i := 0; i < 3; i++ {
		h.createOffer(t, ids.MustNew(ids.PrefixSKU), sellerID, 100000)
	}

	count, err := h.svc.SuspendOffersOfSeller(ctx, sellerID)
	if err != nil {
		t.Fatalf("SuspendOffersOfSeller: %v", err)
	}
	if count != 3 {
		t.Errorf("số offer bị ẩn = %d, mong 3", count)
	}

	offers, err := h.svc.GetOffersBySeller(ctx, sellerID, 100, 0)
	if err != nil {
		t.Fatalf("GetOffersBySeller: %v", err)
	}
	for _, o := range offers {
		if o.IsSellable() {
			t.Errorf("offer %s vẫn bán được sau khi seller bị đình chỉ", o.ID())
		}
	}
}

// BẢO MẬT: thiếu sellerID phải là LỖI, không được trả offer của mọi seller.
func TestThieuSellerIDTraLoiChuKhongTraToanBo(t *testing.T) {
	h := newHarness(t)
	h.createOffer(t, ids.MustNew(ids.PrefixSKU), ids.MustNew(ids.PrefixSeller), 100000)

	got, err := h.svc.GetOffersBySeller(context.Background(), "", 100, 0)
	if err == nil {
		t.Error("thiếu sellerID phải báo lỗi")
	}
	if len(got) != 0 {
		t.Errorf("trả về %d offer dù thiếu sellerID", len(got))
	}
}

// Seller chỉ thấy offer của MÌNH (quy tắc 7 của module seller).
func TestSellerChiThayOfferCuaMinh(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)
	h.createOffer(t, ids.MustNew(ids.PrefixSKU), sellerA, 100000)
	h.createOffer(t, ids.MustNew(ids.PrefixSKU), sellerB, 200000)

	got, err := h.svc.GetOffersBySeller(ctx, sellerA, 100, 0)
	if err != nil {
		t.Fatalf("GetOffersBySeller: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("số offer = %d, mong 1", len(got))
	}
	if got[0].SellerID() != sellerA {
		t.Error("lộ offer của seller khác")
	}
}

func TestChiHoTroPostgres(t *testing.T) {
	if _, err := marketplace.New(marketplace.Config{Storage: "memory"}); err == nil {
		t.Error("mong lỗi với kho lưu trữ memory")
	}
	// Thiếu module phụ thuộc thì hàng rào chống hàng giả không hoạt động —
	// thà không khởi động được còn hơn chạy với hàng rào đã tắt.
	if _, err := marketplace.New(marketplace.Config{Storage: "postgres"}); err == nil {
		t.Error("mong lỗi khi thiếu kết nối database")
	}
}

// SELLER KHÔNG SỬA ĐƯỢC OFFER CỦA SELLER KHÁC.
//
// Không có hàng rào này thì bất kỳ nhà bán nào cũng hạ giá offer của đối
// thủ về 1đ — hoặc lưu trữ nó — chỉ bằng cách đoán định danh. Mà định danh
// offer thì LỘ RA ở trang sản phẩm công khai.
func TestSellerKhongSuaDuocOfferCuaNguoiKhac(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	chuOffer := ids.MustNew(ids.PrefixSeller)
	keTomo := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)

	o := h.createOffer(t, skuID, chuOffer, 500_000)

	// Đọc có kiểm quyền: phải TỪ CHỐI.
	if _, err := h.svc.OwnedOffer(ctx, o.ID(), keTomo); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("OwnedOffer với seller khác: lỗi = %v, mong ErrNotFound", err)
	}

	// CHỦ offer thì đọc được.
	//
	// Hàng rào chặn nhầm người thật còn tệ hơn không có hàng rào: nó làm
	// seller không bán được hàng và không ai hiểu vì sao.
	if _, err := h.svc.OwnedOffer(ctx, o.ID(), chuOffer); err != nil {
		t.Errorf("chủ offer không đọc được offer của mình: %v", err)
	}

	// Offer KHÔNG TỒN TẠI trả CÙNG một lỗi với offer của người khác —
	// phân biệt hai trường hợp cho phép dò xem đối thủ đang bán những gì.
	khong := ids.MustNew(ids.PrefixOffer)
	if _, err := h.svc.OwnedOffer(ctx, khong, chuOffer); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("offer không tồn tại: lỗi = %v, mong ErrNotFound", err)
	}
}

// CHỦ OFFER VẪN sửa được offer của mình.
//
// Hàng rào chặn nhầm người thật còn tệ hơn không có hàng rào: nó làm seller
// không bán được hàng và không ai hiểu vì sao.
func TestChuOfferVanSuaDuocOfferCuaMinh(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller)
	o := h.createOffer(t, ids.MustNew(ids.PrefixSKU), sellerID, 500_000)

	updated, err := h.svc.UpdatePrice(ctx, o.ID(), vnd(450_000), vnd(0), sellerID)
	if err != nil {
		t.Fatalf("UpdatePrice: %v", err)
	}
	if updated.Price().Amount() != 450_000 {
		t.Errorf("giá = %d, mong 450000", updated.Price().Amount())
	}
}
