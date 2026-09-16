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

// Ghi lưu một quan sát. Trùng nguồn thì BỎ QUA, không phải lỗi.
//
// Outbox giao ÍT NHẤT MỘT LẦN, nên cùng một event tới hai lần là hoạt động
// bình thường. Đếm hai lần sẽ làm một lần mua lấn át các quan sát khác.
func (s *Store) Ghi(ctx context.Context, q domain.QuanSatMoi) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO quan_sat_size (
			customer_id, brand_id, size, ket_qua,
			nguon_loai, nguon_id, quan_sat_luc
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT DO NOTHING`,
		q.CustomerID.String(), q.BrandID.String(), q.Size, string(q.KetQua),
		q.NguonLoai, q.NguonID.String(), q.QuanSatLuc)
	if err != nil {
		return fmt.Errorf("recommendation: ghi quan sát size: %w", err)
	}
	return nil
}

// TheoKhachVaThuongHieu đọc quan sát của MỘT khách ở MỘT thương hiệu.
//
// `id` tăng dần theo thứ tự ghi, nên nó là thứ tự thời gian đủ dùng — và
// nó KHÔNG bị hai quan sát cùng micro giây làm cho mơ hồ như `quan_sat_luc`.
func (s *Store) TheoKhachVaThuongHieu(
	ctx context.Context, customerID, brandID string,
) ([]domain.QuanSat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT size, ket_qua, id FROM quan_sat_size
		 WHERE customer_id = $1 AND brand_id = $2
		 ORDER BY id`, customerID, brandID)
	if err != nil {
		return nil, fmt.Errorf("recommendation: đọc quan sát size: %w", err)
	}
	defer rows.Close()

	var out []domain.QuanSat
	for rows.Next() {
		var q domain.QuanSat
		var k string
		if err := rows.Scan(&q.Size, &k, &q.ThuTu); err != nil {
			return nil, fmt.Errorf("recommendation: đọc dòng quan sát: %w", err)
		}
		q.KetQua = domain.KetQua(k)
		out = append(out, q)
	}
	return out, rows.Err()
}
