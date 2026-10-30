package main

import (
	"context"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Hai sổ của lệnh này là phán đoán của người, nên máy không kiểm được chúng
// ĐÚNG. Nhưng máy kiểm được chúng còn SỐNG — và đó là phần hay chết.
//
// Những bài dưới đây chạy được ở CI: chúng chỉ cần LƯỢC ĐỒ, không cần dữ
// liệu. Phần cần dữ liệu là chính lệnh `deadcol`, và nó được gọi tay trên
// môi trường có dữ liệu thật.

var reLedgerKey = regexp.MustCompile(`^[a-z_]+\.[a-z_]+$`)

// TestLedgerKeysAreWellFormed: khóa phải là `bảng.cột`.
//
// Một khóa gõ sai không làm gì đỏ — nó chỉ lặng lẽ không khớp cột nào, nên
// cột thật quay lại danh sách "chưa quyết" và người đọc tưởng sổ đã nói về
// nó.
func TestLedgerKeysAreWellFormed(t *testing.T) {
	for colName, so := range map[string]map[string]string{
		"intentionallyConstant": intentionallyConstant,
		"knownEmpty":            knownEmpty,
	} {
		for k, ly := range so {
			if !reLedgerKey.MatchString(k) {
				t.Errorf("`%s` có khóa %q không đúng dạng `bảng.cột`", colName, k)
			}
			if strings.TrimSpace(ly) == "" {
				t.Errorf("`%s[%q]` không có lý do — một dòng miễn trừ không "+
					"lý do là một dòng không ai rà soát được", colName, k)
			}
		}
	}
}

// TestNoColumnIsInBothLedgers: một cột không thể vừa là hằng số cố ý vừa là
// cột không ai ghi.
//
// Hai sổ nói hai điều NGƯỢC nhau: một cái bảo "giá trị thứ hai sẽ không bao
// giờ tới", cái kia bảo "giá trị thứ hai đáng lẽ phải tới". Một cột nằm cả
// hai chỗ nghĩa là chưa ai thật sự quyết.
func TestNoColumnIsInBothLedgers(t *testing.T) {
	var cases2 []string
	for k := range intentionallyConstant {
		if _, ok := knownEmpty[k]; ok {
			cases2 = append(cases2, k)
		}
	}
	sort.Strings(cases2)
	if len(cases2) > 0 {
		t.Errorf("%d cột nằm trong CẢ HAI sổ: %s", len(cases2), strings.Join(cases2, ", "))
	}
}

// TestEveryLedgerKeyExistsInTheSchema: dòng sổ trỏ tới cột không còn tồn
// tại là một dòng nói dối.
//
// Chính lệnh `deadcol` cũng kiểm điều này, nhưng nó cần DỮ LIỆU để chạy.
// Bài này chỉ cần lược đồ, nên nó gác được ở CI — nơi một migration xóa cột
// sẽ đi qua.
func TestEveryLedgerKeyExistsInTheSchema(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("bỏ qua: cần TEST_DATABASE_URL để đọc lược đồ")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("kết nối: %v", err)
	}
	defer pool.Close()

	present, err := columnExists(ctx, pool)
	if err != nil {
		t.Fatalf("đọc lược đồ: %v", err)
	}
	if len(present) == 0 {
		t.Fatal("lược đồ không có cột nào — bài này đang KHÔNG kiểm gì")
	}

	for colName, so := range map[string]map[string]string{
		"intentionallyConstant": intentionallyConstant,
		"knownEmpty":            knownEmpty,
	} {
		var deadRows []string
		for k := range so {
			if !present[k] {
				deadRows = append(deadRows, k)
			}
		}
		sort.Strings(deadRows)
		if len(deadRows) > 0 {
			t.Errorf("`%s` còn %d dòng trỏ tới cột không tồn tại: %s",
				colName, len(deadRows), strings.Join(deadRows, ", "))
		}
	}
}
