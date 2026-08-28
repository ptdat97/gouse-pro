// Package application chứa các use case của module payment.
//
// Module này là NGUỒN SỰ THẬT DUY NHẤT về tiền. Không module nào khác được
// lưu số dư — muốn biết seller còn bao nhiêu thì gọi GetBalance().
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// AuditRecorder ghi vết kiểm toán cho thao tác nhạy cảm.
//
// Là PORT do tầng application định nghĩa nên nó không biết database hay
// bảng `audit_log`. Ngữ cảnh truyền vào PHẢI mang giao dịch của kho lưu
// trữ — xem domain.TxFunc.
type AuditRecorder interface {
	// RecordLedgerAdjustment ghi việc điều chỉnh sổ cái.
	//
	// Trả lỗi nếu lý do trống hoặc quá ngắn. Đây là thao tác nhạy cảm nhất
	// hệ thống: nó là đường DUY NHẤT tạo ra tiền trong sổ mà không có giao
	// dịch thật phía sau. "Vì sao" là thứ duy nhất phân biệt một lần sửa
	// sai chính đáng với một lần biển thủ.
	RecordLedgerAdjustment(ctx context.Context, in AdjustmentRecord) error
}

// AdjustmentRecord là dữ liệu ghi vào nhật ký khi điều chỉnh sổ cái.
type AdjustmentRecord struct {
	EntryID ids.ID

	// ActorID là nhân viên thực hiện. KHÔNG được rỗng.
	ActorID string

	ReferenceType string
	ReferenceID   ids.ID

	Reason string

	// TotalAmount là tổng một chiều của bút toán (Σ DEBIT = Σ CREDIT).
	//
	// Ghi vào vết để tra cứu nhanh "có lần điều chỉnh nào trên 10 triệu
	// tháng trước không" mà không phải đọc lại từng dòng bút toán.
	TotalAmount int64
	Currency    string

	RequestID string
}

// Service là tầng application của module payment.
type Service struct {
	ledger   domain.LedgerRepository
	balances domain.BalanceRepository
	audit    AuditRecorder
	clock    Clock
}

type Deps struct {
	Ledger   domain.LedgerRepository
	Balances domain.BalanceRepository
	Clock    Clock

	// Audit có thể nil: các use case ghi sổ tự động (doanh thu, hoàn tiền)
	// không cần. Chỉ RecordAdjustmentWithAudit bắt buộc có nó.
	Audit AuditRecorder
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		ledger:   d.Ledger,
		balances: d.Balances,
		audit:    d.Audit,
		clock:    clock,
	}
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
	return s.RecordOrderRevenueWith(ctx, s.ledger, in)
}

