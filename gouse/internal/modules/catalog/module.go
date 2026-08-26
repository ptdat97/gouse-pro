package catalog

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/application"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
	"github.com/fashion-commerce/platform/internal/modules/catalog/infrastructure/inmemory"
	"github.com/fashion-commerce/platform/internal/modules/catalog/infrastructure/postgres"
	cataloghttp "github.com/fashion-commerce/platform/internal/modules/catalog/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Module là cài đặt của API công khai.
//
// Nó CHUYỂN ĐỔI giữa domain object (nội bộ) và DTO (công khai). Lớp chuyển
// đổi này là chỗ ranh giới module được thực thi: domain object không bao giờ
// rời khỏi package con.
type Module struct {
	svc *application.Service
}

// Bảo đảm lúc biên dịch rằng Module thỏa mãn API.
var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage chọn kho lưu trữ: "memory" hoặc "postgres".
	//
	// Kho in-memory cho phép chạy và kiểm chứng mô hình domain khi chưa có
	// database — mẫu adapter giả từ Flamingo.
	Storage string

	// DB là kết nối database. BẮT BUỘC khi Storage = "postgres".
	DB *database.DB

	// Clock cho phép test kiểm soát thời gian. Nil = đồng hồ hệ thống.
	Clock application.Clock
}

// New khởi tạo module catalog.
func New(cfg Config) (*Module, error) {
	switch cfg.Storage {
	case "", "memory":
		return &Module{svc: application.NewService(application.Deps{
			Brands:      inmemory.NewBrandStore(),
			Auths:       inmemory.NewAuthorizationStore(),
			Collections: inmemory.NewCollectionStore(),
			Categories:  inmemory.NewCategoryStore(),
			SizeCharts:  inmemory.NewSizeChartStore(),
			Clock:       cfg.Clock,
		})}, nil

	case "postgres":
		// Cùng các port, khác cài đặt: domain và application KHÔNG đổi một
		// dòng nào khi chuyển kho lưu trữ. Đây là điểm mà kiến trúc ports
		// & adapters trả lại giá trị.
		if cfg.DB == nil {
			return nil, errors.New("catalog: kho lưu trữ postgres cần kết nối database")
		}
		pool := cfg.DB.Pool()
		return &Module{svc: application.NewService(application.Deps{
			Brands:      postgres.NewBrandStore(pool),
			Auths:       postgres.NewAuthorizationStore(pool),
			Collections: postgres.NewCollectionStore(pool),
			Categories:  postgres.NewCategoryStore(pool),
			SizeCharts:  postgres.NewSizeChartStore(pool),
			Clock:       cfg.Clock,
		})}, nil

	default:
		return nil, errors.New("catalog: kho lưu trữ không hợp lệ: " + cfg.Storage)
	}
}

// RegisterRoutes gắn các endpoint HTTP của module vào mux.
//
// Module TỰ đăng ký route của mình. cmd/api không biết đường dẫn, hình dạng
// response, hay việc module có tầng application — nó chỉ trao mux.
//
// Vì sao không lộ Service() ra ngoài: nếu cmd/api cầm được *application.Service,
// nó gọi thẳng use case và nhận về domain object, vòng qua API công khai.
// Ranh giới module sẽ chỉ còn là quy ước chứ không phải điều kiện biên dịch.
// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
//
// Không xuất khẩu ra ngoài package catalog — module khác chỉ dùng API.
// Cùng mẫu với product, seller và marketplace.
func (m *Module) Service() *application.Service { return m.svc }

func (m *Module) RegisterRoutes(mux *http.ServeMux, log *slog.Logger) {
	cataloghttp.NewHandler(m.svc, log).Register(mux)
}

// ---------------------------------------------------------------- API

func (m *Module) GetBrand(ctx context.Context, brandID string) (*BrandView, error) {
	id, err := ids.Parse(brandID, ids.PrefixBrand)
	if err != nil {
		return nil, ErrInvalidID
	}
	b, err := m.svc.GetBrand(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toBrandView(b)
	return &v, nil
}

func (m *Module) GetBrandsByIDs(ctx context.Context, brandIDs []string) (map[string]BrandView, error) {
	parsed := make([]ids.ID, 0, len(brandIDs))
	for _, raw := range brandIDs {
		id, err := ids.Parse(raw, ids.PrefixBrand)
		if err != nil {
			// Bỏ qua id sai định dạng thay vì làm hỏng cả lời gọi:
			// hiển thị 49/50 sản phẩm tốt hơn là cả trang lỗi.
			continue
		}
		parsed = append(parsed, id)
	}

	found, err := m.svc.GetBrandsByIDs(ctx, parsed)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]BrandView, len(found))
	for id, b := range found {
		out[id.String()] = toBrandView(b)
	}
	return out, nil
}

