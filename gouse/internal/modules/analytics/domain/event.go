// Package domain chứa mô hình nghiệp vụ của module analytics.
//
// # Nguyên tắc quan trọng nhất: analytics KHÔNG PHẢI NGUỒN SỰ THẬT
//
//	"GMV tháng này bao nhiêu?"      → analytics (có thể trễ vài phút)
//	"Seller A được trả bao nhiêu?"  → payment  (nguồn sự thật)
//
// Không bao giờ dùng số liệu ở đây để ra quyết định tài chính. Đây là bản
// sao đọc, chấp nhận trễ và chấp nhận mất mát ở mức nhỏ.
//
// # Hệ quả cho thiết kế
//
// Package này KHÔNG import module nghiệp vụ nào. Nếu analytics gọi mọi
// module để làm giàu dữ liệu, nó phụ thuộc toàn hệ thống — và một module
// lỗi sẽ làm hỏng việc ghi nhận mọi loại sự kiện.
package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidEvent = errors.New("analytics: sự kiện không hợp lệ")

	ErrInvalidRange = errors.New("analytics: khoảng thời gian không hợp lệ")

	// ErrDuplicateEvent là sự kiện nghiệp vụ đã được ghi.
	ErrDuplicateEvent = errors.New("analytics: sự kiện đã được ghi nhận")
)

// Category phân nhóm sự kiện.
type Category string

const (
	// CategoryBehavior là hành vi người dùng: xem trang, tìm kiếm, thêm giỏ.
	//
	// Khối lượng RẤT LỚN. Không chống trùng — hai lần xem sản phẩm thật sự
	// là hai lần xem.
	CategoryBehavior Category = "BEHAVIOR"

	// CategoryBusiness là sự kiện từ domain event: đặt hàng, giao hàng.
	//
	// Khối lượng nhỏ hơn nhiều, nhưng PHẢI chống trùng: handler xử lý lại
	// cùng một event là chuyện bình thường, và mỗi lần xử lý lại là một
	// đơn hàng nữa cộng vào GMV.
	CategoryBusiness Category = "BUSINESS"
)

// Tên các sự kiện hành vi của MVP.
//
// Đây là PHỄU CHUYỂN ĐỔI: đo tổng thể chỉ cho biết CÓ vấn đề, đo từng
// bước cho biết vấn đề Ở ĐÂU.
const (
	EventPageView      = "page_view"
	EventProductView   = "product_view"
	EventSearch        = "search"
	EventAddToCart     = "add_to_cart"
	EventCheckoutStart = "checkout_start"
	EventPurchase      = "purchase"
)

// Tên các sự kiện nghiệp vụ của MVP.
const (
	EventOrderPlaced    = "order.placed"
	EventOrderCancelled = "order.cancelled"
	EventDelivered      = "fulfillment.delivered"
)

// Event là một sự kiện đã ghi nhận.
//
// Là STRUCT PHẲNG chứ không phải entity có hành vi: sự kiện là dữ liệu
// bất biến đã xảy ra, không có thao tác nào làm nó đổi.
type Event struct {
	Name     string
	Category Category

	// CustomerID rỗng với khách chưa đăng nhập.
	CustomerID string

	// SessionID nối các sự kiện của MỘT lượt truy cập.
	//
	// Đây là thứ cho phép đo phễu chuyển đổi: không có nó thì biết có
	// 1000 lượt xem và 50 đơn hàng, nhưng KHÔNG biết 50 đơn đó đến từ
	// những lượt xem nào.
	SessionID string

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
	// KHÔNG ĐƯỢC chứa dữ liệu cá nhân nhạy cảm: số đo cơ thể, thông tin
	// thanh toán, mật khẩu. Việc lọc nằm ở SanitizeProperties.
	Properties map[string]any

	IPHash    string
	UserAgent string

	// EventID là id của domain event sinh ra bản ghi này.
	//
	// Rỗng với sự kiện hành vi. Với sự kiện nghiệp vụ, đây là khóa CHỐNG
	// GHI TRÙNG.
	EventID string

	OccurredAt time.Time
	RecordedAt time.Time
}

// Validate kiểm tra sự kiện có ghi được không.
//
// # Cố ý DỄ TÍNH
//
// Chỉ từ chối thứ thật sự không dùng được: tên rỗng, thời điểm rỗng. Sự
// kiện thiếu customer_id hay seller_id vẫn ghi được — một sự kiện thiếu
// ngữ cảnh vẫn hơn không có sự kiện nào, và quy tắc 3 nói việc ghi nhận
// KHÔNG được chặn luồng chính.
func (e Event) Validate() error {
	if strings.TrimSpace(e.Name) == "" {
		return ErrInvalidEvent
	}
	if e.OccurredAt.IsZero() {
		return ErrInvalidEvent
	}
	if e.Category != CategoryBehavior && e.Category != CategoryBusiness {
		return ErrInvalidEvent
	}
	return nil
}

// sensitiveKeys là các khóa KHÔNG BAO GIỜ được lưu vào analytics.
//
// # Vì sao lọc ở đây chứ không tin bên gọi
//
// Bên gọi là tầng HTTP hoặc handler event, và họ chuyển tiếp dữ liệu do
// người dùng gửi lên. Một trường tên "password" lọt vào properties sẽ nằm
// trong database phân tích — nơi nhiều người đọc được và giữ rất lâu.
//
// Danh sách này chặn theo TIỀN TỐ nên "password", "password_hash",
// "passwordConfirm" đều bị chặn.
var sensitiveKeys = []string{
	"password",
	"token",
	"secret",
	"card",
	"cvv",
	"pin",

	// Số đo cơ thể — dữ liệu cá nhân nhạy cảm đặc thù thời trang.
	// Xem docs/04-modules/customer.md mục 3.
	"measurement",
	"body_",
	"bust",
	"waist",
	"hip",

	"ssn",
	"national_id",
}

// SanitizeProperties loại các trường nhạy cảm khỏi dữ liệu bổ sung.
//
// KHÔNG báo lỗi khi gặp trường cấm, chỉ BỎ nó đi: báo lỗi sẽ làm hỏng
// luồng chính vì một trường thừa — đúng thứ quy tắc 3 cấm.
//
// Trả về map MỚI: sửa map của bên gọi là tác dụng phụ họ không mong đợi.
func SanitizeProperties(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(in))
	for k, v := range in {
		if IsSensitiveKey(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// IsSensitiveKey cho biết một khóa có bị cấm lưu không.
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	for _, bad := range sensitiveKeys {
		if strings.Contains(lower, bad) {
			return true
		}
	}
	return false
}
