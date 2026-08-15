// Package http là tầng interfaces của module seller: chuyển HTTP request
// thành lời gọi use case và chuyển kết quả thành JSON đúng đặc tả OpenAPI.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/admin.yaml — đặc tả là nguồn sự thật, không phải struct Go.
//
// # Phân quyền không nằm ở đây
//
// Handler KHÔNG tự kiểm tra vai trò. Việc đó do middleware RequireRole làm
// khi nối route, vì tầng này không nên biết mô hình phân quyền. Điều handler
// PHẢI làm là lấy danh tính người gọi từ AuthContext để ghi vào vết kiểm
// toán — không có danh tính thì không có vết dùng được.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/seller/application"
	"github.com/fashion-commerce/platform/internal/modules/seller/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ các endpoint quản trị nhà bán.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

// NewHandler tạo handler.
func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth và RequireRole — xem ghi chú đầu package.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/admin/sellers", http.HandlerFunc(h.listSellers))
	mux.Handle("GET /api/v1/admin/sellers/{seller_id}",
		http.HandlerFunc(h.getSellerDetail))
	mux.Handle("POST /api/v1/admin/sellers/{seller_id}/approve",
		http.HandlerFunc(h.approveSeller))
	mux.Handle("POST /api/v1/admin/sellers/{seller_id}/suspend",
		http.HandlerFunc(h.suspendSeller))
}

// ---------------------------------------------------------------- Chi tiết

type sellerDetail struct {
	sellerSummary

	LegalName           string `json:"legal_name,omitempty"`
	TaxCode             string `json:"tax_code,omitempty"`
	Email               string `json:"email,omitempty"`
	Phone               string `json:"phone,omitempty"`
	BankAccountVerified bool   `json:"bank_account_verified"`
	SuspensionReason    string `json:"suspension_reason,omitempty"`
	ApprovedBy          string `json:"approved_by,omitempty"`
	ApprovedAt          string `json:"approved_at,omitempty"`
}

// getSellerDetail phục vụ GET /api/v1/admin/sellers/{id}
// (operationId: getSellerDetail).
//
// Phạm vi MVP là hồ sơ cơ bản: chi tiết tài khoản ngân hàng và giấy tờ pháp
// lý thuộc `seller_bank_account` / `seller_document` — Phase 2, chưa có dữ
// liệu nên không trả về trường rỗng giả vờ là có.
func (h *Handler) getSellerDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseSellerID(r.PathValue("seller_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}

	sel, err := h.svc.GetSeller(r.Context(), id)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	out := sellerDetail{
		sellerSummary:       toSummary(sel),
		LegalName:           sel.LegalName(),
		TaxCode:             sel.TaxCode(),
		Email:               sel.Email(),
		Phone:               sel.Phone(),
		BankAccountVerified: sel.BankAccountVerified(),
		SuspensionReason:    sel.SuspensionReason(),
		ApprovedBy:          sel.ApprovedBy(),
	}
	if t := sel.ApprovedAt(); !t.IsZero() {
		out.ApprovedAt = t.UTC().Format(time.RFC3339)
	}

	h.ok(w, r, out)
}

// ---------------------------------------------------------------- Duyệt

type approveRequest struct {
	CommissionRateBP int32  `json:"commission_rate_bp"`
	Notes            string `json:"notes"`
}

type approveResponse struct {
	Seller      approvedSeller `json:"seller"`
	SideEffects []string       `json:"side_effects"`
}

type approvedSeller struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	ApprovedAt string `json:"approved_at,omitempty"`
}

// approveSeller phục vụ POST /api/v1/admin/sellers/{id}/approve
// (operationId: approveSeller).
//
// Duyệt chuyển seller sang APPROVED, CHƯA phải ACTIVE — `side_effects` nêu
// rõ bước còn thiếu để người duyệt không tưởng là đã xong.
func (h *Handler) approveSeller(w http.ResponseWriter, r *http.Request) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"approveSeller chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	id, err := parseSellerID(r.PathValue("seller_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}

	var req approveRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	rate, err := types.NewBasisPoints(req.CommissionRateBP)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"commission_rate_bp phải trong khoảng 0–10000 phần vạn"))
		return
	}

	sel, err := h.svc.ApproveWithAudit(r.Context(), application.ApproveInput{
		SellerID:         id,
		ActorID:          ac.UserID,
		CommissionRateBP: rate,
		Notes:            req.Notes,
		RequestID:        logger.RequestIDFromContext(r.Context()),
	})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	out := approveResponse{
		Seller: approvedSeller{
			ID:     sel.ID().String(),
			Status: string(sel.Status()),
		},
		SideEffects: application.ApprovalSideEffects(sel),
	}
	if t := sel.ApprovedAt(); !t.IsZero() {
		out.Seller.ApprovedAt = t.UTC().Format(time.RFC3339)
	}

	h.ok(w, r, out)
}

// ---------------------------------------------------------------- Danh sách

type sellerSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	SellerType       string `json:"seller_type"`
	Status           string `json:"status"`
	CommissionRateBP int32  `json:"commission_rate_bp"`
	IsActive         bool   `json:"is_active"`
	IsInternal       bool   `json:"is_internal"`
	CreatedAt        string `json:"created_at"`
}

