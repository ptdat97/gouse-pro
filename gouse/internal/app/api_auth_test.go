package app

import (
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// TestDuongQuanTriChanNguoiKhongCoQuyen — lỗi NỐI DÂY nguy hiểm nhất.
//
// Handler quản trị KHÔNG tự kiểm vai trò; việc đó do middleware
// `RequireRole` làm lúc nối route. Đó là thiết kế đúng — tầng interfaces
// không nên biết mô hình phân quyền — nhưng nó có nghĩa là một route quên
// bọc middleware sẽ MỞ TOANG mà bản thân handler vẫn đúng và test module
// của nó vẫn xanh.
//
// Bảng dưới đi qua MỌI đường quản trị, không chọn mẫu: đường bị quên
// thường là đường mới thêm, tức đường không ai nghĩ tới khi viết test.
// duongCanQuyen liệt kê MỌI đường yêu cầu vai trò đặc biệt.
//
// `TestMoiDuongCanQuyenDeuDuocKiem` quét mã nguồn và bắt lỗi nếu danh sách
// này thiếu — nên thêm route mới mà quên khai ở đây sẽ làm test đỏ, không
// phải lặng lẽ mất bảo vệ.
func duongCanQuyen() []struct {
	method string
	path   string
} {
	type d = struct {
		method string
		path   string
	}
	return []d{
		{http.MethodGet, "/api/v1/admin/audit-log"},
		{http.MethodPost, "/api/v1/admin/inventory/adjustments"},
		{http.MethodGet, "/api/v1/admin/orders"},
		{http.MethodGet, "/api/v1/admin/orders/ord_01J9XABC123DEF456GHJKMNPQR"},
		{http.MethodGet, "/api/v1/admin/sellers"},
		{http.MethodGet, "/api/v1/admin/sellers/sel_01J9XABC123DEF456GHJKMNPQR"},
		{http.MethodPost, "/api/v1/admin/ledger/adjustments"},
		{http.MethodPost, "/api/v1/admin/orders/ord_01J9XABC123DEF456GHJKMNPQR/cancel"},
		{http.MethodPost, "/api/v1/admin/sellers/sel_01J9XABC123DEF456GHJKMNPQR/approve"},
		{http.MethodPost, "/api/v1/admin/sellers/sel_01J9XABC123DEF456GHJKMNPQR/suspend"},

		{http.MethodGet, "/api/v1/seller/fulfillment-orders"},
		{http.MethodGet, "/api/v1/seller/fulfillment-orders/ful_01J9XABC123DEF456GHJKMNPQR"},
		{http.MethodGet, "/api/v1/seller/offers"},
		{http.MethodPost, "/api/v1/seller/offers"},
		{http.MethodPatch, "/api/v1/seller/offers/off_01J9XABC123DEF456GHJKMNPQR"},
		{http.MethodPost, "/api/v1/seller/fulfillment-orders/ful_01J9XABC123DEF456GHJKMNPQR/ship"},
		{http.MethodPost, "/api/v1/seller/fulfillment-orders/ful_01J9XABC123DEF456GHJKMNPQR/deliver"},
		{http.MethodGet, "/api/v1/seller/returns"},
		{http.MethodGet, "/api/v1/seller/balance"},
		{http.MethodGet, "/api/v1/seller/settlements"},
		{http.MethodGet, "/api/v1/seller/settlements/stl_01J9XABC123DEF456GHJKMNPQR"},
		{http.MethodPost, "/api/v1/seller/returns/ret_01J9XABC123DEF456GHJKMNPQR/approve"},
		{http.MethodPost, "/api/v1/seller/returns/ret_01J9XABC123DEF456GHJKMNPQR/reject"},
		{http.MethodPost, "/api/v1/seller/returns/ret_01J9XABC123DEF456GHJKMNPQR/receive"},
		{http.MethodPost, "/api/v1/seller/returns/ret_01J9XABC123DEF456GHJKMNPQR/inspect"},
		{http.MethodPut, "/api/v1/seller/inventory/sku_01J9XABC123DEF456GHJKMNPQR"},
	}
}

func TestDuongQuanTriChanNguoiKhongCoQuyen(t *testing.T) {
	a := newAPITest(t)

	// Tài khoản CHỈ có vai trò CUSTOMER — đăng ký công khai không cấp
	// thêm gì.
	tokKhach := a.dangKyVaDangNhap(emailMoi("khach"))

	for _, d := range duongCanQuyen() {
		t.Run(d.method+" "+d.path, func(t *testing.T) {
			// KHÔNG token → 401.
			if got := a.call(d.method, d.path, map[string]any{}, khoaIdem()); got.code != http.StatusUnauthorized {
				t.Errorf("không token: HTTP %d, cần 401 — %s", got.code, got.raw)
			}

			// Token hợp lệ nhưng SAI vai trò → 403.
			//
			// Phân biệt 401 với 403 quan trọng: 401 nói "chưa đăng nhập",
			// 403 nói "đăng nhập rồi nhưng không đủ quyền". Trả nhầm khiến
			// giao diện đá người dùng về trang đăng nhập vô hạn.
			h := khoaIdem()
			h["Authorization"] = "Bearer " + tokKhach
			got := a.call(d.method, d.path, map[string]any{}, h)
			if got.code != http.StatusForbidden {
				t.Errorf("token CUSTOMER: HTTP %d, cần 403 — %s", got.code, got.raw)
			}
		})
	}
}

// TestDuongMeChiCanDANGNHAP, không cần vai trò.
//
// `/api/v1/admin/me` nằm dưới tiền tố `admin` nhưng CỐ Ý chỉ yêu cầu đăng
// nhập: Trung tâm người bán dùng chính nó để khôi phục phiên. Bọc
// `RequireRole("ADMIN")` vào đây sẽ khóa nhà bán ra khỏi ứng dụng của họ.
//
// Ghi thành test vì đây là ngoại lệ, và ngoại lệ không có test là ngoại lệ
// sẽ bị ai đó "sửa" cho nhất quán.
func TestDuongMeChiCanDangNhap(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("khach"))

	if got := a.call(http.MethodGet, "/api/v1/admin/me", nil, nil); got.code != http.StatusUnauthorized {
		t.Errorf("không token: HTTP %d, cần 401", got.code)
	}
	got := a.call(http.MethodGet, "/api/v1/admin/me", nil, bearer(tok))
	if got.code != http.StatusOK {
		t.Errorf("token CUSTOMER: HTTP %d, cần 200 — %s", got.code, got.raw)
	}
}

