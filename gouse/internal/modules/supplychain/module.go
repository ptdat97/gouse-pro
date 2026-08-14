package supplychain

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/supplychain/domain"
	scpg "github.com/fashion-commerce/platform/internal/modules/supplychain/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// Module là cài đặt của API công khai.
type Module struct {
	store *scpg.SignalStore
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Toàn bộ giá trị của module nằm ở chỗ dữ liệu SỐNG SÓT. Một bản
	// in-memory sẽ mất sạch khi khởi động lại — tức là mất đúng thứ module
	// này tồn tại để giữ.
	Storage string

	DB *database.DB
}

// New khởi tạo module supply-chain.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"supplychain: chỉ hỗ trợ kho lưu trữ postgres — dữ liệu lịch sử " +
				"mất là mất vĩnh viễn, không tạo ngược được")
	}
	if cfg.DB == nil {
		return nil, errors.New("supplychain: bắt buộc phải có kết nối database")
	}

	return &Module{store: scpg.NewSignalStore(cfg.DB.Pool())}, nil
}

// ---------------------------------------------------------------- API

func (m *Module) RecordSignal(ctx context.Context, req SignalRequest) error {
	return m.RecordSignals(ctx, []SignalRequest{req})
}

// RecordSignals ghi nhiều tín hiệu trong một lượt đi database.
//
// Nếu ngữ cảnh mang giao dịch của dispatcher event, ghi bằng giao dịch đó:
// việc ghi tín hiệu và việc đánh dấu event đã xử lý phải cùng thành công
// hoặc cùng thất bại.
func (m *Module) RecordSignals(ctx context.Context, reqs []SignalRequest) error {
	if len(reqs) == 0 {
		return nil
	}

	signals := make([]*domain.Signal, 0, len(reqs))
	for _, req := range reqs {
		sig, err := toSignal(req)
		if err != nil {
			return err
		}
		signals = append(signals, sig)
	}

	return m.storeFor(ctx).AppendBatch(ctx, signals)
}

func (m *Module) CountSignals(
	ctx context.Context, from, to string,
) (map[string]int, error) {
	fromT, err := parseTime(from)
	if err != nil {
		return nil, err
	}
	toT, err := parseTime(to)
	if err != nil {
		return nil, err
	}

	counts, err := m.store.CountByType(ctx, fromT, toT)
	if err != nil {
		return nil, err
	}

	out := make(map[string]int, len(counts))
	for t, n := range counts {
		out[string(t)] = n
	}
	return out, nil
}

// storeFor chọn kho theo ngữ cảnh.
//
// Trong ngữ cảnh của dispatcher event thì dùng GIAO DỊCH của nó; ngoài ra
// dùng pool. Nhờ vậy cùng một hàm phục vụ được cả hai đường gọi mà bên gọi
// không phải biết mình đang ở đâu.
func (m *Module) storeFor(ctx context.Context) *scpg.SignalStore {
	if tx, ok := eventbus.TxFrom(ctx); ok {
		return scpg.NewSignalStore(tx)
	}
	return m.store
}

// ---------------------------------------------------------------- Chuyển đổi

func toSignal(req SignalRequest) (*domain.Signal, error) {
	occurredAt, err := parseTime(req.OccurredAt)
	if err != nil {
		return nil, err
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	sig, err := domain.NewSignal(domain.NewSignalParams{
		Type:       domain.SignalType(req.Type),
		SKUID:      ids.ID(req.SKUID),
		ProductID:  ids.ID(req.ProductID),
		CategoryID: ids.ID(req.CategoryID),
		SearchTerm: req.SearchTerm,
		Quantity:   req.Quantity,
		OccurredAt: occurredAt,
		SourceType: req.SourceType,
		SourceID:   ids.ID(req.SourceID),
		Metadata:   req.Metadata,
	})
	if err != nil {
		return nil, ErrInvalidInput
	}
	return sig, nil
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, ErrInvalidInput
	}
	return t, nil
}
