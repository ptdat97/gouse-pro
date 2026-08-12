package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrInvalidSeasonDates = errors.New("catalog: ngày kết thúc mùa phải sau ngày ra mắt")
	ErrInvalidTransition  = errors.New("catalog: chuyển trạng thái không hợp lệ")
)

// CollectionStatus là vòng đời bộ sưu tập.
//
//	PLANNING → ACTIVE → ENDING → ARCHIVED
//
// Không dùng thư viện máy trạng thái: số chuyển đổi ít, viết tay đọc dễ hơn
// cấu hình, và domain layer phải sạch (quy tắc R2 của archcheck).
// Xem docs/11-oss/sylius.md mục "Nhiều state machine tách biệt".
type CollectionStatus string

const (
	// CollectionPlanning — đang chuẩn bị, chưa hiển thị cho khách.
	CollectionPlanning CollectionStatus = "PLANNING"
	// CollectionActive — đang bán.
	CollectionActive CollectionStatus = "ACTIVE"
	// CollectionEnding — sắp hết mùa, thường kèm giảm giá xả hàng.
	CollectionEnding CollectionStatus = "ENDING"
	// CollectionArchived — ngừng bán.
	CollectionArchived CollectionStatus = "ARCHIVED"
)

// canTransitionTo định nghĩa các chuyển đổi hợp lệ.
//
// Không cho quay lại ARCHIVED → ACTIVE: bộ sưu tập đã đóng thì mở lại
// làm sai lệch chỉ số sell-through và báo cáo mùa vụ.
func (s CollectionStatus) canTransitionTo(next CollectionStatus) bool {
	switch s {
	case CollectionPlanning:
		return next == CollectionActive || next == CollectionArchived
	case CollectionActive:
		return next == CollectionEnding || next == CollectionArchived
	case CollectionEnding:
		return next == CollectionArchived
	default:
		return false
	}
}

// Collection là bộ sưu tập — khái niệm HẠNG NHẤT của thời trang.
//
// Vì sao là entity chứ không phải nhãn phân loại:
//   - có ngân sách sản xuất và mốc thời gian
//   - đo được sell-through theo bộ sưu tập
//   - cảnh báo được khi tiến độ sản xuất đe dọa ngày ra mắt
//
// Bộ sưu tập bán không hết trước khi hết mùa mất 50–70% giá trị — đây là
// rủi ro tài chính lớn nhất của own brand.
//
// Xem docs/01-business/own-brand.md mục 4.
type Collection struct {
	id              ids.ID
	brandID         ids.ID
	name            string
	slug            string
	season          string // "FW2026"
	theme           string
	launchDate      time.Time
	endOfSeasonDate time.Time
	status          CollectionStatus
	createdAt       time.Time
	updatedAt       time.Time
}

type NewCollectionParams struct {
	BrandID         ids.ID
	Name            string
	Slug            string
	Season          string
	Theme           string
	LaunchDate      time.Time
	EndOfSeasonDate time.Time
	Now             time.Time
}

func NewCollection(p NewCollectionParams) (*Collection, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, ErrEmptyName
	}
	slug := strings.TrimSpace(p.Slug)
	if !isValidSlug(slug) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidSlug, p.Slug)
	}
	if p.BrandID.IsZero() {
		return nil, errors.New("catalog: bộ sưu tập phải thuộc một thương hiệu")
	}
	// Chỉ kiểm tra khi cả hai ngày được đặt — bộ sưu tập ở giai đoạn
	// PLANNING có thể chưa chốt lịch.
	if !p.LaunchDate.IsZero() && !p.EndOfSeasonDate.IsZero() &&
		!p.EndOfSeasonDate.After(p.LaunchDate) {
		return nil, ErrInvalidSeasonDates
	}

	id, err := ids.New(ids.PrefixCollection)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Collection{
		id:              id,
		brandID:         p.BrandID,
		name:            name,
		slug:            slug,
		season:          p.Season,
		theme:           p.Theme,
		launchDate:      p.LaunchDate,
		endOfSeasonDate: p.EndOfSeasonDate,
		status:          CollectionPlanning,
		createdAt:       now,
		updatedAt:       now,
	}, nil
}