// TestDuongGhiBatBuocKhoaIdempotency — lỗi nối dây loại hai.
//
// `RequireIdempotencyKey` cũng là middleware gắn lúc nối route. Quên nó
// thì hai lần bấm tạo hai đơn, và không có gì báo — đường vẫn chạy, chỉ
// là mất tính chống trùng.
//
// Loại lỗi này KHÔNG lộ ra ở bất kỳ test nào khác: test module không dựng
// middleware, còn test đầu-cuối gọi thẳng service.
func TestDuongGhiBatBuocKhoaIdempotency(t *testing.T) {
	a := newAPITest(t)

	duong := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/api/v1/cart/items", map[string]any{"offer_id": "off_x", "quantity": 1}},
		{http.MethodPost, "/api/v1/checkout", map[string]any{"cart_id": "crt_x"}},
		{http.MethodPost, "/api/v1/orders", map[string]any{}},
		{http.MethodPost, "/api/v1/auth/register", map[string]any{
			"email": emailMoi("nokey"), "password": "MatKhauDuDai@2026"}},
	}

	for _, d := range duong {
		t.Run(d.path, func(t *testing.T) {
			got := a.call(d.method, d.path, d.body, nil)
			if got.code != http.StatusBadRequest {
				t.Errorf("thiếu Idempotency-Key: HTTP %d, cần 400 — %s",
					got.code, got.raw)
			}
		})
	}
}

// TestKhachChiXemDuocDonCuaChinhMinh — cách ly dữ liệu, kiểm qua HTTP.
//
// Quy tắc nằm ở domain (`Order.ViewableBy`) và đã có test riêng. Bài này
// kiểm rằng nó THẬT SỰ được gọi trên đường HTTP — một handler quên hỏi là
// đủ để mở toàn bộ lịch sử đơn hàng.
//
// Trả 404 chứ không phải 403: phân biệt hai mã cho phép dò xem mã đơn nào
// CÓ THẬT.
func TestKhachChiXemDuocDonCuaChinhMinh(t *testing.T) {
	a := newAPITest(t)

	tokA := a.dangKyVaDangNhap(emailMoi("a"))
	tokB := a.dangKyVaDangNhap(emailMoi("b"))

	// B thử mở một mã đơn bất kỳ — kể cả khi đoán trúng định dạng.
	maDon := ids.MustNew(ids.PrefixOrder).String()

	for ten, tok := range map[string]string{"khách A": tokA, "khách B": tokB} {
		got := a.call(http.MethodGet, "/api/v1/orders/"+maDon, nil, bearer(tok))
		if got.code != http.StatusNotFound {
			t.Errorf("%s mở đơn lạ: HTTP %d, cần 404 — %s", ten, got.code, got.raw)
		}
	}
}

// TestDuongNhaBanChanKhachThuong: đường của nhà bán phải chặn tài khoản
// chỉ có vai trò CUSTOMER.
func TestDuongNhaBanChanKhachThuong(t *testing.T) {
	a := newAPITest(t)
	tok := a.dangKyVaDangNhap(emailMoi("khach"))

	duong := []string{
		"/api/v1/seller/offers",
		"/api/v1/seller/fulfillment-orders",
	}
	for _, p := range duong {
		t.Run(p, func(t *testing.T) {
			if got := a.call(http.MethodGet, p, nil, nil); got.code != http.StatusUnauthorized {
				t.Errorf("không token: HTTP %d, cần 401", got.code)
			}
			got := a.call(http.MethodGet, p, nil, bearer(tok))
			if got.code != http.StatusForbidden {
				t.Errorf("token CUSTOMER: HTTP %d, cần 403 — %s", got.code, got.raw)
			}
		})
	}
}
