package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/checkout/application"
	checkouthttp "github.com/fashion-commerce/platform/internal/modules/checkout/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
)

// fakeCart chỉ cài đúng phần checkout cần để tìm giỏ của người gọi.
//
// Các test ở đây kiểm chứng những gì handler CHẶN TRƯỚC khi chạm tới tầng
// application. Chúng cố tình không dựng kho lưu trữ hay bốn module mà
// checkout điều phối: nếu một request đi được tới đó, test đã thất bại
// đúng ở chỗ cần thất bại.
type fakeCart struct{ activeID ids.ID }

var _ application.CartPort = (*fakeCart)(nil)

func (f *fakeCart) ActiveCartID(_ context.Context, _, _ string) (ids.ID, error) {
	return f.activeID, nil
}

func (f *fakeCart) LoadPurchasable(
	_ context.Context, _ ids.ID,
) (application.CartSnapshot, error) {
	panic("test không được đi tới LoadPurchasable")
}

func (f *fakeCart) MarkConverted(_ context.Context, _ ids.ID) error {
	panic("test không được đi tới MarkConverted")
}

func newHandler(t *testing.T, activeCart ids.ID) http.Handler {
	t.Helper()

	svc := application.NewService(application.Deps{
		Carts: &fakeCart{activeID: activeCart},
	})

	mux := http.NewServeMux()
	checkouthttp.NewHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	return httpserver.Chain(mux,
		httpserver.ResolveShopper(nil),
		httpserver.RequireIdempotencyKey(),
	)
}

func post(t *testing.T, h http.Handler, path, body string) (*http.Response, map[string]any) {
	t.Helper()

	r := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "01M02M664T3C76VV002A170C27")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	res := w.Result()

	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res, decoded
}

// TestKhongMoDuocPhienTrenGioNguoiKhac là test quan trọng nhất của tệp này.
//
// Đặc tả yêu cầu client gửi `cart_id`, nhưng module checkout chỉ nhận một
// mã giỏ — nó không biết ai đang gọi. Tin thẳng con số đó nghĩa là bất kỳ
// ai đoán được mã giỏ đều mở được phiên thanh toán trên giỏ người khác, và
// đọc được toàn bộ nội dung giỏ đó trong response.
//
// Handler PHẢI đối chiếu với giỏ đang dùng của chính người gọi.
func TestKhongMoDuocPhienTrenGioNguoiKhac(t *testing.T) {
	cuaToi := ids.MustNew(ids.PrefixCart)
	cuaNguoiKhac := ids.MustNew(ids.PrefixCart)

	h := newHandler(t, cuaToi)

	body, _ := json.Marshal(map[string]any{"cart_id": cuaNguoiKhac.String()})
	res, decoded := post(t, h, "/api/v1/checkout", string(body))

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("mã trạng thái = %d, muốn 403 — mở được phiên trên giỏ người khác",
			res.StatusCode)
	}

	// 403 chứ không phải 404: nói "không tìm thấy" cho một giỏ có thật là
	// nói dối, và vẫn để lộ mã nào tồn tại qua chênh lệch thời gian.
	if code := errCode(decoded); code != "FORBIDDEN" {
		t.Errorf("mã lỗi = %q, muốn FORBIDDEN", code)
	}
}

// TestThieuCartIDBiTuChoi kiểm chứng trường bắt buộc của đặc tả.
func TestThieuCartIDBiTuChoi(t *testing.T) {
	h := newHandler(t, ids.MustNew(ids.PrefixCart))

	res, _ := post(t, h, "/api/v1/checkout", `{}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, muốn 400", res.StatusCode)
	}
}

// TestPhuongThucThanhToanPhaiHopLe kiểm chứng handler chặn giá trị lạ
// TRƯỚC khi tạo đơn.
//
// Đặc tả giới hạn bốn giá trị. Cho một chuỗi bất kỳ đi qua nghĩa là đơn
// được tạo với phương thức thanh toán mà không hệ thống nào xử lý được.
func TestPhuongThucThanhToanPhaiHopLe(t *testing.T) {
	h := newHandler(t, ids.MustNew(ids.PrefixCart))

	id := ids.MustNew(ids.PrefixCheckout)
	body, _ := json.Marshal(map[string]any{"payment_method": "BITCOIN"})
	res, _ := post(t, h, "/api/v1/checkout/"+id.String()+"/complete", string(body))

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, muốn 400", res.StatusCode)
	}
}

// TestPhuongThucVanChuyenPhaiHopLe kiểm chứng danh sách phương thức đóng.
//
// Quan trọng hơn việc chặn giá trị lạ: phí vận chuyển được TRA từ bảng phí
// của máy chủ theo tên phương thức. Không có tên trong bảng thì không có
// phí — và một phiên với phí 0đ là tiền nền tảng tự chịu.
func TestPhuongThucVanChuyenPhaiHopLe(t *testing.T) {
	h := newHandler(t, ids.MustNew(ids.PrefixCart))

	id := ids.MustNew(ids.PrefixCheckout)
	r := httptest.NewRequest("PATCH",
		"/api/v1/checkout/"+id.String()+"/shipping-method",
		bytes.NewBufferString(`{"shipping_method":"TELEPORT"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "01M02M664T3C76VV002A170C27")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, muốn 400", w.Code)
	}
}

// TestKhachHangKhongGuiDuocPhiVanChuyen kiểm chứng phí KHÔNG đến từ client.
//
// Handler dùng DisallowUnknownFields, nên một client cố gửi kèm phí bị từ
// chối thẳng thay vì bị bỏ qua im lặng. Khác biệt quan trọng: bỏ qua im
// lặng làm client tưởng đã đặt được phí và chỉ phát hiện sai ở bước đối
// soát.
func TestKhachHangKhongGuiDuocPhiVanChuyen(t *testing.T) {
	h := newHandler(t, ids.MustNew(ids.PrefixCart))

	id := ids.MustNew(ids.PrefixCheckout)
	r := httptest.NewRequest("PATCH",
		"/api/v1/checkout/"+id.String()+"/shipping-method",
		bytes.NewBufferString(`{"shipping_method":"STANDARD","fee":{"amount":0,"currency":"VND"}}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "01M02M664T3C76VV002A170C27")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, muốn 400 — client đặt được phí vận chuyển", w.Code)
	}
}

// TestHoanTatBatBuocIdempotencyKey kiểm chứng lớp bảo vệ chống đặt hai đơn.
//
// Khách bấm "Đặt hàng" hai lần, hoặc ứng dụng thử lại sau timeout mạng.
// Không có khóa này thì hai lần bấm là hai đơn và hai lần trừ tiền.
func TestHoanTatBatBuocIdempotencyKey(t *testing.T) {
	h := newHandler(t, ids.MustNew(ids.PrefixCart))

	id := ids.MustNew(ids.PrefixCheckout)
	r := httptest.NewRequest("POST",
		"/api/v1/checkout/"+id.String()+"/complete",
		bytes.NewBufferString(`{"payment_method":"COD"}`))
	r.Header.Set("Content-Type", "application/json")
	// CỐ TÌNH không đặt Idempotency-Key.

	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("mã trạng thái = %d, muốn 400 khi thiếu Idempotency-Key", w.Code)
	}
}

// errCode lấy mã lỗi từ response lỗi theo đúng cấu trúc của đặc tả.
func errCode(body map[string]any) string {
	e, ok := body["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := e["code"].(string)
	return code
}
