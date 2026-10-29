// Package application chứa các use case của module fulfillment.
//
// Module này là GÓC NHÌN VẬN HÀNH của đơn hàng. Nó trả lời câu hỏi "ai
// giao, đến đâu rồi", trong khi module order trả lời "khách mua gì, giá
// bao nhiêu".
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
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return types.BayGio() }

var SystemClock Clock = systemClock{}

// ErrForbidden khi seller thao tác trên đơn thực hiện không phải của mình.
var ErrForbidden = errors.New("fulfillment: đơn thực hiện không thuộc về nhà bán này")

// ReturnWindow là thời hạn đổi trả.
//
// Sau khi hết hạn, đơn thực hiện chuyển COMPLETED và số dư seller chuyển
// từ Pending sang Available. Đây là ranh giới TÀI CHÍNH: trả tiền sớm hơn
// nghĩa là trả trước khi biết khách có hoàn hàng không.
// Từ 11/09 đây chỉ là MẶC ĐỊNH khi chưa nối cấu hình vận hành. Con số
// thật đọc từ `returns.window_hours` — CÙNG tham số mà `returns.XinTra`
// dùng để từ chối yêu cầu quá hạn. Hai hằng số riêng cho một quy tắc sớm
// muộn lệch nhau, và lệch ở đây là hoàn tiền cho khách sau khi nhà bán đã
// rút tiền.
const ReturnWindow = 7 * 24 * time.Hour

// ReturnWindowPort cấp thời hạn đổi trả từ cấu hình vận hành.
type ReturnWindowPort interface {
	ReturnWindow() time.Duration
}

// returnWindow trả hạn đang áp dụng, rơi về `ReturnWindow` khi chưa nối.
//
// Rơi về mặc định chứ không lỗi: thiếu cấu hình thì hành vi giống hệt
// trước khi có tham số này, và job nền vẫn chạy. Nhưng khi ĐÃ nối thì hai
// module đọc cùng một con số.
func (s *Service) returnWindow() time.Duration {
	if s.returnWindowPort == nil {
		return ReturnWindow
	}
	if d := s.returnWindowPort.ReturnWindow(); d > 0 {
		return d
	}
	return ReturnWindow
}

// EventPublisher phát domain event.
//
// Là PORT do tầng application định nghĩa nên nó không biết outbox hay
// database. Ngữ cảnh truyền vào phải mang giao dịch của kho lưu trữ.
type EventPublisher interface {
	PublishProgress(ctx context.Context, e ProgressChanged) error

	// PublishCancelled báo một đơn thực hiện đã bị hủy, kèm DÒNG HÀNG.
	//
	// Tách khỏi PublishProgress vì hai event trả lời hai câu khác nhau:
	// progress nói "tiến độ đổi rồi, tính lại trạng thái đơn", còn cái
	// này nói "những món cụ thể này không đi nữa, trả về kho".
	PublishCancelled(ctx context.Context, e FulfillmentCancelled) error

	// PublishCompleted báo một đơn thực hiện đã qua hạn đổi trả.
	//
	// Tín hiệu TÀI CHÍNH: từ đây tiền của nhà bán chuyển từ "đang chờ"
	// sang "rút được". Tách khỏi PublishProgress vì progress nói về giao
	// hàng, còn cái này nói về tiền.
	PublishCompleted(ctx context.Context, e FulfillmentCompleted) error
}

// FulfillmentCompleted là sự thật "một đơn thực hiện đã qua hạn đổi trả".
type FulfillmentCompleted struct {
	FulfillmentID ids.ID
	OrderID       ids.ID
	SellerID      ids.ID

	// SellerPayable là tiền hàng TRỪ hoa hồng nền tảng, đã đóng băng.
	//
	// Mang sẵn trong payload để payment không phải gọi ngược fulfillment:
	// bên nhận event mà phải hỏi lại bên phát thì hai module dính chặt vào
	// nhau, và một bên chậm làm bên kia chậm theo.
	SellerPayable money.Money
}

// FulfillmentCancelled là sự thật "một đơn thực hiện đã bị hủy".
type FulfillmentCancelled struct {
	OrderID       ids.ID
	FulfillmentID ids.ID
	FONumber      string
	SellerID      ids.ID

	// StockLocationID là kho đang giữ hàng. Có thể rỗng với đơn cũ.
	StockLocationID ids.ID

	// ReleaseStock quyết định tồn kho có được trả về khả dụng không.
	//
	// FALSE khi hàng đã rời kho (hủy sau khi giao thất bại): lúc đó hàng
	// đang trên đường về và phải nhập lại qua quy trình hàng trả, có bước
	// kiểm tra chất lượng. Xem FOStatus.StockStillInWarehouse.
	ReleaseStock bool

	Lines []CancelledLine
}

