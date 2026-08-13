package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory/application"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
	inventorypg "github.com/fashion-commerce/platform/internal/modules/inventory/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
}

// Bảo đảm lúc biên dịch rằng Module thỏa mãn API.
var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Không có bản in-memory là quyết định CÓ CHỦ ĐÍCH: cơ chế cốt lõi của
	// inventory là KHÓA LẠC QUAN, và nó không kiểm chứng được bằng bộ nhớ —
	// cần hai giao dịch database thật chạy song song trên cùng một dòng.
	//
	// Một bản in-memory ở đây sẽ tạo cảm giác an toàn giả: test xanh mà
	// không chứng minh được điều quan trọng nhất.
	Storage string

	// DB là kết nối database. BẮT BUỘC.
	DB *database.DB

	// Clock cho phép test kiểm soát thời gian.
	Clock application.Clock
}

// New khởi tạo module inventory.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"inventory: chỉ hỗ trợ kho lưu trữ postgres — khóa lạc quan không " +
				"kiểm chứng được bằng bộ nhớ")
	}
	if cfg.DB == nil {
		return nil, errors.New("inventory: bắt buộc phải có kết nối database")
	}

	pool := cfg.DB.Pool()
	return &Module{svc: application.NewService(application.Deps{
		UnitOfWork: inventorypg.NewUnitOfWork(pool),
		Repos:      inventorypg.Repos(pool),
		Clock:      cfg.Clock,
	})}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- API

func (m *Module) GetAvailability(
	ctx context.Context, skuIDs []string, locationID string,
) (map[string]int, error) {
	parsed := make([]ids.ID, 0, len(skuIDs))
	for _, raw := range skuIDs {
		id, err := ids.Parse(raw, ids.PrefixSKU)
		if err != nil {
			// Bỏ qua id sai định dạng thay vì làm hỏng cả lời gọi.
			continue
		}
		parsed = append(parsed, id)
	}

	var locID ids.ID
	if locationID != "" {
		id, err := ids.Parse(locationID, ids.PrefixStockLocation)
		if err != nil {
			return nil, ErrInvalidID
		}
		locID = id
	}

	found, err := m.svc.GetAvailability(ctx, parsed, locID)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]int, len(found))
	for skuID, av := range found {
		out[skuID.String()] = av.Available
	}
	return out, nil
}

// CheckAvailability kiểm tra đủ hàng cho cả giỏ.
//
// Trả về TỪNG món thiếu: "chỉ còn 2 sản phẩm" hữu ích hơn nhiều so với
// "giỏ hàng có vấn đề" — khách quyết định được giảm số lượng hay bỏ món đó.
func (m *Module) CheckAvailability(
	ctx context.Context, items []AvailabilityRequest,
) (AvailabilityResult, error) {
	skuIDs := make([]ids.ID, 0, len(items))
	for _, it := range items {
		id, err := ids.Parse(it.SKUID, ids.PrefixSKU)
		if err != nil {
			return AvailabilityResult{}, ErrInvalidID
		}
		skuIDs = append(skuIDs, id)
	}

	// Địa điểm lấy từ dòng đầu; MVP chỉ có một địa điểm (mục 14).
	var locID ids.ID
	if len(items) > 0 && items[0].LocationID != "" {
		id, err := ids.Parse(items[0].LocationID, ids.PrefixStockLocation)
		if err != nil {
			return AvailabilityResult{}, ErrInvalidID
		}
		locID = id
	}

	found, err := m.svc.GetAvailability(ctx, skuIDs, locID)
	if err != nil {
		return AvailabilityResult{}, translateErr(err)
	}

	out := AvailabilityResult{AllAvailable: true}
	for _, it := range items {
		skuID := ids.ID(it.SKUID)
		available := found[skuID].Available
		if available < it.Quantity {
			out.AllAvailable = false
			out.Insufficient = append(out.Insufficient, InsufficientItem{
				SKUID:     it.SKUID,
				Requested: it.Quantity,
				Available: available,
			})
		}
	}
	return out, nil
}

func (m *Module) Reserve(ctx context.Context, req ReserveRequest) (*ReservationView, error) {
	itemID, err := ids.Parse(req.ItemID, ids.PrefixInventoryItem)
	if err != nil {
		return nil, ErrInvalidID
	}

	var checkoutID ids.ID
	if req.CheckoutID != "" {
		id, err := ids.Parse(req.CheckoutID, ids.PrefixCheckout)
		if err != nil {
			return nil, ErrInvalidID
		}
		checkoutID = id
	}

	r, err := m.svc.Reserve(ctx, application.ReserveInput{
		ItemID:     itemID,
		CheckoutID: checkoutID,
		Quantity:   req.Quantity,
		TTL:        req.TTL,
	})
	if err != nil {
		return nil, translateErr(err)
	}

	v := toReservationView(r)
	return &v, nil
}

func (m *Module) ReleaseReservation(ctx context.Context, reservationID string) error {
	id, err := ids.Parse(reservationID, ids.PrefixReservation)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.Release(ctx, id))
}

func (m *Module) ExtendReservation(ctx context.Context, reservationID string, ttl time.Duration) error {
	id, err := ids.Parse(reservationID, ids.PrefixReservation)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.ExtendReservation(ctx, id, ttl))
}

func (m *Module) Commit(ctx context.Context, reservationID string) error {
	id, err := ids.Parse(reservationID, ids.PrefixReservation)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.Commit(ctx, id))
}

