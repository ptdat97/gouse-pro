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

// querier là thứ chạy được câu lệnh: pool HOẶC giao dịch.
//
// Cần nó vì cùng một câu đọc phải chạy được cả ngoài giao dịch (đường đọc
// thường) lẫn TRONG giao dịch đang giữ khóa dòng (đường đọc-sửa-ghi). Hai
// bản sao của cùng câu lệnh sẽ trôi xa nhau.
type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

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
	return s.SaveWithEvents(ctx, c, nil)
}

// SaveWithEvents ghi giỏ VÀ chạy fn trong CÙNG một giao dịch.
//
// fn là nơi phát domain event. Cùng giao dịch nghĩa là giỏ và event cùng
// thành công hoặc cùng thất bại — không có trường hợp món đã vào giỏ mà
// tín hiệu nhu cầu bị mất.
func (s *CartStore) SaveWithEvents(
	ctx context.Context, c *domain.Cart, fn domain.TxFunc,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cart: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.ghi(ctx, tx, c); err != nil {
		return err
	}

	if fn != nil {
		if err := fn(withTx(ctx, tx)); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cart: xác nhận giao dịch: %w", err)
	}
	return nil
}

// ghi ghi giỏ và toàn bộ món bằng querier bên gọi đưa vào.
//
// KHÔNG tự mở giao dịch: bên gọi quyết định ranh giới giao dịch, vì chỉ
// bên gọi biết còn thao tác nào phải cùng thành công hay không.
func (s *CartStore) ghi(ctx context.Context, tx querier, c *domain.Cart) error {
	_, err := tx.Exec(ctx, `
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
				product_name, variant_description, image_url, seller_name,
				unit_price, currency, quantity,
				min_order_quantity, max_order_quantity,
				availability, available_quantity,
				source_content_id, source_creator_id,
				added_at, updated_at
			) VALUES (
				$1,$2,$3,$4,$5,
				$6,$7,$8,$9,
				$10,$11,$12,
				$13,$14,
				$15,$16,
				$17,$18,
				$19,$20
			)`,
			it.ID().String(), c.ID().String(), it.OfferID().String(),
			it.SKUID().String(), it.SellerID().String(),
			it.ProductName(), it.VariantDescription(), it.ImageURL(), it.SellerName(),
			it.UnitPrice().Amount(), string(it.UnitPrice().Currency()), it.Quantity(),
			it.MinOrderQuantity(), it.MaxOrderQuantity(),
			string(it.Availability()), it.AvailableQuantity(),
			it.SourceContentID().String(), it.SourceCreatorID().String(),
			it.AddedAt(), it.UpdatedAt())
		if err != nil {
			return fmt.Errorf("cart: ghi món %q: %w", it.ProductName(), err)
		}
	}

	return nil
}

// MutateWithEvents đọc-sửa-ghi một giỏ TRONG MỘT giao dịch, có khóa dòng.
//
// # Vì sao phải có hàm này
//
// Đường cũ là: `FindByID` ở một giao dịch, sửa trong bộ nhớ, rồi `Save` ở
// một giao dịch KHÁC. Hai lượt thêm hàng chạy chồng nhau sẽ cùng đọc "giỏ
// đang có 1", cùng tính "thành 2", cùng ghi 2 — và vì cách ghi là XÓA HẾT
// RỒI GHI LẠI, lượt sau ghi đè trọn vẹn lượt trước. Cả hai đều trả 200.
// Khách bấm sáu lần, giỏ tăng ba.
//
// Đó KHÔNG phải lỗi thiếu khóa mà là RANH GIỚI GIAO DỊCH đặt sai: phép
// đọc-rồi-ghi bị cắt làm đôi. Chỗ đúng để sửa là gộp nó lại, chứ không
// phải thêm mutex ở tầng ứng dụng hay thử lại cho tới khi trúng.
//
// `FOR UPDATE` trên dòng giỏ khiến các lượt cùng giỏ xếp hàng — đúng thứ
// ta muốn ở đây: xung đột là chuyện THƯỜNG XUYÊN (một khách, một giỏ, hai
// tab), và cửa sổ giữ khóa chỉ gồm vài câu lệnh trên chính database này.
// Khóa lạc quan hợp với chỗ xung đột HIẾM, như tồn kho.
//
// Mọi lệnh gọi ra ngoài — tra offer, tra giá — phải nằm TRƯỚC khi vào đây.
func (s *CartStore) MutateWithEvents(
	ctx context.Context, cartID ids.ID,
	apply func(*domain.Cart) error, fn domain.TxFunc,
) (*domain.Cart, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("cart: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Khóa TRƯỚC khi đọc. Đọc rồi mới khóa là để lại đúng cái cửa sổ
	// đang muốn đóng.
	var khoa string
	err = tx.QueryRow(ctx,
		`SELECT id FROM cart WHERE id = $1 FOR UPDATE`, cartID.String()).Scan(&khoa)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cart: khóa giỏ hàng: %w", err)
	}

	c, err := s.findOneVoi(ctx, tx, `WHERE id = $1`, cartID.String())
	if err != nil {
		return nil, err
	}

	if err := apply(c); err != nil {
		return nil, err
	}

	if err := s.ghi(ctx, tx, c); err != nil {
		return nil, err
	}

	if fn != nil {
		if err := fn(withTx(ctx, tx)); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("cart: xác nhận giao dịch: %w", err)
	}
	return c, nil
}

// txKey gắn giao dịch vào ngữ cảnh cho TxFunc.
type txKey struct{}

func withTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFrom lấy giao dịch mà SaveWithEvents đang mở.
func TxFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
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
	return s.findOneVoi(ctx, s.pool, where, args...)
}

func (s *CartStore) findOneVoi(
	ctx context.Context, q querier, where string, args ...any,
) (*domain.Cart, error) {
	row := q.QueryRow(ctx, `SELECT`+cartCols+` FROM cart `+where, args...)

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

	items, err := s.loadItems(ctx, q, ids.ID(id))
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

func (s *CartStore) loadItems(
	ctx context.Context, q querier, cartID ids.ID,
) ([]*domain.Item, error) {
	rows, err := q.Query(ctx, `
		SELECT id, offer_id, sku_id, seller_id,
		       product_name, variant_description, image_url, seller_name,
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
			sellerName                   string
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
			&name, &variant, &imageURL, &sellerName,
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
			SellerName:         sellerName,
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
