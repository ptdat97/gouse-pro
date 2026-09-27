package domain_test

import (
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/affiliate/domain"
)

// Tiền gốc TRỪ khuyến mãi đã phân bổ.
//
// Đây là bài quan trọng nhất của file này. Tính hoa hồng trên giá niêm yết
// của một đơn có giảm giá là trả creator nhiều hơn doanh thu họ tạo ra:
// khách trả 180.000 mà nền tảng trả hoa hồng theo 200.000. Với mã giảm sâu,
// phần chênh ấy ăn hết lãi.
//
// Khoản khuyến mãi là số ÂM, nên hàm CỘNG chúng vào.
func TestTienGocTruKhuyenMai(t *testing.T) {
	goc := domain.TienGocTinhHoaHong(vnd(200_000), []money.Money{vnd(-20_000)})

	if got := goc.Amount(); got != 180_000 {
		t.Fatalf("tiền gốc = %d, cần 180.000 (200.000 − 20.000)", got)
	}
}

// NHIỀU khoản khuyến mãi cùng cộng vào.
//
// Một dòng có thể nhận cả mã giảm của nền tảng và chương trình của nhà bán,
// mỗi cái một khoản riêng. Chỉ trừ khoản đầu tiên là trả hoa hồng trên phần
// khách không trả.
func TestTienGocTruNhieuKhoan(t *testing.T) {
	goc := domain.TienGocTinhHoaHong(vnd(500_000), []money.Money{
		vnd(-50_000), vnd(-30_000), vnd(-20_000),
	})

	if got := goc.Amount(); got != 400_000 {
		t.Fatalf("tiền gốc = %d, cần 400.000 — phải trừ CẢ BA khoản", got)
	}
}

// KHÔNG có khoản nào thì tiền gốc bằng tiền hàng.
func TestTienGocKhongKhoanNaoThiBangTienHang(t *testing.T) {
	for _, km := range [][]money.Money{nil, {}} {
		goc := domain.TienGocTinhHoaHong(vnd(120_000), km)
		if got := goc.Amount(); got != 120_000 {
			t.Errorf("tiền gốc = %d, cần 120.000", got)
		}
	}
}

// Giảm QUÁ tiền hàng thì KẸP về 0, không ra số âm.
//
// Một dòng được giảm 100% (hàng tặng, đền bù) cho tiền gốc âm nếu phân bổ
// lệch một đồng. Hoa hồng ÂM nghĩa là đòi tiền lại của creator qua một
// đường không ai thiết kế — và `commission_base >= 0` ở migration 000058 sẽ
// từ chối ghi, nên lỗi hiện ra ở tầng database dưới dạng một event mắc kẹt
// trong outbox thay vì một con số sai nhìn thấy được.
func TestTienGocKepVe0(t *testing.T) {
	goc := domain.TienGocTinhHoaHong(vnd(100_000), []money.Money{vnd(-150_000)})

	if got := goc.Amount(); got != 0 {
		t.Fatalf("tiền gốc = %d, cần 0 — giảm quá tiền hàng phải kẹp về 0, "+
			"không cho hoa hồng ÂM", got)
	}
	if goc.Currency() != money.VND {
		t.Errorf("kẹp về 0 mà mất đơn vị tiền: %q", goc.Currency())
	}
}

// Giảm ĐÚNG BẰNG tiền hàng cho 0, và đó là câu trả lời đúng.
//
// Tách khỏi bài trên: 0 do cộng ra khác 0 do kẹp, và một bản cài kẹp sai
// dấu sẽ xanh ở một bài mà đỏ ở bài kia.
func TestTienGocGiamHetThiBang0(t *testing.T) {
	goc := domain.TienGocTinhHoaHong(vnd(90_000), []money.Money{vnd(-90_000)})

	if got := goc.Amount(); got != 0 {
		t.Fatalf("tiền gốc = %d, cần 0", got)
	}
}

// Khoản KHÁC đơn vị tiền bị BỎ QUA, không làm sai con số.
//
// Hệ thống một đơn vị tiền nên đây là nhánh không đến được. Bài này khẳng
// định hành vi khi nó đến: thà tính hoa hồng trên tiền hàng chưa giảm — một
// khoản hơi cao — còn hơn cộng một con số vô nghĩa vào tiền của creator.
func TestTienGocBoQuaKhoanKhacDonViTien(t *testing.T) {
	goc := domain.TienGocTinhHoaHong(vnd(200_000), []money.Money{
		money.MustNew(-10, money.USD),
		vnd(-20_000),
	})

	if got := goc.Amount(); got != 180_000 {
		t.Fatalf("tiền gốc = %d, cần 180.000 — khoản USD phải bị bỏ qua, "+
			"khoản VND vẫn phải trừ", got)
	}
	if goc.Currency() != money.VND {
		t.Errorf("đơn vị tiền = %q, cần VND", goc.Currency())
	}
}

// Hoa hồng tính TRÊN tiền gốc đã trừ, không trên giá niêm yết.
//
// Nối hai mảnh lại: `TienGocTinhHoaHong` cho đúng số, và `TaoAttribution`
// đóng băng đúng số ấy. Hai bài riêng có thể cùng xanh mà chỗ NỐI vẫn sai —
// đúng dạng lỗi mà bài này bắt.
func TestHoaHongTinhTrenTienDaTruGiam(t *testing.T) {
	goc := domain.TienGocTinhHoaHong(vnd(200_000), []money.Money{vnd(-20_000)})

	a := quyCongVoiGoc(t, goc, 500)
	if got := a.HoaHong().Amount(); got != 9_000 {
		t.Fatalf("hoa hồng = %d, cần 9.000 (5%% của 180.000) — nếu là 10.000 "+
			"thì phép tính đang dùng giá NIÊM YẾT", got)
	}
}
