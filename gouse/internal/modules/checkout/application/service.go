// Package application chứa các use case của module checkout.
//
// Đây là tầng ĐIỀU PHỐI của hệ thống: nó gọi cart, inventory, marketplace
// và order, nhưng gần như không sở hữu luật nghiệp vụ nào của riêng mình.
//
// Điều khó nhất ở đây không phải logic mà là XỬ LÝ THẤT BẠI GIỮA CHỪNG:
// StartCheckout giữ hàng cho từng món rồi mới tạo phiên, nên một lỗi ở
// bước sau sẽ để lại hàng bị khóa mà không phiên nào tham chiếu tới —
// khóa vĩnh viễn cho tới khi có người phát hiện thủ công.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/checkout/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

var (
	// ErrOutOfStock khi không giữ đủ hàng cho giỏ.
	ErrOutOfStock = errors.New("checkout: không đủ hàng")

	// ErrEmptyCart khi giỏ không có món nào mua được.
	ErrEmptyCart = errors.New("checkout: giỏ hàng không có món nào mua được")
)

// ---------------------------------------------------------------- Cổng ra

// CartPort là những gì checkout cần từ module cart.
type CartPort interface {
	// LoadPurchasable trả các món MUA ĐƯỢC của giỏ.
	//
	// Chỉ món mua được: giỏ giữ cả món hết hàng để khách thấy và quyết
	// định, nhưng chúng không được vào phiên thanh toán.
	LoadPurchasable(ctx context.Context, cartID ids.ID) (CartSnapshot, error)

	// MarkConverted đánh dấu giỏ đã thành đơn.
	MarkConverted(ctx context.Context, cartID ids.ID) error
}

// CartSnapshot là ảnh chụp giỏ tại thời điểm bắt đầu checkout.
type CartSnapshot struct {
	CartID     ids.ID
	CustomerID ids.ID

	// GuestEmail và GuestPhone để notification liên hệ được với khách
	// vãng lai.
	//
	// Bắt buộc phải có trong payload: module notification KHÔNG được gọi
	// ngược lại checkout hay customer để lấy — nó phụ thuộc toàn hệ thống
	// thì không tách được, và một module lỗi làm hỏng cả việc gửi email.
	GuestEmail string
	GuestPhone string

	Currency money.Currency
	Items    []CartItemSnapshot
}

// CartItemSnapshot là một món mua được trong giỏ.
//
// Giá ở đây là giá HIỆN TẠI của giỏ — nó sẽ được ĐÓNG BĂNG khi vào phiên.
type CartItemSnapshot struct {
	CartItemID ids.ID
	OfferID    ids.ID
	SKUID      ids.ID
	SellerID   ids.ID

	ProductName        string
	VariantDescription string
	UnitPrice          money.Money
	Quantity           int

	SourceContentID ids.ID
	SourceCreatorID ids.ID
}

// InventoryPort là những gì checkout cần từ module inventory.
type InventoryPort interface {
	// FindItemsForSKUs tra bản ghi tồn kho của nhiều SKU.
	FindItemsForSKUs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID][]StockItem, error)

	// Reserve giữ hàng. Đây là nơi DUY NHẤT trong hệ thống khóa tồn kho
	// cho khách hàng.
	Reserve(ctx context.Context, itemID, checkoutID ids.ID, qty int, ttl time.Duration) (ids.ID, error)

	Release(ctx context.Context, reservationID ids.ID) error
	Extend(ctx context.Context, reservationID ids.ID, d time.Duration) error
}

// StockItem là một bản ghi tồn kho khả dụng.
type StockItem struct {
	ItemID    ids.ID
	Available int
}

// CommissionPort trả tỷ lệ hoa hồng của seller.
//
// Chỉ TỶ LỆ, không phải số tiền: tính tiền ở hai nơi sẽ ra hai con số khi
// quy tắc làm tròn khác nhau.
type CommissionPort interface {
	RateForSeller(ctx context.Context, sellerID ids.ID) (types.BasisPoints, error)
}

// EventPublisher phát domain event.
//
// Là PORT do tầng application định nghĩa, nên nó không biết outbox hay
// database — chỉ biết "có một sự thật cần thông báo".
//
// Ngữ cảnh truyền vào PHẢI mang giao dịch của kho lưu trữ (xem
// domain.TxFunc): event ghi ở giao dịch khác nghĩa là phiên có thể chuyển
// COMPLETED trong khi event không tồn tại, và tồn kho sẽ mãi ở Reserved
// cho một đơn đã bán xong.
type EventPublisher interface {
	PublishCheckoutCompleted(ctx context.Context, e CheckoutCompleted) error
}

