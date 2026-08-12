// Package httpserver cung cấp HTTP server và middleware chung.
//
// Đây là hạ tầng TRUNG LẬP VỚI DOMAIN — nó không biết gì về order, seller,
// hay bất kỳ khái niệm nghiệp vụ nào. Nếu package này bắt đầu nhắc tới
// khái niệm nghiệp vụ, nó đặt sai chỗ (quy tắc R3 của archcheck).
//
// Phân biệt quan trọng:
//   - Xác thực (bạn là ai) → hạ tầng, thuộc platform
//   - Phân quyền nghiệp vụ (bạn được làm gì với dữ liệu này) → thuộc module
//     sở hữu dữ liệu
package httpserver

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// Middleware là hàm bọc handler.
type Middleware func(http.Handler) http.Handler

// Chain nối nhiều middleware. Middleware đầu tiên là lớp NGOÀI CÙNG.
//
//	Chain(h, RequestID, Recover, Logging)
//	→ RequestID(Recover(Logging(h)))
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

const headerRequestID = "X-Request-ID"

// RequestID gán định danh cho mỗi request.
//
// Định danh này LUÔN có trong response và trong mọi dòng log của request —
// khi khách khiếu nại "tôi bị trừ tiền hai lần lúc 14:23", có thể tra ngược
// toàn bộ chuỗi từ request tới bút toán.
//
// Client có thể tự gửi X-Request-ID; nếu không, server sinh mới.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(headerRequestID)
			if !ids.IsValid(id) {
				// Không tin định dạng do client gửi — sinh mới nếu không hợp lệ.
				id = ids.MustNew(ids.PrefixRequest).String()
			}

			w.Header().Set(headerRequestID, id)
			ctx := logger.WithRequestID(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Recover bắt panic và trả về lỗi 500 có cấu trúc.
//
// Panic ở một request KHÔNG được làm sập toàn bộ tiến trình — điều đó
// biến một lỗi ở luồng hiếm gặp thành sự cố mất toàn bộ dịch vụ.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				requestID := logger.RequestIDFromContext(r.Context())
				log.Error("panic khi xử lý request",
					"panic", rec,
					"stack", string(debug.Stack()),
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
				)

				// Stack trace CHỈ vào log, không bao giờ ra response.
				apierror.Write(w, r, apierror.ErrInternal, requestID, nil)
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder ghi lại mã trạng thái để logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Logging ghi log mỗi request với thời gian xử lý.
//
// KHÔNG log body — body có thể chứa mật khẩu, thông tin thanh toán,
// hoặc dữ liệu cá nhân.
func Logging(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			requestID := logger.RequestIDFromContext(r.Context())
			reqLog := log.With("request_id", requestID)
			ctx := logger.WithContext(r.Context(), reqLog)

			next.ServeHTTP(rec, r.WithContext(ctx))

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"bytes", rec.bytes,
			}
			switch {
			case rec.status >= 500:
				reqLog.Error("request", attrs...)
			case rec.status >= 400:
				reqLog.Warn("request", attrs...)
			default:
				reqLog.Info("request", attrs...)
			}
		})
	}
}

// MaxBytes giới hạn kích thước request body.
//
// Chống tấn công bằng payload lớn làm cạn bộ nhớ.
func MaxBytes(limit int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders đặt các header bảo mật cơ bản.
func SecurityHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			next.ServeHTTP(w, r)
		})
	}
}
