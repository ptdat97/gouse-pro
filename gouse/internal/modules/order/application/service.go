// Package application chứa các use case của module order.
//
// Use case quan trọng nhất là PlaceOrder: nó biến giỏ hàng thành HỢP ĐỒNG
// với khách, đóng băng mọi con số, và TÁCH thành các đơn vị công việc theo
// nguồn hàng — ba việc phải cùng thành công hoặc cùng không.
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// ErrForbidden khi seller thao tác trên đơn thực hiện không phải của mình.
var ErrForbidden = errors.New("order: đơn thực hiện không thuộc về nhà bán này")

// AuditRecorder ghi vết kiểm toán cho thao tác nhạy cảm và cho việc ĐỌC dữ
// liệu cá nhân khách hàng.
//
// Là PORT do tầng application định nghĩa nên nó không biết database.
type AuditRecorder interface {
	// RecordOrderView ghi việc nhân viên XEM chi tiết đơn.
	//
	// docs/06-api/admin-api.md mục 6: mọi truy cập dữ liệu cá nhân khách
	// hàng đều ghi audit. Chi tiết đơn chứa tên người nhận, số điện thoại
	// và địa chỉ — đủ để một nhân viên tò mò tra cứu người quen.
	//
	// Ghi vết việc ĐỌC là bất thường so với phần còn lại của hệ thống, và
	// đó là chủ ý: đọc trộm dữ liệu khách không để lại dấu vết nào khác.
	RecordOrderView(ctx context.Context, in OrderViewRecord) error

	// RecordOrderCancellation ghi việc hủy đơn.
	RecordOrderCancellation(ctx context.Context, in OrderCancelRecord) error
}

// OrderViewRecord là dữ liệu ghi khi nhân viên xem chi tiết đơn.
type OrderViewRecord struct {
	OrderID ids.ID
	ActorID string

	// Reason là lý do truy cập. BẮT BUỘC — "đang xử lý khiếu nại đơn X"
	// phân biệt việc tra cứu chính đáng với việc tò mò.
	Reason    string
	RequestID string
}

// OrderCancelRecord là dữ liệu ghi khi hủy đơn.
type OrderCancelRecord struct {
	OrderID     ids.ID
	OrderNumber string
	ActorID     string
	Reason      string
	RequestID   string
}

// Service là tầng application của module order.
type Service struct {
	orders  domain.Repository
	numbers domain.NumberGenerator
	audit   AuditRecorder
	clock   Clock
}

type Deps struct {
	Orders  domain.Repository
	Numbers domain.NumberGenerator
	Clock   Clock

	// Audit có thể nil: luồng đặt hàng của khách không cần. Chỉ các use
	// case quản trị bắt buộc có nó.
	Audit AuditRecorder
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		orders:  d.Orders,
		numbers: d.Numbers,
		audit:   d.Audit,
		clock:   clock,
	}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Đặt hàng

// PlaceOrderLine là một dòng hàng khách muốn mua.
//
// Mọi con số ở đây do bên gọi (checkout) TRUYỀN XUỐNG, không phải module
// này đi tra: giá và tỷ lệ hoa hồng phải là giá trị khách đã NHÌN THẤY và
// đồng ý, không phải giá trị tại thời điểm ghi database.
type PlaceOrderLine struct {
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

	// Adjustments đã được PHÂN BỔ về từng dòng ở tầng trên.
	//
	// Phân bổ tại đây chứ không giữ một khoản giảm ở mức đơn: khách trả
	// một món thì hoàn tiền đọc trực tiếp phần giảm của món đó, không phải
	// tính lại tỷ lệ.
	Adjustments []domain.Adjustment
}

// PlaceOrderInput là dữ liệu đặt một đơn hàng.
type PlaceOrderInput struct {
	CustomerID ids.ID
	GuestEmail string
	GuestPhone string

	ShippingAddress domain.Address
	BillingAddress  domain.Address

	Currency       money.Currency
	ShippingFee    money.Money
	DiscountAmount money.Money
	TaxAmount      money.Money

	Lines []PlaceOrderLine

	// IdempotencyKey là BẮT BUỘC (quy tắc 5).
	IdempotencyKey string

	// SourceCheckoutID là phiên thanh toán sinh ra đơn.
	//
	// Rỗng với đơn KHÔNG đến từ phiên nào — đơn tạo bằng đường quản trị.
	// Khi có giá trị, nó cưỡng chế bất biến "một phiên sinh tối đa một
	// đơn" qua chỉ mục UNIQUE — xem migrations/000029.
	SourceCheckoutID ids.ID
}

