package http

// Đây là góc nhìn KHÁCH HÀNG của module order — khác hẳn handler.go, vốn
// phục vụ nhân viên vận hành.
//
// # Ba khác biệt so với đường quản trị
//
//  1. Phạm vi dữ liệu do NGƯỜI GỌI quyết định, không do tham số truy vấn.
//     Không có `customer_id` trong query: khách chỉ thấy đơn của mình.
//  2. KHÔNG ghi audit log. Nhật ký thao tác ghi hành động của NHÂN VIÊN
//     (ADR-0011); khách xem đơn của chính mình không phải thứ cần giám sát.
//  3. Có đường cho khách VÃNG LAI: tra đơn bằng mã đơn + số điện thoại.
//
// # Vì sao response KHÔNG có `shipments`
//
// Đặc tả mô tả khách thấy nhiều lô giao. Dữ liệu đó thuộc module
// `fulfillment`, và module `order` KHÔNG được gọi nó — fulfillment đã phụ
// thuộc order, gọi ngược tạo phụ thuộc vòng (ADR-0007, archcheck R5).
//
// Việc ghép là của TRANG, không phải của ENDPOINT. Nhưng endpoint để trang
// lấy lô giao thì CHƯA TỒN TẠI — xem backlog P1.8.

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/order/application"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// CustomerHandler phục vụ các endpoint đơn hàng của khách.
type CustomerHandler struct {
	svc *application.Service
	log *slog.Logger
}

