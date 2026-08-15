package checkout_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/modules/checkout/domain"
	checkoutpg "github.com/fashion-commerce/platform/internal/modules/checkout/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

// ---------------------------------------------------------------- Bản giả

// fakeCart thay module cart.
//
// Bản giả cho phép mô tả thẳng tình huống đang thử — "giỏ có ba món của ba
// seller, giá 299.000đ" — mà không phải dựng cả module cart và bốn module
// nó phụ thuộc.
type fakeCart struct {
	mu        sync.Mutex
	snap      application.CartSnapshot
	converted []ids.ID
	failMark  bool
}

func (f *fakeCart) LoadPurchasable(
	_ context.Context, cartID ids.ID,
) (application.CartSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	snap := f.snap
	snap.CartID = cartID
	return snap, nil
}

func (f *fakeCart) ActiveCartID(_ context.Context, _, _ string) (ids.ID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap.CartID, nil
}

func (f *fakeCart) MarkConverted(_ context.Context, cartID ids.ID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failMark {
		return errors.New("cart: giả lập lỗi đánh dấu chuyển đổi")
	}
	f.converted = append(f.converted, cartID)
	return nil
}

func (f *fakeCart) convertedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.converted)
}

// fakeClock cho phép tua thời gian.
//
// Cần thiết vì phiên sống 15 phút: chờ thật là 15 phút mỗi lần chạy test.
// Tua đồng hồ kiểm chứng đúng thứ cần kiểm chứng — hành vi khi hết hạn —
// mà không phải đợi.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// fakeCommission trả tỷ lệ hoa hồng cố định.
type fakeCommission struct{ rate int32 }

func (f *fakeCommission) RateForSeller(
	context.Context, ids.ID,
) (types.BasisPoints, error) {
	return types.NewBasisPoints(f.rate)
}

// realInventory nối tới module inventory THẬT chạy trên PostgreSQL.
//
// Dùng bản thật ở đây có chủ ý: điều cần kiểm chứng là hàng có bị khóa và
// nhả đúng không, mà cơ chế đó là khóa lạc quan ở database. Bản giả sẽ
// luôn "giữ hàng thành công" và test không chứng minh được gì.
type realInventory struct{ api inventory.API }

func (r *realInventory) FindItemsForSKUs(
	ctx context.Context, skuIDs []ids.ID,
) (map[ids.ID][]application.StockItem, error) {
	strs := make([]string, 0, len(skuIDs))
	for _, id := range skuIDs {
		strs = append(strs, id.String())
	}
	found, err := r.api.GetItemsBySKUs(ctx, strs, "")
	if err != nil {
		return nil, err
	}
	out := make(map[ids.ID][]application.StockItem, len(found))
	for skuID, items := range found {
		for _, it := range items {
			out[ids.ID(skuID)] = append(out[ids.ID(skuID)], application.StockItem{
				ItemID: ids.ID(it.ID), Available: it.Available,
			})
		}
	}
	return out, nil
}

func (r *realInventory) Reserve(
	ctx context.Context, itemID, checkoutID ids.ID, qty int, ttl time.Duration,
) (ids.ID, error) {
	res, err := r.api.Reserve(ctx, inventory.ReserveRequest{
		ItemID: itemID.String(), CheckoutID: checkoutID.String(),
		Quantity: qty, TTL: ttl,
	})
	if err != nil {
		return "", err
	}
	return ids.ID(res.ID), nil
}

func (r *realInventory) Release(ctx context.Context, id ids.ID) error {
	return r.api.ReleaseReservation(ctx, id.String())
}

func (r *realInventory) Extend(ctx context.Context, id ids.ID, d time.Duration) error {
	return r.api.ExtendReservation(ctx, id.String(), d)
}

// realOrder nối tới module order THẬT.
//
// Cũng dùng bản thật: điều cần kiểm chứng là hoàn tất hai lần chỉ ra một
// đơn, mà cơ chế đó là ràng buộc UNIQUE trên khóa idempotency ở database.
type realOrder struct{ api order.API }

