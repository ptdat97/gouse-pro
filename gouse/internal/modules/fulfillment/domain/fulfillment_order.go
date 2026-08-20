// Package domain chứa mô hình nghiệp vụ của module fulfillment.
//
// FulfillmentOrder là GÓC NHÌN VẬN HÀNH của đơn hàng, tách khỏi Order —
// hợp đồng với khách. Xem ADR-0007.
//
// Module này SỞ HỮU fulfillment_order; module order chỉ LẮNG NGHE event để
// tính lại trạng thái tổng hợp, không hỏi ngược (tránh phụ thuộc vòng).
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrNotFound      = errors.New("fulfillment: không tìm thấy")
	ErrInvalidStatus = errors.New("fulfillment: chuyển trạng thái không hợp lệ")
	ErrNoLines       = errors.New("fulfillment: phải có ít nhất một dòng hàng")
)

// FulfillmentType là mô hình thực hiện đơn (fulfillment.md mục 4).
type FulfillmentType string

const (
	// TypePlatform: nền tảng giữ hàng, nền tảng đóng gói — own brand.
	TypePlatform FulfillmentType = "PLATFORM"

	// TypeSeller: seller giữ hàng, seller đóng gói — đa số marketplace.
	TypeSeller FulfillmentType = "SELLER"

	// TypePlatformService: seller SỞ HỮU hàng, để ở kho nền tảng, nền tảng
	// đóng gói. Dịch vụ thu phí.
	//
	// Mô hình này là lý do InventoryItem phải tách owner_id khỏi
	// location_id: hàng nằm ở kho nền tảng nhưng KHÔNG phải tài sản của
	// nền tảng — gộp hai khái niệm sẽ làm phồng bảng cân đối kế toán bằng
	// hàng không thuộc về mình.
	TypePlatformService FulfillmentType = "PLATFORM_SERVICE"
)

// FOStatus là trạng thái một đơn vị công việc vận hành.
type FOStatus string

const (
	FOPending FOStatus = "PENDING"

	// FOAllocated: đã phân bổ nguồn hàng, tồn kho đã chuyển sang Committed.
	FOAllocated FOStatus = "ALLOCATED"

	FOConfirmed FOStatus = "CONFIRMED"
	FOPicking   FOStatus = "PICKING"
	FOPacked    FOStatus = "PACKED"

	// FOHandedOver: đã bàn giao cho đơn vị vận chuyển.
	FOHandedOver FOStatus = "HANDED_OVER"

	FOInTransit      FOStatus = "IN_TRANSIT"
	FODeliveryFailed FOStatus = "DELIVERY_FAILED"
	FODelivered      FOStatus = "DELIVERED"

	// FOCompleted: hết hạn đổi trả.
	//
	// TRẠNG THÁI NÀY CÓ Ý NGHĨA TÀI CHÍNH, khác hẳn DELIVERED:
	//
	//	DELIVERED  → số dư seller vẫn Pending
	//	COMPLETED  → số dư chuyển Available, seller được chi trả
	//
	// Đây là cơ chế bảo vệ nền tảng khỏi rủi ro hoàn hàng sau khi đã trả
	// tiền: phần lớn yêu cầu hoàn xảy ra trong thời hạn đổi trả.
	FOCompleted FOStatus = "COMPLETED"

	FOCancelled FOStatus = "CANCELLED"
)

