package http

// Các struct trong file này là HỢP ĐỒNG DÂY DẪN với client.
//
// Chúng tách biệt với domain object có chủ đích: đổi tên một trường nội bộ
// của domain KHÔNG được làm hỏng client. Nếu serialize thẳng domain object,
// mọi tái cấu trúc nội bộ trở thành thay đổi phá vỡ API công khai.
//
// Tên trường JSON khớp api/paths/storefront.yaml và
// api/components/schemas.yaml.

// brandRef khớp schemas.yaml#/BrandRef.
type brandRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug,omitempty"`
	LogoURL string `json:"logo_url,omitempty"`
}

// brandDetail khớp storefront.yaml#/brand_detail — BrandRef mở rộng.
type brandDetail struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug,omitempty"`
	LogoURL string `json:"logo_url,omitempty"`

	Description     string `json:"description,omitempty"`
	CountryOfOrigin string `json:"country_of_origin,omitempty"`

	// Collections không dùng omitempty: mảng rỗng và thiếu trường có ý nghĩa
	// khác nhau với client. `[]` = thương hiệu chưa có bộ sưu tập nào đang
	// hiển thị; thiếu trường = không biết.
	Collections []collectionRef `json:"collections"`
}

// collectionRef khớp schemas.yaml#/CollectionRef.
type collectionRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Season string `json:"season,omitempty"`
}

// collectionDetail khớp storefront.yaml#/collection_detail.
type collectionDetail struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Season string `json:"season,omitempty"`

	Theme string `json:"theme,omitempty"`

	// LaunchDate là chuỗi "YYYY-MM-DD" theo `format: date` của đặc tả,
	// không phải time.Time (sẽ serialize thành RFC3339 có cả giờ).
	LaunchDate string `json:"launch_date,omitempty"`

	Brand *brandRef `json:"brand,omitempty"`
}

// categoryTreeResponse khớp storefront.yaml#/categories.
//
// Bọc trong `data` vì đặc tả khai báo vậy — cho phép thêm metadata sau này
// mà không phá vỡ client.
type categoryTreeResponse struct {
	Data []categoryNode `json:"data"`
}

// categoryNode là một nút danh mục, đệ quy.
type categoryNode struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Slug     string         `json:"slug,omitempty"`
	Children []categoryNode `json:"children"`
}