func (r *realOrder) PlaceOrder(
	ctx context.Context, in application.PlaceOrderInput,
) (application.PlacedOrder, error) {
	lines := make([]order.PlaceOrderLineInput, 0, len(in.Lines))
	for _, l := range in.Lines {
		lines = append(lines, order.PlaceOrderLineInput{
			OfferID:     l.OfferID.String(),
			SKUID:       l.SKUID.String(),
			SellerID:    l.SellerID.String(),
			ProductName: l.ProductName,
			UnitPrice: order.Amount{
				Value: l.UnitPrice.Amount(), Currency: string(l.UnitPrice.Currency()),
			},
			Quantity:       l.Quantity,
			CommissionRate: int(l.CommissionRate.Value()),
		})
	}

	res, err := r.api.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: in.CustomerID.String(),
		GuestEmail: in.GuestEmail,
		ShippingAddress: order.AddressInput{
			RecipientName: in.ShippingAddress.RecipientName,
			Phone:         in.ShippingAddress.Phone,
			StreetAddress: in.ShippingAddress.StreetAddress,
			Province:      in.ShippingAddress.Province,
		},
		Currency:       string(in.Currency),
		ShippingFee:    order.Amount{Value: in.ShippingFee.Amount(), Currency: string(in.Currency)},
		DiscountAmount: order.Amount{Value: in.DiscountAmount.Amount(), Currency: string(in.Currency)},
		Lines:          lines,
		IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		return application.PlacedOrder{}, err
	}
	return application.PlacedOrder{
		OrderID:     ids.ID(res.Order.ID),
		OrderNumber: res.Order.OrderNumber,
		Replayed:    res.Replayed,
	}, nil
}

// ---------------------------------------------------------------- Bối cảnh

type harness struct {
	svc   *application.Service
	cart  *fakeCart
	inv   inventory.API
	ord   order.API
	db    *database.DB
	clock *fakeClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE checkout_line CASCADE",
		"TRUNCATE checkout CASCADE",
		"TRUNCATE fulfillment_order_line CASCADE",
		"TRUNCATE fulfillment_order CASCADE",
		"TRUNCATE order_line_adjustment CASCADE",
		"TRUNCATE order_line CASCADE",
		`TRUNCATE "order" CASCADE`,
		"TRUNCATE reservation CASCADE",
		"TRUNCATE inventory_movement CASCADE",
		"TRUNCATE inventory_item CASCADE",
		"DELETE FROM stock_location",
		"TRUNCATE event_processed",
		"TRUNCATE event_outbox",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu (%s): %v", stmt, err)
		}
	}

	invModule, err := inventory.New(inventory.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("inventory.New: %v", err)
	}
	ordModule, err := order.New(order.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("order.New: %v", err)
	}

	fc := &fakeCart{}
	clock := &fakeClock{now: time.Now().UTC()}
	svc := application.NewService(application.Deps{
		Checkouts:   checkoutpg.NewCheckoutStore(db.Pool()),
		Carts:       fc,
		Inventory:   &realInventory{api: invModule},
		Commissions: &fakeCommission{rate: 1000},
		Orders:      &realOrder{api: ordModule},
		Clock:       clock,
	})

	return &harness{
		svc: svc, cart: fc, inv: invModule, ord: ordModule,
		db: db, clock: clock,
	}
}

// stockSKU nhập hàng THẬT vào kho và trả mã SKU.
//
// Dùng inventory thật chứ không phải bản giả: điều cần kiểm chứng là hàng
// có bị khóa và nhả đúng không, mà cơ chế đó là khóa lạc quan ở database.
func (h *harness) stockSKU(t *testing.T, qty int) ids.ID {
	t.Helper()
	ctx := context.Background()
	skuID := ids.MustNew(ids.PrefixSKU)
	locID := ids.MustNew(ids.PrefixStockLocation)

	_, err := h.db.Pool().Exec(ctx, `
		INSERT INTO stock_location (id, name, code, kind, created_at, updated_at)
		VALUES ($1, 'Kho chính', $2, 'PLATFORM', now(), now())`,
		locID.String(), "KHO-"+string(locID[len(locID)-6:]))
	if err != nil {
		t.Fatalf("tạo địa điểm kho: %v", err)
	}

	_, err = h.inv.Receive(ctx, inventory.ReceiveRequest{
		SKUID:       skuID.String(),
		LocationID:  locID.String(),
		Quantity:    qty,
		PerformedBy: "test",
	})
	if err != nil {
		t.Fatalf("nhập kho: %v", err)
	}
	return skuID
}

