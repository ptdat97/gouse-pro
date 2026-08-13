package cart_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/cart/application"
	"github.com/fashion-commerce/platform/internal/modules/cart/domain"
	cartpg "github.com/fashion-commerce/platform/internal/modules/cart/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// fakeLookup thay cho bốn module cart phụ thuộc.
//
// Viết tay thay vì dựng cả bốn module thật vì port chỉ có MỘT phương thức,
// và bản giả cho phép mô tả rõ tình huống nghiệp vụ đang thử: "seller bị
// đình chỉ", "offer bị gỡ", "kho chỉ còn 3".
type fakeLookup struct {
	mu    sync.Mutex
	data  map[ids.ID]domain.SyncData
	calls int
}

func newFakeLookup() *fakeLookup {
	return &fakeLookup{data: map[ids.ID]domain.SyncData{}}
}

func (f *fakeLookup) LookupOffers(
	_ context.Context, offerIDs []ids.ID,
) (map[ids.ID]domain.SyncData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++

	out := map[ids.ID]domain.SyncData{}
	for _, id := range offerIDs {
		if d, ok := f.data[id]; ok {
			out[id] = d
		}
		// Không có trong data = offer đã bị gỡ, KHÔNG đưa vào map.
	}
	return out, nil
}

// set khai báo một offer đang bán bình thường.
func (f *fakeLookup) set(offerID ids.ID, price int64, available int) domain.SyncData {
	f.mu.Lock()
	defer f.mu.Unlock()

	d := domain.SyncData{
		OfferExists:       true,
		SellerActive:      true,
		IsSellable:        true,
		SKUID:             ids.MustNew(ids.PrefixSKU),
		SellerID:          ids.MustNew(ids.PrefixSeller),
		ProductName:       "Áo sơ mi linen Oxford",
		UnitPrice:         money.MustNew(price, money.VND),
		AvailableQuantity: available,
	}
	f.data[offerID] = d
	return d
}

func (f *fakeLookup) update(offerID ids.ID, mutate func(*domain.SyncData)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := f.data[offerID]
	mutate(&d)
	f.data[offerID] = d
}

func (f *fakeLookup) remove(offerID ids.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, offerID)
}

func newService(t *testing.T) (*application.Service, *fakeLookup, *pgxpool.Pool) {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần DATABASE_URL để chạy test với PostgreSQL thật")
	}

	db, err := database.Open(context.Background(), database.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("mở database: %v", err)
	}
	t.Cleanup(db.Close)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE cart_item CASCADE",
		"TRUNCATE cart CASCADE",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	lookup := newFakeLookup()
	svc := application.NewService(application.Deps{
		Carts:  cartpg.NewCartStore(db.Pool()),
		Offers: lookup,
	})
	return svc, lookup, db.Pool()
}

// storedPrice đọc THẲNG bảng cart_item, không đi qua service.
//
// Cần thiết vì GetCart tự đồng bộ trước khi trả: đọc qua service thì giá
// mới luôn xuất hiện, kể cả khi nó không hề được ghi xuống. Chỉ truy vấn
// trực tiếp mới phân biệt được "đã lưu" với "tính lại mỗi lần đọc".
//
// Khác biệt đó có hậu quả thật: job nhắc giỏ bỏ quên, phân tích, và tín
// hiệu nhu cầu đều đọc thẳng bảng này.
func storedPrice(t *testing.T, pool *pgxpool.Pool, cartID ids.ID) int64 {
	t.Helper()
	var price int64
	err := pool.QueryRow(context.Background(),
		`SELECT unit_price FROM cart_item WHERE cart_id = $1 LIMIT 1`,
		cartID.String()).Scan(&price)
	if err != nil {
		t.Fatalf("đọc giá đã lưu: %v", err)
	}
	return price
}

func newCart(t *testing.T, svc *application.Service) *domain.Cart {
	t.Helper()
	c, err := svc.GetOrCreateCart(context.Background(), application.GetOrCreateInput{
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		Currency:   money.VND,
	})
	if err != nil {
		t.Fatalf("GetOrCreateCart: %v", err)
	}
	return c
}

