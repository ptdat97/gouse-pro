package domain_test

import (
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

// TestHanBanGiaoLaCreatedAtCongSLA — công thức phải khớp phép chấm điểm.
//
// Truy vấn chấm hiệu suất đếm đơn đúng hạn bằng
// `shipped_at <= created_at + sla`. Nếu hàm này cho ra mốc khác, nhà bán
// sẽ thấy "còn 3 giờ" trên màn hình trong khi báo cáo đã ghi họ trễ — hai
// câu trả lời cho cùng một câu hỏi, và câu nào cũng có vẻ đúng.
func TestHanBanGiaoLaCreatedAtCongSLA(t *testing.T) {
	fo := foMoiTao(t)
	sla := 48 * time.Hour

	mong := fo.CreatedAt().Add(sla)
	if got := fo.HanBanGiao(sla); !got.Equal(mong) {
		t.Errorf("hạn = %v, mong %v = created_at + SLA", got, mong)
	}
}

// TestDoiSLAThiDOI HAN của đơn ĐANG CHẠY — hành vi có chủ ý.
//
// Hạn được TÍNH RA chứ không lưu, nên đổi `fulfillment.shipping_sla_hours`
// dời hạn của mọi đơn đang chạy. Đó là điều `HeQua` của tham số đó đã ghi:
// "đổi con số này làm ĐỔI ĐIỂM hiệu suất của mọi nhà bán ở kỳ đang xem —
// kể cả những đơn đã giao xong từ trước".
//
// Bài này khóa hành vi đó để nó không bị đổi âm thầm: lưu hạn thành cột sẽ
// làm hạn hiển thị đứng yên trong khi điểm hiệu suất vẫn đổi theo cấu hình.
func TestDoiSLAThiDoiHanCuaDonDangChay(t *testing.T) {
	fo := foMoiTao(t)

	h48 := fo.HanBanGiao(48 * time.Hour)
	h24 := fo.HanBanGiao(24 * time.Hour)

	if !h24.Before(h48) {
		t.Errorf("hạ SLA từ 48h xuống 24h mà hạn không sớm lại: %v vs %v", h24, h48)
	}
	if got := h48.Sub(h24); got != 24*time.Hour {
		t.Errorf("chênh lệch = %v, mong 24h", got)
	}
}

// TestDonDaBanGiaoKHONG mang cờ trễ, dù bàn giao muộn.
//
// Việc bàn giao muộn đã tính vào điểm hiệu suất. Hiện lại nhãn "trễ" trên
// một đơn đang đi đường làm nhà bán tưởng còn việc phải làm — và việc đó
// không tồn tại.
func TestDonDaBanGiaoKhongMangCoTre(t *testing.T) {
	sla := 48 * time.Hour
	fo := foDaBanGiao(t, 72*time.Hour) // bàn giao MUỘN 24 giờ

	rat := fo.CreatedAt().Add(200 * time.Hour)
	if fo.TreHan(sla, rat) {
		t.Error("đơn ĐÃ bàn giao vẫn mang cờ trễ — nhà bán tưởng còn việc phải làm")
	}
	// Nhưng nó KHÔNG đúng hạn: điểm hiệu suất phải thấy điều đó.
	if fo.BanGiaoDungHan(sla) {
		t.Error("bàn giao muộn 24 giờ mà vẫn tính là đúng hạn")
	}
}

// TestBanGiaoDUNG mốc hạn vẫn tính là đúng hạn.
//
// Truy vấn chấm điểm dùng `<=`, nên mốc hạn thuộc về bên ĐÚNG HẠN. Lệch
// một giây ở đây làm điểm hiệu suất khác báo cáo.
func TestBanGiaoDungMocHanVanTinhLaDungHan(t *testing.T) {
	sla := 48 * time.Hour

	if fo := foDaBanGiao(t, sla); !fo.BanGiaoDungHan(sla) {
		t.Error("bàn giao ĐÚNG mốc hạn mà bị tính là trễ — " +
			"truy vấn chấm điểm dùng `<=`, hàm này phải khớp")
	}
	if fo := foDaBanGiao(t, sla+time.Second); fo.BanGiaoDungHan(sla) {
		t.Error("bàn giao muộn một giây mà vẫn tính đúng hạn")
	}
}

// TestChuaBanGiaoVaQuaHanThiTRE — việc cần làm NGAY.
func TestChuaBanGiaoVaQuaHanThiTre(t *testing.T) {
	sla := 48 * time.Hour
	fo := foMoiTao(t)

	truocHan := fo.CreatedAt().Add(47 * time.Hour)
	if fo.TreHan(sla, truocHan) {
		t.Error("còn 1 giờ mà đã báo trễ")
	}

	sauHan := fo.CreatedAt().Add(49 * time.Hour)
	if !fo.TreHan(sla, sauHan) {
		t.Error("quá hạn 1 giờ mà không báo trễ — nhà bán không thấy việc cần làm")
	}
}

// TestDonDaHUY khong mang co tre.
//
// Đơn đã hủy không còn việc gì để làm; báo trễ chỉ làm nhiễu danh sách
// việc của nhà bán.
func TestDonDaHuyKhongMangCoTre(t *testing.T) {
	fo := foDaHuy(t)
	rat := fo.CreatedAt().Add(200 * time.Hour)
	if fo.TreHan(48*time.Hour, rat) {
		t.Error("đơn ĐÃ HỦY vẫn mang cờ trễ")
	}
}

var _ = domain.SLAGiaoHang

// foMoiTao dựng một đơn thực hiện vừa được tách, chưa làm gì.
func foMoiTao(t *testing.T) *domain.FulfillmentOrder {
	t.Helper()
	fos, err := domain.SplitIntoFulfillmentOrders(
		splitInput(line(ids.MustNew(ids.PrefixSeller), 300_000, 30_000)), testNow)
	if err != nil {
		t.Fatalf("SplitIntoFulfillmentOrders: %v", err)
	}
	return fos[0]
}

// foDaBanGiao dựng một đơn ĐÃ bàn giao sau `sau` kể từ lúc tạo.
func foDaBanGiao(t *testing.T, sau time.Duration) *domain.FulfillmentOrder {
	t.Helper()
	fo := foMoiTao(t)
	moc := fo.CreatedAt()

	for i, buoc := range []struct {
		ten string
		lam func(time.Time) error
	}{
		{"xác nhận", fo.Confirm},
		{"nhặt hàng", fo.Pick},
		{"đóng gói", fo.Pack},
	} {
		if err := buoc.lam(moc.Add(time.Duration(i+1) * time.Minute)); err != nil {
			t.Fatalf("%s: %v", buoc.ten, err)
		}
	}
	if err := fo.HandOver("GHN", "TRACK-001", domain.BieuPhiMacDinh, moc.Add(sau)); err != nil {
		t.Fatalf("bàn giao: %v", err)
	}
	return fo
}

// foDaHuy dựng một đơn đã hủy.
func foDaHuy(t *testing.T) *domain.FulfillmentOrder {
	t.Helper()
	fo := foMoiTao(t)
	if err := fo.Cancel("hết hàng thật", fo.CreatedAt().Add(time.Hour)); err != nil {
		t.Fatalf("hủy: %v", err)
	}
	return fo
}
