package e2e_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestChiSoDemThatBaiNghiepVu(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, 1)

	truoc := testutil.ToFloat64(
		metrics.BusinessFailures.WithLabelValues(metrics.StageReservation, "out_of_stock"))

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID: cartID, CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "k@example.com", Currency: money.VND,
		Items: []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 9)},
	})
	if _, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{
		CartID: cartID,
	}); err == nil {
		t.Fatal("mở phiên thành công dù thiếu hàng")
	}

	sau := testutil.ToFloat64(
		metrics.BusinessFailures.WithLabelValues(metrics.StageReservation, "out_of_stock"))
	if sau != truoc+1 {
		t.Errorf("bộ đếm hết hàng: %v → %v, cần tăng 1", truoc, sau)
	}

	// Lý do LẠ phải bị cắt về "other", không được tạo nhãn mới.
	//
	// Nhãn Prometheus có bao nhiêu giá trị thì có bấy nhiêu chuỗi thời
	// gian. Một lý do lấy từ thông điệp lỗi — thứ chứa tên sản phẩm hoặc
	// mã đơn — sẽ tạo một chuỗi mới mỗi lần lỗi và giết hệ thống theo dõi.
	truocOther := testutil.ToFloat64(
		metrics.BusinessFailures.WithLabelValues(metrics.StageCheckout, "other"))
	metrics.RecordFailure(metrics.StageCheckout,
		"không đủ hàng: Áo sơ mi linen Oxford (cần 9)")
	sauOther := testutil.ToFloat64(
		metrics.BusinessFailures.WithLabelValues(metrics.StageCheckout, "other"))
	if sauOther != truocOther+1 {
		t.Errorf("lý do lạ không bị cắt về \"other\": %v → %v", truocOther, sauOther)
	}
}
