package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// Vòng đời đợt đối soát phải KHÉP: DRAFT → CONFIRMED → PAID.
//
// # P3-89 — hai hàm khai ra mà không ai gọi
//
// `DoiSoat.XacNhan` và `DoiSoat.DanhDauDaTra` có từ migration 000036, với đủ
// hàng rào trạng thái và test đơn vị. Tới 01/10/2026 **không lời gọi nào** —
// mọi đợt nằm `DRAFT` vĩnh viễn, và nhà bán không bao giờ được chi.
//
// Khép nó cần một quyết định không thuộc về mã: AI ĐƯỢC KÝ CHI. Quyết 01/10
// — hai bước, có người ký, mỗi bước ghi ai bấm và lúc nào.
//
// # Vì sao bài này đi qua HTTP
//
// Thứ đang kiểm là CHUỖI: token → vai trò → khóa idempotency → hàng rào
// trạng thái → vết kiểm toán cùng giao dịch → DTO. Gọi thẳng service bỏ qua
// bốn trong sáu mắt xích, và hai trong số đó (vai trò, idempotency) là lớp
// bảo vệ của một endpoint chuyển tiền.
func TestVongDoiDoiSoatKhepDuocKemNguoiKy(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	id, _ := a.doiSoatNhap(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleOpsFinance)

	const lyDo = "Đối soát kỳ tháng 9, đã khớp với báo cáo giao hàng"

	// ---- Bước 1: chốt ----
	res := a.call(http.MethodPost,
		"/api/v1/admin/settlements/"+id+"/confirm",
		map[string]any{"reason": lyDo}, hopNhat(khoaIdem(), bearer(tok)))
	if res.code != http.StatusOK {
		t.Fatalf("chốt đợt: HTTP %d — %s", res.code, res.raw)
	}

	d, _ := res.body["settlement"].(map[string]any)
	if tt, _ := d["status"].(string); tt != "CONFIRMED" {
		t.Fatalf("sau khi chốt: trạng thái = %q, cần CONFIRMED", tt)
	}
	nguoiChot, _ := d["confirmed_by"].(string)
	if nguoiChot == "" {
		t.Error("chốt xong mà KHÔNG ghi người ký — một cam kết với nhà bán " +
			"không ai chịu trách nhiệm")
	}
	if mocChot, _ := d["confirmed_at"].(string); mocChot == "" {
		t.Error("chốt xong mà không ghi mốc thời gian")
	}

	// ---- Bước 2: ghi nhận đã chuyển tiền ----
	res = a.call(http.MethodPost,
		"/api/v1/admin/settlements/"+id+"/mark-paid",
		map[string]any{"reason": lyDo}, hopNhat(khoaIdem(), bearer(tok)))
	if res.code != http.StatusOK {
		t.Fatalf("ghi nhận đã trả: HTTP %d — %s", res.code, res.raw)
	}

	d, _ = res.body["settlement"].(map[string]any)
	if tt, _ := d["status"].(string); tt != "PAID" {
		t.Fatalf("sau khi trả: trạng thái = %q, cần PAID", tt)
	}
	if nguoiTra, _ := d["paid_by"].(string); nguoiTra == "" {
		t.Fatal("tiền RỜI nền tảng mà không ghi ai ký — đây là bản ghi duy " +
			"nhất nói ai quyết định chuyển nó")
	}

	// ---- Vết kiểm toán: HAI bản ghi, hai Action khác nhau ----
	//
	// Tách `settlement.confirm` khỏi `settlement.mark_paid` chứ không gộp:
	// báo cáo kiểm toán cần phân biệt "ai chốt con số" khỏi "ai quyết định
	// chuyển tiền".
	var soVet int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(DISTINCT action) FROM audit_log
		 WHERE resource_type = 'SETTLEMENT' AND resource_id = $1
		   AND action IN ('settlement.confirm', 'settlement.mark_paid')`,
		id).Scan(&soVet); err != nil {
		t.Fatalf("đọc vết kiểm toán: %v", err)
	}
	if soVet != 2 {
		t.Errorf("vết kiểm toán có %d loại thao tác, cần 2 — ai chốt và ai "+
			"chuyển tiền phải tách được trong báo cáo", soVet)
	}
}

// Sai THỨ TỰ thì 409, không phải 400.
//
// Bên gọi gửi một request HỢP LỆ; chỉ là đợt đã ở trạng thái khác — thường
// vì người khác vừa bấm. 400 sẽ khiến giao diện bảo người dùng sửa dữ liệu,
// mà không có gì để sửa.
func TestKyDoiSoatSaiThuTuThi409(t *testing.T) {
	a := newAPITest(t)

	id, _ := a.doiSoatNhap(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleOpsFinance)
	const lyDo = "Đối soát kỳ tháng 9, đã khớp với báo cáo giao hàng"

	// Đánh dấu đã trả khi đợt còn DRAFT: nhảy bước.
	res := a.call(http.MethodPost,
		"/api/v1/admin/settlements/"+id+"/mark-paid",
		map[string]any{"reason": lyDo}, hopNhat(khoaIdem(), bearer(tok)))
	if res.code != http.StatusConflict {
		t.Fatalf("nhảy bước phải 409, nhận HTTP %d — %s", res.code, res.raw)
	}

	// Chốt HAI lần: lần hai cũng 409.
	if r := a.call(http.MethodPost, "/api/v1/admin/settlements/"+id+"/confirm",
		map[string]any{"reason": lyDo},
		hopNhat(khoaIdem(), bearer(tok))); r.code != http.StatusOK {
		t.Fatalf("chốt lần đầu: HTTP %d — %s", r.code, r.raw)
	}
	res = a.call(http.MethodPost, "/api/v1/admin/settlements/"+id+"/confirm",
		map[string]any{"reason": lyDo}, hopNhat(khoaIdem(), bearer(tok)))
	if res.code != http.StatusConflict {
		t.Fatalf("chốt lần hai phải 409, nhận HTTP %d — %s", res.code, res.raw)
	}
}

// LÝ DO bắt buộc, tối thiểu 20 ký tự.
//
// Với đường tiền, "vì sao" là thứ duy nhất phân biệt một lần chi chính đáng
// với một lần biển thủ — cùng lập luận đã viết cho bút toán điều chỉnh.
//
// Và khi bị từ chối, trạng thái đợt phải GIỮ NGUYÊN: một lời gọi bị chặn
// không được để lại nửa chừng.
func TestKyDoiSoatThieuLyDoThiTuChoi(t *testing.T) {
	a := newAPITest(t)

	id, _ := a.doiSoatNhap(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleOpsFinance)

	for _, lyDo := range []string{"", "ok", "đã kiểm"} {
		res := a.call(http.MethodPost,
			"/api/v1/admin/settlements/"+id+"/confirm",
			map[string]any{"reason": lyDo},
			hopNhat(khoaIdem(), bearer(tok)))
		if res.code < 400 {
			t.Fatalf("lý do %q phải bị từ chối, nhận HTTP %d — %s",
				lyDo, res.code, res.raw)
		}
	}

	if tt := a.trangThaiDoiSoat(t, id); tt != "DRAFT" {
		t.Errorf("bị từ chối rồi mà trạng thái thành %q, cần giữ DRAFT", tt)
	}
}

// Vai trò KHÔNG phải tài chính thì không ký được.
//
// `OPS_SUPPORT` xem được đơn hàng và xử lý khiếu nại, nhưng không chuyển
// tiền. Đó là nghĩa thực tế của "quyền ký chi tách khỏi quyền admin thường"
// trong một hệ phân quyền theo VAI TRÒ.
//
// 403 chứ không 404: người gọi ĐÃ xác thực, chỉ không đủ quyền.
func TestChiVaiTroTaiChinhKyDuocDoiSoat(t *testing.T) {
	a := newAPITest(t)

	id, _ := a.doiSoatNhap(t)
	const lyDo = "Đối soát kỳ tháng 9, đã khớp với báo cáo giao hàng"

	for _, vaiTro := range []string{
		identity.RoleOpsSupport,
		identity.RoleOpsMerchandising,
		identity.RoleOpsWarehouse,
	} {
		tok := a.taoTaiKhoanVaiTro(t, vaiTro)
		res := a.call(http.MethodPost,
			"/api/v1/admin/settlements/"+id+"/confirm",
			map[string]any{"reason": lyDo},
			hopNhat(khoaIdem(), bearer(tok)))
		if res.code != http.StatusForbidden {
			t.Errorf("vai trò %s ký được đợt đối soát: HTTP %d — %s",
				vaiTro, res.code, res.raw)
		}
	}

	if tt := a.trangThaiDoiSoat(t, id); tt != "DRAFT" {
		t.Errorf("vai trò không đủ quyền mà đổi được trạng thái thành %q", tt)
	}
}

// Nhà bán KHÔNG thấy định danh người ký.
//
// Hai mốc thời gian thì có — "đã xác nhận lúc" và "đã trả lúc" là hai câu
// nhà bán hỏi nhiều nhất. Nhưng nhân viên nào bấm nút là việc nội bộ, và
// định danh nhân viên lọt ra ngoài là một rò rỉ không cần thiết.
func TestNhaBanKhongThayNguoiKy(t *testing.T) {
	a := newAPITest(t)

	id, B := a.doiSoatNhap(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleOpsFinance)
	const lyDo = "Đối soát kỳ tháng 9, đã khớp với báo cáo giao hàng"

	if r := a.call(http.MethodPost, "/api/v1/admin/settlements/"+id+"/confirm",
		map[string]any{"reason": lyDo},
		hopNhat(khoaIdem(), bearer(tok))); r.code != http.StatusOK {
		t.Fatalf("chốt đợt: HTTP %d — %s", r.code, r.raw)
	}

	res := a.call(http.MethodGet, "/api/v1/seller/settlements/"+id, nil,
		bearer(B.token))
	if res.code != http.StatusOK {
		t.Fatalf("nhà bán xem đợt: HTTP %d — %s", res.code, res.raw)
	}

	if strings.Contains(res.raw, "confirmed_by") {
		t.Error("mặt của nhà bán trả `confirmed_by` — định danh nhân viên " +
			"không nên rời phạm vi quản trị")
	}

	d, _ := res.body["settlement"].(map[string]any)
	if moc, _ := d["confirmed_at"].(string); moc == "" {
		t.Error("nhà bán KHÔNG thấy mốc xác nhận — đó là câu họ hỏi nhiều " +
			"nhất về một đợt")
	}
}

// ------------------------------------------------------------ tiện ích

// doiSoatNhap dựng một nhà bán có tiền rút được và một đợt đối soát DRAFT.
func (a *apiTest) doiSoatNhap(t *testing.T) (string, nhaBanThu) {
	t.Helper()
	ctx := context.Background()

	B, rutDuoc := a.dungNhaBanCoTienRutDuoc(t)
	if rutDuoc <= 0 {
		t.Skipf("không dựng được số dư rút được (%v)", rutDuoc)
	}

	den := types.BayGio().Add(time.Minute)
	if _, err := a.mods.payment.TaoDoiSoatChoKy(
		ctx, den.Add(-7*24*time.Hour), den, 1000); err != nil {
		t.Fatalf("tạo đợt đối soát: %v", err)
	}

	var id string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT id FROM settlement
		 WHERE seller_id = $1 AND status = 'DRAFT'
		 ORDER BY created_at DESC LIMIT 1`, B.sellerID).Scan(&id); err != nil {
		t.Fatalf("đọc đợt đối soát vừa tạo: %v", err)
	}
	return id, B
}

func (a *apiTest) trangThaiDoiSoat(t *testing.T, id string) string {
	t.Helper()
	var tt string
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT status FROM settlement WHERE id = $1`, id).Scan(&tt); err != nil {
		t.Fatalf("đọc trạng thái đợt: %v", err)
	}
	return tt
}
