package recommendation

import (
	"context"

	"github.com/fashion-commerce/platform/internal/modules/product"
)

// NewProductPort dựng cổng tra SKU → (thương hiệu, size) từ module product.
//
// # Vì sao module này ĐƯỢC PHÉP phụ thuộc `product`
//
// Chiều phụ thuộc là `recommendation → product`. Chiều ngược lại KHÔNG tồn
// tại: `product` không import gói này — tầng interfaces của nó khai một
// cổng hẹp, và `internal/app` nối vào.
//
// Nhờ vậy không có vòng, và đó cũng là điều làm ADR-0019 rẻ hơn hẳn phương
// án ban đầu: không phải luồn thêm `size` và `brand_id` qua cart, checkout,
// order rồi returns, và không phải nâng phiên bản event nào.
func NewProductPort(api product.API) ProductPort { return &productAdapter{api: api} }

type productAdapter struct{ api product.API }

// ThuongHieuVaSize tìm biến thể CHỨA SKU này rồi trả size của nó.
//
// Một SKU thuộc đúng một biến thể, và biến thể mang thuộc tính size. Không
// tìm thấy thì trả rỗng chứ không lỗi: sản phẩm không có thuộc tính size
// (phụ kiện, nước hoa) là chuyện bình thường.
func (a *productAdapter) ThuongHieuVaSize(
	ctx context.Context, skuID string,
) (string, string, error) {
	theoSKU, err := a.api.GetProductsBySKUIDs(ctx, []string{skuID})
	if err != nil {
		return "", "", err
	}
	p, ok := theoSKU[skuID]
	if !ok {
		return "", "", nil
	}

	for _, v := range p.Variants {
		for _, s := range v.SKUs {
			if s.ID == skuID {
				return p.BrandID, v.Size, nil
			}
		}
	}
	return p.BrandID, "", nil
}
