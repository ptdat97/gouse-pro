package notification_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/notification"
	"github.com/fashion-commerce/platform/internal/modules/notification/domain"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// recordingSender ghi lại mọi lần gửi để test kiểm chứng.
type recordingSender struct {
	mu   sync.Mutex
	sent []*domain.Notification

	// failTimes là số lần đầu tiên trả lỗi, để thử nhánh thất bại.
	failTimes int
	calls     int
}

func (s *recordingSender) Channel() domain.Channel { return domain.ChannelEmail }

func (s *recordingSender) Send(
	_ context.Context, n *domain.Notification,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls++
	if s.calls <= s.failTimes {
		return "", errors.New("giả lập nhà cung cấp lỗi")
	}

	s.sent = append(s.sent, n)
	return "provider-msg-" + n.Template(), nil
}

func (s *recordingSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

func (s *recordingSender) last() *domain.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sent) == 0 {
		return nil
	}
	return s.sent[len(s.sent)-1]
}

func newModule(t *testing.T, sender *recordingSender) (*notification.Module, *pgxpool.Pool) {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE notification_log",
		"TRUNCATE event_processed",
		"TRUNCATE event_outbox",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	cfg := notification.Config{
		Storage: "postgres",
		DB:      db,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if sender != nil {
		cfg.Senders = []domain.Sender{sender}
	}

	m, err := notification.New(cfg)
	if err != nil {
		t.Fatalf("notification.New: %v", err)
	}
	return m, db.Pool()
}

func sendReq(eventID, template, recipient string) notification.SendRequest {
	return notification.SendRequest{
		EventID:       eventID,
		Channel:       notification.ChannelEmail,
		Category:      notification.CategoryTransactional,
		Template:      template,
		Recipient:     recipient,
		Subject:       "Xác nhận đơn hàng",
		Body:          "Cảm ơn bạn đã đặt hàng.",
		ReferenceType: "order",
		ReferenceID:   ids.MustNew(ids.PrefixOrder).String(),
	}
}

// GỬI VÀ GHI LOG: mọi lần gửi đều để lại dấu vết.
//
// Bộ phận hỗ trợ cần trả lời "chúng tôi đã gửi email tới đâu" khi khách
// nói không nhận được.
func TestGuiVaGhiLog(t *testing.T) {
	sender := &recordingSender{}
	m, _ := newModule(t, sender)
	ctx := context.Background()

	req := sendReq("evt-1", notification.TemplateOrderConfirmed, "khach@example.com")
	if err := m.Send(ctx, req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if sender.count() != 1 {
		t.Fatalf("số lần gửi = %d, mong 1", sender.count())
	}

	logs, err := m.GetHistory(ctx, "order", req.ReferenceID)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("số bản ghi = %d, mong 1", len(logs))
	}
	if logs[0].Status != notification.StatusSent {
		t.Errorf("trạng thái = %q, mong SENT", logs[0].Status)
	}
	if logs[0].ProviderMessageID == "" {
		t.Error("phải lưu mã tra cứu của nhà cung cấp — cần nó khi khách khiếu nại")
	}
	if logs[0].SentAt == "" {
		t.Error("phải có mốc thời gian gửi")
	}
}

// GỬI HAI LẦN CÙNG MỘT EVENT chỉ gửi MỘT email.
//
// Mô hình at-least-once nghĩa là event SẼ được phát lại. Khách nhận ba
// email "đơn đã đặt" sẽ gọi tổng đài hỏi mình bị tính tiền mấy lần.
func TestGuiHaiLanChiMotEmail(t *testing.T) {
	sender := &recordingSender{}
	m, _ := newModule(t, sender)
	ctx := context.Background()

	req := sendReq("evt-trung", notification.TemplateOrderConfirmed, "khach@example.com")

	if err := m.Send(ctx, req); err != nil {
		t.Fatalf("lần gửi 1: %v", err)
	}
	if err := m.Send(ctx, req); err != nil {
		t.Fatalf("lần gửi 2 phải thành công (bỏ qua im lặng): %v", err)
	}

	if sender.count() != 1 {
		t.Errorf("số email đã gửi = %d, mong ĐÚNG 1 — gửi trùng làm khách "+
			"tưởng bị tính tiền nhiều lần", sender.count())
	}
}

// GỬI SONG SONG cùng một event cũng chỉ ra MỘT email.
//
// Kiểm tra trước khi ghi không chặn được: hai worker cùng đọc thấy "chưa
// gửi" rồi cùng gửi. Ràng buộc UNIQUE ở database là thứ chặn thật.
func TestGuiSongSongChiMotEmail(t *testing.T) {
	sender := &recordingSender{}
	m, _ := newModule(t, sender)
	ctx := context.Background()

	req := sendReq("evt-song-song", notification.TemplateOrderConfirmed, "khach@example.com")

	const n = 10
	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = m.Send(ctx, req)
		}()
	}
	close(start)
	wg.Wait()

	if sender.count() != 1 {
		t.Errorf("số email đã gửi = %d, mong ĐÚNG 1 — %d request song song "+
			"mà gửi nhiều lần nghĩa là chỉ mục UNIQUE không chặn được",
			sender.count(), n)
	}
}

