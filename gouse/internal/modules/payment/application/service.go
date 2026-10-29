// Package application chứa các use case của module payment.
//
// Module này là NGUỒN SỰ THẬT DUY NHẤT về tiền. Không module nào khác được
// lưu số dư — muốn biết seller còn bao nhiêu thì gọi GetBalance().
package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
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

func (systemClock) Now() time.Time { return types.BayGio() }

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

	// RecordSettlementSigned ghi việc KÝ một bước của đợt đối soát.
	//
	// Tách khỏi `RecordLedgerAdjustment` vì hai thao tác trả lời hai câu
	// khác nhau: điều chỉnh sổ cái TẠO RA tiền trong sổ, còn ký đợt đối
	// soát CHUYỂN tiền đã có ra ngoài. Gộp chúng vào một `Action` sẽ làm
	// báo cáo kiểm toán không phân biệt được hai rủi ro khác hẳn nhau.
	RecordSettlementSigned(ctx context.Context, in SettlementSignRecord) error
}

// SettlementSignRecord là dữ liệu ghi vào nhật ký khi ký một bước.
type SettlementSignRecord struct {
	SettlementID ids.ID
	SellerID     ids.ID

	// Step là "confirm" hoặc "mark_paid".
	Step string

	ActorID   string
	Reason    string
	RequestID string

	NetAmount int64
	Currency  string
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
	ledger      domain.LedgerRepository
	balances    domain.BalanceRepository
	settlements SettlementRepository
	intents     domain.IntentRepository
	audit       AuditRecorder
	clock       Clock
}

