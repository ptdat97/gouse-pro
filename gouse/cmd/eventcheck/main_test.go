package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Bộ bài này kiểm CHÍNH CÔNG CỤ, không kiểm dự án.
//
// Một công cụ bỏ sót vi phạm còn nguy hiểm hơn không có công cụ: nó tạo
// cảm giác an toàn giả. Nên mỗi bài dựng một dự án GIẢ có đúng một lỗi
// rồi đòi công cụ tìm ra nó.
//
// Cùng khuôn với `cmd/apicheck/main_test.go`.

// duAn dựng cây thư mục tối thiểu mà eventcheck đọc được.
//
// `khai` là nội dung file hằng số của eventbus; `file` là các file Go
// khác, khóa là đường dẫn tương đối dưới `internal/`.
func project(t *testing.T, declared string, file map[string]string) string {
	t.Helper()
	root2 := t.TempDir()

	must := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	must(filepath.Join(root2, "internal", "platform", "eventbus", "event.go"),
		"package eventbus\n\n"+declared)
	for ten, content := range file {
		must(filepath.Join(root2, "internal", filepath.FromSlash(ten)), content)
	}

	// Hai file mà R4 đọc phải TỒN TẠI, nếu không `run()` dừng ở lỗi đọc
	// file và mọi bài kiểm R1/R2/R3 đỏ vì một lý do không liên quan.
	//
	// Sửa Ở ĐÂY, không ở từng bài: đã hai lần thêm một tầng kiểm mới làm
	// vỡ bộ test cũ vì dự án giả thiếu một file (archcheck thiếu
	// `docs/04-modules/`, apicheck thiếu `api/components/schemas.yaml`), và
	// cả hai lần cách sửa đúng là vá chỗ dựng dự án giả.
	//
	// Nội dung KHỚP NHAU, nên dự án giả mặc định không có vi phạm R4 — bài
	// nào muốn kiểm R4 thì tự ghi đè bằng `receiverSet`.
	must(filepath.Join(root2, "cmd", "worker", "main.go"), fakeWorker(defaultReceivers))
	must(filepath.Join(root2, "internal", "app", "api_ledger_test.go"),
		fakeTestSet(defaultReceivers))

	return root2
}

// defaultReceivers là bộ bên nhận của dự án giả khi bài test không quan tâm R4.
//
// KHÔNG để rỗng: công cụ ĐỎ khi không tìm thấy lời gọi `Subscribe` nào, và
// đó là hành vi đúng — một bộ bên nhận rỗng nghĩa là đã đổi chỗ đăng ký, chứ
// không phải "không có bên nhận nào".
var defaultReceivers = []string{"a.NewMotHandler"}

// receiverSet ghi lại hai file của R4 với hai danh sách bên nhận KHÁC nhau.
//
// Dùng sau `duAn` để dựng đúng tình huống R4 cần bắt.
func receiverSet(t *testing.T, root2 string, ofWorker, ofTest []string) {
	t.Helper()
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root2, "cmd", "worker", "main.go"), fakeWorker(ofWorker))
	write(filepath.Join(root2, "internal", "app", "api_ledger_test.go"),
		fakeTestSet(ofTest))
}

