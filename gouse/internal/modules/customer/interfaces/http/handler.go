// Package http là tầng interfaces của module customer.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/account.yaml.
//
// # Mọi endpoint ở đây ĐỀU cần đăng nhập
//
// Khác cart và checkout: giỏ hàng phải chạy cho khách vãng lai, nhưng hồ sơ
// và sổ địa chỉ thì không có nghĩa gì nếu không có tài khoản. Khách vãng
// lai nhận 401, không phải hồ sơ rỗng.
//
// # Phạm vi dữ liệu do NGƯỜI GỌI quyết định
//
// Không endpoint nào nhận `customer_id`. Định danh lấy từ context — cho
// client truyền vào nghĩa là ai cũng đọc được sổ địa chỉ của người khác.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/customer/application"
	"github.com/fashion-commerce/platform/internal/modules/customer/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	"github.com/fashion-commerce/platform/internal/platform/privacy"
)

// Handler phục vụ các endpoint tài khoản khách hàng.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc `ResolveShopper`.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/me", http.HandlerFunc(h.getProfile))
	mux.Handle("PATCH /api/v1/me", http.HandlerFunc(h.updateProfile))
	mux.Handle("GET /api/v1/me/addresses", http.HandlerFunc(h.listAddresses))
	mux.Handle("POST /api/v1/me/addresses", http.HandlerFunc(h.addAddress))
	mux.Handle("GET /api/v1/me/wishlist", http.HandlerFunc(h.getWishlist))
	mux.Handle("POST /api/v1/me/wishlist", http.HandlerFunc(h.addWishlistItem))
}

// ---------------------------------------------------------------- Hồ sơ

type profileJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`

	// Tier là hạng khách hàng. Đặc tả gọi là `tier`, module gọi là
	// `Status` — cùng một tập giá trị, chỉ khác tên.
	Tier string `json:"tier"`

	// MarketingConsent là hai cờ khách bật/tắt thư khuyến mãi.
	//
	// Nằm ở ĐÂY, không nằm trong `preferences`. Đặc tả từng gộp chúng
	// chung khối với số đo cơ thể chỉ vì cả hai là "tùy chọn của khách",
	// nhưng hai thứ đó khác hẳn: số đo là dữ liệu nhạy cảm cần mã hóa khi
	// lưu (P3-14, chưa có chỗ), còn đây là hai giá trị boolean.
	//
	// Gộp chúng nghĩa là hàng rào đồng ý ở `notification` đứng đó mà
	// không ai cho đồng ý được, và tính năng marketing đầu tiên sẽ gửi
	// đúng luật mà gửi được KHÔNG lá thư nào.
	MarketingConsent marketingConsentJSON `json:"marketing_consent"`
}

// marketingConsentJSON khớp `marketing_consent` của account.yaml.
//
// KHÔNG có `push`: `customer` chỉ quản lý hai loại đồng ý
// MARKETING_EMAIL và MARKETING_SMS. Trả thêm một cờ `push` luôn bằng
// false sẽ là lời hứa không ai giữ — khách tắt nó cũng không đổi gì.
type marketingConsentJSON struct {
	Email bool `json:"email"`
	SMS   bool `json:"sms"`
}

type profileResponse struct {
	Customer profileJSON `json:"customer"`
}

// getProfile phục vụ GET /api/v1/me (operationId: getMyProfile).
//
// # `preferences` KHÔNG có trong response
//
// Đặc tả khai báo số đo cơ thể và size ưa thích, kèm yêu cầu bảo mật của
// chính nó: "mã hóa khi lưu, không đưa vào analytics". Module customer
// chưa có chỗ lưu chúng, và thêm một bảng lưu số đo dạng thô sẽ VI PHẠM
// đúng yêu cầu đó.
//
// Trả về thiếu một trường tùy chọn thì giao diện xử lý được; lưu dữ liệu
// nhạy cảm sai cách thì không sửa ngược được. Xem backlog P3-14.
func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	id, err := h.customerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	v, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	out := toProfile(v)
	dongY, err := h.dongYMarketing(r, id)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}
	out.MarketingConsent = dongY

	h.ok(w, r, http.StatusOK, profileResponse{Customer: out})
}

// dongYMarketing đọc hai cờ đồng ý hiện tại của khách.
//
// Lỗi thì TRẢ LỖI, không trả false: false nghĩa là "khách đã từ chối", và
// nói vậy khi thật ra không đọc được sẽ khiến khách thấy ô đã tắt rồi bật
// lại — ghi đè một lần từ chối có thật bằng một lần đọc hỏng.
func (h *Handler) dongYMarketing(
	r *http.Request, id ids.ID,
) (marketingConsentJSON, error) {
	email, err := h.svc.HasConsent(r.Context(), id, domain.ConsentMarketingEmail)
	if err != nil {
		return marketingConsentJSON{}, err
	}
	sms, err := h.svc.HasConsent(r.Context(), id, domain.ConsentMarketingSMS)
	if err != nil {
		return marketingConsentJSON{}, err
	}
	return marketingConsentJSON{Email: email, SMS: sms}, nil
}

type updateProfileRequest struct {
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`

	// MarketingConsent dùng CON TRỎ ở cả hai tầng, và đó là cả vấn đề.
	//
	// PATCH nghĩa là "sửa những gì tôi gửi". Dùng `bool` thì trường vắng
	// mặt đọc ra `false`, nên một lần sửa số điện thoại sẽ âm thầm RÚT
	// đồng ý nhận thư khuyến mãi — và bản ghi rút ấy có dấu thời gian và
	// nguồn "settings", trông y như khách tự bấm.
	MarketingConsent *marketingConsentPatch `json:"marketing_consent,omitempty"`
}

