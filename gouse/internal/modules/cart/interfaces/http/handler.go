// Package http là tầng interfaces của module cart.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/cart-checkout.yaml và api/components/schemas.yaml#/Cart.
//
// # Khách vãng lai mua được
//
// Mọi endpoint ở đây chạy cho cả khách đã đăng nhập lẫn khách vãng lai —
// đó là yêu cầu MVP (mvp.md mục 4). Danh tính lấy từ `httpserver.Shopper`:
// đã đăng nhập thì có `CustomerID`, chưa thì dùng `SessionID` từ cookie.
//
// # Giỏ KHÔNG nhận giá từ client
//
// `addItemRequest` không có trường giá — giá được TRA từ marketplace. Nhận
// giá từ client nghĩa là khách tự đặt giá cho mình.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/cart/application"
	"github.com/fashion-commerce/platform/internal/modules/cart/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ các endpoint giỏ hàng.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc `ResolveShopper` — không có nó thì handler
// không biết giỏ thuộc về ai.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/cart", http.HandlerFunc(h.getCart))
	mux.Handle("POST /api/v1/cart/items", http.HandlerFunc(h.addItem))
	mux.Handle("PATCH /api/v1/cart/items/{cart_item_id}", http.HandlerFunc(h.updateItem))
	mux.Handle("DELETE /api/v1/cart/items/{cart_item_id}", http.HandlerFunc(h.removeItem))
	mux.Handle("POST /api/v1/cart/merge", http.HandlerFunc(h.merge))
}

// ---------------------------------------------------------------- Kiểu JSON

type moneyJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type sellerRefJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type itemJSON struct {
	ID                 string    `json:"id"`
	OfferID            string    `json:"offer_id"`
	ProductName        string    `json:"product_name"`
	VariantDescription string    `json:"variant_description,omitempty"`
	ImageURL           string    `json:"image_url,omitempty"`
	UnitPrice          moneyJSON `json:"unit_price"`
	Quantity           int       `json:"quantity"`
	LineTotal          moneyJSON `json:"line_total"`

	// Availability ĐÁNH DẤU món không mua được thay vì xóa nó đi: xóa im
	// lặng làm khách bối rối và mất luôn tín hiệu nhu cầu.
	Availability string `json:"availability"`
}

type groupJSON struct {
	Seller   sellerRefJSON `json:"seller"`
	Items    []itemJSON    `json:"items"`
	Subtotal moneyJSON     `json:"subtotal"`
}

type cartJSON struct {
	ID string `json:"id"`

	// Groups nhóm theo seller vì khách cần hiểu hàng đến từ đâu và thời
	// gian giao khác nhau. Giỏ KHÔNG chia theo seller ở tầng dữ liệu —
	// việc nhóm chỉ xảy ra ở đây, lúc dựng response.
	Groups []groupJSON `json:"groups"`

	// Subtotal tính từ các món MUA ĐƯỢC, theo GIÁ HIỆN TẠI.
	Subtotal moneyJSON `json:"subtotal"`
	Total    moneyJSON `json:"total"`

	ExpiresAt string `json:"expires_at,omitempty"`
}

// cartResponse là bao ngoài của mọi response giỏ hàng.
//
// Đặc tả bọc giỏ trong `{"cart": ...}` thay vì trả thẳng object. Giữ đúng
// hình dạng đó cho phép thêm trường anh em sau này (ví dụ `warnings` khi
// gộp giỏ lúc đăng nhập) mà không phá client đang chạy.
type cartResponse struct {
	Cart cartJSON `json:"cart"`
}

// ---------------------------------------------------------------- Endpoint

// getCart phục vụ GET /api/v1/cart (operationId: getCart).
//
// Tạo giỏ nếu chưa có: khách mở trang giỏ hàng lần đầu phải thấy giỏ rỗng,
// không phải lỗi 404.
func (h *Handler) getCart(w http.ResponseWriter, r *http.Request) {
	c, err := h.cartFor(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	// ĐỒNG BỘ trước khi trả: giá và tình trạng hàng phải là của HIỆN TẠI.
	// Bỏ bước này thì khách thấy giá cũ ở giỏ rồi bị tính giá khác ở bước
	// thanh toán — đúng thứ module này hứa sẽ không xảy ra.
	if err := h.svc.Sync(r.Context(), c); err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, c)
}

