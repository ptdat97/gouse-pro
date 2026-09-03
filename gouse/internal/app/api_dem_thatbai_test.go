package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/platform/metrics"
)

// demThatBai đọc giá trị bộ đếm thất bại cho một (stage, reason).
//
// Đọc thẳng từ registry của Prometheus chứ không parse `/metrics`: nếu
// nhãn bị đặt sai tên, bản parse văn bản vẫn "tìm thấy một dòng nào đó"
// và bài test xanh nhầm.
func demThatBai(t *testing.T, stage, reason string) float64 {
	t.Helper()

	// Đọc từ metrics.Registry, KHÔNG phải prometheus.DefaultGatherer.
	//
	// Dự án đăng ký chỉ số vào registry riêng. Đọc nhầm gatherer thì hàm
	// này luôn trả 0, và mọi khẳng định "đã đếm" đều xanh giả — bản đầu
	// của bài này đã hỏng đúng như vậy.
	fams, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("thu thập metrics: %v", err)
	}
	for _, f := range fams {
		if !strings.Contains(f.GetName(), "failure") {
			continue
		}
		for _, m := range f.GetMetric() {
			if khopNhan(m, stage, reason) {
				if c := m.GetCounter(); c != nil {
					return c.GetValue()
				}
			}
		}
	}
	return 0
}

func khopNhan(m *dto.Metric, stage, reason string) bool {
	var coStage, coReason bool
	for _, l := range m.GetLabel() {
		switch l.GetName() {
		case "stage":
			coStage = l.GetValue() == stage
		case "reason":
			coReason = l.GetValue() == reason
		}
	}
	return coStage && coReason
}

// TestDemThatBaiTien: bút toán không cân bằng phải được đếm, và đếm dưới
// nhãn RIÊNG.
//
// Đây là nhãn đáng theo dõi nhất trong cả hệ thống: bút toán lệch nghĩa là
// sổ sách sai. Gộp nó vào "internal" cùng mọi lỗi khác thì con số ấy chìm
// trong nhiễu, và sự cố nghiêm trọng nhất trông y hệt một lỗi vặt.
func TestDemThatBaiTien(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleOpsFinance)

	truoc := demThatBai(t, metrics.StagePayment, "unbalanced")

	h := khoaIdem()
	h["Authorization"] = "Bearer " + tok
	res := a.call(http.MethodPost, "/api/v1/admin/ledger/adjustments",
		map[string]any{
			"reference_type": "ORDER",
			"reference_id":   "ord_01J9XABC123DEF456GHJKMNPQR",
			"reason":         "đối soát chênh lệch kỳ tháng 8 theo biên bản",
			// Σ DEBIT ≠ Σ CREDIT — cố ý.
			"lines": []map[string]any{
				{"account_type": "PLATFORM_REVENUE", "direction": "DEBIT",
					"amount": map[string]any{"amount": 100000, "currency": "VND"}},
				{"account_type": "PLATFORM_CASH", "direction": "CREDIT",
					"amount": map[string]any{"amount": 90000, "currency": "VND"}},
			},
		}, h)

	if res.code < 400 {
		t.Fatalf("bút toán lệch được chấp nhận: HTTP %d — %s", res.code, res.raw)
	}

	if sau := demThatBai(t, metrics.StagePayment, "unbalanced"); sau <= truoc {
		t.Errorf("bút toán lệch KHÔNG được đếm dưới nhãn unbalanced "+
			"(%v → %v) — sự cố nghiêm trọng nhất của đường tiền không có "+
			"chỉ báo riêng", truoc, sau)
	}
}

// TestDemThatBaiGiaoHang: thao tác sai trạng thái ở đơn thực hiện phải
// được đếm dưới stage `fulfillment`.
func TestDemThatBaiGiaoHang(t *testing.T) {
	a := newAPITest(t)
	// DỰNG một đơn thực hiện thay vì tìm đơn có sẵn.
	//
	// Database test là khuôn sạch nên không có đơn nào; bản đầu của bài
	// này `t.Skip` và vì thế không kiểm gì trong mọi lần chạy bình thường
	// — xanh mà rỗng.
	maPhien := a.dungPhienSanHoanTat(emailMoi("demfo"), "0900666777")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	a.phatEvent(t)
	fo := a.timFulfillment(t, maDon)
	if fo.id == "" {
		t.Fatalf("không dựng được đơn thực hiện cho đơn %s", maDon)
	}
	foID, sellerID := fo.id, fo.sellerID

	tok := a.taoTokenNhaBan(t, sellerID)
	truoc := demThatBai(t, metrics.StageFulfillment, "invalid_status")

	// Báo ĐÃ GIAO khi đơn còn chưa bàn giao cho hãng vận chuyển — sai thứ
	// tự trạng thái, và là nhầm lẫn có thật của nhà bán.
	h := khoaIdem()
	h["Authorization"] = "Bearer " + tok
	res = a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/deliver",
		map[string]any{}, h)

	// 404 ở đây nghĩa là ĐƯỜNG DẪN sai, không phải đơn không tồn tại — và
	// bỏ qua nó (như bản đầu của bài này) sẽ che mất chính lỗi đó.
	if res.code == http.StatusNotFound {
		t.Fatalf("route không tồn tại: %s", res.raw)
	}
	if res.code < 400 {
		t.Fatalf("báo đã giao khi chưa bàn giao lại thành công: HTTP %d — %s\n"+
			"bài test cần một thao tác SAI TRẠNG THÁI để đo bộ đếm",
			res.code, res.raw)
	}

	if sau := demThatBai(t, metrics.StageFulfillment, "invalid_status"); sau <= truoc {
		t.Errorf("thao tác sai trạng thái KHÔNG được đếm (%v → %v) — "+
			"stage fulfillment khai trong metrics nhưng không ai ghi vào",
			truoc, sau)
	}
}

// taoTokenNhaBan tạo tài khoản SELLER_OWNER gắn với một gian hàng cụ thể.
func (a *apiTest) taoTokenNhaBan(t *testing.T, sellerID string) string {
	t.Helper()
	ctx := context.Background()

	email := emailMoi("nbdem")
	const matKhau = "MatKhauDuDai@2026"
	u, err := a.mods.identity.Register(ctx, identity.RegisterRequest{
		Email: email, Password: matKhau,
	})
	if err != nil {
		t.Fatalf("tạo tài khoản nhà bán: %v", err)
	}
	if err := a.mods.identity.GrantRole(
		ctx, u.ID, identity.RoleSellerOwner, sellerID); err != nil {
		t.Fatalf("cấp vai trò nhà bán: %v", err)
	}

	res := a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": matKhau}, nil)
	tok, _ := res.body["access_token"].(string)
	if tok == "" {
		t.Fatalf("đăng nhập nhà bán: %s", res.raw)
	}
	return tok
}
