// Package application là các use case của việc trả hàng.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return types.Now() }

// SystemClock là đồng hồ thật.
var SystemClock Clock = systemClock{}

// OrderLine là một dòng hàng trong đơn, dưới góc nhìn của việc trả hàng.
type OrderLine struct {
	ID       ids.ID
	SKUID    ids.ID
	SellerID ids.ID
	Quantity int

	// LineTotal là giá NIÊM YẾT của dòng: đơn giá × số lượng.
	LineTotal money.Money

	// TotalAdjustments là tổng các khoản cộng/trừ đã phân bổ cho dòng này.
	// Âm với khoản giảm giá.
	TotalAdjustments money.Money

	// Commission là hoa hồng nền tảng của dòng, ĐÃ ĐÓNG BĂNG lúc đặt hàng.
	//
	// Cần để ĐẢO NGƯỢC đúng phần: hoàn tiền cho khách mà không đảo hoa
	// hồng nghĩa là nền tảng giữ lại hoa hồng của một đơn không thành.
	Commission money.Money
}

// Order là dữ liệu đơn hàng mà việc trả hàng cần.
type Order struct {
	ID ids.ID

	// Delivered: chỉ đơn ĐÃ GIAO mới trả được.
	Delivered bool

	// DeliveredAt là mốc bắt đầu đếm HẠN ĐỔI TRẢ.
	//
	// Rỗng với đơn cũ chưa có mốc này. Khi đó hạn KHÔNG được cưỡng chế —
	// phía an toàn, vì chặn một yêu cầu hợp lệ chỉ vì thiếu dữ liệu lịch
	// sử là từ chối quyền của khách do lỗi của hệ thống.
	DeliveredAt time.Time

	// OrderDiscount là số tiền giảm ghi ở CẤP ĐƠN.
	//
	// Nếu nó khác 0 mà không dòng nào mang khoản điều chỉnh tương ứng thì
	// phần giảm chưa được phân bổ — xem hàng rào ở ComputeRefund.
	OrderDiscount money.Money

	Lines []OrderLine
}

// OrderPort là cổng đọc đơn hàng.
//
// PORT do returns định nghĩa: nó hỏi đúng thứ mình cần, không phải toàn bộ
// góc nhìn đơn hàng của module order.
type OrderPort interface {
	GetOrderForReturn(ctx context.Context, orderID ids.ID) (Order, error)
}

// ReturnWindowPort cho biết khách được trả hàng trong bao lâu.
//
// Là CỔNG chứ không hằng số trong module này, vì cùng con số ấy quyết định
// khi nào `fulfillment` chuyển tiền cho nhà bán. Hai hằng số riêng nghĩa
// là sớm muộn chúng lệch nhau — và lệch ở đây là hoàn tiền cho khách sau
// khi nhà bán đã rút tiền.
type ReturnWindowPort interface {
	ReturnWindow() time.Duration
}

// InventoryPort nhận hàng hoàn về kho.
type InventoryPort interface {
	// ReceiveReturnedGoods đưa hàng vào trạng thái Returned — KHÔNG phải Available.
	//
	// Bên cài đặt tự tra bản ghi tồn kho từ (SKU, nhà bán): việc trả hàng
	// không cần biết kho nào giữ món đó.
	ReceiveReturnedGoods(ctx context.Context, skuID, sellerID ids.ID, qty int, returnID ids.ID) error

	// RecordInspection chuyển hàng từ Returned sang Available (đạt) hoặc
	// Damaged (loại).
	//
	// Đây là bước DUY NHẤT đưa hàng hoàn trở lại bán được. Không có nó,
	// hàng nằm chết ở Returned vĩnh viễn.
	RecordInspection(
		ctx context.Context, skuID, sellerID ids.ID, qty int, passed bool, reason string,
	) error
}

// PaymentPort ghi bút toán hoàn tiền.
type PaymentPort interface {
	// RecordRefund đảo ngược phần tiền của một lần trả hàng.
	//
	// Ba con số, không phải một: tổng hoàn cho khách, phần thu lại từ nhà
	// bán, và phần thu lại từ doanh thu nền tảng. Chỉ ghi tổng thì sổ vẫn
	// cân nhưng số dư nhà bán sai — và số dư ấy là thứ đem đi chi trả.
	RecordRefund(ctx context.Context, in RefundInput) error
}

