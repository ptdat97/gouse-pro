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
func duAn(t *testing.T, khai string, file map[string]string) string {
	t.Helper()
	goc := t.TempDir()

	must := func(duong, noiDung string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(duong), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(duong, []byte(noiDung), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	must(filepath.Join(goc, "internal", "platform", "eventbus", "event.go"),
		"package eventbus\n\n"+khai)
	for ten, noiDung := range file {
		must(filepath.Join(goc, "internal", filepath.FromSlash(ten)), noiDung)
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
	// nào muốn kiểm R4 thì tự ghi đè bằng `boBenNhan`.
	must(filepath.Join(goc, "cmd", "worker", "main.go"), workerGia(benNhanMacDinh))
	must(filepath.Join(goc, "internal", "app", "api_ledger_test.go"),
		boTestGia(benNhanMacDinh))

	return goc
}

// benNhanMacDinh là bộ bên nhận của dự án giả khi bài test không quan tâm R4.
//
// KHÔNG để rỗng: công cụ ĐỎ khi không tìm thấy lời gọi `Subscribe` nào, và
// đó là hành vi đúng — một bộ bên nhận rỗng nghĩa là đã đổi chỗ đăng ký, chứ
// không phải "không có bên nhận nào".
var benNhanMacDinh = []string{"a.NewMotHandler"}

// boBenNhan ghi lại hai file của R4 với hai danh sách bên nhận KHÁC nhau.
//
// Dùng sau `duAn` để dựng đúng tình huống R4 cần bắt.
func boBenNhan(t *testing.T, goc string, cuaWorker, cuaTest []string) {
	t.Helper()
	ghi := func(duong, noiDung string) {
		t.Helper()
		if err := os.WriteFile(duong, []byte(noiDung), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ghi(filepath.Join(goc, "cmd", "worker", "main.go"), workerGia(cuaWorker))
	ghi(filepath.Join(goc, "internal", "app", "api_ledger_test.go"),
		boTestGia(cuaTest))
}

// workerGia dựng một `cmd/worker/main.go` đăng ký đúng các bên nhận cho
// trước. Tên dạng `goi.NewXxxHandler`.
func workerGia(benNhan []string) string {
	var b strings.Builder
	b.WriteString("package main\n\nfunc run() {\n")
	for _, ten := range benNhan {
		b.WriteString("\tbus.Subscribe(" + ten + "(m, log))\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// boTestGia dựng một `api_ledger_test.go` có hàm `dangKyBenNhan`.
//
// Có THÊM một lời gọi Subscribe NGOÀI hàm ấy, để khẳng định công cụ chỉ đọc
// trong thân hàm: `phatEvent` và các bài test riêng cũng gọi `Subscribe`, và
// gộp chúng vào sẽ làm sổ miễn trừ vô nghĩa.
func boTestGia(benNhan []string) string {
	var b strings.Builder
	b.WriteString("package app\n\nfunc (a *apiTest) dangKyBenNhan() {\n")
	for _, ten := range benNhan {
		b.WriteString("\tbus.Subscribe(" + ten + "(m, log))\n")
	}
	b.WriteString("}\n\nfunc (a *apiTest) rieng() {\n")
	b.WriteString("\tbus.Subscribe(rieng.NewChiOBaiNayHandler(m, log))\n")
	b.WriteString("}\n")
	return b.String()
}

const khaiMotEvent = `const (
	TypeOrderPaid = "order.paid"
)
`

// benPhat dựng một file phát đúng một loại event.
func benPhat(hang string) string {
	return `package m

import "x/internal/platform/eventbus"

func Phat() {
	e, _ := eventbus.NewEvent(eventbus.` + hang + `, "agg", "id", nil)
	_ = e
}
`
}

// benNghe dựng một bên nhận nghe đúng một loại event.
func benNghe(ten, hang string) string {
	return `package m

import "x/internal/platform/eventbus"

type ` + ten + ` struct{}

func (h *` + ten + `) EventTypes() []string {
	return []string{eventbus.` + hang + `}
}
`
}

// chay chạy công cụ với hai sổ RỖNG.
//
// Sổ rỗng có chủ ý: bài test đo phép đối chiếu, không đo nội dung sổ của
// dự án thật.
func chay(t *testing.T, goc string, so ...map[string]string) *checker {
	t.Helper()
	c := &checker{
		root:       goc,
		khongPhat:  map[string]string{},
		khongNghe:  map[string]string{},
		chiOWorker: map[string]string{},
		khaiEvent:  map[string]string{},
		noiPhat:    map[string][]string{},
		noiNghe:    map[string][]string{},
	}
	if len(so) > 0 && so[0] != nil {
		c.khongPhat = so[0]
	}
	if len(so) > 1 && so[1] != nil {
		c.khongNghe = so[1]
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

func TestKhopThiKhongBaoViPham(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})

	if c := chay(t, goc); len(c.viPham) != 0 {
		t.Fatalf("mong 0 vi phạm, nhận: %v", c.viPham)
	}
}

// R1 — khai mà KHÔNG AI PHÁT.
//
// ĐÂY LÀ LỖI THẬT ngày 25/09/2026: bốn loại event ở đúng tình trạng này,
// trong đó `order.placed` được nhắc tên trong chú thích của hai module
// giải thích vì sao họ "không nghe order.placed".
func TestBatEventKhaiMaKhongAiPhat(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})

	c := chay(t, goc)
	if len(c.viPham) != 2 {
		t.Fatalf("mong 2 vi phạm (R1 và R3), nhận %d: %v", len(c.viPham), c.viPham)
	}
	gop := strings.Join(c.viPham, "\n")
	for _, can := range []string{"R1", "R3", "order.paid"} {
		if !strings.Contains(gop, can) {
			t.Errorf("thiếu %q trong: %v", can, c.viPham)
		}
	}
}

// R2 — có phát mà KHÔNG AI NGHE.
func TestBatEventPhatMaKhongAiNghe(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
	})

	c := chay(t, goc)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.HasPrefix(c.viPham[0], "R2") {
		t.Fatalf("mong R2, nhận: %s", c.viPham[0])
	}
}

// R3 nghiêm trọng nhất và KHÔNG có sổ miễn trừ.
//
// Khai lý do "không ai phát" KHÔNG được làm tắt R3: một bên nhận chờ một
// event không bao giờ tới vẫn là mã người ta tin là đang chạy.
func TestR3KhongTatDuocBangSoMienTru(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})

	c := chay(t, goc, map[string]string{"order.paid": "cố ý chưa phát"}, nil)

	var coR3 bool
	for _, v := range c.viPham {
		if strings.HasPrefix(v, "R3") {
			coR3 = true
		}
		if strings.HasPrefix(v, "R1") {
			t.Errorf("khai lý do rồi thì R1 phải im: %s", v)
		}
	}
	if !coR3 {
		t.Fatalf("R3 phải kêu dù đã khai lý do không phát: %v", c.viPham)
	}
}

// Sổ miễn trừ trỏ tới event KHÔNG CÒN là một dòng nói dối.
//
// Nó im lặng ngừng gác trong khi vẫn trông như đang gác.
func TestBatDongMienTruDaChet(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})

	c := chay(t, goc, map[string]string{"order.da_xoa": "lý do cũ"}, nil)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "order.da_xoa") {
		t.Fatalf("phải gọi tên dòng chết: %s", c.viPham[0])
	}
}

