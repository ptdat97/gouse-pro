package httpserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// AuthContext là ngữ cảnh phân quyền của người gọi.
//
// Kiểu này CỐ TÌNH trùng hình dạng với identity.AuthContext nhưng KHÔNG
// import module đó — platform không được biết module nghiệp vụ nào tồn tại
// (quy tắc R3 của archcheck). Cầu nối là hàm TokenVerifier do cmd/api
// truyền vào lúc khởi động.
//
// Cái giá của việc giữ R3 là hai kiểu gần giống nhau phải chuyển đổi thủ
// công ở tầng nối dây. Cái được là platform không sập theo mỗi thay đổi
// của identity, và identity thay được mà không đụng tới HTTP.
type AuthContext struct {
	UserID string

	// Roles là danh sách vai trò, ví dụ ["ADMIN"] hoặc ["OPS_FINANCE"].
	Roles []string

	// Scope là phạm vi rộng nhất: OWN, SELLER, hoặc ALL.
	//
	// Middleware này KHÔNG diễn giải phạm vi — module sở hữu dữ liệu tự
	// dịch nó sang điều kiện truy vấn của mình. Platform không biết bảng
	// nào tồn tại ở đâu.
	Scope string

	// SellerIDs là các gian hàng người này có vai trò SELLER_*.
	SellerIDs []string

	SessionID string
}

// HasRole cho biết người gọi có vai trò này không.
func (a AuthContext) HasRole(role string) bool {
	for _, r := range a.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole cho biết người gọi có ÍT NHẤT MỘT trong các vai trò không.
//
// Danh sách rỗng trả false, không phải true: "không yêu cầu vai trò nào"
// phải được diễn đạt bằng cách không dùng RequireRole, chứ không phải bằng
// một danh sách rỗng đi lọt qua.
func (a AuthContext) HasAnyRole(roles ...string) bool {
	for _, want := range roles {
		if a.HasRole(want) {
			return true
		}
	}
	return false
}

// TokenVerifier đổi access token lấy ngữ cảnh phân quyền.
//
// Interface do BÊN GỌI định nghĩa — platform khai báo thứ nó cần, module
// identity cài đặt. Nhờ vậy chiều phụ thuộc là identity → platform, không
// phải ngược lại.
type TokenVerifier interface {
	// VerifyAccessToken trả về ngữ cảnh phân quyền của token.
	//
	// Trả lỗi khi token sai chữ ký, hết hạn, hoặc không giải mã được.
	// KHÔNG phân biệt các lý do này cho client.
	VerifyAccessToken(ctx context.Context, token string) (AuthContext, error)
}

type authCtxKey struct{}

// WithAuthContext gắn ngữ cảnh phân quyền vào context.
//
// Xuất ra để test dựng được request đã xác thực mà không cần token thật.
func WithAuthContext(ctx context.Context, ac AuthContext) context.Context {
	return context.WithValue(ctx, authCtxKey{}, ac)
}

// AuthContextFrom lấy ngữ cảnh phân quyền từ context.
//
// Trả ok=false khi request chưa qua middleware Auth. Handler PHẢI kiểm tra
// giá trị này — bỏ qua nó nghĩa là coi request chưa xác thực như người dùng
// có UserID rỗng, và một truy vấn `WHERE customer_id = ”` không trả về gì
// trông giống hệt "người này không có đơn nào".
func AuthContextFrom(ctx context.Context) (AuthContext, bool) {
	ac, ok := ctx.Value(authCtxKey{}).(AuthContext)
	return ac, ok
}

// Auth xác thực request bằng access token trong header Authorization.
//
// Thiếu token hoặc token không hợp lệ → 401. Middleware này KHÔNG quyết
// định người dùng được làm gì; đó là việc của RequireRole và của module sở
// hữu dữ liệu.
//
// Vì sao 401 chứ không phải 403: theo docs/06-api/api-guidelines.md mục 4,
// 401 nghĩa là "đăng nhập lại có ích", 403 nghĩa là "đăng nhập lại vô ích".
// Trả sai mã khiến client rơi vào vòng lặp làm mới token vô vọng.
func Auth(v TokenVerifier) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeAuthError(w, r, apierror.New(apierror.CodeUnauthorized,
					"Yêu cầu xác thực"))
				return
			}

			ac, err := v.VerifyAccessToken(r.Context(), token)
			if err != nil {
				// Lý do thất bại chỉ vào log, không ra response: phân biệt
				// "token hết hạn" với "token sai chữ ký" cho client là cho
				// kẻ tấn công biết họ đã đi đúng hướng tới đâu.
				logger.FromContext(r.Context()).Warn("xác thực thất bại",
					"error", err,
					"path", r.URL.Path,
				)
				writeAuthError(w, r, apierror.New(apierror.CodeUnauthorized,
					"Phiên đăng nhập không hợp lệ hoặc đã hết hạn"))
				return
			}

			ctx := WithAuthContext(r.Context(), ac)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth nhận diện người gọi NẾU có token, và cho đi tiếp nếu không.
