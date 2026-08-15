package fulfillment_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

type harness struct {
	ful     *fulfillment.Module
	ord     *order.Module
	bus     *eventbus.Dispatcher
	pool    *pgxpool.Pool
	orderID string
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE fulfillment_order_line CASCADE",
		"TRUNCATE fulfillment_order CASCADE",
		"TRUNCATE order_line_adjustment CASCADE",
		"TRUNCATE order_line CASCADE",
		`TRUNCATE "order" CASCADE`,
		"TRUNCATE event_processed",
		"TRUNCATE event_outbox",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(db.Pool(), log)

	ful, err := fulfillment.New(fulfillment.Config{
		Storage: "postgres", DB: db, Events: eventbus.NewOutbox(db.Pool()),
	})
	if err != nil {
		t.Fatalf("fulfillment.New: %v", err)
	}
	ord, err := order.New(order.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("order.New: %v", err)
	}

	bus.Subscribe(fulfillment.NewSplitHandler(ful, log))
	bus.Subscribe(order.NewProgressHandler(ord, log))

	return &harness{ful: ful, ord: ord, bus: bus, pool: db.Pool()}
}

// placeOrder tạo một đơn hàng thật rồi phát event tách đơn.
//
// Dùng module order THẬT: điều cần kiểm chứng là trạng thái tổng hợp được
// tính lại từ tiến độ, và trạng thái đó nằm trong database của order.
func (h *harness) placeOrder(t *testing.T, sellers ...string) []fulfillment.FulfillmentView {
	t.Helper()
	ctx := context.Background()

	lines := make([]order.PlaceOrderLineInput, 0, len(sellers))
	for _, s := range sellers {
		lines = append(lines, order.PlaceOrderLineInput{
			OfferID:        ids.MustNew(ids.PrefixOffer).String(),
			SKUID:          ids.MustNew(ids.PrefixSKU).String(),
			SellerID:       s,
			ProductName:    "Áo sơ mi linen",
			UnitPrice:      order.Amount{Value: 300000, Currency: "VND"},
			Quantity:       1,
			CommissionRate: 1000,
		})
	}

	res, err := h.ord.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID:     ids.MustNew(ids.PrefixCustomer).String(),
		Currency:       "VND",
		Lines:          lines,
		IdempotencyKey: ids.MustNew(ids.PrefixRequest).String(),
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	h.orderID = res.Order.ID

	// Phát event checkout.completed để fulfillment tách đơn.
	reservations := make([]map[string]any, 0, len(res.Order.Lines))
	for _, l := range res.Order.Lines {
		reservations = append(reservations, map[string]any{
			"line_id":           l.ID,
			"sku_id":            l.SKUID,
			"seller_id":         l.SellerID,
			"quantity":          l.Quantity,
			"line_total":        l.LineTotal.Value,
			"commission_amount": l.CommissionAmount.Value,
		})
	}

	e, err := eventbus.NewEvent(
		eventbus.TypeCheckoutCompleted, eventbus.AggregateCheckout,
		ids.MustNew(ids.PrefixCheckout),
		map[string]any{
			"order_id":     res.Order.ID,
			"order_number": res.Order.OrderNumber,
			"currency":     "VND",
			"reservations": reservations,
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := h.bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := h.bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	fos, err := h.ful.GetOrderFulfillments(ctx, res.Order.ID)
	if err != nil {
		t.Fatalf("GetOrderFulfillments: %v", err)
	}
	return fos
}

// orderStatus đọc trạng thái tổng hợp của đơn.
func (h *harness) orderStatus(t *testing.T) string {
	t.Helper()
	o, err := h.ord.GetOrder(context.Background(), h.orderID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	return o.Status
}

// TÁCH ĐƠN BA NGUỒN HÀNG qua event — ví dụ ở mục 3.2 của ADR-0007.
func TestTachDonBaNguonHangQuaEvent(t *testing.T) {
	h := newHarness(t)

	a := ids.MustNew(ids.PrefixSeller).String()
	b := ids.MustNew(ids.PrefixSeller).String()
	c := ids.MustNew(ids.PrefixSeller).String()

	fos := h.placeOrder(t, a, b, c)

	if len(fos) != 3 {
		t.Fatalf("số đơn thực hiện = %d, mong 3", len(fos))
	}
	for _, fo := range fos {
		if fo.Status != fulfillment.StatusPending {
			t.Errorf("trạng thái = %q, mong PENDING", fo.Status)
		}
		if fo.SellerPayable.Value != 270000 {
			t.Errorf("phải trả seller = %d, mong 270000", fo.SellerPayable.Value)
		}
	}
}

// TÁCH ĐƠN IDEMPOTENT: event phát lại KHÔNG tạo thêm bộ đơn thực hiện.
//
// Tách hai lần nghĩa là seller thấy việc trùng — có thể giao hàng hai lần
// cho một đơn.
func TestTachDonHaiLanKhongTaoThemDon(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	fos := h.placeOrder(t, ids.MustNew(ids.PrefixSeller).String())
	if len(fos) != 1 {
		t.Fatalf("số đơn thực hiện = %d, mong 1", len(fos))
	}

	// Ép phát lại event.
	if _, err := h.pool.Exec(ctx,
		`UPDATE event_outbox SET published_at = NULL`); err != nil {
		t.Fatalf("ép phát lại: %v", err)
	}
	if _, err := h.bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	again, err := h.ful.GetOrderFulfillments(ctx, h.orderID)
	if err != nil {
		t.Fatalf("GetOrderFulfillments: %v", err)
	}
	if len(again) != 1 {
		t.Errorf("số đơn thực hiện sau khi phát lại = %d, mong 1 — tách hai "+
			"lần nghĩa là seller thấy việc trùng và có thể giao hai lần",
			len(again))
	}

	// HAI LỚP BẢO VỆ, kiểm chứng riêng lớp thứ hai.
	//
	// Lớp 1: tầng ứng dụng kiểm tra ExistsForOrder trước khi tách.
	// Lớp 2: chỉ mục UNIQUE (order_id, seller_id) ở database.
	//
	// Bỏ lớp 1 thì lớp 2 vẫn chặn — nhưng bằng cách báo lỗi, không phải bỏ
	// qua êm. Test này chứng minh lớp 2 tồn tại và hoạt động.
	var count int
	if err := h.pool.QueryRow(ctx, `
		SELECT count(*) FROM fulfillment_order WHERE order_id = $1`,
		h.orderID).Scan(&count); err != nil {
		t.Fatalf("đếm đơn thực hiện: %v", err)
	}
	if count != 1 {
		t.Errorf("số hàng trong database = %d, mong 1", count)
	}

	// Cố ghi trực tiếp một đơn thực hiện thứ hai cho cùng seller: chỉ mục
	// UNIQUE phải chặn.
	_, dupErr := h.pool.Exec(ctx, `
		INSERT INTO fulfillment_order (
			id, order_id, fo_number, seller_id, status,
			subtotal, commission_amount, currency, fulfillment_type,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,'PENDING',0,0,'VND','SELLER',now(),now())`,
		ids.MustNew(ids.PrefixFulfillmentOrder).String(),
		h.orderID, "FC-TRUNG-A", again[0].SellerID)
	if dupErr == nil {
		t.Error("database phải chặn đơn thực hiện thứ hai cho cùng seller — " +
			"gom theo seller là nguyên tắc tách đơn, hai hàng nghĩa là tách sai")
	}
}

// SELLER KHÔNG THẤY PHẦN CỦA SELLER KHÁC, kể cả khi biết định danh.
//
// Đây là kiểm chứng quan trọng nhất của module: nếu nó hỏng thì seller đọc
// được dữ liệu đối thủ, và không có cách nào phát hiện từ phía nạn nhân.
func TestSellerKhongThayDuocPhanCuaSellerKhac(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	fos := h.placeOrder(t, sellerA, sellerB)

	var foB string
	for _, fo := range fos {
		if fo.SellerID == sellerB {
			foB = fo.ID
		}
	}
	if foB == "" {
		t.Fatal("không tìm thấy đơn của seller B")
	}

	// A biết chính xác id của B và cố đọc.
	if _, err := h.ful.GetSellerFulfillment(ctx, sellerA, foB); !errors.Is(err, fulfillment.ErrNotFound) {
		t.Errorf("lỗi = %v, mong ErrNotFound — seller A đọc được đơn của B", err)
	}
	if err := h.ful.ConfirmFulfillment(ctx, sellerA, foB); !errors.Is(err, fulfillment.ErrNotFound) {
		t.Errorf("lỗi = %v, mong ErrNotFound — seller A xác nhận được đơn của B", err)
	}

	// Danh sách việc của A chỉ có đúng một đơn — của chính A.
	list, err := h.ful.ListSellerFulfillments(ctx, sellerA, nil, 50, 0)
	if err != nil {
		t.Fatalf("ListSellerFulfillments: %v", err)
	}
	if len(list) != 1 || list[0].SellerID != sellerA {
		t.Errorf("danh sách của seller A = %d đơn, mong đúng 1 của chính A", len(list))
	}
}

// TRẠNG THÁI TỔNG HỢP CỦA ĐƠN suy ra từ tiến độ các nguồn hàng, qua EVENT.
//
// Module order KHÔNG hỏi ngược fulfillment — nó lắng nghe và tự tính. Đây
// là kiểm chứng rằng đường event đó thật sự hoạt động.
func TestTrangThaiDonSuyRaTuTienDoQuaEvent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	fos := h.placeOrder(t, sellerA, sellerB)
	foA, foB := fos[0], fos[1]

	// A đi hết tới lúc bàn giao vận chuyển.
	for _, step := range []func() error{
		func() error { return h.ful.ConfirmFulfillment(ctx, sellerA, foA.ID) },
		func() error { return h.ful.MarkPacked(ctx, sellerA, foA.ID) },
		func() error {
			return h.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: sellerA, FulfillmentID: foA.ID,
				Provider: "GHN", TrackingNumber: "GHN-A-1",
			})
		},
	} {
		if err := step(); err != nil {
			t.Fatalf("bước của seller A: %v", err)
		}
	}

	// Phát event để order tính lại.
	if _, err := h.bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if got := h.orderStatus(t); got != order.StatusPartiallyShipped {
		t.Errorf("trạng thái = %q, mong PARTIALLY_SHIPPED", got)
	}

	// B cũng bàn giao: cả hai đã xuất.
	for _, step := range []func() error{
		func() error { return h.ful.ConfirmFulfillment(ctx, sellerB, foB.ID) },
		func() error { return h.ful.MarkPacked(ctx, sellerB, foB.ID) },
		func() error {
			return h.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: sellerB, FulfillmentID: foB.ID,
				Provider: "GHN", TrackingNumber: "GHN-B-1",
			})
		},
	} {
		if err := step(); err != nil {
			t.Fatalf("bước của seller B: %v", err)
		}
	}
	if _, err := h.bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if got := h.orderStatus(t); got != order.StatusShipped {
		t.Errorf("trạng thái = %q, mong SHIPPED", got)
	}

	// A giao xong trước: đơn phải là PARTIALLY_DELIVERED, KHÔNG phải
	// SHIPPED — hiển thị lùi so với thực tế là lỗi khách nhìn thấy ngay.
	if err := h.ful.MarkDelivered(ctx, sellerA, foA.ID); err != nil {
		t.Fatalf("giao hàng seller A: %v", err)
	}
	if _, err := h.bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if got := h.orderStatus(t); got != order.StatusPartiallyDelivered {
		t.Errorf("trạng thái = %q, mong PARTIALLY_DELIVERED", got)
	}

	// Cả hai giao xong.
	if err := h.ful.MarkDelivered(ctx, sellerB, foB.ID); err != nil {
		t.Fatalf("giao hàng seller B: %v", err)
	}
	if _, err := h.bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if got := h.orderStatus(t); got != order.StatusDelivered {
		t.Errorf("trạng thái = %q, mong DELIVERED", got)
	}
}