// canTransitionTo mã hóa vòng đời của một FulfillmentOrder.
//
//	PENDING → ALLOCATED → CONFIRMED → PICKING → PACKED
//	        → HANDED_OVER → IN_TRANSIT → DELIVERED → COMPLETED
//
//	IN_TRANSIT → DELIVERY_FAILED → IN_TRANSIT  (thử lại)
//	                             → CANCELLED   (trả về người gửi)
//
// Hủy chỉ được ở PENDING/ALLOCATED/CONFIRMED — từ PICKING trở đi đã tốn
// công và vật tư, cần quy trình riêng có tính chi phí (quy tắc 8).
// StockStillInWarehouse cho biết hàng CÒN NGUYÊN trong kho ở trạng thái
// này hay không.
//
// Quyết định điều gì xảy ra với tồn kho khi hủy:
//
//	PENDING · ALLOCATED · CONFIRMED   hàng còn trong kho  → TRẢ VỀ khả dụng
//	DELIVERY_FAILED                   hàng đang trên đường về → KHÔNG trả
//
// Với DELIVERY_FAILED, hàng đã rời kho và đang được chuyển trả. Trả về
// khả dụng ngay nghĩa là bán một món chưa cầm trong tay: nếu nó hỏng,
// thất lạc, hoặc không bao giờ tới, lỗi hiện ra ở KHÁCH THỨ HAI chứ không
// phải khách đầu — và lúc đó không còn cách nào lần ra nguyên nhân.
//
// Hàng trả về được nhập lại bằng quy trình riêng có bước kiểm tra chất
// lượng (ReceiveReturn → InspectionPassed/Failed ở module inventory).
func (s FOStatus) StockStillInWarehouse() bool {
	switch s {
	case FOPending, FOAllocated, FOConfirmed:
		return true
	default:
		return false
	}
}

func (s FOStatus) canTransitionTo(next FOStatus) bool {
	switch s {
	case FOPending:
		return next == FOAllocated || next == FOConfirmed || next == FOCancelled
	case FOAllocated:
		return next == FOConfirmed || next == FOCancelled
	case FOConfirmed:
		return next == FOPicking || next == FOPacked || next == FOCancelled
	case FOPicking:
		return next == FOPacked
	case FOPacked:
		// ĐÃ ĐÓNG GÓI thì hủy cần quy trình riêng (quy tắc 8): có chi phí
		// phát sinh — công đóng gói, vật tư, và có thể đã bàn giao vận chuyển.
		return next == FOHandedOver
	case FOHandedOver:
		return next == FOInTransit || next == FODelivered
	case FOInTransit:
		return next == FODelivered || next == FODeliveryFailed
	case FODeliveryFailed:
		// Giao lại, hoặc trả về người gửi.
		return next == FOInTransit || next == FOCancelled
	case FODelivered:
		// Chỉ tiến sang COMPLETED khi HẾT HẠN ĐỔI TRẢ — không phải ngay
		// khi giao xong. Đó là ranh giới tài chính, xem FOCompleted.
		return next == FOCompleted
	case FOCompleted, FOCancelled:
		// Trạng thái cuối.
		return false
	}
	return false
}

// IsFinal cho biết đơn thực hiện đã kết thúc.
func (s FOStatus) IsFinal() bool {
	return s == FOCompleted || s == FOCancelled
}

// IsShipped cho biết hàng đã rời kho chưa.
//
// Dùng để tính trạng thái tổng hợp của đơn hàng: đã giao thì cũng đã xuất.
func (s FOStatus) IsShipped() bool {
	switch s {
	case FOHandedOver, FOInTransit, FODeliveryFailed, FODelivered, FOCompleted:
		return true
	}
	return false
}

// IsCancellableWithoutCost cho biết hủy ở trạng thái này có phát sinh chi
// phí không.
//
// Từ PICKING trở đi thì đã tốn công lấy hàng, đóng gói và vật tư — hủy cần
// quy trình riêng, không phải thao tác thông thường (quy tắc 8).
func (s FOStatus) IsCancellableWithoutCost() bool {
	return s == FOPending || s == FOAllocated || s == FOConfirmed
}