type marketingConsentPatch struct {
	Email *bool `json:"email,omitempty"`
	SMS   *bool `json:"sms,omitempty"`
}

// updateProfile phục vụ PATCH /api/v1/me (operationId: updateMyProfile).
//
// KHÔNG nhận `email`: đổi email là đổi DANH TÍNH, không phải sửa hồ sơ. Nó
// cần xác minh quyền sở hữu địa chỉ mới — nếu không, đổi email của người
// khác thành của mình là chiếm được tài khoản.
func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req updateProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	id, err := h.customerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	v, err := h.svc.UpdateProfile(r.Context(), id, req.Name, req.Phone)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	if err := h.ghiDongY(r, id, req.MarketingConsent); err != nil {
		h.fail(w, r, translate(err))
		return
	}

	out := toProfile(v)
	dongY, err := h.dongYMarketing(r, id)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}
	out.MarketingConsent = dongY

	h.ok(w, r, http.StatusOK, profileResponse{Customer: out})
}

// ghiDongY ghi lại hai cờ đồng ý khách vừa đổi.
//
// # Mỗi lần bấm là một BẢN GHI MỚI, không phải một lần ghi đè
//
// `customer_consent` là bằng chứng pháp lý, nên câu hỏi nó phải trả lời
// được là "LÚC gửi lá thư đó khách có đồng ý không" — chứ không phải
// "bây giờ khách có đồng ý không". Ghi đè một cờ boolean xóa mất câu trả
// lời ấy.
//
// `source` là "settings": nơi khách bấm. Một lần đồng ý không nói được
// bấm ở đâu thì không dùng làm bằng chứng được.
func (h *Handler) ghiDongY(
	r *http.Request, id ids.ID, p *marketingConsentPatch,
) error {
	if p == nil {
		return nil
	}

	for _, c := range []struct {
		loai domain.ConsentType
		gia  *bool
	}{
		{domain.ConsentMarketingEmail, p.Email},
		{domain.ConsentMarketingSMS, p.SMS},
	} {
		if c.gia == nil {
			continue
		}
		if _, err := h.svc.RecordConsent(r.Context(), domain.NewConsentParams{
			CustomerID: id,
			Type:       c.loai,
			Granted:    *c.gia,
			Source:     "settings",

			// Băm IP ngay tại biên: bên trong không bao giờ thấy địa chỉ
			// nguyên văn, nên không có chỗ nào lỡ tay ghi nó ra nhật ký.
			IPHash:    privacy.HashIP(diaChiIP(r)),
			UserAgent: r.UserAgent(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- Địa chỉ

type addressJSON struct {
	ID string `json:"id"`

	RecipientName string `json:"recipient_name"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"street_address"`
	Ward          string `json:"ward,omitempty"`
	District      string `json:"district,omitempty"`
	Province      string `json:"province"`
	PostalCode    string `json:"postal_code,omitempty"`
	CountryCode   string `json:"country_code"`
	DeliveryNote  string `json:"delivery_note,omitempty"`

	IsDefault bool `json:"is_default"`
}

type listAddressesResponse struct {
	Data []addressJSON `json:"data"`
}

// listAddresses phục vụ GET /api/v1/me/addresses
// (operationId: listMyAddresses).
func (h *Handler) listAddresses(w http.ResponseWriter, r *http.Request) {
	id, err := h.customerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	list, err := h.svc.ListAddresses(r.Context(), id)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	// Mảng rỗng chứ không phải null: khách chưa có địa chỉ nào là trạng
	// thái BÌNH THƯỜNG, và `null` bắt giao diện kiểm tra thêm một lần.
	data := make([]addressJSON, 0, len(list))
	for _, a := range list {
		data = append(data, toAddress(a))
	}

	h.ok(w, r, http.StatusOK, listAddressesResponse{Data: data})
}

type addAddressRequest struct {
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

	IsDefault bool `json:"is_default,omitempty"`
}

type addAddressResponse struct {
	Address addressJSON `json:"address"`
}

// addAddress phục vụ POST /api/v1/me/addresses
// (operationId: addMyAddress).
//
// Địa chỉ trong SỔ khác địa chỉ trong ĐƠN: lúc đặt hàng nó được SAO CHÉP
// vào đơn, nên sửa sổ về sau không làm đổi nơi đơn cũ đã giao tới.
func (h *Handler) addAddress(w http.ResponseWriter, r *http.Request) {
	var req addAddressRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	id, err := h.customerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	v, err := h.svc.AddAddress(r.Context(), domain.NewAddressParams{
		CustomerID:     id,
		RecipientName:  req.RecipientName,
		RecipientPhone: req.Phone,
		Line1:          req.StreetAddress,
		Ward:           req.Ward,
		District:       req.District,
		Province:       req.Province,
		Postcode:       req.PostalCode,
		Country:        req.CountryCode,
		Note:           req.DeliveryNote,
		IsDefault:      req.IsDefault,
	})
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, http.StatusCreated, addAddressResponse{Address: toAddress(v)})
}

// ---------------------------------------------------------------- Yêu thích

type wishlistItemJSON struct {
	// ProductID chứ KHÔNG phải cả object sản phẩm — xem chú thích ở
	// getWishlist.
	ProductID string `json:"product_id"`
	VariantID string `json:"variant_id,omitempty"`
	Note      string `json:"note,omitempty"`

	NotifyWhenAvailable bool   `json:"notify_when_available"`
	AddedAt             string `json:"added_at"`
}

type wishlistResponse struct {
	Data []wishlistItemJSON `json:"data"`
}

// getWishlist phục vụ GET /api/v1/me/wishlist
// (operationId: getMyWishlist).
//
// # Trả `product_id`, KHÔNG trả object sản phẩm
//
// Đặc tả khai báo `product: ProductSummary` — tên, ảnh, giá. Dữ liệu đó
// thuộc module `product`, và `customer` nằm CÙNG TẦNG với nó trong đồ thị
// phụ thuộc (dependency-rules.md mục 6): mũi tên chỉ đi từ trên xuống, nên
// customer không được gọi product.
//
// Việc ghép là của TRANG: giao diện gọi endpoint này lấy danh sách mã, rồi
// gọi `listProducts` một lần cho tất cả. Đó cũng là cách ÍT truy vấn hơn so
// với việc endpoint tự ghép từng món.
func (h *Handler) getWishlist(w http.ResponseWriter, r *http.Request) {
	id, err := h.customerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	v, err := h.svc.GetWishlist(r.Context(), id)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	items := v.Items()
	data := make([]wishlistItemJSON, 0, len(items))
	for _, it := range items {
		data = append(data, wishlistItemJSON{
			ProductID:           it.ProductID.String(),
			VariantID:           it.VariantID.String(),
			Note:                it.Note,
			NotifyWhenAvailable: it.NotifyWhenAvailable,
			AddedAt:             it.AddedAt.UTC().Format(time.RFC3339),
		})
	}

	h.ok(w, r, http.StatusOK, wishlistResponse{Data: data})
}

type addWishlistRequest struct {
	ProductID string `json:"product_id"`
	VariantID string `json:"variant_id,omitempty"`
	Note      string `json:"note,omitempty"`

	NotifyWhenAvailable bool `json:"notify_when_available,omitempty"`
}

type addWishlistResponse struct {
	// Added phân biệt "vừa thêm" với "đã có sẵn".
	//
	// Cả hai đều là 201 vì kết quả giống nhau — món nằm trong danh sách.
	// Nhưng giao diện cần biết để không hiện "đã thêm!" khi khách bấm lại
	// một món họ thích từ tuần trước.
	Added bool `json:"added"`
}

// addWishlistItem phục vụ POST /api/v1/me/wishlist
// (operationId: addWishlistItem).
func (h *Handler) addWishlistItem(w http.ResponseWriter, r *http.Request) {
	var req addWishlistRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.ProductID == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"product_id là trường bắt buộc"))
		return
	}

	id, err := h.customerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	added, err := h.svc.AddToWishlist(r.Context(), id,
		ids.ID(req.ProductID), ids.ID(req.VariantID),
		req.Note, req.NotifyWhenAvailable)
	if err != nil {
		h.fail(w, r, translate(err))
		return
	}

	h.ok(w, r, http.StatusCreated, addWishlistResponse{Added: added})
}

// ---------------------------------------------------------------- Hỗ trợ

// customerID lấy định danh khách hàng của người gọi.
// diaChiIP lấy IP của người gọi, ưu tiên header của bộ cân bằng tải.
//
// Header này client tự đặt được, nên nó KHÔNG dùng cho phân quyền — chỉ
// để ghi kèm bằng chứng đồng ý. Giá trị được BĂM trước khi lưu: IP nguyên
// văn là dữ liệu cá nhân, cần cơ sở pháp lý để giữ.
func diaChiIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return strings.TrimSpace(rip)
	}
	return r.RemoteAddr
}