// fakeWorker dựng một `cmd/worker/main.go` đăng ký đúng các bên nhận cho
// trước. Tên dạng `goi.NewXxxHandler`.
func fakeWorker(receivers []string) string {
	var b strings.Builder
	b.WriteString("package main\n\nfunc run() {\n")
	for _, ten := range receivers {
		b.WriteString("\tbus.Subscribe(" + ten + "(m, log))\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// fakeTestSet dựng một `api_ledger_test.go` có hàm `registerReceivers`.
//
// Có THÊM một lời gọi Subscribe NGOÀI hàm ấy, để khẳng định công cụ chỉ đọc
// trong thân hàm: `phatEvent` và các bài test riêng cũng gọi `Subscribe`, và
// gộp chúng vào sẽ làm sổ miễn trừ vô nghĩa.
func fakeTestSet(receivers []string) string {
	var b strings.Builder
	b.WriteString("package app\n\nfunc (a *apiTest) registerReceivers() {\n")
	for _, ten := range receivers {
		b.WriteString("\tbus.Subscribe(" + ten + "(m, log))\n")
	}
	b.WriteString("}\n\nfunc (a *apiTest) rieng() {\n")
	b.WriteString("\tbus.Subscribe(rieng.NewChiOBaiNayHandler(m, log))\n")
	b.WriteString("}\n")
	return b.String()
}

const oneDeclaredEvent = `const (
	TypeOrderPaid = "order.paid"
)
`

// publishers dựng một file phát đúng một loại event.
func publishers(row string) string {
	return `package m

import "x/internal/platform/eventbus"

func Phat() {
	e, _ := eventbus.NewEvent(eventbus.` + row + `, "agg", "id", nil)
	_ = e
}
`
}

// listeners dựng một bên nhận nghe đúng một loại event.
func listeners(ten, row string) string {
	return `package m

import "x/internal/platform/eventbus"

type ` + ten + ` struct{}

func (h *` + ten + `) EventTypes() []string {
	return []string{eventbus.` + row + `}
}
`
}

// chay chạy công cụ với hai sổ RỖNG.
//
// Sổ rỗng có chủ ý: bài test đo phép đối chiếu, không đo nội dung sổ của
// dự án thật.
func runCheck(t *testing.T, root2 string, overrides ...map[string]string) *checker {
	t.Helper()
	c := &checker{
		root:           root2,
		noPublisher:    map[string]string{},
		noListener:     map[string]string{},
		onlyInWorker:   map[string]string{},
		declaredEvents: map[string]string{},
		publishedIn:    map[string][]string{},
		listenedIn:     map[string][]string{},
	}
	if len(overrides) > 0 && overrides[0] != nil {
		c.noPublisher = overrides[0]
	}
	if len(overrides) > 1 && overrides[1] != nil {
		c.noListener = overrides[1]
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

func TestAMatchingReceiverReportsNoViolation(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})

	if c := runCheck(t, root2); len(c.violations) != 0 {
		t.Fatalf("mong 0 vi phạm, nhận: %v", c.violations)
	}
}

// R1 — khai mà KHÔNG AI PHÁT.
//
// ĐÂY LÀ LỖI THẬT ngày 25/09/2026: bốn loại event ở đúng tình trạng này,
// trong đó `order.placed` được nhắc tên trong chú thích của hai module
// giải thích vì sao họ "không nghe order.placed".
func TestCatchesADeclaredEventNobodyPublishes(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})

	c := runCheck(t, root2)
	if len(c.violations) != 2 {
		t.Fatalf("mong 2 vi phạm (R1 và R3), nhận %d: %v", len(c.violations), c.violations)
	}
	merge := strings.Join(c.violations, "\n")
	for _, can := range []string{"R1", "R3", "order.paid"} {
		if !strings.Contains(merge, can) {
			t.Errorf("thiếu %q trong: %v", can, c.violations)
		}
	}
}

// R2 — có phát mà KHÔNG AI NGHE.
func TestCatchesAPublishedEventNobodyListensTo(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
	})

	c := runCheck(t, root2)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.HasPrefix(c.violations[0], "R2") {
		t.Fatalf("mong R2, nhận: %s", c.violations[0])
	}
}

// R3 nghiêm trọng nhất và KHÔNG có sổ miễn trừ.
//
// Khai lý do "không ai phát" KHÔNG được làm tắt R3: một bên nhận chờ một
// event không bao giờ tới vẫn là mã người ta tin là đang chạy.
func TestR3CannotBeSilencedByAnOverride(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})

	c := runCheck(t, root2, map[string]string{"order.paid": "cố ý chưa phát"}, nil)

	var coR3 bool
	for _, v := range c.violations {
		if strings.HasPrefix(v, "R3") {
			coR3 = true
		}
		if strings.HasPrefix(v, "R1") {
			t.Errorf("khai lý do rồi thì R1 phải im: %s", v)
		}
	}
	if !coR3 {
		t.Fatalf("R3 phải kêu dù đã khai lý do không phát: %v", c.violations)
	}
}

