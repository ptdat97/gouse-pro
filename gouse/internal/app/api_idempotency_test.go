package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Hợp đồng IDEMPOTENCY: đặc tả ⇄ máy chủ thật.
//
// # Vì sao bài này tồn tại
//
// `RequireIdempotencyKey` được bọc THEO TỪNG NHÓM TUYẾN, mỗi nhóm một
// quyết định riêng kèm lý do trong chú thích. Không có gì đối chiếu những
// quyết định ấy với đặc tả, và hai chiều lệch đều hỏng im lặng:
//
//	máy chủ BẮT · đặc tả không khai   client sinh từ đặc tả không gửi
//	                                  header → 400 ở mọi lệnh ghi, và
//	                                  thông báo không nói vì sao client
//	                                  phải biết tự sinh khóa
//	đặc tả khai · máy chủ KHÔNG bắt   client tưởng mình được bảo vệ. Một
//	                                  lần thử lại sau timeout tạo bản ghi
//	                                  THỨ HAI — với đơn hàng là đặt hai
//	                                  lần, với sổ cái là ghi tiền hai lần
//
// Chiều thứ hai nguy hiểm hơn: nó không có triệu chứng cho tới lúc mạng
// chập, và lúc ấy triệu chứng là tiền.
//
// # Vì sao DÒ MÁY CHỦ THẬT, không đọc mã
//
// Middleware bọc quanh một `*http.ServeMux` con, còn tuyến thì do module
// tự đăng ký vào mux ấy. Suy từ mã xem tuyến nào nằm trong chuỗi nào là
// một phép xấp xỉ, và một hàng rào xấp xỉ sẽ sai ở đúng chỗ khó thấy.
//
// Gửi thật một lệnh ghi THIẾU header rồi đọc câu trả lời thì không phải
// suy gì cả.

// noKeyCaught là các thao tác ghi CỐ Ý không bắt khóa idempotency.
//
// Mỗi dòng là một quyết định có người ký. Sổ này tồn tại để "không bắt"
// khác với "quên bọc".
var noKeyCaught = map[string]string{
	"POST /api/v1/auth/login": "trình duyệt gửi form đăng nhập, không phải " +
		"client tự sinh khóa. Đăng nhập hai lần chỉ tạo hai phiên, không " +
		"tạo dữ liệu trùng",
	"POST /api/v1/auth/refresh": "client tự động gọi khi token hết hạn; " +
		"refresh token đã DÙNG MỘT LẦN nên gửi trùng không có tác dụng phụ",
	"POST /api/v1/auth/logout": "đăng xuất vốn idempotent theo định nghĩa — " +
		"gọi lại trên một phiên đã đóng cho cùng kết quả",
	"POST /api/v1/auth/verify-email": "người bấm liên kết trong hộp thư đang " +
		"ở trình duyệt. Token vốn dùng một lần. Bắt có khóa ở đây là chặn " +
		"đúng người mà bước này tồn tại để giúp",
	"POST /api/v1/auth/verify-email/resend": "cùng lý do với verify-email; " +
		"có giới hạn tần suất riêng",
	"POST /api/v1/events": "tín hiệu hành vi gửi theo lô, bắn-và-quên. " +
		"Trùng một lô làm lệch số đếm phân tích, KHÔNG làm lệch tiền hay " +
		"tồn kho — và bắt khóa sẽ làm mọi lượt xem trang tốn một khóa",
	"POST /api/v1/webhooks/payment/{provider}": "bên gửi là cổng thanh toán, " +
		"họ không biết quy ước của ta. Chống trùng bằng định danh sự kiện " +
		"CỦA HỌ trong bảng webhook_event",
	"POST /api/v1/webhooks/shipping/{provider}": "cùng lý do với webhook " +
		"thanh toán",
}