// setCart khai báo nội dung giỏ mà checkout sẽ đọc.
func (h *harness) setCart(items ...application.CartItemSnapshot) {
	h.cart.mu.Lock()
	defer h.cart.mu.Unlock()
	h.cart.snap = application.CartSnapshot{
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		Currency:   money.VND,
		Items:      items,
	}
}

func item(skuID ids.ID, price int64, qty int) application.CartItemSnapshot {
	return application.CartItemSnapshot{
		CartItemID:  ids.MustNew(ids.PrefixCartItem),
		OfferID:     ids.MustNew(ids.PrefixOffer),
		SKUID:       skuID,
		SellerID:    ids.MustNew(ids.PrefixSeller),
		ProductName: "Áo sơ mi linen Oxford",
		UnitPrice:   vnd(price),
		Quantity:    qty,
	}
}

func testAddress() domain.Address {
	return domain.Address{
		RecipientName: "Nguyễn Văn A",
		Phone:         "0900000000",
		StreetAddress: "12 Lý Thường Kiệt",
		Province:      "Hà Nội",
	}
}

// available đọc số hàng còn bán được của một SKU.
func (h *harness) available(t *testing.T, skuID ids.ID) int {
	t.Helper()
	m, err := h.inv.GetAvailability(context.Background(),
		[]string{skuID.String()}, "")
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	return m[skuID.String()]
}

// ---------------------------------------------------------------- Test

// BẮT ĐẦU CHECKOUT thì HÀNG BỊ KHÓA THẬT.
//
// Đây là việc checkout làm mà giỏ hàng không làm, và là lý do nó tồn tại
// riêng. Kiểm chứng bằng inventory THẬT: số hàng khả dụng phải giảm ngay.
func TestBatDauCheckoutThiHangBiKhoaThat(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 3))

	if got := h.available(t, skuID); got != 10 {
		t.Fatalf("hàng khả dụng ban đầu = %d, mong 10", got)
	}

	c, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}

	// 10 − 3 = 7. Hàng đã bị KHÓA, không phải chỉ đánh dấu.
	if got := h.available(t, skuID); got != 7 {
		t.Errorf("hàng khả dụng sau khi giữ = %d, mong 7 — checkout phải "+
			"khóa hàng thật, đó là việc giỏ hàng không làm", got)
	}

	if !c.Lines()[0].HasStock() {
		t.Error("dòng phải có mã giữ hàng")
	}
	if c.Status() != domain.StatusStarted {
		t.Errorf("trạng thái = %q, mong STARTED", c.Status())
	}
}

// KHÔNG ĐỦ HÀNG thì KHÔNG mở phiên, VÀ nhả lại mọi thứ đã giữ.
//
// Đây là nhánh dễ bỏ quên nhất của cả module: giữ được hàng cho hai món
// rồi món thứ ba hết hàng thì HAI reservation kia phải được nhả. Không nhả
// thì hàng bị khóa 15 phút cho một phiên chưa từng tồn tại.
func TestThatBaiGiuaChungThiNhaLaiMoiThuDaGiu(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuA := h.stockSKU(t, 10)
	skuB := h.stockSKU(t, 10)
	skuC := h.stockSKU(t, 1) // chỉ còn 1, khách muốn 5

	h.setCart(
		item(skuA, 299000, 2),
		item(skuB, 450000, 3),
		item(skuC, 199000, 5),
	)

	_, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if !errors.Is(err, application.ErrOutOfStock) {
		t.Fatalf("lỗi = %v, mong ErrOutOfStock", err)
	}

	// HAI món đầu phải được nhả về đủ.
	if got := h.available(t, skuA); got != 10 {
		t.Errorf("hàng của món A = %d, mong 10 — reservation đã tạo phải "+
			"được nhả khi phiên thất bại giữa chừng", got)
	}
	if got := h.available(t, skuB); got != 10 {
		t.Errorf("hàng của món B = %d, mong 10 — bị khóa 15 phút cho một "+
			"phiên chưa từng tồn tại", got)
	}
}

