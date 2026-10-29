package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
)

// WebhookLog ghi nhật ký webhook — cổng tới platform/webhook.
//
// Khai ở đây chứ không import platform/webhook: module nghiệp vụ không
// được phụ thuộc vào một chi tiết hạ tầng. Adapter nằm ở internal/app.
type WebhookLog interface {
	Log(ctx context.Context, provider, maSuKien, loaiSuKien string, than []byte) (LoggedEvent, error)
	MarkDone(ctx context.Context, id string, loi error) error
}

// LoggedEvent là kết quả ghi nhật ký.
type LoggedEvent struct {
	ID              string
	AlreadyReceived bool
	AlreadyDone     bool
}

// ProviderSecret trả khóa HMAC của một cổng thanh toán.
//
// Trả chuỗi rỗng khi không biết nhà cung cấp đó — và khi ấy chữ ký chắc
// chắn KHÔNG hợp lệ. Mặc định là ĐÓNG.
type ProviderSecret func(provider string) string

// MarkPaid đánh dấu đơn đã thanh toán.
//
// Cổng tới module order. Khai ở BÊN GỌI để module payment không phụ thuộc
// order — chiều phụ thuộc đã là order → payment.
type MarkPaid func(ctx context.Context, orderID string) error

// WebhookHandler nhận thông báo trạng thái thanh toán từ cổng thanh toán.
//
// # BA LỚP BẢO VỆ của api/paths/webhooks.yaml, và chỗ nào đáp ứng
//
//  1. Chữ ký HMAC       → httpserver.VerifyHMAC, TRƯỚC mọi thứ khác
//  2. Idempotency       → chỉ mục UNIQUE (provider, provider_event_id)
//  3. ĐỐI CHIẾU SỐ TIỀN → payment_intent (ADR-0017) — lớp này là lý do
//     endpoint không được mở trước khi có bảng đó
//
// # Vì sao lớp ba không thể bỏ
//
// Chữ ký chứng minh thông điệp đến TỪ nhà cung cấp; nó không chứng minh
// con số BÊN TRONG đúng. Lỗi tích hợp phía họ, một khóa bị lộ, hay môi
// trường test gọi nhầm prod — cả ba đều có chữ ký hợp lệ và số tiền sai,
// và cả ba đều kết thúc bằng tiền ghi vào một cuốn sổ BẤT BIẾN.
type WebhookHandler struct {
	svc        *application.Service
	webhookLog WebhookLog
	secretOf   ProviderSecret
	marked     MarkPaid
	log        *slog.Logger
}

func NewWebhookHandler(
	svc *application.Service, webhookLog WebhookLog, secretOf ProviderSecret,
	marked MarkPaid, log *slog.Logger,
) *WebhookHandler {
	return &WebhookHandler{svc: svc, webhookLog: webhookLog, secretOf: secretOf, marked: marked, log: log}
}

func (h *WebhookHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/webhooks/payment/{provider}", http.HandlerFunc(h.receive))
}

// maxWebhookBytes chặn một thân request khổng lồ làm cạn bộ nhớ.
const maxWebhookBytes = 64 << 10

type paymentWebhookJSON struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	Data      struct {
		PaymentIntentID string `json:"payment_intent_id"`
		Amount          int64  `json:"amount"`
		Currency        string `json:"currency"`
		Metadata        struct {
			OrderID    string `json:"order_id"`
			CheckoutID string `json:"checkout_id"`
		} `json:"metadata"`
		FailureReason string `json:"failure_reason"`
	} `json:"data"`
}

