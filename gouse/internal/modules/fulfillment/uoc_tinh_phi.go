package fulfillment

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

// ErrPhuongThucGiaoKhongHopLe: phương thức vận chuyển không có trong biểu phí.
var ErrPhuongThucGiaoKhongHopLe = domain.ErrPhuongThucKhongHopLe

// EstimateShipping ước tính phí vận chuyển cho một đơn nhiều nguồn hàng.
//
// Đây là hàm mà docs/04-modules/checkout.md mục 7 quy định, và là hàm mà
// chú thích `shippingRates` trong checkout đã trỏ tới suốt từ MVP.
//
// MỖI NGUỒN LÀ MỘT KIỆN: hàng của ba nhà bán đi thành ba kiện và tốn ba
// lần phí. Xem `domain.UocTinhPhiGiao`.
func (m *Module) EstimateShipping(
	ctx context.Context, req ShippingEstimateRequest,
) (*ShippingEstimateView, error) {
	donVi := money.Currency(req.Currency)
	if donVi == "" {
		donVi = money.VND
	}

	nguon := make([]domain.NguonHang, 0, len(req.Sources))
	for _, s := range req.Sources {
		nguon = append(nguon, domain.NguonHang{SellerID: s.SellerID})
	}

	res, err := domain.UocTinhPhiGiao(
		domain.PhuongThucGiao(req.Method), nguon, donVi)
	if err != nil {
		return nil, err
	}

	out := &ShippingEstimateView{
		Total:     res.Tong.Amount(),
		Currency:  string(res.Tong.Currency()),
		PerSource: make([]PhiTheoNguonView, 0, len(res.TheoNguon)),
	}
	for _, p := range res.TheoNguon {
		out.PerSource = append(out.PerSource, PhiTheoNguonView{
			SellerID:      p.SellerID,
			Amount:        p.Phi.Amount(),
			EstimatedDays: p.SoNgayDuKien,
		})
	}
	return out, nil
}
