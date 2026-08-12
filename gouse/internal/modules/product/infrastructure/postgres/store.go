// Package postgres cài đặt các port của product bằng PostgreSQL.
//
// SQL viết tay theo ADR-0010.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
)

// ProductStore lưu sản phẩm trong PostgreSQL.
type ProductStore struct {
	pool *pgxpool.Pool
}

func NewProductStore(pool *pgxpool.Pool) *ProductStore {
	return &ProductStore{pool: pool}
}

const productCols = `
	id, brand_id, collection_id, category_id, size_chart_id,
	name, slug, description, care_instructions, material_composition,
	origin_country, product_type, gender_target, status, rejection_reason,
	created_by_seller_id, images, published_at, created_at, updated_at`

// Save lưu sản phẩm CÙNG toàn bộ biến thể và SKU trong MỘT giao dịch.
//
// Product/Variant/SKU là một aggregate: lưu sản phẩm thành công mà lưu SKU
// thất bại sẽ để lại sản phẩm không bán được — và không ai biết cho tới khi
// khách bấm mua.
func (s *ProductStore) Save(ctx context.Context, p *domain.Product) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("product: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO product (
			id, brand_id, collection_id, category_id, size_chart_id,
			name, slug, description, care_instructions, material_composition,
			origin_country, product_type, gender_target, status, rejection_reason,
			created_by_seller_id, images, published_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (id) DO UPDATE SET
			collection_id        = EXCLUDED.collection_id,
			category_id          = EXCLUDED.category_id,
			size_chart_id        = EXCLUDED.size_chart_id,
			name                 = EXCLUDED.name,
			slug                 = EXCLUDED.slug,
			description          = EXCLUDED.description,
			care_instructions    = EXCLUDED.care_instructions,
			material_composition = EXCLUDED.material_composition,
			origin_country       = EXCLUDED.origin_country,
			product_type         = EXCLUDED.product_type,
			gender_target        = EXCLUDED.gender_target,
			status               = EXCLUDED.status,
			rejection_reason     = EXCLUDED.rejection_reason,
			images               = EXCLUDED.images,
			published_at         = EXCLUDED.published_at,
			updated_at           = EXCLUDED.updated_at`

	_, err = tx.Exec(ctx, q,
		p.ID().String(), p.BrandID().String(), p.CollectionID().String(),
		p.CategoryID().String(), p.SizeChartID().String(),
		p.Name(), p.Slug(), p.Description(), p.CareInstructions(),
		p.MaterialComposition(), p.OriginCountry(),
		string(p.Type()), string(p.GenderTarget()), string(p.Status()),
		p.RejectionReason(), p.CreatedBySellerID().String(),
		p.Images(), nullTime(p.PublishedAt()), p.CreatedAt(), p.UpdatedAt())
	if err != nil {
		return translateSaveErr(err)
	}

	// Xóa hết biến thể rồi ghi lại. ON DELETE CASCADE dọn luôn SKU.
	//
	// Đơn giản và luôn đúng: so sánh từng biến thể để cập nhật tại chỗ dễ
	// sót trường hợp biến thể bị gỡ khỏi sản phẩm.
	if _, err := tx.Exec(ctx, `DELETE FROM variant WHERE product_id = $1`, p.ID().String()); err != nil {
		return fmt.Errorf("product: xóa biến thể cũ: %w", err)
	}

	for _, v := range p.Variants() {
		attrs, err := json.Marshal(v.Attributes())
		if err != nil {
			return fmt.Errorf("product: mã hóa thuộc tính: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO variant (
				id, product_id, attributes, attribute_key, images,
				display_order, status, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			v.ID().String(), p.ID().String(), attrs, v.AttributeKey(),
			v.Images(), v.DisplayOrder(), string(v.Status()),
			v.CreatedAt(), v.UpdatedAt()); err != nil {
			return translateSaveErr(err)
		}

		for _, sku := range v.SKUs() {
			d := sku.Dimensions()
			if _, err := tx.Exec(ctx, `
				INSERT INTO sku (
					id, variant_id, sku_code, barcode,
					weight_gram, length_mm, width_mm, height_mm,
					status, created_at, updated_at
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
				sku.ID().String(), v.ID().String(), sku.Code(), sku.Barcode(),
				sku.WeightGram(), d.LengthMM, d.WidthMM, d.HeightMM,
				string(sku.Status()), sku.CreatedAt(), sku.UpdatedAt()); err != nil {
				return translateSaveErr(err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("product: xác nhận giao dịch: %w", err)
	}
	return nil
}

// translateSaveErr chuyển lỗi ràng buộc của database thành lỗi domain.
//
// Ràng buộc ở database là chốt chặn THẬT (chặn được cả tranh chấp đồng
// thời), nhưng lỗi thô của nó vô nghĩa với người dùng — phải dịch lại.
func translateSaveErr(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("product: lưu sản phẩm: %w", err)
	}

	switch {
	case pgErr.ConstraintName == "product_slug_key":
		return domain.ErrSlugTaken
	case pgErr.ConstraintName == "sku_sku_code_key":
		return domain.ErrSKUCodeTaken
	case pgErr.ConstraintName == "variant_attributes_uniq":
		return domain.ErrDuplicateVariant
	}
	return fmt.Errorf("product: lưu sản phẩm: %w", err)
}

