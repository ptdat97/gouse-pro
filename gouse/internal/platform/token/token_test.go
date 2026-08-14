package token_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/fashion-commerce/platform/internal/platform/token"
)

const testSecret = "khoa-bi-mat-du-dai-cho-hs256-toi-thieu-32-ky-tu"

func newIssuer(t *testing.T) *token.Issuer {
	t.Helper()
	iss, err := token.NewIssuer(token.Config{Secret: testSecret})
	if err != nil {
		t.Fatalf("tạo issuer thất bại: %v", err)
	}
	return iss
}

// TestShortSecretRejectedAtStartup: khóa yếu phải bị chặn lúc KHỞI ĐỘNG.
//
// Một hệ thống chạy được với khóa ngắn là hệ thống không ai biết là đang
// yếu — và chữ ký HMAC khóa ngắn dò được bằng vét cạn, sau đó bất kỳ ai
// cũng tự phát hành được token vai trò ADMIN.
func TestShortSecretRejectedAtStartup(t *testing.T) {
	_, err := token.NewIssuer(token.Config{Secret: strings.Repeat("a", 31)})
	if err == nil {
		t.Fatal("khóa ngắn hơn 32 ký tự PHẢI bị từ chối lúc khởi tạo")
	}

	if _, err := token.NewIssuer(token.Config{Secret: strings.Repeat("a", 32)}); err != nil {
		t.Errorf("khóa đủ 32 ký tự phải được chấp nhận, nhận lỗi: %v", err)
	}
}

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	iss := newIssuer(t)
	want := token.Claims{
		UserID:    "usr_01J9XABC123DEF456GHJKMNPQR",
		Roles:     []string{"OPS_FINANCE", "OPS_SUPPORT"},
		Scope:     "ALL",
		SessionID: "ses_01J9XABC123DEF456GHJKMNPQR",
	}

	signed, err := iss.Issue(want, time.Now())
	if err != nil {
		t.Fatalf("phát hành thất bại: %v", err)
	}

	got, err := iss.Verify(signed)
	if err != nil {
		t.Fatalf("xác minh thất bại: %v", err)
	}

	if got.UserID != want.UserID {
		t.Errorf("UserID: mong %q, nhận %q", want.UserID, got.UserID)
	}
	if got.Scope != want.Scope {
		t.Errorf("Scope: mong %q, nhận %q", want.Scope, got.Scope)
	}
	if got.SessionID != want.SessionID {
		t.Errorf("SessionID: mong %q, nhận %q", want.SessionID, got.SessionID)
	}
	if len(got.Roles) != len(want.Roles) {
		t.Fatalf("số vai trò: mong %d, nhận %d", len(want.Roles), len(got.Roles))
	}
	for i := range want.Roles {
		if got.Roles[i] != want.Roles[i] {
			t.Errorf("Roles[%d]: mong %q, nhận %q", i, want.Roles[i], got.Roles[i])
		}
	}
}

// TestAlgNoneRejected là bẫy bảo mật kinh điển của JWT: kẻ tấn công sửa
// header thành {"alg":"none"}, bỏ chữ ký, và bộ xác minh nào TIN vào header
// sẽ chấp nhận token tự chế với vai trò bất kỳ.
//
// Lưu ý khi đọc test này: golang-jwt/v5 tự chặn "none" ở tầng thư viện
// ("'none' signature type is not allowed"), nên test này KHÔNG kiểm chứng
// code của chúng ta — nó chốt lại hành vi mà chúng ta đang dựa vào, để việc
// nâng cấp thư viện làm mất nó sẽ bị phát hiện.
//
// Phần bảo vệ THUỘC VỀ chúng ta nằm ở TestOtherSigningAlgorithmRejected.
func TestAlgNoneRejected(t *testing.T) {
	iss := newIssuer(t)

	// Tự dựng token alg=none với vai trò ADMIN.
	header := base64.RawURLEncoding.EncodeToString(
		[]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(
		`{"sub":"usr_ke_tan_cong","roles":["ADMIN"],"scope":"ALL",` +
			`"sid":"ses_x","exp":99999999999}`))
	forged := header + "." + payload + "."

	if _, err := iss.Verify(forged); err == nil {
		t.Fatal("token alg=none PHẢI bị từ chối — đây là cách tự cấp quyền ADMIN")
	}
}

