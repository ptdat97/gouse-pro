package marketplace

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/catalog"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
	marketpg "github.com/fashion-commerce/platform/internal/modules/marketplace/infrastructure/postgres"
	markethttp "github.com/fashion-commerce/platform/internal/modules/marketplace/interfaces/http"
	"github.com/fashion-commerce/platform/internal/modules/product"
	"github.com/fashion-commerce/platform/internal/modules/seller"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
//
// Marketplace là module PHỤ THUỘC NHIỀU NHẤT: nó cần catalog (kiểm tra
// thương hiệu), product (tra SKU), seller (trạng thái nhà bán), inventory
// (còn hàng không). Đó là bản chất của nó — nó điều phối lời chào bán.
type Config struct {
	Storage string
	DB      *database.DB

	// Catalog để kiểm tra quyền bán thương hiệu — HÀNG RÀO CHỐNG HÀNG GIẢ.
	Catalog catalog.API

	// Product để tra thương hiệu của SKU và kiểm tra SKU còn kinh doanh.
	Product product.API

	// Seller để kiểm tra trạng thái nhà bán và lấy tỷ lệ hoa hồng.
	Seller seller.API

	// Inventory để biết offer còn hàng không.
	//
	// Có thể nil: khi đó buy box KHÔNG lọc theo tồn kho. Chỉ chấp nhận
	// được ở môi trường phát triển.
	Inventory inventory.API

	Clock application.Clock
}

// New khởi tạo module marketplace.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New("marketplace: chỉ hỗ trợ kho lưu trữ postgres")
	}
	if cfg.DB == nil {
		return nil, errors.New("marketplace: bắt buộc phải có kết nối database")
	}
	// Thiếu bất kỳ cái nào thì hàng rào chống hàng giả không hoạt động.
	// Thà không khởi động được còn hơn chạy với hàng rào đã tắt.
	if cfg.Catalog == nil {
		return nil, errors.New("marketplace: bắt buộc phải có module catalog")
	}
	if cfg.Product == nil {
		return nil, errors.New("marketplace: bắt buộc phải có module product")
	}
	if cfg.Seller == nil {
		return nil, errors.New("marketplace: bắt buộc phải có module seller")
	}

	pool := cfg.DB.Pool()
	deps := application.Deps{
		Offers:  marketpg.NewOfferStore(pool),
		History: marketpg.NewPriceHistoryStore(pool),
		Catalog: &catalogAdapter{api: cfg.Catalog},
		Product: &productAdapter{api: cfg.Product},
		Seller:  &sellerAdapter{api: cfg.Seller},
		Clock:   cfg.Clock,
	}
	if cfg.Inventory != nil {
		deps.Inventory = &inventoryAdapter{api: cfg.Inventory}
	}

	return &Module{svc: application.NewService(deps)}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// RegisterRoutes gắn các endpoint công khai của module vào mux.
// RegisterSellerRoutes gắn các endpoint offer của NHÀ BÁN.
//
// Bên gọi PHẢI bọc Auth và RequireRole("SELLER_OWNER", "SELLER_STAFF").
func (m *Module) RegisterSellerRoutes(
	mux *http.ServeMux, stock markethttp.StockPort, log *slog.Logger,
) {
	markethttp.NewSellerHandler(m.svc, stock, log).Register(mux)
}

func (m *Module) RegisterRoutes(mux *http.ServeMux, log *slog.Logger) {
	markethttp.NewHandler(m.svc, log).Register(mux)
}

// ---------------------------------------------------------------- Adapter
//
// Bốn adapter dưới đây là chỗ DUY NHẤT trong module biết tới module khác.
// Tầng application chỉ thấy port của chính nó, nên test không cần dựng cả
// bốn module thật.

type catalogAdapter struct{ api catalog.API }

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

type productAdapter struct{ api product.API }

func (a *productAdapter) BrandOfSKU(ctx context.Context, skuID ids.ID) (ids.ID, bool, error) {
	found, err := a.api.GetProductsBySKUIDs(ctx, []string{skuID.String()})
	if err != nil {
		return "", false, err
	}
	p, ok := found[skuID.String()]
	if !ok {
		return "", false, nil
	}
	return ids.ID(p.BrandID), true, nil
}

