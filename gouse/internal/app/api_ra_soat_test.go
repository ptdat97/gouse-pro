package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/modules/seller"
)

// TestASellerProfileGoesFromSubmitToApproval.
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
func TestASellerProfileGoesFromSubmitToApproval(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()
	tok := a.createAccountWithRole(t, identity.RoleAdmin)

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
		x := idemKey()
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

	peek, err := a.mods.seller.GetSeller(ctx, sel.ID)
	if err != nil {
		t.Fatalf("đọc lại hồ sơ: %v", err)
	}
	if peek.Status != "APPROVED" {
		t.Errorf("trạng thái cuối = %q, cần APPROVED", peek.Status)
	}
	if peek.CommissionRateBP != 1000 {
		t.Errorf("hoa hồng = %d, cần 1000 — tỷ lệ lúc duyệt không được ghi",
			peek.CommissionRateBP)
	}
}

// TestARequestedReturnEmitsASignalWithItsReason.
//
// # Vì sao bài này ở tầng app
//
// Trách nhiệm của module `returns` là PHÁT event đúng, không phải ghi tín
// hiệu — việc ghi là của `supplychain`, và nó có bài riêng. Nên bài này
// kiểm outbox, không kiểm bảng tín hiệu.
//
// `SignalReturn` được khai trong domain supply-chain từ đầu, kèm chú thích
// nói rõ lý do hoàn là dữ liệu CHẤT LƯỢNG của thời trang — và không bên
// phát nào tồn tại. Đây là mắt xích đó.
func TestARequestedReturnEmitsASignalWithItsReason(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	orderID, lineID, _ := a.buildDeliveredOrder(t)
	if orderID == "" {
		t.Skip("không dựng được đơn đã giao")
	}

	res := a.call(http.MethodPost, "/api/v1/orders/"+orderID+"/returns",
		map[string]any{"lines": []any{map[string]any{
			"order_line_id": lineID, "quantity": 1,
			"reason_code": "SIZE_TOO_SMALL",
		}}},
		merge(idemKey(), map[string]string{"X-Guest-Phone": "0900321321"}))
	if res.code != http.StatusCreated {
		t.Fatalf("xin trả hàng: HTTP %d — %s", res.code, res.raw)
	}

	var eventCount int
	var reason string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(*), coalesce(max(payload->'lines'->0->>'reason_code'), '')
		  FROM event_outbox
		 WHERE event_type = 'returns.requested'
		   AND payload->>'order_id' = $1`, orderID).Scan(&eventCount, &reason); err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("xin trả hàng phát %d event returns.requested, cần 1 — "+
			"lý do hoàn không vào được dữ liệu chất lượng", eventCount)
	}
	if reason != "SIZE_TOO_SMALL" {
		t.Errorf("mã lý do trong payload = %q, cần SIZE_TOO_SMALL — thiếu "+
			"nó thì tín hiệu chỉ nói CÓ hàng bị trả, không nói VÌ SAO", reason)
	}
}
