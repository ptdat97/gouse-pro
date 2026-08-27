package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	idsPkg "github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// TestNhaBanTuDangKyQuaHTTP — PH-35.
//
// # Vì sao bài này tồn tại
//
// Trước 27/08, `applyAsSeller` có trong đặc tả và `Module.ApplyAsSeller`
// chạy được, nhưng KHÔNG có route. Cách duy nhất tạo nhà bán là công cụ
// dòng lệnh `cmd/taonhaban`. Một cái chợ mà người bán phải nhờ admin chạy
// lệnh mới vào được thì chưa phải cái chợ.
func TestNhaBanTuDangKyQuaHTTP(t *testing.T) {
	a := newAPITest(t)

	tok := a.dangKyVaDangNhap(emailMoi("nguoinop"))
	const soTaiKhoan = "1903888777666"

	res := a.call(http.MethodPost, "/api/v1/sellers", map[string]any{
		"seller_type":   "BUSINESS",
		"business_name": "Xưởng May Thử Nghiệm",
		"tax_id":        "0101234567",
		"contact_email": "xuong@example.com",
		"contact_phone": "0900112233",
		"bank_account": map[string]any{
			"bank_code":      "VCB",
			"account_number": soTaiKhoan,
			"account_holder": "CONG TY TNHH XUONG MAY THU NGHIEM",
		},
	}, hopNhat(bearer(tok), khoaIdem()))

	if res.code != http.StatusCreated {
		t.Fatalf("nộp hồ sơ: HTTP %d — %s", res.code, res.raw)
	}

	nb, _ := res.body["seller"].(map[string]any)
	maNB, _ := nb["id"].(string)
	if maNB == "" {
		t.Fatalf("không trả mã nhà bán: %s", res.raw)
	}

	// Response KHÔNG được mang theo số tài khoản.
	if strings.Contains(res.raw, soTaiKhoan) {
		t.Error("response CHỨA số tài khoản đầy đủ")
	}
}

// TestSoTaiKhoanKhongNamDangRoTrongDatabase — bất biến BẢO MẬT.
//
// docs/09-operations/security.md mục 5 xếp thông tin thanh toán vào nhóm
// "mã hóa khi lưu". Bài này kiểm điều đó ở nơi duy nhất kiểm được: quét
// TOÀN BỘ các cột văn bản của bảng `seller` tìm chuỗi gốc.
//
// Quét cả bảng chứ không chỉ cột đã biết là chủ ý: ai đó thêm một cột
// "ghi chú" rồi vô tình chép số tài khoản vào đó thì bài này vẫn bắt được.
func TestSoTaiKhoanKhongNamDangRoTrongDatabase(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	tok := a.dangKyVaDangNhap(emailMoi("bimat"))
	const soTaiKhoan = "9988776655443322"

	res := a.call(http.MethodPost, "/api/v1/sellers", map[string]any{
		"seller_type":   "INDIVIDUAL",
		"business_name": "Nhà Bán Bí Mật",
		"contact_email": "bimat@example.com",
		"contact_phone": "0900445566",
		"bank_account": map[string]any{
			"bank_code":      "TCB",
			"account_number": soTaiKhoan,
			"account_holder": "NGUYEN VAN A",
		},
	}, hopNhat(bearer(tok), khoaIdem()))
	if res.code != http.StatusCreated {
		t.Fatalf("nộp hồ sơ: HTTP %d — %s", res.code, res.raw)
	}
	nb, _ := res.body["seller"].(map[string]any)
	maNB, _ := nb["id"].(string)

	var soCotChua int
	err := a.db.Pool().QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT jsonb_each_text(to_jsonb(s)) AS kv FROM seller s WHERE s.id = $1
		) x WHERE (x.kv).value LIKE '%' || $2 || '%'`, maNB, soTaiKhoan).Scan(&soCotChua)
	if err != nil {
		t.Fatalf("quét cột: %v", err)
	}
	if soCotChua > 0 {
		t.Errorf("%d cột của bảng seller chứa số tài khoản ở DẠNG RÕ", soCotChua)
	}

	// Bốn số cuối thì PHẢI có — đó là thứ màn hình hiển thị.
	var last4 string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT bank_account_last4 FROM seller WHERE id = $1`, maNB).Scan(&last4); err != nil {
		t.Fatalf("đọc bốn số cuối: %v", err)
	}
	if last4 != "3322" {
		t.Errorf("bốn số cuối = %q, cần \"3322\"", last4)
	}

	// Và bản mã phải đọc lại được — mã hóa mà không giải mã được thì
	// không trả tiền cho nhà bán được, tức là còn tệ hơn.
	lai, err := a.mods.seller.LaySoTaiKhoan(ctx, maNB)
	if err != nil {
		t.Fatalf("đọc lại số tài khoản: %v", err)
	}
	if lai != soTaiKhoan {
		t.Errorf("giải mã ra %q, gốc %q", lai, soTaiKhoan)
	}
}

// TestXacMinhTaiKhoanKhongCoThiTuChoi.
//
// Trước đây `VerifyBankAccount` luôn thành công, kể cả khi chưa có tài
// khoản nào. Kết quả là dữ liệu thật có nhà bán ACTIVE mang cờ "đã xác
// minh" mà không ai biết nó nói về tài khoản nào — và cờ ấy chính là điều
// kiện để kích hoạt gian hàng.
//
// # Bài này chứng minh CÁI GÌ
//
// Rằng bất biến GIỮ ĐƯỢC — không phải rằng lớp nào giữ nó. Có HAI lớp:
// kiểm ở domain, và ràng buộc `seller_verified_needs_account` ở database.
// Đã kiểm bằng cách phá: bỏ lớp domain thì bài này VẪN XANH, vì ràng buộc
// database chặn thay.
//
// Đó là phòng thủ chiều sâu chạy đúng, nhưng nói rõ ra vẫn hơn để ngầm
// hiểu — người đọc sau sẽ tưởng bài này canh lớp domain.
func TestXacMinhTaiKhoanKhongCoThiTuChoi(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	v, err := a.mods.seller.ApplyAsSeller(ctx, seller.ApplicationRequest{
		Name: "Khong Tai Khoan", Slug: "khong-tai-khoan-" +
			strings.ToLower(idsPkg.MustNew(idsPkg.PrefixSeller).String()[26:]),
		SellerType: "BUSINESS", LegalName: "Cong ty Khong Tai Khoan",
		Email: "ktk@example.com", Phone: "0900000111",
		CommissionRateBP: 1000,
	})
	if err != nil {
		t.Fatalf("nộp hồ sơ: %v", err)
	}

	svc := a.mods.seller.Service()
	if _, err := svc.VerifyBankAccount(ctx, idsPkg.ID(v.ID)); err == nil {
		t.Error("xác minh THÀNH CÔNG cho nhà bán chưa có tài khoản nào")
	} else {
		t.Logf("bị từ chối đúng: %v", err)
	}
}
