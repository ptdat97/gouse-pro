package checkout

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/cart"
	"github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/modules/checkout/domain"
	checkoutpg "github.com/fashion-commerce/platform/internal/modules/checkout/infrastructure/postgres"
	checkouthttp "github.com/fashion-commerce/platform/internal/modules/checkout/interfaces/http"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/modules/promotion"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Hai bất biến của module cần ràng buộc ở tầng database: một giỏ chỉ
	// có một phiên đang chạy, và một khóa hoàn tất chỉ tạo một đơn. Cả hai
	// đều là chỉ mục UNIQUE CÓ ĐIỀU KIỆN — thứ không mô phỏng được bằng
	// kiểm tra trước khi ghi.
	Storage string

	DB *database.DB

	// Bốn module checkout điều phối. Thiếu bất kỳ cái nào thì luồng mua
	// hàng không chạy được, nên tất cả đều BẮT BUỘC.
	Cart        cart.API
	Inventory   inventory.API
	Marketplace marketplace.API
	Order       order.API

	// Promotion cho phép áp mã giảm giá. Có thể nil: phiên vẫn chạy,
	// chỉ là khách không dùng được mã.
	Promotion promotion.API

	Clock application.Clock

	// Events phát domain event. Nil nghĩa là KHÔNG phát.
	//
	// Ở production đây là thiếu sót nghiêm trọng: không có event thì tồn
	// kho không chuyển Reserved → Committed, và tiến trình dọn có thể nhả
	// hàng của một đơn đã thanh toán.
	Events *eventbus.Outbox
}

// New khởi tạo module checkout.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"checkout: chỉ hỗ trợ kho lưu trữ postgres — quy tắc một phiên " +
				"mỗi giỏ và một đơn mỗi khóa cần chỉ mục UNIQUE có điều kiện")
	}
	if cfg.DB == nil {
		return nil, errors.New("checkout: bắt buộc phải có kết nối database")
	}

	for _, dep := range []struct {
		name string
		ok   bool
	}{
		{"cart", cfg.Cart != nil},
		{"inventory", cfg.Inventory != nil},
		{"marketplace", cfg.Marketplace != nil},
		{"order", cfg.Order != nil},
	} {
		if !dep.ok {
			return nil, errors.New("checkout: bắt buộc phải có module " + dep.name)
		}
	}

	deps := application.Deps{
		Checkouts:   checkoutpg.NewCheckoutStore(cfg.DB.Pool()),
		Carts:       &cartAdapter{api: cfg.Cart},
		Inventory:   &inventoryAdapter{api: cfg.Inventory},
		Commissions: &commissionAdapter{api: cfg.Marketplace},
		Orders:      &orderAdapter{api: cfg.Order},
		Clock:       cfg.Clock,
	}
	if cfg.Events != nil {
		deps.Events = &eventPublisher{outbox: cfg.Events}
	}
	if cfg.Promotion != nil {
		deps.Promotions = &promotionAdapter{api: cfg.Promotion}
	}

	return &Module{svc: application.NewService(deps)}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// RegisterRoutes gắn các endpoint phiên thanh toán vào mux.
//
// Bên gọi PHẢI bọc httpserver.ResolveShopper quanh mux này, và
// httpserver.RequireIdempotencyKey cho các đường POST/PATCH.
func (m *Module) RegisterRoutes(mux *http.ServeMux, log *slog.Logger) {
	checkouthttp.NewHandler(m.svc, log).Register(mux)
}

// ---------------------------------------------------------------- API

func (m *Module) StartCheckout(
	ctx context.Context, req StartCheckoutRequest,
) (*CheckoutView, error) {
	cartID, err := ids.Parse(req.CartID, ids.PrefixCart)
	if err != nil {
		return nil, ErrInvalidID
	}

	c, err := m.svc.StartCheckout(ctx, application.StartCheckoutInput{
		CartID:     cartID,
		GuestEmail: strings.TrimSpace(req.GuestEmail),
		GuestPhone: strings.TrimSpace(req.GuestPhone),
	})
	if err != nil {
		return nil, translateErr(err)
	}
	return m.view(c), nil
}

