package domain

import (
	"time"
)

// Granularity là độ mịn của khoảng thời gian.
type Granularity string

const (
	GranularityHour  Granularity = "HOUR"
	GranularityDay   Granularity = "DAY"
	GranularityMonth Granularity = "MONTH"
)

// ValidGranularity kiểm tra độ mịn có hợp lệ không.
func ValidGranularity(g Granularity) bool {
	switch g {
	case GranularityHour, GranularityDay, GranularityMonth:
		return true
	}
	return false
}

// Truncate cắt tròn thời điểm về đầu khoảng.
//
// # Vì sao phải cắt tròn ở MỘT chỗ duy nhất
//
// Chỉ số của "ngày 15/3" phải luôn nằm ở cùng một hàng. Nếu chỗ ghi cắt
// tròn theo giờ địa phương còn chỗ đọc cắt theo UTC, dashboard sẽ hiện
// hai hàng cho cùng một ngày và không con số nào đúng.
//
// LUÔN dùng UTC: múi giờ của máy chủ thay đổi được, còn dữ liệu đã ghi
// thì không.
func (g Granularity) Truncate(t time.Time) time.Time {
	t = t.UTC()
	switch g {
	case GranularityHour:
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case GranularityDay:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return t
}

// Next trả thời điểm đầu khoảng KẾ TIẾP.
//
// Dùng AddDate cho tháng chứ không cộng 30 ngày: tháng có 28 tới 31 ngày,
// và cộng số ngày cố định sẽ trôi dần cho tới khi "tháng 3" bắt đầu từ
// giữa tháng 2.
func (g Granularity) Next(t time.Time) time.Time {
	switch g {
	case GranularityHour:
		return t.Add(time.Hour)
	case GranularityDay:
		return t.AddDate(0, 0, 1)
	case GranularityMonth:
		return t.AddDate(0, 1, 0)
	}
	return t
}

// Tên các chỉ số cốt lõi của MVP.
//
// Xem docs/01-business/kpi.md cho danh sách đầy đủ.
const (
	// MetricGMV là tổng giá trị hàng hóa đã bán.
	//
	// Đơn vị: số nguyên nhỏ nhất của tiền tệ.
	//
	// KHÔNG PHẢI DOANH THU của nền tảng — nền tảng chỉ nhận hoa hồng.
	// Nhầm hai khái niệm này là cách nhanh nhất để báo cáo sai cho nhà đầu
	// tư.
	MetricGMV = "gmv"

	// MetricOrderCount là số đơn hàng.
	MetricOrderCount = "order_count"

	// MetricAOV là giá trị đơn hàng trung bình = GMV / số đơn.
	//
	// Đơn vị: số nguyên nhỏ nhất của tiền tệ.
	MetricAOV = "aov"

	// MetricConversionRate là tỷ lệ chuyển đổi, tính bằng ĐIỂM CƠ BẢN.
	//
	//	1000 = 10%
	//
	// KHÔNG dùng số thực: tỷ lệ hiển thị sai ở chữ số thứ mười lăm vẫn là
	// hiển thị sai, và số nguyên thì không bao giờ sai.
	MetricConversionRate = "conversion_rate"

	// MetricSessionCount là số phiên truy cập.
	MetricSessionCount = "session_count"
)

// Metric là một chỉ số đã tính.
type Metric struct {
	Name string

	// PeriodStart đã CẮT TRÒN theo Granularity.
	PeriodStart time.Time
	Granularity Granularity

	// Dimension cho phép cắt lát: "seller", hoặc rỗng cho toàn sàn.
	Dimension      string
	DimensionValue string

	// Value là số nguyên. Với chỉ số tiền tệ là đơn vị nhỏ nhất, với tỷ
	// lệ là điểm cơ bản.
	Value int64

	// SampleSize là số bản ghi đã dùng để tính.
	//
	// Cần để ĐỌC CHỈ SỐ CHO ĐÚNG: tỷ lệ chuyển đổi 50% từ 2 lượt truy cập
	// không nói lên điều gì, còn từ 20.000 lượt thì có.
	SampleSize int64

	Currency string

	ComputedAt time.Time
}

// TimeRange là một khoảng thời gian truy vấn.
type TimeRange struct {
	// From là mốc BAO GỒM.
	From time.Time

	// To là mốc KHÔNG BAO GỒM.
	//
	// Nửa mở có chủ ý: hai khoảng liền nhau [1/3, 2/3) và [2/3, 3/3) không
	// đếm trùng ngày 2/3. Nếu cả hai mốc đều bao gồm, tổng của các khoảng
	// sẽ lớn hơn tổng thật.
	To time.Time
}

// Validate kiểm tra khoảng thời gian.
func (r TimeRange) Validate() error {
	if r.From.IsZero() || r.To.IsZero() {
		return ErrInvalidRange
	}
	if !r.To.After(r.From) {
		return ErrInvalidRange
	}
	return nil
}

// ComputeAOV tính giá trị đơn hàng trung bình.
//
// # Chia cho 0 trả về 0, không phải lỗi
//
// "Chưa có đơn nào" là trạng thái BÌNH THƯỜNG của một ngày mới bắt đầu
// hoặc một gian hàng mới mở. Trả lỗi sẽ khiến worker tính chỉ số dừng lại
// ở gian hàng đầu tiên chưa bán được gì.
//
// Chia số nguyên nên phần lẻ bị CẮT XUỐNG. AOV 199.999,7đ hiện thành
// 199.999đ — sai lệch dưới một đồng trên một con số hiển thị, chấp nhận
// được. Đây là chỉ số, không phải sổ cái.
func ComputeAOV(gmv int64, orderCount int64) int64 {
	if orderCount <= 0 {
		return 0
	}
	return gmv / orderCount
}

// ComputeConversionRate tính tỷ lệ chuyển đổi bằng ĐIỂM CƠ BẢN.
//
//	1000 = 10%
//
// Nhân TRƯỚC rồi chia SAU: chia trước sẽ ra 0 với mọi tỷ lệ dưới 100%.
//
// Không chặn trên 10000: tỷ lệ vượt 100% là DẤU HIỆU DỮ LIỆU SAI (nhiều
// đơn hơn số phiên), và che nó đi bằng cách cắt xuống 100% sẽ làm sự cố
// đó không bao giờ bị phát hiện.
func ComputeConversionRate(converted, total int64) int64 {
	if total <= 0 {
		return 0
	}
	return converted * 10000 / total
}
