// Package domain chứa mô hình nghiệp vụ của module payment.
//
// NGUYÊN TẮC CỐT LÕI: nền tảng GIỮ TIỀN HỘ nhiều bên. Tiền khách trả KHÔNG
// thuộc về nền tảng — nó là khoản thu hộ, phải trả lại cho seller, creator,
// và nhà cung cấp dịch vụ.
//
// Vì vậy sổ cái ở đây là BÚT TOÁN KÉP BẤT BIẾN, không phải bảng giao dịch
// đơn giản. Xem docs/adr/0008-financial-ledger.md.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	// ErrUnbalanced là lỗi NGHIÊM TRỌNG NHẤT của module này.
	//
	// Σ DEBIT phải = Σ CREDIT. Bút toán không cân bằng nghĩa là tiền xuất
	// hiện từ hư không hoặc biến mất — sổ sách sai và không ai biết sai ở đâu.
	//
	// Chỉ báo "bút toán không cân bằng" phải LUÔN bằng 0 (mvp.md mục 7).
	ErrUnbalanced = errors.New("payment: bút toán không cân bằng, Σ DEBIT ≠ Σ CREDIT")

	ErrNoLines          = errors.New("payment: bút toán phải có ít nhất hai dòng")
	ErrMixedCurrency    = errors.New("payment: mọi dòng trong một bút toán phải cùng đơn vị tiền tệ")
	ErrNonPositiveLine  = errors.New("payment: số tiền mỗi dòng phải lớn hơn 0")
	ErrMissingReference = errors.New("payment: bút toán phải có tham chiếu nguồn gốc")
	ErrMissingIdempKey  = errors.New("payment: bút toán phải có khóa idempotency")
	ErrNotFound         = errors.New("payment: không tìm thấy")

	// ErrDuplicateEntry khi khóa idempotency đã được dùng.
	//
	// Đây KHÔNG phải lỗi: nó nghĩa là bút toán đã được ghi rồi. Bên gọi nên
	// coi như thành công và đọc lại bút toán cũ — ghi hai lần cùng một sự
	// kiện tài chính sẽ nhân đôi số tiền.
	ErrDuplicateEntry = errors.New("payment: bút toán với khóa này đã tồn tại")
)

// AccountType là loại tài khoản trong danh mục (mục 4.2 của đặc tả).
//
// Phân loại theo bản chất kế toán, vì nó quyết định cách tính số dư:
// tài khoản TÀI SẢN và CHI PHÍ tăng khi ghi NỢ; tài khoản NỢ PHẢI TRẢ và
// DOANH THU tăng khi ghi CÓ.
type AccountType string

const (
	// PLATFORM_CASH — tiền mặt nền tảng đang giữ. TÀI SẢN.
	AccountPlatformCash AccountType = "PLATFORM_CASH"

	// PLATFORM_REVENUE — doanh thu nền tảng (hoa hồng, phí). DOANH THU.
	AccountPlatformRevenue AccountType = "PLATFORM_REVENUE"

	// SELLER_PAYABLE — phải trả seller. NỢ PHẢI TRẢ.
	AccountSellerPayable AccountType = "SELLER_PAYABLE"

	// CREATOR_PAYABLE — phải trả creator. NỢ PHẢI TRẢ.
	AccountCreatorPayable AccountType = "CREATOR_PAYABLE"

	// CUSTOMER_REFUND_PAYABLE — phải hoàn khách. NỢ PHẢI TRẢ.
	AccountCustomerRefundPayable AccountType = "CUSTOMER_REFUND_PAYABLE"

	// SUPPLIER_PAYABLE — phải trả nhà cung cấp. NỢ PHẢI TRẢ.
	AccountSupplierPayable AccountType = "SUPPLIER_PAYABLE"

	// COGS — giá vốn hàng bán. CHI PHÍ.
	AccountCOGS AccountType = "COGS"

	// FEE_EXPENSE — chi phí (phí cổng thanh toán, vận chuyển). CHI PHÍ.
	AccountFeeExpense AccountType = "FEE_EXPENSE"

	// INVENTORY_ASSET — giá trị hàng tồn kho (own brand). TÀI SẢN.
	AccountInventoryAsset AccountType = "INVENTORY_ASSET"
)

func (a AccountType) valid() bool {
	switch a {
	case AccountPlatformCash, AccountPlatformRevenue, AccountSellerPayable,
		AccountCreatorPayable, AccountCustomerRefundPayable, AccountSupplierPayable,
		AccountCOGS, AccountFeeExpense, AccountInventoryAsset:
		return true
	}
	return false
}

// IsDebitNormal cho biết tài khoản này TĂNG khi ghi NỢ.
//
// Tài sản và chi phí tăng khi ghi nợ; nợ phải trả và doanh thu tăng khi
// ghi có. Phân biệt này quyết định dấu của số dư — nhầm sẽ ra số âm ở chỗ
// lẽ ra phải dương.
func (a AccountType) IsDebitNormal() bool {
	switch a {
	case AccountPlatformCash, AccountCOGS, AccountFeeExpense, AccountInventoryAsset:
		return true
	}
	return false
}

