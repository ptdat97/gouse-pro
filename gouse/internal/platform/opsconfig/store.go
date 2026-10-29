package opsconfig

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNoKey khi khóa không có trong sổ đăng ký.
	//
	// Sổ đăng ký ĐÓNG là hàng rào chính của gói này: gõ sai tên khóa thì
	// lỗi ngay, không tạo ra một tham số ma mà không đoạn mã nào đọc.
	ErrNoKey = errors.New("opsconfig: khóa không có trong sổ đăng ký")

	// ErrOutOfRange khi giá trị nằm ngoài biên đã khai.
	ErrOutOfRange = errors.New("opsconfig: giá trị ngoài biên cho phép")

	// ErrWrongKind khi giá trị sai kiểu.
	ErrWrongKind = errors.New("opsconfig: giá trị sai kiểu")
)

// Value là một tham số kèm giá trị hiện tại và vết sửa gần nhất.
type Value struct {
	Param Param

	// Current là giá trị đang dùng.
	Current float64

	// IsDefault cho biết chưa ai đặt giá trị này.
	IsDefault bool

	UpdatedAt time.Time
	UpdatedBy string
	Reason    string
}

// Store đọc và ghi tham số vận hành.
//
// # Vì sao có BỘ NHỚ ĐỆM, và vì sao đọc không bao giờ trả lỗi
//
// Tham số này được đọc trên đường phục vụ request. Nếu mỗi lần đọc là một
// truy vấn database thì một trang xem hiệu suất tạo ra bốn truy vấn thừa,
// và tệ hơn: database chậm hay chết sẽ làm hỏng một tính năng vốn chỉ cần
// một con số.
//
// Nên `Read` KHÔNG trả lỗi. Không có giá trị trong bộ đệm thì nó trả GIÁ
// TRỊ MẶC ĐỊNH đã biên dịch vào mã. Hệ thống chạy tiếp bằng con số cũ,
// đúng như trước khi có gói này — hỏng theo hướng an toàn.
type Store struct {
	pool *pgxpool.Pool

	// dem là bộ đệm ĐỌC KHÔNG KHÓA.
	//
	// Con trỏ nguyên tử tới một map CHỈ ĐỌC, thay vì RWMutex quanh một map
	// sửa tại chỗ. Đường đọc chạy trên MỌI request tính hiệu suất; một
	// mutex dùng chung ở đó là điểm tranh chấp cho một thứ gần như không
	// bao giờ đổi.
	//
	// Ghi thì thay CẢ map, không sửa map đang có — nên người đọc luôn thấy
	// một ảnh chụp nhất quán, không thấy trạng thái nửa chừng.
	count atomic.Pointer[map[string]float64]
}

// NewStore tạo store và nạp bộ đệm lần đầu.
//
// Nạp lỗi KHÔNG làm hỏng khởi động: mọi tham số rơi về mặc định, và đó
// đúng là hành vi của hệ thống trước khi có gói này.
func NewStore(ctx context.Context, pool *pgxpool.Pool) *Store {
	s := &Store{pool: pool}
	empty := map[string]float64{}
	s.count.Store(&empty)
	_ = s.Reload(ctx)
	return s
}

