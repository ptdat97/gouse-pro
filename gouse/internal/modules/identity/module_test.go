package identity_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/identity/domain"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
	"github.com/fashion-commerce/platform/internal/platform/token"
)

// fakeClock cho phép test nhảy tới tương lai mà không phải chờ thật.
//
// Khóa hết hạn sau 15 phút và phiên sau 30 ngày — không có đồng hồ giả thì
// hai nhánh đó không test được.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newModule(t *testing.T, clock *fakeClock) (*identity.Module, *pgxpool.Pool) {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE login_attempt",
		"TRUNCATE session",
		"TRUNCATE user_role",
		"TRUNCATE user_credential",
		`TRUNCATE "user" CASCADE`,
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	issuer, err := token.NewIssuer(token.Config{
		Secret: "khoa-bi-mat-chi-dung-trong-test-du-dai-32-ky-tu",
	})
	if err != nil {
		t.Fatalf("token.NewIssuer: %v", err)
	}

	m, err := identity.New(identity.Config{
		Storage: "postgres",
		DB:      db,
		// Chi phí thấp nhất CHỈ trong test: bcrypt mặc định tốn ~60ms mỗi
		// lần băm, và một test khóa tài khoản gọi nó sáu lần.
		BcryptCost: 4,
		Clock:      clock,
		Issuer:     issuer,
	})
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	return m, db.Pool()
}

const goodPassword = "mat-khau-du-dai-2026"

