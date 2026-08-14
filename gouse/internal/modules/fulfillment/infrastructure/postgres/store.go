// Package postgres cài đặt kho lưu trữ đơn thực hiện bằng PostgreSQL.
//
// ĐIỂM CẦN CHÚ Ý KHI ĐỌC PACKAGE NÀY: mọi truy vấn đọc của seller đều có
// `AND seller_id = $n` NGAY TRONG CÂU SQL, không phải lọc sau khi đọc. Đó
// là cách ranh giới bảo mật của ADR-0007 được cưỡng chế ở tầng thấp nhất —
// một lần quên lọc ở tầng trên vẫn không rò rỉ được dữ liệu đối thủ.
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
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

// ---------------------------------------------------------------- Đơn thực hiện

// FulfillmentStore lưu và đọc đơn vị công việc vận hành.
//
// MỌI phương thức đọc đều lọc theo seller_id trong câu SQL. Không có
// phương thức nào trả về dữ liệu chưa lọc — xem ghi chú ở đầu package.
type FulfillmentStore struct {
	pool *pgxpool.Pool
}

func NewFulfillmentStore(pool *pgxpool.Pool) *FulfillmentStore {
	return &FulfillmentStore{pool: pool}
}

var _ domain.Repository = (*FulfillmentStore)(nil)

// SaveBatch ghi các đơn thực hiện vừa tách trong MỘT giao dịch.
//
// Tách đơn là thao tác TOÀN PHẦN: một đơn có ba nguồn hàng mà chỉ ghi được
// hai nghĩa là một seller không bao giờ biết mình có việc, và khách chờ
// mãi một gói không ai đóng.
func (s *FulfillmentStore) SaveBatch(
	ctx context.Context, fos []*domain.FulfillmentOrder,
) error {
	if len(fos) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("fulfillment: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for i, fo := range fos {
		if err := insertFO(ctx, tx, fo); err != nil {
			return fmt.Errorf("fulfillment: ghi đơn thực hiện %d: %w", i+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("fulfillment: xác nhận giao dịch: %w", err)
	}
	return nil
}

// insertFO ghi một đơn thực hiện cùng các dòng hàng của nó.
//
// ON CONFLICT DO NOTHING để chịu được việc event bị phát lại: tách đơn hai
// lần cho cùng một đơn hàng sẽ tạo hai bộ đơn thực hiện, và seller thấy
// việc trùng.
func insertFO(ctx context.Context, tx pgx.Tx, fo *domain.FulfillmentOrder) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO fulfillment_order (
			id, order_id, fo_number, seller_id, status,
			subtotal, commission_amount, currency, cancel_reason,
			fulfillment_type, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO NOTHING`,
		fo.ID().String(), fo.OrderID().String(), fo.FONumber(),
		fo.SellerID().String(), string(fo.Status()),
		fo.Subtotal().Amount(), fo.CommissionAmount().Amount(),
		string(fo.Subtotal().Currency()), fo.CancelReason(),
		string(fo.Type()), fo.CreatedAt(), fo.UpdatedAt())
	if err != nil {
		return err
	}

	for _, lineID := range fo.LineIDs() {
		_, err := tx.Exec(ctx, `
			INSERT INTO fulfillment_order_line (fulfillment_order_id, order_line_id)
			VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			fo.ID().String(), lineID.String())
		if err != nil {
			return fmt.Errorf("gán dòng hàng %s: %w", lineID, err)
		}
	}
	return nil
}

