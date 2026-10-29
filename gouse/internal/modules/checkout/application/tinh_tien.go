package application

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/checkout/domain"
	"github.com/fashion-commerce/platform/internal/platform/opsconfig"
)

// moneyPolicy đọc hai con số kinh doanh, rơi về MẶC ĐỊNH khi chưa nối.
//
// Mặc định lấy từ sổ đăng ký `opsconfig` chứ không viết cứng lại ở đây:
// hai bản sao của cùng một con số sẽ lệch nhau ở lần đổi tiếp theo.
//
// Vì sao KHÔNG rơi về 0: thuế 0 và ngưỡng 0 nghĩa là không thu thuế và
// miễn phí ship cho mọi đơn — hai lỗi tốn tiền, xảy ra im lặng, chỉ vì
// một bản dựng quên nối dây.
func (s *Service) moneyPolicy(unit money.Currency) (domain.MoneyPolicy, error) {
	taxRateBP := int32(configValue(opsconfig.KeyTaxRate))
	threshold := int64(configValue(opsconfig.KeyFreeShippingThreshold))
	handlingHours := int(configValue(opsconfig.KeyDefaultHandlingHours))

	if s.policy != nil {
		taxRateBP = s.policy.TaxRateBP()
		threshold = s.policy.FreeShippingThreshold()
		handlingHours = s.policy.DefaultHandlingHours()
	}

	rate, err := types.NewBasisPoints(taxRateBP)
	if err != nil {
		return domain.MoneyPolicy{}, err
	}
	m, err := money.New(threshold, unit)
	if err != nil {
		return domain.MoneyPolicy{}, err
	}
	return domain.MoneyPolicy{
		TaxRate:               rate,
		FreeShippingThreshold: m,
		DefaultHandlingHours:  handlingHours,
	}, nil
}

func configValue(key string) float64 {
	t, ok := opsconfig.Lookup(key)
	if !ok {
		return 0
	}
	return t.Default
}

// applyMoney tính LẠI phí vận chuyển và thuế từ trạng thái hiện tại.
//
// # Vì sao mọi đường ghi tiền đều phải gọi hàm này
//
// Ba con số phụ thuộc nhau theo thứ tự: giảm giá → phí ship (có xét ngưỡng
// miễn phí) → thuế. Đổi một cái mà không tính lại hai cái sau là để chúng
// lệch nhau, và lệch ở đây nghĩa là khách bị thu sai tiền.
//
// `freeShippingByCode` là miễn phí ship do MÃ GIẢM GIÁ cấp — khác với miễn phí do
// đạt ngưỡng. Hai đường tới cùng một kết quả nhưng vì hai lý do, và hóa
// đơn cần giải thích được lý do nào.
func (s *Service) applyMoney(
	ctx context.Context, c *domain.Checkout, freeShippingByCode bool, now time.Time,
) error {
	cs, err := s.moneyPolicy(c.Currency())
	if err != nil {
		return err
	}

	baseFee := money.Zero(c.Currency())

	switch {
	case freeShippingByCode:
		// Mã miễn phí ship: phí gốc = 0, nên ngưỡng không còn ý nghĩa.
	case c.ShippingMethod() == "":
		// Chưa chọn cách giao thì chưa có phí. Thuế vẫn tính trên tiền
		// hàng, để tổng hiển thị ở màn hình giỏ không nhảy khi khách chọn
		// cách giao xong.
	case s.shipping == nil:
		return ErrShippingEstimateNotWired
	default:
		sellers := make([]string, 0, len(c.SellerIDs()))
		for _, sid := range c.SellerIDs() {
			sellers = append(sellers, sid.String())
		}
		estimate, err := s.shipping.EstimateShipping(
			ctx, c.ShippingMethod(), sellers, string(c.Currency()))
		if err != nil {
			return err
		}
		if baseFee, err = money.New(estimate.Total, c.Currency()); err != nil {
			return err
		}
	}

	return c.ApplyFeesAndTax(baseFee, cs, now)
}