// CheckoutCompleted là sự thật "phiên thanh toán đã tạo đơn thành công".
//
// Payload chứa ĐỦ thông tin để bên nhận xử lý mà KHÔNG phải gọi ngược lại:
// inventory cần reservationID để commit, và nó không nên phải hỏi checkout.
type CheckoutCompleted struct {
	CheckoutID ids.ID
	OrderID    ids.ID

	// OrderNumber là mã hiển thị, dùng để đánh số đơn thực hiện: -A, -B, -C.
	OrderNumber string

	CartID     ids.ID
	CustomerID ids.ID

	// GuestEmail và GuestPhone để notification liên hệ được với khách
	// vãng lai.
	//
	// Bắt buộc phải có trong payload: module notification KHÔNG được gọi
	// ngược lại checkout hay customer để lấy — nó phụ thuộc toàn hệ thống
	// thì không tách được, và một module lỗi làm hỏng cả việc gửi email.
	GuestEmail string
	GuestPhone string

	Currency money.Currency

	// Reservations là các dòng hàng đã giữ chỗ, kèm đủ dữ liệu cho mọi
	// bên nhận.
	Reservations []ReservedLine
}

// ReservedLine là một dòng hàng đã giữ chỗ.
//
// Chứa ĐỦ dữ liệu cho mọi bên nhận: inventory cần reservationID để commit,
// fulfillment cần lineID và tiền để tách đơn, supply-chain cần skuID và
// số lượng để ghi tín hiệu.
//
// Nhồi đủ ngay từ đầu là chủ ý: nếu thiếu, mỗi bên nhận sẽ phải gọi ngược
// lại checkout — đúng thứ kiến trúc event sinh ra để tránh.
type ReservedLine struct {
	ReservationID   ids.ID
	InventoryItemID ids.ID

	// LineID là dòng hàng trong ĐƠN HÀNG, không phải trong phiên.
	//
	// fulfillment gom các dòng theo seller và lưu lại danh sách này để
	// biết gói nào chứa món nào.
	LineID ids.ID

	SKUID    ids.ID
	SellerID ids.ID
	Quantity int

	// ProductName để notification viết được email mà không phải gọi ngược
	// module product.
	ProductName string

	// Tiền ĐÃ ĐÓNG BĂNG, để seller đối soát phần của mình mà không cần
	// thấy toàn bộ đơn hàng.
	LineTotal        money.Money
	CommissionAmount money.Money
}

// OrderPort là những gì checkout cần từ module order.
type OrderPort interface {
	// PlaceOrder tạo đơn từ các con số ĐÃ CHỐT.
	//
	// Mọi con số truyền xuống là con số khách ĐÃ NHÌN THẤY ở màn hình
	// thanh toán. Module order không tra lại giá — đó là thiết kế, không
	// phải thiếu sót.
	PlaceOrder(ctx context.Context, in PlaceOrderInput) (PlacedOrder, error)
}

// PlaceOrderInput là dữ liệu tạo đơn.
type PlaceOrderInput struct {
	CustomerID ids.ID
	GuestEmail string
	GuestPhone string

	ShippingAddress domain.Address

	Currency       money.Currency
	ShippingFee    money.Money
	DiscountAmount money.Money
	TaxAmount      money.Money

	Lines          []PlaceOrderLine
	IdempotencyKey string
}

// PlaceOrderLine là một dòng hàng với con số đã đóng băng.
type PlaceOrderLine struct {
	OfferID  ids.ID
	SKUID    ids.ID
	SellerID ids.ID

	ProductName        string
	VariantDescription string
	UnitPrice          money.Money
	Quantity           int
	CommissionRate     types.BasisPoints
}

// PlacedOrder là kết quả tạo đơn.
type PlacedOrder struct {
	OrderID     ids.ID
	OrderNumber string

	// Replayed = true nghĩa là đơn đã tồn tại từ lần gọi trước.
	Replayed bool
}

// ---------------------------------------------------------------- Service

