// Package postgres cài đặt kho lưu trữ phiên thanh toán bằng PostgreSQL.
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
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/checkout/domain"
)

// CheckoutStore lưu và đọc phiên thanh toán.
type CheckoutStore struct {
	pool *pgxpool.Pool
}

func NewCheckoutStore(pool *pgxpool.Pool) *CheckoutStore {
	return &CheckoutStore{pool: pool}
}

var _ domain.Repository = (*CheckoutStore)(nil)

// Save ghi phiên và các dòng trong MỘT giao dịch.
//
// Dòng hàng CHỈ ghi một lần rồi không đổi: chúng mang giá ĐÃ ĐÓNG BĂNG,
// nên không có câu UPDATE nào chạm vào chúng ở đây hay bất kỳ đâu khác
// trong package này. Chỉ phiên (địa chỉ, phí ship, trạng thái) đổi được.
func (s *CheckoutStore) Save(ctx context.Context, c *domain.Checkout) error {
	return s.SaveWithEvents(ctx, c, nil)
}

// SaveWithEvents ghi phiên VÀ chạy fn trong CÙNG một giao dịch.
//
// fn là nơi tầng application phát domain event. Ngữ cảnh truyền cho fn
// mang giao dịch này, nên event vào outbox cùng lúc với thay đổi phiên —
// cùng thành công hoặc cùng thất bại.
//
// Không có điều đó thì phiên có thể chuyển COMPLETED mà event không được
// ghi, và tồn kho sẽ mãi nằm ở Reserved cho một đơn đã bán xong.
func (s *CheckoutStore) SaveWithEvents(
	ctx context.Context, c *domain.Checkout, fn domain.TxFunc,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("checkout: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	a := c.ShippingAddress()

	_, err = tx.Exec(ctx, `
		INSERT INTO checkout (
			id, cart_id, customer_id, guest_email, guest_phone, currency,
			ship_recipient_name, ship_phone, ship_street, ship_ward,
			ship_district, ship_province, ship_country_code, shipping_method,
			shipping_fee, discount_amount, tax_amount, coupon_code,
			status, expires_at, extended_times, order_id, completion_key,
			created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,
			$7,$8,$9,$10,$11,$12,$13,$14,
			$15,$16,$17,$18,
			$19,$20,$21,$22,$23,
			$24,$25
		)
		ON CONFLICT (id) DO UPDATE SET
			ship_recipient_name = EXCLUDED.ship_recipient_name,
			ship_phone          = EXCLUDED.ship_phone,
			ship_street         = EXCLUDED.ship_street,
			ship_ward           = EXCLUDED.ship_ward,
			ship_district       = EXCLUDED.ship_district,
			ship_province       = EXCLUDED.ship_province,
			ship_country_code   = EXCLUDED.ship_country_code,
			shipping_method     = EXCLUDED.shipping_method,
			shipping_fee        = EXCLUDED.shipping_fee,
			discount_amount     = EXCLUDED.discount_amount,
			tax_amount          = EXCLUDED.tax_amount,
			coupon_code         = EXCLUDED.coupon_code,
			status              = EXCLUDED.status,
			expires_at          = EXCLUDED.expires_at,
			extended_times      = EXCLUDED.extended_times,
			order_id            = EXCLUDED.order_id,
			completion_key      = EXCLUDED.completion_key,
			updated_at          = EXCLUDED.updated_at`,
		c.ID().String(), c.CartID().String(), c.CustomerID().String(),
		c.GuestEmail(), c.GuestPhone(), string(c.Currency()),
		a.RecipientName, a.Phone, a.StreetAddress, a.Ward,
		a.District, a.Province, defaultCountry(a.CountryCode), c.ShippingMethod(),
		c.ShippingFee().Amount(), c.DiscountAmount().Amount(),
		c.TaxAmount().Amount(), c.CouponCode(),
		string(c.Status()), c.ExpiresAt(), c.ExtendedTimes(),
		c.OrderID().String(), c.CompletionKey(),
		c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		// Giỏ này đã có phiên đang chạy. Mở phiên thứ hai sẽ giữ hàng lần
		// thứ hai cho cùng một giỏ.
		if isUnique(err, "checkout_one_active_per_cart") {
			return domain.ErrInvalidStatus
		}
		return fmt.Errorf("checkout: ghi phiên thanh toán: %w", err)
	}

	for _, l := range c.Lines() {
		_, err := tx.Exec(ctx, `
			INSERT INTO checkout_line (
				id, checkout_id, cart_item_id, offer_id, sku_id, seller_id,
				product_name, variant_description, unit_price, currency,
				quantity, commission_rate,
				reservation_id, inventory_item_id, created_at
			) VALUES (
				$1,$2,$3,$4,$5,$6,
				$7,$8,$9,$10,
				$11,$12,
				$13,$14,$15
			)
			ON CONFLICT (id) DO NOTHING`,
			l.ID().String(), c.ID().String(), l.CartItemID().String(),
			l.OfferID().String(), l.SKUID().String(), l.SellerID().String(),
			l.ProductName(), l.VariantDescription(),
			l.UnitPrice().Amount(), string(l.UnitPrice().Currency()),
			l.Quantity(), int(l.CommissionRate().Value()),
			l.ReservationID().String(), l.InventoryItemID().String(),
			l.CreatedAt())
		if err != nil {
			return fmt.Errorf("checkout: ghi dòng %q: %w", l.ProductName(), err)
		}
	}

	if fn != nil {
		if err := fn(withTx(ctx, tx)); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("checkout: xác nhận giao dịch: %w", err)
	}
	return nil
}

// txKey gắn giao dịch vào ngữ cảnh cho TxFunc.
//
// Dùng kiểu riêng chứ không phải chuỗi: hai gói cùng dùng khóa "tx" sẽ ghi
// đè nhau mà không ai biết.
type txKey struct{}

func withTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFrom lấy giao dịch mà SaveWithEvents đang mở.
func TxFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

const checkoutCols = `
	id, cart_id, customer_id, guest_email, guest_phone, currency,
	ship_recipient_name, ship_phone, ship_street, ship_ward,
	ship_district, ship_province, ship_country_code, shipping_method,
	shipping_fee, discount_amount, tax_amount, coupon_code,
	status, expires_at, extended_times, order_id, completion_key,
	created_at, updated_at`

func (s *CheckoutStore) FindByID(ctx context.Context, id ids.ID) (*domain.Checkout, error) {
	return s.findOne(ctx, `WHERE id = $1`, id.String())
}

func (s *CheckoutStore) FindByCompletionKey(
	ctx context.Context, key string,
) (*domain.Checkout, error) {
	return s.findOne(ctx, `WHERE completion_key = $1`, key)
}

func (s *CheckoutStore) FindActiveByCart(
	ctx context.Context, cartID ids.ID,
) (*domain.Checkout, error) {
	return s.findOne(ctx,
		`WHERE cart_id = $1 AND status IN ('STARTED','PENDING_PAYMENT')`,
		cartID.String())
}

// FindExpired lấy các phiên quá hạn mà chưa được dọn.
//
// Mỗi hàng trả về là hàng đang KHÓA HÀNG mà không ai dùng tới.
func (s *CheckoutStore) FindExpired(
	ctx context.Context, now time.Time, limit int,
) ([]*domain.Checkout, error) {
	if limit <= 0 {
		limit = 100
	}

	// `order_id = ''` LOẠI phiên đã tạo được đơn.
	//
	// Không có điều kiện này, một phiên mà chuỗi hoàn tất hỏng giữa chừng
	// (đơn đã tạo, giao dịch ghi phiên chưa xong) sẽ bị dọn như phiên bỏ
	// dở — và nhả toàn bộ hàng của một đơn có thật. Xem `GhiNhanDaTaoDon`.
	rows, err := s.pool.Query(ctx, `SELECT`+checkoutCols+`
		  FROM checkout
		 WHERE status IN ('STARTED','PENDING_PAYMENT')
		   AND order_id = ''
		   AND expires_at < $1
		 ORDER BY expires_at
		 LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("checkout: tìm phiên quá hạn: %w", err)
	}
	defer rows.Close()

	var out []*domain.Checkout
	for rows.Next() {
		c, err := scanCheckout(rows)
		if err != nil {
			return nil, fmt.Errorf("checkout: đọc phiên quá hạn: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checkout: đọc phiên quá hạn: %w", err)
	}

	// Nạp dòng SAU khi đóng con trỏ: truy vấn lồng trong vòng lặp rows sẽ
	// chiếm thêm kết nối và cạn pool khi có tải.
	//
	// Ở đây bắt buộc phải nạp: tiến trình dọn cần reservation_id của từng
	// dòng để nhả hàng, và đó là toàn bộ mục đích của việc quét này.
	for i, c := range out {
		lines, err := s.loadLines(ctx, c.ID())
		if err != nil {
			return nil, err
		}
		out[i] = withLines(c, lines)
	}
	return out, nil
}

func (s *CheckoutStore) CountExpiredPending(ctx context.Context, now time.Time) (int, error) {
	var n int
	// Cùng điều kiện với FindExpired, kể cả `order_id = ''`.
	//
	// Lệch nhau thì chỉ báo nói dối: nó đếm cả những phiên mà job dọn CỐ Ý
	// không đụng tới, con số tăng mãi, và cảnh báo "tiến trình dọn đã
	// ngừng chạy" kêu suốt trong khi tiến trình vẫn chạy bình thường.
	// Một cảnh báo kêu sai vài lần là một cảnh báo không ai đọc nữa.
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM checkout
		 WHERE status IN ('STARTED','PENDING_PAYMENT')
		   AND order_id = ''
		   AND expires_at < $1`,
		now).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("checkout: đếm phiên quá hạn: %w", err)
	}
	return n, nil
}

