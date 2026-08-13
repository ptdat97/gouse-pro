// Package domain chứa mô hình nghiệp vụ của module cart.
//
// RANH GIỚI QUAN TRỌNG NHẤT CỦA MODULE NÀY (cart.md mục 3):
//
//	                 | Cart                  | Checkout
//	-----------------|-----------------------|------------------
//	Thời gian sống   | Nhiều ngày, nhiều phiên| Vài phút
//	GIỮ TỒN KHO      | KHÔNG                 | CÓ
//	Giá              | Động, theo giá hiện tại| ĐÓNG BĂNG
//	Hết hạn          | Dài (30 ngày)         | Ngắn (15 phút)
//
// Hai dòng giữa là toàn bộ lý do module này tồn tại tách khỏi checkout.
//
// VÌ SAO GIỎ HÀNG KHÔNG GIỮ TỒN KHO:
//
//	Nếu giỏ giữ:  khách thêm rồi bỏ quên 2 tuần → hàng bị khóa 2 tuần.
//	              Với hàng khan hiếm, vài trăm giỏ bỏ quên = HẾT HÀNG ẢO,
//	              không bán được cho khách thật sự muốn mua.
//
//	Chỉ checkout: chỉ khách đang thực sự thanh toán mới khóa hàng, thời
//	              gian ngắn, có hết hạn tự động.
//
// HỆ QUẢ PHẢI CHẤP NHẬN: khách có thể thêm vào giỏ rồi tới lúc checkout
// mới biết hết hàng. Đây là đánh đổi ĐÚNG — con số "còn hàng" hiển thị ở
// giỏ là THÔNG TIN THAM KHẢO, không phải cam kết.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	ErrNotFound       = errors.New("cart: không tìm thấy")
	ErrNoOwner        = errors.New("cart: giỏ phải thuộc về một khách hoặc một phiên")
	ErrInvalidQty     = errors.New("cart: số lượng phải lớn hơn 0")
	ErrQtyBelowMin    = errors.New("cart: số lượng dưới mức tối thiểu của offer")
	ErrQtyAboveMax    = errors.New("cart: số lượng vượt mức tối đa của offer")
	ErrCartNotActive  = errors.New("cart: giỏ không còn hoạt động")
	ErrItemNotInCart  = errors.New("cart: món hàng không thuộc giỏ này")
	ErrMixedCurrency  = errors.New("cart: không trộn nhiều đơn vị tiền tệ trong một giỏ")
	ErrDuplicateOwner = errors.New("cart: khách này đã có một giỏ đang hoạt động")
)

// Status là trạng thái của giỏ hàng.
type Status string

const (
	StatusActive Status = "ACTIVE"

	// StatusConverted: giỏ đã thành đơn hàng.
	//
	// KHÔNG xóa giỏ đã chuyển đổi: nó là dữ liệu phân tích cho biết nội
	// dung nào dẫn tới việc mua thật, và là đầu vào của bánh đà creator.
	StatusConverted Status = "CONVERTED"

	// StatusAbandoned: bỏ quên quá lâu.
	StatusAbandoned Status = "ABANDONED"

	// StatusMerged: đã gộp vào giỏ khác khi khách đăng nhập.
	StatusMerged Status = "MERGED"
)

// ItemAvailability là tình trạng của một món trong giỏ.
//
// Giỏ sống nhiều ngày nên món trong giỏ có thể đổi trạng thái bất cứ lúc
// nào: offer bị gỡ, seller bị đình chỉ, hàng hết.
//
// QUY TẮC 6: KHÔNG tự động xóa món không hợp lệ, chỉ ĐÁNH DẤU. Xóa im
// lặng làm khách bối rối — họ nhớ đã thêm món đó và không hiểu vì sao nó
// biến mất, rồi nghi ngờ cả những món còn lại.
type ItemAvailability string

