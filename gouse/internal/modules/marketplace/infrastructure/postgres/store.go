// Package postgres cài đặt các port của marketplace bằng PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
)

// OfferStore lưu offer trong PostgreSQL.
type OfferStore struct {
	pool *pgxpool.Pool
}

func NewOfferStore(pool *pgxpool.Pool) *OfferStore {
	return &OfferStore{pool: pool}
}

const offerCols = `
	id, sku_id, seller_id, price_amount, price_currency, compare_at_amount,
	condition, handling_time_hours, min_order_quantity, max_order_quantity,
	status, version, created_at, updated_at`

func scanOffer(row pgx.Row) (*domain.Offer, error) {
	var (
		p                            domain.RestoreOfferParams
		id, skuID, sellerID          string
		currency, condition, status  string
		priceAmount, compareAtAmount int64
	)
	err := row.Scan(&id, &skuID, &sellerID, &priceAmount, &currency, &compareAtAmount,
		&condition, &p.HandlingTimeHours, &p.MinOrderQuantity, &p.MaxOrderQuantity,
		&status, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	p.ID = ids.ID(id)
	p.SKUID = ids.ID(skuID)
	p.SellerID = ids.ID(sellerID)
	p.Condition = domain.Condition(condition)
	p.Status = domain.Status(status)
	// An toàn dùng MustNew: đã qua CHECK price_amount > 0 của database.
	p.Price = money.MustNew(priceAmount, money.Currency(currency))
	if compareAtAmount > 0 {
		p.CompareAt = money.MustNew(compareAtAmount, money.Currency(currency))
	}
	return domain.RestoreOffer(p), nil
}

func (s *OfferStore) Save(ctx context.Context, o *domain.Offer) error {
	const q = `
		INSERT INTO offer (
			id, sku_id, seller_id, price_amount, price_currency, compare_at_amount,
			condition, handling_time_hours, min_order_quantity, max_order_quantity,
			status, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (id) DO UPDATE SET
			price_amount        = EXCLUDED.price_amount,
			price_currency      = EXCLUDED.price_currency,
			compare_at_amount   = EXCLUDED.compare_at_amount,
			condition           = EXCLUDED.condition,
			handling_time_hours = EXCLUDED.handling_time_hours,
			min_order_quantity  = EXCLUDED.min_order_quantity,
			max_order_quantity  = EXCLUDED.max_order_quantity,
			status              = EXCLUDED.status,
			version             = offer.version + 1,
			updated_at          = EXCLUDED.updated_at`

	_, err := s.pool.Exec(ctx, q,
		o.ID().String(), o.SKUID().String(), o.SellerID().String(),
		o.Price().Amount(), string(o.Price().Currency()), o.CompareAt().Amount(),
		string(o.Condition()), o.HandlingTimeHours(),
		o.MinOrderQuantity(), o.MaxOrderQuantity(),
		string(o.Status()), o.Version(), o.CreatedAt(), o.UpdatedAt())
	if err != nil {
		// QUY TẮC 1: một seller chỉ có MỘT offer ACTIVE cho một SKU.
		//
		// Chỉ mục duy nhất có điều kiện ở database là chốt chặn THẬT —
		// kiểm tra ở tầng ứng dụng không chặn được hai request đồng thời.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "offer_active_uniq" {
			return domain.ErrDuplicateActiveOffer
		}
		return fmt.Errorf("marketplace: lưu offer: %w", err)
	}
	return nil
}

func (s *OfferStore) FindByID(ctx context.Context, id ids.ID) (*domain.Offer, error) {
	return scanOffer(s.pool.QueryRow(ctx,
		`SELECT `+offerCols+` FROM offer WHERE id = $1`, id.String()))
}

// FindBySKU trả MỌI offer của SKU, kể cả đã ngừng bán.
//
// Việc lọc ứng viên buy box là quyết định nghiệp vụ của domain.SelectBuyBox.
func (s *OfferStore) FindBySKU(ctx context.Context, skuID ids.ID) ([]*domain.Offer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+offerCols+` FROM offer WHERE sku_id = $1 ORDER BY id`, skuID.String())
	if err != nil {
		return nil, fmt.Errorf("marketplace: đọc offer theo SKU: %w", err)
	}
	defer rows.Close()
	return collectOffers(rows)
}

func (s *OfferStore) FindBySKUs(
	ctx context.Context, skuIDs []ids.ID,
) (map[ids.ID][]*domain.Offer, error) {
	out := make(map[ids.ID][]*domain.Offer, len(skuIDs))
	if len(skuIDs) == 0 {
		return out, nil
	}

	strs := make([]string, len(skuIDs))
	for i, id := range skuIDs {
		strs[i] = id.String()
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+offerCols+` FROM offer WHERE sku_id = ANY($1) ORDER BY id`, strs)
	if err != nil {
		return nil, fmt.Errorf("marketplace: đọc offer theo lô: %w", err)
	}
	defer rows.Close()

	list, err := collectOffers(rows)
	if err != nil {
		return nil, fmt.Errorf("marketplace: đọc offer theo lô: %w", err)
	}
	for _, o := range list {
		out[o.SKUID()] = append(out[o.SKUID()], o)
	}
	return out, nil
}