// GIỎ KHÔNG GIỮ TỒN KHO — kiểm chứng qua vòng ghi–đọc database.
//
// Khách thêm 10 món trong khi kho chỉ còn 3. Giỏ nhận, đánh dấu, và KHÔNG
// khóa gì cả. Nếu giỏ giữ hàng thì ba món kia bị khóa cho tới khi khách
// quay lại — có thể là không bao giờ.
func TestGioKhongKhoaHangDuQuaSoLuongTon(t *testing.T) {
	svc, lookup, _ := newService(t)
	ctx := context.Background()

	c := newCart(t, svc)
	offerID := ids.MustNew(ids.PrefixOffer)
	lookup.set(offerID, 299000, 3)

	got, err := svc.AddItem(ctx, application.AddItemInput{
		CartID: c.ID(), OfferID: offerID, Quantity: 10,
	})
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	item := got.Items()[0]
	if item.Quantity() != 10 {
		t.Errorf("số lượng = %d, mong giữ nguyên 10", item.Quantity())
	}
	if item.Availability() != domain.AvailabilityQuantityReduced {
		t.Errorf("tình trạng = %q, mong QUANTITY_REDUCED", item.Availability())
	}

	// Đọc lại từ database: dấu phải bền vững qua vòng ghi–đọc.
	reread, err := svc.GetCart(ctx, c.ID())
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if reread.Items()[0].Quantity() != 10 {
		t.Errorf("số lượng đọc lại = %d, mong 10", reread.Items()[0].Quantity())
	}
	if reread.Items()[0].AvailableQuantity() != 3 {
		t.Errorf("số hàng còn = %d, mong 3", reread.Items()[0].AvailableQuantity())
	}
}

// GIÁ CẬP NHẬT ĐỘNG qua vòng ghi–đọc.
//
// Đây là chỗ ĐỐI LẬP với order: cùng một vòng ghi–đọc, order phải ra con số
// CŨ còn cart phải ra con số MỚI.
func TestGiaTrongGioTheoGiaHienTaiQuaDatabase(t *testing.T) {
	svc, lookup, pool := newService(t)
	ctx := context.Background()

	c := newCart(t, svc)
	offerID := ids.MustNew(ids.PrefixOffer)
	lookup.set(offerID, 299000, 100)

	if _, err := svc.AddItem(ctx, application.AddItemInput{
		CartID: c.ID(), OfferID: offerID, Quantity: 2,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	// Seller giảm giá.
	lookup.update(offerID, func(d *domain.SyncData) {
		d.UnitPrice = money.MustNew(249000, money.VND)
	})

	got, err := svc.GetCart(ctx, c.ID())
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if got.Items()[0].UnitPrice().Amount() != 249000 {
		t.Errorf("đơn giá = %v, mong 249000 — giỏ phải theo giá hiện tại",
			got.Items()[0].UnitPrice())
	}
	if got.Subtotal().Amount() != 498000 {
		t.Errorf("tổng = %v, mong 498000", got.Subtotal())
	}

	// Giá mới phải được GHI XUỐNG DATABASE, không chỉ tính trong bộ nhớ.
	//
	// Đọc THẲNG bảng, không qua service: GetCart tự đồng bộ nên nó luôn
	// trả giá đúng dù có ghi hay không. Chỉ truy vấn trực tiếp mới phân
	// biệt được hai trường hợp.
	if got := storedPrice(t, pool, c.ID()); got != 249000 {
		t.Errorf("giá lưu trong database = %d, mong 249000 — job nhắc giỏ bỏ "+
			"quên và phân tích đọc thẳng bảng này, chúng sẽ thấy giá cũ mãi mãi", got)
	}
}

// MÓN KHÔNG HỢP LỆ CHỈ ĐÁNH DẤU, KHÔNG BỊ XÓA — qua database.
func TestMonKhongHopLeVanNamTrongDatabase(t *testing.T) {
	svc, lookup, _ := newService(t)
	ctx := context.Background()

	c := newCart(t, svc)
	gone := ids.MustNew(ids.PrefixOffer)
	suspended := ids.MustNew(ids.PrefixOffer)
	ok := ids.MustNew(ids.PrefixOffer)

	lookup.set(gone, 299000, 10)
	lookup.set(suspended, 450000, 10)
	lookup.set(ok, 199000, 10)

	for _, id := range []ids.ID{gone, suspended, ok} {
		if _, err := svc.AddItem(ctx, application.AddItemInput{
			CartID: c.ID(), OfferID: id, Quantity: 1,
		}); err != nil {
			t.Fatalf("AddItem: %v", err)
		}
	}

	// Offer bị gỡ; seller của món thứ hai bị đình chỉ.
	lookup.remove(gone)
	lookup.update(suspended, func(d *domain.SyncData) { d.SellerActive = false })

	got, err := svc.GetCart(ctx, c.ID())
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}

	// BA món vẫn còn trong giỏ — không có món nào bị xóa im lặng.
	if got.ItemCount() != 3 {
		t.Fatalf("số món = %d, mong 3 — không được tự xóa món", got.ItemCount())
	}
	if !got.HasUnavailableItems() {
		t.Error("giỏ phải báo có món cần khách xử lý")
	}
	// Chỉ món còn bán được tính vào tổng.
	if got.Subtotal().Amount() != 199000 {
		t.Errorf("tổng = %v, mong 199000 — chỉ tính món mua được", got.Subtotal())
	}
	if len(got.PurchasableItems()) != 1 {
		t.Errorf("số món mua được = %d, mong 1", len(got.PurchasableItems()))
	}
}

// QUY TẮC 5: MỘT KHÁCH CHỈ CÓ MỘT GIỎ ACTIVE, dưới tải song song.
//
// Khách mở 10 tab cùng lúc. Kiểm tra ở tầng ứng dụng không chặn được —
// mười request đều thấy "chưa có giỏ" rồi cùng tạo. Chỉ mục UNIQUE CÓ ĐIỀU
// KIỆN ở database là thứ chặn thật.
//
// Hai giỏ ACTIVE nghĩa là khách thêm hàng ở tab này rồi thanh toán ở tab
// kia mà không thấy nó.
func TestMuoiTabChiTaoMotGio(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	customerID := ids.MustNew(ids.PrefixCustomer)

	const n = 10
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		start   = make(chan struct{})
		cartIDs = map[ids.ID]int{}
		errs    []error
	)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			c, err := svc.GetOrCreateCart(ctx, application.GetOrCreateInput{
				CustomerID: customerID,
				Currency:   money.VND,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			cartIDs[c.ID()]++
		}()
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		t.Errorf("một request thất bại: %v", err)
	}
	if len(cartIDs) != 1 {
		t.Fatalf("số giỏ khác nhau = %d, mong 1 — khách có hai giỏ ACTIVE "+
			"sẽ thêm hàng ở tab này rồi thanh toán ở tab kia mà không thấy",
			len(cartIDs))
	}
}

