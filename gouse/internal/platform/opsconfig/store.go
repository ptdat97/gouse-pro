package opsconfig

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrKhongCoKhoa khi khóa không có trong sổ đăng ký.
	//
	// Sổ đăng ký ĐÓNG là hàng rào chính của gói này: gõ sai tên khóa thì
	// lỗi ngay, không tạo ra một tham số ma mà không đoạn mã nào đọc.
	ErrKhongCoKhoa = errors.New("opsconfig: khóa không có trong sổ đăng ký")

	// ErrNgoaiBien khi giá trị nằm ngoài biên đã khai.
	ErrNgoaiBien = errors.New("opsconfig: giá trị ngoài biên cho phép")

	// ErrSaiKieu khi giá trị sai kiểu.
	ErrSaiKieu = errors.New("opsconfig: giá trị sai kiểu")
)

// GiaTri là một tham số kèm giá trị hiện tại và vết sửa gần nhất.
type GiaTri struct {
	Tham ThamSo

	// HienTai là giá trị đang dùng.
	HienTai float64

	// LaMacDinh cho biết chưa ai đặt giá trị này.
	LaMacDinh bool

	SuaLuc time.Time
	SuaBoi string
	LyDo   string
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
// Nên `Doc` KHÔNG trả lỗi. Không có giá trị trong bộ đệm thì nó trả GIÁ
// TRỊ MẶC ĐỊNH đã biên dịch vào mã. Hệ thống chạy tiếp bằng con số cũ,
// đúng như trước khi có gói này — hỏng theo hướng an toàn.
type Store struct {
	pool *pgxpool.Pool

	mu  sync.RWMutex
	dem map[string]float64
}

// NewStore tạo store và nạp bộ đệm lần đầu.
//
// Nạp lỗi KHÔNG làm hỏng khởi động: mọi tham số rơi về mặc định, và đó
// đúng là hành vi của hệ thống trước khi có gói này.
func NewStore(ctx context.Context, pool *pgxpool.Pool) *Store {
	s := &Store{pool: pool, dem: map[string]float64{}}
	_ = s.NapLai(ctx)
	return s
}

// NapLai đọc lại toàn bộ tham số từ database vào bộ đệm.
func (s *Store) NapLai(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT khoa, gia_tri FROM ops_config`)
	if err != nil {
		return fmt.Errorf("opsconfig: đọc tham số: %w", err)
	}
	defer rows.Close()

	moi := map[string]float64{}
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
		t, ok := Tham(k)
		if !ok {
			continue
		}
		// Giá trị ngoài biên cũng bỏ qua: biên có thể siết lại sau khi
		// giá trị đã được đặt, và một giá trị không còn hợp lệ thì mặc
		// định an toàn hơn.
		if t.KiemGiaTri(v) != nil {
			continue
		}
		moi[k] = v
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("opsconfig: duyệt tham số: %w", err)
	}

	s.mu.Lock()
	s.dem = moi
	s.mu.Unlock()
	return nil
}

// Doc trả giá trị đang dùng của một khóa.
//
// KHÔNG trả lỗi, có chủ ý — xem chú thích của Store. Khóa không có trong
// sổ đăng ký trả 0; đó là lỗi lập trình và sẽ lộ ra ở test, không phải
// tình huống cần xử lý lúc chạy.
func (s *Store) Doc(khoa string) float64 {
	t, ok := Tham(khoa)
	if !ok {
		return 0
	}
	if s == nil {
		return t.MacDinh
	}

	s.mu.RLock()
	v, co := s.dem[khoa]
	s.mu.RUnlock()
	if !co {
		return t.MacDinh
	}
	return v
}

// DocThoiLuong đọc một tham số kiểu thời lượng.
func (s *Store) DocThoiLuong(khoa string) time.Duration {
	t, ok := Tham(khoa)
	if !ok {
		return 0
	}
	return t.ThoiLuong(s.Doc(khoa))
}

// DocSoNguyen đọc một tham số kiểu số nguyên.
func (s *Store) DocSoNguyen(khoa string) int {
	return int(s.Doc(khoa))
}

// DanhSach trả mọi tham số kèm giá trị hiện tại, cho giao diện quản trị.
func (s *Store) DanhSach(ctx context.Context) ([]GiaTri, error) {
	daDat := map[string]struct {
		v    float64
		luc  time.Time
		boi  string
		lyDo string
	}{}

	if s != nil && s.pool != nil {
		rows, err := s.pool.Query(ctx,
			`SELECT khoa, gia_tri, sua_luc, sua_boi, ly_do FROM ops_config`)
		if err != nil {
			return nil, fmt.Errorf("opsconfig: đọc tham số: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var k, boi, lyDo string
			var v float64
			var luc time.Time
			if err := rows.Scan(&k, &v, &luc, &boi, &lyDo); err != nil {
				return nil, fmt.Errorf("opsconfig: đọc dòng: %w", err)
			}
			daDat[k] = struct {
				v    float64
				luc  time.Time
				boi  string
				lyDo string
			}{v, luc, boi, lyDo}
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("opsconfig: duyệt: %w", err)
		}
	}

	ra := make([]GiaTri, 0, len(soDangKy))
	for _, t := range MoiThamSo() {
		g := GiaTri{Tham: t, HienTai: t.MacDinh, LaMacDinh: true}
		if d, co := daDat[t.Khoa]; co && t.KiemGiaTri(d.v) == nil {
			g.HienTai, g.LaMacDinh = d.v, false
			g.SuaLuc, g.SuaBoi, g.LyDo = d.luc, d.boi, d.lyDo
		}
		ra = append(ra, g)
	}
	return ra, nil
}

// DatInput là một lần đổi tham số.
type DatInput struct {
	Khoa   string
	GiaTri float64

	// SuaBoi và LyDo BẮT BUỘC.
	//
	// Đổi tham số vận hành là thao tác nhạy cảm: nó đổi cách hệ thống
	// chấm điểm hoặc từ chối, và ảnh hưởng tới người ngoài công ty. Một
	// lần đổi không có người chịu trách nhiệm và không có lý do thì không
	// giải thích được khi nhà bán khiếu nại.
	SuaBoi string
	LyDo   string
}

// Dat ghi một giá trị mới VÀ nạp lại bộ đệm.
//
// Bên gọi (tầng app) chịu trách nhiệm ghi vết kiểm toán trong cùng giao
// dịch — gói này không biết tới audit để giữ platform trung lập.
func (s *Store) Dat(ctx context.Context, in DatInput) error {
	t, ok := Tham(in.Khoa)
	if !ok {
		return fmt.Errorf("%w: %q", ErrKhongCoKhoa, in.Khoa)
	}
	if err := t.KiemGiaTri(in.GiaTri); err != nil {
		return err
	}
	if in.SuaBoi == "" {
		return errors.New("opsconfig: thiếu người thực hiện")
	}
	if in.LyDo == "" {
		return errors.New("opsconfig: thiếu lý do")
	}
	if s == nil || s.pool == nil {
		return errors.New("opsconfig: chưa có kết nối database")
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO ops_config (khoa, gia_tri, sua_luc, sua_boi, ly_do)
		VALUES ($1, $2, now(), $3, $4)
		ON CONFLICT (khoa) DO UPDATE
		   SET gia_tri = EXCLUDED.gia_tri,
		       sua_luc = EXCLUDED.sua_luc,
		       sua_boi = EXCLUDED.sua_boi,
		       ly_do   = EXCLUDED.ly_do`,
		in.Khoa, in.GiaTri, in.SuaBoi, in.LyDo)
	if err != nil {
		return fmt.Errorf("opsconfig: ghi tham số: %w", err)
	}

	// Nạp lại NGAY, không đợi chu kỳ: người vừa đổi sẽ tải lại trang và
	// phải thấy con số mới. Thấy con số cũ khiến họ bấm Lưu lần nữa.
	return s.NapLai(ctx)
}

// ChayNapLaiDinhKy giữ bộ đệm tươi khi có NHIỀU BẢN SAO cùng chạy.
//
// `Dat` chỉ nạp lại bộ đệm của TIẾN TRÌNH ĐANG GHI. Bản sao khác không
// biết gì cho tới lần nạp định kỳ này — nên trong khoảng đó, hai bản sao
// dùng hai giá trị khác nhau.
//
// Đó là đánh đổi CÓ CHỦ Ý: tham số ở đây là chính sách kinh doanh, và một
// khoảng lệch vài chục giây không gây hại. Đổi lại, đường đọc không tốn
// một truy vấn nào. Cách khác — LISTEN/NOTIFY của PostgreSQL — chính xác
// hơn nhưng thêm một kết nối thường trực cho một nhu cầu không cần tới nó.
func (s *Store) ChayNapLaiDinhKy(ctx context.Context, nhip time.Duration) {
	t := time.NewTicker(nhip)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.NapLai(ctx)
		}
	}
}
