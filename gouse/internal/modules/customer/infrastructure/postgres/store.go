// Package postgres cài đặt kho lưu trữ customer bằng PostgreSQL.
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
	"github.com/fashion-commerce/platform/internal/modules/customer/domain"
)

// CustomerStore lưu và đọc hồ sơ khách hàng.
type CustomerStore struct {
	pool *pgxpool.Pool
}

func NewCustomerStore(pool *pgxpool.Pool) *CustomerStore {
	return &CustomerStore{pool: pool}
}

var _ domain.CustomerRepository = (*CustomerStore)(nil)

const customerCols = `
	id, user_id, email, phone, display_name, status,
	order_count, total_spent, currency, version, created_at, updated_at`

func (s *CustomerStore) Save(ctx context.Context, c *domain.Customer) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO customer (
			id, user_id, email, phone, display_name, status,
			order_count, total_spent, currency, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		c.ID().String(), c.UserID().String(), c.Email(), c.Phone(),
		c.DisplayName(), string(c.Status()), c.OrderCount(),
		c.TotalSpent().Amount(), string(c.TotalSpent().Currency()),
		c.Version(), c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		if isUnique(err, "customer_email_key") {
			return domain.ErrDuplicateEmail
		}
		return fmt.Errorf("customer: ghi hồ sơ: %w", err)
	}
	return nil
}

// Update ghi thay đổi bằng KHÓA LẠC QUAN.
//
// Điều kiện `version = $N` là thứ chặn mất cập nhật: hai request cùng đọc
// phiên bản 3 rồi cùng ghi, request thứ hai sẽ không khớp và bị từ chối.
// Không có nó, thay đổi của request đầu biến mất không dấu vết.
func (s *CustomerStore) Update(ctx context.Context, c *domain.Customer) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE customer
		   SET user_id = $2, email = $3, phone = $4, display_name = $5,
		       status = $6, order_count = $7, total_spent = $8,
		       currency = $9, version = $10, updated_at = $11
		 WHERE id = $1 AND version = $12`,
		c.ID().String(), c.UserID().String(), c.Email(), c.Phone(),
		c.DisplayName(), string(c.Status()), c.OrderCount(),
		c.TotalSpent().Amount(), string(c.TotalSpent().Currency()),
		c.Version(), c.UpdatedAt(), c.Version()-1)
	if err != nil {
		if isUnique(err, "customer_email_key") {
			return domain.ErrDuplicateEmail
		}
		return fmt.Errorf("customer: cập nhật hồ sơ: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// Không rõ là "không tồn tại" hay "phiên bản đã đổi". Phân biệt
		// bằng một truy vấn nữa: báo nhầm xung đột cho hồ sơ không tồn tại
		// sẽ khiến bên gọi thử lại mãi mãi.
		var exists bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM customer WHERE id = $1)`,
			c.ID().String()).Scan(&exists); err != nil {
			return fmt.Errorf("customer: kiểm tra hồ sơ: %w", err)
		}
		if !exists {
			return domain.ErrNotFound
		}
		return domain.ErrVersionConflict
	}
	return nil
}

func (s *CustomerStore) FindByID(ctx context.Context, id ids.ID) (*domain.Customer, error) {
	return s.findOne(ctx, `WHERE id = $1`, id.String())
}

// FindByEmail tra hồ sơ theo email ĐÃ CHUẨN HÓA.
//
// # Điều kiện status là LỚP THỨ HAI
//
// Ở đường đi bình thường nó KHÔNG gánh việc gì: Anonymize đã thay email
// bằng chuỗi giả `anonymized+<id>@…`, nên hàng đã ẩn danh không bao giờ
// khớp với một email thật. Đã kiểm chứng ngược để biết điều đó.
//
// Giữ lại vì nó chặn một lớp sai sót khác: nếu có đường nào ẩn danh hồ sơ
// mà QUÊN thay email, dòng này là thứ ngăn khách quay lại bị gắn vào chính
// hồ sơ họ vừa yêu cầu xóa — cùng toàn bộ lịch sử mua hàng đáng lẽ đã
// khuất khỏi tầm mắt họ. Xem TestHoSoAnDanhKhongHienKhiTraEmail.
func (s *CustomerStore) FindByEmail(ctx context.Context, email string) (*domain.Customer, error) {
	return s.findOne(ctx, `WHERE email = $1 AND status <> 'ANONYMIZED'`, email)
}

