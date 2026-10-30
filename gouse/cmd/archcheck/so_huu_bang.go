package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// R6 — SỞ HỮU BẢNG: SQL trong module X chỉ được nhắc tới bảng thuộc X.
//
// # Vì sao quy tắc này tồn tại
//
// `dependency-rules.md` mục 9 đã liệt kê nó là kiểm tra CI bắt buộc từ
// lâu — "Kiểm tra 6: Sở hữu bảng. File SQL trong module X chỉ nhắc tới
// bảng thuộc X" — và nó chưa bao giờ được cài. Một quy tắc viết trong tài
// liệu mà không ai cưỡng chế là một quy tắc sẽ bị phá.
//
// R1 chặn module A import mã của module B. Nhưng nó KHÔNG chặn A viết
// `SELECT ... FROM bang_cua_B` — cùng một sự ghép nối, đi bằng đường khác,
// và là đường khó thấy hơn nhiều: không có import nào để đọc.
//
// Hậu quả nặng hơn ghép nối thường: B đổi lược đồ bảng của mình mà không
// biết A đang đọc nó, nên một migration đúng theo mọi nghĩa vẫn làm A vỡ.
// Ranh giới module chỉ thật khi nó cũng là ranh giới DỮ LIỆU.
//
// # Nguồn sự thật là TÀI LIỆU
//
// Mục "Dữ liệu sở hữu" đã có ở cả 29 tài liệu module, khai 159 bảng và
// không bảng nào bị hai module khai. Công cụ này đọc chính những mục ấy
// thay vì giữ một danh sách riêng — một bản sao thứ hai của bảng sở hữu
// sớm muộn sẽ lệch với tài liệu, và khi ấy không biết bên nào đúng.

// platformTables là các bảng thuộc `internal/platform`, không thuộc module
// nghiệp vụ nào — nên không tài liệu module nào khai chúng.
//
// Mọi module được phép chạm: đó là hạ tầng dùng chung, và chúng không mang
// khái niệm nghiệp vụ nào để mà rò rỉ.
var platformTables = map[string]string{
	"audit_log":       "platform/audit — vết kiểm toán của MỌI module",
	"event_outbox":    "platform/eventbus — hàng đợi phát event",
	"event_processed": "platform/eventbus — đánh dấu đã xử lý, chống xử lý hai lần",
	"ops_config":      "platform/opsconfig — cấu hình nghiệp vụ đổi lúc chạy",
	"webhook_event":   "platform/webhook — chống trùng webhook theo định danh CỦA HỌ",
}

// docNameDiffersFromModule ánh xạ tên file tài liệu sang tên thư mục module.
//
// Hai chỗ lệch, và sửa bằng cách đổi tên file sẽ phải sửa 57 liên kết —
// trong đó `return.md` còn trùng tên với tài liệu LUỒNG trả hàng, nên đổi
// tên là rủi ro không đáng.
//
// Bảng dịch tên là một nguồn sự thật thứ hai, và bình thường tôi tránh nó.
// Nó an toàn ở đây vì `readTableOwnership` ĐỎ khi một thư mục module không tra ra
// tài liệu: lệch tên mới sẽ kêu ngay chứ không im lặng bỏ gác.
var docNameDiffersFromModule = map[string]string{
	"return":       "returns",
	"supply-chain": "supplychain",
}

// reOwnerEntry tách khối ```sql ĐẦU TIÊN sau tiêu đề "Dữ liệu sở hữu".
//
// Chỉ khối đầu: mục ấy thường có thêm các khối `CREATE TABLE` bên dưới để
// giải thích lược đồ, và chúng không phải danh sách sở hữu.
var reOwnerEntry = regexp.MustCompile(
	`(?s)##\s*\d+\.\s*Dữ liệu sở hữu\s*\n+` + "```" + `sql\n(.*?)` + "```")

