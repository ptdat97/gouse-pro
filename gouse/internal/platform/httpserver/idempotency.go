package httpserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Giới hạn độ dài khóa idempotency, khớp common.yaml#/parameters/IdempotencyKey.
const (
	idempotencyKeyMinLen = 16
	idempotencyKeyMaxLen = 64

	headerIdempotencyKey = "Idempotency-Key"
)

type idempotencyKeyCtxKey struct{}

// WithIdempotencyKey gắn khóa idempotency vào context.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, idempotencyKeyCtxKey{}, key)
}

// IdempotencyKeyFrom lấy khóa idempotency từ context.
//
// Trả ok=false khi request không đi qua middleware RequireIdempotencyKey.
func IdempotencyKeyFrom(ctx context.Context) (string, bool) {
	key, ok := ctx.Value(idempotencyKeyCtxKey{}).(string)
	return key, ok
}

// RequireIdempotencyKey bắt buộc header Idempotency-Key với lệnh ghi.
//
// # Phạm vi của middleware này
//
// Nó CHỈ kiểm tra khóa có mặt và đúng định dạng, rồi đưa vào context. Nó
// KHÔNG lưu trữ khóa và KHÔNG phát lại response cũ.
//
// Vì sao không: việc "cùng khóa thì không tạo bản ghi thứ hai" đã được các
// module xử lý bằng ràng buộc UNIQUE trên cột idempotency_key trong chính
// bảng của mình — xem migrations/000007_payment và 000008_order. Đó là chỗ
// DUY NHẤT có thể làm đúng, vì chỉ ở đó việc kiểm tra khóa và việc ghi dữ
// liệu mới nằm trong cùng một giao dịch.
//
// Một bộ nhớ đệm idempotency ở tầng HTTP sẽ nằm NGOÀI giao dịch đó, và tạo
// ra đúng lỗi nó định ngăn: ghi khóa xong rồi giao dịch nghiệp vụ rollback
// → lần thử lại bị coi là trùng lặp và trả về "thành công" cho một thao tác
// chưa bao giờ xảy ra.
//
// Vậy nên middleware này là hàng rào ĐỊNH DẠNG đặt ở biên, để lỗi thiếu
// header bị bắt một lần ở đây thay vì lặp lại trong từng handler.
func RequireIdempotencyKey() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !requiresIdempotencyKey(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			key := strings.TrimSpace(r.Header.Get(headerIdempotencyKey))
			if err := validateIdempotencyKey(key); err != nil {
				logger.FromContext(r.Context()).Warn("khóa idempotency không hợp lệ",
					"path", r.URL.Path,
					"method", r.Method,
				)
				requestID := logger.RequestIDFromContext(r.Context())
				apierror.Write(w, r, err, requestID, nil)
				return
			}

			ctx := WithIdempotencyKey(r.Context(), key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// requiresIdempotencyKey cho biết phương thức có cần khóa không.
//
// POST và PATCH cần, theo docs/06-api/api-guidelines.md mục 5.
//
// PUT và DELETE không cần vì bản thân chúng đã idempotent theo định nghĩa
// HTTP: gọi lại "đặt trạng thái thành X" hay "xóa bản ghi Y" cho cùng kết
// quả. GET không đổi trạng thái.
func requiresIdempotencyKey(method string) bool {
	return method == http.MethodPost || method == http.MethodPatch
}

// validateIdempotencyKey kiểm tra định dạng khóa.
func validateIdempotencyKey(key string) error {
	if key == "" {
		return apierror.New(apierror.CodeValidationFailed,
			"Thiếu header Idempotency-Key").
			WithDetails(map[string]any{"header": headerIdempotencyKey})
	}

	if len(key) < idempotencyKeyMinLen || len(key) > idempotencyKeyMaxLen {
		return apierror.New(apierror.CodeValidationFailed,
			"Idempotency-Key phải dài từ 16 đến 64 ký tự").
			WithDetails(map[string]any{
				"header":     headerIdempotencyKey,
				"min_length": idempotencyKeyMinLen,
				"max_length": idempotencyKeyMaxLen,
			})
	}

	return nil
}