// Chuỗi trong CHÚ THÍCH không phải một khai báo.
//
// Đọc AST chứ không grep. Nếu công cụ grep, mọi đoạn chú thích bàn về một
// event sẽ thành một khai báo — và cảnh báo giả làm người ta tắt hẳn phép
// kiểm.
func TestChuoiTrongChuThichKhongPhaiKhaiBao(t *testing.T) {
	khai := `// Tài liệu nhắc "order.da_bien_mat" ở đây, chỉ để giải thích.
const (
	TypeOrderPaid = "order.paid"
)
`
	goc := duAn(t, khai, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})

	if c := chay(t, goc); len(c.viPham) != 0 {
		t.Fatalf("chú thích không phải khai báo: %v", c.viPham)
	}
}

// File `_test.go` KHÔNG tính là nơi phát.
//
// Một loại event chỉ phát trong test là một loại event production không
// bao giờ phát — đúng thứ công cụ này tìm. Tính nó vào sẽ làm hàng rào im
// lặng ngay khi ai đó viết một bài test cho event chết.
func TestFilePhatTrongTestKhongTinh(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat_test.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go":      benNghe("Nghe", "TypeOrderPaid"),
	})

	c := chay(t, goc)
	gop := strings.Join(c.viPham, "\n")
	if !strings.Contains(gop, "R1") {
		t.Fatalf("phát trong _test.go không được tính là phát: %v", c.viPham)
	}
}

// Dự án THẬT phải sạch.
//
// Thiếu bài này thì công cụ có thể đúng mà không ai chạy nó lên thứ đáng
// chạy — cùng lý do với `TestDuAnThatKhop` của apicheck.
func TestDuAnThatSach(t *testing.T) {
	// `moiChecker` chứ không dựng bằng tay: dựng bằng tay ở đây là một bản
	// sao thứ hai của danh sách sổ, và nó đã lệch một lần — thêm sổ của R4
	// làm bài này đỏ dù dự án thật sạch.
	c := moiChecker("../..")
	if err := c.run(); err != nil {
		t.Fatalf("run trên repo thật: %v", err)
	}
	if len(c.viPham) != 0 {
		t.Fatalf("repo có %d vi phạm hợp đồng event:\n%s",
			len(c.viPham), strings.Join(c.viPham, "\n"))
	}
}

// ------------------------------------------------- R4: bộ bên nhận

