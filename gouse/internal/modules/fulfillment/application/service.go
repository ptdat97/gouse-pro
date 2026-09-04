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

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// ErrForbidden khi seller thao tác trên đơn thực hiện không phải của mình.
var ErrForbidden = errors.New("fulfillment: đơn thực hiện không thuộc về nhà bán này")

// ReturnWindow là thời hạn đổi trả.
//
// Sau khi hết hạn, đơn thực hiện chuyển COMPLETED và số dư seller chuyển
// từ Pending sang Available. Đây là ranh giới TÀI CHÍNH: trả tiền sớm hơn
// nghĩa là trả trước khi biết khách có hoàn hàng không.
const ReturnWindow = 7 * 24 * time.Hour

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
}

// LineProgress là tiến độ của một nguồn hàng.
type LineProgress struct {
	Cancelled bool
	Delivered bool
	Shipped   bool
}

// Service là tầng application của module fulfillment.
type Service struct {
	repo   domain.Repository
	clock  Clock
	events EventPublisher
}

type Deps struct {
	Repo  domain.Repository
	Clock Clock

	// Events có thể nil: khi đó module vẫn hoạt động nhưng KHÔNG phát
	// event, và trạng thái tổng hợp của đơn hàng sẽ không được cập nhật.
	Events EventPublisher
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{repo: d.Repo, clock: clock, events: d.Events}
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
		return fo.HandOver(provider, trackingNumber, now)
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
		return fo.HandOver(provider, trackingNumber, now)
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

	conHangTrongKho := fo.Status().StockStillInWarehouse()

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
	if err := s.publishCancelled(ctx, fo, conHangTrongKho); err != nil {
		return err
	}
	return s.publishProgress(ctx, fo)
}

