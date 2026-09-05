// Package domain chứa mô hình nghiệp vụ của module marketplace.
//
// Offer là ĐƠN VỊ KHÁCH THỰC SỰ MUA — không phải Product, không phải SKU.
// Khách chọn mua từ một nhà bán cụ thể với giá cụ thể.
//
// Xem docs/adr/0007-marketplace-order-model.md.
package domain

import (
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrNonPositivePrice = errors.New("marketplace: giá phải lớn hơn 0")
	ErrMissingSKU       = errors.New("marketplace: thiếu định danh SKU")
	ErrMissingSeller    = errors.New("marketplace: thiếu định danh nhà bán")
	ErrInvalidStatus    = errors.New("marketplace: chuyển trạng thái không hợp lệ")
	ErrCompareAtTooLow  = errors.New("marketplace: giá gạch ngang phải cao hơn giá bán")
	ErrInvalidQuantity  = errors.New("marketplace: số lượng đặt hàng không hợp lệ")
	ErrNotFound         = errors.New("marketplace: không tìm thấy")

	// ErrDuplicateActiveOffer khi seller đã có offer ACTIVE cho SKU này.
	//
	// Quy tắc 1 (mục 11): một seller chỉ có MỘT offer ACTIVE cho một SKU.
	// Hai offer cùng lúc thì không biết giá nào là giá thật, và buy box
	// chọn nhầm sẽ bán với giá seller không định.
	ErrDuplicateActiveOffer = errors.New("marketplace: nhà bán đã có offer đang bán cho SKU này")
)

// Condition là tình trạng hàng.
type Condition string

const (
	ConditionNew         Condition = "NEW"
	ConditionUsedLikeNew Condition = "USED_LIKE_NEW"
	ConditionUsedGood    Condition = "USED_GOOD"
)

func (c Condition) valid() bool {
	switch c {
	case ConditionNew, ConditionUsedLikeNew, ConditionUsedGood:
		return true
	}
	return false
}

// Status là trạng thái của offer.
type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusActive    Status = "ACTIVE"
	StatusSuspended Status = "SUSPENDED"
	StatusArchived  Status = "ARCHIVED"
)

// KHÔNG có OUT_OF_STOCK ở đây, có chủ ý (P3-23).
//
// Hết hàng là sự thật của INVENTORY, không phải của offer. Chép nó vào
// `Offer.status` là lưu một sự thật ở hai nơi, và hai nơi thì sớm muộn
// lệch nhau — chính chú thích của struct Offer bên dưới đã viết ra điều
// đó, rồi enum này lại đi ngược.
//
// Câu "khách bấm mua được không" đã có câu trả lời ĐẦY ĐỦ và tính lúc
// đọc: `ProductOffer.IsSellable = offer ACTIVE && còn hàng`. Một nguồn,
// không có gì để lệch.
//
// Giá trị này chưa bao giờ tới được: không ai gọi `MarkOutOfStock`, và
// event `inventory.depleted` chỉ tồn tại trong chú thích. Cài nốt nó sẽ
// THÊM một kiểu hỏng chưa từng có: event `replenished` chết sau 5 lần thử
// là bị dead-letter vĩnh viễn, offer kẹt OUT_OF_STOCK, và vì
// `IsSellable = o.IsSellable() && coHang` nên offer thành KHÔNG BÁN ĐƯỢC
// dù kho đầy hàng — nhà bán mất doanh thu mà không có lỗi nào báo.

// canTransitionTo mã hóa vòng đời offer.
func (s Status) canTransitionTo(next Status) bool {
	switch s {
	case StatusDraft:
		return next == StatusActive || next == StatusArchived
	case StatusActive:
		return next == StatusSuspended || next == StatusArchived
	case StatusSuspended:
		return next == StatusActive || next == StatusArchived
	case StatusArchived:
		// Ngừng vĩnh viễn. Đơn hàng cũ vẫn trỏ tới offer này nên KHÔNG xóa,
		// nhưng cũng không mở lại được.
		return false
	}
	return false
}

// IsSellable cho biết TRẠNG THÁI offer có cho bán không.
//
// KHÔNG phải câu trả lời cuối cùng cho "khách bấm mua được không": nó
// không biết gì về tồn kho. Câu đầy đủ là `ProductOffer.IsSellable`, bằng
// hàm này VÀ còn hàng.
func (s Status) IsSellable() bool { return s == StatusActive }

