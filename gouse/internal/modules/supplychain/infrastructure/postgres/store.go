// Package postgres cài đặt kho tín hiệu nhu cầu bằng PostgreSQL.
//
// LƯU Ý: package này KHÔNG có hàm nào sửa hay xóa tín hiệu. Tín hiệu là
// quan sát về một thời điểm đã qua — sửa nó nghĩa là sửa lịch sử.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/supplychain/domain"
)

// querier cho phép dùng chung cả pool lẫn giao dịch.
//
// Cần thiết vì bên nhận event ghi tín hiệu bằng GIAO DỊCH của dispatcher:
// việc ghi tín hiệu và việc đánh dấu event đã xử lý phải cùng thành công
// hoặc cùng thất bại.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row

	// SendBatch để ghi nhiều tín hiệu trong một lượt đi database.
	//
	// Cả *pgxpool.Pool lẫn pgx.Tx đều có phương thức này, nên khai báo ở
	// đây thay vì ép kiểu lúc chạy — ép kiểu sẽ panic nếu ai đó truyền vào
	// một cài đặt thiếu nó, và panic đó chỉ lộ ra ở production.
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// SignalStore ghi và đọc tín hiệu nhu cầu.
type SignalStore struct {
	db querier
}

func NewSignalStore(db querier) *SignalStore {
	return &SignalStore{db: db}
}

var _ domain.SignalRepository = (*SignalStore)(nil)

const insertSignal = `
	INSERT INTO demand_signal (
		signal_type, sku_id, product_id, category_id, search_term,
		quantity, occurred_at, source_type, source_id, metadata
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

// Append ghi một tín hiệu.
func (s *SignalStore) Append(ctx context.Context, sig *domain.Signal) error {
	meta, err := marshalMetadata(sig.Metadata())
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, insertSignal,
		string(sig.Type()), sig.SKUID().String(), sig.ProductID().String(),
		sig.CategoryID().String(), sig.SearchTerm(),
		sig.Quantity(), sig.OccurredAt(), sig.SourceType(),
		sig.SourceID().String(), meta)
	if err != nil {
		return fmt.Errorf("supplychain: ghi tín hiệu nhu cầu: %w", err)
	}
	return nil
}

// AppendBatch ghi nhiều tín hiệu trong MỘT lượt đi database.
//
// Một đơn ba dòng sinh ba tín hiệu. Ghi từng cái là ba lượt đi cho một sự
// kiện — với bảng ghi nhiều nhất hệ thống, đó là khác biệt đáng kể.
func (s *SignalStore) AppendBatch(ctx context.Context, signals []*domain.Signal) error {
	if len(signals) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, sig := range signals {
		meta, err := marshalMetadata(sig.Metadata())
		if err != nil {
			return err
		}
		batch.Queue(insertSignal,
			string(sig.Type()), sig.SKUID().String(), sig.ProductID().String(),
			sig.CategoryID().String(), sig.SearchTerm(),
			sig.Quantity(), sig.OccurredAt(), sig.SourceType(),
			sig.SourceID().String(), meta)
	}

	results := s.db.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	for i := range signals {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("supplychain: ghi tín hiệu %d: %w", i+1, err)
		}
	}
	return nil
}

// CountByType đếm tín hiệu theo loại trong một khoảng thời gian.
//
// Dùng cho giám sát ở MVP: con số bằng 0 kéo dài nghĩa là đường ghi tín
// hiệu đã hỏng, và mỗi ngày im lặng là một ngày dữ liệu mất vĩnh viễn.
func (s *SignalStore) CountByType(
	ctx context.Context, from, to time.Time,
) (map[domain.SignalType]int, error) {
	return s.countWhere(ctx, `
		SELECT signal_type, count(*)
		  FROM demand_signal
		 WHERE ($1::timestamptz IS NULL OR occurred_at >= $1)
		   AND ($2::timestamptz IS NULL OR occurred_at <= $2)
		 GROUP BY signal_type`,
		nullTime(from), nullTime(to))
}

// CountForSKU đếm tín hiệu của một SKU theo loại.
func (s *SignalStore) CountForSKU(
	ctx context.Context, skuID ids.ID, from, to time.Time,
) (map[domain.SignalType]int, error) {
	return s.countWhere(ctx, `
		SELECT signal_type, count(*)
		  FROM demand_signal
		 WHERE sku_id = $3
		   AND ($1::timestamptz IS NULL OR occurred_at >= $1)
		   AND ($2::timestamptz IS NULL OR occurred_at <= $2)
		 GROUP BY signal_type`,
		nullTime(from), nullTime(to), skuID.String())
}

func (s *SignalStore) countWhere(
	ctx context.Context, query string, args ...any,
) (map[domain.SignalType]int, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("supplychain: đếm tín hiệu: %w", err)
	}
	defer rows.Close()

	out := map[domain.SignalType]int{}
	for rows.Next() {
		var (
			t string
			n int
		)
		if err := rows.Scan(&t, &n); err != nil {
			return nil, fmt.Errorf("supplychain: đếm tín hiệu: %w", err)
		}
		out[domain.SignalType(t)] = n
	}
	return out, rows.Err()
}

func marshalMetadata(m map[string]string) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("supplychain: mã hóa metadata: %w", err)
	}
	return raw, nil
}

// nullTime trả nil cho thời điểm rỗng, để truy vấn bỏ qua điều kiện.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// Pool trả kho dùng với pool kết nối.
func Pool(pool *pgxpool.Pool) *SignalStore { return NewSignalStore(pool) }