// CancelledLine là một món không đi nữa.
type CancelledLine struct {
	// OrderLineID là dòng của ĐƠN HÀNG mà món này thuộc về.
	//
	// # Vì sao cần nó bên cạnh SKUID
	//
	// inventory trả hàng về kho theo `(sku_id, quantity)` và không cần mã
	// dòng đơn. Nhưng mọi thứ TÍNH TIỀN theo dòng đơn thì cần: module
	// `affiliate` đảo hoa hồng creator theo `order_line_id`, và một đơn có
	// thể có hai dòng CÙNG SKU với hai creator khác nhau — gộp theo SKU sẽ
	// đảo sai người.
	//
	// Có thể RỖNG với đơn thực hiện cũ: `FOLine.OrderLineID` chỉ được điền
	// từ migration 000024 trở đi. Bên nhận phải bỏ qua dòng rỗng chứ không
	// được coi đó là lỗi.
	OrderLineID ids.ID

	SKUID    ids.ID
	Quantity int
}

// ProgressChanged là sự thật "tiến độ một nguồn hàng đã đổi".
//
// Module order nghe event này để tính lại trạng thái tổng hợp. Payload
// chứa tiến độ của MỌI nguồn hàng trong đơn, không chỉ nguồn vừa đổi —
// nhờ vậy order tính được ngay mà không phải hỏi ngược.
type ProgressChanged struct {
	OrderID ids.ID

	// FulfillmentID là nguồn hàng vừa đổi trạng thái.
	FulfillmentID  ids.ID
	FONumber       string
	NewStatus      string
	TrackingNumber string

	// Địa chỉ liên hệ, để notification gửi được mà không gọi ngược module
	// nào — xem notification.md quy tắc 1.
	CustomerID ids.ID
	Email      string
	Phone      string

	// Progress là tiến độ của TẤT CẢ nguồn hàng trong đơn.
	Progress []LineProgress
	// ShippingMethod để payment tra giá trả hãng vận chuyển (ADR-0018).
	ShippingMethod string
}

// LineProgress là tiến độ của một nguồn hàng.
type LineProgress struct {
	Cancelled bool
	Delivered bool
	Shipped   bool
}

// Service là tầng application của module fulfillment.
type Service struct {
	repo             domain.Repository
	clock            Clock
	events           EventPublisher
	thresholds       ThresholdsPort
	returnWindowPort ReturnWindowPort
	schedule         FeeSchedulePort
}

// FeeSchedulePort cấp bảng phí và thời gian vận chuyển, đọc LÚC CHẠY.
//
// Hai con số tiền là giá hiện trên màn hình thanh toán; hai con số ngày là
// lời hứa giao hàng. Chúng là chính sách kinh doanh, không phải hằng số kỹ
// thuật — xem docs/09-operations/cau-hinh-nghiep-vu.md.
type FeeSchedulePort interface {
	FeeSchedule() domain.FeeSchedule
}

