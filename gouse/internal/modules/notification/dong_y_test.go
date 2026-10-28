package notification_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/modules/notification"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// fakeCustomer trả lời câu hỏi đồng ý theo kịch bản của từng bài.
type fakeCustomer struct {
	consent map[string]bool
	failure error

	// daHoi ghi lại loại đồng ý đã bị hỏi, để bài test kiểm ĐÚNG loại.
	daHoi []string
}

func (k *fakeCustomer) CustomerEmail(context.Context, string) (string, error) {
	return "customer@example.com", nil
}

func (k *fakeCustomer) HasConsent(_ context.Context, _, kind string) (bool, error) {
	k.daHoi = append(k.daHoi, kind)
	if k.failure != nil {
		return false, k.failure
	}
	return k.consent[kind], nil
}

// TestMarketingStopsWithoutConsent.
//
// # Vì sao bài này tồn tại
//
// `Category.RequiresConsent()` được viết cẩn thận, kèm chú thích giải
// thích lý do pháp lý — và cho tới 17/09 KHÔNG DÒNG MÃ NÀO GỌI NÓ. Đúng
// dạng lỗi mục 8 của backlog: một quy tắc đúng nằm sau một hàm không ai
// gọi.
//
// docs/04-modules/notification.md mục 5 gọi việc nhầm hai loại thông báo
// là "vi phạm pháp luật ở nhiều thị trường". Hôm nay hệ thống chưa gửi
// thư marketing nào, nên rủi ro còn tiềm ẩn — nhưng lá thư marketing ĐẦU
// TIÊN sẽ đi thẳng ra ngoài mà không ai chặn.
//
// Bài này khóa bốn nhánh của quyết định, và bốn nhánh ấy phải khác nhau:
// chỉ "chắc chắn CÓ đồng ý" mới mở đường.
func TestMarketingStopsWithoutConsent(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		ten        string
		customer   *fakeCustomer
		category   string
		channel    string
		customerID string
		wantSent   bool
	}{
		{
			ten:        "marketing KHÔNG đồng ý → dừng",
			customer:   &fakeCustomer{consent: map[string]bool{}},
			category:   notification.CategoryMarketing,
			channel:    notification.ChannelEmail,
			customerID: "cus_01",
			wantSent:   false,
		},
		{
			ten: "marketing CÓ đồng ý → gửi",
			customer: &fakeCustomer{consent: map[string]bool{
				"MARKETING_EMAIL": true,
			}},
			category:   notification.CategoryMarketing,
			channel:    notification.ChannelEmail,
			customerID: "cus_01",
			wantSent:   true,
		},
		{
			ten:        "GIAO DỊCH thì không hỏi đồng ý, luôn gửi",
			customer:   &fakeCustomer{consent: map[string]bool{}},
			category:   notification.CategoryTransactional,
			channel:    notification.ChannelEmail,
			customerID: "cus_01",
			wantSent:   true,
		},
		{
			ten:        "tra đồng ý HỎNG → dừng, không cho qua",
			customer:   &fakeCustomer{failure: errors.New("database sập")},
			category:   notification.CategoryMarketing,
			channel:    notification.ChannelEmail,
			customerID: "cus_01",
			wantSent:   false,
		},
		{
			ten: "không biết khách nào → dừng",
			customer: &fakeCustomer{consent: map[string]bool{
				"MARKETING_EMAIL": true,
			}},
			category:   notification.CategoryMarketing,
			channel:    notification.ChannelEmail,
			customerID: "",
			wantSent:   false,
		},
	} {
		t.Run(tc.ten, func(t *testing.T) {
			m, pool := newModuleWithCustomer(t, tc.customer)

			err := m.Send(ctx, notification.SendRequest{
				EventID:   "evt_" + tc.ten,
				Channel:   tc.channel,
				Category:  tc.category,
				Template:  "thu_thu_nghiem",
				Recipient: "customer@example.com",
				UserID:    tc.customerID,
				Subject:   "Thử",
				Body:      "Thử",
			})
			if err != nil {
				t.Fatalf("Send: %v", err)
			}

			var status, skipReason string
			if err := pool.QueryRow(ctx, `
				SELECT status, skip_reason FROM notification_log
				 ORDER BY id DESC LIMIT 1`).Scan(&status, &skipReason); err != nil {
				t.Fatalf("đọc nhật ký: %v", err)
			}

			wasSent := status == "SENT"
			if wasSent != tc.wantSent {
				t.Fatalf("trạng thái %q (lý do %q), mong gửi=%v", status, skipReason, tc.wantSent)
			}

			// Bị chặn thì PHẢI có lý do đọc lên hiểu ngay: khách hỏi "sao
			// tôi không nhận được thư" thì người trực trả lời được bằng
			// một câu, không phải bằng một lần đọc mã.
			if !wasSent && skipReason == "" {
				t.Error("thông báo bị chặn mà không ghi lý do")
			}
		})
	}
}

