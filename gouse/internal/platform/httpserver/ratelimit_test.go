package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func limited(limit int, window time.Duration) http.Handler {
	return RateLimit(limit, window)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	))
}

func call(h http.Handler, ip string) int {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	r.Header.Set("X-Real-IP", ip)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

// VƯỢT HẠN MỨC thì bị chặn — đây là thứ chặn việc dò danh sách email.
//
// `identity.Register` CỐ Ý trả "email đã được dùng", nên endpoint đó trả
// lời được "email này có tài khoản chưa". Không giới hạn tần suất thì nó
// thành công cụ quét hàng nghìn email.
func TestVuotHanMucBiChan(t *testing.T) {
	h := limited(3, time.Minute)

	for i := 1; i <= 3; i++ {
		if code := call(h, "1.2.3.4"); code != http.StatusOK {
			t.Fatalf("lượt %d bị chặn sớm: %d", i, code)
		}
	}

	if code := call(h, "1.2.3.4"); code != http.StatusTooManyRequests {
		t.Errorf("lượt thứ 4: mã = %d, muốn 429", code)
	}
}

// GIỚI HẠN THEO IP, không phải toàn cục.
//
// Đếm chung thì một kẻ tấn công làm cả nền tảng không đăng ký được — biến
// biện pháp bảo vệ thành công cụ từ chối dịch vụ.
func TestGioiHanTheoTungIP(t *testing.T) {
	h := limited(2, time.Minute)

	call(h, "1.1.1.1")
	call(h, "1.1.1.1")
	if code := call(h, "1.1.1.1"); code != http.StatusTooManyRequests {
		t.Fatalf("IP thứ nhất phải bị chặn, nhận %d", code)
	}

	if code := call(h, "2.2.2.2"); code != http.StatusOK {
		t.Errorf("IP khác bị chặn oan: %d", code)
	}
}

// CỬA SỔ TRƯỢT: hết cửa sổ thì lượt cũ không còn tính.
func TestHetCuaSoThiDuocGoiLai(t *testing.T) {
	l := &limiter{limit: 2, window: time.Minute, hits: map[string][]time.Time{}}
	base := time.Now()

	duoc := func(t time.Time) bool { ok, _ := l.allow("ip", t); return ok }

	if !duoc(base) || !duoc(base) {
		t.Fatal("hai lượt đầu phải được phép")
	}
	if duoc(base.Add(30 * time.Second)) {
		t.Error("lượt thứ ba trong cửa sổ phải bị chặn")
	}
	if !duoc(base.Add(61 * time.Second)) {
		t.Error("qua cửa sổ rồi phải được phép lại")
	}
}

// KHÔNG rò rỉ bộ nhớ: khóa của IP không quay lại phải được quét dọn.
//
// Dọn theo từng khóa lúc kiểm tra là chưa đủ — khóa của IP một-đi-không-trở
// -lại sẽ nằm lại vĩnh viễn, và map phình theo số IP đã từng gọi. Rò rỉ
// chậm nhưng chắc chắn, chỉ lộ ra sau nhiều tuần chạy.
func TestQuetDonKhoaHetHan(t *testing.T) {
	l := &limiter{limit: 5, window: time.Minute, hits: map[string][]time.Time{}}
	base := time.Now()

	// Nhiều IP gọi một lần rồi biến mất.
	for i := 0; i < sweepThreshold+10; i++ {
		l.allow("ip-"+strconv.Itoa(i), base)
	}
	if len(l.hits) < sweepThreshold {
		t.Fatalf("mong map đã lớn, nhận %d khóa", len(l.hits))
	}

	// Một request MUỘN hơn cửa sổ: mọi khóa cũ phải bị dọn.
	l.allow("moi", base.Add(2*time.Minute))

	if len(l.hits) != 1 {
		t.Errorf("sau khi quét còn %d khóa, mong 1 — khóa hết hạn không "+
			"được dọn và bộ nhớ sẽ phình mãi", len(l.hits))
	}
}

// PREFLIGHT không tính vào hạn mức.
//
// Nó là câu hỏi của trình duyệt, không phải hành động của người dùng. Tính
// nó vào thì mỗi thao tác thật tốn hai lượt và hạn mức thực tế giảm nửa.
func TestPreflightKhongTinhVaoHanMuc(t *testing.T) {
	h := limited(2, time.Minute)

	for i := 0; i < 5; i++ {
		r := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/register", nil)
		r.Header.Set("X-Real-IP", "3.3.3.3")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("preflight lượt %d bị tính vào hạn mức", i+1)
		}
	}

	if code := call(h, "3.3.3.3"); code != http.StatusOK {
		t.Errorf("request thật sau preflight bị chặn: %d", code)
	}
}

// ĐỊA CHỈ IP phải bỏ CỔNG trước khi dùng làm khóa.
//
// `RemoteAddr` là "127.0.0.1:54321" và cổng nguồn ĐỔI theo từng kết nối
// TCP. Giữ nguyên cả cổng thì mỗi request là một khóa khác và bộ đếm không
// bao giờ chạm hạn mức — biện pháp bảo vệ trông như đang chạy, log sạch,
// mà tác dụng thật bằng không.
//
// Lỗi này đã xảy ra thật: 7 lần đăng ký liên tiếp đều qua với hạn mức 5.
func TestKhoaTheoIPKhongKemCong(t *testing.T) {
	h := limited(2, time.Minute)

	// Cùng một máy, ba kết nối TCP khác nhau — cổng nguồn khác nhau.
	codes := make([]int, 0, 3)
	for i, port := range []string{"54321", "54322", "54323"} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		r.RemoteAddr = "203.0.113.7:" + port
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		codes = append(codes, w.Code)
		_ = i
	}

	if codes[2] != http.StatusTooManyRequests {
		t.Errorf("lượt thứ ba từ CÙNG một IP: mã = %d, muốn 429 — cổng nguồn "+
			"đang bị tính vào khóa, nên giới hạn tần suất vô tác dụng",
			codes[2])
	}
}

