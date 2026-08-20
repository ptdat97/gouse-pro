package e2e_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/checkout"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
	checkoutdomain "github.com/fashion-commerce/platform/internal/modules/checkout/domain"
	checkoutpg "github.com/fashion-commerce/platform/internal/modules/checkout/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// ---------------------------------------------------------------- Bối cảnh

// world là một thế giới nhỏ nhưng THẬT: PostgreSQL thật, module thật, event
// đi qua outbox thật rồi được dispatcher thật phát đi.
//
// Chỉ giỏ hàng là bản giả, và có lý do: giỏ chỉ CUNG CẤP ảnh chụp đầu vào.
// Dựng giỏ thật kéo theo marketplace, product và catalog — nhiều bước dựng
// dữ liệu hơn, mà không thêm được một ranh giới nào vào đoạn chuỗi đang
// cần kiểm chứng.
type world struct {
	t *testing.T

	db  *database.DB
	inv inventory.API
	ord order.API
	ful *fulfillment.Module
	bus *eventbus.Dispatcher

	checkout *checkoutapp.Service
	cart     *stubCart

	// owners quyết định "hàng của nhà bán này thuộc về ai" — cùng quy tắc
	// production dùng, chỉ khác nguồn cờ INTERNAL.
	internal map[ids.ID]bool
}

func newWorld(t *testing.T) *world {
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
		"TRUNCATE inventory_item CASCADE",
		"TRUNCATE stock_location CASCADE",
		"TRUNCATE event_processed",
		"TRUNCATE event_outbox",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	invModule, err := inventory.New(inventory.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("inventory.New: %v", err)
	}
	ordModule, err := order.New(order.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("order.New: %v", err)
	}
	fulModule, err := fulfillment.New(fulfillment.Config{
		Storage: "postgres", DB: db, Events: eventbus.NewOutbox(db.Pool()),
	})
	if err != nil {
		t.Fatalf("fulfillment.New: %v", err)
	}

	bus := eventbus.NewDispatcher(db.Pool(), log)
	bus.Subscribe(fulfillment.NewSplitHandler(fulModule, log))
	bus.Subscribe(order.NewProgressHandler(ordModule, log))
	bus.Subscribe(inventory.NewCommitHandler(invModule, log))

	w := &world{
		t: t, db: db, inv: invModule, ord: ordModule, ful: fulModule,
		bus: bus, internal: map[ids.ID]bool{},
	}
	w.cart = &stubCart{}

	w.checkout = checkoutapp.NewService(checkoutapp.Deps{
		Checkouts:   checkoutpg.NewCheckoutStore(db.Pool()),
		Carts:       w.cart,
		Inventory:   &invPort{api: invModule},
		Commissions: &flatCommission{bp: 1000},
		Sellers:     &ownerPort{internal: w.internal},
		Orders:      &orderPort{api: ordModule},
		Events:      checkout.NewEventPublisher(eventbus.NewOutbox(db.Pool())),
	})
	return w
}

// drain phát hết event đang nằm trong outbox.
//
// Ở production việc này do tiến trình worker làm theo nhịp. Ở đây gọi tay
// để test không phải chờ, nhưng đi qua ĐÚNG bộ máy đó — không phải gọi
// thẳng bên nhận.
func (w *world) drain() {
	w.t.Helper()
	for i := 0; i < 5; i++ {
		n, err := w.bus.DispatchBatch(context.Background(), 100)
		if err != nil {
			w.t.Fatalf("phát event: %v", err)
		}
		if n == 0 {
			return
		}
	}
}

// stockFor nhập hàng thuộc về một chủ sở hữu cụ thể và trả mã kho.
func (w *world) stockFor(skuID, owner ids.ID, qty int) {
	w.t.Helper()
	ctx := context.Background()
	locID := ids.MustNew(ids.PrefixStockLocation)

	if _, err := w.db.Pool().Exec(ctx, `
		INSERT INTO stock_location (id, name, code, kind, created_at, updated_at)
		VALUES ($1, 'Kho', $2, 'SELLER', now(), now())`,
		locID.String(), "KHO-"+string(locID[len(locID)-6:])); err != nil {
		w.t.Fatalf("tạo kho: %v", err)
	}

	if _, err := w.inv.Receive(ctx, inventory.ReceiveRequest{
		SKUID:       skuID.String(),
		LocationID:  locID.String(),
		OwnerID:     owner.String(),
		Quantity:    qty,
		PerformedBy: "e2e",
	}); err != nil {
		w.t.Fatalf("nhập kho cho %s: %v", owner, err)
	}
}

