package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
)

// ---------------------------------------------------------------- Giữ hàng

type ReservationStore struct{ db querier }

func NewReservationStore(db querier) *ReservationStore {
	return &ReservationStore{db: db}
}

const reservationCols = `
	id, inventory_item_id, checkout_id, quantity, expires_at,
	status, extensions, created_at, updated_at, version`

func scanReservation(row pgx.Row) (*domain.Reservation, error) {
	var (
		p                      domain.RestoreReservationParams
		id, itemID, checkoutID string
		status                 string
	)
	err := row.Scan(&id, &itemID, &checkoutID, &p.Quantity, &p.ExpiresAt,
		&status, &p.Extensions, &p.CreatedAt, &p.UpdatedAt, &p.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p.ID = ids.ID(id)
	p.InventoryItemID = ids.ID(itemID)
	p.CheckoutID = ids.ID(checkoutID)
	p.Status = domain.ReservationStatus(status)
	return domain.RestoreReservation(p), nil
}

func (s *ReservationStore) Save(ctx context.Context, r *domain.Reservation) error {
	// KHÓA LẠC QUAN trên reservation.
	//
	// `WHERE reservation.version = $10` là thứ biến bất biến "nhả đúng
	// MỘT lần" từ một kiểm tra trong bộ nhớ thành một ràng buộc ở tầng dữ
	// liệu. Kiểm tra trong bộ nhớ thì hai giao dịch cùng đọc thấy ACTIVE,
	// cùng đi qua, và cùng ghi — đã xảy ra thật, xem migration 000028.
	//
	// Dòng chèn MỚI không đi qua nhánh conflict nên không bị chặn.
	const q = `
		INSERT INTO reservation (
			id, inventory_item_id, checkout_id, quantity, expires_at,
			status, extensions, created_at, updated_at, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET
			expires_at = EXCLUDED.expires_at,
			status     = EXCLUDED.status,
			extensions = EXCLUDED.extensions,
			updated_at = EXCLUDED.updated_at,
			version    = reservation.version + 1
		WHERE reservation.version = $10`

	tag, err := s.db.Exec(ctx, q,
		r.ID().String(), r.InventoryItemID().String(), r.CheckoutID().String(),
		r.Quantity(), r.ExpiresAt(), string(r.Status()), r.Extensions(),
		r.CreatedAt(), r.UpdatedAt(), r.Version())
	if err != nil {
		return fmt.Errorf("inventory: lưu giữ hàng: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Không dòng nào khớp nghĩa là phiên bản đã đổi — một giao dịch
		// khác vừa sửa bản ghi này. `withRetry` sẽ chạy lại từ đầu, đọc
		// lại trạng thái mới, và lúc đó quy tắc domain từ chối lượt nhả
		// thứ hai đúng như phải thế.
		return domain.ErrVersionConflict
	}
	return nil
}

func (s *ReservationStore) FindByID(ctx context.Context, id ids.ID) (*domain.Reservation, error) {
	return scanReservation(s.db.QueryRow(ctx,
		`SELECT `+reservationCols+` FROM reservation WHERE id = $1`, id.String()))
}

func (s *ReservationStore) FindByCheckout(
	ctx context.Context, checkoutID ids.ID,
) ([]*domain.Reservation, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+reservationCols+` FROM reservation WHERE checkout_id = $1 ORDER BY id`,
		checkoutID.String())
	if err != nil {
		return nil, fmt.Errorf("inventory: đọc giữ hàng theo checkout: %w", err)
	}
	defer rows.Close()
	return collectReservations(rows)
}

// FindExpired tìm reservation quá hạn chưa xử lý.
//
// Cơ chế hết hạn phải ĐÁNG TIN CẬY: nếu nó ngừng chạy, hàng bị khóa dần và
// cuối cùng không bán được gì (mục 6.3 của đặc tả).
//
// `limit` để tiến trình nền xử lý theo lô, không kéo hết một lần khi tồn
// đọng lớn.
func (s *ReservationStore) FindExpired(
	ctx context.Context, before time.Time, limit int,
) ([]*domain.Reservation, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx,
		`SELECT `+reservationCols+` FROM reservation
		 WHERE status = 'ACTIVE' AND expires_at <= $1
		 ORDER BY expires_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("inventory: đọc giữ hàng quá hạn: %w", err)
	}
	defer rows.Close()
	return collectReservations(rows)
}

// CountExpiredPending đếm reservation quá hạn chưa xử lý — chỉ báo giám sát.
//
// Cảnh báo khi > 100 (mục 13): con số tăng dần nghĩa là tiến trình dọn đã
// ngừng chạy, và hàng đang bị khóa mà không ai biết.
func (s *ReservationStore) CountExpiredPending(ctx context.Context, before time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM reservation WHERE status = 'ACTIVE' AND expires_at <= $1`,
		before).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("inventory: đếm giữ hàng quá hạn: %w", err)
	}
	return n, nil
}

