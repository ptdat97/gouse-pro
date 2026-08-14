// Package analytics ghi sự kiện hành vi và tính chỉ số nghiệp vụ.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// # Nguyên tắc quan trọng nhất: KHÔNG PHẢI NGUỒN SỰ THẬT
//
//	"GMV tháng này bao nhiêu?"      → analytics (có thể trễ vài phút)
//	"Seller A được trả bao nhiêu?"  → payment  (nguồn sự thật)
//
// KHÔNG BAO GIỜ dùng số liệu ở đây để ra quyết định tài chính. Đây là bản
// sao đọc, chấp nhận trễ và chấp nhận mất mát ở mức nhỏ. Trả tiền cho
// seller dựa trên GMV của analytics là cách chắc chắn nhất để trả sai.
//
// # Ràng buộc kiến trúc
//
//	Module này KHÔNG GỌI bất kỳ module nghiệp vụ nào.
//
// Lý do giống `notification`: nếu analytics gọi mọi module để làm giàu dữ
// liệu, nó phụ thuộc toàn hệ thống — và một module lỗi sẽ làm hỏng việc
// ghi nhận mọi loại sự kiện.
//
// Hệ quả: event payload phải chứa ĐỦ thông tin. Nếu thiếu, bổ sung vào
// event chứ không gọi ngược.
//
// # TrackEvent KHÔNG BAO GIỜ chặn luồng chính
//
// Nếu analytics lỗi, việc bán hàng vẫn phải chạy bình thường. Mất một bản
// ghi phân tích là chuyện nhỏ; chặn một đơn hàng thì không.
//
// # Phạm vi MVP
//
// Ghi sự kiện cơ bản và chỉ số cốt lõi (GMV, AOV, chuyển đổi). Phễu chi
// tiết, cohort, kho dữ liệu thuộc Phase 2+ — xem
// docs/04-modules/analytics.md mục 13.
package analytics

import (
	"context"
	"errors"
	"time"
)

// API là hợp đồng công khai của module analytics.
type API interface {
	// TrackEvent ghi nhận một sự kiện.
	//
	// KHÔNG BAO GIỜ CHẶN LUỒNG CHÍNH: hàm này trả nil khi ghi thất bại vì
	// lý do hạ tầng, chỉ ghi cảnh báo ra nhật ký.
	//
	// Lỗi DUY NHẤT trả ra là ErrInvalidInput — lỗi của bên gọi, sửa được.
	//
	// IDEMPOTENT theo EventID với sự kiện nghiệp vụ: gọi lại KHÔNG cộng
	// thêm vào GMV. Sự kiện hành vi (EventID rỗng) KHÔNG chống trùng —
	// hai lần xem sản phẩm thật sự là hai lần xem.
	TrackEvent(ctx context.Context, e EventInput) error

	// TrackBatch ghi nhiều sự kiện trong MỘT lượt.
	//
	// Sự kiện hành vi có khối lượng RẤT LỚN: ghi từng cái là một lượt
	// đi-về database cho mỗi lần khách di chuột.
	//
	// Sự kiện không hợp lệ bị BỎ QUA chứ không làm hỏng cả lô. Trả về số
	// sự kiện thật sự được ghi.
	TrackBatch(ctx context.Context, events []EventInput) (int, error)

	// GetMetric đọc một chỉ số đã tính.
	//
	// Trả về giá trị 0 nếu chưa tính — dashboard mở vào một ngày chưa có
	// worker chạy là chuyện bình thường, không phải lỗi.
	GetMetric(ctx context.Context, req MetricRequest) (MetricView, error)

	// GetTimeSeries đọc chuỗi thời gian của một chỉ số.
	GetTimeSeries(ctx context.Context, req TimeSeriesRequest) ([]MetricView, error)

	// ComputeMetrics tính và lưu chỉ số cốt lõi cho một khoảng.
	//
	// Gọi từ worker chạy định kỳ. Tính LẠI cùng một khoảng sẽ GHI ĐÈ —
	// hai giá trị GMV cho cùng một ngày là hai câu trả lời cho cùng một
	// câu hỏi.
	ComputeMetrics(ctx context.Context, req ComputeRequest) error

	// CountEvents đếm sự kiện thô trong một khoảng.
	//
	// Đọc TRỰC TIẾP từ nhật ký sự kiện, không qua chỉ số tính sẵn. Dùng
	// khi cần con số mới nhất và chấp nhận truy vấn nặng hơn.
	CountEvents(ctx context.Context, req CountRequest) (int64, error)

	// AnonymizeCustomer gỡ định danh khỏi mọi sự kiện của một khách.
	//
	// Gọi khi khách yêu cầu xóa tài khoản. KHÔNG xóa hàng: số liệu tổng
	// hợp đã tính từ chúng, và xóa đi sẽ làm GMV của các tháng trước thay
	// đổi — một chuyện không giải thích được với ai.
	//
	// KHÁC TrackEvent, hàm này TRẢ LỖI: đây là nghĩa vụ pháp lý, và im
	// lặng nuốt lỗi nghĩa là báo cáo "đã xóa" cho một việc chưa làm.
	AnonymizeCustomer(ctx context.Context, customerID string) (int, error)
}