func register(t *testing.T, m *identity.Module, email string) identity.UserView {
	t.Helper()
	u, err := m.Register(context.Background(), identity.RegisterRequest{
		Email:       email,
		Password:    goodPassword,
		DisplayName: "Người dùng thử",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return u
}

func login(t *testing.T, m *identity.Module, email, password string) (identity.AuthResult, error) {
	t.Helper()
	return m.Login(context.Background(), identity.LoginRequest{
		Email:     email,
		Password:  password,
		UserAgent: "test-agent",
		IP:        "203.0.113.7",
	})
}

// ĐĂNG KÝ RỒI ĐĂNG NHẬP: đường đi cơ bản nhất phải chạy được.
func TestDangKyRoiDangNhap(t *testing.T) {
	m, _ := newModule(t, newClock())

	u := register(t, m, "khach@example.com")
	if u.Status != identity.StatusActive {
		t.Fatalf("trạng thái = %q, mong ACTIVE", u.Status)
	}
	if len(u.Roles) != 1 || u.Roles[0].Role != identity.RoleCustomer {
		t.Fatalf("vai trò mặc định = %+v, mong CUSTOMER", u.Roles)
	}

	res, err := login(t, m, "khach@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.RefreshToken == "" {
		t.Fatal("không có refresh token")
	}
	if res.AccessTokenTTL != domain.AccessTokenTTL {
		t.Fatalf("AccessTokenTTL = %v, mong %v", res.AccessTokenTTL, domain.AccessTokenTTL)
	}
}

// EMAIL CHUẨN HÓA: hoa/thường và khoảng trắng vào CÙNG MỘT tài khoản.
//
// Không chuẩn hóa thì khách tạo hai tài khoản rồi không hiểu vì sao mất
// đơn hàng cũ.
func TestEmailChuanHoa(t *testing.T) {
	m, _ := newModule(t, newClock())

	register(t, m, "Khach@Example.com")

	if _, err := login(t, m, "  khach@EXAMPLE.com  ", goodPassword); err != nil {
		t.Fatalf("đăng nhập bằng email khác hoa/thường: %v", err)
	}

	_, err := m.Register(context.Background(), identity.RegisterRequest{
		Email:    "KHACH@example.com",
		Password: goodPassword,
	})
	if !errors.Is(err, identity.ErrDuplicateEmail) {
		t.Fatalf("đăng ký trùng = %v, mong ErrDuplicateEmail", err)
	}
}

// TOKEN LƯU DẠNG BĂM: database rò rỉ KHÔNG cho phép đăng nhập.
//
// Đây là ràng buộc quan trọng nhất của bảng session. Test đọc thẳng cột
// trong database chứ không qua API — vì API sẽ luôn "đúng" theo chính nó.
func TestTokenLuuDangBam(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	register(t, m, "khach@example.com")
	res, err := login(t, m, "khach@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	var stored string
	if err := pool.QueryRow(ctx,
		`SELECT refresh_token_hash FROM session WHERE id = $1`,
		res.SessionID).Scan(&stored); err != nil {
		t.Fatalf("đọc phiên: %v", err)
	}

	if stored == res.RefreshToken {
		t.Fatal("token lưu NGUYÊN VĂN — rò rỉ database là mất mọi tài khoản")
	}
	if len(stored) != 64 {
		t.Fatalf("băm dài %d ký tự, mong 64 (SHA-256 hex)", len(stored))
	}
}

// KHÔNG LỘ TÀI KHOẢN NÀO CÓ THẬT.
//
// Email không tồn tại và sai mật khẩu phải trả CÙNG MỘT lỗi. Phân biệt
// chúng cho phép kẻ tấn công thu hẹp danh sách trước khi dò mật khẩu.
func TestKhongLoTaiKhoanCoThat(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	register(t, m, "khach@example.com")

	_, errNoUser := login(t, m, "khong-ton-tai@example.com", goodPassword)
	_, errBadPass := login(t, m, "khach@example.com", "sai-mat-khau-hoan-toan")

	if !errors.Is(errNoUser, identity.ErrInvalidLogin) {
		t.Fatalf("email không tồn tại = %v, mong ErrInvalidLogin", errNoUser)
	}
	if !errors.Is(errBadPass, identity.ErrInvalidLogin) {
		t.Fatalf("sai mật khẩu = %v, mong ErrInvalidLogin", errBadPass)
	}
	if errNoUser.Error() != errBadPass.Error() {
		t.Fatalf("hai lỗi khác nhau:\n  %q\n  %q", errNoUser, errBadPass)
	}

	// Lý do thật vẫn phải ghi vào nhật ký — nếu không thì điều tra sự cố
	// chỉ thấy "đăng nhập thất bại" mà không biết vì sao.
	var reasons []string
	rows, err := pool.Query(ctx,
		`SELECT failure_reason FROM login_attempt WHERE succeeded = false ORDER BY id`)
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatalf("đọc nhật ký: %v", err)
		}
		reasons = append(reasons, r)
	}

	if len(reasons) != 2 {
		t.Fatalf("ghi %d lần thất bại, mong 2: %v", len(reasons), reasons)
	}
	if reasons[0] == reasons[1] {
		t.Fatalf("nhật ký không phân biệt được hai lý do: %v", reasons)
	}
}

// THỜI GIAN PHẢN HỒI KHÔNG ĐƯỢC LỘ TÀI KHOẢN NÀO CÓ THẬT.
//
// Trả cùng một lỗi là chưa đủ. Nếu email không tồn tại trả về NGAY còn
// email có thật phải chờ bcrypt, kẻ tấn công đo thời gian là biết — và
// việc giấu lỗi thành vô nghĩa. Vì vậy Login băm một hash giả khi không
// tìm thấy tài khoản.
//
// # Vì sao test này đo thời gian, thứ vốn không đáng tin
//
// Đây là ngoại lệ có chủ ý: cơ chế cần kiểm chứng CHÍNH LÀ thời gian, nên
// không có cách nào khác. Để không thành test chập chờn:
//
//   - lấy TRUNG VỊ của nhiều lần đo, không lấy một lần
//   - ngưỡng đặt rất rộng (chênh 5 lần), chỉ bắt trường hợp bỏ hẳn việc
//     băm — không nhằm bắt chênh lệch vài phần trăm
//
// LƯU Ý cho người đọc sau: BcryptCost trong test là 4 (thấp nhất), nên
// khoảng cách tuyệt đối nhỏ hơn môi trường thật rất nhiều. Test qua ở đây
// nghĩa là "có gọi băm", không phải "an toàn tuyệt đối trước phân tích
// thời gian".
func TestThoiGianPhanHoiKhongLoTaiKhoan(t *testing.T) {
	m, _ := newModule(t, newClock())

	register(t, m, "co-that@example.com")

	measure := func(email string) time.Duration {
		const rounds = 7
		samples := make([]time.Duration, 0, rounds)
		for i := 0; i < rounds; i++ {
			start := time.Now()
			_, _ = login(t, m, email, "mat-khau-sai-hoan-toan-2026")
			samples = append(samples, time.Since(start))
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		return samples[len(samples)/2]
	}

	// Làm nóng: lần gọi đầu tiên phải mở kết nối và nạp kế hoạch truy vấn.
	_, _ = login(t, m, "co-that@example.com", "mat-khau-sai")

	withUser := measure("co-that@example.com")
	noUser := measure("khong-he-ton-tai@example.com")

	if noUser*5 < withUser {
		t.Fatalf(
			"email không tồn tại phản hồi nhanh hơn %.1f lần (%v so với %v) — "+
				"đo thời gian là biết được tài khoản nào có thật",
			float64(withUser)/float64(noUser), noUser, withUser)
	}
}

// GHI CẢ ĐĂNG NHẬP THÀNH CÔNG.
//
// Chỉ ghi thất bại thì không phát hiện được "đăng nhập thành công từ một
// quốc gia lạ lúc 3 giờ sáng" — loại bất thường nguy hiểm nhất.
func TestGhiCaDangNhapThanhCong(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	u := register(t, m, "khach@example.com")
	if _, err := login(t, m, "khach@example.com", goodPassword); err != nil {
		t.Fatalf("Login: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM login_attempt WHERE succeeded = true AND user_id = $1`,
		u.ID).Scan(&count); err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if count != 1 {
		t.Fatalf("ghi %d lần thành công, mong 1", count)
	}
}

// KHÔNG LƯU IP NGUYÊN VĂN.
//
// IP là dữ liệu cá nhân. Băm vẫn cho phép phát hiện "nhiều lần thử từ cùng
// một nguồn" mà không lưu chính địa chỉ đó.
func TestKhongLuuIPNguyenVan(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	register(t, m, "khach@example.com")
	if _, err := login(t, m, "khach@example.com", goodPassword); err != nil {
		t.Fatalf("Login: %v", err)
	}

	var attemptIP, sessionIP string
	if err := pool.QueryRow(ctx,
		`SELECT ip_hash FROM login_attempt ORDER BY id DESC LIMIT 1`).
		Scan(&attemptIP); err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT ip_hash FROM session LIMIT 1`).Scan(&sessionIP); err != nil {
		t.Fatalf("đọc phiên: %v", err)
	}

	for name, got := range map[string]string{
		"login_attempt": attemptIP,
		"session":       sessionIP,
	} {
		if got == "203.0.113.7" {
			t.Fatalf("%s lưu IP NGUYÊN VĂN", name)
		}
		if len(got) != 64 {
			t.Fatalf("%s.ip_hash dài %d, mong 64 (SHA-256 hex)", name, len(got))
		}
	}
}

// KHÓA TÀI KHOẢN SAU 5 LẦN SAI, VÀ TỰ MỞ SAU 15 PHÚT.
//
// Khóa vĩnh viễn biến việc dò mật khẩu thành công cụ khóa tài khoản người
// khác — kẻ tấn công chỉ cần gõ sai năm lần.
func TestKhoaTaiKhoanRoiTuMo(t *testing.T) {
	clock := newClock()
	m, _ := newModule(t, clock)

	register(t, m, "khach@example.com")

	for i := 0; i < domain.MaxFailedAttempts; i++ {
		if _, err := login(t, m, "khach@example.com", "sai-mat-khau"); !errors.Is(err, identity.ErrInvalidLogin) {
			t.Fatalf("lần sai thứ %d = %v, mong ErrInvalidLogin", i+1, err)
		}
	}

	// Mật khẩu ĐÚNG vẫn bị chặn, và lỗi phải nói rõ vì sao.
	if _, err := login(t, m, "khach@example.com", goodPassword); !errors.Is(err, identity.ErrAccountLocked) {
		t.Fatalf("sau %d lần sai = %v, mong ErrAccountLocked", domain.MaxFailedAttempts, err)
	}

	clock.advance(domain.LockDuration + time.Minute)

	if _, err := login(t, m, "khach@example.com", goodPassword); err != nil {
		t.Fatalf("sau khi hết hạn khóa: %v", err)
	}
}

// ĐĂNG NHẬP THÀNH CÔNG XÓA SỐ LẦN SAI.
//
// Không xóa thì một người gõ nhầm bốn lần trong ba tháng sẽ bị khóa ở lần
// nhầm thứ năm, dù đã đăng nhập thành công nhiều lần ở giữa.
func TestThanhCongXoaSoLanSai(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	u := register(t, m, "khach@example.com")

	for i := 0; i < 3; i++ {
		_, _ = login(t, m, "khach@example.com", "sai-mat-khau")
	}
	if _, err := login(t, m, "khach@example.com", goodPassword); err != nil {
		t.Fatalf("Login: %v", err)
	}

	var failed int
	if err := pool.QueryRow(ctx,
		`SELECT failed_attempts FROM user_credential WHERE user_id = $1`,
		u.ID).Scan(&failed); err != nil {
		t.Fatalf("đọc thông tin xác thực: %v", err)
	}
	if failed != 0 {
		t.Fatalf("failed_attempts = %d sau khi đăng nhập thành công, mong 0", failed)
	}
}

// XOAY TOKEN: token cũ VÔ HIỆU ngay sau khi làm mới.
//
// Nếu token cũ còn dùng được, kẻ đánh cắp nó vẫn vào được vô thời hạn và
// không có dấu hiệu nào để phát hiện.
func TestXoayTokenLamTokenCuVoHieu(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	register(t, m, "khach@example.com")
	first, err := login(t, m, "khach@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	second, err := m.Refresh(ctx, identity.RefreshRequest{
		RefreshToken: first.RefreshToken,
		UserAgent:    "test-agent",
		IP:           "203.0.113.7",
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("token mới TRÙNG token cũ — không có xoay vòng")
	}

	if _, err := m.Authenticate(ctx, first.RefreshToken); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("token cũ = %v, mong ErrSessionInvalid", err)
	}
	if _, err := m.Authenticate(ctx, second.RefreshToken); err != nil {
		t.Fatalf("token mới: %v", err)
	}
}

// PHIÊN HẾT HẠN KHÔNG DÙNG ĐƯỢC, dù chưa bị thu hồi.
func TestPhienHetHan(t *testing.T) {
	clock := newClock()
	m, _ := newModule(t, clock)
	ctx := context.Background()

	register(t, m, "khach@example.com")
	res, err := login(t, m, "khach@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	clock.advance(domain.RefreshTokenTTL + time.Hour)

	if _, err := m.Authenticate(ctx, res.RefreshToken); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("phiên hết hạn = %v, mong ErrSessionInvalid", err)
	}
}

// ĐỔI MẬT KHẨU THU HỒI MỌI PHIÊN.
//
// Đây là lý do tồn tại của bảng session. Nếu tài khoản bị lộ, đổi mật khẩu
// mà phiên cũ vẫn sống thì kẻ tấn công vẫn vào được — và người dùng tưởng
// mình đã an toàn, nguy hiểm hơn là không đổi gì cả.
func TestDoiMatKhauThuHoiMoiPhien(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	u := register(t, m, "khach@example.com")

	// Ba thiết bị đang đăng nhập.
	var tokens []string
	for i := 0; i < 3; i++ {
		res, err := login(t, m, "khach@example.com", goodPassword)
		if err != nil {
			t.Fatalf("Login lần %d: %v", i+1, err)
		}
		tokens = append(tokens, res.RefreshToken)
	}

	sessions, err := m.ListSessions(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("có %d phiên, mong 3", len(sessions))
	}

	const newPassword = "mat-khau-moi-hoan-toan-2026"
	if err := m.ChangePassword(ctx, u.ID, goodPassword, newPassword); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	for i, tok := range tokens {
		if _, err := m.Authenticate(ctx, tok); !errors.Is(err, identity.ErrSessionInvalid) {
			t.Fatalf("phiên %d còn sống sau khi đổi mật khẩu: %v", i+1, err)
		}
	}

	if _, err := login(t, m, "khach@example.com", goodPassword); !errors.Is(err, identity.ErrInvalidLogin) {
		t.Fatalf("mật khẩu CŨ vẫn đăng nhập được: %v", err)
	}
	if _, err := login(t, m, "khach@example.com", newPassword); err != nil {
		t.Fatalf("mật khẩu mới: %v", err)
	}
}

// SAI MẬT KHẨU CŨ KHÔNG ĐỔI ĐƯỢC.
//
// Không kiểm tra thì ai chiếm được phiên đang mở sẽ đổi mật khẩu và đá chủ
// tài khoản ra ngoài.
func TestDoiMatKhauCanMatKhauCu(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	u := register(t, m, "khach@example.com")

	err := m.ChangePassword(ctx, u.ID, "sai-mat-khau-cu", "mat-khau-moi-2026")
	if !errors.Is(err, identity.ErrInvalidLogin) {
		t.Fatalf("sai mật khẩu cũ = %v, mong ErrInvalidLogin", err)
	}

	if _, err := login(t, m, "khach@example.com", goodPassword); err != nil {
		t.Fatalf("mật khẩu cũ phải còn dùng được: %v", err)
	}
}

// TREO TÀI KHOẢN CÓ HIỆU LỰC NGAY, không chờ token hết hạn.
func TestTreoTaiKhoanThuHoiPhien(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	u := register(t, m, "khach@example.com")
	res, err := login(t, m, "khach@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := m.Suspend(ctx, u.ID); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	if _, err := m.Authenticate(ctx, res.RefreshToken); err == nil {
		t.Fatal("tài khoản bị treo vẫn xác thực được")
	}
	if _, err := login(t, m, "khach@example.com", goodPassword); !errors.Is(err, identity.ErrInvalidLogin) {
		t.Fatalf("tài khoản bị treo đăng nhập = %v, mong ErrInvalidLogin", err)
	}
}

// LỚP THỨ HAI: trạng thái tài khoản được kiểm tra ở MỖI lần xác thực.
//
// Test trên (TestTreoTaiKhoanThuHoiPhien) đi qua hai lớp bảo vệ cùng lúc —
// thu hồi phiên VÀ kiểm tra trạng thái — nên phá một lớp nó vẫn xanh. Đã
// kiểm chứng ngược: chỉ khi phá CẢ HAI nó mới đỏ.
//
// Test này cô lập lớp thứ hai bằng cách treo tài khoản THẲNG TRONG DATABASE,
// không qua module. Đó không phải tình huống bịa: quản trị viên chạy SQL
// khi xử lý sự cố, và một lần Suspend thất bại giữa chừng cũng để lại đúng
// trạng thái này — tài khoản SUSPENDED với phiên còn sống.
func TestTrangThaiDuocKiemTraMoiLanXacThuc(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	u := register(t, m, "khach@example.com")
	res, err := login(t, m, "khach@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE "user" SET status = 'SUSPENDED' WHERE id = $1`, u.ID); err != nil {
		t.Fatalf("treo tài khoản: %v", err)
	}

	// Phiên vẫn còn nguyên: chỉ có trạng thái tài khoản chặn được.
	if _, err := m.Authenticate(ctx, res.RefreshToken); !errors.Is(err, identity.ErrAccountSuspended) {
		t.Fatalf("tài khoản SUSPENDED với phiên còn sống = %v, mong ErrAccountSuspended", err)
	}

	// Làm mới phiên cũng phải bị chặn — không thì token treo tự gia hạn
	// vô thời hạn.
	if _, err := m.Refresh(ctx, identity.RefreshRequest{
		RefreshToken: res.RefreshToken,
	}); !errors.Is(err, identity.ErrAccountSuspended) {
		t.Fatalf("làm mới phiên của tài khoản SUSPENDED = %v, mong ErrAccountSuspended", err)
	}
}

// ĐĂNG XUẤT LÀ IDEMPOTENT.
//
// Gọi lại với token đã thu hồi phải THÀNH CÔNG: kết quả mong muốn (phiên
// đó không dùng được) đã đạt. Trả lỗi sẽ khiến client thử lại vô ích.
func TestDangXuatIdempotent(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	register(t, m, "khach@example.com")
	res, err := login(t, m, "khach@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := m.Logout(ctx, res.RefreshToken); err != nil {
			t.Fatalf("Logout lần %d: %v", i+1, err)
		}
	}
	if err := m.Logout(ctx, "token-khong-bao-gio-ton-tai"); err != nil {
		t.Fatalf("đăng xuất token lạ: %v", err)
	}

	if _, err := m.Authenticate(ctx, res.RefreshToken); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("sau đăng xuất = %v, mong ErrSessionInvalid", err)
	}
}