type Deps struct {
	Repo  domain.Repository
	Clock Clock

	// Thresholds có thể nil: khi đó dùng ngưỡng mặc định đã biên dịch.
	Thresholds ThresholdsPort

	// FeeSchedule có thể nil: khi đó dùng `domain.DefaultFeeSchedule`.
	FeeSchedule FeeSchedulePort

	// Events có thể nil: khi đó module vẫn hoạt động nhưng KHÔNG phát
	// event, và trạng thái tổng hợp của đơn hàng sẽ không được cập nhật.
	Events EventPublisher

	// Han cấp hạn đổi trả. Nil thì rơi về `ReturnWindow` mặc định.
	ReturnWindow ReturnWindowPort
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{repo: d.Repo, clock: clock, events: d.Events,
		thresholds: d.Thresholds, returnWindowPort: d.ReturnWindow, schedule: d.FeeSchedule}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Tách đơn

// SplitOrder tách một đơn hàng thành các đơn thực hiện theo nguồn hàng.
//
//	Giỏ hàng:
//	├── Áo own brand   (kho nền tảng, Hà Nội)
//	├── Giày Seller A  (kho seller A, TP.HCM)
//	└── Túi Seller B   (kho seller B, Đà Nẵng)
//
//	Ba món KHÔNG THỂ đóng chung một gói.
//
// IDEMPOTENT: gọi lại cho cùng một đơn KHÔNG tạo thêm bộ đơn thực hiện.
// Event `checkout.completed` có thể được phát lại, và tách hai lần nghĩa
// là seller thấy việc trùng — có thể giao hàng hai lần.
func (s *Service) SplitOrder(
	ctx context.Context, in domain.SplitInput,
) ([]*domain.FulfillmentOrder, error) {
	exists, err := s.repo.ExistsForOrder(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	if exists {
		// Đã tách rồi — trả về bộ hiện có thay vì tạo thêm.
		return s.repo.ListByOrder(ctx, in.OrderID)
	}

	fos, err := domain.SplitIntoFulfillmentOrders(in, s.clock.Now())
	if err != nil {
		return nil, err
	}

	if err := s.repo.SaveBatch(ctx, fos); err != nil {
		return nil, err
	}
	return fos, nil
}

// ---------------------------------------------------------------- Đọc

func (s *Service) ListByOrder(
	ctx context.Context, orderID ids.ID,
) ([]*domain.FulfillmentOrder, error) {
	return s.repo.ListByOrder(ctx, orderID)
}

// ListSellerWork trả danh sách việc cần xử lý của một seller.
//
// sellerID là tham số ĐẦU TIÊN và BẮT BUỘC ở mọi hàm của seller: đó là
// cách ranh giới bảo mật của ADR-0007 hiện ra trong chữ ký hàm.
func (s *Service) ListSellerWork(
	ctx context.Context, sellerID ids.ID, statuses []domain.FOStatus, limit, offset int,
) ([]*domain.FulfillmentOrder, error) {
	return s.repo.ListBySeller(ctx, sellerID, statuses, limit, offset)
}

func (s *Service) GetSellerFulfillment(
	ctx context.Context, sellerID, foID ids.ID,
) (*domain.FulfillmentOrder, error) {
	return s.loadOwned(ctx, sellerID, foID)
}

// ---------------------------------------------------------------- Vận hành

// Allocate phân bổ nguồn hàng: chọn kho xuất.
func (s *Service) Allocate(ctx context.Context, sellerID, foID, locationID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Allocate(locationID, now)
	})
}

func (s *Service) Confirm(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Confirm(now)
	})
}

func (s *Service) Pick(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Pick(now)
	})
}

func (s *Service) Pack(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Pack(now)
	})
}

// HandOver bàn giao cho đơn vị vận chuyển.
//
// Mã vận đơn BẮT BUỘC: từ đây hàng ra khỏi tầm kiểm soát của seller, và
// không có mã thì không ai trả lời được "hàng của tôi đang ở đâu".
func (s *Service) HandOver(
	ctx context.Context, sellerID, foID ids.ID, provider, trackingNumber string,
) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.HandOver(provider, trackingNumber, s.feeSchedule(), now)
	})
}

// RecordHandOver ghi nhận seller đã BÀN GIAO cho đơn vị vận chuyển, đi qua
// mọi bước trung gian còn thiếu.
//
// # Vì sao một hàm thay vì bắt seller gọi ba lần
//
// Đặc tả cho nhà bán ĐÚNG MỘT hành động: "bàn giao vận chuyển". Đó là mức
// chi tiết đúng cho một cửa hàng nhỏ — họ đóng gói ở bàn làm việc rồi ghi
// nhận đã gửi, không có quy trình kho nhiều bước.
//
// Máy trạng thái vẫn NGUYÊN VẸN: hàm này đi đúng đường hợp lệ ngắn nhất
// (CONFIRMED → PACKED → HANDED_OVER) thay vì nhảy thẳng. Nhảy thẳng sẽ phải
// nới lỏng đồ thị chuyển trạng thái, và khi đó những bước bảo vệ khác —
// như "đã đóng gói thì không hủy được" — cũng lỏng theo.
//
// # Mốc thời gian nghĩa là gì
//
// `confirmed_at` và `packed_at` ghi bằng thời điểm BÀN GIAO, không phải
// thời điểm seller thật sự làm hai việc đó. Đây là thông tin tốt nhất ta
// có: seller ghi nhận bàn giao nghĩa là họ ĐÃ xác nhận và ĐÃ đóng gói.
//
// Bỏ trống hai mốc đó cũng là một lựa chọn, nhưng khi đó chỉ số hiệu suất
// (thời gian xử lý trung bình) không tính được cho phần lớn đơn.
func (s *Service) RecordHandOver(
	ctx context.Context, sellerID, foID ids.ID, provider, trackingNumber string,
) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		switch fo.Status() {
		case domain.FOPending, domain.FOAllocated:
			if err := fo.Confirm(now); err != nil {
				return err
			}
			if err := fo.Pack(now); err != nil {
				return err
			}
		case domain.FOConfirmed, domain.FOPicking:
			if err := fo.Pack(now); err != nil {
				return err
			}
		}
		// Mọi trạng thái khác đi thẳng vào HandOver, và máy trạng thái tự
		// từ chối nếu không hợp lệ (đã giao, đã hủy).
		return fo.HandOver(provider, trackingNumber, s.feeSchedule(), now)
	})
}

