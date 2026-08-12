// Package domain chứa mô hình nghiệp vụ của module product:
// Product → Variant → SKU.
//
// Tầng này KHÔNG biết gì về database, HTTP hay JSON. Nó chỉ biết quy tắc
// nghiệp vụ. Quy tắc R2 của cmd/archcheck cưỡng chế điều đó.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrEmptyName          = errors.New("product: tên sản phẩm không được rỗng")
	ErrEmptySlug          = errors.New("product: slug không được rỗng")
	ErrInvalidStatus      = errors.New("product: chuyển trạng thái không hợp lệ")
	ErrMissingBrand       = errors.New("product: sản phẩm phải thuộc một thương hiệu")
	ErrMissingCategory    = errors.New("product: sản phẩm phải thuộc một danh mục")
	ErrMissingSizeChart   = errors.New("product: sản phẩm thời trang phải có bảng size")
	ErrMissingMaterial    = errors.New("product: thiếu thành phần chất liệu")
	ErrMissingDescription = errors.New("product: thiếu mô tả")
	ErrNoImages           = errors.New("product: sản phẩm phải có ít nhất một ảnh")
	ErrNoVariants         = errors.New("product: sản phẩm phải có ít nhất một biến thể")
)

// ProductType phân loại sản phẩm.
//
// Cố ý lặp lại danh sách này ở cả catalog và product thay vì đưa vào kernel:
// quy tắc R4 giữ kernel tối thiểu, và hai module PHẢI tách rời được. Nếu
// dùng chung một kiểu, tách service sau này sẽ vướng.
//
// Giá trị chuỗi khớp nhau nên chuyển đổi ở ranh giới là an toàn.
type ProductType string

const (
	ProductTypeTop       ProductType = "TOP"
	ProductTypeBottom    ProductType = "BOTTOM"
	ProductTypeDress     ProductType = "DRESS"
	ProductTypeOuterwear ProductType = "OUTERWEAR"
	ProductTypeShoes     ProductType = "SHOES"
	ProductTypeBag       ProductType = "BAG"
	ProductTypeAccessory ProductType = "ACCESSORY"
)

func (p ProductType) valid() bool {
	switch p {
	case ProductTypeTop, ProductTypeBottom, ProductTypeDress, ProductTypeOuterwear,
		ProductTypeShoes, ProductTypeBag, ProductTypeAccessory:
		return true
	}
	return false
}

// NeedsSizeChart cho biết loại sản phẩm này có bắt buộc bảng size không.
//
// Túi xách không có size — bắt buộc bảng size cho mọi loại sẽ chặn việc
// đăng bán những mặt hàng hợp lệ.
func (p ProductType) NeedsSizeChart() bool {
	switch p {
	case ProductTypeBag, ProductTypeAccessory:
		return false
	}
	return true
}

// GenderTarget là đối tượng khách hàng nhắm tới.
type GenderTarget string

const (
	GenderMen    GenderTarget = "MEN"
	GenderWomen  GenderTarget = "WOMEN"
	GenderUnisex GenderTarget = "UNISEX"
	GenderKids   GenderTarget = "KIDS"
)

func (g GenderTarget) valid() bool {
	switch g {
	case GenderMen, GenderWomen, GenderUnisex, GenderKids:
		return true
	}
	return false
}

// Status là trạng thái vòng đời sản phẩm.
//
// Xem docs/04-modules/product.md mục 6.
type Status string

const (
	StatusDraft         Status = "DRAFT"
	StatusPendingReview Status = "PENDING_REVIEW"
	StatusActive        Status = "ACTIVE"
	StatusInactive      Status = "INACTIVE"
	StatusArchived      Status = "ARCHIVED"
)

// canTransitionTo mã hóa máy trạng thái ở mục 6 của đặc tả module.
//
// Đặt luật chuyển trạng thái Ở ĐÂY, không rải rác trong các use case —
// nếu rải rác, mỗi chỗ sẽ quên một nhánh và trạng thái sẽ trôi.
func (s Status) canTransitionTo(next Status) bool {
	switch s {
	case StatusDraft:
		// Nháp có thể gửi duyệt hoặc bỏ hẳn.
		return next == StatusPendingReview || next == StatusArchived
	case StatusPendingReview:
		// Bị từ chối quay về DRAFT để sửa, không phải trạng thái riêng —
		// lý do từ chối lưu ở rejectionReason.
		return next == StatusActive || next == StatusDraft || next == StatusArchived
	case StatusActive:
		return next == StatusInactive || next == StatusArchived
	case StatusInactive:
		// Bật lại được, không cần duyệt lại: nội dung không đổi.
		return next == StatusActive || next == StatusArchived
	case StatusArchived:
		// Ngừng vĩnh viễn. Không có đường ra.
		//
		// Cho phép bật lại sản phẩm đã lưu trữ sẽ làm hỏng giả định của
		// các module khác (đơn hàng cũ, báo cáo) rằng ARCHIVED là cuối cùng.
		return false
	}
	return false
}

