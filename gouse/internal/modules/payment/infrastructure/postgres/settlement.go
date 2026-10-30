package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
	created_at, confirmed_at, confirmed_by, paid_at, paid_by,
	version, updated_at`

// CreateForSeller lập đợt đối soát cho MỘT nhà bán, trong một giao dịch.
//
// # Ba việc phải nằm CÙNG một giao dịch
//
//  1. khóa nhà bán      không hai lượt job nào cùng lập đợt cho họ
//  2. đọc phần nợ       đọc SAU khóa, nếu không số đọc được đã cũ
//  3. ghi đợt + thu hồi  đợt trừ trên giấy mà bút toán không ghi nghĩa là
//     khoản nợ còn nguyên và kỳ sau trừ lại
//
// Tách bất kỳ bước nào ra là mở lại đúng lỗi đang sửa.
//
// # Vì sao KHÓA THEO PHIÊN GIAO DỊCH
//
// Ràng buộc UNIQUE trên `settlement_line.ledger_entry_id` chặn được một
// bút toán lọt vào hai đợt, nhưng KHÔNG chặn được hai đợt khác nhau cùng
// thu một khoản nợ: hai lượt chạy chồng nhau đọc cùng số âm 50.000, mỗi
// lượt gom những dòng khác nhau, và nhà bán bị trừ 100.000.
//
// `pg_advisory_xact_lock` tự nhả khi giao dịch kết thúc — không có đường
// nào quên nhả, kể cả khi tiến trình chết giữa chừng.
//
// `dung` nhận phần nợ đọc được và trả về đợt cùng bút toán thu hồi; trả
// nil cho bút toán nghĩa là kỳ này không thu gì.
func (s *SettlementStore) CreateForSeller(
	ctx context.Context, sellerID ids.ID,
	build func(pendingDebt money.Money) (*domain.Settlement, *domain.LedgerEntry, error),
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("payment: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// hashtext cho ra int4; va chạm chỉ làm hai nhà bán chờ nhau một
	// nhịp, không làm sai tiền.
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1))`, sellerID.String()); err != nil {
		return fmt.Errorf("payment: khóa nhà bán: %w", err)
	}

	sd, err := BalanceForTx(tx).Balance(ctx, domain.Account{
		Type: domain.AccountSellerPayable, OwnerID: sellerID,
	})
	if err != nil {
		return err
	}
	no, err := money.New(0, sd.Amount.Currency())
	if err != nil {
		return err
	}
	if sd.Amount.Amount() < 0 {
		if no, err = money.New(-sd.Amount.Amount(), sd.Amount.Currency()); err != nil {
			return err
		}
	}

	d, clawback, err := build(no)
	if err != nil {
		return err
	}

	if err := s.writeEntry(ctx, tx, d); err != nil {
		return err
	}
	if clawback != nil {
		if err := LedgerForTx(tx).Append(ctx, clawback); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("payment: xác nhận giao dịch: %w", err)
	}
	return nil
}