func (m *Module) CanSellerCreateOffer(ctx context.Context, brandID, sellerID string) (SellPermission, error) {
	bID, err := ids.Parse(brandID, ids.PrefixBrand)
	if err != nil {
		return SellPermission{}, ErrInvalidID
	}
	sID, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return SellPermission{}, ErrInvalidID
	}

	res, err := m.svc.CanSellerSellBrand(ctx, bID, sID)
	if err != nil {
		return SellPermission{}, translateErr(err)
	}
	return SellPermission{
		Allowed:         res.Allowed,
		Reason:          res.Reason,
		ProtectionLevel: string(res.ProtectionLevel),
		RequiredAction:  res.RequiredAction,
	}, nil
}

func (m *Module) GetCollection(ctx context.Context, collectionID string) (*CollectionView, error) {
	id, err := ids.Parse(collectionID, ids.PrefixCollection)
	if err != nil {
		return nil, ErrInvalidID
	}
	c, err := m.svc.GetCollection(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toCollectionView(c, m.svc.Now())
	return &v, nil
}

func (m *Module) GetCategoryTree(ctx context.Context) ([]CategoryView, error) {
	nodes, err := m.svc.GetCategoryTree(ctx)
	if err != nil {
		return nil, translateErr(err)
	}
	return toCategoryViews(nodes), nil
}

func (m *Module) GetCategory(ctx context.Context, categoryID string) (*CategoryView, error) {
	id, err := ids.Parse(categoryID, ids.PrefixCategory)
	if err != nil {
		return nil, ErrInvalidID
	}
	c, err := m.svc.GetCategory(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toCategoryView(c)
	return &v, nil
}

func (m *Module) GetSizeChart(ctx context.Context, sizeChartID string) (*SizeChartView, error) {
	id, err := ids.Parse(sizeChartID, ids.PrefixSizeChart)
	if err != nil {
		return nil, ErrInvalidID
	}
	sc, err := m.svc.GetSizeChart(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toSizeChartView(sc)
	return &v, nil
}

func (m *Module) GetSizeChartFor(ctx context.Context, brandID, productType string) (*SizeChartView, error) {
	id, err := ids.Parse(brandID, ids.PrefixBrand)
	if err != nil {
		return nil, ErrInvalidID
	}
	sc, err := m.svc.GetSizeChartFor(ctx, id, domain.ProductType(productType))
	if err != nil {
		return nil, translateErr(err)
	}
	v := toSizeChartView(sc)
	return &v, nil
}

// ---------------------------------------------------------------- Chuyển đổi

func toBrandView(b *domain.Brand) BrandView {
	return BrandView{
		ID:              b.ID().String(),
		Name:            b.Name(),
		Slug:            b.Slug(),
		Description:     b.Description(),
		LogoURL:         b.LogoURL(),
		BrandType:       string(b.Type()),
		ProtectionLevel: string(b.ProtectionLevel()),
		CountryOfOrigin: b.CountryOfOrigin(),
		Status:          string(b.Status()),
		IsOwnBrand:      b.IsOwnBrand(),
	}
}

// toCollectionView cần `now` vì WeeksRemaining phụ thuộc thời điểm hỏi.
// Thời gian đến từ đồng hồ của service, không phải time.Now() — nếu không,
// test không kiểm soát được kết quả.
func toCollectionView(c *domain.Collection, now time.Time) CollectionView {
	return CollectionView{
		ID:             c.ID().String(),
		BrandID:        c.BrandID().String(),
		Name:           c.Name(),
		Slug:           c.Slug(),
		Season:         c.Season(),
		Theme:          c.Theme(),
		LaunchDate:     c.LaunchDate(),
		EndOfSeason:    c.EndOfSeason(),
		Status:         string(c.Status()),
		IsVisible:      c.IsVisibleToCustomer(),
		WeeksRemaining: c.WeeksRemaining(now),
	}
}

func toCategoryView(c *domain.Category) CategoryView {
	return CategoryView{
		ID:           c.ID().String(),
		ParentID:     c.ParentID().String(),
		Name:         c.Name(),
		Slug:         c.Slug(),
		Depth:        c.Depth(),
		DisplayOrder: c.DisplayOrder(),
		Status:       string(c.Status()),
	}
}

func toCategoryViews(nodes []*application.CategoryNode) []CategoryView {
	out := make([]CategoryView, 0, len(nodes))
	for _, n := range nodes {
		v := toCategoryView(n.Category)
		v.Children = toCategoryViews(n.Children)
		out = append(out, v)
	}
	return out
}

func toSizeChartView(sc *domain.SizeChart) SizeChartView {
	entries := sc.Entries()
	out := make([]SizeEntryView, len(entries))
	for i, e := range entries {
		out[i] = SizeEntryView{Size: e.Size, Measurements: e.Measurements}
	}
	return SizeChartView{
		ID:          sc.ID().String(),
		BrandID:     sc.BrandID().String(),
		ProductType: string(sc.ProductType()),
		System:      string(sc.System()),
		Note:        sc.Note(),
		Entries:     out,
	}
}

// translateErr chuyển lỗi domain sang lỗi công khai.
//
// Module khác không được thấy chi tiết lỗi nội bộ của catalog.
func translateErr(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
