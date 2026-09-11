package domain_test

import (
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/checkout/domain"
)

// phienCoHang dựng một phiên có ĐÚNG `tienHang` tiền hàng.
func phienCoHang(t *testing.T, tienHang int64) *domain.Checkout {
	t.Helper()
	return newCheckout(t, newLine(t, tienHang, 1))
}

func chinhSach(thueBP int32, nguong int64) domain.ChinhSachTien {
	return domain.ChinhSachTien{
		ThueSuat:          types.MustNewBasisPoints(thueBP),
		NguongMienPhiShip: money.MustNew(nguong, money.VND),
	}
}

// TestThueTinhTREN CA phí vận chuyển.
//
// Vận chuyển là một dịch vụ CHỊU THUẾ, không phải khoản thu hộ. Tính thuế
// chỉ trên tiền hàng sẽ ra số thuế thấp hơn thực tế phải nộp — và số thiếu
// đó nằm trong một cuốn sổ bất biến.
func TestThueTinhTrenCaPhiVanChuyen(t *testing.T) {
	c := phienCoHang(t, 100_000) // tiền hàng 100.000đ

	// Ngưỡng CAO để không rơi vào miễn phí ship.
	if err := c.ApDungPhiVaThue(
		money.MustNew(30_000, money.VND), chinhSach(800, 10_000_000), testNow,
	); err != nil {
		t.Fatalf("ApDungPhiVaThue: %v", err)
	}

	// Khách trả 130.000 (đã gồm VAT). Thuế TÁCH RA:
	// 130.000 × 800/10800 = 9.629,6 → 9.630.
	if got := c.TaxAmount().Amount(); got != 9_630 {
		t.Errorf("thuế = %d, mong 9630 — phần VAT nằm trong 130.000 "+
			"(100.000 tiền hàng + 30.000 phí ship). Bỏ phí ra khỏi phần "+
			"tách ra 7.407, tức hóa đơn thiếu thuế của dịch vụ vận chuyển", got)
	}
	// Tổng KHÔNG đổi khi có thuế: thuế đã nằm trong giá.
	if got := c.Total().Amount(); got != 130_000 {
		t.Errorf("tổng = %d, mong 130000 — cộng thuế vào ra 139.630, tức "+
			"thu thuế hai lần", got)
	}
}

// TestMienPhiShipKhiDAT nguong — so sánh ">=", không phải ">".
//
// Ngưỡng 499.000đ nghĩa là đơn ĐÚNG 499.000đ được miễn. Khách nhìn con số
// quảng cáo rồi mua vừa đúng bằng nó là hành vi phổ biến; để họ trượt vì
// một đồng là cách chắc chắn để nhận một khiếu nại đúng.
func TestMienPhiShipKhiDatNguong(t *testing.T) {
	for _, tt := range []struct {
		ten      string
		tienHang int64
		mongPhi  int64
	}{
		{"dưới ngưỡng một đồng", 498_999, 30_000},
		{"ĐÚNG ngưỡng", 499_000, 0},
		{"trên ngưỡng", 600_000, 0},
	} {
		t.Run(tt.ten, func(t *testing.T) {
			c := phienCoHang(t, tt.tienHang)
			if err := c.ApDungPhiVaThue(
				money.MustNew(30_000, money.VND), chinhSach(800, 499_000), testNow,
			); err != nil {
				t.Fatalf("ApDungPhiVaThue: %v", err)
			}
			if got := c.ShippingFee().Amount(); got != tt.mongPhi {
				t.Errorf("phí ship = %d, mong %d", got, tt.mongPhi)
			}
		})
	}
}

// TestNguongXetTREN TIEN HANG SAU GIAM GIA.
//
// Xét trên subtotal TRƯỚC giảm giá thì một mã giảm 200.000đ biến đơn
// 400.000đ thành đơn được miễn phí ship — nền tảng chịu CẢ HAI khoản cho
// một đơn nhỏ hơn ngưỡng.
func TestNguongXetTrenTienHangSauGiamGia(t *testing.T) {
	c := phienCoHang(t, 600_000)
	if err := c.ApplyDiscount("GIAM200", money.MustNew(200_000, money.VND), domain.BenChiuNenTang, testNow); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}

	// 600.000 − 200.000 = 400.000 < 499.000 → KHÔNG miễn phí.
	if err := c.ApDungPhiVaThue(
		money.MustNew(30_000, money.VND), chinhSach(800, 499_000), testNow,
	); err != nil {
		t.Fatalf("ApDungPhiVaThue: %v", err)
	}
	if got := c.ShippingFee().Amount(); got != 30_000 {
		t.Errorf("phí ship = %d, mong 30000 — khách thực trả 400.000đ cho "+
			"hàng, dưới ngưỡng 499.000đ", got)
	}
}

