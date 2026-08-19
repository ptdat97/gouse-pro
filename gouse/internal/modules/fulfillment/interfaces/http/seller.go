package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/application"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// SellerHandler phục vụ các endpoint đơn thực hiện của NHÀ BÁN.
//
// # Ranh giới bảo mật quan trọng nhất của module này
//
// Seller CHỈ được thấy đơn thực hiện của chính mình. Ranh giới đó nằm ở
// TRUY VẤN (`WHERE seller_id = $1`), không phải ở tầng hiển thị — lọc khi
// hiển thị nghĩa là dữ liệu seller khác đã rời khỏi database, và chỉ cần
// một lỗi nhỏ là rò rỉ.
//
// Định danh seller lấy từ `AuthContext.SellerIDs` trong token, KHÔNG từ
// tham số. Cho client truyền `seller_id` là để bất kỳ ai cũng đọc được đơn
// của nhà bán khác.
//
// # Seller KHÔNG thấy gì
//
// Mã đơn tổng, tổng tiền cả đơn, các lô khác trong cùng đơn, tên seller
// khác, email khách, lịch sử mua hàng. Xem `SellerFulfillmentOrder` trong
// đặc tả.
type SellerHandler struct {
	svc *application.Service
	log *slog.Logger
}

func NewSellerHandler(svc *application.Service, log *slog.Logger) *SellerHandler {
	return &SellerHandler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth và RequireRole("SELLER_OWNER",
// "SELLER_STAFF").
func (h *SellerHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/seller/fulfillment-orders",
		http.HandlerFunc(h.list))
	mux.Handle("GET /api/v1/seller/fulfillment-orders/{fulfillment_order_id}",
		http.HandlerFunc(h.get))
	mux.Handle("POST /api/v1/seller/fulfillment-orders/{fulfillment_order_id}/ship",
		http.HandlerFunc(h.ship))
}

type foItemJSON struct {
	OrderLineID        string    `json:"order_line_id"`
	SKUID              string    `json:"sku_id"`
	ProductName        string    `json:"product_name"`
	VariantDescription string    `json:"variant_description,omitempty"`
	Quantity           int       `json:"quantity"`
	UnitPrice          moneyJSON `json:"unit_price"`
	LineTotal          moneyJSON `json:"line_total"`
}

type moneyJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type sellerFOJSON struct {
	ID                string `json:"id"`
	FulfillmentNumber string `json:"fulfillment_number"`
	Status            string `json:"status"`

	Items []foItemJSON `json:"items"`

	// Subtotal và SellerPayable là phần CỦA SELLER NÀY, không phải tổng
	// đơn. Seller đối soát phần của mình mà không thấy đơn của người khác.
	Subtotal      moneyJSON `json:"subtotal"`
	Commission    moneyJSON `json:"commission_amount"`
	SellerPayable moneyJSON `json:"seller_payable"`

	ShippingProvider string `json:"shipping_provider,omitempty"`
	TrackingNumber   string `json:"tracking_number,omitempty"`

	CreatedAt   string `json:"created_at"`
	ConfirmedAt string `json:"confirmed_at,omitempty"`
	PackedAt    string `json:"packed_at,omitempty"`
	ShippedAt   string `json:"shipped_at,omitempty"`
	DeliveredAt string `json:"delivered_at,omitempty"`
}

type listFOResponse struct {
	Data []sellerFOJSON `json:"data"`
}

// list phục vụ GET /api/v1/seller/fulfillment-orders
// (operationId: listMyFulfillmentOrders).
func (h *SellerHandler) list(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	q := r.URL.Query()
	var statuses []domain.FOStatus
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		statuses = []domain.FOStatus{domain.FOStatus(s)}
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 1 || n > 200 {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"limit phải là số nguyên từ 1 đến 200"))
			return
		}
		limit = n
	}

	list, err := h.svc.ListSellerWork(r.Context(), sellerID, statuses, limit, 0)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	data := make([]sellerFOJSON, 0, len(list))
	for _, fo := range list {
		data = append(data, toSellerFO(fo))
	}
	h.ok(w, r, http.StatusOK, listFOResponse{Data: data})
}

// get phục vụ GET /api/v1/seller/fulfillment-orders/{id}
// (operationId: getMyFulfillmentOrder).
func (h *SellerHandler) get(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	fo, err := h.svc.GetSellerFulfillment(r.Context(), sellerID,
		ids.ID(r.PathValue("fulfillment_order_id")))
	if err != nil {
		h.fail(w, r, translateSeller(err))
		return
	}
	h.ok(w, r, http.StatusOK, toSellerFO(fo))
}

type shipRequest struct {
	TrackingNumber   string `json:"tracking_number"`
	ShippingProvider string `json:"shipping_provider"`
}

