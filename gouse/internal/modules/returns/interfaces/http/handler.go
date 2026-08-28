// Package http là tầng HTTP của module returns.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/returns/application"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// OrderAccessPort trả lời "người này có được xem đơn kia không".
//
// PORT do returns định nghĩa: quy tắc quyền xem đơn nằm ở module order,
// một chỗ duy nhất. Cài lại ở đây nghĩa là sớm muộn có một nơi cài lỏng
// hơn — và một nơi lỏng là đủ để lộ lịch sử mua hàng.
type OrderAccessPort interface {
	// ResolveViewableOrder phân giải mã đơn VÀ kiểm quyền trong một bước.
	//
	// Cùng chữ ký với cổng của module fulfillment, và cố ý như vậy: hai
	// module hỏi CÙNG một câu thì phải hỏi theo cùng một cách, nếu không
	// sớm muộn một bên hỏi lỏng hơn.
	//
	// `key` nhận cả mã đơn lẫn mã hiển thị — khách vãng lai chỉ có mã hiển
	// thị trong email xác nhận.
	ResolveViewableOrder(
		ctx context.Context, key, customerID, guestPhone string,
	) (orderID string, allowed bool, err error)
}

// ---------------------------------------------------------------- Khách

type CustomerHandler struct {
	svc    *application.Service
	access OrderAccessPort
	log    *slog.Logger
}

func NewCustomerHandler(
	svc *application.Service, access OrderAccessPort, log *slog.Logger,
) *CustomerHandler {
	return &CustomerHandler{svc: svc, access: access, log: log}
}

func (h *CustomerHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/orders/{order_id}/returns", http.HandlerFunc(h.xinTra))
	mux.Handle("GET /api/v1/orders/{order_id}/returns", http.HandlerFunc(h.danhSach))
}

// xinTraRequest khớp `requestReturn` trong đặc tả.
//
// `reason_code` nằm ở TỪNG DÒNG, không phải cấp yêu cầu: khách trả hai món
// vì hai lý do khác nhau là chuyện thường, và mỗi lý do dẫn tới một hành
// động khắc phục khác nhau.
type xinTraRequest struct {
	Lines []struct {
		OrderLineID  string `json:"order_line_id"`
		Quantity     int    `json:"quantity"`
		ReasonCode   string `json:"reason_code"`
		ReasonDetail string `json:"reason_detail"`
	} `json:"lines"`
}

func (h *CustomerHandler) xinTra(w http.ResponseWriter, r *http.Request) {
	orderID, err := h.kiemQuyen(r, r.PathValue("order_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}

	var req xinTraRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if len(req.Lines) == 0 {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"lines là trường bắt buộc"))
		return
	}

	dong := make([]application.DongXinTra, 0, len(req.Lines))
	for _, it := range req.Lines {
		if it.Quantity <= 0 {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"quantity phải lớn hơn 0"))
			return
		}
		dong = append(dong, application.DongXinTra{
			OrderLineID: ids.ID(strings.TrimSpace(it.OrderLineID)),
			Quantity:    it.Quantity,
			LyDo:        domain.LyDo(strings.ToUpper(strings.TrimSpace(it.ReasonCode))),
			ChiTiet:     it.ReasonDetail,
		})
	}

	shopper, _ := httpserver.ShopperFrom(r.Context())
	y, err := h.svc.XinTra(r.Context(), application.XinTraInput{
		OrderID:    ids.ID(orderID),
		CustomerID: ids.ID(shopper.CustomerID),
		Dong:       dong,
	})
	if err != nil {
		h.fail(w, r, dich(err))
		return
	}

	h.ok(w, r, http.StatusCreated, map[string]any{"return": toJSON(y)})
}

