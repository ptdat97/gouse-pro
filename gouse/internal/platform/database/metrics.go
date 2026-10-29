package database

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Tên chỉ số, đặt cùng quy ước với internal/platform/metrics.
const (
	connMetric     = "gouse_db_pool_connections"
	tenToiDa       = "gouse_db_pool_max_connections"
	waitMetric     = "gouse_db_pool_empty_acquire_total"
	cancelMetric   = "gouse_db_pool_canceled_acquire_total"
	waitTimeMetric = "gouse_db_pool_acquire_wait_seconds_total"
)

// PoolCollector phơi trạng thái pool kết nối cho Prometheus.
//
// # Vì sao là Collector chứ không phải Gauge cập nhật định kỳ
//
// Một goroutine nền ghi vào Gauge mỗi N giây có hai nhược điểm: giá trị
// đọc được luôn cũ tới N giây, và nó chạy kể cả khi không ai scrape. Pool
// đã tự giữ số liệu rồi — Collector chỉ đọc chúng ĐÚNG LÚC scrape, nên
// con số luôn là hiện tại và không tốn gì khi không ai hỏi.
//
// # Vì sao những con số này, và đọc chúng thế nào
//
//	connections{state}      acquired = đang có người dùng; idle = rảnh
//	max_connections         trần cấu hình
//	empty_acquire_total     số lần phải CHỜ vì pool cạn
//	canceled_acquire_total  số lần bỏ cuộc giữa chừng
//	acquire_wait_seconds    tổng thời gian đã chờ
//
// `empty_acquire_total` là chỉ số quan trọng nhất ở đây, và nó không hiện
// ra ở bất kỳ chỗ nào khác: khi pool cạn, request KHÔNG lỗi — nó chỉ đứng
// chờ. Độ trễ tăng vọt trong khi tỷ lệ lỗi vẫn đẹp, và người trực nhìn vào
// bảng theo dõi không hiểu vì sao trang chậm.
//
// `canceled_acquire_total` là bước tiếp theo của cùng vấn đề: chờ lâu quá
// thì context hết hạn. Khác 0 nghĩa là đã có khách bị từ chối vì hết kết
// nối, không phải vì nghiệp vụ.
type PoolCollector struct {
	db *DB

	connect     *prometheus.Desc
	toiDa       *prometheus.Desc
	waitForSlot *prometheus.Desc
	cancelled   *prometheus.Desc
	waitTime    *prometheus.Desc
}

var _ prometheus.Collector = (*PoolCollector)(nil)

// NewPoolCollector tạo bộ thu thập cho một pool.
//
// `nhan` phân biệt nhiều pool trong cùng tiến trình (ví dụ api và worker
// dùng chung mã nhưng khác cấu hình). Để trống thì nhãn `pool` là "chính".
func NewPoolCollector(db *DB, labels string) *PoolCollector {
	if labels == "" {
		labels = "chinh"
	}
	fixedLabels := prometheus.Labels{"pool": labels}

	return &PoolCollector{
		db: db,
		connect: prometheus.NewDesc(connMetric,
			"Số kết nối trong pool theo trạng thái.",
			[]string{"state"}, fixedLabels),
		toiDa: prometheus.NewDesc(tenToiDa,
			"Trần số kết nối của pool.",
			nil, fixedLabels),
		waitForSlot: prometheus.NewDesc(waitMetric,
			"Số lần phải chờ vì pool đã cạn kết nối.",
			nil, fixedLabels),
		cancelled: prometheus.NewDesc(cancelMetric,
			"Số lần bỏ cuộc khi đang chờ kết nối.",
			nil, fixedLabels),
		waitTime: prometheus.NewDesc(waitTimeMetric,
			"Tổng thời gian đã chờ để lấy kết nối, tính bằng giây.",
			nil, fixedLabels),
	}
}

func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connect
	ch <- c.toiDa
	ch <- c.waitForSlot
	ch <- c.cancelled
	ch <- c.waitTime
}

func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	// Pool đã đóng (hoặc chưa mở) thì KHÔNG phát gì.
	//
	// Phát số 0 sẽ tệ hơn im lặng: bảng theo dõi hiện "0 kết nối đang
	// dùng, 0 lượt chờ" — trông y hệt một hệ thống khỏe mạnh đang rảnh.
	if c.db == nil || c.db.pool == nil || c.db.Closed() {
		return
	}
	s := c.db.pool.Stat()

	ch <- prometheus.MustNewConstMetric(c.connect,
		prometheus.GaugeValue, float64(s.AcquiredConns()), "acquired")
	ch <- prometheus.MustNewConstMetric(c.connect,
		prometheus.GaugeValue, float64(s.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(c.toiDa,
		prometheus.GaugeValue, float64(s.MaxConns()))

	// Counter, không phải Gauge: đây là số cộng dồn từ lúc mở pool, và
	// cảnh báo phải đặt trên ĐỘ TĂNG. Tổng 500 lượt chờ tích trong một
	// tuần là bình thường; 500 lượt trong một phút thì không.
	ch <- prometheus.MustNewConstMetric(c.waitForSlot,
		prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.cancelled,
		prometheus.CounterValue, float64(s.CanceledAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.waitTime,
		prometheus.CounterValue, s.AcquireDuration().Seconds())
}
