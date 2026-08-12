// Package domain chứa mô hình nghiệp vụ của module catalog.
//
// RÀNG BUỘC: package này CHỈ được import thư viện chuẩn và internal/kernel.
// Nếu xóa toàn bộ infrastructure/ và interfaces/, code ở đây vẫn phải
// biên dịch được. Đây là điều kiện để kiểm thử quy tắc nghiệp vụ mà không
// cần database, không cần HTTP.
//
// cmd/archcheck quy tắc R2 cưỡng chế điều này — vi phạm làm CI thất bại.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrEmptyName            = errors.New("catalog: tên không được rỗng")
	ErrInvalidSlug          = errors.New("catalog: slug không hợp lệ")
	ErrBrandNotAuthorized   = errors.New("catalog: seller không có ủy quyền cho thương hiệu này")
	ErrAuthorizationExpired = errors.New("catalog: giấy ủy quyền đã hết hạn")
)

// ProtectionLevel xác định ai được phép tạo offer cho thương hiệu.
//
// Đây là cơ chế chống hàng giả — rủi ro sống còn của marketplace thời trang.
// Kiểm tra này là QUY TẮC DOMAIN bắt buộc, không phải quy trình thủ công
// bên ngoài hệ thống.
//
// Xem docs/01-business/marketplace.md mục 4.1.
type ProtectionLevel string

const (
	// ProtectionOpen — seller nào cũng tạo offer được.
	ProtectionOpen ProtectionLevel = "OPEN"
	// ProtectionVerifiedOnly — chỉ seller có giấy ủy quyền còn hiệu lực.
	ProtectionVerifiedOnly ProtectionLevel = "VERIFIED_ONLY"
	// ProtectionRestricted — chỉ nền tảng hoặc seller được chỉ định.
	ProtectionRestricted ProtectionLevel = "RESTRICTED"
)

func (p ProtectionLevel) valid() bool {
	switch p {
	case ProtectionOpen, ProtectionVerifiedOnly, ProtectionRestricted:
		return true
	}
	return false
}

// BrandType phân biệt thương hiệu own brand với thương hiệu bên thứ ba.
type BrandType string

const (
	BrandTypeOwn        BrandType = "OWN"
	BrandTypePartner    BrandType = "PARTNER"
	BrandTypeThirdParty BrandType = "THIRD_PARTY"
)

// Status là trạng thái chung của các thực thể catalog.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// Brand là thương hiệu.
//
// Trường không xuất khẩu (unexported) buộc mọi thay đổi phải đi qua phương
// thức của aggregate — không thể gán trực tiếp và bỏ qua kiểm tra bất biến.
type Brand struct {
	id              ids.ID
	name            string
	slug            string
	description     string
	logoURL         string
	brandType       BrandType
	protectionLevel ProtectionLevel
	ownerSellerID   ids.ID // rỗng nếu là brand của nền tảng
	countryOfOrigin string
	status          Status
	createdAt       time.Time
	updatedAt       time.Time
}

// NewBrandParams gom tham số khởi tạo — tránh hàm có quá nhiều đối số
// dễ truyền nhầm thứ tự.
type NewBrandParams struct {
	Name            string
	Slug            string
	Description     string
	LogoURL         string
	BrandType       BrandType
	ProtectionLevel ProtectionLevel
	OwnerSellerID   ids.ID
	CountryOfOrigin string
	Now             time.Time
}

// NewBrand tạo thương hiệu mới, kiểm tra bất biến ngay khi khởi tạo.
//
// Không tồn tại Brand không hợp lệ trong hệ thống — đây là lý do dùng
// hàm khởi tạo thay vì struct literal.
func NewBrand(p NewBrandParams) (*Brand, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, ErrEmptyName
	}
	slug := strings.TrimSpace(p.Slug)
	if !isValidSlug(slug) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidSlug, p.Slug)
	}

	bt := p.BrandType
	if bt == "" {
		bt = BrandTypeThirdParty
	}
	pl := p.ProtectionLevel
	if pl == "" {
		pl = ProtectionOpen
	}
	if !pl.valid() {
		return nil, fmt.Errorf("catalog: mức bảo vệ không hợp lệ: %q", p.ProtectionLevel)
	}

	id, err := ids.New(ids.PrefixBrand)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Brand{
		id:              id,
		name:            name,
		slug:            slug,
		description:     p.Description,
		logoURL:         p.LogoURL,
		brandType:       bt,
		protectionLevel: pl,
		ownerSellerID:   p.OwnerSellerID,
		countryOfOrigin: p.CountryOfOrigin,
		status:          StatusActive,
		createdAt:       now,
		updatedAt:       now,
	}, nil
}