// FindByIDs lấy nhiều offer theo định danh.
func (s *OfferStore) FindByIDs(
	ctx context.Context, offerIDs []ids.ID,
) (map[ids.ID]*domain.Offer, error) {
	out := make(map[ids.ID]*domain.Offer, len(offerIDs))
	if len(offerIDs) == 0 {
		return out, nil
	}

	strs := make([]string, len(offerIDs))
	for i, id := range offerIDs {
		strs[i] = id.String()
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+offerCols+` FROM offer WHERE id = ANY($1) ORDER BY id`, strs)
	if err != nil {
		return nil, fmt.Errorf("marketplace: đọc offer theo định danh: %w", err)
	}
	defer rows.Close()

	list, err := collectOffers(rows)
	if err != nil {
		return nil, fmt.Errorf("marketplace: đọc offer theo định danh: %w", err)
	}
	for _, o := range list {
		out[o.ID()] = o
	}
	return out, nil
}

// FindBySeller lấy offer của MỘT seller.
//
// BẢO MẬT: lọc theo seller nằm trong TRUY VẤN, không ở tầng hiển thị —
// dữ liệu seller khác không bao giờ rời khỏi database.
func (s *OfferStore) FindBySeller(
	ctx context.Context, sellerID ids.ID, limit, offset int,
) ([]*domain.Offer, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+offerCols+` FROM offer WHERE seller_id = $1
		 ORDER BY id LIMIT $2 OFFSET $3`, sellerID.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("marketplace: đọc offer theo seller: %w", err)
	}
	defer rows.Close()
	return collectOffers(rows)
}

func (s *OfferStore) FindActiveForSellerSKU(
	ctx context.Context, sellerID, skuID ids.ID,
) (*domain.Offer, error) {
	return scanOffer(s.pool.QueryRow(ctx,
		`SELECT `+offerCols+` FROM offer
		 WHERE seller_id = $1 AND sku_id = $2 AND status = 'ACTIVE'`,
		sellerID.String(), skuID.String()))
}

func collectOffers(rows pgx.Rows) ([]*domain.Offer, error) {
	var out []*domain.Offer
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------- Lịch sử giá

// PriceHistoryStore ghi và đọc lịch sử giá offer.
//
// CHỈ CÓ Append và Find. Trigger ở database từ chối UPDATE/DELETE.
type PriceHistoryStore struct {
	pool *pgxpool.Pool
}

func NewPriceHistoryStore(pool *pgxpool.Pool) *PriceHistoryStore {
	return &PriceHistoryStore{pool: pool}
}

func (s *PriceHistoryStore) Append(ctx context.Context, p *domain.PricePoint) error {
	const q = `
		INSERT INTO offer_price_history (
			id, offer_id, sku_id, seller_id,
			price_amount, price_currency, compare_at_amount,
			changed_by, recorded_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

	_, err := s.pool.Exec(ctx, q,
		p.ID.String(), p.OfferID.String(), p.SKUID.String(), p.SellerID.String(),
		p.Price.Amount(), string(p.Price.Currency()), p.CompareAt.Amount(),
		p.ChangedBy.String(), p.RecordedAt)
	if err != nil {
		return fmt.Errorf("marketplace: ghi lịch sử giá offer: %w", err)
	}
	return nil
}

func (s *PriceHistoryStore) FindByOffer(
	ctx context.Context, offerID ids.ID, limit int,
) ([]*domain.PricePoint, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, offer_id, sku_id, seller_id,
		       price_amount, price_currency, compare_at_amount,
		       changed_by, recorded_at
		FROM offer_price_history
		WHERE offer_id = $1 ORDER BY recorded_at DESC, id DESC LIMIT $2`,
		offerID.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: đọc lịch sử giá offer: %w", err)
	}
	defer rows.Close()

	var out []*domain.PricePoint
	for rows.Next() {
		var (
			p                             domain.PricePoint
			id, offerIDStr, skuID         string
			sellerID, currency, changedBy string
			priceAmount, compareAt        int64
		)
		if err := rows.Scan(&id, &offerIDStr, &skuID, &sellerID,
			&priceAmount, &currency, &compareAt, &changedBy, &p.RecordedAt); err != nil {
			return nil, fmt.Errorf("marketplace: đọc lịch sử giá offer: %w", err)
		}
		p.ID = ids.ID(id)
		p.OfferID = ids.ID(offerIDStr)
		p.SKUID = ids.ID(skuID)
		p.SellerID = ids.ID(sellerID)
		p.ChangedBy = ids.ID(changedBy)
		p.Price = money.MustNew(priceAmount, money.Currency(currency))
		if compareAt > 0 {
			p.CompareAt = money.MustNew(compareAt, money.Currency(currency))
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("marketplace: đọc lịch sử giá offer: %w", err)
	}
	return out, nil
}
