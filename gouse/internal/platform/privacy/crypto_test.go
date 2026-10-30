package privacy_test

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/privacy"
)

func testKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("sinh khóa: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestEncryptThenDecryptRoundTrips(t *testing.T) {
	bm, err := privacy.NewCipher(testKey(t))
	if err != nil {
		t.Fatalf("dựng bộ mã hóa: %v", err)
	}

	for _, original := range []string{
		"0123456789",
		"Nguyễn Văn A", // dấu tiếng Việt
		"",             // rỗng
		strings.Repeat("9", 64),
	} {
		sealed, err := bm.Encrypt(original)
		if err != nil {
			t.Fatalf("mã hóa %q: %v", original, err)
		}
		if strings.Contains(sealed, original) && original != "" {
			t.Errorf("bản mã CHỨA nguyên văn %q — không mã hóa gì cả", original)
		}
		again, err := bm.Decrypt(sealed)
		if err != nil {
			t.Fatalf("giải mã %q: %v", original, err)
		}
		if again != original {
			t.Errorf("giải mã ra %q, gốc %q", again, original)
		}
	}
}

// TestEncryptingTwiceGivesTwoDifferentCiphertexts.
//
// Bản mã giống nhau sẽ tiết lộ rằng hai nhà bán dùng chung một số tài
// khoản — một rò rỉ THẬT mà không cần giải mã gì cả. Nonce ngẫu nhiên mỗi
// lần là thứ chặn điều đó.
func TestEncryptingTwiceGivesTwoDifferentCiphertexts(t *testing.T) {
	bm, _ := privacy.NewCipher(testKey(t))
	a, _ := bm.Encrypt("0123456789")
	b, _ := bm.Encrypt("0123456789")
	if a == b {
		t.Error("hai lần mã hóa cùng chuỗi cho CÙNG bản mã — " +
			"nonce không ngẫu nhiên, và số tài khoản trùng nhau lộ ra ngay")
	}
}

// TestFlippingOneByteFailsDecryption — vì sao chọn GCM chứ không phải CBC.
//
// Với số tài khoản ngân hàng, khác biệt giữa "báo lỗi" và "trả về rác
// trông như số thật" là khác biệt giữa dừng lại và chuyển tiền cho người lạ.
func TestFlippingOneByteFailsDecryption(t *testing.T) {
	bm, _ := privacy.NewCipher(testKey(t))
	sealed, _ := bm.Encrypt("0123456789")

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, "v1:"))
	if err != nil {
		t.Fatalf("giải base64: %v", err)
	}
	raw[len(raw)-1] ^= 0x01 // lật đúng MỘT bit
	corrupted := "v1:" + base64.StdEncoding.EncodeToString(raw)

	if _, err := bm.Decrypt(corrupted); !errors.Is(err, privacy.ErrBadCiphertext) {
		t.Errorf("sửa một bit vẫn giải mã được (lỗi: %v) — mất bảo đảm toàn vẹn", err)
	}
}

func TestADifferentKeyCannotDecrypt(t *testing.T) {
	a, _ := privacy.NewCipher(testKey(t))
	b, _ := privacy.NewCipher(testKey(t))

	sealed, _ := a.Encrypt("0123456789")
	if _, err := b.Decrypt(sealed); !errors.Is(err, privacy.ErrBadCiphertext) {
		t.Error("khóa khác vẫn giải mã được")
	}
}

func TestAWrongSizedKeyIsRejected(t *testing.T) {
	for _, k := range []string{
		"",
		"khong-phai-base64!!!",
		base64.StdEncoding.EncodeToString(make([]byte, 16)), // 16 byte
		base64.StdEncoding.EncodeToString(make([]byte, 31)),
	} {
		if _, err := privacy.NewCipher(k); err == nil {
			t.Errorf("khóa %q được chấp nhận — cần từ chối", k)
		}
	}
}

func TestLast4(t *testing.T) {
	cases := map[string]string{
		"0123456789": "6789",
		"123":        "123",
		"":           "",
		"  1234567 ": "4567",
	}
	for input, expect := range cases {
		if got := privacy.Last4(input); got != expect {
			t.Errorf("Last4(%q) = %q, mong %q", input, got, expect)
		}
	}
}
