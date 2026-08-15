package product

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog"
	"github.com/fashion-commerce/platform/internal/modules/product/application"
	"github.com/fashion-commerce/platform/internal/modules/product/domain"
	"github.com/fashion-commerce/platform/internal/modules/product/infrastructure/inmemory"
	productpg "github.com/fashion-commerce/platform/internal/modules/product/infrastructure/postgres"
	producthttp "github.com/fashion-commerce/platform/internal/modules/product/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// Module là cài đặt của API công khai.
//
// Nó CHUYỂN ĐỔI giữa domain object (nội bộ) và DTO (công khai). Lớp chuyển
// đổi này là chỗ ranh giới module được thực thi: domain object không bao
// giờ rời khỏi package con.
type Module struct {
	svc *application.Service
}

// Bảo đảm lúc biên dịch rằng Module thỏa mãn API.
var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage chọn kho lưu trữ: "memory" hoặc "postgres".
	Storage string

	// Catalog là module catalog. BẮT BUỘC — product cần nó để kiểm tra
	// thương hiệu và quyền bán.
	Catalog catalog.API

	// DB là kết nối database. BẮT BUỘC khi Storage = "postgres".
	DB *database.DB

	// Clock cho phép test kiểm soát thời gian. Nil = đồng hồ hệ thống.
	Clock application.Clock

	// Events là nơi phát tín hiệu tìm kiếm không ra kết quả.
	//
	// Có thể nil: tìm kiếm vẫn chạy, chỉ là không ghi tín hiệu. Không chặn
	// tìm kiếm vì mất một tín hiệu nhẹ hơn mất khả năng tìm sản phẩm.
	Events *eventbus.Outbox
}

// New khởi tạo module product.
func New(cfg Config) (*Module, error) {
	if cfg.Catalog == nil {
		// Thiếu catalog thì hàng rào chống hàng giả không hoạt động. Thà
		// không khởi động được còn hơn chạy với hàng rào đã tắt.
		return nil, errors.New("product: bắt buộc phải có module catalog")
	}

	switch cfg.Storage {
	case "", "memory":
		return &Module{svc: application.NewService(application.Deps{
			Products: inmemory.NewProductStore(),
			Catalog:  &catalogAdapter{api: cfg.Catalog},
			Clock:    cfg.Clock,
		})}, nil

	case "postgres":
		// Cùng các port, khác cài đặt: domain và application KHÔNG đổi một
		// dòng nào khi chuyển kho lưu trữ.
		if cfg.DB == nil {
			return nil, errors.New("product: kho lưu trữ postgres cần kết nối database")
		}
		deps := application.Deps{
			Products: productpg.NewProductStore(cfg.DB.Pool()),
			Catalog:  &catalogAdapter{api: cfg.Catalog},
			Clock:    cfg.Clock,
		}
		if cfg.Events != nil {
			deps.SearchSignals = NewSearchSignalPublisher(cfg.Events)
		}
		return &Module{svc: application.NewService(deps)}, nil

	default:
		return nil, errors.New("product: kho lưu trữ không hợp lệ: " + cfg.Storage)
	}
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
//
// Không xuất khẩu ra ngoài package product — module khác chỉ dùng API.
func (m *Module) Service() *application.Service { return m.svc }

// RegisterRoutes gắn các endpoint HTTP của module vào mux.
//
// Module tự đăng ký route của mình. cmd/api KHÔNG cầm được *application.Service,
// nên không thể đi tắt qua tầng application của module khác.
func (m *Module) RegisterRoutes(mux *http.ServeMux, log *slog.Logger) {
	producthttp.NewHandler(m.svc, log).Register(mux)
}

// ---------------------------------------------------------------- Adapter

// catalogAdapter nối catalog.API với application.CatalogPort.
//
// Đây là chỗ DUY NHẤT trong module product biết tới catalog. Tầng
// application chỉ thấy port của chính nó, nên:
//
//   - test của product không cần dựng module catalog
//   - catalog đổi chữ ký chỉ phải sửa ở đây
//   - quy tắc R8 được giữ: application không import module khác
type catalogAdapter struct {
	api catalog.API
}

func (a *catalogAdapter) BrandExists(ctx context.Context, brandID ids.ID) (bool, error) {
	b, err := a.api.GetBrand(ctx, brandID.String())
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) || errors.Is(err, catalog.ErrInvalidID) {
			return false, nil
		}
		return false, err
	}
	// Thương hiệu đã ngừng hoạt động coi như không dùng được: không cho
	// tạo sản phẩm mới mang thương hiệu đó.
	return b.Status == "ACTIVE", nil
}

func (a *catalogAdapter) CanSellerSellBrand(
	ctx context.Context, brandID, sellerID ids.ID,
) (bool, string, error) {
	perm, err := a.api.CanSellerCreateOffer(ctx, brandID.String(), sellerID.String())
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			return false, "BRAND_NOT_FOUND", nil
		}
		return false, "", err
	}
	return perm.Allowed, perm.Reason, nil
}

func (a *catalogAdapter) SizeChartExistsFor(
	ctx context.Context, brandID ids.ID, productType string,
) (ids.ID, bool, error) {
	sc, err := a.api.GetSizeChartFor(ctx, brandID.String(), productType)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	id, err := ids.Parse(sc.ID, ids.PrefixSizeChart)
	if err != nil {
		return "", false, nil
	}
	return id, true, nil
}

// ---------------------------------------------------------------- API

