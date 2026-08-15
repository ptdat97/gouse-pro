package cart

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/cart/application"
	"github.com/fashion-commerce/platform/internal/modules/cart/domain"
	cartpg "github.com/fashion-commerce/platform/internal/modules/cart/infrastructure/postgres"
	carthttp "github.com/fashion-commerce/platform/internal/modules/cart/interfaces/http"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	"github.com/fashion-commerce/platform/internal/modules/product"
	"github.com/fashion-commerce/platform/internal/modules/seller"
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
	// Quy tắc 5 — mỗi khách một giỏ ACTIVE — được cưỡng chế bằng chỉ mục
	// UNIQUE CÓ ĐIỀU KIỆN. Kiểm tra ở tầng ứng dụng vẫn lọt khi khách mở
	// hai tab cùng lúc, và hai giỏ ACTIVE nghĩa là khách thêm hàng ở tab
	// này rồi thanh toán ở tab kia mà không thấy nó.
	Storage string

	DB *database.DB

	// Bốn module để đồng bộ giỏ. Thiếu chúng thì giỏ vẫn hoạt động nhưng
	// giá và tình trạng hàng KHÔNG được làm mới — chỉ chấp nhận được khi
	// chạy test tầng dưới.
	Marketplace marketplace.API
	Product     product.API
	Seller      seller.API
	Inventory   inventory.API

	Clock application.Clock

	// Events phát domain event. Nil nghĩa là KHÔNG phát tín hiệu nhu cầu.
	//
	// Ở production đây là mất mát KHÔNG BÙ ĐƯỢC: dữ liệu lịch sử không tạo
	// ngược được.
	Events *eventbus.Outbox
}

// New khởi tạo module cart.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"cart: chỉ hỗ trợ kho lưu trữ postgres — quy tắc một giỏ ACTIVE " +
				"mỗi khách cần chỉ mục UNIQUE có điều kiện")
	}
	if cfg.DB == nil {
		return nil, errors.New("cart: bắt buộc phải có kết nối database")
	}
	if cfg.Marketplace == nil {
		return nil, errors.New(
			"cart: bắt buộc phải có module marketplace — không có nó thì giỏ " +
				"không tra được giá, và giá là thứ khách nhìn vào để quyết định mua")
	}

	pool := cfg.DB.Pool()
	deps := application.Deps{
		Carts: cartpg.NewCartStore(pool),
		Offers: &offerLookup{
			marketplace: cfg.Marketplace,
			product:     cfg.Product,
			seller:      cfg.Seller,
			inventory:   cfg.Inventory,
		},
		Clock: cfg.Clock,
	}
	if cfg.Events != nil {
		deps.Events = NewEventPublisher(cfg.Events)
	}

	return &Module{svc: application.NewService(deps)}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// RegisterRoutes gắn các endpoint giỏ hàng vào mux.
//
// Bên gọi PHẢI bọc httpserver.ResolveShopper quanh mux này: handler lấy
// danh tính người mua từ context và từ chối phục vụ khi không có.
func (m *Module) RegisterRoutes(mux *http.ServeMux, log *slog.Logger) {
	carthttp.NewHandler(m.svc, log).Register(mux)
}

// ---------------------------------------------------------------- API

func (m *Module) GetOrCreateCart(
	ctx context.Context, req GetOrCreateRequest,
) (*CartView, error) {
	in := application.GetOrCreateInput{
		SessionID: strings.TrimSpace(req.SessionID),
		Currency:  money.Currency(req.Currency),
	}
	if req.CustomerID != "" {
		id, err := ids.Parse(req.CustomerID, ids.PrefixCustomer)
		if err != nil {
			return nil, ErrInvalidID
		}
		in.CustomerID = id
	}
	if in.CustomerID.IsZero() && in.SessionID == "" {
		return nil, ErrInvalidInput
	}

	c, err := m.svc.GetOrCreateCart(ctx, in)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toCartView(c)
	return &v, nil
}

func (m *Module) GetCart(ctx context.Context, cartID string) (*CartView, error) {
	id, err := ids.Parse(cartID, ids.PrefixCart)
	if err != nil {
		return nil, ErrInvalidID
	}
	c, err := m.svc.GetCart(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toCartView(c)
	return &v, nil
}

func (m *Module) AddItem(ctx context.Context, req AddItemRequest) (*CartView, error) {
	cartID, err := ids.Parse(req.CartID, ids.PrefixCart)
	if err != nil {
		return nil, ErrInvalidID
	}
	offerID, err := ids.Parse(req.OfferID, ids.PrefixOffer)
	if err != nil {
		return nil, ErrInvalidID
	}

	in := application.AddItemInput{
		CartID:   cartID,
		OfferID:  offerID,
		Quantity: req.Quantity,
	}
	// Nguồn giới thiệu hỏng thì BỎ QUA chứ không chặn việc thêm hàng: mất
	// một dòng dữ liệu quy kết còn hơn mất một lần khách mua.
	if id, err := ids.Parse(req.SourceContentID, ids.PrefixContent); err == nil {
		in.SourceContentID = id
	}
	if id, err := ids.Parse(req.SourceCreatorID, ids.PrefixCreator); err == nil {
		in.SourceCreatorID = id
	}

	c, err := m.svc.AddItem(ctx, in)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toCartView(c)
	return &v, nil
}

func (m *Module) UpdateItemQuantity(
	ctx context.Context, cartID, itemID string, quantity int,
) (*CartView, error) {
	cid, iid, err := parseCartAndItem(cartID, itemID)
	if err != nil {
		return nil, err
	}
	c, err := m.svc.UpdateQuantity(ctx, cid, iid, quantity)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toCartView(c)
	return &v, nil
}

func (m *Module) RemoveItem(ctx context.Context, cartID, itemID string) (*CartView, error) {
	cid, iid, err := parseCartAndItem(cartID, itemID)
	if err != nil {
		return nil, err
	}
	c, err := m.svc.RemoveItem(ctx, cid, iid)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toCartView(c)
	return &v, nil
}

func (m *Module) ClearCart(ctx context.Context, cartID string) error {
	id, err := ids.Parse(cartID, ids.PrefixCart)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.ClearCart(ctx, id))
}

