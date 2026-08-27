package privacy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Nhãn phiên bản đứng đầu mỗi bản mã.
//
// # Vì sao cần ngay từ đầu, khi mới có MỘT phiên bản
//
// Xoay khóa là việc CHẮC CHẮN sẽ tới. Không có nhãn thì lúc đó không phân
// biệt được bản mã cũ với bản mã mới, và cách duy nhất còn lại là giải mã
// thử — tức là đoán. Thêm bốn ký tự bây giờ rẻ hơn nhiều lần một đợt di
// trú mù sau này.
const nhanV1 = "v1:"

var (
	// ErrKhoaKhongHopLe: khóa không đúng 32 byte sau khi giải mã base64.
	ErrKhoaKhongHopLe = errors.New("privacy: khóa mã hóa phải đúng 32 byte")

	// ErrBanMaHong: bản mã sai định dạng, sai phiên bản, hoặc đã bị sửa.
	//
	// KHÔNG phân biệt ba trường hợp đó với bên gọi: nói rõ "sai khóa" hay
	// "đã bị sửa" là cho kẻ tấn công một kênh phân biệt.
	ErrBanMaHong = errors.New("privacy: không giải mã được")
)

// BoMaHoa mã hóa và giải mã trường dữ liệu nhạy cảm.
//
// # Phạm vi
//
// Dùng cho dữ liệu PHẢI đọc lại được nguyên văn — số tài khoản ngân hàng
// để chuyển tiền, số đo cơ thể để gợi ý size. KHÔNG dùng cho mật khẩu
// (băm một chiều) hay địa chỉ IP (băm, xem HashIP).
//
// # AES-256-GCM
//
// GCM cho cả bí mật lẫn TOÀN VẸN: bản mã bị sửa một bit là giải mã thất
// bại, thay vì trả về rác trông như dữ liệu thật. Với số tài khoản ngân
// hàng, khác biệt đó là giữa "báo lỗi" và "chuyển tiền cho người lạ".
//
// Nonce sinh ngẫu nhiên cho MỖI lần mã hóa và đi kèm bản mã. Dùng lại
// nonce với cùng khóa phá vỡ toàn bộ bảo đảm của GCM.
type BoMaHoa struct {
	gcm cipher.AEAD
}

// NewBoMaHoa dựng bộ mã hóa từ khóa base64 32 byte.
//
// Sinh khóa: openssl rand -base64 32
func NewBoMaHoa(khoaBase64 string) (*BoMaHoa, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(khoaBase64))
	if err != nil {
		return nil, fmt.Errorf("%w: không giải được base64", ErrKhoaKhongHopLe)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("%w: nhận %d byte", ErrKhoaKhongHopLe, len(raw))
	}

	khoi, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("privacy: dựng AES: %w", err)
	}
	gcm, err := cipher.NewGCM(khoi)
	if err != nil {
		return nil, fmt.Errorf("privacy: dựng GCM: %w", err)
	}
	return &BoMaHoa{gcm: gcm}, nil
}

// MaHoa trả về "v1:" + base64(nonce ‖ bản mã).
//
// Hai lần mã hóa CÙNG một chuỗi cho hai kết quả KHÁC nhau, vì nonce ngẫu
// nhiên. Đó là chủ ý: bản mã giống nhau sẽ tiết lộ rằng hai nhà bán dùng
// chung một số tài khoản, mà không cần giải mã gì cả.
func (b *BoMaHoa) MaHoa(roNghia string) (string, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("privacy: sinh nonce: %w", err)
	}
	kin := b.gcm.Seal(nonce, nonce, []byte(roNghia), nil)
	return nhanV1 + base64.StdEncoding.EncodeToString(kin), nil
}

// GiaiMa đọc lại chuỗi đã mã hóa bằng MaHoa.
func (b *BoMaHoa) GiaiMa(banMa string) (string, error) {
	sau, co := strings.CutPrefix(banMa, nhanV1)
	if !co {
		return "", ErrBanMaHong
	}
	kin, err := base64.StdEncoding.DecodeString(sau)
	if err != nil {
		return "", ErrBanMaHong
	}
	n := b.gcm.NonceSize()
	if len(kin) < n {
		return "", ErrBanMaHong
	}
	ro, err := b.gcm.Open(nil, kin[:n], kin[n:], nil)
	if err != nil {
		return "", ErrBanMaHong
	}
	return string(ro), nil
}

// BonSoCuoi trả bốn ký tự cuối của một số tài khoản, để HIỂN THỊ.
//
// Lưu riêng ở dạng rõ là chủ ý, theo đúng quy tắc đã áp cho thẻ ở
// docs/09-operations/security.md mục 7: bốn số cuối hiển thị được, phần
// còn lại thì không. Không có nó thì mọi màn hình muốn hiện "…4567" đều
// phải giải mã, và đường giải mã càng nhiều nơi gọi thì càng khó canh.
func BonSoCuoi(soTaiKhoan string) string {
	s := strings.TrimSpace(soTaiKhoan)
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}
