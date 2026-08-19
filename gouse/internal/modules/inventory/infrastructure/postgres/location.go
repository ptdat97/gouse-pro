package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
)

// LocationStore lưu kho hàng.
type LocationStore struct{ db querier }

func NewLocationStore(db querier) *LocationStore {
	return &LocationStore{db: db}
}

var _ domain.LocationRepository = (*LocationStore)(nil)

const locationCols = `id, name, code, kind, is_active, created_at, updated_at`

// EnsureByCode tạo kho, hoặc trả kho đã có cùng mã.
//
// Dùng ON CONFLICT chứ không phải "đọc rồi ghi": hai tiến trình cùng khởi
// động (API và worker) sẽ cùng chạy hàm này, và khoảng trống giữa đọc và
// ghi đủ để cả hai cùng tưởng mình phải tạo mới.
func (s *LocationStore) EnsureByCode(
	ctx context.Context, l *domain.StockLocation,
) (*domain.StockLocation, error) {
	const q = `
		INSERT INTO stock_location (` + locationCols + `)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (code) DO NOTHING`

	_, err := s.db.Exec(ctx, q,
		l.ID.String(), l.Name, l.Code, string(l.Kind), l.IsActive,
		l.CreatedAt, l.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("inventory: ghi kho hàng: %w", err)
	}

	// Đọc lại LUÔN, kể cả khi vừa ghi thành công: khi ON CONFLICT bỏ qua,
	// kho thật là kho CŨ và định danh khác cái vừa sinh ra. Trả về `l` sẽ
	// đưa cho bên gọi một định danh không tồn tại trong database.
	return s.FindByCode(ctx, l.Code)
}

func (s *LocationStore) FindByCode(
	ctx context.Context, code string,
) (*domain.StockLocation, error) {
	row := s.db.QueryRow(ctx,
		`SELECT `+locationCols+` FROM stock_location WHERE code = $1`, code)

	var (
		l              domain.StockLocation
		id, kind, name string
	)
	err := row.Scan(&id, &name, &l.Code, &kind, &l.IsActive,
		&l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inventory: đọc kho hàng: %w", err)
	}

	l.ID = ids.ID(id)
	l.Name = name
	l.Kind = domain.LocationKind(kind)
	return &l, nil
}