// RefundInput là dữ liệu đảo ngược tài chính của một lần trả hàng.
type RefundInput struct {
	OrderID  ids.ID
	ReturnID ids.ID
	SellerID ids.ID

	// TotalRefund là số tiền trả lại khách, theo GIÁ THỰC TRẢ.
	TotalRefund money.Money

	// ReverseCommission là phần thu lại từ doanh thu nền tảng.
	ReverseCommission money.Money

	// ReverseSellerPayable là phần thu lại từ số dư nhà bán = TotalRefund − ReverseCommission.
	ReverseSellerPayable money.Money
}

// Repository là PORT cho kho lưu trữ.
type Repository interface {
	Save(ctx context.Context, y *domain.ReturnRequest) error

	// SaveWithEvent ghi yêu cầu VÀ chạy fn trong CÙNG giao dịch.
	SaveWithEvent(ctx context.Context, y *domain.ReturnRequest, fn domain.TxFunc) error
	FindByID(ctx context.Context, id ids.ID) (*domain.ReturnRequest, error)
	FindByOrder(ctx context.Context, orderID ids.ID) ([]*domain.ReturnRequest, error)
	FindBySeller(ctx context.Context, sellerID ids.ID, status string, limit int) ([]*domain.ReturnRequest, error)

	// AlreadyRequestedLines trả các order_line_id đã có yêu cầu trả CÒN HIỆU LỰC.
	//
	// Không có bước này, khách gửi hai yêu cầu cho cùng một món và được
	// hoàn tiền hai lần cho một lần mua.
	AlreadyRequestedLines(ctx context.Context, orderID ids.ID) (map[ids.ID]int, error)
}

// Service là tầng application của module returns.
type Service struct {
	repo      Repository
	orders    OrderPort
	inventory InventoryPort
	payment   PaymentPort
	window    ReturnWindowPort
	clock     Clock
	events    EventPublisher
}

// EventPublisher là cổng ra của module tới hàng đợi event.
type EventPublisher interface {
	// PublishReturnRequested phát tín hiệu khách XIN TRẢ hàng, kèm lý do.
	PublishReturnRequested(ctx context.Context, e ReturnRequested) error
}

// ReturnRequested là sự thật "khách xin trả hàng", kèm LÝ DO từng dòng.
//
// Lý do là phần quý nhất của payload. Với thời trang, "size nhỏ" lặp lại
// trên một mã hàng nghĩa là bảng size sai — dữ liệu chất lượng, không chỉ
// là chi phí.
type ReturnRequested struct {
	ReturnID    ids.ID
	OrderID     ids.ID
	CustomerID  ids.ID
	Lines       []SignalLine
	RequestedAt time.Time
}

// SignalLine là một dòng hàng bị trả, kèm lý do chuẩn hóa.
type SignalLine struct {
	SKUID      ids.ID
	Quantity   int
	ReasonCode string
}

// signalLines rút phần supply-chain cần từ yêu cầu trả hàng.
//
// Chỉ SKU, số lượng và lý do — KHÔNG mang tiền hoàn hay ghi chú của khách.
// Payload event nên chứa đủ thứ bên nhận cần và không hơn: dữ liệu thừa
// tạo cảm giác bên nhận phụ thuộc vào nhiều hơn thực tế, và khi bên phát
// bỏ một trường thì không rõ có phá gì không.
func signalLines(y *domain.ReturnRequest) []SignalLine {
	out := make([]SignalLine, 0, len(y.Lines()))
	for _, d := range y.Lines() {
		out = append(out, SignalLine{
			SKUID:      d.SKUID,
			Quantity:   d.Quantity,
			ReasonCode: string(d.ReasonCode),
		})
	}
	return out
}

type Deps struct {
	Repo Repository

	// Events có thể nil: khi đó module vẫn chạy nhưng KHÔNG phát tín hiệu
	// trả hàng, và dữ liệu chất lượng của thời trang mất vĩnh viễn.
	Events EventPublisher

	Orders    OrderPort
	Inventory InventoryPort
	Payment   PaymentPort
	Clock     Clock

	// Window có thể nil ở test không quan tâm tới hạn đổi trả. Ở production
	// thì KHÔNG được nil: thiếu nó nghĩa là hạn không được cưỡng chế, và
	// khách trả hàng sau bao lâu cũng được — kể cả sau khi nhà bán đã rút
	// tiền.
	Window ReturnWindowPort
}

func NewService(d Deps) *Service {
	c := d.Clock
	if c == nil {
		c = SystemClock
	}
	return &Service{repo: d.Repo, orders: d.Orders,
		inventory: d.Inventory, payment: d.Payment, window: d.Window, clock: c,
		events: d.Events}
}