func (c *Collection) ID() ids.ID               { return c.id }
func (c *Collection) BrandID() ids.ID          { return c.brandID }
func (c *Collection) Name() string             { return c.name }
func (c *Collection) Slug() string             { return c.slug }
func (c *Collection) Season() string           { return c.season }
func (c *Collection) Theme() string            { return c.theme }
func (c *Collection) LaunchDate() time.Time    { return c.launchDate }
func (c *Collection) EndOfSeason() time.Time   { return c.endOfSeasonDate }
func (c *Collection) Status() CollectionStatus { return c.status }
func (c *Collection) CreatedAt() time.Time     { return c.createdAt }
func (c *Collection) UpdatedAt() time.Time     { return c.updatedAt }

// IsVisibleToCustomer cho biết bộ sưu tập có hiển thị cho khách không.
//
// PLANNING không hiển thị — đây là cơ chế "xuất bản có lịch" đơn giản,
// thay cho việc nhân đôi bảng như QOR Publish2.
// Xem docs/11-oss/qor.md mục "Publish2".
func (c *Collection) IsVisibleToCustomer() bool {
	return c.status == CollectionActive || c.status == CollectionEnding
}

// Launch chuyển bộ sưu tập sang trạng thái đang bán.
func (c *Collection) Launch(now time.Time) error {
	return c.transition(CollectionActive, now)
}

// MarkEnding đánh dấu sắp hết mùa — tín hiệu cho pricing giảm giá xả hàng.
func (c *Collection) MarkEnding(now time.Time) error {
	return c.transition(CollectionEnding, now)
}

// Archive ngừng bán bộ sưu tập.
func (c *Collection) Archive(now time.Time) error {
	return c.transition(CollectionArchived, now)
}

func (c *Collection) transition(next CollectionStatus, now time.Time) error {
	if !c.status.canTransitionTo(next) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, c.status, next)
	}
	c.status = next
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.updatedAt = now
	return nil
}

// ShouldLaunch cho biết đã tới lúc tự động ra mắt chưa.
//
// Dùng bởi job định kỳ trong cmd/worker — cơ chế công bố theo lịch mà
// không cần nhân đôi bảng dữ liệu.
func (c *Collection) ShouldLaunch(now time.Time) bool {
	return c.status == CollectionPlanning &&
		!c.launchDate.IsZero() &&
		!now.Before(c.launchDate)
}

// ShouldMarkEnding cho biết đã tới lúc chuyển sang giai đoạn xả hàng chưa.
func (c *Collection) ShouldMarkEnding(now time.Time) bool {
	return c.status == CollectionActive &&
		!c.endOfSeasonDate.IsZero() &&
		!now.Before(c.endOfSeasonDate)
}

// WeeksRemaining trả về số tuần còn lại của mùa.
//
// Đây là đầu vào cho quyết định bổ sung hàng: nếu thời gian còn lại ít hơn
// lead time của nhà cung cấp, KHÔNG nên đặt thêm — hàng về sẽ không kịp bán.
// Xem docs/07-workflows/replenishment.md mục 7.
func (c *Collection) WeeksRemaining(now time.Time) int {
	if c.endOfSeasonDate.IsZero() || now.After(c.endOfSeasonDate) {
		return 0
	}
	return int(c.endOfSeasonDate.Sub(now).Hours() / (24 * 7))
}

type RestoreCollectionParams struct {
	ID              ids.ID
	BrandID         ids.ID
	Name            string
	Slug            string
	Season          string
	Theme           string
	LaunchDate      time.Time
	EndOfSeasonDate time.Time
	Status          CollectionStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func RestoreCollection(p RestoreCollectionParams) *Collection {
	return &Collection{
		id:              p.ID,
		brandID:         p.BrandID,
		name:            p.Name,
		slug:            p.Slug,
		season:          p.Season,
		theme:           p.Theme,
		launchDate:      p.LaunchDate,
		endOfSeasonDate: p.EndOfSeasonDate,
		status:          p.Status,
		createdAt:       p.CreatedAt,
		updatedAt:       p.UpdatedAt,
	}
}
