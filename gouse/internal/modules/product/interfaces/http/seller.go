// Tầng interfaces cho mặt GHI của product — P3-25.
//
// Hai nhóm người dùng, hai handler, và chúng KHÔNG dùng chung route:
//
//	nhà bán      tạo sản phẩm của MÌNH, gửi duyệt
//	vận hành     xem hàng chờ duyệt, duyệt hoặc từ chối
//
// Tách vì ranh giới bảo mật khác nhau. Mọi thao tác của nhà bán đều kẹp
// theo `seller_id` LẤY TỪ TOKEN, không phải từ thân request — nhận từ thân
// nghĩa là nhà bán A tạo được sản phẩm đứng tên nhà bán B chỉ bằng cách
// sửa một dòng JSON.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/application"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// SellerHandler phục vụ các endpoint GHI của nhà bán.
type SellerHandler struct {
	svc *application.Service
	log *slog.Logger
}

// NewSellerHandler nhận tầng application, KHÔNG nhận module.
//
// Tầng interfaces không được import module gốc (quy tắc R8 của archcheck):
// module gốc lại import tầng này để đăng ký route, nên nhận module ở đây
// tạo vòng phụ thuộc.
func NewSellerHandler(svc *application.Service, log *slog.Logger) *SellerHandler {
	return &SellerHandler{svc: svc, log: log}
}

// Register gắn route vào mux.
//
// Mux truyền vào PHẢI đã bọc Auth và RequireRole("SELLER_OWNER",
// "SELLER_STAFF") — cùng ràng buộc với đơn thực hiện phía nhà bán.
func (h *SellerHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/seller/products", http.HandlerFunc(h.danhSach))
	mux.Handle("POST /api/v1/seller/products", http.HandlerFunc(h.tao))
	mux.Handle("POST /api/v1/seller/products/{product_id}/variants",
		http.HandlerFunc(h.themBienThe))
	mux.Handle("PATCH /api/v1/seller/products/{product_id}", http.HandlerFunc(h.suaNhap))
	mux.Handle("POST /api/v1/seller/products/{product_id}/submit",
		http.HandlerFunc(h.guiDuyet))
}