type Deps struct {
	Ledger      domain.LedgerRepository
	Balances    domain.BalanceRepository
	Settlements SettlementRepository
	Clock       Clock

	// Intents có thể nil ở các bản dựng chưa nối ý định thanh toán (test
	// của sổ cái). Use case nào cần nó tự báo lỗi rõ thay vì panic.
	Intents domain.IntentRepository

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
		ledger:      d.Ledger,
		balances:    d.Balances,
		settlements: d.Settlements,
		intents:     d.Intents,
		audit:       d.Audit,
		clock:       clock,
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
) (_ *domain.LedgerEntry, result error) {
	// Bọc bằng defer để MỌI đường thoát sớm đều được đếm. Rải lời gọi ở
	// từng nhánh `return` là cách chắc chắn để bỏ sót một nhánh, và nhánh
	// bị sót thường là nhánh hiếm — tức nhánh đáng quan tâm nhất.
	defer func() {
		if result != nil {
			metrics.RecordFailure(metrics.StagePayment, paymentFailureReason(result))
		}
	}()

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
// ReleaseToAvailableInput là dữ liệu chuyển tiền nhà bán sang rút được.
type ReleaseToAvailableInput struct {
	FulfillmentID ids.ID
	SellerID      ids.ID
	Amount        money.Money
	CreatedBy     string
}

// ReleaseToAvailableWith ghi bút toán chuyển ĐANG CHỜ → RÚT ĐƯỢC.
//
// IDEMPOTENT theo đơn thực hiện: outbox giao hàng ít nhất một lần, và
// chuyển hai lần nghĩa là nhà bán rút được gấp đôi số tiền thật.
func (s *Service) ReleaseToAvailableWith(
	ctx context.Context, ledger domain.LedgerRepository, in ReleaseToAvailableInput,
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
) (_ *domain.LedgerEntry, result error) {
	// Bọc bằng defer để MỌI đường thoát sớm đều được đếm. Rải lời gọi ở
	// từng nhánh `return` là cách chắc chắn để bỏ sót một nhánh, và nhánh
	// bị sót thường là nhánh hiếm — tức nhánh đáng quan tâm nhất.
	defer func() {
		if result != nil {
			metrics.RecordFailure(metrics.StagePayment, paymentFailureReason(result))
		}
	}()

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
) (_ *domain.LedgerEntry, result error) {
	// Bọc bằng defer để MỌI đường thoát sớm đều được đếm. Rải lời gọi ở
	// từng nhánh `return` là cách chắc chắn để bỏ sót một nhánh, và nhánh
	// bị sót thường là nhánh hiếm — tức nhánh đáng quan tâm nhất.
	defer func() {
		if result != nil {
			metrics.RecordFailure(metrics.StagePayment, paymentFailureReason(result))
		}
	}()

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

// SellerBalance là số dư nhà bán theo TỪNG TRẠNG THÁI.
type SellerBalance struct {
	// Pending: đơn đã giao, còn trong hạn đổi trả. CHƯA rút được.
	Pending money.Money

	// Available: hết hạn đổi trả, sẵn sàng đưa vào đối soát.
	Available money.Money
}

// GetSellerBalanceDetail trả số dư nhà bán tách theo trạng thái.
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
func (s *Service) GetSellerBalanceDetail(
	ctx context.Context, sellerID ids.ID,
) (SellerBalance, error) {
	pendingAcc, err := s.balances.Balance(ctx, domain.Account{
		Type: domain.AccountSellerPayable, OwnerID: sellerID,
	})
	if err != nil {
		return SellerBalance{}, err
	}
	rut, err := s.balances.Balance(ctx, domain.Account{
		Type: domain.AccountSellerAvailable, OwnerID: sellerID,
	})
	if err != nil {
		return SellerBalance{}, err
	}
	return SellerBalance{Pending: pendingAcc.Amount, Available: rut.Amount}, nil
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

// ---------------------------------------------------------------- Đối soát

// SettlementRepository là PORT cho kho lưu trữ đợt đối soát.
type SettlementRepository interface {
	// CreateForSeller lập đợt cho MỘT nhà bán trong một giao dịch có khóa.
	//
	// `dung` nhận phần đang NỢ (dương; 0 nếu không nợ) đọc được SAU khi đã
	// khóa nhà bán, rồi trả về đợt và bút toán thu hồi. Trả nil cho bút
	// toán nghĩa là kỳ này không thu gì.
	//
	// Đọc nợ, dựng đợt và ghi sổ phải nằm cùng một giao dịch — xem chú
	// thích ở `postgres.SettlementStore.CreateForSeller`.
	CreateForSeller(
		ctx context.Context, sellerID ids.ID,
		dung func(noDangCho money.Money) (*domain.Settlement, *domain.LedgerEntry, error),
	) error

	// SaveWithAudit ghi trạng thái mới và chạy `fn` trong CÙNG một giao dịch.
	//
	// Hai bước của vòng đời đợt đối soát là hai bước duy nhất biến một con
	// số tính được thành TIỀN RỜI nền tảng, nên vết kiểm toán không được
	// phép ghi rời — xem `postgres.SettlementStore.SaveWithAudit`.
	SaveWithAudit(ctx context.Context, d *domain.Settlement, fn domain.TxFunc) error

	FindByID(ctx context.Context, id ids.ID) (*domain.Settlement, error)
	BySeller(ctx context.Context, sellerID ids.ID, limit int) ([]*domain.Settlement, error)

	// ByStatus liệt kê đợt của MỌI nhà bán — mặt QUẢN TRỊ.
	//
	// Tách khỏi `BySeller` vì hai hàm có ranh giới bảo mật ngược
	// nhau; xem chú thích ở `postgres.SettlementStore.ByStatus`.
	ByStatus(ctx context.Context, status string, limit int) ([]*domain.Settlement, error)

	// UnsettledEntries trả các bút toán rút được chưa thuộc đợt nào, gom
	// theo nhà bán.
	UnsettledEntries(
		ctx context.Context, toDate time.Time, limit int,
	) (map[ids.ID][]domain.SettlementLine, error)
}

// NewSettlementForPeriod tạo đợt đối soát cho mọi nhà bán có khoản rút được.
//
// # Vì sao trừ phần ÂM của tài khoản đang chờ
//
// Hoàn tiền thu hồi từ SELLER_PAYABLE. Nhưng khách có thể xin trả ngày 6
// và hàng về ngày 10 — khi tiền ĐÃ chuyển sang rút được. Khoản thu hồi
// khi ấy rơi vào một tài khoản đã rỗng và làm nó âm.
//
// Chi trả trọn phần rút được trong tình huống đó là trả cả khoản vừa hoàn
// cho khách. Nên số thực chi = rút được TRỪ phần âm ấy.
//
// Phần đang chờ DƯƠNG thì KHÔNG cộng vào: đó là tiền của đơn còn trong hạn
// đổi trả, chưa được phép chi.
func (s *Service) NewSettlementForPeriod(
	ctx context.Context, fromDate, toDate time.Time, limitPerRound int,
) (int, error) {
	if s.settlements == nil {
		return 0, errors.New("payment: chưa cấu hình kho lưu trữ đối soát")
	}
	if limitPerRound <= 0 || limitPerRound > 5000 {
		limitPerRound = 1000
	}

	bySeller, err := s.settlements.UnsettledEntries(ctx, toDate, limitPerRound)
	if err != nil {
		return 0, err
	}

	created := 0
	for sellerID, line := range bySeller {
		if len(line) == 0 {
			continue
		}

		err := s.settlements.CreateForSeller(ctx, sellerID,
			func(no money.Money) (*domain.Settlement, *domain.LedgerEntry, error) {
				d, err := domain.NewSettlement(domain.NewSettlementParams{
					SellerID: sellerID, PeriodStart: fromDate, PeriodEnd: toDate,
					Lines: line, Deficit: no, Now: s.clock.Now(),
				})
				if err != nil {
					return nil, nil, err
				}

				// `NewSettlement` KẸP phần bị trừ về không quá tổng đợt, nên
				// `d.Deficit()` là phần THỰC SỰ thu được — không phải `no`.
				// Ghi theo `no` sẽ thu nhiều hơn số đã trừ, và chênh lệch
				// ấy là tiền biến mất khỏi tài khoản nhà bán mà không dòng
				// nào giải thích.
				if d.Deficit().IsZero() {
					return d, nil, nil
				}

				clawback, err := domain.NewClawbackEntry(domain.ClawbackParams{
					SettlementID: d.ID(), SellerID: sellerID,
					Amount:         d.Deficit(),
					IdempotencyKey: "settlement-recovery:" + d.ID().String(),
					CreatedBy:      "worker:tao-doi-soat",
					Now:            s.clock.Now(),
				})
				if err != nil {
					return nil, nil, err
				}
				return d, clawback, nil
			})
		if err != nil {
			// Bút toán đã bị lượt khác gom mất: bỏ qua nhà bán này, lượt
			// sau nhặt phần còn lại. KHÔNG phải lỗi cần dừng cả job.
			if errors.Is(err, domain.ErrDuplicateEntry) {
				continue
			}
			return created, err
		}
		created++
	}
	return created, nil
}

// GetSettlement đọc một đợt đối soát, KÈM kiểm chủ sở hữu.
func (s *Service) GetSettlement(
	ctx context.Context, id, sellerID ids.ID,
) (*domain.Settlement, error) {
	d, err := s.settlements.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// KHÔNG phân biệt "không tồn tại" với "của nhà bán khác".
	if !sellerID.IsZero() && d.SellerID() != sellerID {
		return nil, domain.ErrSettlementNotFound
	}
	return d, nil
}

// ListSettlementsForAdmin liệt kê đợt của MỌI nhà bán, lọc theo trạng thái.
//
// Dành cho mặt QUẢN TRỊ. Hàm này cố ý KHÔNG nhận `sellerID`: nhận nó rồi để
// rỗng nghĩa là có một đường gọi trả dữ liệu mọi gian hàng mà trông như
// đường của nhà bán.
func (s *Service) ListSettlementsForAdmin(
	ctx context.Context, status string, limit int,
) ([]*domain.Settlement, error) {
	if s.settlements == nil {
		return nil, errors.New("payment: chưa nối kho đợt đối soát")
	}
	switch status {
	case "", string(domain.SettlementDraft), string(domain.SettlementConfirmed),
		string(domain.SettlementPaid):
	default:
		// Trạng thái lạ trả RỖNG thay vì mọi đợt: một lỗi đánh máy ở tham
		// số lọc không được biến thành "cho xem hết".
		return nil, nil
	}
	return s.settlements.ByStatus(ctx, status, limit)
}

// ListSettlements trả các đợt của một nhà bán.
func (s *Service) ListSettlements(
	ctx context.Context, sellerID ids.ID, limit int,
) ([]*domain.Settlement, error) {
	return s.settlements.BySeller(ctx, sellerID, limit)
}

// paymentFailureReason phân loại lỗi đường tiền thành nhãn Prometheus.
//
// Tập giá trị phải ĐÓNG và nhỏ: mỗi giá trị nhãn là một chuỗi thời gian
// riêng, nên truyền thẳng thông điệp lỗi — thứ thường chứa mã đơn — sẽ
// làm nổ số chuỗi thời gian.
func paymentFailureReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrUnbalanced):
		// Tách riêng, và là nhãn đáng theo dõi nhất ở đây: bút toán lệch
		// nghĩa là sổ sách sai. Khác 0 là sự cố, không phải chỉ số.
		return "unbalanced"
	case errors.Is(err, domain.ErrDuplicateEntry):
		return "duplicate"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	case errors.Is(err, domain.ErrNoLines),
		errors.Is(err, domain.ErrMixedCurrency),
		errors.Is(err, domain.ErrNonPositiveLine),
		errors.Is(err, domain.ErrMissingReference),
		errors.Is(err, domain.ErrMissingIdempKey):
		return "invalid_entry"
	default:
		return "internal"
	}
}

