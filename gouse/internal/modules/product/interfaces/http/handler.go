// Package http là tầng interfaces của module product: chuyển HTTP request
// thành lời gọi use case và chuyển kết quả thành JSON đúng đặc tả OpenAPI.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Nếu một điều kiện `if` ở đây quyết
// định điều gì được phép về mặt nghiệp vụ, nó đã đặt sai chỗ.
package http

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/application"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Handler phục vụ các endpoint product công khai.
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
// Đường dẫn khớp CHÍNH XÁC với api/openapi.yaml.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/products/{product_id}", http.HandlerFunc(h.getProduct))
	mux.Handle("GET /api/v1/products", http.HandlerFunc(h.listProducts))
	mux.Handle("GET /api/v1/search", http.HandlerFunc(h.search))
}

// searchResponse khớp đặc tả storefront.yaml#/search.
//
// CHỈ có `products`: `brands`, `creators`, `content` trong đặc tả thuộc các
// module chưa tồn tại (creator và content là Phase 2). Trả mảng rỗng cho
// chúng sẽ là nói dối rằng đã tìm và không thấy gì.
type searchResponse struct {
	Products []productSummary `json:"products"`
}

// search phục vụ GET /api/v1/search (operationId: search).
//
// # Không ra kết quả được GHI LẠI
//
// Đây là tín hiệu NHU CẦU KHÔNG ĐƯỢC ĐÁP ỨNG — thứ dữ liệu bán hàng một
// mình không bao giờ cho biết, và không tạo ngược được nếu hôm nay không
// ghi. Việc ghi chạy bất đồng bộ nên không làm chậm phản hồi cho khách.
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	query := strings.TrimSpace(q.Get("q"))
	if query == "" {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"q là tham số bắt buộc"))
		return
	}

	limit := 20
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"limit phải là số nguyên từ 1 đến 100"))
			return
		}
		limit = n
	}

	found, err := h.svc.Search(r.Context(), query, limit, 0)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	out := make([]productSummary, 0, len(found))
	for _, p := range found {
		out = append(out, toProductSummary(p))
	}

	h.ok(w, r, searchResponse{Products: out})
}

// getProduct phục vụ GET /api/v1/products/{product_id} (operationId: getProduct).
func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id, err := ids.Parse(r.PathValue("product_id"), ids.PrefixProduct)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"product_id không đúng định dạng"))
		return
	}

	p, err := h.svc.GetProduct(r.Context(), id)
	if err != nil {
		h.fail(w, r, translate(err, "Không tìm thấy sản phẩm"))
		return
	}

	// Sản phẩm chưa duyệt trả 404, KHÔNG phải 403.
	//
	// 403 xác nhận tài nguyên tồn tại — đủ để đối thủ dò id và biết chúng
	// ta đang chuẩn bị bán gì.
	if !p.IsVisibleToCustomer() {
		h.fail(w, r, apierror.New(apierror.CodeNotFound, "Không tìm thấy sản phẩm"))
		return
	}

	h.ok(w, r, toProductDetail(p))
}

