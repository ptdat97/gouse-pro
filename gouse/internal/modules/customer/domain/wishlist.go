package domain

import (
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Wishlist là danh sách yêu thích của một khách.
//
// MVP: mỗi khách có ĐÚNG MỘT danh sách mặc định. Nhiều danh sách có tên
// ("mùa hè", "quà tặng") là Phase 2 — nhưng entity tách sẵn từ đầu, vì gộp
// danh sách vào chính bảng item là thứ phải viết lại khi thêm tính năng.
type Wishlist struct {
	id         ids.ID
	customerID ids.ID

	name      string
	isDefault bool

	items []WishlistItem

	createdAt time.Time
	updatedAt time.Time
}

// WishlistItem là một món trong danh sách yêu thích.
type WishlistItem struct {
	// ProductID chứ KHÔNG phải VariantID.
	//
	// Khách yêu thích "chiếc áo này", không phải "chiếc áo này size M màu
	// đen". Lưu theo variant thì hết size M là món đồ biến mất khỏi danh
	// sách yêu thích — và khách nghĩ hệ thống mất dữ liệu của họ.
	ProductID ids.ID

	// VariantID TÙY CHỌN: khách CÓ THỂ nói rõ size mình muốn.
	//
	// Khi có, đây là tín hiệu nhu cầu mạnh hơn hẳn — nó nói chính xác
	// khách chờ size nào về hàng.
	VariantID ids.ID

	Note string

	AddedAt time.Time
}

// NewWishlist tạo danh sách yêu thích mặc định.
func NewWishlist(customerID ids.ID, now time.Time) *Wishlist {
	return &Wishlist{
		id:         ids.MustNew(ids.PrefixWishlist),
		customerID: customerID,
		name:       "Yêu thích",
		isDefault:  true,
		createdAt:  now,
		updatedAt:  now,
	}
}

// RestoreWishlistParams dựng lại danh sách từ kho lưu trữ.
type RestoreWishlistParams struct {
	ID         ids.ID
	CustomerID ids.ID
	Name       string
	IsDefault  bool
	Items      []WishlistItem
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func RestoreWishlist(p RestoreWishlistParams) *Wishlist {
	return &Wishlist{
		id:         p.ID,
		customerID: p.CustomerID,
		name:       p.Name,
		isDefault:  p.IsDefault,
		items:      p.Items,
		createdAt:  p.CreatedAt,
		updatedAt:  p.UpdatedAt,
	}
}

func (w *Wishlist) ID() ids.ID           { return w.id }
func (w *Wishlist) CustomerID() ids.ID   { return w.customerID }
func (w *Wishlist) Name() string         { return w.name }
func (w *Wishlist) IsDefault() bool      { return w.isDefault }
func (w *Wishlist) CreatedAt() time.Time { return w.createdAt }
func (w *Wishlist) UpdatedAt() time.Time { return w.updatedAt }

// Items trả BẢN SAO danh sách món.
//
// Bản sao chứ không phải slice gốc: trả slice gốc cho phép bên gọi sửa
// nội dung entity mà không đi qua bất kỳ quy tắc nào ở đây.
func (w *Wishlist) Items() []WishlistItem {
	out := make([]WishlistItem, len(w.items))
	copy(out, w.items)
	return out
}

func (w *Wishlist) Len() int { return len(w.items) }

// BelongsTo kiểm tra danh sách có thuộc về khách này không.
func (w *Wishlist) BelongsTo(customerID ids.ID) bool {
	return w.customerID == customerID
}

// Add thêm một món.
//
// IDEMPOTENT: thêm lại món đã có KHÔNG tạo bản sao và KHÔNG báo lỗi.
// Khách bấm tim hai lần là chuyện thường — báo lỗi ở đây biến một thao
// tác vô hại thành thông báo đỏ trên màn hình.
//
// Trả về true nếu món thật sự được thêm mới.
func (w *Wishlist) Add(item WishlistItem, now time.Time) bool {
	for _, existing := range w.items {
		if existing.ProductID == item.ProductID &&
			existing.VariantID == item.VariantID {
			return false
		}
	}

	item.Note = strings.TrimSpace(item.Note)
	item.AddedAt = now
	w.items = append(w.items, item)
	w.updatedAt = now
	return true
}

// Remove bỏ một món.
//
// IDEMPOTENT như Add: bỏ món không có trong danh sách là thành công, vì
// kết quả mong muốn (món đó không còn) đã đạt.
//
// Trả về true nếu thật sự có món bị bỏ.
func (w *Wishlist) Remove(productID, variantID ids.ID, now time.Time) bool {
	for i, item := range w.items {
		if item.ProductID == productID && item.VariantID == variantID {
			w.items = append(w.items[:i], w.items[i+1:]...)
			w.updatedAt = now
			return true
		}
	}
	return false
}

// Has cho biết món đã có trong danh sách chưa.
//
// Dùng cho trang sản phẩm: hiện trái tim đầy hay rỗng.
func (w *Wishlist) Has(productID, variantID ids.ID) bool {
	for _, item := range w.items {
		if item.ProductID == productID && item.VariantID == variantID {
			return true
		}
	}
	return false
}