type taoSanPhamBody struct {
	BrandID      string `json:"brand_id"`
	CollectionID string `json:"collection_id"`
	CategoryID   string `json:"category_id"`
	SizeChartID  string `json:"size_chart_id"`

	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`

	CareInstructions    string `json:"care_instructions"`
	MaterialComposition string `json:"material_composition"`
	OriginCountry       string `json:"origin_country"`

	ProductType  string `json:"product_type"`
	GenderTarget string `json:"gender_target"`

	Images []string `json:"images"`
}

// tao phục vụ POST /api/v1/seller/products.
//
// Sản phẩm sinh ra ở DRAFT, chưa ai thấy. Nó chỉ ra cửa hàng sau khi qua
// duyệt — xem docs/07-workflows/product-publishing.md mục 4.
func (h *SellerHandler) tao(w http.ResponseWriter, r *http.Request) {
	var body taoSanPhamBody
	if err := decodeJSON(r, &body); err != nil {
		h.fail(w, r, err)
		return
	}

	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	in := application.CreateProductInput{
		SellerID:            sellerID,
		Name:                strings.TrimSpace(body.Name),
		Slug:                strings.TrimSpace(body.Slug),
		Description:         strings.TrimSpace(body.Description),
		CareInstructions:    strings.TrimSpace(body.CareInstructions),
		MaterialComposition: strings.TrimSpace(body.MaterialComposition),
		OriginCountry:       strings.TrimSpace(body.OriginCountry),
		ProductType:         domain.ProductType(body.ProductType),
		GenderTarget:        domain.GenderTarget(body.GenderTarget),
		Images:              body.Images,
	}
	if err := docDinhDanh(&in, body); err != nil {
		h.fail(w, r, err)
		return
	}

	p, err := h.svc.CreateProduct(r.Context(), in)
	if err != nil {
		h.fail(w, r, dichLoiGhi(err))
		return
	}
	h.ok(w, r, http.StatusCreated, toSanPhamNhaBan(p))
}

type themBienTheBody struct {
	Attributes map[string]string `json:"attributes"`
	Images     []string          `json:"images"`
	SKUs       []struct {
		Code       string `json:"sku_code"`
		Barcode    string `json:"barcode"`
		WeightGram int    `json:"weight_gram"`
		LengthMM   int    `json:"length_mm"`
		WidthMM    int    `json:"width_mm"`
		HeightMM   int    `json:"height_mm"`
	} `json:"skus"`
}

func (h *SellerHandler) themBienThe(w http.ResponseWriter, r *http.Request) {
	var body themBienTheBody
	if err := decodeJSON(r, &body); err != nil {
		h.fail(w, r, err)
		return
	}

	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	skus := make([]application.NewSKUInput, 0, len(body.SKUs))
	for _, s := range body.SKUs {
		skus = append(skus, application.NewSKUInput{
			Code:       strings.TrimSpace(s.Code),
			Barcode:    strings.TrimSpace(s.Barcode),
			WeightGram: s.WeightGram,
			Dimensions: domain.Dimensions{
				LengthMM: s.LengthMM, WidthMM: s.WidthMM, HeightMM: s.HeightMM,
			},
		})
	}

	pid, err := ids.Parse(r.PathValue("product_id"), ids.PrefixProduct)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Mã sản phẩm không hợp lệ"))
		return
	}

	p, err := h.svc.AddVariant(r.Context(), application.AddVariantInput{
		ProductID:  pid,
		SellerID:   sellerID,
		Attributes: body.Attributes,
		Images:     body.Images,
		SKUs:       skus,
	})
	if err != nil {
		h.fail(w, r, dichLoiGhi(err))
		return
	}
	h.ok(w, r, http.StatusOK, toSanPhamNhaBan(p))
}

// suaNhapBody: mọi trường là CON TRỎ.
//
// JSON vắng trường → nil → GIỮ NGUYÊN. JSON có trường, kể cả `""` → con
// trỏ tới giá trị ấy → ĐỔI (và `""` với tên hay slug là lỗi, không phải
// "bỏ qua"). Dùng kiểu giá trị thay vì con trỏ thì hai trường hợp này
// trông giống hệt nhau, và một PATCH chỉ đổi chất liệu sẽ xóa mất mô tả.
type suaNhapBody struct {
	Name                *string   `json:"name"`
	Slug                *string   `json:"slug"`
	Description         *string   `json:"description"`
	CareInstructions    *string   `json:"care_instructions"`
	MaterialComposition *string   `json:"material_composition"`
	OriginCountry       *string   `json:"origin_country"`
	CategoryID          *string   `json:"category_id"`
	SizeChartID         *string   `json:"size_chart_id"`
	ProductType         *string   `json:"product_type"`
	GenderTarget        *string   `json:"gender_target"`
	Images              *[]string `json:"images"`

	// BrandID CÓ trong thân để TỪ CHỐI RÕ RÀNG, không phải để dùng.
	//
	// Bỏ hẳn nó thì `DisallowUnknownFields` vẫn chặn, nhưng bằng câu chung
	// chung "Dữ liệu gửi lên không hợp lệ" — nhà bán không biết vì sao. Xem
	// `domain.SuaNhapParams` cho lý do thương hiệu không sửa được.
	BrandID *string `json:"brand_id"`
}

// suaNhap phục vụ PATCH /api/v1/seller/products/{product_id}
// (operationId: updateMyProduct).
//
// # Vì sao endpoint này tồn tại
//
// Cho tới 19/09/2026 không có đường nào sửa một sản phẩm sau khi tạo. Tạo
// thiếu ảnh là tạo một bản ghi KHÔNG BAO GIỜ gửi duyệt được và KHÔNG BAO
// GIỜ sửa được; gõ sai tên cũng vậy. Và bị kiểm duyệt trả về kèm lý do thì
// không sửa theo lý do ấy được — `Reject` đưa về DRAFT, nhưng DRAFT không
// có cửa nào để vào. Xem P3-64.
func (h *SellerHandler) suaNhap(w http.ResponseWriter, r *http.Request) {
	var body suaNhapBody
	if err := decodeJSON(r, &body); err != nil {
		h.fail(w, r, err)
		return
	}
	if body.BrandID != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Không đổi được thương hiệu của sản phẩm đã tạo — tạo sản phẩm "+
				"mới dưới thương hiệu đúng. Thương hiệu được kiểm quyền bán "+
				"lúc tạo; cho đổi ở đây là mở cửa sau qua hàng rào chống hàng giả."))
		return
	}

	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	pid, err := ids.Parse(r.PathValue("product_id"), ids.PrefixProduct)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Mã sản phẩm không hợp lệ"))
		return
	}

	in := domain.SuaNhapParams{
		Name:                body.Name,
		Slug:                body.Slug,
		Description:         body.Description,
		CareInstructions:    body.CareInstructions,
		MaterialComposition: body.MaterialComposition,
		OriginCountry:       body.OriginCountry,
		Images:              body.Images,
	}
	if body.ProductType != nil {
		v := domain.ProductType(*body.ProductType)
		in.ProductType = &v
	}
	if body.GenderTarget != nil {
		v := domain.GenderTarget(*body.GenderTarget)
		in.GenderTarget = &v
	}
	for _, t := range []struct {
		raw    *string
		prefix ids.Prefix
		ten    string
		dich   **ids.ID
	}{
		{body.CategoryID, ids.PrefixCategory, "category_id", &in.CategoryID},
		{body.SizeChartID, ids.PrefixSizeChart, "size_chart_id", &in.SizeChartID},
	} {
		if t.raw == nil {
			continue
		}
		// Chuỗi rỗng đi thẳng xuống miền dưới dạng mã RỖNG: miền phân biệt
		// "bỏ danh mục" (lỗi) với "không nhắc tới danh mục" (giữ nguyên).
		var id ids.ID
		if strings.TrimSpace(*t.raw) != "" {
			id, err = ids.Parse(*t.raw, t.prefix)
			if err != nil {
				h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
					t.ten+" không đúng định dạng"))
				return
			}
		}
		*t.dich = &id
	}

	p, err := h.svc.SuaNhapCuaNhaBan(r.Context(), sellerID, pid, in)
	if err != nil {
		h.fail(w, r, dichLoiGhi(err))
		return
	}
	h.ok(w, r, http.StatusOK, toSanPhamNhaBan(p))
}

// guiDuyet phục vụ POST /api/v1/seller/products/{product_id}/submit.
//
// Thiếu điều kiện thì trả lỗi NÓI RÕ thiếu gì (thiếu ảnh, thiếu chất
// liệu…). Một thông báo chung chung ở đây làm nhà bán bỏ dở việc đăng bán
// vì không biết sửa chỗ nào.
func (h *SellerHandler) guiDuyet(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	pid, err := ids.Parse(r.PathValue("product_id"), ids.PrefixProduct)
	if err != nil {
		h.fail(w, r, apierror.New(apierror.CodeValidationFailed,
			"Mã sản phẩm không hợp lệ"))
		return
	}

	p, err := h.svc.SubmitForReviewOwned(r.Context(), sellerID, pid)
	if err != nil {
		h.fail(w, r, dichLoiGhi(err))
		return
	}
	h.ok(w, r, http.StatusOK, toSanPhamNhaBan(p))
}

func (h *SellerHandler) danhSach(w http.ResponseWriter, r *http.Request) {
	sellerID, err := h.sellerID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	// Bộ lọc trạng thái nhận từ query, KHÁC endpoint công khai: ở đây nhà
	// bán ĐƯỢC xem hàng nháp của chính mình, và câu truy vấn luôn kẹp
	// seller_id nên không lộ hàng của gian khác.
	ds, err := h.svc.ListSellerProducts(r.Context(), sellerID,
		domain.Status(strings.TrimSpace(r.URL.Query().Get("status"))))
	if err != nil {
		h.fail(w, r, dichLoiGhi(err))
		return
	}

	out := make([]sanPhamNhaBan, 0, len(ds))
	for _, p := range ds {
		out = append(out, toSanPhamNhaBan(p))
	}
	h.ok(w, r, http.StatusOK, map[string]any{"data": out})
}

func (h *SellerHandler) sellerID(r *http.Request) (ids.ID, error) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		h.log.ErrorContext(r.Context(),
			"seller product chạy không qua Auth — kiểm tra nối dây")
		return "", apierror.ErrUnauthorized
	}
	if len(ac.SellerIDs) == 0 {
		return "", apierror.New(apierror.CodeForbidden,
			"Tài khoản này không gắn với nhà bán nào")
	}
	return ids.ID(ac.SellerIDs[0]), nil
}

func (h *SellerHandler) ok(w http.ResponseWriter, r *http.Request, status int, body any) {
	if err := apierror.WriteJSON(w, status, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *SellerHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

// sanPhamNhaBan là sản phẩm nhìn từ phía gian hàng.
//
// KHÁC `productDetail` của cửa hàng: có `status` và `rejection_reason` —
// hai thứ khách KHÔNG được thấy, và là hai thứ nhà bán cần nhất.
type sanPhamNhaBan struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug,omitempty"`
	Status string `json:"status"`

	// RejectionReason: vì sao bị từ chối. Rỗng khi chưa từng bị từ chối.
	RejectionReason string `json:"rejection_reason,omitempty"`

	BrandID     string `json:"brand_id,omitempty"`
	ProductType string `json:"product_type,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`

	// Các trường SỬA ĐƯỢC — trả ra để biểu mẫu sửa điền sẵn giá trị hiện
	// tại. Không có chúng thì mọi lần sửa bắt đầu từ ô trống, và một nhà
	// bán chỉ muốn thêm một ảnh sẽ phải gõ lại toàn bộ mô tả — hoặc tệ hơn,
	// gửi lên ô trống và xóa mất nó.
	//
	// Cùng dạng với `variants` ngay dưới: dữ liệu đã nạp sẵn, trước đây bị
	// vứt ở tầng DTO.
	Description         string   `json:"description"`
	CareInstructions    string   `json:"care_instructions"`
	MaterialComposition string   `json:"material_composition"`
	OriginCountry       string   `json:"origin_country"`
	CategoryID          string   `json:"category_id,omitempty"`
	GenderTarget        string   `json:"gender_target,omitempty"`
	Images              []string `json:"images"`

	// Variants KHÔNG dùng omitempty: mảng rỗng và thiếu trường nói hai
	// chuyện khác nhau. `[]` = sản phẩm chưa có biến thể nào, tức CHƯA gửi
	// duyệt được; thiếu trường = không biết.
	//
	// Dữ liệu này đã được nạp sẵn: `queryMany` gọi `loadVariants` cho mọi
	// truy vấn danh sách. Trước 18/09/2026 nó bị vứt ở đúng đây — cùng
	// hình dạng với `shipping_groups` ở P3-50, và hệ quả cũng cùng một
	// kiểu: nhà bán thêm biến thể xong không có cách nào thấy mình đã thêm
	// gì, nên hoặc thêm trùng hoặc bỏ dở.
	Variants []bienTheNhaBan `json:"variants"`
}

