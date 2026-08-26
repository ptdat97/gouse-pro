package app

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	inventoryhttp "github.com/fashion-commerce/platform/internal/modules/inventory/interfaces/http"
	markethttp "github.com/fashion-commerce/platform/internal/modules/marketplace/interfaces/http"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// sellerStock nhập kho ban đầu khi seller tạo offer.
//
// # Vì sao cầu nối này nằm ở đây
//
// Tạo offer và nhập kho là việc của HAI module. Cổng `InventoryPort` của
// marketplace CHỈ ĐỌC — nó không nên giành lấy quyền tạo tồn kho. cmd/api
// là điểm nối duy nhất biết cả hai, cùng mẫu với TokenVerifier và
// CustomerResolver.
type sellerStock struct {
	inv *inventory.Module

	// sellers trả lời "hàng của nhà bán này thuộc về ai".
	//
	// Không phải lúc nào cũng là chính họ: own brand là seller NỘI BỘ và
	// hàng của nó là tài sản NỀN TẢNG. Xem inventory.OwnerForSeller.
	sellers seller.API
}

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

	// CHỦ SỞ HỮU tách khỏi KHO CHỨA, có chủ ý (inventory.md mục 3.1): ghi
	// nhầm chủ sở hữu là ghi sai tài sản trên sổ sách.
	//
	// Và chủ sở hữu KHÔNG mặc nhiên là seller đứng tên offer. Own brand
	// là seller nội bộ nhưng hàng của nó là tài sản nền tảng. Dùng thẳng
	// sellerID ở đây đẻ ra bản ghi tồn kho mà không đường nào bán được:
	// giữ hàng tìm theo chủ sở hữu của seller, và chủ đó là own_platform.
	view, err := s.sellers.GetSeller(ctx, sellerID.String())
	if err != nil {
		return err
	}
	ownerID := inventory.OwnerForSeller(view.ID, view.IsInternal)

	_, err = s.inv.Receive(ctx, inventory.ReceiveRequest{
		SKUID:       skuID.String(),
		LocationID:  locationID,
		OwnerID:     ownerID,
		Quantity:    quantity,
		ReferenceID: "offer-initial",
		PerformedBy: sellerID.String(),
	})
	return err
}

// sellerOwner đổi định danh nhà bán lấy chủ sở hữu tồn kho cho module
// inventory.
//
// Cùng quy tắc, cùng nguồn sự thật với sellerStock ở trên và với
// checkout: `inventory.OwnerForSeller`. Ba đường ghi/đọc tồn kho theo nhà
// bán phải nhất trí, nếu không thì đường này tạo ra bản ghi mà đường kia
// không tìm thấy.
type sellerOwner struct{ sellers seller.API }

var _ inventoryhttp.OwnerResolver = (*sellerOwner)(nil)

func (s *sellerOwner) InventoryOwnerID(
	ctx context.Context, sellerID string,
) (string, error) {
	v, err := s.sellers.GetSeller(ctx, sellerID)
	if err != nil {
		return "", err
	}
	return inventory.OwnerForSeller(v.ID, v.IsInternal), nil
}
