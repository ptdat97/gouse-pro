package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// nhomQuyen mô tả MỘT nhóm đường dẫn và tập vai trò được vào.
//
// Nguồn của bảng này là tài liệu, không phải mã nguồn:
//
//	docs/09-operations/security.md mục 3 — ba tầng phân quyền
//	docs/04-modules/admin.md mục 2      — OPS_MERCHANDISING, OPS_SUPPORT
//	docs/06-api/api-domains.md          — sáu nhóm API
//
// Cố ý KHÔNG đọc `RequireRole` từ mã nguồn: một bài test đọc chính thứ nó
// kiểm thì luôn xanh. Bảng này nói "đáng lẽ phải thế nào", và test so nó
// với "thực tế đang thế nào".
type nhomQuyen struct {
	tenNhom string

	// tienTo khớp theo tiền tố đường dẫn.
	tienTo string

	// vaiTro là tập vai trò ĐƯỢC vào. Mọi vai trò khác phải nhận 403.
	vaiTro []string
}

// maTranQuyen là bảng resource × vai trò.
//
// Thứ tự QUAN TRỌNG: khớp theo tiền tố dài nhất trước, vì
// `/api/v1/admin/ledger` cũng bắt đầu bằng `/api/v1/admin`.
var maTranQuyen = []nhomQuyen{
	{"sổ cái — tài chính", "/api/v1/admin/ledger",
		[]string{identity.RoleAdmin, identity.RoleOpsFinance}},

	{"hồ sơ nhà bán — vận hành hàng hóa", "/api/v1/admin/sellers",
		[]string{identity.RoleAdmin, identity.RoleOpsMerchandising}},

	{"đơn hàng — hỗ trợ khách", "/api/v1/admin/orders",
		[]string{identity.RoleAdmin, identity.RoleOpsSupport}},

	{"nhật ký kiểm toán", "/api/v1/admin/audit-log",
		[]string{identity.RoleAdmin}},

	// Hẹp hơn mọi nhóm khác có lý do: những tham số này quyết định cách hệ
	// thống chấm điểm nhà bán. Người muốn lách một ngưỡng rất cần quyền
	// này, nên nó ở nhóm nhỏ nhất.
	{"cấu hình vận hành", "/api/v1/admin/config",
		[]string{identity.RoleAdmin}},

	{"kiểm kê tồn kho — vận hành kho", "/api/v1/admin/inventory",
		[]string{identity.RoleAdmin, identity.RoleOpsWarehouse}},

	{"hồ sơ khách — hỗ trợ khách", "/api/v1/admin/customers",
		[]string{identity.RoleAdmin, identity.RoleOpsSupport}},

	{"trung tâm người bán", "/api/v1/seller/",
		[]string{identity.RoleSellerOwner, identity.RoleSellerStaff}},
}

// moiVaiTro là mọi vai trò đem ra thử. Vai trò KHÔNG có trong `vaiTro` của
// một nhóm thì phải bị nhóm đó từ chối.
var moiVaiTro = []string{
	identity.RoleCustomer,
	identity.RoleSellerOwner,
	identity.RoleAdmin,
	identity.RoleOpsFinance,
	identity.RoleOpsMerchandising,
	identity.RoleOpsSupport,
	identity.RoleOpsWarehouse,
}

// TestMaTranQuyenTheoResourceVaVaiTro — việc mà backlog mục 2.9 để ngỏ:
// "một bảng liệt kê MỌI resource × MỌI vai trò, không phải rà theo trí nhớ".
//
// # Bài test sẵn có kiểm gì, và bỏ sót gì
//
// `TestDuongQuanTriChanNguoiKhongCoQuyen` thử đúng MỘT vai trò sai —
// CUSTOMER — trên mọi đường. Nó bắt được "quên bọc RequireRole", nhưng
// KHÔNG bắt được "bọc nhầm vai trò".
//
// Bọc nhầm là lỗi nguy hiểm hơn và khó thấy hơn: route tài chính bọc
// `RequireRole("ADMIN", "OPS_MERCHANDISING")` vẫn chặn khách, vẫn chặn nhà
// bán, vẫn xanh mọi test — trong khi nhân viên vận hành hàng hóa đọc được
// sổ cái toàn nền tảng.
//
// Bài này thử MỌI vai trò trên MỌI nhóm đường.
func TestMaTranQuyenTheoResourceVaVaiTro(t *testing.T) {
	a := newAPITest(t)

	// Một tài khoản cho mỗi vai trò, dùng lại cho mọi đường.
	token := map[string]string{}
	for _, vt := range moiVaiTro {
		token[vt] = a.taoTaiKhoanVaiTro(t, vt)
	}

	for _, d := range duongCanQuyen() {
		nhom, co := nhomCuaDuong(d.path)
		if !co {
			// Bắt ở bài phủ bên dưới, không lặp lại ở đây.
			continue
		}

		for _, vt := range moiVaiTro {
			duocVao := coTrong(nhom.vaiTro, vt)
			ten := d.method + " " + d.path + " · " + vt
			t.Run(ten, func(t *testing.T) {
				h := khoaIdem()
				h["Authorization"] = "Bearer " + token[vt]
				got := a.call(d.method, d.path, map[string]any{}, h)

				if duocVao {
					// KHÔNG khẳng định 200: thân request rỗng nên nhiều
					// đường trả 400/404/409. Điều cần khẳng định là nó đi
					// QUA được cửa phân quyền.
					if got.code == http.StatusForbidden {
						t.Errorf("vai trò %s BỊ TỪ CHỐI ở nhóm %q — "+
							"đúng ra phải vào được: HTTP %d %s",
							vt, nhom.tenNhom, got.code, got.raw)
					}
					return
				}

				if got.code != http.StatusForbidden {
					t.Errorf("vai trò %s VÀO ĐƯỢC nhóm %q — HTTP %d, cần 403: %s",
						vt, nhom.tenNhom, got.code, got.raw)
				}
			})
		}
	}
}