type listResponse struct {
	Data []sellerSummary `json:"data"`
}

// listSellers phục vụ GET /api/v1/admin/sellers (operationId: listSellers).
func (h *Handler) listSellers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	f := domain.Filter{
		Status: domain.Status(q.Get("status")),
		Type:   domain.SellerType(q.Get("seller_type")),
	}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"limit phải là số nguyên dương"))
			return
		}
		f.Limit = n
	}

	list, err := h.svc.ListSellers(r.Context(), f)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	out := make([]sellerSummary, 0, len(list))
	for _, sel := range list {
		out = append(out, toSummary(sel))
	}

	h.ok(w, r, listResponse{Data: out})
}

// toSummary dựng phần tóm tắt dùng chung cho cả danh sách và chi tiết.
func toSummary(sel *domain.Seller) sellerSummary {
	return sellerSummary{
		ID:               sel.ID().String(),
		Name:             sel.Name(),
		Slug:             sel.Slug(),
		SellerType:       string(sel.Type()),
		Status:           string(sel.Status()),
		CommissionRateBP: sel.CommissionRate().Value(),
		IsActive:         sel.IsActive(),
		IsInternal:       sel.IsInternal(),
		CreatedAt:        sel.CreatedAt().UTC().Format(time.RFC3339),
	}
}

// ---------------------------------------------------------------- Đình chỉ

type suspendRequest struct {
	Reason      string `json:"reason"`
	ReasonCode  string `json:"reason_code"`
	HoldPayouts *bool  `json:"hold_payouts"`
}

type suspendResponse struct {
	Seller  sellerRef `json:"seller"`
	Effects effects   `json:"effects"`
}

type sellerRef struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type effects struct {
	Note string `json:"note"`
}

// suspendSeller phục vụ POST /api/v1/admin/sellers/{id}/suspend
// (operationId: suspendSeller).
//
// Đây là thao tác nhạy cảm: bắt buộc `reason`, ghi vết kiểm toán trong CÙNG
// giao dịch với việc đổi trạng thái.
func (h *Handler) suspendSeller(w http.ResponseWriter, r *http.Request) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		// Chỉ xảy ra khi route bị nối thiếu middleware Auth. Thất bại theo
		// hướng an toàn: không có danh tính thì không ghi vết được, và
		// thao tác nhạy cảm không có vết là thứ không được phép xảy ra.
		h.log.ErrorContext(r.Context(),
			"suspendSeller chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	id, err := parseSellerID(r.PathValue("seller_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}

	var req suspendRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	sel, err := h.svc.SuspendWithAudit(r.Context(), application.SuspendInput{
		SellerID:   id,
		ActorID:    ac.UserID,
		Reason:     req.Reason,
		ReasonCode: req.ReasonCode,
		RequestID:  logger.RequestIDFromContext(r.Context()),
	})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, suspendResponse{
		Seller: sellerRef{ID: sel.ID().String(), Status: string(sel.Status())},
		Effects: effects{
			Note: "Đơn đang xử lý KHÔNG bị hủy — seller phải hoàn tất hoặc " +
				"chuyển admin xử lý",
		},
	})
}

// ---------------------------------------------------------------- Hỗ trợ

func parseSellerID(raw string) (ids.ID, error) {
	id, err := ids.Parse(raw, ids.PrefixSeller)
	if err != nil {
		return "", apierror.New(apierror.CodeValidationFailed,
			"seller_id không đúng định dạng")
	}
	return id, nil
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apierror.New(apierror.CodeValidationFailed,
			"Dữ liệu gửi lên không hợp lệ")
	}
	return nil
}

// translate chuyển lỗi domain thành lỗi API.
//
// Lý do không đạt yêu cầu trả 400 kèm thông báo hướng dẫn được — nhân viên
// cần biết phải sửa gì, không phải đoán.
func translate(err error) error {
	switch {
	case errors.Is(err, audit.ErrReasonRequired):
		return apierror.New(apierror.CodeValidationFailed,
			"Lý do bắt buộc, tối thiểu 20 ký tự và phải có nội dung thật")

	case errors.Is(err, domain.ErrMissingReason):
		return apierror.New(apierror.CodeValidationFailed, "Phải nêu lý do")

	case errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy nhà bán")

	// Chuyển trạng thái không hợp lệ là XUNG ĐỘT TRẠNG THÁI (409), không
	// phải lỗi hệ thống. Duyệt lại hồ sơ đã duyệt là thao tác sai của
	// người dùng — trả 500 khiến họ tưởng hệ thống hỏng và thử lại mãi.
	case errors.Is(err, domain.ErrInvalidStatus):
		return apierror.New(apierror.CodeConflict,
			"Không thực hiện được với trạng thái hiện tại của hồ sơ")

	case errors.Is(err, domain.ErrNoBankAccount):
		return apierror.New(apierror.CodeConflict,
			"Nhà bán phải có tài khoản ngân hàng đã xác minh")

	default:
		return apierror.From(err)
	}
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
