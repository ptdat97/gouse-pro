// Lệnh deadcol tìm CỘT KHÔNG AI GHI, bằng cách đo trên dữ liệu thật.
//
// # Vì sao nó tồn tại
//
// docs/10-roadmap/backlog.md mục 8 ghi dạng lỗi hay gặp nhất của dự án:
// **một trường có trong hợp đồng API, có cột trong DB, có phương thức
// domain — và không dòng mã nào gán giá trị cho nó.** Mười bốn lần trong
// khoảng một tháng.
//
// Mục ấy liệt kê năm cách phát hiện, xếp theo thứ tự rẻ dần, và nói rõ về
// cách thứ năm:
//
//  5. quét CỘT DB không bao giờ đổi giá trị: một cột chỉ có DUY NHẤT một
//     giá trị trên hàng nghìn dòng thì hoặc là hằng số cố ý, hoặc là
//     không ai ghi
//
//     Bước 5 rẻ nhất trong cả năm, và nó bắt được thứ bốn bước kia bỏ sót:
//     `shipping_method` CÓ mặt trong câu UPDATE nên grep tìm thấy phép gán,
//     có mặt trong SELECT nên đọc lại được, có phương thức domain được gọi —
//     chỉ câu INSERT là thiếu.
//
// Phép rà ấy được chạy TAY đúng một lần, cho đúng một cột. Lệnh này biến nó
// thành thứ chạy lại được.
//
// # Vì sao là LỆNH, không phải hàng rào trong `make check`
//
// Nó đo trên DỮ LIỆU, và CI không có dữ liệu. Một database vừa migrate thì
// mọi cột đều rỗng, nên chạy phép này ở CI sẽ báo 741 cột và không nói gì.
//
// Đó cũng là giới hạn thật của phép đo: nó chỉ đúng bằng dữ liệu nó đọc.
// Trên môi trường phát triển, "chưa cột nào có giá trị thứ hai" có thể chỉ
// nghĩa là chưa ai bấm vào đường đó. Vì vậy sổ `knownEmpty` ghi hệ quả chứ
// không ghi kết luận, và vài dòng nói thẳng "đo lại khi chạy thật".
//
// # Cách dùng
//
//	deadcol                 # dùng DATABASE_URL, ngưỡng 100 dòng
//	deadcol -min=1000       # chỉ xét bảng lớn
//	deadcol -v              # in cả những cột ĐÃ khai trong sổ
//
// Thoát khác 0 khi có cột một-giá-trị KHÔNG nằm trong sổ nào, hoặc khi một
// dòng sổ đã CHẾT (cột không còn tồn tại, hoặc đã có giá trị thứ hai).
//
// # Vì sao ngưỡng mặc định là 100
//
// Đếm trên dữ liệu phát triển 30/10/2026:
//
//	ngưỡng >=1      236 cột
//	ngưỡng >=20      76
//	ngưỡng >=50      59
//	ngưỡng >=100     59
//	ngưỡng >=500     55
//	ngưỡng >=1000    55
//
// Từ 50 trở lên đường cong PHẲNG. Chọn 100 nghĩa là câu trả lời không phụ
// thuộc vào con số đã chọn — dưới 20 thì nó bị các bảng nhỏ chi phối.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type column struct {
	table, name string
	rows        int64
	only        string
}

func (c column) key() string { return c.table + "." + c.name }