func (s *CustomerStore) FindByUserID(ctx context.Context, userID ids.ID) (*domain.Customer, error) {
	if userID.IsZero() {
		// Chuỗi rỗng khớp với MỌI khách vãng lai. Trả về một hồ sơ ngẫu
		// nhiên trong số đó là lỗi rất khó lần ra.
		return nil, domain.ErrNotFound
	}
	return s.findOne(ctx, `WHERE user_id = $1`, userID.String())
}

// FindManyByIDs đọc nhiều hồ sơ trong MỘT truy vấn.
//
// Tồn tại để tránh N+1: hiển thị danh sách 50 đơn hàng mà gọi FindByID 50
// lần là 50 lượt đi-về database cho một trang.
func (s *CustomerStore) FindManyByIDs(
	ctx context.Context, list []ids.ID,
) (map[ids.ID]*domain.Customer, error) {
	out := make(map[ids.ID]*domain.Customer, len(list))
	if len(list) == 0 {
		return out, nil
	}

	args := make([]string, 0, len(list))
	for _, id := range list {
		args = append(args, id.String())
	}

	rows, err := s.pool.Query(ctx,
		`SELECT`+customerCols+` FROM customer WHERE id = ANY($1)`, args)
	if err != nil {
		return nil, fmt.Errorf("customer: đọc nhiều hồ sơ: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, fmt.Errorf("customer: đọc nhiều hồ sơ: %w", err)
		}
		out[c.ID()] = c
	}
	return out, rows.Err()
}

func (s *CustomerStore) findOne(
	ctx context.Context, where string, args ...any,
) (*domain.Customer, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+customerCols+` FROM customer `+where, args...)

	c, err := scanCustomer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("customer: đọc hồ sơ: %w", err)
	}
	return c, nil
}

func scanCustomer(row interface{ Scan(...any) error }) (*domain.Customer, error) {
	var (
		p                domain.RestoreCustomerParams
		id, userID       string
		status, currency string
		amount           int64
	)
	if err := row.Scan(&id, &userID, &p.Email, &p.Phone, &p.DisplayName,
		&status, &p.OrderCount, &amount, &currency, &p.Version,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}

	p.ID = ids.ID(id)
	p.UserID = ids.ID(userID)
	p.Status = domain.Status(status)

	total, err := money.New(amount, money.Currency(currency))
	if err != nil {
		return nil, fmt.Errorf("customer: số tiền không hợp lệ: %w", err)
	}
	p.TotalSpent = total

	return domain.RestoreCustomer(p), nil
}

// ---------------------------------------------------------------- Địa chỉ

// AddressStore lưu và đọc sổ địa chỉ.
type AddressStore struct {
	pool *pgxpool.Pool
}

func NewAddressStore(pool *pgxpool.Pool) *AddressStore {
	return &AddressStore{pool: pool}
}

var _ domain.AddressRepository = (*AddressStore)(nil)

const addressCols = `
	id, customer_id, recipient_name, recipient_phone,
	line1, line2, ward, district, province, postcode, country,
	note, is_default, deleted_at, created_at, updated_at`

func (s *AddressStore) Save(ctx context.Context, a *domain.Address) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO customer_address (
			id, customer_id, recipient_name, recipient_phone,
			line1, line2, ward, district, province, postcode, country,
			note, is_default, deleted_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		a.ID().String(), a.CustomerID().String(), a.RecipientName(),
		a.RecipientPhone(), a.Line1(), a.Line2(), a.Ward(), a.District(),
		a.Province(), a.Postcode(), a.Country(), a.Note(), a.IsDefault(),
		nullTime(a.DeletedAt()), a.CreatedAt(), a.UpdatedAt())
	if err != nil {
		return fmt.Errorf("customer: ghi địa chỉ: %w", err)
	}
	return nil
}

