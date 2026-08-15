package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// Item là MỘT MÓN trong giỏ hàng.
//
// ĐỐI LẬP VỚI order.Line: ở đây KHÔNG có gì đóng băng.
//
//	                 | cart.Item              | order.Line
//	-----------------|------------------------|------------------------
//	Đơn giá          | Cập nhật theo giá hiện tại | ĐÓNG BĂNG
//	Tên sản phẩm     | Đọc lại mỗi lần hiển thị| ĐÓNG BĂNG
//	Tỷ lệ hoa hồng   | KHÔNG lưu              | ĐÓNG BĂNG
//
// Lý do khác nhau: giỏ là Ý ĐỊNH mua, đơn là HỢP ĐỒNG. Ý định thì phải
// phản ánh thực tế hiện tại — giỏ hiện giá cũ sau khi seller giảm giá sẽ
// làm khách bỏ lỡ khuyến mãi, hoặc tệ hơn là thấy giá thấp rồi bị tính
// giá cao ở bước thanh toán.
//
// productName và unitPrice ĐƯỢC LƯU, nhưng chỉ như BẢN CHỤP để hiển thị
// nhanh, không phải nguồn sự thật. Chúng được làm mới mỗi lần đồng bộ giỏ.
type Item struct {
	id     ids.ID
	cartID ids.ID

	// offerID là thứ khách chọn mua: lời chào bán cụ thể của một seller.
	offerID ids.ID

	skuID    ids.ID
	sellerID ids.ID

	// Bản chụp để hiển thị, LÀM MỚI khi đồng bộ — không phải đóng băng.
	productName        string
	variantDescription string
	imageURL           string
	sellerName         string
	unitPrice          money.Money

	quantity int

	// Giới hạn của offer, chụp lại để kiểm tra số lượng mà không phải gọi
	// marketplace ở mọi thao tác tăng giảm.
	minOrderQuantity int
	maxOrderQuantity int

	availability ItemAvailability

	// availableQuantity là số lượng còn bán được tại lần đồng bộ gần nhất.
	//
	// THÔNG TIN THAM KHẢO, không phải cam kết: giỏ không giữ hàng, nên con
	// số này có thể đã cũ ngay khi khách đọc nó.
	availableQuantity int

	// Nguồn giới thiệu — mắt xích của bánh đà creator commerce.
	//
	// Ghi ngay lúc THÊM GIỎ, không đợi lúc mua (cart.md mục 5): nhờ vậy đo
	// được tỷ lệ "thêm giỏ" của từng nội dung, và quy kết đúng khi khách
	// mua sau vài ngày.
	sourceContentID ids.ID
	sourceCreatorID ids.ID

	addedAt   time.Time
	updatedAt time.Time
}

type NewItemParams struct {
	OfferID  ids.ID
	SKUID    ids.ID
	SellerID ids.ID

	ProductName        string
	VariantDescription string
	ImageURL           string
	SellerName         string
	UnitPrice          money.Money
	Quantity           int

	MinOrderQuantity int
	MaxOrderQuantity int

	SourceContentID ids.ID
	SourceCreatorID ids.ID

	Now time.Time
}

// NewItem tạo một món trong giỏ.
func NewItem(p NewItemParams) (*Item, error) {
	if p.OfferID.IsZero() {
		return nil, errors.New("cart: món hàng phải trỏ tới một offer")
	}
	if err := checkQuantityBounds(p.Quantity, p.MinOrderQuantity, p.MaxOrderQuantity); err != nil {
		return nil, err
	}
	if !p.UnitPrice.IsPositive() {
		return nil, errors.New("cart: đơn giá phải lớn hơn 0")
	}

	id, err := ids.New(ids.PrefixCartItem)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Item{
		id:                 id,
		offerID:            p.OfferID,
		skuID:              p.SKUID,
		sellerID:           p.SellerID,
		productName:        strings.TrimSpace(p.ProductName),
		variantDescription: strings.TrimSpace(p.VariantDescription),
		imageURL:           strings.TrimSpace(p.ImageURL),
		sellerName:         strings.TrimSpace(p.SellerName),
		unitPrice:          p.UnitPrice,
		quantity:           p.Quantity,
		minOrderQuantity:   p.MinOrderQuantity,
		maxOrderQuantity:   p.MaxOrderQuantity,
		availability:       AvailabilityAvailable,
		sourceContentID:    p.SourceContentID,
		sourceCreatorID:    p.SourceCreatorID,
		addedAt:            now,
		updatedAt:          now,
	}, nil
}

