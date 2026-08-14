// Package domain chứa mô hình nghiệp vụ của module notification.
//
// RÀNG BUỘC KIẾN TRÚC QUAN TRỌNG NHẤT (notification.md quy tắc 1):
//
//	Module này KHÔNG GỌI bất kỳ module nghiệp vụ nào.
//
// Nếu nó phải gọi `order` để lấy tên sản phẩm và `customer` để lấy email,
// nó phụ thuộc toàn hệ thống — và một module lỗi sẽ làm hỏng việc gửi mọi
// loại thông báo, kể cả mã OTP.
//
// Hệ quả: mọi dữ liệu cần để soạn thông báo phải nằm trong payload event.
// Đó là lý do `checkout.completed` mang theo email và tên sản phẩm.
//
// # Điều module này KHÔNG quyết định
//
// Nó không quyết định "việc gì đáng thông báo" — đó là quyết định NGHIỆP
// VỤ, và nó đến qua event. Module này chỉ trả lời "gửi thế nào, tới đâu,
// và đã gửi chưa".
package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrNoRecipient = errors.New("notification: thiếu địa chỉ người nhận")
	ErrNoTemplate  = errors.New("notification: thiếu mẫu thông báo")
	ErrNotAllowed  = errors.New("notification: người nhận đã tắt loại thông báo này")
)

// Channel là kênh gửi.
type Channel string

const (
	ChannelEmail Channel = "EMAIL"
	ChannelSMS   Channel = "SMS"
	ChannelPush  Channel = "PUSH"
	ChannelInApp Channel = "IN_APP"
)

// Category quyết định thông báo có được gửi hay không.
//
// PHÂN BIỆT NÀY LÀ YÊU CẦU PHÁP LÝ, không phải phân loại cho gọn:
//
//	TRANSACTIONAL  xác nhận đơn, giao hàng, hoàn tiền
//	               → KHÔNG cần đồng ý marketing, LUÔN gửi
//	               → khách KHÔNG tắt được (thông tin thiết yếu về giao
//	                 dịch họ đã trả tiền)
//
//	MARKETING      khuyến mãi, sản phẩm mới
//	               → BẮT BUỘC có đồng ý
//	               → phải có cách hủy đăng ký dễ dàng
//
// Nhầm lẫn hai loại này là vi phạm pháp luật ở nhiều thị trường.
type Category string

const (
	CategoryTransactional Category = "TRANSACTIONAL"
	CategoryMarketing     Category = "MARKETING"
	CategorySocial        Category = "SOCIAL"
)

// RequiresConsent cho biết loại này có cần đồng ý trước khi gửi không.
//
// TRANSACTIONAL trả về false: khách đã trả tiền cho một giao dịch thì có
// quyền biết nó đang ở đâu, và không ai tắt được quyền đó.
func (c Category) RequiresConsent() bool {
	return c == CategoryMarketing || c == CategorySocial
}

// Status là trạng thái gửi.
type Status string

const (
	StatusPending Status = "PENDING"
	StatusSent    Status = "SENT"
	StatusFailed  Status = "FAILED"

	// StatusSkipped: CỐ Ý không gửi.
	//
	// Khác hẳn FAILED. Thiếu địa chỉ email hay chưa có đồng ý marketing là
	// quyết định đúng, không phải sự cố — gộp chung sẽ làm cảnh báo vận
	// hành kêu vì những việc bình thường, và rồi không ai đọc cảnh báo nữa.
	StatusSkipped Status = "SKIPPED"
)

// Các mẫu thông báo của MVP.
//
// Mã mẫu là DỮ LIỆU, không phải mã nguồn: thêm mẫu mới không phải sửa
// kiến trúc. Nhưng khai báo hằng ở đây để bên phát và bên soạn không tự gõ
// chuỗi — một lỗi đánh máy tạo ra thông báo không ai soạn được.
const (
	TemplateOrderConfirmed = "order_confirmed"
	TemplateOrderShipped   = "order_shipped"
	TemplateOrderDelivered = "order_delivered"
	TemplateOrderCancelled = "order_cancelled"
	TemplateSellerNewOrder = "seller_new_order"
)

// Notification là MỘT LẦN GỬI.
//
// Bất biến sau khi tạo, trừ trạng thái gửi. Đây là bản ghi để trả lời
// khách khi họ nói "tôi không nhận được email" — nên nó phải giữ nguyên
// những gì đã gửi, kể cả khi mẫu thông báo sau này đổi.
type Notification struct {
	// eventID là sự kiện sinh ra thông báo này.
	//
	// Cùng với template và recipient, nó tạo nên khóa CHỐNG GỬI TRÙNG.
	// Event ở mô hình at-least-once sẽ được phát lại, và khách nhận ba
	// email "đơn đã đặt" sẽ gọi tổng đài hỏi mình bị tính tiền mấy lần.
	eventID string

	channel  Channel
	category Category
	template string

	// recipient lưu NGUYÊN VĂN, không băm: bộ phận hỗ trợ cần trả lời được
	// "chúng tôi đã gửi tới đâu" khi khách khiếu nại.
	recipient string
	userID    string

	subject string
	body    string

	status Status

	providerMessageID string
	skipReason        string
	lastError         string
	attempts          int

	referenceType string
	referenceID   string

	createdAt time.Time
	sentAt    time.Time
}

