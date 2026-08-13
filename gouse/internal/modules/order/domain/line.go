package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// Line là MỘT DÒNG HÀNG trong đơn — mọi thông tin giao dịch ĐÓNG BĂNG.
//
// NGUYÊN TẮC ĐÓNG BĂNG (quy tắc 1, nguyên tắc P9) là quy tắc quan trọng
// nhất của module này:
//
//	Trường              | Nếu tham chiếu động        | Nếu đóng băng
//	--------------------|----------------------------|------------------
//	product_name        | Seller đổi tên → hóa đơn sai| Hóa đơn luôn đúng
//	unit_price          | Giá đổi → tổng tiền lệch   | Nhất quán
//	commission_rate     | Đổi chính sách → đối soát  | Kiểm toán được
//	                    | tháng trước ra số khác     |
//	variant_description | Sửa variant → không biết   | Truy vết được
//	                    | khách mua size nào         |
//
// KIỂM CHỨNG BẰNG TÌNH HUỐNG THỰC TẾ (mục 4 của đặc tả):
//
//	10/08: Khách mua áo 299.000đ, hoa hồng 10%
//	15/08: Seller giảm giá còn 249.000đ
//	20/08: Nền tảng đổi chính sách hoa hồng thành 12%
//	25/08: Chạy đối soát cho kỳ 01–15/08
//
//	Tham chiếu động: 249.000 × 12% = 29.880đ    ← SAI
//	Đóng băng:       299.000 × 10% = 29.900đ    ← ĐÚNG
//
// Sai lệch này không chỉ là con số — nó phá vỡ niềm tin của seller và
// không giải thích được khi có tranh chấp.
type Line struct {
	id ids.ID

	// offerID là thứ khách THỰC SỰ mua: một lời chào bán cụ thể của một
	// seller với giá cụ thể.
	offerID ids.ID

	// skuID và sellerID sao chép lại để truy vấn nhanh, không phải JOIN
	// ngược sang module khác.
	skuID    ids.ID
	sellerID ids.ID

	// ---- Các trường ĐÓNG BĂNG ----

	productName        string
	variantDescription string
	unitPrice          money.Money

	quantity int

	// commissionRate và commissionAmount ĐÓNG BĂNG tại thời điểm đặt hàng.
	//
	// Đây là chỗ quan trọng nhất: đối soát tháng trước phải ra CÙNG một
	// con số dù chính sách hoa hồng đã đổi.
	commissionRate   types.BasisPoints
	commissionAmount money.Money

	// Quy kết creator, cũng đóng băng.
	attributedCreatorID   ids.ID
	creatorCommissionRate types.BasisPoints

	status LineStatus

	// adjustments là mọi khoản cộng/trừ trên dòng này.
	adjustments []Adjustment

	cancelledAt time.Time
	createdAt   time.Time
	updatedAt   time.Time
}

type NewLineParams struct {
	OfferID  ids.ID
	SKUID    ids.ID
	SellerID ids.ID

	ProductName        string
	VariantDescription string
	UnitPrice          money.Money
	Quantity           int

	CommissionRate types.BasisPoints

	AttributedCreatorID   ids.ID
	CreatorCommissionRate types.BasisPoints

	Now time.Time
}

// NewLine tạo một dòng hàng với dữ liệu ĐÃ ĐÓNG BĂNG.
//
// Hoa hồng được TÍNH NGAY và lưu lại, không tính lúc đọc: tính lúc đọc
// nghĩa là tham chiếu động, và đối soát sẽ ra số khác khi chính sách đổi.
func NewLine(p NewLineParams) (*Line, error) {
	if p.OfferID.IsZero() {
		return nil, errors.New("order: dòng hàng phải trỏ tới một offer")
	}
	if p.Quantity <= 0 {
		return nil, errors.New("order: số lượng phải lớn hơn 0")
	}
	if !p.UnitPrice.IsPositive() {
		return nil, errors.New("order: đơn giá phải lớn hơn 0")
	}
	if strings.TrimSpace(p.ProductName) == "" {
		return nil, errors.New("order: phải đóng băng tên sản phẩm")
	}

	id, err := ids.New(ids.PrefixOrderLine)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	lineTotal := p.UnitPrice.MulQuantity(int64(p.Quantity))

	return &Line{
		id:                    id,
		offerID:               p.OfferID,
		skuID:                 p.SKUID,
		sellerID:              p.SellerID,
		productName:           strings.TrimSpace(p.ProductName),
		variantDescription:    strings.TrimSpace(p.VariantDescription),
		unitPrice:             p.UnitPrice,
		quantity:              p.Quantity,
		commissionRate:        p.CommissionRate,
		commissionAmount:      lineTotal.ApplyRate(p.CommissionRate, money.RoundHalfUp),
		attributedCreatorID:   p.AttributedCreatorID,
		creatorCommissionRate: p.CreatorCommissionRate,
		status:                LineActive,
		createdAt:             now,
		updatedAt:             now,
	}, nil
}