func (m *Module) GetCheckout(ctx context.Context, checkoutID string) (*CheckoutView, error) {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return nil, ErrInvalidID
	}
	c, err := m.svc.GetCheckout(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return m.view(c), nil
}

func (m *Module) SetShippingAddress(
	ctx context.Context, checkoutID string, addr AddressInput,
) (*CheckoutView, error) {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return nil, ErrInvalidID
	}

	c, err := m.svc.SetShippingAddress(ctx, id, domain.Address{
		RecipientName: strings.TrimSpace(addr.RecipientName),
		Phone:         strings.TrimSpace(addr.Phone),
		StreetAddress: strings.TrimSpace(addr.StreetAddress),
		Ward:          strings.TrimSpace(addr.Ward),
		District:      strings.TrimSpace(addr.District),
		Province:      strings.TrimSpace(addr.Province),
		CountryCode:   strings.TrimSpace(addr.CountryCode),
	})
	if err != nil {
		return nil, translateErr(err)
	}
	return m.view(c), nil
}

func (m *Module) SetShippingMethod(
	ctx context.Context, checkoutID, method string, fee Amount,
) (*CheckoutView, error) {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return nil, ErrInvalidID
	}

	feeMoney, err := toMoney(fee)
	if err != nil {
		return nil, err
	}

	c, err := m.svc.SetShipping(ctx, id, method, feeMoney)
	if err != nil {
		return nil, translateErr(err)
	}
	return m.view(c), nil
}

func (m *Module) ApplyCoupon(
	ctx context.Context, checkoutID, code string, discount Amount,
) (*CheckoutView, error) {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return nil, ErrInvalidID
	}

	amount, err := toMoney(discount)
	if err != nil {
		return nil, err
	}

	c, err := m.svc.ApplyDiscount(ctx, id, code, amount)
	if err != nil {
		return nil, translateErr(err)
	}
	return m.view(c), nil
}

func (m *Module) RemoveCoupon(ctx context.Context, checkoutID string) (*CheckoutView, error) {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return nil, ErrInvalidID
	}
	c, err := m.svc.RemoveDiscount(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return m.view(c), nil
}

func (m *Module) MarkReadyForPayment(
	ctx context.Context, checkoutID string,
) (*CheckoutView, error) {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return nil, ErrInvalidID
	}
	c, err := m.svc.MarkPendingPayment(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return m.view(c), nil
}

func (m *Module) ExtendCheckout(ctx context.Context, checkoutID string) (*CheckoutView, error) {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return nil, ErrInvalidID
	}
	c, err := m.svc.Extend(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return m.view(c), nil
}

func (m *Module) CancelCheckout(ctx context.Context, checkoutID string) error {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.Cancel(ctx, id))
}

func (m *Module) CompleteCheckout(
	ctx context.Context, checkoutID, idempotencyKey string,
) (*CompleteResult, error) {
	id, err := ids.Parse(checkoutID, ids.PrefixCheckout)
	if err != nil {
		return nil, ErrInvalidID
	}

	res, err := m.svc.CompleteCheckout(ctx, id, strings.TrimSpace(idempotencyKey))
	if err != nil {
		return nil, translateErr(err)
	}

	return &CompleteResult{
		Checkout:    *m.view(res.Checkout),
		OrderID:     res.OrderID.String(),
		OrderNumber: res.OrderNumber,
		Replayed:    res.Replayed,
	}, nil
}

func (m *Module) ExpireStale(ctx context.Context, limit int) (int, error) {
	n, err := m.svc.ExpireStale(ctx, limit)
	return n, translateErr(err)
}

func (m *Module) CountExpiredPending(ctx context.Context) (int, error) {
	n, err := m.svc.CountExpiredPending(ctx)
	return n, translateErr(err)
}

// ---------------------------------------------------------------- Chuyển đổi

func toMoney(a Amount) (money.Money, error) {
	if a.Value == 0 && a.Currency == "" {
		return money.Money{}, nil
	}
	m, err := money.New(a.Value, money.Currency(a.Currency))
	if err != nil {
		return money.Money{}, ErrInvalidInput
	}
	return m, nil
}