// RestoreItemParams dựng lại từ kho lưu trữ.
type RestoreItemParams struct {
	ID                 ids.ID
	CartID             ids.ID
	OfferID            ids.ID
	SKUID              ids.ID
	SellerID           ids.ID
	ProductName        string
	VariantDescription string
	ImageURL           string
	SellerName         string
	UnitPrice          money.Money
	Quantity           int
	MinOrderQuantity   int
	MaxOrderQuantity   int
	Availability       ItemAvailability
	AvailableQuantity  int
	SourceContentID    ids.ID
	SourceCreatorID    ids.ID
	AddedAt            time.Time
	UpdatedAt          time.Time
}

// RestoreItem dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreItem(p RestoreItemParams) *Item {
	return &Item{
		id:                 p.ID,
		cartID:             p.CartID,
		offerID:            p.OfferID,
		skuID:              p.SKUID,
		sellerID:           p.SellerID,
		productName:        p.ProductName,
		variantDescription: p.VariantDescription,
		imageURL:           p.ImageURL,
		sellerName:         p.SellerName,
		unitPrice:          p.UnitPrice,
		quantity:           p.Quantity,
		minOrderQuantity:   p.MinOrderQuantity,
		maxOrderQuantity:   p.MaxOrderQuantity,
		availability:       p.Availability,
		availableQuantity:  p.AvailableQuantity,
		sourceContentID:    p.SourceContentID,
		sourceCreatorID:    p.SourceCreatorID,
		addedAt:            p.AddedAt,
		updatedAt:          p.UpdatedAt,
	}
}

func (i *Item) ID() ids.ID                 { return i.id }
func (i *Item) CartID() ids.ID             { return i.cartID }
func (i *Item) OfferID() ids.ID            { return i.offerID }
func (i *Item) SKUID() ids.ID              { return i.skuID }
func (i *Item) SellerID() ids.ID           { return i.sellerID }
func (i *Item) ProductName() string        { return i.productName }
func (i *Item) VariantDescription() string { return i.variantDescription }
func (i *Item) ImageURL() string           { return i.imageURL }

// SellerName là tên seller để HIỂN THỊ, chụp tại lần đồng bộ gần nhất.
//
// Rỗng khi giỏ chưa từng đồng bộ kể từ khi cột này ra đời — bên gọi phải
// chịu được chuỗi rỗng, không được coi nó là "seller không tồn tại".
func (i *Item) SellerName() string             { return i.sellerName }
func (i *Item) UnitPrice() money.Money         { return i.unitPrice }
func (i *Item) Quantity() int                  { return i.quantity }
func (i *Item) MinOrderQuantity() int          { return i.minOrderQuantity }
func (i *Item) MaxOrderQuantity() int          { return i.maxOrderQuantity }
func (i *Item) Availability() ItemAvailability { return i.availability }
func (i *Item) AvailableQuantity() int         { return i.availableQuantity }
func (i *Item) SourceContentID() ids.ID        { return i.sourceContentID }
func (i *Item) SourceCreatorID() ids.ID        { return i.sourceCreatorID }
func (i *Item) AddedAt() time.Time             { return i.addedAt }
func (i *Item) UpdatedAt() time.Time           { return i.updatedAt }

// LineTotal là tiền của dòng theo GIÁ HIỆN TẠI.
func (i *Item) LineTotal() money.Money {
	return i.unitPrice.MulQuantity(int64(i.quantity))
}

