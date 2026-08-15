package order

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/order/application"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
	orderpg "github.com/fashion-commerce/platform/internal/modules/order/infrastructure/postgres"
	orderhttp "github.com/fashion-commerce/platform/internal/modules/order/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/database"
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
	// Hai bất biến quan trọng nhất của module — mã đơn duy nhất và khóa
	// idempotency duy nhất — được cưỡng chế bằng SEQUENCE và ràng buộc
	// UNIQUE. Một bản in-memory chỉ kiểm tra trước khi ghi, mà kiểm tra
	// trước khi ghi thì hai request song song vẫn cùng lọt. Với đơn hàng,
	// lọt nghĩa là khách bị trừ tiền hai lần.
	Storage string

	DB *database.DB

	// Audit là nơi ghi nhật ký thao tác quản trị (xem chi tiết đơn, hủy đơn).
	//
	// Thiếu nó thì luồng đặt hàng của khách vẫn chạy, nhưng endpoint quản
	// trị trả lỗi thay vì đọc dữ liệu khách không để lại dấu vết.
	Audit *audit.Recorder
	Clock application.Clock
}

// New khởi tạo module order.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"order: chỉ hỗ trợ kho lưu trữ postgres — mã đơn và khóa " +
				"idempotency cần ràng buộc UNIQUE ở tầng database")
	}
	if cfg.DB == nil {
		return nil, errors.New("order: bắt buộc phải có kết nối database")
	}

	pool := cfg.DB.Pool()
	deps := application.Deps{
		Orders:  orderpg.NewOrderStore(pool),
		Numbers: orderpg.NewNumberStore(pool),
		Clock:   cfg.Clock,
	}
	if cfg.Audit != nil {
		deps.Audit = NewAuditRecorder(cfg.Audit)
	}

	return &Module{svc: application.NewService(deps)}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- Quản trị