func (h *CustomerHandler) danhSach(w http.ResponseWriter, r *http.Request) {
	orderID, err := h.kiemQuyen(r, r.PathValue("order_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}

	ds, err := h.svc.DanhSachTheoDon(r.Context(), ids.ID(orderID))
	if err != nil {
		h.fail(w, r, dich(err))
		return
	}

	out := make([]returnJSON, 0, len(ds))
	for _, y := range ds {
		out = append(out, toJSON(y))
	}
	h.ok(w, r, http.StatusOK, map[string]any{"data": out})
}

// kiemQuyen chặn người này đọc hoặc trả hàng của đơn người khác.
//
// Trả về mã đơn ĐÃ PHÂN GIẢI: khách vãng lai gửi mã hiển thị, không phải
// mã nội bộ.
func (h *CustomerHandler) kiemQuyen(r *http.Request, key string) (string, error) {
	if h.access == nil {
		return "", apierror.New(apierror.CodeForbidden, "Không kiểm tra được quyền")
	}
	shopper, _ := httpserver.ShopperFrom(r.Context())
	orderID, duoc, err := h.access.ResolveViewableOrder(r.Context(), key,
		shopper.CustomerID, r.Header.Get("X-Guest-Phone"))
	if err != nil {
		return "", err
	}
	if !duoc {
		// 404 chứ không phải 403: phân biệt "không có quyền" với "không
		// tồn tại" cho phép dò mã đơn của người khác.
		return "", apierror.New(apierror.CodeNotFound, "Không tìm thấy đơn hàng")
	}
	return orderID, nil
}

// ---------------------------------------------------------------- Nhà bán

type SellerHandler struct {
	svc *application.Service
	log *slog.Logger
}

func NewSellerHandler(svc *application.Service, log *slog.Logger) *SellerHandler {
	return &SellerHandler{svc: svc, log: log}
}

func (h *SellerHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/seller/returns", http.HandlerFunc(h.danhSach))
	mux.Handle("POST /api/v1/seller/returns/{return_id}/approve", http.HandlerFunc(h.duyet))
	mux.Handle("POST /api/v1/seller/returns/{return_id}/reject", http.HandlerFunc(h.tuChoi))
	mux.Handle("POST /api/v1/seller/returns/{return_id}/receive", http.HandlerFunc(h.nhanHang))
	mux.Handle("POST /api/v1/seller/returns/{return_id}/inspect", http.HandlerFunc(h.kiemDinh))
}

func (h *SellerHandler) danhSach(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ds, err := h.svc.DanhSachTheoNhaBan(r.Context(), sellerID,
		strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))), 50)
	if err != nil {
		h.fail(w, r, dich(err))
		return
	}
	out := make([]returnJSON, 0, len(ds))
	for _, y := range ds {
		out = append(out, toJSON(y))
	}
	h.ok(w, r, http.StatusOK, map[string]any{"data": out})
}

func (h *SellerHandler) duyet(w http.ResponseWriter, r *http.Request) {
	h.buoc(w, r, func(sellerID, id ids.ID) (*domain.YeuCauTraHang, error) {
		return h.svc.Duyet(r.Context(), id, sellerID)
	})
}

func (h *SellerHandler) nhanHang(w http.ResponseWriter, r *http.Request) {
	h.buoc(w, r, func(sellerID, id ids.ID) (*domain.YeuCauTraHang, error) {
		return h.svc.NhanHangVaHoanTien(r.Context(), id, sellerID)
	})
}

func (h *SellerHandler) tuChoi(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		// Khách cần biết VÌ SAO để quyết định làm gì tiếp. Từ chối không
		// lý do biến mọi trường hợp thành khiếu nại.
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"reason là trường bắt buộc khi từ chối"))
		return
	}
	h.buoc(w, r, func(sellerID, id ids.ID) (*domain.YeuCauTraHang, error) {
		return h.svc.TuChoi(r.Context(), id, sellerID, req.Reason)
	})
}

// kiemDinh ghi kết quả kiểm định hàng hoàn.
//
// Đây là bước DUY NHẤT đưa hàng hoàn ra khỏi trạng thái Returned. Không có
// nó, mọi món hàng hoàn nằm chết vĩnh viễn: nhà bán mất cả hàng lẫn tiền,
// và con số tồn kho ngày càng xa thực tế.
func (h *SellerHandler) kiemDinh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lines []struct {
			OrderLineID string `json:"order_line_id"`
			Passed      bool   `json:"passed"`
			Note        string `json:"note"`
		} `json:"lines"`
	}
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if len(req.Lines) == 0 {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"lines là trường bắt buộc"))
		return
	}

	kq := make([]application.KetQuaKiemDinhDong, 0, len(req.Lines))
	for _, l := range req.Lines {
		kq = append(kq, application.KetQuaKiemDinhDong{
			OrderLineID: ids.ID(strings.TrimSpace(l.OrderLineID)),
			Dat:         l.Passed,
			GhiChu:      l.Note,
		})
	}

	h.buoc(w, r, func(sellerID, id ids.ID) (*domain.YeuCauTraHang, error) {
		return h.svc.KiemDinh(r.Context(), id, sellerID, kq)
	})
}

func (h *SellerHandler) buoc(
	w http.ResponseWriter, r *http.Request,
	lam func(sellerID, id ids.ID) (*domain.YeuCauTraHang, error),
) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	y, err := lam(sellerID, ids.ID(r.PathValue("return_id")))
	if err != nil {
		h.fail(w, r, dich(err))
		return
	}
	h.ok(w, r, http.StatusOK, map[string]any{"return": toJSON(y)})
}

func (h *SellerHandler) sellerID(r *http.Request) (ids.ID, error) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"seller returns chạy không qua Auth — kiểm tra nối dây")
		return "", apierror.ErrUnauthorized
	}
	if len(ac.SellerIDs) == 0 {
		return "", apierror.New(apierror.CodeForbidden,
			"Tài khoản này không gắn với nhà bán nào")
	}
	// Lấy gian hàng ĐẦU TIÊN, cùng cách module fulfillment làm: một tài
	// khoản gắn nhiều gian hàng là ca chưa hỗ trợ, và đoán bừa còn tệ hơn.
	return ids.ID(ac.SellerIDs[0]), nil
}

// ---------------------------------------------------------------- Chung