// publishCancelled phát event hủy kèm dòng hàng để inventory trả về kho.
func (s *Service) publishCancelled(
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
		lines = append(lines, CancelledLine{SKUID: l.SKUID, Quantity: l.Quantity})
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
) (ketQua error) {
	// Đếm ở advance chứ không ở từng bước: mọi chuyển trạng thái của đơn
	// thực hiện đều đi qua đây, nên một chỗ là đủ và không bước nào mới
	// thêm bị bỏ sót.
	//
	// Bọc bằng defer để MỌI đường thoát sớm đều được đếm — rải lời gọi ở
	// từng nhánh `return` là cách chắc chắn để bỏ sót một nhánh, và nhánh
	// bị sót thường là nhánh hiếm, tức nhánh đáng quan tâm nhất.
	defer func() {
		if ketQua != nil {
			metrics.RecordFailure(metrics.StageFulfillment, lyDoThatBai(ketQua))
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
	cutoff := now.Add(-ReturnWindow)

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

// CapNhatTuHangVanChuyenInput là một sự kiện vận chuyển đã xác minh chữ ký.
type CapNhatTuHangVanChuyenInput struct {
	NhaVanChuyen string
	MaVanDon     string

	// TrangThai theo bảng của đặc tả: PICKED_UP, IN_TRANSIT,
	// OUT_FOR_DELIVERY, DELIVERED, DELIVERY_FAILED, RETURNED_TO_SENDER.
	TrangThai string

	LyDoThatBai string
}

// ErrTrangThaiKhongDoi báo trạng thái này không kéo theo thay đổi nào.
//
// KHÔNG phải lỗi: hãng vận chuyển gửi nhiều mốc hơn số mốc hệ thống theo
// dõi, và đó là chuyện bình thường. Bên gọi ghi nhật ký rồi trả 200.
var ErrTrangThaiKhongDoi = errors.New("fulfillment: trạng thái không kéo theo thay đổi")

// CapNhatTuHangVanChuyen áp một mốc vận chuyển lên đơn thực hiện.
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
func (s *Service) CapNhatTuHangVanChuyen(
	ctx context.Context, in CapNhatTuHangVanChuyenInput,
) error {
	fo, err := s.repo.FindByTracking(ctx, in.NhaVanChuyen, in.MaVanDon)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	var buoc func(*domain.FulfillmentOrder, time.Time) error

	switch in.TrangThai {
	case "IN_TRANSIT", "OUT_FOR_DELIVERY":
		if fo.Status() == domain.FOInTransit {
			return nil
		}
		buoc = func(f *domain.FulfillmentOrder, t time.Time) error {
			return f.MarkInTransit(t)
		}

	case "DELIVERED":
		if fo.Status() == domain.FODelivered {
			return nil
		}
		buoc = func(f *domain.FulfillmentOrder, t time.Time) error {
			return f.Deliver(t)
		}

	case "DELIVERY_FAILED", "RETURNED_TO_SENDER":
		lyDo := strings.TrimSpace(in.LyDoThatBai)
		if lyDo == "" {
			lyDo = "hãng vận chuyển báo giao không thành công"
		}
		buoc = func(f *domain.FulfillmentOrder, t time.Time) error {
			return f.MarkDeliveryFailed(lyDo, t)
		}

	default:
		// PICKED_UP và mọi mốc khác: hệ thống không theo dõi, nhưng webhook
		// VẪN được ghi nhật ký ở tầng trên.
		return ErrTrangThaiKhongDoi
	}

	if err := buoc(fo, now); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, fo); err != nil {
		return err
	}
	return s.publishProgress(ctx, fo)
}

// lyDoThatBai phân loại lỗi thành nhãn Prometheus.
//
// Tập giá trị phải ĐÓNG và nhỏ: mỗi giá trị nhãn khác nhau là một chuỗi
// thời gian riêng, nên truyền thẳng thông điệp lỗi vào đây — thứ thường
// chứa mã đơn — sẽ làm nổ số chuỗi thời gian.
func lyDoThatBai(err error) string {
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
type Ky string

const (
	Ky7Ngay  Ky = "LAST_7_DAYS"
	Ky30Ngay Ky = domain.KyMacDinh
	Ky90Ngay Ky = "LAST_90_DAYS"
)

// KyHopLe kiểm kỳ và trả về độ dài.
//
// Tập ĐÓNG, đúng enum của đặc tả: cho phép kỳ tùy ý nghĩa là mở cửa cho
// một truy vấn quét toàn bộ bảng, và nhà bán không cần điều đó.
func KyHopLe(k Ky) (time.Duration, bool) {
	switch k {
	case Ky7Ngay:
		return 7 * 24 * time.Hour, true
	case Ky30Ngay:
		return 30 * 24 * time.Hour, true
	case Ky90Ngay:
		return 90 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

// HieuSuat là kết quả chấm hiệu suất của một nhà bán.
type HieuSuat struct {
	Ky        Ky
	SLAGio    float64
	SoLieu    domain.SoLieuHieuSuat
	ChiSo     []domain.ChiSoHieuSuat
	ChuaDo    []domain.ChuaDo
	ThongDiep string
}

// TinhHieuSuat chấm hiệu suất của MỘT nhà bán trong một kỳ.
//
// sellerID lấy từ phạm vi trong token, không phải từ tham số của người
// gọi: nhà bán không được xem hiệu suất của nhà bán khác.
func (s *Service) TinhHieuSuat(
	ctx context.Context, sellerID ids.ID, k Ky,
) (*HieuSuat, error) {
	doDai, ok := KyHopLe(k)
	if !ok {
		return nil, ErrKyKhongHopLe
	}

	den := s.clock.Now()
	tu := den.Add(-doDai)

	so, err := s.repo.DemHieuSuat(ctx, sellerID, tu, den, domain.SLAGiaoHang)
	if err != nil {
		return nil, err
	}

	chiSo := domain.TinhChiSo(so)
	return &HieuSuat{
		Ky:        k,
		SLAGio:    domain.SLAGiaoHang.Hours(),
		SoLieu:    so,
		ChiSo:     chiSo,
		ChuaDo:    domain.ChiSoChuaDo(),
		ThongDiep: thongDiep(so, chiSo),
	}, nil
}

// ErrKyKhongHopLe khi tham số `period` nằm ngoài tập cho phép.
var ErrKyKhongHopLe = errors.New("fulfillment: kỳ tính hiệu suất không hợp lệ")

// thongDiep nói cho nhà bán biết NÊN LÀM GÌ, không chỉ họ đang ở đâu.
//
// Đặc tả: "Seller cần hiểu vì sao mình không thắng buy box và cần làm gì
// để cải thiện." Một danh sách con số không trả lời vế thứ hai.
func thongDiep(so domain.SoLieuHieuSuat, chiSo []domain.ChiSoHieuSuat) string {
	if len(chiSo) == 0 {
		return fmt.Sprintf(
			"Chưa đủ dữ liệu để chấm: cần ít nhất %d đơn trong kỳ, hiện có %d.",
			domain.MauToiThieu, so.TongDon)
	}

	// Nêu chỉ số TỆ NHẤT, không nêu tất cả: một danh sách việc cần làm dài
	// thì không ai bắt đầu từ đâu.
	var tệ *domain.ChiSoHieuSuat
	for i := range chiSo {
		c := chiSo[i]
		if c.TrangThai == domain.ChiSoTot {
			continue
		}
		if tệ == nil || c.TrangThai == domain.ChiSoNghiemTrong {
			tệ = &chiSo[i]
		}
	}
	if tệ == nil {
		return "Mọi chỉ số đều đạt ngưỡng."
	}

	switch tệ.Ten {
	case "on_time_shipping_rate":
		return fmt.Sprintf(
			"Giao đúng hạn đang ở %.0f%%, dưới ngưỡng %.0f%%. "+
				"Hạn bàn giao là %.0f giờ kể từ khi đơn được tạo.",
			tệ.GiaTri*100, tệ.Nguong*100, domain.SLAGiaoHang.Hours())
	case "cancellation_rate":
		return fmt.Sprintf(
			"Tỷ lệ hủy đang ở %.1f%%, trên ngưỡng %.1f%%. "+
				"Mỗi đơn hủy là một khách đã đặt rồi không nhận được hàng.",
			tệ.GiaTri*100, tệ.Nguong*100)
	default:
		return "Có chỉ số chưa đạt ngưỡng."
	}
}
