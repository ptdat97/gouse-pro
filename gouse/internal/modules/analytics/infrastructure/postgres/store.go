// Package postgres cài đặt kho lưu trữ analytics bằng PostgreSQL.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/modules/analytics/domain"
)

// EventStore ghi và đọc sự kiện.
type EventStore struct {
	pool *pgxpool.Pool
}

func NewEventStore(pool *pgxpool.Pool) *EventStore {
	return &EventStore{pool: pool}
}

var _ domain.EventRepository = (*EventStore)(nil)

const insertEvent = `
	INSERT INTO event_log (
		event_name, category, customer_id, session_id,
		subject_type, subject_id, seller_id,
		amount, currency, properties,
		ip_hash, user_agent, event_id, occurred_at, recorded_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`

func (s *EventStore) Record(ctx context.Context, e domain.Event) error {
	props, err := marshalProps(e.Properties)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, insertEvent, eventArgs(e, props)...)
	if err != nil {
		if isUnique(err, "event_log_event_id_key") {
			return domain.ErrDuplicateEvent
		}
		return fmt.Errorf("analytics: ghi sự kiện: %w", err)
	}
	return nil
}

// RecordBatch ghi nhiều sự kiện trong MỘT lượt đi-về database.
//
// # Vì sao ON CONFLICT DO NOTHING chứ không phải giao dịch thất bại
//
// Một event lặp trong lô KHÔNG được phép chặn 999 event còn lại. Sự kiện
// hành vi có khối lượng rất lớn và đến từ nhiều nguồn; mất cả lô vì một
// bản ghi trùng là đánh đổi tệ.
//
// Dùng pgx.Batch chứ không nối chuỗi SQL: nối chuỗi với dữ liệu người
// dùng là đường vào của SQL injection, và một trường properties chứa dấu
// nháy là đủ.
func (s *EventStore) RecordBatch(
	ctx context.Context, events []domain.Event,
) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, e := range events {
		props, err := marshalProps(e.Properties)
		if err != nil {
			return 0, err
		}
		batch.Queue(insertEvent+` ON CONFLICT DO NOTHING`, eventArgs(e, props)...)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	var written int
	for range events {
		tag, err := results.Exec()
		if err != nil {
			return written, fmt.Errorf("analytics: ghi lô sự kiện: %w", err)
		}
		written += int(tag.RowsAffected())
	}
	return written, nil
}

func eventArgs(e domain.Event, props []byte) []any {
	recorded := e.RecordedAt
	if recorded.IsZero() {
		recorded = e.OccurredAt
	}

	return []any{
		e.Name, string(e.Category), e.CustomerID, e.SessionID,
		e.SubjectType, e.SubjectID, e.SellerID,
		e.Amount, currencyOrDefault(e.Currency), props,
		e.IPHash, e.UserAgent, e.EventID, e.OccurredAt, recorded,
	}
}

func currencyOrDefault(c string) string {
	if c == "" {
		return "VND"
	}
	return c
}

func marshalProps(in map[string]any) ([]byte, error) {
	if len(in) == 0 {
		return []byte(`{}`), nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("analytics: mã hóa thuộc tính: %w", err)
	}
	return b, nil
}

// sellerFilter thêm điều kiện gian hàng khi cần.
//
// Chuỗi rỗng nghĩa là TOÀN SÀN, không phải "gian hàng có id rỗng". Nhầm
// hai thứ này sẽ làm dashboard toàn sàn hiện số 0.
func sellerFilter(sellerID string) (string, []any) {
	if sellerID == "" {
		return "", nil
	}
	return ` AND seller_id = $4`, []any{sellerID}
}

func (s *EventStore) CountEvents(
	ctx context.Context, name string, r domain.TimeRange, sellerID string,
) (int64, error) {
	clause, extra := sellerFilter(sellerID)
	args := append([]any{name, r.From, r.To}, extra...)

	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM event_log
		 WHERE event_name = $1 AND occurred_at >= $2 AND occurred_at < $3`+clause,
		args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("analytics: đếm sự kiện: %w", err)
	}
	return n, nil
}

// CountDistinctSessions đếm số PHIÊN khác nhau có sự kiện này.
//
// Khác CountEvents: một người xem 20 sản phẩm là 20 sự kiện nhưng MỘT
// phiên. Dùng số sự kiện làm mẫu số của tỷ lệ chuyển đổi sẽ ra con số thấp
// hơn thực tế nhiều lần.
func (s *EventStore) CountDistinctSessions(
	ctx context.Context, name string, r domain.TimeRange, sellerID string,
) (int64, error) {
	clause, extra := sellerFilter(sellerID)
	args := append([]any{name, r.From, r.To}, extra...)

	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT session_id) FROM event_log
		 WHERE event_name = $1 AND occurred_at >= $2 AND occurred_at < $3
		   AND session_id <> ''`+clause,
		args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("analytics: đếm phiên: %w", err)
	}
	return n, nil
}

