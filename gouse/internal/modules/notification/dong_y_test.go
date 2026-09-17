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

// khachGia trả lời câu hỏi đồng ý theo kịch bản của từng bài.
type khachGia struct {
	dongY map[string]bool
	loi   error

	// daHoi ghi lại loại đồng ý đã bị hỏi, để bài test kiểm ĐÚNG loại.
	daHoi []string
}

func (k *khachGia) EmailCuaKhach(context.Context, string) (string, error) {
	return "khach@example.com", nil
}

func (k *khachGia) CoDongY(_ context.Context, _, loai string) (bool, error) {
	k.daHoi = append(k.daHoi, loai)
	if k.loi != nil {
		return false, k.loi
	}
	return k.dongY[loai], nil
}

// TestThuMarketingDungLaiKhiKhachChuaDongY.
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
func TestThuMarketingDungLaiKhiKhachChuaDongY(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		ten      string
		khach    *khachGia
		category string
		channel  string
		khachID  string
		muonGui  bool
	}{
		{
			ten:      "marketing KHÔNG đồng ý → dừng",
			khach:    &khachGia{dongY: map[string]bool{}},
			category: notification.CategoryMarketing,
			channel:  notification.ChannelEmail,
			khachID:  "cus_01",
			muonGui:  false,
		},
		{
			ten: "marketing CÓ đồng ý → gửi",
			khach: &khachGia{dongY: map[string]bool{
				"MARKETING_EMAIL": true,
			}},
			category: notification.CategoryMarketing,
			channel:  notification.ChannelEmail,
			khachID:  "cus_01",
			muonGui:  true,
		},
		{
			ten:      "GIAO DỊCH thì không hỏi đồng ý, luôn gửi",
			khach:    &khachGia{dongY: map[string]bool{}},
			category: notification.CategoryTransactional,
			channel:  notification.ChannelEmail,
			khachID:  "cus_01",
			muonGui:  true,
		},
		{
			ten:      "tra đồng ý HỎNG → dừng, không cho qua",
			khach:    &khachGia{loi: errors.New("database sập")},
			category: notification.CategoryMarketing,
			channel:  notification.ChannelEmail,
			khachID:  "cus_01",
			muonGui:  false,
		},
		{
			ten: "không biết khách nào → dừng",
			khach: &khachGia{dongY: map[string]bool{
				"MARKETING_EMAIL": true,
			}},
			category: notification.CategoryMarketing,
			channel:  notification.ChannelEmail,
			khachID:  "",
			muonGui:  false,
		},
	} {
		t.Run(tc.ten, func(t *testing.T) {
			m, pool := newModuleKemKhach(t, tc.khach)

			err := m.Send(ctx, notification.SendRequest{
				EventID:   "evt_" + tc.ten,
				Channel:   tc.channel,
				Category:  tc.category,
				Template:  "thu_thu_nghiem",
				Recipient: "khach@example.com",
				UserID:    tc.khachID,
				Subject:   "Thử",
				Body:      "Thử",
			})
			if err != nil {
				t.Fatalf("Send: %v", err)
			}

			var status, lyDo string
			if err := pool.QueryRow(ctx, `
				SELECT status, skip_reason FROM notification_log
				 ORDER BY id DESC LIMIT 1`).Scan(&status, &lyDo); err != nil {
				t.Fatalf("đọc nhật ký: %v", err)
			}

			daGui := status == "SENT"
			if daGui != tc.muonGui {
				t.Fatalf("trạng thái %q (lý do %q), mong gửi=%v", status, lyDo, tc.muonGui)
			}

			// Bị chặn thì PHẢI có lý do đọc lên hiểu ngay: khách hỏi "sao
			// tôi không nhận được thư" thì người trực trả lời được bằng
			// một câu, không phải bằng một lần đọc mã.
			if !daGui && lyDo == "" {
				t.Error("thông báo bị chặn mà không ghi lý do")
			}
		})
	}
}

// TestDongYHoiDUNGKenh.
//
// Khách tick ô "nhận email khuyến mãi" KHÔNG có nghĩa là họ đồng ý nhận
// TIN NHẮN khuyến mãi. `customer` giữ hai loại đồng ý riêng cho đúng lý do
// đó, và gộp chúng lại sẽ biến một lần tick ô email thành giấy phép nhắn
// tin — thứ tốn tiền của khách và vi phạm ở nhiều nơi.
func TestDongYHoiDUNGKenh(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		kenh    string
		muonHoi string
		moTaSai string
	}{
		{notification.ChannelEmail, "MARKETING_EMAIL", "email"},
		{notification.ChannelSMS, "MARKETING_SMS", "tin nhắn"},
	} {
		t.Run(tc.kenh, func(t *testing.T) {
			// CHỈ đồng ý email. Kênh SMS phải bị chặn.
			k := &khachGia{dongY: map[string]bool{"MARKETING_EMAIL": true}}
			m, pool := newModuleKemKhach(t, k)

			if err := m.Send(ctx, notification.SendRequest{
				EventID:   "evt_kenh_" + tc.kenh,
				Channel:   tc.kenh,
				Category:  notification.CategoryMarketing,
				Template:  "thu_thu_nghiem",
				Recipient: "khach@example.com",
				UserID:    "cus_01",
				Subject:   "Thử",
				Body:      "Thử",
			}); err != nil {
				t.Fatalf("Send: %v", err)
			}

			if len(k.daHoi) == 0 {
				t.Fatal("không hỏi đồng ý lần nào")
			}
			if got := k.daHoi[len(k.daHoi)-1]; got != tc.muonHoi {
				t.Fatalf("hỏi loại đồng ý %q, cần %q — đồng ý nhận %s "+
					"không phải giấy phép cho kênh khác", got, tc.muonHoi, tc.moTaSai)
			}

			var status string
			if err := pool.QueryRow(ctx, `
				SELECT status FROM notification_log
				 ORDER BY id DESC LIMIT 1`).Scan(&status); err != nil {
				t.Fatalf("đọc nhật ký: %v", err)
			}
			daGui := status == "SENT"
			if daGui != (tc.kenh == notification.ChannelEmail) {
				t.Errorf("kênh %s: trạng thái %q — khách chỉ đồng ý email",
					tc.kenh, status)
			}
		})
	}
}

// newModuleKemKhach dựng module có nối cổng tra khách.
//
// Tách khỏi `newModule` vì phần lớn bài test không cần cổng ấy, và một
// cổng giả luôn trả "đồng ý" gắn sẵn vào mọi bài sẽ che mất chính thứ bộ
// bài này kiểm.
func newModuleKemKhach(
	t *testing.T, khach notification.KhachPort,
) (*notification.Module, *pgxpool.Pool) {
	t.Helper()

	db := testdb.Open(t)
	ctx := context.Background()
	for _, stmt := range []string{"TRUNCATE notification_log"} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	m, err := notification.New(notification.Config{
		Storage: "postgres",
		DB:      db,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Khach:   khach,
	})
	if err != nil {
		t.Fatalf("notification.New: %v", err)
	}
	return m, db.Pool()
}
