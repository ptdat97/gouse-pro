package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
)

func conTro(n int) *int { return &n }

// TestKiemKeChanSoVuotSucChua.
//
// Cột `quantity_*` là `INT` của PostgreSQL — 32 bit. Go dùng `int` 64 bit
// trên máy chủ thật, nên số lớn hơn ĐI QUA hết kiểm tra ở tầng ứng dụng
// rồi mới hỏng ở câu lệnh ghi, và người dùng nhận 500.
//
// Hàng rào đặt ở DOMAIN chứ không ở một handler: mọi đường ghi — quản trị
// viên kiểm kê, nhà bán kiểm kê, và bất kỳ đường nào thêm sau — đều đi
// qua đây. Đặt ở handler thì đường thứ hai phải nhớ tự chặn.
func TestKiemKeChanSoVuotSucChua(t *testing.T) {
	q, err := domain.NewQuantities(10, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// ĐÚNG trần: nhận.
	if _, err := q.KiemKe(conTro(domain.MaxSoLuong), nil); err != nil {
		t.Errorf("đúng trần (%d) bị từ chối: %v", domain.MaxSoLuong, err)
	}

	// Trần CỘNG MỘT: từ chối, và phải là ErrQuaLon chứ không phải lỗi khác.
	if _, err := q.KiemKe(conTro(domain.MaxSoLuong+1), nil); !errors.Is(err, domain.ErrQuaLon) {
		t.Errorf("trần+1 cho lỗi %v, cần ErrQuaLon — lỗi khác nghĩa là "+
			"tầng HTTP sẽ ánh xạ sai mã trạng thái", err)
	}

	// Số hỏng cũng phải chặn: nó ghi vào cùng kiểu cột.
	if _, err := q.KiemKe(nil, conTro(domain.MaxSoLuong+1)); !errors.Is(err, domain.ErrQuaLon) {
		t.Errorf("số hỏng vượt trần cho lỗi %v, cần ErrQuaLon", err)
	}
}

// TestDieuChinhChanSoVuotSucChua: đường điều chỉnh theo chênh lệch cũng
// cộng dồn tới mức tràn được.
func TestDieuChinhChanSoVuotSucChua(t *testing.T) {
	q, err := domain.NewQuantities(domain.MaxSoLuong-5, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.AdjustAvailable(100); !errors.Is(err, domain.ErrQuaLon) {
		t.Errorf("cộng dồn vượt trần cho lỗi %v, cần ErrQuaLon", err)
	}
	// Vẫn phải cho phép điều chỉnh bình thường.
	if _, err := q.AdjustAvailable(-5); err != nil {
		t.Errorf("điều chỉnh hợp lệ bị từ chối: %v", err)
	}
}