type attributionJSON struct {
	ContentID string `json:"content_id,omitempty"`
	CreatorID string `json:"creator_id,omitempty"`
}

type addItemRequest struct {
	OfferID  string `json:"offer_id"`
	Quantity int    `json:"quantity"`

	// Source ghi ngay lúc THÊM GIỎ, không đợi lúc mua — tín hiệu ý định
	// mua mạnh hơn lượt xem, và quy kết đúng khi khách mua sau vài ngày.
	Source *attributionJSON `json:"source,omitempty"`
}

// addItem phục vụ POST /api/v1/cart/items (operationId: addCartItem).
func (h *Handler) addItem(w http.ResponseWriter, r *http.Request) {
	var req addItemRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.OfferID == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"offer_id là trường bắt buộc"))
		return
	}
	if req.Quantity <= 0 {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"quantity phải lớn hơn 0"))
		return
	}

	c, err := h.cartFor(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	in := application.AddItemInput{
		CartID:   c.ID(),
		OfferID:  parseID(req.OfferID),
		Quantity: req.Quantity,
	}
	if req.Source != nil {
		in.SourceContentID = parseID(req.Source.ContentID)
		in.SourceCreatorID = parseID(req.Source.CreatorID)
	}

	updated, err := h.svc.AddItem(r.Context(), in)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, updated)
}

type updateItemRequest struct {
	Quantity int `json:"quantity"`
}

// updateItem phục vụ PATCH /api/v1/cart/items/{cart_item_id}
// (operationId: updateCartItem).
func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	var req updateItemRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	c, err := h.cartFor(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	updated, err := h.svc.UpdateQuantity(r.Context(),
		c.ID(), parseID(r.PathValue("cart_item_id")), req.Quantity)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, updated)
}

// removeItem phục vụ DELETE /api/v1/cart/items/{cart_item_id}
// (operationId: removeCartItem).
func (h *Handler) removeItem(w http.ResponseWriter, r *http.Request) {
	c, err := h.cartFor(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	updated, err := h.svc.RemoveItem(r.Context(),
		c.ID(), parseID(r.PathValue("cart_item_id")))
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	// RemoveItem không tự đồng bộ (khác AddItem và UpdateQuantity), nên
	// đồng bộ ở đây để response cuối cùng cũng mang giá hiện tại.
	if err := h.svc.Sync(r.Context(), updated); err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, updated)
}

// ---------------------------------------------------------------- Gộp giỏ

type mergeWarningJSON struct {
	OfferID     string `json:"offer_id"`
	ProductName string `json:"product_name"`
	Reason      string `json:"reason"`
	WantedQty   int    `json:"wanted_quantity"`
	ActualQty   int    `json:"actual_quantity"`
}

type mergeResponse struct {
	Cart cartJSON `json:"cart"`

	// Warnings PHẢI được giao diện hiển thị.
	//
	// Im lặng bỏ qua nghĩa là khách đăng nhập xong thấy giỏ ít hàng hơn
	// lúc chưa đăng nhập mà không hiểu vì sao — trải nghiệm tệ nhất của
	// luồng này.
	Warnings []mergeWarningJSON `json:"warnings"`
}