// RestoreLineParams dựng lại từ kho lưu trữ.
type RestoreLineParams struct {
	ID                    ids.ID
	OfferID               ids.ID
	SKUID                 ids.ID
	SellerID              ids.ID
	ProductName           string
	VariantDescription    string
	UnitPrice             money.Money
	Quantity              int
	CommissionRate        types.BasisPoints
	CommissionAmount      money.Money
	AttributedCreatorID   ids.ID
	CreatorCommissionRate types.BasisPoints
	Status                LineStatus
	Adjustments           []Adjustment
	CancelledAt           time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// RestoreLine dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreLine(p RestoreLineParams) *Line {
	return &Line{
		id:                    p.ID,
		offerID:               p.OfferID,
		skuID:                 p.SKUID,
		sellerID:              p.SellerID,
		productName:           p.ProductName,
		variantDescription:    p.VariantDescription,
		unitPrice:             p.UnitPrice,
		quantity:              p.Quantity,
		commissionRate:        p.CommissionRate,
		commissionAmount:      p.CommissionAmount,
		attributedCreatorID:   p.AttributedCreatorID,
		creatorCommissionRate: p.CreatorCommissionRate,
		status:                p.Status,
		adjustments:           p.Adjustments,
		cancelledAt:           p.CancelledAt,
		createdAt:             p.CreatedAt,
		updatedAt:             p.UpdatedAt,
	}
}

func (l *Line) ID() ids.ID                        { return l.id }
func (l *Line) OfferID() ids.ID                   { return l.offerID }
func (l *Line) SKUID() ids.ID                     { return l.skuID }
func (l *Line) SellerID() ids.ID                  { return l.sellerID }
func (l *Line) ProductName() string               { return l.productName }
func (l *Line) VariantDescription() string        { return l.variantDescription }
func (l *Line) UnitPrice() money.Money            { return l.unitPrice }
func (l *Line) Quantity() int                     { return l.quantity }
func (l *Line) CommissionRate() types.BasisPoints { return l.commissionRate }
func (l *Line) CommissionAmount() money.Money     { return l.commissionAmount }
func (l *Line) AttributedCreatorID() ids.ID       { return l.attributedCreatorID }
func (l *Line) CreatorCommissionRate() types.BasisPoints {
	return l.creatorCommissionRate
}
func (l *Line) Status() LineStatus     { return l.status }
func (l *Line) CancelledAt() time.Time { return l.cancelledAt }
func (l *Line) CreatedAt() time.Time   { return l.createdAt }
func (l *Line) UpdatedAt() time.Time   { return l.updatedAt }

// LineTotal là tiền hàng của dòng: đơn giá × số lượng.
func (l *Line) LineTotal() money.Money {
	return l.unitPrice.MulQuantity(int64(l.quantity))
}

// SellerPayable là số tiền phải trả seller cho dòng này.
//
//	tiền hàng − hoa hồng nền tảng
//
// Dùng số ĐÃ ĐÓNG BĂNG, nên đối soát ra cùng kết quả dù chính sách đã đổi.
func (l *Line) SellerPayable() money.Money {
	payable, _ := l.LineTotal().Sub(l.commissionAmount)
	return payable
}

// Adjustments trả bản sao danh sách khoản điều chỉnh.
func (l *Line) Adjustments() []Adjustment {
	return append([]Adjustment(nil), l.adjustments...)
}

// AddAdjustment thêm một khoản cộng/trừ vào dòng hàng.
func (l *Line) AddAdjustment(a Adjustment, now time.Time) error {
	if err := a.validate(); err != nil {
		return err
	}
	l.adjustments = append(l.adjustments, a)
	l.touch(now)
	return nil
}

// AdjustmentTotal là tổng các khoản điều chỉnh (âm = giảm, dương = tăng).
func (l *Line) AdjustmentTotal() money.Money {
	sum := money.Zero(l.unitPrice.Currency())
	for _, a := range l.adjustments {
		sum, _ = sum.Add(a.Amount)
	}
	return sum
}

// cancel đánh dấu dòng đã hủy. Không xuất khẩu: chỉ Order được gọi, để
// trạng thái đơn và trạng thái dòng luôn nhất quán.
func (l *Line) cancel(now time.Time) {
	l.status = LineCancelled
	l.cancelledAt = now
	l.touch(now)
}

// MarkReturned đánh dấu dòng đã được trả hàng.
func (l *Line) MarkReturned(now time.Time) error {
	if l.status != LineActive {
		return ErrNotCancellable
	}
	l.status = LineReturned
	l.touch(now)
	return nil
}

func (l *Line) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	l.updatedAt = now
}

// ---------------------------------------------------------------- Adjustment

// AdjustmentType là loại khoản cộng/trừ.
type AdjustmentType string