func (m *Module) MergeOnLogin(
	ctx context.Context, customerID, sessionID string,
) (*MergeResult, error) {
	cid, err := ids.Parse(customerID, ids.PrefixCustomer)
	if err != nil {
		return nil, ErrInvalidID
	}

	res, err := m.svc.MergeOnLogin(ctx, cid, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, translateErr(err)
	}

	warnings := make([]MergeWarningView, 0, len(res.Warnings))
	for _, w := range res.Warnings {
		warnings = append(warnings, MergeWarningView{
			OfferID:     w.OfferID.String(),
			ProductName: w.ProductName,
			Reason:      string(w.Reason),
			WantedQty:   w.WantedQty,
			ActualQty:   w.ActualQty,
		})
	}
	return &MergeResult{Cart: toCartView(res.Cart), Warnings: warnings}, nil
}

func (m *Module) MarkConverted(ctx context.Context, cartID string) error {
	id, err := ids.Parse(cartID, ids.PrefixCart)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.MarkConverted(ctx, id))
}

func parseCartAndItem(cartID, itemID string) (ids.ID, ids.ID, error) {
	cid, err := ids.Parse(cartID, ids.PrefixCart)
	if err != nil {
		return "", "", ErrInvalidID
	}
	iid, err := ids.Parse(itemID, ids.PrefixCartItem)
	if err != nil {
		return "", "", ErrInvalidID
	}
	return cid, iid, nil
}

// ---------------------------------------------------------------- Chuyển đổi

func toAmount(m money.Money) Amount {
	return Amount{Value: m.Amount(), Currency: string(m.Currency())}
}

func toCartView(c *domain.Cart) CartView {
	items := c.Items()
	views := make([]CartItemView, 0, len(items))
	for _, it := range items {
		views = append(views, toItemView(it))
	}

	sellers := c.SellerIDs()
	sellerIDs := make([]string, 0, len(sellers))
	for _, id := range sellers {
		sellerIDs = append(sellerIDs, id.String())
	}

	return CartView{
		ID:                  c.ID().String(),
		CustomerID:          c.CustomerID().String(),
		SessionID:           c.SessionID(),
		Currency:            string(c.Currency()),
		Status:              string(c.Status()),
		Items:               views,
		Subtotal:            toAmount(c.Subtotal()),
		ItemCount:           c.ItemCount(),
		TotalQuantity:       c.TotalQuantity(),
		HasUnavailableItems: c.HasUnavailableItems(),
		SellerIDs:           sellerIDs,
		ExpiresAt:           formatTime(c.ExpiresAt()),
		UpdatedAt:           formatTime(c.UpdatedAt()),
	}
}

func toItemView(it *domain.Item) CartItemView {
	return CartItemView{
		ID:                 it.ID().String(),
		OfferID:            it.OfferID().String(),
		SKUID:              it.SKUID().String(),
		SellerID:           it.SellerID().String(),
		ProductName:        it.ProductName(),
		VariantDescription: it.VariantDescription(),
		ImageURL:           it.ImageURL(),
		UnitPrice:          toAmount(it.UnitPrice()),
		Quantity:           it.Quantity(),
		LineTotal:          toAmount(it.LineTotal()),
		MaxOrderQuantity:   it.MaxOrderQuantity(),
		Availability:       string(it.Availability()),
		AvailableQuantity:  it.AvailableQuantity(),
		SourceContentID:    it.SourceContentID().String(),
		SourceCreatorID:    it.SourceCreatorID().String(),
		AddedAt:            formatTime(it.AddedAt()),
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
	case errors.Is(err, domain.ErrCartNotActive):
		return ErrCartNotActive
	case errors.Is(err, domain.ErrQtyBelowMin),
		errors.Is(err, domain.ErrQtyAboveMax),
		errors.Is(err, domain.ErrInvalidQty):
		return ErrQuantityOutOfRange
	case errors.Is(err, domain.ErrItemNotInCart):
		return ErrNotFound
	case errors.Is(err, domain.ErrNoOwner),
		errors.Is(err, domain.ErrMixedCurrency):
		return ErrInvalidInput
	}
	return err
}
