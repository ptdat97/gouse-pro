// Package metrics là điểm khai báo DUY NHẤT cho mọi chỉ số của hệ thống.
//
// # Vì sao gom về một chỗ thay vì mỗi module tự khai
//
// Chỉ số chỉ dùng được khi tên và nhãn của chúng NHẤT QUÁN. Hai module tự
// đặt `checkout_errors` và `checkout_error_total` sẽ tạo ra hai biểu đồ
// không cộng được với nhau, và người trực sự cố lúc 3 giờ sáng phải đoán
// cái nào là thật.
//
// Gom về đây cũng làm lộ ra thứ quan trọng hơn: DANH SÁCH những gì hệ
// thống đang theo dõi, đọc được trong một màn hình.
//
// # Nguyên tắc chọn chỉ số
//
// Chỉ đo thứ dẫn tới một HÀNH ĐỘNG. Một chỉ số không ai biết phải làm gì
// khi nó tăng là một chỉ số làm loãng bảng theo dõi.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry giữ mọi chỉ số của tiến trình.
//
// Dùng registry RIÊNG thay vì mặc định toàn cục: registry mặc định gom cả
// chỉ số do thư viện bên thứ ba tự đăng ký, và khi đó không ai biết chắc
// bảng theo dõi đang hiện cái gì.
var Registry = prometheus.NewRegistry()

// ---------------------------------------------------------------- HTTP

var (
	// HTTPDuration đo độ trễ theo đường và mã trạng thái.
	//
	// Histogram chứ không phải trung bình: trung bình che mất cái đuôi, mà
	// cái đuôi mới là thứ khách cảm nhận. Một API có trung bình 80ms và
	// p99 4 giây là một API hỏng với 1% khách.
	HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "gouse_http_request_duration_seconds",
		Help: "Độ trễ request HTTP theo đường dẫn và mã trạng thái.",
		// Mốc dày ở khoảng 50ms–1s vì đó là vùng quyết định của một API
		// thương mại; thưa dần về sau vì phân biệt 8s với 10s không đổi
		// hành động nào.
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"route", "status"})

	// HTTPInFlight đếm request đang xử lý.
	//
	// Con số này tăng dần mà không giảm là dấu hiệu SỚM của cạn connection
	// pool hoặc một truy vấn treo — sớm hơn hẳn so với lúc độ trễ tăng.
	HTTPInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gouse_http_requests_in_flight",
		Help: "Số request HTTP đang được xử lý.",
	})
)

// ---------------------------------------------------------------- Outbox

var (
	// OutboxPending là số event CHƯA phát.
	//
	// ĐÂY LÀ CHỈ SỐ QUAN TRỌNG NHẤT của kiến trúc này. Nó tăng dần là
	// triệu chứng sớm của gần như mọi sự cố: worker chết, event kẹt, tồn
	// kho không chuyển Reserved → Committed — và khi đó tiến trình dọn có
	// thể nhả hàng của một đơn ĐÃ THANH TOÁN rồi bán cho người khác.
	//
	// Cảnh báo nên đặt trên ĐỘ TĂNG, không phải trên giá trị tuyệt đối:
	// tồn đọng 500 lúc cao điểm là bình thường, tồn đọng 50 suốt một giờ
	// mà không giảm thì không.
	OutboxPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gouse_outbox_pending_events",
		Help: "Số event trong outbox chưa được phát.",
	})

	// OutboxDeadLettered là số event đã bỏ cuộc sau nhiều lần thử.
	//
	// Con số này KHÁC 0 luôn cần người xem. Mỗi event ở đây là một việc
	// đáng ra phải xảy ra mà đã không xảy ra.
	OutboxDeadLettered = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gouse_outbox_dead_lettered_events",
		Help: "Số event đã bỏ cuộc sau khi thử lại hết số lần cho phép.",
	})

	// OutboxOldestAgeSeconds là tuổi của event chưa phát CŨ NHẤT.
	//
	// Bổ sung cho OutboxPending vì hai con số nói hai chuyện: tồn đọng lớn
	// mà tuổi nhỏ là đang bận; tồn đọng nhỏ mà tuổi lớn là có event KẸT.
	// Trường hợp thứ hai nguy hiểm hơn và dễ bị bỏ qua hơn.
	OutboxOldestAgeSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gouse_outbox_oldest_pending_age_seconds",
		Help: "Tuổi của event chưa phát cũ nhất, tính bằng giây.",
	})

	// WorkerHeartbeat là DẤU THỜI GIAN lần cuối worker chạy xong một vòng.
	//
	// # Vì sao gauge tồn đọng KHÔNG đủ
	//
	// `OutboxPending` do CHÍNH worker đặt. Worker chết thì con số đứng yên
	// ở giá trị cuối cùng — và "0 event tồn đọng" của một worker đã chết
	// trông y hệt "0 event tồn đọng" của một worker khỏe mạnh.
	//
	// Chuyện này đã xảy ra thật (26/08): worker chết cùng lúc PostgreSQL
	// tắt, để lại 67 event tồn đọng suốt nhiều giờ. Không đơn thực hiện
	// nào được tạo, không email nào được gửi, và không có gì kêu.
	//
	// # Cách dùng
	//
	// Cảnh báo trên ĐỘ TƯƠI, không phải trên giá trị:
	//
	//	time() - gouse_worker_heartbeat_timestamp_seconds > 60
	//
	// Kết hợp với `up{job="gouse-worker"} == 0` của Prometheus thì phủ
	// được cả hai kiểu chết: tiến trình biến mất (up = 0) và tiến trình
	// còn sống nhưng vòng lặp treo (nhịp tim cũ dần).
	//
	// Đơn vị là GIÂY UNIX chứ không phải "số giây kể từ lần cuối": một
	// dấu thời gian tuyệt đối vẫn đúng khi Prometheus thu thập trễ, còn
	// một khoảng thời gian tự tính thì không.
	WorkerHeartbeat = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gouse_worker_heartbeat_timestamp_seconds",
		Help: "Dấu thời gian Unix lần cuối worker hoàn tất một vòng chạy.",
	})

	// WorkerJobDuration đo thời gian mỗi job của worker.
	//
	// Bổ sung cho nhịp tim: nhịp tim nói "còn sống", cái này nói "còn kịp".
	// Một job chậm dần là dấu hiệu sớm của tồn đọng sắp tới.
	WorkerJobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gouse_worker_job_duration_seconds",
		Help:    "Thời gian chạy một lượt của từng job nền.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 15, 60},
	}, []string{"job"})

	// WorkerJobFailures đếm lượt chạy thất bại theo job.
	WorkerJobFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gouse_worker_job_failures_total",
		Help: "Số lượt chạy thất bại của từng job nền.",
	}, []string{"job"})

	// HandlerFailures đếm lần xử lý event thất bại, theo bên nhận.
	HandlerFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gouse_event_handler_failures_total",
		Help: "Số lần một bên nhận event xử lý thất bại.",
	}, []string{"handler", "event_type"})
)