type returnLineJSON struct {
	OrderLineID  string    `json:"order_line_id"`
	SKUID        string    `json:"sku_id"`
	Quantity     int       `json:"quantity"`
	ReasonCode   string    `json:"reason_code"`
	ReasonDetail string    `json:"reason_detail,omitempty"`
	Refund       moneyJSON `json:"refund"`

	// Inspection: PENDING, PASSED, FAILED.
	Inspection     string `json:"inspection"`
	InspectionNote string `json:"inspection_note,omitempty"`
}

type moneyJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type returnJSON struct {
	ID           string           `json:"id"`
	OrderID      string           `json:"order_id"`
	Status       string           `json:"status"`
	ReasonCode   string           `json:"reason_code"`
	CustomerNote string           `json:"customer_note,omitempty"`
	RejectReason string           `json:"reject_reason,omitempty"`
	RefundAmount moneyJSON        `json:"refund_amount"`
	Items        []returnLineJSON `json:"items"`
	RequestedAt  string           `json:"requested_at"`
}

func toJSON(y *domain.YeuCauTraHang) returnJSON {
	out := returnJSON{
		ID: y.ID().String(), OrderID: y.OrderID().String(),
		Status: string(y.Status()), ReasonCode: string(y.LyDo()),
		CustomerNote: y.GhiChu(), RejectReason: y.LyDoTuChoi(),
		RefundAmount: moneyJSON{
			Amount:   y.TienHoan().Amount(),
			Currency: string(y.TienHoan().Currency()),
		},
		RequestedAt: y.RequestedAt().Format("2006-01-02T15:04:05Z07:00"),
	}
	for _, d := range y.Dong() {
		out.Items = append(out.Items, returnLineJSON{
			OrderLineID: d.OrderLineID.String(), SKUID: d.SKUID.String(),
			Quantity:   d.Quantity,
			ReasonCode: string(d.LyDo), ReasonDetail: d.ChiTiet,
			Inspection: string(d.KiemDinh), InspectionNote: d.GhiChuKiemDinh,
			Refund: moneyJSON{
				Amount:   d.TienHoan.Amount(),
				Currency: string(d.TienHoan.Currency()),
			},
		})
	}
	return out
}

// dich chuyển lỗi nội bộ thành lỗi HTTP.
func dich(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy yêu cầu trả hàng")
	case errors.Is(err, domain.ErrInvalidReason):
		return apierror.New(apierror.CodeValidationFailed,
			"reason_code không hợp lệ")
	case errors.Is(err, domain.ErrMissingReason):
		return apierror.New(apierror.CodeValidationFailed, "Phải nêu lý do")
	case errors.Is(err, domain.ErrQuantityExceeded):
		return apierror.New(apierror.CodeValidationFailed,
			"Số lượng xin trả vượt số đã mua")
	case errors.Is(err, domain.ErrDuplicateLine):
		return apierror.New(apierror.CodeConflict,
			"Món này đã có yêu cầu trả hàng đang xử lý")
	case errors.Is(err, domain.ErrInvalidStatus):
		return apierror.New(apierror.CodeConflict,
			"Không thực hiện được với trạng thái hiện tại")
	case errors.Is(err, domain.ErrVersionConflict):
		return apierror.New(apierror.CodeConflict,
			"Yêu cầu vừa được cập nhật, vui lòng tải lại")

	case errors.Is(err, domain.ErrChuaNhanHang):
		return apierror.New(apierror.CodeConflict,
			"Chưa nhận được hàng, không kiểm định được")
	case errors.Is(err, domain.ErrDaKiemDinh):
		return apierror.New(apierror.CodeConflict,
			"Món này đã kiểm định rồi")
	case errors.Is(err, domain.ErrThieuLyDoLoai):
		return apierror.New(apierror.CodeValidationFailed,
			"note là trường bắt buộc khi loại hàng")

	case errors.Is(err, domain.ErrGiamGiaChuaPhanBo):
		// 409 kèm thông điệp nói THẲNG vấn đề: đây không phải lỗi của
		// khách, và người vận hành cần biết để xử lý tay.
		return apierror.New(apierror.CodeConflict,
			"Đơn có khuyến mãi chưa phân bổ xuống từng món — "+
				"không tính tự động được số tiền hoàn, vui lòng liên hệ hỗ trợ")
	}
	return err
}

func decodeJSON(r *http.Request, dst any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return apierror.New(apierror.CodeValidationFailed, "Không đọc được request")
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return apierror.New(apierror.CodeValidationFailed,
			fmt.Sprintf("JSON không hợp lệ: %v", err))
	}
	return nil
}

func (h *CustomerHandler) ok(w http.ResponseWriter, r *http.Request, code int, body any) {
	if err := apierror.WriteJSON(w, code, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response", "error", err)
	}
}

func (h *CustomerHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

func (h *SellerHandler) ok(w http.ResponseWriter, r *http.Request, code int, body any) {
	if err := apierror.WriteJSON(w, code, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response", "error", err)
	}
}

func (h *SellerHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}