func (s *Service) MarkInTransit(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.MarkInTransit(now)
	})
}

func (s *Service) MarkDeliveryFailed(
	ctx context.Context, sellerID, foID ids.ID, reason string,
) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.MarkDeliveryFailed(reason, now)
	})
}

func (s *Service) Deliver(ctx context.Context, sellerID, foID ids.ID) error {
	return s.advance(ctx, sellerID, foID, func(fo *domain.FulfillmentOrder, now time.Time) error {
		return fo.Deliver(now)
	})
}

// Cancel hủy phần của một seller, ví dụ vì hết hàng.
//
// Lý do BẮT BUỘC: khách cần lời giải thích khi nhận thông báo.
// Cancel hủy một đơn thực hiện VÀ trả hàng về kho nếu hàng còn ở đó.
//
// Đọc trạng thái TRƯỚC khi hủy: sau khi hủy thì mọi đơn đều là CANCELLED,
// và lúc đó không còn phân biệt được "hàng vẫn trong kho" với "hàng đang
// trên đường trả về".
func (s *Service) Cancel(ctx context.Context, sellerID, foID ids.ID, reason string) error {
	fo, err := s.loadOwned(ctx, sellerID, foID)
	if err != nil {
		return err
	}

	stockStillInWarehouse := fo.Status().StockStillInWarehouse()

	now := s.clock.Now()
	if err := fo.Cancel(reason, now); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, fo); err != nil {
		return err
	}

	// Phát event HỦY trước, event tiến độ sau.
	//
	// Thứ tự này quan trọng khi có sự cố giữa chừng: thà tồn kho đúng mà
	// trạng thái đơn hiển thị chậm, còn hơn đơn hiển thị "đã hủy" trong
	// khi hàng vẫn bị khóa. Hàng bị khóa không bán được cho ai.
	if err := s.emitCancelled(ctx, fo, stockStillInWarehouse); err != nil {
		return err
	}
	return s.publishProgress(ctx, fo)
}

// emitCancelled phát event hủy kèm dòng hàng để inventory trả về kho.
func (s *Service) emitCancelled(
	ctx context.Context, fo *domain.FulfillmentOrder, releaseStock bool,
) error {
	if s.events == nil {
		return nil
	}

	lines := make([]CancelledLine, 0, len(fo.Lines()))
	for _, l := range fo.Lines() {
		if l.SKUID.IsZero() || l.Quantity <= 0 {
			continue
		}
		lines = append(lines, CancelledLine{
			OrderLineID: l.OrderLineID,
			SKUID:       l.SKUID,
			Quantity:    l.Quantity,
		})
	}

	return s.events.PublishCancelled(ctx, FulfillmentCancelled{
		OrderID:         fo.OrderID(),
		FulfillmentID:   fo.ID(),
		FONumber:        fo.FONumber(),
		SellerID:        fo.SellerID(),
		StockLocationID: fo.StockLocationID(),
		ReleaseStock:    releaseStock,
		Lines:           lines,
	})
}

// advance chạy một bước chuyển trạng thái rồi phát event.
func (s *Service) advance(
	ctx context.Context, sellerID, foID ids.ID,
	step func(*domain.FulfillmentOrder, time.Time) error,
) (result error) {
	// Đếm ở advance chứ không ở từng bước: mọi chuyển trạng thái của đơn
	// thực hiện đều đi qua đây, nên một chỗ là đủ và không bước nào mới
	// thêm bị bỏ sót.
	//
	// Bọc bằng defer để MỌI đường thoát sớm đều được đếm — rải lời gọi ở
	// từng nhánh `return` là cách chắc chắn để bỏ sót một nhánh, và nhánh
	// bị sót thường là nhánh hiếm, tức nhánh đáng quan tâm nhất.
	defer func() {
		if result != nil {
			metrics.RecordFailure(metrics.StageFulfillment, failureReason(result))
		}
	}()

	fo, err := s.loadOwned(ctx, sellerID, foID)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	if err := step(fo, now); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, fo); err != nil {
		return err
	}

	return s.publishProgress(ctx, fo)
}

