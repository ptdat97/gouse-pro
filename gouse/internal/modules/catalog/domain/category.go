package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrCategoryCycle   = errors.New("catalog: danh mục không được là tổ tiên của chính nó")
	ErrCategoryTooDeep = errors.New("catalog: cây danh mục quá sâu")
)

// maxCategoryDepth giới hạn độ sâu cây danh mục.
//
// Giới hạn này không tùy tiện: cây quá sâu làm khách khó tìm, làm URL dài,
// và làm truy vấn tổ tiên/hậu duệ tốn kém. Thời trang thường cần 3–4 tầng:
//
//	Nữ > Áo > Áo sơ mi
const maxCategoryDepth = 5

// Category là nút trong cây danh mục.
//
// Ngoài việc duyệt tìm, danh mục còn xác định TỶ LỆ HOA HỒNG theo ngành hàng
// — giày có biên khác áo. Xem docs/01-business/monetization.md mục 2.1.
type Category struct {
	id           ids.ID
	parentID     ids.ID // rỗng nếu là nút gốc
	name         string
	slug         string
	depth        int
	displayOrder int
	status       Status
	createdAt    time.Time
	updatedAt    time.Time
}

type NewCategoryParams struct {
	ParentID     ids.ID
	ParentDepth  int // -1 nếu không có cha
	Name         string
	Slug         string
	DisplayOrder int
	Now          time.Time
}

func NewCategory(p NewCategoryParams) (*Category, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, ErrEmptyName
	}
	slug := strings.TrimSpace(p.Slug)
	if !isValidSlug(slug) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidSlug, p.Slug)
	}

	depth := 0
	if !p.ParentID.IsZero() {
		depth = p.ParentDepth + 1
		if depth >= maxCategoryDepth {
			return nil, fmt.Errorf("%w: tối đa %d tầng", ErrCategoryTooDeep, maxCategoryDepth)
		}
	}

	id, err := ids.New(ids.PrefixCategory)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Category{
		id:           id,
		parentID:     p.ParentID,
		name:         name,
		slug:         slug,
		depth:        depth,
		displayOrder: p.DisplayOrder,
		status:       StatusActive,
		createdAt:    now,
		updatedAt:    now,
	}, nil
}

func (c *Category) ID() ids.ID           { return c.id }
func (c *Category) ParentID() ids.ID     { return c.parentID }
func (c *Category) Name() string         { return c.name }
func (c *Category) Slug() string         { return c.slug }
func (c *Category) Depth() int           { return c.depth }
func (c *Category) DisplayOrder() int    { return c.displayOrder }
func (c *Category) Status() Status       { return c.status }
func (c *Category) IsRoot() bool         { return c.parentID.IsZero() }
func (c *Category) CreatedAt() time.Time { return c.createdAt }
func (c *Category) UpdatedAt() time.Time { return c.updatedAt }

// Rename đổi tên danh mục.
func (c *Category) Rename(name string, now time.Time) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyName
	}
	c.name = name
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.updatedAt = now
	return nil
}

// Deactivate ngừng hiển thị danh mục.
//
// KHÔNG xóa cứng: danh mục có thể đang được sản phẩm và đơn hàng cũ
// tham chiếu. Xem docs/05-data/data-model.md mục 8.
func (c *Category) Deactivate(now time.Time) {
	c.status = StatusInactive
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.updatedAt = now
}

type RestoreCategoryParams struct {
	ID           ids.ID
	ParentID     ids.ID
	Name         string
	Slug         string
	Depth        int
	DisplayOrder int
	Status       Status
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func RestoreCategory(p RestoreCategoryParams) *Category {
	return &Category{
		id:           p.ID,
		parentID:     p.ParentID,
		name:         p.Name,
		slug:         p.Slug,
		depth:        p.Depth,
		displayOrder: p.DisplayOrder,
		status:       p.Status,
		createdAt:    p.CreatedAt,
		updatedAt:    p.UpdatedAt,
	}
}
