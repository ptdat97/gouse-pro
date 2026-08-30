package httpserver

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// RateLimit giới hạn số request theo địa chỉ IP.
//
// # Vì sao đường ĐĂNG KÝ bắt buộc phải có
//
// `identity.Register` CỐ Ý trả "email đã được dùng" thay vì giấu — người
// dùng thật cần biết vì sao không đăng ký được. Cái giá là endpoint đó trả
// lời được câu "email này có tài khoản chưa", nên không giới hạn tần suất
// thì nó thành công cụ dò danh sách email.
//
// identity/public.go ghi rõ ràng buộc này: "đường đăng ký phải có giới hạn
// tần suất ở tầng interfaces".
//
// # Vì sao đếm trong BỘ NHỚ, và hệ quả phải chấp nhận
//
// Bộ đếm không chia sẻ giữa các tiến trình: chạy ba bản sao thì kẻ tấn công
// được gấp ba lượt. Đó là giới hạn ĐÃ BIẾT, không phải sơ suất — bộ đếm dùng
// chung cần Redis, và thêm một phụ thuộc hạ tầng cho MVP là cái giá lớn hơn
// lợi ích.
//
// Nó vẫn chặn được thứ cần chặn nhất: dò hàng nghìn email từ MỘT máy.
//
// # KHÔNG dùng cho đường mua hàng
//
// Giỏ hàng và thanh toán nhiều request là chuyện bình thường. Giới hạn ở đó
// là chặn khách thật vào lúc họ đang tiêu tiền.
func RateLimit(limit int, window time.Duration) Middleware {
	l := &limiter{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Preflight KHÔNG tính: nó là câu hỏi của trình duyệt, không
			// phải hành động của người dùng. Tính nó vào thì mỗi thao tác
			// thật tốn hai lượt.
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			if !l.allow(clientIP(r), time.Now()) {
				logger.FromContext(r.Context()).Warn("chặn vì vượt giới hạn tần suất",
					"path", r.URL.Path)

				// Retry-After để client biết chờ bao lâu thay vì thử lại
				// ngay và tiếp tục bị chặn.
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				apierror.Write(w, r,
					apierror.New(apierror.CodeRateLimitExceeded,
						"Bạn thao tác quá nhanh, vui lòng thử lại sau"),
					logger.RequestIDFromContext(r.Context()),
					logger.FromContext(r.Context()))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitThatBai giới hạn theo IP nhưng CHỈ đếm những lượt THẤT BẠI.
//
// # Vì sao đăng nhập không dùng được RateLimit thường
//
// Đếm mọi lượt đăng nhập thì một địa chỉ IP là một hạn mức — mà văn phòng,
// trường học và mạng di động đều ra Internet bằng MỘT địa chỉ NAT dùng
// chung. Hạn mức 5 lượt / 10 phút áp lên cả tòa nhà nghĩa là người thứ sáu
// đăng nhập đúng mật khẩu vẫn bị chặn.
//
// Đó là kiểu hỏng tệ: biện pháp bảo vệ hoạt động đúng như viết, kẻ tấn công
// vẫn bị chặn, nhưng thiệt hại đổ lên người dùng thật và chỉ lộ ra khi
// khách đã bỏ đi.
//
// Đếm riêng lượt THẤT BẠI giữ nguyên tác dụng chặn dò mật khẩu — kẻ dò thì
// gần như lượt nào cũng sai — trong khi người đăng nhập đúng không bao giờ
// chạm hạn mức dù ngồi cùng mạng với bao nhiêu người.
//
// # Nó bổ sung cho khóa tài khoản, không thay thế
//
// Khóa theo TÀI KHOẢN (identity.MaxFailedAttempts) chặn việc dò mật khẩu
// của một tài khoản. Nó không thấy được kiểu RẢI MẬT KHẨU: một mật khẩu
// phổ biến thử lên hàng nghìn email, mỗi email sai đúng một lần.
//
// Hai lớp nhìn cùng một chuỗi request từ hai phía — theo tài khoản và theo
// đường mạng — nên mỗi lớp bắt được thứ lớp kia mù.
//
// `laThatBai` quyết định lượt nào bị tính. Truyền vào thay vì cố định 401
// để bên gọi tự định nghĩa "thất bại" theo endpoint của mình.
func RateLimitThatBai(
	limit int, window time.Duration, laThatBai func(status int) bool,
) Middleware {
	l := &limiter{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			key := clientIP(r)
			if l.vuot(key, time.Now()) {
				logger.FromContext(r.Context()).Warn(
					"chặn vì quá nhiều lượt thất bại",
					"path", r.URL.Path)

				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				apierror.Write(w, r,
					apierror.New(apierror.CodeRateLimitExceeded,
						"Bạn thao tác quá nhanh, vui lòng thử lại sau"),
					logger.RequestIDFromContext(r.Context()),
					logger.FromContext(r.Context()))
				return
			}

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			if laThatBai(rec.status) {
				l.ghiNhan(key, time.Now())
			}
		})
	}
}

type limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

// sweepThreshold là số khóa tối đa trước khi quét dọn toàn bộ.
//
// Dọn theo từng khóa lúc kiểm tra là chưa đủ: khóa của một IP không quay
// lại thì KHÔNG BAO GIỜ được dọn, và map phình theo số IP đã từng gọi —
// rò rỉ chậm nhưng chắc chắn, chỉ lộ ra sau nhiều tuần chạy.
//
// Quét toàn bộ là O(n), nên chỉ chạy khi map đã lớn thay vì mỗi request.
const sweepThreshold = 1024

// allow ghi nhận một lượt và cho biết có được đi tiếp không.
//
// Dùng CỬA SỔ TRƯỢT chứ không phải cửa sổ cố định: với cửa sổ cố định, kẻ
// tấn công gửi đủ hạn mức ở cuối cửa sổ này rồi đủ hạn mức nữa ở đầu cửa sổ
// sau — gấp đôi hạn mức trong vài giây.
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)

	// conHieuLuc dọn luôn khóa không còn lượt nào: không dọn thì map phình
	// theo số IP đã từng gọi — rò rỉ bộ nhớ chậm nhưng chắc chắn.
	kept := l.conHieuLuc(key, cutoff)

	if len(l.hits) > sweepThreshold {
		l.sweep(cutoff)
	}

	if len(kept) >= l.limit {
		return false
	}

	l.hits[key] = append(l.hits[key], now)
	return true
}

