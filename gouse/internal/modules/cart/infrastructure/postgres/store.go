// Package postgres cài đặt kho lưu trữ giỏ hàng bằng PostgreSQL.
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
	"github.com/fashion-commerce/platform/internal/modules/cart/domain"
)

// CartStore lưu và đọc giỏ hàng.
type CartStore struct {
	pool *pgxpool.Pool
}

func NewCartStore(pool *pgxpool.Pool) *CartStore {
	return &CartStore{pool: pool}
}

var _ domain.Repository = (*CartStore)(nil)

// Save ghi giỏ và toàn bộ món trong MỘT giao dịch.
//
// Cách ghi món: XÓA HẾT RỒI GHI LẠI, không phải so từng dòng.
//
// Với order thì cách đó là sai — dòng hàng ở đó bất biến và có bút toán
// tham chiếu tới. Với giỏ thì đúng: món trong giỏ không có gì tham chiếu
// tới, số lượng món nhỏ, và so từng dòng chỉ thêm nhánh xử lý mà không cứu
// được gì. Cùng một kỹ thuật, đúng ở đây và sai ở kia — khác nhau ở việc
// dữ liệu có bất biến hay không.
func (s *CartStore) Save(ctx context.Context, c *domain.Cart) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cart: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO cart (
			id, customer_id, session_id, currency, status,
			expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			customer_id = EXCLUDED.customer_id,
			status      = EXCLUDED.status,
			expires_at  = EXCLUDED.expires_at,
			updated_at  = EXCLUDED.updated_at`,
		c.ID().String(), c.CustomerID().String(), c.SessionID(),
		string(c.Currency()), string(c.Status()),
		c.ExpiresAt(), c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		// Khách đã có giỏ ACTIVE khác. Chỉ mục UNIQUE có điều kiện là thứ
		// cưỡng chế quy tắc 5 — kiểm tra ở tầng ứng dụng vẫn lọt khi hai
		// tab cùng mở giỏ.
		if isUnique(err, "cart_one_active_per_customer") ||
			isUnique(err, "cart_one_active_per_session") {
			return domain.ErrDuplicateOwner
		}
		return fmt.Errorf("cart: ghi giỏ hàng: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM cart_item WHERE cart_id = $1`,
		c.ID().String()); err != nil {
		return fmt.Errorf("cart: dọn món cũ: %w", err)
	}

	for _, it := range c.Items() {
		_, err := tx.Exec(ctx, `
			INSERT INTO cart_item (
				id, cart_id, offer_id, sku_id, seller_id,
				product_name, variant_description, image_url,
				unit_price, currency, quantity,
				min_order_quantity, max_order_quantity,
				availability, available_quantity,
				source_content_id, source_creator_id,
				added_at, updated_at
			) VALUES (
				$1,$2,$3,$4,$5,
				$6,$7,$8,
				$9,$10,$11,
				$12,$13,
				$14,$15,
				$16,$17,
				$18,$19
			)`,
			it.ID().String(), c.ID().String(), it.OfferID().String(),
			it.SKUID().String(), it.SellerID().String(),
			it.ProductName(), it.VariantDescription(), it.ImageURL(),
			it.UnitPrice().Amount(), string(it.UnitPrice().Currency()), it.Quantity(),
			it.MinOrderQuantity(), it.MaxOrderQuantity(),
			string(it.Availability()), it.AvailableQuantity(),
			it.SourceContentID().String(), it.SourceCreatorID().String(),
			it.AddedAt(), it.UpdatedAt())
		if err != nil {
			return fmt.Errorf("cart: ghi món %q: %w", it.ProductName(), err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cart: xác nhận giao dịch: %w", err)
	}
	return nil
}

const cartCols = `
	id, customer_id, session_id, currency, status,
	expires_at, created_at, updated_at`

func (s *CartStore) FindByID(ctx context.Context, id ids.ID) (*domain.Cart, error) {
	return s.findOne(ctx, `WHERE id = $1`, id.String())
}

func (s *CartStore) FindActiveByCustomer(
	ctx context.Context, customerID ids.ID,
) (*domain.Cart, error) {
	return s.findOne(ctx,
		`WHERE customer_id = $1 AND status = 'ACTIVE'`, customerID.String())
}

func (s *CartStore) FindActiveBySession(
	ctx context.Context, sessionID string,
) (*domain.Cart, error) {
	return s.findOne(ctx,
		`WHERE session_id = $1 AND status = 'ACTIVE'`, sessionID)
}

func (s *CartStore) findOne(
	ctx context.Context, where string, args ...any,
) (*domain.Cart, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+cartCols+` FROM cart `+where, args...)

	var (
		id, customerID, sessionID string
		currency, status          string
		expiresAt                 time.Time
		createdAt, updatedAt      time.Time
	)
	err := row.Scan(&id, &customerID, &sessionID, &currency, &status,
		&expiresAt, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cart: đọc giỏ hàng: %w", err)
	}

	items, err := s.loadItems(ctx, ids.ID(id))
	if err != nil {
		return nil, err
	}

	return domain.RestoreCart(domain.RestoreCartParams{
		ID:         ids.ID(id),
		CustomerID: ids.ID(customerID),
		SessionID:  sessionID,
		Currency:   money.Currency(currency),
		Status:     domain.Status(status),
		Items:      items,
		ExpiresAt:  expiresAt,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}), nil
}