// IsVisibleToCustomer cho biết offer có hiện trên trang sản phẩm không.
//
// Hết hàng vẫn HIỂN THỊ, chỉ không đặt được — yêu cầu này KHÔNG mất đi khi
// bỏ OUT_OF_STOCK khỏi enum: offer hết hàng vẫn ở trạng thái ACTIVE, nên
// vẫn hiện, và `ProductOffer.IsSellable` mới là thứ tắt nút mua.
//
// Lý do của yêu cầu (design-system.md mục 4.1): khách cần biết sản phẩm CÓ
// tổ hợp màu/size đó để đăng ký nhận thông báo. Ẩn đi thì họ tưởng nền
// tảng không bán, và nhu cầu đó không bao giờ được ghi nhận.
//
// DRAFT, SUSPENDED, ARCHIVED thì ẩn: hiện một mức giá không đặt được là
// trải nghiệm tệ hơn không hiện gì.
func (s Status) IsVisibleToCustomer() bool { return s == StatusActive }

// Offer là lời chào bán của MỘT seller cho MỘT SKU.
//
// VÌ SAO OFFER KHÔNG CHỨA SỐ LƯỢNG TỒN KHO (mục 3 của đặc tả):
//
//	Nguồn sự thật về số lượng là InventoryItem, và offer KHÔNG chép lại —
//	kể cả dưới dạng một trạng thái dẫn xuất (xem ghi chú ở enum Status).
//
//	Lý do: một offer có thể được phục vụ từ nhiều kho; tồn kho thay đổi tần
//	suất rất cao và sẽ làm bẩn aggregate Offer; và hai nơi cùng lưu một sự
//	thật thì sớm muộn chúng lệch nhau.
type Offer struct {
	id       ids.ID
	skuID    ids.ID
	sellerID ids.ID

	price     money.Money
	compareAt money.Money

	condition         Condition
	handlingTimeHours int

	minOrderQuantity int
	maxOrderQuantity int

	status Status

	// version cho khóa lạc quan: seller sửa offer rất thường xuyên, và
	// hai lần sửa đồng thời không được ghi đè nhau âm thầm.
	version int64

	createdAt time.Time
	updatedAt time.Time
}

type NewOfferParams struct {
	SKUID             ids.ID
	SellerID          ids.ID
	Price             money.Money
	CompareAt         money.Money
	Condition         Condition
	HandlingTimeHours int
	MinOrderQuantity  int
	MaxOrderQuantity  int
	Now               time.Time
}

// mặc định: seller giao hàng trong 24 giờ.
const defaultHandlingTimeHours = 24

// NewOffer tạo offer mới ở trạng thái DRAFT.
func NewOffer(p NewOfferParams) (*Offer, error) {
	if p.SKUID.IsZero() {
		return nil, ErrMissingSKU
	}
	if p.SellerID.IsZero() {
		return nil, ErrMissingSeller
	}

	// Quy tắc 2: giá phải > 0. Giá 0 gần như luôn là lỗi nhập liệu.
	if !p.Price.IsPositive() {
		return nil, ErrNonPositivePrice
	}

	if !p.CompareAt.IsZero() {
		if p.CompareAt.Currency() != p.Price.Currency() {
			return nil, errors.New("marketplace: giá gạch ngang phải cùng đơn vị tiền tệ")
		}
		// Giá gạch ngang thấp hơn giá bán sẽ hiển thị "giảm -20%".
		if !p.Price.LessThan(p.CompareAt) {
			return nil, ErrCompareAtTooLow
		}
	}

	condition := p.Condition
	if condition == "" {
		condition = ConditionNew
	}
	if !condition.valid() {
		return nil, errors.New("marketplace: tình trạng hàng không hợp lệ: " + string(condition))
	}

	handling := p.HandlingTimeHours
	if handling <= 0 {
		handling = defaultHandlingTimeHours
	}

	minQty, maxQty := p.MinOrderQuantity, p.MaxOrderQuantity
	if minQty <= 0 {
		minQty = 1
	}
	// max = 0 nghĩa là không giới hạn.
	if maxQty > 0 && maxQty < minQty {
		return nil, ErrInvalidQuantity
	}

	id, err := ids.New(ids.PrefixOffer)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Offer{
		id:                id,
		skuID:             p.SKUID,
		sellerID:          p.SellerID,
		price:             p.Price,
		compareAt:         p.CompareAt,
		condition:         condition,
		handlingTimeHours: handling,
		minOrderQuantity:  minQty,
		maxOrderQuantity:  maxQty,
		status:            StatusDraft,
		createdAt:         now,
		updatedAt:         now,
	}, nil
}

