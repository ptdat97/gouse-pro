package domain

import (
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrInvalidDateRange = errors.New("catalog: ngày hết hạn phải sau ngày hiệu lực")
	ErrMissingDocument  = errors.New("catalog: thiếu giấy tờ ủy quyền")
)

// AuthorizationStatus là trạng thái của giấy ủy quyền.
type AuthorizationStatus string

const (
	AuthPending  AuthorizationStatus = "PENDING"
	AuthApproved AuthorizationStatus = "APPROVED"
	AuthRejected AuthorizationStatus = "REJECTED"
	AuthRevoked  AuthorizationStatus = "REVOKED"
)

// BrandAuthorization là giấy ủy quyền cho phép seller bán một thương hiệu
// được bảo vệ.
//
// Đây là LINK TABLE theo mẫu từ Medusa (xem docs/11-oss/medusa.md):
// liên kết brand (module catalog) với seller (module seller), chỉ chứa hai
// định danh + metadata CỦA CHÍNH QUAN HỆ (giấy tờ, hạn hiệu lực).
//
// Nó thuộc module catalog vì "thương hiệu này cho phép ai bán" là khái niệm
// của catalog, không phải của seller.
type BrandAuthorization struct {
	id          ids.ID
	brandID     ids.ID
	sellerID    ids.ID // không có khóa ngoại — seller ở module khác
	documentURL string
	validFrom   time.Time
	validUntil  time.Time
	status      AuthorizationStatus
	approvedBy  string
	approvedAt  time.Time
	createdAt   time.Time
}

type NewAuthorizationParams struct {
	BrandID     ids.ID
	SellerID    ids.ID
	DocumentURL string
	ValidFrom   time.Time
	ValidUntil  time.Time
	Now         time.Time
}

func NewBrandAuthorization(p NewAuthorizationParams) (*BrandAuthorization, error) {
	if p.DocumentURL == "" {
		return nil, ErrMissingDocument
	}
	if !p.ValidUntil.After(p.ValidFrom) {
		return nil, ErrInvalidDateRange
	}

	// Tiền tố RIÊNG, không dùng lại "brd" của thương hiệu: nếu trùng tiền
	// tố thì id ủy quyền và id thương hiệu không phân biệt được, và việc
	// truyền nhầm hai loại cho nhau sẽ không bị chặn — đúng thứ mà tiền tố
	// sinh ra để ngăn.
	id, err := ids.New(ids.PrefixAuthorization)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &BrandAuthorization{
		id:          id,
		brandID:     p.BrandID,
		sellerID:    p.SellerID,
		documentURL: p.DocumentURL,
		validFrom:   p.ValidFrom,
		validUntil:  p.ValidUntil,
		status:      AuthPending,
		createdAt:   now,
	}, nil
}

func (a *BrandAuthorization) ID() ids.ID                  { return a.id }
func (a *BrandAuthorization) BrandID() ids.ID             { return a.brandID }
func (a *BrandAuthorization) SellerID() ids.ID            { return a.sellerID }
func (a *BrandAuthorization) DocumentURL() string         { return a.documentURL }
func (a *BrandAuthorization) ValidFrom() time.Time        { return a.validFrom }
func (a *BrandAuthorization) ValidUntil() time.Time       { return a.validUntil }
func (a *BrandAuthorization) Status() AuthorizationStatus { return a.status }
func (a *BrandAuthorization) ApprovedBy() string          { return a.approvedBy }
func (a *BrandAuthorization) ApprovedAt() time.Time       { return a.approvedAt }
func (a *BrandAuthorization) CreatedAt() time.Time        { return a.createdAt }

// Approve duyệt giấy ủy quyền.
func (a *BrandAuthorization) Approve(by string, now time.Time) error {
	if a.status == AuthRevoked {
		return errors.New("catalog: không thể duyệt giấy đã thu hồi")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	a.status = AuthApproved
	a.approvedBy = by
	a.approvedAt = now
	return nil
}

// Revoke thu hồi giấy ủy quyền.
func (a *BrandAuthorization) Revoke() {
	a.status = AuthRevoked
}

// IsValidAt cho biết giấy ủy quyền có hiệu lực tại thời điểm cho trước không.
//
// Ba điều kiện phải đồng thời đúng:
//
//	đã được duyệt · đã tới ngày hiệu lực · chưa quá hạn
//
// Hạn hiệu lực quan trọng: giấy ủy quyền hết hạn phải TỰ ĐỘNG chặn việc
// tạo offer mới, không cần ai nhớ thu hồi thủ công.
func (a *BrandAuthorization) IsValidAt(t time.Time) bool {
	if a.status != AuthApproved {
		return false
	}
	if t.Before(a.validFrom) {
		return false
	}
	return t.Before(a.validUntil)
}

// ExpiresWithin cho biết giấy sắp hết hạn trong khoảng thời gian tới —
// dùng để cảnh báo seller trước khi offer bị chặn.
func (a *BrandAuthorization) ExpiresWithin(d time.Duration, now time.Time) bool {
	if a.status != AuthApproved {
		return false
	}
	return a.validUntil.After(now) && a.validUntil.Before(now.Add(d))
}

type RestoreAuthorizationParams struct {
	ID          ids.ID
	BrandID     ids.ID
	SellerID    ids.ID
	DocumentURL string
	ValidFrom   time.Time
	ValidUntil  time.Time
	Status      AuthorizationStatus
	ApprovedBy  string
	ApprovedAt  time.Time
	CreatedAt   time.Time
}

func RestoreBrandAuthorization(p RestoreAuthorizationParams) *BrandAuthorization {
	return &BrandAuthorization{
		id:          p.ID,
		brandID:     p.BrandID,
		sellerID:    p.SellerID,
		documentURL: p.DocumentURL,
		validFrom:   p.ValidFrom,
		validUntil:  p.ValidUntil,
		status:      p.Status,
		approvedBy:  p.ApprovedBy,
		approvedAt:  p.ApprovedAt,
		createdAt:   p.CreatedAt,
	}
}