// ship phục vụ POST /api/v1/seller/fulfillment-orders/{id}/ship
// (operationId: shipFulfillmentOrder).
//
// Mã vận đơn BẮT BUỘC: từ đây hàng ra khỏi tầm kiểm soát của seller, và
// không có mã thì không ai trả lời được "hàng của tôi đang ở đâu" — kể cả
// bộ phận hỗ trợ.
func (h *SellerHandler) ship(w http.ResponseWriter, r *http.Request) {
	var req shipRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.TrackingNumber) == "" ||
		strings.TrimSpace(req.ShippingProvider) == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"tracking_number và shipping_provider là trường bắt buộc"))
		return
	}

	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	foID := ids.ID(r.PathValue("fulfillment_order_id"))
	if err := h.svc.RecordHandOver(r.Context(), sellerID, foID,
		strings.TrimSpace(req.ShippingProvider),
		strings.TrimSpace(req.TrackingNumber)); err != nil {
		h.fail(w, r, translateSeller(err))
		return
	}

	// Đọc lại để trả trạng thái MỚI: client cần biết đơn đã chuyển sang
	// bước nào, và tự đoán từ mã trạng thái HTTP là đoán sai sớm muộn.
	fo, err := h.svc.GetSellerFulfillment(r.Context(), sellerID, foID)
	if err != nil {
		h.fail(w, r, translateSeller(err))
		return
	}
	h.ok(w, r, http.StatusOK, toSellerFO(fo))
}

// ---------------------------------------------------------------- Hỗ trợ

// sellerID lấy định danh nhà bán từ TOKEN.
//
// KHÔNG nhận từ tham số hay thân request. Cho client truyền vào nghĩa là
// bất kỳ ai cũng đọc được đơn của nhà bán khác chỉ bằng cách đổi một con số.
//
// Token mang nhiều seller (nhân viên làm cho hai cửa hàng) thì lấy cái đầu.
// Chọn cửa hàng cụ thể là tính năng của giai đoạn sau; lấy cái đầu là hành
// vi xác định, còn gộp dữ liệu nhiều cửa hàng thì không.
func (h *SellerHandler) sellerID(r *http.Request) (ids.ID, error) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"seller fulfillment chạy không qua Auth — kiểm tra nối dây")
		return "", apierror.ErrUnauthorized
	}
	if len(ac.SellerIDs) == 0 {
		return "", apierror.New(apierror.CodeForbidden,
			"Tài khoản này không gắn với nhà bán nào")
	}
	return ids.ID(ac.SellerIDs[0]), nil
}

func (h *SellerHandler) ok(
	w http.ResponseWriter, r *http.Request, status int, body any,
) {
	if err := apierror.WriteJSON(w, status, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *SellerHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

// translateSeller chuyển lỗi domain thành lỗi API.
//
// KHÔNG tìm thấy và KHÔNG PHẢI CỦA MÌNH trả CÙNG một mã: phân biệt hai
// trường hợp là để seller dò được đơn nào tồn tại của nhà bán khác.
func translateSeller(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy đơn thực hiện")

	case errors.Is(err, domain.ErrInvalidStatus):
		// 409 chứ không phải 400: dữ liệu gửi lên đúng, chỉ là đơn đang ở
		// bước không cho phép thao tác này.
		return apierror.New(apierror.CodeConflict,
			"Đơn thực hiện không ở trạng thái cho phép thao tác này")

	default:
		return apierror.From(err)
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

func toMoney(m money.Money) moneyJSON {
	return moneyJSON{Amount: m.Amount(), Currency: string(m.Currency())}
}

func toSellerFO(fo *domain.FulfillmentOrder) sellerFOJSON {
	lines := fo.Lines()
	items := make([]foItemJSON, 0, len(lines))
	for _, l := range lines {
		items = append(items, foItemJSON{
			OrderLineID:        l.OrderLineID.String(),
			SKUID:              l.SKUID.String(),
			ProductName:        l.ProductName,
			VariantDescription: l.VariantDescription,
			Quantity:           l.Quantity,
			UnitPrice:          toMoney(l.UnitPrice),
			LineTotal:          toMoney(l.LineTotal),
		})
	}

	return sellerFOJSON{
		ID:                fo.ID().String(),
		FulfillmentNumber: fo.FONumber(),
		Status:            string(fo.Status()),
		Items:             items,
		Subtotal:          toMoney(fo.Subtotal()),
		Commission:        toMoney(fo.CommissionAmount()),
		SellerPayable:     toMoney(fo.SellerPayable()),
		ShippingProvider:  fo.ShippingProvider(),
		TrackingNumber:    fo.TrackingNumber(),
		CreatedAt:         formatTime(fo.CreatedAt()),
		ConfirmedAt:       formatTime(fo.ConfirmedAt()),
		PackedAt:          formatTime(fo.PackedAt()),
		ShippedAt:         formatTime(fo.ShippedAt()),
		DeliveredAt:       formatTime(fo.DeliveredAt()),
	}
}
