package apierror

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// responseBody khớp CHÍNH XÁC api/components/common.yaml#/schemas/Error.
//
//	{
//	  "error": { "code", "message", "details", "field_errors" },
//	  "request_id": "req_..."
//	}
type responseBody struct {
	Error     errorPayload `json:"error"`
	RequestID string       `json:"request_id"`
}

type errorPayload struct {
	Code        Code           `json:"code"`
	Message     string         `json:"message"`
	Details     map[string]any `json:"details,omitempty"`
	FieldErrors []FieldError   `json:"field_errors,omitempty"`
}

// Write ghi lỗi ra response theo đúng định dạng đặc tả.
//
// requestID LUÔN có trong response — dùng khi hỗ trợ khách hàng để tra ngược
// toàn bộ chuỗi từ request tới bút toán.
//
// Lỗi 5xx được ghi log ở mức error kèm lỗi gốc; lỗi 4xx ghi ở mức debug vì
// chúng là hành vi bình thường của client, không phải sự cố hệ thống.
func Write(w http.ResponseWriter, r *http.Request, err error, requestID string, logger *slog.Logger) {
	apiErr := From(err)
	status := apiErr.HTTPStatus()

	if logger != nil {
		attrs := []any{
			"error_code", string(apiErr.Code),
			"status", status,
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
		}
		if status >= 500 {
			// Lỗi hệ thống: ghi kèm lỗi gốc để gỡ lỗi.
			attrs = append(attrs, "error", apiErr.Error())
			logger.Error("request thất bại", attrs...)
		} else {
			logger.Debug("request bị từ chối", attrs...)
		}
	}

	body := responseBody{
		Error: errorPayload{
			Code:        apiErr.Code,
			Message:     apiErr.Message,
			Details:     apiErr.Details,
			FieldErrors: apiErr.FieldErrors,
		},
		RequestID: requestID,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if encErr := json.NewEncoder(w).Encode(body); encErr != nil && logger != nil {
		// Response đã gửi header, không sửa được nữa — chỉ ghi log.
		logger.Error("không ghi được response lỗi", "error", encErr, "request_id", requestID)
	}
}

// WriteJSON ghi response thành công.
//
// Đặt ở đây để mọi response đi qua cùng một chỗ, đảm bảo header nhất quán.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(v)
}
