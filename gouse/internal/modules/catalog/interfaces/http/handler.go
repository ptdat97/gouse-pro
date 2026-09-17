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
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/application"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
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

// RegisterSellerRoutes gắn route CẦN ĐĂNG NHẬP của nhà bán.
//
// Tách khỏi `Register` vì hai nhóm có ranh giới bảo mật khác nhau: nhóm
// trên ai cũng gọi được, nhóm này phải qua `Auth` + `RequireRole`. Đăng ký
// chung một mux nghĩa là route nhà bán chạy KHÔNG có AuthContext — và khi
// ấy nó trả 401 cho cả token hợp lệ, vì không ai đặt context vào.
func (h *Handler) RegisterSellerRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/seller/brands", http.HandlerFunc(h.thuongHieuChoBan))
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

// ------------------------------------------------ Thương hiệu của nhà bán

// thuongHieuChoBanJSON là một thương hiệu gian hàng ĐƯỢC PHÉP đăng bán.
type thuongHieuChoBanJSON struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Slug            string `json:"slug,omitempty"`
	LogoURL         string `json:"logo_url,omitempty"`
	ProtectionLevel string `json:"protection_level"`
}

// thuongHieuChoBan phục vụ GET /api/v1/seller/brands
// (operationId: listBrandsIMaySell).
//
// # Vì sao endpoint này phải tồn tại
//
// `POST /api/v1/seller/products` đòi `brand_id` và TỪ CHỐI mọi thương hiệu
// gian hàng không được phép bán — hàng rào chống hàng giả, đúng như nó cần
// phải thế. Nhưng cho tới 17/09/2026 không có đường nào để BIẾT mình được
// phép bán thương hiệu nào: `GET /api/v1/brands/{brand_id}` chỉ tra từng
// cái một, theo id.
//
// Hệ quả là luồng đăng sản phẩm không hoàn thành được qua giao diện —
// không phải vì thiếu endpoint GHI, mà vì thiếu endpoint ĐỌC để điền vào
// biểu mẫu. Nhà bán phải tự đâu đó có một ULID.
//
// # Lọc bằng CHÍNH quy tắc của đường ghi
//
// Danh sách này gọi `CanSellerSellBrand` — cùng hàm mà `CreateProduct` gọi
// — thay vì viết lại điều kiện. Viết lại nghĩa là hai bản sao của một quy
// tắc chống hàng giả, và chúng sẽ lệch: biểu mẫu mời chọn một thương hiệu
// rồi đường ghi từ chối nó.
func (h *Handler) thuongHieuChoBan(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	bs, err := h.svc.ListBrands(r.Context(), domain.BrandFilter{
		Status: domain.StatusActive,
		Limit:  brandLimit,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}

	out := make([]thuongHieuChoBanJSON, 0, len(bs))
	for _, b := range bs {
		kq, err := h.svc.CanSellerSellBrand(r.Context(), b.ID(), sellerID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if !kq.Allowed {
			continue
		}
		out = append(out, thuongHieuChoBanJSON{
			ID:              b.ID().String(),
			Name:            b.Name(),
			Slug:            b.Slug(),
			LogoURL:         b.LogoURL(),
			ProtectionLevel: string(kq.ProtectionLevel),
		})
	}

	h.ok(w, r, map[string]any{"data": out})
}

// brandLimit chặn số thương hiệu một lần đọc trả về.
//
// Endpoint này kiểm quyền cho TỪNG thương hiệu, nên không có trần thì một
// danh mục lớn biến một request thành hàng nghìn lượt tra.
const brandLimit = 200

// sellerID lấy gian hàng từ token.
//
// Cùng cách các module khác làm: định danh gian hàng KHÔNG nhận từ tham số,
// vì cho client truyền vào nghĩa là bất kỳ ai cũng xem được dữ liệu gian
// hàng khác chỉ bằng cách đổi một chuỗi.
func (h *Handler) sellerID(r *http.Request) (ids.ID, error) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		return "", apierror.ErrUnauthorized
	}
	if len(ac.SellerIDs) == 0 {
		return "", apierror.New(apierror.CodeForbidden,
			"Tài khoản này không gắn với nhà bán nào")
	}
	return ids.ID(strings.TrimSpace(ac.SellerIDs[0])), nil
}