// stock trả (khả dụng, đã cam kết) của một chủ sở hữu.
func (w *world) stock(skuID, owner ids.ID) (int, int) {
	w.t.Helper()
	items, err := w.inv.GetItemsBySKUs(
		context.Background(), []string{skuID.String()}, "")
	if err != nil {
		w.t.Fatalf("đọc tồn kho: %v", err)
	}
	var available, committed int
	for _, it := range items[skuID.String()] {
		if it.OwnerID == owner.String() {
			available += it.Available
			committed += it.Committed
		}
	}
	return available, committed
}

// ---------------------------------------------------------------- Cổng ra

// invPort nối checkout tới module inventory THẬT.
//
// Đây là bản sao của adapter production (checkout/adapters.go). Nó tồn tại
// vì adapter kia không xuất khẩu — và việc nó phải mang `OwnerID` sang là
// đúng điểm P3-18 từng bỏ sót.
type invPort struct{ api inventory.API }

func (p *invPort) FindItemsForSKUs(
	ctx context.Context, skuIDs []ids.ID,
) (map[ids.ID][]checkoutapp.StockItem, error) {
	strs := make([]string, 0, len(skuIDs))
	for _, id := range skuIDs {
		strs = append(strs, id.String())
	}
	found, err := p.api.GetItemsBySKUs(ctx, strs, "")
	if err != nil {
		return nil, err
	}
	out := make(map[ids.ID][]checkoutapp.StockItem, len(found))
	for skuID, items := range found {
		for _, it := range items {
			out[ids.ID(skuID)] = append(out[ids.ID(skuID)], checkoutapp.StockItem{
				ItemID:    ids.ID(it.ID),
				Available: it.Available,
				OwnerID:   ids.ID(it.OwnerID),
			})
		}
	}
	return out, nil
}

func (p *invPort) Reserve(
	ctx context.Context, itemID, checkoutID ids.ID, qty int, ttl time.Duration,
) (ids.ID, error) {
	res, err := p.api.Reserve(ctx, inventory.ReserveRequest{
		ItemID: itemID.String(), CheckoutID: checkoutID.String(),
		Quantity: qty, TTL: ttl,
	})
	if err != nil {
		return "", err
	}
	return ids.ID(res.ID), nil
}

func (p *invPort) Release(ctx context.Context, id ids.ID) error {
	return p.api.ReleaseReservation(ctx, id.String())
}

func (p *invPort) Extend(ctx context.Context, id ids.ID, d time.Duration) error {
	return p.api.ExtendReservation(ctx, id.String(), d)
}

// ownerPort cài quy tắc thật: INTERNAL → nền tảng, còn lại → chính nhà bán.
type ownerPort struct{ internal map[ids.ID]bool }

func (p *ownerPort) InventoryOwnerID(
	_ context.Context, sellerID ids.ID,
) (ids.ID, error) {
	return ids.ID(inventory.OwnerForSeller(
		sellerID.String(), p.internal[sellerID])), nil
}

type flatCommission struct{ bp int32 }

func (f *flatCommission) RateForSeller(
	context.Context, ids.ID,
) (types.BasisPoints, error) {
	return types.NewBasisPoints(f.bp)
}

// orderPort nối checkout tới module order THẬT.
type orderPort struct{ api order.API }

func (p *orderPort) PlaceOrder(
	ctx context.Context, in checkoutapp.PlaceOrderInput,
) (checkoutapp.PlacedOrder, error) {
	lines := make([]order.PlaceOrderLineInput, 0, len(in.Lines))
	for _, l := range in.Lines {
		lines = append(lines, order.PlaceOrderLineInput{
			OfferID:            l.OfferID.String(),
			SKUID:              l.SKUID.String(),
			SellerID:           l.SellerID.String(),
			ProductName:        l.ProductName,
			VariantDescription: l.VariantDescription,
			UnitPrice: order.Amount{
				Value: l.UnitPrice.Amount(), Currency: string(l.UnitPrice.Currency()),
			},
			Quantity:       l.Quantity,
			CommissionRate: int(l.CommissionRate.Value()),
		})
	}
	res, err := p.api.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID:     in.CustomerID.String(),
		GuestEmail:     in.GuestEmail,
		GuestPhone:     in.GuestPhone,
		Currency:       string(in.Currency),
		Lines:          lines,
		IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil {
		return checkoutapp.PlacedOrder{}, err
	}
	return checkoutapp.PlacedOrder{
		OrderID:     ids.ID(res.Order.ID),
		OrderNumber: res.Order.OrderNumber,
	}, nil
}

// stubCart cung cấp ảnh chụp giỏ.
type stubCart struct{ snap checkoutapp.CartSnapshot }

func (c *stubCart) LoadPurchasable(
	context.Context, ids.ID,
) (checkoutapp.CartSnapshot, error) {
	return c.snap, nil
}