// notMeasurable là các thao tác bài này CHƯA dò tới được.
//
// Ghi ra thay vì bỏ qua im lặng: một thao tác không dò được trông y hệt
// một thao tác đã dò và đạt.
var notMeasurable = map[string]string{
	"POST /api/v1/auth/refresh": "cần một refresh token còn hạn; lấy nó " +
		"đòi dựng cả vòng đăng nhập trong bài rà này. Đã khai lý do không " +
		"bắt khóa ở `khongBatKhoa`",
	"POST /api/v1/webhooks/payment/{provider}": "chặn ở lớp KIỂM CHỮ KÝ " +
		"trước khi tới idempotency. Dò được thì phải ký giả một webhook, " +
		"tức chép lại thuật toán ký — một bản sao sẽ lệch",
	"POST /api/v1/webhooks/shipping/{provider}": "cùng lý do với webhook " +
		"thanh toán",
}

// thanDo là thân request RIÊNG cho vài thao tác.
//
// Mặc định bài rà gửi `{}`. Vài đường trả 401 vì THÂN sai chứ không phải
// vì thiếu quyền — và 401 ấy không phân biệt được với 401 của lớp xác
// thực, nên phải cho chúng một thân đi lọt được.
var thanDo = map[string]map[string]any{}

// readUnimplementedOps đọc sổ `chuaCai` của `cmd/apicheck`.
//
// # Vì sao đọc FILE chứ không chép danh sách
//
// Một thao tác chưa có route thì trả 404, và bài rà này sẽ kết luận "máy
// chủ không bắt khóa" — một cảnh báo GIẢ. Cần biết thao tác nào chưa cài.
//
// Danh sách ấy đã tồn tại ở `cmd/apicheck/chua_cai.go`, nhưng nó nằm
// trong `package main` nên không import được. Chép lại sẽ tạo nguồn sự
// thật THỨ HAI, và hai danh sách cùng nội dung sớm muộn sẽ lệch — đúng
// thứ dự án này gặp với `GRAY`/`GREY`.
//
// Đọc chính file ấy thì chỉ có một nguồn. Đổi chỗ nó thì bài này đỏ ngay,
// chứ không im lặng bỏ gác.
func readUnimplementedOps(t *testing.T) map[string]bool {
	t.Helper()

	route := filepath.Join("..", "..", "cmd", "apicheck", "chua_cai.go")
	content, err := os.ReadFile(route)
	if err != nil {
		t.Fatalf("đọc sổ `chuaCai` của apicheck (%s): %v\n"+
			"Đổi chỗ file ấy thì phải sửa cả bài test này — nó cố ý đọc "+
			"nguồn sự thật DUY NHẤT thay vì chép lại danh sách.", route, err)
	}

	re := regexp.MustCompile(`"([A-Z]+ /api/v1/\S*)":`)
	ra := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(content), -1) {
		ra[m[1]] = true
	}
	if len(ra) == 0 {
		t.Fatal("không đọc được thao tác nào từ `chua_cai.go` — đổi định " +
			"dạng sổ thì phải sửa cả bài test này")
	}
	return ra
}

// reOperation tách `post:` / `patch:` trong file đặc tả.
var reOperation = regexp.MustCompile(`^(\s+)(post|patch):\s*$`)

type writeOps struct {
	method       string
	route        string // đường dẫn đặc tả, còn nguyên `{param}`
	declaredKeys bool
}

// reRef khớp `  /api/v1/x:` rồi `    $ref: \'./paths/y.yaml#/nhom\'`.
var (
	reAPIPath  = regexp.MustCompile(`^\s{2}(/api/v1/\S*):\s*$`)
	reGroupRef = regexp.MustCompile(`\$ref:\s*'\./paths/([^#]+)#/(\S+)'`)
	reGroup    = regexp.MustCompile(`^(\S+):\s*$`)
)