// ---------------------------------------------------------------- DTO

// EventInput là một sự kiện cần ghi nhận.
type EventInput struct {
	// Name là tên sự kiện: "product_view", "order.placed".
	Name string

	// Category: BEHAVIOR hoặc BUSINESS. Bỏ trống = BEHAVIOR.
	//
	//	BEHAVIOR  hành vi người dùng, khối lượng RẤT LỚN, không chống trùng
	//	BUSINESS  từ domain event, PHẢI có EventID để chống trùng
	Category string

	// CustomerID rỗng với khách chưa đăng nhập.
	CustomerID string

	// SessionID nối các sự kiện của MỘT lượt truy cập.
	//
	// Đây là thứ cho phép đo tỷ lệ chuyển đổi: không có nó thì biết có
	// 1000 lượt xem và 50 đơn hàng, nhưng KHÔNG biết 50 đơn đó đến từ
	// những lượt xem nào.
	SessionID string

	// SubjectType: "product", "order", "variant".
	SubjectType string
	SubjectID   string

	SellerID string

	// Amount là số tiền, nil nếu sự kiện không liên quan tới tiền.
	//
	// CON TRỎ chứ không phải 0: "đơn hàng 0đ" và "sự kiện xem sản phẩm"
	// là hai chuyện khác nhau, và cộng nhầm loại thứ hai vào GMV làm sai
	// mọi con số.
	Amount   *int64
	Currency string

	// Properties là dữ liệu tự do.
	//
	// Module TỰ LỌC các trường nhạy cảm (mật khẩu, token, số thẻ, số đo cơ
	// thể) trước khi lưu — không tin bên gọi, vì bên gọi chuyển tiếp dữ
	// liệu do người dùng gửi lên.
	Properties map[string]any

	// IP là địa chỉ nguyên văn; module BĂM trước khi lưu.
	IP        string
	UserAgent string

	// EventID là id của domain event sinh ra bản ghi này.
	//
	// BẮT BUỘC với sự kiện BUSINESS: đây là khóa chống ghi trùng. Thiếu
	// nó, mỗi lần handler xử lý lại là một đơn hàng nữa cộng vào GMV.
	EventID string

	// OccurredAt là thời điểm sự kiện XẢY RA. Bỏ trống = bây giờ.
	//
	// KHÁC thời điểm ghi nhận: một sự kiện từ hàng đợi có thể được ghi
	// nhận vài phút sau khi xảy ra, và chỉ số phải tính theo lúc xảy ra.
	OccurredAt time.Time
}

// MetricRequest là dữ liệu đọc một chỉ số.
type MetricRequest struct {
	// Name: gmv, order_count, aov, conversion_rate, session_count.
	Name string

	// PeriodStart sẽ được CẮT TRÒN theo Granularity.
	PeriodStart time.Time

	// Granularity: HOUR, DAY, MONTH.
	Granularity string

	// SellerID rỗng nghĩa là TOÀN SÀN.
	SellerID string
}

