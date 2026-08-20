package eventbus_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// recordingHandler ghi lại mọi event nhận được.
type recordingHandler struct {
	mu       sync.Mutex
	name     string
	types    []string
	received []ids.ID

	// failTimes là số lần đầu tiên sẽ trả lỗi, để thử cơ chế thử lại.
	failTimes int
	calls     int
}

func (h *recordingHandler) Name() string         { return h.name }
func (h *recordingHandler) EventTypes() []string { return h.types }

func (h *recordingHandler) Handle(ctx context.Context, e eventbus.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.calls++
	if h.calls <= h.failTimes {
		return errors.New("giả lập lỗi tạm thời")
	}

	// Bên nhận PHẢI ghi bằng giao dịch của dispatcher — kiểm tra luôn ở
	// đây để test chứng minh cơ chế đó hoạt động.
	if _, err := eventbus.MustTxFrom(ctx); err != nil {
		return err
	}

	h.received = append(h.received, e.ID)
	return nil
}

func (h *recordingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.received)
}

func (h *recordingHandler) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func newBus(t *testing.T) (*eventbus.Dispatcher, *pgxpool.Pool) {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE event_processed",
		"TRUNCATE event_outbox",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return eventbus.NewDispatcher(db.Pool(), log), db.Pool()
}

func newEvent(t *testing.T, eventType string) eventbus.Event {
	t.Helper()
	e, err := eventbus.NewEvent(eventType, eventbus.AggregateOrder,
		ids.MustNew(ids.PrefixOrder), map[string]any{"total": 299000})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	return e
}

// GIAO DỊCH THẤT BẠI thì event KHÔNG BAO GIỜ được phát.
//
// Đây là nửa quan trọng nhất của Transactional Outbox. Không có nó, hệ
// thống sẽ xử lý những sự thật chưa từng xảy ra: gửi email xác nhận cho
// đơn hàng mà giao dịch đã rollback.
func TestGiaoDichThatBaiThiEventKhongDuocPhat(t *testing.T) {
	bus, pool := newBus(t)
	ctx := context.Background()

	h := &recordingHandler{name: "test.handler", types: []string{"order.placed"}}
	bus.Subscribe(h)

	// Mở giao dịch, ghi event, rồi ROLLBACK.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := bus.Outbox().PublishTx(ctx, tx, newEvent(t, "order.placed")); err != nil {
		t.Fatalf("PublishTx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	n, err := bus.DispatchBatch(ctx, 100)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if n != 0 {
		t.Errorf("số event đã phát = %d, mong 0 — giao dịch rollback mà "+
			"event vẫn phát nghĩa là bên nhận xử lý sự thật chưa xảy ra", n)
	}
	if h.count() != 0 {
		t.Errorf("bên nhận nhận %d event, mong 0", h.count())
	}
}

