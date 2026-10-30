package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestGuestOrderHistoryMergesAfterVerification khóa TOÀN CHUỖI của P3-15, ở mức
// HTTP thật.
//
// # Ngõ cụt trước 07/09
//
// Khách đặt hàng vãng lai bằng email X rồi KHÔNG đăng ký được bằng chính
// email đó. Từ chối là đúng khi chưa có cách chứng minh quyền sở hữu
// email — đơn hàng chứa địa chỉ nhà và số điện thoại người nhận — nhưng
// nó để khách không có đường nào đi tiếp.
//
// # Điều bài này bắt được mà test đơn vị không bắt
//
// Lịch sử vãng lai KHÔNG nằm ở hồ sơ khách hàng; nó nằm ở các ĐƠN với
// `guest_email` và `customer_id` rỗng. Đo trên database phát triển 07/09:
// 0 hồ sơ vãng lai, 3150 đơn vãng lai.
//
// Nghĩa là một cài đặt chỉ gắn HỒ SƠ vào tài khoản sẽ chạy trơn tru, mọi
// test đơn vị xanh, và khách vẫn không thấy đơn cũ nào.
func TestGuestOrderHistoryMergesAfterVerification(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const email = "vanglai-gop@apitest.local"
	const phone = "0900777222"

	// --- Khách VÃNG LAI đặt một đơn ---
	checkoutID := a.buildCheckoutReadyToComplete(email, phone)
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}

	countOwnerless := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := a.db.Pool().QueryRow(ctx,
			`SELECT count(*) FROM "order" WHERE guest_email = $1 AND customer_id = ''`,
			email).Scan(&n); err != nil {
			t.Fatalf("đếm đơn vô chủ: %v", err)
		}
		return n
	}
	if countOwnerless(t) == 0 {
		t.Fatal("không có đơn vãng lai nào — bài test không kiểm được gì")
	}

	// --- Đăng ký bằng CHÍNH email đó ---
	dk := a.call(http.MethodPost, "/api/v1/auth/register",
		map[string]any{"email": email, "password": "MatKhauDuDai@2026"}, idemKey())
	if dk.code != http.StatusCreated {
		t.Fatalf("đăng ký: HTTP %d — %s", dk.code, dk.raw)
	}
	if can, _ := dk.body["email_verification_required"].(bool); !can {
		t.Error("email_verification_required = false — email này CÓ đơn " +
			"vãng lai, giao diện phải biết còn một bước nữa")
	}

	// ĐIỀU QUAN TRỌNG NHẤT: đăng ký xong, lịch sử VẪN bị che.
	before := countOwnerless(t)
	if before == 0 {
		t.Fatal("đơn cũ đã về tài khoản NGAY LÚC ĐĂNG KÝ — chưa ai chứng " +
			"minh quyền sở hữu email")
	}

	// --- Token SAI không gộp được ---
	wrong := a.call(http.MethodPost, "/api/v1/auth/verify-email",
		map[string]any{"token": "token-bia-dat-khong-ton-tai"}, nil)
	if wrong.code == http.StatusOK {
		t.Error("token bịa đặt vẫn xác minh được")
	}
	if countOwnerless(t) != before {
		t.Error("token SAI vẫn kéo được đơn cũ về tài khoản")
	}

	// --- Token ĐÚNG, lấy từ chính thân thư đã gửi ---
	var than string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT body FROM notification_log
		 WHERE recipient = $1 AND template = 'email_verification'
		 ORDER BY created_at DESC LIMIT 1`, email).Scan(&than); err != nil {
		t.Fatalf("không có thư xác minh nào được gửi: %v", err)
	}
	tok := extractToken(t, than)

	xm := a.call(http.MethodPost, "/api/v1/auth/verify-email",
		map[string]any{"token": tok}, nil)
	if xm.code != http.StatusOK {
		t.Fatalf("xác minh: HTTP %d — %s", xm.code, xm.raw)
	}
	if n, _ := xm.body["orders_merged"].(float64); int(n) != before {
		t.Errorf("orders_merged = %v, mong %d — số đơn phải nói ra để khách "+
			"kiểm chứng được, không chỉ true/false", xm.body["orders_merged"], before)
	}
	if after := countOwnerless(t); after != 0 {
		t.Errorf("còn %d đơn vô chủ sau khi xác minh — lịch sử chưa về đúng chủ", after)
	}

	// --- Token DÙNG MỘT LẦN ---
	again := a.call(http.MethodPost, "/api/v1/auth/verify-email",
		map[string]any{"token": tok}, nil)
	if again.code == http.StatusOK {
		t.Error("token dùng được LẦN HAI — liên kết trong hộp thư còn sống mãi")
	}
}

// extractToken lấy token nguyên văn ra khỏi thân thư.
//
// Đọc từ THƯ chứ không từ bảng token: database chỉ giữ bản BĂM, và đó là
// điều bài test này gián tiếp chứng minh — không có đường nào lấy lại bản
// nguyên văn ngoài chính thư đã gửi.
func extractToken(t *testing.T, than string) string {
	t.Helper()
	for _, line := range strings.Split(than, "\n") {
		line = strings.TrimSpace(line)
		if len(line) >= 40 && !strings.ContainsAny(line, " :.") {
			return line
		}
	}
	t.Fatalf("không tách được token từ thư: %q", than)
	return ""
}

// TestGuiLaiLienKetCanDANGNHAP khóa lý do đường này khác `verify-email`.
//
// # Hai đường, hai thứ nhận vào
//
//	verify-email   nhận TOKEN — bản thân nó đã là bằng chứng, nên không
//	               cần đăng nhập (người bấm liên kết thường ở trình duyệt
//	               khác hoặc trên điện thoại)
//	resend         nếu nhận EMAIL thì bất kỳ ai cũng hỏi được "địa chỉ này
//	               có tài khoản chưa" qua việc phản hồi khác nhau
//
// Lấy danh tính từ TOKEN ĐĂNG NHẬP thì không có gì để dò: người gọi đã
// chứng minh mình là ai, và chỉ gửi lại được cho chính mình.
func TestResendingTheLinkRequiresALogin(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	const email = "guilai-lienket@apitest.local"
	const phone = "0900777333"

	// Khách vãng lai đặt một đơn, rồi đăng ký bằng chính email đó.
	checkoutID := a.buildCheckoutReadyToComplete(email, phone)
	if res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey()); res.code != http.StatusOK &&
		res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	if dk := a.call(http.MethodPost, "/api/v1/auth/register",
		map[string]any{"email": email, "password": "MatKhauDuDai@2026"},
		idemKey()); dk.code != http.StatusCreated {
		t.Fatalf("đăng ký: HTTP %d — %s", dk.code, dk.raw)
	}

	countTokens := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := a.db.Pool().QueryRow(ctx, `
			SELECT count(*) FROM email_verification_token t
			  JOIN "user" u ON u.id = t.user_id
			 WHERE u.email = $1 AND t.used_at IS NULL`, email).Scan(&n); err != nil {
			t.Fatalf("đếm token: %v", err)
		}
		return n
	}

	// PHÒNG VỆ HAI LỚP, và cần biết điều đó khi đọc bài này:
	//
	//	middleware   `httpserver.Auth` chặn request không có token hợp lệ
	//	handler      `ac.UserID == ""` chặn khi ngữ cảnh không có danh tính
	//
	// Bỏ RIÊNG lớp nào thì bài test VẪN XANH — lớp kia bắt được. Phải bỏ
	// CẢ HAI mới thấy đỏ, và khi đó là 500 chứ không phải "gửi lại được":
	// lớp sâu hơn (`ids.Parse` trên chuỗi rỗng) vẫn không cho đi tiếp.
	//
	// Ghi lại vì nó dễ dẫn tới kết luận sai theo cả hai chiều: tưởng một
	// lớp là đủ nên gỡ lớp kia, hoặc tưởng bài test vô dụng vì phá một lớp
	// mà vẫn xanh.

	// KHÔNG đăng nhập → từ chối, và KHÔNG phát token nào.
	before := countTokens(t)
	missing := a.call(http.MethodPost, "/api/v1/auth/verify-email/resend", nil, nil)
	if missing.code != http.StatusUnauthorized {
		t.Errorf("HTTP %d, cần 401 — không đăng nhập mà gửi lại được thì "+
			"endpoint này là công cụ dò danh sách email: %s", missing.code, missing.raw)
	}
	if countTokens(t) != before {
		t.Error("lời gọi KHÔNG đăng nhập vẫn phát token — thư rác gửi vào " +
			"hộp thư của người khác")
	}

	// Đăng nhập rồi gửi lại → được.
	dn := a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": "MatKhauDuDai@2026"}, nil)
	tok, _ := dn.body["access_token"].(string)
	if tok == "" {
		t.Fatalf("đăng nhập: %s", dn.raw)
	}

	got := a.call(http.MethodPost, "/api/v1/auth/verify-email/resend", nil,
		map[string]string{"Authorization": "Bearer " + tok})
	if got.code != http.StatusOK {
		t.Fatalf("gửi lại: HTTP %d — %s", got.code, got.raw)
	}

	// Vẫn ĐÚNG MỘT token còn sống: bản mới giết bản cũ.
	//
	// Người ta bấm "gửi lại" chính vì nghi ngờ thư cũ; để hai liên kết
	// cùng sống là làm ngược điều họ vừa yêu cầu.
	if n := countTokens(t); n != 1 {
		t.Errorf("còn %d token chưa dùng, mong đúng 1 — liên kết cũ vẫn "+
			"sống sau khi đã gửi lại", n)
	}
}