// PlaceOrderResult là kết quả đặt hàng.
type PlaceOrderResult struct {
	Order *domain.Order

	// Replayed cho biết đây là kết quả của một lần gọi TRƯỚC ĐÓ.
	//
	// Bên gọi cần biết để không gửi email xác nhận lần thứ hai — idempotent
	// nghĩa là không tạo đơn mới, nhưng tác dụng phụ thì vẫn lặp nếu không
	// kiểm tra cờ này.
	Replayed bool
}

// PlaceOrder tạo đơn hàng từ giỏ đã chốt.
//
// BA VIỆC trong một thao tác:
//
//  1. ĐÓNG BĂNG mọi con số vào dòng hàng (nguyên tắc P9)
//  2. TÁCH thành đơn vị công việc theo nguồn hàng (ADR-0007)
//  3. Ghi cả hai trong MỘT giao dịch
//
// IDEMPOTENT theo IdempotencyKey (quy tắc 5): khách bấm "Đặt hàng" hai
// lần, hoặc client thử lại sau timeout, đều nhận CÙNG một đơn. Hai đơn cho
// một lần mua nghĩa là khách bị trừ tiền hai lần và phải khiếu nại.
//
// Kiểm tra khóa TRƯỚC khi tạo chỉ bắt được phần lớn trường hợp — hai
// request song song vẫn có thể cùng qua cửa. Ràng buộc UNIQUE ở database
// bắt nốt phần còn lại, và nhánh ErrDuplicateOrder bên dưới xử lý nó.
func (s *Service) PlaceOrder(ctx context.Context, in PlaceOrderInput) (*PlaceOrderResult, error) {
	if in.IdempotencyKey == "" {
		return nil, domain.ErrMissingIdempKey
	}

	if existing, err := s.orders.FindByIdempotencyKey(ctx, in.IdempotencyKey); err == nil {
		return s.replay(ctx, existing)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	// Phiên thanh toán này đã sinh đơn rồi thì trả lại chính đơn đó, dù
	// khóa idempotency lần này khác. Cửa kiểm SỚM cho đường tuần tự; cuộc
	// đua thật sự do chỉ mục UNIQUE bên dưới chặn.
	if !in.SourceCheckoutID.IsZero() {
		existing, err := s.orders.FindBySourceCheckout(ctx, in.SourceCheckoutID)
		if err == nil {
			return s.replay(ctx, existing)
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
	}

	now := s.clock.Now()

	lines := make([]*domain.Line, 0, len(in.Lines))
	for i, src := range in.Lines {
		l, err := domain.NewLine(domain.NewLineParams{
			OfferID:               src.OfferID,
			SKUID:                 src.SKUID,
			SellerID:              src.SellerID,
			ProductName:           src.ProductName,
			VariantDescription:    src.VariantDescription,
			UnitPrice:             src.UnitPrice,
			Quantity:              src.Quantity,
			CommissionRate:        src.CommissionRate,
			AttributedCreatorID:   src.AttributedCreatorID,
			CreatorCommissionRate: src.CreatorCommissionRate,
			Now:                   now,
		})
		if err != nil {
			return nil, fmt.Errorf("order: dòng hàng %d: %w", i+1, err)
		}
		for _, a := range src.Adjustments {
			if err := l.AddAdjustment(a, now); err != nil {
				return nil, fmt.Errorf("order: dòng hàng %d: %w", i+1, err)
			}
		}
		lines = append(lines, l)
	}

	number, err := s.numbers.NextOrderNumber(ctx)
	if err != nil {
		return nil, err
	}

	o, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber:      number,
		CustomerID:       in.CustomerID,
		GuestEmail:       in.GuestEmail,
		GuestPhone:       in.GuestPhone,
		ShippingAddress:  in.ShippingAddress,
		BillingAddress:   in.BillingAddress,
		Currency:         in.Currency,
		ShippingFee:      in.ShippingFee,
		DiscountAmount:   in.DiscountAmount,
		TaxAmount:        in.TaxAmount,
		Lines:            lines,
		IdempotencyKey:   in.IdempotencyKey,
		SourceCheckoutID: in.SourceCheckoutID,
		Now:              now,
	})
	if err != nil {
		return nil, err
	}

	// KHÔNG tách đơn ở đây: đơn thực hiện thuộc module fulfillment, được
	// tạo khi module đó nghe event `checkout.completed`.
	//
	// Đặt việc tách ở đây sẽ buộc order phụ thuộc fulfillment, và tạo
	// phụ thuộc vòng vì fulfillment đã trỏ tới order qua order_id.
	if err := s.orders.Save(ctx, o); err != nil {
		// Hai request song song cùng qua được cửa kiểm tra ở trên; ràng
		// buộc UNIQUE chặn cái thứ hai. Đây KHÔNG phải lỗi để báo cho
		// khách — khách đã đặt hàng thành công.
		if errors.Is(err, domain.ErrDuplicateOrder) {
			existing, findErr := s.orders.FindByIdempotencyKey(ctx, in.IdempotencyKey)
			if findErr != nil {
				return nil, err
			}
			return s.replay(ctx, existing)
		}

		// Thua cuộc đua "một phiên một đơn". Khách KHÔNG được thấy lỗi:
		// đơn đã tồn tại và đó chính là đơn của họ.
		if errors.Is(err, domain.ErrCheckoutAlreadyOrdered) {
			existing, findErr := s.orders.FindBySourceCheckout(ctx, in.SourceCheckoutID)
			if findErr != nil {
				return nil, err
			}
			return s.replay(ctx, existing)
		}
		return nil, err
	}

	return &PlaceOrderResult{Order: o}, nil
}

