package domain

import (
	"context"
	"time"
)

// EventRepository là PORT cho kho lưu trữ sự kiện.
type EventRepository interface {
	// Record ghi MỘT sự kiện.
	//
	// Trả ErrDuplicateEvent nếu sự kiện nghiệp vụ đã được ghi — chỉ mục
	// UNIQUE (event_id, event_name) là thứ chặn. Kiểm tra ở tầng ứng dụng
	// KHÔNG cứu được khi hai worker cùng xử lý một event, và khi đó GMV bị
	// cộng hai lần cho một đơn hàng.
	Record(ctx context.Context, e Event) error

	// RecordBatch ghi NHIỀU sự kiện trong một lượt.
	//
	// Tồn tại vì sự kiện hành vi có khối lượng RẤT LỚN: ghi từng cái là
	// một lượt đi-về database cho mỗi lần khách di chuột.
	//
	// Trả về số sự kiện thật sự được ghi. Sự kiện trùng bị BỎ QUA chứ
	// không làm hỏng cả lô — một event lặp không được phép chặn 999 event
	// còn lại.
	RecordBatch(ctx context.Context, events []Event) (int, error)

	// CountEvents đếm số sự kiện theo tên trong một khoảng.
	CountEvents(ctx context.Context, name string, r TimeRange, sellerID string) (int64, error)

	// CountDistinctSessions đếm số PHIÊN khác nhau có sự kiện này.
	//
	// Khác CountEvents: một người xem 20 sản phẩm là 20 sự kiện nhưng MỘT
	// phiên. Dùng số sự kiện làm mẫu số của tỷ lệ chuyển đổi sẽ ra con số
	// thấp hơn thực tế nhiều lần.
	CountDistinctSessions(ctx context.Context, name string, r TimeRange, sellerID string) (int64, error)

	// SumAmount cộng số tiền của các sự kiện trong một khoảng.
	//
	// Bỏ qua sự kiện có amount NULL: chúng không liên quan tới tiền, và
	// cộng chúng như 0 không sai nhưng đếm chúng vào sample_size thì sai.
	//
	// Trả về tổng và SỐ BẢN GHI đã cộng.
	SumAmount(ctx context.Context, name string, r TimeRange, sellerID string) (int64, int64, error)

	// AnonymizeCustomer gỡ định danh khỏi mọi sự kiện của một khách.
	//
	// Gọi khi khách yêu cầu xóa tài khoản. KHÔNG xóa hàng: số liệu tổng
	// hợp đã tính từ chúng, và xóa đi sẽ làm GMV của các tháng trước thay
	// đổi — một chuyện không giải thích được với ai.
	//
	// Trả về số bản ghi đã đổi.
	AnonymizeCustomer(ctx context.Context, customerID string, now time.Time) (int, error)
}

// MetricRepository là PORT cho chỉ số tính sẵn.
type MetricRepository interface {
	// Upsert ghi hoặc GHI ĐÈ một chỉ số.
	//
	// Ghi đè chứ không thêm hàng mới: hai giá trị GMV cho cùng một ngày là
	// hai câu trả lời cho cùng một câu hỏi, và không có cách nào biết cái
	// nào đúng.
	Upsert(ctx context.Context, m Metric) error

	// Get đọc một chỉ số đã tính.
	Get(ctx context.Context, name string, periodStart time.Time,
		g Granularity, dimension, dimensionValue string) (Metric, error)

	// List đọc chuỗi thời gian của một chỉ số.
	List(ctx context.Context, name string, g Granularity,
		r TimeRange, dimension, dimensionValue string) ([]Metric, error)
}
