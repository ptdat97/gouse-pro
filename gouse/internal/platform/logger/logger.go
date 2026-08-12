// Package logger cấu hình log có cấu trúc.
//
// Quy tắc bắt buộc (docs/09-operations/observability.md mục 2):
//   - Log có cấu trúc (JSON), không phải văn bản tự do → truy vấn được
//   - Luôn có request_id → liên kết mọi log của một request
//   - KHÔNG log dữ liệu nhạy cảm: mật khẩu, số thẻ, số đo cơ thể, token
//   - Không log toàn bộ payload request — có thể chứa dữ liệu cá nhân
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// contextKey là kiểu riêng để tránh xung đột khóa context.
type contextKey struct{ name string }

var (
	loggerKey    = &contextKey{"logger"}
	requestIDKey = &contextKey{"request_id"}
)

// sensitiveKeys là các khóa KHÔNG BAO GIỜ được ghi log.
//
// Số đo cơ thể là dữ liệu đặc thù nền tảng thời trang và nhạy cảm hơn
// nhiều người nghĩ — xem docs/09-operations/security.md mục 5.
var sensitiveKeys = map[string]bool{
	"password":          true,
	"password_hash":     true,
	"token":             true,
	"access_token":      true,
	"refresh_token":     true,
	"secret":            true,
	"api_key":           true,
	"card_number":       true,
	"cvv":               true,
	"pin":               true,
	"authorization":     true,
	"body_measurements": true,
	"chest_cm":          true,
	"waist_cm":          true,
	"hip_cm":            true,
	"bank_account":      true,
	"account_number":    true,
}

const redacted = "[ĐÃ ẨN]"

// New tạo logger ghi ra stdout theo cấu hình.
func New(level, format string) *slog.Logger {
	return NewWithWriter(os.Stdout, level, format)
}

// NewWithWriter tạo logger ghi ra writer chỉ định.
//
// Tách ra để test kiểm chứng được HÀNH VI THẬT của việc che dữ liệu nhạy cảm,
// thay vì phải nhân bản logic che trong test — bản sao sẽ phân kỳ theo thời gian
// và test mất giá trị bảo vệ.
func NewWithWriter(w io.Writer, level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:       parseLevel(level),
		ReplaceAttr: redactSensitive,
	}

	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(h)
}

// redactSensitive che các trường nhạy cảm ở tầng handler.
//
// Đặt ở đây thay vì dựa vào kỷ luật của người viết code: một lần quên
// là dữ liệu nhạy cảm nằm vĩnh viễn trong hệ thống log.
func redactSensitive(_ []string, a slog.Attr) slog.Attr {
	if sensitiveKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, redacted)
	}
	return a
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithContext gắn logger vào context để truyền qua các tầng.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext lấy logger từ context.
//
// Trả về logger mặc định nếu không có — code gọi không cần kiểm tra nil,
// và việc thiếu logger không bao giờ làm sập request.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// WithRequestID gắn request id vào context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext lấy request id. Trả chuỗi rỗng nếu không có.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
