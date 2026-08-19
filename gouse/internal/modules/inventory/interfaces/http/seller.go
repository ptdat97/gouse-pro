// Package http là tầng interfaces của module inventory cho NHÀ BÁN.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory/application"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// SellerHandler phục vụ endpoint cập nhật tồn kho của nhà bán.
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
	mux.Handle("PUT /api/v1/seller/inventory/{sku_id}",
		http.HandlerFunc(h.update))
}

type updateInventoryRequest struct {
	StockLocationID string `json:"stock_location_id,omitempty"`

	// QuantityAvailable là số lượng SAU khi kiểm kê — con số TUYỆT ĐỐI,
	// không phải chênh lệch.
	//
	// Đó là cách người kiểm kê nghĩ: "đếm được 50 cái", chứ không phải
	// "thêm 3 cái". Bắt họ tự tính chênh lệch là mời gọi lỗi số học.
	QuantityAvailable int `json:"quantity_available"`

	// Reason BẮT BUỘC (quy tắc 7 của inventory.md): tồn kho lệch mà không
	// có lý do thì không ai đối soát được, và mất mát trông giống hệt sai
	// sót nhập liệu.
	Reason string `json:"reason"`
}

type updateInventoryResponse struct {
	SKUID             string `json:"sku_id"`
	QuantityAvailable int    `json:"quantity_available"`
}

// update phục vụ PUT /api/v1/seller/inventory/{sku_id}
// (operationId: updateInventory).
//
// # Con số TUYỆT ĐỐI, và phép trừ nằm bên trong vòng thử lại
//
// Tính chênh lệch ở đây rồi gọi `Adjust` sẽ là đọc-rồi-ghi ngoài vòng khóa
// lạc quan: giữa hai bước, một khách đặt hàng làm số khả dụng đổi, và
// chênh lệch cũ áp lên số mới cho ra con số KHÔNG PHẢI cái seller đã đếm.
// `SetAvailable` làm phép trừ bên trong vòng thử lại.
func (h *SellerHandler) update(w http.ResponseWriter, r *http.Request) {
	var req updateInventoryRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.QuantityAvailable < 0 {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"quantity_available không được âm"))
		return
	}
	if len(strings.TrimSpace(req.Reason)) < 5 {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"reason phải có ít nhất 5 ký tự — tồn kho lệch mà không có lý do "+
				"thì không đối soát được"))
		return
	}

	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	skuID := ids.ID(r.PathValue("sku_id"))

	// Tìm bản ghi tồn kho CỦA CHÍNH seller này.
	//
	// Cùng một SKU có thể có tồn kho của nhiều chủ sở hữu (hàng seller gửi
	// ở kho nền tảng vẫn thuộc seller). Không lọc theo chủ sở hữu nghĩa là
	// seller sửa được tồn kho hàng của người khác.
	item, err := h.svc.FindOwnedItem(r.Context(), skuID, sellerID,
		ids.ID(req.StockLocationID))
	if err != nil {
		h.fail(w, r, translateInventory(err))
		return
	}

	if err := h.svc.SetAvailable(r.Context(), application.SetAvailableInput{
		ItemID:      item.ID(),
		Target:      req.QuantityAvailable,
		Reason:      strings.TrimSpace(req.Reason),
		PerformedBy: sellerID,
	}); err != nil {
		h.fail(w, r, translateInventory(err))
		return
	}

	// Đọc lại: con số cuối cùng có thể khác con số vừa gửi nếu có đơn
	// hàng chen vào giữa, và giao diện phải hiện con số THẬT.
	updated, err := h.svc.FindOwnedItem(r.Context(), skuID, sellerID,
		ids.ID(req.StockLocationID))
	if err != nil {
		h.fail(w, r, translateInventory(err))
		return
	}

	h.ok(w, r, updateInventoryResponse{
		SKUID:             skuID.String(),
		QuantityAvailable: updated.Available(),
	})
}

func (h *SellerHandler) sellerID(r *http.Request) (ids.ID, error) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"seller inventory chạy không qua Auth — kiểm tra nối dây")
		return "", apierror.ErrUnauthorized
	}
	if len(ac.SellerIDs) == 0 {
		return "", apierror.New(apierror.CodeForbidden,
			"Tài khoản này không gắn với nhà bán nào")
	}
	return ids.ID(ac.SellerIDs[0]), nil
}

func (h *SellerHandler) ok(w http.ResponseWriter, r *http.Request, body any) {
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
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

func translateInventory(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		// KHÔNG phân biệt "chưa có bản ghi tồn kho" với "tồn kho của người
		// khác": phân biệt cho phép dò xem đối thủ có bán SKU nào.
		return apierror.New(apierror.CodeNotFound,
			"Bạn chưa có tồn kho cho sản phẩm này")
	default:
		return apierror.From(err)
	}
}
