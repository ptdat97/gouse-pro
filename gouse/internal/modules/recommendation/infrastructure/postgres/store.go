// Package postgres lưu QUAN SÁT SIZE của khách.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/modules/recommendation/domain"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Record lưu một quan sát. Trùng nguồn thì BỎ QUA, không phải lỗi.
//
// Outbox giao ÍT NHẤT MỘT LẦN, nên cùng một event tới hai lần là hoạt động
// bình thường. Đếm hai lần sẽ làm một lần mua lấn át các quan sát khác.
func (s *Store) Record(ctx context.Context, q domain.NewObservation) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO size_observation (
			customer_id, brand_id, size, outcome,
			source_type, source_id, observed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT DO NOTHING`,
		q.CustomerID.String(), q.BrandID.String(), q.Size, string(q.Outcome),
		q.SourceType, q.SourceID.String(), q.ObservedAt)
	if err != nil {
		return fmt.Errorf("recommendation: ghi quan sát size: %w", err)
	}
	return nil
}

// ByCustomerAndBrand đọc quan sát của MỘT khách ở MỘT thương hiệu.
//
// `id` tăng dần theo thứ tự ghi, nên nó là thứ tự thời gian đủ dùng — và
// nó KHÔNG bị hai quan sát cùng micro giây làm cho mơ hồ như `observed_at`.
func (s *Store) ByCustomerAndBrand(
	ctx context.Context, customerID, brandID string,
) ([]domain.Observation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT size, outcome, id FROM size_observation
		 WHERE customer_id = $1 AND brand_id = $2
		 ORDER BY id`, customerID, brandID)
	if err != nil {
		return nil, fmt.Errorf("recommendation: đọc quan sát size: %w", err)
	}
	defer rows.Close()

	var out []domain.Observation
	for rows.Next() {
		var q domain.Observation
		var k string
		if err := rows.Scan(&q.Size, &k, &q.Seq); err != nil {
			return nil, fmt.Errorf("recommendation: đọc dòng quan sát: %w", err)
		}
		q.Outcome = domain.Outcome(k)
		out = append(out, q)
	}
	return out, rows.Err()
}