func NewCustomerHandler(svc *application.Service, log *slog.Logger) *CustomerHandler {
	return &CustomerHandler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc `ResolveShopper` — không có nó thì handler
// không biết đơn thuộc về ai.
func (h *CustomerHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/orders", http.HandlerFunc(h.listMyOrders))
	mux.Handle("GET /api/v1/orders/{order_id}", http.HandlerFunc(h.getOrder))
	mux.Handle("POST /api/v1/orders/{order_id}/cancel", http.HandlerFunc(h.cancelOrder))
}

// ---------------------------------------------------------------- Kiểu JSON

type summaryJSON struct {
	ID          string     `json:"id"`
	OrderNumber string     `json:"order_number"`
	Status      string     `json:"status"`
	Total       amountJSON `json:"total"`
	ItemCount   int        `json:"item_count"`
	PlacedAt    string     `json:"placed_at"`
}

type paginationJSON struct {
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

type listMyOrdersResponse struct {
	Data       []summaryJSON  `json:"data"`
	Pagination paginationJSON `json:"pagination"`
}

// defaultPageSize là số đơn mỗi trang khi client không nói gì.
const defaultPageSize = 20

// maxPageSize chặn trang quá lớn.
//
// Không phải để tiết kiệm băng thông mà để một request không khóa database
// lâu: khách mua nhiều năm có thể có hàng nghìn đơn.
const maxPageSize = 100

// listMyOrders phục vụ GET /api/v1/orders (operationId: listMyOrders).
//
// CHỈ trả đơn của chính người gọi. Điều kiện lọc nằm trong truy vấn ở tầng
// repository (`ListByCustomer`), không phải ở tầng hiển thị — lọc khi hiển
// thị nghĩa là dữ liệu người khác đã rời khỏi database rồi.
func (h *CustomerHandler) listMyOrders(w http.ResponseWriter, r *http.Request) {
	customerID, err := h.customerID(r)
	if err != nil {
		h.failCustomer(w, r, err)
		return
	}

	limit, offset, err := page(r)
	if err != nil {
		h.failCustomer(w, r, err)
		return
	}

	// Lấy DƯ MỘT bản ghi để biết còn trang sau không, thay vì chạy thêm
	// một truy vấn COUNT trên toàn bộ lịch sử mua hàng.
	orders, err := h.svc.ListCustomerOrders(r.Context(), customerID, limit+1, offset)
	if err != nil {
		h.failCustomer(w, r, translate(err))
		return
	}

	hasMore := len(orders) > limit
	if hasMore {
		orders = orders[:limit]
	}

	// Lọc theo trạng thái ở tầng này là TẠM THỜI và có hệ quả: bản ghi bị
	// loại vẫn tính vào trang, nên một trang có thể trả ít hơn `limit`.
	// Lọc đúng chỗ là trong truy vấn — xem backlog P3-11.
	wanted := strings.TrimSpace(r.URL.Query().Get("status"))

	data := make([]summaryJSON, 0, len(orders))
	for _, o := range orders {
		if wanted != "" && string(o.Status()) != wanted {
			continue
		}
		data = append(data, toSummary(o))
	}

	res := listMyOrdersResponse{
		Data:       data,
		Pagination: paginationJSON{HasMore: hasMore},
	}
	if hasMore {
		next := strconv.Itoa(offset + limit)
		res.Pagination.NextCursor = &next
	}

	h.okCustomer(w, r, http.StatusOK, res)
}

// ---------------------------------------------------------------- Chi tiết

type customerLineJSON struct {
	OrderLineID        string     `json:"order_line_id"`
	ProductName        string     `json:"product_name"`
	VariantDescription string     `json:"variant_description,omitempty"`
	Quantity           int        `json:"quantity"`
	UnitPrice          amountJSON `json:"unit_price"`
	LineTotal          amountJSON `json:"line_total"`
	Status             string     `json:"status"`
}

type customerAddressJSON struct {
	RecipientName string `json:"recipient_name"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"street_address"`
	Ward          string `json:"ward,omitempty"`
	District      string `json:"district,omitempty"`
	Province      string `json:"province"`
	CountryCode   string `json:"country_code"`
}

type customerDetailJSON struct {
	ID          string `json:"id"`
	OrderNumber string `json:"order_number"`
	Status      string `json:"status"`
	PlacedAt    string `json:"placed_at"`
	CompletedAt string `json:"completed_at,omitempty"`

	ShippingAddress *customerAddressJSON `json:"shipping_address,omitempty"`

	// Lines là các dòng hàng của đơn.
	//
	// KHÔNG phải `shipments`: chia hàng thành lô giao là việc của module
	// fulfillment, mà module này không được gọi. Giao diện ghép hai nguồn.
	Lines []customerLineJSON `json:"lines"`

	Subtotal    amountJSON `json:"subtotal"`
	ShippingFee amountJSON `json:"shipping_fee"`
	Discount    amountJSON `json:"discount_amount"`
	Total       amountJSON `json:"total"`

	CanCancel bool `json:"can_cancel"`

	// CancellationReason chỉ có ở đơn khách tự hủy.
	CancellationReason string `json:"cancellation_reason,omitempty"`
}

// getOrder phục vụ GET /api/v1/orders/{order_id} (operationId: getOrder).
//
// Nhận CẢ mã đơn (`ord_...`) lẫn mã hiển thị (`FC-2026-08-001234`): khách
// vãng lai chỉ có mã hiển thị trong email xác nhận, không có mã nội bộ.
func (h *CustomerHandler) getOrder(w http.ResponseWriter, r *http.Request) {
	o, err := h.findOwned(r)
	if err != nil {
		h.failCustomer(w, r, err)
		return
	}
	h.okCustomer(w, r, http.StatusOK, toCustomerDetail(o))
}

// ---------------------------------------------------------------- Hủy đơn

// cancelReasons là năm lý do đặc tả cho phép.
//
// Danh sách ĐÓNG chứ không phải chuỗi tự do: năm lý do này dẫn tới năm
// hành động khác nhau của nền tảng, và văn bản tự do thì không tổng hợp
// được. "Giao quá chậm" là vấn đề vận hành, "tìm được giá tốt hơn" là vấn
// đề giá — trộn chúng vào một ô ghi chú là mất cả hai tín hiệu.
var cancelReasons = map[string]bool{
	"CHANGED_MIND":       true,
	"FOUND_BETTER_PRICE": true,
	"ORDERED_BY_MISTAKE": true,
	"DELIVERY_TOO_SLOW":  true,
	"OTHER":              true,
}

type customerCancelRequest struct {
	Reason string `json:"reason"`
	Note   string `json:"note,omitempty"`
}

type customerCancelResponse struct {
	Order summaryJSON `json:"order"`
}

// cancelOrder phục vụ POST /api/v1/orders/{order_id}/cancel
// (operationId: cancelOrder).
//
// # Response KHÔNG có `refund`
//
// Đặc tả khai báo `refund: {amount, estimated_days}`. Số tiền hoàn và thời
// gian dự kiến thuộc module `payment`, mà module này không được gọi. Đoán
// một con số ở đây tệ hơn là không trả gì: khách đọc "hoàn trong 3 ngày"
// rồi chờ, trong khi không có gì cam kết con số đó.
//
// Hoàn tiền chạy bất đồng bộ qua event sau khi đơn chuyển CANCELLED.
func (h *CustomerHandler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	var req customerCancelRequest
	if err := decodeJSON(r, &req); err != nil {
		h.failCustomer(w, r, err)
		return
	}
	if !cancelReasons[req.Reason] {
		h.failCustomer(w, r, apierror.New(apierror.CodeValidationFailed,
			"reason phải là một trong: CHANGED_MIND, FOUND_BETTER_PRICE, "+
				"ORDERED_BY_MISTAKE, DELIVERY_TOO_SLOW, OTHER"))
		return
	}

	// Tìm đơn TRƯỚC rồi mới hủy: hủy thẳng theo mã trong đường dẫn nghĩa
	// là ai cũng hủy được đơn của người khác.
	o, err := h.findOwned(r)
	if err != nil {
		h.failCustomer(w, r, err)
		return
	}

	cancelled, err := h.svc.CancelOwnOrder(r.Context(), o.ID(), req.Reason)
	if err != nil {
		h.failCustomer(w, r, translate(err))
		return
	}

	h.okCustomer(w, r, http.StatusOK,
		customerCancelResponse{Order: toSummary(cancelled)})
}

// ---------------------------------------------------------------- Hỗ trợ

// customerID lấy định danh khách hàng của người gọi.
//
// Đường này CHỈ dành cho khách đã đăng nhập: danh sách đơn hàng không có
// cách nào giới hạn an toàn cho khách vãng lai — họ không có định danh bền
// vững nào ngoài cookie phiên, và cookie phiên đổi mỗi lần đổi thiết bị.
func (h *CustomerHandler) customerID(r *http.Request) (ids.ID, error) {
	s, ok := httpserver.ShopperFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"đơn hàng của khách chạy không qua ResolveShopper — kiểm tra nối dây")
		return "", apierror.ErrInternal
	}
	if s.CustomerID == "" {
		return "", apierror.New(apierror.CodeUnauthorized,
			"Cần đăng nhập để xem lịch sử đơn hàng")
	}
	return ids.ID(s.CustomerID), nil
}