// GIÁ ĐÓNG BĂNG qua toàn bộ luồng, tới tận đơn hàng.
//
//	14:00 — checkout bắt đầu, áo 299.000đ
//	14:05 — seller đổi giá thành 350.000đ (giỏ đổi theo)
//	14:10 — khách hoàn tất
//
// Đơn hàng phải ghi 299.000đ. Kiểm chứng qua ĐƠN THẬT trong database, không
// phải qua phiên trong bộ nhớ.
func TestGiaDongBangDiTuCheckoutSangDonHang(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 1))

	c, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}

	// Seller đổi giá — giỏ sẽ hiện giá mới, nhưng phiên thì không.
	h.setCart(item(skuID, 350000, 1))

	if _, err := h.svc.SetShippingAddress(ctx, c.ID(), testAddress()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}

	res, err := h.svc.CompleteCheckout(ctx, c.ID(), "dong-bang-gia-1")
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}

	// Đọc ĐƠN THẬT từ database.
	placed, err := h.ord.GetOrder(ctx, res.OrderID.String())
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}

	if placed.Lines[0].UnitPrice.Value != 299000 {
		t.Errorf("đơn giá trong đơn hàng = %d, mong 299000 — khách thấy "+
			"299.000đ ở màn hình thanh toán mà bị trừ số khác",
			placed.Lines[0].UnitPrice.Value)
	}
	if placed.Total.Value != 299000 {
		t.Errorf("tổng đơn = %d, mong 299000", placed.Total.Value)
	}
}

// HOÀN TẤT HAI LẦN chỉ tạo MỘT đơn (quy tắc 5).
func TestHoanTatHaiLanChiTaoMotDon(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 1))

	c, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if _, err := h.svc.SetShippingAddress(ctx, c.ID(), testAddress()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}

	first, err := h.svc.CompleteCheckout(ctx, c.ID(), "khach-bam-hai-lan")
	if err != nil {
		t.Fatalf("lần hoàn tất thứ nhất: %v", err)
	}
	second, err := h.svc.CompleteCheckout(ctx, c.ID(), "khach-bam-hai-lan")
	if err != nil {
		t.Fatalf("lần hoàn tất thứ hai: %v", err)
	}

	if first.OrderID != second.OrderID {
		t.Errorf("hai lần hoàn tất ra hai đơn: %s và %s — khách bị trừ tiền "+
			"hai lần", first.OrderID, second.OrderID)
	}
	if !second.Replayed {
		t.Error("lần thứ hai phải báo Replayed — nếu không, bên gọi sẽ gửi " +
			"email xác nhận lần thứ hai")
	}

	// Giỏ được đánh dấu chuyển đổi ĐÚNG MỘT lần cho mỗi lời gọi thành công,
	// nhưng đơn thì chỉ có một.
	if h.cart.convertedCount() == 0 {
		t.Error("giỏ phải được đánh dấu đã chuyển đổi")
	}
}

// HOÀN TẤT SONG SONG cũng chỉ ra MỘT đơn.
//
// Kiểm tra trạng thái phiên ở tầng ứng dụng không chặn được: mười request
// đến cùng lúc đều thấy phiên còn STARTED. Ràng buộc UNIQUE trên khóa
// idempotency ở module order là thứ chặn thật.
func TestHoanTatSongSongChiRaMotDon(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 1))

	c, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if _, err := h.svc.SetShippingAddress(ctx, c.ID(), testAddress()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}

	const n = 10
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		start    = make(chan struct{})
		orderIDs = map[ids.ID]int{}
		fails    []error
	)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			res, err := h.svc.CompleteCheckout(ctx, c.ID(), "mot-khoa-muoi-request")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fails = append(fails, err)
				return
			}
			orderIDs[res.OrderID]++
		}()
	}
	close(start)
	wg.Wait()

	// Một số request có thể thất bại vì tranh chấp ghi phiên — chấp nhận
	// được, miễn là KHÔNG có hai đơn khác nhau được tạo ra.
	if len(orderIDs) != 1 {
		t.Fatalf("số đơn khác nhau = %d, mong 1 (thất bại: %d) — khách bị "+
			"trừ tiền nhiều lần", len(orderIDs), len(fails))
	}
}

