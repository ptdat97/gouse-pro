package customer_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/customer"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

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

func newModule(t *testing.T, clock *fakeClock) (*customer.Module, *pgxpool.Pool) {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE customer_merge_log",
		"TRUNCATE wishlist_item",
		"TRUNCATE wishlist CASCADE",
		"TRUNCATE customer_consent",
		"TRUNCATE customer_address CASCADE",
		"TRUNCATE customer CASCADE",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	m, err := customer.New(customer.Config{
		Storage: "postgres",
		DB:      db,
		Clock:   clock,
	})
	if err != nil {
		t.Fatalf("customer.New: %v", err)
	}
	return m, db.Pool()
}

func create(t *testing.T, m *customer.Module, email string) customer.CustomerView {
	t.Helper()
	c, err := m.Create(context.Background(), customer.CreateRequest{
		Email:       email,
		DisplayName: "Khách thử",
		Phone:       "0900000000",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return c
}

func addrReq(customerID, line1 string) customer.AddressRequest {
	return customer.AddressRequest{
		CustomerID:     customerID,
		RecipientName:  "Người nhận",
		RecipientPhone: "0911111111",
		Line1:          line1,
		Ward:           "Phường 1",
		District:       "Quận 1",
		Province:       "TP.HCM",
	}
}

// TẠO HỒ SƠ: khách vãng lai và khách đã đăng ký khác nhau ở user_id.
func TestTaoHoSo(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	guest := create(t, m, "vanglai@example.com")
	if guest.Status != customer.StatusGuest {
		t.Fatalf("trạng thái = %q, mong GUEST", guest.Status)
	}
	if guest.UserID != "" {
		t.Fatalf("khách vãng lai có user_id = %q", guest.UserID)
	}

	userID := ids.MustNew(ids.PrefixUser).String()
	registered, err := m.Create(ctx, customer.CreateRequest{
		Email:  "dangky@example.com",
		UserID: userID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if registered.Status != customer.StatusRegistered {
		t.Fatalf("trạng thái = %q, mong REGISTERED", registered.Status)
	}
}

// EMAIL CHUẨN HÓA GIỐNG HỆT identity.
//
// Hai module lưu email khác định dạng thì không bao giờ gộp được danh tính
// khách vãng lai với tài khoản đăng ký sau đó.
func TestEmailChuanHoa(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "  Khach@Example.COM  ")

	var stored string
	if err := pool.QueryRow(ctx,
		`SELECT email FROM customer WHERE id = $1`, c.ID).Scan(&stored); err != nil {
		t.Fatalf("đọc hồ sơ: %v", err)
	}
	if stored != "khach@example.com" {
		t.Fatalf("email lưu %q, mong \"khach@example.com\"", stored)
	}

	_, err := m.Create(ctx, customer.CreateRequest{Email: "KHACH@example.com"})
	if !errors.Is(err, customer.ErrDuplicateEmail) {
		t.Fatalf("tạo trùng = %v, mong ErrDuplicateEmail", err)
	}
}

// KHÁCH VÃNG LAI QUAY LẠI PHẢI VÀO ĐÚNG HỒ SƠ CŨ.
//
// Tạo hồ sơ thứ hai nghĩa là lịch sử mua hàng bị chia ra, và không bao giờ
// gộp lại được vì hai hồ sơ trông như hai người khác nhau.
func TestKhachVangLaiQuayLai(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	req := customer.CreateRequest{Email: "quaylai@example.com"}

	first, err := m.EnsureByEmail(ctx, req)
	if err != nil {
		t.Fatalf("EnsureByEmail: %v", err)
	}
	second, err := m.EnsureByEmail(ctx, req)
	if err != nil {
		t.Fatalf("EnsureByEmail lần 2: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("hai hồ sơ khác nhau: %s và %s", first.ID, second.ID)
	}
}

// ĐUA NHAU TẠO HỒ SƠ CÙNG EMAIL: tất cả nhận CÙNG MỘT hồ sơ.
//
// Kiểm tra "email đã tồn tại chưa" ở tầng ứng dụng KHÔNG cứu được — mười
// request cùng đọc thấy trống rồi cùng ghi. Chỉ ràng buộc UNIQUE chặn
// được, và EnsureByEmail phải đọc lại thay vì báo lỗi ra ngoài.
func TestDuaNhauTaoHoSo(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	const n = 10
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		got []string
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			c, err := m.EnsureByEmail(ctx, customer.CreateRequest{
				Email: "dua-nhau@example.com",
			})
			if err == nil {
				mu.Lock()
				got = append(got, c.ID)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(got) != n {
		t.Fatalf("%d/%d request thành công, mong tất cả", len(got), n)
	}
	for i, id := range got {
		if id != got[0] {
			t.Fatalf("request %d nhận hồ sơ %s, khác %s", i+1, id, got[0])
		}
	}

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM customer WHERE email = $1`,
		"dua-nhau@example.com").Scan(&rows); err != nil {
		t.Fatalf("đếm hồ sơ: %v", err)
	}
	if rows != 1 {
		t.Fatalf("có %d hồ sơ cùng email, mong 1", rows)
	}
}

// GẮN TÀI KHOẢN GIỮ NGUYÊN customer_id.
//
// Đây là lý do khách vãng lai cũng có hồ sơ: khi họ đăng ký, toàn bộ đơn
// hàng cũ tự thuộc về tài khoản mới mà không phải chuyển gì.
func TestGanTaiKhoanGiuNguyenID(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	guest := create(t, m, "vanglai@example.com")
	userID := ids.MustNew(ids.PrefixUser).String()

	if err := m.LinkUser(ctx, guest.ID, userID); err != nil {
		t.Fatalf("LinkUser: %v", err)
	}

	got, err := m.GetCustomerByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetCustomerByUserID: %v", err)
	}
	if got.ID != guest.ID {
		t.Fatalf("customer_id đổi từ %s thành %s", guest.ID, got.ID)
	}
	if got.Status != customer.StatusRegistered {
		t.Fatalf("trạng thái = %q, mong REGISTERED", got.Status)
	}
}

// ĐỊA CHỈ ĐẦU TIÊN TỰ THÀNH MẶC ĐỊNH.
//
// Không có mặc định thì trang thanh toán không biết điền gì, và khách phải
// chọn lại địa chỉ ở mỗi đơn.
func TestDiaChiDauTienLaMacDinh(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	a, err := m.AddAddress(ctx, addrReq(c.ID, "12 Nguyễn Huệ"))
	if err != nil {
		t.Fatalf("AddAddress: %v", err)
	}
	if !a.IsDefault {
		t.Fatal("địa chỉ đầu tiên KHÔNG phải mặc định")
	}

	def, err := m.GetDefaultAddress(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetDefaultAddress: %v", err)
	}
	if def.ID != a.ID {
		t.Fatalf("mặc định là %s, mong %s", def.ID, a.ID)
	}
}

// ĐÚNG MỘT ĐỊA CHỈ MẶC ĐỊNH — kiểm tra THẲNG trong database.
//
// Đây là ràng buộc quan trọng nhất của sổ địa chỉ: hai mặc định nghĩa là
// đơn hàng lấy địa chỉ nào do thứ tự sắp xếp quyết định, và hàng có thể đi
// tới địa chỉ khách đã chuyển đi từ lâu.
func TestDungMotDiaChiMacDinh(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	var addrIDs []string
	for i, line := range []string{"12 Nguyễn Huệ", "34 Lê Lợi", "56 Hai Bà Trưng"} {
		req := addrReq(c.ID, line)
		// Yêu cầu làm mặc định ở MỌI địa chỉ: nếu không gỡ cờ cũ, chỉ mục
		// UNIQUE sẽ báo lỗi ngay ở địa chỉ thứ hai.
		req.IsDefault = true
		a, err := m.AddAddress(ctx, req)
		if err != nil {
			t.Fatalf("AddAddress %d: %v", i+1, err)
		}
		addrIDs = append(addrIDs, a.ID)
	}

	var defaults int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM customer_address
		 WHERE customer_id = $1 AND is_default AND deleted_at IS NULL`,
		c.ID).Scan(&defaults); err != nil {
		t.Fatalf("đếm địa chỉ mặc định: %v", err)
	}
	if defaults != 1 {
		t.Fatalf("có %d địa chỉ mặc định, mong ĐÚNG 1", defaults)
	}

	// Địa chỉ CUỐI CÙNG được đặt mặc định phải là địa chỉ thắng.
	def, err := m.GetDefaultAddress(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetDefaultAddress: %v", err)
	}
	if def.ID != addrIDs[len(addrIDs)-1] {
		t.Fatalf("mặc định là %s, mong %s (địa chỉ đặt sau cùng)",
			def.ID, addrIDs[len(addrIDs)-1])
	}
}

// ĐỔI MẶC ĐỊNH SONG SONG: vẫn ĐÚNG MỘT.
//
// Năm tab cùng bấm "đặt làm mặc định" cho năm địa chỉ khác nhau. Nếu việc
// gỡ cờ cũ và đặt cờ mới không nằm trong một giao dịch, kết quả có thể là
// không có mặc định nào — hoặc lỗi chỉ mục.
func TestDoiMacDinhSongSong(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	var addrIDs []string
	for _, line := range []string{"A", "B", "C", "D", "E"} {
		a, err := m.AddAddress(ctx, addrReq(c.ID, line))
		if err != nil {
			t.Fatalf("AddAddress: %v", err)
		}
		addrIDs = append(addrIDs, a.ID)
	}

	var wg sync.WaitGroup
	wg.Add(len(addrIDs))
	for _, id := range addrIDs {
		go func() {
			defer wg.Done()
			_ = m.SetDefaultAddress(ctx, c.ID, id)
		}()
	}
	wg.Wait()

	var defaults int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM customer_address
		 WHERE customer_id = $1 AND is_default AND deleted_at IS NULL`,
		c.ID).Scan(&defaults); err != nil {
		t.Fatalf("đếm địa chỉ mặc định: %v", err)
	}
	if defaults != 1 {
		t.Fatalf("có %d địa chỉ mặc định sau khi đua, mong ĐÚNG 1", defaults)
	}
}

// CÁCH LY GIỮA CÁC KHÁCH: biết id địa chỉ người khác KHÔNG đọc/sửa được.
//
// Địa chỉ chứa tên, số điện thoại và địa chỉ nhà — rò rỉ nó nguy hiểm hơn
// rò rỉ lịch sử mua hàng.
func TestCachLyDiaChiGiuaCacKhach(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	a := create(t, m, "khach-a@example.com")
	b := create(t, m, "khach-b@example.com")

	addrOfA, err := m.AddAddress(ctx, addrReq(a.ID, "12 Nguyễn Huệ"))
	if err != nil {
		t.Fatalf("AddAddress: %v", err)
	}

	// B sửa địa chỉ của A.
	req := addrReq(b.ID, "ĐỊA CHỈ BỊ CHIẾM")
	if _, err := m.UpdateAddress(ctx, addrOfA.ID, req); !errors.Is(err, customer.ErrAddressNotFound) {
		t.Fatalf("B sửa địa chỉ của A = %v, mong ErrAddressNotFound", err)
	}

	// B đặt địa chỉ của A làm mặc định của mình.
	if err := m.SetDefaultAddress(ctx, b.ID, addrOfA.ID); !errors.Is(err, customer.ErrAddressNotFound) {
		t.Fatalf("B chiếm địa chỉ của A = %v, mong ErrAddressNotFound", err)
	}

	// B xóa địa chỉ của A.
	if err := m.DeleteAddress(ctx, b.ID, addrOfA.ID); !errors.Is(err, customer.ErrAddressNotFound) {
		t.Fatalf("B xóa địa chỉ của A = %v, mong ErrAddressNotFound", err)
	}

	// Địa chỉ của A phải còn nguyên.
	list, err := m.GetAddresses(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetAddresses: %v", err)
	}
	if len(list) != 1 || list[0].Line1 != "12 Nguyễn Huệ" {
		t.Fatalf("địa chỉ của A đã bị đổi: %+v", list)
	}
}

// XÓA ĐỊA CHỈ LÀ XÓA MỀM, và gỡ luôn cờ mặc định.
func TestXoaDiaChiLaXoaMem(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")
	a, err := m.AddAddress(ctx, addrReq(c.ID, "12 Nguyễn Huệ"))
	if err != nil {
		t.Fatalf("AddAddress: %v", err)
	}

	if err := m.DeleteAddress(ctx, c.ID, a.ID); err != nil {
		t.Fatalf("DeleteAddress: %v", err)
	}

	// Hàng vẫn còn trong database — khách cần thấy lại địa chỉ đã dùng khi
	// đặt lại đơn cũ.
	var deletedAt *time.Time
	var isDefault bool
	if err := pool.QueryRow(ctx,
		`SELECT deleted_at, is_default FROM customer_address WHERE id = $1`,
		a.ID).Scan(&deletedAt, &isDefault); err != nil {
		t.Fatalf("đọc địa chỉ: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("địa chỉ bị XÓA CỨNG")
	}
	if isDefault {
		t.Fatal("địa chỉ đã xóa vẫn mang cờ mặc định")
	}

	// Nhưng không hiện trong danh sách.
	list, err := m.GetAddresses(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetAddresses: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("địa chỉ đã xóa vẫn hiện: %+v", list)
	}

	// Địa chỉ mới thêm sau đó lên làm mặc định được.
	fresh, err := m.AddAddress(ctx, addrReq(c.ID, "34 Lê Lợi"))
	if err != nil {
		t.Fatalf("AddAddress sau khi xóa: %v", err)
	}
	if !fresh.IsDefault {
		t.Fatal("địa chỉ mới KHÔNG thành mặc định dù khách không còn địa chỉ nào")
	}
}

// ĐỊA CHỈ THIẾU TRƯỜNG BẮT BUỘC BỊ TỪ CHỐI.
//
// Thiếu tên người nhận hay số điện thoại thì đơn vị vận chuyển không giao
// được — và lỗi đó chỉ lộ ra khi hàng đã đóng gói xong.
func TestDiaChiThieuTruongBatBuoc(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	for name, mutate := range map[string]func(*customer.AddressRequest){
		"thiếu tên người nhận": func(r *customer.AddressRequest) { r.RecipientName = "" },
		"thiếu số điện thoại":  func(r *customer.AddressRequest) { r.RecipientPhone = "" },
		"thiếu dòng địa chỉ":   func(r *customer.AddressRequest) { r.Line1 = "" },
	} {
		req := addrReq(c.ID, "12 Nguyễn Huệ")
		mutate(&req)
		if _, err := m.AddAddress(ctx, req); !errors.Is(err, customer.ErrInvalidInput) {
			t.Fatalf("%s = %v, mong ErrInvalidInput", name, err)
		}
	}
}

// ĐỒNG Ý LÀ NHẬT KÝ CHỈ GHI THÊM.
//
// Nghĩa vụ pháp lý là chứng minh được khách đã đồng ý VÀO LÚC NÀO và Ở
// ĐÂU. Sửa bản ghi cũ là hủy chính bằng chứng đó.
func TestDongYLaNhatKyChiGhiThem(t *testing.T) {
	clock := newClock()
	m, pool := newModule(t, clock)
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	req := customer.ConsentRequest{
		CustomerID:    c.ID,
		Type:          customer.ConsentMarketingEmail,
		Granted:       true,
		Source:        "signup_form",
		PolicyVersion: "2026-01",
		IP:            "203.0.113.7",
	}
	if err := m.RecordConsent(ctx, req); err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}

	ok, err := m.HasConsent(ctx, c.ID, customer.ConsentMarketingEmail)
	if err != nil {
		t.Fatalf("HasConsent: %v", err)
	}
	if !ok {
		t.Fatal("vừa đồng ý mà HasConsent trả false")
	}

	// Rút lại đồng ý.
	clock.advance(24 * time.Hour)
	req.Granted = false
	req.Source = "settings"
	if err := m.RecordConsent(ctx, req); err != nil {
		t.Fatalf("RecordConsent lần 2: %v", err)
	}

	ok, err = m.HasConsent(ctx, c.ID, customer.ConsentMarketingEmail)
	if err != nil {
		t.Fatalf("HasConsent: %v", err)
	}
	if ok {
		t.Fatal("đã rút lại mà HasConsent vẫn trả true")
	}

	// HAI hàng, không phải một hàng bị sửa.
	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM customer_consent WHERE customer_id = $1`,
		c.ID).Scan(&rows); err != nil {
		t.Fatalf("đếm bản ghi đồng ý: %v", err)
	}
	if rows != 2 {
		t.Fatalf("có %d bản ghi, mong 2 — nhật ký bị SỬA thay vì ghi thêm", rows)
	}

	history, err := m.GetConsentHistory(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetConsentHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("lịch sử có %d bản ghi, mong 2", len(history))
	}
	// Mới nhất trước.
	if history[0].Granted || !history[1].Granted {
		t.Fatalf("thứ tự lịch sử sai: %+v", history)
	}
	if history[0].Source != "settings" || history[1].Source != "signup_form" {
		t.Fatalf("nguồn đồng ý không được ghi đúng: %+v", history)
	}
}

// KHÔNG CÓ BẢN GHI NGHĨA LÀ CHƯA ĐỒNG Ý.
//
// Suy diễn ngược lại là gửi thư quảng cáo cho người chưa bao giờ bấm đồng
// ý — vi phạm pháp luật ở nhiều thị trường.
func TestVangMatNghiaLaChuaDongY(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	for _, kind := range []string{
		customer.ConsentMarketingEmail,
		customer.ConsentMarketingSMS,
		customer.ConsentPersonalization,
	} {
		ok, err := m.HasConsent(ctx, c.ID, kind)
		if err != nil {
			t.Fatalf("HasConsent(%s): %v", kind, err)
		}
		if ok {
			t.Fatalf("chưa có bản ghi nào mà HasConsent(%s) trả true", kind)
		}
	}
}

// ĐỒNG Ý PHẢI CÓ NGUỒN.
//
// "Khách đã đồng ý" mà không nói được ở đâu thì không dùng được làm bằng
// chứng.
func TestDongYPhaiCoNguon(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	err := m.RecordConsent(ctx, customer.ConsentRequest{
		CustomerID: c.ID,
		Type:       customer.ConsentMarketingEmail,
		Granted:    true,
		Source:     "   ",
	})
	if !errors.Is(err, customer.ErrInvalidInput) {
		t.Fatalf("nguồn rỗng = %v, mong ErrInvalidInput", err)
	}

	err = m.RecordConsent(ctx, customer.ConsentRequest{
		CustomerID: c.ID,
		Type:       "KHONG_TON_TAI",
		Granted:    true,
		Source:     "settings",
	})
	if !errors.Is(err, customer.ErrInvalidInput) {
		t.Fatalf("loại đồng ý lạ = %v, mong ErrInvalidInput", err)
	}
}

// KHÔNG LƯU IP NGUYÊN VĂN trong nhật ký đồng ý.
func TestKhongLuuIPNguyenVan(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")
	if err := m.RecordConsent(ctx, customer.ConsentRequest{
		CustomerID: c.ID,
		Type:       customer.ConsentDataProcessing,
		Granted:    true,
		Source:     "checkout",
		IP:         "203.0.113.7",
	}); err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}

	var ipHash string
	if err := pool.QueryRow(ctx,
		`SELECT ip_hash FROM customer_consent WHERE customer_id = $1`,
		c.ID).Scan(&ipHash); err != nil {
		t.Fatalf("đọc đồng ý: %v", err)
	}
	if ipHash == "203.0.113.7" {
		t.Fatal("lưu IP NGUYÊN VĂN")
	}
	if len(ipHash) != 64 {
		t.Fatalf("ip_hash dài %d, mong 64 (SHA-256 hex)", len(ipHash))
	}
}

// THÊM MÓN YÊU THÍCH LÀ IDEMPOTENT.
//
// Khách bấm tim hai lần là chuyện thường. Hiện hai lần trong danh sách là
// lỗi, và báo lỗi đỏ cho một thao tác vô hại cũng vậy.
func TestThemYeuThichIdempotent(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")
	productID := ids.MustNew(ids.PrefixProduct).String()

	req := customer.WishlistRequest{CustomerID: c.ID, ProductID: productID}

	added, err := m.AddToWishlist(ctx, req)
	if err != nil {
		t.Fatalf("AddToWishlist: %v", err)
	}
	if !added {
		t.Fatal("lần thêm đầu tiên trả false")
	}

	for i := 0; i < 3; i++ {
		added, err = m.AddToWishlist(ctx, req)
		if err != nil {
			t.Fatalf("AddToWishlist lần %d: %v", i+2, err)
		}
		if added {
			t.Fatalf("lần thêm thứ %d báo THÊM MỚI — có bản sao", i+2)
		}
	}

	w, err := m.GetWishlist(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetWishlist: %v", err)
	}
	if len(w.Items) != 1 {
		t.Fatalf("danh sách có %d món, mong 1", len(w.Items))
	}
}

// ĐUA NHAU THÊM CÙNG MỘT MÓN: chỉ MỘT lần được tính là thêm mới.
//
// Kiểm tra "đã có chưa" ở tầng ứng dụng KHÔNG cứu được — mười request cùng
// đọc thấy chưa có. Khóa chính (wishlist, product, variant) là thứ chặn.
func TestDuaNhauThemYeuThich(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")
	productID := ids.MustNew(ids.PrefixProduct).String()

	const n = 10
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		addedN int
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			added, err := m.AddToWishlist(ctx, customer.WishlistRequest{
				CustomerID: c.ID,
				ProductID:  productID,
			})
			if err == nil && added {
				mu.Lock()
				addedN++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if addedN != 1 {
		t.Fatalf("%d/%d request báo thêm mới, mong ĐÚNG 1", addedN, n)
	}

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM wishlist_item WHERE product_id = $1`,
		productID).Scan(&rows); err != nil {
		t.Fatalf("đếm món yêu thích: %v", err)
	}
	if rows != 1 {
		t.Fatalf("có %d dòng cho cùng một món, mong 1", rows)
	}

	// Và cũng chỉ có MỘT danh sách, dù mười request cùng tạo.
	var lists int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM wishlist WHERE customer_id = $1`,
		c.ID).Scan(&lists); err != nil {
		t.Fatalf("đếm danh sách: %v", err)
	}
	if lists != 1 {
		t.Fatalf("có %d danh sách yêu thích, mong 1", lists)
	}
}

// BỎ MÓN YÊU THÍCH LÀ IDEMPOTENT.
func TestBoYeuThichIdempotent(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")
	req := customer.WishlistRequest{
		CustomerID: c.ID,
		ProductID:  ids.MustNew(ids.PrefixProduct).String(),
	}

	if _, err := m.AddToWishlist(ctx, req); err != nil {
		t.Fatalf("AddToWishlist: %v", err)
	}

	removed, err := m.RemoveFromWishlist(ctx, req)
	if err != nil {
		t.Fatalf("RemoveFromWishlist: %v", err)
	}
	if !removed {
		t.Fatal("lần bỏ đầu tiên trả false")
	}

	// Bỏ lại: thành công, không báo lỗi.
	removed, err = m.RemoveFromWishlist(ctx, req)
	if err != nil {
		t.Fatalf("RemoveFromWishlist lần 2: %v", err)
	}
	if removed {
		t.Fatal("bỏ món không còn trong danh sách mà báo đã bỏ")
	}

	// Khách chưa có danh sách nào cũng không báo lỗi.
	other := create(t, m, "khac@example.com")
	if _, err := m.RemoveFromWishlist(ctx, customer.WishlistRequest{
		CustomerID: other.ID,
		ProductID:  req.ProductID,
	}); err != nil {
		t.Fatalf("bỏ món khi chưa có danh sách: %v", err)
	}
}

// ĐẾM YÊU THÍCH TÍNH THEO NGƯỜI, KHÔNG THEO DÒNG.
//
// Một khách thích cả size M lẫn size L là HAI dòng nhưng MỘT người quan
// tâm. Đếm nhầm sẽ thổi phồng tín hiệu nhu cầu của sản phẩm nhiều biến thể
// — và kế hoạch sản xuất dựa vào con số đó.
func TestDemYeuThichTheoNguoi(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	productID := ids.MustNew(ids.PrefixProduct).String()

	// Khách A thích ba biến thể của CÙNG sản phẩm.
	a := create(t, m, "khach-a@example.com")
	for _, variant := range []string{
		ids.MustNew(ids.PrefixVariant).String(),
		ids.MustNew(ids.PrefixVariant).String(),
		ids.MustNew(ids.PrefixVariant).String(),
	} {
		if _, err := m.AddToWishlist(ctx, customer.WishlistRequest{
			CustomerID: a.ID,
			ProductID:  productID,
			VariantID:  variant,
		}); err != nil {
			t.Fatalf("AddToWishlist: %v", err)
		}
	}

	// Khách B thích một biến thể.
	b := create(t, m, "khach-b@example.com")
	if _, err := m.AddToWishlist(ctx, customer.WishlistRequest{
		CustomerID: b.ID,
		ProductID:  productID,
	}); err != nil {
		t.Fatalf("AddToWishlist: %v", err)
	}

	n, err := m.CountWishlistForProduct(ctx, productID)
	if err != nil {
		t.Fatalf("CountWishlistForProduct: %v", err)
	}
	if n != 2 {
		t.Fatalf("đếm được %d, mong 2 NGƯỜI (A thích 3 biến thể vẫn là 1 người)", n)
	}
}

// WISHLIST CỦA KHÁCH CHƯA THÍCH GÌ LÀ DANH SÁCH RỖNG, KHÔNG PHẢI LỖI.
func TestWishlistRongKhongPhaiLoi(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	w, err := m.GetWishlist(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetWishlist: %v", err)
	}
	if len(w.Items) != 0 {
		t.Fatalf("danh sách mới có %d món", len(w.Items))
	}
}

// GỘP DANH TÍNH BẮT BUỘC XÁC MINH EMAIL.
//
// Không xác minh thì bất kỳ ai đăng ký bằng email người khác đều đọc được
// toàn bộ lịch sử mua hàng của họ — kể cả địa chỉ nhà.
func TestGopDanhTinhBatBuocXacMinh(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	guest := create(t, m, "vanglai@example.com")
	target := create(t, m, "dangky@example.com")

	err := m.MergeGuestIdentity(ctx, customer.MergeRequest{
		SourceCustomerID: guest.ID,
		TargetCustomerID: target.ID,
		EmailVerified:    false,
	})
	if !errors.Is(err, customer.ErrMergeNotVerified) {
		t.Fatalf("gộp chưa xác minh = %v, mong ErrMergeNotVerified", err)
	}

	// Hồ sơ nguồn phải còn NGUYÊN VẸN.
	got, err := m.GetCustomer(ctx, guest.ID)
	if err != nil {
		t.Fatalf("GetCustomer: %v", err)
	}
	if got.Status == customer.StatusAnonymized {
		t.Fatal("hồ sơ bị ẩn danh dù việc gộp đã bị từ chối")
	}

	// Không ghi nhật ký gộp nào.
	var logs int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM customer_merge_log`).Scan(&logs); err != nil {
		t.Fatalf("đếm nhật ký gộp: %v", err)
	}
	if logs != 0 {
		t.Fatalf("ghi %d nhật ký gộp dù đã từ chối", logs)
	}
}

// GỘP ĐÃ XÁC MINH: ghi nhật ký và ẩn danh hồ sơ nguồn.
func TestGopDanhTinhDaXacMinh(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	guest := create(t, m, "vanglai@example.com")
	target := create(t, m, "dangky@example.com")

	if err := m.MergeGuestIdentity(ctx, customer.MergeRequest{
		SourceCustomerID: guest.ID,
		TargetCustomerID: target.ID,
		EmailVerified:    true,
	}); err != nil {
		t.Fatalf("MergeGuestIdentity: %v", err)
	}

	got, err := m.GetCustomer(ctx, guest.ID)
	if err != nil {
		t.Fatalf("GetCustomer: %v", err)
	}
	if got.Status != customer.StatusAnonymized {
		t.Fatalf("hồ sơ nguồn = %q, mong ANONYMIZED", got.Status)
	}

	var reason string
	if err := pool.QueryRow(ctx, `
		SELECT reason FROM customer_merge_log
		 WHERE source_customer_id = $1 AND target_customer_id = $2`,
		guest.ID, target.ID).Scan(&reason); err != nil {
		t.Fatalf("đọc nhật ký gộp: %v", err)
	}
	if reason == "" {
		t.Fatal("nhật ký gộp không ghi căn cứ")
	}

	// Gộp hồ sơ vào CHÍNH NÓ bị từ chối.
	if err := m.MergeGuestIdentity(ctx, customer.MergeRequest{
		SourceCustomerID: target.ID,
		TargetCustomerID: target.ID,
		EmailVerified:    true,
	}); !errors.Is(err, customer.ErrInvalidInput) {
		t.Fatalf("gộp vào chính nó = %v, mong ErrInvalidInput", err)
	}
}

// ẨN DANH GIỮ DỮ LIỆU GIAO DỊCH, XÓA DỮ LIỆU ĐỊNH DANH.
//
// Đơn hàng đã dùng để tính hoa hồng trả cho seller. Xóa nó đi thì đối soát
// tài chính không còn khớp, và nghĩa vụ lưu trữ chứng từ kế toán bị vi
// phạm.
func TestAnDanhGiuDuLieuGiaoDich(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")
	if _, err := m.AddAddress(ctx, addrReq(c.ID, "12 Nguyễn Huệ")); err != nil {
		t.Fatalf("AddAddress: %v", err)
	}

	// Giả lập khách đã mua hàng.
	if _, err := pool.Exec(ctx, `
		UPDATE customer SET order_count = 3, total_spent = 1500000 WHERE id = $1`,
		c.ID); err != nil {
		t.Fatalf("giả lập lịch sử mua: %v", err)
	}

	if err := m.Anonymize(ctx, c.ID); err != nil {
		t.Fatalf("Anonymize: %v", err)
	}

	got, err := m.GetCustomer(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCustomer: %v", err)
	}

	// XÓA: dữ liệu định danh.
	if got.DisplayName != "" || got.Phone != "" {
		t.Fatalf("dữ liệu định danh còn sót: tên=%q điện thoại=%q",
			got.DisplayName, got.Phone)
	}
	if got.Email == "khach@example.com" {
		t.Fatal("email gốc còn nguyên sau khi ẩn danh")
	}

	// GIỮ: dữ liệu giao dịch.
	if got.OrderCount != 3 || got.TotalSpent != 1500000 {
		t.Fatalf("dữ liệu giao dịch bị mất: %d đơn, %d tiền",
			got.OrderCount, got.TotalSpent)
	}

	// Sổ địa chỉ cũng phải sạch — nó chứa tên, số điện thoại, địa chỉ nhà.
	list, err := m.GetAddresses(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetAddresses: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("còn %d địa chỉ sau khi ẩn danh", len(list))
	}

	// Hồ sơ đã ẩn danh KHÔNG sửa được nữa.
	if _, err := m.UpdateProfile(ctx, c.ID, "Tên mới", "0999"); !errors.Is(err, customer.ErrAnonymized) {
		t.Fatalf("sửa hồ sơ đã ẩn danh = %v, mong ErrAnonymized", err)
	}
}

// HAI KHÁCH CÙNG ẨN DANH KHÔNG ĐỤNG NHAU.
//
// Nếu email giả giống nhau, người thứ hai sẽ đụng ràng buộc UNIQUE và
// KHÔNG ẩn danh được — tức là yêu cầu xóa dữ liệu của họ thất bại.
func TestHaiKhachCungAnDanh(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	a := create(t, m, "khach-a@example.com")
	b := create(t, m, "khach-b@example.com")

	if err := m.Anonymize(ctx, a.ID); err != nil {
		t.Fatalf("ẩn danh A: %v", err)
	}
	if err := m.Anonymize(ctx, b.ID); err != nil {
		t.Fatalf("ẩn danh B: %v — email giả bị trùng?", err)
	}
}

// EMAIL CỦA HỒ SƠ ĐÃ ẨN DANH DÙNG LẠI ĐƯỢC.
//
// Khách xóa tài khoản rồi đăng ký lại bằng chính email đó là chuyện bình
// thường. Chặn họ nghĩa là email đó mất vĩnh viễn.
func TestEmailSauKhiAnDanhDungLaiDuoc(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	old := create(t, m, "khach@example.com")
	if err := m.Anonymize(ctx, old.ID); err != nil {
		t.Fatalf("Anonymize: %v", err)
	}

	fresh, err := m.Create(ctx, customer.CreateRequest{Email: "khach@example.com"})
	if err != nil {
		t.Fatalf("đăng ký lại bằng email cũ: %v", err)
	}
	if fresh.ID == old.ID {
		t.Fatal("dùng lại hồ sơ cũ thay vì tạo hồ sơ mới")
	}

	// TRA THEO EMAIL PHẢI RA HỒ SƠ MỚI, KHÔNG PHẢI HỒ SƠ ĐÃ ẨN DANH.
	//
	// Đây là chỗ nguy hiểm nhất của việc ẩn danh: hồ sơ cũ vẫn nằm trong
	// bảng. Nếu truy vấn không loại nó, khách quay lại sẽ được gắn vào
	// chính hồ sơ mà họ vừa yêu cầu xóa — cùng với lịch sử mua hàng đáng
	// lẽ đã biến mất khỏi tầm mắt họ.
	//
	// Kiểm chứng qua EnsureByEmail vì đó là đường khách vãng lai thật sự
	// đi qua khi thanh toán.
	got, err := m.EnsureByEmail(ctx, customer.CreateRequest{Email: "khach@example.com"})
	if err != nil {
		t.Fatalf("EnsureByEmail: %v", err)
	}
	if got.ID == old.ID {
		t.Fatal("tra email ra hồ sơ ĐÃ ẨN DANH — khách được gắn lại " +
			"vào chính hồ sơ họ vừa yêu cầu xóa")
	}
	if got.ID != fresh.ID {
		t.Fatalf("tra email ra %s, mong hồ sơ mới %s", got.ID, fresh.ID)
	}
	if got.Status == customer.StatusAnonymized {
		t.Fatal("hồ sơ trả về là hồ sơ đã ẩn danh")
	}
}

// LỚP THỨ HAI: hồ sơ ẩn danh KHÔNG hiện ra khi tra theo email.
//
// Test trên (TestEmailSauKhiAnDanhDungLaiDuoc) đi qua hai lớp cùng lúc:
// Anonymize thay email bằng chuỗi giả, VÀ truy vấn loại status
// ANONYMIZED. Đã kiểm chứng ngược — bỏ lớp thứ hai, test đó vẫn xanh, vì
// lớp thứ nhất đã đủ.
//
// Test này cô lập lớp thứ hai: ẩn danh THẲNG TRONG DATABASE mà GIỮ NGUYÊN
// email, đúng như một đường ẩn danh viết thiếu sẽ để lại. Nếu truy vấn
// không loại hồ sơ đó, khách quay lại sẽ được gắn vào chính hồ sơ họ vừa
// yêu cầu xóa — cùng toàn bộ lịch sử mua hàng.
func TestHoSoAnDanhKhongHienKhiTraEmail(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	old := create(t, m, "khach@example.com")

	// Ẩn danh KHÔNG đổi email — mô phỏng đường ẩn danh viết thiếu.
	if _, err := pool.Exec(ctx,
		`UPDATE customer SET status = 'ANONYMIZED' WHERE id = $1`, old.ID); err != nil {
		t.Fatalf("ẩn danh thủ công: %v", err)
	}

	got, err := m.EnsureByEmail(ctx, customer.CreateRequest{Email: "khach@example.com"})
	if err != nil {
		t.Fatalf("EnsureByEmail: %v", err)
	}
	if got.ID == old.ID {
		t.Fatal("tra email ra hồ sơ ĐÃ ẨN DANH — khách được gắn lại " +
			"vào chính hồ sơ họ vừa yêu cầu xóa")
	}
	if got.Status == customer.StatusAnonymized {
		t.Fatalf("hồ sơ trả về có trạng thái %q", got.Status)
	}
}

// ĐỌC NHIỀU HỒ SƠ TRONG MỘT TRUY VẤN.
//
// Id không tồn tại phải VẮNG MẶT trong map, không phải giá trị rỗng — bên
// gọi cần phân biệt "khách này đã bị xóa" với "khách này tên rỗng".
func TestDocNhieuHoSo(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	a := create(t, m, "khach-a@example.com")
	b := create(t, m, "khach-b@example.com")
	missing := ids.MustNew(ids.PrefixCustomer).String()

	got, err := m.GetCustomersByIDs(ctx, []string{a.ID, b.ID, missing})
	if err != nil {
		t.Fatalf("GetCustomersByIDs: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("trả %d hồ sơ, mong 2", len(got))
	}
	if _, ok := got[missing]; ok {
		t.Fatal("id không tồn tại lại có trong kết quả")
	}
	if got[a.ID].Email != "khach-a@example.com" {
		t.Fatalf("hồ sơ A sai: %+v", got[a.ID])
	}

	// Danh sách rỗng trả map rỗng, không phải lỗi.
	empty, err := m.GetCustomersByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("GetCustomersByIDs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("danh sách rỗng trả %d hồ sơ", len(empty))
	}
}

// KHÓA LẠC QUAN CHẶN MẤT CẬP NHẬT.
//
// Hai request cùng đọc phiên bản 1 rồi cùng ghi: request thứ hai phải bị
// từ chối, không được âm thầm ghi đè thay đổi của request đầu.
func TestKhoaLacQuanChanMatCapNhat(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	c := create(t, m, "khach@example.com")

	const n = 8
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		okN    int
		conflN int
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := m.UpdateProfile(ctx, c.ID, "Tên", "090000000")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				okN++
			case errors.Is(err, customer.ErrVersionConflict):
				conflN++
			}
		}(i)
	}
	wg.Wait()

	if okN+conflN != n {
		t.Fatalf("%d thành công + %d xung đột = %d, mong %d — có lỗi khác",
			okN, conflN, okN+conflN, n)
	}
	if conflN == 0 {
		t.Fatal("KHÔNG có xung đột nào trong 8 request song song — " +
			"khóa lạc quan không hoạt động")
	}

	// version phải bằng ĐÚNG số lần ghi thành công cộng 1 (lúc tạo).
	var version int
	if err := pool.QueryRow(ctx,
		`SELECT version FROM customer WHERE id = $1`, c.ID).Scan(&version); err != nil {
		t.Fatalf("đọc phiên bản: %v", err)
	}
	if version != okN+1 {
		t.Fatalf("phiên bản = %d, mong %d (%d lần ghi + 1 lúc tạo)",
			version, okN+1, okN)
	}
}
