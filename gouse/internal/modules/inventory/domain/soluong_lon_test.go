package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
)

func ptr(n int) *int { return &n }

// TestCountRejectsQuantityBeyondStorage.
//
// Cột `quantity_*` là `INT` của PostgreSQL — 32 bit. Go dùng `int` 64 bit
// trên máy chủ thật, nên số lớn hơn ĐI QUA hết kiểm tra ở tầng ứng dụng
// rồi mới hỏng ở câu lệnh ghi, và người dùng nhận 500.
//
// Hàng rào đặt ở DOMAIN chứ không ở một handler: mọi đường ghi — quản trị
// viên kiểm kê, nhà bán kiểm kê, và bất kỳ đường nào thêm sau — đều đi
// qua đây. Đặt ở handler thì đường thứ hai phải nhớ tự chặn.
func TestCountRejectsQuantityBeyondStorage(t *testing.T) {
	q, err := domain.NewQuantities(10, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// ĐÚNG trần: nhận.
	if _, err := q.Count(ptr(domain.DefaultCap), nil, 0); err != nil {
		t.Errorf("đúng trần (%d) bị từ chối: %v", domain.DefaultCap, err)
	}

	// Trần CỘNG MỘT: từ chối, và phải là ErrTooLarge chứ không phải lỗi khác.
	if _, err := q.Count(ptr(domain.DefaultCap+1), nil, 0); !errors.Is(err, domain.ErrTooLarge) {
		t.Errorf("trần+1 cho lỗi %v, cần ErrTooLarge — lỗi khác nghĩa là "+
			"tầng HTTP sẽ ánh xạ sai mã trạng thái", err)
	}

	// Số hỏng cũng phải chặn: nó ghi vào cùng kiểu cột.
	if _, err := q.Count(nil, ptr(domain.DefaultCap+1), 0); !errors.Is(err, domain.ErrTooLarge) {
		t.Errorf("số hỏng vượt trần cho lỗi %v, cần ErrTooLarge", err)
	}
}

// TestAdjustRejectsQuantityBeyondStorage: đường điều chỉnh theo chênh lệch cũng
// cộng dồn tới mức tràn được.
func TestAdjustRejectsQuantityBeyondStorage(t *testing.T) {
	q, err := domain.NewQuantities(domain.DefaultCap-5, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.AdjustAvailable(100, 0); !errors.Is(err, domain.ErrTooLarge) {
		t.Errorf("cộng dồn vượt trần cho lỗi %v, cần ErrTooLarge", err)
	}
	// Vẫn phải cho phép điều chỉnh bình thường.
	if _, err := q.AdjustAvailable(-5, 0); err != nil {
		t.Errorf("điều chỉnh hợp lệ bị từ chối: %v", err)
	}
}

// TestBusinessCapCannotExceedStorageCap là hàng rào quan trọng nhất khi
// trần này thành tham số sửa được lúc chạy.
//
// Có HAI trần, khác loại nhau:
//
//	MaxStorable    2.147.483.647  SỰ THẬT về cột INT của PostgreSQL
//	trần nghiệp vụ  10.000.000  LỰA CHỌN, sửa từ giao diện quản trị
//
// Trần nghiệp vụ đặt cao hơn trần lưu trữ thì con số ĐI QUA kiểm tra rồi
// hỏng ở câu lệnh ghi — đúng lỗi 500 mà việc thêm trần sinh ra để tránh.
//
// Sổ đăng ký cấu hình đã kẹp `Max` ở TranLuuTruSoLuong, nhưng domain KHÔNG
// được tin vào đó: nó phải đúng với bất kỳ giá trị nào bên gọi đưa vào, kể
// cả khi nối dây sai hoặc khi có bên gọi thứ hai không đi qua cấu hình.
func TestBusinessCapCannotExceedStorageCap(t *testing.T) {
	q, err := domain.NewQuantities(0, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Bên gọi đưa trần LỚN HƠN sức chứa — domain phải tự kẹp lại.
	over := domain.MaxStorable + 1000
	if _, err := q.Count(ptr(domain.MaxStorable+1), nil, over); !errors.Is(err, domain.ErrTooLarge) {
		t.Errorf("trần nghiệp vụ %d (> sức chứa) cho phép ghi %d: lỗi %v — "+
			"con số này sẽ đi qua kiểm tra rồi hỏng ở câu lệnh ghi",
			over, domain.MaxStorable+1, err)
	}

	// Đúng sức chứa vẫn phải nhận khi trần nghiệp vụ cho phép.
	if _, err := q.Count(ptr(domain.MaxStorable), nil, domain.MaxStorable); err != nil {
		t.Errorf("đúng sức chứa bị từ chối: %v", err)
	}
}

// TestInvalidCapFallsBackToDefault: 0 hoặc số âm nghĩa là "không nêu".
//
// Nối dây thiếu KHÔNG được biến thành trần bằng 0 — khi đó mọi lần kiểm kê
// đều bị từ chối, và cả tính năng kiểm kê ngừng hoạt động.
func TestInvalidCapFallsBackToDefault(t *testing.T) {
	q, err := domain.NewQuantities(0, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, businessCap := range []int{0, -1, -999} {
		if _, err := q.Count(ptr(1000), nil, businessCap); err != nil {
			t.Errorf("trần=%d làm kiểm kê 1000 đơn vị bị từ chối: %v — "+
				"nối dây thiếu không được làm ngừng cả tính năng", businessCap, err)
		}
		if _, err := q.Count(ptr(domain.DefaultCap+1), nil, businessCap); !errors.Is(err, domain.ErrTooLarge) {
			t.Errorf("trần=%d không rơi về mặc định: %v", businessCap, err)
		}
	}
}
