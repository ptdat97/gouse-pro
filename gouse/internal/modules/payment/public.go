// Package payment là module quản lý tiền — sổ cái tài chính bất biến.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// NGUỒN SỰ THẬT DUY NHẤT VỀ TIỀN: không module nào khác được lưu số dư.
// Module seller và creator gọi GetSellerBalance() thay vì tự lưu — hai nơi
// cùng lưu một sự thật thì sớm muộn chúng lệch nhau, và khi lệch thì không
// biết bên nào đúng.
//
// Xem docs/adr/0008-financial-ledger.md.
package payment

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module payment.
type API interface {
	// RecordOrderRevenue ghi sổ doanh thu một đơn hàng.
	//
	// IDEMPOTENT theo orderID: gọi lại KHÔNG ghi thêm. Ghi hai lần cùng
	// một đơn sẽ nhân đôi số tiền phải trả seller.
	//
	// Mọi số tiền phải ĐÃ ĐÓNG BĂNG ở module order — module này không tính
	// lại. Tính lại sẽ ra số khác khi chính sách hoa hồng đổi.
	RecordOrderRevenue(ctx context.Context, req OrderRevenueRequest) (*EntryView, error)

	// RecordRefund ghi sổ hoàn tiền — đảo ngược chuỗi ghi sổ ban đầu.
	RecordRefund(ctx context.Context, req RefundRequest) (*EntryView, error)

	// RecordPayout ghi sổ chi trả cho seller.
	//
	// KIỂM TRA SỐ DƯ trước khi chi: không chi quá số tiền đang nợ.
	RecordPayout(ctx context.Context, req PayoutRequest) (*EntryView, error)

	// GetSellerBalance trả số dư của một nhà bán.
	//
	// Module seller gọi hàm này thay vì tự lưu số dư (quy tắc 4 của seller).
	GetSellerBalance(ctx context.Context, sellerID string) (*BalanceView, error)

	// GetPlatformBalance trả số dư một tài khoản của nền tảng.
	GetPlatformBalance(ctx context.Context, accountType string) (*BalanceView, error)

	// GetEntriesForOrder trả mọi bút toán của một đơn hàng.
	//
	// Trả lời "đơn này đã ghi sổ những gì" — câu hỏi đầu tiên khi có tranh chấp.
	GetEntriesForOrder(ctx context.Context, orderID string) ([]EntryView, error)

	// CheckIntegrity rà soát toàn vẹn sổ sách — chạy HÀNG NGÀY.
	//
	// Kết quả không lành mạnh là sự cố NGHIÊM TRỌNG, không phải "sai số
	// chấp nhận được".
	CheckIntegrity(ctx context.Context) (*IntegrityView, error)
}

// ---------------------------------------------------------------- DTO

// Amount là số tiền kèm đơn vị.
//
// Value là số nguyên theo đơn vị nhỏ nhất của tiền tệ. KHÔNG dùng số thực
// cho tiền — sai số dấu phẩy động tích lũy thành lệch đối soát, và lệch
// đối soát phải điều tra thủ công từng đơn.
type Amount struct {
	Value    int64
	Currency string
}

// OrderRevenueRequest là dữ liệu ghi sổ doanh thu đơn hàng.
type OrderRevenueRequest struct {
	OrderID string

	// GrossAmount là tổng tiền khách trả.
	GrossAmount Amount

	// SellerID rỗng nghĩa là đơn OWN BRAND.
	SellerID      string
	SellerPayable Amount

	// PlatformRevenue: đơn marketplace chỉ HOA HỒNG; đơn own brand TOÀN BỘ.
	//
	// Đây là chỗ phân biệt GMV với doanh thu ngay ở tầng ghi sổ.
	PlatformRevenue Amount

	CreatorID      string
	CreatorPayable Amount

	PaymentFee Amount

	// COGS chỉ có với đơn own brand. Đơn marketplace bằng 0: hàng không
	// phải tài sản của nền tảng.
	COGS Amount

	CreatedBy string
}

// RefundRequest là dữ liệu ghi sổ hoàn tiền.
type RefundRequest struct {
	OrderID  string
	RefundID string
	Amount   Amount

	SellerID         string
	SellerClawback   Amount
	PlatformClawback Amount

	CreatedBy string
}

// PayoutRequest là dữ liệu ghi sổ chi trả.
type PayoutRequest struct {
	PayoutID  string
	SellerID  string
	Amount    Amount
	CreatedBy string
}

// EntryView là một bút toán — CHỈ ĐỌC, BẤT BIẾN.
type EntryView struct {
	ID   string
	Type string

	ReferenceType string
	ReferenceID   string
	Description   string

	Lines []LineView

	// Total bằng Σ DEBIT, cũng bằng Σ CREDIT.
	Total Amount

	CreatedAt string
}

// LineView là một dòng ghi nợ hoặc ghi có.
type LineView struct {
	AccountType    string
	AccountOwnerID string

	// Direction: DEBIT hoặc CREDIT. Amount LUÔN DƯƠNG — hướng nằm ở đây,
	// không nằm ở dấu của số.
	Direction   string
	Amount      Amount
	Description string
}

// BalanceView là số dư một tài khoản.
type BalanceView struct {
	AccountType    string
	AccountOwnerID string

	// Amount là số dư theo bản chất tài khoản. Dương = bình thường.
	Amount Amount

	// TotalDebit và TotalCredit để đối chiếu khi có tranh chấp: seller hỏi
	// "vì sao số dư của tôi là X" thì trả lời được.
	TotalDebit  Amount
	TotalCredit Amount

	EntryCount int
}

// IntegrityView là kết quả rà soát toàn vẹn sổ sách.
type IntegrityView struct {
	TotalDebit  int64
	TotalCredit int64

	// Difference phải LUÔN bằng 0.
	Difference int64

	// UnbalancedEntryIDs phải LUÔN rỗng.
	UnbalancedEntryIDs []string

	CheckedEntries int

	// IsHealthy sai nghĩa là sự cố NGHIÊM TRỌNG.
	IsHealthy bool
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound  = errNotFound{}
	ErrInvalidID = errInvalidID{}

	// ErrUnbalanced khi bút toán vi phạm Σ DEBIT = Σ CREDIT.
	ErrUnbalanced = errUnbalanced{}

	// ErrInsufficientBalance khi chi trả vượt số dư đang nợ.
	ErrInsufficientBalance = errInsufficientBalance{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "payment: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "payment: định danh không hợp lệ" }

type errUnbalanced struct{}

func (errUnbalanced) Error() string {
	return "payment: bút toán không cân bằng, Σ DEBIT ≠ Σ CREDIT"
}

type errInsufficientBalance struct{}

func (errInsufficientBalance) Error() string {
	return "payment: số dư không đủ để chi trả"
}

// ---------------------------------------------------------------- Tiền tố

const (
	LedgerEntryIDPrefix = string(ids.PrefixLedgerEntry)
	PayoutIDPrefix      = string(ids.PrefixPayout)
)

// Danh mục tài khoản, để module khác tra số dư mà không cần đoán tên.
const (
	AccountPlatformCash          = "PLATFORM_CASH"
	AccountPlatformRevenue       = "PLATFORM_REVENUE"
	AccountSellerPayable         = "SELLER_PAYABLE"
	AccountCreatorPayable        = "CREATOR_PAYABLE"
	AccountCustomerRefundPayable = "CUSTOMER_REFUND_PAYABLE"
	AccountCOGS                  = "COGS"
	AccountFeeExpense            = "FEE_EXPENSE"
	AccountInventoryAsset        = "INVENTORY_ASSET"
)