// FulfillmentOrder là ĐƠN VỊ CÔNG VIỆC VẬN HÀNH của MỘT nguồn hàng.
//
// TÁCH KHỎI Order LÀ QUYẾT ĐỊNH CỐT LÕI (ADR-0007, quyết định 2). Năm lý do,
// trong đó hai lý do là QUYẾT ĐỊNH:
//
//  3. RÀNG BUỘC BẢO MẬT
//     Seller được xem phần của mình.
//     Seller KHÔNG được xem Order (chứa hàng của seller khác).
//
//  4. TRANH CHẤP GHI
//     Ba seller cập nhật đồng thời sẽ tranh chấp trên cùng một bản ghi
//     Order nếu gộp chung.
//
// Điểm quan trọng nhất về bảo mật: nếu seller truy cập Order thì phải lọc
// dữ liệu ở tầng hiển thị, và QUÊN MỘT LẦN là rò rỉ dữ liệu đối thủ. Với
// FulfillmentOrder, ranh giới nằm sẵn trong CẤU TRÚC DỮ LIỆU — truy vấn
// theo sellerID tự nhiên chỉ trả phần của họ.
type FulfillmentOrder struct {
	id ids.ID

	// orderID trỏ về hợp đồng gốc với khách.
	orderID ids.ID

	// foNumber là mã hiển thị cho seller, dạng <order_number>-A, -B, -C.
	foNumber string

	// sellerID là CHỦ SỞ HỮU của đơn vị công việc này.
	//
	// Đây là trường tạo nên ranh giới bảo mật: mọi truy vấn của seller đều
	// lọc theo cột này, ngay trong SQL.
	sellerID ids.ID

	// lineIDs là các dòng hàng thuộc nguồn này.
	lineIDs []ids.ID

	// lines là ẢNH CHỤP thông tin nhặt hàng, sao chép lúc tách đơn.
	//
	// Seller cần biết NHẶT GÌ. Chỉ có mã dòng thì họ phải mở đơn hàng gốc,
	// mà quy tắc bảo mật không cho — họ sẽ thấy cả hàng của seller khác,
	// email khách và tổng tiền đơn.
	//
	// Có thể RỖNG với đơn tách trước khi có ảnh chụp này; bên gọi phải
	// chịu được, và khi đó lineIDs vẫn dùng được.
	lines []FOLine

	status FOStatus

	// Số tiền của riêng phần này, để seller đối soát.
	subtotal         money.Money
	commissionAmount money.Money

	// cancelReason bắt buộc khi hủy: seller và khách đều cần biết vì sao.
	cancelReason string

	// failureReason khi giao thất bại.
	failureReason string

	// Thông tin liên hệ, NHÂN BẢN từ đơn hàng lúc tách.
	//
	// Nhân bản có chủ ý: module notification không được gọi module nghiệp
	// vụ nào, nên event phát từ đây phải mang theo địa chỉ liên hệ. Xem
	// migration 000014.
	customerID  ids.ID
	notifyEmail string
	notifyPhone string

	// shippingAddress là nơi hàng phải đến — thứ SELLER cần để in phiếu
	// giao hàng.
	//
	// KHÔNG chứa email khách: chỉ những trường cần cho việc giao. Email
	// nằm ở notifyEmail và chỉ dùng cho module notification, KHÔNG lộ ra
	// API của seller.
	//
	// RỖNG với đơn tách trước khi có trường này — bên gọi phải chịu được.
	shippingAddress ShippingAddress

	// stockLocationID là kho xuất hàng.
	//
	// Rỗng với đơn seller tự giao: nền tảng không biết và không cần biết
	// seller lấy hàng từ đâu.
	stockLocationID ids.ID

	// Vận chuyển. Tên nhà vận chuyển là DỮ LIỆU, không phải mã nguồn —
	// giá và chất lượng của các đối tác thay đổi thường xuyên (P13).
	fulfillmentType   FulfillmentType
	shippingMethod    string
	shippingProvider  string
	trackingNumber    string
	estimatedDelivery time.Time

	completedAt time.Time

	confirmedAt time.Time
	packedAt    time.Time
	shippedAt   time.Time
	deliveredAt time.Time
	cancelledAt time.Time

	createdAt time.Time
	updatedAt time.Time
}

// ShippingAddress là nơi hàng phải đến.
//
// Chỉ những trường CẦN cho việc giao hàng. Không có email khách, không có
// lịch sử mua hàng — seller chỉ cần biết gửi tới đâu và gọi ai.
type ShippingAddress struct {
	RecipientName string

	// Phone để gọi trước khi giao. Đây là dữ liệu cá nhân, và nó ở đây vì
	// KHÔNG giao được hàng nếu không liên hệ được người nhận.
	Phone string

	StreetAddress string
	Ward          string
	District      string
	Province      string
	CountryCode   string
}