func (c *stubCart) MarkConverted(context.Context, ids.ID) error { return nil }

func (c *stubCart) ActiveCartID(context.Context, string, string) (ids.ID, error) {
	return c.snap.CartID, nil
}

func vnd(n int64) money.Money {
	m, err := money.New(n, money.VND)
	if err != nil {
		panic(err)
	}
	return m
}

// ---------------------------------------------------------------- Luồng

// TestDonNhieuNhaBanDiHetChuoi đi hết chuỗi của một đơn hàng chợ:
//
//	giữ hàng → đặt đơn → phát event → tách đơn thực hiện → cam kết tồn kho
//
// Giỏ có ba món của HAI nhà bán, trong đó một nhà bán là own brand (seller
// NỘI BỘ, hàng thuộc nền tảng). Cả hai cùng bán một SKU chung — tình huống
// làm nên cái chợ, và là nơi P3-18 từng sai.
//
// # Vì sao một test lớn thay vì năm test nhỏ
//
// Điều cần kiểm chứng KHÔNG phải từng bước — mỗi bước đã có test riêng
// trong module của nó. Điều cần kiểm chứng là dữ liệu đi qua bốn ranh giới
// mà không rơi mất và không đổi nghĩa. Tách nhỏ ra thì mỗi mảnh lại phải
// dựng bản giả cho mảnh kế bên, và bản giả chính là thứ đã che P3-18.
func TestDonNhieuNhaBanDiHetChuoi(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	ownBrand := ids.MustNew(ids.PrefixSeller) // seller NỘI BỘ
	shop := ids.MustNew(ids.PrefixSeller)     // nhà bán ngoài
	w.internal[ownBrand] = true

	platform := ids.ID(inventory.PlatformOwnerID)

	// SKU dùng CHUNG: cả own brand lẫn nhà bán ngoài đều có hàng.
	shared := ids.MustNew(ids.PrefixSKU)
	w.stockFor(shared, platform, 100)
	w.stockFor(shared, shop, 50)

	// SKU riêng của nhà bán ngoài.
	only := ids.MustNew(ids.PrefixSKU)
	w.stockFor(only, shop, 30)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.snap = checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		GuestPhone: "+84901234567",
		Currency:   money.VND,
		Items: []checkoutapp.CartItemSnapshot{
			// Món 1: own brand, SKU dùng chung → phải lấy hàng NỀN TẢNG.
			line(ownBrand, shared, 300_000, 2),
			// Món 2: nhà bán ngoài, CÙNG SKU → phải lấy hàng CỦA HỌ.
			line(shop, shared, 280_000, 3),
			// Món 3: nhà bán ngoài, SKU riêng.
			line(shop, only, 150_000, 1),
		},
	}

	// ---- Bước 1: mở phiên → giữ hàng
	c, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{
		CartID: cartID,
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}

	if avail, _ := w.stock(shared, platform); avail != 98 {
		t.Errorf("hàng nền tảng sau khi giữ: %d khả dụng, cần 98", avail)
	}
	if avail, _ := w.stock(shared, shop); avail != 47 {
		t.Errorf("hàng nhà bán (SKU chung) sau khi giữ: %d khả dụng, cần 47", avail)
	}
	if avail, _ := w.stock(only, shop); avail != 29 {
		t.Errorf("hàng nhà bán (SKU riêng) sau khi giữ: %d khả dụng, cần 29", avail)
	}

	// ---- Bước 2: địa chỉ + phương thức giao
	if _, err := w.checkout.SetShippingAddress(ctx, c.ID(), address()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := w.checkout.SetShippingMethod(ctx, c.ID(), "STANDARD"); err != nil {
		t.Fatalf("SetShippingMethod: %v", err)
	}

	// ---- Bước 3: hoàn tất → tạo đơn + ghi event vào outbox
	res, err := w.checkout.CompleteCheckout(
		ctx, c.ID(), ids.MustNew(ids.PrefixRequest).String())
	if err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}
	if res.OrderNumber == "" {
		t.Fatal("đơn hàng không có mã")
	}

	// ---- Bước 4: worker phát event
	w.drain()

	// ---- Kiểm chứng A: tồn kho chuyển sang CAM KẾT, đúng của từng chủ
	availP, commP := w.stock(shared, platform)
	if availP != 98 || commP != 2 {
		t.Errorf("hàng nền tảng: %d khả dụng / %d cam kết, cần 98/2", availP, commP)
	}
	availS, commS := w.stock(shared, shop)
	if availS != 47 || commS != 3 {
		t.Errorf("hàng nhà bán (SKU chung): %d/%d, cần 47/3", availS, commS)
	}

	// ---- Kiểm chứng B: HAI đơn thực hiện, mỗi nhà bán một đơn
	fos, err := w.ful.GetOrderFulfillments(ctx, res.OrderID.String())
	if err != nil {
		t.Fatalf("ListByOrder: %v", err)
	}
	if len(fos) != 2 {
		t.Fatalf("số đơn thực hiện = %d, cần 2 (một cho mỗi nhà bán)", len(fos))
	}

	bySeller := map[string]fulfillment.FulfillmentView{}
	for _, fo := range fos {
		bySeller[fo.SellerID] = fo
	}

	foOwn, ok := bySeller[ownBrand.String()]
	if !ok {
		t.Fatal("không có đơn thực hiện cho own brand")
	}
	foShop, ok := bySeller[shop.String()]
	if !ok {
		t.Fatal("không có đơn thực hiện cho nhà bán ngoài")
	}

	// ---- Kiểm chứng C: dòng hàng về đúng nhà bán
	if len(foOwn.LineIDs) != 1 {
		t.Errorf("đơn own brand có %d dòng, cần 1", len(foOwn.LineIDs))
	}
	if len(foShop.LineIDs) != 2 {
		t.Errorf("đơn nhà bán ngoài có %d dòng, cần 2", len(foShop.LineIDs))
	}

	// ---- Kiểm chứng D: tiền
	//
	// own brand: 300.000 × 2 = 600.000, hoa hồng 10% = 60.000
	// nhà bán:   280.000 × 3 + 150.000 = 990.000, hoa hồng 10% = 99.000
	if got := foOwn.Subtotal.Value; got != 600_000 {
		t.Errorf("tiền hàng own brand = %d, cần 600000", got)
	}
	if got := foOwn.SellerPayable.Value; got != 540_000 {
		t.Errorf("phải trả own brand = %d, cần 540000", got)
	}
	if got := foShop.Subtotal.Value; got != 990_000 {
		t.Errorf("tiền hàng nhà bán = %d, cần 990000", got)
	}
	if got := foShop.SellerPayable.Value; got != 891_000 {
		t.Errorf("phải trả nhà bán = %d, cần 891000", got)
	}

	// ---- Kiểm chứng E: mỗi nhà bán nhận được ĐỊA CHỈ GIAO
	//
	// Không có nó thì họ không giao được, và giao diện Trung tâm người bán
	// khóa nút bàn giao.
	for name, pair := range map[string]struct{ seller, fo string }{
		"own brand":     {ownBrand.String(), foOwn.ID},
		"nhà bán ngoài": {shop.String(), foShop.ID},
	} {
		// Đọc qua tầng application để thấy dòng hàng và địa chỉ: đó là góc
		// nhìn NHÀ BÁN, và cũng là góc nhìn Trung tâm người bán dựng lên.
		got, err := w.ful.Service().GetSellerFulfillment(
			ctx, ids.ID(pair.seller), ids.ID(pair.fo))
		if err != nil {
			t.Errorf("đọc đơn của %s: %v", name, err)
			continue
		}
		addr := got.ShippingAddress()
		if addr.IsEmpty() {
			t.Errorf("đơn của %s không có địa chỉ giao — nhà bán không giao được", name)
			continue
		}
		if addr.RecipientName != "Nguyễn Văn A" {
			t.Errorf("đơn của %s: người nhận = %q", name, addr.RecipientName)
		}

		// Mô tả biến thể phải đi tới tận đây: với thời trang, tên sản phẩm
		// không đủ để nhặt đúng hàng.
		for _, l := range got.Lines() {
			if l.VariantDescription == "" {
				t.Errorf("đơn của %s: dòng hàng thiếu mô tả biến thể", name)
			}
		}
	}
}

func line(
	sellerID, skuID ids.ID, price int64, qty int,
) checkoutapp.CartItemSnapshot {
	return checkoutapp.CartItemSnapshot{
		CartItemID:         ids.MustNew(ids.PrefixCartItem),
		OfferID:            ids.MustNew(ids.PrefixOffer),
		SKUID:              skuID,
		SellerID:           sellerID,
		ProductName:        "Áo sơ mi linen Oxford",
		VariantDescription: "Trắng / M",
		UnitPrice:          vnd(price),
		Quantity:           qty,
	}
}

func address() checkoutdomain.Address {
	return checkoutdomain.Address{
		RecipientName: "Nguyễn Văn A",
		Phone:         "+84901234567",
		StreetAddress: "12 Lê Lợi",
		Ward:          "Bến Nghé",
		District:      "Quận 1",
		Province:      "TP. Hồ Chí Minh",
		CountryCode:   "VN",
	}
}