const (
	AvailabilityAvailable ItemAvailability = "AVAILABLE"

	// AvailabilityOutOfStock: còn bán nhưng hết hàng.
	//
	// Vẫn hiển thị để khách đăng ký nhận thông báo khi có hàng lại — đó là
	// tín hiệu nhu cầu, và xóa đi thì mất luôn tín hiệu.
	AvailabilityOutOfStock ItemAvailability = "OUT_OF_STOCK"

	// AvailabilityUnavailable: offer bị gỡ, hoặc seller bị đình chỉ.
	AvailabilityUnavailable ItemAvailability = "UNAVAILABLE"

	// AvailabilityQuantityReduced: tồn kho ít hơn số lượng trong giỏ.
	AvailabilityQuantityReduced ItemAvailability = "QUANTITY_REDUCED"
)

// IsPurchasable cho biết món này có mua được không.
func (a ItemAvailability) IsPurchasable() bool {
	return a == AvailabilityAvailable || a == AvailabilityQuantityReduced
}

// Cart là giỏ hàng của một khách.
//
// KHÔNG chia theo seller (cart.md mục 4): việc chia diễn ra ở bước tạo
// FulfillmentOrder. Giỏ chỉ là danh sách món khách muốn mua; hiển thị có
// thể nhóm theo seller, nhưng đó là chuyện của tầng giao diện.
type Cart struct {
	id ids.ID

	// Một trong hai phải có. Khách vãng lai dùng sessionID; khi đăng nhập,
	// giỏ theo phiên được GỘP vào giỏ của tài khoản.
	customerID ids.ID
	sessionID  string

	currency money.Currency
	status   Status

	items []*Item

	// expiresAt DÀI (30 ngày), khác hẳn checkout (15 phút).
	//
	// Giỏ không giữ hàng nên để lâu không gây hại cho ai; ngược lại, giỏ
	// sống lâu là thứ khách quay lại và mua tiếp.
	expiresAt time.Time

	createdAt time.Time
	updatedAt time.Time
}

type NewCartParams struct {
	CustomerID ids.ID
	SessionID  string
	Currency   money.Currency

	// TTL mặc định 30 ngày nếu để trống.
	TTL time.Duration
	Now time.Time
}

// DefaultTTL là thời gian sống mặc định của giỏ hàng.
const DefaultTTL = 30 * 24 * time.Hour

// NewCart tạo một giỏ hàng rỗng.
func NewCart(p NewCartParams) (*Cart, error) {
	// Giỏ không biết thuộc về ai là giỏ không tìm lại được ở phiên sau.
	if p.CustomerID.IsZero() && strings.TrimSpace(p.SessionID) == "" {
		return nil, ErrNoOwner
	}

	id, err := ids.New(ids.PrefixCart)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ttl := p.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	currency := p.Currency
	if currency == "" {
		currency = money.VND
	}

	return &Cart{
		id:         id,
		customerID: p.CustomerID,
		sessionID:  strings.TrimSpace(p.SessionID),
		currency:   currency,
		status:     StatusActive,
		expiresAt:  now.Add(ttl),
		createdAt:  now,
		updatedAt:  now,
	}, nil
}

// RestoreCartParams dựng lại từ kho lưu trữ.
type RestoreCartParams struct {
	ID         ids.ID
	CustomerID ids.ID
	SessionID  string
	Currency   money.Currency
	Status     Status
	Items      []*Item
	ExpiresAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// RestoreCart dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreCart(p RestoreCartParams) *Cart {
	return &Cart{
		id:         p.ID,
		customerID: p.CustomerID,
		sessionID:  p.SessionID,
		currency:   p.Currency,
		status:     p.Status,
		items:      p.Items,
		expiresAt:  p.ExpiresAt,
		createdAt:  p.CreatedAt,
		updatedAt:  p.UpdatedAt,
	}
}

func (c *Cart) ID() ids.ID               { return c.id }
func (c *Cart) CustomerID() ids.ID       { return c.customerID }
func (c *Cart) SessionID() string        { return c.sessionID }
func (c *Cart) Currency() money.Currency { return c.currency }
func (c *Cart) Status() Status           { return c.status }
func (c *Cart) ExpiresAt() time.Time     { return c.expiresAt }
func (c *Cart) CreatedAt() time.Time     { return c.createdAt }
func (c *Cart) UpdatedAt() time.Time     { return c.updatedAt }

// IsGuestCart cho biết đây có phải giỏ của khách chưa đăng nhập không.
func (c *Cart) IsGuestCart() bool { return c.customerID.IsZero() }

