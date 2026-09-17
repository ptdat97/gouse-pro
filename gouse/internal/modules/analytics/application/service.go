// Package application chứa các use case của module analytics.
package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/analytics/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return types.BayGio() }

var SystemClock Clock = systemClock{}

// Service là tầng application của module analytics.
type Service struct {
	events  domain.EventRepository
	metrics domain.MetricRepository
	clock   Clock
	log     *slog.Logger
}

type Deps struct {
	Events  domain.EventRepository
	Metrics domain.MetricRepository
	Clock   Clock
	Log     *slog.Logger
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		events:  d.Events,
		metrics: d.Metrics,
		clock:   clock,
		log:     log,
	}
}

// ---------------------------------------------------------------- Ghi nhận

// Track ghi nhận một sự kiện.
//
// # KHÔNG BAO GIỜ CHẶN LUỒNG CHÍNH (quy tắc 3)
//
// Hàm này trả nil khi ghi thất bại vì lý do hạ tầng, chỉ ghi cảnh báo ra
// nhật ký. Nếu analytics lỗi, việc BÁN HÀNG vẫn phải chạy bình thường —
// mất một bản ghi phân tích là chuyện nhỏ, chặn một đơn hàng thì không.
//
// Lỗi DUY NHẤT trả ra ngoài là dữ liệu không hợp lệ: đó là lỗi của bên
// gọi, sửa được, và im lặng nuốt nó sẽ khiến sự kiện biến mất mà không ai
// biết vì sao.
func (s *Service) Track(ctx context.Context, e domain.Event) error {
	e.Properties = domain.SanitizeProperties(e.Properties)

	if e.OccurredAt.IsZero() {
		e.OccurredAt = s.clock.Now()
	}
	e.RecordedAt = s.clock.Now()

	if e.Category == "" {
		e.Category = domain.CategoryBehavior
	}

	if err := e.Validate(); err != nil {
		return err
	}

	err := s.events.Record(ctx, e)
	switch {
	case err == nil:
		return nil

	case errors.Is(err, domain.ErrDuplicateEvent):
		// Sự kiện nghiệp vụ đã ghi rồi — kết quả mong muốn đã đạt.
		//
		// Handler event xử lý lại cùng một event là chuyện bình thường;
		// báo lỗi sẽ khiến nó thử lại mãi mãi.
		return nil

	default:
		// Sự cố hạ tầng: GHI NHẬT KÝ, KHÔNG trả lỗi.
		s.log.WarnContext(ctx, "analytics: không ghi được sự kiện",
			"event", e.Name, "error", err)
		return nil
	}
}

// TrackBatch ghi nhiều sự kiện trong một lượt.
//
// Sự kiện hành vi có khối lượng RẤT LỚN: ghi từng cái là một lượt đi-về
// database cho mỗi lần khách di chuột.
//
// Sự kiện KHÔNG HỢP LỆ bị bỏ qua chứ không làm hỏng cả lô — một bản ghi
// sai không được phép làm mất 999 bản ghi đúng.
//
// Trả về số sự kiện thật sự được ghi.
func (s *Service) TrackBatch(ctx context.Context, events []domain.Event) (int, error) {
	now := s.clock.Now()

	valid := make([]domain.Event, 0, len(events))
	for _, e := range events {
		e.Properties = domain.SanitizeProperties(e.Properties)
		if e.OccurredAt.IsZero() {
			e.OccurredAt = now
		}
		e.RecordedAt = now
		if e.Category == "" {
			e.Category = domain.CategoryBehavior
		}

		if err := e.Validate(); err != nil {
			s.log.WarnContext(ctx, "analytics: bỏ qua sự kiện không hợp lệ",
				"event", e.Name)
			continue
		}
		valid = append(valid, e)
	}

	if len(valid) == 0 {
		return 0, nil
	}

	n, err := s.events.RecordBatch(ctx, valid)
	if err != nil {
		s.log.WarnContext(ctx, "analytics: không ghi được lô sự kiện",
			"count", len(valid), "error", err)
		return n, nil
	}
	return n, nil
}

// AnonymizeCustomer gỡ định danh khỏi mọi sự kiện của một khách.
//
// Gọi khi khách yêu cầu xóa tài khoản. KHÔNG xóa hàng: số liệu tổng hợp đã
// tính từ chúng, và xóa đi sẽ làm GMV của các tháng trước thay đổi.
//
// KHÁC với Track, hàm này TRẢ LỖI: đây là nghĩa vụ pháp lý, và im lặng
// nuốt lỗi nghĩa là báo cáo "đã xóa" cho một việc chưa làm.
func (s *Service) AnonymizeCustomer(ctx context.Context, customerID string) (int, error) {
	return s.events.AnonymizeCustomer(ctx, customerID, s.clock.Now())
}

