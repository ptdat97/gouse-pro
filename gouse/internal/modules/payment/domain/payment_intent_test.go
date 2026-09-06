package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

var mocThoiGian = time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)

func intentChoThu(t *testing.T, soTien int64) *domain.PaymentIntent {
	t.Helper()
	p, err := domain.NewPaymentIntent(domain.NewPaymentIntentParams{
		OrderID:    ids.MustNew(ids.PrefixOrder),
		Amount:     vnd(soTien),
		PhuongThuc: "CARD",
		Now:        mocThoiGian,
	})
	if err != nil {
		t.Fatalf("NewPaymentIntent: %v", err)
	}
	return p
}

// TestSoTienLechThiTuChoiThu là LÝ DO kiểu này tồn tại.
//
// # Vì sao chữ ký HMAC là KHÔNG đủ
//
// Chữ ký chứng minh thông điệp đến TỪ nhà cung cấp. Nó không chứng minh
// con số BÊN TRONG đúng. Ba đường làm số tiền sai mà chữ ký vẫn hợp lệ:
// lỗi tích hợp phía họ, một khóa HMAC bị lộ, và môi trường test gọi nhầm
// vào production.
//
// Cả ba đều kết thúc bằng tiền ghi sai vào một cuốn sổ BẤT BIẾN — ghi rồi
// thì phải ghi bút toán đảo, không xóa được (ADR-0008).
func TestSoTienLechThiTuChoiThu(t *testing.T) {
	// Lệch một đồng vẫn là lệch: không có ngưỡng sai số, không làm tròn.
	// Tiền là số nguyên đơn vị nhỏ nhất, nên "gần bằng" chỉ là chỗ cho một
	// lỗi trốn qua.
	for _, tt := range []struct {
		ten   string
		bao   int64
		daThu bool
	}{
		{"thiếu một đồng", 627_999, false},
		{"thừa một đồng", 628_001, false},
		{"bằng không", 0, false},
		{"gấp đôi", 1_256_000, false},
		{"khớp đúng", 628_000, true},
	} {
		t.Run(tt.ten, func(t *testing.T) {
			p := intentChoThu(t, 628_000)

			err := p.DoiChieuVaThu(vnd(tt.bao), "psp", "pi_x", mocThoiGian)

			if tt.daThu {
				if err != nil {
					t.Fatalf("số tiền KHỚP mà vẫn từ chối: %v", err)
				}
				if !p.DaThu() {
					t.Error("số tiền khớp mà intent chưa chuyển sang đã thu")
				}
				return
			}

			if !errors.Is(err, domain.ErrSoTienKhongKhop) {
				t.Fatalf("lỗi = %v, mong ErrSoTienKhongKhop", err)
			}
			if p.DaThu() {
				t.Error("số tiền LỆCH mà vẫn ghi nhận đã thu — " +
					"đây là đường tiền sai đi thẳng vào sổ cái bất biến")
			}
			if p.Status() != domain.IntentChoThanhToan {
				t.Errorf("trạng thái = %s, phải giữ nguyên chờ thanh toán",
					p.Status())
			}
		})
	}
}

// TestDonViTienTeCungPhaiKhop — 628000 VND và 628000 USD cách nhau hai bậc
// độ lớn, mà chỉ so `amount` thì chúng bằng nhau.
//
// Kiểu lỗi này không tự lộ ra ở môi trường một đơn vị tiền tệ, và lộ ra
// đúng vào ngày mở thị trường thứ hai.
func TestDonViTienTeCungPhaiKhop(t *testing.T) {
	p := intentChoThu(t, 628_000)

	usd, err := money.New(628_000, money.Currency("USD"))
	if err != nil {
		t.Skipf("USD chưa được hỗ trợ: %v", err)
	}

	if err := p.DoiChieuVaThu(usd, "psp", "pi_x", mocThoiGian); !errors.Is(
		err, domain.ErrSoTienKhongKhop) {
		t.Fatalf("lỗi = %v, mong ErrSoTienKhongKhop — chỉ so số mà bỏ qua "+
			"đơn vị tiền tệ là so hai thứ khác nhau", err)
	}
	if p.DaThu() {
		t.Error("khác đơn vị tiền tệ mà vẫn ghi nhận đã thu")
	}
}