// CountHoanTatKetLai đếm phiên ĐÃ TẠO ĐƠN nhưng chưa hoàn tất xong.
//
// Đây là mặt trái của việc loại chúng khỏi job dọn: hàng của chúng nằm
// giữ vô thời hạn. Đánh đổi ấy chỉ chấp nhận được nếu số hàng chết ĐẾM
// ĐƯỢC — nếu không thì "thà hàng chết còn hơn hàng ma" chỉ là cách nói
// khác của "giấu vấn đề đi".
//
// Con số này bình thường phải là 0. Khác 0 nghĩa là có chuỗi hoàn tất
// dừng giữa chừng: đơn đã tồn tại, phiên chưa đóng, và cần người đối soát.
func (s *CheckoutStore) CountHoanTatKetLai(ctx context.Context, now time.Time) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM checkout
		 WHERE status IN ('STARTED','PENDING_PAYMENT')
		   AND order_id <> ''
		   AND expires_at < $1`,
		now).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("checkout: đếm phiên kẹt sau khi tạo đơn: %w", err)
	}
	return n, nil
}

// GhiNhanDaTaoDon ghi mã đơn lên phiên ngay sau khi đơn được tạo.
//
// Xem chú thích ở cổng (domain/repository.go) về vì sao cần một lượt ghi
// riêng thay vì đợi Save.
func (s *CheckoutStore) GhiNhanDaTaoDon(
	ctx context.Context, id ids.ID, orderID ids.ID,
) error {
	// Chỉ ghi khi phiên CHƯA có mã đơn: lần gọi lại (khách thử lại, đơn
	// idempotent trả về cùng mã) không được ghi đè, và một mã đơn khác
	// tới đây là dấu hiệu sai sót nghiêm trọng chứ không phải chuyện
	// bình thường — im lặng ghi đè sẽ xóa mất manh mối duy nhất.
	_, err := s.pool.Exec(ctx, `
		UPDATE checkout SET order_id = $2, updated_at = now()
		 WHERE id = $1 AND order_id = ''`,
		id.String(), orderID.String())
	if err != nil {
		return fmt.Errorf("checkout: ghi nhận đơn đã tạo: %w", err)
	}
	return nil
}

func (s *CheckoutStore) findOne(
	ctx context.Context, where string, args ...any,
) (*domain.Checkout, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+checkoutCols+` FROM checkout `+where, args...)

	c, err := scanCheckout(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("checkout: đọc phiên thanh toán: %w", err)
	}

	lines, err := s.loadLines(ctx, c.ID())
	if err != nil {
		return nil, err
	}
	return withLines(c, lines), nil
}

func (s *CheckoutStore) loadLines(ctx context.Context, checkoutID ids.ID) ([]*domain.Line, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, cart_item_id, offer_id, sku_id, seller_id,
		       product_name, variant_description, unit_price, currency,
		       quantity, commission_rate,
		       reservation_id, inventory_item_id, created_at
		  FROM checkout_line
		 WHERE checkout_id = $1
		 ORDER BY created_at, id`, checkoutID.String())
	if err != nil {
		return nil, fmt.Errorf("checkout: đọc dòng hàng: %w", err)
	}
	defer rows.Close()

	var out []*domain.Line
	for rows.Next() {
		var (
			id, cartItemID, offerID    string
			skuID, sellerID            string
			name, variant, currency    string
			unitPrice                  int64
			quantity, commissionRate   int
			reservationID, inventoryID string
			createdAt                  time.Time
		)
		if err := rows.Scan(
			&id, &cartItemID, &offerID, &skuID, &sellerID,
			&name, &variant, &unitPrice, &currency,
			&quantity, &commissionRate,
			&reservationID, &inventoryID, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("checkout: đọc dòng hàng: %w", err)
		}

		out = append(out, domain.RestoreLine(domain.RestoreLineParams{
			ID:                 ids.ID(id),
			CheckoutID:         checkoutID,
			CartItemID:         ids.ID(cartItemID),
			OfferID:            ids.ID(offerID),
			SKUID:              ids.ID(skuID),
			SellerID:           ids.ID(sellerID),
			ProductName:        name,
			VariantDescription: variant,
			UnitPrice:          mustMoney(unitPrice, money.Currency(currency)),
			Quantity:           quantity,
			CommissionRate:     mustRate(commissionRate),
			ReservationID:      ids.ID(reservationID),
			InventoryItemID:    ids.ID(inventoryID),
			CreatedAt:          createdAt,
		}))
	}
	return out, rows.Err()
}