// Product là aggregate root — trang sản phẩm khách nhìn thấy.
//
// RANH GIỚI AGGREGATE: Variant và SKU thuộc aggregate này (xem
// docs/02-domain/aggregates.md). Chúng không tồn tại độc lập và luôn được
// đọc/ghi cùng Product. Offer và Inventory thì KHÔNG — chúng thuộc module
// khác và chỉ tham chiếu bằng định danh.
type Product struct {
	id           ids.ID
	brandID      ids.ID
	collectionID ids.ID // rỗng nếu không thuộc bộ sưu tập nào
	categoryID   ids.ID
	sizeChartID  ids.ID

	name        string
	slug        string
	description string

	// Ba trường đặc thù thời trang. KHÔNG phải tùy chọn — thiếu chúng làm
	// tăng trực tiếp tỷ lệ hoàn hàng (docs/04-modules/product.md mục 4).
	careInstructions    string
	materialComposition string
	originCountry       string

	productType  ProductType
	genderTarget GenderTarget

	status          Status
	rejectionReason string

	// createdBySellerID rỗng nghĩa là danh mục chuẩn của nền tảng.
	createdBySellerID ids.ID

	images   []string
	variants []*Variant

	publishedAt time.Time
	createdAt   time.Time
	updatedAt   time.Time
}

type NewProductParams struct {
	BrandID      ids.ID
	CollectionID ids.ID
	CategoryID   ids.ID
	SizeChartID  ids.ID

	Name        string
	Slug        string
	Description string

	CareInstructions    string
	MaterialComposition string
	OriginCountry       string

	ProductType  ProductType
	GenderTarget GenderTarget

	CreatedBySellerID ids.ID
	Images            []string

	Now time.Time
}

// NewProduct tạo sản phẩm mới ở trạng thái DRAFT.
//
// Chỉ kiểm tra những gì BẮT BUỘC để tồn tại như bản nháp. Yêu cầu đầy đủ
// (ảnh, mô tả, bảng size) được kiểm tra khi gửi duyệt — bắt buộc ngay từ
// đầu sẽ khiến không thể lưu nháp dở dang, điều người dùng luôn cần.
func NewProduct(p NewProductParams) (*Product, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, ErrEmptyName
	}
	slug := strings.TrimSpace(p.Slug)
	if slug == "" {
		return nil, ErrEmptySlug
	}
	if p.BrandID.IsZero() {
		return nil, ErrMissingBrand
	}
	if p.CategoryID.IsZero() {
		return nil, ErrMissingCategory
	}
	if !p.ProductType.valid() {
		return nil, errors.New("product: loại sản phẩm không hợp lệ: " + string(p.ProductType))
	}

	gender := p.GenderTarget
	if gender == "" {
		gender = GenderUnisex
	}
	if !gender.valid() {
		return nil, errors.New("product: đối tượng không hợp lệ: " + string(gender))
	}

	id, err := ids.New(ids.PrefixProduct)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Product{
		id:                  id,
		brandID:             p.BrandID,
		collectionID:        p.CollectionID,
		categoryID:          p.CategoryID,
		sizeChartID:         p.SizeChartID,
		name:                name,
		slug:                slug,
		description:         strings.TrimSpace(p.Description),
		careInstructions:    strings.TrimSpace(p.CareInstructions),
		materialComposition: strings.TrimSpace(p.MaterialComposition),
		originCountry:       strings.TrimSpace(p.OriginCountry),
		productType:         p.ProductType,
		genderTarget:        gender,
		status:              StatusDraft,
		createdBySellerID:   p.CreatedBySellerID,
		images:              append([]string(nil), p.Images...),
		createdAt:           now,
		updatedAt:           now,
	}, nil
}

