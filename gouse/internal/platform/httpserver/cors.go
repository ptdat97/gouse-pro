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
// HeaderChoPhep là danh sách header TRÌNH DUYỆT được phép gửi lên.
//
// # Vì sao là một danh sách khai báo, không phải một chuỗi trong hàm
//
// Danh sách này phải khớp MỌI header giao diện gửi lên. Thiếu một cái thì
// trình duyệt chặn request ở bước preflight — request thật không bao giờ
// rời máy khách, nên **log máy chủ hoàn toàn sạch** và lỗi chỉ hiện ở
// console trình duyệt.
//
// Cảnh báo ấy đã nằm ở đây từ đầu, và ngày 17/09/2026 vẫn bị vi phạm:
// `X-Visit-Id` được thêm vào api-client mà không ai thêm vào đây, và CẢ
// cửa hàng ngừng tải được dữ liệu với mọi trình duyệt thật. Test cũ không
// bắt được vì nó kiểm một danh sách bốn cái viết tay, không kiểm yêu cầu.
//
// Nay danh sách là biến XUẤT KHẨU để `cmd/apicheck` đối chiếu nó với các
// header khai `in: header` trong đặc tả OpenAPI. Thêm header vào đặc tả mà
// quên chỗ này thì CI đỏ.
var HeaderChoPhep = []string{
	// Chuẩn HTTP, mọi lệnh ghi đều cần.
	"Authorization",
	"Content-Type",

	// Idempotency-Key: mọi lệnh ghi API đều idempotent (mvp.md mục 7).
	"Idempotency-Key",

	// X-Request-ID: client truyền mã để tra log khi cần hỗ trợ.
	"X-Request-ID",

	"Accept-Language",

	// X-Guest-Phone: khách VÃNG LAI tra đơn bằng mã đơn kèm số điện thoại
	// (orders.yaml, operationId getOrder).
	"X-Guest-Phone",

	// X-Visit-Id: mã MỘT LƯỢT TRUY CẬP do trình duyệt sinh (ADR-0020).
	// api-client gửi nó kèm MỌI request, nên thiếu nó ở đây là hỏng toàn
	// bộ giao diện, không phải hỏng một tính năng.
	"X-Visit-Id",

	// X-Access-Reason: bắt buộc khi nhân viên xem hồ sơ khách (customer/
	// interfaces/http/admin.go). Chưa có màn hình nào gọi — thêm sẵn ở đây
	// để màn hình đầu tiên không đâm vào đúng bức tường vừa dựng lại.
	"X-Access-Reason",
}

// allowHeaders là HeaderChoPhep đã nối sẵn, tính một lần lúc nạp package.
var allowHeaders = strings.Join(HeaderChoPhep, ", ")

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
				h.Set("Access-Control-Allow-Headers", allowHeaders)
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