func collectReservations(rows pgx.Rows) ([]*domain.Reservation, error) {
	var out []*domain.Reservation
	for rows.Next() {
		r, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------- Nhật ký

// MovementStore ghi và đọc nhật ký biến động.
//
// CHỈ CÓ Append và Find. Ở tầng database, trigger inventory_movement_immutable
// TỪ CHỐI THI HÀNH mọi UPDATE/DELETE — kể cả bằng tài khoản quản trị.
type MovementStore struct{ db querier }

func NewMovementStore(db querier) *MovementStore {
	return &MovementStore{db: db}
}

const movementCols = `
	id, inventory_item_id, sku_id, movement_type, quantity, quantity_after,
	reason, performed_by, reference_id, occurred_at`

func (s *MovementStore) Append(ctx context.Context, m *domain.InventoryMovement) error {
	const q = `
		INSERT INTO inventory_movement (
			id, inventory_item_id, sku_id, movement_type, quantity, quantity_after,
			reason, performed_by, reference_id, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	_, err := s.db.Exec(ctx, q,
		m.ID().String(), m.InventoryItemID().String(), m.SKUID().String(),
		string(m.Type()), m.Quantity(), m.QuantityAfter(),
		m.Reason(), m.PerformedBy().String(), m.ReferenceID().String(),
		m.OccurredAt())
	if err != nil {
		return fmt.Errorf("inventory: ghi nhật ký biến động: %w", err)
	}
	return nil
}

func scanMovement(row pgx.Row) (*domain.InventoryMovement, error) {
	var (
		p                        domain.RestoreMovementParams
		id, itemID, skuID, mType string
		performedBy, referenceID string
	)
	err := row.Scan(&id, &itemID, &skuID, &mType, &p.Quantity, &p.QuantityAfter,
		&p.Reason, &performedBy, &referenceID, &p.OccurredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p.ID = ids.ID(id)
	p.InventoryItemID = ids.ID(itemID)
	p.SKUID = ids.ID(skuID)
	p.Type = domain.MovementType(mType)
	p.PerformedBy = ids.ID(performedBy)
	p.ReferenceID = ids.ID(referenceID)
	return domain.RestoreMovement(p), nil
}

func (s *MovementStore) FindByItem(
	ctx context.Context, itemID ids.ID, limit int,
) ([]*domain.InventoryMovement, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx,
		`SELECT `+movementCols+` FROM inventory_movement
		 WHERE inventory_item_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT $2`,
		itemID.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("inventory: đọc nhật ký: %w", err)
	}
	defer rows.Close()
	return collectMovements(rows)
}

func (s *MovementStore) FindBySKU(
	ctx context.Context, skuID ids.ID, from, to time.Time,
) ([]*domain.InventoryMovement, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+movementCols+` FROM inventory_movement
		 WHERE sku_id = $1
		   AND ($2::timestamptz IS NULL OR occurred_at >= $2)
		   AND ($3::timestamptz IS NULL OR occurred_at < $3)
		 ORDER BY occurred_at, id`,
		skuID.String(), nullTime(from), nullTime(to))
	if err != nil {
		return nil, fmt.Errorf("inventory: đọc nhật ký theo SKU: %w", err)
	}
	defer rows.Close()
	return collectMovements(rows)
}

func collectMovements(rows pgx.Rows) ([]*domain.InventoryMovement, error) {
	var out []*domain.InventoryMovement
	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------- Giao dịch

// UnitOfWork chạy nhiều thao tác trong MỘT giao dịch.
//
// VÌ SAO CẦN: đổi số lượng và ghi nhật ký phải cùng thành công hoặc cùng
// thất bại. Số lượng đổi mà nhật ký không ghi thì quy tắc 4 bị vi phạm và
// sai lệch sau này không truy được nguyên nhân.
type UnitOfWork struct {
	pool *pgxpool.Pool
}

func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

func (u *UnitOfWork) Do(ctx context.Context, fn func(domain.Repos) error) error {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("inventory: mở giao dịch: %w", err)
	}
	// Rollback sau Commit thành công là no-op, nên defer luôn an toàn.
	defer func() { _ = tx.Rollback(ctx) }()

	repos := domain.Repos{
		Items:        NewItemStore(tx),
		Reservations: NewReservationStore(tx),
		Movements:    NewMovementStore(tx),
		Locations:    NewLocationStore(tx),
	}
	if err := fn(repos); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("inventory: xác nhận giao dịch: %w", err)
	}
	return nil
}

// ReposForTx trả bộ repository dùng GIAO DỊCH CỦA BÊN GỌI.
//
// Cần cho bên nhận domain event: dispatcher đã mở một giao dịch để đánh
// dấu event đã xử lý, và việc thay đổi tồn kho phải nằm TRONG giao dịch
// đó. Hai giao dịch tách rời nghĩa là commit tồn kho có thể thành công
// trong khi việc đánh dấu thất bại — lần thử lại sẽ commit lần thứ hai.
//
// KHÔNG dùng cho đường đi thông thường: UnitOfWork có cơ chế thử lại khi
// xung đột phiên bản, còn hàm này không.
func ReposForTx(tx querier) domain.Repos {
	return domain.Repos{
		Items:        NewItemStore(tx),
		Reservations: NewReservationStore(tx),
		Movements:    NewMovementStore(tx),
		Locations:    NewLocationStore(tx),
	}
}

// Repos trả về bộ repository dùng NGOÀI giao dịch (chỉ đọc là chính).
func Repos(pool *pgxpool.Pool) domain.Repos {
	return domain.Repos{
		Items:        NewItemStore(pool),
		Reservations: NewReservationStore(pool),
		Movements:    NewMovementStore(pool),
		Locations:    NewLocationStore(pool),
	}
}