// RestoreProductParams dùng để dựng lại aggregate TỪ KHO LƯU TRỮ.
type RestoreProductParams struct {
	ID           ids.ID
	BrandID      ids.ID
	CollectionID ids.ID
	CategoryID   ids.ID
	SizeChartID  ids.ID

	Name        string
	Slug        string
	Description string

	CareInstructions    string
	MaterialComposition string
	OriginCountry       string

	ProductType  ProductType
	GenderTarget GenderTarget

	Status          Status
	RejectionReason string

	CreatedBySellerID ids.ID
	Images            []string
	Variants          []*Variant

	PublishedAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RestoreProduct dựng lại aggregate mà KHÔNG kiểm tra.
//
// Dữ liệu đã lưu từng hợp lệ theo luật lúc đó. Nếu luật đổi, việc kiểm tra
// lúc đọc sẽ làm hỏng chức năng đọc dữ liệu cũ — phải xử lý bằng migration,
// không phải bằng cách từ chối đọc.
//
// CHỈ tầng infrastructure được gọi hàm này.
func RestoreProduct(p RestoreProductParams) *Product {
	return &Product{
		id:                  p.ID,
		brandID:             p.BrandID,
		collectionID:        p.CollectionID,
		categoryID:          p.CategoryID,
		sizeChartID:         p.SizeChartID,
		name:                p.Name,
		slug:                p.Slug,
		description:         p.Description,
		careInstructions:    p.CareInstructions,
		materialComposition: p.MaterialComposition,
		originCountry:       p.OriginCountry,
		productType:         p.ProductType,
		genderTarget:        p.GenderTarget,
		status:              p.Status,
		rejectionReason:     p.RejectionReason,
		createdBySellerID:   p.CreatedBySellerID,
		images:              p.Images,
		variants:            p.Variants,
		publishedAt:         p.PublishedAt,
		createdAt:           p.CreatedAt,
		updatedAt:           p.UpdatedAt,
	}
}

// ---------------------------------------------------------------- Truy vấn

func (p *Product) ID() ids.ID                  { return p.id }
func (p *Product) BrandID() ids.ID             { return p.brandID }
func (p *Product) CollectionID() ids.ID        { return p.collectionID }
func (p *Product) CategoryID() ids.ID          { return p.categoryID }
func (p *Product) SizeChartID() ids.ID         { return p.sizeChartID }
func (p *Product) Name() string                { return p.name }
func (p *Product) Slug() string                { return p.slug }
func (p *Product) Description() string         { return p.description }
func (p *Product) CareInstructions() string    { return p.careInstructions }
func (p *Product) MaterialComposition() string { return p.materialComposition }
func (p *Product) OriginCountry() string       { return p.originCountry }
func (p *Product) Type() ProductType           { return p.productType }
func (p *Product) GenderTarget() GenderTarget  { return p.genderTarget }
func (p *Product) Status() Status              { return p.status }
func (p *Product) RejectionReason() string     { return p.rejectionReason }
func (p *Product) CreatedBySellerID() ids.ID   { return p.createdBySellerID }
func (p *Product) PublishedAt() time.Time      { return p.publishedAt }
func (p *Product) CreatedAt() time.Time        { return p.createdAt }
func (p *Product) UpdatedAt() time.Time        { return p.updatedAt }

// IsPlatformCatalog cho biết đây có phải danh mục chuẩn của nền tảng không.
//
// Sản phẩm chuẩn cho phép nhiều seller gắn offer vào cùng một trang thay vì
// mỗi seller tạo một trang trùng lặp — nền tảng của mô hình Offer.
func (p *Product) IsPlatformCatalog() bool { return p.createdBySellerID.IsZero() }

// IsVisibleToCustomer cho biết có hiển thị cho khách không.
//
// LƯU Ý: hiển thị được KHÔNG có nghĩa là mua được. Còn cần ít nhất một
// Offer đang hoạt động và còn hàng — hai khái niệm riêng, thuộc module khác.
func (p *Product) IsVisibleToCustomer() bool { return p.status == StatusActive }

// Images trả về bản sao để bên ngoài không sửa được trạng thái nội bộ.
func (p *Product) Images() []string {
	return append([]string(nil), p.images...)
}

// Variants trả về các biến thể.
//
// Trả bản sao của LÁT CẮT, nhưng con trỏ Variant vẫn là bản gốc: Variant
// thuộc cùng aggregate nên việc sửa nó qua phương thức của Product là hợp lệ.
func (p *Product) Variants() []*Variant {
	return append([]*Variant(nil), p.variants...)
}

// VariantByID tìm biến thể theo định danh.
func (p *Product) VariantByID(id ids.ID) (*Variant, bool) {
	for _, v := range p.variants {
		if v.ID() == id {
			return v, true
		}
	}
	return nil, false
}

// SKUs trả về toàn bộ SKU của mọi biến thể.
func (p *Product) SKUs() []*SKU {
	var out []*SKU
	for _, v := range p.variants {
		out = append(out, v.SKUs()...)
	}
	return out
}

// ---------------------------------------------------------------- Hành vi

// AddVariant thêm biến thể vào sản phẩm.
//
// Quy tắc 2 (mục 12): không trùng tổ hợp thuộc tính trong một Product.
// Kiểm tra ở ĐÂY chứ không ở tầng database vì tổ hợp thuộc tính là khái
// niệm nghiệp vụ — ràng buộc UNIQUE trên map thuộc tính rất khó biểu diễn.
func (p *Product) AddVariant(v *Variant, now time.Time) error {
	if v == nil {
		return errors.New("product: biến thể rỗng")
	}
	for _, existing := range p.variants {
		if existing.SameAttributesAs(v) {
			return ErrDuplicateVariant
		}
	}
	v.productID = p.id
	p.variants = append(p.variants, v)
	p.touch(now)
	return nil
}

// SubmitForReview chuyển sản phẩm sang chờ duyệt.
//
// Đây là chỗ áp dụng các kiểm tra tự động ở mục 6 của đặc tả. Kiểm tra tại
// thời điểm gửi duyệt, không phải lúc tạo — cho phép lưu nháp dở dang.
func (p *Product) SubmitForReview(now time.Time) error {
	if err := p.CheckReadyForReview(); err != nil {
		return err
	}
	return p.transition(StatusPendingReview, now)
}

// CheckReadyForReview kiểm tra sản phẩm đã đủ điều kiện gửi duyệt chưa.
//
// Tách riêng khỏi SubmitForReview để giao diện người dùng hiển thị được
// "còn thiếu gì" TRƯỚC khi người dùng bấm gửi — báo lỗi sau khi bấm là
// trải nghiệm tệ và làm seller bỏ dở việc đăng bán.
func (p *Product) CheckReadyForReview() error {
	if p.description == "" {
		return ErrMissingDescription
	}
	if len(p.images) == 0 {
		return ErrNoImages
	}
	if len(p.variants) == 0 {
		return ErrNoVariants
	}
	// Quy tắc 4: sản phẩm thời trang phải có bảng size — nhưng túi và phụ
	// kiện không có size.
	if p.productType.NeedsSizeChart() && p.sizeChartID.IsZero() {
		return ErrMissingSizeChart
	}
	// Chất liệu ảnh hưởng trực tiếp tỷ lệ hoàn hàng, nên là điều kiện bắt
	// buộc chứ không phải khuyến nghị.
	if p.materialComposition == "" {
		return ErrMissingMaterial
	}
	return nil
}

// Approve duyệt và xuất bản sản phẩm.
func (p *Product) Approve(now time.Time) error {
	if err := p.transition(StatusActive, now); err != nil {
		return err
	}
	// publishedAt chỉ đặt LẦN ĐẦU. Tạm ngừng rồi bán lại không phải là
	// xuất bản mới — báo cáo "sản phẩm mới trong tháng" sẽ sai nếu ghi đè.
	if p.publishedAt.IsZero() {
		p.publishedAt = now
	}
	p.rejectionReason = ""
	return nil
}

// Reject từ chối sản phẩm, đưa về DRAFT kèm lý do.
//
// Lý do là BẮT BUỘC: "sản phẩm bị từ chối" không cho seller biết phải sửa
// gì, dẫn tới gửi lại y nguyên và tốn thêm một vòng duyệt.
func (p *Product) Reject(reason string, now time.Time) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("product: phải nêu lý do từ chối")
	}
	if err := p.transition(StatusDraft, now); err != nil {
		return err
	}
	p.rejectionReason = reason
	return nil
}

