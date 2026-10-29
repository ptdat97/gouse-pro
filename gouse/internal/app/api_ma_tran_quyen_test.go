package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// permissionGroup mô tả MỘT nhóm đường dẫn và tập vai trò được vào.
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
type permissionGroup struct {
	groupName string

	// prefix khớp theo tiền tố đường dẫn.
	prefix string

	// role là tập vai trò ĐƯỢC vào. Mọi vai trò khác phải nhận 403.
	role []string
}

// permissionMatrix là bảng resource × vai trò.
//
// Thứ tự QUAN TRỌNG: khớp theo tiền tố dài nhất trước, vì
// `/api/v1/admin/ledger` cũng bắt đầu bằng `/api/v1/admin`.
var permissionMatrix = []permissionGroup{
	{"sổ cái — tài chính", "/api/v1/admin/ledger",
		[]string{identity.RoleAdmin, identity.RoleOpsFinance}},

	// Ký đợt đối soát — ĐƯỜNG TIỀN RA nhà bán (P3-89).
	//
	// CÙNG hai vai trò với sổ cái, và đó là hai vai trò duy nhất mà
	// `Role.RequiresTwoFactor()` trả true trong nhóm nhân viên. OPS_SUPPORT
	// và OPS_MERCHANDISING KHÔNG tới được — đó là nghĩa thực tế của "quyền
	// ký chi tách khỏi quyền admin thường" trong hệ phân quyền theo VAI TRÒ.
	{"ký đợt đối soát — tài chính", "/api/v1/admin/settlements",
		[]string{identity.RoleAdmin, identity.RoleOpsFinance}},

	{"hồ sơ nhà bán — vận hành hàng hóa", "/api/v1/admin/sellers",
		[]string{identity.RoleAdmin, identity.RoleOpsMerchandising}},

	// Duyệt hàng hóa — CÙNG nhóm với duyệt hồ sơ nhà bán, có lý do: cả
	// hai đều là quyết định "cho thứ này lên sàn hay không", và tách ra
	// hai vai trò khác nhau sẽ làm một người duyệt được gian hàng nhưng
	// không duyệt được hàng của chính gian hàng đó.
	{"duyệt sản phẩm — vận hành hàng hóa", "/api/v1/admin/products",
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

// eachRole là mọi vai trò đem ra thử. Vai trò KHÔNG có trong `role` của
// một nhóm thì phải bị nhóm đó từ chối.
var eachRole = []string{
	identity.RoleCustomer,
	identity.RoleSellerOwner,
	identity.RoleAdmin,
	identity.RoleOpsFinance,
	identity.RoleOpsMerchandising,
	identity.RoleOpsSupport,
	identity.RoleOpsWarehouse,
}

// TestThePermissionMatrixIsByResourceAndRole — việc mà backlog mục 2.9 để ngỏ:
// "một bảng liệt kê MỌI resource × MỌI vai trò, không phải rà theo trí nhớ".
//
// # Bài test sẵn có kiểm gì, và bỏ sót gì
//
// `TestAdminPathsBlockTheUnprivileged` thử đúng MỘT vai trò sai —
// CUSTOMER — trên mọi đường. Nó bắt được "quên bọc RequireRole", nhưng
// KHÔNG bắt được "bọc nhầm vai trò".
//
// Bọc nhầm là lỗi nguy hiểm hơn và khó thấy hơn: route tài chính bọc
// `RequireRole("ADMIN", "OPS_MERCHANDISING")` vẫn chặn khách, vẫn chặn nhà
// bán, vẫn xanh mọi test — trong khi nhân viên vận hành hàng hóa đọc được
// sổ cái toàn nền tảng.
//
// Bài này thử MỌI vai trò trên MỌI nhóm đường.
func TestThePermissionMatrixIsByResourceAndRole(t *testing.T) {
	a := newAPITest(t)

	// Một tài khoản cho mỗi vai trò, dùng lại cho mọi đường.
	token := map[string]string{}
	for _, vt := range eachRole {
		token[vt] = a.createAccountWithRole(t, vt)
	}

	for _, d := range pathsNeedingAuth() {
		group, co := groupOfPath(d.path)
		if !co {
			// Bắt ở bài phủ bên dưới, không lặp lại ở đây.
			continue
		}

		for _, vt := range eachRole {
			gotIn := isIn(group.role, vt)
			ten := d.method + " " + d.path + " · " + vt
			t.Run(ten, func(t *testing.T) {
				h := idemKey()
				h["Authorization"] = "Bearer " + token[vt]
				got := a.call(d.method, d.path, map[string]any{}, h)

				if gotIn {
					// KHÔNG khẳng định 200: thân request rỗng nên nhiều
					// đường trả 400/404/409. Điều cần khẳng định là nó đi
					// QUA được cửa phân quyền.
					if got.code == http.StatusForbidden {
						t.Errorf("vai trò %s BỊ TỪ CHỐI ở nhóm %q — "+
							"đúng ra phải vào được: HTTP %d %s",
							vt, group.groupName, got.code, got.raw)
					}
					return
				}

				if got.code != http.StatusForbidden {
					t.Errorf("vai trò %s VÀO ĐƯỢC nhóm %q — HTTP %d, cần 403: %s",
						vt, group.groupName, got.code, got.raw)
				}
			})
		}
	}
}

// TestEveryAuthenticatedPathIsInTheMatrix.
//
// Ma trận chỉ mạnh bằng phạm vi nó phủ. Một đường không rơi vào nhóm nào
// là một đường không ai kiểm theo vai trò — và nó sẽ là đường mới thêm,
// tức đường không ai nghĩ tới.
func TestEveryAuthenticatedPathIsInTheMatrix(t *testing.T) {
	var missingImg []string
	for _, d := range pathsNeedingAuth() {
		if _, co := groupOfPath(d.path); !co {
			missingImg = append(missingImg, d.method+" "+d.path)
		}
	}
	if len(missingImg) > 0 {
		t.Errorf("%d đường KHÔNG thuộc nhóm nào trong permissionMatrix — "+
			"thêm nhóm hoặc mở rộng tiền tố:\n  %s",
			len(missingImg), strings.Join(missingImg, "\n  "))
	}
}

// groupOfPath tìm nhóm khớp, ưu tiên TIỀN TỐ DÀI NHẤT.
func groupOfPath(path string) (permissionGroup, bool) {
	var tot permissionGroup
	var co bool
	for _, n := range permissionMatrix {
		if strings.HasPrefix(path, n.prefix) && len(n.prefix) > len(tot.prefix) {
			tot, co = n, true
		}
	}
	return tot, co
}

func isIn(ds []string, x string) bool {
	for _, v := range ds {
		if v == x {
			return true
		}
	}
	return false
}

// createAccountWithRole tạo tài khoản mang đúng MỘT vai trò rồi đăng nhập.
//
// Vai trò nhà bán cần PHẠM VI là một gian hàng; vai trò quản trị thì không.
func (a *apiTest) createAccountWithRole(t *testing.T, role string) string {
	t.Helper()
	tok, _ := a.createAccountWithRoleAndID(t, role)
	return tok
}

// createAccountWithRoleAndID trả cả TOKEN và MÃ NGƯỜI DÙNG.
//
// Bài test nào khẳng định trên nhật ký thao tác đều cần mã này: nhật ký là
// bảng dùng CHUNG, nên lọc theo hành động thôi sẽ đếm cả vết của bài khác.
func (a *apiTest) createAccountWithRoleAndID(
	t *testing.T, role string,
) (string, string) {
	t.Helper()
	ctx := context.Background()

	email := newEmail("vt-" + strings.ToLower(role))
	const password = "MatKhauDuDai@2026"

	u, err := a.mods.identity.Register(ctx, identity.RegisterRequest{
		Email: email, Password: password,
	})
	if err != nil {
		t.Fatalf("tạo tài khoản %s: %v", role, err)
	}

	if role != identity.RoleCustomer {
		scope := ""
		if role == identity.RoleSellerOwner || role == identity.RoleSellerStaff {
			scope = a.sampleSellerID(t)
		}
		if err := a.mods.identity.GrantRole(ctx, u.ID, role, scope); err != nil {
			t.Fatalf("cấp vai trò %s: %v", role, err)
		}
	}

	res := a.call(http.MethodPost, "/api/v1/auth/login",
		map[string]any{"email": email, "password": password}, nil)
	tok, _ := res.body["access_token"].(string)
	if tok == "" {
		t.Fatalf("đăng nhập %s: %s", role, res.raw)
	}
	return tok, u.ID
}

// sampleSellerID lấy một gian hàng có thật để làm phạm vi.
func (a *apiTest) sampleSellerID(t *testing.T) string {
	t.Helper()
	var id string
	if err := a.db.Pool().QueryRow(context.Background(),
		`SELECT id FROM seller LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("đọc gian hàng mẫu: %v", err)
	}
	return id
}
