// Package domain chứa mô hình nghiệp vụ của module order.
//
// RANH GIỚI QUAN TRỌNG NHẤT (ADR-0007):
//
//	Order            = HỢP ĐỒNG VỚI KHÁCH HÀNG
//	FulfillmentOrder = ĐƠN VỊ CÔNG VIỆC VẬN HÀNH
//
// Order giữ "khách mua gì, giá nào". FulfillmentOrder giữ "ai giao, đến
// đâu". Hai thứ có chủ sở hữu khác nhau, vòng đời khác nhau, và quan trọng
// nhất: RANH GIỚI BẢO MẬT khác nhau.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrNoLines         = errors.New("order: đơn hàng phải có ít nhất một dòng")
	ErrNoCustomer      = errors.New("order: đơn phải thuộc khách đã đăng ký hoặc có email khách vãng lai")
	ErrInvalidStatus   = errors.New("order: chuyển trạng thái không hợp lệ")
	ErrNotCancellable  = errors.New("order: đơn không còn hủy được")
	ErrMissingIdempKey = errors.New("order: thiếu khóa idempotency")
	ErrTotalMismatch   = errors.New("order: tổng tiền không khớp với các dòng hàng")
	ErrNotFound        = errors.New("order: không tìm thấy")
	ErrDuplicateOrder  = errors.New("order: đơn với khóa idempotency này đã tồn tại")
)

// Status là trạng thái tổng hợp của đơn hàng.
//
// LƯU Ý QUAN TRỌNG (quy tắc 7): trạng thái này được SUY RA từ trạng thái
// các FulfillmentOrder, KHÔNG tự đặt tùy tiện. Xem RecalculateStatus.
type Status string

const (
	StatusPendingPayment     Status = "PENDING_PAYMENT"
	StatusPaid               Status = "PAID"
	StatusProcessing         Status = "PROCESSING"
	StatusPartiallyShipped   Status = "PARTIALLY_SHIPPED"
	StatusShipped            Status = "SHIPPED"
	StatusPartiallyDelivered Status = "PARTIALLY_DELIVERED"
	StatusDelivered          Status = "DELIVERED"
	StatusPartiallyCancelled Status = "PARTIALLY_CANCELLED"
	StatusCancelled          Status = "CANCELLED"
	StatusCompleted          Status = "COMPLETED"
)

// IsFinal cho biết đơn đã kết thúc, không đổi trạng thái nữa.
func (s Status) IsFinal() bool {
	return s == StatusCompleted || s == StatusCancelled
}

// CanCancelWholeOrder cho biết còn hủy TOÀN PHẦN được không.
//
// Điều kiện thật (mục 6.1): chưa có FulfillmentOrder nào ở trạng thái
// PACKED trở đi. Hàm này chỉ chặn theo trạng thái tổng hợp; kiểm tra chi
// tiết do tầng application làm khi có danh sách FO.
func (s Status) CanCancelWholeOrder() bool {
	return s == StatusPendingPayment || s == StatusPaid || s == StatusProcessing
}

// LineStatus là trạng thái một dòng hàng.
type LineStatus string

const (
	LineActive    LineStatus = "ACTIVE"
	LineCancelled LineStatus = "CANCELLED"
	LineReturned  LineStatus = "RETURNED"
)

