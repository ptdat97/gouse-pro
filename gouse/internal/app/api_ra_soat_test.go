package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// TestHoSoNhaBanDiTronTuNopToiDuyet.
//
// # Vì sao bài này tồn tại
//
// `SubmitForReview` có đủ ở domain VÀ application, và CHỈ TEST gọi nó.
// Không cửa nào ở production đưa hồ sơ từ APPLIED sang PENDING_REVIEW, mà
// `Approve` lại đòi PENDING_REVIEW — nên hồ sơ nộp qua `ApplyAsSeller`
// không bao giờ duyệt được. Đo trên dữ liệu thật lúc phát hiện: 1 hồ sơ
// kẹt ở APPLIED.
//
// Đây là dạng lỗi hay gặp nhất của dự án này, lần này ở mức USE CASE chứ
// không phải mức trường: mã đúng, test riêng của nó xanh, và không nằm
// trên đường đi nào.
//
// Bài này đi TRỌN đường mà người vận hành thật đi: nộp → vào hàng đợi →
// duyệt. Nó KHÔNG gọi thẳng service ở bước giữa, vì chính chỗ đó là chỗ
// từng đứt.
func TestHoSoNhaBanDiTronTuNopToiDuyet(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	sel, err := a.mods.seller.ApplyAsSeller(ctx, seller.ApplicationRequest{
		Name: "Xưởng Trọn Đường", Slug: "xuong-tron-duong",
		SellerType: "BUSINESS", LegalName: "Công ty TNHH Trọn Đường",
		TaxCode: "0312345678", Email: "trronduong@example.com",
		Phone: "0900123456",
		BankAccount: seller.BankAccountInput{
			BankCode: "VCB", AccountNumber: "1234567890",
			AccountHolder: "CONG TY TRON DUONG",
		},
	})
	if err != nil {
		t.Fatalf("nộp hồ sơ: %v", err)
	}
	if sel.Status != "APPLIED" {
		t.Fatalf("hồ sơ vừa nộp ở trạng thái %q, mong APPLIED", sel.Status)
	}

	h := func() map[string]string {
		x := khoaIdem()
		x["Authorization"] = "Bearer " + tok
		return x
	}

	// Duyệt TẮT phải bị chặn — hàng rào này là lý do hai bước không gộp.
	if res := a.call(http.MethodPost,
		"/api/v1/admin/sellers/"+sel.ID+"/approve",
		map[string]any{"commission_rate_bp": 1000}, h()); res.code == http.StatusOK {
		t.Error("duyệt được hồ sơ CHƯA qua rà soát — mất hàng rào chống " +
			"duyệt tắt, và đó là cả lý do APPLIED tách khỏi PENDING_REVIEW")
	}

	// Vào hàng đợi rà soát.
	res := a.call(http.MethodPost,
		"/api/v1/admin/sellers/"+sel.ID+"/submit-review", nil, h())
	if res.code != http.StatusOK {
		t.Fatalf("đưa vào hàng đợi rà soát: HTTP %d — %s — hồ sơ nộp thật "+
			"không bao giờ duyệt được", res.code, res.raw)
	}
	if got, _ := res.body["status"].(string); got != "PENDING_REVIEW" {
		t.Errorf("trạng thái sau khi nộp rà soát = %q, cần PENDING_REVIEW", got)
	}

	// Giờ mới duyệt được.
	res = a.call(http.MethodPost,
		"/api/v1/admin/sellers/"+sel.ID+"/approve",
		map[string]any{"commission_rate_bp": 1000}, h())
	if res.code != http.StatusOK {
		t.Fatalf("duyệt hồ sơ đã qua rà soát: HTTP %d — %s", res.code, res.raw)
	}

	xem, err := a.mods.seller.GetSeller(ctx, sel.ID)
	if err != nil {
		t.Fatalf("đọc lại hồ sơ: %v", err)
	}
	if xem.Status != "APPROVED" {
		t.Errorf("trạng thái cuối = %q, cần APPROVED", xem.Status)
	}
	if xem.CommissionRateBP != 1000 {
		t.Errorf("hoa hồng = %d, cần 1000 — tỷ lệ lúc duyệt không được ghi",
			xem.CommissionRateBP)
	}
}
