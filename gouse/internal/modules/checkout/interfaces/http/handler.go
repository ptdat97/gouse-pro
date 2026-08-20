// Package http là tầng interfaces của module checkout.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON và mã trạng thái
// lấy TỪ đặc tả api/paths/cart-checkout.yaml.
//
// # Giá ĐÃ ĐÓNG BĂNG khi phiên mở
//
// Response trả giá đã chốt lúc `startCheckout`, không phải giá hiện tại.
// Seller đổi giá giữa chừng thì khách vẫn trả đúng giá đã thấy — đó là
// toàn bộ lý do phiên thanh toán tồn tại.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/modules/checkout/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ các endpoint phiên thanh toán.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/checkout", http.HandlerFunc(h.start))
	mux.Handle("GET /api/v1/checkout/{checkout_id}", http.HandlerFunc(h.get))
	mux.Handle("PATCH /api/v1/checkout/{checkout_id}/shipping-address",
		http.HandlerFunc(h.setAddress))
	mux.Handle("PATCH /api/v1/checkout/{checkout_id}/shipping-method",
		http.HandlerFunc(h.setMethod))
	mux.Handle("POST /api/v1/checkout/{checkout_id}/coupon",
		http.HandlerFunc(h.applyCoupon))
	mux.Handle("POST /api/v1/checkout/{checkout_id}/complete",
		http.HandlerFunc(h.complete))

	// `POST /api/v1/orders` (operationId: placeOrder) là ĐƯỜNG THAY THẾ
	// cho complete ở trên, cho client tự quản lý luồng thanh toán.
	//
	// # Vì sao module checkout phục vụ một đường dẫn của đơn hàng
	//
	// Request nhận `checkout_id` và phải đọc phiên thanh toán để tạo đơn.
	// Module `order` KHÔNG được gọi `checkout` — checkout đã phụ thuộc
	// order, gọi ngược tạo phụ thuộc vòng (ADR-0007, archcheck R5).
	//
	// Đường dẫn thuộc về khái niệm "đơn hàng", nhưng NĂNG LỰC thuộc về
	// checkout. Route đi theo năng lực.
	mux.Handle("POST /api/v1/orders", http.HandlerFunc(h.placeOrder))
}

// ---------------------------------------------------------------- Kiểu JSON

type moneyJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type addressJSON struct {
	RecipientName string `json:"recipient_name"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"street_address"`
	Ward          string `json:"ward,omitempty"`
	District      string `json:"district,omitempty"`
	Province      string `json:"province"`
	PostalCode    string `json:"postal_code,omitempty"`
	CountryCode   string `json:"country_code"`
	AddressType   string `json:"address_type,omitempty"`
	DeliveryNote  string `json:"delivery_note,omitempty"`
}

type lineJSON struct {
	OfferID            string    `json:"offer_id"`
	ProductName        string    `json:"product_name"`
	VariantDescription string    `json:"variant_description,omitempty"`
	UnitPrice          moneyJSON `json:"unit_price"`
	Quantity           int       `json:"quantity"`
	LineTotal          moneyJSON `json:"line_total"`
}

type checkoutJSON struct {
	ID     string     `json:"id"`
	Status string     `json:"status"`
	Lines  []lineJSON `json:"lines"`

	ShippingAddress *addressJSON `json:"shipping_address,omitempty"`

	Subtotal    moneyJSON `json:"subtotal"`
	ShippingFee moneyJSON `json:"shipping_fee"`
	Discount    moneyJSON `json:"discount_amount"`
	Tax         moneyJSON `json:"tax_amount"`
	Total       moneyJSON `json:"total"`

	// ExpiresAt là hạn giữ tồn kho. Hết hạn thì hàng được nhả về kho và
	// khách phải mở phiên mới — giao diện cần đếm ngược cho khách biết.
	ExpiresAt string `json:"expires_at"`
}

// ---------------------------------------------------------------- Mở phiên

type startRequest struct {
	CartID     string `json:"cart_id"`
	GuestEmail string `json:"guest_email,omitempty"`
	GuestPhone string `json:"guest_phone,omitempty"`
}

