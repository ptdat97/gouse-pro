package domain_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
)

func mustQty(t *testing.T, available, reserved, committed, inTransit, damaged, returned int) domain.Quantities {
	t.Helper()
	q, err := domain.NewQuantities(available, reserved, committed, inTransit, damaged, returned)
	if err != nil {
		t.Fatalf("NewQuantities: %v", err)
	}
	return q
}

// Quy tắc 2 (mục 12): không trạng thái nào được âm.
func TestKhongChoSoLuongAm(t *testing.T) {
	cases := []struct {
		ten  string
		args [6]int
	}{
		{"available âm", [6]int{-1, 0, 0, 0, 0, 0}},
		{"reserved âm", [6]int{0, -1, 0, 0, 0, 0}},
		{"committed âm", [6]int{0, 0, -1, 0, 0, 0}},
		{"in_transit âm", [6]int{0, 0, 0, -1, 0, 0}},
		{"damaged âm", [6]int{0, 0, 0, 0, -1, 0}},
		{"returned âm", [6]int{0, 0, 0, 0, 0, -1}},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			a := tc.args
			_, err := domain.NewQuantities(a[0], a[1], a[2], a[3], a[4], a[5])
			if !errors.Is(err, domain.ErrNegativeQuantity) {
				t.Errorf("lỗi = %v, mong ErrNegativeQuantity", err)
			}
		})
	}
}

// Quy tắc 1: tổng các trạng thái = số lượng vật lý.
//
// Mọi phép CHUYỂN trạng thái phải bảo toàn tổng. Chỉ Receive và Ship được
// làm đổi tổng — vì hàng thật sự vào hoặc ra khỏi kho.
func TestChuyenTrangThaiBaoToanTong(t *testing.T) {
	q := mustQty(t, 100, 20, 30, 10, 5, 8)
	tongBanDau := q.Total()

	chuyen := []struct {
		ten string
		fn  func(domain.Quantities) (domain.Quantities, error)
	}{
		{"Reserve", func(x domain.Quantities) (domain.Quantities, error) { return x.Reserve(10) }},
		{"Release", func(x domain.Quantities) (domain.Quantities, error) { return x.Release(5) }},
		{"Commit", func(x domain.Quantities) (domain.Quantities, error) { return x.Commit(7) }},
		{"Uncommit", func(x domain.Quantities) (domain.Quantities, error) { return x.Uncommit(3) }},
		{"MarkDamaged", func(x domain.Quantities) (domain.Quantities, error) { return x.MarkDamaged(2) }},
		{"SendInTransit", func(x domain.Quantities) (domain.Quantities, error) { return x.SendInTransit(4) }},
		{"ArriveFromTransit", func(x domain.Quantities) (domain.Quantities, error) { return x.ArriveFromTransit(6) }},
		{"InspectionPassed", func(x domain.Quantities) (domain.Quantities, error) { return x.InspectionPassed(3) }},
		{"InspectionFailed", func(x domain.Quantities) (domain.Quantities, error) { return x.InspectionFailed(2) }},
	}

	for _, c := range chuyen {
		got, err := c.fn(q)
		if err != nil {
			t.Fatalf("%s: %v", c.ten, err)
		}
		if got.Total() != tongBanDau {
			t.Errorf("%s làm đổi tổng: %d → %d", c.ten, tongBanDau, got.Total())
		}
		q = got
	}
}

// Chỉ Receive và Ship được làm đổi tổng số lượng vật lý.
func TestChiNhapVaXuatLamDoiTong(t *testing.T) {
	q := mustQty(t, 50, 0, 20, 0, 0, 0)

	nhap, err := q.Receive(30)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if nhap.Total() != q.Total()+30 {
		t.Errorf("Receive: tổng = %d, mong %d", nhap.Total(), q.Total()+30)
	}

	xuat, err := q.Ship(15)
	if err != nil {
		t.Fatalf("Ship: %v", err)
	}
	// Hàng đã xuất RỜI KHỎI tồn kho.
	if xuat.Total() != q.Total()-15 {
		t.Errorf("Ship: tổng = %d, mong %d", xuat.Total(), q.Total()-15)
	}

	// Hàng hoàn về làm TĂNG tổng — nó quay lại kho.
	hoan, err := q.ReceiveReturn(5)
	if err != nil {
		t.Fatalf("ReceiveReturn: %v", err)
	}
	if hoan.Total() != q.Total()+5 {
		t.Errorf("ReceiveReturn: tổng = %d, mong %d", hoan.Total(), q.Total()+5)
	}
}

