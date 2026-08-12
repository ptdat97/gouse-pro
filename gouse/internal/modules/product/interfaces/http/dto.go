package http

// Các struct trong file này là HỢP ĐỒNG DÂY DẪN với client.
//
// Tên trường JSON khớp api/components/schemas.yaml#/ProductDetail.
//
// GHI CHÚ QUAN TRỌNG — những trường CHƯA có:
//
//	price_from, compare_at_price  → module pricing (giai đoạn 2.3)
//	available, buy_box_offer      → module inventory + marketplace (giai đoạn 3)
//	rating                        → module review (Phase 2)
//	size_recommendation           → cần lịch sử mua hàng (Phase 2)
//
// Chúng được BỎ QUA thay vì trả giá trị bịa (0 đồng, available: true).
// Giá bịa hiển thị lên trang là sai lệch cho khách; `available: true` khi
// chưa có tồn kho sẽ khiến khách đặt hàng không có thật.
//
// Nhờ `omitempty`, client viết theo đặc tả vẫn hoạt động — trường thiếu
// khác với trường sai.

// productDetail khớp schemas.yaml#/ProductDetail.
type productDetail struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`

	Brand      *brandRef      `json:"brand,omitempty"`
	Collection *collectionRef `json:"collection,omitempty"`
	CategoryID string         `json:"category_id,omitempty"`

	ProductType  string `json:"product_type,omitempty"`
	GenderTarget string `json:"gender_target,omitempty"`

	// Ba trường đặc thù thời trang — tác động TRỰC TIẾP tới tỷ lệ hoàn hàng.
	MaterialComposition string `json:"material_composition,omitempty"`
	CareInstructions    string `json:"care_instructions,omitempty"`
	OriginCountry       string `json:"origin_country,omitempty"`

	Images []image `json:"images,omitempty"`

	// Variants không dùng omitempty: đặc tả khai báo bắt buộc, và mảng rỗng
	// khác với thiếu trường.
	Variants []variant `json:"variants"`

	SizeChart *sizeChart `json:"size_chart,omitempty"`
}

// brandRef khớp schemas.yaml#/BrandRef.
//
// Name dùng omitempty dù đặc tả khai báo BẮT BUỘC. Lý do: tên thương hiệu
// thuộc module catalog, và module product KHÔNG gọi catalog ở tầng trình
// bày (mỗi request sản phẩm sẽ kéo theo một request catalog).
//
// Trả `"name": ""` tệ hơn là bỏ trường: chuỗi rỗng trông như dữ liệu hợp lệ
// và sẽ hiển thị thành khoảng trắng trên trang, còn trường thiếu thì client
// biết là chưa có và đi lấy từ nguồn khác.
type brandRef struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Slug    string `json:"slug,omitempty"`
	LogoURL string `json:"logo_url,omitempty"`
}

// collectionRef khớp schemas.yaml#/CollectionRef. Xem ghi chú ở brandRef
// về việc Name dùng omitempty.
type collectionRef struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Season string `json:"season,omitempty"`
}

// image khớp schemas.yaml#/Image.
type image struct {
	URL string `json:"url"`
	// Alt bắt buộc cho khả năng tiếp cận và SEO.
	Alt   string `json:"alt,omitempty"`
	Order int    `json:"order"`
}

// variant khớp schemas.yaml#/Variant.
type variant struct {
	ID     string       `json:"id"`
	Color  string       `json:"color,omitempty"`
	Images []image      `json:"images,omitempty"`
	SKUs   []skuSummary `json:"skus"`
}

// skuSummary khớp schemas.yaml#/SKUSummary.
//
// KHÔNG có trường `available`: tồn kho thuộc module inventory (giai đoạn 3).
// Trả `available: true` khi chưa biết sẽ khiến khách đặt hàng không có thật.
type skuSummary struct {
	ID      string `json:"id"`
	SKUCode string `json:"sku_code,omitempty"`
	Size    string `json:"size,omitempty"`
}

// sizeChart khớp common.yaml#/schemas/SizeChart.
type sizeChart struct {
	ID          string           `json:"id,omitempty"`
	ProductType string           `json:"product_type,omitempty"`
	System      string           `json:"system,omitempty"`
	Note        string           `json:"note,omitempty"`
	Entries     []sizeChartEntry `json:"entries"`
}

type sizeChartEntry struct {
	Size         string            `json:"size"`
	Measurements map[string]string `json:"measurements,omitempty"`
}

// productSummary khớp schemas.yaml#/ProductSummary — dùng cho danh sách.
type productSummary struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug,omitempty"`
	Brand           *brandRef `json:"brand,omitempty"`
	PrimaryImageURL string    `json:"primary_image_url,omitempty"`

	// AvailableColors liệt kê màu có trong danh mục.
	//
	// LƯU Ý: đặc tả nói available_sizes chỉ liệt kê size CÒN HÀNG. Chưa có
	// module inventory nên KHÔNG trả trường đó — liệt kê size đã hết là
	// đúng cái trải nghiệm tệ mà đặc tả muốn tránh.
	AvailableColors []string `json:"available_colors,omitempty"`
}

// productListResponse là danh sách sản phẩm.
type productListResponse struct {
	Data []productSummary `json:"data"`
}