func (a *productAdapter) IsSKUSellable(ctx context.Context, skuID ids.ID) (bool, error) {
	sellable, err := a.api.IsSellable(ctx, skuID.String())
	if err != nil {
		if errors.Is(err, product.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return sellable, nil
}

func (a *productAdapter) SKUsOfProduct(
	ctx context.Context, productID ids.ID,
) ([]ids.ID, error) {
	skus, err := a.api.GetSKUsByProduct(ctx, productID.String())
	if err != nil {
		if errors.Is(err, product.ErrNotFound) {
			// Sản phẩm không tồn tại trả DANH SÁCH RỖNG, không phải lỗi:
			// tầng gọi phân biệt "không có offer" với "không có sản phẩm"
			// bằng cách khác, và trả 500 ở đây là sai loại lỗi.
			return nil, nil
		}
		return nil, err
	}

	out := make([]ids.ID, 0, len(skus))
	for _, s := range skus {
		out = append(out, ids.ID(s.ID))
	}
	return out, nil
}

type sellerAdapter struct{ api seller.API }

func (a *sellerAdapter) IsActive(ctx context.Context, sellerID ids.ID) (bool, error) {
	active, err := a.api.IsSellerActive(ctx, sellerID.String())
	if err != nil {
		if errors.Is(err, seller.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return active, nil
}

func (a *sellerAdapter) CommissionRate(ctx context.Context, sellerID ids.ID) (types.BasisPoints, error) {
	v, err := a.api.GetSeller(ctx, sellerID.String())
	if err != nil {
		return types.BasisPoints{}, err
	}
	return types.NewBasisPoints(v.CommissionRateBP)
}

type inventoryAdapter struct{ api inventory.API }

func (a *inventoryAdapter) AvailableForSKUs(
	ctx context.Context, skuIDs []ids.ID,
) (map[ids.ID]int, error) {
	strs := make([]string, len(skuIDs))
	for i, id := range skuIDs {
		strs[i] = id.String()
	}

	found, err := a.api.GetAvailability(ctx, strs, "")
	if err != nil {
		return nil, err
	}

	out := make(map[ids.ID]int, len(found))
	for skuID, qty := range found {
		out[ids.ID(skuID)] = qty
	}
	return out, nil
}

// ---------------------------------------------------------------- API

func (m *Module) GetOffer(ctx context.Context, offerID string) (*OfferView, error) {
	id, err := ids.Parse(offerID, ids.PrefixOffer)
	if err != nil {
		return nil, ErrInvalidID
	}
	o, err := m.svc.GetOffer(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toView(o)
	return &v, nil
}

func (m *Module) GetOffersBySKU(ctx context.Context, skuID string) ([]OfferView, error) {
	id, err := ids.Parse(skuID, ids.PrefixSKU)
	if err != nil {
		return nil, ErrInvalidID
	}
	list, err := m.svc.GetOffersBySKU(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return toViews(list), nil
}

func (m *Module) GetOffersBySKUs(
	ctx context.Context, skuIDs []string,
) (map[string][]OfferView, error) {
	parsed := make([]ids.ID, 0, len(skuIDs))
	for _, raw := range skuIDs {
		id, err := ids.Parse(raw, ids.PrefixSKU)
		if err != nil {
			continue
		}
		parsed = append(parsed, id)
	}

	found, err := m.svc.GetOffersBySKUs(ctx, parsed)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string][]OfferView, len(found))
	for skuID, list := range found {
		out[skuID.String()] = toViews(list)
	}
	return out, nil
}

func (m *Module) GetOffersByIDs(
	ctx context.Context, offerIDs []string,
) (map[string]OfferView, error) {
	parsed := make([]ids.ID, 0, len(offerIDs))
	for _, raw := range offerIDs {
		id, err := ids.Parse(raw, ids.PrefixOffer)
		if err != nil {
			// Định danh hỏng được BỎ QUA, không làm hỏng cả lượt gọi: một
			// món lỗi trong giỏ không nên khiến chín món còn lại biến mất.
			continue
		}
		parsed = append(parsed, id)
	}

	found, err := m.svc.GetOffersByIDs(ctx, parsed)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]OfferView, len(found))
	for id, o := range found {
		out[id.String()] = toView(o)
	}
	return out, nil
}

func (m *Module) GetBuyBoxOffer(ctx context.Context, skuID string) (*BuyBoxView, error) {
	id, err := ids.Parse(skuID, ids.PrefixSKU)
	if err != nil {
		return nil, ErrInvalidID
	}

	res, err := m.svc.GetBuyBox(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	if res.Winner == nil {
		// Không offer nào đủ điều kiện — không phải lỗi, chỉ là chưa có
		// ai bán được món này.
		return nil, nil
	}
	v := toBuyBoxView(res)
	return &v, nil
}

func (m *Module) GetBuyBoxOffers(
	ctx context.Context, skuIDs []string,
) (map[string]BuyBoxView, error) {
	parsed := make([]ids.ID, 0, len(skuIDs))
	for _, raw := range skuIDs {
		id, err := ids.Parse(raw, ids.PrefixSKU)
		if err != nil {
			continue
		}
		parsed = append(parsed, id)
	}

	found, err := m.svc.GetBuyBoxes(ctx, parsed)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]BuyBoxView, len(found))
	for skuID, res := range found {
		if res.Winner == nil {
			continue
		}
		out[skuID.String()] = toBuyBoxView(res)
	}
	return out, nil
}

func (m *Module) GetCommissionRate(ctx context.Context, sellerID string) (int32, error) {
	id, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return 0, ErrInvalidID
	}
	rate, err := m.svc.GetCommissionRate(ctx, id)
	if err != nil {
		return 0, translateErr(err)
	}
	return rate.Value(), nil
}