// findOwned đọc đơn trong đường dẫn và KIỂM TRA nó thuộc về người gọi.
//
// # Vì sao "không phải đơn của bạn" trả 404 chứ không phải 403
//
// Mã hiển thị của đơn là chuỗi TĂNG DẦN (FC-2026-08-001234). Trả 403 cho
// đơn có thật và 404 cho đơn không có nghĩa là dò tuần tự sẽ đếm được
// chính xác nền tảng bán bao nhiêu đơn mỗi tháng — con số kinh doanh mà
// không ai định công bố.
//
// Một mã trả lời cho cả hai trường hợp thì phép dò đó không cho biết gì.
func (h *CustomerHandler) findOwned(r *http.Request) (*domain.Order, error) {
	key := strings.TrimSpace(r.PathValue("order_id"))
	if key == "" {
		return nil, apierror.New(apierror.CodeNotFound, "Không tìm thấy đơn hàng")
	}

	var (
		o   *domain.Order
		err error
	)
	if strings.HasPrefix(key, string(ids.PrefixOrder)) {
		o, err = h.svc.GetOrder(r.Context(), ids.ID(key))
	} else {
		o, err = h.svc.GetOrderByNumber(r.Context(), key)
	}
	if err != nil {
		return nil, translate(err)
	}

	if !h.owns(r, o) {
		return nil, apierror.New(apierror.CodeNotFound, "Không tìm thấy đơn hàng")
	}
	return o, nil
}

// owns cho biết người gọi có quyền xem đơn này không.
//
// Hai đường, không hơn:
//
//	Đã đăng nhập  → định danh khách hàng phải TRÙNG
//	Vãng lai      → số điện thoại ở header phải TRÙNG số trên đơn
//
// Số điện thoại rỗng KHÔNG khớp với gì cả. Nếu không, một đơn thiếu số
// điện thoại sẽ mở cho bất kỳ ai không gửi header.
func (h *CustomerHandler) owns(r *http.Request, o *domain.Order) bool {
	var customerID string
	if s, ok := httpserver.ShopperFrom(r.Context()); ok {
		customerID = s.CustomerID
	}

	// Quy tắc nằm ở DOMAIN, không phải ở đây: nó còn được hỏi từ endpoint
	// lô giao của module fulfillment, và hai bản cài đặt sẽ lệch nhau.
	return o.ViewableBy(customerID, r.Header.Get("X-Guest-Phone"))
}