func (h *WebhookHandler) receive(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	reqID := logger.RequestIDFromContext(r.Context())

	than, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBytes+1))
	if err != nil || len(than) > maxWebhookBytes {
		h.rejected(w, r, provider, "thân request quá lớn hoặc không đọc được")
		return
	}

	// LỚP 1 — XÁC MINH CHỮ KÝ, TRƯỚC MỌI THỨ KHÁC.
	//
	// Trước cả việc đọc JSON: thân chưa xác minh là dữ liệu của người lạ.
	// Chữ ký tính trên BYTE THÔ — parse rồi serialize lại sẽ đổi thứ tự
	// khóa và khoảng trắng.
	if err := httpserver.VerifyHMAC(than,
		r.Header.Get("X-Signature"), h.secretOf(provider)); err != nil {
		metrics.RecordFailure(metrics.StageWebhook, "chu_ky_sai")
		h.log.WarnContext(r.Context(), "webhook thanh toán có chữ ký KHÔNG hợp lệ",
			"nha_cung_cap", provider, "request_id", reqID)
		apierror.Write(w, r, apierror.New(apierror.CodeUnauthorized,
			"Chữ ký không hợp lệ"), reqID, h.log)
		return
	}

	var p paymentWebhookJSON
	if err := json.Unmarshal(than, &p); err != nil ||
		strings.TrimSpace(p.EventID) == "" {
		h.rejected(w, r, provider, "thiếu event_id hoặc JSON sai định dạng")
		return
	}

	// LỚP 2 — GHI NHẬT KÝ TRƯỚC KHI XỬ LÝ, và nó cũng là idempotency.
	//
	// Ghi trước nghĩa là sự kiện không mất dù xử lý hỏng. Xử lý trước rồi
	// mới ghi thì một lần chết giữa chừng làm mất hẳn sự kiện, và nhà cung
	// cấp đã coi như xong.
	su, err := h.webhookLog.Log(r.Context(), provider, p.EventID, p.EventType, than)
	if err != nil {
		apierror.Write(w, r, err, reqID, h.log)
		return
	}
	if su.AlreadyReceived && su.AlreadyDone {
		h.respond(w, r, true)
		return
	}

	orderID, err := ids.Parse(strings.TrimSpace(p.Data.Metadata.OrderID), ids.PrefixOrder)
	if err != nil {
		// Không có mã đơn thì không tra được intent, nên không đối chiếu
		// được — và không đối chiếu được thì KHÔNG xử lý. Giữ bản ghi làm
		// bằng chứng.
		_ = h.webhookLog.MarkDone(r.Context(), su.ID, errors.New("order_id không hợp lệ"))
		h.rejected(w, r, provider, "thiếu hoặc sai data.metadata.order_id")
		return
	}

	switch p.EventType {
	case "payment.succeeded":
		h.collected(w, r, su, provider, orderID, p)
	case "payment.failed":
		h.failed(w, r, su, provider, orderID, p)
	default:
		// Loại không xử lý VẪN được ghi nhật ký (yêu cầu 4), và vẫn trả
		// 200: báo lỗi khiến nhà cung cấp gửi lại mãi một loại ta cố tình
		// bỏ qua.
		_ = h.webhookLog.MarkDone(r.Context(), su.ID, nil)
		h.respond(w, r, su.AlreadyReceived)
	}
}

