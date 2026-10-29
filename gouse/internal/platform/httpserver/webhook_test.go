package httpserver_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

func TestACorrectSignaturePasses(t *testing.T) {
	than := []byte(`{"event_id":"evt_1","status":"DELIVERED"}`)
	const secret = "bi-mat-cua-hang-van-chuyen"

	if err := httpserver.VerifyHMAC(than, httpserver.SignHMAC(than, secret), secret); err != nil {
		t.Errorf("chữ ký đúng bị từ chối: %v", err)
	}
	// Nhiều nhà cung cấp gắn tiền tố thuật toán.
	if err := httpserver.VerifyHMAC(than,
		"sha256="+httpserver.SignHMAC(than, secret), secret); err != nil {
		t.Errorf("chữ ký có tiền tố bị từ chối: %v", err)
	}
}

// TestFlippingOneBodyByteBreaksTheSignature — đây là lý do webhook cần chữ ký.
//
// Không có bước này, bất kỳ ai biết địa chỉ endpoint đều gửi được "đã giao
// hàng" giả, và hệ thống sẽ chuyển đơn sang DELIVERED, bắt đầu đếm hạn đổi
// trả, rồi chi tiền cho nhà bán.
func TestFlippingOneBodyByteBreaksTheSignature(t *testing.T) {
	origin := []byte(`{"event_id":"evt_1","status":"IN_TRANSIT"}`)
	const secret = "bi-mat"
	signature := httpserver.SignHMAC(origin, secret)

	body := []byte(`{"event_id":"evt_1","status":"DELIVERED"}`)
	if err := httpserver.VerifyHMAC(body, signature, secret); !errors.Is(err, httpserver.ErrBadSignature) {
		t.Error("thân bị sửa vẫn qua được kiểm chữ ký")
	}
}

func TestADifferentKeyBreaksTheSignature(t *testing.T) {
	than := []byte(`{"a":1}`)
	if err := httpserver.VerifyHMAC(than,
		httpserver.SignHMAC(than, "khoa-a"), "khoa-b"); !errors.Is(err, httpserver.ErrBadSignature) {
		t.Error("khóa khác vẫn qua")
	}
}

// TestNoConfiguredKeyMeansReject — mặc định phải là ĐÓNG.
//
// Một hệ thống cho qua khi chưa cấu hình khóa sẽ chạy được ở môi trường
// phát triển, đi qua mọi bài test, rồi lên production với endpoint mở
// toang mà không ai nhận ra.
func TestNoConfiguredKeyMeansReject(t *testing.T) {
	than := []byte(`{"a":1}`)
	for _, secret := range []string{"", "   "} {
		if err := httpserver.VerifyHMAC(than, "abc", secret); !errors.Is(err, httpserver.ErrBadSignature) {
			t.Errorf("khóa rỗng %q vẫn cho qua", secret)
		}
	}
}

func TestAMalformedSignatureIsRejected(t *testing.T) {
	than := []byte(`{"a":1}`)
	for _, ck := range []string{"", "khong-phai-hex", "zz", "sha256="} {
		if err := httpserver.VerifyHMAC(than, ck, "khoa"); !errors.Is(err, httpserver.ErrBadSignature) {
			t.Errorf("chữ ký %q được chấp nhận", ck)
		}
	}
}