// RestoreOfferParams dựng lại từ kho lưu trữ.
type RestoreOfferParams struct {
	ID                ids.ID
	SKUID             ids.ID
	SellerID          ids.ID
	Price             money.Money
	CompareAt         money.Money
	Condition         Condition
	HandlingTimeHours int
	MinOrderQuantity  int
	MaxOrderQuantity  int
	Status            Status
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// RestoreOffer dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreOffer(p RestoreOfferParams) *Offer {
	return &Offer{
		id:                p.ID,
		skuID:             p.SKUID,
		sellerID:          p.SellerID,
		price:             p.Price,
		compareAt:         p.CompareAt,
		condition:         p.Condition,
		handlingTimeHours: p.HandlingTimeHours,
		minOrderQuantity:  p.MinOrderQuantity,
		maxOrderQuantity:  p.MaxOrderQuantity,
		status:            p.Status,
		version:           p.Version,
		createdAt:         p.CreatedAt,
		updatedAt:         p.UpdatedAt,
	}
}

func (o *Offer) ID() ids.ID             { return o.id }
func (o *Offer) SKUID() ids.ID          { return o.skuID }
func (o *Offer) SellerID() ids.ID       { return o.sellerID }
func (o *Offer) Price() money.Money     { return o.price }
func (o *Offer) CompareAt() money.Money { return o.compareAt }
func (o *Offer) Condition() Condition   { return o.condition }
func (o *Offer) HandlingTimeHours() int { return o.handlingTimeHours }
func (o *Offer) MinOrderQuantity() int  { return o.minOrderQuantity }
func (o *Offer) MaxOrderQuantity() int  { return o.maxOrderQuantity }
func (o *Offer) Status() Status         { return o.status }
func (o *Offer) Version() int64         { return o.version }
func (o *Offer) CreatedAt() time.Time   { return o.createdAt }
func (o *Offer) UpdatedAt() time.Time   { return o.updatedAt }

// IsSellable cho biết khách đặt hàng được không.
func (o *Offer) IsSellable() bool { return o.status.IsSellable() }

// IsVisibleToCustomer cho biết offer có hiện trên trang sản phẩm không.
func (o *Offer) IsVisibleToCustomer() bool { return o.status.IsVisibleToCustomer() }

// HasDiscount cho biết có hiển thị mức giảm không.
func (o *Offer) HasDiscount() bool {
	return !o.compareAt.IsZero() && o.price.LessThan(o.compareAt)
}

// AllowsQuantity kiểm tra số lượng đặt có nằm trong giới hạn không.
func (o *Offer) AllowsQuantity(qty int) bool {
	if qty < o.minOrderQuantity {
		return false
	}
	if o.maxOrderQuantity > 0 && qty > o.maxOrderQuantity {
		return false
	}
	return true
}

// ---------------------------------------------------------------- Hành vi

// Activate đưa offer lên bán.
func (o *Offer) Activate(now time.Time) error {
	return o.transition(StatusActive, now)
}

// Suspend đình chỉ offer.
//
// Dùng khi admin can thiệp hoặc khi seller bị đình chỉ (quy tắc 4).
func (o *Offer) Suspend(now time.Time) error {
	return o.transition(StatusSuspended, now)
}

// Archive ngừng bán vĩnh viễn.
//
// KHÔNG xóa: đơn hàng cũ vẫn trỏ tới offer này và lịch sử mua hàng phải
// hiển thị được.
func (o *Offer) Archive(now time.Time) error {
	return o.transition(StatusArchived, now)
}

// ChangePrice đổi giá bán.
//
// Quy tắc 5: mọi lần đổi giá phải ghi lịch sử. Việc ghi lịch sử do tầng
// application đảm nhiệm — nếu để bên gọi tự nhớ, sẽ có chỗ quên.
func (o *Offer) ChangePrice(price, compareAt money.Money, now time.Time) error {
	if !price.IsPositive() {
		return ErrNonPositivePrice
	}
	if !compareAt.IsZero() {
		if compareAt.Currency() != price.Currency() {
			return errors.New("marketplace: giá gạch ngang phải cùng đơn vị tiền tệ")
		}
		if !price.LessThan(compareAt) {
			return ErrCompareAtTooLow
		}
	}
	o.price = price
	o.compareAt = compareAt
	o.touch(now)
	return nil
}

func (o *Offer) transition(next Status, now time.Time) error {
	if !o.status.canTransitionTo(next) {
		return ErrInvalidStatus
	}
	o.status = next
	o.touch(now)
	return nil
}

func (o *Offer) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	o.updatedAt = now
}
