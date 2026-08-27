// Package postgres cài đặt các port của payment bằng PostgreSQL.
//
// LƯU Ý: package này KHÔNG có hàm nào sửa hay xóa bút toán. Ở tầng
// database còn có trigger chặn — hai lớp bảo vệ cho cùng một bất biến.
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

// querier là thứ chạy được câu lệnh: pool HOẶC giao dịch.
type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// LedgerStore ghi và đọc sổ cái.
//
// pool rỗng nghĩa là kho này BÁM VÀO giao dịch của bên gọi — xem LedgerForTx.
type LedgerStore struct {
	pool *pgxpool.Pool
	q    querier
}

func NewLedgerStore(pool *pgxpool.Pool) *LedgerStore {
	return &LedgerStore{pool: pool, q: pool}
}

// LedgerForTx trả kho sổ cái ghi bằng GIAO DỊCH CỦA BÊN GỌI.
//
// Dành cho bên nhận domain event: dispatcher đã mở một giao dịch để đánh
// dấu event đã xử lý, và bút toán phải nằm TRONG giao dịch đó. Hai giao
// dịch tách rời nghĩa là ghi sổ thành công trong khi đánh dấu thất bại —
// lần thử lại sẽ ghi doanh thu LẦN THỨ HAI.
//
// Cùng lý do và cùng khuôn với inventory.ReposForTx.
func LedgerForTx(tx pgx.Tx) *LedgerStore {
	return &LedgerStore{q: tx}
}

// Append ghi bút toán CÙNG các dòng của nó trong MỘT giao dịch.
//
// Bút toán và các dòng là một aggregate: ghi bút toán thành công mà ghi
// dòng thất bại sẽ để lại một bút toán RỖNG — vừa vô nghĩa vừa làm hỏng
// mọi phép tính số dư sau đó.
func (s *LedgerStore) Append(ctx context.Context, e *domain.LedgerEntry) error {
	return s.append(ctx, e, nil)
}

// AppendWithAudit ghi bút toán và chạy fn trong CÙNG một giao dịch.
//
// Thứ tự: ghi bút toán và các dòng TRƯỚC, chạy fn SAU, commit CUỐI. Nếu fn
// thất bại, `defer Rollback` hủy toàn bộ — không có bút toán nào ở lại mà
// thiếu vết kiểm toán.
func (s *LedgerStore) AppendWithAudit(
	ctx context.Context, e *domain.LedgerEntry, fn domain.TxFunc,
) error {
	return s.append(ctx, e, fn)
}

// txKey gắn giao dịch vào ngữ cảnh cho TxFunc.
type txKey struct{}

// TxFrom lấy giao dịch mà AppendWithAudit đang mở.
func TxFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