// publishProgress phát tiến độ của TOÀN BỘ đơn hàng.
//
// Gửi tiến độ mọi nguồn hàng chứ không chỉ nguồn vừa đổi: module order cần
// biết tổng thể để tính trạng thái tổng hợp, và nếu chỉ gửi một nguồn thì
// nó phải hỏi ngược — đúng thứ ADR-0007 cấm.
func (s *Service) publishProgress(ctx context.Context, changed *domain.FulfillmentOrder) error {
	if s.events == nil {
		return nil
	}

	all, err := s.repo.ListByOrder(ctx, changed.OrderID())
	if err != nil {
		return err
	}

	progress := make([]LineProgress, 0, len(all))
	for _, fo := range all {
		progress = append(progress, LineProgress{
			Cancelled: fo.Status() == domain.FOCancelled,
			Delivered: fo.Status() == domain.FODelivered || fo.Status() == domain.FOCompleted,
			Shipped:   fo.Status().IsShipped(),
		})
	}

	return s.events.PublishProgress(ctx, ProgressChanged{
		OrderID:        changed.OrderID(),
		FulfillmentID:  changed.ID(),
		FONumber:       changed.FONumber(),
		ShippingMethod: changed.ShippingMethod(),
		NewStatus:      string(changed.Status()),
		TrackingNumber: changed.TrackingNumber(),
		CustomerID:     changed.CustomerID(),
		Email:          changed.NotifyEmail(),
		Phone:          changed.NotifyPhone(),
		Progress:       progress,
	})
}

// loadOwned đọc đơn thực hiện và kiểm tra chủ sở hữu HAI LẦN.
//
// Truy vấn đã lọc theo seller_id trong SQL; BelongsTo là hàng rào thứ hai
// ở tầng domain. Thừa một lớp là có chủ ý: chi phí là một phép so sánh,
// còn hậu quả của việc lọt là seller đọc được dữ liệu đối thủ.
func (s *Service) loadOwned(
	ctx context.Context, sellerID, foID ids.ID,
) (*domain.FulfillmentOrder, error) {
	fo, err := s.repo.FindByID(ctx, foID, sellerID)
	if err != nil {
		return nil, err
	}
	if !fo.BelongsTo(sellerID) {
		return nil, ErrForbidden
	}
	return fo, nil
}

// ---------------------------------------------------------------- Hoàn tất

// CompleteDelivered chuyển các đơn đã giao quá hạn đổi trả sang COMPLETED.
//
// ĐÂY LÀ RANH GIỚI TÀI CHÍNH, không phải bước vận hành:
//
//	DELIVERED  → số dư seller vẫn Pending
//	COMPLETED  → số dư chuyển Available, seller được chi trả
//
// Chạy sớm nghĩa là trả tiền cho seller trước khi biết khách có hoàn hàng
// không — và tiền đã chi thì đòi lại rất khó.
//
// Trả về số đơn đã chuyển.
func (s *Service) CompleteDelivered(ctx context.Context, limit int) (int, error) {
	now := s.clock.Now()
	cutoff := now.Add(-s.returnWindow())

	due, err := s.repo.ListDeliveredBefore(ctx, cutoff, limit)
	if err != nil {
		return 0, err
	}

	var done int
	for _, fo := range due {
		if err := fo.Complete(now); err != nil {
			continue
		}
		if err := s.repo.Update(ctx, fo); err != nil {
			return done, fmt.Errorf("fulfillment: hoàn tất đơn %s: %w", fo.FONumber(), err)
		}
		if err := s.publishProgress(ctx, fo); err != nil {
			return done, err
		}

		// Tín hiệu tài chính, RIÊNG khỏi tiến độ giao hàng.
		if s.events != nil {
			payable, err := fo.Subtotal().Sub(fo.CommissionAmount())
			if err != nil {
				return done, fmt.Errorf(
					"fulfillment: tính tiền phải trả nhà bán cho %s: %w",
					fo.FONumber(), err)
			}
			if err := s.events.PublishCompleted(ctx, FulfillmentCompleted{
				FulfillmentID: fo.ID(), OrderID: fo.OrderID(),
				SellerID: fo.SellerID(), SellerPayable: payable,
			}); err != nil {
				return done, err
			}
		}
		done++
	}
	return done, nil
}