// RecordPaymentInput là dữ liệu bút toán THU ĐƯỢC TIỀN.
type RecordPaymentInput struct {
	OrderID        ids.ID
	Amount         money.Money
	IdempotencyKey string
}

// GhiThuTien ghi bút toán chuyển KHOẢN PHẢI THU thành TIỀN MẶT.
//
// ADR-0018 phần B1. Gọi bởi bên nhận `order.paid`, nên nó nhận kho sổ cái
// của giao dịch dispatcher — cùng khuôn với RecordOrderRevenueWith và
// cùng lý do: ghi sổ thành công mà đánh dấu event thất bại thì lần thử
// lại ghi LẦN THỨ HAI, và sổ cái là chỗ không được phép đếm hai lần.
//
// Trùng khóa idempotency trả ErrDuplicateEntry, và bên gọi coi đó là
// THÀNH CÔNG: event phát lại là chuyện bình thường.
func (s *Service) RecordPaymentWith(
	ctx context.Context, ledger domain.LedgerRepository, in RecordPaymentInput) (*domain.LedgerEntry, error) {
	e, err := domain.NewPaymentReceivedEntry(domain.PaymentReceivedParams{
		OrderID:        in.OrderID,
		Amount:         in.Amount,
		IdempotencyKey: in.IdempotencyKey,
		CreatedBy:      "system",
		Now:            s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	// TRA TRƯỚC khi ghi, không ghi rồi bắt lỗi.
	//
	// Event `order.paid` phát lại là đường đi BÌNH THƯỜNG, không phải
	// ngoại lệ. Để câu INSERT vấp khóa idempotency mỗi lần phát lại là
	// làm hỏng giao dịch của dispatcher: trong PostgreSQL, một câu lệnh
	// lỗi làm cả giao dịch không commit được, kể cả khi bên gọi bỏ qua
	// lỗi đó. Kết quả là event bị thử lại vĩnh viễn.
	if cu, err := ledger.FindByIdempotencyKey(ctx, in.IdempotencyKey); err == nil && cu != nil {
		return cu, nil
	}

	if err := ledger.Append(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// ReverseEntryInput là dữ liệu đảo một bút toán đã ghi sai.
type ReverseEntryInput struct {
	// EntryID là bút toán GỐC cần đảo.
	EntryID ids.ID

	// Reason BẮT BUỘC — xem domain.NewReversalEntry.
	Reason string

	ChangedBy string
}

// ReverseEntry ghi bút toán ĐẢO cho một bút toán đã ghi sai.
//
// # Vì sao đây là thao tác NGUY HIỂM NHẤT trong hệ thống
//
// Nó là đường duy nhất làm một con số biến mất khỏi số dư. Chú thích của
// `AppendWithAudit` đã nói: điều chỉnh sổ cái là thao tác duy nhất có thể
// tạo ra tiền từ hư không nếu làm sai. Vì vậy:
//
//   - LÝ DO bắt buộc, và đi vào mô tả bút toán
//   - mỗi bút toán chỉ đảo được MỘT lần, cưỡng chế bằng chỉ mục UNIQUE
//     trên `reverses_entry_id` chứ không bằng câu SELECT ở đây
//   - KHÔNG đảo một bút toán đảo
//   - số tiền lấy nguyên từ bút toán gốc, không tính lại
//
// Idempotency key suy ra TỪ bút toán gốc, nên gọi lại hai lần cho cùng
// một bút toán không sinh hai lần đảo.
func (s *Service) ReverseEntry(
	ctx context.Context, in ReverseEntryInput) (*domain.LedgerEntry, error) {
	original, err := s.ledger.FindByID(ctx, in.EntryID)
	if err != nil {
		return nil, err
	}

	key := "dao:" + in.EntryID.String()
	if cu, err := s.ledger.FindByIdempotencyKey(ctx, key); err == nil && cu != nil {
		return cu, nil
	}

	e, err := domain.NewReversalEntry(domain.ReversalParams{
		Original:       original,
		Reason:         in.Reason,
		IdempotencyKey: key,
		CreatedBy:      in.ChangedBy,
		Now:            s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}

	if err := s.ledger.Append(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// RecordShippingFeeInput là dữ liệu bút toán phí vận chuyển.
type RecordShippingFeeInput struct {
	OrderID        ids.ID
	Fee            money.Money
	IdempotencyKey string
}

// RecordShippingFeeWith ghi bút toán doanh thu PHÍ VẬN CHUYỂN.
//
// Xem domain.NewShippingRevenueEntry. Trùng khóa idempotency là đường đi
// bình thường của một event phát lại, nên tra TRƯỚC khi ghi.
func (s *Service) RecordShippingFeeWith(
	ctx context.Context, ledger domain.LedgerRepository, in RecordShippingFeeInput) (*domain.LedgerEntry, error) {
	if cu, err := ledger.FindByIdempotencyKey(ctx, in.IdempotencyKey); err == nil && cu != nil {
		return cu, nil
	}

	e, err := domain.NewShippingRevenueEntry(domain.ShippingRevenueParams{
		OrderID:        in.OrderID,
		Fee:            in.Fee,
		IdempotencyKey: in.IdempotencyKey,
		CreatedBy:      "payment.revenue_on_checkout_completed",
		Now:            s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := ledger.Append(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// RecordDiscountInput là dữ liệu bút toán khoản giảm giá.
type RecordDiscountInput struct {
	OrderID  ids.ID
	Discount money.Money

	// Allocation là bảng chia chi phí; rỗng = nền tảng gánh trọn.
	Allocation []domain.AllocateCost

	IdempotencyKey string
}

// RecordDiscountWith ghi bút toán khoản GIẢM GIÁ đã cho khách.
//
// Xem domain.NewDiscountEntry. Tra idempotency TRƯỚC khi ghi, cùng lý do
// với RecordPaymentWith: event phát lại là đường đi bình thường.
func (s *Service) RecordDiscountWith(
	ctx context.Context, ledger domain.LedgerRepository, in RecordDiscountInput) (*domain.LedgerEntry, error) {
	if cu, err := ledger.FindByIdempotencyKey(ctx, in.IdempotencyKey); err == nil && cu != nil {
		return cu, nil
	}

	e, err := domain.NewDiscountEntry(domain.DiscountParams{
		OrderID:        in.OrderID,
		Discount:       in.Discount,
		Allocation:     in.Allocation,
		IdempotencyKey: in.IdempotencyKey,
		CreatedBy:      "payment.revenue_on_checkout_completed",
		Now:            s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := ledger.Append(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// RecordShippingCostInput là dữ liệu bút toán chi phí trả hãng.
type RecordShippingCostInput struct {
	FulfillmentID  ids.ID
	ChiPhi         money.Money
	IdempotencyKey string
}

// RecordShippingCostWith ghi nghĩa vụ trả hãng vận chuyển.
//
// Xem domain.NewShippingCostEntry. Tra idempotency TRƯỚC khi ghi: event
// tiến độ được phát lại là đường đi bình thường, và ghi hai lần nghĩa là
// tính chi phí gấp đôi cho một kiện hàng.
func (s *Service) RecordShippingCostWith(
	ctx context.Context, ledger domain.LedgerRepository,
	in RecordShippingCostInput,
) (*domain.LedgerEntry, error) {
	if cu, err := ledger.FindByIdempotencyKey(ctx, in.IdempotencyKey); err == nil && cu != nil {
		return cu, nil
	}

	e, err := domain.NewShippingCostEntry(domain.ShippingCostParams{
		FulfillmentID:  in.FulfillmentID,
		Cost:           in.ChiPhi,
		IdempotencyKey: in.IdempotencyKey,
		CreatedBy:      "payment.ghi_chi_phi_hang_van_chuyen",
		Now:            s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := ledger.Append(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}