// scanner cho phép dùng chung một hàm quét cho QueryRow và rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanCheckout(row scanner) (*domain.Checkout, error) {
	var (
		p                             domain.RestoreCheckoutParams
		id, cartID, customerID        string
		email, phone, currency        string
		addr                          domain.Address
		method, status                string
		shippingFee, discount, tax    int64
		coupon, orderID, completedKey string
		extendedTimes                 int
	)
	if err := row.Scan(
		&id, &cartID, &customerID, &email, &phone, &currency,
		&addr.RecipientName, &addr.Phone, &addr.StreetAddress, &addr.Ward,
		&addr.District, &addr.Province, &addr.CountryCode, &method,
		&shippingFee, &discount, &tax, &coupon,
		&status, &p.ExpiresAt, &extendedTimes, &orderID, &completedKey,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}

	cur := money.Currency(currency)
	p.ID = ids.ID(id)
	p.CartID = ids.ID(cartID)
	p.CustomerID = ids.ID(customerID)
	p.GuestEmail = email
	p.GuestPhone = phone
	p.Currency = cur
	p.ShippingAddress = addr
	p.ShippingMethod = method
	p.ShippingFee = mustMoney(shippingFee, cur)
	p.DiscountAmount = mustMoney(discount, cur)
	p.TaxAmount = mustMoney(tax, cur)
	p.CouponCode = coupon
	p.Status = domain.Status(status)
	p.ExtendedTimes = extendedTimes
	p.OrderID = ids.ID(orderID)
	p.CompletionKey = completedKey

	return domain.RestoreCheckout(p), nil
}