// ---------------------------------------------- Cập nhật từ hãng vận chuyển

// CarrierUpdateInput là một sự kiện vận chuyển đã xác minh chữ ký.
type CarrierUpdateInput struct {
	Carrier        string
	TrackingNumber string

	// Status theo bảng của đặc tả: PICKED_UP, IN_TRANSIT,
	// OUT_FOR_DELIVERY, DELIVERED, DELIVERY_FAILED, RETURNED_TO_SENDER.
	Status string

	FailureReason string
}

// ErrStatusUnchanged báo trạng thái này không kéo theo thay đổi nào.
//
// KHÔNG phải lỗi: hãng vận chuyển gửi nhiều mốc hơn số mốc hệ thống theo
// dõi, và đó là chuyện bình thường. Bên gọi ghi nhật ký rồi trả 200.
var ErrStatusUnchanged = errors.New("fulfillment: trạng thái không kéo theo thay đổi")

// ApplyCarrierUpdate áp một mốc vận chuyển lên đơn thực hiện.
//
// # Vì sao không kiểm chủ sở hữu
//
// Bên gọi là webhook đã xác minh CHỮ KÝ của hãng vận chuyển. Họ không biết
// gian hàng nào, và không cần biết — mã vận đơn là thứ họ có.
//
// # Vì sao trạng thái đã ở đích thì trả nil, không phải lỗi
//
// Hãng vận chuyển gửi trùng là chuyện thường, và cả hai cơ chế — webhook
// lẫn hỏi định kỳ — đều có thể mang cùng một mốc tới. Báo lỗi ở đây khiến
// họ gửi lại mãi cho một việc đã xong.
func (s *Service) ApplyCarrierUpdate(
	ctx context.Context, in CarrierUpdateInput,
) error {
	fo, err := s.repo.FindByTracking(ctx, in.Carrier, in.TrackingNumber)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	var step func(*domain.FulfillmentOrder, time.Time) error

	switch in.Status {
	case "IN_TRANSIT", "OUT_FOR_DELIVERY":
		if fo.Status() == domain.FOInTransit {
			return nil
		}
		step = func(f *domain.FulfillmentOrder, t time.Time) error {
			return f.MarkInTransit(t)
		}

	case "DELIVERED":
		if fo.Status() == domain.FODelivered {
			return nil
		}
		step = func(f *domain.FulfillmentOrder, t time.Time) error {
			return f.Deliver(t)
		}

	case "DELIVERY_FAILED", "RETURNED_TO_SENDER":
		reason := strings.TrimSpace(in.FailureReason)
		if reason == "" {
			reason = "hãng vận chuyển báo giao không thành công"
		}
		step = func(f *domain.FulfillmentOrder, t time.Time) error {
			return f.MarkDeliveryFailed(reason, t)
		}

	default:
		// PICKED_UP và mọi mốc khác: hệ thống không theo dõi, nhưng webhook
		// VẪN được ghi nhật ký ở tầng trên.
		return ErrStatusUnchanged
	}

	if err := step(fo, now); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, fo); err != nil {
		return err
	}
	return s.publishProgress(ctx, fo)
}

// failureReason phân loại lỗi thành nhãn Prometheus.
//
// Tập giá trị phải ĐÓNG và nhỏ: mỗi giá trị nhãn khác nhau là một chuỗi
// thời gian riêng, nên truyền thẳng thông điệp lỗi vào đây — thứ thường
// chứa mã đơn — sẽ làm nổ số chuỗi thời gian.
func failureReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	case errors.Is(err, domain.ErrInvalidStatus):
		return "invalid_status"
	case errors.Is(err, domain.ErrVersionConflict):
		// Tách riêng vì nó KHÁC hẳn về ý nghĩa vận hành: không phải sai
		// sót của ai, mà là hai đường ghi cùng chạm một bản ghi. Con số
		// này tăng nghĩa là tranh chấp đang tăng, không phải lỗi tăng.
		return "version_conflict"
	default:
		return "internal"
	}
}

// Ky là khoảng thời gian tính hiệu suất.
type Period string

const (
	PeriodLast7Days  Period = "LAST_7_DAYS"
	PeriodLast30Days Period = domain.DefaultPeriod
	PeriodLast90Days Period = "LAST_90_DAYS"
)

