// Package application chứa các use case của module payment.
//
// Module này là NGUỒN SỰ THẬT DUY NHẤT về tiền. Không module nào khác được
// lưu số dư — muốn biết seller còn bao nhiêu thì gọi GetBalance().
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// Service là tầng application của module payment.
type Service struct {
	ledger   domain.LedgerRepository
	balances domain.BalanceRepository
	clock    Clock
}

type Deps struct {
	Ledger   domain.LedgerRepository
	Balances domain.BalanceRepository
	Clock    Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{ledger: d.Ledger, balances: d.Balances, clock: clock}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Ghi sổ

// RecordOrderRevenueInput là dữ liệu ghi sổ doanh thu một đơn hàng.
//
// Mọi số tiền ĐÃ ĐÓNG BĂNG ở module order — module này chỉ ghi sổ, không
// tính lại. Tính lại sẽ ra số khác khi chính sách hoa hồng đổi, và đối
// soát tháng trước sẽ sai.
type RecordOrderRevenueInput struct {
	OrderID         ids.ID
	GrossAmount     money.Money
	SellerID        ids.ID
	SellerPayable   money.Money
	PlatformRevenue money.Money
	CreatorID       ids.ID
	CreatorPayable  money.Money
	PaymentFee      money.Money

	// COGS chỉ có với đơn own brand. Bằng 0 với đơn marketplace: hàng
	// không phải tài sản của nền tảng.
	COGS money.Money

	CreatedBy string
}

// RecordOrderRevenue ghi sổ doanh thu một đơn hàng.
//
// IDEMPOTENT theo orderID: gọi lại trả về bút toán cũ, KHÔNG ghi thêm.
// Ghi hai lần cùng một đơn sẽ nhân đôi số tiền phải trả seller — loại lỗi
// tệ nhất có thể xảy ra ở module này.
func (s *Service) RecordOrderRevenue(
	ctx context.Context, in RecordOrderRevenueInput,
) (*domain.LedgerEntry, error) {
	key := "order-revenue:" + in.OrderID.String()

	// Đã ghi rồi thì trả kết quả cũ — đó là ý nghĩa của idempotent.
	if existing, err := s.ledger.FindByIdempotencyKey(ctx, key); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	now := s.clock.Now()
	entry, err := domain.NewOrderRevenueEntry(domain.OrderRevenueParams{
		OrderID:         in.OrderID,
		GrossAmount:     in.GrossAmount,
		SellerID:        in.SellerID,
		SellerPayable:   in.SellerPayable,
		PlatformRevenue: in.PlatformRevenue,
		CreatorID:       in.CreatorID,
		CreatorPayable:  in.CreatorPayable,
		PaymentFee:      in.PaymentFee,
		IdempotencyKey:  key,
		CreatedBy:       in.CreatedBy,
		Now:             now,
	})
	if err != nil {
		return nil, err
	}

	if err := s.ledger.Append(ctx, entry); err != nil {
		// Hai request đồng thời: bên kia vừa ghi xong. Đọc lại kết quả của
		// họ thay vì báo lỗi — kết quả cuối giống nhau.
		if errors.Is(err, domain.ErrDuplicateEntry) {
			return s.ledger.FindByIdempotencyKey(ctx, key)
		}
		return nil, err
	}

	// Đơn own brand ghi thêm giá vốn — bút toán RIÊNG, không gộp vào bút
	// toán doanh thu: hai sự kiện tài chính khác nhau.
	if in.COGS.IsPositive() {
		cogsKey := "order-cogs:" + in.OrderID.String()
		cogsEntry, err := domain.NewCOGSEntry(in.OrderID, in.COGS, cogsKey, in.CreatedBy, now)
		if err != nil {
			return nil, err
		}
		if err := s.ledger.Append(ctx, cogsEntry); err != nil &&
			!errors.Is(err, domain.ErrDuplicateEntry) {
			return nil, err
		}
	}

	return entry, nil
}

// RecordRefundInput là dữ liệu ghi sổ hoàn tiền.
type RecordRefundInput struct {
	OrderID          ids.ID
	RefundID         ids.ID
	Amount           money.Money
	SellerID         ids.ID
	SellerClawback   money.Money
	PlatformClawback money.Money
	CreatedBy        string
}

// RecordRefund ghi sổ hoàn tiền — ĐẢO NGƯỢC chuỗi ghi sổ ban đầu.
func (s *Service) RecordRefund(
	ctx context.Context, in RecordRefundInput,
) (*domain.LedgerEntry, error) {
	ref := in.RefundID
	if ref.IsZero() {
		ref = in.OrderID
	}
	key := "refund:" + ref.String()

	if existing, err := s.ledger.FindByIdempotencyKey(ctx, key); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	entry, err := domain.NewRefundEntry(domain.RefundParams{
		OrderID:          in.OrderID,
		RefundID:         in.RefundID,
		Amount:           in.Amount,
		SellerID:         in.SellerID,
		SellerClawback:   in.SellerClawback,
		PlatformClawback: in.PlatformClawback,
		IdempotencyKey:   key,
		CreatedBy:        in.CreatedBy,
		Now:              s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}

	if err := s.ledger.Append(ctx, entry); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			return s.ledger.FindByIdempotencyKey(ctx, key)
		}
		return nil, err
	}
	return entry, nil
}