// withLines dựng lại phiên kèm dòng hàng.
func withLines(c *domain.Checkout, lines []*domain.Line) *domain.Checkout {
	return domain.RestoreCheckout(domain.RestoreCheckoutParams{
		ID:              c.ID(),
		CartID:          c.CartID(),
		CustomerID:      c.CustomerID(),
		GuestEmail:      c.GuestEmail(),
		GuestPhone:      c.GuestPhone(),
		Currency:        c.Currency(),
		ShippingAddress: c.ShippingAddress(),
		ShippingMethod:  c.ShippingMethod(),
		Lines:           lines,
		ShippingFee:     c.ShippingFee(),
		DiscountAmount:  c.DiscountAmount(),
		TaxAmount:       c.TaxAmount(),
		CouponCode:      c.CouponCode(),
		Status:          c.Status(),
		ExpiresAt:       c.ExpiresAt(),
		ExtendedTimes:   c.ExtendedTimes(),
		OrderID:         c.OrderID(),
		CompletionKey:   c.CompletionKey(),
		CreatedAt:       c.CreatedAt(),
		UpdatedAt:       c.UpdatedAt(),
	})
}

func mustMoney(amount int64, c money.Currency) money.Money {
	m, err := money.New(amount, c)
	if err != nil {
		panic(fmt.Sprintf("checkout: dữ liệu tiền tệ hỏng: %v (%d %s)", err, amount, c))
	}
	return m
}