// replay trả kết quả của lần đặt hàng trước đó.
func (s *Service) replay(_ context.Context, o *domain.Order) (*PlaceOrderResult, error) {
	return &PlaceOrderResult{Order: o, Replayed: true}, nil
}

// ---------------------------------------------------------------- Đọc

func (s *Service) GetOrder(ctx context.Context, id ids.ID) (*domain.Order, error) {
	return s.orders.FindByID(ctx, id)
}

func (s *Service) GetOrderByNumber(ctx context.Context, number string) (*domain.Order, error) {
	return s.orders.FindByOrderNumber(ctx, number)
}

// ListCustomerOrders trả lịch sử đơn của một khách.
//
// `moc` nil nghĩa là đọc từ đầu; khác nil là đọc tiếp sau bản ghi đó.
//
// `status` rỗng nghĩa là mọi trạng thái. Chuỗi không hợp lệ trả
// ErrTrangThaiKhongHopLe — KHÔNG lặng lẽ trả danh sách rỗng, vì rỗng trông
// giống "khách chưa có đơn nào" chứ không giống "bạn gõ sai".
func (s *Service) ListCustomerOrders(
	ctx context.Context, customerID ids.ID, status domain.Status,
	limit int, moc *domain.MocPhanTrang,
) ([]*domain.Order, error) {
	if status != "" && !status.HopLe() {
		return nil, fmt.Errorf("%w: %q", ErrTrangThaiKhongHopLe, status)
	}
	return s.orders.ListByCustomer(ctx, customerID, status, limit, moc)
}

// ErrTrangThaiKhongHopLe khi bộ lọc `status` nằm ngoài tập đã khai.
var ErrTrangThaiKhongHopLe = errors.New("order: trạng thái lọc không hợp lệ")

// ---------------------------------------------------------------- Khách hàng

// MarkPaid ghi nhận đơn đã thanh toán.
func (s *Service) MarkPaid(ctx context.Context, orderID ids.ID) error {
	o, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	if err := o.MarkPaid(now); err != nil {
		return err
	}
	return s.orders.Update(ctx, o)
}

// CancelOrder hủy toàn bộ đơn theo yêu cầu của khách.
//
// Chặn nếu ĐÃ CÓ đơn thực hiện đóng gói xong (mục 6.1): từ đó trở đi việc
// hủy có chi phí thật và cần quy trình riêng.
// CancelOrder hủy toàn bộ đơn theo yêu cầu của khách.
//
// KHÔNG kiểm tra trạng thái đơn thực hiện ở đây: module này không đọc dữ
// liệu của fulfillment. Điều kiện "chưa gói nào đóng xong" (mục 6.1) do
// tầng điều phối kiểm tra trước khi gọi — nó là bên duy nhất thấy cả hai.
// ---------------------------------------------------------------- Quản trị

// ListOrders trả đơn theo bộ lọc, cho giao diện quản trị.
//
// KHÔNG giới hạn theo khách: nhân viên hỗ trợ tra đơn của bất kỳ ai. Việc
// chặn ai được gọi nằm ở tầng route, không có giới hạn tự nhiên ở đây.
func (s *Service) ListOrders(
	ctx context.Context, f domain.Filter,
) ([]*domain.Order, error) {
	return s.orders.List(ctx, f)
}

// ViewOrderInput là yêu cầu xem chi tiết đơn từ giao diện quản trị.
type ViewOrderInput struct {
	OrderID   ids.ID
	ActorID   string
	Reason    string
	RequestID string
}