func (b *Brand) ID() ids.ID                       { return b.id }
func (b *Brand) Name() string                     { return b.name }
func (b *Brand) Slug() string                     { return b.slug }
func (b *Brand) Description() string              { return b.description }
func (b *Brand) LogoURL() string                  { return b.logoURL }
func (b *Brand) Type() BrandType                  { return b.brandType }
func (b *Brand) ProtectionLevel() ProtectionLevel { return b.protectionLevel }
func (b *Brand) OwnerSellerID() ids.ID            { return b.ownerSellerID }
func (b *Brand) CountryOfOrigin() string          { return b.countryOfOrigin }
func (b *Brand) Status() Status                   { return b.status }
func (b *Brand) CreatedAt() time.Time             { return b.createdAt }
func (b *Brand) UpdatedAt() time.Time             { return b.updatedAt }

// IsOwnBrand cho biết đây có phải thương hiệu của nền tảng không.
func (b *Brand) IsOwnBrand() bool { return b.brandType == BrandTypeOwn }

// IsProtected cho biết thương hiệu có yêu cầu kiểm tra ủy quyền không.
func (b *Brand) IsProtected() bool { return b.protectionLevel != ProtectionOpen }

// Rename đổi tên thương hiệu.
func (b *Brand) Rename(name string, now time.Time) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyName
	}
	b.name = name
	b.touch(now)
	return nil
}

// SetProtectionLevel đổi mức bảo vệ.
//
// Nâng mức bảo vệ KHÔNG hủy các offer đang có — việc đó thuộc quy trình
// vận hành, xử lý qua event brand.protection_changed. Đây là quyết định
// có chủ đích: nâng mức bảo vệ mà hủy hàng loạt offer đang bán sẽ gây
// gián đoạn kinh doanh đột ngột cho seller hợp pháp.
func (b *Brand) SetProtectionLevel(level ProtectionLevel, now time.Time) error {
	if !level.valid() {
		return fmt.Errorf("catalog: mức bảo vệ không hợp lệ: %q", level)
	}
	b.protectionLevel = level
	b.touch(now)
	return nil
}

// Deactivate ngừng hoạt động thương hiệu.
func (b *Brand) Deactivate(now time.Time) {
	b.status = StatusInactive
	b.touch(now)
}

func (b *Brand) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	b.updatedAt = now
}

// RestoreBrand dựng lại Brand từ dữ liệu đã lưu.
//
// CHỈ dùng bởi infrastructure khi đọc từ kho lưu trữ. Nó bỏ qua kiểm tra
// bất biến vì dữ liệu đã được kiểm tra khi ghi — nếu kiểm tra lại, một
// thay đổi quy tắc nghiệp vụ sẽ làm không đọc được dữ liệu cũ.
type RestoreBrandParams struct {
	ID              ids.ID
	Name            string
	Slug            string
	Description     string
	LogoURL         string
	BrandType       BrandType
	ProtectionLevel ProtectionLevel
	OwnerSellerID   ids.ID
	CountryOfOrigin string
	Status          Status
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func RestoreBrand(p RestoreBrandParams) *Brand {
	return &Brand{
		id:              p.ID,
		name:            p.Name,
		slug:            p.Slug,
		description:     p.Description,
		logoURL:         p.LogoURL,
		brandType:       p.BrandType,
		protectionLevel: p.ProtectionLevel,
		ownerSellerID:   p.OwnerSellerID,
		countryOfOrigin: p.CountryOfOrigin,
		status:          p.Status,
		createdAt:       p.CreatedAt,
		updatedAt:       p.UpdatedAt,
	}
}

// isValidSlug kiểm tra slug: chữ thường, số, gạch ngang.
func isValidSlug(s string) bool {
	if s == "" || len(s) > 200 {
		return false
	}
	if strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
