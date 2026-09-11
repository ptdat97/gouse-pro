package domain

import (
	"errors"
	"fmt"
	"strings"
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

	// KHOẢN PHẢI THU, không phải tiền mặt — ADR-0018 phần B1.
	//
	// Bút toán này ghi lúc `checkout.completed`, tức lúc khách bấm xong
	// phiên thanh toán. Tiền chưa về, kể cả với đơn trả trước (webhook tới
	// sau) và nhất là với COD (tiền về lúc giao hàng).
	//
	// `NewPaymentReceivedEntry` là bút toán chuyển khoản này thành tiền mặt.
	lines := []Line{{
		Account:     Account{Type: AccountAccountsReceivable},
		Direction:   Debit,
		Amount:      p.GrossAmount,
		Description: "Khách nợ — chưa thu được tiền",
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

// ---------------------------------------------------- Thu tiền và đảo

// PaymentReceivedParams là dữ liệu bút toán THU ĐƯỢC TIỀN.
type PaymentReceivedParams struct {
	OrderID ids.ID

	// Amount là số tiền THẬT SỰ thu được.
	//
	// Nó phải bằng tổng đơn đã ghi ở bút toán doanh thu. Bên gọi lấy con
	// số này từ event `order.paid` — chính con số `payment_intent` đã đối
	// chiếu với nhà cung cấp (ADR-0017 phần 4), nên nó đã qua một lớp
	// kiểm tra tuyệt đối trước khi tới đây.
	Amount money.Money

	IdempotencyKey string
	CreatedBy      string
	Now            time.Time
}

// NewPaymentReceivedEntry dựng bút toán chuyển KHOẢN PHẢI THU thành TIỀN MẶT.
//
//	DEBIT   PLATFORM_CASH          300.000   tiền đã về
//	CREDIT  ACCOUNTS_RECEIVABLE    300.000   khách hết nợ
//
// # Vì sao là bút toán RIÊNG, không sửa bút toán doanh thu
//
// Sổ cái bất biến (ADR-0008): bút toán đã ghi không sửa được. Và điều đó
// đúng cả về nghĩa — hai sự kiện KHÁC nhau đã xảy ra ở hai thời điểm
// khác nhau, nên sổ phải có hai dòng. Gộp lại sẽ xóa mất thông tin "khoản
// này nằm ở dạng phải thu bao lâu", thứ duy nhất trả lời được câu hỏi
// dòng tiền thực tế.
//
// Bút toán này KHÔNG đụng tới doanh thu: doanh thu đã ghi từ trước và
// không đổi. Đây thuần túy là một chuyển dịch giữa hai tài khoản TÀI SẢN.
func NewPaymentReceivedEntry(p PaymentReceivedParams) (*LedgerEntry, error) {
	if !p.Amount.IsPositive() {
		return nil, fmt.Errorf(
			"payment: số tiền thu được phải lớn hơn 0, nhận %s", p.Amount)
	}

	return NewLedgerEntry(NewEntryParams{
		Type:          EntryPaymentReceived,
		ReferenceType: "order",
		ReferenceID:   p.OrderID,
		Description:   "Thu được tiền của đơn hàng",
		Lines: []Line{
			{
				Account:     Account{Type: AccountPlatformCash},
				Direction:   Debit,
				Amount:      p.Amount,
				Description: "Tiền đã về tài khoản nền tảng",
			},
			{
				Account:     Account{Type: AccountAccountsReceivable},
				Direction:   Credit,
				Amount:      p.Amount,
				Description: "Xóa khoản khách nợ",
			},
		},
		IdempotencyKey: p.IdempotencyKey,
		CreatedBy:      p.CreatedBy,
		Now:            p.Now,
	})
}

// DaoNguocParams là dữ liệu bút toán ĐẢO.
type DaoNguocParams struct {
	// Goc là bút toán bị đảo. Mọi dòng của nó được ghi NGƯỢC CHIỀU.
	Goc *LedgerEntry

	// LyDo BẮT BUỘC và được ghi vào mô tả.
	//
	// Một bút toán đảo không có lý do là một con số biến mất khỏi sổ mà
	// không ai giải thích được — đúng thứ kiểm toán tồn tại để chặn.
	LyDo string

	IdempotencyKey string
	CreatedBy      string
	Now            time.Time
}

// NewDaoNguocEntry dựng bút toán ĐẢO một bút toán đã ghi sai.
//
// Mọi dòng đổi chiều: DEBIT thành CREDIT và ngược lại, giữ nguyên tài
// khoản và số tiền. Tổng hai bút toán cộng lại bằng 0 ở mọi tài khoản —
// đó là định nghĩa của "hủy tác dụng" trong một sổ cái không xóa được.
//
// KHÔNG tự tính lại gì: một bút toán đảo mà con số khác bút toán gốc thì
// không phải đảo, nó là một bút toán điều chỉnh khác và cần lý do khác.
func NewDaoNguocEntry(p DaoNguocParams) (*LedgerEntry, error) {
	if p.Goc == nil {
		return nil, errors.New("payment: không có bút toán gốc để đảo")
	}
	if strings.TrimSpace(p.LyDo) == "" {
		return nil, errors.New("payment: bút toán đảo BẮT BUỘC phải nêu lý do")
	}
	if p.Goc.Type() == EntryReversal {
		// Đảo một bút toán đảo là quay lại trạng thái ban đầu bằng con
		// đường vòng — và nó làm chuỗi "cái nào còn hiệu lực" không đọc
		// được nữa. Muốn ghi lại thì ghi một bút toán MỚI, không đảo ngược.
		return nil, errors.New("payment: không đảo một bút toán đảo")
	}

	goc := p.Goc.Lines()
	lines := make([]Line, 0, len(goc))
	for _, l := range goc {
		nguoc := Credit
		if l.Direction == Credit {
			nguoc = Debit
		}
		lines = append(lines, Line{
			Account:     l.Account,
			Direction:   nguoc,
			Amount:      l.Amount,
			Description: "Đảo: " + l.Description,
		})
	}

	e, err := NewLedgerEntry(NewEntryParams{
		Type:           EntryReversal,
		ReferenceType:  p.Goc.ReferenceType(),
		ReferenceID:    p.Goc.ReferenceID(),
		Description:    "Đảo bút toán " + p.Goc.ID().String() + " — " + p.LyDo,
		Lines:          lines,
		IdempotencyKey: p.IdempotencyKey,
		CreatedBy:      p.CreatedBy,
		Now:            p.Now,
	})
	if err != nil {
		return nil, err
	}
	e.reversesEntryID = p.Goc.ID()
	return e, nil
}

// ShippingRevenueParams là dữ liệu bút toán DOANH THU VẬN CHUYỂN.
type ShippingRevenueParams struct {
	OrderID ids.ID
	Fee     money.Money

	IdempotencyKey string
	CreatedBy      string
	Now            time.Time
}

// NewShippingRevenueEntry dựng bút toán cho PHÍ VẬN CHUYỂN khách trả.
//
//	DEBIT   ACCOUNTS_RECEIVABLE    30.000   khách nợ thêm khoản phí này
//	CREDIT  PLATFORM_REVENUE       30.000   nền tảng thu phí
//
// # Vì sao nó phải tồn tại
//
// Bút toán doanh thu đơn hàng chỉ ghi tổng dòng HÀNG. Phí vận chuyển
// khách trả không nằm ở đâu trong sổ cái — nền tảng thu một khoản tiền mà
// sổ sách không ghi.
//
// Lỗ hổng này vô hình cho tới ADR-0018: trước đó sổ ghi NỢ tiền mặt bằng
// tiền hàng và không có gì đối chiếu lại. Khi bút toán thu tiền ghi CÓ
// khoản phải thu đúng bằng số khách trả, phần chênh lộ ra thành số dư
// phải thu ÂM — và bài P3-3 (chuỗi đầy đủ) là chỗ nó lộ ra.
//
// # Vì sao là PLATFORM_REVENUE, và điều đó CHƯA quyết xong
//
// Nền tảng là bên thu phí của khách. Chi phí trả cho hãng vận chuyển là
// một bút toán KHÁC, ghi khi nó phát sinh — và chính bút toán đó mới nói
// ai thật sự hưởng phần chênh. Ghi vào doanh thu nền tảng ở đây KHÔNG kết
// luận điều đó; nó chỉ ngừng việc bỏ sót một khoản tiền có thật.
//
// Bút toán RIÊNG, không gộp vào bút toán doanh thu đơn: phí vận chuyển là
// khoản của CẢ ĐƠN, còn doanh thu ghi theo TỪNG nhà bán. Gộp lại sẽ phải
// chia phí cho từng bên — một câu hỏi nghiệp vụ chưa có câu trả lời.
func NewShippingRevenueEntry(p ShippingRevenueParams) (*LedgerEntry, error) {
	if !p.Fee.IsPositive() {
		return nil, fmt.Errorf(
			"payment: phí vận chuyển phải lớn hơn 0, nhận %s", p.Fee)
	}

	return NewLedgerEntry(NewEntryParams{
		Type:          EntryOrderRevenue,
		ReferenceType: "order",
		ReferenceID:   p.OrderID,
		Description:   "Phí vận chuyển khách trả",
		Lines: []Line{
			{
				Account:     Account{Type: AccountAccountsReceivable},
				Direction:   Debit,
				Amount:      p.Fee,
				Description: "Khách nợ phí vận chuyển",
			},
			{
				Account:     Account{Type: AccountPlatformRevenue},
				Direction:   Credit,
				Amount:      p.Fee,
				Description: "Doanh thu phí vận chuyển",
			},
		},
		IdempotencyKey: p.IdempotencyKey,
		CreatedBy:      p.CreatedBy,
		Now:            p.Now,
	})
}

// DiscountParams là dữ liệu bút toán KHOẢN GIẢM GIÁ.
type DiscountParams struct {
	OrderID  ids.ID
	Discount money.Money

	// SellerID là gian hàng CHỊU khoản giảm.
	//
	// Rỗng nghĩa là nền tảng chịu. Có giá trị thì khoản giảm trừ thẳng vào
	// tiền phải trả gian hàng đó — đúng thứ chương trình do nhà bán tự
	// chạy đã thỏa thuận.
	SellerID ids.ID

	IdempotencyKey string
	CreatedBy      string
	Now            time.Time
}

// NewDiscountEntry dựng bút toán cho khoản GIẢM GIÁ đã cho khách.
//
//	DEBIT   PLATFORM_REVENUE       49.000   nền tảng chịu phần giảm
//	CREDIT  ACCOUNTS_RECEIVABLE    49.000   khách bớt nợ đúng khoản đó
//
// # Vì sao nó phải tồn tại
//
// Bút toán doanh thu ghi tổng dòng hàng GỐC, còn khách chỉ trả phần đã
// trừ. Không ghi khoản giảm thì sau khi thu tiền, khoản phải thu còn dư
// đúng bằng số đã giảm — sổ cái nói khách còn nợ một khoản không ai đòi,
// và con số đó lớn dần theo mỗi đơn dùng mã.
//
// # Vì sao ghi vào PLATFORM_REVENUE, và điều đó CHƯA đầy đủ
//
// Nó khớp với thứ đang xảy ra với TIỀN hôm nay: bút toán doanh thu trả
// nhà bán theo `SellerPayable` tính từ giá GỐC, nên phần giảm thực tế do
// nền tảng gánh dù mã là của ai.
//
// Nhưng `promotion` ĐÃ có quy tắc chia khoản này — `AllocateCost` với ba
// bên chịu (nền tảng, nhà bán, chia đôi) và bất biến "tổng luôn bằng đúng
// số tiền giảm". Kết quả đó được tính rồi KHÔNG ai đọc: `CostAllocations`
// không có bên tiêu thụ nào ngoài chính module promotion.
//
// Nối nó vào đây là việc TIẾP THEO, và nó cần mở rộng cổng giữa checkout
// và promotion (cổng hiện tại vứt allocations đi). Ghi vào doanh thu nền
// tảng KHÔNG kết luận ai chịu — nó chỉ ngừng việc để một khoản tiền có
// thật nằm ngoài sổ.
func NewDiscountEntry(p DiscountParams) (*LedgerEntry, error) {
	if !p.Discount.IsPositive() {
		return nil, fmt.Errorf(
			"payment: khoản giảm giá phải lớn hơn 0, nhận %s", p.Discount)
	}

	return NewLedgerEntry(NewEntryParams{
		Type:          EntryOrderRevenue,
		ReferenceType: "order",
		ReferenceID:   p.OrderID,
		Description:   "Giảm giá cho khách",
		Lines: []Line{
			benChiuKhoanGiam(p.SellerID, p.Discount),
			{
				Account:     Account{Type: AccountAccountsReceivable},
				Direction:   Credit,
				Amount:      p.Discount,
				Description: "Khách bớt nợ phần được giảm",
			},
		},
		IdempotencyKey: p.IdempotencyKey,
		CreatedBy:      p.CreatedBy,
		Now:            p.Now,
	})
}

// benChiuKhoanGiam dựng vế GHI NỢ của bút toán giảm giá.
//
//	nhà bán chịu  → DEBIT SELLER_PAYABLE(gian hàng)  ta nợ họ ít đi
//	nền tảng chịu → DEBIT PLATFORM_REVENUE           doanh thu ta giảm
//
// Chọn sai vế này là lấy tiền của người ngoài công ty, hoặc gánh hộ một
// khoản họ đã đồng ý chịu. Sai theo hướng thứ hai thì không ai khiếu nại,
// nên nó sống rất lâu — đó là trạng thái trước ADR-0018.
func benChiuKhoanGiam(sellerID ids.ID, giam money.Money) Line {
	if sellerID.IsZero() {
		return Line{
			Account:     Account{Type: AccountPlatformRevenue},
			Direction:   Debit,
			Amount:      giam,
			Description: "Nền tảng chịu khoản giảm giá",
		}
	}
	return Line{
		Account:     Account{Type: AccountSellerPayable, OwnerID: sellerID},
		Direction:   Debit,
		Amount:      giam,
		Description: "Nhà bán chịu khoản giảm giá",
	}
}