func mustRate(v int) types.BasisPoints {
	bp, err := types.NewBasisPoints(int32(v))
	if err != nil {
		panic(fmt.Sprintf("checkout: tỷ lệ đã lưu không hợp lệ: %d", v))
	}
	return bp
}

func defaultCountry(c string) string {
	if c == "" {
		return "VN"
	}
	return c
}

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == constraint
}

// GiuDeHoanTat đẩy hạn phiên lên trước một quãng ân hạn, nếu phiên còn
// hiệu lực. Xem domain.Repository để biết vì sao cần.
func (s *CheckoutStore) GiuDeHoanTat(
	ctx context.Context, id ids.ID, now time.Time, anHan time.Duration,
) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE checkout
		   SET expires_at = GREATEST(expires_at, $2::timestamptz + $3::interval),
		       updated_at = $2
		 WHERE id = $1
		   AND status IN ('STARTED','PENDING_PAYMENT')
		   AND expires_at > $2`,
		id.String(), now, anHan.String())
	if err != nil {
		return fmt.Errorf("checkout: giữ phiên để hoàn tất: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}

	// Không giành được. Đọc lại để nói ĐÚNG lý do: "hết hạn" và "đã kết
	// thúc" dẫn tới hai hành động khác nhau ở phía khách.
	var status string
	var expiresAt time.Time
	err = s.pool.QueryRow(ctx,
		`SELECT status, expires_at FROM checkout WHERE id = $1`, id.String()).
		Scan(&status, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("checkout: đọc lại phiên: %w", err)
	}
	// PHÂN BIỆT hai lý do: khách hết hạn phiên cần mở lại giỏ và làm lại,
	// còn phiên đã hủy hay đã thành đơn thì làm lại là sai.
	if domain.Status(status) == domain.StatusExpired {
		return domain.ErrExpired
	}
	if domain.Status(status).IsFinal() {
		return domain.ErrInvalidStatus
	}
	return domain.ErrExpired
}

// GiuDeDonHan đánh dấu một phiên quá hạn là EXPIRED, nếu chưa ai giành.
func (s *CheckoutStore) GiuDeDonHan(
	ctx context.Context, id ids.ID, now time.Time,
) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE checkout
		   SET status = 'EXPIRED', updated_at = $2
		 WHERE id = $1
		   AND status IN ('STARTED','PENDING_PAYMENT')
		   AND expires_at <= $2`,
		id.String(), now)
	if err != nil {
		return false, fmt.Errorf("checkout: giữ phiên để dọn: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