// readWriteOps đọc mọi POST/PATCH, kèm ĐƯỜNG DẪN THẬT.
//
// Đường dẫn nằm ở `openapi.yaml`, còn thân thao tác nằm ở `paths/*.yaml`
// dưới một khóa là TÊN NHÓM. Phải nối hai chỗ ấy lại — cùng cách
// `cmd/apicheck` làm.
//
// Đọc bằng THỤT LỀ chứ không bằng thư viện YAML: dự án có đúng bốn phụ
// thuộc Go trực tiếp, và thêm một cái chỉ để đọc đặc tả trong test là đổi
// một ràng buộc kiến trúc lấy một tiện lợi.
func readWriteOps(t *testing.T) []writeOps {
	t.Helper()
	goc := filepath.Join("..", "..", "api")

	// 1. đường dẫn -> (file, nhóm)
	type position struct{ file, group string }
	positionOf := map[string]position{}

	root, err := os.ReadFile(filepath.Join(goc, "openapi.yaml"))
	if err != nil {
		t.Fatalf("đọc openapi.yaml: %v", err)
	}
	line := strings.Split(string(root), "\n")
	for i, l := range line {
		m := reAPIPath.FindStringSubmatch(l)
		if m == nil || i+1 >= len(line) {
			continue
		}
		if r := reGroupRef.FindStringSubmatch(line[i+1]); r != nil {
			positionOf[m[1]] = position{file: r[1], group: r[2]}
		}
	}
	if len(positionOf) == 0 {
		t.Fatal("không đọc được đường dẫn nào từ openapi.yaml — đổi cấu trúc " +
			"đặc tả thì phải sửa cả bài test này")
	}

	// 2. (file, nhóm) -> thân các thao tác ghi
	than := map[position]map[string]string{}
	for _, v := range positionOf {
		if _, co := than[v]; co {
			continue
		}
		content, err := os.ReadFile(filepath.Join(goc, "paths", v.file))
		if err != nil {
			t.Fatalf("đọc %s: %v", v.file, err)
		}
		than[v] = writeOpBody(string(content), v.group)
	}

	var ra []writeOps
	for route, v := range positionOf {
		for pp, body := range than[v] {
			ra = append(ra, writeOps{
				method:       strings.ToUpper(pp),
				route:        route,
				declaredKeys: strings.Contains(body, "IdempotencyKey"),
			})
		}
	}
	sort.Slice(ra, func(i, j int) bool {
		return ra[i].method+ra[i].route < ra[j].method+ra[j].route
	})
	return ra
}

// writeOpBody trả thân của `post:` và `patch:` bên trong một nhóm.
func writeOpBody(content, group string) map[string]string {
	ra := map[string]string{}
	line := strings.Split(content, "\n")

	inGroup := false
	for i := 0; i < len(line); i++ {
		if m := reGroup.FindStringSubmatch(line[i]); m != nil {
			inGroup = m[1] == group
			continue
		}
		if !inGroup {
			continue
		}
		m := reOperation.FindStringSubmatch(line[i])
		if m == nil {
			continue
		}
		indent := len(m[1])

		var body []string
		j := i + 1
		for ; j < len(line); j++ {
			l := line[j]
			if strings.TrimSpace(l) != "" &&
				len(l)-len(strings.TrimLeft(l, " ")) <= indent {
				break
			}
			body = append(body, l)
		}
		ra[m[2]] = strings.Join(body, "\n")
		i = j - 1
	}
	return ra
}

// replaceParam đổi `{order_id}` thành một mã hợp lệ về CÚ PHÁP.
//
// Giá trị có tồn tại hay không KHÔNG quan trọng: middleware idempotency
// chạy TRƯỚC khi mux định tuyến tới handler, nên một mã không có thật vẫn
// cho ta biết máy chủ có bắt khóa hay không.
var reParam = regexp.MustCompile(`\{[a-z_]+\}`)

func replaceParam(route string) string {
	return reParam.ReplaceAllStringFunc(route, func(string) string {
		return ids.MustNew(ids.PrefixOrder).String()
	})
}