// Order là aggregate root — HỢP ĐỒNG với khách hàng.
//
// Gần như BẤT BIẾN sau khi đặt: quy tắc 2 nói tổng tiền không đổi, trừ khi
// hủy từng phần. Mọi thông tin giao dịch đã ĐÓNG BĂNG.
type Order struct {
	id          ids.ID
	orderNumber string

	// Khách đã đăng ký HOẶC khách vãng lai — quy tắc 6 cho phép khách
	// vãng lai đặt hàng để giảm rào cản chuyển đổi.
	customerID ids.ID
	guestEmail string
	guestPhone string

	// Địa chỉ ĐÓNG BĂNG: khách sửa sổ địa chỉ sau này không được làm thay
	// đổi nơi đơn cũ đã giao tới.
	shippingAddress Address
	billingAddress  Address

	currency money.Currency

	// Các khoản tiền ở mức đơn hàng.
	shippingFee    money.Money
	discountAmount money.Money
	taxAmount      money.Money

	status Status
	lines  []*Line

	// idempotencyKey chống tạo đơn trùng (quy tắc 5).
	//
	// Khách bấm "Đặt hàng" hai lần, hoặc client thử lại sau timeout —
	// không được tạo hai đơn.
	idempotencyKey string

	placedAt    time.Time
	completedAt time.Time
	createdAt   time.Time
	updatedAt   time.Time
}

// Address là địa chỉ ĐÓNG BĂNG tại thời điểm đặt hàng.
type Address struct {
	RecipientName string
	Phone         string
	StreetAddress string
	Ward          string
	District      string
	Province      string
	CountryCode   string
}

func (a Address) IsEmpty() bool {
	return a.RecipientName == "" && a.StreetAddress == ""
}

type NewOrderParams struct {
	OrderNumber string
	CustomerID  ids.ID
	GuestEmail  string
	GuestPhone  string

	ShippingAddress Address
	BillingAddress  Address

	Currency       money.Currency
	ShippingFee    money.Money
	DiscountAmount money.Money
	TaxAmount      money.Money

	Lines          []*Line
	IdempotencyKey string
	Now            time.Time
}

// NewOrder tạo đơn hàng ở trạng thái PENDING_PAYMENT.
func NewOrder(p NewOrderParams) (*Order, error) {
	if len(p.Lines) == 0 {
		return nil, ErrNoLines
	}

	// Quy tắc 6: khách vãng lai được đặt hàng, nhưng phải có cách liên hệ.
	// Đơn không biết thuộc về ai và không liên hệ được là đơn không giao được.
	if p.CustomerID.IsZero() && strings.TrimSpace(p.GuestEmail) == "" {
		return nil, ErrNoCustomer
	}

	if strings.TrimSpace(p.IdempotencyKey) == "" {
		return nil, ErrMissingIdempKey
	}

	currency := p.Currency
	if currency == "" {
		currency = p.Lines[0].UnitPrice().Currency()
	}

	// Mọi dòng phải cùng đơn vị tiền tệ với đơn: cộng tiền khác đơn vị ra
	// một con số vô nghĩa.
	for i, l := range p.Lines {
		if l.UnitPrice().Currency() != currency {
			return nil, fmt.Errorf("order: dòng %d dùng %s, đơn dùng %s",
				i+1, l.UnitPrice().Currency(), currency)
		}
	}

	id, err := ids.New(ids.PrefixOrder)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	o := &Order{
		id:              id,
		orderNumber:     strings.TrimSpace(p.OrderNumber),
		customerID:      p.CustomerID,
		guestEmail:      strings.TrimSpace(p.GuestEmail),
		guestPhone:      strings.TrimSpace(p.GuestPhone),
		shippingAddress: p.ShippingAddress,
		billingAddress:  p.BillingAddress,
		currency:        currency,
		shippingFee:     zeroIfEmpty(p.ShippingFee, currency),
		discountAmount:  zeroIfEmpty(p.DiscountAmount, currency),
		taxAmount:       zeroIfEmpty(p.TaxAmount, currency),
		status:          StatusPendingPayment,
		lines:           append([]*Line(nil), p.Lines...),
		idempotencyKey:  strings.TrimSpace(p.IdempotencyKey),
		placedAt:        now,
		createdAt:       now,
		updatedAt:       now,
	}
	return o, nil
}

func zeroIfEmpty(m money.Money, c money.Currency) money.Money {
	if m.Currency() == "" {
		return money.Zero(c)
	}
	return m
}

