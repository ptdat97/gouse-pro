package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrEmptySizeChart = errors.New("catalog: bảng size phải có ít nhất một mục")
	ErrDuplicateSize  = errors.New("catalog: size bị trùng trong bảng")
	ErrSizeNotInChart = errors.New("catalog: size không có trong bảng")
)

// SizeSystem là hệ thống kích cỡ.
//
// Cùng một chiếc áo có thể là M (ALPHA), 38 (EU), S (US) — đây là nguồn gốc
// lớn nhất của việc chọn sai size.
type SizeSystem string

const (
	SizeSystemAlpha   SizeSystem = "ALPHA"   // S, M, L, XL
	SizeSystemNumeric SizeSystem = "NUMERIC" // 28, 30, 32
	SizeSystemEU      SizeSystem = "EU"
	SizeSystemUS      SizeSystem = "US"
	SizeSystemUK      SizeSystem = "UK"
	SizeSystemJP      SizeSystem = "JP"
	SizeSystemFree    SizeSystem = "FREE"
)

// ProductType phân loại sản phẩm — bảng size khác nhau theo loại.
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

// SizeEntry là một dòng trong bảng size với SỐ ĐO THỰC TẾ.
//
// Measurements dùng map vì số đo khác nhau theo loại sản phẩm:
//
//	áo:   chest_cm, length_cm, shoulder_cm, sleeve_cm
//	quần: waist_cm, hip_cm, inseam_cm
//	giày: foot_length_cm
type SizeEntry struct {
	Size         string
	Measurements map[string]string
}

// SizeChart là bảng quy đổi kích cỡ.
//
// VÌ SAO QUAN TRỌNG: sai size là nguyên nhân hoàn hàng SỐ MỘT trong thời
// trang. Bảng size có số đo thực tế (không chỉ ký hiệu S/M/L) giúp khách
// chọn đúng — giảm trực tiếp chi phí vận chuyển hai chiều, kiểm định lại,
// và rủi ro hàng không bán lại được.
//
// Bảng size gắn với (brand, product_type) vì "M" của thương hiệu A không
// bằng "M" của thương hiệu B.
//
// Xem docs/01-business/kpi.md mục 3.
type SizeChart struct {
	id          ids.ID
	brandID     ids.ID
	productType ProductType
	system      SizeSystem
	note        string
	entries     []SizeEntry
	createdAt   time.Time
	updatedAt   time.Time
}

type NewSizeChartParams struct {
	BrandID     ids.ID
	ProductType ProductType
	System      SizeSystem
	Note        string
	Entries     []SizeEntry
	Now         time.Time
}

func NewSizeChart(p NewSizeChartParams) (*SizeChart, error) {
	if len(p.Entries) == 0 {
		return nil, ErrEmptySizeChart
	}

	seen := make(map[string]struct{}, len(p.Entries))
	entries := make([]SizeEntry, 0, len(p.Entries))
	for _, e := range p.Entries {
		size := strings.TrimSpace(e.Size)
		if size == "" {
			return nil, errors.New("catalog: tên size không được rỗng")
		}
		if _, dup := seen[size]; dup {
			return nil, ErrDuplicateSize
		}
		seen[size] = struct{}{}

		// Sao chép map để aggregate không chia sẻ trạng thái với bên gọi.
		m := make(map[string]string, len(e.Measurements))
		for k, v := range e.Measurements {
			m[k] = v
		}
		entries = append(entries, SizeEntry{Size: size, Measurements: m})
	}

	id, err := ids.New(ids.PrefixSizeChart)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	system := p.System
	if system == "" {
		system = SizeSystemAlpha
	}

	return &SizeChart{
		id:          id,
		brandID:     p.BrandID,
		productType: p.ProductType,
		system:      system,
		note:        p.Note,
		entries:     entries,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

func (s *SizeChart) ID() ids.ID               { return s.id }
func (s *SizeChart) BrandID() ids.ID          { return s.brandID }
func (s *SizeChart) ProductType() ProductType { return s.productType }
func (s *SizeChart) System() SizeSystem       { return s.system }
func (s *SizeChart) Note() string             { return s.note }
func (s *SizeChart) CreatedAt() time.Time     { return s.createdAt }
func (s *SizeChart) UpdatedAt() time.Time     { return s.updatedAt }

// Entries trả về BẢN SAO — ngăn bên gọi sửa trạng thái nội bộ của aggregate.
func (s *SizeChart) Entries() []SizeEntry {
	out := make([]SizeEntry, len(s.entries))
	for i, e := range s.entries {
		m := make(map[string]string, len(e.Measurements))
		for k, v := range e.Measurements {
			m[k] = v
		}
		out[i] = SizeEntry{Size: e.Size, Measurements: m}
	}
	return out
}

// Sizes trả về danh sách tên size theo thứ tự trong bảng.
func (s *SizeChart) Sizes() []string {
	out := make([]string, len(s.entries))
	for i, e := range s.entries {
		out[i] = e.Size
	}
	return out
}

// HasSize kiểm tra một size có trong bảng không.
//
// Dùng khi tạo SKU: không cho tạo SKU với size không tồn tại trong bảng
// size của thương hiệu — chặn lỗi nhập liệu ngay ở tầng domain.
func (s *SizeChart) HasSize(size string) bool {
	for _, e := range s.entries {
		if e.Size == size {
			return true
		}
	}
	return false
}

// MeasurementsFor trả về số đo của một size.
func (s *SizeChart) MeasurementsFor(size string) (map[string]string, error) {
	for _, e := range s.entries {
		if e.Size == size {
			m := make(map[string]string, len(e.Measurements))
			for k, v := range e.Measurements {
				m[k] = v
			}
			return m, nil
		}
	}
	return nil, ErrSizeNotInChart
}

type RestoreSizeChartParams struct {
	ID          ids.ID
	BrandID     ids.ID
	ProductType ProductType
	System      SizeSystem
	Note        string
	Entries     []SizeEntry
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func RestoreSizeChart(p RestoreSizeChartParams) *SizeChart {
	return &SizeChart{
		id:          p.ID,
		brandID:     p.BrandID,
		productType: p.ProductType,
		system:      p.System,
		note:        p.Note,
		entries:     p.Entries,
		createdAt:   p.CreatedAt,
		updatedAt:   p.UpdatedAt,
	}
}
