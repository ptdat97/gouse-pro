package money_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// TestChuanHoaChuHoaTienTe.
//
// `valid()` bản đầu chỉ kiểm `len(c) == 3`, nên "vnd" đi lọt vào hệ thống,
// nằm được trong cột `CHAR(3)` (không có ràng buộc chữ hoa), rồi ra tới
// giao diện.
//
// Ở đó, phép so `currency === "VND"` phân biệt hoa thường coi nó KHÔNG
// phải VND, chia số tiền cho 100, và hiện 250.000 ₫ thành 2.500 ₫ — vẫn
// kèm ký hiệu ₫ nên trông hoàn toàn hợp lệ.
//
// Chuẩn hóa ở `New` che cả đường ĐỌC LẠI: kho lưu trữ dựng Money bằng hàm
// này, nên dữ liệu cũ mang chữ thường được sửa lúc đọc.
func TestChuanHoaChuHoaTienTe(t *testing.T) {
	for _, vao := range []money.Currency{"vnd", "Vnd", "vND", "VND"} {
		m, err := money.New(250_000, vao)
		if err != nil {
			t.Fatalf("New(%q): %v", vao, err)
		}
		if m.Currency() != "VND" {
			t.Errorf("New(%q) cho currency %q, cần VND — chữ thường lọt "+
				"xuống giao diện làm giá hiện sai 100 lần", vao, m.Currency())
		}
	}
}

// TestTuChoiMaTienTeHong.
//
// Mã không phải ba chữ cái làm `Intl.NumberFormat` ở giao diện NÉM lỗi, và
// một lỗi ném lúc render làm trắng cả trang.
//
// Chặn từ gốc rẻ hơn: giao diện vẫn có lớp phòng thủ riêng, nhưng dữ liệu
// hỏng không nên vào được hệ thống ngay từ đầu.
func TestTuChoiMaTienTeHong(t *testing.T) {
	for _, vao := range []money.Currency{"", "VN", "VNDD", "12 ", "€$¥", "V1D"} {
		if _, err := money.New(1000, vao); !errors.Is(err, money.ErrInvalidCurrency) {
			t.Errorf("New(%q) cho lỗi %v, cần ErrInvalidCurrency", vao, err)
		}
	}
}

// TestChuanHoaKhongLamHongPhepSoSanh: hai Money khác chữ hoa thường phải
// coi là CÙNG đơn vị, không phải khác.
func TestChuanHoaKhongLamHongPhepSoSanh(t *testing.T) {
	a, err := money.New(100, "vnd")
	if err != nil {
		t.Fatal(err)
	}
	b, err := money.New(200, "VND")
	if err != nil {
		t.Fatal(err)
	}
	tong, err := a.Add(b)
	if err != nil {
		t.Fatalf("cộng hai Money cùng đơn vị khác chữ hoa: %v — chuẩn hóa "+
			"đáng lẽ làm chúng bằng nhau", err)
	}
	if tong.Amount() != 300 {
		t.Errorf("tổng = %d, cần 300", tong.Amount())
	}
}
