// Package postgres cài đặt kho lưu trữ yêu cầu trả hàng.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
)

// Store lưu yêu cầu trả hàng.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const cols = `
	id, order_id, seller_id, customer_id, status, reason_code,
	customer_note, reject_reason, refund_amount, refund_currency,
	requested_at, decided_at, received_at, refunded_at,
	version, created_at, updated_at`

// Luu ghi yêu cầu VÀ các dòng của nó trong MỘT giao dịch.
//
// Yêu cầu có dòng mà dòng không ghi được là một yêu cầu hoàn 0 đồng — vừa
// vô nghĩa vừa làm khách tưởng đã gửi thành công.
//
// Khóa lạc quan theo `version`: nhà bán duyệt trong lúc khách hủy là ca có
// thật, và ai ghi sau sẽ xóa quyết định của người kia nếu không có ràng buộc.
func (s *Store) Luu(ctx context.Context, y *domain.YeuCauTraHang) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("returns: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		INSERT INTO return_request (`+cols+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (id) DO UPDATE SET
			status        = EXCLUDED.status,
			reject_reason = EXCLUDED.reject_reason,
			decided_at    = EXCLUDED.decided_at,
			received_at   = EXCLUDED.received_at,
			refunded_at   = EXCLUDED.refunded_at,
			updated_at    = EXCLUDED.updated_at,
			version       = return_request.version + 1
		WHERE return_request.version = $15`,
		y.ID().String(), y.OrderID().String(), y.SellerID().String(),
		y.CustomerID().String(), string(y.Status()), string(y.LyDo()),
		y.GhiChu(), y.LyDoTuChoi(),
		y.TienHoan().Amount(), string(y.TienHoan().Currency()),
		y.RequestedAt(), nullTime(y.DecidedAt()), nullTime(y.ReceivedAt()),
		nullTime(y.RefundedAt()),
		y.Version(), y.CreatedAt(), y.UpdatedAt())
	if err != nil {
		return fmt.Errorf("returns: ghi yêu cầu: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrVersionConflict
	}

	// Dòng hàng chỉ ghi một lần, lúc tạo: chúng KHÔNG đổi sau đó.
	for _, d := range y.Dong() {
		if _, err := tx.Exec(ctx, `
			INSERT INTO return_line (
				id, return_request_id, order_line_id, sku_id,
				quantity, line_refund, line_currency,
				reason_code, reason_detail
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (id) DO NOTHING`,
			d.ID.String(), y.ID().String(), d.OrderLineID.String(),
			d.SKUID.String(), d.Quantity,
			d.TienHoan.Amount(), string(d.TienHoan.Currency()),
			string(d.LyDo), d.ChiTiet); err != nil {
			return fmt.Errorf("returns: ghi dòng trả hàng: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("returns: xác nhận giao dịch: %w", err)
	}
	return nil
}

func (s *Store) TimTheoID(ctx context.Context, id ids.ID) (*domain.YeuCauTraHang, error) {
	y, err := s.doc(ctx, `WHERE id = $1`, id.String())
	if err != nil {
		return nil, err
	}
	if len(y) == 0 {
		return nil, domain.ErrNotFound
	}
	return y[0], nil
}

func (s *Store) TimTheoDon(ctx context.Context, orderID ids.ID) ([]*domain.YeuCauTraHang, error) {
	return s.doc(ctx, `WHERE order_id = $1 ORDER BY requested_at DESC`, orderID.String())
}

func (s *Store) TimTheoNhaBan(
	ctx context.Context, sellerID ids.ID, status string, limit int,
) ([]*domain.YeuCauTraHang, error) {
	if status == "" {
		return s.doc(ctx,
			`WHERE seller_id = $1 ORDER BY requested_at DESC LIMIT $2`,
			sellerID.String(), limit)
	}
	return s.doc(ctx,
		`WHERE seller_id = $1 AND status = $2 ORDER BY requested_at DESC LIMIT $3`,
		sellerID.String(), status, limit)
}

// DongDaXinTra đếm số lượng đã xin trả theo từng dòng hàng của đơn.
//
// CHỈ tính các yêu cầu còn hiệu lực: bị từ chối hoặc đã hủy thì không giữ
// chỗ nữa, và khách phải xin lại được.
func (s *Store) DongDaXinTra(ctx context.Context, orderID ids.ID) (map[ids.ID]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.order_line_id, sum(l.quantity)
		  FROM return_line l
		  JOIN return_request r ON r.id = l.return_request_id
		 WHERE r.order_id = $1
		   AND r.status NOT IN ('REJECTED', 'CANCELLED')
		 GROUP BY l.order_line_id`, orderID.String())
	if err != nil {
		return nil, fmt.Errorf("returns: đếm dòng đã xin trả: %w", err)
	}
	defer rows.Close()

	out := map[ids.ID]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[ids.ID(id)] = n
	}
	return out, rows.Err()
}

