// Package domain chứa mô hình nghiệp vụ của module customer.
//
// # Ranh giới với identity
//
//	identity  "ai đang gọi, họ được phép làm gì"   → hạ tầng
//	customer  "người này là ai"                    → nghiệp vụ
//
// Cầu nối là user_id. Package này GIỮ user_id nhưng KHÔNG import identity —
// nó không cần biết mật khẩu hay phiên đăng nhập tồn tại.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrNotFound          = errors.New("customer: không tìm thấy khách hàng")
	ErrInvalidEmail      = errors.New("customer: email không hợp lệ")
	ErrDuplicateEmail    = errors.New("customer: email đã được dùng")
	ErrInvalidAddress    = errors.New("customer: địa chỉ không hợp lệ")
	ErrAddressNotFound   = errors.New("customer: không tìm thấy địa chỉ")
	ErrVersionConflict   = errors.New("customer: dữ liệu đã bị thay đổi bởi thao tác khác")
	ErrAnonymized        = errors.New("customer: hồ sơ đã ẩn danh, không sửa được")
	ErrMergeNotVerified  = errors.New("customer: gộp danh tính bắt buộc xác minh email")
	ErrMergeSameCustomer = errors.New("customer: không thể gộp hồ sơ vào chính nó")
	ErrInvalidConsent    = errors.New("customer: loại đồng ý không hợp lệ")
)

// Status là trạng thái khách hàng.
//
// Bốn TRẠNG THÁI của MỘT khái niệm, không phải bốn entity. Một người đi từ
// GUEST qua REGISTERED lên MEMBER mà vẫn giữ nguyên customer_id và toàn bộ
// lịch sử mua hàng.
type Status string

const (
	// StatusGuest là khách chưa đăng ký nhưng đã đặt hàng.
	StatusGuest Status = "GUEST"

	// StatusRegistered là khách đã có tài khoản đăng nhập.
	StatusRegistered Status = "REGISTERED"

	// StatusMember là khách đã mua ít nhất một đơn.
	StatusMember Status = "MEMBER"

	StatusVIP Status = "VIP"

	// StatusAnonymized là hồ sơ đã gỡ dữ liệu định danh theo yêu cầu xóa.
	//
	// KHÔNG xóa hàng: đơn hàng và bút toán tài chính vẫn trỏ tới
	// customer_id này, và nghĩa vụ lưu trữ chứng từ kế toán không cho phép
	// xóa chúng.
	StatusAnonymized Status = "ANONYMIZED"
)

// Customer là hồ sơ khách hàng.
type Customer struct {
	id ids.ID

	// userID RỖNG nghĩa là khách vãng lai — chưa có tài khoản đăng nhập.
	userID ids.ID

	email       string
	phone       string
	displayName string

	status Status

	orderCount int
	totalSpent money.Money

	version int

	createdAt time.Time
	updatedAt time.Time
}

// NewCustomerParams là dữ liệu tạo hồ sơ mới.
type NewCustomerParams struct {
	Email       string
	Phone       string
	DisplayName string

	// UserID để trống với khách vãng lai.
	UserID ids.ID

	Currency string
	Now      time.Time
}

// NewCustomer tạo hồ sơ khách hàng.
//
// Trạng thái ban đầu suy ra từ UserID: có tài khoản là REGISTERED, không
// có là GUEST. Bên gọi không tự đặt trạng thái — để không có chỗ nào tạo
// ra hồ sơ REGISTERED mà không có tài khoản nào phía sau.
func NewCustomer(p NewCustomerParams) (*Customer, error) {
	email := NormalizeEmail(p.Email)
	if !validEmail(email) {
		return nil, ErrInvalidEmail
	}

	currency := money.Currency(p.Currency)
	if currency == "" {
		currency = money.VND
	}

	status := StatusGuest
	if !p.UserID.IsZero() {
		status = StatusRegistered
	}

	return &Customer{
		id:          ids.MustNew(ids.PrefixCustomer),
		userID:      p.UserID,
		email:       email,
		phone:       strings.TrimSpace(p.Phone),
		displayName: strings.TrimSpace(p.DisplayName),
		status:      status,
		totalSpent:  money.Zero(currency),
		version:     1,
		createdAt:   p.Now,
		updatedAt:   p.Now,
	}, nil
}