// start phục vụ POST /api/v1/checkout (operationId: startCheckout).
//
// # `cart_id` được KIỂM CHỨNG chứ không phải tin tưởng
//
// Đặc tả yêu cầu client gửi `cart_id`, nhưng module checkout không biết ai
// đang gọi — nó chỉ nhận một mã giỏ. Gọi thẳng xuống service nghĩa là bất
// kỳ ai đoán được mã giỏ đều mở được phiên thanh toán trên giỏ người khác,
// và thấy toàn bộ nội dung giỏ đó trong response.
//
// Nên handler đối chiếu với giỏ đang dùng của CHÍNH người gọi. Danh tính
// đến từ cookie phiên hoặc token, không từ thân request.
func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.CartID == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"cart_id là trường bắt buộc"))
		return
	}

	own, err := h.activeCartID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if req.CartID != own.String() {
		// 403 chứ không phải 404: nói "không tìm thấy" cho một giỏ có thật
		// là nói dối, và 404 vẫn để lộ mã giỏ nào tồn tại qua chênh lệch
		// thời gian phản hồi. Từ chối thẳng.
		h.fail(w, r, apierror.New(apierror.CodeForbidden,
			"Giỏ hàng này không thuộc về bạn"))
		return
	}

	c, err := h.svc.StartCheckout(r.Context(), application.StartCheckoutInput{
		CartID:     own,
		GuestEmail: req.GuestEmail,
		GuestPhone: req.GuestPhone,
	})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.write(w, r, http.StatusCreated, toJSON(c))
}

// get phục vụ GET /api/v1/checkout/{id} (operationId: getCheckout).
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.GetCheckout(r.Context(), ids.ID(r.PathValue("checkout_id")))
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}
	h.write(w, r, http.StatusOK, toJSON(c))
}

// ---------------------------------------------------------------- Vận chuyển

// setAddress phục vụ PATCH .../shipping-address
// (operationId: setCheckoutShippingAddress).
func (h *Handler) setAddress(w http.ResponseWriter, r *http.Request) {
	var req addressJSON
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	c, err := h.svc.SetShippingAddress(r.Context(),
		ids.ID(r.PathValue("checkout_id")),
		domain.Address{
			RecipientName: req.RecipientName,
			Phone:         req.Phone,
			StreetAddress: req.StreetAddress,
			Ward:          req.Ward,
			District:      req.District,
			Province:      req.Province,
			CountryCode:   req.CountryCode,
		})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.write(w, r, http.StatusOK, toJSON(c))
}

type shippingMethodRequest struct {
	ShippingMethod string `json:"shipping_method"`
}

// setMethod phục vụ PATCH .../shipping-method
// (operationId: setCheckoutShippingMethod).
//
// Client gửi TÊN phương thức, không gửi phí — phí do máy chủ tra
// (application.shippingRates giải thích vì sao).
func (h *Handler) setMethod(w http.ResponseWriter, r *http.Request) {
	var req shippingMethodRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	c, err := h.svc.SetShippingMethod(r.Context(),
		ids.ID(r.PathValue("checkout_id")),
		application.ShippingMethod(req.ShippingMethod))
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.write(w, r, http.StatusOK, toJSON(c))
}

// ---------------------------------------------------------------- Mã giảm giá

type couponRequest struct {
	Code string `json:"code"`
}

// applyCoupon phục vụ POST .../coupon (operationId: applyCheckoutCoupon).
//
// Số tiền giảm do module promotion TÍNH — client chỉ gửi MÃ. Nhận số tiền
// từ client nghĩa là khách tự đặt mức giảm cho mình.
func (h *Handler) applyCoupon(w http.ResponseWriter, r *http.Request) {
	var req couponRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.Code == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"code là trường bắt buộc"))
		return
	}

	var customerID string
	if s, ok := httpserver.ShopperFrom(r.Context()); ok {
		customerID = s.CustomerID
	}

	c, err := h.svc.ApplyCouponCode(r.Context(),
		ids.ID(r.PathValue("checkout_id")), req.Code, customerID)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.write(w, r, http.StatusOK, toJSON(c))
}

// ---------------------------------------------------------------- Hoàn tất

// paymentMethods là các phương thức đặc tả cho phép.
//
// # Giá trị này CHƯA ĐƯỢC LƯU
//
// Module order không có trường phương thức thanh toán, và thêm một trường
// vào domain nằm ngoài phạm vi đợt này (Architecture Freeze). Handler kiểm
// tra giá trị hợp lệ rồi BỎ QUA nó; đơn luôn ở PENDING_PAYMENT và việc thu
// tiền diễn ra ngoài luồng này.
//
// Ghi rõ ở đây thay vì im lặng: một client gửi CARD sẽ không thấy lỗi nào
// nhưng cũng không có gì bị trừ tiền. Backlog P3 theo dõi việc nối payment.
var paymentMethods = map[string]bool{
	"CARD": true, "BANK_TRANSFER": true, "E_WALLET": true, "COD": true,
}

