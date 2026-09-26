package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// Line là MỘT MÓN trong phiên thanh toán, với GIÁ ĐÃ ĐÓNG BĂNG.
//
// ĐÂY LÀ CHỖ GIÁ CHUYỂN TỪ ĐỘNG SANG TĨNH — ranh giới thật giữa ba module:
//
//	cart.Item      → giá ĐỘNG, đọc lại mỗi lần hiển thị
//	checkout.Line  → giá ĐÓNG BĂNG tại thời điểm bấm "Thanh toán"  ← ở đây
//	order.Line     → giá ĐÓNG BĂNG, giữ vĩnh viễn
//
// Con số đóng băng ở đây được TRUYỀN THẲNG sang order.PlaceOrder, không
// tính lại. Tính lại ở bước tạo đơn sẽ làm hỏng toàn bộ ý nghĩa của việc
// đóng băng: khách thấy 299.000đ ở màn hình thanh toán rồi bị trừ 350.000đ.
//
// commissionRate cũng đóng băng tại đây vì cùng lý do như ở order: đối
// soát tháng trước phải ra cùng một con số dù chính sách hoa hồng đã đổi.
type Line struct {
	id         ids.ID
	checkoutID ids.ID

	// cartItemID để truy vết ngược về giỏ.
	cartItemID ids.ID

	offerID    ids.ID
	skuID      ids.ID
	sellerID   ids.ID
	sellerName string

	// handlingTimeHours là thời gian nhà bán cần để chuẩn bị món này,
	// ĐÓNG BĂNG từ giỏ. Nó quyết định ngày giao dự kiến của nhóm —
	// xem migration 000054.
	handlingTimeHours int

	// ---- ĐÓNG BĂNG tại thời điểm bắt đầu checkout ----

	productName        string
	variantDescription string
	unitPrice          money.Money
	quantity           int
	commissionRate     types.BasisPoints

	// ---- Nguồn QUY CÔNG, mang từ giỏ sang đơn ----
	//
	// Phiên thanh toán không LÀM GÌ với ba trường này. Nó chỉ chở chúng:
	// `cart_item` biết khách đến từ nội dung nào, `order_line` cần biết để
	// tính hoa hồng creator, và giữa hai chỗ ấy là đây.
	//
	// Trước 25/09/2026 chuỗi này ĐỨT ở đúng chỗ này — cả hai đầu đã có cột
	// và index, và không gì nối chúng. Xem migration 000057.
	//
	// CHỈ hai cái mã, không mang tỷ lệ hoa hồng: tỷ lệ đóng băng ở bảng
	// `attribution` của module affiliate. Hai nguồn cho cùng một con số
	// tiền sớm muộn sẽ lệch nhau.
	sourceContentID ids.ID
	sourceCreatorID ids.ID

	// reservationID là mã giữ hàng ở inventory.
	//
	// KHÔNG CÓ Ở cart.Item, và đó chính là khác biệt cốt lõi giữa hai
	// module: dòng này đang KHÓA hàng thật, món trong giỏ thì không.
	//
	// Rỗng nghĩa là chưa giữ được hàng — phiên không được phép tiếp tục.
	reservationID ids.ID

	// inventoryItemID là bản ghi tồn kho cụ thể đang giữ hàng.
	//
	// Một SKU có thể nằm ở nhiều kho; đây là kho ĐÃ CHỌN. Giữ lại để nhả
	// đúng chỗ và để fulfillment biết xuất hàng từ đâu.
	inventoryItemID ids.ID

	createdAt time.Time
}

type NewLineParams struct {
	CartItemID        ids.ID
	OfferID           ids.ID
	SKUID             ids.ID
	SellerID          ids.ID
	SellerName        string
	HandlingTimeHours int

	ProductName        string
	VariantDescription string
	UnitPrice          money.Money
	Quantity           int
	CommissionRate     types.BasisPoints

	ReservationID   ids.ID
	InventoryItemID ids.ID

	Now time.Time

	// Nguồn quy công, chở từ giỏ sang đơn — xem chú thích ở `Line`.
	SourceContentID ids.ID
	SourceCreatorID ids.ID
}

