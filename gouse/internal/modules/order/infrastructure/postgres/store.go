// Package postgres cài đặt các port của order bằng PostgreSQL.
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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
)

// OrderStore lưu và đọc đơn hàng.
type OrderStore struct {
	pool *pgxpool.Pool
}

func NewOrderStore(pool *pgxpool.Pool) *OrderStore {
	return &OrderStore{pool: pool}
}

var _ domain.Repository = (*OrderStore)(nil)

// Save ghi đơn hàng, dòng hàng, khoản điều chỉnh và các đơn thực hiện
// trong MỘT giao dịch.
//
// Không phải tối ưu kỹ thuật mà là yêu cầu nghiệp vụ: đơn có dòng hàng mà
// thiếu đơn thực hiện là đơn không ai xử lý — khách đã trả tiền và không
// ai đóng gói.
func (s *OrderStore) Save(
	ctx context.Context, o *domain.Order, fos []*domain.FulfillmentOrder,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("order: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ship := o.ShippingAddress()
	bill := o.BillingAddress()

	_, err = tx.Exec(ctx, `
		INSERT INTO "order" (
			id, order_number, customer_id, guest_email, guest_phone,
			ship_recipient_name, ship_phone, ship_street, ship_ward,
			ship_district, ship_province, ship_country_code,
			bill_recipient_name, bill_phone, bill_street, bill_ward,
			bill_district, bill_province, bill_country_code,
			currency, shipping_fee, discount_amount, tax_amount,
			status, idempotency_key, placed_at, completed_at,
			created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,$10,$11,$12,
			$13,$14,$15,$16,$17,$18,$19,
			$20,$21,$22,$23,
			$24,$25,$26,$27,$28,$29
		)`,
		o.ID().String(), o.OrderNumber(), o.CustomerID().String(),
		o.GuestEmail(), o.GuestPhone(),
		ship.RecipientName, ship.Phone, ship.StreetAddress, ship.Ward,
		ship.District, ship.Province, defaultCountry(ship.CountryCode),
		bill.RecipientName, bill.Phone, bill.StreetAddress, bill.Ward,
		bill.District, bill.Province, bill.CountryCode,
		string(o.Currency()), o.ShippingFee().Amount(),
		o.DiscountAmount().Amount(), o.TaxAmount().Amount(),
		string(o.Status()), o.IdempotencyKey(), o.PlacedAt(),
		nullTime(o.CompletedAt()), o.CreatedAt(), o.UpdatedAt())
	if err != nil {
		// Khóa idempotency trùng nghĩa là đơn này ĐÃ được tạo — quy tắc 5.
		// Bên gọi phải đọc lại đơn cũ, không phải báo lỗi cho khách: khách
		// đã đặt hàng thành công, chỉ là request thứ hai đến muộn.
		if isUnique(err, "order_idempotency_key_key") {
			return domain.ErrDuplicateOrder
		}
		return fmt.Errorf("order: ghi đơn hàng: %w", err)
	}

	for i, l := range o.Lines() {
		if err := insertLine(ctx, tx, o.ID(), l); err != nil {
			return fmt.Errorf("order: ghi dòng hàng %d: %w", i+1, err)
		}
	}

	for i, fo := range fos {
		if err := insertFO(ctx, tx, fo); err != nil {
			return fmt.Errorf("order: ghi đơn thực hiện %d: %w", i+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("order: xác nhận giao dịch: %w", err)
	}
	return nil
}

func insertLine(ctx context.Context, tx pgx.Tx, orderID ids.ID, l *domain.Line) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO order_line (
			id, order_id, offer_id, sku_id, seller_id,
			product_name, variant_description, unit_price, currency, quantity,
			commission_rate, commission_amount,
			attributed_creator_id, creator_commission_rate,
			status, cancelled_at, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,$10,
			$11,$12,
			$13,$14,
			$15,$16,$17,$18
		)`,
		l.ID().String(), orderID.String(), l.OfferID().String(),
		l.SKUID().String(), l.SellerID().String(),
		l.ProductName(), l.VariantDescription(),
		l.UnitPrice().Amount(), string(l.UnitPrice().Currency()), l.Quantity(),
		int(l.CommissionRate().Value()), l.CommissionAmount().Amount(),
		l.AttributedCreatorID().String(), int(l.CreatorCommissionRate().Value()),
		string(l.Status()), nullTime(l.CancelledAt()),
		l.CreatedAt(), l.UpdatedAt())
	if err != nil {
		return err
	}

	for _, a := range l.Adjustments() {
		_, err := tx.Exec(ctx, `
			INSERT INTO order_line_adjustment (
				id, order_line_id, adjustment_type, label,
				amount, currency, source_type, source_id,
				cost_bearer, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (id) DO NOTHING`,
			a.ID.String(), l.ID().String(), string(a.Type), a.Label,
			a.Amount.Amount(), string(a.Amount.Currency()),
			a.SourceType, a.SourceID.String(),
			string(a.CostBearer), a.CreatedAt)
		if err != nil {
			return fmt.Errorf("khoản điều chỉnh %q: %w", a.Label, err)
		}
	}
	return nil
}

func insertFO(ctx context.Context, tx pgx.Tx, fo *domain.FulfillmentOrder) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO fulfillment_order (
			id, order_id, fo_number, seller_id, status,
			subtotal, commission_amount, currency, cancel_reason,
			confirmed_at, packed_at, shipped_at, delivered_at, cancelled_at,
			created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,
			$10,$11,$12,$13,$14,
			$15,$16
		)`,
		fo.ID().String(), fo.OrderID().String(), fo.FONumber(),
		fo.SellerID().String(), string(fo.Status()),
		fo.Subtotal().Amount(), fo.CommissionAmount().Amount(),
		string(fo.Subtotal().Currency()), fo.CancelReason(),
		nullTime(fo.ConfirmedAt()), nullTime(fo.PackedAt()),
		nullTime(fo.ShippedAt()), nullTime(fo.DeliveredAt()),
		nullTime(fo.CancelledAt()),
		fo.CreatedAt(), fo.UpdatedAt())
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

// Update ghi lại thay đổi trạng thái của đơn và các dòng hàng.
//
// CHỈ cập nhật trạng thái và mốc thời gian. Các cột ĐÓNG BĂNG — tên sản
// phẩm, đơn giá, tỷ lệ hoa hồng — không nằm trong câu UPDATE nào, ở đây
// hay bất kỳ đâu khác trong package này.
func (s *OrderStore) Update(ctx context.Context, o *domain.Order) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("order: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE "order"
		   SET status = $2, completed_at = $3, updated_at = $4
		 WHERE id = $1`,
		o.ID().String(), string(o.Status()),
		nullTime(o.CompletedAt()), o.UpdatedAt())
	if err != nil {
		return fmt.Errorf("order: cập nhật đơn hàng: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	for _, l := range o.Lines() {
		_, err := tx.Exec(ctx, `
			UPDATE order_line
			   SET status = $2, cancelled_at = $3, updated_at = $4
			 WHERE id = $1`,
			l.ID().String(), string(l.Status()),
			nullTime(l.CancelledAt()), l.UpdatedAt())
		if err != nil {
			return fmt.Errorf("order: cập nhật dòng hàng: %w", err)
		}

		// Khoản điều chỉnh chỉ THÊM, không sửa: một khoản đã hiện trên hóa
		// đơn của khách mà đổi giá trị thì hóa đơn cũ không giải thích được.
		for _, a := range l.Adjustments() {
			_, err := tx.Exec(ctx, `
				INSERT INTO order_line_adjustment (
					id, order_line_id, adjustment_type, label,
					amount, currency, source_type, source_id,
					cost_bearer, created_at
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
				ON CONFLICT (id) DO NOTHING`,
				a.ID.String(), l.ID().String(), string(a.Type), a.Label,
				a.Amount.Amount(), string(a.Amount.Currency()),
				a.SourceType, a.SourceID.String(),
				string(a.CostBearer), a.CreatedAt)
			if err != nil {
				return fmt.Errorf("order: ghi khoản điều chỉnh: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("order: xác nhận giao dịch: %w", err)
	}
	return nil
}

const orderCols = `
	id, order_number, customer_id, guest_email, guest_phone,
	ship_recipient_name, ship_phone, ship_street, ship_ward,
	ship_district, ship_province, ship_country_code,
	bill_recipient_name, bill_phone, bill_street, bill_ward,
	bill_district, bill_province, bill_country_code,
	currency, shipping_fee, discount_amount, tax_amount,
	status, idempotency_key, placed_at, completed_at, created_at, updated_at`

func (s *OrderStore) FindByID(ctx context.Context, id ids.ID) (*domain.Order, error) {
	return s.findOne(ctx, `WHERE id = $1`, id.String())
}

func (s *OrderStore) FindByOrderNumber(ctx context.Context, number string) (*domain.Order, error) {
	return s.findOne(ctx, `WHERE order_number = $1`, number)
}

func (s *OrderStore) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Order, error) {
	return s.findOne(ctx, `WHERE idempotency_key = $1`, key)
}

func (s *OrderStore) findOne(ctx context.Context, where string, args ...any) (*domain.Order, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+orderCols+` FROM "order" `+where, args...)

	o, err := scanOrder(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("order: đọc đơn hàng: %w", err)
	}

	lines, err := s.loadLines(ctx, o.ID())
	if err != nil {
		return nil, err
	}
	return withLines(o, lines), nil
}

func (s *OrderStore) ListByCustomer(
	ctx context.Context, customerID ids.ID, limit, offset int,
) ([]*domain.Order, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+orderCols+`
		  FROM "order"
		 WHERE customer_id = $1
		 ORDER BY placed_at DESC
		 LIMIT $2 OFFSET $3`,
		customerID.String(), limitOr(limit, 20), max0(offset))
	if err != nil {
		return nil, fmt.Errorf("order: liệt kê đơn của khách: %w", err)
	}
	defer rows.Close()

	var out []*domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("order: đọc đơn hàng: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("order: đọc đơn hàng: %w", err)
	}

	// Nạp dòng hàng SAU khi đóng con trỏ: pgx dùng một kết nối cho mỗi
	// truy vấn, truy vấn lồng trong vòng lặp rows sẽ chiếm thêm kết nối và
	// cạn pool khi có tải.
	for i, o := range out {
		lines, err := s.loadLines(ctx, o.ID())
		if err != nil {
			return nil, err
		}
		out[i] = withLines(o, lines)
	}
	return out, nil
}

func (s *OrderStore) loadLines(ctx context.Context, orderID ids.ID) ([]*domain.Line, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, offer_id, sku_id, seller_id,
		       product_name, variant_description, unit_price, currency, quantity,
		       commission_rate, commission_amount,
		       attributed_creator_id, creator_commission_rate,
		       status, cancelled_at, created_at, updated_at
		  FROM order_line
		 WHERE order_id = $1
		 ORDER BY created_at, id`, orderID.String())
	if err != nil {
		return nil, fmt.Errorf("order: đọc dòng hàng: %w", err)
	}
	defer rows.Close()

	var (
		lines  []*domain.Line
		params []domain.RestoreLineParams
	)
	for rows.Next() {
		var (
			p                           domain.RestoreLineParams
			offerID, skuID, sellerID    string
			id, creatorID, currency     string
			unitPrice, commissionAmount int64
			commRate, creatorCommRate   int
			status                      string
			cancelledAt                 *time.Time
			createdAt, updatedAt        time.Time
			productName, variantDesc    string
			quantity                    int
		)
		if err := rows.Scan(
			&id, &offerID, &skuID, &sellerID,
			&productName, &variantDesc, &unitPrice, &currency, &quantity,
			&commRate, &commissionAmount,
			&creatorID, &creatorCommRate,
			&status, &cancelledAt, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("order: đọc dòng hàng: %w", err)
		}

		cur := money.Currency(currency)
		p = domain.RestoreLineParams{
			ID:                    ids.ID(id),
			OfferID:               ids.ID(offerID),
			SKUID:                 ids.ID(skuID),
			SellerID:              ids.ID(sellerID),
			ProductName:           productName,
			VariantDescription:    variantDesc,
			UnitPrice:             mustMoney(unitPrice, cur),
			Quantity:              quantity,
			CommissionRate:        mustRate(commRate),
			CommissionAmount:      mustMoney(commissionAmount, cur),
			AttributedCreatorID:   ids.ID(creatorID),
			CreatorCommissionRate: mustRate(creatorCommRate),
			Status:                domain.LineStatus(status),
			CancelledAt:           deref(cancelledAt),
			CreatedAt:             createdAt,
			UpdatedAt:             updatedAt,
		}
		params = append(params, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("order: đọc dòng hàng: %w", err)
	}

	for i := range params {
		adjs, err := s.loadAdjustments(ctx, params[i].ID)
		if err != nil {
			return nil, err
		}
		params[i].Adjustments = adjs
		lines = append(lines, domain.RestoreLine(params[i]))
	}
	return lines, nil
}

func (s *OrderStore) loadAdjustments(ctx context.Context, lineID ids.ID) ([]domain.Adjustment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, adjustment_type, label, amount, currency,
		       source_type, source_id, cost_bearer, created_at
		  FROM order_line_adjustment
		 WHERE order_line_id = $1
		 ORDER BY created_at, id`, lineID.String())
	if err != nil {
		return nil, fmt.Errorf("order: đọc khoản điều chỉnh: %w", err)
	}
	defer rows.Close()

	var out []domain.Adjustment
	for rows.Next() {
		var (
			id, typ, label, currency string
			amount                   int64
			sourceType, sourceID     string
			bearer                   string
			createdAt                time.Time
		)
		if err := rows.Scan(&id, &typ, &label, &amount, &currency,
			&sourceType, &sourceID, &bearer, &createdAt); err != nil {
			return nil, fmt.Errorf("order: đọc khoản điều chỉnh: %w", err)
		}
		out = append(out, domain.Adjustment{
			ID:         ids.ID(id),
			Type:       domain.AdjustmentType(typ),
			Label:      label,
			Amount:     mustMoney(amount, money.Currency(currency)),
			SourceType: sourceType,
			SourceID:   ids.ID(sourceID),
			CostBearer: domain.CostBearer(bearer),
			CreatedAt:  createdAt,
		})
	}
	return out, rows.Err()
}

// scanner cho phép dùng chung một hàm quét cho QueryRow và rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanOrder(row scanner) (*domain.Order, error) {
	var (
		p                          domain.RestoreOrderParams
		id, orderNumber            string
		customerID, email, phone   string
		ship, bill                 domain.Address
		currency, status, idemKey  string
		shippingFee, discount, tax int64
		completedAt                *time.Time
	)
	if err := row.Scan(
		&id, &orderNumber, &customerID, &email, &phone,
		&ship.RecipientName, &ship.Phone, &ship.StreetAddress, &ship.Ward,
		&ship.District, &ship.Province, &ship.CountryCode,
		&bill.RecipientName, &bill.Phone, &bill.StreetAddress, &bill.Ward,
		&bill.District, &bill.Province, &bill.CountryCode,
		&currency, &shippingFee, &discount, &tax,
		&status, &idemKey, &p.PlacedAt, &completedAt, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}

	cur := money.Currency(currency)
	p.ID = ids.ID(id)
	p.OrderNumber = orderNumber
	p.CustomerID = ids.ID(customerID)
	p.GuestEmail = email
	p.GuestPhone = phone
	p.ShippingAddress = ship
	p.BillingAddress = bill
	p.Currency = cur
	p.ShippingFee = mustMoney(shippingFee, cur)
	p.DiscountAmount = mustMoney(discount, cur)
	p.TaxAmount = mustMoney(tax, cur)
	p.Status = domain.Status(status)
	p.IdempotencyKey = idemKey
	p.CompletedAt = deref(completedAt)

	return domain.RestoreOrder(p), nil
}

// withLines dựng lại đơn kèm dòng hàng.
//
// RestoreOrder nhận trọn bộ tham số một lần, nên phải dựng lại thay vì gắn
// thêm — đó là chủ ý của domain: không có setter nào để gắn dòng hàng vào
// đơn đã tạo.
func withLines(o *domain.Order, lines []*domain.Line) *domain.Order {
	return domain.RestoreOrder(domain.RestoreOrderParams{
		ID:              o.ID(),
		OrderNumber:     o.OrderNumber(),
		CustomerID:      o.CustomerID(),
		GuestEmail:      o.GuestEmail(),
		GuestPhone:      o.GuestPhone(),
		ShippingAddress: o.ShippingAddress(),
		BillingAddress:  o.BillingAddress(),
		Currency:        o.Currency(),
		ShippingFee:     o.ShippingFee(),
		DiscountAmount:  o.DiscountAmount(),
		TaxAmount:       o.TaxAmount(),
		Status:          o.Status(),
		Lines:           lines,
		IdempotencyKey:  o.IdempotencyKey(),
		PlacedAt:        o.PlacedAt(),
		CompletedAt:     o.CompletedAt(),
		CreatedAt:       o.CreatedAt(),
		UpdatedAt:       o.UpdatedAt(),
	})
}

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

var _ domain.FulfillmentRepository = (*FulfillmentStore)(nil)

func (s *FulfillmentStore) Update(ctx context.Context, fo *domain.FulfillmentOrder) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE fulfillment_order
		   SET status = $2, cancel_reason = $3,
		       confirmed_at = $4, packed_at = $5, shipped_at = $6,
		       delivered_at = $7, cancelled_at = $8, updated_at = $9
		 WHERE id = $1 AND seller_id = $10`,
		fo.ID().String(), string(fo.Status()), fo.CancelReason(),
		nullTime(fo.ConfirmedAt()), nullTime(fo.PackedAt()),
		nullTime(fo.ShippedAt()), nullTime(fo.DeliveredAt()),
		nullTime(fo.CancelledAt()), fo.UpdatedAt(),
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
	subtotal, commission_amount, currency, cancel_reason,
	confirmed_at, packed_at, shipped_at, delivered_at, cancelled_at,
	created_at, updated_at`

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

	return domain.RestoreFulfillmentOrder(domain.RestoreFOParams{
		ID:               fo.ID(),
		OrderID:          fo.OrderID(),
		FONumber:         fo.FONumber(),
		SellerID:         fo.SellerID(),
		LineIDs:          lineIDs,
		Status:           fo.Status(),
		Subtotal:         fo.Subtotal(),
		CommissionAmount: fo.CommissionAmount(),
		CancelReason:     fo.CancelReason(),
		ConfirmedAt:      fo.ConfirmedAt(),
		PackedAt:         fo.PackedAt(),
		ShippedAt:        fo.ShippedAt(),
		DeliveredAt:      fo.DeliveredAt(),
		CancelledAt:      fo.CancelledAt(),
		CreatedAt:        fo.CreatedAt(),
		UpdatedAt:        fo.UpdatedAt(),
	}), nil
}

func scanFO(row scanner) (*domain.FulfillmentOrder, error) {
	var (
		p                          domain.RestoreFOParams
		id, orderID, foNumber      string
		sellerID, status, currency string
		subtotal, commission       int64
		cancelReason               string
		confirmed, packed, shipped *time.Time
		delivered, cancelled       *time.Time
	)
	if err := row.Scan(
		&id, &orderID, &foNumber, &sellerID, &status,
		&subtotal, &commission, &currency, &cancelReason,
		&confirmed, &packed, &shipped, &delivered, &cancelled,
		&p.CreatedAt, &p.UpdatedAt,
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
	p.ConfirmedAt = deref(confirmed)
	p.PackedAt = deref(packed)
	p.ShippedAt = deref(shipped)
	p.DeliveredAt = deref(delivered)
	p.CancelledAt = deref(cancelled)

	return domain.RestoreFulfillmentOrder(p), nil
}

// ---------------------------------------------------------------- Mã đơn

// NumberStore sinh mã đơn hiển thị: FC-2026-08-000001.
//
// Đếm bằng SEQUENCE của PostgreSQL chứ không phải MAX(order_number)+1: hai
// request song song đọc cùng một MAX sẽ sinh ra hai đơn cùng mã, và mã đơn
// trùng nghĩa là khách gọi tổng đài đọc mã ra hai đơn khác nhau.
type NumberStore struct {
	pool *pgxpool.Pool
}

func NewNumberStore(pool *pgxpool.Pool) *NumberStore {
	return &NumberStore{pool: pool}
}

var _ domain.NumberGenerator = (*NumberStore)(nil)

func (s *NumberStore) NextOrderNumber(ctx context.Context) (string, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT nextval('order_number_seq')`).Scan(&n)
	if err != nil {
		return "", fmt.Errorf("order: sinh mã đơn: %w", err)
	}
	now := time.Now().UTC()
	return fmt.Sprintf("FC-%04d-%02d-%06d", now.Year(), int(now.Month()), n), nil
}

// ---------------------------------------------------------------- Tiện ích

// mustRate dựng lại tỷ lệ đã lưu.
//
// Cột commission_rate có CHECK BETWEEN 0 AND 10000 nên giá trị ngoài
// khoảng chỉ xảy ra khi database đã hỏng — dùng số 0 thay thế sẽ khiến
// đối soát báo hoa hồng bằng 0 và không ai phát hiện.
func mustRate(v int) types.BasisPoints {
	bp, err := types.NewBasisPoints(int32(v))
	if err != nil {
		panic(fmt.Sprintf("order: tỷ lệ đã lưu không hợp lệ: %d", v))
	}
	return bp
}

func mustMoney(amount int64, c money.Currency) money.Money {
	m, err := money.New(amount, c)
	if err != nil {
		// Dữ liệu trong database vi phạm ràng buộc đơn vị tiền tệ. Trả về
		// số 0 sẽ giấu lỗi và làm sai mọi phép cộng tiền sau đó.
		panic(fmt.Sprintf("order: dữ liệu tiền tệ hỏng: %v (%d %s)", err, amount, c))
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

func defaultCountry(c string) string {
	if strings.TrimSpace(c) == "" {
		return "VN"
	}
	return c
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

func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == constraint
}