// TestOtherSigningAlgorithmRejected kiểm chứng việc KHÓA CỨNG một thuật
// toán — phần bảo vệ do code của chúng ta thực hiện, không phải thư viện.
//
// Kẻ tấn công biết khóa bí mật dùng cho HS256 có thể ký lại token bằng
// HS512 với cùng khóa đó. Bộ xác minh nào chấp nhận "bất kỳ thuật toán HMAC
// nào" sẽ nhận token đó là hợp lệ. Ta chỉ chấp nhận ĐÚNG HS256.
func TestOtherSigningAlgorithmRejected(t *testing.T) {
	iss := newIssuer(t)

	// Ký bằng HS512, cùng khóa bí mật.
	forged := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"sub":   "usr_ke_tan_cong",
		"roles": []string{"ADMIN"},
		"sid":   "ses_x",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	signed, err := forged.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("ký token HS512 thất bại: %v", err)
	}

	if _, err := iss.Verify(signed); err == nil {
		t.Fatal("token ký bằng thuật toán KHÁC phải bị từ chối, kể cả khi " +
			"dùng đúng khóa bí mật — chỉ HS256 được chấp nhận")
	}
}

// TestWrongSecretRejected: token ký bằng khóa khác phải bị từ chối.
func TestWrongSecretRejected(t *testing.T) {
	iss := newIssuer(t)
	other, err := token.NewIssuer(token.Config{
		Secret: "mot-khoa-bi-mat-hoan-toan-khac-nhung-du-dai-32",
	})
	if err != nil {
		t.Fatalf("tạo issuer thứ hai thất bại: %v", err)
	}

	signed, err := other.Issue(token.Claims{UserID: "usr_1"}, time.Now())
	if err != nil {
		t.Fatalf("phát hành thất bại: %v", err)
	}

	if _, err := iss.Verify(signed); err == nil {
		t.Fatal("token ký bằng khóa khác PHẢI bị từ chối")
	}
}

// TestTamperedPayloadRejected: sửa nội dung làm chữ ký không còn khớp.
//
// Đây là bất biến khiến JWT dùng được: người dùng cầm token trong tay
// nhưng không tự nâng vai trò của mình lên được.
func TestTamperedPayloadRejected(t *testing.T) {
	iss := newIssuer(t)

	signed, err := iss.Issue(token.Claims{
		UserID: "usr_1",
		Roles:  []string{"OPS_SUPPORT"},
	}, time.Now())
	if err != nil {
		t.Fatalf("phát hành thất bại: %v", err)
	}

	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		t.Fatalf("token phải có 3 phần, nhận %d", len(parts))
	}

	// Đổi vai trò thành ADMIN, giữ nguyên chữ ký.
	var claims map[string]any
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("giải mã payload thất bại: %v", err)
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatalf("payload không phải JSON: %v", err)
	}
	claims["roles"] = []string{"ADMIN"}

	modified, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("mã hóa lại thất bại: %v", err)
	}
	parts[1] = base64.RawURLEncoding.EncodeToString(modified)
	tampered := strings.Join(parts, ".")

	if _, err := iss.Verify(tampered); err == nil {
		t.Fatal("token bị sửa nội dung PHẢI bị từ chối — nếu không, " +
			"người dùng tự nâng vai trò mình lên ADMIN được")
	}
}

