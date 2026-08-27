package privacy_test

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/privacy"
)

func khoaThu(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("sinh khóa: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestMaHoaRoiGiaiMaTraLaiNguyenVan(t *testing.T) {
	bm, err := privacy.NewBoMaHoa(khoaThu(t))
	if err != nil {
		t.Fatalf("dựng bộ mã hóa: %v", err)
	}

	for _, goc := range []string{
		"0123456789",
		"Nguyễn Văn A", // dấu tiếng Việt
		"",             // rỗng
		strings.Repeat("9", 64),
	} {
		kin, err := bm.MaHoa(goc)
		if err != nil {
			t.Fatalf("mã hóa %q: %v", goc, err)
		}
		if strings.Contains(kin, goc) && goc != "" {
			t.Errorf("bản mã CHỨA nguyên văn %q — không mã hóa gì cả", goc)
		}
		lai, err := bm.GiaiMa(kin)
		if err != nil {
			t.Fatalf("giải mã %q: %v", goc, err)
		}
		if lai != goc {
			t.Errorf("giải mã ra %q, gốc %q", lai, goc)
		}
	}
}

// TestHaiLanMaHoaCungChuoiChoHaiBanMaKhacNhau.
//
// Bản mã giống nhau sẽ tiết lộ rằng hai nhà bán dùng chung một số tài
// khoản — một rò rỉ THẬT mà không cần giải mã gì cả. Nonce ngẫu nhiên mỗi
// lần là thứ chặn điều đó.
func TestHaiLanMaHoaCungChuoiChoHaiBanMaKhacNhau(t *testing.T) {
	bm, _ := privacy.NewBoMaHoa(khoaThu(t))
	a, _ := bm.MaHoa("0123456789")
	b, _ := bm.MaHoa("0123456789")
	if a == b {
		t.Error("hai lần mã hóa cùng chuỗi cho CÙNG bản mã — " +
			"nonce không ngẫu nhiên, và số tài khoản trùng nhau lộ ra ngay")
	}
}

// TestSuaMotByteThiGiaiMaThatBai — vì sao chọn GCM chứ không phải CBC.
//
// Với số tài khoản ngân hàng, khác biệt giữa "báo lỗi" và "trả về rác
// trông như số thật" là khác biệt giữa dừng lại và chuyển tiền cho người lạ.
func TestSuaMotByteThiGiaiMaThatBai(t *testing.T) {
	bm, _ := privacy.NewBoMaHoa(khoaThu(t))
	kin, _ := bm.MaHoa("0123456789")

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(kin, "v1:"))
	if err != nil {
		t.Fatalf("giải base64: %v", err)
	}
	raw[len(raw)-1] ^= 0x01 // lật đúng MỘT bit
	hong := "v1:" + base64.StdEncoding.EncodeToString(raw)

	if _, err := bm.GiaiMa(hong); !errors.Is(err, privacy.ErrBanMaHong) {
		t.Errorf("sửa một bit vẫn giải mã được (lỗi: %v) — mất bảo đảm toàn vẹn", err)
	}
}

func TestKhoaKhacThiKhongDocDuoc(t *testing.T) {
	a, _ := privacy.NewBoMaHoa(khoaThu(t))
	b, _ := privacy.NewBoMaHoa(khoaThu(t))

	kin, _ := a.MaHoa("0123456789")
	if _, err := b.GiaiMa(kin); !errors.Is(err, privacy.ErrBanMaHong) {
		t.Error("khóa khác vẫn giải mã được")
	}
}

func TestKhoaSaiKichThuocBiTuChoi(t *testing.T) {
	for _, k := range []string{
		"",
		"khong-phai-base64!!!",
		base64.StdEncoding.EncodeToString(make([]byte, 16)), // 16 byte
		base64.StdEncoding.EncodeToString(make([]byte, 31)),
	} {
		if _, err := privacy.NewBoMaHoa(k); err == nil {
			t.Errorf("khóa %q được chấp nhận — cần từ chối", k)
		}
	}
}

func TestBonSoCuoi(t *testing.T) {
	cac := map[string]string{
		"0123456789": "6789",
		"123":        "123",
		"":           "",
		"  1234567 ": "4567",
	}
	for vao, mong := range cac {
		if got := privacy.BonSoCuoi(vao); got != mong {
			t.Errorf("BonSoCuoi(%q) = %q, mong %q", vao, got, mong)
		}
	}
}
