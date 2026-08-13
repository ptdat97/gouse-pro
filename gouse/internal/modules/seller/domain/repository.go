package domain

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Repository là PORT cho nhà bán.
type Repository interface {
	Save(ctx context.Context, s *Seller) error

	FindByID(ctx context.Context, id ids.ID) (*Seller, error)
	FindBySlug(ctx context.Context, slug string) (*Seller, error)

	// FindByIDs nhận DANH SÁCH để tránh N+1.
	//
	// Trang danh sách offer cần tên seller của từng offer — phải là 1 truy
	// vấn, không phải một truy vấn cho mỗi offer.
	FindByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*Seller, error)

	List(ctx context.Context, f Filter) ([]*Seller, error)
}

// Filter là điều kiện lọc danh sách nhà bán.
type Filter struct {
	Status Status
	Type   SellerType
	Limit  int
	Offset int
}
