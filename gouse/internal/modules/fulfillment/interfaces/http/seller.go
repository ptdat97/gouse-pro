package http

import (
	"encoding/json"
	"errors"
	"fmt"
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
// maxTrangGiaoHang khớp `maximum: 100` của tham số Limit dùng chung.
const maxTrangGiaoHang = 100

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
	mux.Handle("POST /api/v1/seller/fulfillment-orders/{fulfillment_order_id}/deliver",
		http.HandlerFunc(h.deliver))
	mux.Handle("GET /api/v1/seller/performance",
		http.HandlerFunc(h.performance))
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

// shipToJSON là nơi hàng phải đến.
//
// CỐ Ý KHÔNG có email khách. Chỉ những trường cần để giao: người nhận, số
// điện thoại (gọi trước khi giao) và địa chỉ. Email không giúp giao hàng,
// và mọi trường thừa ở đây là dữ liệu cá nhân trao cho một bên thứ ba
// không cần tới nó.
type shipToJSON struct {
	RecipientName string `json:"recipient_name"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"street_address"`
	Ward          string `json:"ward,omitempty"`
	District      string `json:"district,omitempty"`
	Province      string `json:"province"`
	CountryCode   string `json:"country_code"`
}

type sellerFOJSON struct {
	ID                string `json:"id"`
	FulfillmentNumber string `json:"fulfillment_number"`
	Status            string `json:"status"`

	Items []foItemJSON `json:"items"`

	// ShippingAddress vắng mặt với đơn tách TRƯỚC khi trường này ra đời.
	// Giao diện phải chịu được và nói rõ với seller thay vì in phiếu trống.
	ShippingAddress *shipToJSON `json:"shipping_address,omitempty"`

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
		// Trần 100 khớp `common.yaml#/parameters/Limit`.
		//
		// Bản đầu chặn ở 200 trong khi đặc tả khai tối đa 100 — mã DỄ DÃI
		// hơn hợp đồng, và kiểu lệch đó không ai thấy cho tới khi một
		// client sinh từ đặc tả từ chối gửi giá trị mà server vẫn nhận.
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 1 || n > maxTrangGiaoHang {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				fmt.Sprintf("limit phải là số nguyên từ 1 đến %d",
					maxTrangGiaoHang)))
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

	case errors.Is(err, domain.ErrVersionConflict):
		// 409, KHÔNG phải 500. Xung đột phiên bản không phải lỗi hệ thống:
		// một tiến trình khác vừa sửa đúng bản ghi này. Người gọi tải lại
		// rồi thử lại là xong.
		//
		// Trả 500 sai theo hai hướng cùng lúc — người gọi tưởng hệ thống
		// hỏng nên không thử lại, còn giám sát thì kêu báo động cho một
		// tình huống bình thường dưới tải.
		return apierror.New(apierror.CodeConflict,
			"Đơn thực hiện vừa được cập nhật, vui lòng tải lại và thử lại")

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

	var shipTo *shipToJSON
	if a := fo.ShippingAddress(); !a.IsEmpty() {
		shipTo = &shipToJSON{
			RecipientName: a.RecipientName,
			Phone:         a.Phone,
			StreetAddress: a.StreetAddress,
			Ward:          a.Ward,
			District:      a.District,
			Province:      a.Province,
			CountryCode:   a.CountryCode,
		}
	}

	return sellerFOJSON{
		ID:                fo.ID().String(),
		FulfillmentNumber: fo.FONumber(),
		Status:            string(fo.Status()),
		Items:             items,
		ShippingAddress:   shipTo,
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

// deliver xác nhận hàng đã tới tay khách.
//
// # Vì sao mắt xích này quan trọng hơn vẻ ngoài của nó
//
// Nó là chỗ vòng đời đơn hàng KẾT THÚC. Từ đây bắt đầu đếm hạn đổi trả;
// hết hạn thì job nền chuyển đơn sang COMPLETED và số dư seller từ Pending
// sang Available. Thiếu nó thì mọi đơn dừng vĩnh viễn ở HANDED_OVER, không
// ai được trả tiền, và không có gì báo cho ai biết là đang kẹt.
//
// # Vì sao KHÔNG có thân request
//
// "Đã giao" không mang thêm dữ liệu nào ngoài chính sự việc. Thời điểm do
// máy chủ ghi, không nhận từ client: một seller đặt lùi ngày giao sẽ rút
// ngắn hạn đổi trả của khách.
//
// Nguồn đáng tin hơn là webhook của hãng vận chuyển, chưa cài. Khi có, thao
// tác này vẫn giữ cho hàng seller tự giao.
func (h *SellerHandler) deliver(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	foID := ids.ID(r.PathValue("fulfillment_order_id"))
	if err := h.svc.Deliver(r.Context(), sellerID, foID); err != nil {
		h.fail(w, r, translateSeller(err))
		return
	}

	fo, err := h.svc.GetSellerFulfillment(r.Context(), sellerID, foID)
	if err != nil {
		h.fail(w, r, translateSeller(err))
		return
	}
	h.ok(w, r, http.StatusOK, toSellerFO(fo))
}

type chiSoJSON struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Status    string  `json:"status"`
}

type chuaDoJSON struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type hieuSuatJSON struct {
	Period  string      `json:"period"`
	Metrics []chiSoJSON `json:"metrics"`

	// SampleSize để nhà bán tự kiểm chứng con số.
	//
	// Không có nó thì "tỷ lệ hủy 5%" là một khẳng định không kiểm được:
	// 5% của bao nhiêu đơn? Và đó chính là hộp đen mà đặc tả cấm.
	SampleSize int `json:"sample_size"`

	// ShippingSLAHours là thước đo dùng để chấm "đúng hạn".
	ShippingSLAHours float64 `json:"shipping_sla_hours"`

	// NotMeasured là những chỉ số đặc tả có khai mà hệ thống CHƯA đo được.
	//
	// Trả một phần rồi im lặng về phần còn lại tạo ra đúng thứ hộp đen mà
	// đặc tả sinh ra để tránh — chỉ khác là ở phía người viết API.
	NotMeasured []chuaDoJSON `json:"not_measured"`

	Impact struct {
		Message string `json:"message"`
	} `json:"impact"`
}

// performance phục vụ GET /api/v1/seller/performance
// (operationId: getMyPerformance).
//
// # Vì sao KHÔNG trả buy_box_win_rate
//
// Đặc tả khai trường đó, nhưng buy box được tính tại thời điểm hỏi và
// không lưu lại, nên không có lịch sử để tính tỷ lệ. Trả một con số ước
// lượng vào đúng chỗ mà đặc tả gọi là "tác động" sẽ là điều tệ nhất có
// thể làm ở endpoint này: nhà bán sẽ ra quyết định kinh doanh dựa trên nó.
//
// Nó nằm trong `not_measured` kèm lý do.
func (h *SellerHandler) performance(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	ky := application.Ky30Ngay
	if v := r.URL.Query().Get("period"); v != "" {
		ky = application.Ky(v)
	}
	if _, ok := application.KyHopLe(ky); !ok {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"period phải là LAST_7_DAYS, LAST_30_DAYS hoặc LAST_90_DAYS"))
		return
	}

	hs, err := h.svc.TinhHieuSuat(r.Context(), sellerID, ky)
	if err != nil {
		h.fail(w, r, translateSeller(err))
		return
	}

	ra := hieuSuatJSON{
		Period:           string(hs.Ky),
		Metrics:          make([]chiSoJSON, 0, len(hs.ChiSo)),
		SampleSize:       hs.SoLieu.TongDon,
		ShippingSLAHours: hs.SLAGio,
		NotMeasured:      make([]chuaDoJSON, 0, len(hs.ChuaDo)),
	}
	for _, c := range hs.ChiSo {
		ra.Metrics = append(ra.Metrics, chiSoJSON{
			Name: c.Ten, Value: c.GiaTri,
			Threshold: c.Nguong, Status: string(c.TrangThai),
		})
	}
	for _, c := range hs.ChuaDo {
		ra.NotMeasured = append(ra.NotMeasured, chuaDoJSON{
			Name: c.Ten, Reason: c.LyDo,
		})
	}
	ra.Impact.Message = hs.ThongDiep

	h.ok(w, r, http.StatusOK, ra)
}
