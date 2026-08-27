package application_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/returns/application"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func dong(niemYet, dieuChinh int64, sl int) application.DongDonHang {
	return application.DongDonHang{
		ID: ids.MustNew(ids.PrefixOrderLine), SKUID: ids.MustNew(ids.PrefixSKU),
		SellerID: ids.MustNew(ids.PrefixSeller), Quantity: sl,
		LineTotal: vnd(niemYet), TongDieuChinh: vnd(dieuChinh),
	}
}

// TestHoanTheoGiaTHUCTRA — điểm dễ sai nhất của cả luồng trả hàng.
//
// Ví dụ lấy thẳng từ docs/07-workflows/return.md mục 5: đơn 500.000đ giảm
// 50.000đ (10%). Món A niêm yết 200.000đ nhưng khách chỉ trả 180.000đ.
//
// Hoàn 200.000đ là mất 20.000đ MỖI LẦN, nhân với tỷ lệ hoàn hàng của
// ngành thời trang.
func TestHoanTheoGiaThucTra(t *testing.T) {
	monA := dong(200_000, -20_000, 1)
	don := application.DonHang{
		DaGiao: true, GiamGiaCapDon: vnd(50_000),
		Dong: []application.DongDonHang{
			monA,
			dong(200_000, -20_000, 1),
			dong(100_000, -10_000, 1),
		},
	}

	got, err := application.TinhTienHoan(don, monA, 1)
	if err != nil {
		t.Fatalf("tính tiền hoàn: %v", err)
	}
	if got.Amount() != 180_000 {
		t.Errorf("hoàn %d, cần 180000 — hoàn theo giá NIÊM YẾT là trả nhiều "+
			"hơn số khách đã đưa", got.Amount())
	}
}

// TestGiamGiaChuaPhanBoThiTuChoiHoan — HÀNG RÀO.
//
// # Vì sao hàng rào này cần thiết NGAY BÂY GIỜ
//
// `checkout.ApplyCoupon` là route sống và nó đặt giảm giá ở CẤP ĐƠN. Nhưng
// `promotion.AllocateDiscount` — hàm phân bổ phần giảm xuống từng dòng —
// KHÔNG ai gọi. Chính comment của nó cảnh báo: "Không lưu lại thì nền tảng
// hoàn nhiều hơn đã thu."
//
// Nên hôm nay, một đơn có mã giảm giá sẽ có `discount_amount > 0` mà mọi
// dòng đều `TongDieuChinh = 0`. Hoàn theo giá dòng khi đó là hoàn thừa
// đúng bằng phần giảm.
//
// Thà TỪ CHỐI và bắt xử lý tay còn hơn âm thầm trả ra số tiền sai.
func TestGiamGiaChuaPhanBoThiTuChoiHoan(t *testing.T) {
	monA := dong(200_000, 0, 1) // KHÔNG có khoản điều chỉnh
	don := application.DonHang{
		DaGiao: true,
		// Đơn CÓ giảm giá…
		GiamGiaCapDon: vnd(50_000),
		Dong:          []application.DongDonHang{monA, dong(300_000, 0, 1)},
	}

	_, err := application.TinhTienHoan(don, monA, 1)
	if !errors.Is(err, domain.ErrGiamGiaChuaPhanBo) {
		t.Errorf("tính được tiền hoàn cho đơn có giảm giá CHƯA phân bổ "+
			"(lỗi: %v) — sẽ hoàn thừa đúng bằng phần giảm", err)
	}
}

// TestKhongGiamGiaThiHoanTronVen — ca thường, không được vướng hàng rào.
func TestKhongGiamGiaThiHoanTronVen(t *testing.T) {
	monA := dong(199_000, 0, 1)
	don := application.DonHang{
		DaGiao: true, GiamGiaCapDon: vnd(0),
		Dong: []application.DongDonHang{monA},
	}

	got, err := application.TinhTienHoan(don, monA, 1)
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
func TestTraMotPhanLamTronXuong(t *testing.T) {
	// 3 món, thực trả 100.000đ → 33.333,33đ mỗi món.
	monA := dong(100_000, 0, 3)
	don := application.DonHang{DaGiao: true, Dong: []application.DongDonHang{monA}}

	got, err := application.TinhTienHoan(don, monA, 1)
	if err != nil {
		t.Fatalf("tính tiền hoàn: %v", err)
	}
	if got.Amount() != 33_333 {
		t.Errorf("hoàn %d cho 1/3 dòng, cần 33333 (làm tròn XUỐNG)", got.Amount())
	}

	// Trả TRỌN dòng thì lấy đủ, không mất phần dư.
	tron, err := application.TinhTienHoan(don, monA, 3)
	if err != nil {
		t.Fatalf("tính tiền hoàn trọn dòng: %v", err)
	}
	if tron.Amount() != 100_000 {
		t.Errorf("trả trọn dòng hoàn %d, cần 100000 — phần dư bị mất",
			tron.Amount())
	}
}

func TestSoLuongVuotThiTuChoi(t *testing.T) {
	monA := dong(100_000, 0, 2)
	don := application.DonHang{DaGiao: true, Dong: []application.DongDonHang{monA}}

	for _, sl := range []int{0, -1, 3} {
		if _, err := application.TinhTienHoan(don, monA, sl); !errors.Is(err, domain.ErrQuantityExceeded) {
			t.Errorf("số lượng %d được chấp nhận", sl)
		}
	}
}
