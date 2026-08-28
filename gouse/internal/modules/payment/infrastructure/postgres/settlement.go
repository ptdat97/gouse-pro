package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

// SettlementStore lưu đợt đối soát.
type SettlementStore struct{ pool *pgxpool.Pool }

func NewSettlementStore(pool *pgxpool.Pool) *SettlementStore {
	return &SettlementStore{pool: pool}
}

const stlCols = `
	id, seller_id, period_start, period_end, status,
	gross_amount, deficit_amount, net_amount, currency,
	created_at, confirmed_at, paid_at, version, updated_at`

// Luu ghi đợt đối soát VÀ các dòng của nó trong MỘT giao dịch.
//
// Ràng buộc UNIQUE trên `ledger_entry_id` là thứ chặn một bút toán lọt vào
// hai đợt. Hai lượt chạy job chồng nhau đều thấy "chưa gom" nếu chỉ kiểm ở
// tầng ứng dụng — và khi đó nhà bán được trả tiền hai lần cho cùng một đơn.
func (s *SettlementStore) Luu(ctx context.Context, d *domain.DoiSoat) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("payment: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		INSERT INTO settlement (`+stlCols+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (id) DO UPDATE SET
			status       = EXCLUDED.status,
			confirmed_at = EXCLUDED.confirmed_at,
			paid_at      = EXCLUDED.paid_at,
			updated_at   = EXCLUDED.updated_at,
			version      = settlement.version + 1
		WHERE settlement.version = $13`,
		d.ID().String(), d.SellerID().String(), d.PeriodStart(), d.PeriodEnd(),
		string(d.Status()), d.Gross().Amount(), d.Deficit().Amount(),
		d.Net().Amount(), string(d.Gross().Currency()),
		d.CreatedAt(), nullTime(d.ConfirmedAt()), nullTime(d.PaidAt()),
		d.Version(), d.UpdatedAt())
	if err != nil {
		return fmt.Errorf("payment: ghi đợt đối soát: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrVersionConflict
	}

	for _, dong := range d.Dong() {
		_, err := tx.Exec(ctx, `
			INSERT INTO settlement_line (
				id, settlement_id, ledger_entry_id, amount, currency
			) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (id) DO NOTHING`,
			dong.ID.String(), d.ID().String(), dong.LedgerEntryID.String(),
			dong.Amount.Amount(), string(dong.Amount.Currency()))
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) &&
				pgErr.ConstraintName == "settlement_line_ledger_entry_id_key" {
				// Bút toán này đã thuộc một đợt khác. KHÔNG phải lỗi hệ
				// thống — hai lượt job chồng nhau là chuyện thường — nhưng
				// cả đợt phải quay lui, nếu không nó thiếu mất một dòng mà
				// tổng tiền vẫn tính cả dòng ấy.
				return domain.ErrDuplicateEntry
			}
			return fmt.Errorf("payment: ghi dòng đối soát: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("payment: xác nhận giao dịch: %w", err)
	}
	return nil
}

func (s *SettlementStore) TimTheoID(ctx context.Context, id ids.ID) (*domain.DoiSoat, error) {
	ds, err := s.doc(ctx, `WHERE id = $1`, id.String())
	if err != nil {
		return nil, err
	}
	if len(ds) == 0 {
		return nil, domain.ErrSettlementNotFound
	}
	return ds[0], nil
}

func (s *SettlementStore) TimTheoNhaBan(
	ctx context.Context, sellerID ids.ID, limit int,
) ([]*domain.DoiSoat, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.doc(ctx,
		`WHERE seller_id = $1 ORDER BY created_at DESC LIMIT $2`,
		sellerID.String(), limit)
}

// ButToanChuaGom trả các bút toán SELLER_RELEASE chưa thuộc đợt nào.
//
// Gom theo nhà bán để job tạo mỗi nhà bán một đợt.
func (s *SettlementStore) ButToanChuaGom(
	ctx context.Context, denNgay time.Time, limit int,
) (map[ids.ID][]domain.DongDoiSoat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.account_owner_id, e.id, l.amount, l.currency
		  FROM ledger_entry e
		  JOIN ledger_line l ON l.entry_id = e.id
		 WHERE e.entry_type = 'SELLER_RELEASE'
		   AND l.account_type = 'SELLER_AVAILABLE'
		   AND l.direction = 'CREDIT'
		   AND e.created_at <= $1
		   AND NOT EXISTS (
		       SELECT 1 FROM settlement_line sl WHERE sl.ledger_entry_id = e.id
		   )
		 ORDER BY l.account_owner_id, e.created_at
		 LIMIT $2`, denNgay, limit)
	if err != nil {
		return nil, fmt.Errorf("payment: tìm bút toán chưa gom: %w", err)
	}
	defer rows.Close()

	out := map[ids.ID][]domain.DongDoiSoat{}
	for rows.Next() {
		var owner, entryID, tienTe string
		var soTien int64
		if err := rows.Scan(&owner, &entryID, &soTien, &tienTe); err != nil {
			return nil, err
		}
		m, err := money.New(soTien, money.Currency(tienTe))
		if err != nil {
			return nil, err
		}
		out[ids.ID(owner)] = append(out[ids.ID(owner)], domain.DongDoiSoat{
			ID:            ids.MustNew(ids.PrefixSettlement),
			LedgerEntryID: ids.ID(entryID),
			Amount:        m,
		})
	}
	return out, rows.Err()
}

