package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// SellerHandler phục vụ các endpoint offer của NHÀ BÁN.
//
// # Ranh giới bảo mật
//
// Định danh seller lấy từ `AuthContext.SellerIDs` trong token, KHÔNG từ
// tham số. Mọi thao tác sửa đều kiểm tra offer THUỘC VỀ người gọi trước —
// không kiểm thì bất kỳ ai cũng đổi giá offer của đối thủ.
//
// # Ba lớp kiểm soát khi tạo offer, đều nằm ở domain
//
//  1. Bảo vệ thương hiệu — hàng rào chống hàng giả
//  2. Khung giá — chống bán phá giá VÀ lỗi nhập liệu (thiếu một số 0)
//  3. Một seller một offer active cho một SKU
//
// Tầng này KHÔNG cài lại chúng; nó chỉ dịch lỗi thành thông báo hữu ích.
// StockPort nhập kho ban đầu cho một offer mới.
//
// # Vì sao cần
//
// Seller tạo offer xong mà không có tồn kho thì offer đó HẾT HÀNG ngay từ
// giây đầu tiên, và họ không có đường nào để nhập hàng: `updateInventory`
// chỉ SỬA bản ghi đã có. Đặc tả có `initial_inventory` cho đúng việc này.
//
// Interface do BÊN GỌI khai báo, cmd/api nối với module inventory. Nhờ vậy
// cổng `InventoryPort` của tầng application vẫn CHỈ ĐỌC — module này không
// giành lấy quyền tạo tồn kho.
type StockPort interface {
	// ReceiveInitial nhập lô hàng đầu tiên cho một SKU của một seller.
	//
	// IDEMPOTENT theo (sku, kho, chủ sở hữu): gọi lại không nhân đôi hàng.
	ReceiveInitial(
		ctx context.Context, skuID, sellerID ids.ID, locationID string, quantity int,
	) error
}

type SellerHandler struct {
	svc *application.Service
	log *slog.Logger

	// stock có thể nil: khi đó `initial_inventory` bị BỎ QUA và offer vẫn
	// được tạo. Thà seller có offer chưa có hàng còn hơn không tạo được gì.
	stock StockPort
}

func NewSellerHandler(
	svc *application.Service, stock StockPort, log *slog.Logger,
) *SellerHandler {
	return &SellerHandler{svc: svc, stock: stock, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth và RequireRole("SELLER_OWNER",
// "SELLER_STAFF").
func (h *SellerHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/seller/offers", http.HandlerFunc(h.list))
	mux.Handle("POST /api/v1/seller/offers", http.HandlerFunc(h.create))
	mux.Handle("PATCH /api/v1/seller/offers/{offer_id}", http.HandlerFunc(h.update))
}

// sellerOfferJSON là góc nhìn của NHÀ BÁN, khác `offerJSON` của trang công
// khai: có thêm giới hạn số lượng và thời điểm tạo (seller cần để quản lý),
// không có `is_buy_box` (đó là kết quả cạnh tranh, không phải thuộc tính
// offer của họ).
type sellerOfferJSON struct {
	ID       string `json:"id"`
	SKUID    string `json:"sku_id"`
	SellerID string `json:"seller_id"`

	Price     moneyJSON  `json:"price"`
	CompareAt *moneyJSON `json:"compare_at_price,omitempty"`

	Condition         string `json:"condition"`
	HandlingTimeHours int    `json:"handling_time_hours"`
	MinOrderQuantity  int    `json:"min_order_quantity"`
	MaxOrderQuantity  int    `json:"max_order_quantity,omitempty"`

	Status string `json:"status"`

	// IsSellable là câu trả lời cho "khách mua được không".
	//
	// KHÔNG suy ra từ `status` ở giao diện: quy tắc còn phụ thuộc seller
	// có bị đình chỉ không, và suy ở hai nơi thì hai nơi sẽ lệch.
	IsSellable bool `json:"is_sellable"`

	CreatedAt string `json:"created_at"`
}

type listOffersResponse struct {
	Data []sellerOfferJSON `json:"data"`
}

// list phục vụ GET /api/v1/seller/offers (operationId: listMyOffers).
func (h *SellerHandler) list(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 1 || n > 200 {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"limit phải là số nguyên từ 1 đến 200"))
			return
		}
		limit = n
	}

	list, err := h.svc.GetOffersBySeller(r.Context(), sellerID, limit, 0)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	// Lọc trạng thái ở tầng này là TẠM THỜI: bản ghi bị loại vẫn tính vào
	// trang, nên một trang có thể trả ít hơn `limit`. Lọc đúng chỗ là
	// trong truy vấn — xem backlog P3-11.
	wanted := strings.TrimSpace(r.URL.Query().Get("status"))

	data := make([]sellerOfferJSON, 0, len(list))
	for _, o := range list {
		if wanted != "" && string(o.Status()) != wanted {
			continue
		}
		data = append(data, toSellerOffer(o))
	}
	h.ok(w, r, http.StatusOK, listOffersResponse{Data: data})
}