// scanProduct dựng aggregate TỪ MỘT DÒNG, chưa có biến thể.
func scanProduct(row pgx.Row) (*domain.Product, error) {
	var (
		p                                domain.RestoreProductParams
		id, brandID, collectionID, catID string
		sizeChartID, sellerID            string
		productType, gender, status      string
		publishedAt                      *time.Time
	)
	err := row.Scan(
		&id, &brandID, &collectionID, &catID, &sizeChartID,
		&p.Name, &p.Slug, &p.Description, &p.CareInstructions, &p.MaterialComposition,
		&p.OriginCountry, &productType, &gender, &status, &p.RejectionReason,
		&sellerID, &p.Images, &publishedAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	p.ID = ids.ID(id)
	p.BrandID = ids.ID(brandID)
	p.CollectionID = ids.ID(collectionID)
	p.CategoryID = ids.ID(catID)
	p.SizeChartID = ids.ID(sizeChartID)
	p.CreatedBySellerID = ids.ID(sellerID)
	p.ProductType = domain.ProductType(productType)
	p.GenderTarget = domain.GenderTarget(gender)
	p.Status = domain.Status(status)
	p.PublishedAt = timeOrZero(publishedAt)
	return domain.RestoreProduct(p), nil
}

// loadVariants nạp biến thể và SKU cho một tập sản phẩm.
//
// Nạp theo LÔ, không phải mỗi sản phẩm một truy vấn: hiển thị 50 sản phẩm
// phải là 3 truy vấn (sản phẩm, biến thể, SKU), không phải 101.
func (s *ProductStore) loadVariants(ctx context.Context, products map[ids.ID]*domain.Product) error {
	if len(products) == 0 {
		return nil
	}

	productIDs := make([]string, 0, len(products))
	for id := range products {
		productIDs = append(productIDs, id.String())
	}

	// Nạp SKU trước, gom theo variant_id.
	skusByVariant := map[ids.ID][]*domain.SKU{}
	skuRows, err := s.pool.Query(ctx, `
		SELECT s.id, s.variant_id, s.sku_code, s.barcode,
		       s.weight_gram, s.length_mm, s.width_mm, s.height_mm,
		       s.status, s.created_at, s.updated_at
		FROM sku s
		JOIN variant v ON v.id = s.variant_id
		WHERE v.product_id = ANY($1)
		ORDER BY s.sku_code`, productIDs)
	if err != nil {
		return fmt.Errorf("product: đọc SKU: %w", err)
	}
	defer skuRows.Close()

	for skuRows.Next() {
		var (
			sp                domain.RestoreSKUParams
			id, variantID, st string
			d                 domain.Dimensions
		)
		if err := skuRows.Scan(&id, &variantID, &sp.Code, &sp.Barcode,
			&sp.WeightGram, &d.LengthMM, &d.WidthMM, &d.HeightMM,
			&st, &sp.CreatedAt, &sp.UpdatedAt); err != nil {
			return fmt.Errorf("product: đọc SKU: %w", err)
		}
		sp.ID = ids.ID(id)
		sp.VariantID = ids.ID(variantID)
		sp.Dimensions = d
		sp.Status = domain.SKUStatus(st)
		vid := ids.ID(variantID)
		skusByVariant[vid] = append(skusByVariant[vid], domain.RestoreSKU(sp))
	}
	if err := skuRows.Err(); err != nil {
		return fmt.Errorf("product: đọc SKU: %w", err)
	}

	// Nạp biến thể, gắn SKU vào, rồi gắn vào sản phẩm.
	varRows, err := s.pool.Query(ctx, `
		SELECT id, product_id, attributes, images, display_order, status, created_at, updated_at
		FROM variant WHERE product_id = ANY($1) ORDER BY display_order, id`, productIDs)
	if err != nil {
		return fmt.Errorf("product: đọc biến thể: %w", err)
	}
	defer varRows.Close()

	variantsByProduct := map[ids.ID][]*domain.Variant{}
	for varRows.Next() {
		var (
			vp                domain.RestoreVariantParams
			id, productID, st string
			rawAttrs          []byte
		)
		if err := varRows.Scan(&id, &productID, &rawAttrs, &vp.Images,
			&vp.DisplayOrder, &st, &vp.CreatedAt, &vp.UpdatedAt); err != nil {
			return fmt.Errorf("product: đọc biến thể: %w", err)
		}

		attrs := map[string]string{}
		if len(rawAttrs) > 0 {
			if err := json.Unmarshal(rawAttrs, &attrs); err != nil {
				return fmt.Errorf("product: giải mã thuộc tính: %w", err)
			}
		}
		vp.ID = ids.ID(id)
		vp.ProductID = ids.ID(productID)
		vp.Attributes = attrs
		vp.Status = domain.Status(st)
		vp.SKUs = skusByVariant[vp.ID]

		pid := ids.ID(productID)
		variantsByProduct[pid] = append(variantsByProduct[pid], domain.RestoreVariant(vp))
	}
	if err := varRows.Err(); err != nil {
		return fmt.Errorf("product: đọc biến thể: %w", err)
	}

	// Dựng lại sản phẩm kèm biến thể.
	//
	// RestoreProduct nhận biến thể qua tham số nên phải dựng lại toàn bộ
	// aggregate — không có cách gắn biến thể vào sau.
	for id, p := range products {
		products[id] = rebuildWithVariants(p, variantsByProduct[id])
	}
	return nil
}

func rebuildWithVariants(p *domain.Product, variants []*domain.Variant) *domain.Product {
	return domain.RestoreProduct(domain.RestoreProductParams{
		ID:                  p.ID(),
		BrandID:             p.BrandID(),
		CollectionID:        p.CollectionID(),
		CategoryID:          p.CategoryID(),
		SizeChartID:         p.SizeChartID(),
		Name:                p.Name(),
		Slug:                p.Slug(),
		Description:         p.Description(),
		CareInstructions:    p.CareInstructions(),
		MaterialComposition: p.MaterialComposition(),
		OriginCountry:       p.OriginCountry(),
		ProductType:         p.Type(),
		GenderTarget:        p.GenderTarget(),
		Status:              p.Status(),
		RejectionReason:     p.RejectionReason(),
		CreatedBySellerID:   p.CreatedBySellerID(),
		Images:              p.Images(),
		Variants:            variants,
		PublishedAt:         p.PublishedAt(),
		CreatedAt:           p.CreatedAt(),
		UpdatedAt:           p.UpdatedAt(),
	})
}

func (s *ProductStore) FindByID(ctx context.Context, id ids.ID) (*domain.Product, error) {
	p, err := scanProduct(s.pool.QueryRow(ctx,
		`SELECT `+productCols+` FROM product WHERE id = $1`, id.String()))
	if err != nil {
		return nil, err
	}
	m := map[ids.ID]*domain.Product{p.ID(): p}
	if err := s.loadVariants(ctx, m); err != nil {
		return nil, err
	}
	return m[p.ID()], nil
}

func (s *ProductStore) FindBySlug(ctx context.Context, slug string) (*domain.Product, error) {
	p, err := scanProduct(s.pool.QueryRow(ctx,
		`SELECT `+productCols+` FROM product WHERE slug = $1`, slug))
	if err != nil {
		return nil, err
	}
	m := map[ids.ID]*domain.Product{p.ID(): p}
	if err := s.loadVariants(ctx, m); err != nil {
		return nil, err
	}
	return m[p.ID()], nil
}

func (s *ProductStore) FindByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*domain.Product, error) {
	out := make(map[ids.ID]*domain.Product, len(list))
	if len(list) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+productCols+` FROM product WHERE id = ANY($1)`, toStrings(list))
	if err != nil {
		return nil, fmt.Errorf("product: đọc sản phẩm theo lô: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("product: đọc sản phẩm theo lô: %w", err)
		}
		out[p.ID()] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("product: đọc sản phẩm theo lô: %w", err)
	}

	if err := s.loadVariants(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ProductStore) FindByCollection(ctx context.Context, collectionID ids.ID) ([]*domain.Product, error) {
	return s.queryMany(ctx,
		`SELECT `+productCols+` FROM product WHERE collection_id = $1 ORDER BY id`,
		collectionID.String())
}