// TestMoiDuongCanQuyenDeuCoTrongMaTran.
//
// Ma trận chỉ mạnh bằng phạm vi nó phủ. Một đường không rơi vào nhóm nào
// là một đường không ai kiểm theo vai trò — và nó sẽ là đường mới thêm,
// tức đường không ai nghĩ tới.
func TestMoiDuongCanQuyenDeuCoTrongMaTran(t *testing.T) {
	var thieu []string
	for _, d := range duongCanQuyen() {
		if _, co := nhomCuaDuong(d.path); !co {
			thieu = append(thieu, d.method+" "+d.path)
		}
	}
	if len(thieu) > 0 {
		t.Errorf("%d đường KHÔNG thuộc nhóm nào trong maTranQuyen — "+
			"thêm nhóm hoặc mở rộng tiền tố:\n  %s",
			len(thieu), strings.Join(thieu, "\n  "))
	}
}

// nhomCuaDuong tìm nhóm khớp, ưu tiên TIỀN TỐ DÀI NHẤT.
func nhomCuaDuong(path string) (nhomQuyen, bool) {
	var tot nhomQuyen
	var co bool
	for _, n := range maTranQuyen {
		if strings.HasPrefix(path, n.tienTo) && len(n.tienTo) > len(tot.tienTo) {
			tot, co = n, true
		}
	}
	return tot, co
}

func coTrong(ds []string, x string) bool {
	for _, v := range ds {
		if v == x {
			return true
		}
	}
	return false
}

// taoTaiKhoanVaiTro tạo tài khoản mang đúng MỘT vai trò rồi đăng nhập.
//
// Vai trò nhà bán cần PHẠM VI là một gian hàng; vai trò quản trị thì không.
func (a *apiTest) taoTaiKhoanVaiTro(t *testing.T, vaiTro string) string {
	t.Helper()
	tok, _ := a.taoTaiKhoanVaiTroCoID(t, vaiTro)
	return tok
}

// taoTaiKhoanVaiTroCoID trả cả TOKEN và MÃ NGƯỜI DÙNG.
//
// Bài test nào khẳng định trên nhật ký thao tác đều cần mã này: nhật ký là
// bảng dùng CHUNG, nên lọc theo hành động thôi sẽ đếm cả vết của bài khác.
func (a *apiTest) taoTaiKhoanVaiTroCoID(
	t *testing.T, vaiTro string,
) (string, string) {
	t.Helper()
	ctx := context.Background()

	email := emailMoi("vt-" + strings.ToLower(vaiTro))
	const matKhau = "MatKhauDuDai@2026"

	u, err := a.mods.identity.Register(ctx, identity.RegisterRequest{
		Email: email, Password: matKhau,
	})
	if err != nil {
		t.Fatalf("tạo tài khoản %s: %v", vaiTro, err)
	}

	if vaiTro != identity.RoleCustomer {
		scope := ""
		if vaiTro == identity.RoleSellerOwner || vaiTro == identity.RoleSellerStaff {
			scope = a.mauSellerID(t)
		}
		if err := a.mods.identity.GrantRole(ctx, u.ID, vaiTro, scope); err != nil {
			t.Fatalf("cấp vai trò %s: %v", vaiTro, err)
		}
	}

	res := a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": matKhau}, nil)
	tok, _ := res.body["access_token"].(string)
	if tok == "" {
		t.Fatalf("đăng nhập %s: %s", vaiTro, res.raw)
	}
	return tok, u.ID
}

// mauSellerID lấy một gian hàng có thật để làm phạm vi.
func (a *apiTest) mauSellerID(t *testing.T) string {
	t.Helper()
	var id string
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT id FROM seller LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("đọc gian hàng mẫu: %v", err)
	}
	return id
}
