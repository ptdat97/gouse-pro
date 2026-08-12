// Package postgres cài đặt các port của pricing bằng PostgreSQL.
//
// SQL viết tay theo ADR-0010.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
)

// ---------------------------------------------------------------- Bảng giá

type PriceStore struct{ pool *pgxpool.Pool }

func NewPriceStore(pool *pgxpool.Pool) *PriceStore {
	return &PriceStore{pool: pool}
}

const priceCols = `
	id, sku_id, price_type, amount, currency, compare_at,
	valid_from, valid_until, customer_tier, campaign_id, is_active,
	created_at, updated_at`

func scanPrice(row pgx.Row) (*domain.Price, error) {
	var (
		p                         domain.RestorePriceParams
		id, skuID, priceType, cur string
		campaignID                string
		amount, compareAt         int64
		validFrom, validUntil     *time.Time
	)
	err := row.Scan(&id, &skuID, &priceType, &amount, &cur, &compareAt,
		&validFrom, &validUntil, &p.CustomerTier, &campaignID, &p.Active,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	p.ID = ids.ID(id)
	p.SKUID = ids.ID(skuID)
	p.CampaignID = ids.ID(campaignID)
	p.PriceType = domain.PriceType(priceType)
	p.Period = domain.Period{From: timeOrZero(validFrom), To: timeOrZero(validUntil)}

	// Money dựng lại từ (số nguyên, đơn vị tiền tệ). Dùng MustNew là an
	// toàn ở đây: dữ liệu đã qua CHECK amount > 0 của database.
	p.Amount = money.MustNew(amount, money.Currency(cur))
	if compareAt > 0 {
		p.CompareAt = money.MustNew(compareAt, money.Currency(cur))
	}
	return domain.RestorePrice(p), nil
}

func (s *PriceStore) Save(ctx context.Context, p *domain.Price) error {
	const q = `
		INSERT INTO price (
			id, sku_id, price_type, amount, currency, compare_at,
			valid_from, valid_until, customer_tier, campaign_id, is_active,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (id) DO UPDATE SET
			amount        = EXCLUDED.amount,
			compare_at    = EXCLUDED.compare_at,
			valid_from    = EXCLUDED.valid_from,
			valid_until   = EXCLUDED.valid_until,
			customer_tier = EXCLUDED.customer_tier,
			campaign_id   = EXCLUDED.campaign_id,
			is_active     = EXCLUDED.is_active,
			updated_at    = EXCLUDED.updated_at`

	period := p.Period()
	_, err := s.pool.Exec(ctx, q,
		p.ID().String(), p.SKUID().String(), string(p.Type()),
		p.Amount().Amount(), string(p.Amount().Currency()), p.CompareAt().Amount(),
		nullTime(period.From), nullTime(period.To),
		p.CustomerTier(), p.CampaignID().String(), p.IsActive(),
		p.CreatedAt(), p.UpdatedAt())
	if err != nil {
		return fmt.Errorf("pricing: lưu giá: %w", err)
	}
	return nil
}

func (s *PriceStore) FindByID(ctx context.Context, id ids.ID) (*domain.Price, error) {
	return scanPrice(s.pool.QueryRow(ctx,
		`SELECT `+priceCols+` FROM price WHERE id = $1`, id.String()))
}

// FindBySKU trả MỌI mức giá, kể cả đã ngừng áp dụng.
//
// Việc chọn mức nào áp dụng là quyết định NGHIỆP VỤ (domain.SelectBest).
// Nếu lọc ở đây, quy tắc ưu tiên sẽ nằm rải rác ở cả SQL lẫn domain.
func (s *PriceStore) FindBySKU(ctx context.Context, skuID ids.ID) ([]*domain.Price, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+priceCols+` FROM price WHERE sku_id = $1 ORDER BY id`, skuID.String())
	if err != nil {
		return nil, fmt.Errorf("pricing: đọc giá theo SKU: %w", err)
	}
	defer rows.Close()
	return collectPrices(rows)
}