// Service là tầng application của module checkout.
type Service struct {
	checkouts   domain.Repository
	carts       CartPort
	inventory   InventoryPort
	commissions CommissionPort
	orders      OrderPort
	clock       Clock
	events      EventPublisher
}

type Deps struct {
	Checkouts   domain.Repository
	Carts       CartPort
	Inventory   InventoryPort
	Commissions CommissionPort
	Orders      OrderPort
	Clock       Clock

	// Events có thể nil: khi đó phiên vẫn hoạt động nhưng KHÔNG phát event.
	//
	// Chấp nhận được khi chạy test tầng dưới. KHÔNG chấp nhận được ở
	// production: thiếu event nghĩa là tồn kho không chuyển sang Committed.
	Events EventPublisher
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		checkouts:   d.Checkouts,
		carts:       d.Carts,
		inventory:   d.Inventory,
		commissions: d.Commissions,
		orders:      d.Orders,
		clock:       clock,
		events:      d.Events,
	}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// Repo trả kho lưu trữ đang dùng.
//
// Dành cho test dựng nhiều Service chia sẻ cùng một database — mô phỏng
// nhiều tiến trình API cùng chạy, thứ cần thiết để kiểm chứng tranh chấp.
func (s *Service) Repo() domain.Repository { return s.checkouts }

// ---------------------------------------------------------------- Bắt đầu

// StartCheckoutInput là dữ liệu mở một phiên thanh toán.
type StartCheckoutInput struct {
	CartID     ids.ID
	GuestEmail string
	GuestPhone string
	TTL        time.Duration
}

