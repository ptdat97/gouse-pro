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

	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	// Dọn hẳn khóa không còn lượt nào: không dọn thì map phình theo số IP
	// đã từng gọi, và đó là rò rỉ bộ nhớ chậm nhưng chắc chắn.
	if len(kept) == 0 {
		delete(l.hits, key)
	} else {
		l.hits[key] = kept
	}

	if len(l.hits) > sweepThreshold {
		l.sweep(cutoff)
	}

	if len(kept) >= l.limit {
		return false
	}

	l.hits[key] = append(l.hits[key], now)
	return true
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