// THIẾU ĐỊA CHỈ thì BỎ QUA, ghi log, KHÔNG báo lỗi.
//
// Khách vãng lai không nhập email là chuyện bình thường. Trả lỗi sẽ khiến
// event bị thử lại vô ích rồi rơi vào dead letter, và cảnh báo vận hành
// kêu vì việc hoàn toàn bình thường.
func TestThieuDiaChiThiBoQuaKhongBaoLoi(t *testing.T) {
	sender := &recordingSender{}
	m, _ := newModule(t, sender)
	ctx := context.Background()

	req := sendReq("evt-khong-email", notification.TemplateOrderConfirmed, "")
	if err := m.Send(ctx, req); err != nil {
		t.Fatalf("thiếu địa chỉ KHÔNG được báo lỗi: %v", err)
	}

	if sender.count() != 0 {
		t.Errorf("số email đã gửi = %d, mong 0", sender.count())
	}

	// VẪN GHI LOG: khách hỏi "sao tôi không nhận được email" thì phải trả
	// lời được. Không ghi gì nghĩa là không phân biệt được "hệ thống quyết
	// định không gửi" với "hệ thống quên gửi".
	logs, err := m.GetHistory(ctx, "order", req.ReferenceID)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("số bản ghi = %d, mong 1 — bỏ qua vẫn phải ghi log", len(logs))
	}
	if logs[0].Status != notification.StatusSkipped {
		t.Errorf("trạng thái = %q, mong SKIPPED", logs[0].Status)
	}
	if logs[0].SkipReason == "" {
		t.Error("phải ghi lý do bỏ qua")
	}
}

// SKIPPED KHÁC FAILED.
//
// Bỏ qua là quyết định CÓ CHỦ Ý; thất bại là sự cố. Gộp chung sẽ làm cảnh
// báo vận hành kêu vì những việc bình thường, và rồi không ai đọc cảnh báo.
func TestPhanBietBoQuaVaThatBai(t *testing.T) {
	// Nhà cung cấp luôn lỗi.
	sender := &recordingSender{failTimes: 100}
	m, _ := newModule(t, sender)
	ctx := context.Background()

	req := sendReq("evt-that-bai", notification.TemplateOrderConfirmed, "khach@example.com")

	// Thất bại thì TRẢ LỖI — bên gọi cần biết để thử lại.
	if err := m.Send(ctx, req); err == nil {
		t.Error("nhà cung cấp lỗi phải trả lỗi để bên gọi thử lại")
	}

	logs, _ := m.GetHistory(ctx, "order", req.ReferenceID)
	if len(logs) != 1 {
		t.Fatalf("số bản ghi = %d, mong 1", len(logs))
	}
	if logs[0].Status != notification.StatusFailed {
		t.Errorf("trạng thái = %q, mong FAILED", logs[0].Status)
	}
	if logs[0].Error == "" {
		t.Error("phải ghi lỗi để người vận hành biết vì sao")
	}

	counts, _ := m.CountByStatus(ctx)
	if counts[notification.StatusFailed] != 1 {
		t.Errorf("số FAILED = %d, mong 1", counts[notification.StatusFailed])
	}
	if counts[notification.StatusSkipped] != 0 {
		t.Errorf("số SKIPPED = %d, mong 0 — thất bại KHÔNG phải bỏ qua",
			counts[notification.StatusSkipped])
	}
}