// merge phục vụ POST /api/v1/cart/merge (operationId: mergeCartOnLogin).
//
// # KHÔNG nhận tham số nào
//
// Cả hai định danh cần thiết đều đã nằm trong context sau khi qua
// ResolveShopper: `CustomerID` từ token vừa nhận, `SessionID` từ cookie
// phiên vãng lai. Cho client truyền vào nghĩa là ai cũng gộp được giỏ của
// phiên người khác vào tài khoản mình — và đọc được toàn bộ nội dung.
//
// Gọi khi nào: NGAY SAU khi đăng nhập thành công. Gọi lúc khác vẫn an
// toàn — không có giỏ phiên thì nó chỉ trả giỏ tài khoản.
func (h *Handler) merge(w http.ResponseWriter, r *http.Request) {
	s, ok := httpserver.ShopperFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"cart merge chạy không qua ResolveShopper — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrInternal)
		return
	}
	if s.CustomerID == "" {
		h.fail(w, r, apierror.New(apierror.CodeUnauthorized,
			"Cần đăng nhập để gộp giỏ hàng"))
		return
	}

	res, err := h.svc.MergeOnLogin(r.Context(), parseID(s.CustomerID), s.SessionID)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	warnings := make([]mergeWarningJSON, 0, len(res.Warnings))
	for _, wn := range res.Warnings {
		warnings = append(warnings, mergeWarningJSON{
			OfferID:     wn.OfferID.String(),
			ProductName: wn.ProductName,
			Reason:      string(wn.Reason),
			WantedQty:   wn.WantedQty,
			ActualQty:   wn.ActualQty,
		})
	}

	if err := apierror.WriteJSON(w, http.StatusOK, mergeResponse{
		Cart:     toJSON(res.Cart),
		Warnings: warnings,
	}); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

// ---------------------------------------------------------------- Hỗ trợ

// cartFor tìm giỏ của người đang gọi, tạo mới nếu chưa có.
//
// # Giỏ gắn với NGƯỜI, không với tham số từ client
//
// Định danh lấy từ context, KHÔNG từ query hay body. Cho client truyền
// `cart_id` nghĩa là ai cũng đọc và sửa được giỏ của người khác chỉ bằng
// cách đoán định danh.
func (h *Handler) cartFor(r *http.Request) (*domain.Cart, error) {
	s, ok := httpserver.ShopperFrom(r.Context())
	if !ok {
		// Middleware ResolveShopper chưa được nối. Thất bại theo hướng an
		// toàn thay vì gán giỏ cho một danh tính rỗng.
		h.log.ErrorContext(r.Context(),
			"cart chạy không qua ResolveShopper — kiểm tra nối dây")
		return nil, apierror.ErrInternal
	}
	if s.CustomerID == "" && s.SessionID == "" {
		return nil, apierror.New(apierror.CodeValidationFailed,
			"Không xác định được phiên mua hàng")
	}

	return h.svc.GetOrCreateCart(r.Context(), application.GetOrCreateInput{
		CustomerID: parseID(s.CustomerID),
		SessionID:  s.SessionID,
		Currency:   money.VND,
	})
}

func (h *Handler) ok(w http.ResponseWriter, r *http.Request, c *domain.Cart) {
	if err := apierror.WriteJSON(w, http.StatusOK, cartResponse{Cart: toJSON(c)}); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
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
func translate(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy giỏ hàng hoặc món hàng")
	case errors.Is(err, domain.ErrInvalidQty),
		errors.Is(err, domain.ErrQtyBelowMin),
		errors.Is(err, domain.ErrQtyAboveMax):
		// Trả nguyên thông báo của domain: "dưới mức tối thiểu của offer"
		// nói rõ phải làm gì, khác hẳn "số lượng không hợp lệ".
		return apierror.New(apierror.CodeValidationFailed, err.Error())

	case errors.Is(err, domain.ErrCartNotActive):
		return apierror.New(apierror.CodeConflict, "Giỏ hàng không còn hoạt động")
	default:
		return apierror.From(err)
	}
}

// ---------------------------------------------------------------- Chuyển đổi

// parseID chuyển chuỗi thành định danh, KHÔNG kiểm tra tiền tố.
//
// Kiểm tra thật nằm ở tầng application và domain — chúng biết loại định
// danh nào hợp lệ ở đâu. Kiểm tra hai nơi nghĩa là sớm muộn hai nơi lệch.
func parseID(s string) ids.ID { return ids.ID(s) }

