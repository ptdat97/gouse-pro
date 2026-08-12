// Package http là tầng interfaces của module catalog: chuyển HTTP request
// thành lời gọi use case và chuyển kết quả thành JSON đúng đặc tả OpenAPI.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Mọi quyết định nghiệp vụ nằm ở
// application/ và domain/. Nếu một điều kiện `if` ở đây quyết định điều gì
// được phép về mặt nghiệp vụ, nó đã đặt sai chỗ.
//
// Tên trường JSON lấy TỪ đặc tả api/paths/storefront.yaml — đặc tả là nguồn
// sự thật, không phải struct Go.
package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/application"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ các endpoint catalog công khai.
type Handler struct {
	svc *application.Service
	log *slog.Logger
}

// NewHandler tạo handler.
func NewHandler(svc *application.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Đường dẫn khớp CHÍNH XÁC với api/openapi.yaml. Lệch đường dẫn nghĩa là
// client viết theo đặc tả sẽ nhận 404.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/brands/{brand_id}", http.HandlerFunc(h.getBrand))
	mux.Handle("GET /api/v1/collections/{collection_id}", http.HandlerFunc(h.getCollection))
	mux.Handle("GET /api/v1/categories", http.HandlerFunc(h.getCategoryTree))
}

// getBrand phục vụ GET /api/v1/brands/{brand_id} (operationId: getBrand).
func (h *Handler) getBrand(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("brand_id"), ids.PrefixBrand)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"brand_id không đúng định dạng"))
		return
	}

	b, err := h.svc.GetBrand(r.Context(), id)
	if err != nil {
		h.fail(w, r, translate(err, "Không tìm thấy thương hiệu"))
		return
	}

	// Bộ sưu tập của thương hiệu — đặc tả khai báo trường `collections`.
	cols, err := h.svc.ListCollectionsByBrand(r.Context(), id)
	if err != nil {
		h.fail(w, r, translate(err, "Không tìm thấy thương hiệu"))
		return
	}

	refs := make([]collectionRef, 0, len(cols))
	for _, c := range cols {
		// Chỉ trả bộ sưu tập ĐANG hiển thị cho khách. Bộ sưu tập chưa ra mắt
		// là thông tin kinh doanh nhạy cảm: đối thủ biết trước lịch ra mắt
		// có thể chặn đầu bằng chiến dịch riêng.
		if !c.IsVisibleToCustomer() {
			continue
		}
		refs = append(refs, collectionRef{
			ID:     c.ID().String(),
			Name:   c.Name(),
			Season: c.Season(),
		})
	}

	h.ok(w, r, brandDetail{
		ID:              b.ID().String(),
		Name:            b.Name(),
		Slug:            b.Slug(),
		LogoURL:         b.LogoURL(),
		Description:     b.Description(),
		CountryOfOrigin: b.CountryOfOrigin(),
		Collections:     refs,
	})
}

// getCollection phục vụ GET /api/v1/collections/{collection_id}.
func (h *Handler) getCollection(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("collection_id"), ids.PrefixCollection)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"collection_id không đúng định dạng"))
		return
	}

	c, err := h.svc.GetCollection(r.Context(), id)
	if err != nil {
		h.fail(w, r, translate(err, "Không tìm thấy bộ sưu tập"))
		return
	}

	// Bộ sưu tập chưa ra mắt trả 404, KHÔNG phải 403.
	//
	// 403 xác nhận tài nguyên tồn tại — đủ để đối thủ dò ID và biết chúng ta
	// đang chuẩn bị bộ sưu tập nào.
	if !c.IsVisibleToCustomer() {
		h.fail(w, r, apierror.New(apierror.CodeNotFound, "Không tìm thấy bộ sưu tập"))
		return
	}

	out := collectionDetail{
		ID:     c.ID().String(),
		Name:   c.Name(),
		Season: c.Season(),
		Theme:  c.Theme(),
		// Đặc tả khai báo format: date — chỉ ngày, không giờ.
		LaunchDate: c.LaunchDate().Format("2006-01-02"),
	}

	// Thương hiệu của bộ sưu tập. Nếu không lấy được, vẫn trả bộ sưu tập
	// thay vì lỗi cả request — thiếu tên thương hiệu tốt hơn là trang trắng.
	if b, err := h.svc.GetBrand(r.Context(), c.BrandID()); err == nil {
		out.Brand = &brandRef{
			ID:      b.ID().String(),
			Name:    b.Name(),
			Slug:    b.Slug(),
			LogoURL: b.LogoURL(),
		}
	}

	h.ok(w, r, out)
}

// getCategoryTree phục vụ GET /api/v1/categories.
//
// Trả CẢ CÂY trong một lời gọi. Cây danh mục nhỏ, đổi hiếm, và client cần
// toàn bộ để dựng thanh điều hướng — bắt client gọi từng cấp tạo ra chuỗi
// request tuần tự làm chậm lần tải đầu.
func (h *Handler) getCategoryTree(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.svc.GetCategoryTree(r.Context())
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}
	h.ok(w, r, categoryTreeResponse{Data: toCategoryNodes(nodes)})
}

func toCategoryNodes(nodes []*application.CategoryNode) []categoryNode {
	out := make([]categoryNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, categoryNode{
			ID:       n.Category.ID().String(),
			Name:     n.Category.Name(),
			Slug:     n.Category.Slug(),
			Children: toCategoryNodes(n.Children),
		})
	}
	return out
}

// ---------------------------------------------------------------- Hỗ trợ

func (h *Handler) ok(w http.ResponseWriter, r *http.Request, body any) {
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		// Response đã bắt đầu gửi — không sửa được status nữa, chỉ ghi log.
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

// translate chuyển lỗi domain thành lỗi API.
//
// ErrNotFound của domain là chuyện bình thường (khách gõ sai URL), không
// phải sự cố — nó phải thành 404 chứ không phải 500.
func translate(err error, notFoundMsg string) error {
	if errors.Is(err, domain.ErrNotFound) {
		return apierror.New(apierror.CodeNotFound, notFoundMsg)
	}
	return apierror.From(err)
}