func (m *Module) GetProduct(ctx context.Context, productID string) (*ProductView, error) {
	id, err := ids.Parse(productID, ids.PrefixProduct)
	if err != nil {
		return nil, ErrInvalidID
	}
	p, err := m.svc.GetProduct(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toProductView(p)
	return &v, nil
}

func (m *Module) GetProductsByIDs(ctx context.Context, productIDs []string) (map[string]ProductView, error) {
	parsed := make([]ids.ID, 0, len(productIDs))
	for _, raw := range productIDs {
		id, err := ids.Parse(raw, ids.PrefixProduct)
		if err != nil {
			// Bỏ qua id sai định dạng thay vì làm hỏng cả lời gọi:
			// hiển thị 19/20 món trong giỏ tốt hơn là cả trang lỗi.
			continue
		}
		parsed = append(parsed, id)
	}

	found, err := m.svc.GetProductsByIDs(ctx, parsed)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]ProductView, len(found))
	for id, p := range found {
		out[id.String()] = toProductView(p)
	}
	return out, nil
}

func (m *Module) GetProductsBySKUIDs(ctx context.Context, skuIDs []string) (map[string]ProductView, error) {
	parsed := make([]ids.ID, 0, len(skuIDs))
	for _, raw := range skuIDs {
		id, err := ids.Parse(raw, ids.PrefixSKU)
		if err != nil {
			continue
		}
		parsed = append(parsed, id)
	}

	found, err := m.svc.GetProductsBySKUIDs(ctx, parsed)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]ProductView, len(found))
	for skuID, p := range found {
		out[skuID.String()] = toProductView(p)
	}
	return out, nil
}

func (m *Module) GetVariantsByProduct(ctx context.Context, productID string) ([]VariantView, error) {
	id, err := ids.Parse(productID, ids.PrefixProduct)
	if err != nil {
		return nil, ErrInvalidID
	}
	p, err := m.svc.GetProduct(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return toVariantViews(p.Variants()), nil
}

func (m *Module) GetSKUsByProduct(ctx context.Context, productID string) ([]SKUView, error) {
	id, err := ids.Parse(productID, ids.PrefixProduct)
	if err != nil {
		return nil, ErrInvalidID
	}
	p, err := m.svc.GetProduct(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return toSKUViews(p.SKUs()), nil
}

func (m *Module) IsSellable(ctx context.Context, skuID string) (bool, error) {
	id, err := ids.Parse(skuID, ids.PrefixSKU)
	if err != nil {
		return false, ErrInvalidID
	}

	found, err := m.svc.GetProductsBySKUIDs(ctx, []ids.ID{id})
	if err != nil {
		return false, translateErr(err)
	}
	p, ok := found[id]
	if !ok {
		return false, ErrNotFound
	}

	// Bán được cần CẢ HAI: sản phẩm đang hiển thị VÀ SKU còn kinh doanh.
	// Thiếu một trong hai thì offer trỏ vào hàng không bán được.
	if !p.IsVisibleToCustomer() {
		return false, nil
	}
	for _, sku := range p.SKUs() {
		if sku.ID() == id {
			return sku.IsSellable(), nil
		}
	}
	return false, ErrNotFound
}

// ---------------------------------------------------------------- Chuyển đổi

func toProductView(p *domain.Product) ProductView {
	return ProductView{
		ID:                  p.ID().String(),
		BrandID:             p.BrandID().String(),
		CollectionID:        p.CollectionID().String(),
		CategoryID:          p.CategoryID().String(),
		SizeChartID:         p.SizeChartID().String(),
		Name:                p.Name(),
		Slug:                p.Slug(),
		Description:         p.Description(),
		CareInstructions:    p.CareInstructions(),
		MaterialComposition: p.MaterialComposition(),
		OriginCountry:       p.OriginCountry(),
		ProductType:         string(p.Type()),
		GenderTarget:        string(p.GenderTarget()),
		Status:              string(p.Status()),
		IsVisible:           p.IsVisibleToCustomer(),
		SellerID:            p.CreatedBySellerID().String(),
		Images:              p.Images(),
		Variants:            toVariantViews(p.Variants()),
	}
}

func toVariantViews(list []*domain.Variant) []VariantView {
	out := make([]VariantView, 0, len(list))
	for _, v := range list {
		out = append(out, VariantView{
			ID:         v.ID().String(),
			ProductID:  v.ProductID().String(),
			Attributes: v.Attributes(),
			Color:      v.Color(),
			Size:       v.Size(),
			Images:     v.Images(),
			Status:     string(v.Status()),
			SKUs:       toSKUViews(v.SKUs()),
		})
	}
	return out
}

func toSKUViews(list []*domain.SKU) []SKUView {
	out := make([]SKUView, 0, len(list))
	for _, s := range list {
		d := s.Dimensions()
		out = append(out, SKUView{
			ID:         s.ID().String(),
			VariantID:  s.VariantID().String(),
			Code:       s.Code(),
			Barcode:    s.Barcode(),
			WeightGram: s.WeightGram(),
			LengthMM:   d.LengthMM,
			WidthMM:    d.WidthMM,
			HeightMM:   d.HeightMM,
			Status:     string(s.Status()),
			IsSellable: s.IsSellable(),
			CanShip:    s.CanShip(),
		})
	}
	return out
}

// translateErr chuyển lỗi domain sang lỗi công khai.
//
// Module khác không được thấy chi tiết lỗi nội bộ của product.
func translateErr(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
