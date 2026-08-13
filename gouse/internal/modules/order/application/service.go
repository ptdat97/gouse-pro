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

// Service là tầng application của module order.
type Service struct {
	orders  domain.Repository
	fos     domain.FulfillmentRepository
	numbers domain.NumberGenerator
	clock   Clock
}

type Deps struct {
	Orders  domain.Repository
	FOs     domain.FulfillmentRepository
	Numbers domain.NumberGenerator
	Clock   Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{orders: d.Orders, fos: d.FOs, numbers: d.Numbers, clock: clock}
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
}

// PlaceOrderResult là kết quả đặt hàng.
type PlaceOrderResult struct {
	Order             *domain.Order
	FulfillmentOrders []*domain.FulfillmentOrder

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
		OrderNumber:     number,
		CustomerID:      in.CustomerID,
		GuestEmail:      in.GuestEmail,
		GuestPhone:      in.GuestPhone,
		ShippingAddress: in.ShippingAddress,
		BillingAddress:  in.BillingAddress,
		Currency:        in.Currency,
		ShippingFee:     in.ShippingFee,
		DiscountAmount:  in.DiscountAmount,
		TaxAmount:       in.TaxAmount,
		Lines:           lines,
		IdempotencyKey:  in.IdempotencyKey,
		Now:             now,
	})
	if err != nil {
		return nil, err
	}

	// Tách NGAY lúc đặt, không đợi tới khi thanh toán xong: seller cần
	// thấy việc của mình ngay, và mã FO phải ổn định từ đầu để mọi thông
	// báo về sau trỏ cùng một chỗ.
	fos, err := domain.SplitIntoFulfillmentOrders(o, now)
	if err != nil {
		return nil, err
	}

	if err := s.orders.Save(ctx, o, fos); err != nil {
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
		return nil, err
	}

	return &PlaceOrderResult{Order: o, FulfillmentOrders: fos}, nil
}

// replay trả kết quả của lần đặt hàng trước đó.
func (s *Service) replay(ctx context.Context, o *domain.Order) (*PlaceOrderResult, error) {
	fos, err := s.fos.ListByOrder(ctx, o.ID())
	if err != nil {
		return nil, err
	}
	return &PlaceOrderResult{Order: o, FulfillmentOrders: fos, Replayed: true}, nil
}

// ---------------------------------------------------------------- Đọc

func (s *Service) GetOrder(ctx context.Context, id ids.ID) (*domain.Order, error) {
	return s.orders.FindByID(ctx, id)
}

func (s *Service) GetOrderByNumber(ctx context.Context, number string) (*domain.Order, error) {
	return s.orders.FindByOrderNumber(ctx, number)
}

func (s *Service) ListCustomerOrders(
	ctx context.Context, customerID ids.ID, limit, offset int,
) ([]*domain.Order, error) {
	return s.orders.ListByCustomer(ctx, customerID, limit, offset)
}

// ListOrderFulfillments trả mọi đơn thực hiện của một đơn hàng.
//
// CHỈ dành cho khách và quản trị viên — khách theo dõi được cả ba gói của
// mình. KHÔNG được lộ ra API của seller.
func (s *Service) ListOrderFulfillments(
	ctx context.Context, orderID ids.ID,
) ([]*domain.FulfillmentOrder, error) {
	return s.fos.ListByOrder(ctx, orderID)
}

// ---------------------------------------------------------------- Seller

// ListSellerWork trả danh sách việc cần xử lý của một seller.
//
// sellerID là tham số ĐẦU TIÊN và BẮT BUỘC ở mọi hàm trong phần này: đó là
// cách ranh giới bảo mật của ADR-0007 hiện ra trong chữ ký hàm. Không có
// hàm nào ở đây gọi được mà không nói mình là seller nào.
func (s *Service) ListSellerWork(
	ctx context.Context, sellerID ids.ID, statuses []domain.FOStatus, limit, offset int,
) ([]*domain.FulfillmentOrder, error) {
	return s.fos.ListBySeller(ctx, sellerID, statuses, limit, offset)
}

func (s *Service) GetSellerFulfillment(
	ctx context.Context, sellerID, foID ids.ID,
) (*domain.FulfillmentOrder, error) {
	return s.loadOwned(ctx, sellerID, foID)
}

// ConfirmFulfillment: seller xác nhận sẽ giao phần này.
func (s *Service) ConfirmFulfillment(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Confirm(now)
	})
}

// PackFulfillment: seller đã đóng gói xong.
func (s *Service) PackFulfillment(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Pack(now)
	})
}

// ShipFulfillment: seller đã bàn giao vận chuyển.
func (s *Service) ShipFulfillment(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Ship(now)
	})
}

