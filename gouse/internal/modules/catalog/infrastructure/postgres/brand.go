// Package postgres cài đặt các port của catalog bằng PostgreSQL.
//
// SQL viết tay theo ADR-0010. Struct trong package này KHÔNG phải thực thể
// domain — chúng chỉ là dữ liệu trung gian giữa bảng và aggregate, và
// KHÔNG được rời khỏi package này.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

// BrandStore lưu thương hiệu trong PostgreSQL.
type BrandStore struct {
	pool *pgxpool.Pool
}

func NewBrandStore(pool *pgxpool.Pool) *BrandStore {
	return &BrandStore{pool: pool}
}

// Save lưu hoặc cập nhật thương hiệu.
//
// Dùng UPSERT thay vì kiểm tra tồn tại rồi INSERT/UPDATE: hai bước có
// khoảng trống giữa chúng, và hai request đồng thời sẽ cùng thấy "chưa tồn
// tại" rồi cùng INSERT.
//
// Ràng buộc UNIQUE trên slug do DATABASE bảo đảm. Không tự kiểm tra trước
// vì kiểm tra ở tầng ứng dụng không chặn được tranh chấp đồng thời.
func (s *BrandStore) Save(ctx context.Context, b *domain.Brand) error {
	const q = `
		INSERT INTO brand (
			id, name, slug, description, logo_url, brand_type,
			protection_level, owner_seller_id, country_of_origin, status,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET
			name              = EXCLUDED.name,
			slug              = EXCLUDED.slug,
			description       = EXCLUDED.description,
			logo_url          = EXCLUDED.logo_url,
			brand_type        = EXCLUDED.brand_type,
			protection_level  = EXCLUDED.protection_level,
			owner_seller_id   = EXCLUDED.owner_seller_id,
			country_of_origin = EXCLUDED.country_of_origin,
			status            = EXCLUDED.status,
			updated_at        = EXCLUDED.updated_at`

	_, err := s.pool.Exec(ctx, q,
		b.ID().String(), b.Name(), b.Slug(), b.Description(), b.LogoURL(),
		string(b.Type()), string(b.ProtectionLevel()), b.OwnerSellerID().String(),
		b.CountryOfOrigin(), string(b.Status()), b.CreatedAt(), b.UpdatedAt(),
	)
	if err != nil {
		// Vi phạm UNIQUE trên slug là lỗi NGHIỆP VỤ, không phải sự cố hệ
		// thống — phải chuyển thành lỗi domain để tầng trên xử lý đúng.
		if isUniqueViolation(err, "brand_slug_key") {
			return domain.ErrSlugTaken
		}
		return fmt.Errorf("catalog: lưu thương hiệu: %w", err)
	}
	return nil
}

// brandCols là danh sách cột dùng chung cho mọi truy vấn đọc.
//
// Gom vào một hằng để thứ tự cột và scanBrand không bao giờ lệch nhau —
// lệch thứ tự cột là lỗi âm thầm, dữ liệu vẫn đọc được nhưng sai chỗ.
const brandCols = `
	id, name, slug, description, logo_url, brand_type,
	protection_level, owner_seller_id, country_of_origin, status,
	created_at, updated_at`

// scanBrand dựng aggregate từ một dòng.
//
// Dùng RestoreBrand (không kiểm tra) vì dữ liệu đã lưu từng hợp lệ theo
// luật lúc ghi. Kiểm tra lại lúc đọc sẽ làm hỏng việc đọc dữ liệu cũ khi
// luật đổi — đó là việc của migration.
func scanBrand(row pgx.Row) (*domain.Brand, error) {
	var (
		p                              domain.RestoreBrandParams
		id, ownerSellerID              string
		brandType, protectionLvl, stat string
	)
	err := row.Scan(
		&id, &p.Name, &p.Slug, &p.Description, &p.LogoURL, &brandType,
		&protectionLvl, &ownerSellerID, &p.CountryOfOrigin, &stat,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	p.ID = ids.ID(id)
	p.OwnerSellerID = ids.ID(ownerSellerID)
	p.BrandType = domain.BrandType(brandType)
	p.ProtectionLevel = domain.ProtectionLevel(protectionLvl)
	p.Status = domain.Status(stat)
	return domain.RestoreBrand(p), nil
}

func (s *BrandStore) FindByID(ctx context.Context, id ids.ID) (*domain.Brand, error) {
	b, err := scanBrand(s.pool.QueryRow(ctx,
		`SELECT `+brandCols+` FROM brand WHERE id = $1`, id.String()))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("catalog: đọc thương hiệu: %w", err)
	}
	return b, nil
}

func (s *BrandStore) FindBySlug(ctx context.Context, slug string) (*domain.Brand, error) {
	b, err := scanBrand(s.pool.QueryRow(ctx,
		`SELECT `+brandCols+` FROM brand WHERE slug = $1`, slug))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("catalog: đọc thương hiệu theo slug: %w", err)
	}
	return b, nil
}

// FindByIDs lấy nhiều thương hiệu trong MỘT truy vấn.
//
// Dùng `= ANY($1)` thay vì `IN (...)` ghép chuỗi: tránh SQL injection và
// tránh việc mỗi độ dài danh sách sinh ra một câu lệnh khác nhau (làm hỏng
// bộ nhớ đệm kế hoạch truy vấn của PostgreSQL).
func (s *BrandStore) FindByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*domain.Brand, error) {
	out := make(map[ids.ID]*domain.Brand, len(list))
	if len(list) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+brandCols+` FROM brand WHERE id = ANY($1)`, toStrings(list))
	if err != nil {
		return nil, fmt.Errorf("catalog: đọc thương hiệu theo lô: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		b, err := scanBrand(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog: đọc thương hiệu theo lô: %w", err)
		}
		out[b.ID()] = b
	}
	// rows.Err() bắt lỗi xảy ra GIỮA CHỪNG khi duyệt — bỏ qua nó nghĩa là
	// trả về kết quả thiếu mà không ai biết.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: đọc thương hiệu theo lô: %w", err)
	}
	return out, nil
}

func (s *BrandStore) List(ctx context.Context, f domain.BrandFilter) ([]*domain.Brand, error) {
	// Điều kiện rỗng khớp mọi dòng: `$1 = '' OR cột = $1`. Cách này giữ
	// MỘT câu lệnh duy nhất cho mọi tổ hợp bộ lọc, nên PostgreSQL dùng lại
	// được kế hoạch truy vấn.
	q := `SELECT ` + brandCols + ` FROM brand
	      WHERE ($1 = '' OR brand_type = $1)
	        AND ($2 = '' OR status = $2)
	      ORDER BY id`

	args := []any{string(f.Type), string(f.Status)}
	if f.Limit > 0 {
		q += ` LIMIT $3`
		args = append(args, f.Limit)
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("catalog: liệt kê thương hiệu: %w", err)
	}
	defer rows.Close()

	var out []*domain.Brand
	for rows.Next() {
		b, err := scanBrand(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog: liệt kê thương hiệu: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: liệt kê thương hiệu: %w", err)
	}
	return out, nil
}

// toStrings chuyển danh sách định danh sang chuỗi cho tham số truy vấn.
func toStrings(list []ids.ID) []string {
	out := make([]string, len(list))
	for i, id := range list {
		out[i] = id.String()
	}
	return out
}

// nullTime chuyển time.Time rỗng thành NULL và ngược lại.
//
// Phân biệt "chưa xảy ra" với "xảy ra lúc năm 0001" — nếu lưu time.Time
// rỗng thẳng vào cột, mọi so sánh thời gian sẽ coi nó là một mốc có thật
// trong quá khứ xa.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