// IsEmpty cho biết địa chỉ có dùng được không.
//
// Thiếu người nhận hoặc thiếu đường phố thì phiếu giao hàng vô nghĩa.
func (a ShippingAddress) IsEmpty() bool {
	return strings.TrimSpace(a.RecipientName) == "" ||
		strings.TrimSpace(a.StreetAddress) == ""
}

// FOLine là một dòng hàng trong đơn thực hiện — góc nhìn NHẶT HÀNG.
//
// Sao chép từ payload event lúc tách đơn, không tham chiếu order_line: đây
// là con số seller đã thấy lúc giao hàng, còn order_line hiện tại có thể đã
// khác (hủy một phần, điều chỉnh).
type FOLine struct {
	OrderLineID ids.ID
	SKUID       ids.ID

	ProductName string

	// VariantDescription ("Trắng / M") quyết định nhặt ĐÚNG ô kệ nào.
	VariantDescription string

	Quantity int

	// Có CẢ UnitPrice lẫn LineTotal: chia LineTotal cho Quantity là phép
	// chia số nguyên và nó làm tròn sai với giá không chia hết.
	UnitPrice money.Money
	LineTotal money.Money
}

type NewFulfillmentOrderParams struct {
	OrderID          ids.ID
	FONumber         string
	SellerID         ids.ID
	LineIDs          []ids.ID
	Lines            []FOLine
	Subtotal         money.Money
	CommissionAmount money.Money

	// Thông tin liên hệ để thông báo cho khách về đơn này.
	CustomerID  ids.ID
	NotifyEmail string
	NotifyPhone string

	// ShippingAddress là nơi hàng phải đến — seller cần để in phiếu giao.
	ShippingAddress ShippingAddress

	// Type mặc định SELLER nếu để trống — đó là trường hợp phổ biến nhất
	// của marketplace.
	Type FulfillmentType

	Now time.Time
}

// NewFulfillmentOrder tạo một đơn vị công việc vận hành.
func NewFulfillmentOrder(p NewFulfillmentOrderParams) (*FulfillmentOrder, error) {
	if p.OrderID.IsZero() {
		return nil, errors.New("order: đơn thực hiện phải trỏ về đơn hàng gốc")
	}
	if p.SellerID.IsZero() {
		return nil, errors.New("order: đơn thực hiện phải có nhà bán")
	}
	if len(p.LineIDs) == 0 {
		return nil, errors.New("order: đơn thực hiện phải có ít nhất một dòng hàng")
	}

	id, err := ids.New(ids.PrefixFulfillmentOrder)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	fulfillmentType := p.Type
	if fulfillmentType == "" {
		fulfillmentType = TypeSeller
	}

	return &FulfillmentOrder{
		id:               id,
		orderID:          p.OrderID,
		foNumber:         p.FONumber,
		sellerID:         p.SellerID,
		lineIDs:          append([]ids.ID(nil), p.LineIDs...),
		lines:            append([]FOLine(nil), p.Lines...),
		status:           FOPending,
		subtotal:         p.Subtotal,
		commissionAmount: p.CommissionAmount,
		customerID:       p.CustomerID,
		notifyEmail:      strings.TrimSpace(p.NotifyEmail),
		notifyPhone:      strings.TrimSpace(p.NotifyPhone),
		shippingAddress:  p.ShippingAddress,
		fulfillmentType:  fulfillmentType,
		createdAt:        now,
		updatedAt:        now,
	}, nil
}

