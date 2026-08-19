package marketplace

import (
	"context"
	"errors"
	"fmt"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/application"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
)

// SeedInput là dữ liệu tạo offer mẫu.
type SeedInput struct {
	// SellerID là nhà bán đứng tên offer. Thường là seller own-brand của
	// nền tảng, tạo lúc khởi động.
	SellerID string

	// SKUIDs là các SKU cần chào bán. Phải CÓ THẬT từ module product.
	SKUIDs []string

	// PriceAmount là giá bán, theo đơn vị nhỏ nhất của tiền tệ.
	PriceAmount int64
}

// SeedResult là kết quả nạp dữ liệu mẫu.
type SeedResult struct {
	// OfferIDs là các offer đang bán — dùng để gọi thử `addCartItem`.
	OfferIDs []string
}

// SeedDemo tạo offer mẫu cho các SKU đã có.
//
// CHỈ dùng cho môi trường phát triển.
//
// # Vì sao seed này cần tồn tại
//
// Khách KHÔNG mua SKU, khách mua OFFER — lời chào bán cụ thể của một nhà
// bán. `addCartItem` nhận `offer_id`, không nhận `sku_id`. Không có offer
// thì catalog đầy sản phẩm mà giỏ hàng vẫn rỗng vĩnh viễn.
//
// Bỏ qua SKU đã có offer thay vì báo lỗi: hàm chạy lại mỗi lần khởi động.
func SeedDemo(ctx context.Context, m *Module, in SeedInput) (SeedResult, error) {
	var out SeedResult
	if m == nil || in.SellerID == "" || len(in.SKUIDs) == 0 {
		return out, nil
	}

	sellerID, err := ids.Parse(in.SellerID, ids.PrefixSeller)
	if err != nil {
		return out, fmt.Errorf("seller_id không hợp lệ: %w", err)
	}

	amount := in.PriceAmount
	if amount <= 0 {
		amount = 490_000
	}
	price, err := money.New(amount, money.VND)
	if err != nil {
		return out, fmt.Errorf("giá không hợp lệ: %w", err)
	}

	for _, raw := range in.SKUIDs {
		skuID, err := ids.Parse(raw, ids.PrefixSKU)
		if err != nil {
			return out, fmt.Errorf("sku_id không hợp lệ: %w", err)
		}

		o, err := m.svc.CreateOffer(ctx, application.CreateOfferInput{
			SKUID:             skuID,
			SellerID:          sellerID,
			Price:             price,
			Condition:         domain.ConditionNew,
			HandlingTimeHours: 24,
			MinOrderQuantity:  1,
			MaxOrderQuantity:  10,

			// Bán được NGAY. Offer ở trạng thái nháp không hiện ra cho
			// khách, và dữ liệu mẫu không hiện ra thì vô dụng.
			Activate: true,
		})
		if errors.Is(err, domain.ErrDuplicateActiveOffer) {
			// Đã nạp từ lần khởi động trước. Lấy lại offer cũ để bên gọi
			// vẫn có định danh dùng được.
			existing, findErr := m.svc.GetOffersBySKU(ctx, skuID)
			if findErr == nil && len(existing) > 0 {
				out.OfferIDs = append(out.OfferIDs, existing[0].ID().String())
			}
			continue
		}
		if err != nil {
			return out, fmt.Errorf("tạo offer cho %s: %w", raw, err)
		}
		out.OfferIDs = append(out.OfferIDs, o.ID().String())
	}

	return out, nil
}