// Sổ miễn trừ trỏ tới event KHÔNG CÒN là một dòng nói dối.
//
// Nó im lặng ngừng gác trong khi vẫn trông như đang gác.
func TestCatchesADeadExemptionRow(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})

	c := runCheck(t, root2, map[string]string{"order.da_xoa": "lý do cũ"}, nil)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "order.da_xoa") {
		t.Fatalf("phải gọi tên dòng chết: %s", c.violations[0])
	}
}

// Chuỗi trong CHÚ THÍCH không phải một khai báo.
//
// Đọc AST chứ không grep. Nếu công cụ grep, mọi đoạn chú thích bàn về một
// event sẽ thành một khai báo — và cảnh báo giả làm người ta tắt hẳn phép
// kiểm.
func TestAStringInACommentIsNotADeclaration(t *testing.T) {
	declared := `// Tài liệu nhắc "order.da_bien_mat" ở đây, chỉ để giải thích.
const (
	TypeOrderPaid = "order.paid"
)
`
	root2 := project(t, declared, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})

	if c := runCheck(t, root2); len(c.violations) != 0 {
		t.Fatalf("chú thích không phải khai báo: %v", c.violations)
	}
}

// File `_test.go` KHÔNG tính là nơi phát.
//
// Một loại event chỉ phát trong test là một loại event production không
// bao giờ phát — đúng thứ công cụ này tìm. Tính nó vào sẽ làm hàng rào im
// lặng ngay khi ai đó viết một bài test cho event chết.
func TestAPublishingFileInTestsDoesNotCount(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat_test.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go":      listeners("Nghe", "TypeOrderPaid"),
	})

	c := runCheck(t, root2)
	merge := strings.Join(c.violations, "\n")
	if !strings.Contains(merge, "R1") {
		t.Fatalf("phát trong _test.go không được tính là phát: %v", c.violations)
	}
}

// Dự án THẬT phải sạch.
//
// Thiếu bài này thì công cụ có thể đúng mà không ai chạy nó lên thứ đáng
// chạy — cùng lý do với `TestTheRealProjectMatches` của apicheck.
func TestTheRealProjectIsClean(t *testing.T) {
	// `newChecker` chứ không dựng bằng tay: dựng bằng tay ở đây là một bản
	// sao thứ hai của danh sách sổ, và nó đã lệch một lần — thêm sổ của R4
	// làm bài này đỏ dù dự án thật sạch.
	c := newChecker("../..")
	if err := c.run(); err != nil {
		t.Fatalf("run trên repo thật: %v", err)
	}
	if len(c.violations) != 0 {
		t.Fatalf("repo có %d vi phạm hợp đồng event:\n%s",
			len(c.violations), strings.Join(c.violations, "\n"))
	}
}

// ------------------------------------------------- R4: bộ bên nhận

// R4 — worker đăng ký một bên nhận mà bộ của test KHÔNG.
//
// ĐÂY LÀ LỖI THẬT, tìm ra 01/10/2026: `inventory.NewReleaseHandler` — đường
// RA của kho — có ở worker và thiếu ở `registerReceivers` suốt nhiều tháng, nên
// mọi bài test tích hợp chạy với một hệ thống không nhả hàng về kho.
func TestCatchesAReceiverMissingFromTheTestSet(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})
	receiverSet(t, root2,
		[]string{"a.NewMotHandler", "inv.NewReleaseHandler"},
		[]string{"a.NewMotHandler"})

	c := runCheck(t, root2)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	for _, can := range []string{"R4", "inv.NewReleaseHandler", "registerReceivers"} {
		if !strings.Contains(c.violations[0], can) {
			t.Errorf("thiếu %q trong: %s", can, c.violations[0])
		}
	}
}