const (
	AdjustmentPromotion  AdjustmentType = "PROMOTION"
	AdjustmentTax        AdjustmentType = "TAX"
	AdjustmentShipping   AdjustmentType = "SHIPPING"
	AdjustmentCommission AdjustmentType = "COMMISSION"
	AdjustmentFee        AdjustmentType = "FEE"
	AdjustmentManual     AdjustmentType = "MANUAL"
)

// CostBearer cho biết AI chịu chi phí của khoản điều chỉnh.
//
// Bổ sung cho marketplace (nguồn: Sylius + TikTok Shop, xem
// docs/11-oss/synthesis.md). Không có trường này thì không đối soát được
// "tổng giảm giá do seller chịu trong kỳ này là bao nhiêu".
type CostBearer string

const (
	BearerPlatform CostBearer = "PLATFORM"
	BearerSeller   CostBearer = "SELLER"
	BearerShared   CostBearer = "SHARED"
)

// Adjustment là MỘT KHOẢN cộng/trừ trên dòng hàng — THỰC THỂ HẠNG NHẤT.
//
// VÌ SAO LÀ THỰC THỂ CHỨ KHÔNG PHẢI TRƯỜNG (entities.md mục 2.10):
//
//	Nếu là trường (discount_amount)  | Nếu là thực thể
//	---------------------------------|---------------------------
//	Không biết giảm giá từ đâu ra    | Truy vết tới quy tắc cụ thể
//	Không biết ai chịu chi phí       | cost_bearer rõ ràng
//	Hoàn từng phần tính sai          | Gắn từng dòng → tính đúng
//	Tính lại giỏ dễ sót/trùng        | Xóa hết rồi tính lại, an toàn
//	Không giải thích được cho khách  | Liệt kê được từng khoản
//
// VÍ DỤ về hoàn tiền từng phần:
//
//	Đơn 3 món, tổng 500.000đ, giảm 50.000đ (10%)
//
//	Không có Adjustment: order.discount_amount = 50000
//	  → khách trả món C (100.000đ), hoàn bao nhiêu?
//	  → phải tính lại tỷ lệ, dễ sai
//
//	Có Adjustment (phân bổ khi đặt hàng):
//	  OrderLine A → −20.000, B → −20.000, C → −10.000
//	  → khách trả món C, hoàn 100.000 − 10.000 = 90.000đ
//	  → ĐỌC TRỰC TIẾP, không tính lại
type Adjustment struct {
	ID   ids.ID
	Type AdjustmentType

	// Label là nhãn hiển thị cho khách: "Giảm giá THUDONG20".
	Label string

	// Amount: ÂM là giảm, DƯƠNG là tăng.
	//
	// Đây là chỗ DUY NHẤT trong hệ thống cho phép số tiền âm, và có lý do:
	// một khoản điều chỉnh vốn dĩ có hai chiều, và tách thành hai loại
	// (giảm/tăng) sẽ nhân đôi số nhánh xử lý ở mọi nơi cộng dồn.
	Amount money.Money

	// SourceType và SourceID trỏ tới nguồn gốc (quy tắc khuyến mãi, quy
	// tắc thuế). Tham chiếu vượt module — chỉ giữ định danh.
	SourceType string
	SourceID   ids.ID

	CostBearer CostBearer

	CreatedAt time.Time
}

// NewAdjustment tạo một khoản điều chỉnh.
func NewAdjustment(
	t AdjustmentType, label string, amount money.Money,
	sourceType string, sourceID ids.ID, bearer CostBearer, now time.Time,
) (Adjustment, error) {
	id, err := ids.New(ids.PrefixAdjustment)
	if err != nil {
		return Adjustment{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if bearer == "" {
		bearer = BearerPlatform
	}

	a := Adjustment{
		ID:         id,
		Type:       t,
		Label:      strings.TrimSpace(label),
		Amount:     amount,
		SourceType: sourceType,
		SourceID:   sourceID,
		CostBearer: bearer,
		CreatedAt:  now,
	}
	if err := a.validate(); err != nil {
		return Adjustment{}, err
	}
	return a, nil
}

func (a Adjustment) validate() error {
	if a.Amount.IsZero() {
		return errors.New("order: khoản điều chỉnh bằng 0 không có tác dụng")
	}
	if strings.TrimSpace(a.Label) == "" {
		// Nhãn là thứ khách nhìn thấy. Không có nhãn thì hóa đơn hiện một
		// khoản trừ vô danh — khách không hiểu và sẽ khiếu nại.
		return errors.New("order: khoản điều chỉnh phải có nhãn hiển thị")
	}
	switch a.CostBearer {
	case BearerPlatform, BearerSeller, BearerShared:
	default:
		return errors.New("order: bên chịu chi phí không hợp lệ: " + string(a.CostBearer))
	}
	return nil
}

// IsDiscount cho biết đây có phải khoản giảm không.
func (a Adjustment) IsDiscount() bool { return a.Amount.IsNegative() }
