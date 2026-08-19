package domain_test

import (
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
)

// orderFor dựng đơn với chủ sở hữu chỉ định.
func orderFor(t *testing.T, customerID ids.ID, guestEmail, guestPhone string) *domain.Order {
	t.Helper()

	rate, err := types.NewBasisPoints(1000)
	if err != nil {
		t.Fatalf("tỷ lệ hoa hồng: %v", err)
	}
	line, err := domain.NewLine(domain.NewLineParams{
		OfferID:        ids.MustNew(ids.PrefixOffer),
		SKUID:          ids.MustNew(ids.PrefixSKU),
		SellerID:       ids.MustNew(ids.PrefixSeller),
		ProductName:    "Áo sơ mi",
		UnitPrice:      money.MustNew(299_000, money.VND),
		Quantity:       1,
		CommissionRate: rate,
	})
	if err != nil {
		t.Fatalf("NewLine: %v", err)
	}

	o, err := domain.NewOrder(domain.NewOrderParams{
		OrderNumber:    "FC-2026-08-000001",
		CustomerID:     customerID,
		GuestEmail:     guestEmail,
		GuestPhone:     guestPhone,
		Currency:       money.VND,
		Lines:          []*domain.Line{line},
		IdempotencyKey: string(ids.MustNew(ids.PrefixOrder)),
		Now:            time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	return o
}

// QUYỀN XEM ĐƠN — quy tắc này được hỏi từ ba nơi (chi tiết đơn, hủy đơn, lô
// giao), nên nó phải đúng ở MỘT chỗ duy nhất.
//
// Mỗi nơi tự cài lại nghĩa là sớm muộn một nơi cài lỏng hơn, và MỘT nơi
// lỏng là đủ để lộ lịch sử mua hàng của người khác.
func TestQuyenXemDon(t *testing.T) {
	chuDon := ids.MustNew(ids.PrefixCustomer)
	keTomo := ids.MustNew(ids.PrefixCustomer)

	daDangNhap := orderFor(t, chuDon, "a@example.com", "+84901234567")
	vangLai := orderFor(t, "", "b@example.com", "+84907654321")
	khongSDT := orderFor(t, "", "c@example.com", "")

	cases := []struct {
		name       string
		order      *domain.Order
		customerID string
		phone      string
		want       bool
	}{
		{"chủ đơn xem đơn của mình", daDangNhap, chuDon.String(), "", true},
		{"người khác KHÔNG xem được", daDangNhap, keTomo.String(), "", false},
		{
			name: "biết số điện thoại KHÔNG mở được đơn đã có chủ",
			// Đơn đã gắn với tài khoản thì số điện thoại không còn là
			// chìa khóa — nếu không, ai biết số của khách đều đọc được đơn.
			order: daDangNhap, customerID: "", phone: "+84901234567", want: false,
		},
		{"vãng lai đúng số điện thoại", vangLai, "", "+84907654321", true},
		{"vãng lai sai số điện thoại", vangLai, "", "+84900000000", false},
		{"vãng lai không gửi số điện thoại", vangLai, "", "", false},
		{
			name: "số điện thoại có khoảng trắng vẫn khớp",
			// Người dùng dán từ tin nhắn thì hay kèm khoảng trắng.
			order: vangLai, customerID: "", phone: "  +84907654321  ", want: true,
		},
		{
			name: "đơn KHÔNG có số điện thoại thì không ai mở bằng số rỗng",
			// Chuỗi rỗng không được khớp với chuỗi rỗng — nếu không, một
			// đơn thiếu số điện thoại mở cho bất kỳ ai không gửi gì.
			order: khongSDT, customerID: "", phone: "", want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.order.ViewableBy(c.customerID, c.phone); got != c.want {
				t.Errorf("ViewableBy = %v, muốn %v", got, c.want)
			}
		})
	}
}
