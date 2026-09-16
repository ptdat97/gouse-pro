package domain_test

import (
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/recommendation/domain"
)

func qs(size string, k domain.KetQua, thuTu int64) domain.QuanSat {
	return domain.QuanSat{Size: size, KetQua: k, ThuTu: thuTu}
}

var thangSML = []string{"S", "M", "L", "XL"}

// TestTraHangThangMoiLanMua.
//
// Một lần trả hàng vì size là khách CHỦ ĐỘNG nói ra rằng size đó sai, và
// sai theo hướng nào. Một lần mua rồi im lặng thì mơ hồ: có thể vừa, có
// thể họ ngại trả. Nên phàn nàn phải thắng.
func TestTraHangThangMoiLanMua(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs("M", domain.KetQuaDaMua, 1),
		qs("M", domain.KetQuaChat, 2),
	}, thangSML)
	if !ok {
		t.Fatal("có phàn nàn CHẬT mà không gợi ý gì")
	}
	if got.Size != "L" {
		t.Errorf("gợi ý %q, cần L — chật thì phải LÊN một size", got.Size)
	}
	if got.LyDo != domain.LyDoTraHang {
		t.Errorf("lý do %q, cần RETURN_HISTORY", got.LyDo)
	}
}

func TestRongThiXuongMotSize(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs("L", domain.KetQuaRong, 1),
	}, thangSML)
	if !ok || got.Size != "M" {
		t.Errorf("gợi ý %q (ok=%v), cần M — rộng thì phải XUỐNG một size",
			got.Size, ok)
	}
}

// TestChuaPhanNanThiDungSizeDaMua — đường yếu hơn, vẫn có ích.
func TestChuaPhanNanThiDungSizeDaMua(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs("M", domain.KetQuaDaMua, 1),
	}, thangSML)
	if !ok || got.Size != "M" || got.LyDo != domain.LyDoDaMua {
		t.Errorf("gợi ý %q lý do %q (ok=%v), cần M / PREVIOUS_PURCHASE",
			got.Size, got.LyDo, ok)
	}
}

// TestPhanNanMoiNhatThang: khách đổi dáng người theo thời gian.
func TestPhanNanMoiNhatThang(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs("M", domain.KetQuaChat, 1), // cũ: chật → L
		qs("L", domain.KetQuaRong, 5), // mới: rộng → M
	}, thangSML)
	if !ok || got.Size != "M" {
		t.Errorf("gợi ý %q, cần M — quan sát MỚI NHẤT phải thắng", got.Size)
	}
}

// TestChatOSizeLonNhatThiKHONGGoiY.
//
// Ca dễ sai nhất. Khách nói XL vẫn chật, mà XL đã là lớn nhất thang — KHÔNG
// có size nào để gợi ý.
//
// Và tuyệt đối KHÔNG được rơi xuống "size đã mua": khách vừa nói size ấy
// sai, gợi ý lại chính nó là phớt lờ điều họ nói, rồi họ trả hàng lần nữa.
func TestChatOSizeLonNhatThiKHONGGoiY(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs("XL", domain.KetQuaDaMua, 1),
		qs("XL", domain.KetQuaChat, 2),
	}, thangSML)
	if ok {
		t.Errorf("gợi ý %q trong khi XL đã là lớn nhất thang — và đó lại "+
			"chính là size khách vừa nói là chật", got.Size)
	}
}

// TestSizeNgoaiThangDangBanThiKHONGGoiY.
//
// Khách từng mua size 38 của một mã giày; sản phẩm đang xem là áo bán theo
// S/M/L. Không có phép quy đổi nào giữa hai thang, nên không gợi ý.
func TestSizeNgoaiThangDangBanThiKHONGGoiY(t *testing.T) {
	if got, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs("38", domain.KetQuaDaMua, 1),
	}, thangSML); ok {
		t.Errorf("gợi ý %q cho size nằm ngoài thang đang bán", got.Size)
	}
}

func TestKhongCoQuanSatThiKHONGGoiY(t *testing.T) {
	if _, ok := domain.SuyLuanSize(nil, thangSML); ok {
		t.Error("gợi ý dù không có quan sát nào")
	}
}

func TestThangRongThiKHONGGoiY(t *testing.T) {
	if _, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs("M", domain.KetQuaDaMua, 1),
	}, nil); ok {
		t.Error("gợi ý dù sản phẩm không bán size nào")
	}
}

// TestGiuNguyenThuTuThangBenGoiDuaVao.
//
// Thang KHÔNG được sắp theo bảng chữ cái: "S, M, L" sắp chữ cái thành
// "L, M, S" và mọi phép "lên một size" đảo chiều. Bài này dùng một thang
// mà thứ tự chữ cái NGƯỢC hẳn thứ tự thật.
func TestGiuNguyenThuTuThangBenGoiDuaVao(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs("S", domain.KetQuaChat, 1),
	}, []string{"S", "M", "L"})
	if !ok || got.Size != "M" {
		t.Errorf("gợi ý %q, cần M — thang phải giữ đúng thứ tự bên gọi "+
			"đưa vào, không sắp lại theo bảng chữ cái", got.Size)
	}
}

// TestKhongPhanBietHoaThuongVaKhoangTrang.
func TestKhongPhanBietHoaThuongVaKhoangTrang(t *testing.T) {
	got, ok := domain.SuyLuanSize([]domain.QuanSat{
		qs(" m ", domain.KetQuaChat, 1),
	}, []string{"s", "m", "l"})
	if !ok || got.Size != "L" {
		t.Errorf("gợi ý %q (ok=%v), cần L", got.Size, ok)
	}
}
