package domain

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrDuplicateVariant = errors.New("product: trùng tổ hợp thuộc tính trong sản phẩm")
	ErrNoAttributes     = errors.New("product: biến thể phải có ít nhất một thuộc tính")
	ErrDuplicateSKUCode = errors.New("product: mã SKU đã tồn tại")
)

// Các khóa thuộc tính chuẩn.
//
// Chuẩn hóa tên khóa để bộ lọc "màu đỏ" hoạt động xuyên suốt danh mục.
// Nếu mỗi seller tự đặt tên khóa ("color", "colour", "mau_sac"), bộ lọc
// theo màu sẽ vô dụng.
const (
	AttrColor    = "color"
	AttrSize     = "size"
	AttrMaterial = "material"
	AttrPattern  = "pattern"
	AttrFit      = "fit"
)

// Variant là một tổ hợp thuộc tính của sản phẩm: "Màu Trắng, Size M".
//
// Không phải aggregate root — thuộc aggregate Product và luôn được đọc/ghi
// cùng Product.
type Variant struct {
	id        ids.ID
	productID ids.ID

	// attributes giữ tổ hợp thuộc tính, ví dụ {color: "Trắng", size: "M"}.
	//
	// Dùng map thay vì trường cố định vì thuộc tính khác nhau theo loại
	// sản phẩm: áo có độ dài tay, quần có kiểu ống, giày không có cả hai.
	attributes map[string]string

	// images là ảnh RIÊNG theo màu.
	//
	// Khách chọn màu đỏ phải thấy ảnh áo đỏ, không phải ảnh áo trắng —
	// ảnh sai màu là một nguyên nhân hoàn hàng.
	images []string

	displayOrder int
	status       Status

	skus []*SKU

	createdAt time.Time
	updatedAt time.Time
}

type NewVariantParams struct {
	Attributes   map[string]string
	Images       []string
	DisplayOrder int
	Now          time.Time
}

// NewVariant tạo biến thể mới.
func NewVariant(p NewVariantParams) (*Variant, error) {
	if len(p.Attributes) == 0 {
		return nil, ErrNoAttributes
	}

	// Chuẩn hóa: khóa viết thường, bỏ khoảng trắng thừa. Không chuẩn hóa
	// thì "Color" và "color" thành hai thuộc tính khác nhau và việc so sánh
	// trùng lặp sẽ bỏ sót.
	attrs := make(map[string]string, len(p.Attributes))
	for k, v := range p.Attributes {
		key := strings.ToLower(strings.TrimSpace(k))
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			return nil, errors.New("product: thuộc tính có khóa hoặc giá trị rỗng")
		}
		attrs[key] = val
	}

	id, err := ids.New(ids.PrefixVariant)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Variant{
		id:           id,
		attributes:   attrs,
		images:       append([]string(nil), p.Images...),
		displayOrder: p.DisplayOrder,
		status:       StatusActive,
		createdAt:    now,
		updatedAt:    now,
	}, nil
}

// RestoreVariantParams dựng lại biến thể từ kho lưu trữ.
type RestoreVariantParams struct {
	ID           ids.ID
	ProductID    ids.ID
	Attributes   map[string]string
	Images       []string
	DisplayOrder int
	Status       Status
	SKUs         []*SKU
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RestoreVariant dựng lại biến thể mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreVariant(p RestoreVariantParams) *Variant {
	return &Variant{
		id:           p.ID,
		productID:    p.ProductID,
		attributes:   p.Attributes,
		images:       p.Images,
		displayOrder: p.DisplayOrder,
		status:       p.Status,
		skus:         p.SKUs,
		createdAt:    p.CreatedAt,
		updatedAt:    p.UpdatedAt,
	}
}

func (v *Variant) ID() ids.ID           { return v.id }
func (v *Variant) ProductID() ids.ID    { return v.productID }
func (v *Variant) DisplayOrder() int    { return v.displayOrder }
func (v *Variant) Status() Status       { return v.status }
func (v *Variant) CreatedAt() time.Time { return v.createdAt }
func (v *Variant) UpdatedAt() time.Time { return v.updatedAt }

// Attributes trả về bản sao — map là kiểu tham chiếu, trả thẳng sẽ cho
// bên ngoài sửa được trạng thái nội bộ của aggregate.
func (v *Variant) Attributes() map[string]string {
	out := make(map[string]string, len(v.attributes))
	for k, val := range v.attributes {
		out[k] = val
	}
	return out
}

// Attribute lấy một thuộc tính.
func (v *Variant) Attribute(key string) (string, bool) {
	val, ok := v.attributes[strings.ToLower(key)]
	return val, ok
}

// Color và Size là hai thuộc tính dùng thường xuyên nhất nên có hàm riêng.
func (v *Variant) Color() string { val, _ := v.Attribute(AttrColor); return val }
func (v *Variant) Size() string  { val, _ := v.Attribute(AttrSize); return val }

func (v *Variant) Images() []string {
	return append([]string(nil), v.images...)
}

func (v *Variant) SKUs() []*SKU {
	return append([]*SKU(nil), v.skus...)
}

// AttributeKey là chuỗi định danh tổ hợp thuộc tính, dùng để so sánh trùng.
//
// Sắp xếp khóa trước khi ghép: map trong Go duyệt theo thứ tự ngẫu nhiên,
// không sắp xếp thì cùng một tổ hợp sẽ sinh ra chuỗi khác nhau mỗi lần và
// việc phát hiện trùng lặp sẽ chập chờn.
func (v *Variant) AttributeKey() string {
	keys := make([]string, 0, len(v.attributes))
	for k := range v.attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(strings.ToLower(v.attributes[k]))
	}
	return b.String()
}

// SameAttributesAs cho biết hai biến thể có cùng tổ hợp thuộc tính không.
func (v *Variant) SameAttributesAs(other *Variant) bool {
	if other == nil {
		return false
	}
	return v.AttributeKey() == other.AttributeKey()
}

// AddSKU thêm SKU vào biến thể.
//
// Quy tắc 1 (mục 12): sku_code duy nhất TOÀN HỆ THỐNG. Ở đây chỉ kiểm tra
// được phạm vi biến thể — tính duy nhất toàn cục do tầng application kiểm
// tra qua repository và do ràng buộc UNIQUE ở database bảo đảm.
func (v *Variant) AddSKU(s *SKU, now time.Time) error {
	if s == nil {
		return errors.New("product: SKU rỗng")
	}
	for _, existing := range v.skus {
		if strings.EqualFold(existing.Code(), s.Code()) {
			return ErrDuplicateSKUCode
		}
	}
	s.variantID = v.id
	v.skus = append(v.skus, s)
	v.touch(now)
	return nil
}

// SKUByCode tìm SKU theo mã.
func (v *Variant) SKUByCode(code string) (*SKU, bool) {
	for _, s := range v.skus {
		if strings.EqualFold(s.Code(), code) {
			return s, true
		}
	}
	return nil, false
}

// AddImage thêm ảnh cho biến thể.
func (v *Variant) AddImage(url string, now time.Time) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("product: đường dẫn ảnh rỗng")
	}
	v.images = append(v.images, url)
	v.touch(now)
	return nil
}

// Deactivate tạm ngừng biến thể (ví dụ hết màu này vĩnh viễn).
func (v *Variant) Deactivate(now time.Time) {
	v.status = StatusInactive
	v.touch(now)
}

func (v *Variant) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	v.updatedAt = now
}
