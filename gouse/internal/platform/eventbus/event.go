// Package eventbus cung cấp domain event và Transactional Outbox.
//
// Đây là hạ tầng TRUNG LẬP VỚI DOMAIN (quy tắc R3 của cmd/archcheck): nó
// không biết đơn hàng hay tồn kho là gì. Nó chỉ biết "có một sự thật đã
// xảy ra, cần đưa tới những ai quan tâm".
//
// BÀI TOÁN NÓ GIẢI (ADR-0006): một sự kiện nghiệp vụ như "đơn hàng được
// đặt" kéo theo nhiều module phải phản ứng. Nếu module order gọi thẳng
// chín module đó:
//
//	− order phụ thuộc 9 module → vi phạm ranh giới nghiêm trọng
//	− Thêm bên nhận thứ 10 phải sửa module order
//	− Một module lỗi làm hỏng việc đặt hàng
//
// CÁCH GIẢI: order phát một sự thật, không biết ai nghe.
//
// # Đảm bảo AT-LEAST-ONCE, không phải exactly-once
//
// Event ghi vào outbox TRONG CÙNG giao dịch với thay đổi nghiệp vụ, rồi
// một tiến trình riêng đọc và phát. Hệ quả:
//
//	✓ Giao dịch thành công → event CHẮC CHẮN được phát (sớm hay muộn)
//	✓ Giao dịch thất bại   → event KHÔNG BAO GIỜ được phát
//	✗ Event có thể được phát NHIỀU LẦN
//
// Dòng cuối là lý do mọi bên nhận PHẢI idempotent. Đó là yêu cầu bắt buộc
// của kiến trúc này, không phải lời khuyên — xem Handler.
package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrNoType      = errors.New("eventbus: event phải có loại")
	ErrNoAggregate = errors.New("eventbus: event phải trỏ tới một aggregate")
	ErrNoPayload   = errors.New("eventbus: event phải có dữ liệu")
)

// Event là một SỰ THẬT NGHIỆP VỤ ĐÃ XẢY RA.
//
// Tên đặt ở THÌ QUÁ KHỨ và mô tả sự thật, không phải mệnh lệnh:
//
//	Đúng:  order.placed · payment.captured · quality.approved
//	Sai:   send.email  · update.inventory  · process.order
//
// Khác biệt này quyết định: `send.email` là mệnh lệnh trá hình — bên phát
// phải biết bên nhận làm gì. `order.placed` là sự thật — thêm bên nhận mới
// (gửi SMS, ghi thống kê, tính hoa hồng) không cần sửa module đơn hàng.
type Event struct {
	// ID là định danh của LẦN XẢY RA này.
	//
	// Bên nhận dùng nó để bỏ qua event trùng — nền tảng của idempotency.
	ID ids.ID

	// Type dạng "order.placed".
	Type string

	// Version cho phép tiến hóa schema mà không phá bên nhận cũ.
	Version int

	AggregateType string
	AggregateID   ids.ID

	// Payload chứa ĐỦ thông tin để bên nhận xử lý mà KHÔNG phải gọi ngược
	// lại bên phát.
	//
	// Nếu mọi bên nhận đều phải gọi ngược để lấy chi tiết, event trở nên
	// vô dụng và tạo đúng thứ ghép nối mà nó sinh ra để tránh.
	//
	// Nhưng cũng không nhồi toàn bộ aggregate — chỉ những gì bên nhận cần.
	Payload json.RawMessage

	// CorrelationID nối toàn bộ chuỗi từ MỘT hành động của khách.
	// CausationID cho biết event nào sinh ra event này.
	//
	// Hai trường này là thứ duy nhất trả lời được "vì sao bút toán này tồn
	// tại" khi có tranh chấp ba tháng sau.
	CorrelationID string
	CausationID   string

	OccurredAt time.Time
}

// NewEvent tạo một event với dữ liệu đã tuần tự hóa.
func NewEvent(eventType, aggregateType string, aggregateID ids.ID, payload any) (Event, error) {
	if strings.TrimSpace(eventType) == "" {
		return Event{}, ErrNoType
	}
	if strings.TrimSpace(aggregateType) == "" || aggregateID.IsZero() {
		return Event{}, ErrNoAggregate
	}
	if payload == nil {
		return Event{}, ErrNoPayload
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}

	id, err := ids.New(ids.PrefixEvent)
	if err != nil {
		return Event{}, err
	}

	return Event{
		ID:            id,
		Type:          eventType,
		Version:       1,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       raw,
		OccurredAt:    time.Now().UTC(),
	}, nil
}

// WithTrace gắn thông tin truy vết vào event.
func (e Event) WithTrace(correlationID, causationID string) Event {
	e.CorrelationID = correlationID
	e.CausationID = causationID
	return e
}

// Unmarshal đọc dữ liệu event vào cấu trúc của bên nhận.
func (e Event) Unmarshal(dst any) error {
	return json.Unmarshal(e.Payload, dst)
}

// Handler là một bên nhận event.
//
// YÊU CẦU BẮT BUỘC — IDEMPOTENT: hàm này sẽ được gọi nhiều lần với cùng
// một event. Đó không phải trường hợp hiếm mà là hoạt động bình thường của
// mô hình at-least-once.
//
// Cơ chế bỏ qua event trùng do eventbus lo (bảng event_processed), nhưng
// nó CHỈ đúng khi bên nhận dùng giao dịch được truyền vào:
//
//	Xử lý nghiệp vụ + đánh dấu đã xử lý PHẢI trong CÙNG giao dịch.
//
// Ghi sổ thành công mà đánh dấu thất bại nghĩa là lần thử lại sẽ ghi sổ
// lần thứ hai — tiền bị nhân đôi.
type Handler interface {
	// Name là định danh của bên nhận, ví dụ
	// "inventory.commit_on_order_placed".
	//
	// Dùng làm khóa idempotency: mỗi bên nhận xử lý độc lập, nên
	// notification đã xử lý không có nghĩa payment cũng đã xử lý.
	Name() string

	// EventTypes là các loại event bên nhận này quan tâm.
	EventTypes() []string

	// Handle xử lý event.
	//
	// Trả lỗi → eventbus thử lại. Sau nhiều lần thất bại, event chuyển
	// sang dead letter và cần người vận hành xem.
	Handle(ctx context.Context, e Event) error
}

// ---------------------------------------------------------------- Danh mục

// Các loại event của hệ thống.
//
// Khai báo tập trung để bên phát và bên nghe không tự gõ chuỗi — một lỗi
// đánh máy ở đây tạo ra event không ai nghe, và không có gì báo lỗi.
const (
	TypeOrderPlaced    = "order.placed"
	TypeOrderPaid      = "order.paid"
	TypeOrderCancelled = "order.cancelled"

	TypeCartItemAdded = "cart.item_added"

	TypeCheckoutStarted   = "checkout.started"
	TypeCheckoutExpired   = "checkout.expired"
	TypeCheckoutCompleted = "checkout.completed"

	TypeInventoryReserved  = "inventory.reserved"
	TypeInventoryCommitted = "inventory.committed"
	TypeInventoryReleased  = "inventory.reservation_released"
)

// Các loại aggregate.
const (
	AggregateOrder    = "Order"
	AggregateCart     = "Cart"
	AggregateCheckout = "Checkout"
	AggregateItem     = "InventoryItem"
)