// R4 chiều NGƯỢC — bên nhận chỉ chạy trong test.
//
// Tệ hơn chiều kia: bài test xanh cho một đường KHÔNG chạy ở production, nên
// nó tạo ra lòng tin sai chứ chỉ không thiếu phép kiểm.
func TestCatchesAReceiverOnlyInTheTestSet(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})
	receiverSet(t, root2,
		[]string{"a.NewMotHandler"},
		[]string{"a.NewMotHandler", "b.NewChiTrongTestHandler"})

	c := runCheck(t, root2)
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "b.NewChiTrongTestHandler") {
		t.Errorf("không chỉ ra bên nhận chỉ có ở test: %s", c.violations[0])
	}
}

// Sổ miễn trừ TẮT được R4 — nhưng chỉ cho đúng bên nhận đã khai.
func TestAnOverrideCanSilenceR4(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})
	receiverSet(t, root2,
		[]string{"a.NewMotHandler", "sc.NewSignalHandler", "x.NewKhacHandler"},
		[]string{"a.NewMotHandler"})

	c := runWithR4Overrides(t, root2, map[string]string{
		"sc.NewSignalHandler": "module không nằm trong Modules của app",
	})

	// MỘT vi phạm còn lại: `x.NewKhacHandler` không có lý do nào.
	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "x.NewKhacHandler") {
		t.Errorf("sổ miễn trừ tắt sai bên nhận: %s", c.violations[0])
	}
}

// Lý do khai cho một bên nhận KHÔNG tồn tại là một sổ đã mục.
//
// Bên nhận đổi tên hoặc bị xóa, và dòng trong sổ ở lại — từ đó nó miễn trừ
// cho một cái tên không còn nghĩa gì, và lần sau có bên nhận THẬT thiếu thì
// không ai biết dòng ấy đang che cái gì.
func TestCatchesARottenR4Override(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})

	c := runWithR4Overrides(t, root2, map[string]string{
		"da.NewDoiTenHandler": "lý do cũ cho một tên không còn tồn tại",
	})

	if len(c.violations) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.violations), c.violations)
	}
	if !strings.Contains(c.violations[0], "da.NewDoiTenHandler") {
		t.Errorf("không chỉ ra dòng sổ đã mục: %s", c.violations[0])
	}
}

// Lời gọi Subscribe NGOÀI `registerReceivers` không được tính.
//
// `phatEvent` và các bài test riêng cũng gọi `Subscribe` cho bên nhận của
// riêng chúng — ví dụ `payment.NewCarrierCostHandler` kèm giá cố định.
// Tính cả chúng sẽ làm sổ miễn trừ vô nghĩa, vì mọi thứ đều "có ở test".
//
// `fakeTestSet` luôn ghi thêm `rieng.NewChiOBaiNayHandler` ngoài hàm ấy, nên
// nếu công cụ đọc cả file thì bài này đỏ ở chiều "chỉ có ở test".
func TestSubscribeOutsideAFunctionDoesNotCount(t *testing.T) {
	root2 := project(t, oneDeclaredEvent, map[string]string{
		"modules/a/phat.go": publishers("TypeOrderPaid"),
		"modules/b/nghe.go": listeners("Nghe", "TypeOrderPaid"),
	})

	if c := runCheck(t, root2); len(c.violations) != 0 {
		t.Fatalf("mong 0 vi phạm, nhận: %v", c.violations)
	}
}

// runWithR4Overrides chạy công cụ với sổ miễn trừ R4 cho trước.
func runWithR4Overrides(t *testing.T, root2 string, overrides map[string]string) *checker {
	t.Helper()
	c := &checker{
		root:           root2,
		noPublisher:    map[string]string{},
		noListener:     map[string]string{},
		onlyInWorker:   overrides,
		declaredEvents: map[string]string{},
		publishedIn:    map[string][]string{},
		listenedIn:     map[string][]string{},
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}
