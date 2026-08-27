package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment/application"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
)

// GhiSuKien là cổng ra để ghi nhật ký webhook.
//
// PORT do module định nghĩa: fulfillment không import platform/webhook,
// và bên khởi chạy nối hai thứ lại.
type GhiSuKien interface {
	Ghi(ctx context.Context, nhaCungCap, maSuKien, loaiSuKien string, than []byte) (SuKienDaGhi, error)
	DanhDauXong(ctx context.Context, id string, loi error) error
}

// SuKienDaGhi khớp webhook.SuKien.
type SuKienDaGhi struct {
	ID            string
	DaNhanTruocDo bool
	DaXuLyXong    bool
}

// BiMatNhaCungCap trả khóa HMAC của một hãng vận chuyển.
//
// Trả chuỗi rỗng khi không biết hãng đó — và khi ấy chữ ký chắc chắn
// KHÔNG hợp lệ, đúng như mong muốn: hãng chưa cấu hình thì không gửi được
// gì vào hệ thống.
type BiMatNhaCungCap func(nhaCungCap string) string

// WebhookHandler nhận cập nhật vận chuyển từ hãng giao hàng.
//
// # Năm yêu cầu của api/paths/webhooks.yaml, và chỗ nào đáp ứng
//
//  1. Xác minh chữ ký  → httpserver.KiemChuKyHMAC, trước mọi thứ khác
//  2. Idempotent       → chỉ mục UNIQUE (provider, provider_event_id)
//  3. Trả 200 nhanh    → ghi nhật ký rồi mới xử lý; xử lý hỏng VẪN trả 200
//  4. Ghi mọi webhook  → kể cả loại không xử lý (PICKED_UP…)
//  5. Không tin tuyệt đối → cần job đối chiếu định kỳ; CHƯA có, xem ghi chú
type WebhookHandler struct {
	svc    *application.Service
	nhatKy GhiSuKien
	biMat  BiMatNhaCungCap
	log    *slog.Logger
}

func NewWebhookHandler(
	svc *application.Service, nhatKy GhiSuKien, biMat BiMatNhaCungCap, log *slog.Logger,
) *WebhookHandler {
	return &WebhookHandler{svc: svc, nhatKy: nhatKy, biMat: biMat, log: log}
}

func (h *WebhookHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/webhooks/shipping/{provider}", http.HandlerFunc(h.nhan))
}

// maxWebhookBytes chặn một thân request khổng lồ làm cạn bộ nhớ.
//
// 64KB rộng hơn nhiều lần mọi webhook vận chuyển thật.
const maxWebhookBytes = 64 << 10

type shippingWebhookJSON struct {
	EventID        string `json:"event_id"`
	TrackingNumber string `json:"tracking_number"`
	Status         string `json:"status"`
	FailureReason  string `json:"failure_reason"`
}

