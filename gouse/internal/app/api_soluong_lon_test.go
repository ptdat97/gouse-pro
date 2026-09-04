package app

import (
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
	"github.com/fashion-commerce/platform/internal/platform/opsconfig"
)

// TestSoLuongVuotSucChuaTra400.
//
// # Chỗ lệch giữa Go và PostgreSQL
//
// Cột `quantity_*` là `INT` của PostgreSQL — 32 bit, trần 2.147.483.647.
// Go dùng `int` 64 bit trên máy chủ thật, nên một giá trị lớn hơn ĐI QUA
// hết mọi kiểm tra ở tầng ứng dụng rồi mới hỏng ở câu lệnh ghi.
//
// Hệ quả trước khi sửa: người kiểm kê gõ thừa vài số 0 và nhận về "Đã có
// lỗi xảy ra" (500) thay vì một lời nhắc sửa. Họ đi báo sự cố, còn giám
// sát đếm nó vào tỷ lệ lỗi máy chủ — che mất lỗi thật.
//
// Bài này đo ở RANH GIỚI, không chỉ ở con số lố bịch: đúng trần phải nhận,
// trần cộng một phải từ chối.
func TestSoLuongVuotSucChuaTra400(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleOpsWarehouse)
	sku, loc, own, _ := a.mauTonKho(t)

	dat := func(q int64) reply {
		h := khoaIdem()
		h["Authorization"] = "Bearer " + tok
		return a.call(http.MethodPost, "/api/v1/admin/inventory/adjustments",
			map[string]any{
				"sku_id":             sku,
				"stock_location_id":  loc,
				"inventory_owner_id": own,
				"reason":             "kiểm kê thử ranh giới số lượng, bài test tự động",
				"adjustments":        map[string]any{"quantity_available": q},
			}, h)
	}

	// ĐÚNG trần: phải nhận.
	if got := dat(domain.TranMacDinh); got.code != http.StatusOK {
		t.Errorf("đúng trần (%d) bị từ chối: HTTP %d — %s",
			domain.TranMacDinh, got.code, got.raw)
	}

	// Trần CỘNG MỘT: phải từ chối, và phải là 400.
	for _, q := range []int64{
		int64(domain.TranMacDinh) + 1,
		3_000_000_000,
		9_000_000_000_000,
	} {
		got := dat(q)
		if got.code >= 500 {
			t.Errorf("số lượng %d trả HTTP %d — đây là lỗi của người gõ, "+
				"không phải lỗi máy chủ, và 500 khiến họ đi báo sự cố thay "+
				"vì sửa con số: %s", q, got.code, got.raw)
		}
		if got.code != http.StatusBadRequest {
			t.Errorf("số lượng %d trả HTTP %d, cần 400: %s",
				q, got.code, got.raw)
		}
	}
}

// TestDoiTranSoLuongTuGiaoDienCoHieuLucNgay.
//
// Trần số lượng là tham số VẬN HÀNH: mặc định 10 triệu đơn vị cho một SKU
// tại một kho — con số không kho thời trang nào chạm tới, nên vượt nó gần
// như chắc chắn là gõ thừa số 0.
//
// Nhưng có mặt hàng đếm bằng đơn vị nhỏ (chỉ, cúc, hạt cườm), nên con số
// phải nâng được mà không cần build lại.
func TestDoiTranSoLuongTuGiaoDienCoHieuLucNgay(t *testing.T) {
	a := newAPITest(t)
	admin := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)
	kho := a.taoTaiKhoanVaiTro(t, identity.RoleOpsWarehouse)
	sku, loc, own, _ := a.mauTonKho(t)

	dat := func(q int64) reply {
		h := khoaIdem()
		h["Authorization"] = "Bearer " + kho
		return a.call(http.MethodPost, "/api/v1/admin/inventory/adjustments",
			map[string]any{
				"sku_id": sku, "stock_location_id": loc,
				"inventory_owner_id": own,
				"reason":             "kiểm kê thử trần sửa được, bài test tự động",
				"adjustments":        map[string]any{"quantity_available": q},
			}, h)
	}

	// Mặc định 10 triệu: 20 triệu bị từ chối.
	if got := dat(20_000_000); got.code != http.StatusBadRequest {
		t.Fatalf("20 triệu với trần mặc định: HTTP %d, cần 400 — %s",
			got.code, got.raw)
	}

	// Nâng trần lên 50 triệu.
	if got := a.datCauHinh(t, admin, opsconfig.KeyTranSoLuongSKU, 50_000_000,
		"nâng trần tồn kho cho mặt hàng đếm bằng đơn vị nhỏ như cúc và chỉ",
	); got.code != http.StatusOK {
		t.Fatalf("đổi trần: HTTP %d — %s", got.code, got.raw)
	}

	// Cùng con số, giờ phải nhận — không cần khởi động lại.
	if got := dat(20_000_000); got.code != http.StatusOK {
		t.Errorf("20 triệu sau khi nâng trần lên 50 triệu: HTTP %d — "+
			"cấu hình không tới được chỗ dùng nó: %s", got.code, got.raw)
	}
}

// TestTranSoLuongKhongDatVuotSucChuaDuoc.
//
// Sổ đăng ký kẹp `Max` ở trần lưu trữ. Không có nó, một quản trị viên đặt
// trần nghiệp vụ 5 tỷ sẽ làm mọi con số dưới mức đó ĐI QUA kiểm tra rồi
// hỏng ở câu lệnh ghi — tức tự tay dựng lại đúng lỗi 500 mà trần này sinh
// ra để tránh.
func TestTranSoLuongKhongDatVuotSucChuaDuoc(t *testing.T) {
	a := newAPITest(t)
	admin := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	got := a.datCauHinh(t, admin, opsconfig.KeyTranSoLuongSKU, 5_000_000_000,
		"thử đặt trần nghiệp vụ vượt sức chứa của cột trong database")
	if got.code != http.StatusBadRequest {
		t.Errorf("đặt trần 5 tỷ (> sức chứa int32): HTTP %d, cần 400 — %s",
			got.code, got.raw)
	}
}