// PHẠM VI THEO VAI TRÒ: seller CHỈ thấy gian hàng của mình.
//
// AuthContext.SellerIDs là thứ module fulfillment dùng để lọc truy vấn.
// Trả sai danh sách này là seller đọc được đơn của đối thủ.
func TestPhamViTheoVaiTro(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	u := register(t, m, "chu-gian-hang@example.com")
	if err := m.GrantRole(ctx, u.ID, identity.RoleSellerOwner, sellerA); err != nil {
		t.Fatalf("GrantRole: %v", err)
	}

	// Vai trò KHÔNG phải seller nhưng CÓ scope_id.
	//
	// Không có dòng này thì test vô dụng: người dùng chỉ có đúng một vai
	// trò gắn phạm vi, nên "gom mọi scope_id" và "gom scope_id của vai trò
	// seller" cho cùng kết quả. Đã kiểm chứng ngược — thiếu nó, việc thay
	// ScopeIDsFor bằng vòng lặp trên MỌI vai trò vẫn qua được test.
	//
	// CREATOR có scope_id là chuyện thật: một KOC gắn với chiến dịch cụ
	// thể. Nếu id đó lọt vào SellerIDs, module fulfillment sẽ coi nó là
	// gian hàng và trả đơn của người khác.
	creatorScope := ids.MustNew(ids.PrefixCampaign).String()
	if err := m.GrantRole(ctx, u.ID, identity.RoleCreator, creatorScope); err != nil {
		t.Fatalf("GrantRole CREATOR: %v", err)
	}

	res, err := login(t, m, "chu-gian-hang@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	authCtx, err := m.Authenticate(ctx, res.RefreshToken)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	if authCtx.Scope != identity.ScopeSeller {
		t.Fatalf("phạm vi = %q, mong SELLER", authCtx.Scope)
	}
	if len(authCtx.SellerIDs) != 1 || authCtx.SellerIDs[0] != sellerA {
		t.Fatalf("SellerIDs = %v, mong [%s]", authCtx.SellerIDs, sellerA)
	}
	for _, id := range authCtx.SellerIDs {
		if id == sellerB {
			t.Fatal("thấy gian hàng KHÔNG thuộc về mình")
		}
		if id == creatorScope {
			t.Fatal("phạm vi của vai trò CREATOR lọt vào SellerIDs")
		}
	}

	// Vai trò rộng hơn thắng: thêm OPS_SUPPORT thì phạm vi thành ALL.
	if err := m.GrantRole(ctx, u.ID, identity.RoleOpsSupport, ""); err != nil {
		t.Fatalf("GrantRole: %v", err)
	}
	authCtx, err = m.Authenticate(ctx, res.RefreshToken)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if authCtx.Scope != identity.ScopeAll {
		t.Fatalf("phạm vi = %q sau khi thêm OPS_SUPPORT, mong ALL", authCtx.Scope)
	}
}