// SumAmount cộng số tiền của các sự kiện trong một khoảng.
//
// COALESCE vì SUM trả NULL khi không có hàng nào — và quét NULL vào int64
// sẽ lỗi thay vì trả 0. "Chưa bán được gì" là trạng thái bình thường của
// một ngày mới bắt đầu.
//
// Đếm riêng số bản ghi CÓ amount: sự kiện amount NULL không liên quan tới
// tiền, và tính chúng vào sample_size sẽ làm AOV thấp giả tạo.
func (s *EventStore) SumAmount(
	ctx context.Context, name string, r domain.TimeRange, sellerID string,
) (int64, int64, error) {
	clause, extra := sellerFilter(sellerID)
	args := append([]any{name, r.From, r.To}, extra...)

	var total, count int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(amount), 0), count(amount) FROM event_log
		 WHERE event_name = $1 AND occurred_at >= $2 AND occurred_at < $3`+clause,
		args...).Scan(&total, &count)
	if err != nil {
		return 0, 0, fmt.Errorf("analytics: cộng số tiền: %w", err)
	}
	return total, count, nil
}

// AnonymizeCustomer gỡ định danh khỏi mọi sự kiện của một khách.
//
// KHÔNG xóa hàng: số liệu tổng hợp đã tính từ chúng, và xóa đi sẽ làm GMV
// của các tháng trước thay đổi — một chuyện không giải thích được với ai.
//
// Xóa cả session_id, ip_hash và user_agent: giữ lại chúng thì vẫn nối
// được các sự kiện thành hành vi của MỘT người, và việc ẩn danh thành vô
// nghĩa.
func (s *EventStore) AnonymizeCustomer(
	ctx context.Context, customerID string, _ time.Time,
) (int, error) {
	if customerID == "" {
		// Chuỗi rỗng khớp với MỌI sự kiện của khách chưa đăng nhập. Ẩn
		// danh toàn bộ chúng vì một lời gọi sai tham số là mất dữ liệu
		// không tạo ngược được.
		return 0, nil
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE event_log
		   SET customer_id = '', session_id = '', ip_hash = '', user_agent = ''
		 WHERE customer_id = $1`, customerID)
	if err != nil {
		return 0, fmt.Errorf("analytics: ẩn danh sự kiện: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// ---------------------------------------------------------------- Chỉ số

// MetricStore lưu và đọc chỉ số tính sẵn.
type MetricStore struct {
	pool *pgxpool.Pool
}

func NewMetricStore(pool *pgxpool.Pool) *MetricStore {
	return &MetricStore{pool: pool}
}

var _ domain.MetricRepository = (*MetricStore)(nil)

const metricCols = `
	metric_name, period_start, granularity, dimension, dimension_value,
	value, sample_size, currency, computed_at`

// Upsert ghi hoặc GHI ĐÈ một chỉ số.
//
// Ghi đè chứ không thêm hàng mới: hai giá trị GMV cho cùng một ngày là hai
// câu trả lời cho cùng một câu hỏi, và không có cách nào biết cái nào
// đúng.
func (s *MetricStore) Upsert(ctx context.Context, m domain.Metric) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO metric_snapshot (`+metricCols+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (metric_name, period_start, granularity, dimension, dimension_value)
		DO UPDATE SET value = EXCLUDED.value,
		              sample_size = EXCLUDED.sample_size,
		              currency = EXCLUDED.currency,
		              computed_at = EXCLUDED.computed_at`,
		m.Name, m.PeriodStart, string(m.Granularity), m.Dimension,
		m.DimensionValue, m.Value, m.SampleSize,
		currencyOrDefault(m.Currency), m.ComputedAt)
	if err != nil {
		return fmt.Errorf("analytics: ghi chỉ số: %w", err)
	}
	return nil
}

func (s *MetricStore) Get(
	ctx context.Context, name string, periodStart time.Time,
	g domain.Granularity, dimension, dimensionValue string,
) (domain.Metric, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+metricCols+`
		  FROM metric_snapshot
		 WHERE metric_name = $1 AND period_start = $2 AND granularity = $3
		   AND dimension = $4 AND dimension_value = $5`,
		name, periodStart, string(g), dimension, dimensionValue)

	m, err := scanMetric(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// Chỉ số CHƯA TÍNH trả về giá trị 0, không phải lỗi.
		//
		// Dashboard mở vào một ngày chưa có worker chạy là chuyện bình
		// thường, và "chưa có dữ liệu" hiển thị đúng là số 0.
		return domain.Metric{
			Name:           name,
			PeriodStart:    periodStart,
			Granularity:    g,
			Dimension:      dimension,
			DimensionValue: dimensionValue,
		}, nil
	}
	if err != nil {
		return domain.Metric{}, fmt.Errorf("analytics: đọc chỉ số: %w", err)
	}
	return m, nil
}

func (s *MetricStore) List(
	ctx context.Context, name string, g domain.Granularity,
	r domain.TimeRange, dimension, dimensionValue string,
) ([]domain.Metric, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+metricCols+`
		  FROM metric_snapshot
		 WHERE metric_name = $1 AND granularity = $2
		   AND period_start >= $3 AND period_start < $4
		   AND dimension = $5 AND dimension_value = $6
		 ORDER BY period_start`,
		name, string(g), r.From, r.To, dimension, dimensionValue)
	if err != nil {
		return nil, fmt.Errorf("analytics: đọc chuỗi chỉ số: %w", err)
	}
	defer rows.Close()

	var out []domain.Metric
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			return nil, fmt.Errorf("analytics: đọc chuỗi chỉ số: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanMetric(row interface{ Scan(...any) error }) (domain.Metric, error) {
	var (
		m           domain.Metric
		granularity string
	)
	if err := row.Scan(&m.Name, &m.PeriodStart, &granularity,
		&m.Dimension, &m.DimensionValue, &m.Value, &m.SampleSize,
		&m.Currency, &m.ComputedAt); err != nil {
		return domain.Metric{}, err
	}
	m.Granularity = domain.Granularity(granularity)
	return m, nil
}

// ---------------------------------------------------------------- Tiện ích

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