// listProducts phục vụ GET /api/v1/products.
//
// CHỈ trả sản phẩm đang hiển thị. Bộ lọc theo trạng thái KHÔNG nhận từ query
// string: cho phép `?status=DRAFT` trên endpoint công khai sẽ lộ toàn bộ
// hàng chưa duyệt của mọi seller.
func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// `ids` là đường TRA THEO LÔ, không phải một bộ lọc như những cái khác.
	//
	// # Vì sao nó cần tồn tại
	//
	// Nhiều trang có một danh sách MÃ sản phẩm mà không có tên hay ảnh —
	// danh sách yêu thích là ví dụ rõ nhất: module `customer` nằm cùng tầng
	// với `product` nên không gọi được, và nó chỉ trả `product_id`.
	//
	// Không có đường này thì trang phải gọi `getProduct` cho từng mã. Danh
	// sách 30 món = 30 lượt đi-về — đúng vấn đề N+1 mà `GetProductsByIDs`
	// sinh ra để tránh.
	//
	// Trả về theo ĐÚNG THỨ TỰ mã được hỏi: bên gọi đã sắp xếp danh sách của
	// họ (yêu thích theo thời gian thêm), và trả về thứ tự khác buộc họ sắp
	// lại — hoặc tệ hơn, họ không nhận ra và hiển thị sai thứ tự.
	if raw := q.Get("ids"); raw != "" {
		h.listByIDs(w, r, raw)
		return
	}

	f := domain.Filter{OnlyVisible: true, Limit: 50}

	if raw := q.Get("brand_id"); raw != "" {
		id, err := ids.Parse(raw, ids.PrefixBrand)
		if err != nil {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"brand_id không đúng định dạng"))
			return
		}
		f.BrandID = id
	}
	if raw := q.Get("category_id"); raw != "" {
		id, err := ids.Parse(raw, ids.PrefixCategory)
		if err != nil {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"category_id không đúng định dạng"))
			return
		}
		f.CategoryID = id
	}
	if raw := q.Get("collection_id"); raw != "" {
		id, err := ids.Parse(raw, ids.PrefixCollection)
		if err != nil {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"collection_id không đúng định dạng"))
			return
		}
		f.CollectionID = id
	}

	// Bộ lọc theo THUỘC TÍNH biến thể và theo sản phẩm.
	//
	// Đây là cách người ta mua quần áo: lọc size mình mặc vừa và màu mình
	// thích, rồi mới nhìn. Danh mục không có hai bộ lọc này bắt khách mở
	// từng sản phẩm để biết có mua được không.
	f.Sizes = tachDanhSach(q.Get("size"))
	f.ColorFamilies = tachDanhSach(q.Get("color"))

	if raw := strings.TrimSpace(q.Get("gender_target")); raw != "" {
		f.Gender = domain.GenderTarget(strings.ToUpper(raw))
	}
	if raw := strings.TrimSpace(q.Get("product_type")); raw != "" {
		f.ProductType = domain.ProductType(strings.ToUpper(raw))
	}

	list, err := h.svc.ListProducts(r.Context(), f)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	out := make([]productSummary, 0, len(list))
	for _, p := range list {
		out = append(out, toProductSummary(p))
	}
	h.ok(w, r, productListResponse{Data: out})
}

// maxBatchIDs giới hạn số mã tra một lần.
//
// Không giới hạn thì một request kéo cả bảng sản phẩm — và mệnh đề IN với
// hàng nghìn phần tử là truy vấn chậm mà không chỉ mục nào cứu được.
const maxBatchIDs = 100

// listByIDs tra nhiều sản phẩm theo mã, trong MỘT truy vấn.
func (h *Handler) listByIDs(w http.ResponseWriter, r *http.Request, raw string) {
	parts := strings.Split(raw, ",")
	if len(parts) > maxBatchIDs {
		h.fail(w, r, apierror.Newf(apierror.CodeValidationFailed,
			"tối đa %d mã mỗi lần", maxBatchIDs))
		return
	}

	wanted := make([]ids.ID, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := ids.Parse(p, ids.PrefixProduct)
		if err != nil {
			h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
				"ids chứa mã sản phẩm không đúng định dạng"))
			return
		}
		wanted = append(wanted, id)
	}

	found, err := h.svc.GetProductsByIDs(r.Context(), wanted)
	if err != nil {
		h.fail(w, r, apierror.From(err))
		return
	}

	out := make([]productSummary, 0, len(wanted))
	for _, id := range wanted {
		p, ok := found[id]
		if !ok {
			// Mã không tồn tại thì VẮNG MẶT, không phải một ô rỗng. Bên
			// gọi thấy danh sách ngắn hơn và tự hiểu — trả về phần tử rỗng
			// buộc mọi vòng lặp phải kiểm tra null.
			continue
		}
		// Sản phẩm chưa duyệt KHÔNG lọt ra đường công khai, kể cả khi bên
		// gọi hỏi đích danh mã của nó.
		if !p.IsVisibleToCustomer() {
			continue
		}
		out = append(out, toProductSummary(p))
	}

	h.ok(w, r, productListResponse{Data: out})
}

// ---------------------------------------------------------------- Chuyển đổi