// ViewOrderAsAdmin đọc chi tiết đơn VÀ ghi vết việc đọc.
//
// Đây là use case hiếm hoi ghi vết cho một thao tác CHỈ ĐỌC. Lý do ở
// admin-api.md mục 6: đọc trộm dữ liệu khách không để lại dấu vết nào khác,
// và cảnh báo "nhân viên xem nhiều hồ sơ trong thời gian ngắn" chỉ dựng
// được nếu mỗi lần đọc đều có bản ghi.
//
// Ghi vết TRƯỚC khi trả dữ liệu: nếu ghi sau và tiến trình chết giữa chừng,
// nhân viên đã thấy dữ liệu mà không có vết.
func (s *Service) ViewOrderAsAdmin(
	ctx context.Context, in ViewOrderInput,
) (*domain.Order, error) {
	if s.audit == nil {
		return nil, errors.New(
			"order: thiếu AuditRecorder — không được đọc dữ liệu khách khi " +
				"chưa có đường ghi vết")
	}
	if strings.TrimSpace(in.ActorID) == "" {
		return nil, errors.New("order: thiếu định danh người truy cập")
	}

	o, err := s.orders.FindByID(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}

	// Ghi vết KHÔNG nằm trong giao dịch nào: đây là thao tác đọc, không có
	// thay đổi nghiệp vụ để gắn vào. Nếu ghi vết hỏng thì KHÔNG trả dữ
	// liệu — thà nhân viên phải thử lại còn hơn có một lần đọc không dấu.
	if err := s.audit.RecordOrderView(ctx, OrderViewRecord{
		OrderID:   in.OrderID,
		ActorID:   in.ActorID,
		Reason:    in.Reason,
		RequestID: in.RequestID,
	}); err != nil {
		return nil, err
	}

	return o, nil
}

// CancelOrderInput là yêu cầu hủy đơn từ giao diện quản trị.
type CancelOrderInput struct {
	OrderID   ids.ID
	ActorID   string
	Reason    string
	RequestID string
}

// CancelOrderAsAdmin hủy đơn VÀ ghi vết kiểm toán trong CÙNG giao dịch.
//
// Khác CancelOrder ở chỗ lý do được GHI LẠI. CancelOrder nhận `reason`
// nhưng bỏ qua nó — chấp nhận được với hủy tự động (hết hạn giữ hàng), vì
// khi đó lý do luôn là một; không chấp nhận được khi người thật hủy đơn của
// khách đã trả tiền.
func (s *Service) CancelOrderAsAdmin(
	ctx context.Context, in CancelOrderInput,
) (*domain.Order, error) {
	if s.audit == nil {
		return nil, errors.New(
			"order: thiếu AuditRecorder — hủy đơn là thao tác nhạy cảm")
	}
	if strings.TrimSpace(in.ActorID) == "" {
		return nil, errors.New("order: thiếu định danh người thực hiện")
	}

	o, err := s.orders.FindByID(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	if err := o.Cancel(s.clock.Now()); err != nil {
		return nil, err
	}

	err = s.orders.UpdateWithAudit(ctx, o, func(txCtx context.Context) error {
		return s.audit.RecordOrderCancellation(txCtx, OrderCancelRecord{
			OrderID:     in.OrderID,
			OrderNumber: o.OrderNumber(),
			ActorID:     in.ActorID,
			Reason:      in.Reason,
			RequestID:   in.RequestID,
		})
	})
	if err != nil {
		return nil, err
	}

	return o, nil
}

func (s *Service) CancelOrder(ctx context.Context, orderID ids.ID, reason string) error {
	_, err := s.CancelOwnOrder(ctx, orderID, reason)
	return err
}

// CancelOwnOrder hủy đơn theo yêu cầu của CHÍNH KHÁCH HÀNG.
//
// Khác CancelOrderAsAdmin ở hai điểm:
//
//   - KHÔNG ghi audit log. Nhật ký thao tác dành cho hành động của nhân
//     viên (ADR-0011); khách hủy đơn của mình không phải thứ cần giám sát.
//   - Lý do được LƯU VÀO ĐƠN thay vì vào nhật ký, vì đây là dữ liệu nghiệp
//     vụ: "giao quá chậm" và "tìm được giá tốt hơn" dẫn tới hai hành động
//     khác nhau của nền tảng.
//
// Bên gọi PHẢI kiểm tra quyền sở hữu TRƯỚC. Hàm này không biết ai đang gọi
// — truyền vào một mã đơn bất kỳ là hủy được đơn đó.
func (s *Service) CancelOwnOrder(
	ctx context.Context, orderID ids.ID, reason string,
) (*domain.Order, error) {
	o, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if err := o.CancelWithReason(reason, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.orders.Update(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// ApplyFulfillmentProgress tính lại trạng thái tổng hợp từ tiến độ các
// nguồn hàng.
//
// QUY TẮC 7: trạng thái đơn được SUY RA từ đơn thực hiện, không tự đặt.
// Gọi bởi bên nhận event từ module fulfillment — module này KHÔNG hỏi
// ngược, vì hỏi ngược tạo phụ thuộc vòng (ADR-0007).
func (s *Service) ApplyFulfillmentProgress(
	ctx context.Context, orderID ids.ID, progress []domain.FulfillmentProgress,
) error {
	o, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return err
	}
	if !o.RecalculateStatus(progress, s.clock.Now()) {
		return nil
	}
	return s.orders.Update(ctx, o)
}
