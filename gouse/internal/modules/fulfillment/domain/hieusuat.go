package domain

import (
	"fmt"
	"time"
)

// DeliverySLA là thời hạn nhà bán phải bàn giao cho đơn vị vận chuyển,
// tính từ lúc đơn thực hiện được tạo.
//
// # Vì sao là một HẰNG SỐ CÔNG KHAI, không phải cột trong bảng
//
// Đặc tả yêu cầu "chỉ số, ngưỡng, và tác động đều công khai và tường
// minh", với lý do: mô hình chấm điểm hộp đen tạo tranh chấp không giải
// quyết được. Một thời hạn khác nhau theo từng đơn mà không ai nhìn thấy
// chính là hộp đen.
//
// Con số này đi thẳng vào response, nên nhà bán luôn đọc được thước đo
// đang dùng để chấm mình.
const DeliverySLA = 48 * time.Hour

// Ngưỡng đạt của từng chỉ số.
//
// Lấy từ ví dụ trong đặc tả (api/paths/seller.yaml#/performance) chứ không
// tự đặt: đó là con số đã được thống nhất khi viết hợp đồng API.
const (
	CancelRateThreshold = 0.03 // tỷ lệ hủy TỐI ĐA
	OnTimeThreshold     = 0.95 // tỷ lệ giao đúng hạn TỐI THIỂU
)

// Thresholds là bộ ngưỡng đang áp dụng.
//
// Gom thành MỘT kiểu thay vì ba tham số rời: ba con số float truyền liền
// nhau là chỗ dễ đảo thứ tự nhất, và đảo `CancelRate` với `OnTimeRate` cho ra
// kết luận ngược hẳn mà trình biên dịch không nói gì.
type Thresholds struct {
	DeliverySLA time.Duration
	CancelRate  float64
	OnTimeRate  float64
	MinSample   int
}

// DefaultThresholds là bộ ngưỡng dùng khi chưa ai đặt.
//
// Vẫn nằm ở đây, không chuyển hết sang cấu hình: hệ thống phải chạy đúng
// khi database không đọc được, và "chạy đúng" nghĩa là dùng đúng con số đã
// thống nhất khi viết hợp đồng API.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DeliverySLA: DeliverySLA,
		CancelRate:  CancelRateThreshold,
		OnTimeRate:  OnTimeThreshold,
		MinSample:   MinSample,
	}
}

// MetricStatus là kết luận về một chỉ số.
type MetricStatus string

const (
	MetricGood     MetricStatus = "GOOD"
	MetricWarning  MetricStatus = "WARNING"
	MetricCritical MetricStatus = "CRITICAL"
)

// PerformanceMetric là một phép đo kèm ngưỡng và kết luận.
type PerformanceMetric struct {
	Ten        string
	Value      float64
	Thresholds float64
	Status     MetricStatus
}

// PerformanceStats là số đếm thô lấy từ kho lưu trữ.
//
// Tách khỏi phần tính toán để việc phân loại ngưỡng kiểm được mà không cần
// database — đó là chỗ dễ sai nhất và cũng là chỗ rẻ nhất để kiểm.
type PerformanceStats struct {
	// TotalOrders là số đơn thực hiện tạo ra trong kỳ.
	TotalOrders int

	// CancelledOrders là số đơn đã hủy trong kỳ.
	CancelledOrders int

	// DeliveredOrders là số đơn đã bàn giao cho vận chuyển (có shipped_at).
	DeliveredOrders int

	// OnTimeOrders là số đơn bàn giao trong hạn SLA.
	OnTimeOrders int
}

// ComputeMetrics đổi số đếm thô thành danh sách chỉ số kèm kết luận.
//
// # Vì sao KHÔNG trả chỉ số khi mẫu quá nhỏ
//
// Một nhà bán có 3 đơn, hủy 1, ra tỷ lệ hủy 33% và bị chấm CRITICAL. Con
// số đó không nói lên điều gì về chất lượng — nó nói về việc mẫu quá nhỏ.
// Chấm một gian hàng mới mở là NGHIÊM TRỌNG vì họ hủy đúng một đơn là kiểu
// bất công mà đặc tả sinh ra để tránh.
//
// # Vì sao NHẬN ngưỡng thay vì đọc hằng số
//
// Ba con số này là CHÍNH SÁCH KINH DOANH, sửa được từ giao diện quản trị
// (xem platform/opsconfig). Domain không đọc cấu hình — nó nhận vào và
// tính, nên vẫn kiểm được mà không cần database, và vẫn đúng với bất kỳ
// bộ ngưỡng nào.
func ComputeMetrics(s PerformanceStats, ng Thresholds) []PerformanceMetric {
	var ra []PerformanceMetric

	if s.TotalOrders >= ng.MinSample {
		cancelRate := float64(s.CancelledOrders) / float64(s.TotalOrders)
		ra = append(ra, PerformanceMetric{
			Ten:        "cancellation_rate",
			Value:      cancelRate,
			Thresholds: ng.CancelRate,
			// Vượt ngưỡng là NGHIÊM TRỌNG, không phải cảnh báo: mỗi đơn
			// hủy là một khách đã trả tiền rồi không nhận được hàng.
			Status: gradeInverseThreshold(cancelRate, ng.CancelRate),
		})
	}

	if s.DeliveredOrders >= ng.MinSample {
		onTimeRate := float64(s.OnTimeOrders) / float64(s.DeliveredOrders)
		ra = append(ra, PerformanceMetric{
			Ten:        "on_time_shipping_rate",
			Value:      onTimeRate,
			Thresholds: ng.OnTimeRate,
			Status:     gradeAgainstThreshold(onTimeRate, ng.OnTimeRate),
		})
	}

	return ra
}