// bienTheNhaBan là một biến thể nhìn từ phía gian hàng.
//
// Chỉ những trường nhà bán cần để biết mình ĐÃ THÊM GÌ và CÒN THIẾU GÌ.
// Không trả giá và tồn kho: cả hai thuộc module khác (`pricing`,
// `inventory`) và nhét chúng vào đây sẽ biến một lượt đọc sản phẩm thành
// ba lượt gọi liên module.
type bienTheNhaBan struct {
	ID string `json:"id"`

	// Attributes là tổ hợp làm nên biến thể: màu, size…
	Attributes map[string]string `json:"attributes"`

	Images []string `json:"images"`

	// SKUs: mỗi biến thể có ít nhất một mã bán hàng.
	SKUs []skuNhaBan `json:"skus"`
}

type skuNhaBan struct {
	ID   string `json:"id"`
	Code string `json:"sku_code"`
}

func toBienTheNhaBan(vs []*domain.Variant) []bienTheNhaBan {
	out := make([]bienTheNhaBan, 0, len(vs))
	for _, v := range vs {
		skus := make([]skuNhaBan, 0, len(v.SKUs()))
		for _, s := range v.SKUs() {
			skus = append(skus, skuNhaBan{ID: s.ID().String(), Code: s.Code()})
		}
		out = append(out, bienTheNhaBan{
			ID:         v.ID().String(),
			Attributes: v.Attributes(),
			Images:     v.Images(),
			SKUs:       skus,
		})
	}
	return out
}

