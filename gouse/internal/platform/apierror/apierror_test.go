package apierror_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/apierror"
)

// TestErrorShapeMatchesOpenAPISpec kiểm chứng response khớp CHÍNH XÁC
// api/components/common.yaml#/schemas/Error.
//
// Nếu lệch, client nhận response không đúng đặc tả mà chúng ta công bố.
func TestErrorShapeMatchesOpenAPISpec(t *testing.T) {
	err := apierror.New(apierror.CodeInsufficientInventory, "Sản phẩm không đủ số lượng").
		WithDetails(map[string]any{
			"offer_id":  "off_01J9XABC123DEF456GHJKMNPQR",
			"requested": 2,
			"available": 1,
		})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", nil)
	apierror.Write(rec, req, err, "req_01J9XABC123DEF456GHJKMNPQR", nil)

	var got map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &got); e != nil {
		t.Fatalf("response không phải JSON hợp lệ: %v", e)
	}

	// Cấu trúc bắt buộc: { error: {...}, request_id: "..." }
	if _, ok := got["error"]; !ok {
		t.Fatal("thiếu trường bắt buộc 'error'")
	}
	if _, ok := got["request_id"]; !ok {
		t.Fatal("thiếu trường bắt buộc 'request_id' — luôn phải có để hỗ trợ khách hàng")
	}

	errObj := got["error"].(map[string]any)
	if errObj["code"] != "INSUFFICIENT_INVENTORY" {
		t.Errorf("code: mong INSUFFICIENT_INVENTORY, nhận %v", errObj["code"])
	}
	if errObj["message"] == "" {
		t.Error("message không được rỗng")
	}

	// details phải giữ nguyên thông tin để client hiển thị hữu ích
	details := errObj["details"].(map[string]any)
	if details["available"].(float64) != 1 {
		t.Errorf("details.available: mong 1, nhận %v", details["available"])
	}

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("INSUFFICIENT_INVENTORY phải trả 422, nhận %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: mong application/json, nhận %q", ct)
	}
}

// TestInternalErrorDoesNotLeakDetails là test BẢO MẬT.
//
// Lỗi nội bộ có thể chứa câu lệnh SQL, tên bảng, đường dẫn file, thông tin
// kết nối — không bao giờ được lộ ra response.
func TestInternalErrorDoesNotLeakDetails(t *testing.T) {
	internal := errors.New(
		`pq: relation "ledger_entry" does not exist (host=db-prod-01.internal user=app)`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	apierror.Write(rec, req, internal, "req_01J9XABC123DEF456GHJKMNPQR", nil)

	body := rec.Body.String()
	leaks := []string{"ledger_entry", "db-prod-01", "pq:", "relation", "user=app"}
	for _, leak := range leaks {
		if strings.Contains(body, leak) {
			t.Errorf("RÒ RỈ THÔNG TIN NỘI BỘ %q trong response:\n%s", leak, body)
		}
	}

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("lỗi không xác định phải trả 500, nhận %d", rec.Code)
	}

	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	errObj := got["error"].(map[string]any)
	if errObj["code"] != "INTERNAL_ERROR" {
		t.Errorf("mong INTERNAL_ERROR, nhận %v", errObj["code"])
	}
}

// TestInternalErrorPreservesOriginalForLogging — lỗi gốc phải giữ được
// để ghi log, dù không lộ ra response.
func TestInternalErrorPreservesOriginalForLogging(t *testing.T) {
	original := errors.New("lỗi kết nối database")
	wrapped := apierror.Wrap(original, apierror.CodeInternalError, "Đã có lỗi xảy ra")

	if !errors.Is(wrapped, original) {
		t.Fatal("lỗi gốc phải truy được bằng errors.Is để gỡ lỗi")
	}
	if !strings.Contains(wrapped.Error(), "lỗi kết nối database") {
		t.Error("Error() phải chứa lỗi gốc cho log")
	}
}

