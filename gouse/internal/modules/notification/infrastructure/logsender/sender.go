// Package logsender là bộ gửi email GHI RA NHẬT KÝ thay vì gửi thật.
//
// VÌ SAO CÓ Ở MVP: nền tảng chưa ký hợp đồng với nhà cung cấp dịch vụ gửi
// email. Nhưng luồng nghiệp vụ phải chạy được đầu-cuối ngay từ bây giờ —
// nếu không, việc soạn nội dung, chống gửi trùng và ghi nhật ký sẽ không
// được kiểm chứng cho tới tận lúc tích hợp thật.
//
// Đây KHÔNG phải bản giả cho test. Nó chạy ở môi trường phát triển và
// staging, ghi ra log đúng thứ sẽ được gửi đi.
//
// ĐỔI SANG NHÀ CUNG CẤP THẬT là viết một package như package này, cài đặt
// cùng interface `domain.Sender`, rồi đổi một dòng ở nơi khởi tạo. Module
// notification không đổi gì — đó là điều nguyên tắc P13 bảo đảm.
package logsender

import (
	"context"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/notification/domain"
)

// Sender ghi thông báo ra nhật ký.
type Sender struct {
	log *slog.Logger
}

func New(log *slog.Logger) *Sender {
	if log == nil {
		log = slog.Default()
	}
	return &Sender{log: log}
}

var _ domain.Sender = (*Sender)(nil)

func (s *Sender) Channel() domain.Channel { return domain.ChannelEmail }

// Send ghi nội dung ra nhật ký và trả về một mã tra cứu giả.
//
// Mã trả về có tiền tố `evt_` để phân biệt rõ với mã của nhà cung cấp
// thật — đọc log thấy nó là biết email CHƯA thật sự được gửi đi.
func (s *Sender) Send(
	_ context.Context, n *domain.Notification,
) (string, error) {
	s.log.Info("GỬI EMAIL (chỉ ghi log, chưa gửi thật)",
		"tới", n.Recipient(),
		"tiêu_đề", n.Subject(),
		"mẫu", n.Template(),
		"tham_chiếu", n.ReferenceID(),
		"nội_dung", n.Body())

	id, err := ids.New(ids.PrefixEvent)
	if err != nil {
		return "", err
	}
	return "log_" + id.String(), nil
}
