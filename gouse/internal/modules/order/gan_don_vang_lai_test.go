package order_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/order"
)

// TestGanDonVangLaiKHONGCuopDonCuaNguoiKhac là hàng rào của P3-15.
//
// # Điều đang được bảo vệ
//
// Một đơn có thể mang CẢ `customer_id` lẫn `guest_email` — ràng buộc CHECK
// của bảng chỉ đòi có ít nhất một trong hai. Nghĩa là đơn của khách ĐÃ CÓ
// TÀI KHOẢN vẫn có thể mang một địa chỉ email trong trường guest.
//
// Nếu phép gắn chỉ lọc theo email, một lần xác minh email sẽ kéo đơn của
// người khác về tài khoản mình — cùng với địa chỉ nhà và số điện thoại
// người nhận trên đơn đó.
//
// Điều kiện `customer_id = ”` là thứ chặn việc đó, và nó là loại ràng
// buộc dễ bị bỏ quên nhất: bỏ nó đi thì mọi bài test một-khách vẫn xanh.
func TestGanDonVangLaiKhongCuopDonCuaNguoiKhac(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	const email = "chung-email@example.com"
	nguoiKhac := ids.MustNew(ids.PrefixCustomer)
	nguoiMoi := ids.MustNew(ids.PrefixCustomer)

	// Đơn A: VÃNG LAI — đây là đơn ĐƯỢC phép chuyển chủ.
	if _, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		GuestEmail: email, GuestPhone: "0900000000", Currency: "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 299000, 1, 1000, "Áo sơ mi"),
		},
		IdempotencyKey: "don-vang-lai-a",
	}); err != nil {
		t.Fatalf("đặt đơn vãng lai: %v", err)
	}

	// Đơn B: ĐÃ THUỘC người khác, nhưng mang CÙNG email ở trường guest.
	if _, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: nguoiKhac.String(),
		GuestEmail: email, GuestPhone: "0900000000", Currency: "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 450000, 1, 1000, "Quần âu"),
		},
		IdempotencyKey: "don-cua-nguoi-khac-b",
	}); err != nil {
		t.Fatalf("đặt đơn của người khác: %v", err)
	}

	n, err := m.GanDonVangLaiChoKhach(ctx, email, nguoiMoi.String())
	if err != nil {
		t.Fatalf("GanDonVangLaiChoKhach: %v", err)
	}

	// ĐÚNG MỘT đơn được chuyển: đơn vãng lai. Đơn của người khác KHÔNG.
	if n != 1 {
		t.Errorf("chuyển %d đơn, mong đúng 1 — con số 2 nghĩa là đã CƯỚP "+
			"đơn của khách %s", n, nguoiKhac)
	}

	danhSach, err := m.ListCustomerOrders(ctx, nguoiKhac.String(), 10)
	if err != nil {
		t.Fatalf("đọc đơn của người khác: %v", err)
	}
	if len(danhSach) != 1 {
		t.Errorf("khách %s còn %d đơn, mong 1 — đơn của họ đã bị chuyển đi",
			nguoiKhac, len(danhSach))
	}
}

// TestDemDonVangLaiChiDemDonVOCHU — con số này quyết định có gửi thư xác
// minh hay không, nên đếm nhầm đơn của người khác là hứa với khách một
// lịch sử không phải của họ.
func TestDemDonVangLaiChiDemDonVoChu(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	const email = "dem-email@example.com"

	if _, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		GuestEmail: email, GuestPhone: "0900000000", Currency: "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 100000, 1, 1000, "Áo"),
		},
		IdempotencyKey: "dem-vang-lai",
	}); err != nil {
		t.Fatalf("đặt đơn vãng lai: %v", err)
	}
	if _, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		GuestEmail: email, GuestPhone: "0900000000", Currency: "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 200000, 1, 1000, "Quần"),
		},
		IdempotencyKey: "dem-co-chu",
	}); err != nil {
		t.Fatalf("đặt đơn có chủ: %v", err)
	}

	n, err := m.DemDonVangLai(ctx, email)
	if err != nil {
		t.Fatalf("DemDonVangLai: %v", err)
	}
	if n != 1 {
		t.Errorf("đếm được %d, mong 1 — chỉ đơn VÔ CHỦ mới được tính", n)
	}
}
