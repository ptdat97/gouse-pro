// Package domain chứa mô hình nghiệp vụ của module checkout.
//
// CHECKOUT LÀ AGGREGATE RIÊNG, KHÔNG PHẢI MỘT TRẠNG THÁI CỦA GIỎ HÀNG
// (aggregates.md mục 3.5):
//
//	Cart                        | Checkout
//	----------------------------|---------------------------
//	Sống lâu (30 ngày)          | Sống ngắn (15 phút)
//	KHÔNG giữ tồn kho           | CÓ giữ tồn kho
//	Giá cập nhật động           | Giá ĐÓNG BĂNG
//	Thay đổi tự do              | Hạn chế thay đổi
//
// Gộp chung sẽ dẫn tới MỘT TRONG HAI hậu quả, không tránh được cả hai:
// hoặc giỏ hàng khóa tồn kho vô ích, hoặc giá đổi giữa chừng thanh toán.
//
// VÌ SAO ĐÓNG BĂNG GIÁ (mục 5 của đặc tả):
//
//	14:00 — Khách bắt đầu checkout, áo giá 299.000đ
//	14:05 — Seller đổi giá thành 350.000đ
//	14:10 — Khách hoàn tất thanh toán
//
//	Không đóng băng: khách thấy 299.000đ nhưng bị trừ 350.000đ
//	Đóng băng:       khách trả đúng 299.000đ như đã thấy
//
// Đây là module ĐIỀU PHỐI: nó gọi nhiều module nhất trong hệ thống và gần
// như không sở hữu luật nghiệp vụ nào của riêng mình. Luật ở đây chỉ có
// một loại: thứ tự và điều kiện của các bước.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrNotFound        = errors.New("checkout: không tìm thấy")
	ErrNoLines         = errors.New("checkout: phiên thanh toán phải có ít nhất một món")
	ErrNoCustomer      = errors.New("checkout: phải có khách đã đăng ký hoặc email khách vãng lai")
	ErrExpired         = errors.New("checkout: phiên thanh toán đã hết hạn")
	ErrInvalidStatus   = errors.New("checkout: chuyển trạng thái không hợp lệ")
	ErrNoAddress       = errors.New("checkout: phải có địa chỉ giao hàng")
	ErrTooManyExtends  = errors.New("checkout: đã hết số lần gia hạn")
	ErrMissingIdemKey  = errors.New("checkout: thiếu khóa idempotency")
	ErrAlreadyComplete = errors.New("checkout: phiên thanh toán đã hoàn tất")
)

// Status là trạng thái của phiên thanh toán.
type Status string

const (
	// StatusStarted: đã giữ hàng và đóng băng giá, đang chờ khách nhập
	// thông tin.
	StatusStarted Status = "STARTED"

	// StatusPendingPayment: đủ thông tin, đang chờ tiền.
	StatusPendingPayment Status = "PENDING_PAYMENT"

	StatusCompleted Status = "COMPLETED"
	StatusCancelled Status = "CANCELLED"
	StatusExpired   Status = "EXPIRED"
)

// IsFinal cho biết phiên đã kết thúc.
func (s Status) IsFinal() bool {
	return s == StatusCompleted || s == StatusCancelled || s == StatusExpired
}

// IsHoldingStock cho biết phiên còn đang giữ hàng không.
//
// Dùng để biết khi nào phải NHẢ hàng: hàng đang bị khóa mà phiên đã kết
// thúc là hàng không bán được cho ai.
func (s Status) IsHoldingStock() bool {
	return s == StatusStarted || s == StatusPendingPayment
}

// DefaultTTL là thời hạn mặc định của phiên thanh toán.
//
// 15 phút, khác hẳn 30 ngày của giỏ hàng. Lý do là điều duy nhất cần nhớ
// về hai con số này: phiên NÀY đang khóa hàng, giỏ thì không.
const DefaultTTL = 15 * time.Minute

// ExtendDuration là thời gian mỗi lần gia hạn.
const ExtendDuration = 10 * time.Minute