// StartCheckout mở phiên thanh toán từ một giỏ hàng.
//
// BỐN BƯỚC, theo đúng thứ tự ở mục 4 của đặc tả:
//
//  1. Đọc giỏ, lấy các món MUA ĐƯỢC
//  2. GIỮ TỒN KHO cho từng món (TTL 15 phút)
//  3. ĐÓNG BĂNG giá vào từng dòng
//  4. Tạo phiên
//
// THỨ TỰ NÀY KHÔNG ĐẢO ĐƯỢC. Đóng băng giá trước khi giữ hàng nghĩa là có
// lúc khách thấy giá đã chốt cho món không mua được.
//
// XỬ LÝ THẤT BẠI GIỮA CHỪNG là phần quan trọng nhất của hàm này: giữ được
// hàng cho ba món rồi món thứ tư hết hàng thì BA reservation kia phải được
// nhả. Không nhả thì hàng bị khóa 15 phút cho một phiên chưa từng tồn tại,
// và với hàng khan hiếm thì đó là ba lần mất đơn.
func (s *Service) StartCheckout(
	ctx context.Context, in StartCheckoutInput,
) (*domain.Checkout, error) {
	// Đã có phiên đang chạy cho giỏ này thì trả lại phiên đó.
	//
	// Khách bấm "Thanh toán", quay lại giỏ, bấm lần nữa — mở phiên thứ hai
	// sẽ giữ hàng LẦN THỨ HAI cho cùng một giỏ, tức là khóa gấp đôi số
	// hàng thật cần.
	existing, err := s.checkouts.FindActiveByCart(ctx, in.CartID)
	if err == nil && !existing.IsExpired(s.clock.Now()) {
		return existing, nil
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	snap, err := s.carts.LoadPurchasable(ctx, in.CartID)
	if err != nil {
		return nil, err
	}
	if len(snap.Items) == 0 {
		return nil, ErrEmptyCart
	}

	now := s.clock.Now()
	ttl := in.TTL
	if ttl <= 0 {
		ttl = domain.DefaultTTL
	}

	// Mã phiên sinh TRƯỚC khi giữ hàng: reservation cần biết nó thuộc về
	// phiên nào, và inventory dùng mã đó để dọn khi phiên chết.
	checkoutID, err := ids.New(ids.PrefixCheckout)
	if err != nil {
		return nil, err
	}

	lines, reservations, err := s.reserveAll(ctx, checkoutID, snap, ttl, now)
	if err != nil {
		// Nhả MỌI thứ đã giữ được. Đây là nhánh dễ bỏ quên nhất của cả
		// module, và bỏ quên nó nghĩa là mỗi lần một món hết hàng thì các
		// món khác trong giỏ bị khóa vô ích 15 phút.
		s.releaseAll(ctx, reservations)
		return nil, err
	}

	c, err := domain.NewCheckout(domain.NewCheckoutParams{
		ID:         checkoutID,
		CartID:     snap.CartID,
		CustomerID: snap.CustomerID,
		GuestEmail: in.GuestEmail,
		GuestPhone: in.GuestPhone,
		Currency:   snap.Currency,
		Lines:      lines,
		TTL:        ttl,
		Now:        now,
	})
	if err != nil {
		s.releaseAll(ctx, reservations)
		return nil, err
	}
	if err := s.checkouts.Save(ctx, c); err != nil {
		s.releaseAll(ctx, reservations)
		return nil, err
	}
	return c, nil
}

// reserveAll giữ hàng cho mọi món và đóng băng giá vào từng dòng.
//
// Trả về cả danh sách reservation đã tạo, KỂ CẢ khi lỗi: bên gọi cần chúng
// để nhả. Trả nil khi lỗi sẽ làm mất dấu những gì đã khóa.
func (s *Service) reserveAll(
	ctx context.Context, checkoutID ids.ID, snap CartSnapshot,
	ttl time.Duration, now time.Time,
) ([]*domain.Line, []ids.ID, error) {
	skuIDs := make([]ids.ID, 0, len(snap.Items))
	for _, it := range snap.Items {
		skuIDs = append(skuIDs, it.SKUID)
	}

	// MỘT lượt gọi cho cả giỏ, không phải một lượt mỗi món.
	stock, err := s.inventory.FindItemsForSKUs(ctx, skuIDs)
	if err != nil {
		return nil, nil, err
	}

	var (
		lines        []*domain.Line
		reservations []ids.ID
	)

	for _, it := range snap.Items {
		itemID, ok := pickStockItem(stock[it.SKUID], it.Quantity)
		if !ok {
			return nil, reservations, fmt.Errorf("%w: %s (cần %d)",
				ErrOutOfStock, it.ProductName, it.Quantity)
		}

		reservationID, err := s.inventory.Reserve(ctx, itemID, checkoutID, it.Quantity, ttl)
		if err != nil {
			// Có thể là tranh chấp: người khác vừa mua mất. Không phân
			// biệt hai trường hợp ở đây — với khách thì cả hai đều là
			// "không mua được món này".
			return nil, reservations, fmt.Errorf("%w: %s: %v",
				ErrOutOfStock, it.ProductName, err)
		}
		reservations = append(reservations, reservationID)

		rate, err := s.commissionRate(ctx, it.SellerID)
		if err != nil {
			return nil, reservations, err
		}

		line, err := domain.NewLine(domain.NewLineParams{
			CartItemID:         it.CartItemID,
			OfferID:            it.OfferID,
			SKUID:              it.SKUID,
			SellerID:           it.SellerID,
			ProductName:        it.ProductName,
			VariantDescription: it.VariantDescription,
			// ĐÓNG BĂNG: từ đây tới lúc tạo đơn, con số này không đổi dù
			// seller có sửa giá.
			UnitPrice:       it.UnitPrice,
			Quantity:        it.Quantity,
			CommissionRate:  rate,
			ReservationID:   reservationID,
			InventoryItemID: itemID,
			Now:             now,
		})
		if err != nil {
			return nil, reservations, err
		}
		lines = append(lines, line)
	}

	return lines, reservations, nil
}

// pickStockItem chọn kho để lấy hàng.
//
// Quy tắc hiện tại: kho ĐẦU TIÊN còn đủ hàng. Đơn giản và đủ cho MVP.
//
// Chọn kho gần khách nhất sẽ giảm phí ship và thời gian giao, nhưng cần
// biết địa chỉ — thứ khách chưa nhập tại thời điểm này. Đổi sang tiêu chí
// đó là việc của giai đoạn sau, và nó chỉ ảnh hưởng hàm này.
func pickStockItem(items []StockItem, need int) (ids.ID, bool) {
	for _, it := range items {
		if it.Available >= need {
			return it.ItemID, true
		}
	}
	return "", false
}

// releaseAll nhả mọi reservation, bỏ qua lỗi từng cái.
//
// Bỏ qua lỗi CÓ CHỦ Ý: hàm này chạy trên đường xử lý lỗi, và dừng lại ở
// cái đầu tiên thất bại sẽ để những cái sau bị khóa luôn. Reservation nào
// không nhả được vẫn còn TTL — tiến trình nền của inventory sẽ dọn.
func (s *Service) releaseAll(ctx context.Context, reservations []ids.ID) {
	for _, id := range reservations {
		_ = s.inventory.Release(ctx, id)
	}
}

func (s *Service) commissionRate(
	ctx context.Context, sellerID ids.ID,
) (types.BasisPoints, error) {
	if s.commissions == nil || sellerID.IsZero() {
		return types.BasisPoints{}, nil
	}
	return s.commissions.RateForSeller(ctx, sellerID)
}

// ---------------------------------------------------------------- Sửa phiên

func (s *Service) GetCheckout(ctx context.Context, id ids.ID) (*domain.Checkout, error) {
	return s.checkouts.FindByID(ctx, id)
}

// SetShippingAddress đặt địa chỉ giao hàng.
func (s *Service) SetShippingAddress(
	ctx context.Context, id ids.ID, addr domain.Address,
) (*domain.Checkout, error) {
	return s.mutate(ctx, id, func(c *domain.Checkout, now time.Time) error {
		return c.SetShippingAddress(addr, now)
	})
}

// SetShipping đặt phương thức và phí vận chuyển.
func (s *Service) SetShipping(
	ctx context.Context, id ids.ID, method string, fee money.Money,
) (*domain.Checkout, error) {
	return s.mutate(ctx, id, func(c *domain.Checkout, now time.Time) error {
		return c.SetShipping(method, fee, now)
	})
}

// ApplyDiscount áp một khoản giảm giá.
func (s *Service) ApplyDiscount(
	ctx context.Context, id ids.ID, code string, amount money.Money,
) (*domain.Checkout, error) {
	return s.mutate(ctx, id, func(c *domain.Checkout, now time.Time) error {
		return c.ApplyDiscount(code, amount, now)
	})
}

// RemoveDiscount gỡ mã giảm giá.
func (s *Service) RemoveDiscount(ctx context.Context, id ids.ID) (*domain.Checkout, error) {
	return s.mutate(ctx, id, func(c *domain.Checkout, now time.Time) error {
		return c.RemoveDiscount(now)
	})
}

// MarkPendingPayment chuyển phiên sang chờ thanh toán.
func (s *Service) MarkPendingPayment(ctx context.Context, id ids.ID) (*domain.Checkout, error) {
	return s.mutate(ctx, id, func(c *domain.Checkout, now time.Time) error {
		return c.MarkPendingPayment(now)
	})
}

// Extend gia hạn phiên VÀ gia hạn mọi reservation của nó.
//
// Hai việc phải đi cùng nhau: phiên sống lâu hơn reservation thì tới lúc
// đặt hàng mới phát hiện hàng đã bị nhả và bán cho người khác — đúng lúc
// khách vừa chuyển khoản xong.
func (s *Service) Extend(ctx context.Context, id ids.ID) (*domain.Checkout, error) {
	c, err := s.checkouts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	if err := c.Extend(domain.ExtendDuration, now); err != nil {
		return nil, err
	}

	for _, reservationID := range c.ReservationIDs() {
		if err := s.inventory.Extend(ctx, reservationID, domain.ExtendDuration); err != nil {
			return nil, fmt.Errorf("checkout: gia hạn giữ hàng: %w", err)
		}
	}

	if err := s.checkouts.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Cancel hủy phiên và NHẢ HÀNG.
func (s *Service) Cancel(ctx context.Context, id ids.ID) error {
	c, err := s.checkouts.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if err := c.Cancel(s.clock.Now()); err != nil {
		return err
	}

	// Nhả hàng TRƯỚC khi ghi: nếu ghi thành công mà nhả thất bại, phiên
	// mang trạng thái CANCELLED nhưng hàng vẫn khóa, và không tiến trình
	// nào đi tìm nó nữa vì phiên đã kết thúc.
	s.releaseAll(ctx, c.ReservationIDs())

	return s.checkouts.Save(ctx, c)
}

func (s *Service) mutate(
	ctx context.Context, id ids.ID,
	apply func(*domain.Checkout, time.Time) error,
) (*domain.Checkout, error) {
	c, err := s.checkouts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := apply(c, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.checkouts.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ---------------------------------------------------------------- Hoàn tất

// CompleteResult là kết quả hoàn tất phiên thanh toán.
type CompleteResult struct {
	Checkout    *domain.Checkout
	OrderID     ids.ID
	OrderNumber string

	// Replayed = true nghĩa là phiên này ĐÃ hoàn tất từ trước.
	Replayed bool
}

// CompleteCheckout tạo đơn hàng từ phiên thanh toán.
//
// QUY TẮC 5: IDEMPOTENT. Khách bấm "Thanh toán" hai lần, hoặc client thử
// lại sau timeout — không được tạo hai đơn.
//
// Hai lớp bảo vệ, vì một lớp không đủ:
//
//  1. Phiên đã COMPLETED  → trả đơn cũ ngay
//  2. order.PlaceOrder    → cũng idempotent theo cùng khóa
//
// Lớp 2 là lớp bắt được trường hợp khó: hai request song song cùng qua
// được lớp 1, rồi ràng buộc UNIQUE ở database của module order chặn cái
// thứ hai.
//
// LƯU Ý về thứ tự (mục 10 của đặc tả): KHÔNG hủy phiên khi thanh toán thất
// bại. Cho khách thử lại phương thức khác trong thời gian TTL còn lại —
// hủy ngay là trải nghiệm tệ và làm mất đơn hàng.
func (s *Service) CompleteCheckout(
	ctx context.Context, id ids.ID, idempotencyKey string,
) (*CompleteResult, error) {
	if idempotencyKey == "" {
		return nil, domain.ErrMissingIdemKey
	}

	c, err := s.checkouts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Lớp 1: phiên đã hoàn tất rồi.
	if c.Status() == domain.StatusCompleted {
		return &CompleteResult{
			Checkout: c,
			OrderID:  c.OrderID(),
			Replayed: true,
		}, nil
	}

	now := s.clock.Now()

	// Hết hạn thì KHÔNG cho tạo đơn: hàng có thể đã bị nhả và bán cho
	// người khác. Kiểm tra theo ĐỒNG HỒ chứ không chỉ theo trạng thái —
	// tiến trình nền chạy theo chu kỳ nên luôn có khoảng trống.
	if c.IsExpired(now) {
		return nil, domain.ErrExpired
	}
	if c.Status().IsFinal() {
		return nil, domain.ErrInvalidStatus
	}
	if c.ShippingAddress().IsEmpty() {
		return nil, domain.ErrNoAddress
	}

	lines := make([]PlaceOrderLine, 0, len(c.Lines()))
	for _, l := range c.Lines() {
		// Quy tắc 1: không giữ được hàng thì không vào đơn.
		if !l.HasStock() {
			return nil, fmt.Errorf("%w: %s", ErrOutOfStock, l.ProductName())
		}
		lines = append(lines, PlaceOrderLine{
			OfferID:  l.OfferID(),
			SKUID:    l.SKUID(),
			SellerID: l.SellerID(),

			// Truyền THẲNG con số đã đóng băng. Tính lại ở đây sẽ phá vỡ
			// toàn bộ ý nghĩa của việc đóng băng ở bước StartCheckout.
			ProductName:        l.ProductName(),
			VariantDescription: l.VariantDescription(),
			UnitPrice:          l.UnitPrice(),
			Quantity:           l.Quantity(),
			CommissionRate:     l.CommissionRate(),
		})
	}

	// Lớp 2: order.PlaceOrder idempotent theo cùng khóa.
	placed, err := s.orders.PlaceOrder(ctx, PlaceOrderInput{
		CustomerID:      c.CustomerID(),
		GuestEmail:      c.GuestEmail(),
		GuestPhone:      c.GuestPhone(),
		ShippingAddress: c.ShippingAddress(),
		Currency:        c.Currency(),
		ShippingFee:     c.ShippingFee(),
		DiscountAmount:  c.DiscountAmount(),
		TaxAmount:       c.TaxAmount(),
		Lines:           lines,
		IdempotencyKey:  idempotencyKey,
	})
	if err != nil {
		// Tạo đơn thất bại: KHÔNG hủy phiên, KHÔNG nhả hàng. Khách thử
		// lại được trong thời gian TTL còn lại.
		return nil, err
	}

	if err := c.Complete(placed.OrderID, idempotencyKey, now); err != nil {
		return nil, err
	}

	// Ghi trạng thái phiên VÀ phát event trong CÙNG một giao dịch.
	//
	// Đây là chỗ Transactional Outbox phát huy tác dụng: nếu phiên chuyển
	// COMPLETED mà event không được ghi, tồn kho sẽ mãi nằm ở Reserved cho
	// một đơn đã bán xong — và không tiến trình nào đi tìm nó, vì phiên đã
	// kết thúc nên job dọn không quét tới.
	if err := s.checkouts.SaveWithEvents(ctx, c, func(txCtx context.Context) error {
		if s.events == nil {
			return nil
		}
		return s.events.PublishCheckoutCompleted(txCtx,
			s.completedEvent(c, placed.OrderID, placed.OrderNumber))
	}); err != nil {
		return nil, err
	}

	// Đánh dấu giỏ đã chuyển đổi. Lỗi ở đây KHÔNG làm hỏng đơn: đơn đã
	// tạo xong và khách đã trả tiền. Giỏ còn ACTIVE chỉ gây phiền là khách
	// thấy hàng cũ trong giỏ — khó chịu, nhưng không mất tiền của ai.
	if err := s.carts.MarkConverted(ctx, c.CartID()); err != nil {
		_ = err
	}

	return &CompleteResult{
		Checkout:    c,
		OrderID:     placed.OrderID,
		OrderNumber: placed.OrderNumber,
		Replayed:    placed.Replayed,
	}, nil
}

// completedEvent dựng dữ liệu event từ phiên đã hoàn tất.
func (s *Service) completedEvent(
	c *domain.Checkout, orderID ids.ID, orderNumber string,
) CheckoutCompleted {
	lines := c.Lines()
	reservations := make([]ReservedLine, 0, len(lines))
	for _, l := range lines {
		if !l.HasStock() {
			continue
		}
		// Hoa hồng tính từ con số ĐÃ ĐÓNG BĂNG trong phiên — cùng cách
		// module order tính, nên hai bên ra cùng kết quả.
		lineTotal := l.LineTotal()
		commission := lineTotal.ApplyRate(l.CommissionRate(), money.RoundHalfUp)

		reservations = append(reservations, ReservedLine{
			ReservationID:    l.ReservationID(),
			InventoryItemID:  l.InventoryItemID(),
			LineID:           l.ID(),
			SKUID:            l.SKUID(),
			SellerID:         l.SellerID(),
			Quantity:         l.Quantity(),
			ProductName:      l.ProductName(),
			LineTotal:        lineTotal,
			CommissionAmount: commission,
		})
	}

	return CheckoutCompleted{
		CheckoutID:   c.ID(),
		OrderID:      orderID,
		OrderNumber:  orderNumber,
		CartID:       c.CartID(),
		CustomerID:   c.CustomerID(),
		GuestEmail:   c.GuestEmail(),
		GuestPhone:   c.GuestPhone(),
		Currency:     c.Currency(),
		Reservations: reservations,
	}
}

// ---------------------------------------------------------------- Dọn dẹp

// ExpireStale đánh dấu các phiên quá hạn và NHẢ HÀNG.
//
// Chạy bởi tiến trình nền. Đây là hàm giữ cho lời hứa "giữ hàng có thời
// hạn" thành sự thật — không có nó thì mọi phiên bỏ dở đều khóa hàng cho
// tới khi ai đó phát hiện thủ công.
//
// Trả về số phiên đã dọn.
func (s *Service) ExpireStale(ctx context.Context, limit int) (int, error) {
	now := s.clock.Now()

	stale, err := s.checkouts.FindExpired(ctx, now, limit)
	if err != nil {
		return 0, err
	}

	var done int
	for _, c := range stale {
		if err := c.MarkExpired(now); err != nil {
			continue
		}

		// Nhả hàng trước khi ghi — cùng lý do như ở Cancel: phiên đánh dấu
		// EXPIRED mà hàng còn khóa thì không tiến trình nào tìm nó nữa.
		s.releaseAll(ctx, c.ReservationIDs())

		if err := s.checkouts.Save(ctx, c); err != nil {
			return done, err
		}
		done++
	}
	return done, nil
}

// CountExpiredPending đếm số phiên quá hạn chưa dọn.
//
// Chỉ báo giám sát: con số tăng dần nghĩa là tiến trình dọn đã ngừng chạy.
func (s *Service) CountExpiredPending(ctx context.Context) (int, error) {
	return s.checkouts.CountExpiredPending(ctx, s.clock.Now())
}