func toSanPhamNhaBan(p *domain.Product) sanPhamNhaBan {
	return sanPhamNhaBan{
		ID:              p.ID().String(),
		Name:            p.Name(),
		Slug:            p.Slug(),
		Status:          string(p.Status()),
		RejectionReason: p.RejectionReason(),
		BrandID:         p.BrandID().String(),
		ProductType:     string(p.Type()),
		CreatedAt:       p.CreatedAt().UTC().Format(time.RFC3339),
		Variants:        toBienTheNhaBan(p.Variants()),

		Description:         p.Description(),
		CareInstructions:    p.CareInstructions(),
		MaterialComposition: p.MaterialComposition(),
		OriginCountry:       p.OriginCountry(),
		CategoryID:          p.CategoryID().String(),
		GenderTarget:        string(p.GenderTarget()),
		Images:              mangRong(p.Images()),
	}
}

// docDinhDanh đọc bốn mã tham chiếu từ thân request.
//
// Sai định dạng thì TỪ CHỐI chứ không âm thầm bỏ qua: bỏ qua nghĩa là sản
// phẩm gắn nhầm danh mục mà người tạo tưởng đã gắn đúng.
func docDinhDanh(in *application.CreateProductInput, body taoSanPhamBody) error {
	brandID, err := ids.Parse(body.BrandID, ids.PrefixBrand)
	if err != nil {
		return apierror.New(apierror.CodeValidationFailed,
			"brand_id là trường bắt buộc và phải đúng định dạng")
	}
	in.BrandID = brandID

	for _, t := range []struct {
		raw    string
		prefix ids.Prefix
		ten    string
		dich   *ids.ID
	}{
		{body.CollectionID, ids.PrefixCollection, "collection_id", &in.CollectionID},
		{body.CategoryID, ids.PrefixCategory, "category_id", &in.CategoryID},
		{body.SizeChartID, ids.PrefixSizeChart, "size_chart_id", &in.SizeChartID},
	} {
		if strings.TrimSpace(t.raw) == "" {
			continue
		}
		id, err := ids.Parse(t.raw, t.prefix)
		if err != nil {
			return apierror.New(apierror.CodeValidationFailed,
				t.ten+" không đúng định dạng")
		}
		*t.dich = id
	}
	return nil
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

// maxLimit là số bản ghi TỐI ĐA một lần đọc trả về.
//
// Trần nằm ở tầng HTTP vì `limitOr` của tầng kho chỉ xử lý cận DƯỚI. Không
// có nó thì `?limit=999999` kéo cả bảng về trong một request — và trang
// quản trị là nơi dễ bị gọi như vậy nhất, vì dữ liệu ở đó không lọc theo
// gian hàng nào.
const maxLimit = 100

// gioiHanTu đọc `limit` từ query, kẹp giữa 1 và `tran`.
//
// `tran` truyền vào chứ không lấy từ hằng số bên trong: chỗ gọi phải NHÌN
// THẤY cận trên. Giấu nó trong helper nghĩa là người đọc endpoint không
// biết có trần, và bài test phủ `TestMoiChoDocLimitDeuCoTran` quét đúng
// điều đó — nó đọc 25 dòng quanh chỗ đọc limit.
func gioiHanTu(q string, macDinh, tran int) int {
	n, err := strconv.Atoi(strings.TrimSpace(q))
	if err != nil || n <= 0 {
		return macDinh
	}
	if n > tran {
		return tran
	}
	return n
}

// dichLoiGhi chuyển lỗi của mặt GHI thành lỗi API.
//
// # Vì sao mỗi điều kiện thiếu có thông báo RIÊNG
//
// `CheckReadyForReview` kiểm năm thứ. Gộp tất cả vào "dữ liệu không hợp
// lệ" là bắt nhà bán đoán xem thiếu gì, và đoán sai vài lần thì họ bỏ dở
// việc đăng bán — chú thích của chính hàm đó đã nói ra điều này.
func dichLoiGhi(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		// Sản phẩm không tồn tại HOẶC của gian hàng khác — cùng một mã.
		// Phân biệt hai trường hợp cho phép dò mã sản phẩm chưa phát hành
		// của đối thủ.
		return apierror.New(apierror.CodeNotFound, "Không tìm thấy sản phẩm")

	case errors.Is(err, application.ErrNotAuthorized):
		// HÀNG RÀO CHỐNG HÀNG GIẢ. Lý do đi kèm để giao diện chỉ được
		// hành động cụ thể ("Tải lên giấy ủy quyền") thay vì báo chung.
		return apierror.New(apierror.CodeForbidden, "Gian hàng chưa được phép bán thương hiệu này: "+err.Error())

	case errors.Is(err, application.ErrCategoryNotFound):
		return apierror.New(apierror.CodeValidationFailed, "Danh mục không tồn tại")
	case errors.Is(err, application.ErrBrandNotFound):
		return apierror.New(apierror.CodeValidationFailed, "Thương hiệu không tồn tại")

	case errors.Is(err, domain.ErrMissingDescription):
		return apierror.New(apierror.CodeValidationFailed, "Sản phẩm chưa có mô tả")
	case errors.Is(err, domain.ErrNoImages):
		return apierror.New(apierror.CodeValidationFailed, "Sản phẩm chưa có ảnh nào")
	case errors.Is(err, domain.ErrNoVariants):
		return apierror.New(apierror.CodeValidationFailed, "Sản phẩm chưa có biến thể nào")
	case errors.Is(err, domain.ErrMissingSizeChart):
		return apierror.New(apierror.CodeValidationFailed, "Sản phẩm thời trang phải có bảng size")
	case errors.Is(err, domain.ErrMissingMaterial):
		return apierror.New(apierror.CodeValidationFailed, "Sản phẩm chưa khai thành phần chất liệu")
	case errors.Is(err, domain.ErrMissingBrand):
		return apierror.New(apierror.CodeValidationFailed, "Sản phẩm phải thuộc một thương hiệu")
	case errors.Is(err, domain.ErrMissingCategory):
		// Miền ĐÒI danh mục, tầng HTTP lại coi `category_id` là tùy chọn.
		// Chênh lệch ấy khiến mọi lần tạo sản phẩm thiếu danh mục trả về
		// 500 "vui lòng thử lại" — một lời khuyên không bao giờ đúng, vì
		// thử lại y hệt sẽ hỏng y hệt. Xem P3-63.
		return apierror.New(apierror.CodeValidationFailed,
			"Sản phẩm phải thuộc một danh mục — thiếu category_id")

	// Hai lỗi dưới đây TỪNG là `errors.New` tại chỗ — xem product.go.
	case errors.Is(err, domain.ErrInvalidProductType):
		return apierror.New(apierror.CodeValidationFailed,
			"Loại sản phẩm không hợp lệ — cần một trong TOP, BOTTOM, DRESS, "+
				"OUTERWEAR, SHOES, BAG, ACCESSORY")
	case errors.Is(err, domain.ErrInvalidGender):
		return apierror.New(apierror.CodeValidationFailed,
			"Đối tượng khách không hợp lệ — cần một trong WOMEN, MEN, UNISEX, KIDS")
	case errors.Is(err, domain.ErrEmptyImageURL):
		return apierror.New(apierror.CodeValidationFailed,
			"Có một đường dẫn ảnh bị rỗng")
	case errors.Is(err, domain.ErrEmptyRejectReason):
		return apierror.New(apierror.CodeValidationFailed,
			"Phải nêu lý do từ chối")

	// ------------------------------------------------ Biến thể và SKU
	case errors.Is(err, domain.ErrEmptyAttribute):
		return apierror.New(apierror.CodeValidationFailed,
			"Thuộc tính biến thể không được để trống (ví dụ màu rỗng)")
	case errors.Is(err, domain.ErrNoAttributes):
		return apierror.New(apierror.CodeValidationFailed,
			"Biến thể phải có ít nhất một thuộc tính (ví dụ màu, size)")
	case errors.Is(err, domain.ErrDuplicateVariant):
		return apierror.New(apierror.CodeConflict,
			"Sản phẩm đã có biến thể với đúng tổ hợp thuộc tính này")
	case errors.Is(err, domain.ErrEmptySKUCode):
		return apierror.New(apierror.CodeValidationFailed, "Mã SKU không được rỗng")
	case errors.Is(err, domain.ErrMaMauKhongHopLe):
		return apierror.New(apierror.CodeValidationFailed,
			"Mã màu không hợp lệ — cần dạng #RRGGBB")
	case errors.Is(err, domain.ErrNegativeWeight):
		return apierror.New(apierror.CodeValidationFailed,
			"Khối lượng không được âm")
	case errors.Is(err, domain.ErrNegativeDim):
		return apierror.New(apierror.CodeValidationFailed,
			"Kích thước không được âm")

	// ------------------------------------------------ Trùng ở tầng lưu trữ
	//
	// 409 chứ không phải 400: dữ liệu ĐÚNG định dạng, chỉ là có người dùng
	// trước. Nhà bán cần đổi giá trị, không cần sửa cách nhập.
	case errors.Is(err, domain.ErrSlugTaken):
		return apierror.New(apierror.CodeConflict,
			"Slug này đã có sản phẩm khác dùng — chọn slug khác")
	case errors.Is(err, domain.ErrSKUCodeTaken):
		return apierror.New(apierror.CodeConflict,
			"Mã SKU này đã thuộc về một sản phẩm khác")
	case errors.Is(err, domain.ErrEmptyName):
		return apierror.New(apierror.CodeValidationFailed, "Tên sản phẩm không được rỗng")
	case errors.Is(err, domain.ErrEmptySlug):
		return apierror.New(apierror.CodeValidationFailed, "Slug không được rỗng")
	case errors.Is(err, domain.ErrDuplicateSKUCode):
		return apierror.New(apierror.CodeConflict, "Mã SKU đã tồn tại")

	case errors.Is(err, domain.ErrInvalidStatus):
		// 409 chứ không phải 400: dữ liệu đúng, chỉ là sản phẩm đang ở
		// bước không cho phép thao tác này (ví dụ gửi duyệt hai lần).
		return apierror.New(apierror.CodeConflict,
			"Sản phẩm không ở trạng thái cho phép thao tác này")
	}
	return err
}

// mangRong: `[]` thay vì `null` cho danh sách rỗng.
//
// Giao diện đọc `images.length`; `null` làm nó nổ. Và `[]` với thiếu trường
// nói hai chuyện khác nhau — đây là "chưa có ảnh nào", không phải "không
// biết".
func mangRong(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