type completeRequest struct {
	PaymentMethod string `json:"payment_method"`

	// PaymentToken do cổng thanh toán cấp. Hệ thống KHÔNG BAO GIỜ nhận số
	// thẻ đầy đủ — và không ghi trường này vào log.
	PaymentToken string `json:"payment_token,omitempty"`
}

type orderSummaryJSON struct {
	ID          string    `json:"id"`
	OrderNumber string    `json:"order_number"`
	Status      string    `json:"status"`
	Total       moneyJSON `json:"total"`
	ItemCount   int       `json:"item_count"`
	PlacedAt    string    `json:"placed_at"`
}

type completeResponse struct {
	Order orderSummaryJSON `json:"order"`

	// PaymentRedirectURL có mặt khi phương thức thanh toán cần chuyển
	// hướng. Luôn null cho tới khi nối cổng thanh toán.
	PaymentRedirectURL *string `json:"payment_redirect_url"`
}

// complete phục vụ POST .../complete (operationId: completeCheckout).
//
// BẮT BUỘC `Idempotency-Key`: khách bấm "Đặt hàng" hai lần, hoặc client thử
// lại sau timeout — hai đơn nghĩa là khách bị trừ tiền hai lần.
func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	var req completeRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if !paymentMethods[req.PaymentMethod] {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"payment_method không hợp lệ"))
		return
	}

	key, ok := httpserver.IdempotencyKeyFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"complete chạy không qua RequireIdempotencyKey — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrInternal)
		return
	}

	res, err := h.svc.CompleteCheckout(r.Context(),
		ids.ID(r.PathValue("checkout_id")), key)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	c := res.Checkout
	h.write(w, r, http.StatusCreated, completeResponse{
		Order: orderSummaryJSON{
			ID:          res.OrderID.String(),
			OrderNumber: res.OrderNumber,
			// Đơn vừa tạo LUÔN ở PENDING_PAYMENT (order.PlaceOrder đặt
			// trạng thái này); thu tiền là một bước riêng.
			Status:    "PENDING_PAYMENT",
			Total:     toMoney(c.Total()),
			ItemCount: len(c.Lines()),
			PlacedAt:  c.UpdatedAt().UTC().Format(time.RFC3339),
		},
	})
}

type placeOrderRequest struct {
	CheckoutID string `json:"checkout_id"`
}

// placeOrder phục vụ POST /api/v1/orders (operationId: placeOrder).
//
// Cùng một việc với `complete`, khác ở chỗ mã phiên nằm trong THÂN request
// thay vì đường dẫn, và không nhận phương thức thanh toán.
//
// Luồng khuyến nghị vẫn là qua checkout: nó đảm bảo giữ tồn kho và đóng
// băng giá. Đường này tồn tại cho client tự quản lý bước thanh toán.
func (h *Handler) placeOrder(w http.ResponseWriter, r *http.Request) {
	var req placeOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.CheckoutID == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"checkout_id là trường bắt buộc"))
		return
	}

	key, ok := httpserver.IdempotencyKeyFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"placeOrder chạy không qua RequireIdempotencyKey — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrInternal)
		return
	}

	res, err := h.svc.CompleteCheckout(r.Context(), ids.ID(req.CheckoutID), key)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	c := res.Checkout
	h.write(w, r, http.StatusCreated, completeResponse{
		Order: orderSummaryJSON{
			ID:          res.OrderID.String(),
			OrderNumber: res.OrderNumber,
			Status:      "PENDING_PAYMENT",
			Total:       toMoney(c.Total()),
			ItemCount:   len(c.Lines()),
			PlacedAt:    c.UpdatedAt().UTC().Format(time.RFC3339),
		},
	})
}

// ---------------------------------------------------------------- Hỗ trợ

// activeCartID tìm giỏ đang dùng của người gọi.
func (h *Handler) activeCartID(r *http.Request) (ids.ID, error) {
	s, ok := httpserver.ShopperFrom(r.Context())
	if !ok {
		// Middleware ResolveShopper chưa được nối. Thất bại theo hướng an
		// toàn thay vì cho qua với danh tính rỗng.
		h.log.ErrorContext(r.Context(),
			"checkout chạy không qua ResolveShopper — kiểm tra nối dây")
		return "", apierror.ErrInternal
	}
	if s.CustomerID == "" && s.SessionID == "" {
		return "", apierror.New(apierror.CodeValidationFailed,
			"Không xác định được phiên mua hàng")
	}

	cartID, err := h.svc.ActiveCartID(r.Context(), s.CustomerID, s.SessionID)
	if err != nil {
		return "", translate(err)
	}
	return cartID, nil
}