// RestoreFOParams dựng lại từ kho lưu trữ.
type RestoreFOParams struct {
	ID                ids.ID
	OrderID           ids.ID
	FONumber          string
	SellerID          ids.ID
	LineIDs           []ids.ID
	Lines             []FOLine
	Status            FOStatus
	Subtotal          money.Money
	CommissionAmount  money.Money
	CancelReason      string
	FailureReason     string
	CustomerID        ids.ID
	NotifyEmail       string
	NotifyPhone       string
	ShippingAddress   ShippingAddress
	StockLocationID   ids.ID
	Type              FulfillmentType
	ShippingMethod    string
	ShippingProvider  string
	TrackingNumber    string
	EstimatedDelivery time.Time
	CompletedAt       time.Time
	ConfirmedAt       time.Time
	PackedAt          time.Time
	ShippedAt         time.Time
	DeliveredAt       time.Time
	CancelledAt       time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// RestoreFulfillmentOrder dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreFulfillmentOrder(p RestoreFOParams) *FulfillmentOrder {
	return &FulfillmentOrder{
		id:                p.ID,
		orderID:           p.OrderID,
		foNumber:          p.FONumber,
		sellerID:          p.SellerID,
		lineIDs:           p.LineIDs,
		lines:             p.Lines,
		status:            p.Status,
		subtotal:          p.Subtotal,
		commissionAmount:  p.CommissionAmount,
		cancelReason:      p.CancelReason,
		failureReason:     p.FailureReason,
		customerID:        p.CustomerID,
		notifyEmail:       p.NotifyEmail,
		notifyPhone:       p.NotifyPhone,
		shippingAddress:   p.ShippingAddress,
		stockLocationID:   p.StockLocationID,
		fulfillmentType:   p.Type,
		shippingMethod:    p.ShippingMethod,
		shippingProvider:  p.ShippingProvider,
		trackingNumber:    p.TrackingNumber,
		estimatedDelivery: p.EstimatedDelivery,
		completedAt:       p.CompletedAt,
		confirmedAt:       p.ConfirmedAt,
		packedAt:          p.PackedAt,
		shippedAt:         p.ShippedAt,
		deliveredAt:       p.DeliveredAt,
		cancelledAt:       p.CancelledAt,
		createdAt:         p.CreatedAt,
		updatedAt:         p.UpdatedAt,
	}
}

func (f *FulfillmentOrder) ID() ids.ID                    { return f.id }
func (f *FulfillmentOrder) OrderID() ids.ID               { return f.orderID }
func (f *FulfillmentOrder) FONumber() string              { return f.foNumber }
func (f *FulfillmentOrder) SellerID() ids.ID              { return f.sellerID }
func (f *FulfillmentOrder) Status() FOStatus              { return f.status }
func (f *FulfillmentOrder) Subtotal() money.Money         { return f.subtotal }
func (f *FulfillmentOrder) CommissionAmount() money.Money { return f.commissionAmount }
func (f *FulfillmentOrder) CancelReason() string          { return f.cancelReason }
func (f *FulfillmentOrder) FailureReason() string         { return f.failureReason }
func (f *FulfillmentOrder) CustomerID() ids.ID            { return f.customerID }
func (f *FulfillmentOrder) NotifyEmail() string           { return f.notifyEmail }
func (f *FulfillmentOrder) NotifyPhone() string           { return f.notifyPhone }

// ShippingAddress là nơi hàng phải đến. RỖNG với đơn tách trước khi có
// trường này.
func (f *FulfillmentOrder) ShippingAddress() ShippingAddress {
	return f.shippingAddress
}
func (f *FulfillmentOrder) StockLocationID() ids.ID      { return f.stockLocationID }
func (f *FulfillmentOrder) Type() FulfillmentType        { return f.fulfillmentType }
func (f *FulfillmentOrder) ShippingMethod() string       { return f.shippingMethod }
func (f *FulfillmentOrder) ShippingProvider() string     { return f.shippingProvider }
func (f *FulfillmentOrder) TrackingNumber() string       { return f.trackingNumber }
func (f *FulfillmentOrder) EstimatedDelivery() time.Time { return f.estimatedDelivery }
func (f *FulfillmentOrder) CompletedAt() time.Time       { return f.completedAt }
func (f *FulfillmentOrder) ConfirmedAt() time.Time       { return f.confirmedAt }
func (f *FulfillmentOrder) PackedAt() time.Time          { return f.packedAt }
func (f *FulfillmentOrder) ShippedAt() time.Time         { return f.shippedAt }
func (f *FulfillmentOrder) DeliveredAt() time.Time       { return f.deliveredAt }
func (f *FulfillmentOrder) CancelledAt() time.Time       { return f.cancelledAt }
func (f *FulfillmentOrder) CreatedAt() time.Time         { return f.createdAt }
func (f *FulfillmentOrder) UpdatedAt() time.Time         { return f.updatedAt }