func (m *Module) Ship(ctx context.Context, itemID string, quantity int, orderID string) error {
	id, err := ids.Parse(itemID, ids.PrefixInventoryItem)
	if err != nil {
		return ErrInvalidID
	}
	var ordID ids.ID
	if orderID != "" {
		parsed, err := ids.Parse(orderID, ids.PrefixOrder)
		if err != nil {
			return ErrInvalidID
		}
		ordID = parsed
	}
	return translateErr(m.svc.Ship(ctx, id, quantity, ordID))
}

func (m *Module) Receive(ctx context.Context, req ReceiveRequest) (*ItemView, error) {
	skuID, err := ids.Parse(req.SKUID, ids.PrefixSKU)
	if err != nil {
		return nil, ErrInvalidID
	}
	locID, err := ids.Parse(req.LocationID, ids.PrefixStockLocation)
	if err != nil {
		return nil, ErrInvalidID
	}

	// Rỗng = hàng của nền tảng. Dùng hằng thay vì chuỗi rỗng để phân biệt
	// "hàng nền tảng" với "quên điền chủ sở hữu".
	ownerID := domain.PlatformOwner
	if req.OwnerID != "" {
		ownerID = ids.ID(req.OwnerID)
	}

	item, err := m.svc.Receive(ctx, application.ReceiveInput{
		Key:         domain.ItemKey{SKUID: skuID, LocationID: locID, OwnerID: ownerID},
		Quantity:    req.Quantity,
		ReferenceID: ids.ID(req.ReferenceID),
		PerformedBy: ids.ID(req.PerformedBy),
		BatchID:     ids.ID(req.BatchID),
	})
	if err != nil {
		return nil, translateErr(err)
	}

	v := toItemView(item)
	return &v, nil
}

func (m *Module) Adjust(ctx context.Context, req AdjustRequest) error {
	id, err := ids.Parse(req.ItemID, ids.PrefixInventoryItem)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.Adjust(ctx, application.AdjustInput{
		ItemID:      id,
		Delta:       req.Delta,
		Reason:      req.Reason,
		PerformedBy: ids.ID(req.PerformedBy),
	}))
}

func (m *Module) ReceiveReturn(ctx context.Context, itemID string, quantity int, returnID string) error {
	id, err := ids.Parse(itemID, ids.PrefixInventoryItem)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.ReceiveReturn(ctx, id, quantity, ids.ID(returnID)))
}

func (m *Module) ProcessReturnInspection(ctx context.Context, req InspectionRequest) error {
	id, err := ids.Parse(req.ItemID, ids.PrefixInventoryItem)
	if err != nil {
		return ErrInvalidID
	}
	return translateErr(m.svc.InspectReturn(
		ctx, id, req.Quantity, req.Passed, req.Reason, ids.ID(req.PerformedBy)))
}

// ---------------------------------------------------------------- Chuyển đổi

func (m *Module) GetItemsBySKUs(
	ctx context.Context, skuIDs []string, locationID string,
) (map[string][]ItemView, error) {
	parsed := make([]ids.ID, 0, len(skuIDs))
	for _, raw := range skuIDs {
		id, err := ids.Parse(raw, ids.PrefixSKU)
		if err != nil {
			continue
		}
		parsed = append(parsed, id)
	}

	var locID ids.ID
	if locationID != "" {
		id, err := ids.Parse(locationID, ids.PrefixStockLocation)
		if err != nil {
			return nil, ErrInvalidID
		}
		locID = id
	}

	found, err := m.svc.GetItemsBySKUs(ctx, parsed, locID)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string][]ItemView, len(found))
	for skuID, items := range found {
		views := make([]ItemView, 0, len(items))
		for _, it := range items {
			views = append(views, toItemView(it))
		}
		out[skuID.String()] = views
	}
	return out, nil
}

func toItemView(i *domain.InventoryItem) ItemView {
	q := i.Quantities()
	return ItemView{
		ID:              i.ID().String(),
		SKUID:           i.SKUID().String(),
		LocationID:      i.LocationID().String(),
		OwnerID:         i.OwnerID().String(),
		Available:       q.Available(),
		Reserved:        q.Reserved(),
		Committed:       q.Committed(),
		InTransit:       q.InTransit(),
		Damaged:         q.Damaged(),
		Returned:        q.Returned(),
		Total:           q.Total(),
		IsPlatformOwned: i.IsPlatformOwned(),
		Version:         i.Version(),
	}
}

func toReservationView(r *domain.Reservation) ReservationView {
	return ReservationView{
		ID:         r.ID().String(),
		ItemID:     r.InventoryItemID().String(),
		CheckoutID: r.CheckoutID().String(),
		Quantity:   r.Quantity(),
		ExpiresAt:  r.ExpiresAt().Format(time.RFC3339),
		Status:     string(r.Status()),
	}
}

// translateErr chuyển lỗi nội bộ sang lỗi công khai.
//
// PHÂN BIỆT hết hàng với xung đột là quan trọng với bên gọi: hết hàng thì
// KHÔNG thử lại (hàng không tự xuất hiện), xung đột thì NÊN thử lại.
func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrInsufficientStock):
		return ErrInsufficientStock
	case errors.Is(err, domain.ErrVersionConflict):
		return ErrConflict
	}
	return err
}
