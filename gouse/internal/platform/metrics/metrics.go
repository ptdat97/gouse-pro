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

	// WorkerHeartbeat là nhịp của BỘ ĐIỀU PHỐI, độc lập với công việc.
	//
	// # Bốn câu hỏi khác nhau, bốn chỉ số khác nhau
	//
	//	tiến trình còn sống?   up{job="gouse-worker"}   (Prometheus tự có)
	//	bộ điều phối còn chạy? WorkerHeartbeat
	//	job này còn tiến triển? WorkerJobLastSuccess{job}
	//	job này đang chạy?      WorkerJobRunning{job}
	//
	// Gộp chúng vào một con số là mất khả năng phân biệt, và mỗi lần mất
	// đi một cách phân biệt là một kiểu sự cố không ai thấy.
	//
	// # Vì sao KHÔNG đập theo lần hoàn tất job (sửa 26/08)
	//
	// Bản đầu đặt nhịp tim trong `runOnce`, tức mỗi lần MỘT job chạy xong.
	// Các job chạy ở goroutine riêng với nhịp 5s, 30s, 60s, 5 phút và 10
	// phút. Hệ quả là nhịp tim toàn cục được làm mới bởi BẤT KỲ job nào —
	// nên job quan trọng nhất (phát event, 5 giây) có thể treo hoàn toàn
	// trong khi job dọn dẹp 30 giây vẫn giữ nhịp tim tươi rói.
	//
	// Đó là ÂM TÍNH GIẢ: event chất đống, đơn thực hiện không được tạo, và
	// bảng theo dõi báo "worker khỏe". Nguy hiểm hơn hẳn dương tính giả,
	// vì dương tính giả thì có người tới xem.
	//
	// Nay nhịp tim do một ticker RIÊNG đặt, không phụ thuộc job nào. Nó
	// trả lời đúng một câu: bộ điều phối còn sống không. Tiến triển của
	// từng job hỏi `WorkerJobLastSuccess`.
	//
	// Đơn vị là GIÂY UNIX tuyệt đối chứ không phải "số giây kể từ lần
	// cuối": dấu thời gian vẫn đúng khi Prometheus thu thập trễ, còn một
	// khoảng thời gian tự tính thì không.
	WorkerHeartbeat = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gouse_worker_heartbeat_timestamp_seconds",
		Help: "Dấu thời gian Unix của nhịp bộ điều phối, độc lập với job.",
	})

	// WorkerJobLastSuccess là lần cuối MỖI job chạy xong THÀNH CÔNG.
	//
	// Đây là chỉ số bắt được một job treo — thứ nhịp tim toàn cục không
	// thấy. Cảnh báo đặt riêng cho từng job vì ngưỡng khác nhau: job phát
	// event chạy mỗi 5 giây, job tính chỉ số chạy mỗi 5 phút.
	//
	// Nhãn là `job_name`, KHÔNG phải `job`.
	//
	// `job` là nhãn DÀNH RIÊNG của Prometheus — nó gắn tên scrape job vào
	// mọi chuỗi. Đặt trùng tên thì nhãn của ứng dụng bị đổi thành
	// `exported_job`, và mọi biểu thức viết theo `job` sẽ khớp nhầm hoặc
	// không khớp gì. Lỗi này chỉ lộ ra khi chạy Prometheus thật.
	//
	// Tên job là một tập ĐÓNG và nhỏ (5 giá trị) — không có nguy cơ bùng
	// nổ số chuỗi thời gian.
	WorkerJobLastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gouse_worker_job_last_success_timestamp_seconds",
		Help: "Dấu thời gian Unix lần cuối job chạy xong thành công.",
	}, []string{"job_name"})

	// WorkerJobRunning cho biết job có ĐANG chạy hay không (1 hoặc 0).
	//
	// Tồn tại để cảnh báo phân biệt được hai thứ trông giống nhau: một job
	// TREO và một job đang chạy LÂU. Không có nó thì cảnh báo về độ tươi
	// sẽ kêu oan mỗi khi một lượt chạy dài hơn ngưỡng — đúng dương tính
	// giả mà thiết kế này phải tránh.
	WorkerJobRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gouse_worker_job_running",
		Help: "1 nếu job đang chạy, 0 nếu không.",
	}, []string{"job_name"})

	// WorkerJobDuration đo thời gian mỗi job của worker.
	//
	// Bổ sung cho nhịp tim: nhịp tim nói "còn sống", cái này nói "còn kịp".
	// Một job chậm dần là dấu hiệu sớm của tồn đọng sắp tới.
	WorkerJobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gouse_worker_job_duration_seconds",
		Help:    "Thời gian chạy một lượt của từng job nền.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 15, 60},
	}, []string{"job_name"})

	// WorkerJobFailures đếm lượt chạy thất bại theo job.
	WorkerJobFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gouse_worker_job_failures_total",
		Help: "Số lượt chạy thất bại của từng job nền.",
	}, []string{"job_name"})

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
		WorkerHeartbeat, WorkerJobLastSuccess, WorkerJobRunning,
		WorkerJobDuration, WorkerJobFailures,
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
	StageWebhook     = "webhook"
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

	// Webhook: ba lý do phân biệt được ba hành động khác nhau.
	//
	//   chu_ky_sai              → có thể là nỗ lực giả mạo, phải bằng 0
	//   khong_thay_ma_van_don   → dữ liệu lệch giữa ta và hãng vận chuyển
	//   xu_ly_that_bai          → lỗi phía ta, cần thử lại
	"chu_ky_sai":            true,
	"khong_thay_ma_van_don": true,
	"xu_ly_that_bai":        true,
	"than_khong_hop_le":     true,
}

func safeReason(reason string) string {
	if lyDoChoPhep[reason] {
		return reason
	}
	return "other"
}
