// Package apierror cung cấp định dạng lỗi thống nhất cho mọi endpoint.
//
// Cấu trúc khớp CHÍNH XÁC với api/components/common.yaml#/schemas/Error.
// Nếu hai bên lệch, client sẽ nhận response không đúng đặc tả mà chúng ta
// công bố — test TestErrorShapeMatchesOpenAPISpec bảo vệ điều này.
//
// Nguyên tắc: client xử lý theo `code` (máy đọc được, ổn định), KHÔNG parse
// `message` — message có thể đổi và có thể đa ngôn ngữ.
//
// Xem docs/06-api/api-guidelines.md mục 6.
package apierror

import (
	"errors"
	"fmt"
	"net/http"
)

// Code là mã lỗi máy đọc được, SCREAMING_SNAKE_CASE, ổn định qua các phiên bản.
type Code string

// Danh mục mã lỗi. Phải khớp enum trong common.yaml#/schemas/ErrorCode.
const (
	// Chung
	CodeValidationFailed  Code = "VALIDATION_FAILED"
	CodeUnauthorized      Code = "UNAUTHORIZED"
	CodeForbidden         Code = "FORBIDDEN"
	CodeNotFound          Code = "NOT_FOUND"
	CodeConflict          Code = "CONFLICT"
	CodeIdempotencyReused Code = "IDEMPOTENCY_KEY_REUSED"
	CodeIdempotencyInProg Code = "IDEMPOTENT_REQUEST_IN_PROGRESS"
	CodeRateLimitExceeded Code = "RATE_LIMIT_EXCEEDED"
	CodeInternalError     Code = "INTERNAL_ERROR"

	// Tồn kho và bán hàng
	CodeInsufficientInventory Code = "INSUFFICIENT_INVENTORY"
	CodeOfferNotAvailable     Code = "OFFER_NOT_AVAILABLE"
	CodePriceOutOfRange       Code = "PRICE_OUT_OF_RANGE"

	// Checkout và thanh toán
	CodeCheckoutExpired Code = "CHECKOUT_EXPIRED"
	CodePaymentFailed   Code = "PAYMENT_FAILED"

	// Đơn hàng và trả hàng
	CodeOrderNotCancellable Code = "ORDER_NOT_CANCELLABLE"
	CodeReturnWindowExpired Code = "RETURN_WINDOW_EXPIRED"

	// Marketplace
	CodeSellerNotAuthorized Code = "SELLER_NOT_AUTHORIZED"
	CodeBrandProtected      Code = "BRAND_PROTECTED"
	CodeSampleNotApproved   Code = "SAMPLE_NOT_APPROVED"

	// Tài chính
	CodeLedgerEntryUnbalanced Code = "LEDGER_ENTRY_UNBALANCED"

	// Nội dung
	CodeInvalidProductTag Code = "INVALID_PRODUCT_TAG"
)

// httpStatus ánh xạ mã lỗi sang mã HTTP.
//
// Phân biệt quan trọng:
//
//	400 — sai ĐỊNH DẠNG ("quantity phải là số")
//	422 — đúng định dạng, sai NGHIỆP VỤ ("quantity vượt tồn kho")
//	409 — xung đột TRẠNG THÁI ("đơn đã giao, không hủy được")
//	401 — chưa đăng nhập → đăng nhập lại CÓ ích
//	403 — đã đăng nhập, không đủ quyền → đăng nhập lại VÔ ích
func httpStatus(c Code) int {
	switch c {
	case CodeValidationFailed:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden, CodeSellerNotAuthorized, CodeBrandProtected:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict, CodeIdempotencyReused, CodeIdempotencyInProg,
		CodeCheckoutExpired, CodeOrderNotCancellable, CodeReturnWindowExpired:
		return http.StatusConflict
	case CodeInsufficientInventory, CodeOfferNotAvailable, CodePriceOutOfRange,
		CodePaymentFailed, CodeLedgerEntryUnbalanced, CodeInvalidProductTag,
		CodeSampleNotApproved:
		return http.StatusUnprocessableEntity
	case CodeRateLimitExceeded:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// FieldError là lỗi kiểm tra dữ liệu của một trường cụ thể (cho form).
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error là lỗi API có cấu trúc.
//
// Cài đặt interface error nên có thể dùng với errors.Is/errors.As và
// bọc lỗi bằng %w.
type Error struct {
	Code        Code
	Message     string
	Details     map[string]any
	FieldErrors []FieldError

	// wrapped giữ lỗi gốc để gỡ lỗi. KHÔNG BAO GIỜ lộ ra response —
	// nó có thể chứa chi tiết cấu trúc nội bộ hoặc thông tin nhạy cảm.
	wrapped error
}

func (e *Error) Error() string {
	if e.wrapped != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.wrapped)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.wrapped }

// HTTPStatus trả về mã HTTP tương ứng.
func (e *Error) HTTPStatus() int { return httpStatus(e.Code) }

// Is cho phép so sánh theo Code: errors.Is(err, apierror.ErrNotFound).
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// New tạo lỗi API.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Newf tạo lỗi API với message định dạng.
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap bọc lỗi gốc để giữ ngữ cảnh gỡ lỗi mà không lộ ra response.
func Wrap(err error, code Code, message string) *Error {
	return &Error{Code: code, Message: message, wrapped: err}
}

// WithDetails gắn thông tin chi tiết để client hiển thị hữu ích.
//
// "Chỉ còn 2 sản phẩm" tốt hơn nhiều so với "hết hàng" — khách cần đủ
// thông tin để quyết định giảm số lượng hay bỏ món đó.
func (e *Error) WithDetails(details map[string]any) *Error {
	clone := *e
	clone.Details = details
	return &clone
}

// WithFieldErrors gắn lỗi theo từng trường (cho form).
func (e *Error) WithFieldErrors(fes ...FieldError) *Error {
	clone := *e
	clone.FieldErrors = fes
	return &clone
}

// Các lỗi dựng sẵn để so sánh bằng errors.Is.
var (
	ErrNotFound     = New(CodeNotFound, "Không tìm thấy")
	ErrUnauthorized = New(CodeUnauthorized, "Chưa xác thực")
	ErrForbidden    = New(CodeForbidden, "Không đủ quyền")
	ErrConflict     = New(CodeConflict, "Xung đột trạng thái")
	ErrInternal     = New(CodeInternalError, "Lỗi hệ thống")
)

// From chuyển một error bất kỳ thành *Error.
//
// Lỗi không xác định được chuyển thành INTERNAL_ERROR với message chung —
// KHÔNG lộ nội dung lỗi gốc ra response, vì nó có thể chứa cấu trúc nội bộ
// hoặc thông tin nhạy cảm. Lỗi gốc vẫn được giữ trong wrapped để ghi log.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return &Error{
		Code:    CodeInternalError,
		Message: "Đã có lỗi xảy ra, vui lòng thử lại",
		wrapped: err,
	}
}