type createOfferRequest struct {
	SKUID     string     `json:"sku_id"`
	Price     moneyJSON  `json:"price"`
	CompareAt *moneyJSON `json:"compare_at_price,omitempty"`

	Condition         string `json:"condition,omitempty"`
	HandlingTimeHours int    `json:"handling_time_hours,omitempty"`
	MinOrderQuantity  int    `json:"min_order_quantity,omitempty"`
	MaxOrderQuantity  int    `json:"max_order_quantity,omitempty"`

	// InitialInventory nhập lô hàng đầu tiên NGAY khi tạo offer.
	//
	// Không có nó thì offer hết hàng từ giây đầu tiên và seller không có
	// đường nào để nhập: `updateInventory` chỉ SỬA bản ghi đã có.
	InitialInventory *initialInventoryJSON `json:"initial_inventory,omitempty"`
}

type initialInventoryJSON struct {
	StockLocationID string `json:"stock_location_id,omitempty"`
	Quantity        int    `json:"quantity"`
}

type offerResponse struct {
	Offer sellerOfferJSON `json:"offer"`
}

// create phục vụ POST /api/v1/seller/offers (operationId: createOffer).
//
// Offer được ĐƯA LÊN BÁN ngay. Tạo ở trạng thái nháp rồi bắt seller bấm
// thêm một nút nữa là một bước thừa: họ vừa điền giá xong, ý định đã rõ.
func (h *SellerHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createOfferRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.SKUID == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"sku_id là trường bắt buộc"))
		return
	}

	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	price, err := money.New(req.Price.Amount, money.Currency(req.Price.Currency))
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"giá không hợp lệ"))
		return
	}

	var compareAt money.Money
	if req.CompareAt != nil {
		compareAt, err = money.New(req.CompareAt.Amount,
			money.Currency(req.CompareAt.Currency))
		if err != nil {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"giá gạch ngang không hợp lệ"))
			return
		}
	}

	condition := domain.Condition(req.Condition)
	if req.Condition == "" {
		condition = domain.ConditionNew
	}
	handling := req.HandlingTimeHours
	if handling <= 0 {
		handling = 24
	}
	minQty := req.MinOrderQuantity
	if minQty <= 0 {
		minQty = 1
	}

	o, err := h.svc.CreateOffer(r.Context(), application.CreateOfferInput{
		SKUID:             ids.ID(req.SKUID),
		SellerID:          sellerID,
		Price:             price,
		CompareAt:         compareAt,
		Condition:         condition,
		HandlingTimeHours: handling,
		MinOrderQuantity:  minQty,
		MaxOrderQuantity:  req.MaxOrderQuantity,
		Activate:          true,
	})
	if err != nil {
		h.fail(w, r, translateOffer(err))
		return
	}

	// Nhập kho ban đầu SAU khi offer đã tạo.
	//
	// Thất bại ở đây KHÔNG hủy offer: offer đã tồn tại và hợp lệ, chỉ là
	// chưa có hàng — seller sửa được bằng `updateInventory`. Hủy offer để
	// "sạch sẽ" sẽ vứt đi thứ họ vừa tạo thành công.
	if req.InitialInventory != nil && req.InitialInventory.Quantity > 0 {
		if h.stock == nil {
			h.log.WarnContext(r.Context(),
				"bỏ qua initial_inventory: chưa nối StockPort",
				"offer_id", o.ID())
		} else if err := h.stock.ReceiveInitial(r.Context(), o.SKUID(), sellerID,
			req.InitialInventory.StockLocationID,
			req.InitialInventory.Quantity); err != nil {
			h.log.ErrorContext(r.Context(), "nhập kho ban đầu thất bại",
				"error", err, "offer_id", o.ID())
		}
	}

	h.ok(w, r, http.StatusCreated, offerResponse{Offer: toSellerOffer(o)})
}

type updateOfferRequest struct {
	Price     *moneyJSON `json:"price,omitempty"`
	CompareAt *moneyJSON `json:"compare_at_price,omitempty"`

	HandlingTimeHours *int    `json:"handling_time_hours,omitempty"`
	Status            *string `json:"status,omitempty"`
}

