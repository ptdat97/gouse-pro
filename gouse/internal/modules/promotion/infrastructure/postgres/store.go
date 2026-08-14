// Package postgres cài đặt kho lưu trữ promotion bằng PostgreSQL.
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
	"github.com/fashion-commerce/platform/internal/modules/promotion/domain"
)

// PromotionStore lưu và đọc khuyến mãi.
type PromotionStore struct {
	pool *pgxpool.Pool
}

func NewPromotionStore(pool *pgxpool.Pool) *PromotionStore {
	return &PromotionStore{pool: pool}
}

var _ domain.PromotionRepository = (*PromotionStore)(nil)

const promotionCols = `
	id, name, description, kind, discount_type,
	discount_bps, discount_amount, max_discount_amount, currency,
	min_order_amount, cost_bearer, platform_share_bps, seller_share_bps,
	seller_id, max_uses, max_uses_per_customer, used_count,
	max_budget, used_budget, status, starts_at, ends_at,
	version, created_at, updated_at`

func (s *PromotionStore) Save(ctx context.Context, p *domain.Promotion) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO promotion (
			id, name, description, kind, discount_type,
			discount_bps, discount_amount, max_discount_amount, currency,
			min_order_amount, cost_bearer, platform_share_bps, seller_share_bps,
			seller_id, max_uses, max_uses_per_customer, used_count,
			max_budget, used_budget, status, starts_at, ends_at,
			version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,
		          $17,$18,$19,$20,$21,$22,$23,$24,$25)`,
		p.ID().String(), p.Name(), p.Description(), string(p.Kind()),
		string(p.DiscountType()), p.DiscountBPS().Value(),
		p.DiscountAmount().Amount(), p.MaxDiscountAmount().Amount(),
		string(p.DiscountAmount().Currency()), p.MinOrderAmount().Amount(),
		string(p.CostBearer()), p.PlatformShare().Value(), p.SellerShare().Value(),
		p.SellerID().String(), p.MaxUses(), p.MaxUsesPerCustomer(), p.UsedCount(),
		p.MaxBudget().Amount(), p.UsedBudget().Amount(), string(p.Status()),
		p.StartsAt(), p.EndsAt(), p.Version(), p.CreatedAt(), p.UpdatedAt())
	if err != nil {
		return fmt.Errorf("promotion: ghi khuyến mãi: %w", err)
	}
	return nil
}

// Update ghi thay đổi bằng KHÓA LẠC QUAN.
//
// # Vì sao xung đột ở đây là chuyện THƯỜNG XUYÊN
//
// Một mã đang chạy quảng cáo có thể có hàng trăm người cùng áp trong một
// giây, và MỖI lượt đều tăng used_count. Không có điều kiện `version = ?`
// thì bộ đếm sẽ thấp hơn số lượt thật — và mã giới hạn 100 lượt sẽ được
// dùng vài trăm lần.
func (s *PromotionStore) Update(ctx context.Context, p *domain.Promotion) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE promotion
		   SET name = $2, description = $3, status = $4,
		       used_count = $5, used_budget = $6,
		       starts_at = $7, ends_at = $8,
		       version = $9, updated_at = $10
		 WHERE id = $1 AND version = $11`,
		p.ID().String(), p.Name(), p.Description(), string(p.Status()),
		p.UsedCount(), p.UsedBudget().Amount(),
		p.StartsAt(), p.EndsAt(),
		p.Version(), p.UpdatedAt(), p.Version()-1)
	if err != nil {
		return fmt.Errorf("promotion: cập nhật khuyến mãi: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// Phân biệt "không tồn tại" với "phiên bản đã đổi": báo nhầm xung
		// đột cho bản ghi không tồn tại khiến bên gọi thử lại mãi mãi.
		var exists bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM promotion WHERE id = $1)`,
			p.ID().String()).Scan(&exists); err != nil {
			return fmt.Errorf("promotion: kiểm tra khuyến mãi: %w", err)
		}
		if !exists {
			return domain.ErrNotFound
		}
		return domain.ErrVersionConflict
	}
	return nil
}

func (s *PromotionStore) FindByID(ctx context.Context, id ids.ID) (*domain.Promotion, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT`+promotionCols+` FROM promotion WHERE id = $1`, id.String())

	p, err := scanPromotion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("promotion: đọc khuyến mãi: %w", err)
	}
	return p, nil
}

