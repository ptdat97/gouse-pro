package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CORS cho phép trình duyệt ở origin KHÁC gọi API.
//
// Cần thiết vì giao diện quản trị chạy ở tiến trình riêng (Next.js, cổng
// 3000) còn API ở cổng 8080 — với trình duyệt đó là hai origin khác nhau,
// và mặc định mọi request bị chặn.
//
// # Danh sách trắng, KHÔNG dùng `*`
//
// Response có `Access-Control-Allow-Credentials: true` vì refresh token nằm
// ở cookie. Chuẩn CORS CẤM kết hợp `*` với credentials, và điều đó là đúng:
// cho phép mọi origin gửi kèm cookie nghĩa là bất kỳ trang web nào cũng gọi
// được API dưới danh nghĩa người dùng đang đăng nhập.
//
// Vì thế origin được ĐỐI CHIẾU với danh sách cấu hình, rồi mới phản hồi lại
// đúng origin đó.
//
// Xem docs/09-operations/security.md mục "CORS chặt — chỉ domain của mình".
func CORS(allowedOrigins []string) Middleware {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Không có Origin nghĩa là request không phải từ trình duyệt
			// (curl, dịch vụ khác). CORS không áp dụng.
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// LUÔN đặt Vary, kể cả khi origin không được phép: thiếu nó,
			// một proxy có thể trả cho origin A cái response đã cache cho
			// origin B — và khi đó danh sách trắng thành vô nghĩa.
			w.Header().Add("Vary", "Origin")

			if _, ok := allowed[origin]; !ok {
				// KHÔNG đặt header CORS. Trình duyệt sẽ chặn, và đó đúng là
				// điều cần xảy ra. Vẫn xử lý request bình thường cho client
				// không phải trình duyệt.
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions &&
				r.Header.Get("Access-Control-Request-Method") != "" {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				h.Set("Access-Control-Allow-Methods",
					"GET, POST, PATCH, PUT, DELETE, OPTIONS")
				// Danh sách này phải khớp MỌI header giao diện gửi lên.
				// Thiếu một cái thì trình duyệt chặn request, và lỗi chỉ
				// hiện ở console trình duyệt — log máy chủ hoàn toàn sạch,
				// nên rất dễ đi tìm nhầm chỗ.
				//
				// X-Guest-Phone: khách vãng lai tra đơn bằng mã đơn kèm số
				// điện thoại (orders.yaml, operationId getOrder).
				h.Set("Access-Control-Allow-Headers",
					"Authorization, Content-Type, Idempotency-Key, "+
						"X-Request-ID, Accept-Language, X-Guest-Phone")
				h.Set("Access-Control-Max-Age",
					strconv.Itoa(int(preflightMaxAge.Seconds())))

				// Preflight KHÔNG đi tiếp tới handler: nó chỉ hỏi "tôi có
				// được gửi request thật không", không phải là request thật.
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Client đọc được request id để báo khi cần hỗ trợ.
			h.Set("Access-Control-Expose-Headers",
				"X-Request-ID, X-RateLimit-Limit, X-RateLimit-Remaining, "+
					"X-RateLimit-Reset, Retry-After")

			next.ServeHTTP(w, r)
		})
	}
}

// preflightMaxAge là thời gian trình duyệt nhớ kết quả preflight.
//
// Mười phút: đủ để không preflight lại ở mỗi request, đủ ngắn để đổi danh
// sách origin có hiệu lực nhanh khi cần khóa một domain.
const preflightMaxAge = 10 * time.Minute
