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

// WithVersion đặt phiên bản payload của event.
//
// Dùng khi payload thêm một trường mà bên nhận BẮT BUỘC phải có để làm
// đúng việc — xem ADR-0016 phần 1. Thêm trường thuần hiển thị thì KHÔNG
// tăng phiên bản, vì mỗi lần tăng là một lần mọi bên nhận phải khai lại.
//
// Bên phát tăng phiên bản TRƯỚC, bên nhận khai `MaxEventVersion` sau; giữa
// hai lần triển khai, dispatcher HOÃN event thay vì đưa cho bên nhận chưa
// hiểu. Đó là toàn bộ mục đích của cơ chế này.
func (e Event) WithVersion(v int) Event {
	if v > 0 {
		e.Version = v
	}
	return e
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

// DefaultMaxEventVersion là phiên bản cao nhất mà một Handler KHÔNG khai
// báo gì được coi là hiểu.
//
// Là 1 chứ không phải "mọi phiên bản", và đó là cả điểm mấu chốt của
// ADR-0016: mặc định dễ dãi dựng lại đúng sự cố 19/08 — bên nhận cũ vui vẻ
// nuốt event mới rồi bỏ qua trường nó không biết, trong im lặng.
//
// Hôm nay mọi event đều ở v1 nên mặc định này không bắt bên nhận nào phải
// sửa gì. Nó chỉ có tác dụng vào ngày ai đó nâng một loại event lên v2.
const DefaultMaxEventVersion = 1

// VersionedHandler là bên nhận KHAI BÁO phiên bản cao nhất nó hiểu.
//
// Cài interface này khi payload của một loại event lên phiên bản mới và
// bên nhận đã biết đọc phiên bản đó. Không cài thì eventbus coi bên nhận
// chỉ hiểu tới `DefaultMaxEventVersion`.
//
// Vì sao khai ở BÊN NHẬN chứ không kiểm trong `Handle`: bên nhận quên kiểm
// chính là bên nhận gây ra sự cố. Đặt ở dispatcher thì không ai quên được
// — xem ADR-0016 mục "Phương án đã cân nhắc".
type VersionedHandler interface {
	Handler

	// MaxEventVersion trả phiên bản cao nhất bên nhận hiểu cho MỘT loại
	// event. Nhận `eventType` vì một bên nhận nghe nhiều loại, và chúng
	// tiến hóa độc lập với nhau.
	MaxEventVersion(eventType string) int
}

// MaxVersionOf trả phiên bản cao nhất mà `h` hiểu cho `eventType`.
//
// Giá trị khai báo nhỏ hơn 1 được nâng về `DefaultMaxEventVersion`: một
// bên nhận trả 0 (trường int chưa gán) sẽ chặn CẢ event v1, tức là chặn
// mọi thứ đang chạy — hỏng theo kiểu im lặng và toàn diện.
func MaxVersionOf(h Handler, eventType string) int {
	vh, ok := h.(VersionedHandler)
	if !ok {
		return DefaultMaxEventVersion
	}
	if v := vh.MaxEventVersion(eventType); v > DefaultMaxEventVersion {
		return v
	}
	return DefaultMaxEventVersion
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

	// TypeFulfillmentCancelled: một đơn thực hiện bị hủy.
	//
	// TÁCH khỏi fulfillment.progress có chủ ý. progress mang cờ tiến độ
	// cho module order tính trạng thái tổng hợp; event này mang DÒNG HÀNG
	// để inventory trả hàng về kho. Nhồi dòng hàng vào progress sẽ bắt ba
	// bên nhận hiện có tải dữ liệu họ không dùng, và đổi payload đang chạy
	// là việc cần triển khai bên nhận trước (xem domain-events.md mục 8.1).
	TypeFulfillmentCancelled = "fulfillment.cancelled"

	TypeCartItemAdded = "cart.item_added"

	TypeCheckoutStarted   = "checkout.started"
	TypeCheckoutExpired   = "checkout.expired"
	TypeCheckoutCompleted = "checkout.completed"

	TypeFulfillmentProgress = "fulfillment.progress_changed"

	// TypeFulfillmentCompleted: một đơn thực hiện đã qua hạn đổi trả.
	//
	// TÁCH khỏi fulfillment.progress vì nó nói một chuyện KHÁC. progress
	// mang cờ tiến độ giao hàng cho module order tính trạng thái tổng hợp;
	// event này là tín hiệu TÀI CHÍNH — từ đây tiền của nhà bán chuyển từ
	// "đang chờ" sang "rút được".
	//
	// Payload mang sẵn số tiền phải trả nhà bán để payment không phải gọi
	// ngược fulfillment: bên nhận event mà phải hỏi lại bên phát thì hai
	// module dính chặt vào nhau, và một bên chậm làm bên kia chậm theo.
	TypeFulfillmentCompleted = "fulfillment_order.completed"

	// TypeSearchNoResult là khách tìm mà KHÔNG ra kết quả.
	//
	// Đây là tín hiệu NHU CẦU KHÔNG ĐƯỢC ĐÁP ỨNG — thứ dữ liệu bán hàng
	// một mình không bao giờ cho biết. Ghi từ MVP vì nó không tạo ngược
	// được: không ghi hôm nay thì Phase 3 khởi động với lịch sử trống.
	TypeSearchNoResult = "search.no_result"

	TypeInventoryReserved  = "inventory.reserved"
	TypeInventoryCommitted = "inventory.committed"
	TypeInventoryReleased  = "inventory.reservation_released"

	// TypeInventoryDepleted là SKU vừa hết sạch hàng khả dụng.
	//
	// docs/02-domain/domain-events.md gọi đây là "event quan trọng chiến
	// lược": mỗi lần hết hàng là một lần nhu cầu CÓ THẬT bị bỏ lỡ, và nó
	// biến mất khỏi mọi báo cáo doanh số. Module supply-chain tồn tại từ
	// MVP chính vì loại dữ liệu này không tạo ngược được.
	TypeInventoryDepleted = "inventory.depleted"
)

// Các loại aggregate.
const (
	AggregateOrder       = "Order"
	AggregateCart        = "Cart"
	AggregateCheckout    = "Checkout"
	AggregateItem        = "InventoryItem"
	AggregateFulfillment = "FulfillmentOrder"

	// AggregateSearch không phải một aggregate thật — tìm kiếm không có
	// thực thể nào. Dùng để event có đủ trường bắt buộc; định danh là bản
	// băm của từ khóa.
	AggregateSearch = "Search"
)
