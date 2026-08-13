package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/cart/domain"
)

var testNow = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money {
	m, err := money.New(n, money.VND)
	if err != nil {
		panic(err)
	}
	return m
}

func newCart(t *testing.T) *domain.Cart {
	t.Helper()
	c, err := domain.NewCart(domain.NewCartParams{
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		Currency:   money.VND,
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewCart: %v", err)
	}
	return c
}

func itemParams(price int64, qty int) domain.NewItemParams {
	return domain.NewItemParams{
		OfferID:     ids.MustNew(ids.PrefixOffer),
		SKUID:       ids.MustNew(ids.PrefixSKU),
		SellerID:    ids.MustNew(ids.PrefixSeller),
		ProductName: "Áo sơ mi linen",
		UnitPrice:   vnd(price),
		Quantity:    qty,
		Now:         testNow,
	}
}

// QUY TẮC 1: GIỎ HÀNG KHÔNG GIỮ TỒN KHO.
//
// Đây là quy tắc quan trọng nhất của module, và cách kiểm chứng nó ở tầng
// domain là chứng minh module KHÔNG CÓ ĐƯỜNG NÀO để giữ hàng: không có
// phương thức nào nhận reservation, không có trường nào lưu nó.
//
// Nếu giỏ giữ hàng: khách thêm rồi bỏ quên 2 tuần → hàng khóa 2 tuần. Với
// hàng khan hiếm, vài trăm giỏ bỏ quên = hết hàng ảo, không bán được cho
// khách thật sự muốn mua.
//
// Test này khóa lại điều đó bằng cách kiểm tra availableQuantity chỉ là
// THÔNG TIN THAM KHẢO: nó không ngăn khách thêm nhiều hơn số hàng còn.
func TestGioHangKhongGiuTonKho(t *testing.T) {
	c := newCart(t)

	// Kho chỉ còn 3, nhưng khách vẫn thêm được 10 vào giỏ.
	//
	// Đây KHÔNG phải lỗi mà là thiết kế: giỏ không giữ hàng nên không có
	// cơ sở gì để từ chối. Việc kiểm tra thật diễn ra ở checkout.
	item, err := c.AddItem(itemParams(299000, 10))
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	item.Sync(domain.SyncData{
		OfferExists:       true,
		SellerActive:      true,
		IsSellable:        true,
		UnitPrice:         vnd(299000),
		AvailableQuantity: 3,
	}, testNow)

	// Món vẫn ở trong giỏ với số lượng KHÁCH đã chọn.
	if item.Quantity() != 10 {
		t.Errorf("số lượng = %d, mong giữ nguyên 10 — giỏ không được tự "+
			"giảm số lượng thay khách", item.Quantity())
	}
	// Chỉ ĐÁNH DẤU để giao diện hiển thị "chỉ còn 3".
	if item.Availability() != domain.AvailabilityQuantityReduced {
		t.Errorf("tình trạng = %q, mong QUANTITY_REDUCED", item.Availability())
	}
	// Và món vẫn mua được — checkout sẽ là nơi chốt con số thật.
	if !item.Availability().IsPurchasable() {
		t.Error("món thiếu hàng vẫn phải mua được — checkout mới là nơi chốt")
	}
}

// QUY TẮC 2: GIÁ TRONG GIỎ LÀ GIÁ HIỆN TẠI, CẬP NHẬT ĐỘNG.
//
// ĐỐI LẬP với order, nơi mọi con số đóng băng. Lý do khác nhau: giỏ là Ý
// ĐỊNH mua, đơn là HỢP ĐỒNG.
//
// Giỏ hiện giá cũ sau khi seller giảm giá sẽ làm khách bỏ lỡ khuyến mãi;
// tệ hơn là hiện giá thấp rồi bị tính giá cao ở bước thanh toán.
func TestGiaTrongGioCapNhatDongKhongDongBang(t *testing.T) {
	c := newCart(t)

	item, err := c.AddItem(itemParams(299000, 2))
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if c.Subtotal().Amount() != 598000 {
		t.Fatalf("tổng ban đầu = %v, mong 598000", c.Subtotal())
	}

	// Seller giảm giá còn 249.000đ.
	item.Sync(domain.SyncData{
		OfferExists:       true,
		SellerActive:      true,
		IsSellable:        true,
		UnitPrice:         vnd(249000),
		AvailableQuantity: 100,
	}, testNow)

	// Giỏ phải theo giá MỚI — ngược hẳn với order.Line.
	if item.UnitPrice().Amount() != 249000 {
		t.Errorf("đơn giá = %v, mong 249000 — giỏ phải theo giá hiện tại",
			item.UnitPrice())
	}
	if c.Subtotal().Amount() != 498000 {
		t.Errorf("tổng sau khi seller giảm giá = %v, mong 498000", c.Subtotal())
	}
}

// QUY TẮC 6: KHÔNG TỰ ĐỘNG XÓA MÓN KHÔNG HỢP LỆ, CHỈ ĐÁNH DẤU.
//
// Xóa im lặng làm khách bối rối: họ nhớ đã thêm món đó, không hiểu vì sao
// nó biến mất, rồi nghi ngờ cả những món còn lại.
func TestMonKhongHopLeChiDanhDauKhongBiXoa(t *testing.T) {
	for _, tc := range []struct {
		ten  string
		sync domain.SyncData
		mong domain.ItemAvailability
	}{
		{
			ten:  "offer bị gỡ",
			sync: domain.SyncData{OfferExists: false, SellerActive: true},
			mong: domain.AvailabilityUnavailable,
		},
		{
			ten:  "seller bị đình chỉ",
			sync: domain.SyncData{OfferExists: true, SellerActive: false},
			mong: domain.AvailabilityUnavailable,
		},
		{
			ten: "hết hàng",
			sync: domain.SyncData{
				OfferExists: true, SellerActive: true,
				IsSellable: true, AvailableQuantity: 0,
			},
			mong: domain.AvailabilityOutOfStock,
		},
		{
			ten: "offer ngừng bán",
			sync: domain.SyncData{
				OfferExists: true, SellerActive: true,
				IsSellable: false, AvailableQuantity: 50,
			},
			mong: domain.AvailabilityOutOfStock,
		},
	} {
		t.Run(tc.ten, func(t *testing.T) {
			c := newCart(t)
			item, err := c.AddItem(itemParams(299000, 1))
			if err != nil {
				t.Fatalf("AddItem: %v", err)
			}

			item.Sync(tc.sync, testNow)

			if item.Availability() != tc.mong {
				t.Errorf("tình trạng = %q, mong %q", item.Availability(), tc.mong)
			}
			// Điều quan trọng nhất: món VẪN Ở TRONG GIỎ.
			if c.ItemCount() != 1 {
				t.Errorf("số món trong giỏ = %d, mong 1 — không được tự xóa",
					c.ItemCount())
			}
			// Nhưng không tính vào tổng tiền: hiện một con số bao gồm hàng
			// đã hết sẽ làm khách bất ngờ ở bước thanh toán.
			if c.Subtotal().Amount() != 0 {
				t.Errorf("tổng tiền = %v, mong 0 — món không mua được không "+
					"được tính vào tổng", c.Subtotal())
			}
			// Và không lọt vào danh sách checkout dùng.
			if len(c.PurchasableItems()) != 0 {
				t.Error("món không mua được không được lọt vào PurchasableItems")
			}
		})
	}
}

// THÊM CÙNG MỘT OFFER HAI LẦN thì CỘNG DỒN, không tạo hai dòng.
//
// Khách mong thấy "số lượng 2", không phải hai dòng giống hệt nhau.
func TestThemCungOfferThiCongDon(t *testing.T) {
	c := newCart(t)

	p := itemParams(299000, 1)
	if _, err := c.AddItem(p); err != nil {
		t.Fatalf("AddItem lần 1: %v", err)
	}
	if _, err := c.AddItem(p); err != nil {
		t.Fatalf("AddItem lần 2: %v", err)
	}

	if c.ItemCount() != 1 {
		t.Errorf("số dòng = %d, mong 1 — cùng offer phải cộng dồn", c.ItemCount())
	}
	if c.TotalQuantity() != 2 {
		t.Errorf("tổng số lượng = %d, mong 2", c.TotalQuantity())
	}
}

// QUY TẮC 4: tôn trọng min/max_order_quantity của offer.
//
// Vượt max thì BÁO LỖI chứ không tự cắt: khách chọn 10 mà giỏ im lặng để 5
// là hiểu nhầm sẽ lộ ra ở bước thanh toán.
func TestTonTrongGioiHanSoLuongCuaOffer(t *testing.T) {
	c := newCart(t)

	p := itemParams(299000, 6)
	p.MaxOrderQuantity = 5
	if _, err := c.AddItem(p); !errors.Is(err, domain.ErrQtyAboveMax) {
		t.Errorf("lỗi = %v, mong ErrQtyAboveMax", err)
	}

	p2 := itemParams(299000, 1)
	p2.MinOrderQuantity = 2
	if _, err := c.AddItem(p2); !errors.Is(err, domain.ErrQtyBelowMin) {
		t.Errorf("lỗi = %v, mong ErrQtyBelowMin", err)
	}

	// max = 0 nghĩa là KHÔNG giới hạn — cùng quy ước với marketplace.
	p3 := itemParams(299000, 999)
	p3.MaxOrderQuantity = 0
	if _, err := c.AddItem(p3); err != nil {
		t.Errorf("max = 0 phải là không giới hạn, nhưng lỗi: %v", err)
	}

	// Cộng dồn cũng phải kiểm tra giới hạn: thêm 3 rồi thêm 3 nữa với
	// max = 5 thì phải bị chặn, không được lọt vì mỗi lần đều hợp lệ.
	c2 := newCart(t)
	p4 := itemParams(299000, 3)
	p4.MaxOrderQuantity = 5
	if _, err := c2.AddItem(p4); err != nil {
		t.Fatalf("AddItem lần 1: %v", err)
	}
	if _, err := c2.AddItem(p4); !errors.Is(err, domain.ErrQtyAboveMax) {
		t.Errorf("lỗi = %v, mong ErrQtyAboveMax — cộng dồn phải kiểm tra "+
			"tổng, không phải từng lần", err)
	}
}

// GỘP GIỎ KHI ĐĂNG NHẬP (cart.md mục 6).
//
//	Món TRÙNG offer  → cộng số lượng
//	Món KHÔNG trùng  → thêm vào
//	Nguồn giới thiệu → giữ của lần thêm GẦN NHẤT
func TestGopGioKhiDangNhap(t *testing.T) {
	// Giỏ của tài khoản, đã có sẵn từ trước.
	account := newCart(t)
	shared := itemParams(299000, 1)
	if _, err := account.AddItem(shared); err != nil {
		t.Fatalf("AddItem giỏ tài khoản: %v", err)
	}

	// Giỏ vãng lai: một món TRÙNG, một món MỚI.
	guest, err := domain.NewCart(domain.NewCartParams{
		SessionID: "phien-abc",
		Currency:  money.VND,
		Now:       testNow,
	})
	if err != nil {
		t.Fatalf("NewCart vãng lai: %v", err)
	}
	if _, err := guest.AddItem(shared); err != nil {
		t.Fatalf("AddItem trùng: %v", err)
	}
	if _, err := guest.AddItem(itemParams(450000, 2)); err != nil {
		t.Fatalf("AddItem mới: %v", err)
	}

	warnings, err := account.MergeFrom(guest, testNow)
	if err != nil {
		t.Fatalf("MergeFrom: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("có %d cảnh báo, mong 0", len(warnings))
	}

	// Món trùng cộng dồn: 1 + 1 = 2. Món mới thêm vào.
	if account.ItemCount() != 2 {
		t.Errorf("số dòng sau gộp = %d, mong 2", account.ItemCount())
	}
	if got, _ := account.ItemByOffer(shared.OfferID); got.Quantity() != 2 {
		t.Errorf("số lượng món trùng = %d, mong 2", got.Quantity())
	}

	// Giỏ nguồn được đánh dấu đã gộp, KHÔNG bị xóa: cần truy vết được
	// giỏ vãng lai này đã đi đâu.
	if guest.Status() != domain.StatusMerged {
		t.Errorf("trạng thái giỏ vãng lai = %q, mong MERGED", guest.Status())
	}
}

// GỘP GIỎ VƯỢT GIỚI HẠN thì CẮT nhưng phải BÁO.
//
// Đây là chỗ DUY NHẤT hệ thống tự đổi số lượng của khách, và nó không được
// im lặng: khách đăng nhập xong thấy giỏ ít hàng hơn lúc chưa đăng nhập mà
// không hiểu vì sao là trải nghiệm tệ nhất của luồng này.
func TestGopGioVuotGioiHanThiCatNhungPhaiBao(t *testing.T) {
	p := itemParams(299000, 3)
	p.MaxOrderQuantity = 5

	account := newCart(t)
	if _, err := account.AddItem(p); err != nil {
		t.Fatalf("AddItem giỏ tài khoản: %v", err)
	}

	guest, err := domain.NewCart(domain.NewCartParams{
		SessionID: "phien-xyz",
		Currency:  money.VND,
		Now:       testNow,
	})
	if err != nil {
		t.Fatalf("NewCart: %v", err)
	}
	if _, err := guest.AddItem(p); err != nil {
		t.Fatalf("AddItem giỏ vãng lai: %v", err)
	}

	// 3 + 3 = 6, vượt max = 5.
	warnings, err := account.MergeFrom(guest, testNow)
	if err != nil {
		t.Fatalf("MergeFrom: %v", err)
	}

	if len(warnings) != 1 {
		t.Fatalf("số cảnh báo = %d, mong 1 — cắt số lượng mà không báo là "+
			"im lặng đổi lựa chọn của khách", len(warnings))
	}
	w := warnings[0]
	if w.Reason != domain.MergeQuantityCapped {
		t.Errorf("lý do = %q, mong QUANTITY_CAPPED", w.Reason)
	}
	if w.WantedQty != 6 || w.ActualQty != 5 {
		t.Errorf("cảnh báo báo %d → %d, mong 6 → 5", w.WantedQty, w.ActualQty)
	}

	got, _ := account.ItemByOffer(p.OfferID)
	if got.Quantity() != 5 {
		t.Errorf("số lượng sau gộp = %d, mong 5", got.Quantity())
	}
}

// GIỎ PHẢI BIẾT THUỘC VỀ AI: khách đã đăng nhập hoặc một phiên.
//
// Giỏ không có chủ là giỏ không tìm lại được ở phiên sau.
func TestGioPhaiCoChu(t *testing.T) {
	_, err := domain.NewCart(domain.NewCartParams{Currency: money.VND, Now: testNow})
	if !errors.Is(err, domain.ErrNoOwner) {
		t.Errorf("lỗi = %v, mong ErrNoOwner", err)
	}

	// Khách vãng lai chỉ cần sessionID.
	c, err := domain.NewCart(domain.NewCartParams{
		SessionID: "phien-1", Currency: money.VND, Now: testNow,
	})
	if err != nil {
		t.Fatalf("khách vãng lai phải tạo được giỏ: %v", err)
	}
	if !c.IsGuestCart() {
		t.Error("giỏ không có customerID phải là giỏ vãng lai")
	}
}

// THỜI GIAN SỐNG DÀI — khác hẳn checkout.
//
//	Cart     : 30 ngày   (không giữ hàng nên để lâu không hại ai)
//	Checkout : 15 phút   (giữ hàng nên phải nhả sớm)
func TestGioSongLauHonCheckoutRatNhieu(t *testing.T) {
	c := newCart(t)

	if got := c.ExpiresAt().Sub(testNow); got != domain.DefaultTTL {
		t.Errorf("thời gian sống = %v, mong %v", got, domain.DefaultTTL)
	}
	if domain.DefaultTTL < 24*time.Hour {
		t.Error("giỏ hàng phải sống nhiều ngày — giỏ sống lâu là thứ khách " +
			"quay lại và mua tiếp")
	}

	// Chưa hết hạn ở ngày thứ 29, hết hạn sau ngày thứ 30.
	if c.IsExpired(testNow.Add(29 * 24 * time.Hour)) {
		t.Error("giỏ không được hết hạn ở ngày thứ 29")
	}
	if !c.IsExpired(testNow.Add(31 * 24 * time.Hour)) {
		t.Error("giỏ phải hết hạn sau 30 ngày")
	}
}

// KHÔNG TRỘN NHIỀU ĐƠN VỊ TIỀN TỆ trong một giỏ.
//
// Cộng tiền khác đơn vị ra một con số vô nghĩa.
func TestKhongTronNhieuDonViTienTe(t *testing.T) {
	c := newCart(t)

	p := itemParams(299000, 1)
	usd, err := money.New(50, money.Currency("USD"))
	if err != nil {
		t.Skipf("USD không được hỗ trợ: %v", err)
	}
	p.UnitPrice = usd

	if _, err := c.AddItem(p); !errors.Is(err, domain.ErrMixedCurrency) {
		t.Errorf("lỗi = %v, mong ErrMixedCurrency", err)
	}
}

// GIỎ ĐÃ CHUYỂN ĐỔI thì KHÔNG sửa được nữa, và KHÔNG bị xóa.
//
// Giữ lại vì nó cho biết nội dung nào dẫn tới việc mua thật — dữ liệu đầu
// vào của bánh đà creator commerce.
func TestGioDaThanhDonThiKhoaLai(t *testing.T) {
	c := newCart(t)
	if _, err := c.AddItem(itemParams(299000, 1)); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if err := c.MarkConverted(testNow); err != nil {
		t.Fatalf("MarkConverted: %v", err)
	}

	if _, err := c.AddItem(itemParams(450000, 1)); !errors.Is(err, domain.ErrCartNotActive) {
		t.Errorf("lỗi = %v, mong ErrCartNotActive", err)
	}
	// Món cũ vẫn còn: giỏ đã chuyển đổi là dữ liệu phân tích.
	if c.ItemCount() != 1 {
		t.Errorf("số món = %d, mong 1 — giỏ đã chuyển đổi không bị xóa", c.ItemCount())
	}
}

// NGUỒN GIỚI THIỆU ghi ngay lúc THÊM GIỎ, không đợi lúc mua.
//
// Nhờ vậy đo được tỷ lệ "thêm giỏ" của từng nội dung — tín hiệu ý định mua
// mạnh hơn lượt xem nhiều — và quy kết đúng khi khách mua sau vài ngày.
func TestGhiNhanNguonGioiThieuTuLucThemGio(t *testing.T) {
	c := newCart(t)

	contentID := ids.MustNew(ids.PrefixContent)
	creatorID := ids.MustNew(ids.PrefixCreator)

	p := itemParams(299000, 1)
	p.SourceContentID = contentID
	p.SourceCreatorID = creatorID

	item, err := c.AddItem(p)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	if item.SourceContentID() != contentID {
		t.Errorf("nguồn nội dung = %q, mong %q", item.SourceContentID(), contentID)
	}
	if item.SourceCreatorID() != creatorID {
		t.Errorf("nguồn creator = %q, mong %q", item.SourceCreatorID(), creatorID)
	}

	// Thêm lại cùng offer từ nội dung KHÁC: giữ nguồn của lần GẦN NHẤT —
	// nội dung khiến khách quay lại thêm lần nữa là nội dung có công.
	newContent := ids.MustNew(ids.PrefixContent)
	p2 := p
	p2.SourceContentID = newContent
	p2.SourceCreatorID = ids.ID("")
	if _, err := c.AddItem(p2); err != nil {
		t.Fatalf("AddItem lần 2: %v", err)
	}
	if item.SourceContentID() != newContent {
		t.Errorf("nguồn nội dung = %q, mong cập nhật thành %q",
			item.SourceContentID(), newContent)
	}
}

// NHÓM THEO SELLER KHI HIỂN THỊ, nhưng giỏ không chia ở tầng dữ liệu.
//
// Khách cần hiểu hàng đến từ đâu và thời gian giao sẽ khác nhau. Việc chia
// thật diễn ra ở bước tạo FulfillmentOrder.
func TestNhomTheoSellerDeHienThi(t *testing.T) {
	c := newCart(t)

	sellerA := ids.MustNew(ids.PrefixSeller)
	sellerB := ids.MustNew(ids.PrefixSeller)

	for _, s := range []ids.ID{sellerA, sellerB, sellerA} {
		p := itemParams(299000, 1)
		p.SellerID = s
		if _, err := c.AddItem(p); err != nil {
			t.Fatalf("AddItem: %v", err)
		}
	}

	sellers := c.SellerIDs()
	if len(sellers) != 2 {
		t.Errorf("số seller = %d, mong 2 (không trùng lặp)", len(sellers))
	}
	// Ba dòng vẫn là ba dòng: giỏ KHÔNG gộp theo seller.
	if c.ItemCount() != 3 {
		t.Errorf("số dòng = %d, mong 3 — giỏ không chia theo seller", c.ItemCount())
	}
}

// XÓA MÓN là hành động của KHÁCH, và là đường DUY NHẤT món rời khỏi giỏ.
func TestChiKhachMoiXoaDuocMon(t *testing.T) {
	c := newCart(t)
	item, err := c.AddItem(itemParams(299000, 1))
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if err := c.RemoveItem(item.ID(), testNow); err != nil {
		t.Fatalf("RemoveItem: %v", err)
	}
	if c.ItemCount() != 0 {
		t.Errorf("số món = %d, mong 0", c.ItemCount())
	}

	// Xóa món không thuộc giỏ thì báo lỗi, không im lặng thành công.
	if err := c.RemoveItem(ids.MustNew(ids.PrefixCartItem), testNow); !errors.Is(err, domain.ErrItemNotInCart) {
		t.Errorf("lỗi = %v, mong ErrItemNotInCart", err)
	}
}