func (m *Module) ListOrders(ctx context.Context, f ListFilter) ([]OrderSummary, error) {
	filter := domain.Filter{
		OrderNumber: f.OrderNumber,
		Status:      f.Status,
		Limit:       f.Limit,
		Offset:      f.Offset,
	}
	if f.CustomerID != "" {
		filter.CustomerID = ids.ID(f.CustomerID)
	}

	orders, err := m.svc.ListOrders(ctx, filter)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]OrderSummary, 0, len(orders))
	for _, o := range orders {
		out = append(out, OrderSummary{
			ID:          o.ID().String(),
			OrderNumber: o.OrderNumber(),
			Status:      string(o.Status()),
			Total:       toAmount(o.Total()),
			LineCount:   len(o.Lines()),
			PlacedAt:    o.PlacedAt().UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (m *Module) ViewOrderAsAdmin(
	ctx context.Context, req ViewOrderRequest,
) (*OrderView, error) {
	id, err := ids.Parse(req.OrderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}

	o, err := m.svc.ViewOrderAsAdmin(ctx, application.ViewOrderInput{
		OrderID:   id,
		ActorID:   req.ActorID,
		Reason:    req.Reason,
		RequestID: req.RequestID,
	})
	if err != nil {
		return nil, translateErr(err)
	}
	v := toOrderView(o)
	return &v, nil
}

func (m *Module) CancelOrderAsAdmin(
	ctx context.Context, req CancelOrderRequest,
) (*OrderView, error) {
	id, err := ids.Parse(req.OrderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}

	o, err := m.svc.CancelOrderAsAdmin(ctx, application.CancelOrderInput{
		OrderID:   id,
		ActorID:   req.ActorID,
		Reason:    req.Reason,
		RequestID: req.RequestID,
	})
	if err != nil {
		return nil, translateErr(err)
	}
	v := toOrderView(o)
	return &v, nil
}

// RegisterAdminRoutes gắn các endpoint đơn hàng của quản trị vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth và RequireRole. Gắn nhầm vào mux công khai
// nghĩa là bất kỳ ai cũng đọc được địa chỉ và số điện thoại của mọi khách.
func (m *Module) RegisterAdminRoutes(mux *http.ServeMux, log *slog.Logger) {
	orderhttp.NewHandler(m.svc, log).Register(mux)
}

// ---------------------------------------------------------------- Khách hàng

func (m *Module) PlaceOrder(
	ctx context.Context, req PlaceOrderRequest,
) (*PlaceOrderResult, error) {
	in := application.PlaceOrderInput{
		GuestEmail:      strings.TrimSpace(req.GuestEmail),
		GuestPhone:      strings.TrimSpace(req.GuestPhone),
		ShippingAddress: toAddress(req.ShippingAddress),
		BillingAddress:  toAddress(req.BillingAddress),
		IdempotencyKey:  strings.TrimSpace(req.IdempotencyKey),
	}

	if req.CustomerID != "" {
		id, err := ids.Parse(req.CustomerID, ids.PrefixCustomer)
		if err != nil {
			return nil, ErrInvalidID
		}
		in.CustomerID = id
	}

	currency := money.Currency(req.Currency)
	if currency == "" && len(req.Lines) > 0 {
		currency = money.Currency(req.Lines[0].UnitPrice.Currency)
	}
	in.Currency = currency

	var err error
	if in.ShippingFee, err = toMoneyOrZero(req.ShippingFee, currency); err != nil {
		return nil, err
	}
	if in.DiscountAmount, err = toMoneyOrZero(req.DiscountAmount, currency); err != nil {
		return nil, err
	}
	if in.TaxAmount, err = toMoneyOrZero(req.TaxAmount, currency); err != nil {
		return nil, err
	}

	in.Lines = make([]application.PlaceOrderLine, 0, len(req.Lines))
	for _, src := range req.Lines {
		line, err := toPlaceOrderLine(src, currency)
		if err != nil {
			return nil, err
		}
		in.Lines = append(in.Lines, line)
	}

	res, err := m.svc.PlaceOrder(ctx, in)
	if err != nil {
		return nil, translateErr(err)
	}

	return &PlaceOrderResult{
		Order:    toOrderView(res.Order),
		Replayed: res.Replayed,
	}, nil
}

func toPlaceOrderLine(
	src PlaceOrderLineInput, currency money.Currency,
) (application.PlaceOrderLine, error) {
	var out application.PlaceOrderLine

	offerID, err := ids.Parse(src.OfferID, ids.PrefixOffer)
	if err != nil {
		return out, ErrInvalidID
	}
	sellerID, err := ids.Parse(src.SellerID, ids.PrefixSeller)
	if err != nil {
		return out, ErrInvalidID
	}

	unitPrice, err := toMoney(src.UnitPrice, currency)
	if err != nil {
		return out, err
	}
	rate, err := types.NewBasisPoints(int32(src.CommissionRate))
	if err != nil {
		return out, ErrInvalidInput
	}

	out = application.PlaceOrderLine{
		OfferID:            offerID,
		SellerID:           sellerID,
		ProductName:        src.ProductName,
		VariantDescription: src.VariantDescription,
		UnitPrice:          unitPrice,
		Quantity:           src.Quantity,
		CommissionRate:     rate,
	}

	if src.SKUID != "" {
		skuID, err := ids.Parse(src.SKUID, ids.PrefixSKU)
		if err != nil {
			return out, ErrInvalidID
		}
		out.SKUID = skuID
	}
	if src.AttributedCreatorID != "" {
		creatorID, err := ids.Parse(src.AttributedCreatorID, ids.PrefixCreator)
		if err != nil {
			return out, ErrInvalidID
		}
		out.AttributedCreatorID = creatorID

		creatorRate, err := types.NewBasisPoints(int32(src.CreatorCommissionRate))
		if err != nil {
			return out, ErrInvalidInput
		}
		out.CreatorCommissionRate = creatorRate
	}

	for _, a := range src.Adjustments {
		amount, err := toMoney(a.Amount, currency)
		if err != nil {
			return out, err
		}
		adj, err := domain.NewAdjustment(
			domain.AdjustmentType(a.Type), a.Label, amount,
			a.SourceType, ids.ID(a.SourceID),
			domain.CostBearer(a.CostBearer), time.Time{})
		if err != nil {
			return out, ErrInvalidInput
		}
		out.Adjustments = append(out.Adjustments, adj)
	}
	return out, nil
}

func (m *Module) GetOrder(ctx context.Context, orderID string) (*OrderView, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	o, err := m.svc.GetOrder(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toOrderView(o)
	return &v, nil
}

func (m *Module) GetOrderByNumber(ctx context.Context, number string) (*OrderView, error) {
	o, err := m.svc.GetOrderByNumber(ctx, strings.TrimSpace(number))
	if err != nil {
		return nil, translateErr(err)
	}
	v := toOrderView(o)
	return &v, nil
}

func (m *Module) ListCustomerOrders(
	ctx context.Context, customerID string, limit, offset int,
) ([]OrderView, error) {
	id, err := ids.Parse(customerID, ids.PrefixCustomer)
	if err != nil {
		return nil, ErrInvalidID
	}
	orders, err := m.svc.ListCustomerOrders(ctx, id, limit, offset)
	if err != nil {
		return nil, translateErr(err)
	}
	out := make([]OrderView, 0, len(orders))
	for _, o := range orders {
		out = append(out, toOrderView(o))
	}
	return out, nil
}

func (m *Module) MarkOrderPaid(ctx context.Context, orderID string) error {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.MarkPaid(ctx, id))
}

// ApplyFulfillmentProgress tính lại trạng thái tổng hợp của đơn.
//
// Gọi bởi bên nhận event từ module fulfillment. Module này KHÔNG hỏi ngược
// fulfillment — hỏi ngược tạo phụ thuộc vòng (ADR-0007).
func (m *Module) ApplyFulfillmentProgress(
	ctx context.Context, orderID string, progress []FulfillmentProgressInput,
) error {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return ErrInvalidID
	}

	out := make([]domain.FulfillmentProgress, 0, len(progress))
	for _, p := range progress {
		out = append(out, domain.FulfillmentProgress{
			Cancelled: p.Cancelled,
			Delivered: p.Delivered,
			Shipped:   p.Shipped,
		})
	}

	return translateErr(m.svc.ApplyFulfillmentProgress(ctx, id, out))
}

func (m *Module) CancelOrder(ctx context.Context, orderID, reason string) error {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.CancelOrder(ctx, id, strings.TrimSpace(reason)))
}

// ---------------------------------------------------------------- Chuyển đổi

func toAddress(a AddressInput) domain.Address {
	return domain.Address{
		RecipientName: strings.TrimSpace(a.RecipientName),
		Phone:         strings.TrimSpace(a.Phone),
		StreetAddress: strings.TrimSpace(a.StreetAddress),
		Ward:          strings.TrimSpace(a.Ward),
		District:      strings.TrimSpace(a.District),
		Province:      strings.TrimSpace(a.Province),
		CountryCode:   strings.TrimSpace(a.CountryCode),
	}
}

func fromAddress(a domain.Address) AddressInput {
	return AddressInput{
		RecipientName: a.RecipientName,
		Phone:         a.Phone,
		StreetAddress: a.StreetAddress,
		Ward:          a.Ward,
		District:      a.District,
		Province:      a.Province,
		CountryCode:   a.CountryCode,
	}
}

func toMoney(a Amount, fallback money.Currency) (money.Money, error) {
	cur := money.Currency(a.Currency)
	if cur == "" {
		cur = fallback
	}
	m, err := money.New(a.Value, cur)
	if err != nil {
		return money.Money{}, ErrInvalidInput
	}
	return m, nil
}

func toMoneyOrZero(a Amount, fallback money.Currency) (money.Money, error) {
	if a.Value == 0 {
		return money.Zero(fallback), nil
	}
	return toMoney(a, fallback)
}

func toAmount(m money.Money) Amount {
	return Amount{Value: m.Amount(), Currency: string(m.Currency())}
}

func toOrderView(o *domain.Order) OrderView {
	lines := o.Lines()
	views := make([]OrderLineView, 0, len(lines))
	for _, l := range lines {
		views = append(views, toLineView(l))
	}

	return OrderView{
		ID:              o.ID().String(),
		OrderNumber:     o.OrderNumber(),
		CustomerID:      o.CustomerID().String(),
		GuestEmail:      o.GuestEmail(),
		GuestPhone:      o.GuestPhone(),
		ShippingAddress: fromAddress(o.ShippingAddress()),
		Status:          string(o.Status()),
		Lines:           views,
		Subtotal:        toAmount(o.Subtotal()),
		ShippingFee:     toAmount(o.ShippingFee()),
		DiscountAmount:  toAmount(o.DiscountAmount()),
		TaxAmount:       toAmount(o.TaxAmount()),
		Total:           toAmount(o.Total()),
		PlacedAt:        formatTime(o.PlacedAt()),
		CompletedAt:     formatTime(o.CompletedAt()),
	}
}

func toLineView(l *domain.Line) OrderLineView {
	adjs := l.Adjustments()
	adjViews := make([]AdjustmentView, 0, len(adjs))
	for _, a := range adjs {
		adjViews = append(adjViews, AdjustmentView{
			ID:         a.ID.String(),
			Type:       string(a.Type),
			Label:      a.Label,
			Amount:     toAmount(a.Amount),
			CostBearer: string(a.CostBearer),
		})
	}

	return OrderLineView{
		ID:                 l.ID().String(),
		OfferID:            l.OfferID().String(),
		SKUID:              l.SKUID().String(),
		SellerID:           l.SellerID().String(),
		ProductName:        l.ProductName(),
		VariantDescription: l.VariantDescription(),
		UnitPrice:          toAmount(l.UnitPrice()),
		Quantity:           l.Quantity(),
		LineTotal:          toAmount(l.LineTotal()),
		CommissionRate:     int(l.CommissionRate().Value()),
		CommissionAmount:   toAmount(l.CommissionAmount()),
		SellerPayable:      toAmount(l.SellerPayable()),
		Status:             string(l.Status()),
		Adjustments:        adjViews,
	}
}

// formatTime trả chuỗi rỗng cho mốc thời gian chưa xảy ra.
//
// Trả về "0001-01-01T00:00:00Z" sẽ khiến giao diện hiện một ngày vô nghĩa —
// chuỗi rỗng nói đúng điều đang xảy ra: chưa có.
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
	case errors.Is(err, domain.ErrInvalidStatus):
		return ErrInvalidStatus
	case errors.Is(err, domain.ErrNotCancellable):
		return ErrNotCancellable
	case errors.Is(err, domain.ErrMissingIdempKey),
		errors.Is(err, domain.ErrNoCustomer),
		errors.Is(err, domain.ErrNoLines):
		return ErrInvalidInput
	}
	return err
}