func (m *Module) CanSellerCreateOffer(
	ctx context.Context, sellerID, skuID string,
) (bool, string, error) {
	sID, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return false, "", ErrInvalidID
	}
	kID, err := ids.Parse(skuID, ids.PrefixSKU)
	if err != nil {
		return false, "", ErrInvalidID
	}

	err = m.svc.CheckCanCreateOffer(ctx, sID, kID)
	switch {
	case err == nil:
		return true, "OK", nil
	case errors.Is(err, application.ErrSKUNotFound):
		return false, "SKU_NOT_FOUND", nil
	case errors.Is(err, application.ErrSKUNotSellable):
		return false, "SKU_NOT_SELLABLE", nil
	case errors.Is(err, application.ErrSellerInactive):
		return false, "SELLER_INACTIVE", nil
	}

	var authErr *application.NotAuthorizedError
	if errors.As(err, &authErr) {
		return false, authErr.Reason, nil
	}
	return false, "", err
}

// ---------------------------------------------------------------- Chuyển đổi

func toView(o *domain.Offer) OfferView {
	return OfferView{
		ID:                o.ID().String(),
		SKUID:             o.SKUID().String(),
		SellerID:          o.SellerID().String(),
		PriceAmount:       o.Price().Amount(),
		PriceCurrency:     string(o.Price().Currency()),
		CompareAtAmount:   o.CompareAt().Amount(),
		Condition:         string(o.Condition()),
		HandlingTimeHours: o.HandlingTimeHours(),
		MinOrderQuantity:  o.MinOrderQuantity(),
		MaxOrderQuantity:  o.MaxOrderQuantity(),
		Status:            string(o.Status()),
		IsSellable:        o.IsSellable(),
	}
}

func toViews(list []*domain.Offer) []OfferView {
	out := make([]OfferView, 0, len(list))
	for _, o := range list {
		out = append(out, toView(o))
	}
	return out
}

func toBuyBoxView(r domain.BuyBoxResult) BuyBoxView {
	return BuyBoxView{
		Offer:            toView(r.Winner),
		Score:            r.Score,
		OtherOffersCount: r.OtherCount,
	}
}

func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrDuplicateActiveOffer):
		return ErrDuplicateActiveOffer
	case errors.Is(err, application.ErrNotAuthorized):
		return ErrNotAuthorized
	}
	return err
}
