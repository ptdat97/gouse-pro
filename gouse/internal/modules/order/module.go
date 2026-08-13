package order

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/order/application"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
	orderpg "github.com/fashion-commerce/platform/internal/modules/order/infrastructure/postgres"
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

	DB    *database.DB
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
	return &Module{svc: application.NewService(application.Deps{
		Orders:  orderpg.NewOrderStore(pool),
		FOs:     orderpg.NewFulfillmentStore(pool),
		Numbers: orderpg.NewNumberStore(pool),
		Clock:   cfg.Clock,
	})}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

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
		Order:        toOrderView(res.Order),
		Fulfillments: toFulfillmentViews(res.FulfillmentOrders),
		Replayed:     res.Replayed,
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

func (m *Module) GetOrderFulfillments(
	ctx context.Context, orderID string,
) ([]FulfillmentView, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	fos, err := m.svc.ListOrderFulfillments(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return toFulfillmentViews(fos), nil
}

func (m *Module) MarkOrderPaid(ctx context.Context, orderID string) error {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.MarkPaid(ctx, id))
}

func (m *Module) CancelOrder(ctx context.Context, orderID, reason string) error {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.CancelOrder(ctx, id, strings.TrimSpace(reason)))
}

// ---------------------------------------------------------------- Nhà bán

func (m *Module) ListSellerFulfillments(
	ctx context.Context, sellerID string, statuses []string, limit, offset int,
) ([]FulfillmentView, error) {
	id, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return nil, ErrInvalidID
	}

	filter := make([]domain.FOStatus, 0, len(statuses))
	for _, st := range statuses {
		filter = append(filter, domain.FOStatus(st))
	}

	fos, err := m.svc.ListSellerWork(ctx, id, filter, limit, offset)
	if err != nil {
		return nil, translateErr(err)
	}
	return toFulfillmentViews(fos), nil
}

func (m *Module) GetSellerFulfillment(
	ctx context.Context, sellerID, fulfillmentID string,
) (*FulfillmentView, error) {
	sid, fid, err := parseSellerAndFO(sellerID, fulfillmentID)
	if err != nil {
		return nil, err
	}
	fo, err := m.svc.GetSellerFulfillment(ctx, sid, fid)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toFulfillmentView(fo)
	return &v, nil
}

func (m *Module) ConfirmFulfillment(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.sellerStep(ctx, sellerID, fulfillmentID, m.svc.ConfirmFulfillment)
}

func (m *Module) PackFulfillment(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.sellerStep(ctx, sellerID, fulfillmentID, m.svc.PackFulfillment)
}

func (m *Module) ShipFulfillment(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.sellerStep(ctx, sellerID, fulfillmentID, m.svc.ShipFulfillment)
}

func (m *Module) DeliverFulfillment(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.sellerStep(ctx, sellerID, fulfillmentID, m.svc.DeliverFulfillment)
}

func (m *Module) CancelFulfillment(
	ctx context.Context, sellerID, fulfillmentID, reason string,
) error {
	sid, fid, err := parseSellerAndFO(sellerID, fulfillmentID)
	if err != nil {
		return err
	}
	return translateErr(m.svc.CancelFulfillment(ctx, sid, fid, strings.TrimSpace(reason)))
}

func (m *Module) sellerStep(
	ctx context.Context, sellerID, fulfillmentID string,
	step func(context.Context, ids.ID, ids.ID) error,
) error {
	sid, fid, err := parseSellerAndFO(sellerID, fulfillmentID)
	if err != nil {
		return err
	}
	return translateErr(step(ctx, sid, fid))
}

func parseSellerAndFO(sellerID, fulfillmentID string) (ids.ID, ids.ID, error) {
	sid, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return "", "", ErrInvalidID
	}
	fid, err := ids.Parse(fulfillmentID, ids.PrefixFulfillmentOrder)
	if err != nil {
		return "", "", ErrInvalidID
	}
	return sid, fid, nil
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

func toFulfillmentViews(fos []*domain.FulfillmentOrder) []FulfillmentView {
	out := make([]FulfillmentView, 0, len(fos))
	for _, fo := range fos {
		out = append(out, toFulfillmentView(fo))
	}
	return out
}

func toFulfillmentView(fo *domain.FulfillmentOrder) FulfillmentView {
	lineIDs := fo.LineIDs()
	strIDs := make([]string, 0, len(lineIDs))
	for _, id := range lineIDs {
		strIDs = append(strIDs, id.String())
	}

	return FulfillmentView{
		ID:               fo.ID().String(),
		OrderID:          fo.OrderID().String(),
		FONumber:         fo.FONumber(),
		SellerID:         fo.SellerID().String(),
		Status:           string(fo.Status()),
		LineIDs:          strIDs,
		Subtotal:         toAmount(fo.Subtotal()),
		CommissionAmount: toAmount(fo.CommissionAmount()),
		SellerPayable:    toAmount(fo.SellerPayable()),
		CancelReason:     fo.CancelReason(),
		ConfirmedAt:      formatTime(fo.ConfirmedAt()),
		PackedAt:         formatTime(fo.PackedAt()),
		ShippedAt:        formatTime(fo.ShippedAt()),
		DeliveredAt:      formatTime(fo.DeliveredAt()),
		CancelledAt:      formatTime(fo.CancelledAt()),
		CreatedAt:        formatTime(fo.CreatedAt()),
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
	case errors.Is(err, application.ErrForbidden):
		return ErrForbidden
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
