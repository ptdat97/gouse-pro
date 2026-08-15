// Package http là tầng interfaces của module payment.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/admin.yaml — đặc tả là nguồn sự thật, không phải struct Go.
//
// # Endpoint nhạy cảm nhất hệ thống
//
// Điều chỉnh sổ cái là đường DUY NHẤT ghi vào sổ mà không có giao dịch thật
// phía sau. Handler này không nới lỏng bất kỳ lớp bảo vệ nào:
//
//	Σ DEBIT = Σ CREDIT   — domain từ chối bút toán không cân
//	Lý do ≥ 20 ký tự     — bộ ghi vết từ chối lý do rác
//	Idempotency-Key      — middleware chặn, và sổ cái có UNIQUE
//	Vết kiểm toán        — cùng giao dịch với bút toán
//	Vai trò ADMIN/OPS_FINANCE + 2FA — nối ở tầng route
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ các endpoint tài chính của quản trị.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth, RequireRole("ADMIN", "OPS_FINANCE") và
// RequireIdempotencyKey.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/admin/ledger/adjustments",
		http.HandlerFunc(h.createAdjustment))
}

type amountJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// lineJSON khớp components/schemas.yaml#/LedgerLine.
//
// `amount` là OBJECT {amount, currency}, không phải số trần — quy ước bắt
// buộc của dự án. Số tiền không kèm đơn vị là số vô nghĩa ở một nền tảng
// sẽ có nhiều thị trường.
type lineJSON struct {
	AccountType string     `json:"account_type"`
	OwnerID     string     `json:"owner_id,omitempty"`
	Direction   string     `json:"direction"`
	Amount      amountJSON `json:"amount"`
	Description string     `json:"description,omitempty"`
}

type adjustmentRequest struct {
	Reason        string     `json:"reason"`
	ReferenceType string     `json:"reference_type"`
	ReferenceID   string     `json:"reference_id"`
	Lines         []lineJSON `json:"lines"`
}

type adjustmentResponse struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	ReferenceType string     `json:"reference_type"`
	ReferenceID   string     `json:"reference_id"`
	Lines         []lineJSON `json:"lines"`
	Total         amountJSON `json:"total"`
	CreatedAt     string     `json:"created_at"`
}

// createAdjustment phục vụ POST /api/v1/admin/ledger/adjustments
// (operationId: createLedgerAdjustment).
//
// KHÔNG có endpoint nào SỬA bút toán cũ. Sửa sai trong sổ cái bất biến chỉ
// có một cách: ghi một bút toán điều chỉnh MỚI — xem ADR-0008.
func (h *Handler) createAdjustment(w http.ResponseWriter, r *http.Request) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"createAdjustment chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	key, ok := httpserver.IdempotencyKeyFrom(r.Context())
	if !ok {
		// Middleware RequireIdempotencyKey chưa được nối. Thất bại theo
		// hướng an toàn: không có khóa thì một lần thử lại vì mạng chập
		// chờn sẽ ghi bút toán thứ hai và nhân đôi số tiền.
		h.log.ErrorContext(r.Context(),
			"createAdjustment chạy không qua RequireIdempotencyKey")
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Thiếu header Idempotency-Key"))
		return
	}

	var req adjustmentRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	// Không cố định tiền tố: điều chỉnh có thể tham chiếu đơn hàng, seller,
	// hay lô chi trả. Chỉ kiểm tra định dạng định danh hợp lệ.
	if !ids.IsValid(req.ReferenceID) {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"reference_id không đúng định dạng"))
		return
	}
	refID := ids.ID(req.ReferenceID)

	lines, err := toDomainLines(req.Lines)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	entry, err := h.svc.RecordAdjustmentWithAudit(r.Context(),
		application.AdjustmentInput{
			ReferenceType:  req.ReferenceType,
			ReferenceID:    refID,
			Lines:          lines,
			ActorID:        ac.UserID,
			Reason:         req.Reason,
			IdempotencyKey: key,
			RequestID:      logger.RequestIDFromContext(r.Context()),
		})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, toResponse(entry))
}

