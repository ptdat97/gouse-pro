package logger_test

import (
	"context"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// TestRedactsSensitiveData là test BẢO MẬT.
//
// Một lần quên che là dữ liệu nhạy cảm nằm VĨNH VIỄN trong hệ thống log.
// Vì vậy việc che đặt ở tầng handler, không dựa vào kỷ luật người viết code.
//
// Test này dùng logger THẬT (qua NewWithWriter), không nhân bản logic che —
// bản sao sẽ phân kỳ theo thời gian và test mất giá trị bảo vệ.
func TestRedactsSensitiveData(t *testing.T) {
	cases := []struct {
		key    string
		secret string
	}{
		{"password", "MatKhauBiMat123XYZ"},
		{"password_hash", "$2a$10$hashbimatXYZ"},
		{"token", "eyJhbGciOiJIUzI1NiJ9.tokenbimat"},
		{"access_token", "at_gia_tri_bi_mat"},
		{"refresh_token", "rt_gia_tri_bi_mat"},
		{"secret", "gia_tri_tuyet_mat"},
		{"api_key", "sk_live_khoa_bi_mat"},
		{"card_number", "4111111111111111"},
		{"cvv", "CVV-123-BIMAT"},
		{"pin", "PIN-9876-BIMAT"},
		{"authorization", "Bearer token_bi_mat_xyz"},
		{"bank_account", "TKNH-9704-BIMAT"},
		{"account_number", "STK-0123456789-BIMAT"},
		// Đặc thù nền tảng thời trang: số đo cơ thể là dữ liệu cá nhân nhạy cảm.
		// Xem docs/09-operations/security.md mục 5.
		//
		// Giá trị test phải ĐỦ DÀI VÀ DUY NHẤT: số ngắn như "98" xuất hiện
		// ngẫu nhiên trong dấu thời gian và gây báo động giả.
		{"body_measurements", "SODO-90-60-90-BIMAT"},
		{"chest_cm", "SODONGUC-98-BIMAT"},
		{"waist_cm", "SODOEO-76-BIMAT"},
		{"hip_cm", "SODOHONG-94-BIMAT"},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			var buf strings.Builder
			l := logger.NewWithWriter(&buf, "debug", "json")
			l.Info("thao tác test", tc.key, tc.secret)

			out := buf.String()
			if strings.Contains(out, tc.secret) {
				t.Errorf("RÒ RỈ dữ liệu nhạy cảm qua trường %q:\n%s", tc.key, out)
			}
			if !strings.Contains(out, "[ĐÃ ẨN]") {
				t.Errorf("trường %q phải bị che, log: %s", tc.key, out)
			}
		})
	}
}

func TestRedactionIsCaseInsensitive(t *testing.T) {
	// Người viết code có thể dùng "Password" hoặc "TOKEN" — việc che
	// không được phụ thuộc vào cách viết hoa.
	for _, key := range []string{"Password", "TOKEN", "Card_Number", "CVV"} {
		var buf strings.Builder
		l := logger.NewWithWriter(&buf, "debug", "json")
		l.Info("test", key, "gia-tri-bi-mat")

		if strings.Contains(buf.String(), "gia-tri-bi-mat") {
			t.Errorf("trường %q (khác cách viết hoa) KHÔNG bị che:\n%s", key, buf.String())
		}
	}
}

func TestNonSensitiveFieldsPreserved(t *testing.T) {
	// Che quá tay cũng là vấn đề: log mất giá trị gỡ lỗi.
	var buf strings.Builder
	l := logger.NewWithWriter(&buf, "debug", "json")
	l.Info("đơn hàng được tạo",
		"order_id", "ord_01J9XABC",
		"seller_id", "sel_01J9XABC",
		"amount", 628000,
		"request_id", "req_01J9XABC",
	)

	out := buf.String()
	for _, want := range []string{"ord_01J9XABC", "sel_01J9XABC", "628000", "req_01J9XABC"} {
		if !strings.Contains(out, want) {
			t.Errorf("trường không nhạy cảm %q bị mất:\n%s", want, out)
		}
	}
}

func TestLogLevelFiltering(t *testing.T) {
	var buf strings.Builder
	l := logger.NewWithWriter(&buf, "warn", "json")

	l.Debug("debug không được xuất hiện")
	l.Info("info cũng không")
	l.Warn("warn phải xuất hiện")
	l.Error("error phải xuất hiện")

	out := buf.String()
	if strings.Contains(out, "không được xuất hiện") || strings.Contains(out, "cũng không") {
		t.Errorf("log dưới ngưỡng vẫn xuất hiện:\n%s", out)
	}
	if !strings.Contains(out, "warn phải xuất hiện") {
		t.Errorf("log warn bị mất:\n%s", out)
	}
	if !strings.Contains(out, "error phải xuất hiện") {
		t.Errorf("log error bị mất:\n%s", out)
	}
}

func TestJSONFormatIsStructured(t *testing.T) {
	// Log phải TRUY VẤN ĐƯỢC — đó là lý do dùng JSON thay vì văn bản tự do.
	var buf strings.Builder
	l := logger.NewWithWriter(&buf, "info", "json")
	l.Info("test", "order_id", "ord_x")

	out := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(out, "{") || !strings.HasSuffix(out, "}") {
		t.Errorf("định dạng json phải cho ra JSON object:\n%s", out)
	}
	if !strings.Contains(out, `"order_id":"ord_x"`) {
		t.Errorf("trường phải là cặp khóa-giá trị truy vấn được:\n%s", out)
	}
}

func TestTextFormatForDevelopment(t *testing.T) {
	var buf strings.Builder
	l := logger.NewWithWriter(&buf, "info", "text")
	l.Info("test", "order_id", "ord_x")

	out := buf.String()
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("định dạng text không được cho ra JSON:\n%s", out)
	}
	if !strings.Contains(out, "order_id=ord_x") {
		t.Errorf("định dạng text phải có cặp khóa=giá trị:\n%s", out)
	}
}

func TestContextPropagation(t *testing.T) {
	ctx := context.Background()

	// Không có logger trong context → trả mặc định, KHÔNG nil.
	// Code gọi không cần kiểm tra nil, và thiếu logger không làm sập request.
	if logger.FromContext(ctx) == nil {
		t.Fatal("FromContext phải trả logger mặc định, không nil")
	}

	var buf strings.Builder
	l := logger.NewWithWriter(&buf, "info", "json")
	ctx = logger.WithContext(ctx, l)

	if got := logger.FromContext(ctx); got != l {
		t.Error("FromContext phải trả đúng logger đã gắn")
	}
}

func TestRequestIDPropagation(t *testing.T) {
	ctx := context.Background()

	if id := logger.RequestIDFromContext(ctx); id != "" {
		t.Errorf("context rỗng phải trả chuỗi rỗng, nhận %q", id)
	}

	ctx = logger.WithRequestID(ctx, "req_01J9XABC123DEF456GHJKMNPQR")
	if got := logger.RequestIDFromContext(ctx); got != "req_01J9XABC123DEF456GHJKMNPQR" {
		t.Errorf("request id không truyền đúng, nhận %q", got)
	}
}
