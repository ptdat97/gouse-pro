// Package postgres cài đặt kho lưu trữ của module affiliate bằng PostgreSQL.
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
	"github.com/fashion-commerce/platform/internal/modules/affiliate/domain"
)

// Store ghi và đọc quy công cùng khoản hoa hồng.
//
// CHỈ chạm hai bảng `attribution` và `commission_record` — cả hai do module
// này sở hữu (R6 của `cmd/archcheck` cưỡng chế điều đó).
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

var _ domain.Repo = (*Store)(nil)

// Luu ghi bản quy công và khoản hoa hồng trong MỘT giao dịch.
//
// Một bản quy công không kèm khoản tiền là một lời hứa không có số; một
// khoản tiền không kèm bản quy công là một số không giải thích được. Nửa
// này thành công mà nửa kia thất bại là trạng thái không được phép tồn tại.
func (s *Store) Save(
	ctx context.Context, a *domain.Attribution, c *domain.CommissionRecord,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("affiliate: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO attribution (
			id, order_id, order_line_id, seller_id, creator_id, content_id,
			commission_rate, commission_base, currency,
			cost_bearer, attribution_model, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ID().String(), a.OrderID().String(), a.OrderLineID().String(),
		a.SellerID().String(), a.CreatorID().String(), a.ContentID().String(),
		int(a.Rate().Value()), a.Base().Amount(), string(a.Base().Currency()),
		string(a.CostBearer()), string(a.Model()), a.CreatedAt())
	if err != nil {
		// UNIQUE (order_line_id): dòng này đã được quy công.
		//
		// CHẶN Ở ĐÂY, không bằng một lượt đọc trước: `order.paid` giao ít
		// nhất một lần, và hai lượt worker chồng nhau đều thấy "chưa quy
		// công" nếu chỉ kiểm ở tầng ứng dụng. Kết quả sẽ là creator nhận
		// hoa hồng gấp đôi.
		if isDuplicateKey(err) {
			return domain.ErrAlreadyAttributed
		}
		return fmt.Errorf("affiliate: ghi quy công: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO commission_record (
			id, attribution_id, creator_id, amount, currency, status,
			reversed_reason, reversed_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		c.ID().String(), c.AttributionID().String(), c.CreatorID().String(),
		c.Amount().Amount(), string(c.Amount().Currency()), string(c.Status()),
		c.ReversedReason(), nullTime(c.ReversedAt()), c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		if isDuplicateKey(err) {
			return domain.ErrAlreadyAttributed
		}
		return fmt.Errorf("affiliate: ghi khoản hoa hồng: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("affiliate: chốt giao dịch: %w", err)
	}
	return nil
}

// commissionColumns là danh sách cột dùng cho mọi truy vấn đọc khoản hoa hồng.
//
// Một chuỗi dùng lại thay vì gõ lại ở từng truy vấn: thêm một cột mà quên
// một chỗ đọc là lỗi `scanCommission` không bắt được — thứ tự vẫn khớp, chỉ
// thiếu dữ liệu.
const commissionColumns = `c.id, c.attribution_id, c.creator_id, c.amount,
	c.currency, c.status, c.reversed_reason, c.reversed_at,
	c.created_at, c.updated_at`

// ByOrderAndSeller trả các khoản hoa hồng của một đơn.
//
// NỐI qua `attribution` vì `commission_record` không giữ mã đơn: mã đơn là
// thuộc tính của việc QUY CÔNG, và chép nó sang bảng trạng thái sẽ là hai
// chỗ phải cùng đúng.
func (s *Store) ByOrderAndSeller(
	ctx context.Context, orderID, sellerID ids.ID,
) ([]*domain.CommissionRecord, error) {
	// `$2 = ''` chọn MỌI nhà bán. Một truy vấn thay vì hai nhánh chuỗi SQL
	// nối tay — chuỗi nối tay là chỗ lỗ SQL injection hay xuất hiện.
	rows, err := s.pool.Query(ctx, `
		SELECT `+commissionColumns+`
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_id = $1
		   AND ($2 = '' OR a.seller_id = $2)
		 ORDER BY c.created_at`,
		orderID.String(), sellerID.String())
	if err != nil {
		return nil, fmt.Errorf("affiliate: đọc hoa hồng theo đơn: %w", err)
	}
	defer rows.Close()

	return scanMany(rows)
}

// CapNhat ghi lại trạng thái đã đổi.
//
// KHÔNG sửa `amount`: số tiền đến từ bản quy công bất biến, nên một câu
// UPDATE chạm được vào nó là một đường sửa tiền đã hứa. Cột duy nhất có
// nghĩa ở đây là trạng thái và lý do đảo.
func (s *Store) Update(ctx context.Context, c *domain.CommissionRecord) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE commission_record
		   SET status = $2, reversed_reason = $3, reversed_at = $4,
		       updated_at = $5
		 WHERE id = $1`,
		c.ID().String(), string(c.Status()), c.ReversedReason(),
		nullTime(c.ReversedAt()), c.UpdatedAt())
	if err != nil {
		return fmt.Errorf("affiliate: cập nhật hoa hồng: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) TotalsByCreator(
	ctx context.Context, creatorID ids.ID,
) (map[domain.CommissionStatus]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT status, COALESCE(SUM(amount), 0)
		  FROM commission_record
		 WHERE creator_id = $1
		 GROUP BY status`,
		creatorID.String())
	if err != nil {
		return nil, fmt.Errorf("affiliate: tổng hoa hồng theo creator: %w", err)
	}
	defer rows.Close()

	// MỌI trạng thái có mặt, kể cả khi bằng 0.
	//
	// Bên đọc hiện "Chờ đến hạn: —" khi khóa không tồn tại và "0 ₫" khi có;
	// hai cách hiện cho cùng một sự thật là chỗ để nghi ngờ dữ liệu.
	out := map[domain.CommissionStatus]int64{
		domain.CommissionPending:   0,
		domain.CommissionAvailable: 0,
		domain.CommissionReversed:  0,
	}
	for rows.Next() {
		var (
			st  string
			sum int64
		)
		if err := rows.Scan(&st, &sum); err != nil {
			return nil, fmt.Errorf("affiliate: đọc tổng hoa hồng: %w", err)
		}
		out[domain.CommissionStatus(st)] = sum
	}
	return out, rows.Err()
}