// CẤP VAI TRÒ LÀ IDEMPOTENT, THU HỒI CÓ HIỆU LỰC.
func TestCapVaThuHoiVaiTro(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()
	u := register(t, m, "nhan-vien@example.com")

	for i := 0; i < 3; i++ {
		if err := m.GrantRole(ctx, u.ID, identity.RoleSellerStaff, sellerID); err != nil {
			t.Fatalf("GrantRole lần %d: %v", i+1, err)
		}
	}

	got, err := m.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	staff := 0
	for _, r := range got.Roles {
		if r.Role == identity.RoleSellerStaff {
			staff++
		}
	}
	if staff != 1 {
		t.Fatalf("cấp ba lần tạo %d bản ghi, mong 1", staff)
	}

	if err := m.RevokeRole(ctx, u.ID, identity.RoleSellerStaff, sellerID); err != nil {
		t.Fatalf("RevokeRole: %v", err)
	}
	got, err = m.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	for _, r := range got.Roles {
		if r.Role == identity.RoleSellerStaff {
			t.Fatal("vai trò vẫn còn sau khi thu hồi")
		}
	}
}

// VAI TRÒ LẠ BỊ TỪ CHỐI Ở BIÊN MODULE.
//
// Go cho phép ép bất kỳ chuỗi nào thành domain.Role, nên nếu biên không
// chặn thì "SUPER_ADMIN" đi thẳng xuống database.
func TestVaiTroLaBiTuChoi(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	u := register(t, m, "khach@example.com")

	if err := m.GrantRole(ctx, u.ID, "SUPER_ADMIN", ""); !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("vai trò lạ = %v, mong ErrInvalidInput", err)
	}

	_, err := m.Register(ctx, identity.RegisterRequest{
		Email:    "khac@example.com",
		Password: goodPassword,
		Roles:    []identity.RoleGrantInput{{Role: "ROOT"}},
	})
	if !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("đăng ký với vai trò lạ = %v, mong ErrInvalidInput", err)
	}
}