// Lines trả bản sao ảnh chụp thông tin nhặt hàng.
//
// RỖNG với đơn tách trước khi có ảnh chụp này — bên gọi phải chịu được và
// dùng LineIDs thay thế.
func (f *FulfillmentOrder) Lines() []FOLine {
	return append([]FOLine(nil), f.lines...)
}

// LineIDs trả bản sao danh sách dòng hàng.
func (f *FulfillmentOrder) LineIDs() []ids.ID {
	return append([]ids.ID(nil), f.lineIDs...)
}

// SellerPayable là số tiền phải trả seller cho phần này.
func (f *FulfillmentOrder) SellerPayable() money.Money {
	payable, _ := f.subtotal.Sub(f.commissionAmount)
	return payable
}

// BelongsTo cho biết đơn thực hiện này có thuộc về seller không.
//
// Hàng rào cuối cùng ở tầng domain. Truy vấn đã lọc theo sellerID, nhưng
// một lần gọi FindByID với id lấy từ nơi khác vẫn có thể lọt — hàm này để
// tầng application kiểm tra tường minh.
func (f *FulfillmentOrder) BelongsTo(sellerID ids.ID) bool {
	return f.sellerID == sellerID
}

// ---------------------------------------------------------------- Hành vi

func (f *FulfillmentOrder) Confirm(now time.Time) error {
	if err := f.transition(FOConfirmed, now); err != nil {
		return err
	}
	f.confirmedAt = now
	return nil
}

func (f *FulfillmentOrder) Pack(now time.Time) error {
	if err := f.transition(FOPacked, now); err != nil {
		return err
	}
	f.packedAt = now
	return nil
}

// Allocate phân bổ nguồn hàng: chọn kho, tồn kho chuyển sang Committed.
func (f *FulfillmentOrder) Allocate(locationID ids.ID, now time.Time) error {
	if err := f.transition(FOAllocated, now); err != nil {
		return err
	}
	f.stockLocationID = locationID
	return nil
}

// Pick bắt đầu lấy hàng.
func (f *FulfillmentOrder) Pick(now time.Time) error {
	return f.transition(FOPicking, now)
}

// HandOver bàn giao cho đơn vị vận chuyển.
//
// Từ đây hàng ra khỏi tầm kiểm soát của seller, nên mã vận đơn là BẮT
// BUỘC: không có nó thì không ai trả lời được "hàng của tôi đang ở đâu".
func (f *FulfillmentOrder) HandOver(provider, trackingNumber string, now time.Time) error {
	if strings.TrimSpace(trackingNumber) == "" {
		return errors.New("fulfillment: bàn giao vận chuyển bắt buộc phải có mã vận đơn")
	}
	if err := f.transition(FOHandedOver, now); err != nil {
		return err
	}
	f.shippingProvider = strings.TrimSpace(provider)
	f.trackingNumber = strings.TrimSpace(trackingNumber)
	f.shippedAt = now
	return nil
}

// MarkInTransit ghi nhận hàng đang trên đường.
func (f *FulfillmentOrder) MarkInTransit(now time.Time) error {
	return f.transition(FOInTransit, now)
}