// GỘP GIỎ KHI ĐĂNG NHẬP — tài khoản ĐÃ có giỏ.
func TestGopGioKhiDangNhapQuaDatabase(t *testing.T) {
	svc, lookup, _ := newService(t)
	ctx := context.Background()

	customerID := ids.MustNew(ids.PrefixCustomer)
	shared := ids.MustNew(ids.PrefixOffer)
	guestOnly := ids.MustNew(ids.PrefixOffer)
	lookup.set(shared, 299000, 100)
	lookup.set(guestOnly, 450000, 100)

	// Giỏ tài khoản, có sẵn một món.
	account, err := svc.GetOrCreateCart(ctx, application.GetOrCreateInput{
		CustomerID: customerID, Currency: money.VND,
	})
	if err != nil {
		t.Fatalf("tạo giỏ tài khoản: %v", err)
	}
	if _, err := svc.AddItem(ctx, application.AddItemInput{
		CartID: account.ID(), OfferID: shared, Quantity: 1,
	}); err != nil {
		t.Fatalf("AddItem giỏ tài khoản: %v", err)
	}

	// Giỏ vãng lai: một món trùng, một món riêng.
	guest, err := svc.GetOrCreateCart(ctx, application.GetOrCreateInput{
		SessionID: "phien-abc", Currency: money.VND,
	})
	if err != nil {
		t.Fatalf("tạo giỏ vãng lai: %v", err)
	}
	for _, id := range []ids.ID{shared, guestOnly} {
		if _, err := svc.AddItem(ctx, application.AddItemInput{
			CartID: guest.ID(), OfferID: id, Quantity: 1,
		}); err != nil {
			t.Fatalf("AddItem giỏ vãng lai: %v", err)
		}
	}

	res, err := svc.MergeOnLogin(ctx, customerID, "phien-abc")
	if err != nil {
		t.Fatalf("MergeOnLogin: %v", err)
	}

	if res.Cart.ID() != account.ID() {
		t.Errorf("gộp vào giỏ %s, mong giỏ tài khoản %s", res.Cart.ID(), account.ID())
	}
	if res.Cart.ItemCount() != 2 {
		t.Fatalf("số món sau gộp = %d, mong 2", res.Cart.ItemCount())
	}
	got, ok := res.Cart.ItemByOffer(shared)
	if !ok {
		t.Fatal("không tìm thấy món trùng sau khi gộp")
	}
	if got.Quantity() != 2 {
		t.Errorf("số lượng món trùng = %d, mong 2 (cộng dồn)", got.Quantity())
	}

	// Giỏ vãng lai đánh dấu MERGED, KHÔNG bị xóa — cần truy vết được nó
	// đã đi đâu.
	merged, err := svc.GetCart(ctx, guest.ID())
	if err != nil {
		t.Fatalf("đọc giỏ vãng lai: %v", err)
	}
	if merged.Status() != domain.StatusMerged {
		t.Errorf("trạng thái giỏ vãng lai = %q, mong MERGED", merged.Status())
	}

	// Và khách chỉ còn MỘT giỏ ACTIVE: nếu giỏ vãng lai vẫn ACTIVE thì
	// lần đăng nhập sau sẽ gộp lại lần nữa và nhân đôi số lượng.
	again, err := svc.GetOrCreateCart(ctx, application.GetOrCreateInput{
		CustomerID: customerID, Currency: money.VND,
	})
	if err != nil {
		t.Fatalf("GetOrCreateCart sau gộp: %v", err)
	}
	if again.ID() != account.ID() {
		t.Errorf("giỏ đang dùng = %s, mong %s", again.ID(), account.ID())
	}
}

