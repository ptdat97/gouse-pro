package payment

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

var (
	// ErrMethodNotPrepaid: COD hoặc rỗng — không có intent.
	//
	// Bên gọi coi đây là "không có gì để làm", KHÔNG phải lỗi: đó là câu
	// trả lời đúng cho một đơn COD (ADR-0017 phần 2).
	ErrMethodNotPrepaid = domain.ErrMethodNotPrepaid

	// ErrAmountMismatch: số tiền webhook báo KHÁC số hệ thống chờ thu.
	//
	// Tầng HTTP PHẢI dịch thành 422 và không xử lý gì thêm.
	ErrAmountMismatch = domain.ErrAmountMismatch

	// ErrIntentNotFound: đơn này không có ý định thanh toán nào.
	//
	// Với webhook, điều đó nghĩa là nhà cung cấp báo về một đơn hệ thống
	// KHÔNG chờ thu tiền — đơn COD, đơn không tồn tại, hoặc môi trường
	// test gọi nhầm vào production. Cả ba đều không được xử lý.
	ErrIntentNotFound = domain.ErrIntentNotFound
)

// NewIntent tạo bản ghi số tiền CHỜ THU cho một đơn trả trước.
//
// IDEMPOTENT theo đơn. Xem ADR-0017.
func (m *Module) NewIntent(
	ctx context.Context, req NewIntentRequest,
) (*IntentView, error) {
	orderID, err := ids.Parse(req.OrderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	amount, err := money.New(req.Amount, money.Currency(req.Currency))
	if err != nil {
		return nil, err
	}

	p, err := m.svc.NewIntent(ctx, application.NewIntentInput{
		OrderID: orderID, Amount: amount, Method: req.Method,
	})
	if err != nil {
		return nil, err
	}
	return toIntentView(p), nil
}

// MatchAndCapture là LỚP BẢO VỆ THỨ BA của webhook thanh toán.
//
// Trả ErrAmountMismatch khi số tiền lệch — tầng HTTP dịch thành 422 và
// KHÔNG xử lý gì thêm.
func (m *Module) MatchAndCapture(
	ctx context.Context, orderID string, amount int64, currency string,
	provider, providerRef string,
) (*IntentView, bool, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, false, ErrInvalidID
	}
	m2, err := money.New(amount, money.Currency(currency))
	if err != nil {
		return nil, false, err
	}

	res, err := m.svc.MatchAndCapture(ctx, id, m2, provider, providerRef)
	if err != nil {
		return nil, false, err
	}
	return toIntentView(res.Intent), res.AlreadyCaptured, nil
}

// RecordFailure ghi nhận nhà cung cấp báo thanh toán thất bại.
func (m *Module) RecordFailure(
	ctx context.Context, orderID, reason string,
) (*IntentView, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	p, err := m.svc.RecordFailure(ctx, id, reason)
	if err != nil {
		return nil, err
	}
	return toIntentView(p), nil
}

// GetIntentByOrder đọc ý định thanh toán của một đơn.
func (m *Module) GetIntentByOrder(
	ctx context.Context, orderID string,
) (*IntentView, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	p, err := m.svc.GetIntentByOrder(ctx, id)
	if err != nil {
		return nil, err
	}
	return toIntentView(p), nil
}

func toIntentView(p *domain.PaymentIntent) *IntentView {
	v := &IntentView{
		ID:       p.ID().String(),
		OrderID:  p.OrderID().String(),
		Amount:   p.Amount().Amount(),
		Currency: string(p.Amount().Currency()),
		Method:   p.Method(),
		Status:   string(p.Status()),
	}
	if t := p.CapturedAt(); !t.IsZero() {
		v.CapturedAt = t.UTC().Format(time.RFC3339)
	}
	return v
}

// IsNotPrepaidMethod cho biết lỗi này là "COD, không cần intent".
//
// Bên gọi dùng nó để phân biệt "không có gì để làm" với "hỏng thật".
func IsNotPrepaidMethod(err error) bool {
	return errors.Is(err, domain.ErrMethodNotPrepaid)
}

// SettlementDiscrepancyView là một bất nhất giữa ý định thanh toán và đơn hàng.
type SettlementDiscrepancyView struct {
	IntentID    string
	OrderID     string
	Amount      int64
	Currency    string
	CapturedAt  string
	OrderStatus string
}

// ReconcileCaptured tìm đơn ĐÃ THU TIỀN mà trạng thái đơn chưa theo kịp.
//
// `orderStatus` do BÊN GỌI cấp — payment không gọi ngược order được.
// Xem `application.ReconcileCaptured`.
func (m *Module) ReconcileCaptured(
	ctx context.Context, since time.Time, limit int,
	orderStatus func(ctx context.Context, orderID string) (string, error),
) ([]SettlementDiscrepancyView, error) {
	discrepancies, err := m.svc.ReconcileCaptured(ctx, since, limit, orderStatus)
	if err != nil {
		return nil, err
	}
	out := make([]SettlementDiscrepancyView, 0, len(discrepancies))
	for _, l := range discrepancies {
		out = append(out, SettlementDiscrepancyView{
			IntentID: l.IntentID, OrderID: l.OrderID,
			Amount: l.Amount, Currency: l.Currency,
			CapturedAt:  l.CapturedAt.UTC().Format(time.RFC3339),
			OrderStatus: l.OrderStatus,
		})
	}
	return out, nil
}

// CountOverduePendingIntents đếm intent còn chờ thu và cũ hơn `truoc`.
func (m *Module) CountOverduePendingIntents(ctx context.Context, before time.Time) (int, error) {
	return m.svc.CountOverduePending(ctx, before)
}