// TestHTTPStatusMapping kiểm chứng phân biệt 400/422/409 và 401/403.
func TestHTTPStatusMapping(t *testing.T) {
	cases := []struct {
		code   apierror.Code
		status int
		why    string
	}{
		{apierror.CodeValidationFailed, 400, "sai định dạng"},
		{apierror.CodeInsufficientInventory, 422, "đúng định dạng, sai nghiệp vụ"},
		{apierror.CodeOrderNotCancellable, 409, "xung đột trạng thái"},
		{apierror.CodeCheckoutExpired, 409, "xung đột trạng thái"},
		{apierror.CodeIdempotencyReused, 409, "key dùng cho nội dung khác"},
		{apierror.CodeUnauthorized, 401, "chưa đăng nhập, đăng nhập lại có ích"},
		{apierror.CodeForbidden, 403, "đã đăng nhập, đăng nhập lại vô ích"},
		{apierror.CodeBrandProtected, 403, "không đủ quyền bán thương hiệu"},
		{apierror.CodeNotFound, 404, ""},
		{apierror.CodeRateLimitExceeded, 429, ""},
		{apierror.CodeInternalError, 500, ""},
		{apierror.CodeLedgerEntryUnbalanced, 422, "vi phạm bất biến nghiệp vụ"},
	}
	for _, tc := range cases {
		got := apierror.New(tc.code, "x").HTTPStatus()
		if got != tc.status {
			t.Errorf("%s: mong %d, nhận %d (%s)", tc.code, tc.status, got, tc.why)
		}
	}
}

// TestErrorsIsComparesByCode cho phép so sánh lỗi ở tầng application.
func TestErrorsIsComparesByCode(t *testing.T) {
	err := apierror.New(apierror.CodeNotFound, "Không tìm thấy đơn hàng")

	if !errors.Is(err, apierror.ErrNotFound) {
		t.Error("errors.Is phải so sánh theo Code")
	}
	if errors.Is(err, apierror.ErrForbidden) {
		t.Error("mã khác nhau không được coi là bằng")
	}
}

// TestFieldErrorsForForms kiểm chứng lỗi kiểm tra dữ liệu theo trường.
func TestFieldErrorsForForms(t *testing.T) {
	err := apierror.New(apierror.CodeValidationFailed, "Dữ liệu không hợp lệ").
		WithFieldErrors(
			apierror.FieldError{
				Field: "quantity", Code: "MUST_BE_POSITIVE",
				Message: "Số lượng phải lớn hơn 0",
			},
			apierror.FieldError{
				Field: "offer_id", Code: "INVALID_FORMAT",
				Message: "Định danh không hợp lệ",
			},
		)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items", nil)
	apierror.Write(rec, req, err, "req_x", nil)

	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	fes := got["error"].(map[string]any)["field_errors"].([]any)
	if len(fes) != 2 {
		t.Fatalf("mong 2 field_errors, nhận %d", len(fes))
	}
	first := fes[0].(map[string]any)
	if first["field"] != "quantity" || first["code"] != "MUST_BE_POSITIVE" {
		t.Errorf("field_error đầu tiên sai: %v", first)
	}
}

// TestWithDetailsDoesNotMutateOriginal — các lỗi dựng sẵn là biến toàn cục,
// gắn details không được làm hỏng chúng cho lần dùng sau.
func TestWithDetailsDoesNotMutateOriginal(t *testing.T) {
	base := apierror.New(apierror.CodeNotFound, "Không tìm thấy")
	withDetails := base.WithDetails(map[string]any{"id": "ord_x"})

	if base.Details != nil {
		t.Error("WithDetails không được sửa lỗi gốc")
	}
	if withDetails.Details == nil {
		t.Error("bản sao phải có details")
	}
}

func TestOmitEmptyFields(t *testing.T) {
	// details và field_errors rỗng không được xuất hiện trong JSON —
	// giữ response gọn.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	apierror.Write(rec, req, apierror.ErrNotFound, "req_x", nil)

	body := rec.Body.String()
	if strings.Contains(body, "details") {
		t.Errorf("details rỗng không được xuất hiện: %s", body)
	}
	if strings.Contains(body, "field_errors") {
		t.Errorf("field_errors rỗng không được xuất hiện: %s", body)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]any{"id": "ord_x", "total": 628000}

	if err := apierror.WriteJSON(rec, http.StatusCreated, payload); err != nil {
		t.Fatalf("WriteJSON lỗi: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("mong 201, nhận %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type sai: %q", ct)
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body không phải JSON: %v", err)
	}
	if got["id"] != "ord_x" {
		t.Errorf("body sai: %v", got)
	}
}

func TestFromNilReturnsNil(t *testing.T) {
	if apierror.From(nil) != nil {
		t.Error("From(nil) phải trả nil")
	}
}
