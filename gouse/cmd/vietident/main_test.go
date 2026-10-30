package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestTheSyllableShapeAcceptsVietnameseAndRejectsEnglish là bài kiểm cốt
// lõi: nếu hình dạng sai thì mọi con số của lệnh này vô nghĩa.
func TestTheSyllableShapeAcceptsVietnameseAndRejectsEnglish(t *testing.T) {
	vietnamese := []string{
		"ten", "hop", "nguong", "chung", "ghi", "ban", "han", "tham",
		"quyet", "khuon", "phuong", "nhom", "tong", "mong", "chay",
	}
	for _, s := range vietnamese {
		if !reSyllable.MatchString(s) {
			t.Errorf("%q là âm tiết tiếng Việt mà hình dạng KHÔNG nhận", s)
		}
	}

	// Những từ này KHÔNG được khớp, và lý do quan trọng: tiếng Việt
	// không có cụm phụ âm đầu kiểu `str`/`spl`, không có phụ âm cuối
	// `s`/`d`/`r`/`l`, và không có âm tiết dài như vậy.
	english := []string{
		"strength", "offer", "price", "world", "cards", "export",
		"handler", "shipping", "amount", "settlement",
	}
	for _, s := range english {
		if reSyllable.MatchString(s) {
			t.Errorf("%q là tiếng Anh mà hình dạng lại nhận — phép đo sẽ ồn", s)
		}
	}
}

// TestSplitCamelKeepsAcronymsWhole gác lý do splitCamel được viết tay.
//
// Tách `JSON` thành bốn mảnh `j s o n` sinh ra bốn âm tiết GIẢ, và phép
// đo ồn tới mức không ai đọc — tức là tắt hẳn hàng rào.
func TestSplitCamelKeepsAcronymsWhole(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"GetHTTPResponse", []string{"Get", "HTTP", "Response"}},
		{"jsonName", []string{"json", "Name"}},
		{"chan429", []string{"chan", "429"}},
		{"ID", []string{"ID"}},
		{"maULIDTest", []string{"ma", "ULID", "Test"}},
		{"x", []string{"x"}},
	}
	for _, tc := range cases {
		got := splitCamel(tc.in)
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("splitCamel(%q) = %v, cần %v", tc.in, got, tc.want)
		}
	}
}

// TestAnAllowedSyllableIsNotReported kiểm bước loại trừ thật sự chạy.
func TestAnAllowedSyllableIsNotReported(t *testing.T) {
	if !allowed["ctx"] {
		t.Fatal("sổ loại trừ không có `ctx` — bài này đang kiểm sai thứ")
	}
	if got := vietnameseSyllables("ctxDone"); len(got) != 0 {
		t.Errorf("`ctxDone` bị báo vi phạm vì %v, mà `ctx` đã ở sổ loại trừ", got)
	}
	if got := vietnameseSyllables("tongCtx"); len(got) == 0 {
		t.Error("`tongCtx` KHÔNG bị báo — một âm tiết được loại trừ đang " +
			"che cả định danh, nên mọi tên có `ctx` sẽ vô hình")
	}
}

// TestDeclaredIdentsSeesFunctionParameters là bài hồi quy của chính lý do
// lệnh này tồn tại.
//
// Phép rà bằng biểu thức chính quy chỉ đọc đầu dòng khai báo, nên tham số
// hàm vô hình — và `tapCon` ở `cmd/apicheck` sống qua CẢ lượt đổi tên
// được báo là "XONG TOÀN REPO".
func TestDeclaredIdentsSeesFunctionParameters(t *testing.T) {
	src := `package p

func samplePairs(tapCon bool) (ra []string) {
	var ( mongOK bool )
	for _, ten := range ra {
		_ = ten
	}
	f := func(phi int) {}
	_, _, _ = tapCon, mongOK, f
	return nil
}

type s struct{ biMat string }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatalf("đọc nguồn mẫu: %v", err)
	}

	got := map[string]bool{}
	for _, id := range declaredIdents(f) {
		got[id.Name] = true
	}

	// Năm dạng đã bị bỏ sót một lần, mỗi dạng một tên.
	for _, want := range []string{
		"tapCon", // tham số hàm
		"ra",     // kết quả CÓ TÊN
		"mongOK", // var (…) trong thân hàm
		"ten",    // biến for … range
		"phi",    // tham số của func literal
		"biMat",  // trường struct
	} {
		if !got[want] {
			t.Errorf("declaredIdents KHÔNG thấy %q", want)
		}
	}

	// Và KHÔNG được bắt nơi DÙNG: `ast.Inspect` trên mọi `*ast.Ident`
	// sẽ kéo cả tên của thư viện ngoài vào phép đo.
	if got["parser"] || got["ParseFile"] {
		t.Error("declaredIdents bắt cả nơi DÙNG — phép đo sẽ chìm trong " +
			"tên của thư viện ngoài")
	}
}

// TestTheRatchetMatchesTheRealCount là hàng rào thật, chạy trong `make check`.
//
// Đỏ ở CẢ HAI phía có chủ ý. Đo cao hơn ngưỡng nghĩa là có tên tiếng Việt
// mới lọt vào. Đo THẤP hơn ngưỡng nghĩa là ngưỡng đã tụt lại sau sự thật,
// và một ngưỡng tụt lại thì không gác gì nữa — nó chỉ còn là một con số.
func TestTheRatchetMatchesTheRealCount(t *testing.T) {
	hits, files, idents, err := scan("../..")
	if err != nil {
		t.Fatalf("quét: %v", err)
	}
	if files == 0 || idents == 0 {
		t.Fatal("quét được 0 tệp hoặc 0 định danh — bài này đang KHÔNG kiểm gì")
	}
	if len(hits) != ratchet {
		t.Errorf("đo được %d định danh còn âm tiết Việt, ngưỡng là %d.\n"+
			"Cao hơn: đổi tên, ĐỪNG nới ngưỡng. Thấp hơn: hạ ngưỡng xuống %d.",
			len(hits), ratchet, len(hits))
	}
}

var _ = ast.Inspect