// EVENT ĐẶT HÀNG sinh email xác nhận, dữ liệu lấy TỪ PAYLOAD.
//
// Module này KHÔNG gọi module nghiệp vụ nào — tên sản phẩm và email đều
// đến từ payload event.
func TestEventDatHangSinhEmailXacNhan(t *testing.T) {
	sender := &recordingSender{}
	m, pool := newModule(t, sender)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(notification.NewOrderNotifier(m, log))

	orderID := ids.MustNew(ids.PrefixOrder).String()
	e, err := eventbus.NewEvent(
		eventbus.TypeCheckoutCompleted, eventbus.AggregateCheckout,
		ids.MustNew(ids.PrefixCheckout),
		map[string]any{
			"order_id":     orderID,
			"order_number": "FC-2026-08-001234",
			"guest_email":  "khach@example.com",
			"currency":     "VND",
			"reservations": []map[string]any{
				{"product_name": "Áo sơ mi linen", "quantity": 2, "line_total": 598000},
				{"product_name": "Quần âu", "quantity": 1, "line_total": 450000},
			},
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	if sender.count() != 1 {
		t.Fatalf("số email = %d, mong 1", sender.count())
	}

	sent := sender.last()
	if sent.Recipient() != "khach@example.com" {
		t.Errorf("người nhận = %q, mong khach@example.com", sent.Recipient())
	}

	// Nội dung phải chứa mã đơn và TÊN SẢN PHẨM ĐÃ ĐÓNG BĂNG.
	//
	// Tên đến từ payload chứ không tra lại: seller đổi tên sau khi khách
	// mua thì email vẫn phải khớp thứ khách đã thấy lúc đặt.
	body := sent.Body()
	for _, want := range []string{"FC-2026-08-001234", "Áo sơ mi linen", "Quần âu"} {
		if !contains(body, want) {
			t.Errorf("nội dung email thiếu %q", want)
		}
	}

	// Tổng tiền hiển thị cho người đọc: 598.000 + 450.000 = 1.048.000
	if !contains(body, "1.048.000") {
		t.Errorf("nội dung thiếu tổng tiền đã định dạng, có: %s", body)
	}
}

// EVENT GIAO HÀNG sinh email theo BA MỐC, không phải mọi bước.
//
// Gửi email cho mọi bước (đã xác nhận, đang lấy hàng, đã đóng gói) là làm
// phiền khách — họ chỉ quan tâm ba mốc: hàng đã đi, đã tới, hoặc bị hủy.
func TestChiGuiEmailOBaMocGiaoHang(t *testing.T) {
	sender := &recordingSender{}
	m, pool := newModule(t, sender)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(notification.NewOrderNotifier(m, log))

	orderID := ids.MustNew(ids.PrefixOrder).String()

	publish := func(status string) {
		t.Helper()
		e, err := eventbus.NewEvent(
			eventbus.TypeFulfillmentProgress, eventbus.AggregateFulfillment,
			ids.MustNew(ids.PrefixFulfillmentOrder),
			map[string]any{
				"order_id":        orderID,
				"fo_number":       "FC-2026-08-001234-A",
				"new_status":      status,
				"tracking_number": "GHN123456",
				"email":           "khach@example.com",
			})
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		if err := bus.Outbox().Publish(ctx, e); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// Ba bước KHÔNG sinh email.
	for _, st := range []string{"ALLOCATED", "CONFIRMED", "PICKING", "PACKED"} {
		publish(st)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if sender.count() != 0 {
		t.Errorf("số email sau các bước nội bộ = %d, mong 0 — gửi email cho "+
			"mọi bước là làm phiền khách", sender.count())
	}

	// HANDED_OVER sinh email "đã gửi", kèm mã vận đơn.
	publish("HANDED_OVER")
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if sender.count() != 1 {
		t.Fatalf("số email sau khi bàn giao = %d, mong 1", sender.count())
	}
	if !contains(sender.last().Body(), "GHN123456") {
		t.Error("email báo gửi hàng phải có mã vận đơn — không có nó thì " +
			"khách không tra được hàng đang ở đâu")
	}

	// DELIVERED sinh email "đã giao".
	publish("DELIVERED")
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}
	if sender.count() != 2 {
		t.Errorf("số email sau khi giao = %d, mong 2", sender.count())
	}
	if sender.last().Template() != notification.TemplateOrderDelivered {
		t.Errorf("mẫu = %q, mong order_delivered", sender.last().Template())
	}
}

// PHÁT LẠI EVENT không gửi email lần hai.
func TestPhatLaiEventKhongGuiHaiLan(t *testing.T) {
	sender := &recordingSender{}
	m, pool := newModule(t, sender)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(notification.NewOrderNotifier(m, log))

	e, err := eventbus.NewEvent(
		eventbus.TypeCheckoutCompleted, eventbus.AggregateCheckout,
		ids.MustNew(ids.PrefixCheckout),
		map[string]any{
			"order_id":     ids.MustNew(ids.PrefixOrder).String(),
			"order_number": "FC-2026-08-000001",
			"guest_email":  "khach@example.com",
			"reservations": []map[string]any{
				{"product_name": "Áo sơ mi", "quantity": 1, "line_total": 299000},
			},
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("lượt 1: %v", err)
	}

	// Ép phát lại.
	if _, err := pool.Exec(ctx, `UPDATE event_outbox SET published_at = NULL`); err != nil {
		t.Fatalf("ép phát lại: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("lượt 2: %v", err)
	}

	if sender.count() != 1 {
		t.Errorf("số email = %d, mong ĐÚNG 1 sau khi phát lại", sender.count())
	}
}

// KHÔNG CẤU HÌNH BỘ GỬI thì dùng bộ ghi-log, không lỗi.
//
// Luồng nghiệp vụ phải chạy được đầu-cuối trước khi nền tảng ký hợp đồng
// với nhà cung cấp dịch vụ email.
func TestKhongCauHinhBoGuiVanChayDuoc(t *testing.T) {
	m, _ := newModule(t, nil)
	ctx := context.Background()

	req := sendReq("evt-log", notification.TemplateOrderConfirmed, "khach@example.com")
	if err := m.Send(ctx, req); err != nil {
		t.Fatalf("bộ ghi-log phải gửi được: %v", err)
	}

	logs, _ := m.GetHistory(ctx, "order", req.ReferenceID)
	if len(logs) != 1 || logs[0].Status != notification.StatusSent {
		t.Errorf("trạng thái = %v, mong SENT", logs)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