// HẾT HẠN thì NHẢ HÀNG và không đặt đơn được nữa.
//
// Đây là hàm giữ cho lời hứa "giữ hàng có thời hạn" thành sự thật. Không
// có nó thì mọi phiên bỏ dở đều khóa hàng cho tới khi có người phát hiện
// thủ công — và không ai đi tìm cho tới lúc hết hàng bán.
func TestHetHanThiNhaHangVaKhongDatDonDuoc(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 4))

	c, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}

	if got := h.available(t, skuID); got != 6 {
		t.Fatalf("hàng khả dụng sau khi giữ = %d, mong 6", got)
	}

	// Khách bỏ đi. 16 phút sau, phiên đã quá hạn theo đồng hồ nhưng CHƯA
	// ai đánh dấu EXPIRED — tiến trình nền chạy theo chu kỳ nên luôn có
	// khoảng trống này, và mọi thao tác phải bị chặn ngay trong đó.
	h.clock.advance(16 * time.Minute)

	// Không đặt đơn được nữa: hàng có thể đã bị nhả và bán cho người khác.
	if _, err := h.svc.SetShippingAddress(ctx, c.ID(), testAddress()); !errors.Is(err, domain.ErrExpired) {
		t.Errorf("đặt địa chỉ: lỗi = %v, mong ErrExpired", err)
	}
	if _, err := h.svc.CompleteCheckout(ctx, c.ID(), "phien-het-han"); !errors.Is(err, domain.ErrExpired) {
		t.Errorf("hoàn tất: lỗi = %v, mong ErrExpired", err)
	}

	// Chỉ báo giám sát thấy phiên quá hạn chưa dọn.
	pending, err := h.svc.CountExpiredPending(ctx)
	if err != nil {
		t.Fatalf("CountExpiredPending: %v", err)
	}
	if pending != 1 {
		t.Errorf("phiên quá hạn chưa dọn = %d, mong 1", pending)
	}

	// Tiến trình nền dọn và NHẢ HÀNG.
	done, err := h.svc.ExpireStale(ctx, 100)
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if done != 1 {
		t.Errorf("số phiên đã dọn = %d, mong 1", done)
	}

	if got := h.available(t, skuID); got != 10 {
		t.Errorf("hàng khả dụng sau khi dọn = %d, mong 10 — hàng bị khóa "+
			"vĩnh viễn cho một phiên đã chết", got)
	}

	after, _ := h.svc.CountExpiredPending(ctx)
	if after != 0 {
		t.Errorf("phiên quá hạn còn lại = %d, mong 0", after)
	}
}

// HỦY PHIÊN thì NHẢ HÀNG ngay.
func TestHuyPhienThiNhaHangNgay(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 3))

	c, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if got := h.available(t, skuID); got != 7 {
		t.Fatalf("hàng khả dụng = %d, mong 7", got)
	}

	if err := h.svc.Cancel(ctx, c.ID()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if got := h.available(t, skuID); got != 10 {
		t.Errorf("hàng khả dụng sau khi hủy = %d, mong 10", got)
	}
}

// MỘT GIỎ CHỈ CÓ MỘT PHIÊN đang chạy.
//
// Khách bấm "Thanh toán", quay lại giỏ, bấm lần nữa: phiên thứ hai sẽ giữ
// hàng LẦN THỨ HAI cho cùng một giỏ, tức là khóa gấp đôi số hàng thật cần.
func TestMotGioChiCoMotPhienDangChay(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 3))
	cartID := ids.MustNew(ids.PrefixCart)

	first, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{CartID: cartID})
	if err != nil {
		t.Fatalf("StartCheckout lần 1: %v", err)
	}
	second, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{CartID: cartID})
	if err != nil {
		t.Fatalf("StartCheckout lần 2: %v", err)
	}

	if first.ID() != second.ID() {
		t.Errorf("hai lần bắt đầu ra hai phiên: %s và %s", first.ID(), second.ID())
	}

	// Và quan trọng hơn: hàng chỉ bị khóa MỘT lần.
	if got := h.available(t, skuID); got != 7 {
		t.Errorf("hàng khả dụng = %d, mong 7 — phiên thứ hai đã khóa hàng "+
			"lần nữa cho cùng một giỏ", got)
	}
}