// Deactivate tạm ngừng bán.
func (p *Product) Deactivate(now time.Time) error {
	return p.transition(StatusInactive, now)
}

// Reactivate bán lại sau khi tạm ngừng.
func (p *Product) Reactivate(now time.Time) error {
	return p.transition(StatusActive, now)
}

// Archive ngừng bán vĩnh viễn.
//
// KHÔNG xóa dữ liệu (quy tắc 5): đơn hàng cũ vẫn trỏ tới sản phẩm này và
// phải hiển thị được lịch sử mua hàng.
func (p *Product) Archive(now time.Time) error {
	return p.transition(StatusArchived, now)
}

func (p *Product) transition(next Status, now time.Time) error {
	if !p.status.canTransitionTo(next) {
		return ErrInvalidStatus
	}
	p.status = next
	p.touch(now)
	return nil
}

// AddImage thêm ảnh sản phẩm.
func (p *Product) AddImage(url string, now time.Time) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("product: đường dẫn ảnh rỗng")
	}
	p.images = append(p.images, url)
	p.touch(now)
	return nil
}

// AssignSizeChart gắn bảng size.
func (p *Product) AssignSizeChart(id ids.ID, now time.Time) {
	p.sizeChartID = id
	p.touch(now)
}

func (p *Product) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	p.updatedAt = now
}
