// Package notification gửi thông báo và ghi nhật ký mọi lần gửi.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// # Ràng buộc kiến trúc quan trọng nhất
//
//	Module này KHÔNG GỌI bất kỳ module nghiệp vụ nào.
//
// Nếu nó phải gọi `order` để lấy tên sản phẩm và `customer` để lấy email,
// nó phụ thuộc toàn hệ thống — và một module lỗi sẽ làm hỏng việc gửi mọi
// loại thông báo, kể cả mã OTP.
//
// Hệ quả cho thiết kế event: payload phải chứa ĐỦ thông tin. Đó là lý do
// `checkout.completed` mang theo email và tên sản phẩm đã đóng băng.
//
// # Điều module này KHÔNG quyết định
//
// Nó không quyết định "việc gì đáng thông báo" — đó là quyết định NGHIỆP
// VỤ và đến qua event. Module này chỉ trả lời "gửi thế nào, tới đâu, và
// đã gửi chưa".
package notification

import "context"

// API là hợp đồng công khai của module notification.
type API interface {
	// Send gửi một thông báo.
	//
	// Dùng cho các trường hợp gửi TRỰC TIẾP (ví dụ mã OTP). Đa số thông
	// báo được kích hoạt qua event, không qua hàm này.
	//
	// IDEMPOTENT theo (EventID, Template, Recipient): gọi lại KHÔNG gửi
	// thêm. Khách nhận ba email "đơn đã đặt" sẽ gọi tổng đài hỏi mình bị
	// tính tiền mấy lần.
	Send(ctx context.Context, req SendRequest) error

	// GetHistory trả lịch sử thông báo của một đối tượng.
	//
	// Trả lời "đơn hàng này đã gửi những email nào" — câu hỏi đầu tiên khi
	// khách nói không nhận được gì.
	GetHistory(ctx context.Context, refType, refID string) ([]NotificationView, error)

	// CountByStatus đếm thông báo theo trạng thái, cho giám sát.
	CountByStatus(ctx context.Context) (map[string]int, error)
}

// ---------------------------------------------------------------- DTO

// SendRequest là dữ liệu gửi một thông báo.
type SendRequest struct {
	// EventID là sự kiện sinh ra thông báo — khóa CHỐNG GỬI TRÙNG.
	//
	// Bỏ trống nghĩa là không chống trùng được; chỉ chấp nhận cho thông
	// báo không đến từ event.
	EventID string

	// Channel: EMAIL, SMS, PUSH, IN_APP. Bỏ trống = EMAIL.
	Channel string

	// Category: TRANSACTIONAL, MARKETING, SOCIAL. Bỏ trống = TRANSACTIONAL.
	//
	// PHÂN BIỆT NÀY LÀ YÊU CẦU PHÁP LÝ:
	//
	//	TRANSACTIONAL  luôn gửi, khách không tắt được
	//	MARKETING      bắt buộc có đồng ý
	//
	// Nhầm lẫn hai loại là vi phạm pháp luật ở nhiều thị trường.
	Category string

	Template  string
	Recipient string
	UserID    string

	Subject string
	Body    string

	ReferenceType string
	ReferenceID   string
}

// NotificationView là một bản ghi trong nhật ký gửi.
type NotificationView struct {
	EventID  string
	Channel  string
	Category string
	Template string

	Recipient string
	UserID    string
	Subject   string

	// Status: PENDING, SENT, FAILED, SKIPPED.
	//
	// SKIPPED khác hẳn FAILED: bỏ qua là quyết định CÓ CHỦ Ý (thiếu địa
	// chỉ, chưa có đồng ý), còn thất bại là sự cố cần xem.
	Status string

	ProviderMessageID string
	SkipReason        string
	Error             string
	Attempts          int

	ReferenceType string
	ReferenceID   string

	CreatedAt string
	SentAt    string
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrInvalidInput = errInvalidInput{}
)

type errInvalidInput struct{}

func (errInvalidInput) Error() string { return "notification: dữ liệu không hợp lệ" }

// ---------------------------------------------------------------- Hằng

// Kênh gửi.
const (
	ChannelEmail = "EMAIL"
	ChannelSMS   = "SMS"
	ChannelPush  = "PUSH"
	ChannelInApp = "IN_APP"
)

// Loại thông báo.
const (
	CategoryTransactional = "TRANSACTIONAL"
	CategoryMarketing     = "MARKETING"
	CategorySocial        = "SOCIAL"
)

// Trạng thái gửi.
const (
	StatusPending = "PENDING"
	StatusSent    = "SENT"
	StatusFailed  = "FAILED"
	StatusSkipped = "SKIPPED"
)

// Mẫu thông báo của MVP.
const (
	TemplateOrderConfirmed = "order_confirmed"
	TemplateOrderShipped   = "order_shipped"
	TemplateOrderDelivered = "order_delivered"
	TemplateOrderCancelled = "order_cancelled"
)