// Reload đọc lại toàn bộ tham số từ database vào bộ đệm.
func (s *Store) Reload(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT key, value FROM ops_config`)
	if err != nil {
		return fmt.Errorf("opsconfig: đọc tham số: %w", err)
	}
	defer rows.Close()

	fresh := map[string]float64{}
	for rows.Next() {
		var k string
		var v float64
		if err := rows.Scan(&k, &v); err != nil {
			return fmt.Errorf("opsconfig: đọc dòng tham số: %w", err)
		}
		// BỎ QUA khóa không còn trong sổ đăng ký.
		//
		// Xóa một khóa khỏi mã nguồn mà database vẫn còn dòng cũ là
		// chuyện bình thường khi triển khai lại; dòng đó không được phép
		// làm hỏng việc nạp những khóa còn dùng.
		t, ok := Lookup(k)
		if !ok {
			continue
		}
		// Giá trị ngoài biên cũng bỏ qua: biên có thể siết lại sau khi
		// giá trị đã được đặt, và một giá trị không còn hợp lệ thì mặc
		// định an toàn hơn.
		if t.Validate(v) != nil {
			continue
		}
		fresh[k] = v
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("opsconfig: duyệt tham số: %w", err)
	}

	s.count.Store(&fresh)
	return nil
}

// Read trả giá trị đang dùng của một khóa.
//
// KHÔNG trả lỗi, có chủ ý — xem chú thích của Store. Khóa không có trong
// sổ đăng ký trả 0; đó là lỗi lập trình và sẽ lộ ra ở test, không phải
// tình huống cần xử lý lúc chạy.
func (s *Store) Read(key string) float64 {
	t, ok := Lookup(key)
	if !ok {
		return 0
	}
	if s == nil {
		return t.Default
	}

	count := s.count.Load()
	if count == nil {
		return t.Default
	}
	v, ok := (*count)[key]
	if !ok {
		return t.Default
	}
	return v
}

// ReadDuration đọc một tham số kiểu thời lượng.
func (s *Store) ReadDuration(key string) time.Duration {
	t, ok := Lookup(key)
	if !ok {
		return 0
	}
	return t.Duration(s.Read(key))
}

// ReadInt đọc một tham số kiểu số nguyên.
func (s *Store) ReadInt(key string) int {
	return int(s.Read(key))
}

// List trả mọi tham số kèm giá trị hiện tại, cho giao diện quản trị.
func (s *Store) List(ctx context.Context) ([]Value, error) {
	alreadySet := map[string]struct {
		v      float64
		at     time.Time
		by     string
		reason string
	}{}

	if s != nil && s.pool != nil {
		rows, err := s.pool.Query(ctx,
			`SELECT key, value, updated_at, updated_by, reason FROM ops_config`)
		if err != nil {
			return nil, fmt.Errorf("opsconfig: đọc tham số: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var k, by, reason string
			var v float64
			var at time.Time
			if err := rows.Scan(&k, &v, &at, &by, &reason); err != nil {
				return nil, fmt.Errorf("opsconfig: đọc dòng: %w", err)
			}
			alreadySet[k] = struct {
				v      float64
				at     time.Time
				by     string
				reason string
			}{v, at, by, reason}
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("opsconfig: duyệt: %w", err)
		}
	}

	out := make([]Value, 0, len(registry))
	for _, t := range AllParams() {
		g := Value{Param: t, Current: t.Default, IsDefault: true}
		if d, ok := alreadySet[t.Key]; ok && t.Validate(d.v) == nil {
			g.Current, g.IsDefault = d.v, false
			g.UpdatedAt, g.UpdatedBy, g.Reason = d.at, d.by, d.reason
		}
		out = append(out, g)
	}
	return out, nil
}

// SetInput là một lần đổi tham số.
type SetInput struct {
	Key   string
	Value float64

	// UpdatedBy và Reason BẮT BUỘC.
	//
	// Đổi tham số vận hành là thao tác nhạy cảm: nó đổi cách hệ thống
	// chấm điểm hoặc từ chối, và ảnh hưởng tới người ngoài công ty. Một
	// lần đổi không có người chịu trách nhiệm và không có lý do thì không
	// giải thích được khi nhà bán khiếu nại.
	UpdatedBy string
	Reason    string
}

// Tx là phần giao dịch mà bên gọi cần để ghi vết trong CÙNG giao dịch.
//
// Interface tối thiểu, khai ở đây: gói này không import `audit` để giữ
// platform không có phụ thuộc chéo không cần thiết.
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Set ghi một giá trị mới, chạy `writeAudit` trong CÙNG giao dịch, rồi nạp lại
// bộ đệm.
//
// # Vì sao ghi vết phải nằm TRONG giao dịch
//
// Bản đầu ghi vết bằng `audit.Write` (ngoài giao dịch) rồi mới ghi giá
// trị. Tài liệu của chính `audit.Write` nói rõ nó CHỈ dành cho thao tác
// đọc, và "với thao tác GHI, dùng WriteTx".
//
// Hậu quả của cách làm sai: ghi vết xong mà ghi giá trị hỏng thì nhật ký
// còn lại một dòng nói rằng tham số đã đổi — trong khi nó chưa đổi. Nhật
// ký kiểm toán nói dối còn tệ hơn không có nhật ký, vì người điều tra tin
// vào nó.
//
// # Vì sao KHÓA theo khóa tham số
//
// Giá trị CŨ đi vào vết kiểm toán. Hai quản trị viên đổi cùng một tham số
// cùng lúc mà không khóa thì cả hai cùng đọc giá trị cũ giống nhau, và một
// trong hai dòng nhật ký ghi sai điểm xuất phát — "đổi từ 48 thành 36"
// trong khi thực tế nó đi từ 24.
//
// `pg_advisory_xact_lock` khóa theo TÊN KHÓA chứ không theo hàng, nên nó
// đúng cả khi hàng chưa tồn tại — trường hợp mà `SELECT … FOR UPDATE`
// không khóa được gì.
func (s *Store) Set(
	ctx context.Context, in SetInput,
	writeAudit func(ctx context.Context, tx Tx, giaTriCu float64) error,
) error {
	t, ok := Lookup(in.Key)
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoKey, in.Key)
	}
	if err := t.Validate(in.Value); err != nil {
		return err
	}
	if in.UpdatedBy == "" {
		return errors.New("opsconfig: thiếu người thực hiện")
	}
	if in.Reason == "" {
		return errors.New("opsconfig: thiếu lý do")
	}
	if s == nil || s.pool == nil {
		return errors.New("opsconfig: chưa có kết nối database")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("opsconfig: mở giao dịch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1))`, in.Key); err != nil {
		return fmt.Errorf("opsconfig: giành khóa tham số: %w", err)
	}

	// Đọc giá trị cũ TRONG giao dịch, sau khi đã giành khóa.
	old := t.Default
	var read float64
	switch err := tx.QueryRow(ctx,
		`SELECT value FROM ops_config WHERE key = $1`, in.Key).
		Scan(&read); {
	case err == nil:
		old = read
	case errors.Is(err, pgx.ErrNoRows):
		// Chưa ai đặt: giá trị cũ là mặc định.
	default:
		return fmt.Errorf("opsconfig: đọc giá trị cũ: %w", err)
	}

	if writeAudit != nil {
		if err := writeAudit(ctx, tx, old); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO ops_config (key, value, updated_at, updated_by, reason)
		VALUES ($1, $2, now(), $3, $4)
		ON CONFLICT (key) DO UPDATE
		   SET value = EXCLUDED.value,
		       updated_at = EXCLUDED.updated_at,
		       updated_by = EXCLUDED.updated_by,
		       reason   = EXCLUDED.reason`,
		in.Key, in.Value, in.UpdatedBy, in.Reason); err != nil {
		return fmt.Errorf("opsconfig: ghi tham số: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("opsconfig: xác nhận giao dịch: %w", err)
	}

	// Nạp lại NGAY, không đợi chu kỳ: người vừa đổi sẽ tải lại trang và
	// phải thấy con số mới. Thấy con số cũ khiến họ bấm Lưu lần nữa.
	return s.Reload(ctx)
}

// RunPeriodicReload giữ bộ đệm tươi khi có NHIỀU BẢN SAO cùng chạy.
//
// `Set` chỉ nạp lại bộ đệm của TIẾN TRÌNH ĐANG GHI. Bản sao khác không
// biết gì cho tới lần nạp định kỳ này — nên trong khoảng đó, hai bản sao
// dùng hai giá trị khác nhau.
//
// Đó là đánh đổi CÓ CHỦ Ý: tham số ở đây là chính sách kinh doanh, và một
// khoảng lệch vài chục giây không gây hại. Đổi lại, đường đọc không tốn
// một truy vấn nào. Cách khác — LISTEN/NOTIFY của PostgreSQL — chính xác
// hơn nhưng thêm một kết nối thường trực cho một nhu cầu không cần tới nó.
func (s *Store) RunPeriodicReload(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.Reload(ctx)
		}
	}
}
