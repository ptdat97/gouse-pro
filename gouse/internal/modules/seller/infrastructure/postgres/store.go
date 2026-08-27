// Package postgres cài đặt port của seller bằng PostgreSQL.
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

	"github.com/fashion-commerce/platform/internal/platform/privacy"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/seller/domain"
)

// Store lưu nhà bán trong PostgreSQL.
type Store struct {
	pool *pgxpool.Pool

	// maHoa mã hóa số tài khoản ngân hàng. Có thể nil: khi đó nhà bán vẫn
	// lưu được nhưng KHÔNG khai được tài khoản — thà từ chối còn hơn ghi
	// số tài khoản ở dạng rõ.
	maHoa *privacy.BoMaHoa
}

func NewStore(pool *pgxpool.Pool, maHoa *privacy.BoMaHoa) *Store {
	return &Store{pool: pool, maHoa: maHoa}
}

// cols CỐ Ý không có `bank_account_number_enc`.
//
// Đường đọc thường không cần số tài khoản đầy đủ, và thứ không được đọc
// thì không lộ được. Muốn số đầy đủ thì gọi LaySoTaiKhoan — một hàm
// riêng, đếm được, và là chỗ duy nhất giải mã.
const cols = `
	id, name, slug, seller_type, status, legal_name, tax_code, email, phone,
	commission_rate, bank_account_verified, suspension_reason,
	approved_by, approved_at, created_at, updated_at,
	bank_code, bank_account_last4, bank_account_holder`

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
		&p.ApprovedBy, &approvedAt, &p.CreatedAt, &p.UpdatedAt,
		&p.BankAccount.BankCode, &p.BankAccount.Last4, &p.BankAccount.Holder)
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

// execer là thứ chạy được câu lệnh: pool hoặc một giao dịch đang mở.
//
// Nhờ nó, Save và SaveWithAudit dùng CHUNG một câu SQL — hai bản sao của
// cùng câu lệnh sẽ lệch nhau ngay lần thêm cột đầu tiên.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (s *Store) Save(ctx context.Context, sel *domain.Seller) error {
	return save(ctx, s.pool, sel)
}

// SaveWithAudit ghi nhà bán và chạy fn trong CÙNG một giao dịch.
//
// Thứ tự có chủ ý: ghi thay đổi TRƯỚC, chạy fn SAU, commit CUỐI. Nếu fn
// thất bại, `defer Rollback` hủy cả hai — không có trường hợp seller đổi
// trạng thái mà vết kiểm toán biến mất.
func (s *Store) SaveWithAudit(
	ctx context.Context, sel *domain.Seller, fn domain.TxFunc,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("seller: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := save(ctx, tx, sel); err != nil {
		return err
	}

	if fn != nil {
		// Ngữ cảnh MANG giao dịch, để fn ghi bằng chính nó.
		if err := fn(withTx(ctx, tx)); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("seller: xác nhận giao dịch: %w", err)
	}
	return nil
}

// txKey gắn giao dịch vào ngữ cảnh cho TxFunc.
type txKey struct{}

func withTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFrom lấy giao dịch mà SaveWithAudit đang mở.
func TxFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

