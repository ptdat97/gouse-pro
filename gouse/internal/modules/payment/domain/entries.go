package domain

import (
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// OrderRevenueParams là dữ liệu ghi sổ doanh thu một đơn hàng.
//
// Mọi số tiền đã được ĐÓNG BĂNG ở module order tại thời điểm đặt hàng —
// module này chỉ ghi sổ, không tính lại. Tính lại sẽ ra số khác khi chính
// sách hoa hồng đổi, và đối soát tháng trước sẽ sai.
type OrderRevenueParams struct {
	OrderID ids.ID

	// GrossAmount là tổng tiền khách trả.
	GrossAmount money.Money

	// SellerID rỗng nghĩa là đơn OWN BRAND (seller nội bộ).
	SellerID ids.ID

	// SellerPayable là số tiền phải trả seller sau khi trừ hoa hồng.
	// Bỏ qua với đơn own brand.
	SellerPayable money.Money

	// PlatformRevenue là doanh thu của nền tảng.
	//
	// Đơn marketplace: chỉ HOA HỒNG.
	// Đơn own brand: TOÀN BỘ tiền hàng.
	//
	// Đây là chỗ phân biệt GMV với doanh thu ngay ở tầng ghi sổ.
	PlatformRevenue money.Money

	// CreatorID và CreatorPayable cho hoa hồng creator. Bỏ qua nếu không có.
	CreatorID      ids.ID
	CreatorPayable money.Money

	// PaymentFee là phí cổng thanh toán.
	PaymentFee money.Money

	IdempotencyKey string
	CreatedBy      string
	Now            time.Time
}

// NewOrderRevenueEntry dựng bút toán ghi doanh thu một đơn hàng.
//
// ĐƠN MARKETPLACE (mục 4.3 của đặc tả):
//
//	DEBIT   PLATFORM_CASH                     300.000
//	CREDIT  SELLER_PAYABLE (seller A)         250.500
//	CREDIT  PLATFORM_REVENUE                   30.000
//	CREDIT  CREATOR_PAYABLE (creator X)        15.000
//	CREDIT  FEE_EXPENSE                         4.500
//
// ĐƠN OWN BRAND (mục 4.4): doanh thu ghi TOÀN PHẦN, không có SELLER_PAYABLE.
//
// Hàm này KHÔNG tự tính số tiền — mọi con số do bên gọi truyền vào từ dữ
// liệu đã đóng băng. Nó chỉ dựng bút toán và để constructor kiểm tra cân bằng.
func NewOrderRevenueEntry(p OrderRevenueParams) (*LedgerEntry, error) {
	if !p.GrossAmount.IsPositive() {
		return nil, fmt.Errorf("payment: tổng tiền đơn phải lớn hơn 0, nhận %s", p.GrossAmount)
	}

	lines := []Line{{
		Account:     Account{Type: AccountPlatformCash},
		Direction:   Debit,
		Amount:      p.GrossAmount,
		Description: "Tiền khách thanh toán",
	}}

	// Đơn own brand không có seller payable: tiền thuộc về nền tảng.
	if !p.SellerID.IsZero() && p.SellerPayable.IsPositive() {
		lines = append(lines, Line{
			Account:     Account{Type: AccountSellerPayable, OwnerID: p.SellerID},
			Direction:   Credit,
			Amount:      p.SellerPayable,
			Description: "Phải trả nhà bán",
		})
	}

	if p.PlatformRevenue.IsPositive() {
		lines = append(lines, Line{
			Account:     Account{Type: AccountPlatformRevenue},
			Direction:   Credit,
			Amount:      p.PlatformRevenue,
			Description: "Doanh thu nền tảng",
		})
	}

	if !p.CreatorID.IsZero() && p.CreatorPayable.IsPositive() {
		lines = append(lines, Line{
			Account:     Account{Type: AccountCreatorPayable, OwnerID: p.CreatorID},
			Direction:   Credit,
			Amount:      p.CreatorPayable,
			Description: "Hoa hồng creator",
		})
	}

	if p.PaymentFee.IsPositive() {
		lines = append(lines, Line{
			Account:     Account{Type: AccountFeeExpense},
			Direction:   Credit,
			Amount:      p.PaymentFee,
			Description: "Phí cổng thanh toán",
		})
	}

	return NewLedgerEntry(NewEntryParams{
		Type:           EntryOrderRevenue,
		ReferenceType:  "ORDER",
		ReferenceID:    p.OrderID,
		Description:    "Ghi nhận doanh thu đơn hàng",
		IdempotencyKey: p.IdempotencyKey,
		Lines:          lines,
		CreatedBy:      p.CreatedBy,
		Now:            p.Now,
	})
}

// NewCOGSEntry dựng bút toán ghi giá vốn hàng bán — CHỈ cho đơn own brand.
//
//	DEBIT   COGS               120.000
//	CREDIT  INVENTORY_ASSET    120.000
//
// Đơn marketplace KHÔNG có bút toán này: hàng không phải tài sản của nền tảng.
func NewCOGSEntry(
	orderID ids.ID, cogs money.Money, idempotencyKey, createdBy string, now time.Time,
) (*LedgerEntry, error) {
	if !cogs.IsPositive() {
		return nil, fmt.Errorf("payment: giá vốn phải lớn hơn 0, nhận %s", cogs)
	}

	return NewLedgerEntry(NewEntryParams{
		Type:          EntryCOGS,
		ReferenceType: "ORDER",
		ReferenceID:   orderID,
		Description:   "Ghi nhận giá vốn hàng bán",
		Lines: []Line{
			{
				Account:     Account{Type: AccountCOGS},
				Direction:   Debit,
				Amount:      cogs,
				Description: "Giá vốn hàng bán",
			},
			{
				Account:     Account{Type: AccountInventoryAsset},
				Direction:   Credit,
				Amount:      cogs,
				Description: "Giảm giá trị hàng tồn kho",
			},
		},
		IdempotencyKey: idempotencyKey,
		CreatedBy:      createdBy,
		Now:            now,
	})
}

// RefundParams là dữ liệu ghi sổ hoàn tiền.
type RefundParams struct {
	OrderID  ids.ID
	RefundID ids.ID

	// Amount là số tiền hoàn cho khách.
	Amount money.Money

	// SellerID và SellerClawback là phần thu hồi lại từ seller.
	SellerID       ids.ID
	SellerClawback money.Money

	// PlatformClawback là phần hoa hồng nền tảng phải trả lại.
	PlatformClawback money.Money

	IdempotencyKey string
	CreatedBy      string
	Now            time.Time
}

// NewRefundEntry dựng bút toán hoàn tiền — ĐẢO NGƯỢC chuỗi ghi sổ ban đầu.
//
//	DEBIT   SELLER_PAYABLE      (thu hồi từ seller)
//	DEBIT   PLATFORM_REVENUE    (trả lại hoa hồng)
//	CREDIT  CUSTOMER_REFUND_PAYABLE
//
// Ghi vào CUSTOMER_REFUND_PAYABLE chứ không trừ thẳng PLATFORM_CASH: tiền
// chưa thực sự rời khỏi nền tảng cho tới khi hoàn thành công. Hai bước rõ
// ràng hơn một bước gộp.
func NewRefundEntry(p RefundParams) (*LedgerEntry, error) {
	if !p.Amount.IsPositive() {
		return nil, fmt.Errorf("payment: số tiền hoàn phải lớn hơn 0, nhận %s", p.Amount)
	}

	var lines []Line

	if !p.SellerID.IsZero() && p.SellerClawback.IsPositive() {
		lines = append(lines, Line{
			Account:     Account{Type: AccountSellerPayable, OwnerID: p.SellerID},
			Direction:   Debit,
			Amount:      p.SellerClawback,
			Description: "Thu hồi từ nhà bán do hoàn hàng",
		})
	}
	if p.PlatformClawback.IsPositive() {
		lines = append(lines, Line{
			Account:     Account{Type: AccountPlatformRevenue},
			Direction:   Debit,
			Amount:      p.PlatformClawback,
			Description: "Hoàn lại hoa hồng do hoàn hàng",
		})
	}

	lines = append(lines, Line{
		Account:     Account{Type: AccountCustomerRefundPayable, OwnerID: p.OrderID},
		Direction:   Credit,
		Amount:      p.Amount,
		Description: "Phải hoàn tiền khách",
	})

	ref := p.RefundID
	if ref.IsZero() {
		ref = p.OrderID
	}

	return NewLedgerEntry(NewEntryParams{
		Type:           EntryRefund,
		ReferenceType:  "REFUND",
		ReferenceID:    ref,
		Description:    "Hoàn tiền khách hàng",
		IdempotencyKey: p.IdempotencyKey,
		Lines:          lines,
		CreatedBy:      p.CreatedBy,
		Now:            p.Now,
	})
}

// NewPayoutEntry dựng bút toán chi trả cho seller.
//
//	DEBIT   SELLER_PAYABLE   (giảm khoản phải trả)
//	CREDIT  PLATFORM_CASH    (tiền rời khỏi nền tảng)
func NewPayoutEntry(
	payoutID, sellerID ids.ID, amount money.Money,
	idempotencyKey, createdBy string, now time.Time,
) (*LedgerEntry, error) {
	if !amount.IsPositive() {
		return nil, fmt.Errorf("payment: số tiền chi trả phải lớn hơn 0, nhận %s", amount)
	}
	if sellerID.IsZero() {
		return nil, fmt.Errorf("payment: chi trả phải ghi rõ nhà bán")
	}

	return NewLedgerEntry(NewEntryParams{
		Type:          EntryPayout,
		ReferenceType: "PAYOUT",
		ReferenceID:   payoutID,
		Description:   "Chi trả cho nhà bán",
		Lines: []Line{
			{
				Account:     Account{Type: AccountSellerPayable, OwnerID: sellerID},
				Direction:   Debit,
				Amount:      amount,
				Description: "Giảm khoản phải trả nhà bán",
			},
			{
				Account:     Account{Type: AccountPlatformCash},
				Direction:   Credit,
				Amount:      amount,
				Description: "Tiền chuyển ra khỏi nền tảng",
			},
		},
		IdempotencyKey: idempotencyKey,
		CreatedBy:      createdBy,
		Now:            now,
	})
}

// NewAdjustmentEntry dựng bút toán ĐIỀU CHỈNH khi phát hiện ghi sai.
//
// Đây là cách DUY NHẤT sửa sai trong sổ cái bất biến. Ví dụ (ADR-0008):
//
//	Phát hiện ghi nhầm hoa hồng 30.000đ, đúng phải 25.000đ
//
//	SAI:  UPDATE ledger_line SET amount = 25000
//	      → không ai biết đã từng ghi 30.000đ
//
//	ĐÚNG: Bút toán ADJUSTMENT
//	        DEBIT   PLATFORM_REVENUE   5.000
//	        CREDIT  SELLER_PAYABLE     5.000
//	      → kết quả giống nhau, nhưng GIỮ ĐƯỢC LỊCH SỬ
//
// `reason` là BẮT BUỘC: điều chỉnh không lý do là điểm mù trong kiểm toán.
func NewAdjustmentEntry(
	referenceType string, referenceID ids.ID, lines []Line,
	reason, idempotencyKey, createdBy string, now time.Time,
) (*LedgerEntry, error) {
	if reason == "" {
		return nil, fmt.Errorf("payment: bút toán điều chỉnh bắt buộc phải nêu lý do")
	}

	return NewLedgerEntry(NewEntryParams{
		Type:           EntryAdjustment,
		ReferenceType:  referenceType,
		ReferenceID:    referenceID,
		Description:    reason,
		IdempotencyKey: idempotencyKey,
		Lines:          lines,
		CreatedBy:      createdBy,
		Now:            now,
	})
}

// SellerReleaseParams là dữ liệu chuyển tiền nhà bán sang trạng thái rút được.
type SellerReleaseParams struct {
	FulfillmentID ids.ID
	SellerID      ids.ID
	Amount        money.Money

	IdempotencyKey string
	CreatedBy      string
	Now            time.Time
}

// NewSellerReleaseEntry dựng bút toán chuyển ĐANG CHỜ → RÚT ĐƯỢC.
//
//	DEBIT   SELLER_PAYABLE    (giảm nợ đang chờ)
//	CREDIT  SELLER_AVAILABLE  (tăng nợ rút được)
//
// Tổng nợ phải trả nhà bán KHÔNG đổi — tiền chỉ đổi chỗ. Đó là lý do bút
// toán này luôn cân bằng theo đúng nghĩa đen.
func NewSellerReleaseEntry(p SellerReleaseParams) (*LedgerEntry, error) {
	if !p.Amount.IsPositive() {
		return nil, fmt.Errorf(
			"payment: số tiền chuyển sang rút được phải lớn hơn 0, nhận %s", p.Amount)
	}
	if p.SellerID.IsZero() {
		return nil, fmt.Errorf("payment: thiếu định danh nhà bán")
	}

	return NewLedgerEntry(NewEntryParams{
		Type:          EntrySellerRelease,
		ReferenceType: "FULFILLMENT_ORDER",
		ReferenceID:   p.FulfillmentID,
		Description:   "Hết hạn đổi trả — tiền nhà bán chuyển sang rút được",
		Lines: []Line{
			{
				Account:     Account{Type: AccountSellerPayable, OwnerID: p.SellerID},
				Direction:   Debit,
				Amount:      p.Amount,
				Description: "Giảm phải trả đang chờ",
			},
			{
				Account:     Account{Type: AccountSellerAvailable, OwnerID: p.SellerID},
				Direction:   Credit,
				Amount:      p.Amount,
				Description: "Tăng phải trả rút được",
			},
		},
		IdempotencyKey: p.IdempotencyKey,
		CreatedBy:      p.CreatedBy,
		Now:            p.Now,
	})
}
