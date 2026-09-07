package e2e_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	checkoutapp "github.com/fashion-commerce/platform/internal/modules/checkout/application"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

// datDonVoiPhuongThuc đặt một đơn qua chuỗi thật rồi phát hết event.
func (w *world) datDonVoiPhuongThuc(
	t *testing.T, phuongThuc string,
) (sellerID ids.ID, fos []fulfillment.FulfillmentView) {
	t.Helper()
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	w.stockFor(skuID, shop, 20)

	cartID := ids.MustNew(ids.PrefixCart)
	w.cart.put(checkoutapp.CartSnapshot{
		CartID:     cartID,
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		GuestEmail: "khach@example.com",
		Currency:   money.VND,
		Items:      []checkoutapp.CartItemSnapshot{line(shop, skuID, 300_000, 2)},
	})

	c, err := w.checkout.StartCheckout(ctx, checkoutapp.StartCheckoutInput{CartID: cartID})
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}
	if _, err := w.checkout.SetShippingAddress(ctx, c.ID(), address()); err != nil {
		t.Fatalf("SetShippingAddress: %v", err)
	}
	if _, err := w.checkout.SetShippingMethod(ctx, c.ID(), "STANDARD"); err != nil {
		t.Fatalf("SetShippingMethod: %v", err)
	}
	res, err := w.checkout.CompleteCheckout(
		ctx, c.ID(), ids.MustNew(ids.PrefixRequest).String(), phuongThuc)
	if err != nil {
		t.Fatalf("CompleteCheckout(%s): %v", phuongThuc, err)
	}
	w.drain()

	list, err := w.ful.GetOrderFulfillments(ctx, res.OrderID.String())
	if err != nil {
		t.Fatalf("GetOrderFulfillments: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("có %d đơn thực hiện, mong 1", len(list))
	}
	w.orderIDCuoi = res.OrderID
	return shop, list
}

// TestDonTraTruocKHONGGiaoDuocTruocKhiThuTien — bài chính của ADR-0018 A2.
//
// Trước thay đổi này, nhà bán đưa được một đơn CARD chưa trả đồng nào đi
// hết tới HANDED_OVER: hàng rời kho cho một khoản tiền không bao giờ tới.
// Đo được lúc phát hiện: 3166 đơn PENDING_PAYMENT, 3171 đơn thực hiện.
func TestDonTraTruocKhongGiaoDuocTruocKhiThuTien(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop, fos := w.datDonVoiPhuongThuc(t, "CARD")
	fo := fos[0]

	var pay string
	var ver int
	_ = w.db.Pool().QueryRow(ctx, "SELECT payload->>'payment_method', event_version FROM event_outbox WHERE event_type='checkout.completed' LIMIT 1").Scan(&pay, &ver)
	t.Logf("DEBUG payload payment_method=%q version=%d", pay, ver)
	err := w.ful.ConfirmFulfillment(ctx, shop.String(), fo.ID)
	if err == nil {
		t.Fatal("nhà bán XÁC NHẬN được đơn CARD chưa trả tiền — hàng sẽ rời kho " +
			"cho một khoản tiền chưa tới")
	}

	// HỦY vẫn phải được: khách bỏ đơn chưa thanh toán là đường thoát bình
	// thường nhất của chính những đơn đang bị khóa. Chặn nó thì hàng kẹt
	// trong kho vĩnh viễn.
	if err := w.ful.CancelFulfillment(
		ctx, shop.String(), fo.ID, "khách không thanh toán"); err != nil {
		t.Errorf("HỦY đơn đang chờ thanh toán phải được phép: %v", err)
	}
}

// TestDonCODGiaoDuocNGAY — nửa còn lại của quy tắc, và dễ làm sai nhất.
//
// Với COD tiền về LÚC GIAO, nên chờ thanh toán trước khi giao là chặn
// chính đường thu tiền. Không có bài này thì một lần khóa quá tay sẽ làm
// đứng toàn bộ COD mà mọi bài khác vẫn xanh.
func TestDonCODGiaoDuocNgay(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop, fos := w.datDonVoiPhuongThuc(t, "COD")

	if err := w.ful.ConfirmFulfillment(ctx, shop.String(), fos[0].ID); err != nil {
		t.Fatalf("đơn COD phải xử lý được ngay: %v", err)
	}
}

