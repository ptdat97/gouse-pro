package domain

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// LocationKind phân biệt kho của nền tảng với kho riêng của seller.
//
// KHÁC với chủ sở hữu hàng (InventoryItem.OwnerID): hàng của seller gửi ở
// kho nền tảng vẫn THUỘC SỞ HỮU seller. Trộn hai khái niệm này lại là ghi
// nhận sai tài sản trên sổ sách (inventory.md mục 3.1).
type LocationKind string

const (
	LocationPlatform LocationKind = "PLATFORM"
	LocationSeller   LocationKind = "SELLER"
)

func (k LocationKind) IsValid() bool {
	return k == LocationPlatform || k == LocationSeller
}

// StockLocation là một nơi chứa hàng.
//
// KHÔNG có hành vi nghiệp vụ nào: mọi quy tắc về số lượng nằm ở
// InventoryItem. Kiểu này tồn tại để bản ghi tồn kho có chỗ để trỏ tới.
type StockLocation struct {
	ID   ids.ID
	Name string

	// Code là mã người dùng đặt, DUY NHẤT toàn hệ thống.
	//
	// Có mã riêng ngoài id vì nhân viên kho đọc và gõ nó hằng ngày —
	// "HCM-01" dùng được, `loc_01M02...` thì không.
	Code string

	Kind      LocationKind
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewStockLocation dựng một kho mới.
func NewStockLocation(name, code string, kind LocationKind, now time.Time) (*StockLocation, error) {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)

	if name == "" {
		return nil, errors.New("inventory: kho phải có tên")
	}
	if code == "" {
		return nil, errors.New("inventory: kho phải có mã")
	}
	if !kind.IsValid() {
		return nil, errors.New("inventory: loại kho không hợp lệ")
	}

	id, err := ids.New(ids.PrefixStockLocation)
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &StockLocation{
		ID:        id,
		Name:      name,
		Code:      code,
		Kind:      kind,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// LocationRepository là PORT cho kho hàng.
type LocationRepository interface {
	// EnsureByCode trả kho có mã này, tạo mới nếu chưa có.
	//
	// Idempotent theo MÃ chứ không theo id: bên gọi biết "kho HCM-01",
	// không biết định danh nội bộ. Gọi lại lần hai trả đúng kho cũ thay vì
	// tạo kho thứ hai cùng mã — thứ mà ràng buộc UNIQUE sẽ từ chối.
	EnsureByCode(ctx context.Context, l *StockLocation) (*StockLocation, error)

	FindByCode(ctx context.Context, code string) (*StockLocation, error)
}