func save(ctx context.Context, ex execer, sel *domain.Seller) error {
	const q = `
		INSERT INTO seller (
			id, name, slug, seller_type, status, legal_name, tax_code, email, phone,
			commission_rate, bank_account_verified, suspension_reason,
			approved_by, approved_at, created_at, updated_at,
			bank_code, bank_account_last4, bank_account_holder,
			bank_account_number_enc
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,
		          $17,$18,$19,
		          -- Mang theo bản mã ĐANG CÓ, nếu dòng đã tồn tại.
		          --
		          -- Entity không giữ số tài khoản, nên nếu để cột này rỗng
		          -- ở dòng đề xuất thì ràng buộc seller_verified_needs_account
		          -- sẽ vi phạm ngay khi ai đó bật cờ đã-xác-minh.
		          --
		          -- PostgreSQL kiểm CHECK trên DÒNG ĐỀ XUẤT trước khi phát
		          -- hiện xung đột và chuyển sang nhánh DO UPDATE. Nhánh
		          -- DO UPDATE không đụng tới cột này là đúng, nhưng không
		          -- cứu được: dòng đề xuất đã bị bác trước đó rồi.
		          COALESCE((SELECT bank_account_number_enc
		                      FROM seller WHERE id = $1), ''))
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
			updated_at            = EXCLUDED.updated_at,
			bank_code             = EXCLUDED.bank_code,
			bank_account_last4    = EXCLUDED.bank_account_last4,
			bank_account_holder   = EXCLUDED.bank_account_holder`
	// KHÔNG đụng tới bank_account_number_enc: entity không mang số đầy đủ,
	// nên ghi nó từ đây chỉ có thể ghi chuỗi rỗng đè lên bản mã đang có.
	// Số tài khoản chỉ được ghi qua LuuKemTaiKhoan.

	var approvedAt *time.Time
	if t := sel.ApprovedAt(); !t.IsZero() {
		approvedAt = &t
	}

	_, err := ex.Exec(ctx, q,
		sel.ID().String(), sel.Name(), sel.Slug(), string(sel.Type()), string(sel.Status()),
		sel.LegalName(), sel.TaxCode(), sel.Email(), sel.Phone(),
		sel.CommissionRate().Value(), sel.BankAccountVerified(), sel.SuspensionReason(),
		sel.ApprovedBy(), approvedAt, sel.CreatedAt(), sel.UpdatedAt(),
		sel.BankAccount().BankCode, sel.BankAccount().Last4, sel.BankAccount().Holder)
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

// ---------------------------------------------------- Tài khoản ngân hàng

// LuuKemTaiKhoan ghi nhà bán VÀ số tài khoản đã mã hóa, trong MỘT giao dịch.
//
// Số đầy đủ đi thẳng từ tham số vào bản mã, không đi qua entity — xem
// domain.TaiKhoanNganHang để biết vì sao.
//
// Hai lượt ghi rời sẽ để lại nhà bán không có tài khoản khi lượt sau hỏng,
// và ràng buộc `seller_verified_needs_account` sẽ chặn mọi lần xác minh
// sau đó mà không ai hiểu vì sao.
func (s *Store) LuuKemTaiKhoan(
	ctx context.Context, sel *domain.Seller, soDayDu string,
) error {
	if strings.TrimSpace(soDayDu) == "" {
		return domain.ErrNoBankAccount
	}
	if s.maHoa == nil {
		// Thà từ chối còn hơn ghi số tài khoản ở dạng rõ.
		return errors.New(
			"seller: chưa cấu hình khóa mã hóa (ENCRYPTION_KEY), " +
				"không nhận được tài khoản ngân hàng")
	}

	kin, err := s.maHoa.MaHoa(strings.TrimSpace(soDayDu))
	if err != nil {
		return fmt.Errorf("seller: mã hóa số tài khoản: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("seller: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := save(ctx, tx, sel); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE seller SET bank_account_number_enc = $2 WHERE id = $1`,
		sel.ID().String(), kin); err != nil {
		return fmt.Errorf("seller: ghi số tài khoản: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("seller: xác nhận giao dịch: %w", err)
	}
	return nil
}

// LaySoTaiKhoan giải mã và trả về số tài khoản ĐẦY ĐỦ.
//
// # Đây là chỗ DUY NHẤT giải mã
//
// Chỉ đường chi trả được gọi. Mọi màn hình hiển thị dùng bốn số cuối trong
// entity, và không cần tới hàm này.
//
// Bên gọi có trách nhiệm ghi audit: đọc số tài khoản của một nhà bán là
// truy cập dữ liệu nhạy cảm, và nó phải đếm được ai đọc, lúc nào, vì sao.
func (s *Store) LaySoTaiKhoan(ctx context.Context, id ids.ID) (string, error) {
	if s.maHoa == nil {
		return "", errors.New("seller: chưa cấu hình khóa mã hóa (ENCRYPTION_KEY)")
	}

	var kin string
	err := s.pool.QueryRow(ctx,
		`SELECT bank_account_number_enc FROM seller WHERE id = $1`,
		id.String()).Scan(&kin)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("seller: đọc số tài khoản: %w", err)
	}
	if kin == "" {
		return "", domain.ErrNoBankAccount
	}
	return s.maHoa.GiaiMa(kin)
}