// RecordOrderRevenueWith ghi doanh thu bằng kho sổ cái bên gọi đưa vào.
//
// Dành cho bên nhận domain event: dispatcher đã mở giao dịch để đánh dấu
// event đã xử lý, và bút toán phải nằm TRONG giao dịch đó — xem
// postgres.LedgerForTx. Cùng khuôn với inventory.CommitInRepos.
func (s *Service) RecordOrderRevenueWith(
	ctx context.Context, ledger domain.LedgerRepository, in RecordOrderRevenueInput,
) (*domain.LedgerEntry, error) {
	// Khoá gồm CẢ nhà bán, không chỉ đơn hàng.
	//
	// Một đơn trộn hàng của nhiều nhà bán sinh ra NHIỀU bút toán — mỗi
	// nhà bán một cái, đúng như docs/07-workflows/marketplace-order.md
	// mục 4 mô tả. Khoá chỉ theo đơn sẽ khiến nhà bán thứ hai trở đi bị
	// coi là trùng lặp và tiền của họ không bao giờ được ghi sổ.
	//
	// Đơn own brand không có nhà bán: phần sau khoá rỗng, khoá vẫn xác định.
	key := "order-revenue:" + in.OrderID.String() + ":" + in.SellerID.String()

	// Đã ghi rồi thì trả kết quả cũ — đó là ý nghĩa của idempotent.
	if existing, err := ledger.FindByIdempotencyKey(ctx, key); err == nil {
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

	if err := ledger.Append(ctx, entry); err != nil {
		// Hai request đồng thời: bên kia vừa ghi xong. Đọc lại kết quả của
		// họ thay vì báo lỗi — kết quả cuối giống nhau.
		if errors.Is(err, domain.ErrDuplicateEntry) {
			return ledger.FindByIdempotencyKey(ctx, key)
		}
		return nil, err
	}

	// Đơn own brand ghi thêm giá vốn — bút toán RIÊNG, không gộp vào bút
	// toán doanh thu: hai sự kiện tài chính khác nhau.
	if in.COGS.IsPositive() {
		cogsKey := "order-cogs:" + in.OrderID.String() + ":" + in.SellerID.String()
		cogsEntry, err := domain.NewCOGSEntry(in.OrderID, in.COGS, cogsKey, in.CreatedBy, now)
		if err != nil {
			return nil, err
		}
		if err := ledger.Append(ctx, cogsEntry); err != nil &&
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
// ChuyenSangRutDuocInput là dữ liệu chuyển tiền nhà bán sang rút được.
type ChuyenSangRutDuocInput struct {
	FulfillmentID ids.ID
	SellerID      ids.ID
	Amount        money.Money
	CreatedBy     string
}

// ChuyenSangRutDuocWith ghi bút toán chuyển ĐANG CHỜ → RÚT ĐƯỢC.
//
// IDEMPOTENT theo đơn thực hiện: outbox giao hàng ít nhất một lần, và
// chuyển hai lần nghĩa là nhà bán rút được gấp đôi số tiền thật.
func (s *Service) ChuyenSangRutDuocWith(
	ctx context.Context, ledger domain.LedgerRepository, in ChuyenSangRutDuocInput,
) error {
	key := "seller-release:" + in.FulfillmentID.String()

	if _, err := ledger.FindByIdempotencyKey(ctx, key); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}

	entry, err := domain.NewSellerReleaseEntry(domain.SellerReleaseParams{
		FulfillmentID:  in.FulfillmentID,
		SellerID:       in.SellerID,
		Amount:         in.Amount,
		IdempotencyKey: key,
		CreatedBy:      in.CreatedBy,
		Now:            s.clock.Now(),
	})
	if err != nil {
		return err
	}

	if err := ledger.Append(ctx, entry); err != nil &&
		!errors.Is(err, domain.ErrDuplicateEntry) {
		return err
	}
	return nil
}

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

// AdjustmentInput là dữ liệu điều chỉnh sổ cái từ giao diện quản trị.
type AdjustmentInput struct {
	ReferenceType string
	ReferenceID   ids.ID

	Lines []domain.Line

	// ActorID là nhân viên thực hiện. BẮT BUỘC.
	ActorID string

	// Reason BẮT BUỘC, tối thiểu 20 ký tự.
	Reason string

	IdempotencyKey string
	RequestID      string
}

// RecordAdjustmentWithAudit ghi bút toán điều chỉnh VÀ vết kiểm toán trong
// CÙNG một giao dịch.
//
// Đây là đường mà giao diện quản trị phải dùng, không phải RecordAdjustment.
//
// # Vì sao thao tác này nguy hiểm nhất hệ thống
//
// Mọi bút toán khác đều có một sự kiện thật phía sau: khách trả tiền, hàng
// được giao, tiền được chuyển. Bút toán điều chỉnh KHÔNG — nó chỉ có lời
// giải thích của người tạo ra nó.
//
// Vì thế ba lớp bảo vệ phải cùng có mặt, và phải nguyên tử với nhau:
//
//  1. Σ DEBIT = Σ CREDIT      — cưỡng chế trong constructor của domain
//  2. Lý do có ý nghĩa         — cưỡng chế ở bộ ghi vết
//  3. Vết kiểm toán            — cùng giao dịch với bút toán
func (s *Service) RecordAdjustmentWithAudit(
	ctx context.Context, in AdjustmentInput,
) (*domain.LedgerEntry, error) {
	if s.audit == nil {
		return nil, errors.New(
			"payment: thiếu AuditRecorder — điều chỉnh sổ cái không được " +
				"chạy khi chưa có đường ghi vết kiểm toán")
	}
	if strings.TrimSpace(in.ActorID) == "" {
		return nil, errors.New("payment: thiếu định danh người thực hiện")
	}

	// Gọi lại cùng khóa idempotency trả kết quả cũ, KHÔNG ghi bút toán thứ
	// hai — nhân đôi một bút toán điều chỉnh là nhân đôi số tiền.
	if existing, err := s.ledger.FindByIdempotencyKey(ctx, in.IdempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	entry, err := domain.NewAdjustmentEntry(
		in.ReferenceType, in.ReferenceID, in.Lines,
		in.Reason, in.IdempotencyKey, in.ActorID, s.clock.Now())
	if err != nil {
		return nil, err
	}

	total, currency := oneSideTotal(in.Lines)

	err = s.ledger.AppendWithAudit(ctx, entry, func(txCtx context.Context) error {
		return s.audit.RecordLedgerAdjustment(txCtx, AdjustmentRecord{
			EntryID:       entry.ID(),
			ActorID:       in.ActorID,
			ReferenceType: in.ReferenceType,
			ReferenceID:   in.ReferenceID,
			Reason:        in.Reason,
			TotalAmount:   total,
			Currency:      currency,
			RequestID:     in.RequestID,
		})
	})
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// oneSideTotal cộng các dòng DEBIT.
//
// Vì bút toán luôn cân bằng, tổng một chiều là "quy mô" của nó. Cộng cả hai
// chiều sẽ ra số gấp đôi và gây hiểu nhầm khi đọc nhật ký.
func oneSideTotal(lines []domain.Line) (int64, string) {
	var total int64
	var currency string
	for _, l := range lines {
		if l.Direction == domain.Debit {
			total += l.Amount.Amount()
			currency = string(l.Amount.Currency())
		}
	}
	return total, currency
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

// SoDuNhaBan là số dư nhà bán theo TỪNG TRẠNG THÁI.
type SoDuNhaBan struct {
	// DangCho: đơn đã giao, còn trong hạn đổi trả. CHƯA rút được.
	DangCho money.Money

	// RutDuoc: hết hạn đổi trả, sẵn sàng đưa vào đối soát.
	RutDuoc money.Money
}

// GetSoDuNhaBan trả số dư nhà bán tách theo trạng thái.
//
// # Vì sao chỉ hai trạng thái, trong khi đặc tả nêu năm
//
// `pending` và `available` là hai trạng thái CÓ THẬT trong sổ cái hôm nay,
// mỗi cái là một tài khoản riêng và số dư tính được từ bút toán.
//
// Ba trạng thái còn lại — `processing`, `on_hold`, `reserve_held` — chưa
// tồn tại: chưa có luồng chi trả để "đang xử lý", chưa có cơ chế giữ tiền
// khi tranh chấp, và chưa có chính sách reserve. Tầng HTTP trả 0 cho
// chúng, và đó là con số ĐÚNG — thật sự không có đồng nào ở những trạng
// thái ấy. Bịa ra số khác mới là nói dối.
func (s *Service) GetSoDuNhaBan(
	ctx context.Context, sellerID ids.ID,
) (SoDuNhaBan, error) {
	cho, err := s.balances.Balance(ctx, domain.Account{
		Type: domain.AccountSellerPayable, OwnerID: sellerID,
	})
	if err != nil {
		return SoDuNhaBan{}, err
	}
	rut, err := s.balances.Balance(ctx, domain.Account{
		Type: domain.AccountSellerAvailable, OwnerID: sellerID,
	})
	if err != nil {
		return SoDuNhaBan{}, err
	}
	return SoDuNhaBan{DangCho: cho.Amount, RutDuoc: rut.Amount}, nil
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
