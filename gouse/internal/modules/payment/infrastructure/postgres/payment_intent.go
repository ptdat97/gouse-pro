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
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

// IntentStore lưu ý định thanh toán.
type IntentStore struct {
	pool *pgxpool.Pool
}

func NewIntentStore(pool *pgxpool.Pool) *IntentStore {
	return &IntentStore{pool: pool}
}

var _ domain.IntentRepository = (*IntentStore)(nil)

const intentCols = ` id, order_id, amount, currency, payment_method, status,
	provider, provider_intent_id, failure_reason,
	captured_at, created_at, updated_at`

func (s *IntentStore) Create(ctx context.Context, p *domain.PaymentIntent) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO payment_intent (
			id, order_id, amount, currency, payment_method, status,
			provider, provider_intent_id, failure_reason,
			captured_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		p.ID().String(), p.OrderID().String(),
		p.Amount().Amount(), string(p.Amount().Currency()),
		p.PhuongThuc(), string(p.Status()),
		p.NhaCungCap(), p.MaNhaCungCap(), p.LyDoThatBai(),
		// nullTime: ràng buộc `payment_intent_captured_co_moc` đòi
		// captured_at NULL khi chưa thu, nên giá trị zero của Go phải
		// thành NULL chứ không phải mốc năm 1.
		nullTime(p.CapturedAt()), p.CreatedAt(), p.UpdatedAt())
	if err != nil {
		// Chỉ mục UNIQUE trên order_id là thứ chặn hai intent cho một đơn,
		// không phải một câu SELECT trước đó: hai request hoàn tất song
		// song đều thấy "chưa có". Hai intent nghĩa là thu tiền hai lần.
		if laTrungKhoa(err, "payment_intent_order_id_key") {
			return domain.ErrIntentTrungDon
		}
		return fmt.Errorf("payment: ghi ý định thanh toán: %w", err)
	}
	return nil
}

func (s *IntentStore) FindByOrder(
	ctx context.Context, orderID ids.ID,
) (*domain.PaymentIntent, error) {
	return s.motDong(ctx, `WHERE order_id = $1`, orderID.String())
}

func (s *IntentStore) FindByID(
	ctx context.Context, id ids.ID,
) (*domain.PaymentIntent, error) {
	return s.motDong(ctx, `WHERE id = $1`, id.String())
}

func (s *IntentStore) Update(ctx context.Context, p *domain.PaymentIntent) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE payment_intent
		   SET status = $2, provider = $3, provider_intent_id = $4,
		       failure_reason = $5, captured_at = $6, updated_at = $7
		 WHERE id = $1`,
		p.ID().String(), string(p.Status()), p.NhaCungCap(), p.MaNhaCungCap(),
		p.LyDoThatBai(), nullTime(p.CapturedAt()), p.UpdatedAt())
	if err != nil {
		return fmt.Errorf("payment: cập nhật ý định thanh toán: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrIntentKhongTim
	}
	return nil
}

func (s *IntentStore) motDong(
	ctx context.Context, where string, args ...any,
) (*domain.PaymentIntent, error) {
	return quetIntent(s.pool.QueryRow(ctx,
		`SELECT`+intentCols+` FROM payment_intent `+where, args...))
}

// quetIntent đọc MỘT dòng thành aggregate.
//
// Dùng chung cho lời gọi một dòng và lời gọi theo lô: hai bản chép đôi sẽ
// lệch nhau ngay lần thêm cột tiếp theo, và lệch theo kiểu im lặng.
func quetIntent(row interface {
	Scan(dest ...any) error
}) (*domain.PaymentIntent, error) {
	var (
		id, orderID, curr     string
		phuongThuc, trangThai string
		nhaCC, maNhaCC, lyDo  string
		soTien                int64
		capturedAt            *time.Time
		createdAt, updatedAt  time.Time
	)
	if err := row.Scan(&id, &orderID, &soTien, &curr, &phuongThuc, &trangThai,
		&nhaCC, &maNhaCC, &lyDo, &capturedAt, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrIntentKhongTim
		}
		return nil, fmt.Errorf("payment: đọc ý định thanh toán: %w", err)
	}

	m, err := money.New(soTien, money.Currency(curr))
	if err != nil {
		return nil, fmt.Errorf("payment: số tiền của intent %s: %w", id, err)
	}

	var thoiDiemThu time.Time
	if capturedAt != nil {
		thoiDiemThu = *capturedAt
	}

	return domain.RestoreIntent(domain.RestoreIntentParams{
		ID:           ids.ID(id),
		OrderID:      ids.ID(orderID),
		Amount:       m,
		PhuongThuc:   phuongThuc,
		Status:       domain.TrangThaiIntent(trangThai),
		NhaCungCap:   nhaCC,
		MaNhaCungCap: maNhaCC,
		LyDoThatBai:  lyDo,
		CapturedAt:   thoiDiemThu,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}), nil
}

func laTrungKhoa(err error, tenRangBuoc string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, tenRangBuoc)
}

func (s *IntentStore) DaThuTuMoc(
	ctx context.Context, moc time.Time, limit int,
) ([]*domain.PaymentIntent, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx,
		`SELECT`+intentCols+`
		   FROM payment_intent
		  WHERE status = 'CAPTURED' AND captured_at >= $1
		  ORDER BY captured_at
		  LIMIT $2`, moc, limit)
	if err != nil {
		return nil, fmt.Errorf("payment: đọc intent đã thu: %w", err)
	}
	defer rows.Close()

	var out []*domain.PaymentIntent
	for rows.Next() {
		p, err := quetIntent(rows)
		if err != nil {
			return nil, fmt.Errorf("payment: đọc intent đã thu: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *IntentStore) DemChoThuQuaHan(
	ctx context.Context, truoc time.Time,
) (int, error) {
	var n int
	// Chỉ mục `payment_intent_cho_thu` phục vụ thẳng truy vấn này.
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM payment_intent
		 WHERE status = 'REQUIRES_PAYMENT' AND created_at < $1`,
		truoc).Scan(&n); err != nil {
		return 0, fmt.Errorf("payment: đếm intent chờ thu quá hạn: %w", err)
	}
	return n, nil
}