// MaxExtends là số lần gia hạn tối đa.
//
// Có giới hạn vì gia hạn vô hạn nghĩa là khóa hàng vô hạn — đúng thứ mà
// việc tách checkout khỏi giỏ sinh ra để tránh.
const MaxExtends = 2

// Address là địa chỉ giao hàng.
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
	return strings.TrimSpace(a.RecipientName) == "" ||
		strings.TrimSpace(a.StreetAddress) == ""
}

// Checkout là phiên thanh toán — aggregate root.
type Checkout struct {
	id ids.ID

	// cartID trỏ về giỏ đã sinh ra phiên này.
	//
	// Giữ lại để đánh dấu giỏ CONVERTED sau khi đặt hàng thành công, và để
	// truy vết được đơn hàng đến từ giỏ nào.
	cartID ids.ID

	customerID ids.ID
	guestEmail string
	guestPhone string

	currency money.Currency

	shippingAddress Address
	shippingMethod  string

	lines []*Line

	// Các khoản ở mức phiên. shippingFee tính sau khi có địa chỉ.
	shippingFee    money.Money
	discountAmount money.Money
	taxAmount      money.Money
	couponCode     string

	status Status

	// expiresAt là thời điểm hàng được nhả.
	//
	// Đây là trường quan trọng nhất về vận hành: mọi reservation của phiên
	// này sống tới đúng lúc đó, và tiến trình nền dựa vào nó để dọn.
	expiresAt  time.Time
	extendedAt int

	// orderID điền sau khi đặt hàng thành công.
	orderID ids.ID

	// idempotencyKey của lần hoàn tất, để gọi lại không tạo hai đơn.
	completionKey string

	createdAt time.Time
	updatedAt time.Time
}

type NewCheckoutParams struct {
	// ID cho phép bên gọi sinh mã TRƯỚC khi giữ hàng.
	//
	// Cần thiết vì reservation phải biết nó thuộc phiên nào, mà việc giữ
	// hàng diễn ra TRƯỚC khi phiên được tạo — không thể tạo phiên trước
	// rồi giữ hàng sau, vì phiên không giữ được hàng là phiên vô nghĩa.
	//
	// Để trống thì tự sinh.
	ID ids.ID

	CartID     ids.ID
	CustomerID ids.ID
	GuestEmail string
	GuestPhone string

	Currency money.Currency
	Lines    []*Line

	TTL time.Duration
	Now time.Time
}