func toMoney(m money.Money) moneyJSON {
	return moneyJSON{Amount: m.Amount(), Currency: string(m.Currency())}
}

func toJSON(c *domain.Cart) cartJSON {
	out := cartJSON{
		ID: c.ID().String(),
		// Khởi tạo rỗng chứ không để nil: `groups` là trường BẮT BUỘC của
		// đặc tả, và `null` không phải một mảng.
		Groups:   make([]groupJSON, 0),
		Subtotal: toMoney(c.Subtotal()),
		// Total bằng Subtotal ở tầng giỏ: phí ship và giảm giá chỉ có con
		// số thật ở phiên thanh toán, khi đã biết địa chỉ giao.
		Total: toMoney(c.Subtotal()),
	}
	if exp := c.ExpiresAt(); !exp.IsZero() {
		out.ExpiresAt = exp.UTC().Format(time.RFC3339)
	}

	out.Groups = groupBySeller(c)
	return out
}

// groupBySeller nhóm các món theo nhà bán, GIỮ THỨ TỰ ỔN ĐỊNH.
//
// Thứ tự ổn định là yêu cầu thật, không phải sự cầu kỳ: duyệt map trong Go
// trả thứ tự ngẫu nhiên, nên không sắp xếp thì các nhóm nhảy chỗ mỗi lần
// khách tải lại trang.
func groupBySeller(c *domain.Cart) []groupJSON {
	type bucket struct {
		seller sellerRefJSON
		items  []itemJSON
		total  int64
	}

	order := make([]string, 0)
	buckets := make(map[string]*bucket)

	for _, it := range c.Items() {
		key := it.SellerID().String()
		b, ok := buckets[key]
		if !ok {
			b = &bucket{seller: sellerRefJSON{ID: key, Name: it.SellerName()}}
			buckets[key] = b
			order = append(order, key)
		}
		// Tên tra được ở món nào thì dùng món đó: giỏ cũ chưa đồng bộ lại
		// có thể có món thiếu tên.
		if b.seller.Name == "" {
			b.seller.Name = it.SellerName()
		}

		b.items = append(b.items, itemJSON{
			ID:                 it.ID().String(),
			OfferID:            it.OfferID().String(),
			ProductName:        it.ProductName(),
			VariantDescription: it.VariantDescription(),
			ImageURL:           it.ImageURL(),
			UnitPrice:          toMoney(it.UnitPrice()),
			Quantity:           it.Quantity(),
			LineTotal:          toMoney(it.LineTotal()),
			Availability:       availabilityJSON(it.Availability()),
		})

		// Chỉ cộng món MUA ĐƯỢC, giống hệt cách domain tính Subtotal của
		// giỏ. Cộng cả món hết hàng thì tổng các nhóm không khớp tổng giỏ.
		if it.Availability().IsPurchasable() {
			b.total += it.LineTotal().Amount()
		}
	}

	sort.Strings(order)

	out := make([]groupJSON, 0, len(order))
	for _, key := range order {
		b := buckets[key]
		out = append(out, groupJSON{
			Seller:   b.seller,
			Items:    b.items,
			Subtotal: moneyJSON{Amount: b.total, Currency: string(c.Currency())},
		})
	}
	return out
}

// availabilityJSON chuyển trạng thái của domain sang từ vựng của đặc tả.
//
// Hai bộ từ vựng KHÔNG trùng nhau và đó là chủ ý: domain nói về nguyên
// nhân (QUANTITY_REDUCED — không đủ số lượng khách yêu cầu), API nói về
// thứ khách thấy (LOW_STOCK — sắp hết).
func availabilityJSON(a domain.ItemAvailability) string {
	switch a {
	case domain.AvailabilityOutOfStock:
		return "OUT_OF_STOCK"
	case domain.AvailabilityUnavailable:
		return "UNAVAILABLE"
	case domain.AvailabilityQuantityReduced:
		return "LOW_STOCK"
	default:
		return "IN_STOCK"
	}
}