// TimeSeriesRequest là dữ liệu đọc chuỗi thời gian.
type TimeSeriesRequest struct {
	Name        string
	Granularity string

	// From là mốc BAO GỒM, To là mốc KHÔNG BAO GỒM.
	//
	// Nửa mở có chủ ý: hai khoảng liền nhau không đếm trùng mốc chung.
	From time.Time
	To   time.Time

	SellerID string
}

// ComputeRequest là dữ liệu tính chỉ số.
type ComputeRequest struct {
	PeriodStart time.Time
	Granularity string

	// SellerID rỗng nghĩa là tính cho TOÀN SÀN.
	SellerID string

	Currency string
}

// CountRequest là dữ liệu đếm sự kiện thô.
type CountRequest struct {
	Name string

	From time.Time
	To   time.Time

	SellerID string
}

// MetricView là một chỉ số đã tính.
type MetricView struct {
	Name        string
	PeriodStart time.Time
	Granularity string

	// Value là SỐ NGUYÊN.
	//
	//	Chỉ số tiền tệ  → đơn vị nhỏ nhất (đồng, cent)
	//	Tỷ lệ           → ĐIỂM CƠ BẢN (1000 = 10%)
	//
	// KHÔNG dùng số thực: tỷ lệ hiển thị sai ở chữ số thứ mười lăm vẫn là
	// hiển thị sai.
	Value int64

	// SampleSize là số bản ghi đã dùng để tính.
	//
	// PHẢI hiển thị cùng chỉ số: tỷ lệ chuyển đổi 50% từ 2 lượt truy cập
	// không nói lên điều gì, còn từ 20.000 lượt thì có.
	SampleSize int64

	Currency   string
	ComputedAt time.Time

	SellerID string
}

// ---------------------------------------------------------------- Lỗi

var (
	// ErrInvalidInput là dữ liệu không hợp lệ: tên sự kiện rỗng, độ mịn
	// không tồn tại, khoảng thời gian ngược.
	ErrInvalidInput = errors.New("analytics: dữ liệu không hợp lệ")
)

// ---------------------------------------------------------------- Hằng

// Nhóm sự kiện.
const (
	CategoryBehavior = "BEHAVIOR"
	CategoryBusiness = "BUSINESS"
)

// Sự kiện hành vi — PHỄU CHUYỂN ĐỔI.
//
// Đo tổng thể chỉ cho biết CÓ vấn đề; đo từng bước cho biết vấn đề Ở ĐÂU.
const (
	EventPageView      = "page_view"
	EventProductView   = "product_view"
	EventSearch        = "search"
	EventAddToCart     = "add_to_cart"
	EventCheckoutStart = "checkout_start"
	EventPurchase      = "purchase"
)

// Sự kiện nghiệp vụ, đến từ domain event.
const (
	EventOrderPlaced    = "order.placed"
	EventOrderCancelled = "order.cancelled"
	EventDelivered      = "fulfillment.delivered"
)

// Chỉ số cốt lõi của MVP.
const (
	// MetricGMV là tổng giá trị hàng hóa đã bán.
	//
	// KHÔNG PHẢI DOANH THU của nền tảng — nền tảng chỉ nhận hoa hồng.
	// Nhầm hai khái niệm này là cách nhanh nhất để báo cáo sai.
	MetricGMV = "gmv"

	MetricOrderCount = "order_count"

	// MetricAOV là giá trị đơn hàng trung bình.
	MetricAOV = "aov"

	// MetricConversionRate tính bằng ĐIỂM CƠ BẢN: 1000 = 10%.
	MetricConversionRate = "conversion_rate"

	MetricSessionCount = "session_count"
)

// Độ mịn khoảng thời gian.
const (
	GranularityHour  = "HOUR"
	GranularityDay   = "DAY"
	GranularityMonth = "MONTH"
)