// ---------------------------------------------- Thất bại theo nghiệp vụ

// BusinessFailures đếm những thất bại mà NGƯỜI VẬN HÀNH cần biết.
//
// Tách khỏi lỗi HTTP vì chúng trả lời câu khác. `500` nói "code hỏng";
// những con số ở đây nói "khách không mua được hàng" — và một hệ thống có
// thể trả 200 cho tất cả trong khi không ai đặt được đơn.
//
// `stage` là bước trong chuỗi: reservation · checkout · payment ·
// fulfillment. `reason` là lý do đã PHÂN LOẠI, không phải thông điệp lỗi
// thô — nhãn có số giá trị không giới hạn sẽ làm nổ bộ nhớ Prometheus.
var BusinessFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "gouse_business_failures_total",
	Help: "Thất bại nghiệp vụ theo bước và lý do đã phân loại.",
}, []string{"stage", "reason"})

// ---------------------------------------------------------------- Đăng ký

func init() {
	Registry.MustRegister(
		HTTPDuration, HTTPInFlight,
		OutboxPending, OutboxDeadLettered, OutboxOldestAgeSeconds,
		WorkerHeartbeat, WorkerJobDuration, WorkerJobFailures,
		HandlerFailures, BusinessFailures,

		// Chỉ số của chính tiến trình Go: số goroutine, bộ nhớ, GC.
		//
		// Số goroutine tăng đều không giảm là rò rỉ goroutine — thường do
		// một context không bao giờ bị hủy, và nó giết tiến trình sau vài
		// ngày chứ không phải vài phút.
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// Các bước trong chuỗi mua hàng, dùng làm nhãn `stage`.
const (
	StageReservation = "reservation"
	StageCheckout    = "checkout"
	StagePayment     = "payment"
	StageFulfillment = "fulfillment"
)

// RecordFailure đếm một thất bại nghiệp vụ.
//
// # Vì sao `reason` phải là chuỗi ĐÃ PHÂN LOẠI
//
// Nhãn Prometheus có bao nhiêu giá trị khác nhau thì có bấy nhiêu chuỗi
// thời gian. Truyền thẳng thông điệp lỗi vào đây — thứ thường chứa mã đơn
// hoặc tên sản phẩm — là tạo ra một chuỗi mới cho mỗi lần lỗi, và nó giết
// Prometheus trong vài giờ.
//
// Hàm này CẮT về một tập hữu hạn thay vì tin vào bên gọi: một nhãn sai ở
// chỗ hiếm gặp sẽ không ai phát hiện cho tới khi hệ thống theo dõi sập.
func RecordFailure(stage, reason string) {
	BusinessFailures.WithLabelValues(stage, safeReason(reason)).Inc()
}

// lyDoChoPhep là tập ĐÓNG các lý do được phép làm nhãn.
var lyDoChoPhep = map[string]bool{
	"out_of_stock":     true,
	"empty_cart":       true,
	"invalid_status":   true,
	"version_conflict": true,
	"not_found":        true,
	"forbidden":        true,
	"validation":       true,
	"conflict":         true,
	"internal":         true,
}

func safeReason(reason string) string {
	if lyDoChoPhep[reason] {
		return reason
	}
	return "other"
}
