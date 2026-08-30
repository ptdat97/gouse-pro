package http

import (
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

// minLyDoKiemKe là độ dài tối thiểu của lý do điều chỉnh.
//
// Dài hơn phía nhà bán (5) vì quyền rộng hơn: quản trị viên sửa được tồn
// kho của BẤT KỲ chủ sở hữu nào, kể cả hàng thuộc về seller. "sai" hay
// "fix" không giải trình được điều gì khi cần đối soát sau ba tháng.
const minLyDoKiemKe = 10

// AdminHandler phục vụ điều chỉnh tồn kho thủ công của quản trị viên.
type AdminHandler struct {
	svc *application.Service
	log *slog.Logger
}

func NewAdminHandler(svc *application.Service, log *slog.Logger) *AdminHandler {
	return &AdminHandler{svc: svc, log: log}
}

func (h *AdminHandler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/admin/inventory/adjustments",
		http.HandlerFunc(h.adjust))
}

type adminAdjustRequest struct {
	SKUID           string `json:"sku_id"`
	StockLocationID string `json:"stock_location_id"`

	// InventoryOwnerID không bắt buộc, và chỉ cần khi NHẬP NHẰNG.
	//
	// Cùng một SKU ở cùng một kho có thể có tồn kho của nhiều chủ sở hữu —
	// hàng seller gửi ở kho nền tảng vẫn thuộc seller. Khi chỉ có một chủ
	// thì bắt gõ thêm một mã dài chỉ khiến người vận hành chép dán sai.
	InventoryOwnerID string `json:"inventory_owner_id"`

	Reason string `json:"reason"`

	Adjustments struct {
		// CON TRỎ chứ không phải int: phải phân biệt "khai 0" với "không
		// khai". Dùng int thường thì mọi request không nhắc tới số hỏng sẽ
		// lặng lẽ đặt nó về 0 — xóa sạch hàng hỏng đã ghi nhận.
		QuantityAvailable *int `json:"quantity_available"`
		QuantityDamaged   *int `json:"quantity_damaged"`
	} `json:"adjustments"`
}

type soLuongJSON struct {
	QuantityAvailable int `json:"quantity_available"`
	QuantityReserved  int `json:"quantity_reserved"`
	QuantityCommitted int `json:"quantity_committed"`
	QuantityInTransit int `json:"quantity_in_transit"`
	QuantityDamaged   int `json:"quantity_damaged"`
	QuantityReturned  int `json:"quantity_returned"`
}

type adminAdjustResponse struct {
	SKUID  string      `json:"sku_id"`
	Before soLuongJSON `json:"before"`
	After  soLuongJSON `json:"after"`
}

// adjust phục vụ POST /api/v1/admin/inventory/adjustments
// (operationId: adjustInventory).
//
// # Con số TUYỆT ĐỐI, phép trừ nằm dưới tầng application
//
// Người kiểm kê đếm ra một con số, không đếm ra một chênh lệch. Tính chênh
// lệch ở đây rồi gửi xuống là đọc-rồi-ghi ngoài vòng khóa lạc quan: giữa
// hai bước có khách đặt hàng, và chênh lệch cũ áp lên số mới cho ra con số
// KHÔNG PHẢI cái đã đếm. `Service.Count` làm phép trừ bên trong vòng thử
// lại.
//
// # Vết kiểm toán là nhật ký BIẾN ĐỘNG, không phải audit log
//
// Mọi thay đổi tồn kho ghi một dòng movement kèm lý do, người thực hiện và
// số lượng sau (quy tắc 4 của inventory.md). Đó là vết chặt hơn audit log
// cho việc này: nó gắn với chính bản ghi tồn kho và đối soát được theo SKU.
func (h *AdminHandler) adjust(w http.ResponseWriter, r *http.Request) {
	var req adminAdjustRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	skuID, err := ids.Parse(req.SKUID, ids.PrefixSKU)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"sku_id không hợp lệ"))
		return
	}
	if strings.TrimSpace(req.StockLocationID) == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"stock_location_id là trường bắt buộc"))
		return
	}

	lyDo := strings.TrimSpace(req.Reason)
	if len(lyDo) < minLyDoKiemKe {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"reason phải có ít nhất 10 ký tự — tồn kho lệch mà không có lý do "+
				"thì không đối soát được, và mất mát trông giống hệt sai sót "+
				"nhập liệu"))
		return
	}

	kd, hong := req.Adjustments.QuantityAvailable, req.Adjustments.QuantityDamaged
	if kd == nil && hong == nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"adjustments phải khai ít nhất một trong quantity_available "+
				"hoặc quantity_damaged"))
		return
	}
	if (kd != nil && *kd < 0) || (hong != nil && *hong < 0) {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"số lượng kiểm kê không được âm"))
		return
	}

	auth, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"kiểm kê tồn kho chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	item, err := h.svc.TimItemKiemKe(r.Context(), skuID,
		ids.ID(req.StockLocationID), ids.ID(req.InventoryOwnerID))
	if err != nil {
		h.fail(w, r, dichLoiKiemKe(err))
		return
	}

	ra, err := h.svc.Count(r.Context(), application.CountInput{
		ItemID:      item.ID(),
		Available:   kd,
		Damaged:     hong,
		Reason:      lyDo,
		PerformedBy: ids.ID(auth.UserID),
	})
	if err != nil {
		h.fail(w, r, dichLoiKiemKe(err))
		return
	}

	h.ok(w, r, adminAdjustResponse{
		SKUID:  skuID.String(),
		Before: doiSoLuong(ra.Before),
		After:  doiSoLuong(ra.After),
	})
}

func doiSoLuong(q domain.Quantities) soLuongJSON {
	return soLuongJSON{
		QuantityAvailable: q.Available(),
		QuantityReserved:  q.Reserved(),
		QuantityCommitted: q.Committed(),
		QuantityInTransit: q.InTransit(),
		QuantityDamaged:   q.Damaged(),
		QuantityReturned:  q.Returned(),
	}
}

func dichLoiKiemKe(err error) error {
	switch {
	case errors.Is(err, application.ErrItemNhapNhang):
		// 409 chứ không phải 400: request hợp lệ, nhưng TRẠNG THÁI dữ liệu
		// khiến nó không đủ để chỉ đúng một bản ghi.
		return apierror.New(apierror.CodeConflict,
			"SKU này ở kho đó có tồn kho của nhiều chủ sở hữu — "+
				"nêu rõ inventory_owner_id")
	case errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeNotFound,
			"Không có bản ghi tồn kho cho SKU này ở kho đó")
	default:
		return apierror.From(err)
	}
}

func (h *AdminHandler) ok(w http.ResponseWriter, r *http.Request, body any) {
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *AdminHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err,
		logger.RequestIDFromContext(r.Context()), h.log)
}