// MarkDeliveryFailed ghi nhận giao thất bại.
//
// Lý do BẮT BUỘC: khách cần biết vì sao chưa nhận được hàng, và bộ phận
// vận hành cần biết có nên giao lại hay trả về.
func (f *FulfillmentOrder) MarkDeliveryFailed(reason string, now time.Time) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("fulfillment: giao thất bại bắt buộc phải nêu lý do")
	}
	if err := f.transition(FODeliveryFailed, now); err != nil {
		return err
	}
	f.failureReason = strings.TrimSpace(reason)
	return nil
}

func (f *FulfillmentOrder) Deliver(now time.Time) error {
	if err := f.transition(FODelivered, now); err != nil {
		return err
	}
	f.deliveredAt = now
	f.failureReason = ""
	return nil
}

// Complete đánh dấu hết hạn đổi trả.
//
// ĐÂY LÀ RANH GIỚI TÀI CHÍNH, không phải bước vận hành:
//
//	DELIVERED  → số dư seller vẫn Pending
//	COMPLETED  → số dư chuyển Available, seller được chi trả
//
// Gọi bởi tiến trình nền sau khi hết thời hạn đổi trả. Gọi sớm nghĩa là
// trả tiền cho seller trước khi biết khách có trả hàng không.
func (f *FulfillmentOrder) Complete(now time.Time) error {
	if err := f.transition(FOCompleted, now); err != nil {
		return err
	}
	f.completedAt = now
	return nil
}

// Cancel hủy đơn vị công việc này.
//
// Lý do là BẮT BUỘC: seller cần biết vì sao bị hủy, và khách cần lời giải
// thích khi nhận thông báo.
//
// Từ PACKED trở đi KHÔNG hủy được bằng hàm này (quy tắc 8) — đã tốn công
// đóng gói và có thể đã bàn giao vận chuyển, nên cần quy trình riêng có
// tính chi phí.
func (f *FulfillmentOrder) Cancel(reason string, now time.Time) error {
	if reason == "" {
		return errors.New("order: hủy đơn thực hiện bắt buộc phải nêu lý do")
	}
	if err := f.transition(FOCancelled, now); err != nil {
		return err
	}
	f.cancelReason = reason
	f.cancelledAt = now
	return nil
}

func (f *FulfillmentOrder) transition(next FOStatus, now time.Time) error {
	if !f.status.canTransitionTo(next) {
		return ErrInvalidStatus
	}
	f.status = next
	if now.IsZero() {
		now = time.Now().UTC()
	}
	f.updatedAt = now
	return nil
}

// ---------------------------------------------------------------- Tách đơn

// SplitIntoFulfillmentOrders tách đơn hàng thành các đơn vị công việc.
//
// MỘT FulfillmentOrder cho MỖI SELLER (mục 3.2 của ADR-0007):
//
//	Giỏ hàng:
//	├── Áo own brand   (kho nền tảng, Hà Nội)
//	├── Giày Seller A  (kho seller A, TP.HCM)
//	└── Túi Seller B   (kho seller B, Đà Nẵng)
//
//	Ba món KHÔNG THỂ đóng chung một gói.
//
// Own brand cũng được tách như seller bình thường: nó là một seller nội bộ
// (INTERNAL), nên đơn lẫn own brand và hàng seller đi CHUNG một luồng.
//
// Mã FO đánh theo thứ tự: <order_number>-A, -B, -C. Seller thấy mã của
// mình mà không cần biết có bao nhiêu seller khác trong đơn.
// SplitInput là dữ liệu cần để tách đơn.
//
// Nhận dữ liệu THUẦN chứ không nhận *order.Order: module này không được
// phụ thuộc module order — đó là phụ thuộc ngược, và nó sẽ tạo vòng khi
// order lắng nghe event từ đây.
//
// Bên gọi (tầng application) dịch từ event sang cấu trúc này.
type SplitInput struct {
	OrderID     ids.ID
	OrderNumber string
	Currency    money.Currency

	// Thông tin liên hệ, sao chép xuống từng đơn thực hiện.
	//
	// Cần thiết để event phát từ module này mang theo địa chỉ — module
	// notification không được gọi ngược để lấy.
	CustomerID  ids.ID
	NotifyEmail string
	NotifyPhone string

	// ShippingAddress sao chép xuống TỪNG đơn thực hiện: mỗi seller giao
	// phần của mình tới cùng một nơi, và mỗi người cần phiếu giao riêng.
	ShippingAddress ShippingAddress

	Lines []SplitLine
}