// TestConsentIsCheckedPerChannel.
//
// Khách tick ô "nhận email khuyến mãi" KHÔNG có nghĩa là họ đồng ý nhận
// TIN NHẮN khuyến mãi. `customer` giữ hai loại đồng ý riêng cho đúng lý do
// đó, và gộp chúng lại sẽ biến một lần tick ô email thành giấy phép nhắn
// tin — thứ tốn tiền của khách và vi phạm ở nhiều nơi.
func TestConsentIsCheckedPerChannel(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		channelName string
		wantAsked   string
		wrongDesc   string
	}{
		{notification.ChannelEmail, "MARKETING_EMAIL", "email"},
		{notification.ChannelSMS, "MARKETING_SMS", "tin nhắn"},
	} {
		t.Run(tc.channelName, func(t *testing.T) {
			// CHỈ đồng ý email. Kênh SMS phải bị chặn.
			k := &fakeCustomer{consent: map[string]bool{"MARKETING_EMAIL": true}}
			m, pool := newModuleWithCustomer(t, k)

			if err := m.Send(ctx, notification.SendRequest{
				EventID:   "evt_kenh_" + tc.channelName,
				Channel:   tc.channelName,
				Category:  notification.CategoryMarketing,
				Template:  "thu_thu_nghiem",
				Recipient: "customer@example.com",
				UserID:    "cus_01",
				Subject:   "Thử",
				Body:      "Thử",
			}); err != nil {
				t.Fatalf("Send: %v", err)
			}

			if len(k.daHoi) == 0 {
				t.Fatal("không hỏi đồng ý lần nào")
			}
			if got := k.daHoi[len(k.daHoi)-1]; got != tc.wantAsked {
				t.Fatalf("hỏi loại đồng ý %q, cần %q — đồng ý nhận %s "+
					"không phải giấy phép cho kênh khác", got, tc.wantAsked, tc.wrongDesc)
			}

			var status string
			if err := pool.QueryRow(ctx, `
				SELECT status FROM notification_log
				 ORDER BY id DESC LIMIT 1`).Scan(&status); err != nil {
				t.Fatalf("đọc nhật ký: %v", err)
			}
			wasSent := status == "SENT"
			if wasSent != (tc.channelName == notification.ChannelEmail) {
				t.Errorf("kênh %s: trạng thái %q — khách chỉ đồng ý email",
					tc.channelName, status)
			}
		})
	}
}

// newModuleWithCustomer dựng module có nối cổng tra khách.
//
// Tách khỏi `newModule` vì phần lớn bài test không cần cổng ấy, và một
// cổng giả luôn trả "đồng ý" gắn sẵn vào mọi bài sẽ che mất chính thứ bộ
// bài này kiểm.
func newModuleWithCustomer(
	t *testing.T, customer notification.CustomerPort) (*notification.Module, *pgxpool.Pool) {
	t.Helper()

	db := testdb.Open(t)
	ctx := context.Background()
	for _, stmt := range []string{"TRUNCATE notification_log"} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	m, err := notification.New(notification.Config{
		Storage:  "postgres",
		DB:       db,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Customer: customer,
	})
	if err != nil {
		t.Fatalf("notification.New: %v", err)
	}
	return m, db.Pool()
}
