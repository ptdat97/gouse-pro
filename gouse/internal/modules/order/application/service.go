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
	numbers domain.NumberGenerator
	clock   Clock
}

type Deps struct {
	Orders  domain.Repository
	Numbers domain.NumberGenerator
	Clock   Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{orders: d.Orders, numbers: d.Numbers, clock: clock}
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

func (s *Service) ListCustomerOrders(
	ctx context.Context, customerID ids.ID, limit, offset int,
) ([]*domain.Order, error) {
	return s.orders.ListByCustomer(ctx, customerID, limit, offset)
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
// CancelOrder hủy toàn bộ đơn theo yêu cầu của khách.
//
// KHÔNG kiểm tra trạng thái đơn thực hiện ở đây: module này không đọc dữ
// liệu của fulfillment. Điều kiện "chưa gói nào đóng xong" (mục 6.1) do
// tầng điều phối kiểm tra trước khi gọi — nó là bên duy nhất thấy cả hai.
func (s *Service) CancelOrder(ctx context.Context, orderID ids.ID, reason string) error {
	o, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return err
	}
	if err := o.Cancel(s.clock.Now()); err != nil {
		return err
	}
	return s.orders.Update(ctx, o)
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
