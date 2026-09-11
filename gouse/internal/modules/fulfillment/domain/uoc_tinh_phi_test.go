package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
)

func nguon(sellerIDs ...string) []domain.NguonHang {
	out := make([]domain.NguonHang, 0, len(sellerIDs))
	for _, id := range sellerIDs {
		out = append(out, domain.NguonHang{SellerID: id})
	}
	return out
}

// TestPhiTinhTHEO TUNG NGUON — đây là toàn bộ điểm của P3-8.
//
// # Lỗi đang sửa
//
// Biểu phí cũ nằm trong checkout và thu MỘT lần cho cả đơn, bất kể hàng
// đến từ mấy nguồn. Nhưng hàng của ba nhà bán nằm ở ba kho khác nhau nên
// nó đi thành BA kiện và tốn ba lần phí.
//
// Nghĩa là nền tảng bù phần chênh trên mọi đơn nhiều nguồn — và bù nhiều
// nhất đúng ở loại đơn mà cái chợ tồn tại để tạo ra. Không lỗi nào báo,
// không con số nào âm; nó chỉ hiện ra ở bảng lãi lỗ.
func TestPhiTinhTheoTungNguon(t *testing.T) {
	for _, tt := range []struct {
		ten      string
		nguon    []domain.NguonHang
		mongTong int64
	}{
		{"một nhà bán", nguon("sel_a"), 30_000},
		{"hai nhà bán", nguon("sel_a", "sel_b"), 60_000},
		{"ba nhà bán", nguon("sel_a", "sel_b", "sel_c"), 90_000},
	} {
		t.Run(tt.ten, func(t *testing.T) {
			got, err := domain.UocTinhPhiGiao(
				domain.GiaoTieuChuan, tt.nguon, money.VND, domain.BieuPhiMacDinh)
			if err != nil {
				t.Fatalf("UocTinhPhiGiao: %v", err)
			}
			if got.Tong.Amount() != tt.mongTong {
				t.Errorf("tổng phí = %d, mong %d — phí KHÔNG theo số nguồn "+
					"nghĩa là nền tảng bù phần chênh trên mọi đơn nhiều nhà bán",
					got.Tong.Amount(), tt.mongTong)
			}
			if len(got.TheoNguon) != len(tt.nguon) {
				t.Errorf("chi tiết có %d dòng, mong %d",
					len(got.TheoNguon), len(tt.nguon))
			}
		})
	}
}

// TestChiTietTheoNguonMangThoiGianGiao — mục 7 của đặc tả yêu cầu hiển thị
// thời gian giao RIÊNG cho từng nhóm hàng, không gộp thành một con số.
//
// "Khách cần biết món nào đến trước." Gộp lại là bỏ mất thông tin đó, và
// không lấy lại được ở tầng trên.
func TestChiTietTheoNguonMangThoiGianGiao(t *testing.T) {
	got, err := domain.UocTinhPhiGiao(
		domain.GiaoNhanh, nguon("sel_a", "sel_b"), money.VND, domain.BieuPhiMacDinh)
	if err != nil {
		t.Fatalf("UocTinhPhiGiao: %v", err)
	}

	for i, p := range got.TheoNguon {
		if p.SellerID == "" {
			t.Errorf("dòng %d không có mã nhà bán — trang không ghép được "+
				"phí với nhóm hàng nào", i)
		}
		if p.SoNgayDuKien <= 0 {
			t.Errorf("dòng %d có số ngày = %d, phải > 0", i, p.SoNgayDuKien)
		}
	}
}

// TestGiaoNhanhDatHonVaNhanhHon — nếu hai phương thức cho cùng phí và cùng
// thời gian thì việc cho khách chọn là vô nghĩa.
func TestGiaoNhanhDatHonVaNhanhHon(t *testing.T) {
	tc, err := domain.UocTinhPhiGiao(domain.GiaoTieuChuan, nguon("sel_a"), money.VND, domain.BieuPhiMacDinh)
	if err != nil {
		t.Fatalf("tiêu chuẩn: %v", err)
	}
	nhanh, err := domain.UocTinhPhiGiao(domain.GiaoNhanh, nguon("sel_a"), money.VND, domain.BieuPhiMacDinh)
	if err != nil {
		t.Fatalf("nhanh: %v", err)
	}

	if nhanh.Tong.Amount() <= tc.Tong.Amount() {
		t.Errorf("giao nhanh %d không đắt hơn tiêu chuẩn %d",
			nhanh.Tong.Amount(), tc.Tong.Amount())
	}
	if nhanh.TheoNguon[0].SoNgayDuKien >= tc.TheoNguon[0].SoNgayDuKien {
		t.Errorf("giao nhanh %d ngày không nhanh hơn tiêu chuẩn %d ngày",
			nhanh.TheoNguon[0].SoNgayDuKien, tc.TheoNguon[0].SoNgayDuKien)
	}
}

// TestKhongCoNguonThiPhiBangKhong — không có gì để giao thì không thu.
func TestKhongCoNguonThiPhiBangKhong(t *testing.T) {
	got, err := domain.UocTinhPhiGiao(domain.GiaoTieuChuan, nil, money.VND, domain.BieuPhiMacDinh)
	if err != nil {
		t.Fatalf("UocTinhPhiGiao: %v", err)
	}
	if got.Tong.Amount() != 0 {
		t.Errorf("tổng phí = %d cho đơn không có nguồn nào, mong 0",
			got.Tong.Amount())
	}
}

// TestPhuongThucLaThiTuChoi — tập ĐÓNG.
//
// Rơi về một mức mặc định là đoán tiền khách phải trả.
func TestPhuongThucLaThiTuChoi(t *testing.T) {
	_, err := domain.UocTinhPhiGiao(
		domain.PhuongThucGiao("TAU_VU_TRU"), nguon("sel_a"), money.VND, domain.BieuPhiMacDinh)
	if !errors.Is(err, domain.ErrPhuongThucKhongHopLe) {
		t.Fatalf("lỗi = %v, mong ErrPhuongThucKhongHopLe", err)
	}
}

// TestPhiGiuNGUYEN DON VI TIEN TE của đơn.
//
// Trả phí bằng VND cho một đơn tính bằng USD thì phép cộng vào tổng đơn
// hoặc lỗi, hoặc tệ hơn: cộng hai con số khác đơn vị thành một số vô nghĩa.
func TestPhiGiuNguyenDonViTienTe(t *testing.T) {
	got, err := domain.UocTinhPhiGiao(domain.GiaoTieuChuan, nguon("sel_a"), money.USD, domain.BieuPhiMacDinh)
	if err != nil {
		t.Fatalf("UocTinhPhiGiao: %v", err)
	}
	if got.Tong.Currency() != money.USD {
		t.Errorf("đơn vị = %s, mong USD", got.Tong.Currency())
	}
}
