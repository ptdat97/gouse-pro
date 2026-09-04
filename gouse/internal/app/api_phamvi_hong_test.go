package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// TestPhamViNhaBanHongTra403KhongPhai500.
//
// # Tình huống, và cách nó lộ ra
//
// Một tài khoản được cấp vai trò SELLER_OWNER với phạm vi là gian hàng
// KHÔNG tồn tại. Chuyện này xảy ra được vì `identity.GrantRole` không thể
// kiểm tra mã gian hàng: identity nằm ở tầng nền, seller ở tầng nghiệp vụ,
// và identity gọi ngược lên là vi phạm ranh giới. Đó là ràng buộc kiến
// trúc đúng, nhưng hệ quả là grant có thể trỏ vào hư không.
//
// Tìm ra khi đo tải PH-17: một tài khoản trong dữ liệu phát triển mang hai
// phạm vi, cái đầu trỏ tới gian hàng đã biến mất, và MỌI lượt gọi endpoint
// tồn kho của nó trả 500.
//
// # Vì sao 500 là câu trả lời sai
//
// Máy chủ vẫn chạy đúng, người gọi vẫn gửi đúng — dữ liệu phân quyền mới
// là thứ sai. Trả 500 sai theo hai hướng cùng lúc: người dùng thấy "lỗi hệ
// thống" và đi báo sự cố thay vì gọi quản trị viên, còn giám sát đếm nó
// vào tỷ lệ lỗi máy chủ, che mất một vấn đề cần người sửa bằng tay.
func TestPhamViNhaBanHongTra403KhongPhai500(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	email := emailMoi("phamvihong")
	const matKhau = "MatKhauDuDai@2026"
	u, err := a.mods.identity.Register(ctx, identity.RegisterRequest{
		Email: email, Password: matKhau,
	})
	if err != nil {
		t.Fatalf("tạo tài khoản: %v", err)
	}

	// Phạm vi trỏ tới một gian hàng KHÔNG tồn tại.
	const gianHangMa = "sel_01M0D5KHONGTONTAI000001X"
	if err := a.mods.identity.GrantRole(
		ctx, u.ID, identity.RoleSellerOwner, gianHangMa); err != nil {
		t.Fatalf("cấp vai trò: %v", err)
	}

	res := a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": matKhau}, nil)
	tok, _ := res.body["access_token"].(string)
	if tok == "" {
		t.Fatalf("đăng nhập: %s", res.raw)
	}

	h := khoaIdem()
	h["Authorization"] = "Bearer " + tok
	got := a.call(http.MethodPut,
		"/api/v1/seller/inventory/sku_01M0C73VPBMPV3KJRZ6A295ZDF",
		map[string]any{
			"quantity_available": 10,
			"reason":             "kiểm kê thử với phạm vi hỏng",
		}, h)

	if got.code >= 500 {
		t.Errorf("phạm vi nhà bán hỏng trả HTTP %d — đây là dữ liệu phân "+
			"quyền sai, không phải mã hỏng, và 500 khiến người dùng đi báo "+
			"sự cố thay vì gọi quản trị viên: %s", got.code, got.raw)
	}
	if got.code != http.StatusForbidden {
		t.Errorf("phạm vi nhà bán hỏng trả HTTP %d, cần 403: %s",
			got.code, got.raw)
	}
}