// Update ghi lại nội dung địa chỉ.
//
// Điều kiện `customer_id` là LỚP CÁCH LY THỨ HAI, độc lập với BelongsTo ở
// tầng application. Hai lớp vì một lớp có thể bị sửa hỏng mà không ai nhận
// ra: biết id địa chỉ của người khác là sửa được tên, số điện thoại và địa
// chỉ nhà của họ.
//
// Đã kiểm chứng ngược: bỏ MỘT trong hai lớp thì test cách ly vẫn đỏ.
func (s *AddressStore) Update(ctx context.Context, a *domain.Address) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE customer_address
		   SET recipient_name = $3, recipient_phone = $4,
		       line1 = $5, line2 = $6, ward = $7, district = $8,
		       province = $9, postcode = $10, country = $11, note = $12,
		       is_default = $13, deleted_at = $14, updated_at = $15
		 WHERE id = $1 AND customer_id = $2`,
		a.ID().String(), a.CustomerID().String(),
		a.RecipientName(), a.RecipientPhone(),
		a.Line1(), a.Line2(), a.Ward(), a.District(), a.Province(),
		a.Postcode(), a.Country(), a.Note(), a.IsDefault(),
		nullTime(a.DeletedAt()), a.UpdatedAt())
	if err != nil {
		return fmt.Errorf("customer: cập nhật địa chỉ: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrAddressNotFound
	}
	return nil
}

func (s *AddressStore) FindByID(ctx context.Context, id ids.ID) (*domain.Address, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT`+addressCols+` FROM customer_address WHERE id = $1`, id.String())

	a, err := scanAddress(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAddressNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("customer: đọc địa chỉ: %w", err)
	}
	return a, nil
}