// NewCheckout mở một phiên thanh toán.
//
// Các dòng hàng truyền vào ĐÃ ĐÓNG BĂNG GIÁ ở tầng application — hàm này
// không đi tra giá, vì tra giá ở đây nghĩa là giá có thể khác con số khách
// vừa nhìn thấy ở giỏ.
func NewCheckout(p NewCheckoutParams) (*Checkout, error) {
	if len(p.Lines) == 0 {
		return nil, ErrNoLines
	}
	// Quy tắc 6: khách vãng lai được checkout, nhưng phải liên hệ được.
	if p.CustomerID.IsZero() && strings.TrimSpace(p.GuestEmail) == "" {
		return nil, ErrNoCustomer
	}

	currency := p.Currency
	if currency == "" {
		currency = p.Lines[0].UnitPrice().Currency()
	}

	id := p.ID
	if id.IsZero() {
		newID, err := ids.New(ids.PrefixCheckout)
		if err != nil {
			return nil, err
		}
		id = newID
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ttl := p.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	c := &Checkout{
		id:             id,
		cartID:         p.CartID,
		customerID:     p.CustomerID,
		guestEmail:     strings.TrimSpace(p.GuestEmail),
		guestPhone:     strings.TrimSpace(p.GuestPhone),
		currency:       currency,
		lines:          append([]*Line(nil), p.Lines...),
		shippingFee:    money.Zero(currency),
		discountAmount: money.Zero(currency),
		taxAmount:      money.Zero(currency),
		status:         StatusStarted,
		expiresAt:      now.Add(ttl),
		createdAt:      now,
		updatedAt:      now,
	}

	for _, l := range c.lines {
		l.setCheckoutID(id)
	}
	return c, nil
}

// RestoreCheckoutParams dựng lại từ kho lưu trữ.
type RestoreCheckoutParams struct {
	ID              ids.ID
	CartID          ids.ID
	CustomerID      ids.ID
	GuestEmail      string
	GuestPhone      string
	Currency        money.Currency
	ShippingAddress Address
	ShippingMethod  string
	Lines           []*Line
	ShippingFee     money.Money
	DiscountAmount  money.Money
	TaxAmount       money.Money
	CouponCode      string
	Status          Status
	ExpiresAt       time.Time
	ExtendedTimes   int
	OrderID         ids.ID
	CompletionKey   string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RestoreCheckout dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreCheckout(p RestoreCheckoutParams) *Checkout {
	return &Checkout{
		id:              p.ID,
		cartID:          p.CartID,
		customerID:      p.CustomerID,
		guestEmail:      p.GuestEmail,
		guestPhone:      p.GuestPhone,
		currency:        p.Currency,
		shippingAddress: p.ShippingAddress,
		shippingMethod:  p.ShippingMethod,
		lines:           p.Lines,
		shippingFee:     p.ShippingFee,
		discountAmount:  p.DiscountAmount,
		taxAmount:       p.TaxAmount,
		couponCode:      p.CouponCode,
		status:          p.Status,
		expiresAt:       p.ExpiresAt,
		extendedAt:      p.ExtendedTimes,
		orderID:         p.OrderID,
		completionKey:   p.CompletionKey,
		createdAt:       p.CreatedAt,
		updatedAt:       p.UpdatedAt,
	}
}

func (c *Checkout) ID() ids.ID                  { return c.id }
func (c *Checkout) CartID() ids.ID              { return c.cartID }
func (c *Checkout) CustomerID() ids.ID          { return c.customerID }
func (c *Checkout) GuestEmail() string          { return c.guestEmail }
func (c *Checkout) GuestPhone() string          { return c.guestPhone }
func (c *Checkout) Currency() money.Currency    { return c.currency }
func (c *Checkout) ShippingAddress() Address    { return c.shippingAddress }
func (c *Checkout) ShippingMethod() string      { return c.shippingMethod }
func (c *Checkout) ShippingFee() money.Money    { return c.shippingFee }
func (c *Checkout) DiscountAmount() money.Money { return c.discountAmount }
func (c *Checkout) TaxAmount() money.Money      { return c.taxAmount }
func (c *Checkout) CouponCode() string          { return c.couponCode }
func (c *Checkout) Status() Status              { return c.status }
func (c *Checkout) ExpiresAt() time.Time        { return c.expiresAt }
func (c *Checkout) ExtendedTimes() int          { return c.extendedAt }
func (c *Checkout) OrderID() ids.ID             { return c.orderID }
func (c *Checkout) CompletionKey() string       { return c.completionKey }
func (c *Checkout) CreatedAt() time.Time        { return c.createdAt }
func (c *Checkout) UpdatedAt() time.Time        { return c.updatedAt }

// IsGuest cho biết đây có phải phiên của khách vãng lai không.
func (c *Checkout) IsGuest() bool { return c.customerID.IsZero() }

// Lines trả về bản sao lát cắt.
func (c *Checkout) Lines() []*Line {
	return append([]*Line(nil), c.lines...)
}

// IsExpired cho biết phiên đã quá hạn chưa.
//
// LƯU Ý: quá hạn theo ĐỒNG HỒ khác với trạng thái EXPIRED. Phiên có thể
// quá hạn vài giây trước khi tiến trình nền kịp đánh dấu, và trong khoảng
// đó nó vẫn còn giữ hàng. Mọi thao tác quan trọng phải hỏi hàm này chứ
// không chỉ nhìn trạng thái.
func (c *Checkout) IsExpired(now time.Time) bool {
	return !c.expiresAt.IsZero() && now.After(c.expiresAt)
}

// TimeLeft là thời gian còn lại, để client hiển thị đồng hồ đếm ngược.
func (c *Checkout) TimeLeft(now time.Time) time.Duration {
	if c.IsExpired(now) {
		return 0
	}
	return c.expiresAt.Sub(now)
}

// ReservationIDs trả mọi mã giữ hàng của phiên này.
//
// Dùng khi nhả hàng: phiên hủy hay hết hạn thì mọi reservation phải được
// nhả, không sót cái nào — sót một cái là khóa hàng vĩnh viễn cho tới khi
// có người phát hiện thủ công.
func (c *Checkout) ReservationIDs() []ids.ID {
	var out []ids.ID
	for _, l := range c.lines {
		if !l.ReservationID().IsZero() {
			out = append(out, l.ReservationID())
		}
	}
	return out
}

// Subtotal là tổng tiền hàng theo GIÁ ĐÃ ĐÓNG BĂNG.
func (c *Checkout) Subtotal() money.Money {
	sum := money.Zero(c.currency)
	for _, l := range c.lines {
		sum, _ = sum.Add(l.LineTotal())
	}
	return sum
}

// Total là tổng tiền khách phải trả.
//
//	subtotal + phí ship + thuế − giảm giá
//
// Đây là CON SỐ KHÁCH NHÌN THẤY, và nó phải bằng đúng con số vào đơn hàng.
func (c *Checkout) Total() money.Money {
	total := c.Subtotal()
	total, _ = total.Add(c.shippingFee)
	total, _ = total.Add(c.taxAmount)
	total, _ = total.Sub(c.discountAmount)
	return total
}

// SellerIDs trả danh sách nguồn hàng, không trùng lặp.
//
// Dùng để tính phí ship theo từng nguồn và hiển thị thời gian giao riêng
// cho từng nhóm (quy tắc 7): khách cần biết món nào đến trước.
func (c *Checkout) SellerIDs() []ids.ID {
	seen := map[ids.ID]bool{}
	var out []ids.ID
	for _, l := range c.lines {
		if l.SellerID().IsZero() || seen[l.SellerID()] {
			continue
		}
		seen[l.SellerID()] = true
		out = append(out, l.SellerID())
	}
	return out
}

// ---------------------------------------------------------------- Hành vi

// SetShippingAddress đặt địa chỉ giao hàng.
func (c *Checkout) SetShippingAddress(a Address, now time.Time) error {
	if err := c.mutable(now); err != nil {
		return err
	}
	if a.IsEmpty() {
		return ErrNoAddress
	}
	c.shippingAddress = a
	c.touch(now)
	return nil
}

// SetShipping đặt phương thức và phí vận chuyển.
//
// Phí do tầng application tính (gọi fulfillment), không phải domain: cách
// tính phụ thuộc hãng vận chuyển và khoảng cách, những thứ nằm ngoài
// module này.
func (c *Checkout) SetShipping(method string, fee money.Money, now time.Time) error {
	if err := c.mutable(now); err != nil {
		return err
	}
	if fee.Currency() != "" && fee.Currency() != c.currency {
		return errors.New("checkout: phí vận chuyển khác đơn vị tiền tệ của phiên")
	}
	c.shippingMethod = strings.TrimSpace(method)
	if fee.Currency() != "" {
		c.shippingFee = fee
	}
	c.touch(now)
	return nil
}

// ApplyDiscount áp một khoản giảm giá ở mức phiên.
func (c *Checkout) ApplyDiscount(code string, amount money.Money, now time.Time) error {
	if err := c.mutable(now); err != nil {
		return err
	}
	if amount.IsNegative() {
		return errors.New("checkout: khoản giảm giá phải là số dương")
	}
	c.couponCode = strings.TrimSpace(code)
	c.discountAmount = amount
	c.touch(now)
	return nil
}

// RemoveDiscount gỡ mã giảm giá.
func (c *Checkout) RemoveDiscount(now time.Time) error {
	if err := c.mutable(now); err != nil {
		return err
	}
	c.couponCode = ""
	c.discountAmount = money.Zero(c.currency)
	c.touch(now)
	return nil
}

// SetTax đặt tiền thuế.
func (c *Checkout) SetTax(amount money.Money, now time.Time) error {
	if err := c.mutable(now); err != nil {
		return err
	}
	c.taxAmount = amount
	c.touch(now)
	return nil
}

// MarkPendingPayment chuyển sang chờ thanh toán.
//
// Yêu cầu ĐÃ CÓ địa chỉ: không có địa chỉ thì không tính được phí ship, và
// đơn tạo ra sẽ không giao được.
func (c *Checkout) MarkPendingPayment(now time.Time) error {
	if err := c.mutable(now); err != nil {
		return err
	}
	if c.shippingAddress.IsEmpty() {
		return ErrNoAddress
	}
	c.status = StatusPendingPayment
	c.touch(now)
	return nil
}

// Extend gia hạn phiên thanh toán.
//
// Có thật vì lý do thật: khách đang chuyển khoản ngân hàng cần thêm thời
// gian, và bắt họ làm lại từ đầu là mất đơn hàng.
//
// Nhưng CÓ GIỚI HẠN (MaxExtends): gia hạn vô hạn nghĩa là khóa hàng vô
// hạn — đúng thứ mà việc tách checkout khỏi giỏ sinh ra để tránh.
//
// Bên gọi phải gia hạn CẢ reservation ở inventory: phiên sống lâu hơn
// reservation thì tới lúc đặt hàng mới phát hiện hàng đã bị nhả.
func (c *Checkout) Extend(d time.Duration, now time.Time) error {
	if c.status.IsFinal() {
		return ErrInvalidStatus
	}
	// Hết hạn rồi thì KHÔNG gia hạn được: hàng có thể đã bị nhả và bán cho
	// người khác. Khách phải bắt đầu phiên mới, nơi việc giữ hàng được
	// kiểm tra lại từ đầu.
	if c.IsExpired(now) {
		return ErrExpired
	}
	if c.extendedAt >= MaxExtends {
		return ErrTooManyExtends
	}
	if d <= 0 {
		d = ExtendDuration
	}

	c.expiresAt = c.expiresAt.Add(d)
	c.extendedAt++
	c.touch(now)
	return nil
}

// Complete đánh dấu phiên đã tạo đơn thành công.
//
// QUY TẮC 5: hoàn tất phải IDEMPOTENT. completionKey lưu lại để lần gọi
// thứ hai nhận ra và trả đơn cũ thay vì tạo đơn thứ hai.
func (c *Checkout) Complete(orderID ids.ID, key string, now time.Time) error {
	if c.status == StatusCompleted {
		return ErrAlreadyComplete
	}
	if c.status.IsFinal() {
		return ErrInvalidStatus
	}
	c.status = StatusCompleted
	c.orderID = orderID
	c.completionKey = key
	c.touch(now)
	return nil
}

// Cancel hủy phiên theo yêu cầu của khách.
//
// Bên gọi PHẢI nhả hàng sau đó — xem ReservationIDs.
func (c *Checkout) Cancel(now time.Time) error {
	if c.status.IsFinal() {
		return ErrInvalidStatus
	}
	c.status = StatusCancelled
	c.touch(now)
	return nil
}

// MarkExpired đánh dấu phiên đã hết hạn.
//
// Gọi bởi tiến trình nền. Bên gọi PHẢI nhả hàng sau đó.
func (c *Checkout) MarkExpired(now time.Time) error {
	if c.status.IsFinal() {
		return ErrInvalidStatus
	}
	c.status = StatusExpired
	c.touch(now)
	return nil
}

// mutable kiểm tra phiên còn sửa được không.
//
// Hai điều kiện, và điều kiện thứ hai là chỗ dễ quên: phiên chưa bị đánh
// dấu EXPIRED nhưng đã quá hạn theo đồng hồ thì vẫn KHÔNG được sửa. Tiến
// trình nền chạy theo chu kỳ, nên luôn có một khoảng trống giữa "hết hạn
// thật" và "được đánh dấu hết hạn".
func (c *Checkout) mutable(now time.Time) error {
	if c.status.IsFinal() {
		return ErrInvalidStatus
	}
	if c.IsExpired(now) {
		return ErrExpired
	}
	return nil
}

func (c *Checkout) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.updatedAt = now
}