func (h *WebhookHandler) nhan(w http.ResponseWriter, r *http.Request) {
	nhaCungCap := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))

	than, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBytes+1))
	if err != nil || len(than) > maxWebhookBytes {
		h.tuChoi(w, r, nhaCungCap, "thân request quá lớn hoặc không đọc được")
		return
	}

	// XÁC MINH CHỮ KÝ TRƯỚC MỌI THỨ KHÁC.
	//
	// Trước cả việc đọc JSON: thân chưa xác minh là dữ liệu của người lạ,
	// và mọi thao tác trên nó đều là bề mặt tấn công.
	//
	// Chữ ký tính trên BYTE THÔ, không phải trên JSON đã parse rồi
	// serialize lại — vòng đó đổi thứ tự khóa và khoảng trắng.
	if err := httpserver.KiemChuKyHMAC(than,
		r.Header.Get("X-Signature"), h.biMat(nhaCungCap)); err != nil {
		metrics.RecordFailure(metrics.StageWebhook, "chu_ky_sai")
		h.log.WarnContext(r.Context(), "webhook có chữ ký KHÔNG hợp lệ",
			"nha_cung_cap", nhaCungCap,
			"request_id", logger.RequestIDFromContext(r.Context()))
		apierror.Write(w, r, apierror.New(apierror.CodeUnauthorized,
			"Chữ ký không hợp lệ"), logger.RequestIDFromContext(r.Context()), h.log)
		return
	}

	var p shippingWebhookJSON
	if err := json.Unmarshal(than, &p); err != nil ||
		strings.TrimSpace(p.EventID) == "" {
		h.tuChoi(w, r, nhaCungCap, "thiếu event_id hoặc JSON sai định dạng")
		return
	}

	// GHI NHẬT KÝ TRƯỚC KHI XỬ LÝ.
	//
	// Ghi trước nghĩa là sự kiện không mất dù xử lý hỏng: nó nằm lại ở
	// trạng thái chưa-xử-lý và job thử lại nhặt được. Xử lý trước rồi mới
	// ghi thì một lần chết giữa chừng làm mất hẳn sự kiện, và nhà cung cấp
	// đã coi như xong.
	su, err := h.nhatKy.Ghi(r.Context(), nhaCungCap, p.EventID, p.Status, than)
	if err != nil {
		apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
		return
	}

	if su.DaNhanTruocDo && su.DaXuLyXong {
		// 200, KHÔNG phải lỗi: báo lỗi khiến nhà cung cấp gửi lại mãi cho
		// một việc đã xong.
		h.tra(w, r, true)
		return
	}

	loiXuLy := h.svc.CapNhatTuHangVanChuyen(r.Context(),
		application.CapNhatTuHangVanChuyenInput{
			NhaVanChuyen: nhaCungCap,
			MaVanDon:     strings.TrimSpace(p.TrackingNumber),
			TrangThai:    strings.ToUpper(strings.TrimSpace(p.Status)),
			LyDoThatBai:  p.FailureReason,
		})

	switch {
	case loiXuLy == nil, errors.Is(loiXuLy, application.ErrTrangThaiKhongDoi):
		// Mốc hệ thống không theo dõi vẫn tính là đã xử lý xong: không có
		// gì để làm lại.
		_ = h.nhatKy.DanhDauXong(r.Context(), su.ID, nil)

	case errors.Is(loiXuLy, domain.ErrNotFound):
		// 404 là câu trả lời ĐÚNG cho mã vận đơn lạ, và đặc tả yêu cầu nó.
		// Vẫn giữ bản ghi: nó là bằng chứng hãng đã gửi cho một mã ta
		// không biết, thường là dấu hiệu lệch dữ liệu giữa hai bên.
		_ = h.nhatKy.DanhDauXong(r.Context(), su.ID, loiXuLy)
		metrics.RecordFailure(metrics.StageWebhook, "khong_thay_ma_van_don")
		apierror.Write(w, r, apierror.New(apierror.CodeNotFound,
			"Không tìm thấy lô hàng với mã vận đơn này"),
			logger.RequestIDFromContext(r.Context()), h.log)
		return

	default:
		// Xử lý hỏng nhưng sự kiện ĐÃ được ghi durable. Trả 200 để nhà
		// cung cấp thôi gửi lại; việc thử lại là của ta, không phải của họ.
		_ = h.nhatKy.DanhDauXong(r.Context(), su.ID, loiXuLy)
		metrics.RecordFailure(metrics.StageWebhook, "xu_ly_that_bai")
		h.log.ErrorContext(r.Context(), "xử lý webhook vận chuyển thất bại",
			"error", loiXuLy, "nha_cung_cap", nhaCungCap,
			"ma_su_kien", p.EventID)
	}

	h.tra(w, r, su.DaNhanTruocDo)
}

func (h *WebhookHandler) tra(w http.ResponseWriter, r *http.Request, daXuLy bool) {
	body := map[string]any{"received": true, "already_processed": daXuLy}
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response", "error", err)
	}
}

func (h *WebhookHandler) tuChoi(
	w http.ResponseWriter, r *http.Request, nhaCungCap, vaoDe string,
) {
	metrics.RecordFailure(metrics.StageWebhook, "than_khong_hop_le")
	h.log.WarnContext(r.Context(), "webhook không hợp lệ",
		"nha_cung_cap", nhaCungCap, "ly_do", vaoDe)
	apierror.Write(w, r, apierror.New(apierror.CodeValidationFailed, vaoDe),
		logger.RequestIDFromContext(r.Context()), h.log)
}