// Phép chuyển THẤT BẠI không được để lại trạng thái dở dang.
//
// Đây là lý do Quantities là value object bất biến: nếu thao tác trực tiếp
// trên bản gốc, một lỗi giữa chừng sẽ làm hỏng bất biến vĩnh viễn.
func TestChuyenThatBaiKhongDeLaiTrangThaiDoDang(t *testing.T) {
	q := mustQty(t, 5, 0, 0, 0, 0, 0)

	got, err := q.Reserve(10) // chỉ có 5
	if !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatalf("lỗi = %v, mong ErrInsufficientStock", err)
	}
	// Giá trị trả về phải là bản GỐC nguyên vẹn.
	if got.Available() != 5 || got.Reserved() != 0 {
		t.Errorf("sau khi thất bại: available=%d reserved=%d, mong 5 và 0",
			got.Available(), got.Reserved())
	}
	if q.Available() != 5 {
		t.Errorf("bản gốc bị sửa: available=%d", q.Available())
	}
}

func TestKhongChuyenDuocQuaSoLuongDangCo(t *testing.T) {
	q := mustQty(t, 10, 3, 2, 1, 0, 4)

	cases := []struct {
		ten string
		fn  func() (domain.Quantities, error)
	}{
		{"Reserve quá available", func() (domain.Quantities, error) { return q.Reserve(11) }},
		{"Release quá reserved", func() (domain.Quantities, error) { return q.Release(4) }},
		{"Commit quá reserved", func() (domain.Quantities, error) { return q.Commit(4) }},
		{"Uncommit quá committed", func() (domain.Quantities, error) { return q.Uncommit(3) }},
		{"Ship quá committed", func() (domain.Quantities, error) { return q.Ship(3) }},
		{"InspectionPassed quá returned", func() (domain.Quantities, error) { return q.InspectionPassed(5) }},
		{"ArriveFromTransit quá in_transit", func() (domain.Quantities, error) { return q.ArriveFromTransit(2) }},
	}
	for _, c := range cases {
		t.Run(c.ten, func(t *testing.T) {
			if _, err := c.fn(); !errors.Is(err, domain.ErrInsufficientStock) {
				t.Errorf("lỗi = %v, mong ErrInsufficientStock", err)
			}
		})
	}
}

func TestSoLuongChuyenPhaiDuong(t *testing.T) {
	q := mustQty(t, 10, 10, 10, 10, 10, 10)
	for _, qty := range []int{0, -1, -100} {
		if _, err := q.Reserve(qty); err == nil {
			t.Errorf("Reserve(%d) phải lỗi", qty)
		}
		if _, err := q.Receive(qty); err == nil {
			t.Errorf("Receive(%d) phải lỗi", qty)
		}
		if _, err := q.Ship(qty); err == nil {
			t.Errorf("Ship(%d) phải lỗi", qty)
		}
	}
}

// QUY TẮC 3 (mục 12): hàng hoàn KHÔNG BAO GIỜ tự động vào Available.
//
// Vi phạm dẫn tới bán lại hàng hỏng cho khách khác — thiệt hại uy tín lớn
// hơn nhiều so với giá trị món hàng.
func TestHangHoanPhaiQuaKiemDinh(t *testing.T) {
	q := mustQty(t, 0, 0, 0, 0, 0, 0)

	sauHoan, err := q.ReceiveReturn(10)
	if err != nil {
		t.Fatalf("ReceiveReturn: %v", err)
	}
	// Hàng hoàn KHÔNG được cộng thẳng vào available.
	if sauHoan.Available() != 0 {
		t.Errorf("available = %d sau khi nhận hàng hoàn, mong 0 — hàng hoàn phải qua kiểm định",
			sauHoan.Available())
	}
	if sauHoan.Returned() != 10 {
		t.Errorf("returned = %d, mong 10", sauHoan.Returned())
	}

	// Đạt kiểm định → mới vào available.
	dat, err := sauHoan.InspectionPassed(6)
	if err != nil {
		t.Fatalf("InspectionPassed: %v", err)
	}
	if dat.Available() != 6 || dat.Returned() != 4 {
		t.Errorf("sau kiểm định đạt: available=%d returned=%d, mong 6 và 4",
			dat.Available(), dat.Returned())
	}

	// Không đạt → vào damaged, KHÔNG vào available.
	khongDat, err := dat.InspectionFailed(4)
	if err != nil {
		t.Fatalf("InspectionFailed: %v", err)
	}
	if khongDat.Damaged() != 4 {
		t.Errorf("damaged = %d, mong 4", khongDat.Damaged())
	}
	if khongDat.Available() != 6 {
		t.Errorf("available = %d, hàng hỏng đã lọt vào available", khongDat.Available())
	}
}

