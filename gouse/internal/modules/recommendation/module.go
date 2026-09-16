package recommendation

import (
	"context"
	"errors"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/recommendation/domain"
	recpg "github.com/fashion-commerce/platform/internal/modules/recommendation/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

type Module struct {
	store   *recpg.Store
	product ProductPort
}

var _ API = (*Module)(nil)

// ProductPort tra SKU về (thương hiệu, size).
//
// Chỉ dùng lúc XỬ LÝ EVENT để dựng quan sát, KHÔNG dùng lúc phục vụ
// request: trang chi tiết sản phẩm chỉ đọc read model của module này, nên
// gợi ý không bao giờ làm chậm trang (đặc tả mục 5).
type ProductPort interface {
	// ThuongHieuVaSize trả mã thương hiệu và nhãn size của một SKU.
	ThuongHieuVaSize(ctx context.Context, skuID string) (brandID, size string, err error)
}

type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	Storage string

	DB *database.DB

	// Product BẮT BUỘC: không có nó thì không dựng được quan sát nào, và
	// module chạy như thể không tồn tại.
	Product ProductPort
}

func New(cfg Config) (*Module, error) {
	if cfg.Storage != "postgres" {
		return nil, errors.New("recommendation: chỉ hỗ trợ storage=postgres")
	}
	if cfg.DB == nil {
		return nil, errors.New("recommendation: thiếu kết nối database")
	}
	if cfg.Product == nil {
		return nil, errors.New(
			"recommendation: thiếu cổng product — không có nó thì không " +
				"dựng được quan sát size nào")
	}
	return &Module{store: recpg.NewStore(cfg.DB.Pool()), product: cfg.Product}, nil
}

// GoiYSize gợi ý size, hoặc nil khi không đủ căn cứ.
//
// KHÔNG trả lỗi khi thiếu dữ liệu: gợi ý là tính năng TĂNG CƯỜNG, và hỏng
// gợi ý không được làm hỏng trang sản phẩm (đặc tả mục 5). Chỉ lỗi hạ tầng
// thật mới trả lỗi, và bên gọi cũng nên bỏ qua nó.
func (m *Module) GoiYSize(
	ctx context.Context, req GoiYSizeRequest,
) (*GoiYSizeView, error) {
	if strings.TrimSpace(req.CustomerID) == "" ||
		strings.TrimSpace(req.BrandID) == "" || len(req.CacSize) == 0 {
		return nil, nil
	}

	quanSat, err := m.store.TheoKhachVaThuongHieu(ctx, req.CustomerID, req.BrandID)
	if err != nil {
		return nil, err
	}

	goiY, ok := domain.SuyLuanSize(quanSat, req.CacSize)
	if !ok {
		return nil, nil
	}
	return &GoiYSizeView{
		SuggestedSize: goiY.Size,
		Reason:        string(goiY.LyDo),
	}, nil
}

// ghiQuanSat dựng một quan sát từ SKU và ghi xuống.
//
// Bỏ qua im lặng khi không tra được thương hiệu hoặc size: sản phẩm không
// có thuộc tính size (phụ kiện, nước hoa) là chuyện bình thường, và báo lỗi
// sẽ làm event kẹt trong hàng đợi mãi.
func (m *Module) ghiQuanSat(
	ctx context.Context, q domain.QuanSatMoi, skuID string,
) error {
	brandID, size, err := m.product.ThuongHieuVaSize(ctx, skuID)
	if err != nil {
		return err
	}
	if brandID == "" || strings.TrimSpace(size) == "" {
		return nil
	}
	q.BrandID = ids.ID(brandID)
	q.Size = size
	return m.store.Ghi(ctx, q)
}
