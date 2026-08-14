package domain

import (
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Address là một mục trong sổ địa chỉ.
//
// # KHÁC với địa chỉ trong đơn hàng
//
//	Address (ở đây)   địa chỉ HIỆN TẠI, sửa được, xóa được
//	order.shipping_*  bản SAO ĐÓNG BĂNG lúc đặt hàng (nguyên tắc P9)
//
// Khách chuyển nhà rồi sửa sổ địa chỉ thì đơn cũ vẫn phải hiện địa chỉ đã
// giao tới. Nếu order tham chiếu tới entity này, mọi đơn cũ sẽ đổi địa chỉ
// theo — và đối soát với đơn vị vận chuyển sẽ không khớp.
type Address struct {
	id         ids.ID
	customerID ids.ID

	// Người nhận có thể KHÁC chủ tài khoản: mua tặng, giao tới văn phòng.
	recipientName  string
	recipientPhone string

	line1    string
	line2    string
	ward     string
	district string
	province string
	postcode string
	country  string

	note string

	isDefault bool
	deletedAt time.Time

	createdAt time.Time
	updatedAt time.Time
}

// NewAddressParams là dữ liệu thêm địa chỉ.
type NewAddressParams struct {
	CustomerID ids.ID

	RecipientName  string
	RecipientPhone string

	Line1    string
	Line2    string
	Ward     string
	District string
	Province string
	Postcode string
	Country  string

	Note      string
	IsDefault bool

	Now time.Time
}

// NewAddress tạo địa chỉ mới.
//
// Ba trường bắt buộc: tên người nhận, số điện thoại, dòng địa chỉ đầu.
// Thiếu bất kỳ thứ nào thì đơn vị vận chuyển không giao được — và lỗi đó
// chỉ lộ ra khi hàng đã đóng gói xong.
func NewAddress(p NewAddressParams) (*Address, error) {
	name := strings.TrimSpace(p.RecipientName)
	phone := strings.TrimSpace(p.RecipientPhone)
	line1 := strings.TrimSpace(p.Line1)

	if name == "" || phone == "" || line1 == "" {
		return nil, ErrInvalidAddress
	}

	country := strings.ToUpper(strings.TrimSpace(p.Country))
	if country == "" {
		country = "VN"
	}
	if len(country) != 2 {
		return nil, ErrInvalidAddress
	}

	return &Address{
		id:             ids.MustNew(ids.PrefixAddress),
		customerID:     p.CustomerID,
		recipientName:  name,
		recipientPhone: phone,
		line1:          line1,
		line2:          strings.TrimSpace(p.Line2),
		ward:           strings.TrimSpace(p.Ward),
		district:       strings.TrimSpace(p.District),
		province:       strings.TrimSpace(p.Province),
		postcode:       strings.TrimSpace(p.Postcode),
		country:        country,
		note:           strings.TrimSpace(p.Note),
		isDefault:      p.IsDefault,
		createdAt:      p.Now,
		updatedAt:      p.Now,
	}, nil
}

// RestoreAddressParams dựng lại địa chỉ từ kho lưu trữ.
type RestoreAddressParams struct {
	ID             ids.ID
	CustomerID     ids.ID
	RecipientName  string
	RecipientPhone string
	Line1          string
	Line2          string
	Ward           string
	District       string
	Province       string
	Postcode       string
	Country        string
	Note           string
	IsDefault      bool
	DeletedAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func RestoreAddress(p RestoreAddressParams) *Address {
	return &Address{
		id:             p.ID,
		customerID:     p.CustomerID,
		recipientName:  p.RecipientName,
		recipientPhone: p.RecipientPhone,
		line1:          p.Line1,
		line2:          p.Line2,
		ward:           p.Ward,
		district:       p.District,
		province:       p.Province,
		postcode:       p.Postcode,
		country:        p.Country,
		note:           p.Note,
		isDefault:      p.IsDefault,
		deletedAt:      p.DeletedAt,
		createdAt:      p.CreatedAt,
		updatedAt:      p.UpdatedAt,
	}
}

func (a *Address) ID() ids.ID             { return a.id }
func (a *Address) CustomerID() ids.ID     { return a.customerID }
func (a *Address) RecipientName() string  { return a.recipientName }
func (a *Address) RecipientPhone() string { return a.recipientPhone }
func (a *Address) Line1() string          { return a.line1 }
func (a *Address) Line2() string          { return a.line2 }
func (a *Address) Ward() string           { return a.ward }
func (a *Address) District() string       { return a.district }
func (a *Address) Province() string       { return a.province }
func (a *Address) Postcode() string       { return a.postcode }
func (a *Address) Country() string        { return a.country }
func (a *Address) Note() string           { return a.note }
func (a *Address) IsDefault() bool        { return a.isDefault }
func (a *Address) DeletedAt() time.Time   { return a.deletedAt }
func (a *Address) CreatedAt() time.Time   { return a.createdAt }
func (a *Address) UpdatedAt() time.Time   { return a.updatedAt }

// IsDeleted cho biết địa chỉ đã xóa mềm chưa.
func (a *Address) IsDeleted() bool { return !a.deletedAt.IsZero() }

// BelongsTo kiểm tra địa chỉ có thuộc về khách này không.
//
// LỚP BẢO VỆ ĐỘC LẬP với điều kiện WHERE trong SQL. Hai lớp vì một lớp có
// thể bị sửa hỏng mà không ai nhận ra: biết id địa chỉ của người khác là
// đọc được tên, số điện thoại và địa chỉ nhà của họ.
func (a *Address) BelongsTo(customerID ids.ID) bool {
	return a.customerID == customerID
}

// SetDefault đặt hoặc bỏ cờ mặc định.
//
// Việc bảo đảm CHỈ MỘT địa chỉ mặc định KHÔNG nằm ở đây — entity này chỉ
// thấy chính nó. Ràng buộc thật là chỉ mục UNIQUE một phần ở database.
func (a *Address) SetDefault(v bool, now time.Time) {
	a.isDefault = v
	a.updatedAt = now
}

// Update sửa nội dung địa chỉ.
func (a *Address) Update(p NewAddressParams) error {
	name := strings.TrimSpace(p.RecipientName)
	phone := strings.TrimSpace(p.RecipientPhone)
	line1 := strings.TrimSpace(p.Line1)

	if name == "" || phone == "" || line1 == "" {
		return ErrInvalidAddress
	}

	a.recipientName = name
	a.recipientPhone = phone
	a.line1 = line1
	a.line2 = strings.TrimSpace(p.Line2)
	a.ward = strings.TrimSpace(p.Ward)
	a.district = strings.TrimSpace(p.District)
	a.province = strings.TrimSpace(p.Province)
	a.postcode = strings.TrimSpace(p.Postcode)
	a.note = strings.TrimSpace(p.Note)
	a.updatedAt = p.Now
	return nil
}

// Delete xóa MỀM địa chỉ.
//
// Không xóa cứng: khách vẫn muốn thấy lại địa chỉ đã dùng khi đặt lại đơn
// cũ.
//
// Gỡ luôn cờ mặc định. Chỉ mục UNIQUE ở database đã loại địa chỉ đã xóa
// (`WHERE is_default AND deleted_at IS NULL`) nên nó KHÔNG chặn địa chỉ
// mới. Gỡ cờ ở đây là để dữ liệu tự nói đúng: một địa chỉ vừa "đã xóa"
// vừa "đang là mặc định" buộc mọi chỗ đọc phải nhớ kiểm tra cả hai cột,
// và chỗ nào quên sẽ hiện địa chỉ đã xóa lên trang thanh toán.
func (a *Address) Delete(now time.Time) {
	if a.IsDeleted() {
		return
	}
	a.deletedAt = now
	a.isDefault = false
	a.updatedAt = now
}
