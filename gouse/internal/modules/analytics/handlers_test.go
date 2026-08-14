package analytics_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/analytics"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// newBus dựng bộ phát event có analytics đăng ký nghe.
func newBus(t *testing.T, m *analytics.Module, pool *pgxpool.Pool) *eventbus.Dispatcher {
	t.Helper()

	ctx := context.Background()
	for _, stmt := range []string{"TRUNCATE event_outbox", "TRUNCATE event_processed"} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	bus := eventbus.NewDispatcher(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bus.Subscribe(analytics.NewEventRecorder(m))
	return bus
}

// publishCheckout phát event đặt hàng với các dòng cho trước.
type line struct {
	sellerID  string
	quantity  int
	lineTotal int64
}

func publishCheckout(
	t *testing.T, bus *eventbus.Dispatcher, orderID string, lines []line,
) {
	t.Helper()
	ctx := context.Background()

	reservations := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		reservations = append(reservations, map[string]any{
			"sku_id":     ids.MustNew(ids.PrefixSKU).String(),
			"seller_id":  l.sellerID,
			"quantity":   l.quantity,
			"line_total": l.lineTotal,
		})
	}

	e, err := eventbus.NewEvent(
		eventbus.TypeCheckoutCompleted, eventbus.AggregateCheckout,
		ids.MustNew(ids.PrefixCheckout),
		map[string]any{
			"order_id":     orderID,
			"checkout_id":  ids.MustNew(ids.PrefixCheckout).String(),
			"customer_id":  ids.MustNew(ids.PrefixCustomer).String(),
			"currency":     "VND",
			"reservations": reservations,
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func dispatch(t *testing.T, bus *eventbus.Dispatcher) {
	t.Helper()
	if _, err := bus.DispatchBatch(context.Background(), 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
}

// EVENT ĐẶT HÀNG CHẢY VÀO GMV — mắt xích cuối của kiến trúc event.
//
// Không có handler này, module analytics có đủ hàm tính chỉ số nhưng
// không có dữ liệu nào để tính.
func TestEventDatHangChayVaoGMV(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()

	publishCheckout(t, bus, ids.MustNew(ids.PrefixOrder).String(), []line{
		{sellerID: sellerA, quantity: 2, lineTotal: 600_000},
	})
	dispatch(t, bus)

	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: time.Now().UTC(),
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	got, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricGMV,
		PeriodStart: time.Now().UTC(),
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if got.Value != 600_000 {
		t.Fatalf("GMV = %d, mong 600.000đ — event không chảy vào chỉ số", got.Value)
	}
}

// ĐƠN NHIỀU GIAN HÀNG TÁCH THÀNH NHIỀU SỰ KIỆN.
//
// Một đơn từ hai gian hàng phải cộng vào GMV của ĐÚNG hai gian hàng đó.
// Ghi một sự kiện mang tổng đơn thì dashboard của seller sẽ hiện cả doanh
// số của đối thủ — và không có cách nào tách ra sau.
func TestDonNhieuGianHangTachTheoSeller(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	publishCheckout(t, bus, ids.MustNew(ids.PrefixOrder).String(), []line{
		{sellerID: sellerA, quantity: 1, lineTotal: 300_000},
		{sellerID: sellerB, quantity: 1, lineTotal: 700_000},
	})
	dispatch(t, bus)

	now := time.Now().UTC()
	for _, sellerID := range []string{sellerA, sellerB, ""} {
		if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
			PeriodStart: now,
			Granularity: analytics.GranularityDay,
			SellerID:    sellerID,
		}); err != nil {
			t.Fatalf("ComputeMetrics: %v", err)
		}
	}

	for _, tc := range []struct {
		sellerID string
		want     int64
	}{
		{sellerA, 300_000},
		{sellerB, 700_000},
		{"", 1_000_000},
	} {
		got, err := m.GetMetric(ctx, analytics.MetricRequest{
			Name:        analytics.MetricGMV,
			PeriodStart: now,
			Granularity: analytics.GranularityDay,
			SellerID:    tc.sellerID,
		})
		if err != nil {
			t.Fatalf("GetMetric: %v", err)
		}
		if got.Value != tc.want {
			t.Fatalf("GMV của %q = %d, mong %d", tc.sellerID, got.Value, tc.want)
		}
	}
}

// NHIỀU DÒNG CÙNG GIAN HÀNG LÀ MỘT ĐƠN, KHÔNG PHẢI NHIỀU ĐƠN.
//
// Không gom thì số đơn bị thổi lên theo số dòng, và AOV — vốn là GMV chia
// số đơn — thấp đi đúng theo tỷ lệ đó.
func TestNhieuDongCungGianHangLaMotDon(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()

	publishCheckout(t, bus, ids.MustNew(ids.PrefixOrder).String(), []line{
		{sellerID: sellerA, quantity: 1, lineTotal: 200_000},
		{sellerID: sellerA, quantity: 1, lineTotal: 300_000},
		{sellerID: sellerA, quantity: 2, lineTotal: 400_000},
	})
	dispatch(t, bus)

	now := time.Now().UTC()
	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: now,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	count, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricOrderCount,
		PeriodStart: now,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if count.Value != 1 {
		t.Fatalf("số đơn = %d, mong 1 — ba dòng cùng gian hàng là MỘT đơn",
			count.Value)
	}

	aov, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricAOV,
		PeriodStart: now,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if aov.Value != 900_000 {
		t.Fatalf("AOV = %d, mong 900.000đ — đếm dòng thay vì đơn làm AOV "+
			"thấp đi ba lần", aov.Value)
	}
}

// PHÁT LẠI EVENT KHÔNG CỘNG THÊM VÀO GMV.
//
// Giao ít-nhất-một-lần nghĩa là event SẼ được phát lại. Mỗi lần phát lại
// cộng thêm một đơn nữa vào GMV là số liệu phình lên không ai giải thích
// được.
func TestPhatLaiEventKhongCongThemGMV(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	orderID := ids.MustNew(ids.PrefixOrder).String()

	// Cùng MỘT event, phát lại bằng cách xóa dấu đã-xử-lý.
	publishCheckout(t, bus, orderID, []line{
		{sellerID: sellerA, quantity: 1, lineTotal: 500_000},
	})
	dispatch(t, bus)

	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(ctx, `TRUNCATE event_processed`); err != nil {
			t.Fatalf("dọn dấu đã xử lý: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE event_outbox SET published_at = NULL`); err != nil {
			t.Fatalf("đánh dấu phát lại: %v", err)
		}
		dispatch(t, bus)
	}

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE subject_id = $1`,
		orderID).Scan(&rows); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if rows != 1 {
		t.Fatalf("phát lại 4 lần ghi %d sự kiện, mong 1 — GMV bị cộng %d lần",
			rows, rows)
	}
}

// HAI GIAN HÀNG TRONG MỘT ĐƠN PHẢI CÓ event_id KHÁC NHAU.
//
// Chỉ mục chống trùng là (event_id, event_name). Nếu hai sự kiện của cùng
// một đơn dùng chung event_id, sự kiện thứ hai bị coi là bản trùng và bị
// bỏ — GMV của gian hàng thứ hai BIẾN MẤT.
//
// Test này tách khỏi TestDonNhieuGianHang có chủ ý: test kia so số liệu
// đã tính, còn test này đọc thẳng số hàng trong nhật ký, nên nó chỉ ra
// đúng chỗ hỏng.
func TestHaiGianHangCoEventIDKhacNhau(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	orderID := ids.MustNew(ids.PrefixOrder).String()
	publishCheckout(t, bus, orderID, []line{
		{sellerID: ids.MustNew(ids.PrefixSeller).String(), quantity: 1, lineTotal: 300_000},
		{sellerID: ids.MustNew(ids.PrefixSeller).String(), quantity: 1, lineTotal: 700_000},
	})
	dispatch(t, bus)

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE subject_id = $1`,
		orderID).Scan(&rows); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if rows != 2 {
		t.Fatalf("ghi %d sự kiện cho đơn hai gian hàng, mong 2 — gian hàng "+
			"thứ hai bị coi là bản trùng và GMV của họ biến mất", rows)
	}

	var distinct int
	if err := pool.QueryRow(ctx,
		`SELECT count(DISTINCT event_id) FROM event_log WHERE subject_id = $1`,
		orderID).Scan(&distinct); err != nil {
		t.Fatalf("đếm event_id: %v", err)
	}
	if distinct != 2 {
		t.Fatalf("có %d event_id khác nhau, mong 2", distinct)
	}
}

// CHỈ GHI MỐC DELIVERED, bỏ qua các bước nội bộ của seller.
//
// Vòng đời giao hàng có chín trạng thái, phần lớn là bước nội bộ. Ghi hết
// là nhồi nhật ký bằng dữ liệu không ai hỏi tới — và khối lượng đó phải
// trả giá ở mọi truy vấn chỉ số sau này.
func TestChiGhiMocDelivered(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	orderID := ids.MustNew(ids.PrefixOrder).String()

	publishProgress := func(status string) {
		t.Helper()
		e, err := eventbus.NewEvent(
			eventbus.TypeFulfillmentProgress, eventbus.AggregateFulfillment,
			ids.MustNew(ids.PrefixFulfillmentOrder),
			map[string]any{
				"order_id":       orderID,
				"fulfillment_id": ids.MustNew(ids.PrefixFulfillmentOrder).String(),
				"new_status":     status,
			})
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		if err := bus.Outbox().Publish(ctx, e); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	for _, st := range []string{
		"ALLOCATED", "CONFIRMED", "PICKING", "PACKED", "HANDED_OVER", "IN_TRANSIT",
	} {
		publishProgress(st)
	}
	dispatch(t, bus)

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE event_name = $1`,
		analytics.EventDelivered).Scan(&rows); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if rows != 0 {
		t.Fatalf("ghi %d sự kiện cho các bước nội bộ, mong 0", rows)
	}

	publishProgress("DELIVERED")
	dispatch(t, bus)

	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE event_name = $1`,
		analytics.EventDelivered).Scan(&rows); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if rows != 1 {
		t.Fatalf("mốc DELIVERED ghi %d sự kiện, mong 1", rows)
	}
}

// ĐỌC ĐÚNG TÊN TRƯỜNG `new_status`.
//
// Bên phát dùng `new_status`, không phải `status`. Đọc sai tên thì JSON
// KHÔNG lỗi — nó trả về chuỗi rỗng, và MỌI mốc giao hàng lặng lẽ bị bỏ
// qua. Đây là loại lỗi không có gì báo.
//
// Test trên (TestChiGhiMocDelivered) không bắt được một mình: nếu đọc sai
// tên, cả sáu bước nội bộ VÀ mốc DELIVERED đều bị bỏ — phần đầu của test
// đó vẫn xanh. Test này chốt phần còn lại.
func TestDocDungTenTruongNewStatus(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	e, err := eventbus.NewEvent(
		eventbus.TypeFulfillmentProgress, eventbus.AggregateFulfillment,
		ids.MustNew(ids.PrefixFulfillmentOrder),
		map[string]any{
			"order_id":   ids.MustNew(ids.PrefixOrder).String(),
			"new_status": "DELIVERED",
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	dispatch(t, bus)

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE event_name = $1`,
		analytics.EventDelivered).Scan(&rows); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if rows != 1 {
		t.Fatalf("ghi %d sự kiện, mong 1 — đọc sai tên trường thì mọi mốc "+
			"giao hàng lặng lẽ bị bỏ qua mà không có gì báo", rows)
	}
}

// EVENT THÊM GIỎ CHẢY VÀO PHỄU CHUYỂN ĐỔI.
func TestEventThemGioChayVaoPheu(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	cartID := ids.MustNew(ids.PrefixCart).String()
	skuID := ids.MustNew(ids.PrefixSKU).String()
	sellerID := ids.MustNew(ids.PrefixSeller).String()

	e, err := eventbus.NewEvent(
		eventbus.TypeCartItemAdded, eventbus.AggregateCart,
		ids.ID(cartID),
		map[string]any{
			"cart_id":   cartID,
			"sku_id":    skuID,
			"seller_id": sellerID,
			"quantity":  2,
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	dispatch(t, bus)

	var (
		gotSession, gotSubject, gotSeller string
	)
	if err := pool.QueryRow(ctx, `
		SELECT session_id, subject_id, seller_id FROM event_log
		 WHERE event_name = $1`, analytics.EventAddToCart).
		Scan(&gotSession, &gotSubject, &gotSeller); err != nil {
		t.Fatalf("đọc sự kiện: %v", err)
	}

	// session_id phải là id giỏ hàng — đó là thứ nối các bước của MỘT lượt
	// mua sắm, và tỷ lệ chuyển đổi đếm theo đơn vị đó.
	if gotSession != cartID {
		t.Fatalf("session_id = %q, mong id giỏ hàng %q", gotSession, cartID)
	}
	if gotSubject != skuID {
		t.Fatalf("subject_id = %q, mong sku_id %q", gotSubject, skuID)
	}
	if gotSeller != sellerID {
		t.Fatalf("seller_id = %q, mong %q", gotSeller, sellerID)
	}
}

// EVENT KHÔNG QUAN TÂM KHÔNG LÀM HỎNG GÌ.
//
// Handler nhận cả loại event mình không đăng ký (dispatcher lọc theo
// EventTypes, nhưng lớp phòng vệ này vẫn cần): trả nil chứ không lỗi, để
// event không kẹt lại trong hàng đợi và chặn mọi event sau nó.
func TestEventKhongQuanTamKhongLamHongGi(t *testing.T) {
	m, pool := newModule(t, newClock())
	bus := newBus(t, m, pool)
	ctx := context.Background()

	e, err := eventbus.NewEvent(
		eventbus.TypeOrderCancelled, eventbus.AggregateOrder,
		ids.MustNew(ids.PrefixOrder),
		map[string]any{"order_id": ids.MustNew(ids.PrefixOrder).String()})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	var dead int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_outbox WHERE dead_lettered_at IS NOT NULL`).
		Scan(&dead); err != nil {
		t.Fatalf("đếm event lỗi: %v", err)
	}
	if dead != 0 {
		t.Fatalf("%d event rơi vào dead letter", dead)
	}
}