// Direction là hướng của một dòng bút toán.
type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

// EntryType là loại sự kiện tài chính.
type EntryType string

const (
	EntryOrderRevenue EntryType = "ORDER_REVENUE"
	EntryCOGS         EntryType = "COGS"
	EntryRefund       EntryType = "REFUND"
	EntryPayout       EntryType = "PAYOUT"
	EntryAdjustment   EntryType = "ADJUSTMENT"
	EntryFee          EntryType = "FEE"
)

// Account định danh một tài khoản cụ thể.
//
// Tài khoản của nền tảng chỉ cần loại; tài khoản phải trả cần thêm định
// danh chủ sở hữu để biết "phải trả AI bao nhiêu".
type Account struct {
	Type AccountType

	// OwnerID rỗng với tài khoản của nền tảng (PLATFORM_CASH, COGS...).
	// Bắt buộc với tài khoản phải trả (SELLER_PAYABLE cần biết seller nào).
	OwnerID ids.ID
}

// RequiresOwner cho biết loại tài khoản này có bắt buộc chủ sở hữu không.
//
// SELLER_PAYABLE mà không biết seller nào thì vô nghĩa: không đối soát và
// không chi trả được.
func (a AccountType) RequiresOwner() bool {
	switch a {
	case AccountSellerPayable, AccountCreatorPayable,
		AccountCustomerRefundPayable, AccountSupplierPayable:
		return true
	}
	return false
}

// Key là chuỗi định danh duy nhất của tài khoản, dùng làm khóa gom nhóm.
func (a Account) Key() string {
	if a.OwnerID.IsZero() {
		return string(a.Type)
	}
	return string(a.Type) + ":" + a.OwnerID.String()
}

func (a Account) validate() error {
	if !a.Type.valid() {
		return fmt.Errorf("payment: loại tài khoản không hợp lệ: %s", a.Type)
	}
	if a.Type.RequiresOwner() && a.OwnerID.IsZero() {
		return fmt.Errorf("payment: tài khoản %s bắt buộc phải có chủ sở hữu", a.Type)
	}
	return nil
}

// Line là MỘT DÒNG ghi nợ hoặc ghi có.
//
// Amount LUÔN DƯƠNG — hướng nằm ở Direction, không nằm ở dấu của số. Dấu
// âm rất dễ đọc nhầm khi cộng dồn báo cáo, và một dấu trừ đặt sai chỗ làm
// bút toán vẫn "cân bằng" nhưng sai hoàn toàn.
type Line struct {
	Account   Account
	Direction Direction
	Amount    money.Money

	// Description giải thích dòng này để người đọc sổ hiểu ngay.
	Description string
}

func (l Line) validate() error {
	if err := l.Account.validate(); err != nil {
		return err
	}
	if l.Direction != Debit && l.Direction != Credit {
		return fmt.Errorf("payment: hướng không hợp lệ: %s", l.Direction)
	}
	if !l.Amount.IsPositive() {
		return fmt.Errorf("%w: %s", ErrNonPositiveLine, l.Amount)
	}
	return nil
}

// LedgerEntry là MỘT SỰ KIỆN TÀI CHÍNH — aggregate root, BẤT BIẾN.
//
// Không có phương thức sửa đổi. Sửa sai bằng cách ghi bút toán ĐIỀU CHỈNH
// mới, không phải sửa bút toán cũ:
//
//	SAI:  UPDATE ledger_line SET amount = 25000 WHERE ...
//	      → không ai biết đã từng ghi 30.000đ
//
//	ĐÚNG: Bút toán mới loại ADJUSTMENT
//	      → kết quả giống nhau, nhưng GIỮ ĐƯỢC LỊCH SỬ
//
// Ở tầng database còn có RULE chặn UPDATE/DELETE — kể cả lỗi code hay thao
// tác thủ công nhầm, database vẫn từ chối.
type LedgerEntry struct {
	id ids.ID

	entryType EntryType

	// referenceType và referenceID trỏ tới nguồn gốc sự kiện (đơn hàng,
	// yêu cầu hoàn tiền, đợt chi trả). Tham chiếu vượt module — chỉ giữ
	// định danh.
	referenceType string
	referenceID   ids.ID

	description string

	// idempotencyKey chống ghi trùng.
	//
	// Ghi hai lần cùng một sự kiện tài chính sẽ NHÂN ĐÔI số tiền — loại
	// lỗi tệ nhất có thể xảy ra ở module này.
	idempotencyKey string

	lines []Line

	createdBy string
	createdAt time.Time
	// KHÔNG có updatedAt — bản ghi bất biến.
}

type NewEntryParams struct {
	Type           EntryType
	ReferenceType  string
	ReferenceID    ids.ID
	Description    string
	IdempotencyKey string
	Lines          []Line
	CreatedBy      string
	Now            time.Time
}

