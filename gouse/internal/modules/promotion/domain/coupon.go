package domain

import (
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// Coupon là một mã giảm giá cụ thể.
//
// # Vì sao tách khỏi Promotion
//
// Một chương trình khuyến mãi có thể phát hành NHIỀU mã: mã chung
// "SALE10" cho tất cả, và mã riêng cho từng khách (xin lỗi sau sự cố,
// tặng khách VIP). Gộp vào một bảng thì mỗi mã riêng là một bản sao đầy
// đủ của toàn bộ cấu hình khuyến mãi — và sửa điều kiện chiến dịch nghĩa
// là sửa hàng nghìn hàng.
type Coupon struct {
	id          ids.ID
	promotionID ids.ID

	// code là thứ khách GÕ VÀO, luôn CHỮ HOA.
	code string

	// customerID khác rỗng nghĩa là mã RIÊNG cho một khách.
	//
	// Người khác biết mã cũng không dùng được.
	customerID ids.ID

	usedCount int
	active    bool

	createdAt time.Time
}

// NewCouponParams là dữ liệu tạo mã.
type NewCouponParams struct {
	PromotionID ids.ID
	Code        string

	// CustomerID để trống với mã dùng chung.
	CustomerID ids.ID

	Now time.Time
}

// NewCoupon tạo mã giảm giá.
func NewCoupon(p NewCouponParams) (*Coupon, error) {
	code := NormalizeCode(p.Code)
	if code == "" {
		return nil, ErrInvalidInput
	}
	if p.PromotionID.IsZero() {
		return nil, ErrInvalidInput
	}

	return &Coupon{
		id:          ids.MustNew(ids.PrefixCoupon),
		promotionID: p.PromotionID,
		code:        code,
		customerID:  p.CustomerID,
		active:      true,
		createdAt:   p.Now,
	}, nil
}

// RestoreCouponParams dựng lại mã từ kho lưu trữ.
type RestoreCouponParams struct {
	ID          ids.ID
	PromotionID ids.ID
	Code        string
	CustomerID  ids.ID
	UsedCount   int
	Active      bool
	CreatedAt   time.Time
}

func RestoreCoupon(p RestoreCouponParams) *Coupon {
	return &Coupon{
		id:          p.ID,
		promotionID: p.PromotionID,
		code:        p.Code,
		customerID:  p.CustomerID,
		usedCount:   p.UsedCount,
		active:      p.Active,
		createdAt:   p.CreatedAt,
	}
}

func (c *Coupon) ID() ids.ID           { return c.id }
func (c *Coupon) PromotionID() ids.ID  { return c.promotionID }
func (c *Coupon) Code() string         { return c.code }
func (c *Coupon) CustomerID() ids.ID   { return c.customerID }
func (c *Coupon) UsedCount() int       { return c.usedCount }
func (c *Coupon) Active() bool         { return c.active }
func (c *Coupon) CreatedAt() time.Time { return c.createdAt }

// IsPersonal cho biết đây có phải mã riêng của một khách không.
func (c *Coupon) IsPersonal() bool { return !c.customerID.IsZero() }

// CheckOwner kiểm tra khách này có được dùng mã không.
//
// # Mã riêng phải trả lỗi RÕ RÀNG, không giả vờ không tồn tại
//
// Ngược với nguyên tắc ở identity (giấu sự tồn tại của tài khoản), ở đây
// người dùng thật cần biết vì sao mã không áp được. Mã giảm giá không phải
// bí mật cần bảo vệ — biết mã của người khác không cho ta bất kỳ quyền gì.
func (c *Coupon) CheckOwner(customerID ids.ID) error {
	if !c.IsPersonal() {
		return nil
	}
	if c.customerID != customerID {
		return ErrWrongCustomer
	}
	return nil
}

// Deactivate tắt mã.
func (c *Coupon) Deactivate() { c.active = false }

// IncrementUse tăng bộ đếm lượt dùng.
//
// Bộ đếm này chỉ để HIỂN THỊ ("mã đã dùng 47 lần"). Giới hạn thật nằm ở
// Promotion; đếm ở đây cũng không thay thế được bảng lượt sử dụng, thứ là
// nguồn sự thật.
func (c *Coupon) IncrementUse() { c.usedCount++ }

// DecrementUse giảm bộ đếm khi đơn bị hủy.
//
// Chặn ở 0: bộ đếm âm là dấu hiệu giải phóng nhiều hơn đã dùng, và nó chỉ
// làm con số hiển thị trở nên vô nghĩa thay vì báo lỗi ở đâu đó.
func (c *Coupon) DecrementUse() {
	if c.usedCount > 0 {
		c.usedCount--
	}
}

// NormalizeCode chuẩn hóa mã về dạng so sánh được.
//
// Khách gõ "sale10" và " SALE10 " phải ra CÙNG một mã — không thì họ nghĩ
// mã hỏng và gọi tổng đài.
func NormalizeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// Usage là một lượt sử dụng mã.
//
// # Đây là NGUỒN SỰ THẬT về việc mã đã dùng chưa
//
// Bộ đếm usedCount trên Promotion và Coupon chỉ là bản tóm tắt để đọc
// nhanh. Khi hai con số lệch nhau, bảng lượt sử dụng đúng.
type Usage struct {
	CouponID    ids.ID
	PromotionID ids.ID

	// CustomerID có thể RỖNG: khách vãng lai cũng dùng mã được.
	CustomerID ids.ID

	OrderID ids.ID

	Discount money.Money

	UsedAt time.Time

	// ReleasedAt khác zero nghĩa là đã giải phóng (đơn bị hủy).
	//
	// KHÔNG xóa hàng: cần biết mã từng được dùng cho đơn nào và vì sao
	// được trả lại — nếu không, tranh chấp "tôi đã dùng mã rồi" không có
	// gì để tra.
	ReleasedAt time.Time
}

// IsReleased cho biết lượt này đã được giải phóng chưa.
func (u Usage) IsReleased() bool { return !u.ReleasedAt.IsZero() }