// RestoreCustomerParams dựng lại hồ sơ từ kho lưu trữ.
type RestoreCustomerParams struct {
	ID          ids.ID
	UserID      ids.ID
	Email       string
	Phone       string
	DisplayName string
	Status      Status
	OrderCount  int
	TotalSpent  money.Money
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func RestoreCustomer(p RestoreCustomerParams) *Customer {
	return &Customer{
		id:          p.ID,
		userID:      p.UserID,
		email:       p.Email,
		phone:       p.Phone,
		displayName: p.DisplayName,
		status:      p.Status,
		orderCount:  p.OrderCount,
		totalSpent:  p.TotalSpent,
		version:     p.Version,
		createdAt:   p.CreatedAt,
		updatedAt:   p.UpdatedAt,
	}
}

func (c *Customer) ID() ids.ID              { return c.id }
func (c *Customer) UserID() ids.ID          { return c.userID }
func (c *Customer) Email() string           { return c.email }
func (c *Customer) Phone() string           { return c.phone }
func (c *Customer) DisplayName() string     { return c.displayName }
func (c *Customer) Status() Status          { return c.status }
func (c *Customer) OrderCount() int         { return c.orderCount }
func (c *Customer) TotalSpent() money.Money { return c.totalSpent }
func (c *Customer) Version() int            { return c.version }
func (c *Customer) CreatedAt() time.Time    { return c.createdAt }
func (c *Customer) UpdatedAt() time.Time    { return c.updatedAt }

// IsGuest cho biết đây có phải khách vãng lai không.
func (c *Customer) IsGuest() bool { return c.userID.IsZero() }

// LinkUser gắn hồ sơ với tài khoản đăng nhập.
//
// Đây là bước biến khách vãng lai thành khách đã đăng ký, GIỮ NGUYÊN
// customer_id nên toàn bộ đơn hàng cũ vẫn thuộc về họ.
func (c *Customer) LinkUser(userID ids.ID, now time.Time) error {
	if c.status == StatusAnonymized {
		return ErrAnonymized
	}
	c.userID = userID
	if c.status == StatusGuest {
		c.status = StatusRegistered
	}
	c.touch(now)
	return nil
}

// UpdateProfile sửa thông tin liên hệ.
func (c *Customer) UpdateProfile(displayName, phone string, now time.Time) error {
	if c.status == StatusAnonymized {
		return ErrAnonymized
	}
	c.displayName = strings.TrimSpace(displayName)
	c.phone = strings.TrimSpace(phone)
	c.touch(now)
	return nil
}

// RecordOrder cập nhật thống kê sau khi một đơn hoàn tất.
//
// IDEMPOTENCY KHÔNG NẰM Ở ĐÂY: hàm này luôn cộng dồn. Việc chặn xử lý
// một event hai lần là trách nhiệm của bảng event_processed — nếu để ở
// đây, mỗi bên gọi lại phải tự nhớ id đơn nào đã tính.
func (c *Customer) RecordOrder(amount money.Money, now time.Time) error {
	if c.status == StatusAnonymized {
		return ErrAnonymized
	}

	total, err := c.totalSpent.Add(amount)
	if err != nil {
		return err
	}
	c.totalSpent = total
	c.orderCount++

	// Mua lần đầu là lúc khách vãng lai thành MEMBER.
	//
	// KHÔNG hạ cấp VIP: hạng đã lên không tự mất đi vì một phép so sánh
	// trong hàm này.
	if c.status == StatusGuest || c.status == StatusRegistered {
		c.status = StatusMember
	}

	c.touch(now)
	return nil
}

// Anonymize gỡ dữ liệu định danh theo yêu cầu xóa tài khoản.
//
// # Vì sao KHÔNG xóa hàng
//
//	XÓA:   tên, email, số điện thoại — dữ liệu định danh
//	GIỮ:   customer_id, order_count, total_spent — dữ liệu giao dịch
//
// Đơn hàng đã dùng để tính hoa hồng trả cho seller. Xóa nó đi thì đối
// soát tài chính không còn khớp, và nghĩa vụ lưu trữ chứng từ kế toán bị
// vi phạm. Thay dữ liệu định danh bằng giá trị giả là cách duy nhất thỏa
// mãn cả hai yêu cầu.
//
// # Email giả phải viết CHỮ THƯỜNG
//
// Bảng có ràng buộc CHECK (email = lower(email)), còn ID là ULID viết
// HOA. Bỏ bước hạ chữ thường thì Anonymize thất bại với lỗi CHECK — và
// lỗi đó chỉ lộ ra khi có người thật yêu cầu xóa tài khoản.
//
// # Vì sao vẫn nhét ID vào email giả
//
// KHÔNG phải vì ràng buộc UNIQUE: chỉ mục customer_email_key có điều kiện
// `WHERE status <> 'ANONYMIZED'`, nên nó KHÔNG xét các hàng đã ẩn danh.
// Nhiều hồ sơ dùng chung một email giả sẽ không đụng nhau.
//
// Lý do thật là ĐIỀU TRA SỰ CỐ: khi một đơn hàng cũ trỏ tới hồ sơ đã ẩn
// danh, dòng email là chỗ duy nhất còn nói được đây là hồ sơ nào mà không
// phải mở lại bảng khác. Dùng chung một chuỗi cho mọi hồ sơ thì mọi bản
// ghi trông giống hệt nhau.
//
// Miền `.invalid` được RFC 2606 dành riêng cho mục đích này: nó bảo đảm
// KHÔNG BAO GIỜ tồn tại thật, nên không có nguy cơ thư đi tới hộp thư của
// người khác.
func (c *Customer) Anonymize(now time.Time) {
	if c.status == StatusAnonymized {
		return
	}

	c.email = "anonymized+" + strings.ToLower(c.id.String()) + "@anonymized.invalid"
	c.phone = ""
	c.displayName = ""
	c.userID = ""
	c.status = StatusAnonymized
	c.touch(now)
}

func (c *Customer) touch(now time.Time) {
	c.updatedAt = now
	c.version++
}

// ---------------------------------------------------------------- Email

// NormalizeEmail chuẩn hóa email về dạng so sánh được.
//
// PHẢI KHỚP với identity.NormalizeEmail: hai module lưu email khác định
// dạng thì không bao giờ gộp được danh tính khách vãng lai với tài khoản.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// validEmail kiểm tra định dạng ở mức TỐI THIỂU.
//
// KHÔNG dùng biểu thức chính quy đầy đủ theo RFC 5322: nó dài hàng trăm
// ký tự, chậm, và vẫn chấp nhận những địa chỉ không tồn tại. Cách duy
// nhất biết email có thật là GỬI THƯ TỚI ĐÓ.
func validEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	if strings.ContainsAny(s, " \t\n") {
		return false
	}
	return strings.Contains(s[at+1:], ".")
}
