package cart

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/cart/domain"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/marketplace"
	"github.com/fashion-commerce/platform/internal/modules/product"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// offerLookup cài đặt domain.OfferLookup bằng cách gọi các module khác.
//
// ĐÂY LÀ CHỖ QUYẾT ĐỊNH HIỆU NĂNG CỦA MODULE (cart.md mục 11): hiển thị
// giỏ 10 món cần dữ liệu từ bốn module. Làm ngây thơ thì mỗi lần khách mở
// giỏ là 40 lượt gọi.
//
// Cách làm ở đây: BỐN lượt gọi theo lô, bất kể giỏ có bao nhiêu món.
//
//  1. marketplace.GetOffersByIDs     → giá, giới hạn số lượng, còn bán không
//  2. product.GetProductsBySKUIDs    → tên sản phẩm, mô tả biến thể, ảnh
//  3. seller.GetSellersByIDs         → seller còn hoạt động không
//  4. inventory.GetAvailability      → số lượng còn (CHỈ để hiển thị)
//
// Lượt gọi thứ tư là chỗ dễ hiểu nhầm nhất: nó KHÔNG giữ hàng, chỉ đọc.
// Con số đọc được có thể sai ngay sau đó — và điều đó chấp nhận được, vì
// giỏ không hứa gì với khách. Cam kết chỉ có ở checkout.
type offerLookup struct {
	marketplace marketplace.API
	product     product.API
	seller      seller.API
	inventory   inventory.API
}

var _ domain.OfferLookup = (*offerLookup)(nil)

func (l *offerLookup) LookupOffers(
	ctx context.Context, offerIDs []ids.ID,
) (map[ids.ID]domain.SyncData, error) {
	out := make(map[ids.ID]domain.SyncData, len(offerIDs))
	if len(offerIDs) == 0 {
		return out, nil
	}

	strIDs := make([]string, 0, len(offerIDs))
	for _, id := range offerIDs {
		strIDs = append(strIDs, id.String())
	}

	offers, err := l.marketplace.GetOffersByIDs(ctx, strIDs)
	if err != nil {
		return nil, err
	}

	// Gom SKU và seller để tra theo lô. Nhiều offer có thể trỏ cùng một
	// SKU hoặc cùng một seller, nên dùng tập hợp thay vì lát cắt.
	skuSet := map[string]bool{}
	sellerSet := map[string]bool{}
	for _, o := range offers {
		if o.SKUID != "" {
			skuSet[o.SKUID] = true
		}
		if o.SellerID != "" {
			sellerSet[o.SellerID] = true
		}
	}

	skus, err := l.lookupSKUs(ctx, keys(skuSet))
	if err != nil {
		return nil, err
	}
	sellers, err := l.lookupSellers(ctx, keys(sellerSet))
	if err != nil {
		return nil, err
	}
	stock, err := l.lookupStock(ctx, keys(skuSet))
	if err != nil {
		return nil, err
	}

	for _, id := range offerIDs {
		o, ok := offers[id.String()]
		if !ok {
			// Offer đã bị gỡ. KHÔNG đưa vào map — bên gọi hiểu là
			// UNAVAILABLE và giữ món lại trong giỏ để khách tự quyết định.
			continue
		}

		price, err := money.New(o.PriceAmount, money.Currency(o.PriceCurrency))
		if err != nil {
			// Giá hỏng ở nguồn. Bỏ qua giá thay vì bỏ qua cả offer: khách
			// vẫn thấy món trong giỏ với giá cũ, tốt hơn là món biến mất.
			price = money.Money{}
		}

		d := domain.SyncData{
			OfferExists:       true,
			IsSellable:        o.IsSellable,
			SKUID:             ids.ID(o.SKUID),
			SellerID:          ids.ID(o.SellerID),
			UnitPrice:         price,
			MinOrderQuantity:  o.MinOrderQuantity,
			MaxOrderQuantity:  o.MaxOrderQuantity,
			AvailableQuantity: stock[o.SKUID],
		}

		// Seller không tra được thì coi như KHÔNG hoạt động: thà đánh dấu
		// UNAVAILABLE nhầm còn hơn để khách đặt hàng của seller đã bị đình
		// chỉ rồi phải hủy đơn.
		d.SellerActive = sellers[o.SellerID]

		if sku, ok := skus[o.SKUID]; ok {
			d.ProductName = sku.ProductName
			d.VariantDescription = sku.VariantDescription
			d.ImageURL = sku.ImageURL
		}

		out[id] = d
	}
	return out, nil
}

// skuInfo là thông tin hiển thị của một SKU.
type skuInfo struct {
	ProductName        string
	VariantDescription string
	ImageURL           string
}

func (l *offerLookup) lookupSKUs(
	ctx context.Context, skuIDs []string,
) (map[string]skuInfo, error) {
	out := map[string]skuInfo{}
	if l.product == nil || len(skuIDs) == 0 {
		return out, nil
	}

	// Trả về SẢN PHẨM chứa SKU, không phải SKU rời: tên hiển thị nằm ở
	// sản phẩm, còn màu/size nằm ở biến thể. Cả hai đều cần để khách nhận
	// ra món mình đã thêm.
	views, err := l.product.GetProductsBySKUIDs(ctx, skuIDs)
	if err != nil {
		return nil, err
	}

	for skuID, p := range views {
		info := skuInfo{ProductName: p.Name}
		if len(p.Images) > 0 {
			info.ImageURL = p.Images[0]
		}

		// Tìm biến thể chứa SKU này để lấy mô tả "Trắng / M".
		//
		// Vòng lặp lồng trông tốn kém nhưng không phải: một sản phẩm thời
		// trang có vài chục biến thể, và đây là dữ liệu đã nằm sẵn trong
		// bộ nhớ — không có lượt gọi nào thêm.
		for _, v := range p.Variants {
			if !variantHasSKU(v, skuID) {
				continue
			}
			info.VariantDescription = variantLabel(v)
			if len(v.Images) > 0 {
				info.ImageURL = v.Images[0]
			}
			break
		}

		out[skuID] = info
	}
	return out, nil
}

func variantHasSKU(v product.VariantView, skuID string) bool {
	for _, s := range v.SKUs {
		if s.ID == skuID {
			return true
		}
	}
	return false
}

// variantLabel dựng nhãn hiển thị của biến thể: "Trắng / M".
func variantLabel(v product.VariantView) string {
	switch {
	case v.Color != "" && v.Size != "":
		return v.Color + " / " + v.Size
	case v.Color != "":
		return v.Color
	default:
		return v.Size
	}
}

func (l *offerLookup) lookupSellers(
	ctx context.Context, sellerIDs []string,
) (map[string]bool, error) {
	out := map[string]bool{}
	if l.seller == nil || len(sellerIDs) == 0 {
		return out, nil
	}

	views, err := l.seller.GetSellersByIDs(ctx, sellerIDs)
	if err != nil {
		return nil, err
	}
	for id, v := range views {
		out[id] = v.IsActive
	}
	return out, nil
}

func (l *offerLookup) lookupStock(
	ctx context.Context, skuIDs []string,
) (map[string]int, error) {
	out := map[string]int{}
	if l.inventory == nil || len(skuIDs) == 0 {
		return out, nil
	}

	// locationID rỗng = tổng mọi kho. Giỏ chỉ cần biết "có hàng hay không",
	// việc chọn kho nào để lấy là chuyện của fulfillment.
	stock, err := l.inventory.GetAvailability(ctx, skuIDs, "")
	if err != nil {
		return nil, err
	}
	return stock, nil
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