type NewParams struct {
	EventID  string
	Channel  Channel
	Category Category
	Template string

	Recipient string
	UserID    string

	Subject string
	Body    string

	ReferenceType string
	ReferenceID   string

	Now time.Time
}

// New tạo một thông báo ở trạng thái chờ gửi.
func New(p NewParams) (*Notification, error) {
	if strings.TrimSpace(p.Recipient) == "" {
		return nil, ErrNoRecipient
	}
	if strings.TrimSpace(p.Template) == "" {
		return nil, ErrNoTemplate
	}

	channel := p.Channel
	if channel == "" {
		channel = ChannelEmail
	}
	category := p.Category
	if category == "" {
		category = CategoryTransactional
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Notification{
		eventID:       strings.TrimSpace(p.EventID),
		channel:       channel,
		category:      category,
		template:      strings.TrimSpace(p.Template),
		recipient:     strings.TrimSpace(p.Recipient),
		userID:        p.UserID,
		subject:       p.Subject,
		body:          p.Body,
		status:        StatusPending,
		referenceType: p.ReferenceType,
		referenceID:   p.ReferenceID,
		createdAt:     now,
	}, nil
}

// NewSkipped tạo bản ghi cho một thông báo CỐ Ý không gửi.
//
// VÌ SAO VẪN GHI LOG khi không gửi: khách hỏi "sao tôi không nhận được
// email" thì phải trả lời được. Không ghi gì cả nghĩa là không phân biệt
// được "hệ thống quyết định không gửi" với "hệ thống quên gửi".
func NewSkipped(p NewParams, reason string) *Notification {
	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	channel := p.Channel
	if channel == "" {
		channel = ChannelEmail
	}

	recipient := strings.TrimSpace(p.Recipient)
	if recipient == "" {
		// Ràng buộc CHECK ở database yêu cầu recipient không rỗng, mà lý do
		// bỏ qua phổ biến nhất CHÍNH LÀ thiếu địa chỉ. Dùng giá trị thay
		// thế tường minh thay vì bỏ luôn bản ghi.
		recipient = "(không có địa chỉ)"
	}

	return &Notification{
		eventID:       strings.TrimSpace(p.EventID),
		channel:       channel,
		category:      p.Category,
		template:      strings.TrimSpace(p.Template),
		recipient:     recipient,
		userID:        p.UserID,
		status:        StatusSkipped,
		skipReason:    reason,
		referenceType: p.ReferenceType,
		referenceID:   p.ReferenceID,
		createdAt:     now,
	}
}

// RestoreParams dựng lại từ kho lưu trữ.
type RestoreParams struct {
	EventID           string
	Channel           Channel
	Category          Category
	Template          string
	Recipient         string
	UserID            string
	Subject           string
	Body              string
	Status            Status
	ProviderMessageID string
	SkipReason        string
	LastError         string
	Attempts          int
	ReferenceType     string
	ReferenceID       string
	CreatedAt         time.Time
	SentAt            time.Time
}

// Restore dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func Restore(p RestoreParams) *Notification {
	return &Notification{
		eventID:           p.EventID,
		channel:           p.Channel,
		category:          p.Category,
		template:          p.Template,
		recipient:         p.Recipient,
		userID:            p.UserID,
		subject:           p.Subject,
		body:              p.Body,
		status:            p.Status,
		providerMessageID: p.ProviderMessageID,
		skipReason:        p.SkipReason,
		lastError:         p.LastError,
		attempts:          p.Attempts,
		referenceType:     p.ReferenceType,
		referenceID:       p.ReferenceID,
		createdAt:         p.CreatedAt,
		sentAt:            p.SentAt,
	}
}

func (n *Notification) EventID() string           { return n.eventID }
func (n *Notification) Channel() Channel          { return n.channel }
func (n *Notification) Category() Category        { return n.category }
func (n *Notification) Template() string          { return n.template }
func (n *Notification) Recipient() string         { return n.recipient }
func (n *Notification) UserID() string            { return n.userID }
func (n *Notification) Subject() string           { return n.subject }
func (n *Notification) Body() string              { return n.body }
func (n *Notification) Status() Status            { return n.status }
func (n *Notification) ProviderMessageID() string { return n.providerMessageID }
func (n *Notification) SkipReason() string        { return n.skipReason }
func (n *Notification) LastError() string         { return n.lastError }
func (n *Notification) Attempts() int             { return n.attempts }
func (n *Notification) ReferenceType() string     { return n.referenceType }
func (n *Notification) ReferenceID() string       { return n.referenceID }
func (n *Notification) CreatedAt() time.Time      { return n.createdAt }
func (n *Notification) SentAt() time.Time         { return n.sentAt }

// MarkSent ghi nhận đã giao cho nhà cung cấp.
//
// LƯU Ý: "đã gửi" nghĩa là nhà cung cấp đã nhận, KHÔNG phải khách đã đọc.
// Trạng thái giao thật (delivered, bounced) đến sau qua webhook.
func (n *Notification) MarkSent(providerMessageID string, now time.Time) {
	n.status = StatusSent
	n.providerMessageID = providerMessageID
	n.attempts++
	n.sentAt = now
	n.lastError = ""
}

// MarkFailed ghi nhận gửi thất bại.
func (n *Notification) MarkFailed(err error, now time.Time) {
	n.status = StatusFailed
	n.attempts++
	if err != nil {
		n.lastError = err.Error()
	}
}