// ReturnWindowClosed cho biết đơn đã quá hạn đổi trả chưa.
//
// # Vì sao thiếu dữ liệu thì KHÔNG chặn
//
// `DeliveredAt` rỗng với đơn tạo trước migration 000049, và cổng `window` rỗng ở
// những bản dựng chưa nối cấu hình. Cả hai trường hợp đều trả false —
// phía an toàn: chặn một yêu cầu hợp lệ vì thiếu dữ liệu lịch sử là từ
// chối quyền của khách do lỗi của hệ thống.
//
// Ngược lại thì sao? Cho qua một yêu cầu quá hạn làm nền tảng hoàn tiền
// sau khi nhà bán đã rút. Đó là mất tiền, nhưng nó CHỈ xảy ra với dữ liệu
// cũ và mất một lần; còn chặn oan thì xảy ra với mọi khách có đơn cũ.
func (s *Service) ReturnWindowClosed(ord Order, now time.Time) bool {
	if s.window == nil || ord.DeliveredAt.IsZero() {
		return false
	}
	window := s.window.ReturnWindow()
	if window <= 0 {
		return false
	}
	return now.After(ord.DeliveredAt.Add(window))
}

// ComputeRefund tính số tiền hoàn cho một dòng, theo GIÁ THỰC TRẢ.
//
// # Hàng rào chống hoàn thừa
//
// docs/07-workflows/return.md mục 5 gọi đây là điểm dễ sai nhất của cả
// luồng: hoàn theo giá niêm yết thay vì giá thực trả làm nền tảng trả ra
// nhiều hơn đã thu vào.
//
// Ví dụ của tài liệu: đơn 500.000đ giảm 50.000đ. Món A niêm yết 200.000đ
// nhưng khách chỉ trả 180.000đ. Hoàn 200.000đ là mất 20.000đ mỗi lần.
//
// Hệ thống HIỆN TẠI có `checkout.ApplyCoupon` đặt giảm giá ở cấp đơn,
// nhưng KHÔNG gì phân bổ nó xuống dòng — `promotion.AllocateDiscount` tồn
// tại và không ai gọi. Nên hàm này TỪ CHỐI khi thấy giảm giá cấp đơn mà
// dòng không mang khoản điều chỉnh nào.
//
// Từ chối và bắt xử lý tay còn hơn âm thầm trả ra số tiền sai.
func ComputeRefund(ord Order, lines OrderLine, returnQuantity int) (money.Money, error) {
	if returnQuantity <= 0 || returnQuantity > lines.Quantity {
		return money.Money{}, domain.ErrQuantityExceeded
	}

	if ord.OrderDiscount.IsPositive() && !hasAdjustments(ord) {
		return money.Money{}, domain.ErrDiscountNotAllocated
	}

	// Giá thực trả của cả dòng = niêm yết + điều chỉnh (điều chỉnh âm khi
	// là khoản giảm).
	actuallyPaid, err := lines.LineTotal.Add(lines.TotalAdjustments)
	if err != nil {
		return money.Money{}, err
	}
	if actuallyPaid.Amount() < 0 {
		return money.Money{}, fmt.Errorf(
			"returns: giá thực trả của dòng %s là số âm", lines.ID)
	}

	if returnQuantity == lines.Quantity {
		return actuallyPaid, nil
	}

	// Trả một phần: chia theo tỷ lệ, LÀM TRÒN XUỐNG.
	//
	// Làm tròn xuống là chủ ý — làm tròn lên nghĩa là trả nhiều hơn đã
	// thu, và phần chênh nhân với số lần trả hàng của cả nền tảng.
	perUnit := actuallyPaid.Amount() / int64(lines.Quantity)
	return money.New(perUnit*int64(returnQuantity), actuallyPaid.Currency())
}

// hasAdjustments cho biết CÓ ÍT NHẤT một dòng mang khoản điều chỉnh.
func hasAdjustments(ord Order) bool {
	for _, d := range ord.Lines {
		if !d.TotalAdjustments.IsZero() {
			return true
		}
	}
	return false
}

// RequestInput là yêu cầu trả hàng của khách.
type RequestInput struct {
	OrderID    ids.ID
	CustomerID ids.ID
	Note       string

	Lines []RequestedLine
}