// ValidPeriod kiểm kỳ và trả về độ dài.
//
// Tập ĐÓNG, đúng enum của đặc tả: cho phép kỳ tùy ý nghĩa là mở cửa cho
// một truy vấn quét toàn bộ bảng, và nhà bán không cần điều đó.
func ValidPeriod(k Period) (time.Duration, bool) {
	switch k {
	case PeriodLast7Days:
		return 7 * 24 * time.Hour, true
	case PeriodLast30Days:
		return 30 * 24 * time.Hour, true
	case PeriodLast90Days:
		return 90 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

// ThresholdsPort đọc bộ ngưỡng đang áp dụng.
//
// Cổng do BÊN GỌI khai: module fulfillment nói nó cần biết ngưỡng, không
// nói ngưỡng đến từ đâu. Nil thì dùng mặc định đã biên dịch — hệ thống
// phải chạy đúng kể cả khi chưa nối dây phần cấu hình.
type ThresholdsPort interface {
	Thresholds() domain.Thresholds
}

// Performance là kết quả chấm hiệu suất của một nhà bán.
type Performance struct {
	Period     Period
	SLAHours   float64
	Stats      domain.PerformanceStats
	Metrics    []domain.PerformanceMetric
	Unmeasured []domain.Unmeasured
	Message    string
}

// ComputePerformance chấm hiệu suất của MỘT nhà bán trong một kỳ.
//
// sellerID lấy từ phạm vi trong token, không phải từ tham số của người
// gọi: nhà bán không được xem hiệu suất của nhà bán khác.
func (s *Service) ComputePerformance(
	ctx context.Context, sellerID ids.ID, k Period) (*Performance, error) {
	length, ok := ValidPeriod(k)
	if !ok {
		return nil, ErrInvalidPeriod
	}

	until := s.clock.Now()
	from := until.Add(-length)

	ng := s.CurrentSLA()

	stats, err := s.repo.CountForPerformance(ctx, sellerID, from, until, ng.DeliverySLA)
	if err != nil {
		return nil, err
	}

	metrics := domain.ComputeMetrics(stats, ng)
	return &Performance{
		Period:     k,
		SLAHours:   ng.DeliverySLA.Hours(),
		Stats:      stats,
		Metrics:    metrics,
		Unmeasured: domain.UnmeasuredMetrics(stats, ng),
		Message:    message(stats, metrics, ng),
	}, nil
}

// ErrInvalidPeriod khi tham số `period` nằm ngoài tập cho phép.
var ErrInvalidPeriod = errors.New("fulfillment: kỳ tính hiệu suất không hợp lệ")

// thongDiep nói cho nhà bán biết NÊN LÀM GÌ, không chỉ họ đang ở đâu.
//
// Đặc tả: "Seller cần hiểu vì sao mình không thắng buy box và cần làm gì
// để cải thiện." Một danh sách con số không trả lời vế thứ hai.
func message(
	stats domain.PerformanceStats, metrics []domain.PerformanceMetric, ng domain.Thresholds) string {
	if len(metrics) == 0 {
		return fmt.Sprintf(
			"Chưa đủ dữ liệu để chấm: cần ít nhất %d đơn trong kỳ, hiện có %d.",
			ng.MinSample, stats.TotalOrders)
	}

	// Nêu chỉ số TỆ NHẤT, không nêu tất cả: một danh sách việc cần làm dài
	// thì không ai bắt đầu từ đâu.
	var worst *domain.PerformanceMetric
	for i := range metrics {
		c := metrics[i]
		if c.Status == domain.MetricGood {
			continue
		}
		if worst == nil || c.Status == domain.MetricCritical {
			worst = &metrics[i]
		}
	}
	if worst == nil {
		return "Mọi chỉ số đều đạt ngưỡng."
	}

	switch worst.Ten {
	case "on_time_shipping_rate":
		return fmt.Sprintf(
			"Giao đúng hạn đang ở %.0f%%, dưới ngưỡng %.0f%%. "+
				"Hạn bàn giao là %.0f giờ kể từ khi đơn được tạo.",
			worst.Value*100, worst.Thresholds*100, ng.DeliverySLA.Hours())
	case "cancellation_rate":
		return fmt.Sprintf(
			"Tỷ lệ hủy đang ở %.1f%%, trên ngưỡng %.1f%%. "+
				"Mỗi đơn hủy là một khách đã đặt rồi không nhận được hàng.",
			worst.Value*100, worst.Thresholds*100)
	default:
		return "Có chỉ số chưa đạt ngưỡng."
	}
}

// UnlockByOrder mở khóa mọi đơn thực hiện của một đơn hàng.
//
// Đầu kia của cửa chặn thanh toán (ADR-0018 phần A2): đơn TRẢ TRƯỚC sinh
// ra bị khóa, và đây là đường duy nhất mở. Thiếu nó thì hàng kẹt vĩnh viễn.
//
// Bỏ qua đơn đã mở sẵn thay vì ghi lại: ghi lại sẽ đẩy `version` và
// `updated_at` tới trước mỗi lần event được phát lại, làm mọi báo cáo
// "đơn này đổi lúc nào" nói sai — và event phát lại là chuyện bình thường.
func (s *Service) UnlockByOrder(ctx context.Context, orderID ids.ID) (int, error) {
	fos, err := s.repo.ListByOrder(ctx, orderID)
	if err != nil {
		return 0, err
	}

	var n int
	for _, fo := range fos {
		if !fo.AwaitingPayment() {
			continue
		}
		fo.UnlockPayment()
		if err := s.repo.Update(ctx, fo); err != nil {
			return n, fmt.Errorf(
				"fulfillment: mở khóa đơn %s: %w", fo.FONumber(), err)
		}
		n++
	}
	return n, nil
}

// CancelByOrder hủy MỌI đơn thực hiện của một đơn hàng đã bị hủy.
//
// # Vì sao nó là đầu kia của `order.cancelled`
//
// Hủy cả đơn mà không hủy các đơn thực hiện để lại hai hậu quả: nhà bán
// vẫn thấy việc phải làm cho một đơn không còn tồn tại, và — nặng hơn —
// hàng nằm mãi ở trạng thái CAM KẾT vì đường nhả kho chỉ mở khi đơn THỰC
// HIỆN bị hủy (`inventory.ReleaseOnFulfillmentCancelled`).
//
// KHÔNG kiểm chủ sở hữu: đây là hệ thống tự hủy theo lệnh của chính đơn
// hàng, không phải một gian hàng thao tác. Vì vậy hàm này KHÔNG được nối
// ra HTTP.
//
// Bỏ qua đơn không hủy được (đã bàn giao, đã giao, đã hủy) thay vì trả
// lỗi: máy trạng thái đã chặn đúng chỗ, và một gói đã rời kho thì việc
// đúng là để nó đi tiếp rồi xử lý bằng luồng TRẢ HÀNG. Trả lỗi ở đây sẽ
// làm event kẹt trong hàng đợi và những gói CÒN hủy được cũng không hủy.
//
// Trả về SỐ đơn thực hiện đã hủy.
func (s *Service) CancelByOrder(
	ctx context.Context, orderID ids.ID, reason string,
) (int, error) {
	fos, err := s.repo.ListByOrder(ctx, orderID)
	if err != nil {
		return 0, err
	}

	now := s.clock.Now()
	var cancelledAlready int
	for _, fo := range fos {
		stockStillInWarehouse := fo.Status().StockStillInWarehouse()

		if err := fo.Cancel(reason, now); err != nil {
			// Không hủy được: đã bàn giao hoặc đã hủy từ trước.
			continue
		}
		if err := s.repo.Update(ctx, fo); err != nil {
			return cancelledAlready, fmt.Errorf(
				"fulfillment: hủy đơn thực hiện %s: %w", fo.FONumber(), err)
		}

		// Thứ tự GIỐNG `Cancel`: event hủy trước, tiến độ sau. Thà tồn
		// kho đúng mà trạng thái hiển thị chậm, còn hơn đơn hiển thị "đã
		// hủy" trong khi hàng vẫn bị khóa.
		if err := s.emitCancelled(ctx, fo, stockStillInWarehouse); err != nil {
			return cancelledAlready, err
		}
		if err := s.publishProgress(ctx, fo); err != nil {
			return cancelledAlready, err
		}
		cancelledAlready++
	}
	return cancelledAlready, nil
}

// feeSchedule đọc bảng phí vận chuyển, MỖI LẦN dùng.
//
// Rơi về mặc định của domain khi chưa nối cấu hình — đúng bằng hành vi
// trước khi có tham số vận hành.
func (s *Service) feeSchedule() domain.FeeSchedule {
	if s.schedule == nil {
		return domain.DefaultFeeSchedule
	}
	return s.schedule.FeeSchedule()
}
