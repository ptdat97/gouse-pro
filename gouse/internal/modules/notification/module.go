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

// KhachPort trả địa chỉ liên lạc của một khách hàng đã đăng ký.
//
// # Vì sao module này cần biết tới khách hàng
//
// Thư giao dịch phải có người nhận. Payload của `checkout.completed` mang
// `guest_email` — địa chỉ khách VÃNG LAI tự gõ vào ô thanh toán. Khách ĐÃ
// ĐĂNG KÝ không gõ ô đó, nên trường ấy rỗng, và cho tới 17/09 hệ quả là:
// người có tài khoản KHÔNG nhận được thư xác nhận đơn lẫn thư báo giao
// hàng, còn khách vãng lai thì có.
//
// Tra ở ĐÂY chứ không nhồi email vào payload event, vì ba lý do:
//
//  1. Cùng một phép tra vá được CẢ HAI đường — `checkout.completed` và
//     `fulfillment.progress` đều mang `customer_id`. Đi đường payload thì
//     fulfillment còn phải lưu lại email rồi phát tiếp, tức ba chỗ sửa.
//  2. Không phải tăng phiên bản event, tức tám bên nhận không phải khai
//     lại `MaxEventVersion` cho một trường chỉ một bên dùng (ADR-0016).
//  3. Gửi thư là việc BẤT ĐỒNG BỘ. Một lượt tra thêm ở đây không nằm trên
//     đường thanh toán của khách; nhồi vào payload thì checkout phải gọi
//     customer ngay giữa lúc đặt đơn.
//
// Nil thì module vẫn chạy: khách vãng lai vẫn nhận thư như cũ, khách đã
// đăng ký bị ghi SKIPPED kèm lý do — đúng hành vi trước bản sửa này.
type KhachPort interface {
	// EmailCuaKhach trả email của hồ sơ khách. Chuỗi rỗng nghĩa là không
	// tra được, và đó KHÔNG phải lỗi cần chặn đường gửi.
	EmailCuaKhach(ctx context.Context, customerID string) (string, error)

	// CoDongY cho biết khách CÓ ĐANG đồng ý nhận loại này không.
	//
	// Hỏi lúc GỬI chứ không tin payload event: đồng ý có thể bị rút SAU
	// khi event được phát, và gửi theo dữ liệu cũ nghĩa là gửi thư cho
	// người vừa bấm hủy đăng ký.
	CoDongY(ctx context.Context, customerID, loai string) (bool, error)
}

// dongYTu dựng cổng tra đồng ý cho tầng application.
//
// Trả nil khi chưa nối `Khach`, và nil ở đó nghĩa là MỌI thông báo cần
// đồng ý đều bị từ chối — thất bại theo hướng đóng.
func dongYTu(k KhachPort) application.DongYPort {
	if k == nil {
		return nil
	}
	return dongYAdapter{k: k}
}

type dongYAdapter struct{ k KhachPort }

func (a dongYAdapter) CoDongY(
	ctx context.Context, customerID, loai string,
) (bool, error) {
	return a.k.CoDongY(ctx, customerID, loai)
}

// KhachPortFunc nối dây bằng một hàm, cho phép nối TRỄ.
//
// Cần vì `internal/app` có vòng khởi tạo: `customer` nhận notification để
// gửi thư xác minh email, nên nó được dựng SAU. Một closure đọc biến
// module lúc GỌI thay vì lúc dựng gỡ được vòng đó mà không cần adapter
// có trạng thái thay đổi được.
type KhachPortFunc struct {
	Email func(ctx context.Context, customerID string) (string, error)
	DongY func(ctx context.Context, customerID, loai string) (bool, error)
}

func (f KhachPortFunc) EmailCuaKhach(
	ctx context.Context, customerID string,
) (string, error) {
	if f.Email == nil {
		return "", nil
	}
	return f.Email(ctx, customerID)
}

// CoDongY trả false khi chưa nối hàm: không chứng minh được đồng ý thì
// không gửi.
func (f KhachPortFunc) CoDongY(
	ctx context.Context, customerID, loai string,
) (bool, error) {
	if f.DongY == nil {
		return false, nil
	}
	return f.DongY(ctx, customerID, loai)
}

// Module là cài đặt của API công khai.
type Module struct {
	svc   *application.Service
	khach KhachPort
	log   *slog.Logger
}

// emailNguoiNhan chọn địa chỉ gửi thư.
//
// Ưu tiên email khách tự gõ ở ô thanh toán: đó là địa chỉ họ CHỌN cho đơn
// này, và với đơn đặt hộ thì nó khác email tài khoản.
func (m *Module) emailNguoiNhan(
	ctx context.Context, emailPhien, customerID string,
) string {
	if emailPhien != "" {
		return emailPhien
	}
	if m.khach == nil || customerID == "" {
		return ""
	}
	email, err := m.khach.EmailCuaKhach(ctx, customerID)
	if err != nil {
		// KHÔNG chặn đường gửi: tầng application đã ghi SKIPPED kèm lý do
		// khi thiếu địa chỉ, nên sự việc vẫn có vết. Trả lỗi ở đây biến
		// một sự cố của module customer thành event bị hoãn vô hạn.
		m.log.ErrorContext(ctx, "không tra được email khách để gửi thư",
			"error", err, "customer_id", customerID)
		return ""
	}
	return email
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

	// Khach để tra email của khách ĐÃ ĐĂNG KÝ — xem KhachPort.
	Khach KhachPort
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

	return &Module{khach: cfg.Khach, log: log, svc: application.NewService(application.Deps{
		Repo:    notificationpg.NewLogStore(cfg.DB.Pool()),
		Senders: senders,
		Clock:   cfg.Clock,
		Log:     log,
		DongY:   dongYTu(cfg.Khach),
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
