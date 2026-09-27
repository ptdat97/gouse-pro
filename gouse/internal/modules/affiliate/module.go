package affiliate

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/affiliate/application"
	affiliatepg "github.com/fashion-commerce/platform/internal/modules/affiliate/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Config là tham số khởi tạo module affiliate.
type Config struct {
	// Storage: chỉ "postgres". Xem `New`.
	Storage string
	DB      *database.DB

	// Order là cổng đọc đơn. BẮT BUỘC.
	Order OrderPort

	Log   *slog.Logger
	Clock func() time.Time

	// TyLe là tỷ lệ hoa hồng creator, điểm cơ bản. 0 = mặc định của miền.
	TyLe int
}

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
	log *slog.Logger
}

var _ API = (*Module)(nil)

// New khởi tạo module affiliate.
//
// CHỈ postgres, và đó không phải sự lười: chống quy công trùng dựa vào
// `UNIQUE (order_line_id)` ở tầng database. Một kho trong bộ nhớ sẽ phải cài
// lại ràng buộc ấy, và bản cài lại sẽ không có ràng buộc thật — hai lượt
// worker chồng nhau cùng thấy "chưa quy công" rồi trả creator hoa hồng gấp
// đôi. Dự án này đã gặp đúng dạng lỗi ấy: một kho trong bộ nhớ lặng lẽ bỏ
// qua bộ lọc (P3-72).
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"affiliate: chỉ hỗ trợ kho lưu trữ postgres — chống quy công " +
				"trùng cần ràng buộc UNIQUE ở tầng database")
	}
	if cfg.DB == nil {
		return nil, errors.New("affiliate: bắt buộc phải có kết nối database")
	}
	if cfg.Order == nil {
		return nil, errors.New(
			"affiliate: bắt buộc phải có cổng đọc đơn — `order.paid` chỉ " +
				"mang tổng tiền, không mang dòng hàng để quy công")
	}

	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}

	svc, err := application.NewService(application.Deps{
		Repo:  affiliatepg.NewStore(cfg.DB.Pool()),
		Don:   &orderAdapter{api: cfg.Order},
		Log:   log,
		Clock: cfg.Clock,
		TyLe:  cfg.TyLe,
	})
	if err != nil {
		return nil, err
	}

	return &Module{svc: svc, log: log}, nil
}

// ---------------------------------------------------------------- API

func (m *Module) SoDuCreator(
	ctx context.Context, creatorID string,
) (map[string]int64, error) {
	tong, err := m.svc.SoDuCreator(ctx, creatorID)
	if err != nil {
		return nil, err
	}

	out := make(map[string]int64, len(tong))
	for st, v := range tong {
		out[string(st)] = v
	}
	return out, nil
}