// NewLine tạo một dòng với giá ĐÃ ĐÓNG BĂNG.
func NewLine(p NewLineParams) (*Line, error) {
	if p.OfferID.IsZero() {
		return nil, errors.New("checkout: dòng hàng phải trỏ tới một offer")
	}
	if p.Quantity <= 0 {
		return nil, errors.New("checkout: số lượng phải lớn hơn 0")
	}
	if !p.UnitPrice.IsPositive() {
		return nil, errors.New("checkout: đơn giá phải lớn hơn 0")
	}
	if strings.TrimSpace(p.ProductName) == "" {
		return nil, errors.New("checkout: phải đóng băng tên sản phẩm")
	}

	id, err := ids.New(ids.PrefixCheckoutLine)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Line{
		id:                 id,
		cartItemID:         p.CartItemID,
		offerID:            p.OfferID,
		skuID:              p.SKUID,
		sellerID:           p.SellerID,
		sellerName:         strings.TrimSpace(p.SellerName),
		handlingTimeHours:  p.HandlingTimeHours,
		productName:        strings.TrimSpace(p.ProductName),
		variantDescription: strings.TrimSpace(p.VariantDescription),
		unitPrice:          p.UnitPrice,
		quantity:           p.Quantity,
		commissionRate:     p.CommissionRate,

		sourceContentID: p.SourceContentID,
		sourceCreatorID: p.SourceCreatorID,

		reservationID:   p.ReservationID,
		inventoryItemID: p.InventoryItemID,
		createdAt:       now,
	}, nil
}

// RestoreLineParams dựng lại từ kho lưu trữ.
type RestoreLineParams struct {
	ID                 ids.ID
	CheckoutID         ids.ID
	CartItemID         ids.ID
	OfferID            ids.ID
	SKUID              ids.ID
	SellerID           ids.ID
	SellerName         string
	HandlingTimeHours  int
	ProductName        string
	VariantDescription string
	UnitPrice          money.Money
	Quantity           int
	CommissionRate     types.BasisPoints
	ReservationID      ids.ID
	InventoryItemID    ids.ID
	CreatedAt          time.Time

	// Nguồn quy công, chở từ giỏ sang đơn — xem chú thích ở `Line`.
	SourceContentID ids.ID
	SourceCreatorID ids.ID
}

// RestoreLine dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreLine(p RestoreLineParams) *Line {
	return &Line{
		id:                 p.ID,
		checkoutID:         p.CheckoutID,
		cartItemID:         p.CartItemID,
		offerID:            p.OfferID,
		skuID:              p.SKUID,
		sellerID:           p.SellerID,
		sellerName:         p.SellerName,
		handlingTimeHours:  p.HandlingTimeHours,
		productName:        p.ProductName,
		variantDescription: p.VariantDescription,
		unitPrice:          p.UnitPrice,
		quantity:           p.Quantity,
		commissionRate:     p.CommissionRate,

		sourceContentID: p.SourceContentID,
		sourceCreatorID: p.SourceCreatorID,

		reservationID:   p.ReservationID,
		inventoryItemID: p.InventoryItemID,
		createdAt:       p.CreatedAt,
	}
}

func (l *Line) ID() ids.ID         { return l.id }
func (l *Line) CheckoutID() ids.ID { return l.checkoutID }
func (l *Line) CartItemID() ids.ID { return l.cartItemID }
func (l *Line) OfferID() ids.ID    { return l.offerID }
func (l *Line) SKUID() ids.ID      { return l.skuID }
func (l *Line) SellerID() ids.ID   { return l.sellerID }
func (l *Line) SellerName() string { return l.sellerName }

// HandlingTimeHours là thời gian chuẩn bị hàng, 0 nghĩa là KHÔNG BIẾT.
func (l *Line) HandlingTimeHours() int            { return l.handlingTimeHours }
func (l *Line) ProductName() string               { return l.productName }
func (l *Line) VariantDescription() string        { return l.variantDescription }
func (l *Line) UnitPrice() money.Money            { return l.unitPrice }
func (l *Line) Quantity() int                     { return l.quantity }
func (l *Line) CommissionRate() types.BasisPoints { return l.commissionRate }

func (l *Line) SourceContentID() ids.ID { return l.sourceContentID }
func (l *Line) SourceCreatorID() ids.ID { return l.sourceCreatorID }
func (l *Line) ReservationID() ids.ID   { return l.reservationID }
func (l *Line) InventoryItemID() ids.ID { return l.inventoryItemID }
func (l *Line) CreatedAt() time.Time    { return l.createdAt }

// LineTotal là tiền của dòng theo giá ĐÃ ĐÓNG BĂNG.
func (l *Line) LineTotal() money.Money {
	return l.unitPrice.MulQuantity(int64(l.quantity))
}

// HasStock cho biết dòng này đã giữ được hàng chưa.
//
// Quy tắc 1: BẮT BUỘC giữ tồn kho trước khi cho checkout. Dòng không giữ
// được hàng mà vẫn vào đơn nghĩa là bán thứ không có.
func (l *Line) HasStock() bool { return !l.reservationID.IsZero() }

// setCheckoutID gắn dòng vào phiên. Không xuất khẩu: chỉ tầng lưu trữ và
// constructor của Checkout được dùng.
func (l *Line) setCheckoutID(id ids.ID) { l.checkoutID = id }