// KHÔNG BÁN QUÁ SỐ HÀNG CÓ, dưới tải song song.
//
// Kho có 5 sản phẩm, 10 khách cùng checkout mỗi người 1 món. Đúng 5 người
// giữ được hàng — khóa lạc quan ở inventory là thứ quyết định điều đó.
func TestMuoiKhachTranhNamSanPhamThiDungNamNguoiGiuDuoc(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 5)

	const n = 10
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		start   = make(chan struct{})
		ok      int
		outOf   int
		others  []error
		reserve = func(i int) {
			defer wg.Done()
			<-start

			// Mỗi khách một giỏ riêng.
			cart := &fakeCart{snap: application.CartSnapshot{
				CustomerID: ids.MustNew(ids.PrefixCustomer),
				Currency:   money.VND,
				Items:      []application.CartItemSnapshot{item(skuID, 299000, 1)},
			}}
			svc := application.NewService(application.Deps{
				Checkouts:   h.svc.Repo(),
				Carts:       cart,
				Inventory:   &realInventory{api: h.inv},
				Commissions: &fakeCommission{rate: 1000},
				Orders:      &realOrder{api: h.ord},
			})

			_, err := svc.StartCheckout(ctx, application.StartCheckoutInput{
				CartID: ids.MustNew(ids.PrefixCart),
			})

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, application.ErrOutOfStock):
				outOf++
			default:
				others = append(others, fmt.Errorf("khách %d: %w", i, err))
			}
		}
	)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go reserve(i)
	}
	close(start)
	wg.Wait()

	for _, err := range others {
		t.Errorf("lỗi ngoài dự kiến: %v", err)
	}
	if ok != 5 {
		t.Errorf("số khách giữ được hàng = %d, mong ĐÚNG 5 — bán quá số "+
			"hàng có nghĩa là phải hủy đơn và xin lỗi khách", ok)
	}
	if outOf != 5 {
		t.Errorf("số khách bị từ chối = %d, mong 5", outOf)
	}
	if got := h.available(t, skuID); got != 0 {
		t.Errorf("hàng khả dụng còn lại = %d, mong 0", got)
	}
}

// GIA HẠN gia hạn CẢ reservation, không chỉ phiên.
//
// Phiên sống lâu hơn reservation thì tới lúc đặt hàng mới phát hiện hàng
// đã bị nhả và bán cho người khác — đúng lúc khách vừa chuyển khoản xong.
func TestGiaHanGiaHanCaHangDangGiu(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 2))

	c, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
		TTL:    time.Minute,
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	before := c.ExpiresAt()

	extended, err := h.svc.Extend(ctx, c.ID())
	if err != nil {
		t.Fatalf("Extend: %v", err)
	}
	if !extended.ExpiresAt().After(before) {
		t.Errorf("thời hạn = %v, mong lùi sau %v", extended.ExpiresAt(), before)
	}
	if extended.ExtendedTimes() != 1 {
		t.Errorf("số lần gia hạn = %d, mong 1", extended.ExtendedTimes())
	}

	// Hàng vẫn đang bị giữ.
	if got := h.available(t, skuID); got != 8 {
		t.Errorf("hàng khả dụng = %d, mong 8", got)
	}
}

// TẠO ĐƠN THẤT BẠI thì KHÔNG hủy phiên, KHÔNG nhả hàng (quy tắc 4).
//
// Cho khách thử lại phương thức thanh toán khác trong thời gian TTL còn
// lại. Hủy ngay là trải nghiệm tệ và làm mất đơn hàng.
func TestTaoDonThatBaiThiGiuNguyenPhienChoKhachThuLai(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 2))

	c, err := h.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}

	// Thiếu địa chỉ → tạo đơn thất bại.
	if _, err := h.svc.CompleteCheckout(ctx, c.ID(), "thieu-dia-chi"); !errors.Is(err, domain.ErrNoAddress) {
		t.Fatalf("lỗi = %v, mong ErrNoAddress", err)
	}

	// Phiên VẪN CÒN, hàng VẪN GIỮ.
	after, err := h.svc.GetCheckout(ctx, c.ID())
	if err != nil {
		t.Fatalf("GetCheckout: %v", err)
	}
	if after.Status().IsFinal() {
		t.Errorf("trạng thái = %q, mong phiên vẫn còn sống — hủy ngay khi "+
			"thất bại là làm mất đơn hàng", after.Status())
	}
	if got := h.available(t, skuID); got != 8 {
		t.Errorf("hàng khả dụng = %d, mong giữ nguyên 8", got)
	}

	// Khách nhập địa chỉ rồi thử lại: thành công.
	if _, err := h.svc.SetShippingAddress(ctx, c.ID(), testAddress()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := h.svc.CompleteCheckout(ctx, c.ID(), "thu-lai-thanh-cong"); err != nil {
		t.Errorf("thử lại phải thành công: %v", err)
	}
}