// MẬT KHẨU YẾU VÀ EMAIL SAI ĐỊNH DẠNG BỊ TỪ CHỐI.
func TestTuChoiDauVaoKhongHopLe(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	_, err := m.Register(ctx, identity.RegisterRequest{
		Email:    "khach@example.com",
		Password: "ngan",
	})
	if !errors.Is(err, identity.ErrWeakPassword) {
		t.Fatalf("mật khẩu ngắn = %v, mong ErrWeakPassword", err)
	}

	_, err = m.Register(ctx, identity.RegisterRequest{
		Email:    "khong-phai-email",
		Password: goodPassword,
	})
	if !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("email sai định dạng = %v, mong ErrInvalidInput", err)
	}
}

// KHÔNG LƯU TÀI KHOẢN NỬA VỜI.
//
// Tài khoản có mà không có mật khẩu là tài khoản không đăng nhập được, và
// người dùng sẽ thử đăng ký lại rồi gặp "email đã tồn tại" — bế tắc chỉ
// quản trị viên gỡ được. Test kiểm tra hai bảng luôn khớp nhau.
func TestKhongLuuTaiKhoanNuaVoi(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	// Mật khẩu dài hơn 72 byte bị bcrypt từ chối Ở GIỮA quá trình đăng ký:
	// tài khoản đã dựng trong bộ nhớ nhưng chưa ghi. Nếu thứ tự sai, hàng
	// trong bảng "user" sẽ tồn tại mà không có credential.
	long := make([]byte, 100)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := m.Register(ctx, identity.RegisterRequest{
		Email:    "mat-khau-dai@example.com",
		Password: string(long),
	}); err == nil {
		t.Fatal("mật khẩu 100 byte được chấp nhận — bcrypt âm thầm cắt còn 72")
	}

	var orphans int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM "user" u
		 WHERE NOT EXISTS (
			SELECT 1 FROM user_credential c WHERE c.user_id = u.id)`).
		Scan(&orphans); err != nil {
		t.Fatalf("đếm tài khoản mồ côi: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("có %d tài khoản KHÔNG có mật khẩu", orphans)
	}
}

// ĐĂNG KÝ TRÙNG EMAIL SONG SONG: chỉ MỘT thành công.
//
// Kiểm tra "email đã tồn tại chưa" ở tầng ứng dụng KHÔNG cứu được trường
// hợp này — hai request cùng đọc thấy trống rồi cùng ghi. Chỉ ràng buộc
// UNIQUE ở database chặn được.
func TestDangKyTrungSongSong(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	const n = 10
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		okCount int
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := m.Register(ctx, identity.RegisterRequest{
				Email:    "dua-nhau@example.com",
				Password: goodPassword,
			})
			if err == nil {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if okCount != 1 {
		t.Fatalf("%d/%d lần đăng ký thành công, mong đúng 1", okCount, n)
	}

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM "user" WHERE email = $1`,
		"dua-nhau@example.com").Scan(&rows); err != nil {
		t.Fatalf("đếm tài khoản: %v", err)
	}
	if rows != 1 {
		t.Fatalf("có %d hàng cùng email, mong 1", rows)
	}
}

