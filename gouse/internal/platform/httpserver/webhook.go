package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// ErrChuKySai: chữ ký không khớp, hoặc thiếu, hoặc sai định dạng.
//
// KHÔNG phân biệt ba trường hợp với bên gọi. Nói rõ "thiếu header" hay
// "sai khóa" là cho kẻ giả mạo biết họ đang sai ở bước nào.
var ErrChuKySai = errors.New("httpserver: chữ ký webhook không hợp lệ")

// KiemChuKyHMAC xác minh chữ ký HMAC-SHA256 của một webhook.
//
// # Vì sao dùng hmac.Equal chứ không phải ==
//
// So sánh chuỗi thường dừng ở byte đầu tiên khác nhau, nên thời gian so
// sánh rò rỉ số byte đúng ở đầu. Kẻ tấn công đo thời gian có thể dò từng
// byte chữ ký — 32 byte × 256 lần thử thay vì 256³² lần.
//
// hmac.Equal chạy hằng thời gian.
//
// # Vì sao so trên BYTE THÔ của thân request
//
// Không phải trên JSON đã parse rồi serialize lại. Thứ tự khóa, khoảng
// trắng và cách escape đều đổi khi đi qua vòng parse–serialize, và chữ ký
// tính trên chuỗi khác thì không bao giờ khớp.
func KiemChuKyHMAC(thanRaw []byte, chuKy, biMat string) error {
	if strings.TrimSpace(biMat) == "" {
		// Không cấu hình khóa thì TỪ CHỐI, không phải cho qua. Mặc định
		// mở là cách một webhook giả đi thẳng vào hệ thống.
		return ErrChuKySai
	}

	chuKy = strings.TrimSpace(chuKy)
	// Nhiều nhà cung cấp gắn tiền tố thuật toán: "sha256=abc123…".
	if _, sau, co := strings.Cut(chuKy, "="); co {
		chuKy = sau
	}

	nhan, err := hex.DecodeString(chuKy)
	if err != nil {
		return ErrChuKySai
	}

	mac := hmac.New(sha256.New, []byte(biMat))
	mac.Write(thanRaw)
	if !hmac.Equal(nhan, mac.Sum(nil)) {
		return ErrChuKySai
	}
	return nil
}

// KyHMAC tính chữ ký cho một thân request.
//
// Dùng trong test và trong công cụ mô phỏng nhà cung cấp. Production KHÔNG
// gọi hàm này — chữ ký do bên gửi tính.
func KyHMAC(thanRaw []byte, biMat string) string {
	mac := hmac.New(sha256.New, []byte(biMat))
	mac.Write(thanRaw)
	return hex.EncodeToString(mac.Sum(nil))
}