func (s *Store) doc(
	ctx context.Context, where string, args ...any,
) ([]*domain.YeuCauTraHang, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+cols+` FROM return_request `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("returns: đọc yêu cầu: %w", err)
	}
	defer rows.Close()

	var out []*domain.YeuCauTraHang
	var ma []string
	theoID := map[ids.ID]*domain.KhoiPhucParams{}

	for rows.Next() {
		var p domain.KhoiPhucParams
		var id, orderID, sellerID, customerID, status, lyDo, tienTe string
		var soTien int64
		var decided, received, refunded *time.Time

		if err := rows.Scan(&id, &orderID, &sellerID, &customerID, &status,
			&lyDo, &p.GhiChu, &p.LyDoTuChoi, &soTien, &tienTe,
			&p.RequestedAt, &decided, &received, &refunded,
			&p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}

		tien, err := money.New(soTien, money.Currency(tienTe))
		if err != nil {
			return nil, fmt.Errorf("returns: số tiền hỏng ở bản ghi %s: %w", id, err)
		}

		p.ID = ids.ID(id)
		p.OrderID = ids.ID(orderID)
		p.SellerID = ids.ID(sellerID)
		p.CustomerID = ids.ID(customerID)
		p.Status = domain.TrangThai(status)
		p.LyDo = domain.LyDo(lyDo)
		p.TienHoan = tien
		p.DecidedAt = deref(decided)
		p.ReceivedAt = deref(received)
		p.RefundedAt = deref(refunded)

		theoID[p.ID] = &p
		ma = append(ma, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ma) == 0 {
		return nil, nil
	}

	// Nạp dòng hàng theo LÔ, không phải từng cái: danh sách yêu cầu của
	// một gian hàng bận có hàng chục bản ghi.
	lrows, err := s.pool.Query(ctx, `
		SELECT return_request_id, id, order_line_id, sku_id,
		       quantity, line_refund, line_currency, reason_code, reason_detail
		  FROM return_line WHERE return_request_id = ANY($1)
		 ORDER BY created_at, id`, ma)
	if err != nil {
		return nil, fmt.Errorf("returns: đọc dòng trả hàng: %w", err)
	}
	defer lrows.Close()

	for lrows.Next() {
		var reqID, id, lineID, skuID, tienTe, lyDo, chiTiet string
		var qty int
		var soTien int64
		if err := lrows.Scan(&reqID, &id, &lineID, &skuID, &qty, &soTien,
			&tienTe, &lyDo, &chiTiet); err != nil {
			return nil, err
		}
		tien, err := money.New(soTien, money.Currency(tienTe))
		if err != nil {
			return nil, err
		}
		if p := theoID[ids.ID(reqID)]; p != nil {
			p.Dong = append(p.Dong, domain.Dong{
				ID: ids.ID(id), OrderLineID: ids.ID(lineID),
				SKUID: ids.ID(skuID), Quantity: qty, TienHoan: tien,
				LyDo: domain.LyDo(lyDo), ChiTiet: chiTiet,
			})
		}
	}
	if err := lrows.Err(); err != nil {
		return nil, err
	}

	for _, id := range ma {
		out = append(out, domain.KhoiPhuc(*theoID[ids.ID(id)]))
	}
	return out, nil
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