// tieuDe gọi một lượt và trả về bộ tiêu đề hạn mức.
func tieuDe(h http.Handler, ip string) (int, http.Header) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	r.Header.Set("X-Real-IP", ip)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code, w.Header()
}

// TestBaoHanMucConLaiTrenMOILuot.
//
// Bộ tiêu đề chỉ có ích khi nó xuất hiện TRƯỚC lúc bị chặn. Gắn riêng vào
// response 429 thì client không có gì để tự giãn nhịp — nó cứ gõ tới khi
// bị chặn, đúng hành vi mà giới hạn tần suất muốn tránh.
//
// CORS đã khai ba tiêu đề này trong `Access-Control-Expose-Headers` từ
// trước, nên trình duyệt vốn đã sẵn sàng đọc thứ chưa ai gửi.
func TestBaoHanMucConLaiTrenMoiLuot(t *testing.T) {
	h := limited(3, time.Minute)

	for i, mong := range []string{"2", "1", "0"} {
		code, head := tieuDe(h, "9.9.9.9")
		if code != http.StatusOK {
			t.Fatalf("lượt %d bị chặn sớm: %d", i+1, code)
		}
		if got := head.Get("X-RateLimit-Limit"); got != "3" {
			t.Errorf("lượt %d: X-RateLimit-Limit = %q, cần \"3\"", i+1, got)
		}
		if got := head.Get("X-RateLimit-Remaining"); got != mong {
			t.Errorf("lượt %d: X-RateLimit-Remaining = %q, cần %q — client "+
				"không thấy hạn mức cạn dần thì không có gì để tự giãn nhịp",
				i+1, got, mong)
		}
	}

	code, head := tieuDe(h, "9.9.9.9")
	if code != http.StatusTooManyRequests {
		t.Fatalf("lượt thứ 4: mã = %d, muốn 429", code)
	}
	if got := head.Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("lúc bị chặn X-RateLimit-Remaining = %q, cần \"0\"", got)
	}

	// Reset là Unix timestamp theo đặc tả, và phải ở TƯƠNG LAI.
	dat, err := strconv.ParseInt(head.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		t.Fatalf("X-RateLimit-Reset = %q, cần Unix timestamp: %v",
			head.Get("X-RateLimit-Reset"), err)
	}
	if !time.Unix(dat, 0).After(time.Now()) {
		t.Errorf("X-RateLimit-Reset ở quá khứ — client sẽ thử lại ngay")
	}
}

// TestChoDoiTheoLuotCuNhatChuKhongTronCuaSo.
//
// Cửa sổ TRƯỢT: chỗ trống đầu tiên xuất hiện khi lượt CŨ NHẤT rơi ra khỏi
// cửa sổ. Bảo client chờ trọn một cửa sổ là bắt họ đợi lâu hơn cần thiết —
// người bị chặn oan ngồi đợi hết phút trong khi chỗ đã trống từ lâu.
func TestChoDoiTheoLuotCuNhatChuKhongTronCuaSo(t *testing.T) {
	l := &limiter{limit: 2, window: time.Minute, hits: map[string][]time.Time{}}
	base := time.Now()

	l.allow("ip", base)
	l.allow("ip", base.Add(50*time.Second))

	// Bị chặn ở giây thứ 55: lượt cũ nhất (giây 0) hết hạn ở giây 60, nên
	// chỉ còn phải chờ 5 giây — không phải trọn 60.
	now := base.Add(55 * time.Second)
	vuot, datLai := l.vuot("ip", now)
	if !vuot {
		t.Fatal("phải bị chặn ở lượt thứ ba")
	}
	if got := giayCho(datLai, now); got != 5 {
		t.Errorf("Retry-After = %d giây, cần 5 — chờ trọn cửa sổ là bắt "+
			"người dùng đợi lâu hơn cần thiết", got)
	}
}

// TestBoDemThatBaiKhongNoiConMayLuot.
//
// Khác biệt CÓ CHỦ Ý so với RateLimit thường. Bộ đếm của đường đăng nhập
// đếm lượt THẤT BẠI; nói "còn 2 lượt nữa" là đưa cho kẻ rải mật khẩu đúng
// ngân sách để đi chậm mà không bao giờ chạm ngưỡng.
//
// Không nói thì nó phải tự dò, và dò nghĩa là ăn 429 — tức là lộ diện.
func TestBoDemThatBaiKhongNoiConMayLuot(t *testing.T) {
	hong := func(status int) bool { return status == http.StatusUnauthorized }
	h := RateLimitThatBai(2, time.Minute, hong)(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	var head http.Header
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		r.Header.Set("X-Real-IP", "8.8.8.8")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		head = w.Header()
	}

	for _, k := range []string{
		"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset",
	} {
		if got := head.Get(k); got != "" {
			t.Errorf("đường đăng nhập gửi %s = %q — nói thẳng ngân sách "+
				"còn lại cho kẻ rải mật khẩu", k, got)
		}
	}
	// Nhưng vẫn phải có Retry-After: người bị chặn oan cần biết chờ bao lâu.
	if head.Get("Retry-After") == "" {
		t.Error("thiếu Retry-After khi bị chặn")
	}
}