// update phục vụ PATCH /api/v1/seller/offers/{offer_id}
// (operationId: updateOffer).
//
// Mọi thay đổi giá đều GHI LỊCH SỬ ở tầng application — cần cho việc phát
// hiện thao túng giá (tăng rồi giảm để giả vờ khuyến mãi).
func (h *SellerHandler) update(w http.ResponseWriter, r *http.Request) {
	var req updateOfferRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	offerID := ids.ID(r.PathValue("offer_id"))

	// KIỂM TRA QUYỀN SỞ HỮU TRƯỚC mọi thay đổi.
	//
	// Quy tắc nằm ở tầng application (`OwnedOffer`), không phải ở đây: nó
	// được hỏi từ mọi đường ghi của seller, và mỗi nơi tự kiểm lại nghĩa
	// là sớm muộn một nơi quên.
	//
	// Không tìm thấy và không phải của mình trả CÙNG một lỗi — phân biệt
	// cho phép dò xem đối thủ đang bán những gì.
	o, err := h.svc.OwnedOffer(r.Context(), offerID, sellerID)
	if err != nil {
		h.fail(w, r, translateOffer(err))
		return
	}

	if req.Price != nil {
		price, convErr := money.New(req.Price.Amount, money.Currency(req.Price.Currency))
		if convErr != nil {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"giá không hợp lệ"))
			return
		}
		compareAt := o.CompareAt()
		if req.CompareAt != nil {
			compareAt, convErr = money.New(req.CompareAt.Amount,
				money.Currency(req.CompareAt.Currency))
			if convErr != nil {
				h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
					"giá gạch ngang không hợp lệ"))
				return
			}
		}

		o, err = h.svc.UpdatePrice(r.Context(), offerID, price, compareAt, sellerID)
		if err != nil {
			h.fail(w, r, translateOffer(err))
			return
		}
	}

	if req.Status != nil {
		switch *req.Status {
		case "ACTIVE":
			o, err = h.svc.ActivateOffer(r.Context(), offerID)
		case "ARCHIVED":
			o, err = h.svc.ArchiveOffer(r.Context(), offerID)
		default:
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"status chỉ nhận ACTIVE hoặc ARCHIVED"))
			return
		}
		if err != nil {
			h.fail(w, r, translateOffer(err))
			return
		}
	}

	h.ok(w, r, http.StatusOK, offerResponse{Offer: toSellerOffer(o)})
}

// ---------------------------------------------------------------- Hỗ trợ

func (h *SellerHandler) sellerID(r *http.Request) (ids.ID, error) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"seller offers chạy không qua Auth — kiểm tra nối dây")
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

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apierror.New(apierror.CodeValidationFailed,
			"Dữ liệu gửi lên không hợp lệ")
	}
	return nil
}

// translateOffer chuyển lỗi domain thành lỗi API.
//
// Hàng rào chống hàng giả trả thông báo NÊU RÕ HÀNH ĐỘNG cần làm: "cần giấy
// ủy quyền" khác hẳn "không tạo được offer", và seller không đoán được phải
// làm gì với thông báo thứ hai.
func translateOffer(err error) error {
	var notAuthorized *application.NotAuthorizedError
	if errors.As(err, &notAuthorized) {
		return apierror.New(apierror.CodeSellerNotAuthorized,
			"Bạn chưa được phép bán thương hiệu này. "+
				"Tải lên giấy ủy quyền hoặc liên hệ nền tảng.")
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy offer")

	case errors.Is(err, domain.ErrDuplicateActiveOffer):
		return apierror.New(apierror.CodeConflict,
			"Bạn đã có một offer đang bán cho sản phẩm này")

	case errors.Is(err, application.ErrSKUNotSellable):
		return apierror.New(apierror.CodeValidationFailed,
			"Sản phẩm này chưa được duyệt để bán")

	case errors.Is(err, application.ErrSellerInactive):
		return apierror.New(apierror.CodeForbidden,
			"Tài khoản nhà bán của bạn đang bị tạm ngưng")

	default:
		return apierror.From(err)
	}
}

func toMoney(m money.Money) moneyJSON {
	return moneyJSON{Amount: m.Amount(), Currency: string(m.Currency())}
}

func toSellerOffer(o *domain.Offer) sellerOfferJSON {
	out := sellerOfferJSON{
		ID:                o.ID().String(),
		SKUID:             o.SKUID().String(),
		SellerID:          o.SellerID().String(),
		Price:             toMoney(o.Price()),
		Condition:         string(o.Condition()),
		HandlingTimeHours: o.HandlingTimeHours(),
		MinOrderQuantity:  o.MinOrderQuantity(),
		MaxOrderQuantity:  o.MaxOrderQuantity(),
		Status:            string(o.Status()),
		IsSellable:        o.Status() == domain.StatusActive,
		CreatedAt:         o.CreatedAt().UTC().Format(time.RFC3339),
	}
	if !o.CompareAt().IsZero() {
		c := toMoney(o.CompareAt())
		out.CompareAt = &c
	}
	return out
}