// ghi ghi đợt đối soát VÀ các dòng của nó bằng giao dịch bên gọi đã mở.
//
// Ràng buộc UNIQUE trên `ledger_entry_id` là thứ chặn một bút toán lọt vào
// hai đợt. Hai lượt chạy job chồng nhau đều thấy "chưa gom" nếu chỉ kiểm ở
// tầng ứng dụng — và khi đó nhà bán được trả tiền hai lần cho cùng một đơn.
func (s *SettlementStore) writeEntry(ctx context.Context, tx pgx.Tx, d *domain.Settlement) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO settlement (`+stlCols+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (id) DO UPDATE SET
			status       = EXCLUDED.status,
			confirmed_at = EXCLUDED.confirmed_at,
			confirmed_by = EXCLUDED.confirmed_by,
			paid_at      = EXCLUDED.paid_at,
			paid_by      = EXCLUDED.paid_by,
			updated_at   = EXCLUDED.updated_at,
			version      = settlement.version + 1
		WHERE settlement.version = $15`,
		d.ID().String(), d.SellerID().String(), d.PeriodStart(), d.PeriodEnd(),
		string(d.Status()), d.Gross().Amount(), d.Deficit().Amount(),
		d.Net().Amount(), string(d.Gross().Currency()),
		d.CreatedAt(),
		nullTime(d.ConfirmedAt()), d.ConfirmedBy().String(),
		nullTime(d.PaidAt()), d.PaidBy().String(),
		d.Version(), d.UpdatedAt())
	if err != nil {
		return fmt.Errorf("payment: ghi đợt đối soát: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrVersionConflict
	}

	for _, line := range d.Lines() {
		_, err := tx.Exec(ctx, `
			INSERT INTO settlement_line (
				id, settlement_id, ledger_entry_id, amount, currency
			) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (id) DO NOTHING`,
			line.ID.String(), d.ID().String(), line.LedgerEntryID.String(),
			line.Amount.Amount(), string(line.Amount.Currency()))
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

	return nil
}

func (s *SettlementStore) FindByID(ctx context.Context, id ids.ID) (*domain.Settlement, error) {
	ds, err := s.readOne(ctx, `WHERE id = $1`, id.String())
	if err != nil {
		return nil, err
	}
	if len(ds) == 0 {
		return nil, domain.ErrSettlementNotFound
	}
	return ds[0], nil
}

// ByStatus liệt kê đợt của MỌI nhà bán, lọc theo trạng thái.
//
// # Vì sao một hàm riêng thay vì nới `BySeller`
//
// Hai hàm trả lời hai câu có RANH GIỚI BẢO MẬT ngược nhau: `BySeller`
// lọc theo nhà bán lấy từ token, còn hàm này CỐ Ý không lọc. Gộp lại thành
// một hàm có tham số `sellerID` rỗng nghĩa là một chỗ quên truyền token sẽ
// trả dữ liệu của mọi gian hàng — và chỗ quên ấy không trông giống lỗi.
//
// `status` rỗng = mọi trạng thái.
func (s *SettlementStore) ByStatus(
	ctx context.Context, status string, limit int,
) ([]*domain.Settlement, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// `$1 = ''` chọn MỌI trạng thái. Một truy vấn thay vì hai nhánh chuỗi
	// SQL nối tay — chuỗi nối tay là chỗ lỗ SQL injection hay xuất hiện.
	return s.readOne(ctx,
		`WHERE ($1 = '' OR status = $1) ORDER BY created_at DESC LIMIT $2`,
		status, limit)
}

func (s *SettlementStore) BySeller(
	ctx context.Context, sellerID ids.ID, limit int,
) ([]*domain.Settlement, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.readOne(ctx,
		`WHERE seller_id = $1 ORDER BY created_at DESC LIMIT $2`,
		sellerID.String(), limit)
}

// UnsettledEntries trả các bút toán SELLER_RELEASE chưa thuộc đợt nào.
//
// Gom theo nhà bán để job tạo mỗi nhà bán một đợt.
func (s *SettlementStore) UnsettledEntries(
	ctx context.Context, toDate time.Time, limit int,
) (map[ids.ID][]domain.SettlementLine, error) {
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
		 LIMIT $2`, toDate, limit)
	if err != nil {
		return nil, fmt.Errorf("payment: tìm bút toán chưa gom: %w", err)
	}
	defer rows.Close()

	out := map[ids.ID][]domain.SettlementLine{}
	for rows.Next() {
		var owner, entryID, currency string
		var amount int64
		if err := rows.Scan(&owner, &entryID, &amount, &currency); err != nil {
			return nil, err
		}
		m, err := money.New(amount, money.Currency(currency))
		if err != nil {
			return nil, err
		}
		out[ids.ID(owner)] = append(out[ids.ID(owner)], domain.SettlementLine{
			ID:            ids.MustNew(ids.PrefixSettlement),
			LedgerEntryID: ids.ID(entryID),
			Amount:        m,
		})
	}
	return out, rows.Err()
}

func (s *SettlementStore) readOne(
	ctx context.Context, where string, args ...any,
) ([]*domain.Settlement, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+stlCols+` FROM settlement `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("payment: đọc đợt đối soát: %w", err)
	}
	defer rows.Close()

	var settlementIDs []string
	byID := map[ids.ID]*domain.RestoreSettlementParams{}
	for rows.Next() {
		var p domain.RestoreSettlementParams
		var id, sellerID, status, currency string
		var confirmedBy, paidBy string
		var gross, deficit, net int64
		var confirmed, paid *time.Time

		if err := rows.Scan(&id, &sellerID, &p.PeriodStart, &p.PeriodEnd,
			&status, &gross, &deficit, &net, &currency,
			&p.CreatedAt, &confirmed, &confirmedBy, &paid, &paidBy,
			&p.Version, &p.UpdatedAt); err != nil {
			return nil, err
		}

		cur := money.Currency(currency)
		mk := func(v int64) money.Money {
			m, _ := money.New(v, cur)
			return m
		}
		p.ID = ids.ID(id)
		p.SellerID = ids.ID(sellerID)
		p.Status = domain.SettlementStatus(status)
		p.Gross, p.Deficit, p.Net = mk(gross), mk(deficit), mk(net)
		p.ConfirmedAt, p.PaidAt = deref(confirmed), deref(paid)
		p.ConfirmedBy, p.PaidBy = ids.ID(confirmedBy), ids.ID(paidBy)

		byID[p.ID] = &p
		settlementIDs = append(settlementIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(settlementIDs) == 0 {
		return nil, nil
	}

	// JOIN sang `ledger_entry` để lấy NGUỒN của khoản rút được.
	//
	// Một dòng đối soát chỉ có số tiền là một dòng nhà bán không đối chiếu
	// được với sổ của họ. `reference_id` cho biết đơn thực hiện nào —
	// chính là số họ nhìn thấy ở màn hình "Việc cần làm".
	lrows, err := s.pool.Query(ctx, `
		SELECT sl.settlement_id, sl.id, sl.ledger_entry_id, sl.amount,
		       sl.currency, e.reference_type, e.reference_id, e.created_at
		  FROM settlement_line sl
		  JOIN ledger_entry e ON e.id = sl.ledger_entry_id
		 WHERE sl.settlement_id = ANY($1)
		 ORDER BY sl.created_at, sl.id`, settlementIDs)
	if err != nil {
		return nil, fmt.Errorf("payment: đọc dòng đối soát: %w", err)
	}
	defer lrows.Close()

	for lrows.Next() {
		var stlID, id, entryID, currency, refType, refID string
		var amount int64
		var createTime time.Time
		if err := lrows.Scan(&stlID, &id, &entryID, &amount, &currency,
			&refType, &refID, &createTime); err != nil {
			return nil, err
		}
		m, err := money.New(amount, money.Currency(currency))
		if err != nil {
			return nil, err
		}
		if p := byID[ids.ID(stlID)]; p != nil {
			p.Lines = append(p.Lines, domain.SettlementLine{
				ID: ids.ID(id), LedgerEntryID: ids.ID(entryID), Amount: m,
				ReferenceType: refType, ReferenceID: ids.ID(refID),
				CreatedAt: createTime,
			})
		}
	}
	if err := lrows.Err(); err != nil {
		return nil, err
	}

	out := make([]*domain.Settlement, 0, len(settlementIDs))
	for _, id := range settlementIDs {
		out = append(out, domain.RestoreSettlement(*byID[ids.ID(id)]))
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

// SaveWithAudit ghi trạng thái mới của đợt đối soát và chạy `fn` trong CÙNG
// một giao dịch.
//
// # Vì sao audit phải cùng giao dịch
//
// Đây là hai bước duy nhất biến một con số tính được thành TIỀN RỜI nền
// tảng. Một đợt chuyển sang `PAID` mà vết kiểm toán hỏng nghĩa là có một
// khoản chi không ai truy được ai đã ký — và vết kiểm toán không tạo ngược
// được.
//
// Cùng khuôn `LedgerStore.AppendWithAudit`: ghi TRƯỚC, chạy `fn` SAU, commit
// CUỐI. `fn` thất bại thì `defer Rollback` hủy cả hai.
//
// Kiểm phiên bản của `ghi` vẫn là chốt chặn thật cho hai người bấm cùng lúc:
// lượt thứ hai thấy `ErrVersionConflict` và không ghi gì.
func (s *SettlementStore) SaveWithAudit(
	ctx context.Context, d *domain.Settlement, fn domain.TxFunc,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("payment: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.writeEntry(ctx, tx, d); err != nil {
		return err
	}

	if fn != nil {
		if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("payment: chốt giao dịch: %w", err)
	}
	return nil
}
