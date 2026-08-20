package checkout_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/checkout"
	"github.com/fashion-commerce/platform/internal/modules/checkout/application"
	checkoutpg "github.com/fashion-commerce/platform/internal/modules/checkout/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// quantities đọc số lượng theo trạng thái của một SKU.
func (h *harness) quantities(t *testing.T, skuID ids.ID) (available, reserved, committed int) {
	t.Helper()
	items, err := h.inv.GetItemsBySKUs(context.Background(),
		[]string{skuID.String()}, "")
	if err != nil {
		t.Fatalf("GetItemsBySKUs: %v", err)
	}
	for _, it := range items[skuID.String()] {
		available += it.Available
		reserved += it.Reserved
		committed += it.Committed
	}
	return
}

// ĐẶT HÀNG XONG thì hàng chuyển RESERVED → COMMITTED, qua event.
//
// VÌ SAO BƯỚC NÀY KHÔNG THỂ ĐỂ "LÀM SAU": tiến trình dọn reservation quá
// hạn sẽ NHẢ hàng còn ở trạng thái Reserved. Nếu đơn đã thanh toán mà hàng
// vẫn Reserved, tiến trình đó biến một đơn đã thu tiền thành đơn không có
// hàng — và bán hàng đó cho khách khác.
func TestDatHangXongThiHangChuyenSangCommitted(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Dựng eventbus THẬT: đây là thứ đang được kiểm chứng.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(h.db.Pool(), log)

	invModule, err := inventory.New(inventory.Config{Storage: "postgres", DB: h.db})
	if err != nil {
		t.Fatalf("inventory.New: %v", err)
	}
	bus.Subscribe(inventory.NewCommitHandler(invModule, log))

	// Dựng lại service checkout CÓ bộ phát event.
	svc := application.NewService(application.Deps{
		Checkouts:   checkoutpg.NewCheckoutStore(h.db.Pool()),
		Carts:       h.cart,
		Inventory:   &realInventory{api: h.inv},
		Commissions: &fakeCommission{rate: 1000},
		Sellers:     &fakeSellerOwner{},
		Orders:      &realOrder{api: h.ord},
		Clock:       h.clock,
		Events:      checkout.NewEventPublisher(bus.Outbox()),
	})

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 3))

	c, err := svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}

	// Sau khi giữ hàng: 7 khả dụng, 3 đang giữ, 0 cam kết.
	avail, reserved, committed := h.quantities(t, skuID)
	if avail != 7 || reserved != 3 || committed != 0 {
		t.Fatalf("sau khi giữ hàng: khả dụng=%d giữ=%d cam kết=%d, mong 7/3/0",
			avail, reserved, committed)
	}

	if _, err := svc.SetShippingAddress(ctx, c.ID(), testAddress()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := svc.CompleteCheckout(ctx, c.ID(), "dat-hang-1"); err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}

	// NGAY SAU khi đặt hàng, event chưa được phát — hàng vẫn ở Reserved.
	//
	// Đây là nhất quán CUỐI, không phải tức thời: đó là đánh đổi của kiến
	// trúc event, và nó chấp nhận được vì reservation vẫn còn hiệu lực.
	_, reserved, committed = h.quantities(t, skuID)
	if reserved != 3 || committed != 0 {
		t.Errorf("trước khi phát event: giữ=%d cam kết=%d, mong 3/0",
			reserved, committed)
	}

	// Tiến trình nền phát event.
	n, err := bus.DispatchBatch(ctx, 100)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if n != 1 {
		t.Fatalf("số event đã phát = %d, mong 1", n)
	}

	// Giờ hàng đã CAM KẾT: 7 khả dụng, 0 đang giữ, 3 cam kết.
	avail, reserved, committed = h.quantities(t, skuID)
	if avail != 7 || reserved != 0 || committed != 3 {
		t.Errorf("sau khi phát event: khả dụng=%d giữ=%d cam kết=%d, mong 7/0/3 — "+
			"hàng còn ở Reserved sẽ bị tiến trình dọn NHẢ và bán cho khách khác",
			avail, reserved, committed)
	}
}