func TestHetHangVaSapHetHang(t *testing.T) {
	// Chỉ xét available: hàng đang giữ cho checkout khác không bán được
	// cho khách mới.
	q := mustQty(t, 0, 50, 30, 20, 10, 5)
	if !q.IsDepleted() {
		t.Error("available = 0 phải là hết hàng, dù còn hàng ở trạng thái khác")
	}

	q2 := mustQty(t, 1, 0, 0, 0, 0, 0)
	if q2.IsDepleted() {
		t.Error("available = 1 không phải hết hàng")
	}
	if !q2.IsLowStock(10) {
		t.Error("available = 1 phải là sắp hết hàng với ngưỡng 10")
	}
	if q2.IsLowStock(0) {
		t.Error("available = 1 không phải sắp hết hàng với ngưỡng 0")
	}
}

// Quy tắc 7: điều chỉnh thủ công (kiểm kê) có thể âm, nhưng không được
// làm số lượng thành âm.
func TestDieuChinhThuCong(t *testing.T) {
	q := mustQty(t, 10, 0, 0, 0, 0, 0)

	tang, err := q.AdjustAvailable(5, 0)
	if err != nil {
		t.Fatalf("AdjustAvailable(+5): %v", err)
	}
	if tang.Available() != 15 {
		t.Errorf("available = %d, mong 15", tang.Available())
	}

	giam, err := q.AdjustAvailable(-4, 0)
	if err != nil {
		t.Fatalf("AdjustAvailable(-4): %v", err)
	}
	if giam.Available() != 6 {
		t.Errorf("available = %d, mong 6", giam.Available())
	}

	// Không được làm thành âm.
	if _, err := q.AdjustAvailable(-11, 0); !errors.Is(err, domain.ErrNegativeQuantity) {
		t.Errorf("lỗi = %v, mong ErrNegativeQuantity", err)
	}
	// Điều chỉnh 0 là vô nghĩa.
	if _, err := q.AdjustAvailable(0, 0); err == nil {
		t.Error("điều chỉnh 0 phải báo lỗi")
	}
}

// Chạy NHIỀU phép chuyển ngẫu nhiên, bất biến phải giữ nguyên sau mỗi bước.
//
// Test theo tính chất bắt được tổ hợp mà test theo kịch bản bỏ sót: không
// ai nghĩ ra được đủ mọi thứ tự thao tác có thể xảy ra trong thực tế.
func TestBatBienGiuVungQuaChuoiThaoTacNgauNhien(t *testing.T) {
	rng := rand.New(rand.NewSource(20260813))

	for lan := 0; lan < 200; lan++ {
		q := mustQty(t, 100, 0, 0, 0, 0, 0)
		tongVatLy := q.Total()

		for buoc := 0; buoc < 40; buoc++ {
			qty := 1 + rng.Intn(10)

			var (
				got     domain.Quantities
				err     error
				doiTong int // tổng thay đổi bao nhiêu nếu thành công
			)
			switch rng.Intn(10) {
			case 0:
				got, err = q.Reserve(qty)
			case 1:
				got, err = q.Release(qty)
			case 2:
				got, err = q.Commit(qty)
			case 3:
				got, err = q.Uncommit(qty)
			case 4:
				got, err = q.Ship(qty)
				doiTong = -qty
			case 5:
				got, err = q.Receive(qty)
				doiTong = qty
			case 6:
				got, err = q.ReceiveReturn(qty)
				doiTong = qty
			case 7:
				got, err = q.InspectionPassed(qty)
			case 8:
				got, err = q.InspectionFailed(qty)
			case 9:
				got, err = q.MarkDamaged(qty)
			}

			if err != nil {
				// Thất bại: trạng thái phải KHÔNG đổi.
				if got.Total() != q.Total() {
					t.Fatalf("lần %d bước %d: thao tác thất bại nhưng đổi tổng", lan, buoc)
				}
				continue
			}

			tongVatLy += doiTong
			if got.Total() != tongVatLy {
				t.Fatalf("lần %d bước %d: tổng = %d, mong %d", lan, buoc, got.Total(), tongVatLy)
			}
			// Không thành phần nào được âm.
			for ten, v := range map[string]int{
				"available": got.Available(), "reserved": got.Reserved(),
				"committed": got.Committed(), "in_transit": got.InTransit(),
				"damaged": got.Damaged(), "returned": got.Returned(),
			} {
				if v < 0 {
					t.Fatalf("lần %d bước %d: %s = %d (âm)", lan, buoc, ten, v)
				}
			}
			q = got
		}
	}
}