func toProductDetail(p *domain.Product) productDetail {
	out := productDetail{
		ID:                  p.ID().String(),
		Name:                p.Name(),
		Slug:                p.Slug(),
		Description:         p.Description(),
		CategoryID:          p.CategoryID().String(),
		ProductType:         string(p.Type()),
		GenderTarget:        string(p.GenderTarget()),
		MaterialComposition: p.MaterialComposition(),
		CareInstructions:    p.CareInstructions(),
		OriginCountry:       p.OriginCountry(),
		Images:              toImages(p.Images()),
		Variants:            toVariants(p.Variants()),
	}

	// Tên thương hiệu và bộ sưu tập thuộc module catalog. Ở đây chỉ trả
	// định danh; tầng BFF hoặc client ghép thêm nếu cần.
	//
	// Gọi catalog từ tầng interfaces sẽ tạo phụ thuộc chéo khó gỡ và làm
	// mỗi request sản phẩm kéo theo một request catalog.
	if !p.BrandID().IsZero() {
		out.Brand = &brandRef{ID: p.BrandID().String()}
	}
	if !p.CollectionID().IsZero() {
		out.Collection = &collectionRef{ID: p.CollectionID().String()}
	}

	return out
}

func toProductSummary(p *domain.Product) productSummary {
	out := productSummary{
		ID:   p.ID().String(),
		Name: p.Name(),
		Slug: p.Slug(),
	}
	if !p.BrandID().IsZero() {
		out.Brand = &brandRef{ID: p.BrandID().String()}
	}
	if imgs := p.Images(); len(imgs) > 0 {
		out.PrimaryImageURL = imgs[0]
	}

	// Gom màu có sẵn, không trùng lặp và có thứ tự ổn định.
	seen := make(map[string]struct{})
	for _, v := range p.Variants() {
		c := v.Color()
		if c == "" {
			continue
		}
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out.AvailableColors = append(out.AvailableColors, c)
	}
	sort.Strings(out.AvailableColors)

	return out
}

func toImages(urls []string) []image {
	out := make([]image, 0, len(urls))
	for i, u := range urls {
		out = append(out, image{URL: u, Order: i})
	}
	return out
}

func toVariants(list []*domain.Variant) []variant {
	out := make([]variant, 0, len(list))
	for _, v := range list {
		// Biến thể đã tạm ngừng không hiển thị cho khách.
		if v.Status() != domain.StatusActive {
			continue
		}
		out = append(out, variant{
			ID:     v.ID().String(),
			Color:  v.Color(),
			Images: toImages(v.Images()),
			// Size là thuộc tính của BIẾN THỂ, nhưng client cần nó ở mức
			// SKU để dựng danh sách size chọn được — nên truyền xuống.
			SKUs: toSKUSummaries(v.SKUs(), v.Size()),
		})
	}
	return out
}

func toSKUSummaries(list []*domain.SKU, size string) []skuSummary {
	out := make([]skuSummary, 0, len(list))
	for _, s := range list {
		// SKU đã ngừng kinh doanh không hiển thị.
		//
		// Khác với HẾT HÀNG: size hết hàng vẫn hiển thị (gạch chéo, cho
		// đăng ký nhận thông báo), nhưng mặt hàng đã ngừng kinh doanh thì
		// không bao giờ về nữa.
		if !s.IsSellable() {
			continue
		}
		out = append(out, skuSummary{
			ID:      s.ID().String(),
			SKUCode: s.Code(),
			Size:    size,
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

// maxGiaTriLoc chặn một bộ lọc kéo cả bảng.
//
// 20 rộng hơn nhiều so với nhu cầu thật: không ai lọc cùng lúc 20 size.
const maxGiaTriLoc = 20

// tachDanhSach tách chuỗi "M,L,XL" thành danh sách.
//
// Cắt ở maxGiaTriLoc thay vì báo lỗi: bộ lọc quá dài là lỗi của giao diện,
// không phải của khách, và trả lỗi cho họ không giúp được gì.
func tachDanhSach(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
		if len(out) == maxGiaTriLoc {
			break
		}
	}
	return out
}
