// Package http phơi bày đường THU DỮ LIỆU HÀNH VI cho storefront.
//
// # Vì sao module này cần một tầng HTTP, muộn hơn mọi module khác
//
// Trước đây analytics chỉ nghe domain event: thêm giỏ, đặt hàng, giao
// hàng. Đó là nửa CUỐI phễu — nửa đã quyết định mua.
//
// Nửa ĐẦU phễu không phát domain event nào, vì xem một sản phẩm không làm
// thay đổi trạng thái nghiệp vụ nào cả. Nó chỉ tồn tại ở trình duyệt. Nên
// nếu không có đường nhận từ client thì dữ liệu ấy KHÔNG BAO GIỜ tồn tại.
//
// Đo trước khi có tuyến này: 0 sự kiện xem sản phẩm / tìm kiếm / xem trang,
// và hệ quả là `conversion_rate` = 0 với cỡ mẫu 0 — nền tảng trả lời được
// "bán bao nhiêu" nhưng mù với "bao nhiêu người xem mà không mua".
//
// docs/00-overview/vision.md gọi đây là chỗ đa số nền tảng thương mại điện
// tử đứt, và nói rõ dữ liệu này KHÔNG tạo ngược được.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/analytics"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// tenChoPhep là danh sách ĐÓNG các sự kiện client được ghi.
//
// # Vì sao đóng
//
// Đây là endpoint CÔNG KHAI, không xác thực. Danh sách mở nghĩa là bất kỳ
// ai cũng ghi được sự kiện tên tùy ý vào bảng mà mọi chỉ số kinh doanh đọc
// từ đó — `purchase` giả sẽ thổi phồng tỷ lệ chuyển đổi, và không có cách
// nào phân biệt sau này.
//
// Sự kiện NGHIỆP VỤ (đặt hàng, giao hàng) KHÔNG nằm ở đây: chúng đến từ
// domain event, nơi server tự biết sự thật. Client chỉ được kể về những
// việc chỉ client mới thấy.
var tenChoPhep = map[string]bool{
	analytics.EventPageView:    true,
	analytics.EventProductView: true,
	analytics.EventSearch:      true,
}

// tranMotLo là số sự kiện tối đa trong MỘT lời gọi.
//
// Gom lô là cách giảm số lượt ghi: client giữ sự kiện trong bộ nhớ rồi gửi
// một lần. Trần này chặn lô khổng lồ làm nghẽn một giao dịch database.
const tranMotLo = 50

// GioiHanPort quyết định một phiên còn được ghi tiếp hay không.
//
// Cổng do tầng này khai, nên nó không biết tới `opsconfig`.
type GioiHanPort interface {
	// ChoPhep trả về false khi phiên đã vượt ngưỡng trong cửa sổ hiện tại.
	ChoPhep(sessionID string, soSuKien int) bool
}

type Handler struct {
	module  *analytics.Module
	gioiHan GioiHanPort
	log     *slog.Logger
}

func NewHandler(m *analytics.Module, gioiHan GioiHanPort, log *slog.Logger) *Handler {
	return &Handler{module: m, gioiHan: gioiHan, log: log}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/events", http.HandlerFunc(h.ghiLo))
}

type suKienJSON struct {
	Name        string         `json:"name"`
	SubjectType string         `json:"subject_type"`
	SubjectID   string         `json:"subject_id"`
	Properties  map[string]any `json:"properties"`
}

type loJSON struct {
	// SessionID do client sinh, KHÔNG phải định danh người.
	//
	// Nó nối các sự kiện của một lượt truy cập để đo được phễu. Server
	// không gắn nó với tài khoản nào ở đây — khách đăng nhập rồi thì sự
	// kiện nghiệp vụ phía sau đã mang `customer_id`.
	SessionID string       `json:"session_id"`
	Events    []suKienJSON `json:"events"`
}

// ghiLo phục vụ POST /api/v1/events (operationId: trackBehaviorEvents).
//
// KHÔNG thu địa chỉ IP và user-agent. Phễu nhu cầu chỉ cần biết MÓN NÀO
// được xem bao nhiêu lần trong bao nhiêu phiên — không cần biết AI. Thu
// thêm dữ liệu không dùng tới là tự tạo nghĩa vụ bảo vệ mà không đổi lại
// được gì.
func (h *Handler) ghiLo(w http.ResponseWriter, r *http.Request) {
	var req loJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).
		Decode(&req); err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"dữ liệu không đọc được"))
		return
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	if req.SessionID == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"session_id là bắt buộc"))
		return
	}
	if len(req.Events) == 0 {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"lô không có sự kiện nào"))
		return
	}
	if len(req.Events) > tranMotLo {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"một lô tối đa 50 sự kiện"))
		return
	}

	if h.gioiHan != nil && !h.gioiHan.ChoPhep(req.SessionID, len(req.Events)) {
		// 429 chứ không 400: client KHÔNG sai, nó chỉ gửi quá nhanh. Phân
		// biệt này quan trọng vì client nên thử lại sau, không nên bỏ lô.
		h.fail(w, r, apierror.New(apierror.CodeRateLimitExceeded,
			"phiên này đã ghi quá nhiều sự kiện, thử lại sau"))
		return
	}

	// Thời gian do SERVER đặt, không nhận từ client.
	//
	// Mốc của client sai lệch vì đồng hồ máy người dùng lệch, vì múi giờ,
	// và vì có người cố tình gửi mốc quá khứ để bẻ báo cáo. Gom lô làm mốc
	// trễ vài giây — sai số nhỏ hơn nhiều so với tin vào đồng hồ lạ.
	now := time.Now().UTC()

	vao := make([]analytics.EventInput, 0, len(req.Events))
	for _, e := range req.Events {
		ten := strings.TrimSpace(e.Name)
		if !tenChoPhep[ten] {
			// BỎ QUA sự kiện lạ thay vì từ chối cả lô: client cũ gửi một
			// tên đã gỡ bỏ không nên làm mất những sự kiện hợp lệ đi cùng.
			continue
		}
		vao = append(vao, analytics.EventInput{
			Name:        ten,
			Category:    analytics.CategoryBehavior,
			SessionID:   req.SessionID,
			SubjectType: strings.TrimSpace(e.SubjectType),
			SubjectID:   strings.TrimSpace(e.SubjectID),
			Properties:  e.Properties,
			OccurredAt:  now,
		})
	}

	if len(vao) == 0 {
		// Không sự kiện nào hợp lệ. Trả 200 với số 0 chứ không báo lỗi:
		// đây là đường ĐO, và làm client thấy lỗi vì một tên sai sẽ khiến
		// họ tắt hẳn việc gửi.
		h.ok(w, r, map[string]int{"accepted": 0})
		return
	}

	// TrackBatch trả SỐ ĐÃ GHI kèm lỗi: một sự kiện hỏng không làm mất
	// cả lô. Ghi được phần nào thì báo phần đó, và chỉ thất bại hẳn khi
	// không ghi được gì.
	n, err := h.module.TrackBatch(r.Context(), vao)
	if err != nil && n == 0 {
		h.fail(w, r, apierror.From(err))
		return
	}
	if err != nil {
		h.log.WarnContext(r.Context(), "một phần lô sự kiện không ghi được",
			"error", err, "đã_ghi", n, "gửi", len(vao))
	}

	h.ok(w, r, map[string]int{"accepted": n})
}

func (h *Handler) ok(w http.ResponseWriter, r *http.Request, body any) {
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}
