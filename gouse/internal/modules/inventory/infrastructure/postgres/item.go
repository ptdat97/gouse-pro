// Package postgres cài đặt các port của inventory bằng PostgreSQL.
//
// ĐÂY LÀ NƠI KHÓA LẠC QUAN ĐƯỢC CÀI ĐẶT — cơ chế chống bán quá số lượng.
// Xem docs/04-modules/inventory.md mục 5.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
)

// querier là phần chung giữa *pgxpool.Pool và pgx.Tx.
//
// Nhờ nó, cùng một repository dùng được cả trong lẫn ngoài giao dịch —
// không phải viết hai bản gần giống nhau.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ItemStore lưu bản ghi tồn kho trong PostgreSQL.
type ItemStore struct {
	db querier
}

func NewItemStore(db querier) *ItemStore {
	return &ItemStore{db: db}
}

const itemCols = `
	id, sku_id, stock_location_id, inventory_owner_id,
	quantity_available, quantity_reserved, quantity_committed,
	quantity_in_transit, quantity_damaged, quantity_returned,
	production_batch_id, version, created_at, updated_at`

func scanItem(row pgx.Row) (*domain.InventoryItem, error) {
	var (
		p                         domain.RestoreItemParams
		id, skuID, locID, ownerID string
		batchID                   string
		av, rs, cm, it, dm, rt    int
	)
	err := row.Scan(&id, &skuID, &locID, &ownerID,
		&av, &rs, &cm, &it, &dm, &rt,
		&batchID, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	q, err := domain.NewQuantities(av, rs, cm, it, dm, rt)
	if err != nil {
		// Số lượng âm trong database là lỗi NGHIÊM TRỌNG: ràng buộc CHECK
		// lẽ ra đã chặn. Báo rõ thay vì âm thầm trả về dữ liệu hỏng.
		return nil, fmt.Errorf("inventory: dữ liệu tồn kho hỏng ở bản ghi %s: %w", id, err)
	}

	p.ID = ids.ID(id)
	p.SKUID = ids.ID(skuID)
	p.LocationID = ids.ID(locID)
	p.OwnerID = ids.ID(ownerID)
	p.ProductionBatchID = ids.ID(batchID)
	p.Quantities = q
	return domain.RestoreInventoryItem(p), nil
}

func (s *ItemStore) Create(ctx context.Context, item *domain.InventoryItem) error {
	const q = `
		INSERT INTO inventory_item (
			id, sku_id, stock_location_id, inventory_owner_id,
			quantity_available, quantity_reserved, quantity_committed,
			quantity_in_transit, quantity_damaged, quantity_returned,
			production_batch_id, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`

	qty := item.Quantities()
	_, err := s.db.Exec(ctx, q,
		item.ID().String(), item.SKUID().String(), item.LocationID().String(),
		item.OwnerID().String(),
		qty.Available(), qty.Reserved(), qty.Committed(),
		qty.InTransit(), qty.Damaged(), qty.Returned(),
		item.ProductionBatchID().String(), item.Version(),
		item.CreatedAt(), item.UpdatedAt())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "inventory_item_key_uniq" {
			return domain.ErrDuplicateItem
		}
		return fmt.Errorf("inventory: tạo bản ghi tồn kho: %w", err)
	}
	return nil
}

func (s *ItemStore) FindByID(ctx context.Context, id ids.ID) (*domain.InventoryItem, error) {
	return scanItem(s.db.QueryRow(ctx,
		`SELECT `+itemCols+` FROM inventory_item WHERE id = $1`, id.String()))
}

func (s *ItemStore) FindByKey(ctx context.Context, key domain.ItemKey) (*domain.InventoryItem, error) {
	return scanItem(s.db.QueryRow(ctx,
		`SELECT `+itemCols+` FROM inventory_item
		 WHERE sku_id = $1 AND stock_location_id = $2 AND inventory_owner_id = $3`,
		key.SKUID.String(), key.LocationID.String(), key.OwnerID.String()))
}