// NewLedgerEntry tạo bút toán và KIỂM TRA BẤT BIẾN Σ DEBIT = Σ CREDIT.
//
// Kiểm tra trong constructor là có chủ đích: không có đường nào tạo được
// bút toán không cân bằng. Nếu để kiểm tra ở tầng application, sẽ có chỗ
// quên và tiền sẽ xuất hiện từ hư không.
func NewLedgerEntry(p NewEntryParams) (*LedgerEntry, error) {
	if len(p.Lines) < 2 {
		// Bút toán kép cần ÍT NHẤT hai dòng: tiền đi từ đâu tới đâu.
		return nil, ErrNoLines
	}
	if p.ReferenceID.IsZero() || strings.TrimSpace(p.ReferenceType) == "" {
		return nil, ErrMissingReference
	}
	if strings.TrimSpace(p.IdempotencyKey) == "" {
		return nil, ErrMissingIdempKey
	}

	for i, l := range p.Lines {
		if err := l.validate(); err != nil {
			return nil, fmt.Errorf("dòng %d: %w", i+1, err)
		}
	}

	if err := checkBalanced(p.Lines); err != nil {
		return nil, err
	}

	id, err := ids.New(ids.PrefixLedgerEntry)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &LedgerEntry{
		id:             id,
		entryType:      p.Type,
		referenceType:  strings.TrimSpace(p.ReferenceType),
		referenceID:    p.ReferenceID,
		description:    strings.TrimSpace(p.Description),
		idempotencyKey: strings.TrimSpace(p.IdempotencyKey),
		lines:          append([]Line(nil), p.Lines...),
		createdBy:      p.CreatedBy,
		createdAt:      now,
	}, nil
}

// checkBalanced kiểm tra BẤT BIẾN CỐT LÕI: Σ DEBIT = Σ CREDIT.
//
// Đồng thời kiểm tra mọi dòng cùng đơn vị tiền tệ — cộng 300.000 VND với
// 20 USD ra một con số vô nghĩa nhưng trông vẫn "cân bằng".
func checkBalanced(lines []Line) error {
	currency := lines[0].Amount.Currency()

	var debit, credit int64
	for i, l := range lines {
		if l.Amount.Currency() != currency {
			return fmt.Errorf("%w: dòng 1 dùng %s, dòng %d dùng %s",
				ErrMixedCurrency, currency, i+1, l.Amount.Currency())
		}
		if l.Direction == Debit {
			debit += l.Amount.Amount()
		} else {
			credit += l.Amount.Amount()
		}
	}

	if debit != credit {
		return fmt.Errorf("%w: DEBIT %d, CREDIT %d, lệch %d",
			ErrUnbalanced, debit, credit, debit-credit)
	}
	return nil
}

// RestoreEntryParams dựng lại từ kho lưu trữ.
type RestoreEntryParams struct {
	ID             ids.ID
	Type           EntryType
	ReferenceType  string
	ReferenceID    ids.ID
	Description    string
	IdempotencyKey string
	Lines          []Line
	CreatedBy      string
	CreatedAt      time.Time
}

// RestoreLedgerEntry dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreLedgerEntry(p RestoreEntryParams) *LedgerEntry {
	return &LedgerEntry{
		id:             p.ID,
		entryType:      p.Type,
		referenceType:  p.ReferenceType,
		referenceID:    p.ReferenceID,
		description:    p.Description,
		idempotencyKey: p.IdempotencyKey,
		lines:          p.Lines,
		createdBy:      p.CreatedBy,
		createdAt:      p.CreatedAt,
	}
}

func (e *LedgerEntry) ID() ids.ID             { return e.id }
func (e *LedgerEntry) Type() EntryType        { return e.entryType }
func (e *LedgerEntry) ReferenceType() string  { return e.referenceType }
func (e *LedgerEntry) ReferenceID() ids.ID    { return e.referenceID }
func (e *LedgerEntry) Description() string    { return e.description }
func (e *LedgerEntry) IdempotencyKey() string { return e.idempotencyKey }
func (e *LedgerEntry) CreatedBy() string      { return e.createdBy }
func (e *LedgerEntry) CreatedAt() time.Time   { return e.createdAt }

// Lines trả về bản sao — bút toán bất biến, bên ngoài không được sửa.
func (e *LedgerEntry) Lines() []Line {
	return append([]Line(nil), e.lines...)
}

// Currency là đơn vị tiền tệ của bút toán.
func (e *LedgerEntry) Currency() money.Currency {
	if len(e.lines) == 0 {
		return ""
	}
	return e.lines[0].Amount.Currency()
}

// Total là tổng số tiền của bút toán (bằng Σ DEBIT, cũng bằng Σ CREDIT).
func (e *LedgerEntry) Total() money.Money {
	var sum int64
	for _, l := range e.lines {
		if l.Direction == Debit {
			sum += l.Amount.Amount()
		}
	}
	return money.MustNew(sum, e.Currency())
}

// IsBalanced kiểm tra lại bất biến — dùng cho job đối soát hàng ngày.
//
// Bút toán đã qua constructor thì luôn cân bằng; hàm này để phát hiện dữ
// liệu bị hỏng do can thiệp bất thường vào database.
func (e *LedgerEntry) IsBalanced() bool {
	return checkBalanced(e.lines) == nil
}