func scanMany(rows pgx.Rows) ([]*domain.CommissionRecord, error) {
	var out []*domain.CommissionRecord
	for rows.Next() {
		c, err := scanCommission(rows)
		if err != nil {
			return nil, fmt.Errorf("affiliate: đọc khoản hoa hồng: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanCommission(row interface{ Scan(...any) error }) (*domain.CommissionRecord, error) {
	var (
		id, attrID, creatorID string
		amount                int64
		currency, status      string
		reversedReason        string
		reversedAt            *time.Time
		createdAt, updatedAt  time.Time
	)
	if err := row.Scan(
		&id, &attrID, &creatorID, &amount, &currency, &status,
		&reversedReason, &reversedAt, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}

	p := domain.RestoreCommissionParams{
		ID:             ids.ID(id),
		AttributionID:  ids.ID(attrID),
		CreatorID:      ids.ID(creatorID),
		Amount:         money.MustNew(amount, money.Currency(currency)),
		Status:         domain.CommissionStatus(status),
		ReversedReason: reversedReason,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
	if reversedAt != nil {
		p.ReversedAt = *reversedAt
	}
	return domain.RestoreCommission(p), nil
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func isDuplicateKey(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505"
}

// ByOrderLines trả các khoản hoa hồng của một tập dòng đơn.
//
// Tập RỖNG trả danh sách rỗng mà không chạm database. Nếu để `= ANY('{}')`
// đi xuống thì PostgreSQL trả 0 dòng — đúng kết quả — nhưng một lần sửa câu
// SQL sau này làm mất điều kiện ấy sẽ đảo hoa hồng của toàn hệ thống. Chặn ở
// Go là chặn ở chỗ đọc được.
func (s *Store) ByOrderLines(
	ctx context.Context, orderLineIDs []ids.ID,
) ([]*domain.CommissionRecord, error) {
	if len(orderLineIDs) == 0 {
		return nil, nil
	}

	lineIDs := make([]string, 0, len(orderLineIDs))
	for _, id := range orderLineIDs {
		if id.IsZero() {
			continue
		}
		lineIDs = append(lineIDs, id.String())
	}
	if len(lineIDs) == 0 {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+commissionColumns+`
		  FROM commission_record c
		  JOIN attribution a ON a.id = c.attribution_id
		 WHERE a.order_line_id = ANY($1)
		 ORDER BY c.created_at`, lineIDs)
	if err != nil {
		return nil, fmt.Errorf("affiliate: đọc hoa hồng theo dòng đơn: %w", err)
	}
	defer rows.Close()

	return scanMany(rows)
}