// TestThueTinhTrenTienHangSauGiamGia — thuế trên số khách THỰC TRẢ.
//
// Tính trên subtotal trước giảm giá là thu thuế của một khoản khách không
// trả.
func TestThueTinhTrenTienHangSauGiamGia(t *testing.T) {
	c := phienCoHang(t, 200_000)
	if err := c.ApplyDiscount("GIAM50", money.MustNew(50_000, money.VND), domain.BenChiuNenTang, testNow); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}
	if err := c.ApDungPhiVaThue(
		money.Zero(money.VND), chinhSach(800, 10_000_000), testNow,
	); err != nil {
		t.Fatalf("ApDungPhiVaThue: %v", err)
	}

	// 150.000 đã gồm VAT → thuế = 150.000 × 800/10800 = 11.111,1 → 11.111.
	if got := c.TaxAmount().Amount(); got != 11_111 {
		t.Errorf("thuế = %d, mong 11111 — phần VAT trong 150.000 (sau "+
			"giảm giá). Tách trên 200.000 ra 14.815 — ghi hóa đơn phần "+
			"thuế của khoản khách không trả", got)
	}
}

// TestTinhLaiThayVI CONG DON — gọi hai lần cho cùng một kết quả.
//
// Đây là bất biến khiến việc "mọi đường ghi tiền đều gọi lại hàm này" an
// toàn. Nếu nó cộng dồn thì áp mã rồi đổi cách giao sẽ nhân đôi thuế.
func TestTinhLaiThayViCongDon(t *testing.T) {
	c := phienCoHang(t, 100_000)
	cs := chinhSach(800, 10_000_000)
	phi := money.MustNew(30_000, money.VND)

	if err := c.ApDungPhiVaThue(phi, cs, testNow); err != nil {
		t.Fatalf("lần 1: %v", err)
	}
	lan1Thue, lan1Phi := c.TaxAmount().Amount(), c.ShippingFee().Amount()

	if err := c.ApDungPhiVaThue(phi, cs, testNow); err != nil {
		t.Fatalf("lần 2: %v", err)
	}
	if c.TaxAmount().Amount() != lan1Thue || c.ShippingFee().Amount() != lan1Phi {
		t.Errorf("gọi lần hai đổi kết quả: thuế %d→%d, phí %d→%d — hàm này "+
			"phải TÍNH LẠI chứ không cộng dồn",
			lan1Thue, c.TaxAmount().Amount(), lan1Phi, c.ShippingFee().Amount())
	}
}

// TestThueSuatKhongThiKhongThuThue — đặt về 0 là một lựa chọn hợp lệ.
func TestThueSuatKhongThiKhongThuThue(t *testing.T) {
	c := phienCoHang(t, 100_000)
	if err := c.ApDungPhiVaThue(
		money.Zero(money.VND), chinhSach(0, 10_000_000), testNow,
	); err != nil {
		t.Fatalf("ApDungPhiVaThue: %v", err)
	}
	if got := c.TaxAmount().Amount(); got != 0 {
		t.Errorf("thuế = %d, mong 0", got)
	}
}

// TestLamTronNUA LEN theo quy định thuế.
//
// `money.RoundHalfUp` — chú thích của kernel ghi "dùng cho thuế theo quy
// định". Hoa hồng làm tròn XUỐNG; trộn hai quy tắc làm đối soát ra hai
// kết quả khác nhau cho cùng một đơn.
func TestLamTronNuaLen(t *testing.T) {
	// 102.000 × 800/10800 = 7.555,55 → nửa lên = 7.556, xuống = 7.555.
	c := phienCoHang(t, 102_000)
	if err := c.ApDungPhiVaThue(
		money.Zero(money.VND), chinhSach(800, 10_000_000), testNow,
	); err != nil {
		t.Fatalf("ApDungPhiVaThue: %v", err)
	}
	if got := c.TaxAmount().Amount(); got != 7_556 {
		t.Errorf("thuế = %d, mong 7556 (làm tròn NỬA LÊN). Làm tròn xuống "+
			"cho 7555 — đúng quy tắc của hoa hồng, sai quy tắc của thuế", got)
	}
}