// ---------------------------------------------------------------- Chỉ số

// ComputeInput là dữ liệu tính chỉ số cho MỘT khoảng.
type ComputeInput struct {
	// PeriodStart sẽ được CẮT TRÒN theo Granularity.
	PeriodStart time.Time
	Granularity domain.Granularity

	// SellerID rỗng nghĩa là TOÀN SÀN.
	SellerID string

	Currency string
}

// ComputeMetrics tính và lưu các chỉ số cốt lõi cho một khoảng.
//
// # Bốn chỉ số của MVP
//
//	gmv              tổng giá trị hàng hóa đã bán
//	order_count      số đơn hàng
//	aov              gmv / order_count
//	conversion_rate  số phiên có mua / số phiên có xem sản phẩm
//
// GMV KHÔNG PHẢI DOANH THU của nền tảng — nền tảng chỉ nhận hoa hồng.
// Nhầm hai khái niệm này là cách nhanh nhất để báo cáo sai cho nhà đầu tư.
//
// # Vì sao tính từ event_log chứ không hỏi module order
//
// Quy tắc 1: analytics KHÔNG gọi module nghiệp vụ nào. Nếu nó hỏi `order`
// để tính GMV, một sự cố ở `order` sẽ làm sập cả dashboard — và tệ hơn,
// analytics trở thành một tải đọc bất thường lên database giao dịch.
func (s *Service) ComputeMetrics(ctx context.Context, in ComputeInput) error {
	if !domain.ValidGranularity(in.Granularity) {
		return domain.ErrInvalidRange
	}

	start := in.Granularity.Truncate(in.PeriodStart)
	r := domain.TimeRange{From: start, To: in.Granularity.Next(start)}
	now := s.clock.Now()

	currency := in.Currency
	if currency == "" {
		currency = "VND"
	}

	dimension, dimensionValue := "", ""
	if in.SellerID != "" {
		dimension, dimensionValue = "seller", in.SellerID
	}

	base := domain.Metric{
		PeriodStart:    start,
		Granularity:    in.Granularity,
		Dimension:      dimension,
		DimensionValue: dimensionValue,
		Currency:       currency,
		ComputedAt:     now,
	}

	// GMV cộng số tiền của mọi dòng sự kiện; SỐ ĐƠN đếm đơn KHÁC NHAU.
	//
	// Hai phép đếm khác nhau vì `order.placed` phát MỘT sự kiện cho MỖI
	// nhà bán. Cộng tiền theo dòng là đúng (các phần cộng lại thành tổng
	// đơn), nhưng đếm dòng thì một đơn trộn hàng ba nhà bán thành ba đơn
	// — và `aov` chia cho con số ấy nên sai theo.
	gmv, donCoTien, err := s.events.SumAmount(
		ctx, domain.EventOrderPlaced, r, in.SellerID)
	if err != nil {
		return err
	}
	orderCount, err := s.events.CountDistinctSubjects(
		ctx, domain.EventOrderPlaced, r, in.SellerID)
	if err != nil {
		return err
	}

	gmvMetric := base
	gmvMetric.Name = domain.MetricGMV
	gmvMetric.Value = gmv
	gmvMetric.SampleSize = orderCount
	if err := s.metrics.Upsert(ctx, gmvMetric); err != nil {
		return err
	}

	countMetric := base
	countMetric.Name = domain.MetricOrderCount
	countMetric.Value = orderCount
	countMetric.SampleSize = orderCount
	if err := s.metrics.Upsert(ctx, countMetric); err != nil {
		return err
	}

	// AOV chia cho số đơn ĐÃ GÓP TIỀN, không cho `orderCount`.
	//
	// Hai con số chỉ khác nhau khi có sự kiện `order.placed` thiếu
	// `amount` — dữ liệu hỏng của bên gọi. Khi đó `order_count` vẫn phải
	// nói đúng số đơn đã đặt, còn AOV thì không được để một bản ghi hỏng
	// kéo xuống.
	aovMetric := base
	aovMetric.Name = domain.MetricAOV
	aovMetric.Value = domain.ComputeAOV(gmv, donCoTien)
	aovMetric.SampleSize = donCoTien
	if err := s.metrics.Upsert(ctx, aovMetric); err != nil {
		return err
	}

	// SỐ LƯỢT TRUY CẬP đếm theo PHIÊN, không theo sự kiện.
	//
	// Một người xem 20 sản phẩm là 20 sự kiện nhưng MỘT lượt truy cập.
	// Dùng số sự kiện làm mẫu số sẽ ra tỷ lệ thấp hơn thực tế nhiều lần,
	// và người đọc sẽ đi tìm một vấn đề không tồn tại.
	viewSessions, err := s.events.CountDistinctSessions(
		ctx, domain.EventProductView, r, in.SellerID)
	if err != nil {
		return err
	}

	sessionMetric := base
	sessionMetric.Name = domain.MetricSessionCount
	sessionMetric.Value = viewSessions
	sessionMetric.SampleSize = viewSessions
	if err := s.metrics.Upsert(ctx, sessionMetric); err != nil {
		return err
	}

	// TỶ LỆ CHUYỂN ĐỔI: bao nhiêu lượt XEM HÀNG dẫn tới mua.
	//
	// Cả hai đầu nay đếm cùng một không gian mã — mã LƯỢT TRUY CẬP — nhờ
	// `checkout.completed` phiên bản 9 mang `visit_id` (ADR-0020 bước 3).
	// Trước đó tử số đếm mã phiên thanh toán còn mẫu số đếm mã lượt truy
	// cập, nên tỷ lệ bằng 0 vĩnh viễn dù bán được bao nhiêu.
	buySessions, err := s.events.CountDistinctSessions(
		ctx, domain.EventOrderPlaced, r, in.SellerID)
	if err != nil {
		return err
	}

	convMetric := base
	convMetric.Name = domain.MetricConversionRate
	convMetric.Value = domain.ComputeConversionRate(buySessions, viewSessions)
	convMetric.SampleSize = viewSessions
	if err := s.metrics.Upsert(ctx, convMetric); err != nil {
		return err
	}

	// TỶ LỆ PHIÊN THANH TOÁN THÀNH ĐƠN — đo được ngay, vì cả hai đầu đều
	// ở máy chủ.
	checkouts, err := s.events.CountDistinctSubjects(
		ctx, domain.EventCheckoutStart, r, in.SellerID)
	if err != nil {
		return err
	}

	cartConv := base
	cartConv.Name = domain.MetricCartConversionRate
	cartConv.Value = domain.ComputeConversionRate(orderCount, checkouts)
	cartConv.SampleSize = checkouts
	return s.metrics.Upsert(ctx, cartConv)
}