func (s *ProductStore) FindBySKUCode(ctx context.Context, code string) (*domain.Product, error) {
	// Chuẩn hóa giống domain.NewSKU — quét mã vạch chữ thường vẫn phải ra hàng.
	p, err := scanProduct(s.pool.QueryRow(ctx, `
		SELECT `+prefixCols("p")+`
		FROM product p
		JOIN variant v ON v.product_id = p.id
		JOIN sku s ON s.variant_id = v.id
		WHERE s.sku_code = $1`, strings.ToUpper(strings.TrimSpace(code))))
	if err != nil {
		return nil, err
	}
	m := map[ids.ID]*domain.Product{p.ID(): p}
	if err := s.loadVariants(ctx, m); err != nil {
		return nil, err
	}
	return m[p.ID()], nil
}

func (s *ProductStore) FindBySKUIDs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID]*domain.Product, error) {
	out := make(map[ids.ID]*domain.Product, len(skuIDs))
	if len(skuIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT s.id, `+prefixCols("p")+`
		FROM sku s
		JOIN variant v ON v.id = s.variant_id
		JOIN product p ON p.id = v.product_id
		WHERE s.id = ANY($1)`, toStrings(skuIDs))
	if err != nil {
		return nil, fmt.Errorf("product: tra ngược từ SKU: %w", err)
	}
	defer rows.Close()

	byProduct := map[ids.ID]*domain.Product{}
	skuToProduct := map[ids.ID]ids.ID{}
	for rows.Next() {
		var skuID string
		var (
			p                                domain.RestoreProductParams
			id, brandID, collectionID, catID string
			sizeChartID, sellerID            string
			productType, gender, status      string
			publishedAt                      *time.Time
		)
		if err := rows.Scan(&skuID,
			&id, &brandID, &collectionID, &catID, &sizeChartID,
			&p.Name, &p.Slug, &p.Description, &p.CareInstructions, &p.MaterialComposition,
			&p.OriginCountry, &productType, &gender, &status, &p.RejectionReason,
			&sellerID, &p.Images, &publishedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("product: tra ngược từ SKU: %w", err)
		}
		p.ID = ids.ID(id)
		p.BrandID = ids.ID(brandID)
		p.CollectionID = ids.ID(collectionID)
		p.CategoryID = ids.ID(catID)
		p.SizeChartID = ids.ID(sizeChartID)
		p.CreatedBySellerID = ids.ID(sellerID)
		p.ProductType = domain.ProductType(productType)
		p.GenderTarget = domain.GenderTarget(gender)
		p.Status = domain.Status(status)
		p.PublishedAt = timeOrZero(publishedAt)

		prod := domain.RestoreProduct(p)
		byProduct[prod.ID()] = prod
		skuToProduct[ids.ID(skuID)] = prod.ID()
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("product: tra ngược từ SKU: %w", err)
	}

	if err := s.loadVariants(ctx, byProduct); err != nil {
		return nil, err
	}
	for skuID, productID := range skuToProduct {
		out[skuID] = byProduct[productID]
	}
	return out, nil
}

func (s *ProductStore) List(ctx context.Context, f domain.Filter) ([]*domain.Product, error) {
	// Một câu lệnh cho mọi tổ hợp bộ lọc: điều kiện rỗng khớp mọi dòng.
	// PostgreSQL dùng lại được kế hoạch truy vấn thay vì phân tích lại mỗi
	// tổ hợp.
	//
	// BẢO MẬT: lọc theo seller nằm trong TRUY VẤN. Dữ liệu seller khác
	// không bao giờ rời khỏi database.
	q := `SELECT ` + productCols + ` FROM product
	      WHERE ($1 = '' OR brand_id = $1)
	        AND ($2 = '' OR category_id = $2)
	        AND ($3 = '' OR collection_id = $3)
	        AND ($4 = '' OR created_by_seller_id = $4)
	        AND ($5 = '' OR product_type = $5)
	        AND ($6 = '' OR gender_target = $6)
	        AND ($7 = '' OR status = $7)
	        AND (NOT $8::bool OR status = 'ACTIVE')
	      ORDER BY id`

	args := []any{
		f.BrandID.String(), f.CategoryID.String(), f.CollectionID.String(),
		f.SellerID.String(), string(f.ProductType), string(f.Gender),
		string(f.Status), f.OnlyVisible,
	}
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT $%d", len(args)+1)
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", len(args)+1)
		args = append(args, f.Offset)
	}

	return s.queryMany(ctx, q, args...)
}

func (s *ProductStore) queryMany(ctx context.Context, q string, args ...any) ([]*domain.Product, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("product: truy vấn sản phẩm: %w", err)
	}
	defer rows.Close()

	var ordered []ids.ID
	m := map[ids.ID]*domain.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("product: truy vấn sản phẩm: %w", err)
		}
		m[p.ID()] = p
		ordered = append(ordered, p.ID())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("product: truy vấn sản phẩm: %w", err)
	}

	if err := s.loadVariants(ctx, m); err != nil {
		return nil, err
	}

	// Giữ nguyên thứ tự của ORDER BY — map không có thứ tự.
	out := make([]*domain.Product, 0, len(ordered))
	for _, id := range ordered {
		out = append(out, m[id])
	}
	return out, nil
}

// prefixCols thêm tiền tố bảng vào danh sách cột, cho truy vấn có JOIN.
func prefixCols(alias string) string {
	parts := strings.Split(strings.ReplaceAll(productCols, "\n", " "), ",")
	for i, p := range parts {
		parts[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func toStrings(list []ids.ID) []string {
	out := make([]string, len(list))
	for i, id := range list {
		out[i] = id.String()
	}
	return out
}

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