// SELLER HỦY PHẦN CỦA MÌNH: đơn chuyển PARTIALLY_CANCELLED.
func TestSellerHuyPhanCuaMinh(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	fos := h.placeOrder(t, sellerA, sellerB)

	var foB string
	for _, fo := range fos {
		if fo.SellerID == sellerB {
			foB = fo.ID
		}
	}

	if err := h.ful.CancelFulfillment(ctx, sellerB, foB, "hết hàng tại kho"); err != nil {
		t.Fatalf("CancelFulfillment: %v", err)
	}
	if _, err := h.bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if got := h.orderStatus(t); got != order.StatusPartiallyCancelled {
		t.Errorf("trạng thái = %q, mong PARTIALLY_CANCELLED", got)
	}

	// Lý do hủy phải đọc lại được: khách cần lời giải thích.
	view, err := h.ful.GetSellerFulfillment(ctx, sellerB, foB)
	if err != nil {
		t.Fatalf("GetSellerFulfillment: %v", err)
	}
	if view.CancelReason != "hết hàng tại kho" {
		t.Errorf("lý do hủy = %q", view.CancelReason)
	}
}

// BÀN GIAO VẬN CHUYỂN lưu mã vận đơn qua vòng ghi–đọc.
func TestBanGiaoLuuMaVanDon(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()
	fos := h.placeOrder(t, sellerID)
	foID := fos[0].ID

	if err := h.ful.ConfirmFulfillment(ctx, sellerID, foID); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := h.ful.MarkPacked(ctx, sellerID, foID); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Thiếu mã vận đơn thì bị chặn.
	err := h.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
		SellerID: sellerID, FulfillmentID: foID, Provider: "GHN",
	})
	if err == nil {
		t.Error("bàn giao không có mã vận đơn phải bị chặn")
	}

	if err := h.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
		SellerID: sellerID, FulfillmentID: foID,
		Provider: "GHN", TrackingNumber: "GHN123456",
	}); err != nil {
		t.Fatalf("HandOverToCarrier: %v", err)
	}

	view, err := h.ful.GetSellerFulfillment(ctx, sellerID, foID)
	if err != nil {
		t.Fatalf("GetSellerFulfillment: %v", err)
	}
	if view.TrackingNumber != "GHN123456" {
		t.Errorf("mã vận đơn = %q, mong GHN123456", view.TrackingNumber)
	}
	if view.ShippingProvider != "GHN" {
		t.Errorf("đơn vị vận chuyển = %q, mong GHN", view.ShippingProvider)
	}
}

