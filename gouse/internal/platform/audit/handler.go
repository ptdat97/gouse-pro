package audit

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ endpoint đọc nhật ký thao tác.
//
// # Vì sao handler nằm cùng package, không ở internal/modules/.../interfaces/
//
// audit là năng lực platform (ADR-0011), không phải module nghiệp vụ — nó
// không có tầng domain/application/infrastructure để tách. Đặt handler ở
// cmd/api sẽ làm main biết hình dạng response của một tính năng; đặt ở một
// module giả sẽ tạo ra module rỗng mà ADR-0011 đã loại.
//
// # Endpoint này CHỈ ĐỌC
//
// Không có endpoint tạo, sửa, hay xóa bản ghi nhật ký. Bản ghi chỉ sinh ra
// như HỆ QUẢ của thao tác khác — không ai "ghi nhật ký" một cách trực tiếp,
// vì như thế thì nhật ký ghi được điều không xảy ra.
type Handler struct {
	rec *Recorder
	log *slog.Logger
}

// NewHandler tạo handler.
func NewHandler(rec *Recorder, log *slog.Logger) *Handler {
	return &Handler{rec: rec, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc middleware Auth và RequireRole("ADMIN") —
// admin-api.md mục 7: chỉ vai trò ADMIN đọc được nhật ký. Handler này
// không tự kiểm tra vai trò vì platform không biết "ADMIN" nghĩa là gì
// trong mô hình phân quyền (quy tắc R3).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/admin/audit-log", http.HandlerFunc(h.list))
}

type recordJSON struct {
	ID           string         `json:"id"`
	ActorType    string         `json:"actor_type"`
	ActorID      string         `json:"actor_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	RequestID    string         `json:"request_id,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	OccurredAt   string         `json:"occurred_at"`
}

type listResponse struct {
	Data       []recordJSON `json:"data"`
	Pagination pagination   `json:"pagination"`
}

type pagination struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// list phục vụ GET /api/v1/admin/audit-log (operationId: listAuditLog).
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	f := Filter{
		ResourceType: q.Get("resource_type"),
		ResourceID:   q.Get("resource_id"),
		Action:       q.Get("action"),
		ActorID:      q.Get("actor_id"),
		Cursor:       q.Get("cursor"),
	}

	if v := q.Get("limit"); v != "" {
		// TỪ CHỐI khi vượt trần, không cắt bớt im lặng.
		//
		// Đặc tả khai `maximum: 100` cho tham số này, nên một giá trị lớn
		// hơn là request SAI — và nói thẳng thì client sửa được.
		//
		// Cắt bớt im lặng nguy hiểm hơn ở đúng chỗ nó có vẻ hiền lành:
		// client xin 500, nhận 100, tưởng đã lấy hết và dừng phân trang.
		// Dữ liệu mất mà không có lỗi nào.
		//
		// `Query` vẫn cắt bớt như một lớp phòng thủ cuối — nhưng nó không
		// còn là chỗ DUY NHẤT xử lý chuyện này.
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				fmt.Sprintf("limit phải là số nguyên từ 1 đến %d", maxLimit)))
			return
		}
		f.Limit = n
	}

	// Đặc tả khai báo `format: date` — chỉ ngày, không giờ.
	var err error
	if f.From, err = parseDate(q.Get("from")); err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"from phải theo định dạng YYYY-MM-DD"))
		return
	}
	if f.To, err = parseDate(q.Get("to")); err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"to phải theo định dạng YYYY-MM-DD"))
		return
	}
	if !f.To.IsZero() {
		// "đến ngày 31/08" phải bao gồm CẢ NGÀY 31, không phải dừng lúc
		// 00:00 sáng hôm đó — nếu không, nhân viên lọc theo tháng sẽ mất
		// toàn bộ bản ghi của ngày cuối tháng mà không biết.
		f.To = f.To.Add(24*time.Hour - time.Nanosecond)
	}

	records, next, err := h.rec.Query(r.Context(), f)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	out := make([]recordJSON, 0, len(records))
	for _, rec := range records {
		out = append(out, recordJSON{
			ID:           rec.ID,
			ActorType:    rec.ActorType,
			ActorID:      rec.ActorID,
			Action:       rec.Action,
			ResourceType: rec.ResourceType,
			ResourceID:   rec.ResourceID,
			Reason:       rec.Reason,
			RequestID:    rec.RequestID,
			Metadata:     rec.Metadata,
			OccurredAt:   rec.OccurredAt.UTC().Format(time.RFC3339),
		})
	}

	h.ok(w, r, listResponse{
		Data: out,
		Pagination: pagination{
			NextCursor: next,
			HasMore:    next != "",
		},
	})
}

// parseDate đọc ngày dạng YYYY-MM-DD. Chuỗi rỗng trả về thời điểm zero.
func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02", s)
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