func main() {
	min := flag.Int64("min", 100, "số dòng tối thiểu để xét một bảng")
	verbose := flag.Bool("v", false, "in cả những cột đã khai trong sổ")
	flag.Parse()

	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		fmt.Fprintln(os.Stderr,
			"deadcol: cần DATABASE_URL trỏ tới database CÓ DỮ LIỆU.\n"+
				"Phép đo này vô nghĩa trên database vừa migrate.")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deadcol: kết nối:", err)
		os.Exit(2)
	}
	defer pool.Close()

	cols, err := scan(ctx, pool, *min)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deadcol:", err)
		os.Exit(2)
	}

	var violations []string

	// 1. Cột một-giá-trị chưa ai quyết.
	var undecided []column
	for _, c := range cols {
		if _, ok := intentionallyConstant[c.key()]; ok {
			continue
		}
		if _, ok := knownEmpty[c.key()]; ok {
			continue
		}
		undecided = append(undecided, c)
	}
	for _, c := range undecided {
		violations = append(violations, fmt.Sprintf(
			"CỘT %s chỉ có MỘT giá trị (%s) trên %d dòng — phải QUYẾT: "+
				"hằng số cố ý thì khai vào `intentionallyConstant`, "+
				"không ai ghi thì khai vào `knownEmpty` kèm HỆ QUẢ",
			c.key(), c.only, c.rows))
	}

	// 2. Dòng sổ đã CHẾT.
	//
	// Cùng khuôn với `notImplemented` của apicheck: một dòng miễn trừ trỏ
	// tới thứ không còn đúng là một dòng nói dối, và nó nguy hiểm hơn việc
	// thiếu dòng — nó làm người đọc tin rằng ai đó đã xem xét.
	present := map[string]column{}
	for _, c := range cols {
		present[c.key()] = c
	}
	tonTai, err := columnExists(ctx, pool)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deadcol:", err)
		os.Exit(2)
	}
	for _, so := range []struct {
		colName string
		m       map[string]string
	}{{"intentionallyConstant", intentionallyConstant}, {"knownEmpty", knownEmpty}} {
		for k := range so.m {
			if !tonTai[k] {
				violations = append(violations, fmt.Sprintf(
					"SỔ `%s` còn dòng cho %s mà lược đồ không có cột ấy — xóa đi",
					so.colName, k))
				continue
			}
			if _, ok := present[k]; !ok {
				violations = append(violations, fmt.Sprintf(
					"SỔ `%s` còn dòng cho %s mà cột ấy NAY ĐÃ CÓ nhiều giá "+
						"trị (hoặc bảng dưới ngưỡng %d dòng) — xóa đi",
					so.colName, k, *min))
			}
		}
	}

	if *verbose {
		fmt.Printf("deadcol: %d bảng·cột một-giá-trị trên >=%d dòng\n", len(cols), *min)
		for _, c := range cols {
			label := "CHƯA QUYẾT"
			if ly, ok := intentionallyConstant[c.key()]; ok {
				label = "hằng số: " + ly
			} else if ly, ok := knownEmpty[c.key()]; ok {
				label = "không ai ghi: " + ly
			}
			fmt.Printf("  %-46s %7d  %-30s %s\n", c.key(), c.rows, c.only, label)
		}
	}

	fmt.Printf("deadcol: %d cột một-giá-trị · %d hằng số đã khai · %d cột KHÔNG AI GHI đã khai\n",
		len(cols), len(intentionallyConstant), len(knownEmpty))

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Fprintf(os.Stderr, "\ndeadcol: %d vi phạm\n\n", len(violations))
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, " ", v)
		}
		os.Exit(1)
	}
	fmt.Println("deadcol: OK — mọi cột một-giá-trị đều đã được QUYẾT, không dòng sổ nào chết")
}

// scan trả các cột có ĐÚNG MỘT giá trị phân biệt trên bảng đủ lớn.
//
// Dùng `count(DISTINCT col)` chứ không `count(*) = count(col)`: phép sau bỏ
// sót cột mà MỌI hàng đều NULL, và đó chính là hình dạng của một cột không
// ai ghi.
func scan(ctx context.Context, pool *pgxpool.Pool, min int64) ([]column, error) {
	rows, err := pool.Query(ctx, `
		SELECT c.table_name, c.column_name
		  FROM information_schema.columns c
		  JOIN information_schema.tables t
		    ON t.table_name = c.table_name AND t.table_schema = c.table_schema
		 WHERE c.table_schema = 'public' AND t.table_type = 'BASE TABLE'
		   AND c.table_name <> 'schema_migrations'
		 ORDER BY c.table_name, c.ordinal_position`)
	if err != nil {
		return nil, fmt.Errorf("liệt kê cột: %w", err)
	}
	type tc struct{ t, n string }
	var all []tc
	for rows.Next() {
		var a tc
		if err := rows.Scan(&a.t, &a.n); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, a)
	}
	rows.Close()

	var out []column
	for _, a := range all {
		var total, distinct int64
		q := fmt.Sprintf(`SELECT count(*), count(DISTINCT %q) FROM %q`, a.n, a.t)
		if err := pool.QueryRow(ctx, q).Scan(&total, &distinct); err != nil {
			return nil, fmt.Errorf("đếm %s.%s: %w", a.t, a.n, err)
		}
		if total < min || distinct > 1 {
			continue
		}
		var only *string
		_ = pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT %q::text FROM %q WHERE %q IS NOT NULL LIMIT 1`,
				a.n, a.t, a.n)).Scan(&only)
		v := "NULL ở mọi hàng"
		if only != nil {
			if v = *only; v == "" {
				v = `rỗng`
			}
		}
		out = append(out, column{table: a.t, name: a.n, rows: total, only: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rows != out[j].rows {
			return out[i].rows > out[j].rows
		}
		return out[i].key() < out[j].key()
	})
	return out, nil
}

// columnExists trả tập `bảng.cột` có trong lược đồ.
func columnExists(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `
		SELECT table_name || '.' || column_name
		  FROM information_schema.columns
		 WHERE table_schema = 'public'`)
	if err != nil {
		return nil, fmt.Errorf("liệt kê cột của lược đồ: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}