// GIAO DỊCH THÀNH CÔNG thì event CHẮC CHẮN được phát.
func TestGiaoDichThanhCongThiEventDuocPhat(t *testing.T) {
	bus, pool := newBus(t)
	ctx := context.Background()

	h := &recordingHandler{name: "test.handler", types: []string{"order.placed"}}
	bus.Subscribe(h)

	tx, _ := pool.Begin(ctx)
	e := newEvent(t, "order.placed")
	if err := bus.Outbox().PublishTx(ctx, tx, e); err != nil {
		t.Fatalf("PublishTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	n, err := bus.DispatchBatch(ctx, 100)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if n != 1 {
		t.Fatalf("số event đã phát = %d, mong 1", n)
	}
	if h.count() != 1 {
		t.Fatalf("bên nhận nhận %d event, mong 1", h.count())
	}
	if h.received[0] != e.ID {
		t.Errorf("event nhận được = %s, mong %s", h.received[0], e.ID)
	}
}

// PHÁT LẠI KHÔNG XỬ LÝ HAI LẦN — cưỡng chế idempotency.
//
// Mô hình at-least-once nghĩa là event SẼ được phát nhiều lần. Bảng
// event_processed là thứ đảm bảo bên nhận chỉ chạy một lần.
func TestPhatLaiKhongXuLyHaiLan(t *testing.T) {
	bus, pool := newBus(t)
	ctx := context.Background()

	h := &recordingHandler{name: "test.handler", types: []string{"order.placed"}}
	bus.Subscribe(h)

	e := newEvent(t, "order.placed")
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("lượt phát 1: %v", err)
	}

	if h.count() != 1 {
		t.Fatalf("sau lượt 1: nhận %d event, mong 1", h.count())
	}

	// Ép phát lại CHÍNH event đó: mô phỏng tình huống worker chết sau khi
	// handler chạy xong nhưng trước khi kịp đánh dấu published_at.
	//
	// Đây là đường đi thật của mô hình at-least-once, không phải tình
	// huống giả tưởng.
	if _, err := pool.Exec(ctx,
		`UPDATE event_outbox SET published_at = NULL WHERE event_id = $1`,
		e.ID.String()); err != nil {
		t.Fatalf("ép phát lại: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("lượt phát 2: %v", err)
	}

	if h.count() != 1 {
		t.Errorf("bên nhận xử lý %d lần, mong ĐÚNG 1 — xử lý hai lần với "+
			"tiền nghĩa là ghi sổ hai lần", h.count())
	}
}

// NHIỀU BÊN NHẬN cho cùng một event, mỗi bên xử lý ĐỘC LẬP.
//
// notification đã xử lý không có nghĩa payment cũng đã xử lý — khóa
// idempotency phải là cặp (event_id, handler), không phải chỉ event_id.
func TestNhieuBenNhanXuLyDocLap(t *testing.T) {
	bus, _ := newBus(t)
	ctx := context.Background()

	a := &recordingHandler{name: "handler.a", types: []string{"order.placed"}}
	b := &recordingHandler{name: "handler.b", types: []string{"order.placed"}}
	bus.Subscribe(a)
	bus.Subscribe(b)

	if err := bus.Outbox().Publish(ctx, newEvent(t, "order.placed")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if a.count() != 1 || b.count() != 1 {
		t.Errorf("A nhận %d, B nhận %d — mong mỗi bên 1", a.count(), b.count())
	}
}

// BÊN NHẬN LỖI thì event được THỬ LẠI, không mất.
func TestBenNhanLoiThiThuLai(t *testing.T) {
	bus, _ := newBus(t)
	ctx := context.Background()

	// Lỗi hai lần đầu, lần thứ ba thành công.
	h := &recordingHandler{
		name: "test.handler", types: []string{"order.placed"}, failTimes: 2,
	}
	bus.Subscribe(h)

	if err := bus.Outbox().Publish(ctx, newEvent(t, "order.placed")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i := 1; i <= 3; i++ {
		if _, err := bus.DispatchBatch(ctx, 100); err != nil {
			t.Fatalf("lượt %d: %v", i, err)
		}
	}

	if h.callCount() != 3 {
		t.Errorf("số lần gọi = %d, mong 3 (2 lần lỗi + 1 lần thành công)",
			h.callCount())
	}
	if h.count() != 1 {
		t.Errorf("số event xử lý thành công = %d, mong 1", h.count())
	}
}

// LỖI VĨNH VIỄN thì chuyển DEAD LETTER, không thử lại vô hạn.
//
// Một event hỏng thử lại mãi sẽ chặn hàng đợi và làm mọi event sau nó
// không bao giờ được phát.
func TestLoiVinhVienThiChuyenDeadLetter(t *testing.T) {
	bus, _ := newBus(t)
	ctx := context.Background()

	// Luôn lỗi.
	h := &recordingHandler{
		name: "test.handler", types: []string{"order.placed"}, failTimes: 1000,
	}
	bus.Subscribe(h)

	if err := bus.Outbox().Publish(ctx, newEvent(t, "order.placed")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Chạy nhiều lượt hơn ngưỡng.
	for i := 0; i < 10; i++ {
		if _, err := bus.DispatchBatch(ctx, 100); err != nil {
			t.Fatalf("lượt %d: %v", i, err)
		}
	}

	stats, err := bus.Outbox().Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.DeadLettered != 1 {
		t.Errorf("số event dead letter = %d, mong 1 — event hỏng phải bị "+
			"bỏ cuộc, không chặn hàng đợi mãi", stats.DeadLettered)
	}
	if stats.Pending != 0 {
		t.Errorf("số event chờ = %d, mong 0", stats.Pending)
	}

	// Số lần gọi phải DỪNG LẠI ở ngưỡng, không tiếp tục sau khi dead letter.
	calls := h.callCount()
	if calls > 5 {
		t.Errorf("số lần thử = %d, mong không quá 5 — thử lại vô hạn làm "+
			"kẹt hàng đợi", calls)
	}
}

// KHÔNG AI NGHE cũng là kết quả HỢP LỆ.
//
// Bên phát không biết và không cần biết có ai nghe hay không. Event không
// có bên nhận phải được đánh dấu đã phát, nếu không nó kẹt lại chặn hàng đợi.
func TestKhongAiNgheVanDanhDauDaPhat(t *testing.T) {
	bus, _ := newBus(t)
	ctx := context.Background()

	if err := bus.Outbox().Publish(ctx, newEvent(t, "loai.khong.ai.nghe")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	n, err := bus.DispatchBatch(ctx, 100)
	if err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if n != 1 {
		t.Errorf("số event xử lý = %d, mong 1", n)
	}

	stats, _ := bus.Outbox().Stats(ctx)
	if stats.Pending != 0 {
		t.Errorf("còn %d event chờ, mong 0 — event không ai nghe sẽ kẹt "+
			"lại chặn mọi event sau nó", stats.Pending)
	}
}

// THỨ TỰ PHÁT theo thứ tự GHI.
func TestPhatTheoThuTuGhi(t *testing.T) {
	bus, _ := newBus(t)
	ctx := context.Background()

	h := &recordingHandler{name: "test.handler", types: []string{"order.placed"}}
	bus.Subscribe(h)

	var want []ids.ID
	for i := 0; i < 5; i++ {
		e := newEvent(t, "order.placed")
		if err := bus.Outbox().Publish(ctx, e); err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
		want = append(want, e.ID)
	}

	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if len(h.received) != 5 {
		t.Fatalf("nhận %d event, mong 5", len(h.received))
	}
	for i := range want {
		if h.received[i] != want[i] {
			t.Errorf("event thứ %d = %s, mong %s", i, h.received[i], want[i])
		}
	}
}

// CHỈ SỐ GIÁM SÁT phản ánh đúng trạng thái.
func TestChiSoGiamSat(t *testing.T) {
	bus, _ := newBus(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := bus.Outbox().Publish(ctx, newEvent(t, "order.placed")); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	stats, err := bus.Outbox().Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Pending != 3 {
		t.Errorf("số chờ = %d, mong 3", stats.Pending)
	}
	if stats.OldestPendingAge <= 0 {
		t.Error("độ trễ event cũ nhất phải lớn hơn 0 — đây là chỉ báo " +
			"bộ đọc còn sống hay đã chết")
	}

	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	after, _ := bus.Outbox().Stats(ctx)
	if after.Pending != 0 {
		t.Errorf("sau khi phát: số chờ = %d, mong 0", after.Pending)
	}
}

// TestCorrelationLayTuNguCanhKhiBenPhatKhongDat — PH-22.
//
// # Vì sao mặc định ở outbox chứ không bắt mỗi bên phát tự nhớ
//
// Bắt từng bên phát gọi `WithTrace` là cách chắc chắn để có một nửa số
// event thiếu nó. Trước 20/08 đúng như vậy: chỉ 2 trong 4 nơi phát có gọi,
// `cart` và `product` thì không.
//
// Một trường truy vết chỉ có giá trị khi nó CÓ MẶT ở mọi mắt xích. Thiếu
// một mắt là chuỗi đứt, và chuỗi đứt thì không lần được gì — kể cả những
// mắt còn nguyên.
func TestCorrelationLayTuNguCanhKhiBenPhatKhongDat(t *testing.T) {
	_, pool := newBus(t)
	outbox := eventbus.NewOutbox(pool)

	const requestID = "req_01J9XABC123DEF456GHJKMNPQR"
	ctx := logger.WithRequestID(context.Background(), requestID)

	// Bên phát KHÔNG gọi WithTrace — đúng như cart và product đang làm.
	e, err := eventbus.NewEvent(
		"cart.item_added", eventbus.AggregateCart,
		ids.MustNew(ids.PrefixCart), map[string]any{"sku_id": "sku_x"})
	if err != nil {
		t.Fatalf("dựng event: %v", err)
	}
	if e.CorrelationID != "" {
		t.Fatalf("dựng sai: event đã có correlation %q", e.CorrelationID)
	}

	if err := outbox.Publish(ctx, e); err != nil {
		t.Fatalf("phát event: %v", err)
	}

	var corr string
	if err := pool.QueryRow(context.Background(),
		`SELECT coalesce(correlation_id, '') FROM event_outbox WHERE event_id = $1`,
		e.ID.String()).Scan(&corr); err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}

	if corr != requestID {
		t.Errorf("correlation_id = %q, cần %q — chuỗi truy vết bị đứt ngay "+
			"tại bên phát không tự đặt", corr, requestID)
	}
}

// TestBenPhatTuDatThiGiuNguyen: giá trị bên phát đặt có Ý NGHĨA NGHIỆP VỤ
// (thường là mã đơn), nên nó phải thắng mã request.
//
// Mã đơn nối được cả những việc xảy ra nhiều ngày sau — lúc request HTTP
// ban đầu đã kết thúc từ lâu.
func TestBenPhatTuDatThiGiuNguyen(t *testing.T) {
	_, pool := newBus(t)
	outbox := eventbus.NewOutbox(pool)

	ctx := logger.WithRequestID(context.Background(), "req_khac")

	orderID := ids.MustNew(ids.PrefixOrder).String()
	e, err := eventbus.NewEvent(
		"checkout.completed", eventbus.AggregateCheckout,
		ids.MustNew(ids.PrefixCheckout), map[string]any{"order_id": orderID})
	if err != nil {
		t.Fatalf("dựng event: %v", err)
	}
	e = e.WithTrace(orderID, "")

	if err := outbox.Publish(ctx, e); err != nil {
		t.Fatalf("phát event: %v", err)
	}

	var corr string
	if err := pool.QueryRow(context.Background(),
		`SELECT coalesce(correlation_id, '') FROM event_outbox WHERE event_id = $1`,
		e.ID.String()).Scan(&corr); err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}
	if corr != orderID {
		t.Errorf("correlation_id = %q, cần mã đơn %q", corr, orderID)
	}
}