// GỘP GIỎ khi tài khoản CHƯA có giỏ: chỉ ĐỔI CHỦ.
//
// Đây là đường đi phổ biến nhất — khách thêm hàng rồi mới đăng ký — và đổi
// chủ giữ nguyên mọi nguồn giới thiệu, thứ sẽ mất nếu tạo giỏ mới rồi chép
// món sang.
func TestDangNhapKhiChuaCoGioThiDoiChu(t *testing.T) {
	svc, lookup, _ := newService(t)
	ctx := context.Background()

	offerID := ids.MustNew(ids.PrefixOffer)
	lookup.set(offerID, 299000, 100)
	contentID := ids.MustNew(ids.PrefixContent)

	guest, err := svc.GetOrCreateCart(ctx, application.GetOrCreateInput{
		SessionID: "phien-moi", Currency: money.VND,
	})
	if err != nil {
		t.Fatalf("tạo giỏ vãng lai: %v", err)
	}
	if _, err := svc.AddItem(ctx, application.AddItemInput{
		CartID: guest.ID(), OfferID: offerID, Quantity: 1,
		SourceContentID: contentID,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	customerID := ids.MustNew(ids.PrefixCustomer)
	res, err := svc.MergeOnLogin(ctx, customerID, "phien-moi")
	if err != nil {
		t.Fatalf("MergeOnLogin: %v", err)
	}

	// CÙNG một giỏ, chỉ đổi chủ.
	if res.Cart.ID() != guest.ID() {
		t.Errorf("giỏ = %s, mong giữ nguyên %s — đổi chủ chứ không tạo mới",
			res.Cart.ID(), guest.ID())
	}
	if res.Cart.CustomerID() != customerID {
		t.Errorf("chủ giỏ = %s, mong %s", res.Cart.CustomerID(), customerID)
	}
	// Nguồn giới thiệu còn nguyên: đây là lý do chính để đổi chủ thay vì
	// chép món sang giỏ mới.
	if res.Cart.Items()[0].SourceContentID() != contentID {
		t.Errorf("nguồn nội dung = %q, mong %q — đổi chủ phải giữ nguyên quy kết",
			res.Cart.Items()[0].SourceContentID(), contentID)
	}
}

// ĐỒNG BỘ GIỎ 10 MÓN CHỈ MỘT LƯỢT TRA CỨU.
//
// cart.md mục 11: hiển thị giỏ 10 món cần dữ liệu từ bốn module. Gọi trong
// vòng lặp thì mỗi lần khách mở giỏ là hàng chục lượt gọi.
func TestDongBoGioMuoiMonChiMotLuotTraCuu(t *testing.T) {
	svc, lookup, _ := newService(t)
	ctx := context.Background()

	c := newCart(t, svc)
	for i := 0; i < 10; i++ {
		id := ids.MustNew(ids.PrefixOffer)
		lookup.set(id, 100000, 50)
		if _, err := svc.AddItem(ctx, application.AddItemInput{
			CartID: c.ID(), OfferID: id, Quantity: 1,
		}); err != nil {
			t.Fatalf("AddItem: %v", err)
		}
	}

	lookup.mu.Lock()
	lookup.calls = 0
	lookup.mu.Unlock()

	if _, err := svc.GetCart(ctx, c.ID()); err != nil {
		t.Fatalf("GetCart: %v", err)
	}

	lookup.mu.Lock()
	calls := lookup.calls
	lookup.mu.Unlock()

	if calls != 1 {
		t.Errorf("số lượt tra cứu = %d, mong 1 — giỏ 10 món mà gọi trong "+
			"vòng lặp thì mỗi lần mở giỏ là 10 lượt gọi qua bốn module", calls)
	}
}

// GIỎ ĐÃ THÀNH ĐƠN thì khóa lại, nhưng KHÔNG bị xóa khỏi database.
func TestGioDaThanhDonVanNamTrongDatabase(t *testing.T) {
	svc, lookup, _ := newService(t)
	ctx := context.Background()

	c := newCart(t, svc)
	offerID := ids.MustNew(ids.PrefixOffer)
	lookup.set(offerID, 299000, 10)
	if _, err := svc.AddItem(ctx, application.AddItemInput{
		CartID: c.ID(), OfferID: offerID, Quantity: 1,
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if err := svc.MarkConverted(ctx, c.ID()); err != nil {
		t.Fatalf("MarkConverted: %v", err)
	}

	got, err := svc.GetCart(ctx, c.ID())
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if got.Status() != domain.StatusConverted {
		t.Errorf("trạng thái = %q, mong CONVERTED", got.Status())
	}
	if got.ItemCount() != 1 {
		t.Errorf("số món = %d, mong 1 — giỏ đã chuyển đổi là dữ liệu phân tích",
			got.ItemCount())
	}

	// Không thêm được món vào giỏ đã chốt.
	if _, err := svc.AddItem(ctx, application.AddItemInput{
		CartID: c.ID(), OfferID: offerID, Quantity: 1,
	}); !errors.Is(err, domain.ErrCartNotActive) {
		t.Errorf("lỗi = %v, mong ErrCartNotActive", err)
	}

	// Và khách được cấp giỏ MỚI cho lần mua sau.
	fresh, err := svc.GetOrCreateCart(ctx, application.GetOrCreateInput{
		CustomerID: got.CustomerID(), Currency: money.VND,
	})
	if err != nil {
		t.Fatalf("GetOrCreateCart sau khi chốt đơn: %v", err)
	}
	if fresh.ID() == c.ID() {
		t.Error("phải cấp giỏ mới sau khi giỏ cũ đã thành đơn")
	}
}

// THÊM CÙNG OFFER HAI LẦN thì CỘNG DỒN — kiểm chứng cả ràng buộc UNIQUE
// (cart_id, offer_id) ở database.
func TestThemCungOfferHaiLanQuaDatabase(t *testing.T) {
	svc, lookup, _ := newService(t)
	ctx := context.Background()

	c := newCart(t, svc)
	offerID := ids.MustNew(ids.PrefixOffer)
	lookup.set(offerID, 299000, 100)

	for i := 0; i < 3; i++ {
		if _, err := svc.AddItem(ctx, application.AddItemInput{
			CartID: c.ID(), OfferID: offerID, Quantity: 1,
		}); err != nil {
			t.Fatalf("AddItem lần %d: %v", i+1, err)
		}
	}

	got, err := svc.GetCart(ctx, c.ID())
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if got.ItemCount() != 1 {
		t.Errorf("số dòng = %d, mong 1 — cùng offer phải cộng dồn", got.ItemCount())
	}
	if got.TotalQuantity() != 3 {
		t.Errorf("tổng số lượng = %d, mong 3", got.TotalQuantity())
	}
}
