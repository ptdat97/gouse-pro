package domain

import (
	"context"
	"errors"
)

// ErrDuplicate khi thông báo này đã được gửi rồi.
//
// Bên gọi nên coi đó là THÀNH CÔNG, không phải lỗi: mô hình at-least-once
// nghĩa là event sẽ được phát lại, và lần thứ hai không gửi gì là hành vi
// ĐÚNG.
var ErrDuplicate = errors.New("notification: thông báo này đã được gửi")

// Repository là PORT cho kho lưu trữ thông báo.
type Repository interface {
	// Save ghi bản ghi thông báo.
	//
	// Trả ErrDuplicate nếu (event_id, template, recipient) đã tồn tại —
	// đó là khóa chống gửi trùng, và ràng buộc UNIQUE ở database là thứ
	// cưỡng chế nó dưới tải song song.
	Save(ctx context.Context, n *Notification) error

	// Update ghi lại kết quả gửi.
	Update(ctx context.Context, n *Notification) error

	// ListByReference trả lịch sử thông báo của một đối tượng.
	//
	// Trả lời "đơn hàng này đã gửi những email nào" — câu hỏi đầu tiên khi
	// khách nói không nhận được gì.
	ListByReference(ctx context.Context, refType, refID string) ([]*Notification, error)

	// CountByStatus đếm theo trạng thái, cho giám sát.
	CountByStatus(ctx context.Context) (map[Status]int, error)
}

// Sender là PORT cho việc gửi thật.
//
// Nguyên tắc P13: nhà cung cấp dịch vụ gửi email SẼ đổi. Domain định nghĩa
// interface, nhà cung cấp cài đặt — đổi nhà cung cấp là viết adapter mới,
// KHÔNG sửa module này.
//
// Domain không biết tên nhà cung cấp nào.
type Sender interface {
	// Channel cho biết bộ gửi này phục vụ kênh nào.
	Channel() Channel

	// Send gửi thông báo, trả về mã tra cứu của nhà cung cấp.
	//
	// Mã đó dùng để đối chiếu khi khách khiếu nại "không nhận được".
	Send(ctx context.Context, n *Notification) (providerMessageID string, err error)
}