// RequestedLine là một dòng khách xin trả, KÈM lý do của riêng nó.
type RequestedLine struct {
	OrderLineID ids.ID
	Quantity    int
	ReasonCode  domain.ReasonCode
	Detail      string
}

// Request tạo một yêu cầu trả hàng.
func (s *Service) Request(
	ctx context.Context, in RequestInput,
) (*domain.ReturnRequest, error) {
	if len(in.Lines) == 0 {
		return nil, domain.ErrNoLines
	}

	ord, err := s.orders.GetOrderForReturn(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	if !ord.Delivered {
		return nil, fmt.Errorf("%w: đơn chưa giao xong", domain.ErrInvalidStatus)
	}

	// HẠN ĐỔI TRẢ — trước hôm nay KHÔNG có kiểm tra nào ở đây.
	//
	// `Delivered` là true cho cả đơn COMPLETED, tức là đơn đã hết hạn đổi trả
	// và số dư nhà bán đã chuyển sang KHẢ DỤNG. Cho trả sau mốc đó nghĩa
	// là nền tảng hoàn tiền cho khách trong khi nhà bán đã rút được —
	// đúng thứ mà chú thích của `Order.Complete` gọi là "rất khó thu hồi".
	if s.ReturnWindowClosed(ord, s.clock.Now()) {
		return nil, domain.ErrReturnWindowClosed
	}

	alreadyRequested, err := s.repo.AlreadyRequestedLines(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}

	byID := map[ids.ID]OrderLine{}
	for _, d := range ord.Lines {
		byID[d.ID] = d
	}

	var returnLines []domain.Lines
	var sellerID ids.ID
	for _, requested := range in.Lines {
		lineID, qty := requested.OrderLineID, requested.Quantity
		d, ok := byID[lineID]
		if !ok {
			return nil, fmt.Errorf("%w: dòng %s không thuộc đơn này",
				domain.ErrNotFound, lineID)
		}
		if alreadyRequested[lineID]+qty > d.Quantity {
			return nil, domain.ErrDuplicateLine
		}

		amount, err := ComputeRefund(ord, d, qty)
		if err != nil {
			return nil, err
		}

		// MỘT yêu cầu chỉ thuộc MỘT nhà bán: người duyệt là nhà bán, và
		// một yêu cầu trải hai gian hàng thì không ai duyệt được trọn vẹn.
		if sellerID.IsZero() {
			sellerID = d.SellerID
		} else if sellerID != d.SellerID {
			return nil, fmt.Errorf(
				"returns: một yêu cầu chỉ được chứa hàng của MỘT nhà bán")
		}

		returnLines = append(returnLines, domain.Lines{
			ID:           ids.MustNew(ids.PrefixReturnLine),
			OrderLineID:  lineID,
			SKUID:        d.SKUID,
			Quantity:     qty,
			RefundAmount: amount,
			ReasonCode:   requested.ReasonCode,
			Detail:       requested.Detail,
		})
	}

	y, err := domain.New(domain.NewParams{
		OrderID: in.OrderID, SellerID: sellerID, CustomerID: in.CustomerID,
		Note: in.Note, Lines: returnLines, Now: s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	// Ghi yêu cầu VÀ phát tín hiệu nhu cầu trong CÙNG giao dịch.
	//
	// Lý do hoàn là dữ liệu CHẤT LƯỢNG: "size nhỏ" lặp lại trên một mã
	// hàng nghĩa là bảng size sai, và sửa bảng size rẻ hơn nhiều so với
	// chịu tỷ lệ hoàn cao mãi. Ghi rời nghĩa là có yêu cầu trả hàng mà lý
	// do không vào được dữ liệu ấy — và nó không tạo ngược được.
	if s.events != nil {
		if err := s.repo.SaveWithEvent(ctx, y, func(txCtx context.Context) error {
			return s.events.PublishReturnRequested(txCtx, ReturnRequested{
				ReturnID:    y.ID(),
				OrderID:     y.OrderID(),
				CustomerID:  y.CustomerID(),
				Lines:       signalLines(y),
				RequestedAt: y.RequestedAt(),
			})
		}); err != nil {
			return nil, err
		}
	} else if err := s.repo.Save(ctx, y); err != nil {
		return nil, err
	}
	return y, nil
}

// Approve nhà bán chấp nhận yêu cầu.
func (s *Service) Approve(ctx context.Context, id, sellerID ids.ID) (*domain.ReturnRequest, error) {
	return s.transition(ctx, id, sellerID, func(y *domain.ReturnRequest, now time.Time) error {
		return y.Approve(now)
	})
}

// Reject nhà bán từ chối yêu cầu, kèm lý do.
func (s *Service) Reject(
	ctx context.Context, id, sellerID ids.ID, reason string,
) (*domain.ReturnRequest, error) {
	return s.transition(ctx, id, sellerID, func(y *domain.ReturnRequest, now time.Time) error {
		return y.Reject(reason, now)
	})
}

// ReceiveAndRefund ghi nhận hàng về kho rồi hoàn tiền.
//
// # Thứ tự quan trọng
//
// Nhận hàng TRƯỚC, hoàn tiền SAU. Hoàn tiền trước rồi nhận hàng thất bại
// nghĩa là đã trả tiền cho món hàng chưa bao giờ về.
//
// Hàng vào trạng thái Returned, KHÔNG phải Available. Quy tắc bắt buộc của
// docs/07-workflows/return.md mục 4: hàng hoàn tự động thành Available
// nghĩa là bán lại hàng hỏng cho khách khác.
func (s *Service) ReceiveAndRefund(
	ctx context.Context, id, sellerID ids.ID,
) (*domain.ReturnRequest, error) {
	y, err := s.transition(ctx, id, sellerID, func(y *domain.ReturnRequest, now time.Time) error {
		return y.GoodsReceived(now)
	})
	if err != nil {
		return nil, err
	}

	for _, d := range y.Lines() {
		if err := s.inventory.ReceiveReturnedGoods(ctx, d.SKUID, y.SellerID(), d.Quantity, y.ID()); err != nil {
			return nil, fmt.Errorf("returns: nhập hàng hoàn về kho: %w", err)
		}
	}

	reverseCommission, err := s.computeCommissionReversal(ctx, y)
	if err != nil {
		return nil, err
	}
	reverseSellerPayable, err := y.RefundAmount().Sub(reverseCommission)
	if err != nil {
		return nil, err
	}

	if err := s.payment.RecordRefund(ctx, RefundInput{
		OrderID: y.OrderID(), ReturnID: y.ID(), SellerID: y.SellerID(),
		TotalRefund: y.RefundAmount(), ReverseCommission: reverseCommission, ReverseSellerPayable: reverseSellerPayable,
	}); err != nil {
		return nil, fmt.Errorf("returns: ghi sổ hoàn tiền: %w", err)
	}

	// ĐỌC LẠI trước lượt ghi cuối.
	//
	// `transition` ở trên đã ghi một lần, nên phiên bản dưới database đã tăng còn
	// bản trong bộ nhớ thì chưa. Ghi tiếp bằng bản cũ sẽ xung đột phiên
	// bản với chính mình.
	//
	// Cùng cách module fulfillment làm: mỗi lượt đổi trạng thái bắt đầu
	// bằng một lượt đọc.
	after, err := s.repo.FindByID(ctx, y.ID())
	if err != nil {
		return nil, err
	}
	if err := after.Refunded(s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, after); err != nil {
		return nil, err
	}
	return after, nil
}

// computeCommissionReversal tính phần hoa hồng phải thu lại.
//
// Theo TỶ LỆ phần đã hoàn trên tổng dòng: trả một nửa số lượng thì đảo một
// nửa hoa hồng. Làm tròn XUỐNG, cùng lý do với tiền hoàn — làm tròn lên ở
// đây nghĩa là thu lại của nhà bán nhiều hơn phần họ thực sự mất.
func (s *Service) computeCommissionReversal(
	ctx context.Context, y *domain.ReturnRequest,
) (money.Money, error) {
	ord, err := s.orders.GetOrderForReturn(ctx, y.OrderID())
	if err != nil {
		return money.Money{}, err
	}

	byID := map[ids.ID]OrderLine{}
	for _, d := range ord.Lines {
		byID[d.ID] = d
	}

	total := money.Money{}
	for _, d := range y.Lines() {
		original, ok := byID[d.OrderLineID]
		if !ok || original.Quantity == 0 || original.Commission.IsZero() {
			continue
		}
		share := original.Commission.Amount() * int64(d.Quantity) / int64(original.Quantity)
		m, err := money.New(share, original.Commission.Currency())
		if err != nil {
			return money.Money{}, err
		}
		if total.IsZero() {
			total = m
			continue
		}
		if total, err = total.Add(m); err != nil {
			return money.Money{}, err
		}
	}
	if total.IsZero() {
		return money.New(0, y.RefundAmount().Currency())
	}
	return total, nil
}

// transition đọc–sửa–ghi một yêu cầu, kèm kiểm CHỦ SỞ HỮU.
func (s *Service) transition(
	ctx context.Context, id, sellerID ids.ID,
	step func(*domain.ReturnRequest, time.Time) error,
) (*domain.ReturnRequest, error) {
	y, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// KHÔNG phân biệt "không tồn tại" với "của nhà bán khác": phân biệt
	// hai trường hợp cho phép dò mã yêu cầu của đối thủ.
	if !sellerID.IsZero() && y.SellerID() != sellerID {
		return nil, domain.ErrNotFound
	}
	if err := step(y, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, y); err != nil {
		return nil, err
	}
	return y, nil
}

// ListByOrder trả các yêu cầu trả hàng của một đơn.
func (s *Service) ListByOrder(ctx context.Context, orderID ids.ID) ([]*domain.ReturnRequest, error) {
	return s.repo.FindByOrder(ctx, orderID)
}

// ListBySeller trả các yêu cầu của một gian hàng.
func (s *Service) ListBySeller(
	ctx context.Context, sellerID ids.ID, status string, limit int,
) ([]*domain.ReturnRequest, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.FindBySeller(ctx, sellerID, status, limit)
}

// LineInspectionOutcome là kết quả kiểm định một dòng hàng hoàn.
type LineInspectionOutcome struct {
	OrderLineID ids.ID
	Passed      bool
	Note        string
}

// Inspection ghi kết quả kiểm định và đưa hàng ra khỏi trạng thái Returned.
//
// # Thứ tự: LƯU TRẠNG THÁI TRƯỚC, ghi tồn kho SAU
//
// Hai module, không có giao dịch chung, nên một trong hai lượt ghi có thể
// hỏng. Hai thứ tự cho hai kiểu hỏng:
//
//	tồn kho trước → hỏng ở bước lưu: hàng ĐÃ chuyển, dòng vẫn PENDING,
//	                lần thử lại chuyển TIẾP → hàng ma trong kho
//	lưu trước     → hỏng ở bước tồn kho: dòng ghi "đã kiểm", hàng còn kẹt
//	                ở Returned → hàng chết, nhưng KHÔNG có hàng ma
//
// Chọn thứ hai. Hàng chết là mất mát đếm được và sửa tay được; hàng ma là
// bán thứ không tồn tại, và nó chỉ lộ ra khi khách đặt rồi không giao được.
//
// Thứ tự đầu là thứ tôi viết trước, và phép kiểm chứng bằng cách phá đã
// lộ ra nó: bỏ ràng buộc lý do khiến lượt lưu hỏng, và tồn kho đã chuyển
// mất rồi.
//
// Rủi ro còn lại: dòng "đã kiểm" mà hàng chưa chuyển thì KHÔNG có gì tự
// phát hiện. Cần một truy vấn đối chiếu định kỳ — chưa có, ghi ở backlog.
func (s *Service) Inspection(
	ctx context.Context, id, sellerID ids.ID, outcome []LineInspectionOutcome) (*domain.ReturnRequest, error) {
	if len(outcome) == 0 {
		return nil, domain.ErrNoLines
	}

	y, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !sellerID.IsZero() && y.SellerID() != sellerID {
		return nil, domain.ErrNotFound
	}

	byLine := map[ids.ID]domain.Lines{}
	for _, d := range y.Lines() {
		byLine[d.OrderLineID] = d
	}

	now := s.clock.Now()
	for _, result := range outcome {
		if _, ok := byLine[result.OrderLineID]; !ok {
			return nil, domain.ErrNotFound
		}

		// Hàng rào chống kiểm hai lần nằm ở đây, TRƯỚC mọi lượt ghi.
		if err := y.RecordInspection(result.OrderLineID, result.Passed, result.Note, now); err != nil {
			return nil, err
		}

	}

	// LƯU TRƯỚC: nếu bước này hỏng thì chưa có gì chuyển trong kho.
	if err := s.repo.Save(ctx, y); err != nil {
		return nil, err
	}

	for _, result := range outcome {
		lines := byLine[result.OrderLineID]
		if err := s.inventory.RecordInspection(ctx, lines.SKUID, y.SellerID(),
			lines.Quantity, result.Passed, result.Note); err != nil {
			return nil, fmt.Errorf("returns: ghi kết quả kiểm định vào tồn kho: %w", err)
		}
	}
	return y, nil
}