// TestExpiredTokenDistinguishable: hết hạn phải TÁCH khỏi lỗi không hợp lệ.
//
// Tách để tầng gọi ghi log phân biệt "phiên hết hạn bình thường" với "có
// người thử token giả". Sự phân biệt này chỉ dành cho log, không cho client.
func TestExpiredTokenDistinguishable(t *testing.T) {
	iss, err := token.NewIssuer(token.Config{
		Secret: testSecret,
		TTL:    time.Minute,
	})
	if err != nil {
		t.Fatalf("tạo issuer thất bại: %v", err)
	}

	// Phát hành với thời điểm đã lùi quá TTL.
	past := time.Now().Add(-2 * time.Minute)
	signed, err := iss.Issue(token.Claims{UserID: "usr_1"}, past)
	if err != nil {
		t.Fatalf("phát hành thất bại: %v", err)
	}

	_, err = iss.Verify(signed)
	if !errors.Is(err, token.ErrExpired) {
		t.Fatalf("token hết hạn phải trả ErrExpired, nhận: %v", err)
	}
}

func TestValidTokenNotExpired(t *testing.T) {
	iss := newIssuer(t)

	signed, err := iss.Issue(token.Claims{UserID: "usr_1"}, time.Now())
	if err != nil {
		t.Fatalf("phát hành thất bại: %v", err)
	}

	if _, err := iss.Verify(signed); err != nil {
		t.Errorf("token còn hạn phải hợp lệ, nhận lỗi: %v", err)
	}
}

// TestTokenCarriesNoPersonalData: token KHÔNG được chứa email hay tên.
//
// Token đi kèm mọi request và nằm trong log của trình duyệt, proxy, và hệ
// thống giám sát. Dữ liệu không đặt vào là dữ liệu không rò rỉ được.
func TestTokenCarriesNoPersonalData(t *testing.T) {
	iss := newIssuer(t)

	signed, err := iss.Issue(token.Claims{
		UserID: "usr_1",
		Roles:  []string{"ADMIN"},
	}, time.Now())
	if err != nil {
		t.Fatalf("phát hành thất bại: %v", err)
	}

	parts := strings.Split(signed, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("giải mã payload thất bại: %v", err)
	}

	// Payload JWT chỉ được mã hóa base64, KHÔNG mã hóa bảo mật — ai cũng
	// đọc được. Vì thế nó không được chứa trường nhạy cảm nào.
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatalf("payload không phải JSON: %v", err)
	}

	for _, forbidden := range []string{"email", "phone", "name", "display_name", "password"} {
		if _, found := claims[forbidden]; found {
			t.Errorf("token KHÔNG được chứa trường %q — payload đọc được "+
				"bằng base64, không cần khóa", forbidden)
		}
	}
}

func TestMissingSubjectRejected(t *testing.T) {
	iss := newIssuer(t)

	// Token hợp lệ về chữ ký nhưng thiếu định danh người dùng.
	signed, err := iss.Issue(token.Claims{Roles: []string{"ADMIN"}}, time.Now())
	if err != nil {
		t.Fatalf("phát hành thất bại: %v", err)
	}

	if _, err := iss.Verify(signed); err == nil {
		t.Fatal("token thiếu UserID phải bị từ chối — nếu lọt, " +
			"truy vấn `WHERE user_id = ''` trông giống hệt 'không có dữ liệu'")
	}
}

func TestMalformedTokenRejected(t *testing.T) {
	iss := newIssuer(t)

	cases := map[string]string{
		"chuỗi rỗng":          "",
		"không phải JWT":      "khong-phai-token",
		"thiếu chữ ký":        "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c3JfMSJ9",
		"quá nhiều phần":      "a.b.c.d",
		"base64 không hợp lệ": "!!!.???.###",
	}

	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := iss.Verify(tok); err == nil {
				t.Errorf("token %q phải bị từ chối", name)
			}
		})
	}
}

func TestDefaultTTLMatchesIdentityDomain(t *testing.T) {
	iss := newIssuer(t)

	// 15 phút, khớp identity/domain.AccessTokenTTL. Hai chỗ lệch nhau
	// nghĩa là tài liệu nói một đằng, hệ thống làm một nẻo.
	if got := iss.TTL(); got != 15*time.Minute {
		t.Errorf("TTL mặc định phải là 15 phút, nhận %v", got)
	}
}
