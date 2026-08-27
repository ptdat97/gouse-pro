package httpserver_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

func TestChuKyDungThiQua(t *testing.T) {
	than := []byte(`{"event_id":"evt_1","status":"DELIVERED"}`)
	const biMat = "bi-mat-cua-hang-van-chuyen"

	if err := httpserver.KiemChuKyHMAC(than, httpserver.KyHMAC(than, biMat), biMat); err != nil {
		t.Errorf("chữ ký đúng bị từ chối: %v", err)
	}
	// Nhiều nhà cung cấp gắn tiền tố thuật toán.
	if err := httpserver.KiemChuKyHMAC(than,
		"sha256="+httpserver.KyHMAC(than, biMat), biMat); err != nil {
		t.Errorf("chữ ký có tiền tố bị từ chối: %v", err)
	}
}

// TestSuaMotByteThanThiChuKySai — đây là lý do webhook cần chữ ký.
//
// Không có bước này, bất kỳ ai biết địa chỉ endpoint đều gửi được "đã giao
// hàng" giả, và hệ thống sẽ chuyển đơn sang DELIVERED, bắt đầu đếm hạn đổi
// trả, rồi chi tiền cho nhà bán.
func TestSuaMotByteThanThiChuKySai(t *testing.T) {
	goc := []byte(`{"event_id":"evt_1","status":"IN_TRANSIT"}`)
	const biMat = "bi-mat"
	chuKy := httpserver.KyHMAC(goc, biMat)

	gia := []byte(`{"event_id":"evt_1","status":"DELIVERED"}`)
	if err := httpserver.KiemChuKyHMAC(gia, chuKy, biMat); !errors.Is(err, httpserver.ErrChuKySai) {
		t.Error("thân bị sửa vẫn qua được kiểm chữ ký")
	}
}

func TestKhoaKhacThiChuKySai(t *testing.T) {
	than := []byte(`{"a":1}`)
	if err := httpserver.KiemChuKyHMAC(than,
		httpserver.KyHMAC(than, "khoa-a"), "khoa-b"); !errors.Is(err, httpserver.ErrChuKySai) {
		t.Error("khóa khác vẫn qua")
	}
}

// TestKhongCauHinhKhoaThiTuChoi — mặc định phải là ĐÓNG.
//
// Một hệ thống cho qua khi chưa cấu hình khóa sẽ chạy được ở môi trường
// phát triển, đi qua mọi bài test, rồi lên production với endpoint mở
// toang mà không ai nhận ra.
func TestKhongCauHinhKhoaThiTuChoi(t *testing.T) {
	than := []byte(`{"a":1}`)
	for _, biMat := range []string{"", "   "} {
		if err := httpserver.KiemChuKyHMAC(than, "abc", biMat); !errors.Is(err, httpserver.ErrChuKySai) {
			t.Errorf("khóa rỗng %q vẫn cho qua", biMat)
		}
	}
}

func TestChuKySaiDinhDangBiTuChoi(t *testing.T) {
	than := []byte(`{"a":1}`)
	for _, ck := range []string{"", "khong-phai-hex", "zz", "sha256="} {
		if err := httpserver.KiemChuKyHMAC(than, ck, "khoa"); !errors.Is(err, httpserver.ErrChuKySai) {
			t.Errorf("chữ ký %q được chấp nhận", ck)
		}
	}
}