// GetMetric đọc một chỉ số đã tính.
//
// Trả về giá trị 0 nếu chưa tính — dashboard mở vào một ngày chưa có
// worker chạy là chuyện bình thường.
// GomLuotXem trả số PHIÊN đã xem từng sản phẩm trong một khoảng.
//
// Đường ĐỌC của dữ liệu hành vi, phục vụ việc dựng tín hiệu nhu cầu. Module
// này KHÔNG tự ghi tín hiệu — nó không biết supply-chain tồn tại; tầng
// composition nối hai đầu lại.
func (s *Service) GomLuotXem(
	ctx context.Context, tu, den time.Time,
) (map[string]int, error) {
	return s.events.GomLuotXemTheoSanPham(ctx, domain.TimeRange{From: tu, To: den})
}

func (s *Service) GetMetric(
	ctx context.Context, name string, periodStart time.Time,
	g domain.Granularity, sellerID string,
) (domain.Metric, error) {
	if !domain.ValidGranularity(g) {
		return domain.Metric{}, domain.ErrInvalidRange
	}

	dimension, dimensionValue := "", ""
	if sellerID != "" {
		dimension, dimensionValue = "seller", sellerID
	}

	return s.metrics.Get(ctx, name, g.Truncate(periodStart), g, dimension, dimensionValue)
}

// GetTimeSeries đọc chuỗi thời gian của một chỉ số.
func (s *Service) GetTimeSeries(
	ctx context.Context, name string, g domain.Granularity,
	r domain.TimeRange, sellerID string,
) ([]domain.Metric, error) {
	if !domain.ValidGranularity(g) {
		return nil, domain.ErrInvalidRange
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}

	dimension, dimensionValue := "", ""
	if sellerID != "" {
		dimension, dimensionValue = "seller", sellerID
	}

	return s.metrics.List(ctx, name, g, r, dimension, dimensionValue)
}

// CountEvents đếm sự kiện thô trong một khoảng.
//
// Đường đọc TRỰC TIẾP từ event_log, không qua chỉ số tính sẵn. Dùng khi
// cần con số mới nhất và chấp nhận truy vấn nặng hơn.
func (s *Service) CountEvents(
	ctx context.Context, name string, r domain.TimeRange, sellerID string,
) (int64, error) {
	if err := r.Validate(); err != nil {
		return 0, err
	}
	return s.events.CountEvents(ctx, name, r, sellerID)
}
