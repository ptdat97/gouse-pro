// Package http là tầng interfaces của module marketplace.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/storefront.yaml.
package http

import (
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ các endpoint offer công khai.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/products/{product_id}/offers",
		http.HandlerFunc(h.listProductOffers))
}

type moneyJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type offerJSON struct {
	ID       string `json:"id"`
	SKUID    string `json:"sku_id"`
	SellerID string `json:"seller_id"`

	Price          moneyJSON  `json:"price"`
	CompareAtPrice *moneyJSON `json:"compare_at_price,omitempty"`

	Condition         string `json:"condition"`
	HandlingTimeHours int    `json:"handling_time_hours"`

	// IsBuyBox: offer được chọn mặc định khi khách bấm "Thêm vào giỏ".
	IsBuyBox bool `json:"is_buy_box"`

	// IsSellable KHÁC với việc offer có hiển thị hay không.
	//
	// Offer hết hàng VẪN hiện (để khách đăng ký nhận thông báo) nhưng
	// `is_sellable: false`. Giao diện dùng cờ này để gạch chéo thay vì ẩn.
	IsSellable bool   `json:"is_sellable"`
	Status     string `json:"status"`
}

type listResponse struct {
	Data []offerJSON `json:"data"`
}

// listProductOffers phục vụ GET /api/v1/products/{id}/offers
// (operationId: listProductOffers).
//
// Khách so sánh được tổng chi phí và chất lượng phục vụ, không chỉ giá —
// nên response giữ cả thời gian xử lý và tình trạng hàng.
func (h *Handler) listProductOffers(w http.ResponseWriter, r *http.Request) {
	productID, err := ids.Parse(r.PathValue("product_id"), ids.PrefixProduct)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"product_id không đúng định dạng"))
		return
	}

	var skuID ids.ID
	if raw := r.URL.Query().Get("sku_id"); raw != "" {
		id, err := ids.Parse(raw, ids.PrefixSKU)
		if err != nil {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"sku_id không đúng định dạng"))
			return
		}
		skuID = id
	}

	offers, err := h.svc.ListProductOffers(r.Context(), productID, skuID)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	out := make([]offerJSON, 0, len(offers))
	for _, po := range offers {
		o := po.Offer
		item := offerJSON{
			ID:       o.ID().String(),
			SKUID:    o.SKUID().String(),
			SellerID: o.SellerID().String(),
			Price: moneyJSON{
				Amount:   o.Price().Amount(),
				Currency: string(o.Price().Currency()),
			},
			Condition:         string(o.Condition()),
			HandlingTimeHours: o.HandlingTimeHours(),
			IsBuyBox:          po.IsBuyBox,
			IsSellable:        o.IsSellable(),
			Status:            string(o.Status()),
		}

		// Giá gạch ngang chỉ hiện khi CÓ giảm giá thật. Hiện một mức giá
		// gạch ngang bằng đúng giá bán là quảng cáo sai lệch.
		if cmp := o.CompareAt(); cmp.IsPositive() {
			item.CompareAtPrice = &moneyJSON{
				Amount:   cmp.Amount(),
				Currency: string(cmp.Currency()),
			}
		}

		out = append(out, item)
	}

	h.ok(w, r, listResponse{Data: out})
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