//
// # Vì sao cần một biến thể không chặn
//
// Đường mua hàng phải chạy được cho khách VÃNG LAI (mvp.md mục 4): không
// tài khoản, không token. Nhưng khách ĐÃ đăng nhập cũng đi đúng đường đó,
// và giỏ của họ phải gắn với hồ sơ khách hàng chứ không phải cookie phiên.
//
// Auth() không dùng được vì nó trả 401 khi thiếu token. Bỏ hẳn xác thực
// cũng không được vì khi đó khách đăng nhập bị coi là vãng lai, và giỏ của
// họ biến mất mỗi lần đổi thiết bị.
//
// # Token HỎNG bị bỏ qua, không bị từ chối
//
// Token hết hạn giữa lúc khách đang mua hàng là chuyện thường. Trả 401 ở
// đây làm hỏng giỏ hàng đang có; coi như khách vãng lai thì họ mua tiếp
// được, và client tự làm mới token cho lần gọi sau.
//
// HỆ QUẢ PHẢI BIẾT: middleware này KHÔNG bảo vệ được gì. Mọi endpoint đứng
// sau nó phải tự an toàn với người gọi ẩn danh.
func OptionalAuth(v TokenVerifier) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			ac, err := v.VerifyAccessToken(r.Context(), token)
			if err != nil {
				logger.FromContext(r.Context()).Warn(
					"token không hợp lệ ở đường không bắt buộc xác thực",
					"error", err,
					"path", r.URL.Path,
				)
				next.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithAuthContext(r.Context(), ac)))
		})
	}
}

// RequireRole chặn request không có ít nhất một trong các vai trò.
//
// PHẢI đặt SAU Auth trong chuỗi middleware. Nếu đặt trước, context chưa có
// AuthContext và mọi request đều bị từ chối — chuỗi sai thứ tự thất bại
// theo hướng an toàn, nhưng vẫn là lỗi cấu hình cần sửa.
//
// Đây là hàng rào THÔ theo vai trò, không phải phân quyền nghiệp vụ. Nó trả
// lời "vai trò này có được chạm tới nhóm endpoint này không", không trả lời
// "người này có được sửa bản ghi cụ thể kia không" — câu sau thuộc về module
// sở hữu dữ liệu, nơi biết bản ghi đó là của ai.
func RequireRole(roles ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac, ok := AuthContextFrom(r.Context())
			if !ok {
				// Chưa qua Auth. Đây là lỗi nối dây của chúng ta, không
				// phải lỗi của client — nên ghi log ở mức Error.
				logger.FromContext(r.Context()).Error(
					"RequireRole chạy trước Auth — kiểm tra thứ tự middleware",
					"path", r.URL.Path,
				)
				writeAuthError(w, r, apierror.New(apierror.CodeUnauthorized,
					"Yêu cầu xác thực"))
				return
			}

			if !ac.HasAnyRole(roles...) {
				logger.FromContext(r.Context()).Warn("từ chối vì thiếu vai trò",
					"user_id", ac.UserID,
					"path", r.URL.Path,
					"required", roles,
				)
				// KHÔNG nói người dùng thiếu vai trò nào — đó là bản đồ mô
				// hình phân quyền, và người bị từ chối không cần bản đồ đó.
				writeAuthError(w, r, apierror.New(apierror.CodeForbidden,
					"Không đủ quyền thực hiện thao tác này"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken lấy token từ header Authorization.
//
// Chấp nhận đúng dạng "Bearer <token>"; tên scheme không phân biệt hoa
// thường theo RFC 7235.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}

	scheme, token, found := strings.Cut(h, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}

// writeAuthError trả lỗi xác thực theo định dạng chuẩn.
func writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := logger.RequestIDFromContext(r.Context())
	apierror.Write(w, r, err, requestID, nil)
}
