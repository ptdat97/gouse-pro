// Package http là tầng interfaces của module marketplace.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/storefront.yaml.
package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

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
	// Giá buy box theo LÔ, cho trang danh sách.
	mux.Handle("GET /api/v1/offers/buy-box",
		http.HandlerFunc(h.listBuyBoxPrices))

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
			IsSellable:        po.IsSellable,
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

// maxBuyBoxProducts chặn một lời gọi kéo cả danh mục.
//
// 100 rộng hơn nhiều so với một trang danh sách (24 món), đủ hẹp để
// endpoint công khai không thành công cụ trích xuất bảng giá.
const maxBuyBoxProducts = 100

type buyBoxJSON struct {
	ProductID string     `json:"product_id"`
	PriceFrom moneyJSON  `json:"price_from"`
	CompareAt *moneyJSON `json:"compare_at_price,omitempty"`
}

type buyBoxResponse struct {
	Data []buyBoxJSON `json:"data"`
}

// listBuyBoxPrices phục vụ GET /api/v1/offers/buy-box?product_ids=...
// (operationId: listBuyBoxPrices).
//
// # Vì sao là endpoint riêng chứ không nhét vào ProductSummary
//
// Giá thuộc về OFFER. Module product nằm cùng tầng nên không gọi được
// marketplace, và nhồi giá vào danh mục sẽ bắt MỌI lời gọi sản phẩm kéo
// theo truy vấn giá — kể cả trang quản trị, nơi không hiển thị giá bán.
//
// Trang tự ghép hai nguồn, cùng mẫu với việc tra tên nhà bán.
//
// # Vì sao lấy từ BUY BOX chứ không phải min của mọi offer
//
// Buy box đã loại offer hết hàng và nhà bán bị đình chỉ, nên con số trả về
// là giá khách THẬT SỰ mua được. Lấy min trên mọi offer sẽ quảng cáo một
// mức giá không đặt được — khách bấm vào rồi thấy giá khác.
//
// Sản phẩm không có offer nào bán được thì VẮNG MẶT, không trả giá 0: giá
// 0 hiển thị ra là "miễn phí".
func (h *Handler) listBuyBoxPrices(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("product_ids"))
	if raw == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"product_ids là tham số bắt buộc"))
		return
	}

	parts := strings.Split(raw, ",")
	if len(parts) > maxBuyBoxProducts {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			fmt.Sprintf("tối đa %d sản phẩm mỗi lần gọi", maxBuyBoxProducts)))
		return
	}

	// Mã hỏng bị BỎ QUA, không làm hỏng cả lời gọi: trang đang hiển thị
	// một danh sách.
	parsed := make([]ids.ID, 0, len(parts))
	seen := make(map[ids.ID]bool, len(parts))
	for _, p := range parts {
		id, err := ids.Parse(strings.TrimSpace(p), ids.PrefixProduct)
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		parsed = append(parsed, id)
	}

	ranges, err := h.svc.GetPriceRanges(r.Context(), parsed)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	// GIỮ ĐÚNG THỨ TỰ hỏi: bên gọi ghép kết quả vào danh sách đã sắp xếp,
	// và thứ tự map trong Go là ngẫu nhiên.
	data := make([]buyBoxJSON, 0, len(parsed))
	for _, id := range parsed {
		pr, ok := ranges[id]
		if !ok {
			continue
		}
		item := buyBoxJSON{
			ProductID: id.String(),
			PriceFrom: moneyJSON{
				Amount:   pr.From.Amount(),
				Currency: string(pr.From.Currency()),
			},
		}
		if pr.CompareAt.IsPositive() {
			item.CompareAt = &moneyJSON{
				Amount:   pr.CompareAt.Amount(),
				Currency: string(pr.CompareAt.Currency()),
			}
		}
		data = append(data, item)
	}

	h.ok(w, r, buyBoxResponse{Data: data})
}