// DeliverFulfillment: đã giao đến khách.
func (s *Service) DeliverFulfillment(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Deliver(now)
	})
}

// CancelFulfillment: seller hủy phần của mình, ví dụ vì hết hàng.
//
// Hai việc phải đi cùng nhau: hủy đơn thực hiện VÀ hủy các dòng hàng tương
// ứng trong hợp đồng. Chỉ hủy một bên thì khách vẫn bị tính tiền món không
// bao giờ được giao.
//
// Từ PACKED trở đi KHÔNG hủy được bằng hàm này (quy tắc 8): đã tốn công
// đóng gói và có thể đã bàn giao vận chuyển, nên cần quy trình riêng có
// tính chi phí.
func (s *Service) CancelFulfillment(
	ctx context.Context, sellerID, foID ids.ID, reason string,
) error {
	fo, err := s.loadOwned(ctx, sellerID, foID)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	if err := fo.Cancel(reason, now); err != nil {
		return err
	}
	if err := s.fos.Update(ctx, fo); err != nil {
		return err
	}

	o, err := s.orders.FindByID(ctx, fo.OrderID())
	if err != nil {
		return err
	}
	for _, lineID := range fo.LineIDs() {
		// Bỏ qua lỗi "không hủy được": dòng đã bị hủy từ trước là kết quả
		// mong muốn, không phải sự cố.
		_ = o.CancelLine(lineID, now)
	}

	fos, err := s.fos.ListByOrder(ctx, o.ID())
	if err != nil {
		return err
	}
	// Bỏ qua giá trị trả về: dòng hàng đã bị hủy ở trên nên đơn phải được
	// ghi lại dù trạng thái tổng hợp không đổi.
	o.RecalculateStatus(fos, now)

	return s.orders.Update(ctx, o)
}

// advance chạy một bước chuyển trạng thái rồi đồng bộ trạng thái đơn hàng.
func (s *Service) advance(
	ctx context.Context, sellerID, foID ids.ID,
	step func(*domain.FulfillmentOrder, time.Time) error,
) error {
	fo, err := s.loadOwned(ctx, sellerID, foID)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	if err := step(fo, now); err != nil {
		return err
	}
	if err := s.fos.Update(ctx, fo); err != nil {
		return err
	}

	return s.syncOrderStatus(ctx, fo.OrderID(), now)
}

// syncOrderStatus tính lại trạng thái tổng hợp của đơn.
//
// Quy tắc 7: trạng thái đơn được SUY RA từ các đơn thực hiện, không tự đặt.
// Đặt tay ở hai chỗ sẽ dẫn tới đơn báo "đã giao" khi một gói còn ở kho.
func (s *Service) syncOrderStatus(ctx context.Context, orderID ids.ID, now time.Time) error {
	o, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return err
	}
	fos, err := s.fos.ListByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if !o.RecalculateStatus(fos, now) {
		return nil
	}
	return s.orders.Update(ctx, o)
}

// loadOwned đọc đơn thực hiện và kiểm tra chủ sở hữu HAI LẦN.
//
// Truy vấn đã lọc theo seller_id trong SQL; BelongsTo là hàng rào thứ hai ở
// tầng domain. Thừa một lớp là có chủ ý: chi phí là một phép so sánh, còn
// hậu quả của việc lọt là seller đọc được dữ liệu đối thủ.
func (s *Service) loadOwned(
	ctx context.Context, sellerID, foID ids.ID,
) (*domain.FulfillmentOrder, error) {
	fo, err := s.fos.FindByID(ctx, foID, sellerID)
	if err != nil {
		return nil, err
	}
	if !fo.BelongsTo(sellerID) {
		return nil, ErrForbidden
	}
	return fo, nil
}

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
func (s *Service) CancelOrder(ctx context.Context, orderID ids.ID, reason string) error {
	o, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return err
	}

	fos, err := s.fos.ListByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	for _, fo := range fos {
		if fo.Status() == domain.FOCancelled {
			continue
		}
		if !fo.Status().IsCancellableWithoutCost() {
			return fmt.Errorf("%w: đơn thực hiện %s đã ở trạng thái %s",
				domain.ErrNotCancellable, fo.FONumber(), fo.Status())
		}
	}

	now := s.clock.Now()
	if err := o.Cancel(now); err != nil {
		return err
	}

	if reason == "" {
		reason = "khách hàng yêu cầu hủy đơn"
	}
	for _, fo := range fos {
		if fo.Status() == domain.FOCancelled {
			continue
		}
		if err := fo.Cancel(reason, now); err != nil {
			return err
		}
		if err := s.fos.Update(ctx, fo); err != nil {
			return err
		}
	}

	return s.orders.Update(ctx, o)
}
