package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// countProfileViewTraces đếm số dòng nhật ký `customer.view` cho một khách.
func (a *apiTest) countProfileViewTraces(t *testing.T, customerID string) int {
	t.Helper()
	var n int
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log
		  WHERE action = 'customer.view' AND resource_id = $1`,
		customerID).Scan(&n); err != nil {
		t.Fatalf("đếm vết: %v", err)
	}
	return n
}

// customerSample tạo một khách có thật rồi trả về mã hồ sơ.
func (a *apiTest) customerSample(t *testing.T) string {
	t.Helper()
	email := newEmail("khachmau")
	a.registerAndLogIn(email)

	var id string
	if err := a.db.Pool().QueryRow(context.Background(),
		// lower(): cột email có ràng buộc `email = lower(email)`, còn
		// newEmail sinh phần ULID bằng chữ HOA.
		`SELECT id FROM customer WHERE email = lower($1)`, email).Scan(&id); err != nil {
		t.Fatalf("đọc hồ sơ khách: %v", err)
	}
	return id
}

func (a *apiTest) viewCustomerProfile(
	t *testing.T, tok, customerID, reason string,
) reply {
	t.Helper()
	h := map[string]string{"Authorization": "Bearer " + tok}
	if reason != "" {
		h["X-Access-Reason"] = reason
	}
	return a.call(http.MethodGet, "/api/v1/admin/customers/"+customerID, nil, h)
}

// TestViewingACustomerProfileLeavesATrail — đường cơ bản, và lời hứa của đặc tả:
// "Mọi lần gọi endpoint này đều ghi audit log".
func TestViewingACustomerProfileLeavesATrail(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsSupport)
	buyerID := a.customerSample(t)

	before := a.countProfileViewTraces(t, buyerID)

	res := a.viewCustomerProfile(t, tok, buyerID,
		"xử lý khiếu nại đơn ORD-12345, khách báo giao thiếu hàng")
	if res.code != http.StatusOK {
		t.Fatalf("xem hồ sơ: HTTP %d — %s", res.code, res.raw)
	}

	if after := a.countProfileViewTraces(t, buyerID); after != before+1 {
		t.Errorf("sau một lần xem, số vết là %d, cần %d — "+
			"một lần đọc dữ liệu cá nhân KHÔNG có dấu", after, before+1)
	}

	if written, _ := res.body["audit_logged"].(bool); !written {
		t.Errorf("audit_logged không phải true: %s", res.raw)
	}
}

// TestViewingACustomerProfileDemandsAReason: thiếu lý do, hoặc lý do rác, thì KHÔNG được đọc.
//
// Một trường lý do trống làm toàn bộ nhật ký truy cập trở nên vô dụng: nó
// ghi lại rằng có người đã xem, nhưng không trả lời được câu hỏi duy nhất
// đáng hỏi khi điều tra — xem để làm gì.
func TestViewingACustomerProfileDemandsAReason(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsSupport)
	buyerID := a.customerSample(t)

	for _, reason := range []string{"", "xem", "test test test test"} {
		before := a.countProfileViewTraces(t, buyerID)

		res := a.viewCustomerProfile(t, tok, buyerID, reason)
		if res.code != http.StatusBadRequest {
			t.Errorf("lý do %q được chấp nhận: HTTP %d — %s",
				reason, res.code, res.raw)
		}

		// Từ chối rồi thì KHÔNG được để lại vết giả: một dòng nhật ký cho
		// lần đọc chưa từng xảy ra làm sai lệch chính cuộc điều tra sau này.
		if after := a.countProfileViewTraces(t, buyerID); after != before {
			t.Errorf("lý do %q bị từ chối nhưng vẫn ghi %d vết",
				reason, after-before)
		}
	}
}

// TestTheReasonLengthThresholdMatchesTheSpec ghim ngưỡng độ dài lý do.
//
// Đặc tả ban đầu ghi 10 ký tự trong khi `audit.ValidateReason` cưỡng chế
// 20 — API sẽ từ chối một lý do 15 ký tự trong lúc tài liệu hứa 10 là đủ.
// Kiểu lệch này không làm hỏng test nào và chỉ lộ ra khi người tích hợp
// gặp lỗi 400 không giải thích được.
//
// Bài này đo ở ranh giới: 19 ký tự phải trượt, 20 ký tự phải qua.
func TestTheReasonLengthThresholdMatchesTheSpec(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsSupport)
	buyerID := a.customerSample(t)

	// 19 ký tự — dưới ngưỡng đúng một bậc.
	if res := a.viewCustomerProfile(t, tok, buyerID, "khieu nai don ORD12"); res.code !=
		http.StatusBadRequest {
		t.Errorf("lý do 19 ký tự được chấp nhận: HTTP %d — %s",
			res.code, res.raw)
	}

	// 20 ký tự — vừa đủ.
	if res := a.viewCustomerProfile(t, tok, buyerID, "khieu nai don ORD123"); res.code !=
		http.StatusOK {
		t.Errorf("lý do 20 ký tự bị từ chối: HTTP %d — %s", res.code, res.raw)
	}
}

// TestViewingACustomerProfileLeaksNoInternalData: response chỉ mang những trường đặc tả
// khai. Không có mật khẩu, không có mã người dùng, không có tổng chi tiêu.
func TestViewingACustomerProfileLeaksNoInternalData(t *testing.T) {
	a := newAPITest(t)
	tok := a.createAccountWithRole(t, identity.RoleOpsSupport)
	buyerID := a.customerSample(t)

	res := a.viewCustomerProfile(t, tok, buyerID,
		"đối chiếu thông tin liên hệ theo yêu cầu của khách")
	if res.code != http.StatusOK {
		t.Fatalf("xem hồ sơ: HTTP %d — %s", res.code, res.raw)
	}

	kh, _ := res.body["customer"].(map[string]any)
	allowed := map[string]bool{
		"id": true, "name": true, "email": true,
		"phone": true, "tier": true, "created_at": true,
	}
	for k := range kh {
		if !allowed[k] {
			t.Errorf("trường %q không nằm trong đặc tả — "+
				"hồ sơ khách trả nhiều hơn cần thiết: %s", k, res.raw)
		}
	}
}

// TestViewingACustomerProfileWithoutAnAuditPathIsRejected.
//
// Đây là mặt còn lại của lời hứa, và là mặt dễ mất. Nếu ghi vết hỏng mà vẫn
// trả dữ liệu, thì "mọi lần gọi đều ghi audit log" là một câu nói dối —
// đúng vào lúc nó quan trọng nhất.
//
// Dựng lại bằng cách xóa quyền ghi bảng audit_log trong một giao dịch riêng
// là quá nặng; thay vào đó đo ở tầng thấp hơn: gọi service với cổng ghi vết
// bỏ trống.
func TestViewingACustomerProfileWithoutAnAuditPathIsRejected(t *testing.T) {
	a := newAPITest(t)
	buyerID := a.customerSample(t)

	// Bỏ hẳn bảng nhật ký khỏi tầm với: lệnh ghi sẽ hỏng, và đó chính là
	// tình huống cần kiểm.
	ctx := context.Background()
	if _, err := a.db.Pool().Exec(ctx,
		`ALTER TABLE audit_log RENAME TO audit_log_tam`); err != nil {
		t.Fatalf("đổi tên bảng nhật ký: %v", err)
	}
	t.Cleanup(func() {
		if _, err := a.db.Pool().Exec(context.Background(),
			`ALTER TABLE audit_log_tam RENAME TO audit_log`); err != nil {
			t.Fatalf("khôi phục bảng nhật ký: %v", err)
		}
	})

	tok := a.createAccountWithRole(t, identity.RoleOpsSupport)
	res := a.viewCustomerProfile(t, tok, buyerID,
		"xử lý khiếu nại đơn ORD-99999, khách báo sai địa chỉ")

	if res.code == http.StatusOK {
		t.Fatalf("ghi vết hỏng mà vẫn trả hồ sơ khách — "+
			"lời hứa \"mọi lần gọi đều ghi audit log\" là nói dối: %s",
			res.raw)
	}
}