// Mọi POST/PATCH: máy chủ BẮT khóa ⇔ đặc tả KHAI khóa.
func TestTheIdempotencyContractMatchesTheSpec(t *testing.T) {
	a := newAPITest(t)

	// Ba danh tính, thử lần lượt cho tới khi qua được lớp xác thực.
	customerEmail := newEmail("idem")
	customer := a.registerAndLogIn(customerEmail)

	// `login` trả 401 với thân rỗng — và 401 ấy KHÔNG phân biệt được với
	// 401 của lớp xác thực, nên phải cho nó một thân đi lọt được.
	thanDo["POST /api/v1/auth/login"] = map[string]any{
		"email": customerEmail, "password": "MatKhauDuDai@2026",
	}
	shop := buildSeller(t, a, "idem"+ids.MustNew(ids.PrefixSeller).String()[22:])

	identity2 := []map[string]string{
		nil, // khách vãng lai
		bearer(customer),
		bearer(shop.token), // nhà bán
		bearer(a.createAccountWithRole(t, "ADMIN")), // quản trị
	}

	notImplemented := readUnimplementedOps(t)

	var drift, unmeasured []string
	for _, op := range readWriteOps(t) {
		key := op.method + " " + op.route
		if _, co := notMeasurable[key]; co {
			continue
		}
		// Thao tác CHƯA CÓ ROUTE thì 404, và mọi kết luận về idempotency
		// của nó là cảnh báo giả. `apicheck` đã gác việc hoãn ấy có lý do.
		if notImplemented[key] {
			continue
		}

		caught, measured := a.serverCatchesTheKey(t, op, identity2)
		if !measured {
			unmeasured = append(unmeasured, key)
			continue
		}

		_, declaredExemptions := noKeyCaught[key]
		switch {
		case caught && !op.declaredKeys:
			drift = append(drift, fmt.Sprintf(
				"%s: máy chủ BẮT khóa mà đặc tả KHÔNG khai — client sinh từ "+
					"đặc tả sẽ ăn 400 ở mọi lệnh ghi", key))

		case !caught && op.declaredKeys:
			drift = append(drift, fmt.Sprintf(
				"%s: đặc tả KHAI khóa mà máy chủ KHÔNG bắt — client tưởng "+
					"mình được bảo vệ, và một lần thử lại tạo bản ghi thứ hai",
				key))

		case !caught && !op.declaredKeys && !declaredExemptions:
			drift = append(drift, fmt.Sprintf(
				"%s: lệnh ghi KHÔNG có khóa idempotency và KHÔNG có lý do. "+
					"Thêm vào `khongBatKhoa` kèm lý do, hoặc bọc "+
					"`RequireIdempotencyKey`", key))

		case caught && declaredExemptions:
			drift = append(drift, fmt.Sprintf(
				"%s: có dòng trong `khongBatKhoa` mà máy chủ VẪN bắt khóa — "+
					"xóa dòng ấy đi, nó đang nói dối", key))
		}
	}

	if len(unmeasured) > 0 {
		t.Errorf("%d thao tác ghi không dò tới được (lớp xác thực chặn trước):"+
			"\n  %s\nThêm danh tính vào bài test, hoặc khai vào `chuaDoDuoc` "+
			"kèm lý do — một thao tác không dò được trông y hệt một thao tác "+
			"đã dò và đạt.", len(unmeasured), strings.Join(unmeasured, "\n  "))
	}
	if len(drift) > 0 {
		t.Errorf("%d thao tác lệch hợp đồng idempotency:\n  %s",
			len(drift), strings.Join(drift, "\n  "))
	}

	// Sổ miễn trừ trỏ tới thao tác không còn tồn tại cũng là một dòng nói
	// dối: nó im lặng ngừng gác trong khi vẫn trông như đang gác.
	co := map[string]bool{}
	for _, op := range readWriteOps(t) {
		co[op.method+" "+op.route] = true
	}
	for key := range noKeyCaught {
		if !co[key] {
			t.Errorf("`khongBatKhoa` còn dòng cho %q mà đặc tả không còn "+
				"thao tác ấy — xóa đi", key)
		}
	}
}

// serverCatchesTheKey gửi lệnh ghi THIẾU header và đọc câu trả lời.
//
// Trả `measured=false` khi mọi danh tính đều bị chặn ở lớp xác thực — lúc
// ấy ta không biết gì về idempotency và phải NÓI RA, không được đoán.
func (a *apiTest) serverCatchesTheKey(
	t *testing.T, op writeOps, identity2 []map[string]string,
) (caught, measured bool) {
	t.Helper()
	route := replaceParam(op.route)

	than := map[string]any{}
	if t2, co := thanDo[op.method+" "+op.route]; co {
		than = t2
	}

	for _, h := range identity2 {
		res := a.call(op.method, route, than, h)

		if strings.Contains(res.raw, "Idempotency-Key") {
			return true, true
		}
		if res.code == http.StatusUnauthorized || res.code == http.StatusForbidden {
			continue // thử danh tính mạnh hơn
		}
		// Đi qua được lớp xác thực mà không đòi khóa.
		return false, true
	}
	return false, false
}