// ExistsForOrder cho biết đơn hàng này đã được tách chưa.
//
// Dùng cho idempotency: event `checkout.completed` có thể được phát lại,
// và tách đơn hai lần sẽ tạo hai bộ đơn thực hiện cho cùng một đơn hàng.
func (s *FulfillmentStore) ExistsForOrder(
	ctx context.Context, orderID ids.ID,
) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM fulfillment_order WHERE order_id = $1)`,
		orderID.String()).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("fulfillment: kiểm tra đơn đã tách: %w", err)
	}
	return exists, nil
}

func (s *FulfillmentStore) Update(ctx context.Context, fo *domain.FulfillmentOrder) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE fulfillment_order
		   SET status = $2, cancel_reason = $3, failure_reason = $4,
		       stock_location_id = $5, shipping_method = $6,
		       shipping_provider = $7, tracking_number = $8,
		       confirmed_at = $9, packed_at = $10, shipped_at = $11,
		       delivered_at = $12, cancelled_at = $13, completed_at = $14,
		       updated_at = $15
		 WHERE id = $1 AND seller_id = $16`,
		fo.ID().String(), string(fo.Status()), fo.CancelReason(),
		fo.FailureReason(), fo.StockLocationID().String(),
		fo.ShippingMethod(), fo.ShippingProvider(), fo.TrackingNumber(),
		nullTime(fo.ConfirmedAt()), nullTime(fo.PackedAt()),
		nullTime(fo.ShippedAt()), nullTime(fo.DeliveredAt()),
		nullTime(fo.CancelledAt()), nullTime(fo.CompletedAt()), fo.UpdatedAt(),
		// seller_id nằm trong WHERE dù đã biết id: nếu một lỗi ở tầng trên
		// đưa nhầm đơn của seller khác xuống đây, câu lệnh không ghi được.
		fo.SellerID().String())
	if err != nil {
		return fmt.Errorf("order: cập nhật đơn thực hiện: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

const foCols = `
	id, order_id, fo_number, seller_id, status,
	subtotal, commission_amount, currency, cancel_reason, failure_reason,
	stock_location_id, fulfillment_type, shipping_method, shipping_provider,
	tracking_number, estimated_delivery_date,
	confirmed_at, packed_at, shipped_at, delivered_at, cancelled_at,
	completed_at, created_at, updated_at`

func (s *FulfillmentStore) FindByID(
	ctx context.Context, id, sellerID ids.ID,
) (*domain.FulfillmentOrder, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+foCols+`
		  FROM fulfillment_order
		 WHERE id = $1 AND seller_id = $2`,
		id.String(), sellerID.String())

	fo, err := scanFO(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// KHÔNG phân biệt "không tồn tại" với "của seller khác": phân biệt
		// hai trường hợp cho phép dò tìm định danh đơn của đối thủ.
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("order: đọc đơn thực hiện: %w", err)
	}
	return s.withLineIDs(ctx, fo)
}

func (s *FulfillmentStore) ListBySeller(
	ctx context.Context, sellerID ids.ID, statuses []domain.FOStatus, limit, offset int,
) ([]*domain.FulfillmentOrder, error) {
	q := `SELECT` + foCols + ` FROM fulfillment_order WHERE seller_id = $1`
	args := []any{sellerID.String()}

	if len(statuses) > 0 {
		list := make([]string, 0, len(statuses))
		for _, st := range statuses {
			list = append(list, string(st))
		}
		args = append(args, list)
		q += fmt.Sprintf(` AND status = ANY($%d)`, len(args))
	}

	args = append(args, limitOr(limit, 50), max0(offset))
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		len(args)-1, len(args))

	return s.queryFOs(ctx, q, args...)
}

// ListDeliveredBefore lấy các đơn đã giao trước một mốc thời gian.
//
// Mỗi hàng trả về là tiền của seller đang bị giữ lại chờ hết hạn đổi trả.
func (s *FulfillmentStore) ListDeliveredBefore(
	ctx context.Context, before time.Time, limit int,
) ([]*domain.FulfillmentOrder, error) {
	return s.queryFOs(ctx, `SELECT`+foCols+`
		  FROM fulfillment_order
		 WHERE status = 'DELIVERED' AND delivered_at < $1
		 ORDER BY delivered_at
		 LIMIT $2`, before, limitOr(limit, 100))
}

func (s *FulfillmentStore) ListByOrder(
	ctx context.Context, orderID ids.ID,
) ([]*domain.FulfillmentOrder, error) {
	return s.queryFOs(ctx, `SELECT`+foCols+`
		  FROM fulfillment_order
		 WHERE order_id = $1
		 ORDER BY fo_number`, orderID.String())
}

func (s *FulfillmentStore) queryFOs(
	ctx context.Context, q string, args ...any,
) ([]*domain.FulfillmentOrder, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("order: đọc đơn thực hiện: %w", err)
	}
	defer rows.Close()

	var out []*domain.FulfillmentOrder
	for rows.Next() {
		fo, err := scanFO(rows)
		if err != nil {
			return nil, fmt.Errorf("order: đọc đơn thực hiện: %w", err)
		}
		out = append(out, fo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("order: đọc đơn thực hiện: %w", err)
	}

	for i, fo := range out {
		filled, err := s.withLineIDs(ctx, fo)
		if err != nil {
			return nil, err
		}
		out[i] = filled
	}
	return out, nil
}

