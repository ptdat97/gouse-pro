package domain_test

import (
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

func timChiSo(ds []domain.ChiSoHieuSuat, ten string) (domain.ChiSoHieuSuat, bool) {
	for _, c := range ds {
		if c.Ten == ten {
			return c, true
		}
	}
	return domain.ChiSoHieuSuat{}, false
}

// TestMauQuaNhoThiKhongCham là hàng rào chống bất công.
//
// Một gian hàng mới có 3 đơn, hủy 1, ra tỷ lệ 33% và bị chấm CRITICAL. Con
// số đó không nói gì về chất lượng — nó nói mẫu quá nhỏ. Đặc tả sinh ra để
// tránh đúng kiểu bất công này ("mô hình chấm điểm hộp đen tạo tranh chấp
// không giải quyết được và cảm giác bất công").
func TestMauQuaNhoThiKhongCham(t *testing.T) {
	ds := domain.TinhChiSo(domain.SoLieuHieuSuat{
		TongDon: 3, DonHuy: 1, DonDaGiao: 2, DonGiaoDungHan: 1,
	})
	if len(ds) != 0 {
		t.Errorf("mẫu 3 đơn vẫn bị chấm %d chỉ số — gian hàng mới mở hủy "+
			"một đơn sẽ bị đánh giá NGHIÊM TRỌNG: %+v", len(ds), ds)
	}
}

// TestDuMauThiCham: qua ngưỡng mẫu thì phải có chỉ số.
func TestDuMauThiCham(t *testing.T) {
	ds := domain.TinhChiSo(domain.SoLieuHieuSuat{
		TongDon: 100, DonHuy: 2, DonDaGiao: 90, DonGiaoDungHan: 88,
	})
	if len(ds) != 2 {
		t.Fatalf("cần 2 chỉ số, có %d: %+v", len(ds), ds)
	}

	huy, _ := timChiSo(ds, "cancellation_rate")
	if huy.GiaTri != 0.02 {
		t.Errorf("tỷ lệ hủy = %v, cần 0.02", huy.GiaTri)
	}
	if huy.TrangThai != domain.ChiSoTot {
		t.Errorf("hủy 2%% dưới ngưỡng 3%% mà xếp %s", huy.TrangThai)
	}
}

// TestHaiChiSoNGUOC CHIỀU nhau, và nhầm chiều là lỗi im lặng.
//
// `cancellation_rate` càng THẤP càng tốt; `on_time_shipping_rate` càng CAO
// càng tốt. Dùng chung một phép so sánh cho cả hai sẽ cho ra kết luận
// ngược hẳn — và không có gì báo, vì cả hai đều là số trong khoảng 0..1.
func TestHaiChiSoNguocChieuNhau(t *testing.T) {
	// Hủy 10% — RẤT xấu. Giao đúng hạn 10% — cũng RẤT xấu.
	ds := domain.TinhChiSo(domain.SoLieuHieuSuat{
		TongDon: 100, DonHuy: 10, DonDaGiao: 100, DonGiaoDungHan: 10,
	})

	huy, ok := timChiSo(ds, "cancellation_rate")
	if !ok {
		t.Fatal("thiếu cancellation_rate")
	}
	if huy.TrangThai != domain.ChiSoNghiemTrong {
		t.Errorf("hủy 10%% (ngưỡng 3%%) xếp %s, cần CRITICAL", huy.TrangThai)
	}

	dungHan, ok := timChiSo(ds, "on_time_shipping_rate")
	if !ok {
		t.Fatal("thiếu on_time_shipping_rate")
	}
	if dungHan.TrangThai != domain.ChiSoNghiemTrong {
		t.Errorf("giao đúng hạn 10%% (ngưỡng 95%%) xếp %s, cần CRITICAL",
			dungHan.TrangThai)
	}
}

// TestVungCanhBao: không có vùng đệm thì chỉ số nhảy giữa TỐT và NGHIÊM
// TRỌNG chỉ vì một đơn, và nhà bán không có tín hiệu sớm nào.
func TestVungCanhBao(t *testing.T) {
	// 92% đúng hạn: dưới ngưỡng 95% nhưng trên 90% × 95% = 85,5%.
	ds := domain.TinhChiSo(domain.SoLieuHieuSuat{
		TongDon: 100, DonHuy: 0, DonDaGiao: 100, DonGiaoDungHan: 92,
	})
	dungHan, _ := timChiSo(ds, "on_time_shipping_rate")
	if dungHan.TrangThai != domain.ChiSoCanhBao {
		t.Errorf("đúng hạn 92%% xếp %s, cần WARNING — không có vùng đệm thì "+
			"nhà bán chỉ biết mình có vấn đề khi đã NGHIÊM TRỌNG",
			dungHan.TrangThai)
	}
}

// TestKhongChiaChoKhong: không có đơn nào đã giao thì không tính tỷ lệ
// đúng hạn — chia cho 0 cho ra NaN, và NaN đi thẳng vào JSON là một trường
// không parse được ở phía client.
func TestKhongChiaChoKhong(t *testing.T) {
	ds := domain.TinhChiSo(domain.SoLieuHieuSuat{
		TongDon: 50, DonHuy: 50, DonDaGiao: 0, DonGiaoDungHan: 0,
	})
	if _, co := timChiSo(ds, "on_time_shipping_rate"); co {
		t.Error("không có đơn nào đã giao mà vẫn tính tỷ lệ đúng hạn")
	}
	// Tỷ lệ hủy vẫn tính được và phải là 100%.
	huy, ok := timChiSo(ds, "cancellation_rate")
	if !ok || huy.GiaTri != 1 {
		t.Errorf("tỷ lệ hủy = %v (có=%v), cần 1", huy.GiaTri, ok)
	}
}

// TestChiSoChuaDoNoiRoLyDo.
//
// Trả một phần chỉ số rồi im lặng về phần còn lại tạo ra đúng thứ hộp đen
// mà đặc tả sinh ra để tránh — chỉ khác là ở phía người viết API.
func TestChiSoChuaDoNoiRoLyDo(t *testing.T) {
	ds := domain.ChiSoChuaDo()
	if len(ds) == 0 {
		t.Fatal("không khai chỉ số nào là chưa đo")
	}
	for _, c := range ds {
		if c.Ten == "" {
			t.Error("có mục thiếu tên chỉ số")
		}
		if len(c.LyDo) < 20 {
			t.Errorf("lý do của %q quá ngắn (%q) — một dòng không giải "+
				"thích được thì không khác gì im lặng", c.Ten, c.LyDo)
		}
	}
}