func (s *PromotionStore) ListActive(
	ctx context.Context, now time.Time,
) ([]*domain.Promotion, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+promotionCols+`
		  FROM promotion
		 WHERE status = 'ACTIVE' AND starts_at <= $1 AND ends_at > $1
		 ORDER BY created_at DESC`, now)
	if err != nil {
		return nil, fmt.Errorf("promotion: đọc khuyến mãi đang chạy: %w", err)
	}
	defer rows.Close()

	var out []*domain.Promotion
	for rows.Next() {
		p, err := scanPromotion(rows)
		if err != nil {
			return nil, fmt.Errorf("promotion: đọc khuyến mãi đang chạy: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ExpireDue chuyển các khuyến mãi quá hạn sang EXPIRED.
//
// MỘT câu UPDATE chứ không phải đọc-rồi-ghi từng bản ghi: đây là việc của
// worker chạy định kỳ, và số bản ghi có thể lớn.
//
// KHÔNG tăng version: đây không phải thay đổi nghiệp vụ do người dùng gây
// ra, và tăng version sẽ làm mọi thao tác đang dở của khách xung đột.
func (s *PromotionStore) ExpireDue(ctx context.Context, now time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE promotion SET status = 'EXPIRED', updated_at = $1
		 WHERE ends_at <= $1 AND status NOT IN ('EXPIRED', 'DRAFT')`, now)
	if err != nil {
		return 0, fmt.Errorf("promotion: hết hạn khuyến mãi: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func scanPromotion(row interface{ Scan(...any) error }) (*domain.Promotion, error) {
	var (
		p                       domain.RestorePromotionParams
		id, sellerID            string
		kind, discountType      string
		bearer, status          string
		currency                string
		discountBPS             int32
		platformBPS, sellerBPS  int32
		discountAmt, maxDiscAmt int64
		minOrder                int64
		maxBudget, usedBudget   int64
	)
	if err := row.Scan(&id, &p.Name, &p.Description, &kind, &discountType,
		&discountBPS, &discountAmt, &maxDiscAmt, &currency,
		&minOrder, &bearer, &platformBPS, &sellerBPS,
		&sellerID, &p.MaxUses, &p.MaxUsesPerCustomer, &p.UsedCount,
		&maxBudget, &usedBudget, &status, &p.StartsAt, &p.EndsAt,
		&p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}

	cur := money.Currency(currency)

	p.ID = ids.ID(id)
	p.SellerID = ids.ID(sellerID)
	p.Kind = domain.Kind(kind)
	p.DiscountType = domain.DiscountType(discountType)
	p.CostBearer = domain.CostBearer(bearer)
	p.Status = domain.Status(status)

	var err error
	if p.DiscountBPS, err = types.NewBasisPoints(discountBPS); err != nil {
		return nil, fmt.Errorf("promotion: tỷ lệ giảm không hợp lệ: %w", err)
	}
	if p.PlatformShareBPS, err = types.NewBasisPoints(platformBPS); err != nil {
		return nil, fmt.Errorf("promotion: tỷ lệ nền tảng không hợp lệ: %w", err)
	}
	if p.SellerShareBPS, err = types.NewBasisPoints(sellerBPS); err != nil {
		return nil, fmt.Errorf("promotion: tỷ lệ seller không hợp lệ: %w", err)
	}

	for _, f := range []struct {
		dst *money.Money
		amt int64
	}{
		{&p.DiscountAmount, discountAmt},
		{&p.MaxDiscountAmount, maxDiscAmt},
		{&p.MinOrderAmount, minOrder},
		{&p.MaxBudget, maxBudget},
		{&p.UsedBudget, usedBudget},
	} {
		m, err := money.New(f.amt, cur)
		if err != nil {
			return nil, fmt.Errorf("promotion: số tiền không hợp lệ: %w", err)
		}
		*f.dst = m
	}

	return domain.RestorePromotion(p), nil
}

// ---------------------------------------------------------------- Mã

// CouponStore lưu và đọc mã giảm giá.
type CouponStore struct {
	pool *pgxpool.Pool
}

func NewCouponStore(pool *pgxpool.Pool) *CouponStore {
	return &CouponStore{pool: pool}
}

var _ domain.CouponRepository = (*CouponStore)(nil)

const couponCols = `
	id, promotion_id, code, customer_id, used_count, active, created_at`

func (s *CouponStore) Save(ctx context.Context, c *domain.Coupon) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO coupon (id, promotion_id, code, customer_id, used_count, active, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID().String(), c.PromotionID().String(), c.Code(),
		c.CustomerID().String(), c.UsedCount(), c.Active(), c.CreatedAt())
	if err != nil {
		if isUnique(err, "coupon_code_key") {
			return domain.ErrInvalidInput
		}
		return fmt.Errorf("promotion: ghi mã giảm giá: %w", err)
	}
	return nil
}

// FindByCode tra mã theo chuỗi ĐÃ CHUẨN HÓA (chữ hoa).
func (s *CouponStore) FindByCode(ctx context.Context, code string) (*domain.Coupon, error) {
	return s.findOne(ctx, `WHERE code = $1`, code)
}

func (s *CouponStore) FindByID(ctx context.Context, id ids.ID) (*domain.Coupon, error) {
	return s.findOne(ctx, `WHERE id = $1`, id.String())
}

func (s *CouponStore) findOne(
	ctx context.Context, where string, args ...any,
) (*domain.Coupon, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+couponCols+` FROM coupon `+where, args...)

	var (
		p                   domain.RestoreCouponParams
		id, promoID, custID string
	)
	err := row.Scan(&id, &promoID, &p.Code, &custID,
		&p.UsedCount, &p.Active, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrCouponNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("promotion: đọc mã giảm giá: %w", err)
	}

	p.ID = ids.ID(id)
	p.PromotionID = ids.ID(promoID)
	p.CustomerID = ids.ID(custID)
	return domain.RestoreCoupon(p), nil
}

func (s *CouponStore) Update(ctx context.Context, c *domain.Coupon) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE coupon SET used_count = $2, active = $3 WHERE id = $1`,
		c.ID().String(), c.UsedCount(), c.Active())
	if err != nil {
		return fmt.Errorf("promotion: cập nhật mã giảm giá: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrCouponNotFound
	}
	return nil
}

// ---------------------------------------------------------------- Lượt dùng

// UsageStore ghi và đọc lượt sử dụng.
type UsageStore struct {
	pool *pgxpool.Pool
}

func NewUsageStore(pool *pgxpool.Pool) *UsageStore {
	return &UsageStore{pool: pool}
}

var _ domain.UsageRepository = (*UsageStore)(nil)

// Record ghi nhận một lượt sử dụng.
//
// IDEMPOTENT Ở TẦNG DATABASE: ràng buộc UNIQUE (coupon_id, order_id) chặn
// ghi trùng. Kiểm tra "đã ghi chưa" ở tầng ứng dụng KHÔNG cứu được khi
// handler xử lý cùng một event hai lần — cả hai lần cùng đọc thấy chưa có
// rồi cùng ghi, và ngân sách bị trừ hai lần cho một đơn.
func (s *UsageStore) Record(ctx context.Context, u domain.Usage) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO coupon_usage (
			coupon_id, promotion_id, customer_id, order_id,
			discount_amount, currency, used_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		u.CouponID.String(), u.PromotionID.String(), u.CustomerID.String(),
		u.OrderID.String(), u.Discount.Amount(),
		string(u.Discount.Currency()), u.UsedAt)
	if err != nil {
		if isUnique(err, "coupon_usage_coupon_id_order_id_key") {
			return domain.ErrAlreadyUsed
		}
		return fmt.Errorf("promotion: ghi lượt sử dụng: %w", err)
	}
	return nil
}

// Release đánh dấu các lượt của một đơn đã được giải phóng.
//
// Điều kiện `released_at IS NULL` là thứ chặn trừ ngân sách hai lần: gọi
// lại với đơn đã giải phóng sẽ không khớp hàng nào, và RETURNING trả về
// danh sách rỗng.
func (s *UsageStore) Release(
	ctx context.Context, orderID ids.ID, now time.Time,
) ([]domain.Usage, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE coupon_usage SET released_at = $2
		 WHERE order_id = $1 AND released_at IS NULL
		 RETURNING coupon_id, promotion_id, customer_id, order_id,
		           discount_amount, currency, used_at, released_at`,
		orderID.String(), now)
	if err != nil {
		return nil, fmt.Errorf("promotion: giải phóng lượt sử dụng: %w", err)
	}
	defer rows.Close()

	var out []domain.Usage
	for rows.Next() {
		u, err := scanUsage(rows)
		if err != nil {
			return nil, fmt.Errorf("promotion: giải phóng lượt sử dụng: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountByCustomer đếm số lượt CHƯA giải phóng của một khách với một mã.
//
// Loại lượt đã giải phóng: đếm cả chúng sẽ chặn nhầm khách có đơn bị hủy
// vì lý do của nền tảng — họ mất lượt mà không mua được gì.
func (s *UsageStore) CountByCustomer(
	ctx context.Context, couponID, customerID ids.ID,
) (int, error) {
	// Khách vãng lai không có customer_id, nên không đếm được lượt của họ.
	// Trả 0 chứ không phải lỗi: giới hạn theo khách chỉ có nghĩa với khách
	// đã định danh, và bên gọi đã biết điều đó.
	if customerID.IsZero() {
		return 0, nil
	}

	var n int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM coupon_usage
		 WHERE coupon_id = $1 AND customer_id = $2 AND released_at IS NULL`,
		couponID.String(), customerID.String()).Scan(&n); err != nil {
		return 0, fmt.Errorf("promotion: đếm lượt sử dụng: %w", err)
	}
	return n, nil
}

func (s *UsageStore) ListByOrder(
	ctx context.Context, orderID ids.ID,
) ([]domain.Usage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT coupon_id, promotion_id, customer_id, order_id,
		       discount_amount, currency, used_at, released_at
		  FROM coupon_usage WHERE order_id = $1 ORDER BY id`,
		orderID.String())
	if err != nil {
		return nil, fmt.Errorf("promotion: đọc lượt sử dụng: %w", err)
	}
	defer rows.Close()

	var out []domain.Usage
	for rows.Next() {
		u, err := scanUsage(rows)
		if err != nil {
			return nil, fmt.Errorf("promotion: đọc lượt sử dụng: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func scanUsage(row interface{ Scan(...any) error }) (domain.Usage, error) {
	var (
		u                         domain.Usage
		couponID, promoID, custID string
		orderID, currency         string
		amount                    int64
		released                  *time.Time
	)
	if err := row.Scan(&couponID, &promoID, &custID, &orderID,
		&amount, &currency, &u.UsedAt, &released); err != nil {
		return domain.Usage{}, err
	}

	u.CouponID = ids.ID(couponID)
	u.PromotionID = ids.ID(promoID)
	u.CustomerID = ids.ID(custID)
	u.OrderID = ids.ID(orderID)
	if released != nil {
		u.ReleasedAt = *released
	}

	m, err := money.New(amount, money.Currency(currency))
	if err != nil {
		return domain.Usage{}, fmt.Errorf("promotion: số tiền không hợp lệ: %w", err)
	}
	u.Discount = m

	return u, nil
}

// ---------------------------------------------------------------- Tiện ích

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
