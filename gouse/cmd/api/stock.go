package main

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	markethttp "github.com/fashion-commerce/platform/internal/modules/marketplace/interfaces/http"
)

// sellerStock nhập kho ban đầu khi seller tạo offer.
//
// # Vì sao cầu nối này nằm ở đây
//
// Tạo offer và nhập kho là việc của HAI module. Cổng `InventoryPort` của
// marketplace CHỈ ĐỌC — nó không nên giành lấy quyền tạo tồn kho. cmd/api
// là điểm nối duy nhất biết cả hai, cùng mẫu với TokenVerifier và
// CustomerResolver.
type sellerStock struct{ inv *inventory.Module }

var _ markethttp.StockPort = (*sellerStock)(nil)

// ReceiveInitial nhập lô hàng đầu tiên cho một SKU của một seller.
func (s *sellerStock) ReceiveInitial(
	ctx context.Context, skuID, sellerID ids.ID, locationID string, quantity int,
) error {
	if locationID == "" {
		// Seller chưa nói kho nào thì dùng kho RIÊNG của họ.
		//
		// Mã kho suy ra từ định danh seller nên nó XÁC ĐỊNH: gọi lại lần
		// hai trả đúng kho cũ thay vì tạo kho thứ hai, và hàng của họ
		// không nằm rải rác nhiều chỗ.
		id, err := s.inv.EnsureLocation(ctx,
			"Kho của nhà bán", "SELLER-"+sellerID.String(), "SELLER")
		if err != nil {
			return err
		}
		locationID = id
	}

	// OwnerID là SELLER, không phải nền tảng — kể cả khi hàng nằm ở kho
	// nền tảng. Hai khái niệm này tách nhau có chủ ý (inventory.md mục
	// 3.1): ghi nhầm chủ sở hữu là ghi sai tài sản trên sổ sách.
	_, err := s.inv.Receive(ctx, inventory.ReceiveRequest{
		SKUID:       skuID.String(),
		LocationID:  locationID,
		OwnerID:     sellerID.String(),
		Quantity:    quantity,
		ReferenceID: "offer-initial",
		PerformedBy: sellerID.String(),
	})
	return err
}
