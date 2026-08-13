// Package postgres cài đặt port của seller bằng PostgreSQL.
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
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/seller/domain"
)

// Store lưu nhà bán trong PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const cols = `
	id, name, slug, seller_type, status, legal_name, tax_code, email, phone,
	commission_rate, bank_account_verified, suspension_reason,
	approved_by, approved_at, created_at, updated_at`

func scan(row pgx.Row) (*domain.Seller, error) {
	var (
		p                      domain.RestoreSellerParams
		id, sellerType, status string
		rate                   int32
		approvedAt             *time.Time
	)
	err := row.Scan(&id, &p.Name, &p.Slug, &sellerType, &status,
		&p.LegalName, &p.TaxCode, &p.Email, &p.Phone,
		&rate, &p.BankAccountVerified, &p.SuspensionReason,
		&p.ApprovedBy, &approvedAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	bp, err := types.NewBasisPoints(rate)
	if err != nil {
		return nil, fmt.Errorf("seller: tỷ lệ hoa hồng hỏng ở bản ghi %s: %w", id, err)
	}

	p.ID = ids.ID(id)
	p.SellerType = domain.SellerType(sellerType)
	p.Status = domain.Status(status)
	p.CommissionRate = bp
	if approvedAt != nil {
		p.ApprovedAt = *approvedAt
	}
	return domain.RestoreSeller(p), nil
}

func (s *Store) Save(ctx context.Context, sel *domain.Seller) error {
	const q = `
		INSERT INTO seller (
			id, name, slug, seller_type, status, legal_name, tax_code, email, phone,
			commission_rate, bank_account_verified, suspension_reason,
			approved_by, approved_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (id) DO UPDATE SET
			name                  = EXCLUDED.name,
			slug                  = EXCLUDED.slug,
			status                = EXCLUDED.status,
			legal_name            = EXCLUDED.legal_name,
			tax_code              = EXCLUDED.tax_code,
			email                 = EXCLUDED.email,
			phone                 = EXCLUDED.phone,
			commission_rate       = EXCLUDED.commission_rate,
			bank_account_verified = EXCLUDED.bank_account_verified,
			suspension_reason     = EXCLUDED.suspension_reason,
			approved_by           = EXCLUDED.approved_by,
			approved_at           = EXCLUDED.approved_at,
			updated_at            = EXCLUDED.updated_at`

	var approvedAt *time.Time
	if t := sel.ApprovedAt(); !t.IsZero() {
		approvedAt = &t
	}

	_, err := s.pool.Exec(ctx, q,
		sel.ID().String(), sel.Name(), sel.Slug(), string(sel.Type()), string(sel.Status()),
		sel.LegalName(), sel.TaxCode(), sel.Email(), sel.Phone(),
		sel.CommissionRate().Value(), sel.BankAccountVerified(), sel.SuspensionReason(),
		sel.ApprovedBy(), approvedAt, sel.CreatedAt(), sel.UpdatedAt())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.ConstraintName == "seller_slug_key" {
				return domain.ErrSlugTaken
			}
			// Vi phạm CHECK nghĩa là dữ liệu lọt qua kiểm tra ở domain
			// nhưng vẫn sai — lỗi lập trình, phải báo rõ chứ không nuốt.
			if pgErr.Code == "23514" {
				return fmt.Errorf("seller: vi phạm ràng buộc %s: %w", pgErr.ConstraintName, err)
			}
		}
		return fmt.Errorf("seller: lưu nhà bán: %w", err)
	}
	return nil
}

func (s *Store) FindByID(ctx context.Context, id ids.ID) (*domain.Seller, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM seller WHERE id = $1`, id.String()))
}

func (s *Store) FindBySlug(ctx context.Context, slug string) (*domain.Seller, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM seller WHERE slug = $1`, slug))
}

func (s *Store) FindByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*domain.Seller, error) {
	out := make(map[ids.ID]*domain.Seller, len(list))
	if len(list) == 0 {
		return out, nil
	}

	strs := make([]string, len(list))
	for i, id := range list {
		strs[i] = id.String()
	}

	rows, err := s.pool.Query(ctx, `SELECT `+cols+` FROM seller WHERE id = ANY($1)`, strs)
	if err != nil {
		return nil, fmt.Errorf("seller: đọc nhà bán theo lô: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		sel, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("seller: đọc nhà bán theo lô: %w", err)
		}
		out[sel.ID()] = sel
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("seller: đọc nhà bán theo lô: %w", err)
	}
	return out, nil
}

func (s *Store) List(ctx context.Context, f domain.Filter) ([]*domain.Seller, error) {
	q := `SELECT ` + cols + ` FROM seller
	      WHERE ($1 = '' OR status = $1)
	        AND ($2 = '' OR seller_type = $2)
	      ORDER BY id`

	args := []any{string(f.Status), string(f.Type)}
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT $%d", len(args)+1)
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", len(args)+1)
		args = append(args, f.Offset)
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("seller: liệt kê nhà bán: %w", err)
	}
	defer rows.Close()

	var out []*domain.Seller
	for rows.Next() {
		sel, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("seller: liệt kê nhà bán: %w", err)
		}
		out = append(out, sel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("seller: liệt kê nhà bán: %w", err)
	}
	return out, nil
}