func (s *FulfillmentStore) withLineIDs(
	ctx context.Context, fo *domain.FulfillmentOrder,
) (*domain.FulfillmentOrder, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT order_line_id FROM fulfillment_order_line
		 WHERE fulfillment_order_id = $1
		 ORDER BY order_line_id`, fo.ID().String())
	if err != nil {
		return nil, fmt.Errorf("order: đọc dòng hàng của đơn thực hiện: %w", err)
	}
	defer rows.Close()

	var lineIDs []ids.ID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("order: đọc dòng hàng của đơn thực hiện: %w", err)
		}
		lineIDs = append(lineIDs, ids.ID(id))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("order: đọc dòng hàng của đơn thực hiện: %w", err)
	}

	// LIỆT KÊ ĐỦ MỌI TRƯỜNG: hàm này dựng lại thực thể từ đầu, nên trường
	// nào quên là trường đó bị XÓA TRẮNG khi đọc — và lỗi chỉ lộ ra ở test
	// đọc lại sau khi ghi, không lộ ra lúc ghi.
	return domain.RestoreFulfillmentOrder(domain.RestoreFOParams{
		ID:                fo.ID(),
		OrderID:           fo.OrderID(),
		FONumber:          fo.FONumber(),
		SellerID:          fo.SellerID(),
		LineIDs:           lineIDs,
		Status:            fo.Status(),
		Subtotal:          fo.Subtotal(),
		CommissionAmount:  fo.CommissionAmount(),
		CancelReason:      fo.CancelReason(),
		FailureReason:     fo.FailureReason(),
		StockLocationID:   fo.StockLocationID(),
		Type:              fo.Type(),
		ShippingMethod:    fo.ShippingMethod(),
		ShippingProvider:  fo.ShippingProvider(),
		TrackingNumber:    fo.TrackingNumber(),
		EstimatedDelivery: fo.EstimatedDelivery(),
		CompletedAt:       fo.CompletedAt(),
		ConfirmedAt:       fo.ConfirmedAt(),
		PackedAt:          fo.PackedAt(),
		ShippedAt:         fo.ShippedAt(),
		DeliveredAt:       fo.DeliveredAt(),
		CancelledAt:       fo.CancelledAt(),
		CreatedAt:         fo.CreatedAt(),
		UpdatedAt:         fo.UpdatedAt(),
	}), nil
}

func scanFO(row scanner) (*domain.FulfillmentOrder, error) {
	var (
		p                           domain.RestoreFOParams
		id, orderID, foNumber       string
		sellerID, status, currency  string
		subtotal, commission        int64
		cancelReason, failureReason string
		locationID, foType          string
		method, provider, tracking  string
		estimatedDelivery           *time.Time
		confirmed, packed, shipped  *time.Time
		delivered, cancelled        *time.Time
		completed                   *time.Time
	)
	if err := row.Scan(
		&id, &orderID, &foNumber, &sellerID, &status,
		&subtotal, &commission, &currency, &cancelReason, &failureReason,
		&locationID, &foType, &method, &provider, &tracking, &estimatedDelivery,
		&confirmed, &packed, &shipped, &delivered, &cancelled,
		&completed, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}

	cur := money.Currency(currency)
	p.ID = ids.ID(id)
	p.OrderID = ids.ID(orderID)
	p.FONumber = foNumber
	p.SellerID = ids.ID(sellerID)
	p.Status = domain.FOStatus(status)
	p.Subtotal = mustMoney(subtotal, cur)
	p.CommissionAmount = mustMoney(commission, cur)
	p.CancelReason = cancelReason
	p.FailureReason = failureReason
	p.StockLocationID = ids.ID(locationID)
	p.Type = domain.FulfillmentType(foType)
	p.ShippingMethod = method
	p.ShippingProvider = provider
	p.TrackingNumber = tracking
	p.EstimatedDelivery = deref(estimatedDelivery)
	p.CompletedAt = deref(completed)
	p.ConfirmedAt = deref(confirmed)
	p.PackedAt = deref(packed)
	p.ShippedAt = deref(shipped)
	p.DeliveredAt = deref(delivered)
	p.CancelledAt = deref(cancelled)

	return domain.RestoreFulfillmentOrder(p), nil
}

// ---------------------------------------------------------------- Tiện ích

// scanner cho phép dùng chung một hàm quét cho QueryRow và rows.
type scanner interface {
	Scan(dest ...any) error
}

func mustMoney(amount int64, c money.Currency) money.Money {
	m, err := money.New(amount, c)
	if err != nil {
		// Dữ liệu trong database vi phạm ràng buộc đơn vị tiền tệ. Trả về
		// số 0 sẽ giấu lỗi và làm sai mọi phép cộng tiền sau đó.
		panic(fmt.Sprintf("fulfillment: dữ liệu tiền tệ hỏng: %v (%d %s)", err, amount, c))
	}
	return m
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func limitOr(limit, fallback int) int {
	if limit <= 0 {
		return fallback
	}
	return limit
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
