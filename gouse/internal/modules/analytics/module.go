package analytics

import (
	"context"
	"errors"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/modules/analytics/application"
	"github.com/fashion-commerce/platform/internal/modules/analytics/domain"
	analyticspg "github.com/fashion-commerce/platform/internal/modules/analytics/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/privacy"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Chống ghi trùng sự kiện nghiệp vụ dựa vào chỉ mục UNIQUE. Kiểm tra
	// trước khi ghi vẫn lọt khi hai worker cùng xử lý một event, và khi đó
	// GMV bị cộng hai lần cho một đơn hàng.
	Storage string

	DB  *database.DB
	Log *slog.Logger

	Clock application.Clock
}

// New khởi tạo module analytics.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"analytics: chỉ hỗ trợ kho lưu trữ postgres — chống ghi trùng sự " +
				"kiện nghiệp vụ cần chỉ mục UNIQUE ở tầng database")
	}
	if cfg.DB == nil {
		return nil, errors.New("analytics: bắt buộc phải có kết nối database")
	}

	pool := cfg.DB.Pool()

	return &Module{svc: application.NewService(application.Deps{
		Events:  analyticspg.NewEventStore(pool),
		Metrics: analyticspg.NewMetricStore(pool),
		Clock:   cfg.Clock,
		Log:     cfg.Log,
	})}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- API

func (m *Module) TrackEvent(ctx context.Context, e EventInput) error {
	if err := m.svc.Track(ctx, toDomainEvent(e)); err != nil {
		return translateErr(err)
	}
	return nil
}

func (m *Module) TrackBatch(ctx context.Context, events []EventInput) (int, error) {
	list := make([]domain.Event, 0, len(events))
	for _, e := range events {
		list = append(list, toDomainEvent(e))
	}

	n, err := m.svc.TrackBatch(ctx, list)
	if err != nil {
		return n, translateErr(err)
	}
	return n, nil
}

func (m *Module) GetMetric(ctx context.Context, req MetricRequest) (MetricView, error) {
	got, err := m.svc.GetMetric(ctx, req.Name, req.PeriodStart,
		domain.Granularity(req.Granularity), req.SellerID)
	if err != nil {
		return MetricView{}, translateErr(err)
	}
	return toMetricView(got), nil
}

func (m *Module) GetTimeSeries(
	ctx context.Context, req TimeSeriesRequest,
) ([]MetricView, error) {
	list, err := m.svc.GetTimeSeries(ctx, req.Name,
		domain.Granularity(req.Granularity),
		domain.TimeRange{From: req.From, To: req.To}, req.SellerID)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]MetricView, 0, len(list))
	for _, got := range list {
		out = append(out, toMetricView(got))
	}
	return out, nil
}

func (m *Module) ComputeMetrics(ctx context.Context, req ComputeRequest) error {
	return translateErr(m.svc.ComputeMetrics(ctx, application.ComputeInput{
		PeriodStart: req.PeriodStart,
		Granularity: domain.Granularity(req.Granularity),
		SellerID:    req.SellerID,
		Currency:    req.Currency,
	}))
}

func (m *Module) CountEvents(ctx context.Context, req CountRequest) (int64, error) {
	n, err := m.svc.CountEvents(ctx, req.Name,
		domain.TimeRange{From: req.From, To: req.To}, req.SellerID)
	if err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

func (m *Module) AnonymizeCustomer(ctx context.Context, customerID string) (int, error) {
	n, err := m.svc.AnonymizeCustomer(ctx, customerID)
	if err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

// ---------------------------------------------------------------- Chuyển đổi

func toDomainEvent(e EventInput) domain.Event {
	return domain.Event{
		Name:        e.Name,
		Category:    domain.Category(e.Category),
		CustomerID:  e.CustomerID,
		SessionID:   e.SessionID,
		SubjectType: e.SubjectType,
		SubjectID:   e.SubjectID,
		SellerID:    e.SellerID,
		Amount:      e.Amount,
		Currency:    e.Currency,
		Properties:  e.Properties,

		// Băm IP NGAY tại biên module: bên trong không bao giờ thấy địa
		// chỉ nguyên văn, nên không có chỗ nào lỡ tay ghi nó ra nhật ký.
		IPHash:    privacy.HashIP(e.IP),
		UserAgent: e.UserAgent,

		EventID:    e.EventID,
		OccurredAt: e.OccurredAt,
	}
}

func toMetricView(m domain.Metric) MetricView {
	return MetricView{
		Name:        m.Name,
		PeriodStart: m.PeriodStart,
		Granularity: string(m.Granularity),
		Value:       m.Value,
		SampleSize:  m.SampleSize,
		Currency:    m.Currency,
		ComputedAt:  m.ComputedAt,
		SellerID:    m.DimensionValue,
	}
}

// translateErr đổi lỗi domain thành lỗi CÔNG KHAI.
func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrInvalidEvent),
		errors.Is(err, domain.ErrInvalidRange):
		return ErrInvalidInput
	default:
		return err
	}
}
