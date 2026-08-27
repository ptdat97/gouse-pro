package payment

import (
	"context"

	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// sellerKindAdapter nối cổng SellerKind với API công khai của module seller.
//
// Đặt ở đây chứ không ở điểm khởi chạy: bên khởi chạy có nhiều hơn một
// (cmd/worker và test tích hợp), và mỗi nơi tự viết một bản sao nghĩa là
// test kiểm một adapter KHÁC với adapter chạy thật.
//
// payment ở tầng giao dịch, seller ở tầng nghiệp vụ bên dưới — import
// xuôi chiều, hợp lệ theo docs/03-architecture/dependency-rules.md mục 6.
// Cổng vẫn cần thiết để tầng application không biết tới seller.
type sellerKindAdapter struct{ sellers seller.API }

var _ SellerKind = (*sellerKindAdapter)(nil)

// NewSellerKind dựng bộ trả lời "nhà bán này có phải own brand không".
func NewSellerKind(sellers seller.API) SellerKind {
	return &sellerKindAdapter{sellers: sellers}
}

func (a *sellerKindAdapter) IsInternal(ctx context.Context, sellerID string) (bool, error) {
	v, err := a.sellers.GetSeller(ctx, sellerID)
	if err != nil {
		return false, err
	}
	return v.IsInternal, nil
}