// DefaultPeriod là kỳ dùng khi người gọi không nêu.
//
// Khai ở domain để test và handler cùng đọc một nguồn: chép tay chuỗi
// "LAST_30_DAYS" ở hai nơi thì một ngày nào đó chúng lệch nhau.
const DefaultPeriod = "LAST_30_DAYS"

// MinSample là số đơn tối thiểu để một chỉ số có nghĩa.
const MinSample = 10

// gradeAgainstThreshold dùng cho chỉ số CÀNG CAO CÀNG TỐT.
//
// Vùng cảnh báo là 90% ngưỡng: rơi xuống dưới ngưỡng một chút là dấu hiệu
// cần chú ý, rơi sâu là vấn đề thật. Không có vùng đệm thì chỉ số nhảy
// giữa TỐT và NGHIÊM TRỌNG chỉ vì một đơn.
func gradeAgainstThreshold(value, thresholds float64) MetricStatus {
	switch {
	case value >= thresholds:
		return MetricGood
	case value >= thresholds*0.9:
		return MetricWarning
	default:
		return MetricCritical
	}
}

// gradeInverseThreshold dùng cho chỉ số CÀNG THẤP CÀNG TỐT.
func gradeInverseThreshold(value, thresholds float64) MetricStatus {
	switch {
	case value <= thresholds:
		return MetricGood
	case value <= thresholds*1.5:
		return MetricWarning
	default:
		return MetricCritical
	}
}

// Unmeasured là một chỉ số đặc tả có khai nhưng hệ thống CHƯA đo được.
//
// # Vì sao trả ra thay vì im lặng bỏ qua
//
// Đặc tả yêu cầu minh bạch để tránh tranh chấp. Trả bốn chỉ số và im lặng
// về ba chỉ số còn lại tạo ra đúng thứ hộp đen ấy, chỉ khác là ở phía
// người viết API: nhà bán không có cách nào biết mình đang được chấm bằng
// những gì.
//
// Nói thẳng "chưa đo, vì lý do này" trung thực hơn hẳn một con số bịa.
type Unmeasured struct {
	Ten    string
	Reason string
}

// UnmeasuredMetrics liệt kê mọi chỉ số KHÔNG có trong `metrics`, kèm lý do.
//
// # Hai lý do khác nhau, và trước đây chỉ một cái được nói ra
//
// Một chỉ số vắng mặt vì hai chuyện hoàn toàn khác nhau:
//
//	hệ thống chưa đo được   thiếu nguồn dữ liệu — vĩnh viễn cho tới khi
//	                        có người xây. Bốn chỉ số ở cuối hàm này.
//	chưa đủ MẪU             đo được, nhưng kỳ này quá ít đơn để con số có
//	                        nghĩa. Tự hết khi gian hàng bán thêm.
//
// Bản trước chỉ trả loại thứ nhất. Loại thứ hai biến mất KHÔNG một lời:
// `on_time_shipping_rate` đơn giản là không có trong `metrics`, và nhà bán
// không có cách nào biết vì sao — đúng thứ hộp đen mà endpoint này sinh ra
// để tránh, chỉ khác là ở phía người viết API.
//
// Nói "chưa đủ mẫu, cần N đơn, hiện có M" thì nhà bán biết phải làm gì và
// biết khi nào chỉ số sẽ xuất hiện.
func UnmeasuredMetrics(s PerformanceStats, ng Thresholds) []Unmeasured {
	ra := []Unmeasured{}

	// Hai chỉ số ĐO ĐƯỢC, vắng mặt vì mẫu. Điều kiện ở đây phải là PHỦ
	// ĐỊNH của điều kiện trong `ComputeMetrics` — lệch nhau thì sẽ có chỉ số
	// vừa nằm trong `metrics` vừa nằm ở đây, hoặc tệ hơn: vắng ở cả hai.
	if s.TotalOrders < ng.MinSample {
		ra = append(ra, Unmeasured{
			Ten: "cancellation_rate",
			Reason: fmt.Sprintf("chưa đủ mẫu: cần ít nhất %d đơn trong kỳ, "+
				"hiện có %d", ng.MinSample, s.TotalOrders),
		})
	}
	if s.DeliveredOrders < ng.MinSample {
		ra = append(ra, Unmeasured{
			Ten: "on_time_shipping_rate",
			Reason: fmt.Sprintf("chưa đủ mẫu: cần ít nhất %d đơn ĐÃ BÀN GIAO "+
				"trong kỳ, hiện có %d", ng.MinSample, s.DeliveredOrders),
		})
	}

	return append(ra, []Unmeasured{
		{
			Ten: "return_rate_description",
			Reason: "cần dữ liệu của module returns; đó là module ở tầng khác " +
				"nên phải qua một read model chung, chưa dựng",
		},
		{
			Ten:    "average_rating",
			Reason: "hệ thống chưa có đánh giá của khách — không có bảng nào lưu",
		},
		{
			Ten: "inventory_accuracy",
			Reason: "chưa thống nhất công thức: nhật ký kho có ghi lượt điều " +
				"chỉnh nhưng chưa định nghĩa thế nào là một lần lệch",
		},
		{
			Ten: "buy_box_win_rate",
			Reason: "buy box tính tại thời điểm hỏi và không được lưu lại, " +
				"nên không có lịch sử để tính tỷ lệ",
		},
	}...)
}
