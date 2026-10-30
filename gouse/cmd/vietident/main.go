// Command vietident tìm định danh Go còn mang âm tiết tiếng Việt.
//
// # Vì sao lệnh này phải tồn tại
//
// Việc đổi định danh sang tiếng Anh từng được báo "XONG TOÀN REPO, bộ lọc
// trả về RỖNG trên 7934 định danh" (28/10/2026). Đo lại ngày 30/10 cho ra
// **129 tên còn tiếng Việt, 53 trong mã chạy thật** — kể cả
// `PaymentMethod.HopLe`, `TyLe` của affiliate, `ChiPhi` của payment.
//
// Hai lý do nó sót, và lệnh này sửa cả hai:
//
//	DANH SÁCH TỪ là tập MỞ   → đo theo HÌNH DẠNG âm tiết (tập đóng)
//	BIỂU THỨC CHÍNH QUY mù   → đọc bằng go/ast, thấy cả tham số hàm
//
// Phép rà bằng biểu thức chính quy không thấy tham số hàm, kết quả có
// tên, `var (…)` nhóm trong thân hàm, biến `for … range`, tham số của
// func literal. `tapCon` — một tham số bool — sống sót đúng vì vậy.
//
// # Vì sao nó ở `cmd/` chứ không phải một bài test
//
// Nó CÓ ở `make check`, qua `go test ./cmd/vietident/`: bài test đọc
// ngưỡng rồi so với số đo thật. Nhưng số đo cần đọc được bằng mắt khi
// đang đổi tên, nên phần quét là một lệnh chạy tay được.
//
// Dùng:
//
//	go run ./cmd/vietident          # chỉ con số và ngưỡng
//	go run ./cmd/vietident -v       # kèm từng tên, theo tệp
package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hit là một định danh đáng ngờ.
type hit struct {
	file string
	line int
	name string
	syls []string
}

func main() {
	verbose := flag.Bool("v", false, "in từng tên, nhóm theo tệp")
	root := flag.String("root", ".", "gốc cây mã nguồn")
	flag.Parse()

	hits, files, idents, err := scan(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vietident: %v\n", err)
		os.Exit(1)
	}

	if *verbose && len(hits) > 0 {
		byFile := map[string][]hit{}
		for _, h := range hits {
			byFile[h.file] = append(byFile[h.file], h)
		}
		names := make([]string, 0, len(byFile))
		for f := range byFile {
			names = append(names, f)
		}
		sort.Strings(names)
		for _, f := range names {
			fmt.Printf("--- %s\n", f)
			for _, h := range byFile[f] {
				fmt.Printf("   %-26s :%-5d %s\n",
					h.name, h.line, strings.Join(h.syls, " "))
			}
		}
		fmt.Println()
	}

	fmt.Printf("vietident: %d định danh còn âm tiết Việt · %d tệp · %d định danh đã đọc\n",
		len(hits), files, idents)

	if len(hits) > ratchet {
		fmt.Printf("\nvietident: NGƯỠNG là %d, đo được %d — tăng thêm %d.\n",
			ratchet, len(hits), len(hits)-ratchet)
		fmt.Println("Ngưỡng này CHỈ ĐƯỢC GIẢM. Đổi tên, đừng nới ngưỡng.")
		os.Exit(1)
	}
	if len(hits) < ratchet {
		fmt.Printf("\nvietident: đo được %d, THẤP HƠN ngưỡng %d — hạ `ratchet` xuống %d.\n",
			len(hits), ratchet, len(hits))
		fmt.Println("Ngưỡng tụt lại sau số đo thì nó không còn gác gì nữa.")
		os.Exit(1)
	}
}

// scan đọc mọi tệp .go dưới root và trả các định danh đáng ngờ.
func scan(root string) (hits []hit, files, idents int, err error) {
	fset := token.NewFileSet()

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			// Thư mục bắt đầu bằng `_` không vào build của Go.
			if strings.HasPrefix(d.Name(), "_") && d.Name() != "_" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return fmt.Errorf("đọc %s: %w", path, perr)
		}
		files++

		rel, _ := filepath.Rel(root, path)
		for _, id := range declaredIdents(f) {
			idents++
			if syls := vietnameseSyllables(id.Name); len(syls) > 0 {
				hits = append(hits, hit{
					file: rel,
					line: fset.Position(id.Pos()).Line,
					name: id.Name,
					syls: syls,
				})
			}
		}
		return nil
	})
	return hits, files, idents, err
}
