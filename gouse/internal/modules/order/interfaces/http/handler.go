// Package http là tầng interfaces của module order cho giao diện quản trị.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/admin.yaml.
//
// # Vì sao response KHÔNG có lô giao hàng và bút toán
//
// `admin.md` mục 6 mô tả trang chi tiết đơn liên kết: đơn → lô giao → bút
// toán → lịch sử thao tác. Nhưng module `order` KHÔNG được gọi `fulfillment`
// hay `payment` — cả hai đã phụ thuộc order, gọi ngược tạo phụ thuộc vòng
// (ADR-0007, archcheck R5).
//
// Việc liên kết là của TRANG, không phải của ENDPOINT: giao diện gọi ba
// endpoint rồi ghép lại. Đó cũng là cách đúng về mặt dữ liệu, vì trạng thái
// lô giao được cập nhật bất đồng bộ qua event.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/order/application"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ các endpoint đơn hàng của quản trị.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth và RequireRole.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/admin/orders", http.HandlerFunc(h.listOrders))
	mux.Handle("GET /api/v1/admin/orders/{order_id}", http.HandlerFunc(h.getOrder))
	mux.Handle("POST /api/v1/admin/orders/{order_id}/cancel",
		http.HandlerFunc(h.cancelOrder))
}

type amountJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// ---------------------------------------------------------------- Danh sách

type orderSummary struct {
	ID          string     `json:"id"`
	OrderNumber string     `json:"order_number"`
	Status      string     `json:"status"`
	Total       amountJSON `json:"total"`
	LineCount   int        `json:"line_count"`
	PlacedAt    string     `json:"placed_at"`
}

type listResponse struct {
	Data []orderSummary `json:"data"`
}

// listOrders phục vụ GET /api/v1/admin/orders (operationId: listAdminOrders).
//
// KHÔNG trả dữ liệu cá nhân khách hàng — tên và số điện thoại chỉ có ở
// endpoint chi tiết, nơi có ghi vết truy cập. Nếu danh sách trả sẵn thông
// tin cá nhân thì mọi lần mở màn hình tìm kiếm là một lần đọc dữ liệu khách
// không có lý do kèm theo.
func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	f := domain.Filter{
		OrderNumber: q.Get("order_number"),
		Status:      q.Get("status"),
	}

	if v := q.Get("customer_id"); v != "" {
		if !ids.IsValid(v) {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"customer_id không đúng định dạng"))
			return
		}
		f.CustomerID = ids.ID(v)
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
		// "đến ngày 31/08" phải bao gồm CẢ NGÀY 31 — nếu không, nhân viên
		// lọc theo tháng mất sạch đơn của ngày cuối tháng mà không biết.
		f.To = f.To.Add(24*time.Hour - time.Nanosecond)
	}

	orders, err := h.svc.ListOrders(r.Context(), f)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	out := make([]orderSummary, 0, len(orders))
	for _, o := range orders {
		out = append(out, orderSummary{
			ID:          o.ID().String(),
			OrderNumber: o.OrderNumber(),
			Status:      string(o.Status()),
			Total: amountJSON{
				Amount:   o.Total().Amount(),
				Currency: string(o.Total().Currency()),
			},
			LineCount: len(o.Lines()),
			PlacedAt:  o.PlacedAt().UTC().Format(time.RFC3339),
		})
	}

	h.ok(w, r, listResponse{Data: out})
}

// ---------------------------------------------------------------- Chi tiết

type orderDetail struct {
	ID          string     `json:"id"`
	OrderNumber string     `json:"order_number"`
	Status      string     `json:"status"`
	CustomerID  string     `json:"customer_id,omitempty"`
	GuestEmail  string     `json:"guest_email,omitempty"`
	Shipping    shipping   `json:"shipping"`
	Lines       []lineJSON `json:"lines"`
	Subtotal    amountJSON `json:"subtotal"`
	ShippingFee amountJSON `json:"shipping_fee"`
	Discount    amountJSON `json:"discount_amount"`
	Total       amountJSON `json:"total"`
	PlacedAt    string     `json:"placed_at"`
	CompletedAt string     `json:"completed_at,omitempty"`
}

// shipping là địa chỉ ĐÓNG BĂNG tại thời điểm đặt hàng.
//
// Đây là dữ liệu cá nhân — lý do endpoint này bắt buộc ghi vết truy cập.
type shipping struct {
	RecipientName string `json:"recipient_name"`
	Phone         string `json:"phone"`
	Street        string `json:"street"`
	Ward          string `json:"ward"`
	District      string `json:"district"`
	Province      string `json:"province"`
}

type lineJSON struct {
	ID           string     `json:"id"`
	SKUID        string     `json:"sku_id"`
	SellerID     string     `json:"seller_id"`
	Quantity     int        `json:"quantity"`
	Status       string     `json:"status"`
	LineageTotal amountJSON `json:"line_total"`
}