func (s *SettlementStore) doc(
	ctx context.Context, where string, args ...any,
) ([]*domain.DoiSoat, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+stlCols+` FROM settlement `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("payment: đọc đợt đối soát: %w", err)
	}
	defer rows.Close()

	var ma []string
	theoID := map[ids.ID]*domain.KhoiPhucDoiSoatParams{}
	for rows.Next() {
		var p domain.KhoiPhucDoiSoatParams
		var id, sellerID, status, tienTe string
		var gross, deficit, net int64
		var confirmed, paid *time.Time

		if err := rows.Scan(&id, &sellerID, &p.PeriodStart, &p.PeriodEnd,
			&status, &gross, &deficit, &net, &tienTe,
			&p.CreatedAt, &confirmed, &paid, &p.Version, &p.UpdatedAt); err != nil {
			return nil, err
		}

		cur := money.Currency(tienTe)
		mk := func(v int64) money.Money {
			m, _ := money.New(v, cur)
			return m
		}
		p.ID = ids.ID(id)
		p.SellerID = ids.ID(sellerID)
		p.Status = domain.TrangThaiDoiSoat(status)
		p.Gross, p.Deficit, p.Net = mk(gross), mk(deficit), mk(net)
		p.ConfirmedAt, p.PaidAt = deref(confirmed), deref(paid)

		theoID[p.ID] = &p
		ma = append(ma, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ma) == 0 {
		return nil, nil
	}

	lrows, err := s.pool.Query(ctx, `
		SELECT settlement_id, id, ledger_entry_id, amount, currency
		  FROM settlement_line WHERE settlement_id = ANY($1)
		 ORDER BY created_at, id`, ma)
	if err != nil {
		return nil, fmt.Errorf("payment: đọc dòng đối soát: %w", err)
	}
	defer lrows.Close()

	for lrows.Next() {
		var stlID, id, entryID, tienTe string
		var soTien int64
		if err := lrows.Scan(&stlID, &id, &entryID, &soTien, &tienTe); err != nil {
			return nil, err
		}
		m, err := money.New(soTien, money.Currency(tienTe))
		if err != nil {
			return nil, err
		}
		if p := theoID[ids.ID(stlID)]; p != nil {
			p.Dong = append(p.Dong, domain.DongDoiSoat{
				ID: ids.ID(id), LedgerEntryID: ids.ID(entryID), Amount: m,
			})
		}
	}
	if err := lrows.Err(); err != nil {
		return nil, err
	}

	out := make([]*domain.DoiSoat, 0, len(ma))
	for _, id := range ma {
		out = append(out, domain.KhoiPhucDoiSoat(*theoID[ids.ID(id)]))
	}
	return out, nil
}

// deref đổi con trỏ thời gian thành giá trị; nil thành zero.
func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