func (h *Handler) customerID(r *http.Request) (ids.ID, error) {
	s, ok := httpserver.ShopperFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"tài khoản chạy không qua ResolveShopper — kiểm tra nối dây")
		return "", apierror.ErrInternal
	}
	if s.CustomerID == "" {
		return "", apierror.New(apierror.CodeUnauthorized,
			"Cần đăng nhập để dùng tính năng này")
	}
	return ids.ID(s.CustomerID), nil
}

func (h *Handler) ok(w http.ResponseWriter, r *http.Request, status int, body any) {
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
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy hồ sơ khách hàng")
	case errors.Is(err, domain.ErrAddressNotFound):
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy địa chỉ")
	case errors.Is(err, domain.ErrVersionConflict):
		// 409, KHÔNG phải 500. Xung đột phiên bản không phải lỗi hệ thống:
		// một tiến trình khác vừa sửa đúng bản ghi này. Người gọi tải lại
		// rồi thử lại là xong.
		//
		// Trả 500 sai theo hai hướng cùng lúc — người gọi tưởng hệ thống
		// hỏng nên không thử lại, còn giám sát thì kêu báo động cho một
		// tình huống bình thường dưới tải.
		return apierror.New(apierror.CodeConflict,
			"Hồ sơ vừa được cập nhật ở nơi khác, vui lòng tải lại và thử lại")

	default:
		return apierror.From(err)
	}
}

// ---------------------------------------------------------------- Chuyển đổi

func toProfile(c *domain.Customer) profileJSON {
	return profileJSON{
		ID:    c.ID().String(),
		Name:  c.DisplayName(),
		Email: c.Email(),
		Phone: c.Phone(),
		Tier:  string(c.Status()),
	}
}

func toAddress(a *domain.Address) addressJSON {
	// Line2 gộp vào street_address: đặc tả chỉ có MỘT dòng địa chỉ, và bỏ
	// Line2 đi là mất số căn hộ — thứ quyết định gói hàng tới đúng cửa.
	street := a.Line1()
	if a.Line2() != "" {
		street += ", " + a.Line2()
	}

	return addressJSON{
		ID:            a.ID().String(),
		RecipientName: a.RecipientName(),
		Phone:         a.RecipientPhone(),
		StreetAddress: street,
		Ward:          a.Ward(),
		District:      a.District(),
		Province:      a.Province(),
		PostalCode:    a.Postcode(),
		CountryCode:   a.Country(),
		DeliveryNote:  a.Note(),
		IsDefault:     a.IsDefault(),
	}
}
