package domain_test

import (
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

func findMetric(ds []domain.PerformanceMetric, ten string) (domain.PerformanceMetric, bool) {
	for _, c := range ds {
		if c.Ten == ten {
			return c, true
		}
	}
	return domain.PerformanceMetric{}, false
}

// TestTooSmallASampleIsNotGraded là hàng rào chống bất công.
//
// Một gian hàng mới có 3 đơn, hủy 1, ra tỷ lệ 33% và bị chấm CRITICAL. Con
// số đó không nói gì về chất lượng — nó nói mẫu quá nhỏ. Đặc tả sinh ra để
// tránh đúng kiểu bất công này ("mô hình chấm điểm hộp đen tạo tranh chấp
// không giải quyết được và cảm giác bất công").
func TestTooSmallASampleIsNotGraded(t *testing.T) {
	ds := domain.ComputeMetrics(domain.PerformanceStats{
		TotalOrders: 3, CancelledOrders: 1, DeliveredOrders: 2, OnTimeOrders: 1,
	}, domain.DefaultThresholds())
	if len(ds) != 0 {
		t.Errorf("mẫu 3 đơn vẫn bị chấm %d chỉ số — gian hàng mới mở hủy "+
			"một đơn sẽ bị đánh giá NGHIÊM TRỌNG: %+v", len(ds), ds)
	}
}

// TestWithEnoughSampleItIsGraded: qua ngưỡng mẫu thì phải có chỉ số.
func TestWithEnoughSampleItIsGraded(t *testing.T) {
	ds := domain.ComputeMetrics(domain.PerformanceStats{
		TotalOrders: 100, CancelledOrders: 2, DeliveredOrders: 90, OnTimeOrders: 88,
	}, domain.DefaultThresholds())
	if len(ds) != 2 {
		t.Fatalf("cần 2 chỉ số, có %d: %+v", len(ds), ds)
	}

	cancel, _ := findMetric(ds, "cancellation_rate")
	if cancel.Value != 0.02 {
		t.Errorf("tỷ lệ hủy = %v, cần 0.02", cancel.Value)
	}
	if cancel.Status != domain.MetricGood {
		t.Errorf("hủy 2%% dưới ngưỡng 3%% mà xếp %s", cancel.Status)
	}
}

// TestHaiChiSoNGUOC CHIỀU nhau, và nhầm chiều là lỗi im lặng.
//
// `cancellation_rate` càng THẤP càng tốt; `on_time_shipping_rate` càng CAO
// càng tốt. Dùng chung một phép so sánh cho cả hai sẽ cho ra kết luận
// ngược hẳn — và không có gì báo, vì cả hai đều là số trong khoảng 0..1.
func TestTwoMetricsPointInOppositeDirections(t *testing.T) {
	// Hủy 10% — RẤT xấu. Giao đúng hạn 10% — cũng RẤT xấu.
	ds := domain.ComputeMetrics(domain.PerformanceStats{
		TotalOrders: 100, CancelledOrders: 10, DeliveredOrders: 100, OnTimeOrders: 10,
	}, domain.DefaultThresholds())

	cancel, ok := findMetric(ds, "cancellation_rate")
	if !ok {
		t.Fatal("thiếu cancellation_rate")
	}
	if cancel.Status != domain.MetricCritical {
		t.Errorf("hủy 10%% (ngưỡng 3%%) xếp %s, cần CRITICAL", cancel.Status)
	}

	onTime, ok := findMetric(ds, "on_time_shipping_rate")
	if !ok {
		t.Fatal("thiếu on_time_shipping_rate")
	}
	if onTime.Status != domain.MetricCritical {
		t.Errorf("giao đúng hạn 10%% (ngưỡng 95%%) xếp %s, cần CRITICAL",
			onTime.Status)
	}
}

// TestTheWarningBand: không có vùng đệm thì chỉ số nhảy giữa TỐT và NGHIÊM
// TRỌNG chỉ vì một đơn, và nhà bán không có tín hiệu sớm nào.
func TestTheWarningBand(t *testing.T) {
	// 92% đúng hạn: dưới ngưỡng 95% nhưng trên 90% × 95% = 85,5%.
	ds := domain.ComputeMetrics(domain.PerformanceStats{
		TotalOrders: 100, CancelledOrders: 0, DeliveredOrders: 100, OnTimeOrders: 92,
	}, domain.DefaultThresholds())
	onTime, _ := findMetric(ds, "on_time_shipping_rate")
	if onTime.Status != domain.MetricWarning {
		t.Errorf("đúng hạn 92%% xếp %s, cần WARNING — không có vùng đệm thì "+
			"nhà bán chỉ biết mình có vấn đề khi đã NGHIÊM TRỌNG",
			onTime.Status)
	}
}

// TestNoDivisionByZero: không có đơn nào đã giao thì không tính tỷ lệ
// đúng hạn — chia cho 0 cho ra NaN, và NaN đi thẳng vào JSON là một trường
// không parse được ở phía client.
func TestNoDivisionByZero(t *testing.T) {
	ds := domain.ComputeMetrics(domain.PerformanceStats{
		TotalOrders: 50, CancelledOrders: 50, DeliveredOrders: 0, OnTimeOrders: 0,
	}, domain.DefaultThresholds())
	if _, found := findMetric(ds, "on_time_shipping_rate"); found {
		t.Error("không có đơn nào đã giao mà vẫn tính tỷ lệ đúng hạn")
	}
	// Tỷ lệ hủy vẫn tính được và phải là 100%.
	cancel, ok := findMetric(ds, "cancellation_rate")
	if !ok || cancel.Value != 1 {
		t.Errorf("tỷ lệ hủy = %v (có=%v), cần 1", cancel.Value, ok)
	}
}

// TestUnmeasuredMetricsStateTheReason.
//
// Trả một phần chỉ số rồi im lặng về phần còn lại tạo ra đúng thứ hộp đen
// mà đặc tả sinh ra để tránh — chỉ khác là ở phía người viết API.
func TestUnmeasuredMetricsStateTheReason(t *testing.T) {
	// Mẫu ĐỦ cho cả hai chỉ số đo được, nên danh sách chỉ còn bốn chỉ số
	// thiếu nguồn dữ liệu.
	full := domain.PerformanceStats{TotalOrders: 100, DeliveredOrders: 100}
	ds := domain.UnmeasuredMetrics(full, domain.DefaultThresholds())
	if len(ds) == 0 {
		t.Fatal("không khai chỉ số nào là chưa đo")
	}
	for _, c := range ds {
		if c.Ten == "" {
			t.Error("có mục thiếu tên chỉ số")
		}
		if len(c.Reason) < 20 {
			t.Errorf("lý do của %q quá ngắn (%q) — một dòng không giải "+
				"thích được thì không khác gì im lặng", c.Ten, c.Reason)
		}
	}
}

// MỘT chỉ số vắng mặt vì MẪU cũng phải được nói ra.
//
// Trước 17/09/2026, `UnmeasuredMetrics` chỉ liệt kê những chỉ số thiếu NGUỒN DỮ
// LIỆU. Chỉ số đo được mà kỳ này ít đơn quá thì biến mất khỏi `metrics`
// không một lời — nhà bán không có cách nào biết vì sao, tức đúng thứ hộp
// đen endpoint này sinh ra để tránh.
func TestAnInsufficientSampleMustAlsoBeStated(t *testing.T) {
	ng := domain.DefaultThresholds()

	// Một gian hàng mới: ít đơn, chưa bàn giao cái nào.
	after := domain.PerformanceStats{TotalOrders: 3, DeliveredOrders: 0}

	if got := domain.ComputeMetrics(after, ng); len(got) != 0 {
		t.Fatalf("mẫu quá nhỏ mà vẫn chấm: %+v", got)
	}

	byName := map[string]string{}
	for _, c := range domain.UnmeasuredMetrics(after, ng) {
		byName[c.Ten] = c.Reason
	}

	for _, ten := range []string{"cancellation_rate", "on_time_shipping_rate"} {
		reason, found := byName[ten]
		if !found {
			t.Errorf("%q vắng khỏi metrics mà KHÔNG có trong not_measured "+
				"— nhà bán không có cách nào biết vì sao", ten)
			continue
		}
		// Lý do phải nêu CON SỐ: "chưa đủ mẫu" một mình không cho biết
		// còn thiếu bao nhiêu, nên không ai làm gì được với nó.
		if !strings.Contains(reason, "3") && !strings.Contains(reason, "0") {
			t.Errorf("lý do của %q không nêu số đơn hiện có: %q", ten, reason)
		}
		if !strings.Contains(reason, "10") {
			t.Errorf("lý do của %q không nêu ngưỡng mẫu tối thiểu: %q", ten, reason)
		}
	}
}

// MỘT chỉ số KHÔNG được vừa nằm trong `metrics` vừa nằm trong `not_measured`.
//
// Hai danh sách dùng hai điều kiện phải là phủ định của nhau. Lệch nhau thì
// hoặc mâu thuẫn, hoặc — tệ hơn — một chỉ số vắng ở CẢ HAI.
func TestNoMetricIsBothMeasuredAndUnmeasured(t *testing.T) {
	ng := domain.DefaultThresholds()

	for _, stats := range []domain.PerformanceStats{
		{TotalOrders: 0, DeliveredOrders: 0},
		{TotalOrders: 9, DeliveredOrders: 9},
		{TotalOrders: 10, DeliveredOrders: 9},
		{TotalOrders: 100, DeliveredOrders: 10},
		{TotalOrders: 100, DeliveredOrders: 100},
	} {
		measured := map[string]bool{}
		for _, c := range domain.ComputeMetrics(stats, ng) {
			measured[c.Ten] = true
		}
		unmeasured := map[string]bool{}
		for _, c := range domain.UnmeasuredMetrics(stats, ng) {
			unmeasured[c.Ten] = true
		}

		for _, ten := range []string{"cancellation_rate", "on_time_shipping_rate"} {
			switch {
			case measured[ten] && unmeasured[ten]:
				t.Errorf("%+v: %q vừa được chấm vừa báo chưa đo", stats, ten)
			case !measured[ten] && !unmeasured[ten]:
				t.Errorf("%+v: %q biến mất khỏi CẢ HAI danh sách", stats, ten)
			}
		}
	}
}
