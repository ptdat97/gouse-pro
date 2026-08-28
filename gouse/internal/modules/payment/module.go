package payment

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
	paymentpg "github.com/fashion-commerce/platform/internal/modules/payment/infrastructure/postgres"
	paymenthttp "github.com/fashion-commerce/platform/internal/modules/payment/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/audit"
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
	// Sổ cái bất biến cần trigger ở tầng database để chặn UPDATE/DELETE.
	// Một bản in-memory chỉ "không cung cấp phương thức sửa" — với sổ sách
	// tài chính, khác biệt đó là quá lớn để chấp nhận.
	Storage string

	DB    *database.DB
	Clock application.Clock

	// Audit là nơi ghi nhật ký thao tác nhạy cảm.
	//
	// Thiếu nó thì module vẫn khởi tạo được (ghi sổ tự động không cần),
	// nhưng CreateLedgerAdjustment sẽ trả lỗi thay vì âm thầm bỏ qua.
	Audit *audit.Recorder
}

// New khởi tạo module payment.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"payment: chỉ hỗ trợ kho lưu trữ postgres — sổ cái bất biến cần " +
				"trigger ở tầng database")
	}
	if cfg.DB == nil {
		return nil, errors.New("payment: bắt buộc phải có kết nối database")
	}

	pool := cfg.DB.Pool()
	deps := application.Deps{
		Ledger:   paymentpg.NewLedgerStore(pool),
		Balances: paymentpg.NewBalanceStore(pool),
		Clock:    cfg.Clock,
	}
	if cfg.Audit != nil {
		deps.Audit = NewAuditRecorder(cfg.Audit)
	}

	return &Module{svc: application.NewService(deps)}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// RegisterAdminRoutes gắn các endpoint tài chính của quản trị vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth, RequireRole("ADMIN", "OPS_FINANCE") và
// RequireIdempotencyKey. Gắn nhầm vào mux công khai nghĩa là bất kỳ ai cũng
// ghi được bút toán vào sổ cái.
// RegisterSellerRoutes gắn endpoint số dư của NHÀ BÁN.
//
// Bên gọi PHẢI bọc Auth và RequireRole("SELLER_OWNER", "SELLER_STAFF").
func (m *Module) RegisterSellerRoutes(mux *http.ServeMux, log *slog.Logger) {
	paymenthttp.NewSellerHandler(m.svc, log).Register(mux)
}

func (m *Module) RegisterAdminRoutes(mux *http.ServeMux, log *slog.Logger) {
	paymenthttp.NewHandler(m.svc, log).Register(mux)
}

// ---------------------------------------------------------------- API

func (m *Module) RecordOrderRevenue(
	ctx context.Context, req OrderRevenueRequest,
) (*EntryView, error) {
	in, err := toRevenueInput(req)
	if err != nil {
		return nil, err
	}
	entry, err := m.svc.RecordOrderRevenue(ctx, in)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toEntryView(entry)
	return &v, nil
}

// RecordOrderRevenueInEventTx ghi doanh thu bằng GIAO DỊCH của dispatcher.
//
// Ngữ cảnh PHẢI mang giao dịch do eventbus mở — cùng ràng buộc và cùng lý
// do với CommitInEventTx của inventory: ghi sổ thành công mà đánh dấu
// event thất bại thì lần thử lại ghi doanh thu LẦN THỨ HAI, và sổ cái là
// chỗ không được phép đếm hai lần.
func (m *Module) RecordOrderRevenueInEventTx(
	ctx context.Context, req OrderRevenueRequest,
) error {
	in, err := toRevenueInput(req)
	if err != nil {
		return err
	}

	tx, err := eventbus.MustTxFrom(ctx)
	if err != nil {
		return err
	}

	_, err = m.svc.RecordOrderRevenueWith(ctx, paymentpg.LedgerForTx(tx), in)
	return translateErr(err)
}

// ChuyenSangRutDuocInEventTx chuyển tiền nhà bán sang rút được, bằng GIAO
// DỊCH của dispatcher.
//
// Cùng ràng buộc với RecordOrderRevenueInEventTx: ghi sổ thành công mà
// đánh dấu event thất bại thì lần thử lại chuyển LẦN THỨ HAI, và nhà bán
// rút được gấp đôi số tiền thật.
func (m *Module) ChuyenSangRutDuocInEventTx(
	ctx context.Context, req SellerReleaseRequest,
) error {
	foID, err := ids.Parse(req.FulfillmentID, ids.PrefixFulfillmentOrder)
	if err != nil {
		return ErrInvalidID
	}
	sellerID, err := ids.Parse(req.SellerID, ids.PrefixSeller)
	if err != nil {
		return ErrInvalidID
	}
	amount, err := toMoney(req.Amount)
	if err != nil {
		return err
	}

	tx, err := eventbus.MustTxFrom(ctx)
	if err != nil {
		return err
	}

	return translateErr(m.svc.ChuyenSangRutDuocWith(ctx, paymentpg.LedgerForTx(tx),
		application.ChuyenSangRutDuocInput{
			FulfillmentID: foID, SellerID: sellerID, Amount: amount,
			CreatedBy: req.CreatedBy,
		}))
}