// FindBySKUs lấy tồn kho của nhiều SKU trong MỘT truy vấn.
//
// locationID rỗng nghĩa là mọi địa điểm — cần cho câu hỏi "tổng còn bao
// nhiêu trên toàn hệ thống".
func (s *ItemStore) FindBySKUs(
	ctx context.Context, skuIDs []ids.ID, locationID ids.ID,
) (map[ids.ID][]*domain.InventoryItem, error) {
	out := make(map[ids.ID][]*domain.InventoryItem, len(skuIDs))
	if len(skuIDs) == 0 {
		return out, nil
	}

	rows, err := s.db.Query(ctx,
		`SELECT `+itemCols+` FROM inventory_item
		 WHERE sku_id = ANY($1)
		   AND ($2 = '' OR stock_location_id = $2)
		 ORDER BY id`,
		toStrings(skuIDs), locationID.String())
	if err != nil {
		return nil, fmt.Errorf("inventory: đọc tồn kho theo lô: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: đọc tồn kho theo lô: %w", err)
		}
		out[item.SKUID()] = append(out[item.SKUID()], item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inventory: đọc tồn kho theo lô: %w", err)
	}
	return out, nil
}

func (s *ItemStore) FindByOwner(
	ctx context.Context, ownerID ids.ID, limit, offset int,
) ([]*domain.InventoryItem, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx,
		`SELECT `+itemCols+` FROM inventory_item
		 WHERE inventory_owner_id = $1 ORDER BY id LIMIT $2 OFFSET $3`,
		ownerID.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("inventory: đọc tồn kho theo chủ sở hữu: %w", err)
	}
	defer rows.Close()

	var out []*domain.InventoryItem
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("inventory: đọc tồn kho theo chủ sở hữu: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inventory: đọc tồn kho theo chủ sở hữu: %w", err)
	}
	return out, nil
}

// ApplyChange ghi thay đổi số lượng bằng KHÓA LẠC QUAN.
//
// ĐÂY LÀ HÀM QUAN TRỌNG NHẤT CỦA CẢ MODULE.
//
// Hai điều kiện trong WHERE là mấu chốt (mục 5.2 của đặc tả):
//
//	version = $expected  → phát hiện có tiến trình khác vừa sửa
//	quantity_* >= 0      → do ràng buộc CHECK của database bảo đảm
//
// Câu UPDATE này KIỂM TRA VÀ CẬP NHẬT NGUYÊN TỬ. Không có khoảng trống
// giữa đọc và ghi, nên hai khách mua cùng lúc không thể cùng thắng:
// PostgreSQL tuần tự hóa hai câu UPDATE trên cùng một dòng, người thứ hai
// thấy version đã đổi và bị từ chối.
//
// VÌ SAO KHÔNG DÙNG SELECT ... FOR UPDATE (khóa bi quan):
// với live commerce, hàng nghìn người mua cùng một SKU trong vài giây.
// Khóa bi quan tạo hàng đợi tuần tự — mọi request xếp hàng chờ, độ trễ
// tăng vọt, kết nối database cạn kiệt. Khóa lạc quan cho phép xử lý song
// song, chỉ request thật sự xung đột mới phải thử lại.
func (s *ItemStore) ApplyChange(
	ctx context.Context, item *domain.InventoryItem, expectedVersion int64,
) error {
	const q = `
		UPDATE inventory_item SET
			quantity_available  = $1,
			quantity_reserved   = $2,
			quantity_committed  = $3,
			quantity_in_transit = $4,
			quantity_damaged    = $5,
			quantity_returned   = $6,
			version             = version + 1,
			updated_at          = $7
		WHERE id = $8 AND version = $9`

	qty := item.Quantities()
	tag, err := s.db.Exec(ctx, q,
		qty.Available(), qty.Reserved(), qty.Committed(),
		qty.InTransit(), qty.Damaged(), qty.Returned(),
		item.UpdatedAt(), item.ID().String(), expectedVersion)
	if err != nil {
		// Vi phạm CHECK nghĩa là số lượng âm lọt được tới database — lỗi
		// logic ở tầng ứng dụng. Chỉ báo "tồn kho âm" phải LUÔN bằng 0.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			return fmt.Errorf("%w: ràng buộc %s",
				domain.ErrNegativeQuantity, pgErr.ConstraintName)
		}
		return fmt.Errorf("inventory: ghi thay đổi tồn kho: %w", err)
	}

	// 0 dòng bị ảnh hưởng = version không khớp = có người khác vừa sửa.
	//
	// Bên gọi phân biệt lỗi này với ErrInsufficientStock: xung đột phiên
	// bản NÊN thử lại, hết hàng thì KHÔNG (mục 5.4).
	if tag.RowsAffected() == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

// ---------------------------------------------------------------- Tiện ích

func toStrings(list []ids.ID) []string {
	out := make([]string, len(list))
	for i, id := range list {
		out[i] = id.String()
	}
	return out
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