// HOÀN TẤT ĐƠN QUÁ HẠN ĐỔI TRẢ — ranh giới TÀI CHÍNH.
//
//	DELIVERED  → số dư seller vẫn Pending
//	COMPLETED  → số dư chuyển Available, seller được chi trả
//
// Chạy sớm nghĩa là trả tiền cho seller trước khi biết khách có hoàn hàng
// không.
func TestHoanTatDonQuaHanDoiTra(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()
	fos := h.placeOrder(t, sellerID)
	foID := fos[0].ID

	for _, step := range []func() error{
		func() error { return h.ful.ConfirmFulfillment(ctx, sellerID, foID) },
		func() error { return h.ful.MarkPacked(ctx, sellerID, foID) },
		func() error {
			return h.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: sellerID, FulfillmentID: foID,
				Provider: "GHN", TrackingNumber: "GHN-1",
			})
		},
		func() error { return h.ful.MarkDelivered(ctx, sellerID, foID) },
	} {
		if err := step(); err != nil {
			t.Fatalf("bước giao hàng: %v", err)
		}
	}

	// Vừa giao xong: CHƯA hoàn tất, tiền seller vẫn bị giữ.
	n, err := h.ful.CompleteDelivered(ctx, 100)
	if err != nil {
		t.Fatalf("CompleteDelivered: %v", err)
	}
	if n != 0 {
		t.Errorf("số đơn hoàn tất = %d, mong 0 — chưa hết hạn đổi trả mà đã "+
			"trả tiền seller nghĩa là trả trước khi biết khách có hoàn hàng không", n)
	}

	// Lùi thời điểm giao về 8 ngày trước (quá hạn 7 ngày).
	if _, err := h.pool.Exec(ctx,
		`UPDATE fulfillment_order SET delivered_at = now() - interval '8 days'
		  WHERE id = $1`, foID); err != nil {
		t.Fatalf("lùi thời điểm giao: %v", err)
	}

	n, err = h.ful.CompleteDelivered(ctx, 100)
	if err != nil {
		t.Fatalf("CompleteDelivered lần 2: %v", err)
	}
	if n != 1 {
		t.Fatalf("số đơn hoàn tất = %d, mong 1", n)
	}

	view, _ := h.ful.GetSellerFulfillment(ctx, sellerID, foID)
	if view.Status != fulfillment.StatusCompleted {
		t.Errorf("trạng thái = %q, mong COMPLETED", view.Status)
	}
	if view.CompletedAt == "" {
		t.Error("COMPLETED phải có mốc thời gian — đó là mốc tính hạn chi trả")
	}
}
