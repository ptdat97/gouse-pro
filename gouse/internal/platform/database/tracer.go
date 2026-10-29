package database

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	queryTimeMetric = "gouse_db_query_duration_seconds"
	queryErrMetric  = "gouse_db_query_errors_total"
)

// ThaoTac là tập ĐÓNG các loại câu lệnh dùng làm nhãn.
//
// # Vì sao KHÔNG lấy câu SQL làm nhãn
//
// Mỗi giá trị nhãn khác nhau là một chuỗi thời gian riêng. Câu SQL thì gần
// như vô hạn — chỉ cần một câu sinh động (danh sách IN dài khác nhau, hay
// tệ hơn là nội suy giá trị vào chuỗi) là Prometheus phình tới mức sập.
//
// Đây không phải nỗi lo lý thuyết: nó là cách phổ biến nhất khiến một hệ
// thống giám sát tự giết chính nó, và nó xảy ra âm thầm — mọi thứ chạy tốt
// cho tới khi Prometheus hết bộ nhớ lúc 3 giờ sáng.
//
// # Vì sao chia theo ĐỘNG TỪ là đủ
//
// Câu hỏi vận hành thật sự là "đọc chậm hay ghi chậm": đọc chậm thường là
// thiếu chỉ mục, ghi chậm thường là tranh chấp khóa. Hai hướng điều tra
// hoàn toàn khác nhau, và động từ đủ để phân biệt.
//
// Muốn biết truy vấn NÀO chậm thì `pg_stat_statements` trả lời tốt hơn
// hẳn, và trả lời ở đúng chỗ — trong database, nơi có sẵn kế hoạch thực
// thi. Cố nhét việc đó vào Prometheus là dùng sai công cụ.
const (
	OpSelect = "select"
	OpInsert = "insert"
	OpUpdate = "update"
	OpDelete = "delete"
	OpOther  = "khac"
)

// QueryTracer đo thời gian và đếm lỗi của mọi câu lệnh đi qua pool.
type QueryTracer struct {
	elapsed    *prometheus.HistogramVec
	errCounter *prometheus.CounterVec
}

var (
	_ pgx.QueryTracer      = (*QueryTracer)(nil)
	_ prometheus.Collector = (*QueryTracer)(nil)
)

// startKey là kiểu khóa riêng để nhét mốc thời gian vào context.
//
// Kiểu RIÊNG chứ không phải string: khóa kiểu string có thể trùng với khóa
// của gói khác và một bên sẽ lặng lẽ ghi đè giá trị của bên kia.
type startKey struct{}

// NewQueryTracer tạo bộ đo cho một pool.
func NewQueryTracer(labels string) *QueryTracer {
	if labels == "" {
		labels = "chinh"
	}
	fixedLabels := prometheus.Labels{"pool": labels}

	return &QueryTracer{
		elapsed: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        queryTimeMetric,
			Help:        "Thời gian thực thi câu lệnh database, theo loại thao tác.",
			ConstLabels: fixedLabels,

			// Thang đo trải từ 1ms tới ~4s.
			//
			// Mốc dưới đủ nhỏ để thấy truy vấn nhanh tách khỏi truy vấn
			// trung bình — nếu mốc nhỏ nhất là 10ms thì phần lớn truy vấn
			// rơi hết vào một ô và biểu đồ không nói lên gì.
			//
			// Mốc trên 4s vì quá đó thì con số chính xác không còn quan
			// trọng: truy vấn 4 giây và truy vấn 40 giây dẫn tới cùng một
			// hành động.
			Buckets: []float64{
				0.001, 0.005, 0.01, 0.025, 0.05,
				0.1, 0.25, 0.5, 1, 2, 4,
			},
		}, []string{"operation"}),

		errCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        queryErrMetric,
			Help:        "Số câu lệnh database trả lỗi, theo loại thao tác.",
			ConstLabels: fixedLabels,
		}, []string{"operation"}),
	}
}

func (t *QueryTracer) Describe(ch chan<- *prometheus.Desc) {
	t.elapsed.Describe(ch)
	t.errCounter.Describe(ch)
}

func (t *QueryTracer) Collect(ch chan<- prometheus.Metric) {
	t.elapsed.Collect(ch)
	t.errCounter.Collect(ch)
}

func (t *QueryTracer) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData,
) context.Context {
	return context.WithValue(ctx, startKey{}, time.Now())
}

func (t *QueryTracer) TraceQueryEnd(
	ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData,
) {
	start, ok := ctx.Value(startKey{}).(time.Time)
	if !ok {
		// Không có mốc bắt đầu thì KHÔNG đoán bừa một con số.
		//
		// Ghi 0 giây vào biểu đồ sẽ kéo phân vị xuống và làm hệ thống
		// trông nhanh hơn thực tế — sai theo hướng nguy hiểm nhất.
		return
	}

	op := opFromTag(data.CommandTag)
	t.elapsed.WithLabelValues(op).Observe(time.Since(start).Seconds())

	// KHÔNG cần lọc `pgx.ErrNoRows` ở đây, và việc đó đã được kiểm chứng
	// bằng thực nghiệm chứ không phải suy đoán: pgx báo kết thúc truy vấn
	// TRƯỚC khi `Row.Scan` chạy, nên "không có dòng nào" nổi lên ở Scan và
	// `data.Err` vẫn là nil.
	//
	// Điều đó QUAN TRỌNG với chỉ số này: "không tìm thấy" là câu trả lời
	// hợp lệ và xảy ra liên tục ở đường tra cứu. Nếu nó lọt vào đây, con
	// số lỗi sẽ luôn khác 0 và vì thế không ai dùng để cảnh báo nữa.
	//
	// Bản đầu có thêm một lá chắn `!errors.Is(data.Err, pgx.ErrNoRows)`.
	// Nó là MÃ CHẾT — gỡ đi thì không bài test nào đổi kết quả — nên bỏ,
	// thay bằng ghi chú này. Giữ lại sẽ ngụ ý một mối nguy không có thật.
	if data.Err != nil {
		t.errCounter.WithLabelValues(op).Inc()
	}
}

// opFromTag lấy loại thao tác từ CommandTag mà PostgreSQL trả về.
//
// Dùng CommandTag chứ không phân tích chuỗi SQL: đó là điều PostgreSQL nói
// về câu lệnh nó VỪA CHẠY, nên nó đúng kể cả khi câu lệnh có chú thích ở
// đầu, có CTE, hay trải nhiều dòng — ba thứ mà cách cắt tiền tố SQL đều
// đọc sai.
//
// Câu lệnh lỗi thì CommandTag rỗng và ta trả `khac`; điều đó chấp nhận
// được vì lỗi đã được đếm riêng.
func opFromTag(tag pgconn.CommandTag) string {
	s := tag.String()
	if i := strings.IndexByte(s, ' '); i > 0 {
		s = s[:i]
	}
	switch strings.ToUpper(s) {
	case "SELECT":
		return OpSelect
	case "INSERT":
		return OpInsert
	case "UPDATE":
		return OpUpdate
	case "DELETE":
		return OpDelete
	default:
		return OpOther
	}
}
