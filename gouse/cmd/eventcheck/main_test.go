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
	return goc
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
		root:      goc,
		khongPhat: map[string]string{},
		khongNghe: map[string]string{},
		khaiEvent: map[string]string{},
		noiPhat:   map[string][]string{},
		noiNghe:   map[string][]string{},
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
	c := &checker{
		root:      "../..",
		khongPhat: khongAiPhat,
		khongNghe: khongAiNghe,
		khaiEvent: map[string]string{},
		noiPhat:   map[string][]string{},
		noiNghe:   map[string][]string{},
	}
	if err := c.run(); err != nil {
		t.Fatalf("run trên repo thật: %v", err)
	}
	if len(c.viPham) != 0 {
		t.Fatalf("repo có %d vi phạm hợp đồng event:\n%s",
			len(c.viPham), strings.Join(c.viPham, "\n"))
	}
}