func (s *PriceStore) FindBySKUs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID][]*domain.Price, error) {
	out := make(map[ids.ID][]*domain.Price, len(skuIDs))
	if len(skuIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+priceCols+` FROM price WHERE sku_id = ANY($1) ORDER BY id`, toStrings(skuIDs))
	if err != nil {
		return nil, fmt.Errorf("pricing: đọc giá theo lô: %w", err)
	}
	defer rows.Close()

	list, err := collectPrices(rows)
	if err != nil {
		return nil, fmt.Errorf("pricing: đọc giá theo lô: %w", err)
	}
	for _, p := range list {
		out[p.SKUID()] = append(out[p.SKUID()], p)
	}
	return out, nil
}

func collectPrices(rows pgx.Rows) ([]*domain.Price, error) {
	var out []*domain.Price
	for rows.Next() {
		p, err := scanPrice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------- Khung giá

type ConstraintStore struct{ pool *pgxpool.Pool }

func NewConstraintStore(pool *pgxpool.Pool) *ConstraintStore {
	return &ConstraintStore{pool: pool}
}

const constraintCols = `
	id, sku_id, min_price, max_price, reference_price, currency,
	suspicious_below_bp, created_at, updated_at`

func scanConstraint(row pgx.Row) (*domain.PriceConstraint, error) {
	var (
		p                domain.RestoreConstraintParams
		id, skuID, cur   string
		minP, maxP, refP int64
	)
	err := row.Scan(&id, &skuID, &minP, &maxP, &refP, &cur,
		&p.SuspiciousBelowBP, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	p.ID = ids.ID(id)
	p.SKUID = ids.ID(skuID)
	// 0 nghĩa là không giới hạn — giữ Money rỗng để phân biệt với "giới
	// hạn bằng 0 đồng".
	if minP > 0 {
		p.MinPrice = money.MustNew(minP, money.Currency(cur))
	}
	if maxP > 0 {
		p.MaxPrice = money.MustNew(maxP, money.Currency(cur))
	}
	if refP > 0 {
		p.ReferencePrice = money.MustNew(refP, money.Currency(cur))
	}
	return domain.RestorePriceConstraint(p), nil
}

// Save lưu khung giá. UNIQUE trên sku_id bảo đảm mỗi SKU chỉ một khung.
func (s *ConstraintStore) Save(ctx context.Context, c *domain.PriceConstraint) error {
	const q = `
		INSERT INTO price_constraint (
			id, sku_id, min_price, max_price, reference_price, currency,
			suspicious_below_bp, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (sku_id) DO UPDATE SET
			min_price           = EXCLUDED.min_price,
			max_price           = EXCLUDED.max_price,
			reference_price     = EXCLUDED.reference_price,
			currency            = EXCLUDED.currency,
			suspicious_below_bp = EXCLUDED.suspicious_below_bp,
			updated_at          = EXCLUDED.updated_at`

	_, err := s.pool.Exec(ctx, q,
		c.ID().String(), c.SKUID().String(),
		c.MinPrice().Amount(), c.MaxPrice().Amount(), c.ReferencePrice().Amount(),
		string(constraintCurrency(c)), c.SuspiciousBelowBP(),
		c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		return fmt.Errorf("pricing: lưu khung giá: %w", err)
	}
	return nil
}

// constraintCurrency lấy đơn vị tiền tệ từ giới hạn nào có giá trị.
func constraintCurrency(c *domain.PriceConstraint) money.Currency {
	if !c.MinPrice().IsZero() {
		return c.MinPrice().Currency()
	}
	if !c.MaxPrice().IsZero() {
		return c.MaxPrice().Currency()
	}
	return money.VND
}

func (s *ConstraintStore) FindBySKU(ctx context.Context, skuID ids.ID) (*domain.PriceConstraint, error) {
	return scanConstraint(s.pool.QueryRow(ctx,
		`SELECT `+constraintCols+` FROM price_constraint WHERE sku_id = $1`, skuID.String()))
}

func (s *ConstraintStore) FindBySKUs(
	ctx context.Context, skuIDs []ids.ID,
) (map[ids.ID]*domain.PriceConstraint, error) {
	out := make(map[ids.ID]*domain.PriceConstraint, len(skuIDs))
	if len(skuIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+constraintCols+` FROM price_constraint WHERE sku_id = ANY($1)`,
		toStrings(skuIDs))
	if err != nil {
		return nil, fmt.Errorf("pricing: đọc khung giá theo lô: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		c, err := scanConstraint(rows)
		if err != nil {
			return nil, fmt.Errorf("pricing: đọc khung giá theo lô: %w", err)
		}
		out[c.SKUID()] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pricing: đọc khung giá theo lô: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------- Lịch sử

// HistoryStore ghi và đọc lịch sử giá.
//
// CHỈ CÓ Append và Find — không có Update, không có Delete.
//
// Ở tầng database, trigger price_history_immutable TỪ CHỐI THI HÀNH mọi
// UPDATE/DELETE, kể cả khi ai đó gõ SQL trực tiếp bằng tài khoản quản trị.
// Đây là khác biệt thật so với kho in-memory: in-memory chỉ "không cung
// cấp phương thức", database thì "không cho phép hành động".
type HistoryStore struct{ pool *pgxpool.Pool }

func NewHistoryStore(pool *pgxpool.Pool) *HistoryStore {
	return &HistoryStore{pool: pool}
}

func (s *HistoryStore) Append(ctx context.Context, p *domain.PricePoint) error {
	const q = `
		INSERT INTO price_history (
			id, sku_id, price_type, amount, currency, compare_at,
			reason, changed_by, recorded_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

	_, err := s.pool.Exec(ctx, q,
		p.ID().String(), p.SKUID().String(), string(p.Type()),
		p.Amount().Amount(), string(p.Amount().Currency()), p.CompareAt().Amount(),
		string(p.Reason()), p.ChangedBy().String(), p.RecordedAt())
	if err != nil {
		return fmt.Errorf("pricing: ghi lịch sử giá: %w", err)
	}
	return nil
}

func (s *HistoryStore) FindBySKU(
	ctx context.Context, skuID ids.ID, r domain.DateRange,
) ([]*domain.PricePoint, error) {
	// Khoảng rỗng nghĩa là không giới hạn phía đó — dùng NULL để SQL bỏ
	// qua điều kiện tương ứng.
	rows, err := s.pool.Query(ctx, `
		SELECT id, sku_id, price_type, amount, currency, compare_at,
		       reason, changed_by, recorded_at
		FROM price_history
		WHERE sku_id = $1
		  AND ($2::timestamptz IS NULL OR recorded_at >= $2)
		  AND ($3::timestamptz IS NULL OR recorded_at < $3)
		ORDER BY recorded_at, id`,
		skuID.String(), nullTime(r.From), nullTime(r.To))
	if err != nil {
		return nil, fmt.Errorf("pricing: đọc lịch sử giá: %w", err)
	}
	defer rows.Close()

	var out []*domain.PricePoint
	for rows.Next() {
		var (
			p                            domain.RestorePricePointParams
			id, skuIDStr, priceType, cur string
			reason, changedBy            string
			amount, compareAt            int64
		)
		if err := rows.Scan(&id, &skuIDStr, &priceType, &amount, &cur, &compareAt,
			&reason, &changedBy, &p.RecordedAt); err != nil {
			return nil, fmt.Errorf("pricing: đọc lịch sử giá: %w", err)
		}
		p.ID = ids.ID(id)
		p.SKUID = ids.ID(skuIDStr)
		p.PriceType = domain.PriceType(priceType)
		p.Reason = domain.ChangeReason(reason)
		p.ChangedBy = ids.ID(changedBy)
		p.Amount = money.MustNew(amount, money.Currency(cur))
		if compareAt > 0 {
			p.CompareAt = money.MustNew(compareAt, money.Currency(cur))
		}
		out = append(out, domain.RestorePricePoint(p))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pricing: đọc lịch sử giá: %w", err)
	}
	return out, nil
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

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