// vuot cho biết khóa ĐÃ chạm hạn mức, KHÔNG ghi nhận thêm lượt nào.
//
// Tách khỏi `allow` vì `RateLimitThatBai` chỉ biết lượt này có tính hay
// không SAU khi handler chạy xong — lúc kiểm thì chưa được ghi.
func (l *limiter) vuot(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.conHieuLuc(key, now.Add(-l.window))) >= l.limit
}

// ghiNhan ghi một lượt vào bộ đếm.
func (l *limiter) ghiNhan(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	l.conHieuLuc(key, cutoff)
	if len(l.hits) > sweepThreshold {
		l.sweep(cutoff)
	}
	l.hits[key] = append(l.hits[key], now)
}

// conHieuLuc lọc bỏ những lượt đã ra ngoài cửa sổ và ghi lại kết quả.
//
// Bên gọi PHẢI đang giữ khóa mu.
func (l *limiter) conHieuLuc(key string, cutoff time.Time) []time.Time {
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, key)
	} else {
		l.hits[key] = kept
	}
	return kept
}

// sweep bỏ mọi khóa không còn lượt nào trong cửa sổ.
//
// Bên gọi PHẢI đang giữ khóa mu.
func (l *limiter) sweep(cutoff time.Time) {
	for k, times := range l.hits {
		alive := false
		for _, t := range times {
			if t.After(cutoff) {
				alive = true
				break
			}
		}
		if !alive {
			delete(l.hits, k)
		}
	}
}

// clientIP lấy địa chỉ người gọi.
//
// TIN VÀO X-Forwarded-For chỉ đúng khi có proxy tin cậy đứng trước và proxy
// đó GHI ĐÈ header. Không có proxy thì bất kỳ ai cũng tự đặt header này và
// vượt qua giới hạn bằng cách đổi giá trị mỗi request.
//
// Đây là giới hạn ĐÃ BIẾT của cách làm hiện tại. Khi triển khai thật, proxy
// phải được cấu hình ghi đè header — xem docs/09-operations.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return strings.TrimSpace(rip)
	}

	// CẮT BỎ CỔNG. `RemoteAddr` là "127.0.0.1:54321", và cổng nguồn ĐỔI
	// theo từng kết nối TCP — giữ nguyên thì mỗi request là một khóa khác
	// và bộ đếm không bao giờ chạm hạn mức.
	//
	// Đây là kiểu hỏng tệ nhất: biện pháp bảo vệ trông như đang chạy, log
	// sạch, test bằng một khóa cố định vẫn xanh — chỉ có tác dụng thật là
	// bằng không.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
