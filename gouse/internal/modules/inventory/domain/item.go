package domain

import (
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrMissingSKU      = errors.New("inventory: thiếu định danh SKU")
	ErrMissingLocation = errors.New("inventory: thiếu địa điểm lưu kho")
	ErrMissingOwner    = errors.New("inventory: thiếu chủ sở hữu tồn kho")

	// ErrVersionConflict khi có tiến trình khác vừa sửa cùng bản ghi.
	//
	// Khác hoàn toàn với ErrInsufficientStock: xung đột phiên bản NÊN thử
	// lại (lần sau có thể thành công), còn hết hàng thì KHÔNG (thử lại chỉ
	// lãng phí tài nguyên). Xem mục 5.4 của đặc tả.
	ErrVersionConflict = errors.New("inventory: xung đột phiên bản, có tiến trình khác vừa sửa")
)

// PlatformOwner là định danh chủ sở hữu cho hàng của nền tảng.
//
// Dùng hằng thay vì chuỗi rỗng để phân biệt rõ "hàng của nền tảng" với
// "quên điền chủ sở hữu" — hai thứ rất khác nhau khi đối soát tài sản.
const PlatformOwner ids.ID = "own_platform"

// InventoryItem là tồn kho của MỘT SKU tại MỘT địa điểm thuộc MỘT chủ sở hữu.
//
// KHÓA ĐỊNH DANH NGHIỆP VỤ: (sku_id, stock_location_id, inventory_owner_id).
//
// Ba trường này cho phép mô hình hóa cả ba mô hình fulfillment bằng MỘT
// cấu trúc (mục 3.1 của đặc tả):
//
//	Own brand:         owner=PLATFORM,  location=kho nền tảng
//	Seller tự giao:    owner=seller_A,  location=kho seller A
//	Nền tảng giao hộ:  owner=seller_A,  location=kho nền tảng
//
// Trường hợp thứ ba là lý do phải TÁCH owner khỏi location: hàng nằm ở kho
// nền tảng nhưng THUỘC SỞ HỮU seller, không được ghi nhận là tài sản của
// nền tảng.
type InventoryItem struct {
	id         ids.ID
	skuID      ids.ID
	locationID ids.ID
	ownerID    ids.ID

	quantities Quantities

	// productionBatchID cho phép truy vết theo lô sản xuất (Phase 3).
	// Rỗng nghĩa là không theo dõi theo lô.
	productionBatchID ids.ID

	// version là bộ đếm cho KHÓA LẠC QUAN.
	//
	// Mỗi lần ghi thành công, database tăng version lên 1. Lần ghi tiếp
	// theo phải mang đúng version đã đọc, nếu không nghĩa là có người khác
	// vừa sửa và thao tác bị từ chối.
	//
	// Xem mục 5.2 của đặc tả và infrastructure/postgres/item.go.
	version int64

	createdAt time.Time
	updatedAt time.Time
}

type NewItemParams struct {
	SKUID             ids.ID
	LocationID        ids.ID
	OwnerID           ids.ID
	ProductionBatchID ids.ID
	Now               time.Time
}

// NewInventoryItem tạo bản ghi tồn kho rỗng.
//
// Số lượng ban đầu LUÔN bằng 0. Hàng vào kho phải qua Receive để có bản
// ghi trong nhật ký biến động (quy tắc 4) — tạo item với số lượng sẵn sẽ
// tạo ra hàng "từ hư không" mà không truy vết được.
func NewInventoryItem(p NewItemParams) (*InventoryItem, error) {
	if p.SKUID.IsZero() {
		return nil, ErrMissingSKU
	}
	if p.LocationID.IsZero() {
		return nil, ErrMissingLocation
	}
	if p.OwnerID.IsZero() {
		return nil, ErrMissingOwner
	}

	id, err := ids.New(ids.PrefixInventoryItem)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &InventoryItem{
		id:                id,
		skuID:             p.SKUID,
		locationID:        p.LocationID,
		ownerID:           p.OwnerID,
		productionBatchID: p.ProductionBatchID,
		quantities:        Empty(),
		version:           0,
		createdAt:         now,
		updatedAt:         now,
	}, nil
}