var reTableName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// readTableOwnership đọc bảng sở hữu từ `docs/04-modules/*.md`.
func (c *checker) readTableOwnership() (map[string]string, error) {
	root2 := filepath.Join(c.root, "..", "docs", "04-modules")
	entries, err := os.ReadDir(root2)
	if err != nil {
		return nil, fmt.Errorf("đọc tài liệu module: %w\n"+
			"(R6 đọc mục \"Dữ liệu sở hữu\" của chính tài liệu — đổi chỗ "+
			"thư mục ấy thì phải sửa cả công cụ này)", err)
	}

	owner := map[string]string{}
	hasDoc := map[string]bool{}

	for _, e := range entries {
		entryName := e.Name()
		if e.IsDir() || !strings.HasSuffix(entryName, ".md") || entryName == "README.md" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root2, entryName))
		if err != nil {
			return nil, err
		}

		mod := strings.TrimSuffix(entryName, ".md")
		if m, co := docNameDiffersFromModule[mod]; co {
			mod = m
		}
		hasDoc[mod] = true

		m := reOwnerEntry.FindSubmatch(content)
		if m == nil {
			return nil, fmt.Errorf("tài liệu %s không có mục \"N. Dữ liệu sở "+
				"hữu\" kèm khối ```sql — R6 không biết module ấy sở hữu bảng "+
				"nào", entryName)
		}
		for _, line := range strings.Split(string(m[1]), "\n") {
			tables := strings.Trim(strings.TrimSpace(
				strings.SplitN(line, "--", 2)[0]), `",`)
			if !reTableName.MatchString(tables) {
				continue
			}
			if before, duplicate := owner[tables]; duplicate && before != mod {
				return nil, fmt.Errorf("bảng %q bị HAI module khai sở hữu: "+
					"%s và %s — sở hữu phải là một, nếu không không ai biết "+
					"đổi lược đồ phải hỏi ai", tables, before, mod)
			}
			owner[tables] = mod
		}
	}

	// Một thư mục module không tra ra tài liệu nghĩa là bảng dịch tên đã
	// cũ — và khi ấy R6 sẽ im lặng bỏ gác module ấy.
	moduleDir, err := os.ReadDir(filepath.Join(c.root, "internal", "modules"))
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, d := range moduleDir {
		if d.IsDir() && !hasDoc[d.Name()] {
			missing = append(missing, d.Name())
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("module %s không có tài liệu tương ứng trong "+
			"docs/04-modules/ — thêm tài liệu, hoặc thêm dòng vào "+
			"`docNameDiffersFromModule` nếu tên file khác tên thư mục",
			strings.Join(missing, ", "))
	}
	return owner, nil
}

// reTableInSQL tìm tên bảng trong câu SQL.
//
// Bốn dạng câu chạm bảng, và chỉ bốn: `FROM`, `JOIN`, `INSERT INTO`,
// `UPDATE`, `DELETE FROM`. Không cố phân tích SQL đầy đủ — một bộ phân tích
// SQL trong công cụ kiểm ranh giới là một thứ phải bảo trì riêng, và bốn
// mẫu này đã bắt được mọi truy vấn của dự án.
var reTableInSQL = regexp.MustCompile(
	`(?i)\b(?:FROM|JOIN|INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+"?([a-z_][a-z0-9_]*)"?`)

// checkTableOwnership thực thi R6 trên mã của các module.
func (c *checker) checkTableOwnership() error {
	owner, err := c.readTableOwnership()
	if err != nil {
		return err
	}

	root2 := filepath.Join(c.root, "internal", "modules")
	return filepath.Walk(root2, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, _ := filepath.Rel(root2, path)
		mod := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		reported := map[string]bool{}
		for _, m := range reTableInSQL.FindAllSubmatchIndex(content, -1) {
			tables := strings.ToLower(string(content[m[2]:m[3]]))

			ownerOf, hasOwner := owner[tables]
			switch {
			case !hasOwner:
				// Không ai khai: có thể là bảng platform, một CTE, hay một
				// bảng chưa vào tài liệu. Platform thì cho qua; còn lại để
				// `kiemBangChuaKhai` xử lý, vì ở đây không phân biệt được
				// CTE với bảng thật.
				continue
			case ownerOf == mod:
				continue
			}

			if reported[tables] {
				continue
			}
			reported[tables] = true

			relRoot, _ := filepath.Rel(c.root, path)
			c.violations = append(c.violations, Violation{
				Rule: "R6",
				File: filepath.ToSlash(relRoot),
				Line: lineOf(content, m[2]),
				Message: fmt.Sprintf(
					"module %q chạm bảng %q của module %q", mod, tables, ownerOf),
				Hint: fmt.Sprintf("dùng cổng công khai của %s; %s đổi lược "+
					"đồ bảng ấy sẽ làm %s vỡ mà không ai biết", ownerOf, ownerOf, mod),
			})
		}
		return nil
	})
}

func lineOf(content []byte, offset int) int {
	return 1 + strings.Count(string(content[:offset]), "\n")
}