// toRevenueInput chuyển yêu cầu công khai thành đầu vào của tầng ứng dụng.
func toRevenueInput(req OrderRevenueRequest) (application.RecordOrderRevenueInput, error) {
	var zero application.RecordOrderRevenueInput

	orderID, err := ids.Parse(req.OrderID, ids.PrefixOrder)
	if err != nil {
		return zero, ErrInvalidID
	}

	gross, err := toMoney(req.GrossAmount)
	if err != nil {
		return zero, err
	}

	in := application.RecordOrderRevenueInput{
		OrderID:     orderID,
		GrossAmount: gross,
		CreatedBy:   req.CreatedBy,
	}

	if req.SellerID != "" {
		sellerID, err := ids.Parse(req.SellerID, ids.PrefixSeller)
		if err != nil {
			return zero, ErrInvalidID
		}
		in.SellerID = sellerID
		if in.SellerPayable, err = toMoneyOrZero(req.SellerPayable, gross.Currency()); err != nil {
			return zero, err
		}
	}
	if req.CreatorID != "" {
		creatorID, err := ids.Parse(req.CreatorID, ids.PrefixCreator)
		if err != nil {
			return zero, ErrInvalidID
		}
		in.CreatorID = creatorID
		if in.CreatorPayable, err = toMoneyOrZero(req.CreatorPayable, gross.Currency()); err != nil {
			return zero, err
		}
	}

	for _, f := range []struct {
		src Amount
		dst *money.Money
	}{
		{req.PlatformRevenue, &in.PlatformRevenue},
		{req.PaymentFee, &in.PaymentFee},
		{req.COGS, &in.COGS},
	} {
		v, err := toMoneyOrZero(f.src, gross.Currency())
		if err != nil {
			return zero, err
		}
		*f.dst = v
	}

	return in, nil
}

func (m *Module) RecordRefund(ctx context.Context, req RefundRequest) (*EntryView, error) {
	orderID, err := ids.Parse(req.OrderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	amount, err := toMoney(req.Amount)
	if err != nil {
		return nil, err
	}

	in := application.RecordRefundInput{
		OrderID:   orderID,
		Amount:    amount,
		CreatedBy: req.CreatedBy,
	}
	if req.RefundID != "" {
		in.RefundID = ids.ID(req.RefundID)
	}
	if req.SellerID != "" {
		sellerID, err := ids.Parse(req.SellerID, ids.PrefixSeller)
		if err != nil {
			return nil, ErrInvalidID
		}
		in.SellerID = sellerID
	}
	if in.SellerClawback, err = toMoneyOrZero(req.SellerClawback, amount.Currency()); err != nil {
		return nil, err
	}
	if in.PlatformClawback, err = toMoneyOrZero(req.PlatformClawback, amount.Currency()); err != nil {
		return nil, err
	}

	entry, err := m.svc.RecordRefund(ctx, in)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toEntryView(entry)
	return &v, nil
}

func (m *Module) RecordPayout(ctx context.Context, req PayoutRequest) (*EntryView, error) {
	payoutID, err := ids.Parse(req.PayoutID, ids.PrefixPayout)
	if err != nil {
		return nil, ErrInvalidID
	}
	sellerID, err := ids.Parse(req.SellerID, ids.PrefixSeller)
	if err != nil {
		return nil, ErrInvalidID
	}
	amount, err := toMoney(req.Amount)
	if err != nil {
		return nil, err
	}

	entry, err := m.svc.RecordPayout(ctx, payoutID, sellerID, amount, req.CreatedBy)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toEntryView(entry)
	return &v, nil
}

func (m *Module) CreateLedgerAdjustment(
	ctx context.Context, req AdjustmentRequest,
) (*EntryView, error) {
	if !ids.IsValid(req.ReferenceID) {
		return nil, ErrInvalidID
	}

	lines := make([]domain.Line, 0, len(req.Lines))
	for _, l := range req.Lines {
		amount, err := money.New(l.Amount, money.Currency(l.Currency))
		if err != nil {
			return nil, errors.New("payment: số tiền không hợp lệ: " + l.Currency)
		}
		if l.OwnerID != "" && !ids.IsValid(l.OwnerID) {
			return nil, ErrInvalidID
		}
		lines = append(lines, domain.Line{
			Account: domain.Account{
				Type:    domain.AccountType(l.AccountType),
				OwnerID: ids.ID(l.OwnerID),
			},
			Direction:   domain.Direction(l.Direction),
			Amount:      amount,
			Description: l.Description,
		})
	}

	entry, err := m.svc.RecordAdjustmentWithAudit(ctx, application.AdjustmentInput{
		ReferenceType:  req.ReferenceType,
		ReferenceID:    ids.ID(req.ReferenceID),
		Lines:          lines,
		ActorID:        req.ActorID,
		Reason:         req.Reason,
		IdempotencyKey: req.IdempotencyKey,
		RequestID:      req.RequestID,
	})
	if err != nil {
		return nil, translateErr(err)
	}

	v := toEntryView(entry)
	return &v, nil
}

func (m *Module) GetSellerBalance(ctx context.Context, sellerID string) (*BalanceView, error) {
	id, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return nil, ErrInvalidID
	}
	b, err := m.svc.GetSellerBalance(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toBalanceView(b)
	return &v, nil
}

func (m *Module) GetPlatformBalance(ctx context.Context, accountType string) (*BalanceView, error) {
	b, err := m.svc.GetBalance(ctx, domain.Account{Type: domain.AccountType(accountType)})
	if err != nil {
		return nil, translateErr(err)
	}
	v := toBalanceView(b)
	return &v, nil
}

func (m *Module) GetEntriesForOrder(ctx context.Context, orderID string) ([]EntryView, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	entries, err := m.svc.GetEntriesForOrder(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]EntryView, 0, len(entries))
	for _, e := range entries {
		out = append(out, toEntryView(e))
	}
	return out, nil
}