// IsActive cho biết giỏ còn dùng được không.
func (c *Cart) IsActive() bool { return c.status == StatusActive }

// IsExpired cho biết giỏ đã quá hạn chưa.
func (c *Cart) IsExpired(now time.Time) bool {
	return !c.expiresAt.IsZero() && now.After(c.expiresAt)
}

// Items trả về bản sao lát cắt.
func (c *Cart) Items() []*Item {
	return append([]*Item(nil), c.items...)
}

// ItemCount là số dòng trong giỏ.
func (c *Cart) ItemCount() int { return len(c.items) }

// TotalQuantity là tổng số món, dùng cho huy hiệu trên biểu tượng giỏ.
func (c *Cart) TotalQuantity() int {
	var n int
	for _, it := range c.items {
		n += it.Quantity()
	}
	return n
}

// ItemByID tìm món theo định danh.
func (c *Cart) ItemByID(id ids.ID) (*Item, bool) {
	for _, it := range c.items {
		if it.ID() == id {
			return it, true
		}
	}
	return nil, false
}

// ItemByOffer tìm món theo offer.
//
// Dùng để CỘNG DỒN thay vì thêm dòng mới: khách thêm cùng một offer hai
// lần thì mong thấy "số lượng 2", không phải hai dòng giống hệt nhau.
func (c *Cart) ItemByOffer(offerID ids.ID) (*Item, bool) {
	for _, it := range c.items {
		if it.OfferID() == offerID {
			return it, true
		}
	}
	return nil, false
}

// Subtotal là tổng tiền theo GIÁ HIỆN TẠI.
//
// QUY TẮC 2: giá trong giỏ là giá hiện tại, CẬP NHẬT ĐỘNG — ngược hẳn với
// order, nơi mọi con số đóng băng.
//
// Con số này KHÔNG phải cam kết. Nó đổi khi seller đổi giá, và khách sẽ
// thấy giá khác ở bước checkout nếu giá vừa thay đổi. Đóng băng chỉ diễn
// ra ở checkout, khi khách thực sự cam kết mua.
func (c *Cart) Subtotal() money.Money {
	sum := money.Zero(c.currency)
	for _, it := range c.items {
		// Món không mua được không tính vào tổng: hiện một con số bao gồm
		// hàng đã hết sẽ khiến khách bất ngờ ở bước thanh toán.
		if !it.Availability().IsPurchasable() {
			continue
		}
		sum, _ = sum.Add(it.LineTotal())
	}
	return sum
}

// PurchasableItems trả các món thực sự mua được.
//
// Đây là thứ checkout dùng, KHÔNG phải Items(): giỏ giữ cả món hết hàng để
// khách thấy và quyết định, nhưng không đưa chúng vào đơn.
func (c *Cart) PurchasableItems() []*Item {
	var out []*Item
	for _, it := range c.items {
		if it.Availability().IsPurchasable() {
			out = append(out, it)
		}
	}
	return out
}

// HasUnavailableItems cho biết có món nào cần khách xử lý không.
func (c *Cart) HasUnavailableItems() bool {
	for _, it := range c.items {
		if !it.Availability().IsPurchasable() {
			return true
		}
	}
	return false
}

// SellerIDs trả danh sách nhà bán có hàng trong giỏ, không trùng lặp.
//
// Dùng để NHÓM KHI HIỂN THỊ: khách cần hiểu hàng đến từ đâu và thời gian
// giao sẽ khác nhau. Giỏ vẫn không chia theo seller ở tầng dữ liệu.
func (c *Cart) SellerIDs() []ids.ID {
	seen := map[ids.ID]bool{}
	var out []ids.ID
	for _, it := range c.items {
		if it.SellerID().IsZero() || seen[it.SellerID()] {
			continue
		}
		seen[it.SellerID()] = true
		out = append(out, it.SellerID())
	}
	return out
}

// ---------------------------------------------------------------- Hành vi

