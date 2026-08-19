// Package http là tầng interfaces của module fulfillment cho KHÁCH HÀNG.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/orders.yaml.
package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/application"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// OrderAccessPort trả lời câu "người này có được xem đơn đó không".
//
// # Vì sao HỎI thay vì tự kiểm tra
//
// Quyền xem đơn là quy tắc của module `order` — nó biết đơn thuộc về ai.
// Module này cài lại quy tắc đó nghĩa là có HAI bản, và sớm muộn một bản
// lỏng hơn bản kia. Một bản lỏng là đủ để lộ lịch sử mua hàng.
//
// Interface do BÊN GỌI khai báo; cmd/api nối nó với order.API. Nhờ vậy
// module này không import order và vẫn không phụ thuộc module nào.
type OrderAccessPort interface {
	// ResolveViewableOrder phân giải mã đơn VÀ kiểm tra quyền trong một
	// bước. `key` nhận cả mã đơn lẫn mã hiển thị — khách vãng lai chỉ có
	// mã hiển thị trong email xác nhận.
	ResolveViewableOrder(
		ctx context.Context, key, customerID, guestPhone string,
	) (orderID string, allowed bool, err error)
}

// CustomerHandler phục vụ endpoint lô giao cho khách.
type CustomerHandler struct {
	svc    *application.Service
	access OrderAccessPort
	log    *slog.Logger
}

func NewCustomerHandler(
	svc *application.Service, access OrderAccessPort, log *slog.Logger,
) *CustomerHandler {
	return &CustomerHandler{svc: svc, access: access, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc `ResolveShopper`.
func (h *CustomerHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/orders/{order_id}/shipments",
		http.HandlerFunc(h.listShipments))
}

type shipmentJSON struct {
	FulfillmentNumber string `json:"fulfillment_number"`

	// SellerID chứ KHÔNG phải object nhà bán.
	//
	// Tên và đánh giá của nhà bán thuộc module `seller`, mà module này
	// KHÔNG phụ thuộc module nào — nó chỉ lắng nghe event. Việc ghép tên
	// là của TRANG.
	SellerID string `json:"seller_id"`

	Status string `json:"status"`

	TrackingNumber   string `json:"tracking_number,omitempty"`
	ShippingProvider string `json:"shipping_provider,omitempty"`
	EstimatedArrival string `json:"estimated_delivery_date,omitempty"`

	// OrderLineIDs cho phép TRANG ghép mà KHÔNG cần thêm lượt gọi nào.
	//
	// Trang chi tiết đơn đã có sẵn dòng hàng từ `getOrder`; nó chỉ cần
	// biết dòng nào đi trong gói nào. Trả kèm tên và giá ở đây nghĩa là
	// nhân bản dữ liệu của module order — và hai bản sẽ lệch khi đơn bị
	// hủy một phần.
	OrderLineIDs []string `json:"order_line_ids"`

	ShippedAt   string `json:"shipped_at,omitempty"`
	DeliveredAt string `json:"delivered_at,omitempty"`
}

type listShipmentsResponse struct {
	Data []shipmentJSON `json:"data"`
}

// listShipments phục vụ GET /api/v1/orders/{order_id}/shipments
// (operationId: listOrderShipments).
//
// # Vì sao khách cần endpoint này
//
// Khách thấy MỘT đơn nhưng hàng từ nhiều nguồn về thành NHIỀU gói, giao ở
// những thời điểm khác nhau. Không có nó thì khách nhận một phần hàng và
// tưởng đơn bị thiếu.
func (h *CustomerHandler) listShipments(w http.ResponseWriter, r *http.Request) {
	orderID := strings.TrimSpace(r.PathValue("order_id"))

	var customerID string
	s, ok := httpserver.ShopperFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"lô giao chạy không qua ResolveShopper — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrInternal)
		return
	}
	customerID = s.CustomerID

	// KIỂM TRA QUYỀN TRƯỚC khi đọc bất cứ thứ gì.
	resolved, allowed, err := h.access.ResolveViewableOrder(
		r.Context(), orderID, customerID, r.Header.Get("X-Guest-Phone"))
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}
	if !allowed {
		// 404 chứ không phải 403, và GIỐNG HỆT câu trả lời cho đơn không
		// tồn tại: mã đơn tăng dần nên hai thông báo khác nhau sẽ đếm được
		// số đơn nền tảng bán mỗi tháng.
		h.fail(w, r, apierror.New(apierror.CodeNotFound, "Không tìm thấy đơn hàng"))
		return
	}

	list, err := h.svc.ListByOrder(r.Context(), ids.ID(resolved))
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	data := make([]shipmentJSON, 0, len(list))
	for _, f := range list {
		lineIDs := make([]string, 0, len(f.LineIDs()))
		for _, id := range f.LineIDs() {
			lineIDs = append(lineIDs, id.String())
		}

		data = append(data, shipmentJSON{
			FulfillmentNumber: f.FONumber(),
			SellerID:          f.SellerID().String(),
			Status:            string(f.Status()),
			TrackingNumber:    f.TrackingNumber(),
			ShippingProvider:  f.ShippingProvider(),
			OrderLineIDs:      lineIDs,
			EstimatedArrival:  formatTime(f.EstimatedDelivery()),
			ShippedAt:         formatTime(f.ShippedAt()),
			DeliveredAt:       formatTime(f.DeliveredAt()),
		})
	}

	if err := apierror.WriteJSON(w, http.StatusOK,
		listShipmentsResponse{Data: data}); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *CustomerHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

// formatTime đổi thời điểm sang RFC3339, hoặc chuỗi rỗng nếu chưa có.
//
// Thời điểm rỗng nghĩa là VIỆC ĐÓ CHƯA XẢY RA — chưa giao, chưa nhận. Trả
// "0001-01-01T00:00:00Z" ra giao diện thì khách thấy một ngày vô nghĩa từ
// năm thứ nhất.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