// ListByCustomer trả các địa chỉ CHƯA xóa, mặc định lên đầu.
func (s *AddressStore) ListByCustomer(
	ctx context.Context, customerID ids.ID,
) ([]*domain.Address, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+addressCols+`
		  FROM customer_address
		 WHERE customer_id = $1 AND deleted_at IS NULL
		 ORDER BY is_default DESC, created_at DESC`, customerID.String())
	if err != nil {
		return nil, fmt.Errorf("customer: đọc sổ địa chỉ: %w", err)
	}
	defer rows.Close()

	var out []*domain.Address
	for rows.Next() {
		a, err := scanAddress(rows)
		if err != nil {
			return nil, fmt.Errorf("customer: đọc sổ địa chỉ: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *AddressStore) FindDefault(
	ctx context.Context, customerID ids.ID,
) (*domain.Address, error) {
	row := s.pool.QueryRow(ctx, `SELECT`+addressCols+`
		  FROM customer_address
		 WHERE customer_id = $1 AND is_default AND deleted_at IS NULL`,
		customerID.String())

	a, err := scanAddress(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAddressNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("customer: đọc địa chỉ mặc định: %w", err)
	}
	return a, nil
}

func (s *AddressStore) ClearDefault(
	ctx context.Context, customerID ids.ID, now time.Time,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE customer_address SET is_default = false, updated_at = $2
		 WHERE customer_id = $1 AND is_default`, customerID.String(), now)
	if err != nil {
		return fmt.Errorf("customer: gỡ địa chỉ mặc định: %w", err)
	}
	return nil
}

// SetDefault đặt một địa chỉ làm mặc định trong MỘT giao dịch.
//
// # Vì sao phải là một giao dịch
//
// Chỉ mục UNIQUE cho phép ĐÚNG MỘT địa chỉ mặc định. Nếu gỡ cờ cũ và đặt
// cờ mới ở hai giao dịch riêng, có một khoảnh khắc khách KHÔNG có địa chỉ
// mặc định nào — và nếu bước hai thất bại, khoảnh khắc đó thành vĩnh viễn.
//
// Thứ tự cũng quan trọng: gỡ TRƯỚC, đặt SAU. Ngược lại sẽ đụng chỉ mục
// ngay ở câu đầu tiên.
func (s *AddressStore) SetDefault(
	ctx context.Context, customerID, addressID ids.ID, now time.Time,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("customer: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE customer_address SET is_default = false, updated_at = $2
		 WHERE customer_id = $1 AND is_default`,
		customerID.String(), now); err != nil {
		return fmt.Errorf("customer: gỡ địa chỉ mặc định: %w", err)
	}

	// Điều kiện customer_id là LỚP CÁCH LY: không có nó, biết id địa chỉ
	// của người khác là đặt được nó làm mặc định cho chính mình.
	tag, err := tx.Exec(ctx, `
		UPDATE customer_address SET is_default = true, updated_at = $3
		 WHERE id = $2 AND customer_id = $1 AND deleted_at IS NULL`,
		customerID.String(), addressID.String(), now)
	if err != nil {
		return fmt.Errorf("customer: đặt địa chỉ mặc định: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrAddressNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("customer: xác nhận giao dịch: %w", err)
	}
	return nil
}

func scanAddress(row interface{ Scan(...any) error }) (*domain.Address, error) {
	var (
		p              domain.RestoreAddressParams
		id, customerID string
		deleted        *time.Time
	)
	if err := row.Scan(&id, &customerID, &p.RecipientName, &p.RecipientPhone,
		&p.Line1, &p.Line2, &p.Ward, &p.District, &p.Province, &p.Postcode,
		&p.Country, &p.Note, &p.IsDefault, &deleted,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}

	p.ID = ids.ID(id)
	p.CustomerID = ids.ID(customerID)
	if deleted != nil {
		p.DeletedAt = *deleted
	}
	return domain.RestoreAddress(p), nil
}

// ---------------------------------------------------------------- Đồng ý

// ConsentStore ghi và đọc nhật ký đồng ý.
//
// CHỈ GHI THÊM: không có Update, không có Delete. Sửa nhật ký đồng ý là
// hủy giá trị pháp lý của chính nó.
type ConsentStore struct {
	pool *pgxpool.Pool
}

func NewConsentStore(pool *pgxpool.Pool) *ConsentStore {
	return &ConsentStore{pool: pool}
}

var _ domain.ConsentRepository = (*ConsentStore)(nil)

const consentCols = `
	id, customer_id, consent_type, granted, source,
	policy_version, ip_hash, user_agent, recorded_at`

func (s *ConsentStore) Record(ctx context.Context, c *domain.Consent) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO customer_consent (
			id, customer_id, consent_type, granted, source,
			policy_version, ip_hash, user_agent, recorded_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		c.ID().String(), c.CustomerID().String(), string(c.Type()),
		c.Granted(), c.Source(), c.PolicyVersion(), c.IPHash(),
		c.UserAgent(), c.RecordedAt())
	if err != nil {
		return fmt.Errorf("customer: ghi đồng ý: %w", err)
	}
	return nil
}

// Current trả bản ghi MỚI NHẤT của mỗi loại đồng ý.
//
// DISTINCT ON là cách của PostgreSQL để lấy "hàng đầu tiên trong mỗi
// nhóm". Cách thay thế — window function hoặc self-join — đều dài hơn và
// chậm hơn cho đúng việc này.
func (s *ConsentStore) Current(
	ctx context.Context, customerID ids.ID,
) (map[domain.ConsentType]*domain.Consent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (consent_type)`+consentCols+`
		  FROM customer_consent
		 WHERE customer_id = $1
		 ORDER BY consent_type, recorded_at DESC, id DESC`,
		customerID.String())
	if err != nil {
		return nil, fmt.Errorf("customer: đọc đồng ý: %w", err)
	}
	defer rows.Close()

	out := make(map[domain.ConsentType]*domain.Consent)
	for rows.Next() {
		c, err := scanConsent(rows)
		if err != nil {
			return nil, fmt.Errorf("customer: đọc đồng ý: %w", err)
		}
		out[c.Type()] = c
	}
	return out, rows.Err()
}

func (s *ConsentStore) History(
	ctx context.Context, customerID ids.ID,
) ([]*domain.Consent, error) {
	rows, err := s.pool.Query(ctx, `SELECT`+consentCols+`
		  FROM customer_consent WHERE customer_id = $1
		 ORDER BY recorded_at DESC, id DESC`, customerID.String())
	if err != nil {
		return nil, fmt.Errorf("customer: đọc lịch sử đồng ý: %w", err)
	}
	defer rows.Close()

	var out []*domain.Consent
	for rows.Next() {
		c, err := scanConsent(rows)
		if err != nil {
			return nil, fmt.Errorf("customer: đọc lịch sử đồng ý: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanConsent(row interface{ Scan(...any) error }) (*domain.Consent, error) {
	var (
		p                    domain.RestoreConsentParams
		id, customerID, kind string
	)
	if err := row.Scan(&id, &customerID, &kind, &p.Granted, &p.Source,
		&p.PolicyVersion, &p.IPHash, &p.UserAgent, &p.RecordedAt); err != nil {
		return nil, err
	}

	p.ID = ids.ID(id)
	p.CustomerID = ids.ID(customerID)
	p.Type = domain.ConsentType(kind)
	return domain.RestoreConsent(p), nil
}

// ---------------------------------------------------------------- Wishlist

// WishlistStore lưu và đọc danh sách yêu thích.
type WishlistStore struct {
	pool *pgxpool.Pool
}

func NewWishlistStore(pool *pgxpool.Pool) *WishlistStore {
	return &WishlistStore{pool: pool}
}

var _ domain.WishlistRepository = (*WishlistStore)(nil)

func (s *WishlistStore) Save(ctx context.Context, w *domain.Wishlist) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("customer: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO wishlist (id, customer_id, name, is_default, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO UPDATE
		   SET name = EXCLUDED.name, updated_at = EXCLUDED.updated_at`,
		w.ID().String(), w.CustomerID().String(), w.Name(), w.IsDefault(),
		w.CreatedAt(), w.UpdatedAt()); err != nil {
		return fmt.Errorf("customer: ghi danh sách yêu thích: %w", err)
	}

	for _, item := range w.Items() {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wishlist_item (
				wishlist_id, product_id, variant_id, note,
				notify_when_available, added_at
			) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT DO NOTHING`,
			w.ID().String(), item.ProductID.String(), item.VariantID.String(),
			item.Note, item.NotifyWhenAvailable, item.AddedAt); err != nil {
			return fmt.Errorf("customer: ghi món yêu thích: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("customer: xác nhận giao dịch: %w", err)
	}
	return nil
}

func (s *WishlistStore) FindDefault(
	ctx context.Context, customerID ids.ID,
) (*domain.Wishlist, error) {
	var (
		p      domain.RestoreWishlistParams
		id, cu string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT id, customer_id, name, is_default, created_at, updated_at
		  FROM wishlist WHERE customer_id = $1 AND is_default`,
		customerID.String()).
		Scan(&id, &cu, &p.Name, &p.IsDefault, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("customer: đọc danh sách yêu thích: %w", err)
	}

	p.ID = ids.ID(id)
	p.CustomerID = ids.ID(cu)

	items, err := s.loadItems(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Items = items

	return domain.RestoreWishlist(p), nil
}

func (s *WishlistStore) loadItems(
	ctx context.Context, wishlistID ids.ID,
) ([]domain.WishlistItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT product_id, variant_id, note, notify_when_available, added_at
		  FROM wishlist_item WHERE wishlist_id = $1
		 ORDER BY added_at DESC`, wishlistID.String())
	if err != nil {
		return nil, fmt.Errorf("customer: đọc món yêu thích: %w", err)
	}
	defer rows.Close()

	var out []domain.WishlistItem
	for rows.Next() {
		var (
			item             domain.WishlistItem
			product, variant string
		)
		if err := rows.Scan(&product, &variant, &item.Note,
			&item.NotifyWhenAvailable, &item.AddedAt); err != nil {
			return nil, fmt.Errorf("customer: đọc món yêu thích: %w", err)
		}
		item.ProductID = ids.ID(product)
		item.VariantID = ids.ID(variant)
		out = append(out, item)
	}
	return out, rows.Err()
}

// AddItem thêm một món.
//
// IDEMPOTENT Ở TẦNG DATABASE: khóa chính (wishlist, product, variant) chặn
// bản sao. Kiểm tra "đã có chưa" ở tầng ứng dụng KHÔNG cứu được khi khách
// bấm tim hai lần thật nhanh — hai request cùng đọc thấy chưa có.
//
// RowsAffected phân biệt "thêm mới" với "đã có sẵn", nhờ ON CONFLICT DO
// NOTHING không đếm hàng bị bỏ qua.
func (s *WishlistStore) AddItem(
	ctx context.Context, wishlistID ids.ID, item domain.WishlistItem,
) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO wishlist_item (
			wishlist_id, product_id, variant_id, note,
			notify_when_available, added_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT DO NOTHING`,
		wishlistID.String(), item.ProductID.String(), item.VariantID.String(),
		item.Note, item.NotifyWhenAvailable, item.AddedAt)
	if err != nil {
		return false, fmt.Errorf("customer: thêm món yêu thích: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *WishlistStore) RemoveItem(
	ctx context.Context, wishlistID, productID, variantID ids.ID,
) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM wishlist_item
		 WHERE wishlist_id = $1 AND product_id = $2 AND variant_id = $3`,
		wishlistID.String(), productID.String(), variantID.String())
	if err != nil {
		return false, fmt.Errorf("customer: bỏ món yêu thích: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// CountByProduct đếm số khách đã thích một sản phẩm.
//
// COUNT(DISTINCT wishlist_id) chứ không phải COUNT(*): một khách thích cả
// size M lẫn size L là HAI hàng nhưng MỘT người quan tâm. Đếm nhầm sẽ thổi
// phồng tín hiệu nhu cầu của sản phẩm nhiều biến thể.
func (s *WishlistStore) CountByProduct(ctx context.Context, productID ids.ID) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT wishlist_id) FROM wishlist_item WHERE product_id = $1`,
		productID.String()).Scan(&n); err != nil {
		return 0, fmt.Errorf("customer: đếm lượt yêu thích: %w", err)
	}
	return n, nil
}