// RestoreItemParams dựng lại từ kho lưu trữ.
type RestoreItemParams struct {
	ID                ids.ID
	SKUID             ids.ID
	LocationID        ids.ID
	OwnerID           ids.ID
	Quantities        Quantities
	ProductionBatchID ids.ID
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// RestoreInventoryItem dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreInventoryItem(p RestoreItemParams) *InventoryItem {
	return &InventoryItem{
		id:                p.ID,
		skuID:             p.SKUID,
		locationID:        p.LocationID,
		ownerID:           p.OwnerID,
		quantities:        p.Quantities,
		productionBatchID: p.ProductionBatchID,
		version:           p.Version,
		createdAt:         p.CreatedAt,
		updatedAt:         p.UpdatedAt,
	}
}

func (i *InventoryItem) ID() ids.ID                { return i.id }
func (i *InventoryItem) SKUID() ids.ID             { return i.skuID }
func (i *InventoryItem) LocationID() ids.ID        { return i.locationID }
func (i *InventoryItem) OwnerID() ids.ID           { return i.ownerID }
func (i *InventoryItem) Quantities() Quantities    { return i.quantities }
func (i *InventoryItem) ProductionBatchID() ids.ID { return i.productionBatchID }
func (i *InventoryItem) Version() int64            { return i.version }
func (i *InventoryItem) CreatedAt() time.Time      { return i.createdAt }
func (i *InventoryItem) UpdatedAt() time.Time      { return i.updatedAt }

// IsPlatformOwned cho biết hàng thuộc sở hữu nền tảng hay seller.
//
// Quan trọng khi đối soát: hàng của seller nằm ở kho nền tảng KHÔNG phải
// tài sản của nền tảng.
func (i *InventoryItem) IsPlatformOwned() bool { return i.ownerID == PlatformOwner }

// Available là số lượng bán được — câu hỏi thường gặp nhất của module này.
func (i *InventoryItem) Available() int { return i.quantities.Available() }

// ---------------------------------------------------------------- Hành vi

// apply áp dụng một phép biến đổi số lượng lên aggregate.
//
// Gom vào một chỗ để mọi thay đổi đều đi qua cùng một đường: cập nhật số
// lượng và đánh dấu thời điểm sửa. version KHÔNG tăng ở đây — nó do
// DATABASE tăng lúc ghi, vì chỉ database mới biết thao tác có thắng cuộc
// tranh chấp hay không.
func (i *InventoryItem) apply(fn func(Quantities) (Quantities, error), now time.Time) error {
	next, err := fn(i.quantities)
	if err != nil {
		return err
	}
	i.quantities = next
	i.touch(now)
	return nil
}

func (i *InventoryItem) Reserve(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.Reserve(qty) }, now)
}

func (i *InventoryItem) Release(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.Release(qty) }, now)
}

func (i *InventoryItem) Commit(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.Commit(qty) }, now)
}

func (i *InventoryItem) Uncommit(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.Uncommit(qty) }, now)
}

func (i *InventoryItem) Ship(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.Ship(qty) }, now)
}

func (i *InventoryItem) Receive(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.Receive(qty) }, now)
}

func (i *InventoryItem) ReceiveReturn(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.ReceiveReturn(qty) }, now)
}

func (i *InventoryItem) InspectionPassed(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.InspectionPassed(qty) }, now)
}

func (i *InventoryItem) InspectionFailed(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.InspectionFailed(qty) }, now)
}

func (i *InventoryItem) MarkDamaged(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.MarkDamaged(qty) }, now)
}

func (i *InventoryItem) SendInTransit(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.SendInTransit(qty) }, now)
}

func (i *InventoryItem) ArriveFromTransit(qty int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.ArriveFromTransit(qty) }, now)
}

func (i *InventoryItem) AdjustAvailable(delta int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) { return q.AdjustAvailable(delta) }, now)
}

// KiemKe đặt số khả dụng và số hỏng theo kết quả đếm. Xem Quantities.KiemKe.
func (i *InventoryItem) KiemKe(available, damaged *int, now time.Time) error {
	return i.apply(func(q Quantities) (Quantities, error) {
		return q.KiemKe(available, damaged)
	}, now)
}

func (i *InventoryItem) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	i.updatedAt = now
}
