package notification

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/notification/application"
	"github.com/fashion-commerce/platform/internal/modules/notification/domain"
	"github.com/fashion-commerce/platform/internal/modules/notification/infrastructure/logsender"
	notificationpg "github.com/fashion-commerce/platform/internal/modules/notification/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Chống gửi trùng dựa vào chỉ mục UNIQUE. Kiểm tra trước khi ghi vẫn
	// lọt khi hai worker cùng xử lý một event, và khi đó khách nhận hai
	// email giống hệt nhau.
	Storage string

	DB  *database.DB
	Log *slog.Logger

	// Senders là các bộ gửi thật.
	//
	// Bỏ trống thì dùng bộ ghi-log: nội dung được ghi ra nhật ký thay vì
	// gửi đi. Nhờ vậy luồng nghiệp vụ chạy được đầu-cuối trước khi nền
	// tảng ký hợp đồng với nhà cung cấp dịch vụ email.
	Senders []domain.Sender

	Clock application.Clock
}

// New khởi tạo module notification.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"notification: chỉ hỗ trợ kho lưu trữ postgres — chống gửi trùng " +
				"cần chỉ mục UNIQUE ở tầng database")
	}
	if cfg.DB == nil {
		return nil, errors.New("notification: bắt buộc phải có kết nối database")
	}

	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}

	senders := cfg.Senders
	if len(senders) == 0 {
		senders = []domain.Sender{logsender.New(log)}
	}

	return &Module{svc: application.NewService(application.Deps{
		Repo:    notificationpg.NewLogStore(cfg.DB.Pool()),
		Senders: senders,
		Clock:   cfg.Clock,
		Log:     log,
	})}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- API

func (m *Module) Send(ctx context.Context, req SendRequest) error {
	return m.svc.Send(ctx, application.SendInput{
		EventID:       req.EventID,
		Channel:       domain.Channel(req.Channel),
		Category:      domain.Category(req.Category),
		Template:      req.Template,
		Recipient:     req.Recipient,
		UserID:        req.UserID,
		Subject:       req.Subject,
		Body:          req.Body,
		ReferenceType: req.ReferenceType,
		ReferenceID:   req.ReferenceID,
	})
}

func (m *Module) GetHistory(
	ctx context.Context, refType, refID string,
) ([]NotificationView, error) {
	logs, err := m.svc.History(ctx, refType, refID)
	if err != nil {
		return nil, err
	}

	out := make([]NotificationView, 0, len(logs))
	for _, n := range logs {
		out = append(out, toView(n))
	}
	return out, nil
}

func (m *Module) CountByStatus(ctx context.Context) (map[string]int, error) {
	counts, err := m.svc.Stats(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[string]int, len(counts))
	for st, n := range counts {
		out[string(st)] = n
	}
	return out, nil
}

// ---------------------------------------------------------------- Chuyển đổi

func toView(n *domain.Notification) NotificationView {
	return NotificationView{
		EventID:           n.EventID(),
		Channel:           string(n.Channel()),
		Category:          string(n.Category()),
		Template:          n.Template(),
		Recipient:         n.Recipient(),
		UserID:            n.UserID(),
		Subject:           n.Subject(),
		Status:            string(n.Status()),
		ProviderMessageID: n.ProviderMessageID(),
		SkipReason:        n.SkipReason(),
		Error:             n.LastError(),
		Attempts:          n.Attempts(),
		ReferenceType:     n.ReferenceType(),
		ReferenceID:       n.ReferenceID(),
		CreatedAt:         formatTime(n.CreatedAt()),
		SentAt:            formatTime(n.SentAt()),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