// collected xử lý payment.succeeded — nơi LỚP 3 đứng.
func (h *WebhookHandler) collected(
	w http.ResponseWriter, r *http.Request, su LoggedEvent,
	provider string, orderID ids.ID, p paymentWebhookJSON,
) {
	reqID := logger.RequestIDFromContext(r.Context())

	// Số tiền LẠ (âm, đơn vị tiền tệ không tồn tại) bị chặn ngay ở đây.
	// Đẩy xuống domain thì `money.New` cũng từ chối, nhưng lỗi sẽ mang
	// hình dạng "hỏng nội bộ" thay vì "bên ngoài gửi rác".
	amountIn, err := money.New(p.Data.Amount, money.Currency(strings.ToUpper(
		strings.TrimSpace(p.Data.Currency))))
	if err != nil {
		_ = h.webhookLog.MarkDone(r.Context(), su.ID, err)
		metrics.RecordFailure(metrics.StageWebhook, "so_tien_khong_hop_le")
		h.rejected(w, r, provider, "data.amount hoặc data.currency không hợp lệ")
		return
	}

	res, err := h.svc.MatchAndCapture(r.Context(), orderID, amountIn,
		provider, strings.TrimSpace(p.Data.PaymentIntentID))

	switch {
	case errors.Is(err, domain.ErrAmountMismatch):
		// LỚP 3 CHẶN ĐƯỢC MỘT LẦN. Đây là 422 mà webhooks.yaml quy định:
		// KHÔNG xử lý, cảnh báo NGAY.
		//
		// Mức ERROR chứ không WARN: chữ ký hợp lệ mà số tiền sai nghĩa là
		// hoặc nhà cung cấp đang lỗi, hoặc khóa của họ đã lộ. Cả hai đều
		// cần người xem trong hôm nay.
		_ = h.webhookLog.MarkDone(r.Context(), su.ID, err)
		metrics.RecordFailure(metrics.StageWebhook, "so_tien_khong_khop")
		h.log.ErrorContext(r.Context(), "webhook thanh toán báo SỐ TIỀN KHÁC hệ thống chờ thu",
			"nha_cung_cap", provider, "order_id", orderID.String(),
			"so_tien_bao", p.Data.Amount, "don_vi_bao", p.Data.Currency,
			"ma_su_kien", p.EventID, "request_id", reqID)
		apierror.Write(w, r, apierror.New(apierror.CodePaymentFailed,
			"Số tiền không khớp ý định thanh toán"), reqID, h.log)
		return

	case errors.Is(err, domain.ErrIntentNotFound):
		// Nhà cung cấp báo về một đơn hệ thống KHÔNG chờ thu tiền: đơn
		// COD, đơn không tồn tại, hoặc môi trường test gọi nhầm vào
		// production. Cả ba đều không được xử lý.
		_ = h.webhookLog.MarkDone(r.Context(), su.ID, err)
		metrics.RecordFailure(metrics.StageWebhook, "khong_co_intent")
		h.log.ErrorContext(r.Context(), "webhook thanh toán cho đơn KHÔNG chờ thu tiền",
			"nha_cung_cap", provider, "order_id", orderID.String(), "request_id", reqID)
		apierror.Write(w, r, apierror.New(apierror.CodeNotFound,
			"Không có ý định thanh toán cho đơn này"), reqID, h.log)
		return

	case err != nil:
		// Hỏng vì lý do khác: sự kiện ĐÃ ghi durable, trả 200 để nhà cung
		// cấp thôi gửi lại. Việc thử lại là của ta.
		_ = h.webhookLog.MarkDone(r.Context(), su.ID, err)
		metrics.RecordFailure(metrics.StageWebhook, "xu_ly_that_bai")
		h.log.ErrorContext(r.Context(), "xử lý webhook thanh toán thất bại",
			"error", err, "nha_cung_cap", provider, "order_id", orderID.String())
		h.respond(w, r, su.AlreadyReceived)
		return
	}

	// Đánh dấu đơn đã trả tiền — CHỈ ở lần thu thật đầu tiên.
	//
	// Nhà cung cấp gửi trùng là bình thường, và gọi MarkPaid lần hai sẽ
	// gặp máy trạng thái từ chối (đơn không còn PENDING_PAYMENT) — một lỗi
	// GIẢ, sinh ra bởi chính cơ chế bảo vệ.
	if !res.AlreadyCaptured && h.marked != nil {
		if err := h.marked(r.Context(), orderID.String()); err != nil {
			// Tiền ĐÃ ghi nhận thu; chỉ trạng thái đơn chưa theo kịp.
			// KHÔNG quay ngược intent: tiền về là sự thật đã xảy ra.
			h.log.ErrorContext(r.Context(),
				"đã thu tiền nhưng KHÔNG đánh dấu được đơn đã trả",
				"error", err, "order_id", orderID.String(),
				"goi_y", "đối soát tay: intent CAPTURED mà đơn vẫn PENDING_PAYMENT")
			metrics.RecordFailure(metrics.StageWebhook, "danh_dau_da_tra_that_bai")
		}
	}

	_ = h.webhookLog.MarkDone(r.Context(), su.ID, nil)
	h.respond(w, r, su.AlreadyReceived)
}

// failed xử lý payment.failed.
//
// KHÔNG đối chiếu số tiền: thất bại nghĩa là không có tiền nào chuyển, nên
// con số trong payload không có gì để kiểm.
func (h *WebhookHandler) failed(
	w http.ResponseWriter, r *http.Request, su LoggedEvent,
	provider string, orderID ids.ID, p paymentWebhookJSON,
) {
	_, err := h.svc.RecordFailure(r.Context(), orderID,
		strings.TrimSpace(p.Data.FailureReason))
	if err != nil && !errors.Is(err, domain.ErrIntentNotFound) {
		h.log.ErrorContext(r.Context(), "không ghi được thất bại thanh toán",
			"error", err, "nha_cung_cap", provider, "order_id", orderID.String())
	}
	_ = h.webhookLog.MarkDone(r.Context(), su.ID, err)
	h.respond(w, r, su.AlreadyReceived)
}

func (h *WebhookHandler) respond(w http.ResponseWriter, r *http.Request, processed bool) {
	body := map[string]any{"received": true, "already_processed": processed}
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response", "error", err)
	}
}

func (h *WebhookHandler) rejected(
	w http.ResponseWriter, r *http.Request, provider, topic string,
) {
	metrics.RecordFailure(metrics.StageWebhook, "than_khong_hop_le")
	h.log.WarnContext(r.Context(), "webhook thanh toán không hợp lệ",
		"nha_cung_cap", provider, "ly_do", topic)
	apierror.Write(w, r, apierror.New(apierror.CodeValidationFailed, topic),
		logger.RequestIDFromContext(r.Context()), h.log)
}