// Sync làm mới bản chụp của món theo dữ liệu hiện tại của offer.
//
// Đây là hàm hiện thực hóa QUY TẮC 2 (giá cập nhật động) và QUY TẮC 6
// (không tự xóa, chỉ đánh dấu). Nó KHÔNG bao giờ xóa món và KHÔNG bao giờ
// tự giảm số lượng — chỉ ghi lại tình trạng để khách tự quyết định.
//
// Ngoại lệ duy nhất là QUANTITY_REDUCED: số lượng trong giỏ vẫn giữ
// nguyên, chỉ đánh dấu để giao diện hiển thị "chỉ còn N". Giảm số lượng
// tại đây sẽ là hệ thống tự quyết thay khách.
func (i *Item) Sync(s SyncData, now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if s.ProductName != "" {
		i.productName = s.ProductName
	}
	if s.VariantDescription != "" {
		i.variantDescription = s.VariantDescription
	}
	if s.ImageURL != "" {
		i.imageURL = s.ImageURL
	}
	if s.SellerName != "" {
		i.sellerName = s.SellerName
	}
	if s.UnitPrice.IsPositive() {
		i.unitPrice = s.UnitPrice
	}
	if s.MinOrderQuantity > 0 {
		i.minOrderQuantity = s.MinOrderQuantity
	}
	i.maxOrderQuantity = s.MaxOrderQuantity
	i.availableQuantity = s.AvailableQuantity

	switch {
	case !s.OfferExists, !s.SellerActive:
		// Offer bị gỡ hoặc seller bị đình chỉ. Món ở lại giỏ với dấu
		// UNAVAILABLE — khách thấy và tự xóa, hoặc chọn hàng thay thế.
		i.availability = AvailabilityUnavailable
	case !s.IsSellable, s.AvailableQuantity <= 0:
		i.availability = AvailabilityOutOfStock
	case s.AvailableQuantity < i.quantity:
		i.availability = AvailabilityQuantityReduced
	default:
		i.availability = AvailabilityAvailable
	}

	i.updatedAt = now
}

// SyncData là dữ liệu hiện tại của offer, do tầng application thu thập.
//
// Tầng domain KHÔNG tự đi gọi module khác — nó nhận dữ liệu vào và quyết
// định. Nhờ vậy toàn bộ luật ở trên kiểm chứng được mà không cần database
// hay module nào khác.
type SyncData struct {
	OfferExists  bool
	SellerActive bool
	IsSellable   bool

	// SKUID và SellerID chỉ dùng khi THÊM món mới — món đã có trong giỏ
	// không đổi hai trường này: offer đổi SKU nghĩa là một offer khác.
	SKUID    ids.ID
	SellerID ids.ID

	ProductName        string
	VariantDescription string
	ImageURL           string
	UnitPrice          money.Money

	// SellerName để hiển thị giỏ NHÓM THEO SELLER mà không phải gọi thêm
	// module seller ở tầng HTTP.
	SellerName string

	MinOrderQuantity int
	MaxOrderQuantity int

	// AvailableQuantity là số lượng bán được TẠI THỜI ĐIỂM ĐỒNG BỘ.
	//
	// Chỉ để hiển thị. Giỏ không giữ hàng nên con số này có thể sai ngay
	// sau khi đọc — cam kết chỉ có ở checkout.
	AvailableQuantity int
}

// setQuantity đổi số lượng. Không xuất khẩu: chỉ Cart được gọi, để giới
// hạn min/max luôn được kiểm tra ở một chỗ.
func (i *Item) setQuantity(q int, now time.Time) {
	i.quantity = q
	i.touch(now)

	// Số lượng mới có thể vượt tồn kho hiện biết — cập nhật dấu ngay để
	// khách không phải đợi lần đồng bộ sau mới thấy cảnh báo.
	if i.availability.IsPurchasable() && i.availableQuantity > 0 {
		if q > i.availableQuantity {
			i.availability = AvailabilityQuantityReduced
		} else {
			i.availability = AvailabilityAvailable
		}
	}
}

func (i *Item) setUnitPrice(p money.Money, now time.Time) {
	if !p.IsPositive() {
		return
	}
	i.unitPrice = p
	i.touch(now)
}

func (i *Item) setSource(contentID, creatorID ids.ID, now time.Time) {
	i.sourceContentID = contentID
	i.sourceCreatorID = creatorID
	i.touch(now)
}

// cloneInto tạo bản sao của món cho một giỏ khác, dùng khi gộp giỏ.
//
// Định danh MỚI: món trong giỏ đích là một bản ghi riêng, và giỏ nguồn vẫn
// giữ nguyên dữ liệu của nó để truy vết được.
func (i *Item) cloneInto(cartID ids.ID, now time.Time) *Item {
	id, err := ids.New(ids.PrefixCartItem)
	if err != nil {
		// ids.New chỉ lỗi khi nguồn ngẫu nhiên hỏng. Dùng lại định danh cũ
		// sẽ tạo hai bản ghi trùng khóa chính ở hai giỏ khác nhau.
		panic("cart: không sinh được định danh món hàng: " + err.Error())
	}

	clone := *i
	clone.id = id
	clone.cartID = cartID
	clone.updatedAt = now
	return &clone
}

func (i *Item) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	i.updatedAt = now
}
