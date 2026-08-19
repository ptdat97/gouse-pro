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

	if !l.allow("ip", base) || !l.allow("ip", base) {
		t.Fatal("hai lượt đầu phải được phép")
	}
	if l.allow("ip", base.Add(30*time.Second)) {
		t.Error("lượt thứ ba trong cửa sổ phải bị chặn")
	}
	if !l.allow("ip", base.Add(61*time.Second)) {
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
