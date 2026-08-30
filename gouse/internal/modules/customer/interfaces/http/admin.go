package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/customer/application"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// AdminHandler phục vụ việc nhân viên tra cứu hồ sơ khách hàng.
type AdminHandler struct {
	svc *application.Service
	log *slog.Logger
}

func NewAdminHandler(svc *application.Service, log *slog.Logger) *AdminHandler {
	return &AdminHandler{svc: svc, log: log}
}

func (h *AdminHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/admin/customers/{customer_id}",
		http.HandlerFunc(h.getCustomer))
}

type adminCustomerJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`

	// Tier là hạng khách. Đặc tả gọi là `tier`, module gọi là `Status`.
	Tier      string `json:"tier"`
	CreatedAt string `json:"created_at"`
}

type adminCustomerResponse struct {
	Customer adminCustomerJSON `json:"customer"`

	// AuditLogged LUÔN true, và đó là điều đáng nói.
	//
	// Nó không phải cờ điều kiện: nếu ghi vết hỏng thì response này không
	// tồn tại, vì handler đã trả lỗi. Trường này tồn tại để giao diện nói
	// thẳng với nhân viên rằng lần xem vừa rồi đã có dấu — biết mình bị
	// ghi lại là thứ ngăn việc tra cứu tò mò hiệu quả hơn mọi rào kỹ thuật.
	AuditLogged bool `json:"audit_logged"`
}

// getCustomer phục vụ GET /api/v1/admin/customers/{customer_id}
// (operationId: getCustomerAsAdmin).
//
// # Lý do truy cập nằm ở HEADER, không phải query
//
// `X-Access-Reason` theo đúng đặc tả. Đặt ở query string thì lý do — thường
// có tên khách hoặc mã khiếu nại — đi vào access log của mọi proxy trên
// đường, và nhật ký truy cập dữ liệu cá nhân lại tự rải dữ liệu cá nhân ra
// chỗ khác.
//
// # Số đo cơ thể
//
// Đặc tả yêu cầu không trả số đo nếu không có quyền đặc biệt. Module
// customer CHƯA lưu số đo (xem ghi chú ở `getProfile`), nên yêu cầu này
// thoả mãn ngay từ cấu trúc chứ không nhờ một câu lệnh lọc — không có gì
// để rò. Khi nào thêm số đo, đây là chỗ phải xem lại.
func (h *AdminHandler) getCustomer(w http.ResponseWriter, r *http.Request) {
	lyDo := strings.TrimSpace(r.Header.Get("X-Access-Reason"))
	if lyDo == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Thiếu header X-Access-Reason — mọi lần xem hồ sơ khách đều "+
				"được ghi nhật ký, và một lần ghi không có lý do thì không "+
				"trả lời được câu hỏi duy nhất đáng hỏi khi điều tra"))
		return
	}

	customerID, err := ids.Parse(r.PathValue("customer_id"), ids.PrefixCustomer)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"customer_id không hợp lệ"))
		return
	}

	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"tra cứu hồ sơ khách chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	c, err := h.svc.GetAsAdmin(r.Context(), application.ViewCustomerInput{
		CustomerID: customerID,
		ActorID:    ac.UserID,
		Reason:     lyDo,
		RequestID:  logger.RequestIDFromContext(r.Context()),
	})
	if err != nil {
		h.fail(w, r, dichLoiXemKhach(err))
		return
	}

	h.ok(w, r, adminCustomerResponse{
		Customer: adminCustomerJSON{
			ID:        c.ID().String(),
			Name:      c.DisplayName(),
			Email:     c.Email(),
			Phone:     c.Phone(),
			Tier:      string(c.Status()),
			CreatedAt: c.CreatedAt().UTC().Format(time.RFC3339),
		},
		AuditLogged: true,
	})
}

func dichLoiXemKhach(err error) error {
	switch {
	case errors.Is(err, audit.ErrReasonRequired):
		// 400 chứ không phải 500: lý do quá ngắn hoặc là chuỗi rác là lỗi
		// của người gọi, và họ sửa được.
		return apierror.New(apierror.CodeValidationFailed,
			"X-Access-Reason phải là lý do có ý nghĩa, tối thiểu 20 ký tự — "+
				"ví dụ \"xử lý khiếu nại đơn ORD-12345\"")
	default:
		return translate(err)
	}
}

func (h *AdminHandler) ok(w http.ResponseWriter, r *http.Request, body any) {
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *AdminHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}
