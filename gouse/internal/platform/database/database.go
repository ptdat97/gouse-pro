// Package database cung cấp kết nối PostgreSQL dùng chung cho mọi module.
//
// Đây là hạ tầng TRUNG LẬP VỚI DOMAIN (quy tắc R3 của cmd/archcheck): nó
// không biết gì về thương hiệu, sản phẩm hay giá. Module tự viết truy vấn
// của mình trong infrastructure/postgres/ của module đó.
//
// Xem docs/adr/0010-database-layer.md.
package database

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config cấu hình kết nối.
type Config struct {
	// DSN là chuỗi kết nối, ví dụ:
	// postgres://user:pass@host:5432/dbname?sslmode=disable
	DSN string

	// MaxConns giới hạn số kết nối đồng thời.
	//
	// Quá cao thì PostgreSQL tốn bộ nhớ cho mỗi kết nối và context switch
	// nhiều; quá thấp thì request xếp hàng chờ. Mặc định 10 là điểm khởi
	// đầu hợp lý cho một tiến trình API, cần đo lại khi có tải thật.
	MaxConns int32

	// MinConns giữ sẵn một số kết nối để request đầu tiên không phải chờ
	// bắt tay TCP + xác thực.
	MinConns int32

	// MaxConnLifetime giới hạn tuổi thọ kết nối.
	//
	// Cần thiết khi có load balancer hoặc failover: kết nối sống mãi sẽ
	// bám vào một máy chủ đã bị thay thế.
	MaxConnLifetime time.Duration

	// MaxConnIdleTime đóng kết nối rảnh quá lâu để trả tài nguyên về.
	MaxConnIdleTime time.Duration

	// ConnectTimeout giới hạn thời gian chờ khi mở kết nối.
	ConnectTimeout time.Duration
}

// Mặc định hợp lý cho một tiến trình API.
const (
	defaultMaxConns        = 10
	defaultMinConns        = 2
	defaultMaxConnLifetime = time.Hour
	defaultMaxConnIdleTime = 30 * time.Minute
	defaultConnectTimeout  = 5 * time.Second
)

// ErrNoDSN khi thiếu chuỗi kết nối.
var ErrNoDSN = errors.New("database: thiếu DSN")

// DB bọc pool kết nối.
//
// Bọc lại thay vì dùng thẳng *pgxpool.Pool để tầng gọi không phụ thuộc
// trực tiếp vào pgx — đổi driver sau này chỉ sửa package này.
type DB struct {
	pool *pgxpool.Pool

	// daDong đánh dấu pool đã đóng.
	//
	// pgxpool KHÔNG phơi ra trạng thái này, mà `Stat()` sau khi đóng vẫn
	// trả về một bộ số (toàn 0) chứ không báo lỗi. Bộ thu thập chỉ số cần
	// phân biệt được hai chuyện: pool rảnh và pool đã chết. Xem metrics.go.
	//
	// atomic vì Close có thể chạy song song với một lần scrape.
	daDong atomic.Bool
}

// Open mở pool kết nối và kiểm chứng bằng một lần ping.
//
// Ping ngay lúc khởi động là CÓ CHỦ ĐÍCH: pgxpool tạo kết nối lười, nên
// không ping thì tiến trình khởi động "thành công" rồi mới đổ vỡ ở request
// đầu tiên của khách. Thà không khởi động được còn hơn khởi động rồi hỏng.
func Open(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		return nil, ErrNoDSN
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		// KHÔNG bọc lỗi gốc vào message trả ra ngoài: DSN chứa mật khẩu.
		return nil, errors.New("database: DSN không hợp lệ")
	}

	poolCfg.MaxConns = orDefaultInt32(cfg.MaxConns, defaultMaxConns)
	poolCfg.MinConns = orDefaultInt32(cfg.MinConns, defaultMinConns)
	poolCfg.MaxConnLifetime = orDefaultDuration(cfg.MaxConnLifetime, defaultMaxConnLifetime)
	poolCfg.MaxConnIdleTime = orDefaultDuration(cfg.MaxConnIdleTime, defaultMaxConnIdleTime)
	poolCfg.ConnConfig.ConnectTimeout = orDefaultDuration(cfg.ConnectTimeout, defaultConnectTimeout)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: không tạo được pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, poolCfg.ConnConfig.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: không kết nối được: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Pool trả về pool bên dưới cho tầng infrastructure của module.
//
// CHỈ infrastructure/postgres/ của từng module được dùng. Tầng application
// và domain KHÔNG được chạm tới — quy tắc R2 và R8 cưỡng chế điều đó.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

// Close đóng toàn bộ kết nối.
func (db *DB) Close() {
	if db.pool != nil {
		db.daDong.Store(true)
		db.pool.Close()
	}
}

// DaDong cho biết pool đã đóng chưa.
func (db *DB) DaDong() bool { return db.daDong.Load() }

// Ping kiểm tra kết nối còn sống không.
//
// Dùng cho health check `ready`. KHÔNG dùng cho `live`: một sự cố database
// ngắn sẽ khiến bộ điều phối khởi động lại toàn bộ tiến trình, làm sự cố
// nặng thêm thay vì nhẹ đi.
func (db *DB) Ping(ctx context.Context) error {
	if db.pool == nil {
		return errors.New("database: chưa mở kết nối")
	}
	return db.pool.Ping(ctx)
}

// Stats trả về số liệu pool để giám sát.
//
// Số kết nối đang dùng chạm trần MaxConns kéo dài là dấu hiệu request đang
// xếp hàng chờ — cần tăng pool hoặc tìm truy vấn chậm.
type Stats struct {
	AcquiredConns int32
	IdleConns     int32
	TotalConns    int32
	MaxConns      int32
}

func (db *DB) Stats() Stats {
	if db.pool == nil {
		return Stats{}
	}
	s := db.pool.Stat()
	return Stats{
		AcquiredConns: s.AcquiredConns(),
		IdleConns:     s.IdleConns(),
		TotalConns:    s.TotalConns(),
		MaxConns:      s.MaxConns(),
	}
}

func orDefaultInt32(v, def int32) int32 {
	if v <= 0 {
		return def
	}
	return v
}

func orDefaultDuration(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}