func (h *Handler) write(w http.ResponseWriter, r *http.Request, status int, body any) {
	if err := apierror.WriteJSON(w, status, body); err != nil {
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

func translate(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy phiên thanh toán")

	case errors.Is(err, domain.ErrExpired):
		// 409 chứ không phải 400: dữ liệu client gửi lên đúng, chỉ là phiên
		// đã hết hạn và hàng đã được nhả về kho.
		return apierror.New(apierror.CodeCheckoutExpired,
			"Phiên thanh toán đã hết hạn, vui lòng thử lại")

	case errors.Is(err, domain.ErrNoAddress):
		return apierror.New(apierror.CodeValidationFailed,
			"Phải có địa chỉ giao hàng trước khi đặt đơn")

	case errors.Is(err, application.ErrUnknownShippingMethod):
		return apierror.New(apierror.CodeValidationFailed,
			"Phương thức vận chuyển không hợp lệ")

	case errors.Is(err, application.ErrEmptyCart),
		errors.Is(err, domain.ErrNoLines):
		return apierror.New(apierror.CodeValidationFailed,
			"Giỏ hàng không có món nào mua được")

	// Thiếu danh tính người mua là lỗi NHẬP LIỆU, không phải lỗi máy chủ.
	//
	// Trước đây nhánh này không tồn tại nên nó rơi xuống `default` và trả
	// 500 kèm "Đã có lỗi xảy ra, vui lòng thử lại" — vừa giấu nguyên nhân,
	// vừa bảo người dùng thử lại một việc không bao giờ thành công. Thông
	// điệp phải nói RA thiếu gì.
	case errors.Is(err, domain.ErrNoCustomer):
		return apierror.New(apierror.CodeValidationFailed,
			"Cần email để gửi xác nhận đơn — đăng nhập hoặc điền email")

	case errors.Is(err, domain.ErrMissingIdemKey):
		return apierror.New(apierror.CodeValidationFailed,
			"Thiếu header Idempotency-Key")

	case errors.Is(err, application.ErrOutOfStock):
		return apierror.New(apierror.CodeInsufficientInventory,
			"Một số sản phẩm không đủ số lượng")

	case errors.Is(err, application.ErrPromotionUnavailable):
		return apierror.New(apierror.CodeValidationFailed,
			"Tính năng mã giảm giá chưa sẵn sàng")

	default:
		return apierror.From(err)
	}
}

func toMoney(m money.Money) moneyJSON {
	return moneyJSON{Amount: m.Amount(), Currency: string(m.Currency())}
}

func toJSON(c *domain.Checkout) checkoutJSON {
	lines := c.Lines()
	out := checkoutJSON{
		ID:     c.ID().String(),
		Status: string(c.Status()),
		// Khởi tạo rỗng chứ không để nil: `lines` là trường BẮT BUỘC của
		// đặc tả, và `null` không phải một mảng.
		Lines:       make([]lineJSON, 0, len(lines)),
		Subtotal:    toMoney(c.Subtotal()),
		ShippingFee: toMoney(c.ShippingFee()),
		Discount:    toMoney(c.DiscountAmount()),
		Tax:         toMoney(c.TaxAmount()),
		Total:       toMoney(c.Total()),
		ExpiresAt:   c.ExpiresAt().UTC().Format(time.RFC3339),
	}

	for _, l := range lines {
		out.Lines = append(out.Lines, lineJSON{
			OfferID:            l.OfferID().String(),
			ProductName:        l.ProductName(),
			VariantDescription: l.VariantDescription(),
			UnitPrice:          toMoney(l.UnitPrice()),
			Quantity:           l.Quantity(),
			LineTotal:          toMoney(l.LineTotal()),
		})
	}

	if addr := c.ShippingAddress(); !addr.IsEmpty() {
		out.ShippingAddress = &addressJSON{
			RecipientName: addr.RecipientName,
			Phone:         addr.Phone,
			StreetAddress: addr.StreetAddress,
			Ward:          addr.Ward,
			District:      addr.District,
			Province:      addr.Province,
			CountryCode:   addr.CountryCode,
		}
	}

	return out
}
