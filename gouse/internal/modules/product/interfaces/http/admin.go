package http

import (
	"net/http"
	"strings"

	"log/slog"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/application"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// AdminHandler phục vụ luồng DUYỆT sản phẩm của nhân viên vận hành.
//
// Tách khỏi SellerHandler vì ranh giới bảo mật ngược nhau: nhà bán chỉ
// thấy sản phẩm CỦA MÌNH, người duyệt phải thấy của MỌI gian hàng. Dùng
// chung một handler nghĩa là một lần quên kẹp `seller_id` sẽ biến endpoint
// của nhà bán thành cửa xem toàn sàn.
type AdminHandler struct {
	svc *application.Service
	log *slog.Logger
}

func NewAdminHandler(svc *application.Service, log *slog.Logger) *AdminHandler {
	return &AdminHandler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth và RequireRole("ADMIN",
// "OPS_MERCHANDISING") — duyệt hàng hóa là việc của bộ phận hàng hóa.
func (h *AdminHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/admin/products/pending",
		http.HandlerFunc(h.choDuyet))
	mux.Handle("POST /api/v1/admin/products/{product_id}/approve",
		http.HandlerFunc(h.duyet))
	mux.Handle("POST /api/v1/admin/products/{product_id}/reject",
		http.HandlerFunc(h.tuChoi))
}

// choDuyet trả hàng đang chờ duyệt của MỌI gian hàng.
func (h *AdminHandler) choDuyet(w http.ResponseWriter, r *http.Request) {
	ds, err := h.svc.ListProducts(r.Context(), domain.Filter{
		Status: domain.StatusPendingReview,
		Limit:  gioiHanTu(r.URL.Query().Get("limit"), 50, maxLimit),
	})
	if err != nil {
		h.fail(w, r, dichLoiGhi(err))
		return
	}

	out := make([]sanPhamNhaBan, 0, len(ds))
	for _, p := range ds {
		out = append(out, toSanPhamNhaBan(p))
	}
	h.ok(w, r, http.StatusOK, map[string]any{"data": out})
}

func (h *AdminHandler) duyet(w http.ResponseWriter, r *http.Request) {
	pid, err := ids.Parse(r.PathValue("product_id"), ids.PrefixProduct)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Mã sản phẩm không hợp lệ"))
		return
	}

	p, err := h.svc.Approve(r.Context(), pid)
	if err != nil {
		h.fail(w, r, dichLoiGhi(err))
		return
	}
	h.ok(w, r, http.StatusOK, toSanPhamNhaBan(p))
}

type tuChoiBody struct {
	Reason string `json:"reason"`
}

// tuChoi từ chối sản phẩm kèm LÝ DO.
//
// Lý do BẮT BUỘC: nhà bán phải biết sửa gì. Một lần từ chối không nói lý
// do là một gian hàng đứng im không hiểu vì sao, và họ sẽ gửi lại đúng
// sản phẩm đó — tốn công cả hai bên.
func (h *AdminHandler) tuChoi(w http.ResponseWriter, r *http.Request) {
	var body tuChoiBody
	if err := decodeJSON(r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"reason là trường bắt buộc — nhà bán cần biết phải sửa gì"))
		return
	}

	pid, err := ids.Parse(r.PathValue("product_id"), ids.PrefixProduct)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Mã sản phẩm không hợp lệ"))
		return
	}

	p, err := h.svc.Reject(r.Context(), pid, strings.TrimSpace(body.Reason))
	if err != nil {
		h.fail(w, r, dichLoiGhi(err))
		return
	}
	h.ok(w, r, http.StatusOK, toSanPhamNhaBan(p))
}

func (h *AdminHandler) ok(w http.ResponseWriter, r *http.Request, status int, body any) {
	if err := apierror.WriteJSON(w, status, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *AdminHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}