// PHÁT LẠI EVENT không cam kết hàng hai lần.
//
// Mô hình at-least-once nghĩa là event SẼ được phát lại. Cam kết hai lần
// nghĩa là trừ tồn kho gấp đôi cho một đơn.
func TestPhatLaiEventKhongCamKetHaiLan(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(h.db.Pool(), log)

	invModule, err := inventory.New(inventory.Config{Storage: "postgres", DB: h.db})
	if err != nil {
		t.Fatalf("inventory.New: %v", err)
	}
	bus.Subscribe(inventory.NewCommitHandler(invModule, log))

	svc := application.NewService(application.Deps{
		Checkouts:   checkoutpg.NewCheckoutStore(h.db.Pool()),
		Carts:       h.cart,
		Inventory:   &realInventory{api: h.inv},
		Commissions: &fakeCommission{rate: 1000},
		Sellers:     &fakeSellerOwner{},
		Orders:      &realOrder{api: h.ord},
		Clock:       h.clock,
		Events:      checkout.NewEventPublisher(bus.Outbox()),
	})

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 2))

	c, _ := svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})
	if _, err := svc.SetShippingAddress(ctx, c.ID(), testAddress()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := svc.CompleteCheckout(ctx, c.ID(), "dat-hang-2"); err != nil {
		t.Fatalf("CompleteCheckout: %v", err)
	}

	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("lượt phát 1: %v", err)
	}

	_, reserved, committed := h.quantities(t, skuID)
	if reserved != 0 || committed != 2 {
		t.Fatalf("sau lượt 1: giữ=%d cam kết=%d, mong 0/2", reserved, committed)
	}

	// Ép phát lại: mô phỏng worker chết sau khi handler chạy xong nhưng
	// trước khi kịp đánh dấu published_at.
	if _, err := h.db.Pool().Exec(ctx,
		`UPDATE event_outbox SET published_at = NULL`); err != nil {
		t.Fatalf("ép phát lại: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("lượt phát 2: %v", err)
	}

	_, reserved, committed = h.quantities(t, skuID)
	if committed != 2 {
		t.Errorf("sau lượt 2: cam kết=%d, mong giữ nguyên 2 — cam kết hai "+
			"lần nghĩa là trừ tồn kho gấp đôi cho một đơn", committed)
	}
}

// KHÔNG PHÁT EVENT nếu giao dịch tạo đơn thất bại.
//
// Nửa còn lại của Transactional Outbox: bên nhận không được xử lý một sự
// thật chưa từng xảy ra.
func TestTaoDonThatBaiThiKhongPhatEvent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(h.db.Pool(), log)

	svc := application.NewService(application.Deps{
		Checkouts:   checkoutpg.NewCheckoutStore(h.db.Pool()),
		Carts:       h.cart,
		Inventory:   &realInventory{api: h.inv},
		Commissions: &fakeCommission{rate: 1000},
		Sellers:     &fakeSellerOwner{},
		Orders:      &realOrder{api: h.ord},
		Clock:       h.clock,
		Events:      checkout.NewEventPublisher(bus.Outbox()),
	})

	skuID := h.stockSKU(t, 10)
	h.setCart(item(skuID, 299000, 1))

	c, _ := svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID: ids.MustNew(ids.PrefixCart),
	})

	// Thiếu địa chỉ → tạo đơn thất bại.
	if _, err := svc.CompleteCheckout(ctx, c.ID(), "that-bai"); err == nil {
		t.Fatal("thiếu địa chỉ phải làm việc tạo đơn thất bại")
	}

	stats, err := bus.Outbox().Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Pending != 0 {
		t.Errorf("số event chờ = %d, mong 0 — đơn chưa tạo mà event đã phát "+
			"nghĩa là tồn kho bị cam kết cho một đơn không tồn tại",
			stats.Pending)
	}
}
