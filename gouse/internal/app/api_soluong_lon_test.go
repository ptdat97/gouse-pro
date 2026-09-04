package app

import (
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
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
	if got := dat(domain.MaxSoLuong); got.code != http.StatusOK {
		t.Errorf("đúng trần (%d) bị từ chối: HTTP %d — %s",
			domain.MaxSoLuong, got.code, got.raw)
	}

	// Trần CỘNG MỘT: phải từ chối, và phải là 400.
	for _, q := range []int64{
		int64(domain.MaxSoLuong) + 1,
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