func toAmount(m money.Money) Amount {
	return Amount{Value: m.Amount(), Currency: string(m.Currency())}
}

func (m *Module) view(c *domain.Checkout) *CheckoutView {
	now := m.svc.Now()

	lines := c.Lines()
	lineViews := make([]CheckoutLineView, 0, len(lines))
	for _, l := range lines {
		lineViews = append(lineViews, CheckoutLineView{
			ID:                 l.ID().String(),
			OfferID:            l.OfferID().String(),
			SKUID:              l.SKUID().String(),
			SellerID:           l.SellerID().String(),
			ProductName:        l.ProductName(),
			VariantDescription: l.VariantDescription(),
			UnitPrice:          toAmount(l.UnitPrice()),
			Quantity:           l.Quantity(),
			LineTotal:          toAmount(l.LineTotal()),
			ReservationID:      l.ReservationID().String(),
		})
	}

	sellers := c.SellerIDs()
	sellerIDs := make([]string, 0, len(sellers))
	for _, id := range sellers {
		sellerIDs = append(sellerIDs, id.String())
	}

	a := c.ShippingAddress()

	return &CheckoutView{
		ID:         c.ID().String(),
		CartID:     c.CartID().String(),
		CustomerID: c.CustomerID().String(),
		GuestEmail: c.GuestEmail(),
		Currency:   string(c.Currency()),
		Status:     string(c.Status()),
		ShippingAddress: AddressInput{
			RecipientName: a.RecipientName,
			Phone:         a.Phone,
			StreetAddress: a.StreetAddress,
			Ward:          a.Ward,
			District:      a.District,
			Province:      a.Province,
			CountryCode:   a.CountryCode,
		},
		ShippingMethod: c.ShippingMethod(),
		Lines:          lineViews,
		Subtotal:       toAmount(c.Subtotal()),
		ShippingFee:    toAmount(c.ShippingFee()),
		DiscountAmount: toAmount(c.DiscountAmount()),
		TaxAmount:      toAmount(c.TaxAmount()),
		Total:          toAmount(c.Total()),
		CouponCode:     c.CouponCode(),
		SellerIDs:      sellerIDs,
		ExpiresAt:      formatTime(c.ExpiresAt()),
		SecondsLeft:    int(c.TimeLeft(now) / time.Second),
		ExtendedTimes:  c.ExtendedTimes(),
		OrderID:        c.OrderID().String(),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrExpired):
		return ErrExpired
	case errors.Is(err, application.ErrOutOfStock):
		return ErrOutOfStock
	case errors.Is(err, domain.ErrNoAddress):
		return ErrNoAddress
	case errors.Is(err, domain.ErrTooManyExtends):
		return ErrTooManyExtends
	case errors.Is(err, domain.ErrInvalidStatus),
		errors.Is(err, domain.ErrAlreadyComplete):
		return ErrInvalidStatus
	case errors.Is(err, application.ErrEmptyCart),
		errors.Is(err, domain.ErrNoLines),
		errors.Is(err, domain.ErrNoCustomer),
		errors.Is(err, domain.ErrMissingIdemKey):
		return ErrInvalidInput
	}
	return err
}

// promotionAdapter nối cổng ra của checkout với module promotion.
//
// Đây là chỗ DUY NHẤT trong module biết `promotion` tồn tại — tầng
// application chỉ thấy PromotionPort của chính nó.
type promotionAdapter struct{ api promotion.API }

var _ application.PromotionPort = (*promotionAdapter)(nil)

func (a *promotionAdapter) ValidateCoupon(
	ctx context.Context, code, customerID string, orderTotal money.Money,
) (money.Money, bool, error) {
	res, err := a.api.ValidateCoupon(ctx, promotion.ValidateRequest{
		Code:       code,
		CustomerID: customerID,
		OrderTotal: orderTotal.Amount(),
		Currency:   string(orderTotal.Currency()),
	})
	if err != nil {
		return money.Money{}, false, err
	}

	discount, err := money.New(res.Discount, money.Currency(res.Currency))
	if err != nil {
		return money.Money{}, false, err
	}
	return discount, res.FreeShipping, nil
}