// TestThuTienXongThiMoKhoaGiaoHang — đầu kia của cửa chặn.
//
// Đi qua chuỗi THẬT: order.MarkPaid ghi trạng thái và phát `order.paid`
// trong cùng giao dịch, dispatcher đưa tới fulfillment, fulfillment mở
// khóa. Thiếu bất kỳ mắt nào thì đơn trả trước bị khóa VĨNH VIỄN — hỏng
// nặng hơn hẳn thứ quy tắc này nhắm tới.
func TestThuTienXongThiMoKhoaGiaoHang(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop, fos := w.datDonVoiPhuongThuc(t, "BANK_TRANSFER")
	fo := fos[0]

	if err := w.ful.ConfirmFulfillment(ctx, shop.String(), fo.ID); err == nil {
		t.Fatal("đơn trả trước phải bị khóa trước khi thu tiền")
	}

	// Tiền về.
	if err := w.ord.MarkOrderPaid(ctx, w.orderIDCuoi.String()); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	w.drain()

	if err := w.ful.ConfirmFulfillment(ctx, shop.String(), fo.ID); err != nil {
		t.Fatalf("sau khi thu tiền phải giao được: %v", err)
	}
}

// TestQuyTacKhoaLaCUATRENDUONGDI, không phải một điều kiện rải rác.
//
// Kiểm ở tầng domain rằng MỌI bước tiến đều bị chặn, không chỉ bước đầu.
// Nếu quy tắc nằm rải ở từng use case thì hàm thêm vào tháng sau sẽ quên,
// và cái quên đó nghĩa là hàng rời kho.
func TestKhoaChanMoiBuocTien(t *testing.T) {
	fo := foChoThanhToan(t)

	for _, b := range []struct {
		ten string
		lam func() error
	}{
		{"xác nhận", func() error { return fo.Confirm(testNowE2E()) }},
		{"nhặt hàng", func() error { return fo.Pick(testNowE2E()) }},
		{"đóng gói", func() error { return fo.Pack(testNowE2E()) }},
		{"bàn giao", func() error {
			return fo.HandOver("GHN", "TRACK-1", testNowE2E())
		}},
	} {
		if err := b.lam(); !errors.Is(err, domain.ErrChoThanhToan) {
			t.Errorf("%s: lỗi = %v, mong ErrChoThanhToan", b.ten, err)
		}
	}
}

// foChoThanhToan dựng một đơn thực hiện ĐANG KHÓA, không cần database.
func foChoThanhToan(t *testing.T) *domain.FulfillmentOrder {
	t.Helper()
	fos, err := domain.SplitIntoFulfillmentOrders(domain.SplitInput{
		OrderID:      ids.MustNew(ids.PrefixOrder),
		OrderNumber:  "FC-2026-09-000001",
		Currency:     money.VND,
		ChoThanhToan: true,
		Lines: []domain.SplitLine{{
			LineID:           ids.MustNew(ids.PrefixOrderLine),
			SKUID:            ids.MustNew(ids.PrefixSKU),
			SellerID:         ids.MustNew(ids.PrefixSeller),
			Quantity:         1,
			UnitPrice:        mustMoney(t, 300_000),
			LineTotal:        mustMoney(t, 300_000),
			CommissionAmount: mustMoney(t, 30_000),
		}},
	}, testNowE2E())
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}
	return fos[0]
}

func mustMoney(t *testing.T, v int64) money.Money {
	t.Helper()
	m, err := money.New(v, money.VND)
	if err != nil {
		t.Fatalf("money.New: %v", err)
	}
	return m
}

func testNowE2E() time.Time {
	return time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
}