// AddItem thêm một món vào giỏ, hoặc CỘNG DỒN nếu offer đã có.
//
// Cộng dồn chứ không thêm dòng mới: khách thêm cùng offer hai lần thì mong
// thấy số lượng tăng, không phải hai dòng giống hệt nhau.
//
// Giới hạn min/max của offer được tôn trọng (quy tắc 4). Vượt max thì báo
// lỗi chứ không tự cắt: khách chọn 10 mà giỏ im lặng để 5 là hiểu nhầm sẽ
// lộ ra ở bước thanh toán.
func (c *Cart) AddItem(p NewItemParams) (*Item, error) {
	if !c.IsActive() {
		return nil, ErrCartNotActive
	}
	if p.UnitPrice.Currency() != c.currency {
		return nil, ErrMixedCurrency
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if existing, ok := c.ItemByOffer(p.OfferID); ok {
		newQty := existing.Quantity() + p.Quantity
		if err := checkQuantityBounds(newQty, p.MinOrderQuantity, p.MaxOrderQuantity); err != nil {
			return nil, err
		}
		existing.setQuantity(newQty, now)

		// Giá cập nhật theo lần thêm mới nhất: giỏ hiển thị giá HIỆN TẠI.
		existing.setUnitPrice(p.UnitPrice, now)

		// Giữ nguồn giới thiệu của lần thêm GẦN NHẤT (cart.md mục 6):
		// nội dung khiến khách quay lại thêm lần nữa là nội dung có công.
		if !p.SourceContentID.IsZero() || !p.SourceCreatorID.IsZero() {
			existing.setSource(p.SourceContentID, p.SourceCreatorID, now)
		}

		c.touch(now)
		return existing, nil
	}

	item, err := NewItem(p)
	if err != nil {
		return nil, err
	}
	item.cartID = c.id
	c.items = append(c.items, item)
	c.touch(now)
	return item, nil
}

// UpdateQuantity đổi số lượng một món.
func (c *Cart) UpdateQuantity(itemID ids.ID, quantity int, now time.Time) error {
	if !c.IsActive() {
		return ErrCartNotActive
	}
	item, ok := c.ItemByID(itemID)
	if !ok {
		return ErrItemNotInCart
	}
	if err := checkQuantityBounds(quantity, item.minOrderQuantity, item.maxOrderQuantity); err != nil {
		return err
	}
	item.setQuantity(quantity, now)
	c.touch(now)
	return nil
}

// RemoveItem xóa một món khỏi giỏ.
//
// Đây là chỗ DUY NHẤT món rời khỏi giỏ, và nó luôn do KHÁCH chủ động
// (quy tắc 6). Hệ thống chỉ đánh dấu, không bao giờ tự xóa.
func (c *Cart) RemoveItem(itemID ids.ID, now time.Time) error {
	if !c.IsActive() {
		return ErrCartNotActive
	}
	for i, it := range c.items {
		if it.ID() == itemID {
			c.items = append(c.items[:i], c.items[i+1:]...)
			c.touch(now)
			return nil
		}
	}
	return ErrItemNotInCart
}

// Clear xóa toàn bộ món trong giỏ.
func (c *Cart) Clear(now time.Time) error {
	if !c.IsActive() {
		return ErrCartNotActive
	}
	c.items = nil
	c.touch(now)
	return nil
}

// MarkConverted đánh dấu giỏ đã thành đơn hàng.
//
// Giỏ KHÔNG bị xóa: nó cho biết nội dung nào dẫn tới việc mua thật, và đó
// là dữ liệu đầu vào của bánh đà creator commerce.
func (c *Cart) MarkConverted(now time.Time) error {
	if !c.IsActive() {
		return ErrCartNotActive
	}
	c.status = StatusConverted
	c.touch(now)
	return nil
}

// MarkAbandoned đánh dấu giỏ bị bỏ quên.
func (c *Cart) MarkAbandoned(now time.Time) error {
	if !c.IsActive() {
		return ErrCartNotActive
	}
	c.status = StatusAbandoned
	c.touch(now)
	return nil
}

// AssignToCustomer gắn giỏ vãng lai cho một tài khoản.
//
// Dùng khi khách đăng nhập mà TÀI KHOẢN CHƯA CÓ giỏ nào — khi đó không
// cần gộp, chỉ cần đổi chủ.
func (c *Cart) AssignToCustomer(customerID ids.ID, now time.Time) error {
	if !c.IsActive() {
		return ErrCartNotActive
	}
	if customerID.IsZero() {
		return ErrNoOwner
	}
	c.customerID = customerID
	// Giữ sessionID để truy vết được giỏ này vốn từ phiên nào.
	c.touch(now)
	return nil
}

// MergeFrom gộp giỏ theo phiên vào giỏ này khi khách đăng nhập.
//
// Quy tắc gộp (cart.md mục 6):
//
//	Món TRÙNG offer      → cộng số lượng, tôn trọng max_order_quantity
//	Món KHÔNG trùng      → thêm vào
//	Nguồn giới thiệu     → giữ của lần thêm GẦN NHẤT
//
// Trả về danh sách CẢNH BÁO cho những món không gộp trọn vẹn được. Bên gọi
// PHẢI hiển thị chúng: im lặng bỏ qua nghĩa là khách đăng nhập xong thấy
// giỏ ít hàng hơn lúc chưa đăng nhập mà không hiểu vì sao.
func (c *Cart) MergeFrom(other *Cart, now time.Time) ([]MergeWarning, error) {
	if !c.IsActive() {
		return nil, ErrCartNotActive
	}
	if other == nil {
		return nil, nil
	}
	if other.currency != c.currency {
		return nil, ErrMixedCurrency
	}

	var warnings []MergeWarning

	for _, src := range other.items {
		existing, ok := c.ItemByOffer(src.OfferID())
		if !ok {
			clone := src.cloneInto(c.id, now)
			c.items = append(c.items, clone)
			continue
		}

		want := existing.Quantity() + src.Quantity()
		err := checkQuantityBounds(want, existing.minOrderQuantity, existing.maxOrderQuantity)
		if errors.Is(err, ErrQtyAboveMax) {
			// Cắt về mức tối đa thay vì bỏ cả món — nhưng BÁO cho khách.
			// Đây là chỗ duy nhất hệ thống tự đổi số lượng, và nó không
			// được im lặng.
			existing.setQuantity(existing.maxOrderQuantity, now)
			warnings = append(warnings, MergeWarning{
				OfferID:     src.OfferID(),
				ProductName: src.ProductName(),
				Reason:      MergeQuantityCapped,
				WantedQty:   want,
				ActualQty:   existing.maxOrderQuantity,
			})
			continue
		}
		if err != nil {
			warnings = append(warnings, MergeWarning{
				OfferID:     src.OfferID(),
				ProductName: src.ProductName(),
				Reason:      MergeRejected,
				WantedQty:   want,
				ActualQty:   existing.Quantity(),
			})
			continue
		}

		existing.setQuantity(want, now)
		if !src.SourceContentID().IsZero() || !src.SourceCreatorID().IsZero() {
			existing.setSource(src.SourceContentID(), src.SourceCreatorID(), now)
		}
	}

	other.status = StatusMerged
	other.touch(now)
	c.touch(now)
	return warnings, nil
}

// MergeReason là lý do một món không gộp trọn vẹn được.
type MergeReason string

const (
	// MergeQuantityCapped: đã cắt về mức tối đa của offer.
	MergeQuantityCapped MergeReason = "QUANTITY_CAPPED"

	// MergeRejected: không gộp được, giữ nguyên số lượng cũ.
	MergeRejected MergeReason = "REJECTED"
)

// MergeWarning là một món cần khách biết sau khi gộp giỏ.
type MergeWarning struct {
	OfferID     ids.ID
	ProductName string
	Reason      MergeReason
	WantedQty   int
	ActualQty   int
}

// Touch gia hạn giỏ khi khách còn tương tác.
func (c *Cart) Touch(now time.Time, ttl time.Duration) {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	c.expiresAt = now.Add(ttl)
	c.touch(now)
}

func (c *Cart) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.updatedAt = now
}

// checkQuantityBounds kiểm tra số lượng nằm trong giới hạn của offer.
//
// max = 0 nghĩa là KHÔNG giới hạn — cùng quy ước với marketplace.OfferView,
// để hai module không diễn giải cùng một con số theo hai cách.
func checkQuantityBounds(qty, min, max int) error {
	if qty <= 0 {
		return ErrInvalidQty
	}
	if min > 0 && qty < min {
		return ErrQtyBelowMin
	}
	if max > 0 && qty > max {
		return ErrQtyAboveMax
	}
	return nil
}