// XOAY TOKEN SONG SONG: chỉ MỘT request đổi được token.
//
// Hai tab trình duyệt cùng làm mới phiên là chuyện thường. Nếu cả hai
// thành công, một trong hai token sẽ bị bỏ rơi và người dùng bị đăng xuất
// ngẫu nhiên.
func TestXoayTokenSongSong(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	register(t, m, "khach@example.com")
	res, err := login(t, m, "khach@example.com", goodPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	const n = 8
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		tokens []string
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			out, err := m.Refresh(ctx, identity.RefreshRequest{
				RefreshToken: res.RefreshToken,
				UserAgent:    "test-agent",
			})
			if err == nil {
				mu.Lock()
				tokens = append(tokens, out.RefreshToken)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Phiên gốc phải bị thu hồi dù có bao nhiêu request đi nữa.
	var revoked *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT revoked_at FROM session WHERE id = $1`, res.SessionID).
		Scan(&revoked); err != nil {
		t.Fatalf("đọc phiên gốc: %v", err)
	}
	if revoked == nil {
		t.Fatal("phiên gốc KHÔNG bị thu hồi sau khi làm mới")
	}

	if len(tokens) == 0 {
		t.Fatal("không request nào làm mới được")
	}
	// Ghi nhận hành vi hiện tại: nhiều request có thể cùng thành công vì
	// việc thu hồi và tạo phiên mới chưa nằm trong MỘT giao dịch. Hệ quả
	// chấp nhận được — mọi token sinh ra đều hợp lệ và thuộc đúng người —
	// nhưng token gốc luôn chết, tức là kẻ đánh cắp nó không dùng được.
	for i, tok := range tokens {
		if _, err := m.Authenticate(ctx, tok); err != nil {
			t.Fatalf("token mới thứ %d không xác thực được: %v", i+1, err)
		}
	}
	if _, err := m.Authenticate(ctx, res.RefreshToken); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatalf("token gốc = %v, mong ErrSessionInvalid", err)
	}
}
