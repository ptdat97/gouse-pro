package application_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/returns/application"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
)

// Lý do nay nằm ở TỪNG DÒNG (đặc tả requestReturn), nên các bài dưới đây
// chỉ kiểm phép tính tiền — phần không phụ thuộc lý do.

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func lines(listPrice, adjustments int64, qty int) application.OrderLine {
	return application.OrderLine{
		ID: ids.MustNew(ids.PrefixOrderLine), SKUID: ids.MustNew(ids.PrefixSKU),
		SellerID: ids.MustNew(ids.PrefixSeller), Quantity: qty,
		LineTotal: vnd(listPrice), TotalAdjustments: vnd(adjustments),
	}
}

// TestHoanTheoGiaTHUCTRA — điểm dễ sai nhất của cả luồng trả hàng.
//
// Ví dụ lấy thẳng từ docs/07-workflows/return.md mục 5: đơn 500.000đ giảm
// 50.000đ (10%). Món A niêm yết 200.000đ nhưng khách chỉ trả 180.000đ.
//
// Hoàn 200.000đ là mất 20.000đ MỖI LẦN, nhân với tỷ lệ hoàn hàng của
// ngành thời trang.
func TestRefundUsesAmountActuallyPaid(t *testing.T) {
	lineA := lines(200_000, -20_000, 1)
	ord := application.Order{
		Delivered: true, OrderDiscount: vnd(50_000),
		Lines: []application.OrderLine{
			lineA,
			lines(200_000, -20_000, 1),
			lines(100_000, -10_000, 1),
		},
	}

	got, err := application.ComputeRefund(ord, lineA, 1)
	if err != nil {
		t.Fatalf("tính tiền hoàn: %v", err)
	}
	if got.Amount() != 180_000 {
		t.Errorf("hoàn %d, cần 180000 — hoàn theo giá NIÊM YẾT là trả nhiều "+
			"hơn số khách đã đưa", got.Amount())
	}
}

// TestRefundRejectedWhenDiscountNotAllocated — HÀNG RÀO.
//
// # Vì sao hàng rào này cần thiết NGAY BÂY GIỜ
//
// `checkout.ApplyCoupon` là route sống và nó đặt giảm giá ở CẤP ĐƠN. Nhưng
// `promotion.AllocateDiscount` — hàm phân bổ phần giảm xuống từng dòng —
// KHÔNG ai gọi. Chính comment của nó cảnh báo: "Không lưu lại thì nền tảng
// hoàn nhiều hơn đã thu."
//
// Nên hôm nay, một đơn có mã giảm giá sẽ có `discount_amount > 0` mà mọi
// dòng đều `TotalAdjustments = 0`. Hoàn theo giá dòng khi đó là hoàn thừa
// đúng bằng phần giảm.
//
// Thà TỪ CHỐI và bắt xử lý tay còn hơn âm thầm trả ra số tiền sai.
func TestRefundRejectedWhenDiscountNotAllocated(t *testing.T) {
	lineA := lines(200_000, 0, 1) // KHÔNG có khoản điều chỉnh
	ord := application.Order{
		Delivered: true,
		// Đơn CÓ giảm giá…
		OrderDiscount: vnd(50_000),
		Lines:         []application.OrderLine{lineA, lines(300_000, 0, 1)},
	}

	_, err := application.ComputeRefund(ord, lineA, 1)
	if !errors.Is(err, domain.ErrDiscountNotAllocated) {
		t.Errorf("tính được tiền hoàn cho đơn có giảm giá CHƯA phân bổ "+
			"(lỗi: %v) — sẽ hoàn thừa đúng bằng phần giảm", err)
	}
}

// TestFullRefundWithoutDiscount — ca thường, không được vướng hàng rào.
func TestFullRefundWithoutDiscount(t *testing.T) {
	lineA := lines(199_000, 0, 1)
	ord := application.Order{
		Delivered: true, OrderDiscount: vnd(0),
		Lines: []application.OrderLine{lineA},
	}

	got, err := application.ComputeRefund(ord, lineA, 1)
	if err != nil {
		t.Fatalf("đơn không giảm giá vẫn bị chặn: %v", err)
	}
	if got.Amount() != 199_000 {
		t.Errorf("hoàn %d, cần 199000", got.Amount())
	}
}

// TestTraMotPhanLamTronXUONG.
//
// Làm tròn LÊN nghĩa là trả nhiều hơn đã thu, và phần chênh nhân với số
// lần trả hàng của cả nền tảng.
func TestPartialReturnRoundsDown(t *testing.T) {
	// 3 món, thực trả 100.000đ → 33.333,33đ mỗi món.
	lineA := lines(100_000, 0, 3)
	ord := application.Order{Delivered: true, Lines: []application.OrderLine{lineA}}

	got, err := application.ComputeRefund(ord, lineA, 1)
	if err != nil {
		t.Fatalf("tính tiền hoàn: %v", err)
	}
	if got.Amount() != 33_333 {
		t.Errorf("hoàn %d cho 1/3 dòng, cần 33333 (làm tròn XUỐNG)", got.Amount())
	}

	// Trả TRỌN dòng thì lấy đủ, không mất phần dư.
	rounded, err := application.ComputeRefund(ord, lineA, 3)
	if err != nil {
		t.Fatalf("tính tiền hoàn trọn dòng: %v", err)
	}
	if rounded.Amount() != 100_000 {
		t.Errorf("trả trọn dòng hoàn %d, cần 100000 — phần dư bị mất",
			rounded.Amount())
	}
}

func TestQuantityOverOrderedIsRejected(t *testing.T) {
	lineA := lines(100_000, 0, 2)
	ord := application.Order{Delivered: true, Lines: []application.OrderLine{lineA}}

	for _, qty := range []int{0, -1, 3} {
		if _, err := application.ComputeRefund(ord, lineA, qty); !errors.Is(err, domain.ErrQuantityExceeded) {
			t.Errorf("số lượng %d được chấp nhận", qty)
		}
	}
}