// toDomainLines chuyển các dòng từ JSON sang domain.
//
// KHÔNG kiểm tra cân bằng ở đây: đó là bất biến của domain, và kiểm tra hai
// nơi nghĩa là sớm muộn hai nơi lệch nhau. Tầng này chỉ chuyển đổi.
func toDomainLines(in []lineJSON) ([]domain.Line, error) {
	if len(in) < 2 {
		return nil, apierror.New(apierror.CodeValidationFailed,
			"Bút toán kép cần ít nhất hai dòng: tiền đi từ đâu tới đâu")
	}

	out := make([]domain.Line, 0, len(in))
	for i, l := range in {
		amount, err := money.New(l.Amount.Amount, money.Currency(l.Amount.Currency))
		if err != nil {
			return nil, apierror.Newf(apierror.CodeValidationFailed,
				"dòng %d: số tiền hoặc đơn vị tiền tệ không hợp lệ", i+1)
		}

		// Tài khoản của nền tảng (PLATFORM_REVENUE, PLATFORM_CASH) không có
		// chủ sở hữu; tài khoản phải trả thì có.
		var ownerID ids.ID
		if l.OwnerID != "" {
			if !ids.IsValid(l.OwnerID) {
				return nil, apierror.Newf(apierror.CodeValidationFailed,
					"dòng %d: owner_id không đúng định dạng", i+1)
			}
			ownerID = ids.ID(l.OwnerID)
		}

		out = append(out, domain.Line{
			Account: domain.Account{
				Type:    domain.AccountType(l.AccountType),
				OwnerID: ownerID,
			},
			Direction:   domain.Direction(l.Direction),
			Amount:      amount,
			Description: l.Description,
		})
	}
	return out, nil
}

func toResponse(e *domain.LedgerEntry) adjustmentResponse {
	lines := make([]lineJSON, 0, len(e.Lines()))
	var total int64
	var currency string

	for _, l := range e.Lines() {
		lines = append(lines, lineJSON{
			AccountType: string(l.Account.Type),
			OwnerID:     l.Account.OwnerID.String(),
			Direction:   string(l.Direction),
			Amount: amountJSON{
				Amount:   l.Amount.Amount(),
				Currency: string(l.Amount.Currency()),
			},
			Description: l.Description,
		})
		if l.Direction == domain.Debit {
			total += l.Amount.Amount()
			currency = string(l.Amount.Currency())
		}
	}

	return adjustmentResponse{
		ID:            e.ID().String(),
		Type:          string(e.Type()),
		ReferenceType: e.ReferenceType(),
		ReferenceID:   e.ReferenceID().String(),
		Lines:         lines,
		Total:         amountJSON{Amount: total, Currency: currency},
		CreatedAt:     e.CreatedAt().UTC().Format("2006-01-02T15:04:05Z"),
	}
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
// Bút toán không cân trả 422 kèm CHI TIẾT chênh lệch — người vận hành cần
// biết lệch bao nhiêu để sửa, không phải đoán.
func translate(err error) error {
	switch {
	case errors.Is(err, domain.ErrUnbalanced):
		return apierror.New(apierror.CodeLedgerEntryUnbalanced,
			"Tổng ghi nợ phải bằng tổng ghi có")

	case errors.Is(err, audit.ErrReasonRequired):
		return apierror.New(apierror.CodeValidationFailed,
			"Lý do bắt buộc, tối thiểu 20 ký tự và phải có nội dung thật")

	case errors.Is(err, domain.ErrNoLines),
		errors.Is(err, domain.ErrMissingReference),
		errors.Is(err, domain.ErrNonPositiveLine),
		errors.Is(err, domain.ErrMixedCurrency):
		return apierror.New(apierror.CodeValidationFailed, err.Error())

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