// RecordPayout ghi sổ chi trả cho seller.
//
// KIỂM TRA SỐ DƯ trước khi chi: không chi quá số tiền đang nợ seller. Chi
// vượt sẽ tạo số dư âm — nghĩa là nền tảng đưa tiền của mình cho seller mà
// không có cơ sở.
func (s *Service) RecordPayout(
	ctx context.Context, payoutID, sellerID ids.ID, amount money.Money, createdBy string,
) (*domain.LedgerEntry, error) {
	key := "payout:" + payoutID.String()

	if existing, err := s.ledger.FindByIdempotencyKey(ctx, key); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	balance, err := s.balances.Balance(ctx, domain.Account{
		Type: domain.AccountSellerPayable, OwnerID: sellerID,
	})
	if err != nil {
		return nil, err
	}
	if balance.Amount.LessThan(amount) {
		return nil, fmt.Errorf("%w: số dư %s, yêu cầu chi %s",
			ErrInsufficientBalance, balance.Amount, amount)
	}

	entry, err := domain.NewPayoutEntry(
		payoutID, sellerID, amount, key, createdBy, s.clock.Now())
	if err != nil {
		return nil, err
	}

	if err := s.ledger.Append(ctx, entry); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			return s.ledger.FindByIdempotencyKey(ctx, key)
		}
		return nil, err
	}
	return entry, nil
}

// RecordAdjustment ghi bút toán ĐIỀU CHỈNH khi phát hiện ghi sai.
//
// Đây là cách DUY NHẤT sửa sai trong sổ cái bất biến.
func (s *Service) RecordAdjustment(
	ctx context.Context, refType string, refID ids.ID,
	lines []domain.Line, reason, idempotencyKey, createdBy string,
) (*domain.LedgerEntry, error) {
	if existing, err := s.ledger.FindByIdempotencyKey(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	entry, err := domain.NewAdjustmentEntry(
		refType, refID, lines, reason, idempotencyKey, createdBy, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if err := s.ledger.Append(ctx, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// ErrInsufficientBalance khi chi trả vượt số dư.
var ErrInsufficientBalance = errors.New("payment: số dư không đủ để chi trả")

// ---------------------------------------------------------------- Đọc

// GetBalance trả số dư của một tài khoản.
//
// Đây là hàm mà module seller và creator gọi thay vì tự lưu số dư. Hai nơi
// cùng lưu một sự thật thì sớm muộn chúng lệch nhau.
func (s *Service) GetBalance(ctx context.Context, account domain.Account) (domain.Balance, error) {
	return s.balances.Balance(ctx, account)
}

func (s *Service) GetSellerBalance(ctx context.Context, sellerID ids.ID) (domain.Balance, error) {
	return s.balances.Balance(ctx, domain.Account{
		Type: domain.AccountSellerPayable, OwnerID: sellerID,
	})
}

func (s *Service) GetEntriesForOrder(
	ctx context.Context, orderID ids.ID,
) ([]*domain.LedgerEntry, error) {
	return s.ledger.FindByReference(ctx, "ORDER", orderID)
}

// ---------------------------------------------------------------- Đối soát

// IntegrityCheck là kết quả kiểm tra toàn vẹn sổ sách.
//
// BA CHỈ SỐ PHẢI BẰNG 0 HÀNG NGÀY (deliverables.md mục 12.7). Đây là hai
// trong ba: bút toán không cân bằng, và tổng nợ ≠ tổng có.
type IntegrityCheck struct {
	TotalDebit  int64
	TotalCredit int64

	// Difference phải LUÔN bằng 0.
	Difference int64

	// UnbalancedEntries phải LUÔN rỗng.
	UnbalancedEntries []string

	CheckedEntries int
}

func (c IntegrityCheck) IsHealthy() bool {
	return c.Difference == 0 && len(c.UnbalancedEntries) == 0
}

// CheckIntegrity rà soát toàn vẹn sổ sách — chạy HÀNG NGÀY.
//
// Kết quả khác 0 là sự cố NGHIÊM TRỌNG, không phải "sai số chấp nhận được".
// Tiền không tự nhiên xuất hiện hay biến mất; lệch nghĩa là có lỗi ở đâu đó
// và mọi con số đối soát sau đó đều đáng ngờ.
func (s *Service) CheckIntegrity(ctx context.Context, from, to time.Time) (IntegrityCheck, error) {
	debit, credit, err := s.balances.TotalDebitCredit(ctx)
	if err != nil {
		return IntegrityCheck{}, err
	}

	entries, err := s.ledger.FindAll(ctx, from, to, 10000)
	if err != nil {
		return IntegrityCheck{}, err
	}
	report := domain.CheckIntegrity(entries)

	return IntegrityCheck{
		TotalDebit:        debit,
		TotalCredit:       credit,
		Difference:        debit - credit,
		UnbalancedEntries: report.UnbalancedEntries,
		CheckedEntries:    report.TotalEntries,
	}, nil
}
