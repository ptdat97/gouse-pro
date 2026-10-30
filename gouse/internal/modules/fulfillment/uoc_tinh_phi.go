package fulfillment

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

// ErrInvalidShipMethod: phương thức vận chuyển không có trong biểu phí.
var ErrInvalidShipMethod = domain.ErrInvalidShipMethod

// EstimateShipping ước tính phí vận chuyển cho một đơn nhiều nguồn hàng.
//
// Đây là hàm mà docs/04-modules/checkout.md mục 7 quy định, và là hàm mà
// chú thích `shippingRates` trong checkout đã trỏ tới suốt từ MVP.
//
// MỖI NGUỒN LÀ MỘT KIỆN: hàng của ba nhà bán đi thành ba kiện và tốn ba
// lần phí. Xem `domain.EstimateShippingFees`.
func (m *Module) EstimateShipping(
	ctx context.Context, req ShippingEstimateRequest,
) (*ShippingEstimateView, error) {
	unit := money.Currency(req.Currency)
	if unit == "" {
		unit = money.VND
	}

	sources := make([]domain.Source, 0, len(req.Sources))
	for _, s := range req.Sources {
		sources = append(sources, domain.Source{SellerID: s.SellerID})
	}

	// Biểu phí đọc MỖI LẦN báo giá: đổi phí vận chuyển phải có tác dụng
	// ở lượt thanh toán kế tiếp, không phải ở lần triển khai kế tiếp.
	res, err := domain.EstimateShippingFees(
		domain.ShipMethod(req.Method), sources, unit, m.feeSchedule())
	if err != nil {
		return nil, err
	}

	out := &ShippingEstimateView{
		Total:     res.Total.Amount(),
		Currency:  string(res.Total.Currency()),
		PerSource: make([]SourceFeeView, 0, len(res.PerSource)),
	}
	for _, p := range res.PerSource {
		out.PerSource = append(out.PerSource, SourceFeeView{
			SellerID:      p.SellerID,
			Amount:        p.Fee.Amount(),
			EstimatedDays: p.EstimatedDays,
		})
	}
	return out, nil
}