func (s *CartStore) loadItems(ctx context.Context, cartID ids.ID) ([]*domain.Item, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, offer_id, sku_id, seller_id,
		       product_name, variant_description, image_url,
		       unit_price, currency, quantity,
		       min_order_quantity, max_order_quantity,
		       availability, available_quantity,
		       source_content_id, source_creator_id,
		       added_at, updated_at
		  FROM cart_item
		 WHERE cart_id = $1
		 ORDER BY added_at, id`, cartID.String())
	if err != nil {
		return nil, fmt.Errorf("cart: đọc món hàng: %w", err)
	}
	defer rows.Close()

	var out []*domain.Item
	for rows.Next() {
		var (
			id, offerID, skuID, sellerID string
			name, variant, imageURL      string
			unitPrice                    int64
			currency                     string
			quantity, minQty, maxQty     int
			availability                 string
			availableQty                 int
			sourceContent, sourceCreator string
			addedAt, updatedAt           time.Time
		)
		if err := rows.Scan(
			&id, &offerID, &skuID, &sellerID,
			&name, &variant, &imageURL,
			&unitPrice, &currency, &quantity,
			&minQty, &maxQty,
			&availability, &availableQty,
			&sourceContent, &sourceCreator,
			&addedAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("cart: đọc món hàng: %w", err)
		}

		out = append(out, domain.RestoreItem(domain.RestoreItemParams{
			ID:                 ids.ID(id),
			CartID:             cartID,
			OfferID:            ids.ID(offerID),
			SKUID:              ids.ID(skuID),
			SellerID:           ids.ID(sellerID),
			ProductName:        name,
			VariantDescription: variant,
			ImageURL:           imageURL,
			UnitPrice:          mustMoney(unitPrice, money.Currency(currency)),
			Quantity:           quantity,
			MinOrderQuantity:   minQty,
			MaxOrderQuantity:   maxQty,
			Availability:       domain.ItemAvailability(availability),
			AvailableQuantity:  availableQty,
			SourceContentID:    ids.ID(sourceContent),
			SourceCreatorID:    ids.ID(sourceCreator),
			AddedAt:            addedAt,
			UpdatedAt:          updatedAt,
		}))
	}
	return out, rows.Err()
}

func (s *CartStore) Delete(ctx context.Context, id ids.ID) error {
	// cart_item có ON DELETE CASCADE nên món đi theo giỏ.
	tag, err := s.pool.Exec(ctx, `DELETE FROM cart WHERE id = $1`, id.String())
	if err != nil {
		return fmt.Errorf("cart: xóa giỏ hàng: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func mustMoney(amount int64, c money.Currency) money.Money {
	m, err := money.New(amount, c)
	if err != nil {
		panic(fmt.Sprintf("cart: dữ liệu tiền tệ hỏng: %v (%d %s)", err, amount, c))
	}
	return m
}

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == constraint
}