// ---------------------------------------------------------------- Gộp

// MergeLogStore ghi nhật ký gộp danh tính.
type MergeLogStore struct {
	pool *pgxpool.Pool
}

func NewMergeLogStore(pool *pgxpool.Pool) *MergeLogStore {
	return &MergeLogStore{pool: pool}
}

var _ domain.MergeLogRepository = (*MergeLogStore)(nil)

func (s *MergeLogStore) Record(ctx context.Context, m domain.MergeRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO customer_merge_log (
			source_customer_id, target_customer_id, reason, merged_at
		) VALUES ($1,$2,$3,$4)`,
		m.SourceCustomerID.String(), m.TargetCustomerID.String(),
		m.Reason, m.MergedAt)
	if err != nil {
		return fmt.Errorf("customer: ghi nhật ký gộp: %w", err)
	}
	return nil
}

func (s *MergeLogStore) ListByTarget(
	ctx context.Context, targetID ids.ID,
) ([]domain.MergeRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source_customer_id, target_customer_id, reason, merged_at
		  FROM customer_merge_log WHERE target_customer_id = $1
		 ORDER BY merged_at DESC`, targetID.String())
	if err != nil {
		return nil, fmt.Errorf("customer: đọc nhật ký gộp: %w", err)
	}
	defer rows.Close()

	var out []domain.MergeRecord
	for rows.Next() {
		var (
			m              domain.MergeRecord
			source, target string
		)
		if err := rows.Scan(&source, &target, &m.Reason, &m.MergedAt); err != nil {
			return nil, fmt.Errorf("customer: đọc nhật ký gộp: %w", err)
		}
		m.SourceCustomerID = ids.ID(source)
		m.TargetCustomerID = ids.ID(target)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- Tiện ích

// nullTime đổi thời điểm rỗng thành NULL.
//
// time.Time rỗng ghi thẳng xuống là năm 0001 — một giá trị trông như dữ
// liệu thật và làm hỏng mọi so sánh khoảng thời gian.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// isUnique cho biết lỗi có phải vi phạm chỉ mục UNIQUE cụ thể không.
func isUnique(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