// SplitLine là một dòng hàng cần phân về nguồn.
type SplitLine struct {
	LineID   ids.ID
	SellerID ids.ID
	SKUID    ids.ID
	Quantity int

	// ProductName và VariantDescription là thứ SELLER cần để NHẶT ĐÚNG
	// hàng. Sao chép xuống đơn thực hiện chứ không tra ngược module order:
	// seller không được phép xem đơn hàng gốc.
	ProductName        string
	VariantDescription string

	UnitPrice        money.Money
	LineTotal        money.Money
	CommissionAmount money.Money
}

// foSuffix sinh hậu tố phân biệt các đơn thực hiện của cùng một đơn hàng:
// A, B, … Z, AA, AB, …
//
// # Vì sao không phải string(rune('A'+i))
//
// Cách đó đúng tới nhà bán thứ 26 rồi lặng lẽ tràn sang ký tự khác: đơn
// thứ 27 nhận hậu tố "[", thứ 28 nhận "\". Không có lỗi nào được báo —
// chỉ là mã đơn thực hiện chứa ký tự thoát, đi vào URL, log và mã vạch.
//
// 27 nhà bán trong một đơn nghe như chuyện không xảy ra, cho tới khi có
// một giỏ hàng lớn. Chi phí để đúng ở đây là bảy dòng.
func foSuffix(i int) string {
	const letters = 26
	out := ""
	for {
		out = string(rune('A'+i%letters)) + out
		i = i/letters - 1
		if i < 0 {
			return out
		}
	}
}

func SplitIntoFulfillmentOrders(in SplitInput, now time.Time) ([]*FulfillmentOrder, error) {
	lines := in.Lines
	if len(lines) == 0 {
		return nil, ErrNoLines
	}

	// Gom dòng hàng theo seller, GIỮ THỨ TỰ xuất hiện để mã FO ổn định:
	// chạy lại phải ra cùng kết quả, không phụ thuộc thứ tự duyệt map.
	type group struct {
		sellerID   ids.ID
		lineIDs    []ids.ID
		subtotal   money.Money
		commission money.Money
		lines      []FOLine
	}
	var groups []*group
	index := map[ids.ID]*group{}

	for _, l := range lines {
		g, ok := index[l.SellerID]
		if !ok {
			g = &group{
				sellerID:   l.SellerID,
				subtotal:   money.Zero(in.Currency),
				commission: money.Zero(in.Currency),
			}
			index[l.SellerID] = g
			groups = append(groups, g)
		}
		g.lineIDs = append(g.lineIDs, l.LineID)
		g.lines = append(g.lines, FOLine{
			OrderLineID:        l.LineID,
			SKUID:              l.SKUID,
			ProductName:        l.ProductName,
			VariantDescription: l.VariantDescription,
			Quantity:           l.Quantity,
			UnitPrice:          l.UnitPrice,
			LineTotal:          l.LineTotal,
		})
		g.subtotal, _ = g.subtotal.Add(l.LineTotal)
		g.commission, _ = g.commission.Add(l.CommissionAmount)
	}

	out := make([]*FulfillmentOrder, 0, len(groups))
	for i, g := range groups {
		fo, err := NewFulfillmentOrder(NewFulfillmentOrderParams{
			OrderID:          in.OrderID,
			FONumber:         in.OrderNumber + "-" + foSuffix(i),
			SellerID:         g.sellerID,
			LineIDs:          g.lineIDs,
			Lines:            g.lines,
			Subtotal:         g.subtotal,
			CommissionAmount: g.commission,
			CustomerID:       in.CustomerID,
			NotifyEmail:      in.NotifyEmail,
			NotifyPhone:      in.NotifyPhone,
			ShippingAddress:  in.ShippingAddress,
			Now:              now,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, fo)
	}
	return out, nil
}
