package app

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/opsconfig"
)

// opsConfigHandler phục vụ trang cấu hình vận hành của giao diện quản trị.
//
// # Vì sao nằm ở internal/app chứ không phải một module
//
// Tham số vận hành cắt ngang nhiều module: hôm nay là ngưỡng của
// fulfillment, mai có thể là tham số của promotion. Đặt nó vào một module
// nghiệp vụ nghĩa là module đó trở thành nơi mọi module khác phải hỏi —
// đúng thứ mà kiến trúc mô-đun này tránh.
//
// `internal/app` là gốc lắp ghép, nơi duy nhất được biết tất cả.
type opsConfigHandler struct {
	cfg   *opsconfig.Store
	audit *audit.Recorder
	log   *slog.Logger
}

func (h *opsConfigHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/admin/config", http.HandlerFunc(h.danhSach))
	mux.Handle("PUT /api/v1/admin/config/{key}", http.HandlerFunc(h.dat))
}

type thamSoJSON struct {
	Key     string  `json:"key"`
	Type    string  `json:"type"`
	Value   float64 `json:"value"`
	Default float64 `json:"default"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`

	// IsDefault cho giao diện phân biệt "chưa ai đặt" với "đặt trùng
	// đúng giá trị mặc định" — hai chuyện khác nhau khi đi tìm xem ai đã
	// đổi gì.
	IsDefault bool `json:"is_default"`

	Description string `json:"description"`

	// Impact nói điều gì xảy ra khi đổi.
	//
	// Bắt buộc hiện trên giao diện: người đổi con số hiếm khi là người
	// viết đoạn mã đọc nó, và "48" không tự nói rằng hạ nó xuống sẽ làm
	// hàng loạt gian hàng đột ngột bị chấm là giao trễ.
	Impact string `json:"impact"`

	UpdatedAt string `json:"updated_at,omitempty"`
	UpdatedBy string `json:"updated_by,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func (h *opsConfigHandler) danhSach(w http.ResponseWriter, r *http.Request) {
	ds, err := h.cfg.DanhSach(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}

	ra := make([]thamSoJSON, 0, len(ds))
	for _, g := range ds {
		t := thamSoJSON{
			Key: g.Tham.Khoa, Type: string(g.Tham.Kieu),
			Value: g.HienTai, Default: g.Tham.MacDinh,
			Min: g.Tham.Min, Max: g.Tham.Max,
			IsDefault:   g.LaMacDinh,
			Description: g.Tham.MoTa,
			Impact:      g.Tham.HeQua,
			UpdatedBy:   g.SuaBoi,
			Reason:      g.LyDo,
		}
		if !g.SuaLuc.IsZero() {
			t.UpdatedAt = g.SuaLuc.UTC().Format(time.RFC3339)
		}
		ra = append(ra, t)
	}
	h.ok(w, r, map[string]any{"data": ra})
}

type datThamSoRequest struct {
	Value float64 `json:"value"`

	// Reason BẮT BUỘC, tối thiểu 20 ký tự.
	Reason string `json:"reason"`
}

// dat đổi một tham số.
//
// # Vì sao ghi vết BẮT BUỘC
//
// Đổi tham số vận hành ảnh hưởng tới người NGOÀI công ty: hạ hạn giao hàng
// làm hàng loạt gian hàng đột ngột bị chấm là giao trễ, và điểm đó ảnh
// hưởng tới việc họ thắng buy box. Một lần đổi không có người chịu trách
// nhiệm và không có lý do thì không giải thích được khi nhà bán khiếu nại.
//
// Ghi vết TRƯỚC khi đổi, và ghi vết hỏng thì KHÔNG đổi — cùng thứ tự với
// `order.GetOrderAsAdmin` và `customer.GetAsAdmin`.
func (h *opsConfigHandler) dat(w http.ResponseWriter, r *http.Request) {
	khoa := r.PathValue("key")
	tham, co := opsconfig.Tham(khoa)
	if !co {
		// 404 chứ không phải 400: khóa là một tài nguyên, và sổ đăng ký
		// ĐÓNG nên khóa lạ đơn giản là không tồn tại.
		h.fail(w, r, apierror.New(apierror.CodeNotFound,
			"Không có tham số cấu hình với khóa này"))
		return
	}

	var req datThamSoRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Dữ liệu gửi lên không hợp lệ"))
		return
	}

	lyDo := strings.TrimSpace(req.Reason)
	if err := audit.ValidateReason(lyDo); err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"reason phải là lý do có ý nghĩa, tối thiểu 20 ký tự — "+
				"đổi tham số vận hành ảnh hưởng tới nhà bán, và một lần đổi "+
				"không có lý do thì không giải thích được khi có khiếu nại"))
		return
	}

	if err := tham.KiemGiaTri(req.Value); err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed, err.Error()))
		return
	}

	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"cấu hình vận hành chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	// Giá trị CŨ đi vào vết kiểm toán.
	//
	// "Đổi thành 24" không trả lời được câu hỏi quan trọng nhất khi điều
	// tra: đổi từ bao nhiêu?
	cu := h.cfg.Doc(khoa)

	if err := h.audit.Write(r.Context(), audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      ac.UserID,
		Action:       "ops_config.set",
		ResourceType: audit.ResourceConfig,
		ResourceID:   khoa,
		Reason:       lyDo,
		RequestID:    logger.RequestIDFromContext(r.Context()),
		Metadata: map[string]any{
			"gia_tri_cu":  cu,
			"gia_tri_moi": req.Value,
		},
	}); err != nil {
		h.fail(w, r, err)
		return
	}

	if err := h.cfg.Dat(r.Context(), opsconfig.DatInput{
		Khoa: khoa, GiaTri: req.Value,
		SuaBoi: ac.UserID, LyDo: lyDo,
	}); err != nil {
		h.fail(w, r, dichLoiCauHinh(err))
		return
	}

	h.log.InfoContext(r.Context(), "đổi tham số vận hành",
		"khoa", khoa, "cu", cu, "moi", req.Value, "boi", ac.UserID)

	h.ok(w, r, map[string]any{
		"key": khoa, "value": req.Value, "previous_value": cu,
	})
}

func dichLoiCauHinh(err error) error {
	switch {
	case errors.Is(err, opsconfig.ErrKhongCoKhoa):
		return apierror.New(apierror.CodeNotFound,
			"Không có tham số cấu hình với khóa này")
	case errors.Is(err, opsconfig.ErrNgoaiBien),
		errors.Is(err, opsconfig.ErrSaiKieu):
		return apierror.New(apierror.CodeValidationFailed, err.Error())
	default:
		return apierror.From(err)
	}
}

func (h *opsConfigHandler) ok(w http.ResponseWriter, r *http.Request, body any) {
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *opsConfigHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}