// RestoreOrderParams dựng lại từ kho lưu trữ.
type RestoreOrderParams struct {
	ID              ids.ID
	OrderNumber     string
	CustomerID      ids.ID
	GuestEmail      string
	GuestPhone      string
	ShippingAddress Address
	BillingAddress  Address
	Currency        money.Currency
	ShippingFee     money.Money
	DiscountAmount  money.Money
	TaxAmount       money.Money
	Status          Status
	Lines           []*Line
	IdempotencyKey  string
	PlacedAt        time.Time
	CompletedAt     time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RestoreOrder dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreOrder(p RestoreOrderParams) *Order {
	return &Order{
		id:              p.ID,
		orderNumber:     p.OrderNumber,
		customerID:      p.CustomerID,
		guestEmail:      p.GuestEmail,
		guestPhone:      p.GuestPhone,
		shippingAddress: p.ShippingAddress,
		billingAddress:  p.BillingAddress,
		currency:        p.Currency,
		shippingFee:     p.ShippingFee,
		discountAmount:  p.DiscountAmount,
		taxAmount:       p.TaxAmount,
		status:          p.Status,
		lines:           p.Lines,
		idempotencyKey:  p.IdempotencyKey,
		placedAt:        p.PlacedAt,
		completedAt:     p.CompletedAt,
		createdAt:       p.CreatedAt,
		updatedAt:       p.UpdatedAt,
	}
}

func (o *Order) ID() ids.ID                  { return o.id }
func (o *Order) OrderNumber() string         { return o.orderNumber }
func (o *Order) CustomerID() ids.ID          { return o.customerID }
func (o *Order) GuestEmail() string          { return o.guestEmail }
func (o *Order) GuestPhone() string          { return o.guestPhone }
func (o *Order) ShippingAddress() Address    { return o.shippingAddress }
func (o *Order) BillingAddress() Address     { return o.billingAddress }
func (o *Order) Currency() money.Currency    { return o.currency }
func (o *Order) ShippingFee() money.Money    { return o.shippingFee }
func (o *Order) DiscountAmount() money.Money { return o.discountAmount }
func (o *Order) TaxAmount() money.Money      { return o.taxAmount }
func (o *Order) Status() Status              { return o.status }
func (o *Order) IdempotencyKey() string      { return o.idempotencyKey }
func (o *Order) PlacedAt() time.Time         { return o.placedAt }
func (o *Order) CompletedAt() time.Time      { return o.completedAt }
func (o *Order) CreatedAt() time.Time        { return o.createdAt }
func (o *Order) UpdatedAt() time.Time        { return o.updatedAt }

// IsGuestOrder cho biết đây có phải đơn của khách vãng lai không.
func (o *Order) IsGuestOrder() bool { return o.customerID.IsZero() }

// Lines trả về bản sao lát cắt.
func (o *Order) Lines() []*Line {
	return append([]*Line(nil), o.lines...)
}

// ActiveLines trả các dòng CÒN HIỆU LỰC (chưa hủy, chưa trả).
//
// Dùng để tính tổng tiền thực nhận sau khi hủy từng phần.
func (o *Order) ActiveLines() []*Line {
	var out []*Line
	for _, l := range o.lines {
		if l.Status() == LineActive {
			out = append(out, l)
		}
	}
	return out
}

// LineByID tìm dòng theo định danh.
func (o *Order) LineByID(id ids.ID) (*Line, bool) {
	for _, l := range o.lines {
		if l.ID() == id {
			return l, true
		}
	}
	return nil, false
}

// Subtotal là tổng tiền hàng của các dòng CÒN HIỆU LỰC.
func (o *Order) Subtotal() money.Money {
	sum := money.Zero(o.currency)
	for _, l := range o.ActiveLines() {
		sum, _ = sum.Add(l.LineTotal())
	}
	return sum
}

// Total là tổng tiền khách phải trả.
//
//	subtotal + phí ship + thuế − giảm giá
//
// Tính từ các dòng CÒN HIỆU LỰC, nên hủy từng phần tự động làm giảm tổng.
//
// LƯU Ý về phí ship khi hủy một phần (mục 6.3): KHÔNG thu lại phí ship dù
// đơn không còn đạt ngưỡng miễn phí. Chi phí xử lý tranh chấp và tổn hại
// trải nghiệm lớn hơn số tiền thu về, và khách bị phạt vì lỗi của seller.
func (o *Order) Total() money.Money {
	total := o.Subtotal()
	total, _ = total.Add(o.shippingFee)
	total, _ = total.Add(o.taxAmount)
	total, _ = total.Sub(o.discountAmount)
	return total
}

// SellerIDs trả danh sách seller có hàng trong đơn, không trùng lặp.
//
// Đây là đầu vào để TÁCH ĐƠN: mỗi seller thành một FulfillmentOrder.
func (o *Order) SellerIDs() []ids.ID {
	seen := map[ids.ID]bool{}
	var out []ids.ID
	for _, l := range o.ActiveLines() {
		if seen[l.SellerID()] {
			continue
		}
		seen[l.SellerID()] = true
		out = append(out, l.SellerID())
	}
	return out
}

// ---------------------------------------------------------------- Hành vi

// MarkPaid chuyển sang PAID khi thanh toán thành công.
func (o *Order) MarkPaid(now time.Time) error {
	if o.status != StatusPendingPayment {
		return ErrInvalidStatus
	}
	o.status = StatusPaid
	o.touch(now)
	return nil
}

// MarkProcessing chuyển sang PROCESSING khi đã tạo FulfillmentOrder.
func (o *Order) MarkProcessing(now time.Time) error {
	if o.status != StatusPaid {
		return ErrInvalidStatus
	}
	o.status = StatusProcessing
	o.touch(now)
	return nil
}

// Cancel hủy TOÀN BỘ đơn.
//
// Điều kiện: chưa có FulfillmentOrder nào đóng gói xong (mục 6.1). Kiểm
// tra chi tiết do tầng application làm; ở đây chỉ chặn theo trạng thái.
func (o *Order) Cancel(now time.Time) error {
	if !o.status.CanCancelWholeOrder() {
		return ErrNotCancellable
	}
	for _, l := range o.lines {
		if l.Status() == LineActive {
			l.cancel(now)
		}
	}
	o.status = StatusCancelled
	o.touch(now)
	return nil
}

// CancelLine hủy MỘT dòng hàng (hủy từng phần).
//
// Tình huống phổ biến: seller B hết hàng, hai seller còn lại vẫn giao được.
//
// Tổng tiền tự động giảm vì Total() tính từ ActiveLines. Phí ship KHÔNG
// thu lại (mục 6.3).
func (o *Order) CancelLine(lineID ids.ID, now time.Time) error {
	if o.status.IsFinal() {
		return ErrNotCancellable
	}

	line, ok := o.LineByID(lineID)
	if !ok {
		return ErrNotFound
	}
	if line.Status() != LineActive {
		return ErrNotCancellable
	}

	line.cancel(now)

	// Hủy dòng cuối cùng nghĩa là hủy cả đơn.
	if len(o.ActiveLines()) == 0 {
		o.status = StatusCancelled
	} else {
		o.status = StatusPartiallyCancelled
	}
	o.touch(now)
	return nil
}

// Complete chốt đơn khi hết hạn đổi trả.
//
// PHÂN BIỆT DELIVERED và COMPLETED (mục 5.3) — đây là phân biệt có ý nghĩa
// TÀI CHÍNH:
//
//	DELIVERED  → hàng đã đến tay khách, số dư seller vẫn PENDING
//	COMPLETED  → hết hạn đổi trả, số dư chuyển sang AVAILABLE, được payout
//
// Phân biệt này bảo vệ nền tảng: trả tiền seller ngay khi giao hàng thì
// khi khách hoàn hàng phải đòi lại tiền — rất khó thu hồi.
func (o *Order) Complete(now time.Time) error {
	if o.status != StatusDelivered && o.status != StatusPartiallyDelivered {
		return ErrInvalidStatus
	}
	o.status = StatusCompleted
	o.completedAt = now
	o.touch(now)
	return nil
}

// RecalculateStatus tính lại trạng thái tổng hợp TỪ các FulfillmentOrder.
//
// QUY TẮC 7 (mục 12): trạng thái tổng hợp SUY RA từ FO, không tự đặt.
//
// Module order LẮNG NGHE event từ fulfillment và gọi hàm này. Nó KHÔNG hỏi
// ngược fulfillment — hỏi ngược tạo phụ thuộc vòng giữa hai module.
//
// Quy tắc tính (mục 5.2):
//
//	Tất cả FO đã hủy      → CANCELLED
//	Một số FO đã hủy      → PARTIALLY_CANCELLED
//	Tất cả FO đã giao     → DELIVERED
//	Một số FO đã giao     → PARTIALLY_DELIVERED
//	Tất cả FO đã xuất     → SHIPPED
//	Một số FO đã xuất     → PARTIALLY_SHIPPED
//
// Trả về true nếu trạng thái ĐÃ ĐỔI. Bên gọi dùng nó để bỏ qua lần ghi
// database không cần thiết: mỗi bước của một seller kích hoạt hàm này, mà
// phần lớn các bước không làm đổi trạng thái tổng hợp.
// FulfillmentProgress là tiến độ của MỘT nguồn hàng.
//
// Dữ liệu THUẦN chứ không phải *fulfillment.FulfillmentOrder: module order
// KHÔNG được phụ thuộc module fulfillment — đó là phụ thuộc ngược, và nó
// tạo vòng vì fulfillment đã trỏ tới order qua order_id.
//
// Module order LẮNG NGHE event từ fulfillment và tự tính, không hỏi ngược
// (ADR-0007). Tầng application dịch từ event sang cấu trúc này.
type FulfillmentProgress struct {
	// Cancelled và Delivered là hai trạng thái CUỐI có ý nghĩa với khách.
	Cancelled bool
	Delivered bool

	// Shipped đúng khi hàng đã rời kho — bao gồm cả trường hợp đã giao.
	Shipped bool
}

func (o *Order) RecalculateStatus(progress []FulfillmentProgress, now time.Time) bool {
	if len(progress) == 0 {
		return false
	}
	// Đơn đã chốt thì không tính lại: COMPLETED là trạng thái cuối.
	if o.status == StatusCompleted {
		return false
	}

	var total, cancelled, delivered, shipped int
	for _, p := range progress {
		total++
		switch {
		case p.Cancelled:
			cancelled++
		case p.Delivered:
			delivered++
			// Đã giao thì cũng đã xuất — tính vào cả hai để quy tắc
			// "tất cả đã xuất" không bị sai khi một phần đã giao xong.
			shipped++
		case p.Shipped:
			shipped++
		}
	}

	// THỨ TỰ XÉT QUAN TRỌNG: tiến độ xa hơn được ưu tiên.
	//
	// Đã giao thì cũng đã xuất, nên `shipped` bao gồm cả FO đã giao. Nếu
	// xét "tất cả đã xuất" TRƯỚC "một số đã giao", một đơn có 2 FO — một
	// đã giao, một mới xuất — sẽ báo SHIPPED thay vì PARTIALLY_DELIVERED,
	// tức là hiển thị lùi so với thực tế.
	next := o.status
	switch {
	case cancelled == total:
		next = StatusCancelled
	case delivered == total:
		next = StatusDelivered
	case delivered > 0:
		next = StatusPartiallyDelivered
	case shipped == total:
		next = StatusShipped
	case shipped > 0:
		next = StatusPartiallyShipped
	case cancelled > 0:
		next = StatusPartiallyCancelled
	}

	if next == o.status {
		return false
	}
	o.status = next
	o.touch(now)
	return true
}

func (o *Order) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	o.updatedAt = now
}