// TestGuiTrungKhongThuHaiLan — yêu cầu 2 của webhooks.yaml: nhà cung cấp
// SẼ gửi trùng, và đó là hành vi bình thường chứ không phải lỗi.
//
// Nhưng gửi trùng với số tiền KHÁC thì là tín hiệu xấu thật sự, và phải
// bị từ chối chứ không được nuốt.
func TestGuiTrungKhongThuHaiLan(t *testing.T) {
	p := intentChoThu(t, 628_000)

	if err := p.DoiChieuVaThu(vnd(628_000), "psp", "pi_x", mocThoiGian); err != nil {
		t.Fatalf("lần đầu: %v", err)
	}
	mocDauTien := p.CapturedAt()

	// Gửi lại ĐÚNG số tiền: không lỗi, và không đổi mốc thu tiền.
	sau := mocThoiGian.Add(time.Hour)
	if err := p.DoiChieuVaThu(vnd(628_000), "psp", "pi_x", sau); err != nil {
		t.Fatalf("gửi trùng bị báo lỗi: %v — nhà cung cấp sẽ gửi lại mãi", err)
	}
	if !p.CapturedAt().Equal(mocDauTien) {
		t.Error("gửi trùng làm đổi mốc thu tiền — mốc phải là lần THẬT đầu tiên")
	}

	// Gửi lại với số tiền KHÁC: phải từ chối.
	if err := p.DoiChieuVaThu(vnd(999_000), "psp", "pi_x", sau); !errors.Is(
		err, domain.ErrSoTienKhongKhop) {
		t.Fatalf("lỗi = %v, mong ErrSoTienKhongKhop — gửi lại với số tiền "+
			"khác là dấu hiệu xấu, không phải bản trùng vô hại", err)
	}
}

// TestCODKhongCoIntent khóa quy tắc ở ADR-0017 phần 2.
//
// COD không có cuộc trao đổi nào với cổng thanh toán, nên không có gì để
// đối chiếu và không webhook nào sẽ tới. Tạo intent cho nó là dựng ra một
// hàng đợi rác làm mọi cảnh báo dựa trên tồn đọng thành vô dụng.
func TestCODKhongCoIntent(t *testing.T) {
	for _, pt := range []string{"COD", "", "BITCOIN"} {
		_, err := domain.NewPaymentIntent(domain.NewPaymentIntentParams{
			OrderID:    ids.MustNew(ids.PrefixOrder),
			Amount:     vnd(100_000),
			PhuongThuc: pt,
			Now:        mocThoiGian,
		})
		if !errors.Is(err, domain.ErrPhuongThucKhongTraTruoc) {
			t.Errorf("phương thức %q: lỗi = %v, mong ErrPhuongThucKhongTraTruoc",
				pt, err)
		}
	}

	for _, pt := range []string{"CARD", "BANK_TRANSFER", "E_WALLET"} {
		if _, err := domain.NewPaymentIntent(domain.NewPaymentIntentParams{
			OrderID:    ids.MustNew(ids.PrefixOrder),
			Amount:     vnd(100_000),
			PhuongThuc: pt,
			Now:        mocThoiGian,
		}); err != nil {
			t.Errorf("phương thức trả trước %q bị từ chối: %v", pt, err)
		}
	}
}

// TestIntentKhongDongVeKhongDuoc — intent 0 đồng khớp với MỌI webhook báo
// 0, tức là vô hiệu hóa chính lớp bảo vệ này.
func TestIntentKhongDongVeKhongDuoc(t *testing.T) {
	for _, soTien := range []int64{0, -1} {
		m, err := money.New(soTien, money.VND)
		if err != nil {
			continue // money từ chối sẵn thì càng tốt
		}
		if _, err := domain.NewPaymentIntent(domain.NewPaymentIntentParams{
			OrderID: ids.MustNew(ids.PrefixOrder), Amount: m,
			PhuongThuc: "CARD", Now: mocThoiGian,
		}); err == nil {
			t.Errorf("số tiền %d được chấp nhận — intent này khớp mọi webhook", soTien)
		}
	}
}

// TestThatBaiRoiKhongThuDuoc — đã báo thất bại thì không được thu.
//
// Nếu chuyển được thì một webhook `payment.failed` tới trước và
// `payment.succeeded` giả tới sau sẽ thu được tiền chưa từng có.
func TestThatBaiRoiKhongThuDuoc(t *testing.T) {
	p := intentChoThu(t, 628_000)

	if err := p.GhiThatBai("the bi tu choi", mocThoiGian); err != nil {
		t.Fatalf("GhiThatBai: %v", err)
	}

	err := p.DoiChieuVaThu(vnd(628_000), "psp", "pi_x", mocThoiGian)
	if !errors.Is(err, domain.ErrIntentSaiTrangThai) {
		t.Fatalf("lỗi = %v, mong ErrIntentSaiTrangThai", err)
	}
	if p.DaThu() {
		t.Error("intent đã thất bại mà vẫn thu được tiền")
	}
}