// getOrder phục vụ GET /api/v1/admin/orders/{id}
// (operationId: getAdminOrderDetail).
//
// BẮT BUỘC tham số `reason`: response chứa tên người nhận, số điện thoại và
// địa chỉ. Mỗi lần gọi ghi một bản ghi vào nhật ký thao tác.
func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"getOrder chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	id, err := parseOrderID(r.PathValue("order_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}

	o, err := h.svc.ViewOrderAsAdmin(r.Context(), application.ViewOrderInput{
		OrderID:   id,
		ActorID:   ac.UserID,
		Reason:    r.URL.Query().Get("reason"),
		RequestID: logger.RequestIDFromContext(r.Context()),
	})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, toDetail(o))
}

// ---------------------------------------------------------------- Hủy đơn

type cancelRequest struct {
	Reason string `json:"reason"`
}

type cancelResponse struct {
	Order   cancelledOrder `json:"order"`
	Effects cancelEffects  `json:"effects"`
}

type cancelledOrder struct {
	ID          string `json:"id"`
	OrderNumber string `json:"order_number"`
	Status      string `json:"status"`
}

type cancelEffects struct {
	Note string `json:"note"`
}

// cancelOrder phục vụ POST /api/v1/admin/orders/{id}/cancel
// (operationId: cancelAdminOrder).
func (h *Handler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"cancelOrder chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	id, err := parseOrderID(r.PathValue("order_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}

	var req cancelRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	o, err := h.svc.CancelOrderAsAdmin(r.Context(), application.CancelOrderInput{
		OrderID:   id,
		ActorID:   ac.UserID,
		Reason:    req.Reason,
		RequestID: logger.RequestIDFromContext(r.Context()),
	})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, cancelResponse{
		Order: cancelledOrder{
			ID:          o.ID().String(),
			OrderNumber: o.OrderNumber(),
			Status:      string(o.Status()),
		},
		Effects: cancelEffects{
			Note: "Hoàn tiền và nhả tồn kho chạy bất đồng bộ qua event — " +
				"kiểm tra sổ cái và tồn kho sau vài giây",
		},
	})
}

// ---------------------------------------------------------------- Hỗ trợ

func toDetail(o *domain.Order) orderDetail {
	addr := o.ShippingAddress()

	lines := make([]lineJSON, 0, len(o.Lines()))
	for _, l := range o.Lines() {
		lines = append(lines, lineJSON{
			ID:       l.ID().String(),
			SKUID:    l.SKUID().String(),
			SellerID: l.SellerID().String(),
			Quantity: l.Quantity(),
			Status:   string(l.Status()),
			LineageTotal: amountJSON{
				Amount:   l.LineTotal().Amount(),
				Currency: string(l.LineTotal().Currency()),
			},
		})
	}

	out := orderDetail{
		ID:          o.ID().String(),
		OrderNumber: o.OrderNumber(),
		Status:      string(o.Status()),
		CustomerID:  o.CustomerID().String(),
		GuestEmail:  o.GuestEmail(),
		Shipping: shipping{
			RecipientName: addr.RecipientName,
			Phone:         addr.Phone,
			Street:        addr.StreetAddress,
			Ward:          addr.Ward,
			District:      addr.District,
			Province:      addr.Province,
		},
		Lines:       lines,
		Subtotal:    toAmount(o.Subtotal()),
		ShippingFee: toAmount(o.ShippingFee()),
		Discount:    toAmount(o.DiscountAmount()),
		Total:       toAmount(o.Total()),
		PlacedAt:    o.PlacedAt().UTC().Format(time.RFC3339),
	}
	if t := o.CompletedAt(); !t.IsZero() {
		out.CompletedAt = t.UTC().Format(time.RFC3339)
	}
	return out
}

func toAmount(m money.Money) amountJSON {
	return amountJSON{Amount: m.Amount(), Currency: string(m.Currency())}
}

func parseOrderID(raw string) (ids.ID, error) {
	id, err := ids.Parse(raw, ids.PrefixOrder)
	if err != nil {
		return "", apierror.New(apierror.CodeValidationFailed,
			"order_id không đúng định dạng")
	}
	return id, nil
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02", s)
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

func translate(err error) error {
	switch {
	case errors.Is(err, audit.ErrReasonRequired):
		return apierror.New(apierror.CodeValidationFailed,
			"Lý do bắt buộc, tối thiểu 20 ký tự và phải có nội dung thật")
	case errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy đơn hàng")
	case errors.Is(err, domain.ErrNotCancellable):
		return apierror.New(apierror.CodeOrderNotCancellable,
			"Đơn không hủy được với trạng thái hiện tại")
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