// R4 — worker đăng ký một bên nhận mà bộ của test KHÔNG.
//
// ĐÂY LÀ LỖI THẬT, tìm ra 01/10/2026: `inventory.NewReleaseHandler` — đường
// RA của kho — có ở worker và thiếu ở `dangKyBenNhan` suốt nhiều tháng, nên
// mọi bài test tích hợp chạy với một hệ thống không nhả hàng về kho.
func TestBatBenNhanThieuOBoTest(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})
	boBenNhan(t, goc,
		[]string{"a.NewMotHandler", "inv.NewReleaseHandler"},
		[]string{"a.NewMotHandler"})

	c := chay(t, goc)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	for _, can := range []string{"R4", "inv.NewReleaseHandler", "dangKyBenNhan"} {
		if !strings.Contains(c.viPham[0], can) {
			t.Errorf("thiếu %q trong: %s", can, c.viPham[0])
		}
	}
}

// R4 chiều NGƯỢC — bên nhận chỉ chạy trong test.
//
// Tệ hơn chiều kia: bài test xanh cho một đường KHÔNG chạy ở production, nên
// nó tạo ra lòng tin sai chứ chỉ không thiếu phép kiểm.
func TestBatBenNhanChiCoOBoTest(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})
	boBenNhan(t, goc,
		[]string{"a.NewMotHandler"},
		[]string{"a.NewMotHandler", "b.NewChiTrongTestHandler"})

	c := chay(t, goc)
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "b.NewChiTrongTestHandler") {
		t.Errorf("không chỉ ra bên nhận chỉ có ở test: %s", c.viPham[0])
	}
}

// Sổ miễn trừ TẮT được R4 — nhưng chỉ cho đúng bên nhận đã khai.
func TestSoMienTruTatDuocR4(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})
	boBenNhan(t, goc,
		[]string{"a.NewMotHandler", "sc.NewSignalHandler", "x.NewKhacHandler"},
		[]string{"a.NewMotHandler"})

	c := chayVoiSoR4(t, goc, map[string]string{
		"sc.NewSignalHandler": "module không nằm trong Modules của app",
	})

	// MỘT vi phạm còn lại: `x.NewKhacHandler` không có lý do nào.
	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "x.NewKhacHandler") {
		t.Errorf("sổ miễn trừ tắt sai bên nhận: %s", c.viPham[0])
	}
}

// Lý do khai cho một bên nhận KHÔNG tồn tại là một sổ đã mục.
//
// Bên nhận đổi tên hoặc bị xóa, và dòng trong sổ ở lại — từ đó nó miễn trừ
// cho một cái tên không còn nghĩa gì, và lần sau có bên nhận THẬT thiếu thì
// không ai biết dòng ấy đang che cái gì.
func TestBatSoR4DaMuc(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})

	c := chayVoiSoR4(t, goc, map[string]string{
		"da.NewDoiTenHandler": "lý do cũ cho một tên không còn tồn tại",
	})

	if len(c.viPham) != 1 {
		t.Fatalf("mong 1 vi phạm, nhận %d: %v", len(c.viPham), c.viPham)
	}
	if !strings.Contains(c.viPham[0], "da.NewDoiTenHandler") {
		t.Errorf("không chỉ ra dòng sổ đã mục: %s", c.viPham[0])
	}
}

// Lời gọi Subscribe NGOÀI `dangKyBenNhan` không được tính.
//
// `phatEvent` và các bài test riêng cũng gọi `Subscribe` cho bên nhận của
// riêng chúng — ví dụ `payment.NewCarrierCostHandler` kèm giá cố định.
// Tính cả chúng sẽ làm sổ miễn trừ vô nghĩa, vì mọi thứ đều "có ở test".
//
// `boTestGia` luôn ghi thêm `rieng.NewChiOBaiNayHandler` ngoài hàm ấy, nên
// nếu công cụ đọc cả file thì bài này đỏ ở chiều "chỉ có ở test".
func TestSubscribeNgoaiHamKhongTinh(t *testing.T) {
	goc := duAn(t, khaiMotEvent, map[string]string{
		"modules/a/phat.go": benPhat("TypeOrderPaid"),
		"modules/b/nghe.go": benNghe("Nghe", "TypeOrderPaid"),
	})

	if c := chay(t, goc); len(c.viPham) != 0 {
		t.Fatalf("mong 0 vi phạm, nhận: %v", c.viPham)
	}
}

// chayVoiSoR4 chạy công cụ với sổ miễn trừ R4 cho trước.
func chayVoiSoR4(t *testing.T, goc string, so map[string]string) *checker {
	t.Helper()
	c := &checker{
		root:       goc,
		khongPhat:  map[string]string{},
		khongNghe:  map[string]string{},
		chiOWorker: so,
		khaiEvent:  map[string]string{},
		noiPhat:    map[string][]string{},
		noiNghe:    map[string][]string{},
	}
	if err := c.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}
