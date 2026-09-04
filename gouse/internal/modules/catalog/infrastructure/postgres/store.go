package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

// ---------------------------------------------------------------- Ủy quyền

type AuthorizationStore struct{ pool *pgxpool.Pool }

func NewAuthorizationStore(pool *pgxpool.Pool) *AuthorizationStore {
	return &AuthorizationStore{pool: pool}
}

const authCols = `
	id, brand_id, seller_id, status, document_url,
	valid_from, valid_until, approved_by, approved_at, created_at`

func scanAuth(row pgx.Row) (*domain.BrandAuthorization, error) {
	var (
		p                          domain.RestoreAuthorizationParams
		id, brandID, sellerID, stt string
		approvedAt                 *time.Time
	)
	err := row.Scan(&id, &brandID, &sellerID, &stt, &p.DocumentURL,
		&p.ValidFrom, &p.ValidUntil, &p.ApprovedBy, &approvedAt, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p.ID = ids.ID(id)
	p.BrandID = ids.ID(brandID)
	p.SellerID = ids.ID(sellerID)
	p.Status = domain.AuthorizationStatus(stt)
	p.ApprovedAt = timeOrZero(approvedAt)
	return domain.RestoreBrandAuthorization(p), nil
}

func (s *AuthorizationStore) Save(ctx context.Context, a *domain.BrandAuthorization) error {
	const q = `
		INSERT INTO brand_authorization (
			id, brand_id, seller_id, status, document_url,
			valid_from, valid_until, approved_by, approved_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET
			status      = EXCLUDED.status,
			document_url = EXCLUDED.document_url,
			valid_from  = EXCLUDED.valid_from,
			valid_until = EXCLUDED.valid_until,
			approved_by = EXCLUDED.approved_by,
			approved_at = EXCLUDED.approved_at`

	_, err := s.pool.Exec(ctx, q,
		a.ID().String(), a.BrandID().String(), a.SellerID().String(),
		string(a.Status()), a.DocumentURL(),
		a.ValidFrom(), a.ValidUntil(),
		a.ApprovedBy(), nullTime(a.ApprovedAt()), a.CreatedAt())
	if err != nil {
		// Chỉ MỘT ủy quyền APPROVED cho mỗi (brand, seller) — ràng buộc
		// UNIQUE có điều kiện ở database. Hai bản ghi APPROVED cùng lúc sẽ
		// khiến việc thu hồi không dứt điểm.
		if isUniqueViolation(err, "brand_authorization_active_uniq") {
			return domain.ErrDuplicateAuthorization
		}
		return fmt.Errorf("catalog: lưu ủy quyền: %w", err)
	}
	return nil
}

func (s *AuthorizationStore) FindByID(ctx context.Context, id ids.ID) (*domain.BrandAuthorization, error) {
	return scanAuth(s.pool.QueryRow(ctx,
		`SELECT `+authCols+` FROM brand_authorization WHERE id = $1`, id.String()))
}

// FindActiveForSeller tìm ủy quyền ĐANG HIỆU LỰC của seller cho thương hiệu.
//
// Lọc theo status ở đây, nhưng việc kiểm tra khoảng thời gian để cho domain
// (BrandAuthorization.IsValidAt): thời điểm "bây giờ" là quyết định của
// tầng ứng dụng qua Clock, không phải của database — dùng now() của
// database thì test không kiểm soát được thời gian.
func (s *AuthorizationStore) FindActiveForSeller(
	ctx context.Context, brandID, sellerID ids.ID,
) (*domain.BrandAuthorization, error) {
	return scanAuth(s.pool.QueryRow(ctx,
		`SELECT `+authCols+` FROM brand_authorization
		 WHERE brand_id = $1 AND seller_id = $2 AND status = 'APPROVED'`,
		brandID.String(), sellerID.String()))
}

// FindExpiring tìm ủy quyền sắp hết hạn trong `withinDays` ngày tới.
//
// Dùng now() của database ở đây là chấp nhận được: đây là job cảnh báo
// chạy nền, không phải quyết định nghiệp vụ ảnh hưởng tới khách hàng.
func (s *AuthorizationStore) FindExpiring(
	ctx context.Context, now time.Time, withinDays int,
) ([]*domain.BrandAuthorization, error) {
	// Mốc thời gian từ THAM SỐ, không dùng `now()` của database.
	//
	// Hai lý do: cùng một câu truy vấn phải trả lời được cho bất kỳ mốc
	// nào (test kiểm chứng được), và mốc phải giống hệt mốc mà tầng ứng
	// dụng đang dùng — `now()` của PostgreSQL là giờ máy chủ database,
	// có thể lệch giờ máy chủ ứng dụng.
	rows, err := s.pool.Query(ctx,
		`SELECT `+authCols+` FROM brand_authorization
		 WHERE status = 'APPROVED'
		   AND valid_until > $1
		   AND valid_until <= $1 + make_interval(days => $2)
		 ORDER BY valid_until`, now, withinDays)
	if err != nil {
		return nil, fmt.Errorf("catalog: đọc ủy quyền sắp hết hạn: %w", err)
	}
	defer rows.Close()
	return collectAuths(rows)
}

func collectAuths(rows pgx.Rows) ([]*domain.BrandAuthorization, error) {
	var out []*domain.BrandAuthorization
	for rows.Next() {
		a, err := scanAuth(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------- Bộ sưu tập

type CollectionStore struct{ pool *pgxpool.Pool }

func NewCollectionStore(pool *pgxpool.Pool) *CollectionStore {
	return &CollectionStore{pool: pool}
}

const collectionCols = `
	id, brand_id, name, slug, season, theme,
	launch_date, end_of_season_date, status, created_at, updated_at`

func scanCollection(row pgx.Row) (*domain.Collection, error) {
	var (
		p                domain.RestoreCollectionParams
		id, brandID, stt string
	)
	err := row.Scan(&id, &brandID, &p.Name, &p.Slug, &p.Season, &p.Theme,
		&p.LaunchDate, &p.EndOfSeasonDate, &stt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p.ID = ids.ID(id)
	p.BrandID = ids.ID(brandID)
	p.Status = domain.CollectionStatus(stt)
	return domain.RestoreCollection(p), nil
}

func (s *CollectionStore) Save(ctx context.Context, c *domain.Collection) error {
	const q = `
		INSERT INTO collection (
			id, brand_id, name, slug, season, theme,
			launch_date, end_of_season_date, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET
			name               = EXCLUDED.name,
			slug               = EXCLUDED.slug,
			season             = EXCLUDED.season,
			theme              = EXCLUDED.theme,
			launch_date        = EXCLUDED.launch_date,
			end_of_season_date = EXCLUDED.end_of_season_date,
			status             = EXCLUDED.status,
			updated_at         = EXCLUDED.updated_at`

	_, err := s.pool.Exec(ctx, q,
		c.ID().String(), c.BrandID().String(), c.Name(), c.Slug(), c.Season(), c.Theme(),
		c.LaunchDate(), c.EndOfSeason(), string(c.Status()), c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		if isUniqueViolation(err, "collection_slug_key") {
			return domain.ErrSlugTaken
		}
		return fmt.Errorf("catalog: lưu bộ sưu tập: %w", err)
	}
	return nil
}

func (s *CollectionStore) FindByID(ctx context.Context, id ids.ID) (*domain.Collection, error) {
	return scanCollection(s.pool.QueryRow(ctx,
		`SELECT `+collectionCols+` FROM collection WHERE id = $1`, id.String()))
}

func (s *CollectionStore) FindBySlug(ctx context.Context, slug string) (*domain.Collection, error) {
	return scanCollection(s.pool.QueryRow(ctx,
		`SELECT `+collectionCols+` FROM collection WHERE slug = $1`, slug))
}

func (s *CollectionStore) FindByBrand(ctx context.Context, brandID ids.ID) ([]*domain.Collection, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+collectionCols+` FROM collection WHERE brand_id = $1 ORDER BY launch_date DESC, id`,
		brandID.String())
	if err != nil {
		return nil, fmt.Errorf("catalog: đọc bộ sưu tập theo thương hiệu: %w", err)
	}
	defer rows.Close()
	return collectCollections(rows)
}

// FindByStatus lấy bộ sưu tập theo trạng thái.
//
// Worker định kỳ dùng để tìm bộ sưu tập tới hạn ra mắt hoặc kết thúc mùa.
func (s *CollectionStore) FindByStatus(ctx context.Context, st domain.CollectionStatus) ([]*domain.Collection, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+collectionCols+` FROM collection WHERE status = $1 ORDER BY id`, string(st))
	if err != nil {
		return nil, fmt.Errorf("catalog: đọc bộ sưu tập theo trạng thái: %w", err)
	}
	defer rows.Close()
	return collectCollections(rows)
}

func collectCollections(rows pgx.Rows) ([]*domain.Collection, error) {
	var out []*domain.Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------- Danh mục

type CategoryStore struct{ pool *pgxpool.Pool }

func NewCategoryStore(pool *pgxpool.Pool) *CategoryStore {
	return &CategoryStore{pool: pool}
}

const categoryCols = `
	id, parent_id, name, slug, depth, display_order, status, created_at, updated_at`

func scanCategory(row pgx.Row) (*domain.Category, error) {
	var (
		p                 domain.RestoreCategoryParams
		id, parentID, stt string
	)
	err := row.Scan(&id, &parentID, &p.Name, &p.Slug, &p.Depth,
		&p.DisplayOrder, &stt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	p.ID = ids.ID(id)
	p.ParentID = ids.ID(parentID)
	p.Status = domain.Status(stt)
	return domain.RestoreCategory(p), nil
}

func (s *CategoryStore) Save(ctx context.Context, c *domain.Category) error {
	const q = `
		INSERT INTO category (
			id, parent_id, name, slug, depth, display_order, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET
			parent_id     = EXCLUDED.parent_id,
			name          = EXCLUDED.name,
			slug          = EXCLUDED.slug,
			depth         = EXCLUDED.depth,
			display_order = EXCLUDED.display_order,
			status        = EXCLUDED.status,
			updated_at    = EXCLUDED.updated_at`

	_, err := s.pool.Exec(ctx, q,
		c.ID().String(), c.ParentID().String(), c.Name(), c.Slug(),
		c.Depth(), c.DisplayOrder(), string(c.Status()), c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		if isUniqueViolation(err, "category_slug_key") {
			return domain.ErrSlugTaken
		}
		return fmt.Errorf("catalog: lưu danh mục: %w", err)
	}
	return nil
}

func (s *CategoryStore) FindByID(ctx context.Context, id ids.ID) (*domain.Category, error) {
	return scanCategory(s.pool.QueryRow(ctx,
		`SELECT `+categoryCols+` FROM category WHERE id = $1`, id.String()))
}

func (s *CategoryStore) FindBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	return scanCategory(s.pool.QueryRow(ctx,
		`SELECT `+categoryCols+` FROM category WHERE slug = $1`, slug))
}

// FindAll lấy TOÀN BỘ cây danh mục trong một truy vấn.
//
// Cây danh mục nhỏ và đổi hiếm. Truy vấn đệ quy từng cấp sẽ tạo nhiều lượt
// đi lại database mà không được lợi gì.
func (s *CategoryStore) FindAll(ctx context.Context) ([]*domain.Category, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+categoryCols+` FROM category ORDER BY depth, display_order, id`)
	if err != nil {
		return nil, fmt.Errorf("catalog: đọc cây danh mục: %w", err)
	}
	defer rows.Close()

	var out []*domain.Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog: đọc cây danh mục: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: đọc cây danh mục: %w", err)
	}
	return out, nil
}

func (s *CategoryStore) FindChildren(ctx context.Context, parentID ids.ID) ([]*domain.Category, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+categoryCols+` FROM category WHERE parent_id = $1 ORDER BY display_order, id`,
		parentID.String())
	if err != nil {
		return nil, fmt.Errorf("catalog: đọc danh mục con: %w", err)
	}
	defer rows.Close()

	var out []*domain.Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog: đọc danh mục con: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: đọc danh mục con: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------- Bảng size

type SizeChartStore struct{ pool *pgxpool.Pool }

func NewSizeChartStore(pool *pgxpool.Pool) *SizeChartStore {
	return &SizeChartStore{pool: pool}
}

// Save lưu bảng size CÙNG các dòng của nó trong MỘT giao dịch.
//
// Bảng size và các dòng của nó là một aggregate: lưu bảng thành công mà
// lưu dòng thất bại sẽ để lại bảng size rỗng — tệ hơn là không có bảng
// size, vì giao diện sẽ hiển thị bảng trống thay vì ẩn đi.
func (s *SizeChartStore) Save(ctx context.Context, sc *domain.SizeChart) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("catalog: mở giao dịch: %w", err)
	}
	// Rollback sau khi Commit thành công là no-op, nên defer luôn an toàn.
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO size_chart (id, brand_id, product_type, system, note, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET
			note       = EXCLUDED.note,
			system     = EXCLUDED.system,
			updated_at = EXCLUDED.updated_at`

	if _, err := tx.Exec(ctx, q,
		sc.ID().String(), sc.BrandID().String(), string(sc.ProductType()),
		string(sc.System()), sc.Note(), sc.CreatedAt(), sc.UpdatedAt()); err != nil {
		if isUniqueViolation(err, "size_chart_brand_type_uniq") {
			return domain.ErrDuplicateSizeChart
		}
		return fmt.Errorf("catalog: lưu bảng size: %w", err)
	}

	// Xóa hết rồi ghi lại: đơn giản và luôn đúng. Bảng size có vài chục
	// dòng nên chi phí không đáng kể, còn việc so sánh từng dòng để cập
	// nhật tại chỗ dễ sót trường hợp dòng bị xóa.
	if _, err := tx.Exec(ctx,
		`DELETE FROM size_chart_entry WHERE size_chart_id = $1`, sc.ID().String()); err != nil {
		return fmt.Errorf("catalog: xóa dòng bảng size cũ: %w", err)
	}

	for i, e := range sc.Entries() {
		m, err := json.Marshal(e.Measurements)
		if err != nil {
			return fmt.Errorf("catalog: mã hóa số đo: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO size_chart_entry (size_chart_id, size, measurements, display_order)
			 VALUES ($1,$2,$3,$4)`,
			sc.ID().String(), e.Size, m, i); err != nil {
			return fmt.Errorf("catalog: lưu dòng bảng size: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("catalog: xác nhận giao dịch: %w", err)
	}
	return nil
}

func (s *SizeChartStore) FindByID(ctx context.Context, id ids.ID) (*domain.SizeChart, error) {
	return s.findOne(ctx,
		`SELECT id, brand_id, product_type, system, note, created_at, updated_at
		 FROM size_chart WHERE id = $1`, id.String())
}

func (s *SizeChartStore) FindForBrandAndType(
	ctx context.Context, brandID ids.ID, pt domain.ProductType,
) (*domain.SizeChart, error) {
	return s.findOne(ctx,
		`SELECT id, brand_id, product_type, system, note, created_at, updated_at
		 FROM size_chart WHERE brand_id = $1 AND product_type = $2`,
		brandID.String(), string(pt))
}

func (s *SizeChartStore) findOne(ctx context.Context, q string, args ...any) (*domain.SizeChart, error) {
	var (
		p                domain.RestoreSizeChartParams
		id, brandID      string
		productType, sys string
	)
	err := s.pool.QueryRow(ctx, q, args...).Scan(
		&id, &brandID, &productType, &sys, &p.Note, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("catalog: đọc bảng size: %w", err)
	}

	p.ID = ids.ID(id)
	p.BrandID = ids.ID(brandID)
	p.ProductType = domain.ProductType(productType)
	p.System = domain.SizeSystem(sys)

	entries, err := s.entriesFor(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Entries = entries
	return domain.RestoreSizeChart(p), nil
}

func (s *SizeChartStore) entriesFor(ctx context.Context, id ids.ID) ([]domain.SizeEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT size, measurements FROM size_chart_entry
		 WHERE size_chart_id = $1 ORDER BY display_order, size`, id.String())
	if err != nil {
		return nil, fmt.Errorf("catalog: đọc dòng bảng size: %w", err)
	}
	defer rows.Close()

	var out []domain.SizeEntry
	for rows.Next() {
		var (
			size string
			raw  []byte
		)
		if err := rows.Scan(&size, &raw); err != nil {
			return nil, fmt.Errorf("catalog: đọc dòng bảng size: %w", err)
		}
		m := map[string]string{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &m); err != nil {
				return nil, fmt.Errorf("catalog: giải mã số đo: %w", err)
			}
		}
		out = append(out, domain.SizeEntry{Size: size, Measurements: m})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: đọc dòng bảng size: %w", err)
	}
	return out, nil
}
