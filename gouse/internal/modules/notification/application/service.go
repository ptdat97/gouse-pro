// Package application chứa các use case của module notification.
//
// RÀNG BUỘC: module này KHÔNG gọi bất kỳ module nghiệp vụ nào. Mọi dữ liệu
// cần để soạn thông báo đến từ payload event.
package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/notification/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// Service là tầng application của module notification.
type Service struct {
	repo    domain.Repository
	senders map[domain.Channel]domain.Sender
	clock   Clock
	log     *slog.Logger
}

type Deps struct {
	Repo domain.Repository

	// Senders theo kênh. Thiếu kênh nào thì thông báo của kênh đó được ghi
	// log với trạng thái SKIPPED, không phải FAILED — chưa cấu hình nhà
	// cung cấp là quyết định vận hành, không phải sự cố.
	Senders []domain.Sender

	Clock Clock
	Log   *slog.Logger
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	log := d.Log
	if log == nil {
		log = slog.Default()
	}

	senders := map[domain.Channel]domain.Sender{}
	for _, s := range d.Senders {
		senders[s.Channel()] = s
	}

	return &Service{repo: d.Repo, senders: senders, clock: clock, log: log}
}

// SendInput là dữ liệu gửi một thông báo.
type SendInput struct {
	// EventID là sự kiện sinh ra thông báo.
	//
	// Cùng với Template và Recipient, nó là khóa CHỐNG GỬI TRÙNG. Bỏ trống
	// nghĩa là không chống trùng được — chỉ chấp nhận cho thông báo gửi
	// trực tiếp không đến từ event (ví dụ OTP).
	EventID string

	Channel  domain.Channel
	Category domain.Category
	Template string

	Recipient string
	UserID    string

	Subject string
	Body    string

	ReferenceType string
	ReferenceID   string
}

// Send gửi một thông báo và GHI LOG kết quả.
//
// # Ba đường đi, ba kết quả khác nhau
//
//	Thiếu địa chỉ         → SKIPPED, ghi log, KHÔNG lỗi
//	Đã gửi rồi            → bỏ qua im lặng, KHÔNG lỗi
//	Nhà cung cấp lỗi      → FAILED, trả lỗi để bên gọi thử lại
//
// Hai trường hợp đầu KHÔNG phải sự cố. Trả lỗi cho chúng sẽ khiến event bị
// thử lại vô ích rồi rơi vào dead letter, và cảnh báo vận hành kêu vì
// những việc hoàn toàn bình thường.
func (s *Service) Send(ctx context.Context, in SendInput) error {
	now := s.clock.Now()

	params := domain.NewParams{
		EventID:       in.EventID,
		Channel:       in.Channel,
		Category:      in.Category,
		Template:      in.Template,
		Recipient:     in.Recipient,
		UserID:        in.UserID,
		Subject:       in.Subject,
		Body:          in.Body,
		ReferenceType: in.ReferenceType,
		ReferenceID:   in.ReferenceID,
		Now:           now,
	}

	n, err := domain.New(params)
	if err != nil {
		// Thiếu địa chỉ là trường hợp phổ biến nhất: khách vãng lai không
		// nhập email, hoặc dữ liệu cũ trước khi payload mang email.
		//
		// VẪN GHI LOG: khách hỏi "sao tôi không nhận được email" thì phải
		// trả lời được. Không ghi gì nghĩa là không phân biệt được "hệ
		// thống quyết định không gửi" với "hệ thống quên gửi".
		if errors.Is(err, domain.ErrNoRecipient) {
			return s.recordSkip(ctx, params, "thiếu địa chỉ người nhận")
		}
		return err
	}

	sender, ok := s.senders[n.Channel()]
	if !ok {
		return s.recordSkip(ctx, params, "chưa cấu hình bộ gửi cho kênh "+string(n.Channel()))
	}

	// Ghi log TRƯỚC khi gửi.
	//
	// Thứ tự này là chủ ý: ràng buộc UNIQUE ở database là thứ chặn gửi
	// trùng, và nó chỉ chặn được nếu bản ghi vào trước. Gửi trước rồi ghi
	// sau nghĩa là hai worker song song đều gửi xong mới phát hiện trùng.
	if err := s.repo.Save(ctx, n); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			// ĐÃ GỬI RỒI. Đây là đường đi BÌNH THƯỜNG của mô hình
			// at-least-once, không phải lỗi.
			s.log.Debug("bỏ qua thông báo đã gửi",
				"event_id", in.EventID, "template", in.Template)
			return nil
		}
		return err
	}

	providerID, sendErr := sender.Send(ctx, n)
	if sendErr != nil {
		n.MarkFailed(sendErr, s.clock.Now())
		if err := s.repo.Update(ctx, n); err != nil {
			return err
		}
		// TRẢ LỖI để bên gọi thử lại: nhà cung cấp lỗi là sự cố thật.
		return sendErr
	}

	n.MarkSent(providerID, s.clock.Now())
	return s.repo.Update(ctx, n)
}

// recordSkip ghi lại một thông báo CỐ Ý không gửi.
func (s *Service) recordSkip(
	ctx context.Context, params domain.NewParams, reason string,
) error {
	n := domain.NewSkipped(params, reason)

	if err := s.repo.Save(ctx, n); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			return nil
		}
		return err
	}

	s.log.Info("bỏ qua thông báo",
		"lý_do", reason,
		"template", params.Template,
		"tham_chiếu", params.ReferenceID)
	return nil
}

// History trả lịch sử thông báo của một đối tượng.
//
// Trả lời "đơn hàng này đã gửi những email nào" — câu hỏi đầu tiên khi
// khách nói không nhận được gì.
func (s *Service) History(
	ctx context.Context, refType, refID string,
) ([]*domain.Notification, error) {
	return s.repo.ListByReference(ctx, refType, refID)
}

// Stats đếm thông báo theo trạng thái, cho giám sát.
func (s *Service) Stats(ctx context.Context) (map[domain.Status]int, error) {
	return s.repo.CountByStatus(ctx)
}