// page đọc tham số phân trang.
//
// `cursor` ở đây là VỊ TRÍ BẮT ĐẦU được mã hóa dưới dạng chuỗi, không phải
// con trỏ theo khóa. Kho lưu trữ hiện dùng OFFSET, nên con trỏ thật chưa
// làm được — và đặc tả coi `next_cursor` là chuỗi mờ nên client không thấy
// khác biệt. Hệ quả có thật: đơn mới đặt xen vào giữa hai lần lật trang có
// thể làm một đơn xuất hiện hai lần. Xem backlog P3-12.
func page(r *http.Request) (limit, offset int, err error) {
	q := r.URL.Query()

	limit = defaultPageSize
	if v := q.Get("limit"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 1 || n > maxPageSize {
			return 0, 0, apierror.Newf(apierror.CodeValidationFailed,
				"limit phải là số nguyên từ 1 đến %d", maxPageSize)
		}
		limit = n
	}

	if v := q.Get("cursor"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 0 {
			return 0, 0, apierror.New(apierror.CodeValidationFailed,
				"cursor không hợp lệ")
		}
		offset = n
	}

	return limit, offset, nil
}

func (h *CustomerHandler) okCustomer(
	w http.ResponseWriter, r *http.Request, status int, body any,
) {
	if err := apierror.WriteJSON(w, status, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *CustomerHandler) failCustomer(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

// ---------------------------------------------------------------- Chuyển đổi

func toSummary(o *domain.Order) summaryJSON {
	return summaryJSON{
		ID:          o.ID().String(),
		OrderNumber: o.OrderNumber(),
		Status:      string(o.Status()),
		Total:       toAmount(o.Total()),
		// Đếm dòng CÒN HIỆU LỰC: đơn hủy một phần thì số món khách thực sự
		// nhận ít hơn số dòng ban đầu.
		ItemCount: len(o.ActiveLines()),
		PlacedAt:  o.PlacedAt().UTC().Format(time.RFC3339),
	}
}

func toCustomerDetail(o *domain.Order) customerDetailJSON {
	lines := make([]customerLineJSON, 0, len(o.Lines()))
	for _, l := range o.Lines() {
		lines = append(lines, customerLineJSON{
			OrderLineID:        l.ID().String(),
			ProductName:        l.ProductName(),
			VariantDescription: l.VariantDescription(),
			Quantity:           l.Quantity(),
			UnitPrice:          toAmount(l.UnitPrice()),
			LineTotal:          toAmount(l.LineTotal()),
			Status:             string(l.Status()),
		})
	}

	out := customerDetailJSON{
		ID:          o.ID().String(),
		OrderNumber: o.OrderNumber(),
		Status:      string(o.Status()),
		PlacedAt:    o.PlacedAt().UTC().Format(time.RFC3339),
		Lines:       lines,
		Subtotal:    toAmount(o.Subtotal()),
		ShippingFee: toAmount(o.ShippingFee()),
		Discount:    toAmount(o.DiscountAmount()),
		Total:       toAmount(o.Total()),

		// can_cancel theo TRẠNG THÁI TỔNG HỢP. Điều kiện đầy đủ còn cần
		// "chưa có lô nào PACKED" (order.md mục 6.1), mà module này không
		// biết. Nên nút hủy có thể hiện ra rồi request bị từ chối 409 —
		// khó chịu, nhưng an toàn hơn là ẩn nút của đơn còn hủy được.
		CanCancel:          o.Status().CanCancelWholeOrder(),
		CancellationReason: o.CancellationReason(),
	}

	if !o.CompletedAt().IsZero() {
		out.CompletedAt = o.CompletedAt().UTC().Format(time.RFC3339)
	}

	if addr := o.ShippingAddress(); !addr.IsEmpty() {
		out.ShippingAddress = &customerAddressJSON{
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
