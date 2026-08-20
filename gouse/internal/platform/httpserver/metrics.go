package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/metrics"
)

// Metrics đo độ trễ và số request đang xử lý.
//
// # Vì sao nhãn là MẪU đường dẫn chứ không phải đường dẫn thật
//
// `/api/v1/orders/ord_01ABC…` là một giá trị nhãn khác với
// `/api/v1/orders/ord_01XYZ…`. Dùng đường dẫn thật nghĩa là mỗi đơn hàng
// sinh ra một chuỗi thời gian riêng — Prometheus sẽ ngốn hết bộ nhớ trong
// vài giờ, và biểu đồ thì vô nghĩa vì mỗi đường chỉ có một điểm.
//
// Đây là lỗi kinh điển khi gắn metrics lần đầu, và nó chỉ lộ ra ở
// production nơi có dữ liệu thật.
//
// Go 1.22+ cho biết mẫu đã khớp qua `r.Pattern`, nên không phải tự chuẩn
// hóa bằng biểu thức chính quy.
func Metrics() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			metrics.HTTPInFlight.Inc()
			defer metrics.HTTPInFlight.Dec()

			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			metrics.HTTPDuration.WithLabelValues(
				routeLabel(r), strconv.Itoa(rec.status),
			).Observe(time.Since(start).Seconds())
		})
	}
}

// routeLabel trả mẫu đường dẫn đã khớp, hoặc "unknown".
//
// "unknown" gộp mọi request không khớp route nào — chủ yếu là 404. Gộp
// chúng lại là có chủ ý: một máy quét dò đường sẽ tạo ra hàng nghìn đường
// dẫn khác nhau, và mỗi cái một chuỗi thời gian là một cách để kẻ tấn công
// làm sập hệ thống theo dõi của chính mình.
func routeLabel(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return "unknown"
}