func (s *LedgerStore) append(
	ctx context.Context, e *domain.LedgerEntry, fn domain.TxFunc,
) error {
	// Bên gọi đang giữ giao dịch: ghi thẳng vào đó, KHÔNG mở giao dịch
	// lồng. Mở thêm một giao dịch ở đây là quay lại đúng cái lỗi mà
	// LedgerForTx sinh ra để tránh.
	if s.pool == nil {
		return s.ghi(ctx, s.q, e, fn)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("payment: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.ghi(ctx, tx, e, fn); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("payment: xác nhận giao dịch: %w", err)
	}
	return nil
}

// ghi ghi bút toán và các dòng bằng querier bên gọi đưa vào.
func (s *LedgerStore) ghi(
	ctx context.Context, tx querier, e *domain.LedgerEntry, fn domain.TxFunc,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ledger_entry (
			id, entry_type, reference_type, reference_id,
			description, idempotency_key, created_by, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.ID().String(), string(e.Type()), e.ReferenceType(), e.ReferenceID().String(),
		e.Description(), e.IdempotencyKey(), e.CreatedBy(), e.CreatedAt())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "ledger_entry_idempotency_key_key" {
			return domain.ErrDuplicateEntry
		}
		return fmt.Errorf("payment: ghi bút toán: %w", err)
	}

	for i, l := range e.Lines() {
		_, err := tx.Exec(ctx, `
			INSERT INTO ledger_line (
				entry_id, account_type, account_owner_id,
				direction, amount, currency, description
			) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			e.ID().String(), string(l.Account.Type), l.Account.OwnerID.String(),
			string(l.Direction), l.Amount.Amount(), string(l.Amount.Currency()),
			l.Description)
		if err != nil {
			return fmt.Errorf("payment: ghi dòng bút toán %d: %w", i+1, err)
		}
	}

	if fn != nil {
		// Ngữ cảnh MANG giao dịch, để fn ghi bằng chính nó.
		if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
			return err
		}
	}
	return nil
}

const entryCols = `
	id, entry_type, reference_type, reference_id,
	description, idempotency_key, created_by, created_at`

func (s *LedgerStore) FindByID(ctx context.Context, id ids.ID) (*domain.LedgerEntry, error) {
	return s.findOne(ctx, `SELECT `+entryCols+` FROM ledger_entry WHERE id = $1`, id.String())
}

func (s *LedgerStore) FindByIdempotencyKey(
	ctx context.Context, key string,
) (*domain.LedgerEntry, error) {
	return s.findOne(ctx,
		`SELECT `+entryCols+` FROM ledger_entry WHERE idempotency_key = $1`, key)
}

func (s *LedgerStore) findOne(ctx context.Context, q string, args ...any) (*domain.LedgerEntry, error) {
	var (
		p                    domain.RestoreEntryParams
		id, entryType, refID string
	)
	err := s.q.QueryRow(ctx, q, args...).Scan(
		&id, &entryType, &p.ReferenceType, &refID,
		&p.Description, &p.IdempotencyKey, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("payment: đọc bút toán: %w", err)
	}

	p.ID = ids.ID(id)
	p.Type = domain.EntryType(entryType)
	p.ReferenceID = ids.ID(refID)

	lines, err := s.linesFor(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Lines = lines
	return domain.RestoreLedgerEntry(p), nil
}

func (s *LedgerStore) linesFor(ctx context.Context, entryID ids.ID) ([]domain.Line, error) {
	rows, err := s.q.Query(ctx, `
		SELECT account_type, account_owner_id, direction, amount, currency, description
		FROM ledger_line WHERE entry_id = $1 ORDER BY id`, entryID.String())
	if err != nil {
		return nil, fmt.Errorf("payment: đọc dòng bút toán: %w", err)
	}
	defer rows.Close()

	var out []domain.Line
	for rows.Next() {
		var (
			accType, ownerID, direction string
			currency, description       string
			amount                      int64
		)
		if err := rows.Scan(&accType, &ownerID, &direction, &amount, &currency, &description); err != nil {
			return nil, fmt.Errorf("payment: đọc dòng bút toán: %w", err)
		}
		out = append(out, domain.Line{
			Account: domain.Account{
				Type:    domain.AccountType(accType),
				OwnerID: ids.ID(ownerID),
			},
			Direction:   domain.Direction(direction),
			Amount:      money.MustNew(amount, money.Currency(currency)),
			Description: description,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("payment: đọc dòng bút toán: %w", err)
	}
	return out, nil
}

func (s *LedgerStore) FindByReference(
	ctx context.Context, refType string, refID ids.ID,
) ([]*domain.LedgerEntry, error) {
	return s.findMany(ctx,
		`SELECT `+entryCols+` FROM ledger_entry
		 WHERE reference_type = $1 AND reference_id = $2
		 ORDER BY created_at, id`, refType, refID.String())
}

func (s *LedgerStore) FindAll(
	ctx context.Context, from, to time.Time, limit int,
) ([]*domain.LedgerEntry, error) {
	if limit <= 0 {
		limit = 1000
	}
	return s.findMany(ctx,
		`SELECT `+entryCols+` FROM ledger_entry
		 WHERE ($1::timestamptz IS NULL OR created_at >= $1)
		   AND ($2::timestamptz IS NULL OR created_at < $2)
		 ORDER BY created_at, id LIMIT $3`,
		nullTime(from), nullTime(to), limit)
}

func (s *LedgerStore) findMany(
	ctx context.Context, q string, args ...any,
) ([]*domain.LedgerEntry, error) {
	rows, err := s.q.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("payment: đọc bút toán: %w", err)
	}

	type header struct {
		p domain.RestoreEntryParams
	}
	var headers []header

	for rows.Next() {
		var (
			h                    header
			id, entryType, refID string
		)
		if err := rows.Scan(&id, &entryType, &h.p.ReferenceType, &refID,
			&h.p.Description, &h.p.IdempotencyKey, &h.p.CreatedBy, &h.p.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("payment: đọc bút toán: %w", err)
		}
		h.p.ID = ids.ID(id)
		h.p.Type = domain.EntryType(entryType)
		h.p.ReferenceID = ids.ID(refID)
		headers = append(headers, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("payment: đọc bút toán: %w", err)
	}
	rows.Close()

	out := make([]*domain.LedgerEntry, 0, len(headers))
	for _, h := range headers {
		lines, err := s.linesFor(ctx, h.p.ID)
		if err != nil {
			return nil, err
		}
		h.p.Lines = lines
		out = append(out, domain.RestoreLedgerEntry(h.p))
	}
	return out, nil
}

// ---------------------------------------------------------------- Số dư

// BalanceStore tính số dư bằng truy vấn gom nhóm.
type BalanceStore struct {
	pool *pgxpool.Pool
}

func NewBalanceStore(pool *pgxpool.Pool) *BalanceStore {
	return &BalanceStore{pool: pool}
}

// Balance tính số dư của một tài khoản TỪ BÚT TOÁN.
//
// Không đọc từ bảng snapshot: snapshot là cache có thể lệch, còn đây là
// nguồn sự thật. Khi cần tốc độ, tầng gọi dùng snapshot và job hàng ngày
// đối chiếu hai bên.
func (s *BalanceStore) Balance(ctx context.Context, account domain.Account) (domain.Balance, error) {
	var (
		debit, credit int64
		currency      *string
		entryCount    int
	)
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0),
			MIN(currency),
			COUNT(DISTINCT entry_id)
		FROM ledger_line
		WHERE account_type = $1 AND account_owner_id = $2`,
		string(account.Type), account.OwnerID.String()).
		Scan(&debit, &credit, &currency, &entryCount)
	if err != nil {
		return domain.Balance{}, fmt.Errorf("payment: tính số dư: %w", err)
	}

	cur := money.VND
	if currency != nil {
		cur = money.Currency(*currency)
	}

	// Dấu theo BẢN CHẤT tài khoản: tài sản và chi phí tăng khi ghi nợ;
	// nợ phải trả và doanh thu tăng khi ghi có.
	net := credit - debit
	if account.Type.IsDebitNormal() {
		net = debit - credit
	}

	return domain.Balance{
		Account:     account,
		Amount:      money.MustNew(net, cur),
		TotalDebit:  money.MustNew(debit, cur),
		TotalCredit: money.MustNew(credit, cur),
		EntryCount:  entryCount,
	}, nil
}

func (s *BalanceStore) BalancesByOwner(
	ctx context.Context, ownerID ids.ID,
) (map[string]domain.Balance, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT account_type,
			COALESCE(SUM(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0),
			MIN(currency),
			COUNT(DISTINCT entry_id)
		FROM ledger_line
		WHERE account_owner_id = $1
		GROUP BY account_type`, ownerID.String())
	if err != nil {
		return nil, fmt.Errorf("payment: tính số dư theo chủ sở hữu: %w", err)
	}
	defer rows.Close()

	out := map[string]domain.Balance{}
	for rows.Next() {
		var (
			accType       string
			debit, credit int64
			currency      *string
			entryCount    int
		)
		if err := rows.Scan(&accType, &debit, &credit, &currency, &entryCount); err != nil {
			return nil, fmt.Errorf("payment: tính số dư theo chủ sở hữu: %w", err)
		}

		account := domain.Account{Type: domain.AccountType(accType), OwnerID: ownerID}
		cur := money.VND
		if currency != nil {
			cur = money.Currency(*currency)
		}
		net := credit - debit
		if account.Type.IsDebitNormal() {
			net = debit - credit
		}

		out[account.Key()] = domain.Balance{
			Account:     account,
			Amount:      money.MustNew(net, cur),
			TotalDebit:  money.MustNew(debit, cur),
			TotalCredit: money.MustNew(credit, cur),
			EntryCount:  entryCount,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("payment: tính số dư theo chủ sở hữu: %w", err)
	}
	return out, nil
}

// TotalDebitCredit trả tổng ghi nợ và ghi có TOÀN HỆ THỐNG.
//
// Hai con số này phải BẰNG NHAU. Lệch nghĩa là có bút toán không cân bằng
// lọt vào database — sự cố nghiêm trọng, không phải "sai số chấp nhận được".
func (s *BalanceStore) TotalDebitCredit(ctx context.Context) (int64, int64, error) {
	var debit, credit int64
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN direction = 'DEBIT'  THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount ELSE 0 END), 0)
		FROM ledger_line`).Scan(&debit, &credit)
	if err != nil {
		return 0, 0, fmt.Errorf("payment: tính tổng nợ/có: %w", err)
	}
	return debit, credit, nil
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