func (m *Module) CheckIntegrity(ctx context.Context) (*IntegrityView, error) {
	// Khoảng rỗng = kiểm tra toàn bộ.
	check, err := m.svc.CheckIntegrity(ctx, time.Time{}, time.Time{})
	if err != nil {
		return nil, translateErr(err)
	}
	return &IntegrityView{
		TotalDebit:         check.TotalDebit,
		TotalCredit:        check.TotalCredit,
		Difference:         check.Difference,
		UnbalancedEntryIDs: check.UnbalancedEntries,
		CheckedEntries:     check.CheckedEntries,
		IsHealthy:          check.IsHealthy(),
	}, nil
}

// ---------------------------------------------------------------- Chuyển đổi

func toMoney(a Amount) (money.Money, error) {
	m, err := money.New(a.Value, money.Currency(a.Currency))
	if err != nil {
		return money.Money{}, errors.New("payment: số tiền không hợp lệ: " + a.Currency)
	}
	return m, nil
}

// toMoneyOrZero chuyển Amount sang Money, dùng đơn vị mặc định nếu để trống.
//
// Bên gọi thường chỉ điền Value cho các khoản phụ và bỏ trống Currency —
// mặc định theo đơn vị của tổng tiền là hành vi đúng, và constructor của
// bút toán vẫn chặn nếu trộn đơn vị.
func toMoneyOrZero(a Amount, fallback money.Currency) (money.Money, error) {
	if a.Value == 0 {
		return money.Money{}, nil
	}
	cur := money.Currency(a.Currency)
	if cur == "" {
		cur = fallback
	}
	m, err := money.New(a.Value, cur)
	if err != nil {
		return money.Money{}, errors.New("payment: số tiền không hợp lệ")
	}
	return m, nil
}

func toAmount(m money.Money) Amount {
	return Amount{Value: m.Amount(), Currency: string(m.Currency())}
}

func toEntryView(e *domain.LedgerEntry) EntryView {
	lines := e.Lines()
	out := make([]LineView, 0, len(lines))
	for _, l := range lines {
		out = append(out, LineView{
			AccountType:    string(l.Account.Type),
			AccountOwnerID: l.Account.OwnerID.String(),
			Direction:      string(l.Direction),
			Amount:         toAmount(l.Amount),
			Description:    l.Description,
		})
	}

	return EntryView{
		ID:            e.ID().String(),
		Type:          string(e.Type()),
		ReferenceType: e.ReferenceType(),
		ReferenceID:   e.ReferenceID().String(),
		Description:   e.Description(),
		Lines:         out,
		Total:         toAmount(e.Total()),
		CreatedAt:     e.CreatedAt().Format(time.RFC3339),
	}
}

func toBalanceView(b domain.Balance) BalanceView {
	return BalanceView{
		AccountType:    string(b.Account.Type),
		AccountOwnerID: b.Account.OwnerID.String(),
		Amount:         toAmount(b.Amount),
		TotalDebit:     toAmount(b.TotalDebit),
		TotalCredit:    toAmount(b.TotalCredit),
		EntryCount:     b.EntryCount,
	}
}

func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrUnbalanced):
		return ErrUnbalanced
	case errors.Is(err, application.ErrInsufficientBalance):
		return ErrInsufficientBalance
	}
	return err
}
