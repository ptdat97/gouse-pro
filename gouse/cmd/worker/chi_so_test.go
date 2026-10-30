package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/analytics"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// Bộ bài này gác ca mục 8 tìm được ngày 30/10/2026: cột
// `metric_snapshot.granularity` chỉ có DAY trên 100% hàng, trong khi
// HOUR và MONTH có đủ hằng số Go, đủ `Truncate`/`Next`, đủ CHECK ở
// database — và KHÔNG có bên gọi.
//
// Không bài test nào đỏ vì thiếu bên gọi. Đó là lý do mục 8 cần một công
// cụ đo DỮ LIỆU (`cmd/deadcol`) chứ không chỉ cần thêm test.

// reGranularityCheck khớp định nghĩa cột granularity trong migration.
//
// Dạng DDL cố ý hẹp: nếu ai đó đổi cách khai báo, biểu thức này KHÔNG
// khớp và bài test DỪNG với lời nhắn, thay vì lặng lẽ kiểm một danh sách
// cũ. Hỏng về phía ồn ào là hỏng đúng hướng.
var reGranularityCheck = regexp.MustCompile(
	`granularity TEXT NOT NULL CHECK \(granularity IN \(([^)]*)\)\)`)

// granularitiesInSchema đọc danh sách granularity mà DATABASE cho phép.
//
// Đọc từ migration — nguồn sự thật — chứ không từ một danh sách viết tay
// cạnh bài test: danh sách viết tay sẽ thiếu đúng giá trị vừa được thêm,
// và khi đó bài test xanh trong khi hàng rào đã thủng.
func granularitiesInSchema(t *testing.T) []string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("tìm migration: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("không thấy migration nào — bài này đang KHÔNG kiểm gì")
	}
	sort.Strings(files)

	var found []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("đọc %s: %v", f, err)
		}
		// Migration SAU ghi đè migration TRƯỚC, nên lấy lần khớp CUỐI.
		for _, mm := range reGranularityCheck.FindAllStringSubmatch(string(b), -1) {
			var vals []string
			for _, v := range strings.Split(mm[1], ",") {
				vals = append(vals, strings.Trim(strings.TrimSpace(v), "'"))
			}
			found = vals
		}
	}
	return found
}

// TestEveryGranularityInTheSchemaHasAJobWritingIt là hàng rào của ca này.
//
// Nó đối chiếu HAI danh sách: giá trị database cho phép, và giá trị thực
// sự có một job truyền vào `ComputeMetrics`. Thêm một mức granularity mà
// quên bên gọi sẽ làm bài này đỏ.
func TestEveryGranularityInTheSchemaHasAJobWritingIt(t *testing.T) {
	want := granularitiesInSchema(t)
	if len(want) == 0 {
		t.Fatal("không đọc được CHECK granularity — bài này đang KHÔNG kiểm gì")
	}

	now := time.Now().UTC()
	have := map[string]bool{}
	for _, w := range append(metricWindows(now), monthWindows(now)...) {
		have[w.granularity] = true
	}

	for _, g := range want {
		if !have[g] {
			t.Errorf("granularity %q được lược đồ cho phép nhưng KHÔNG "+
				"job nào ghi nó — đúng dạng mục 8: đủ mọi mảnh trừ bên gọi", g)
		}
	}
}

// TestThePreviousMonthSurvivesAShortFebruary kiểm cái bẫy lùi tháng.
//
// Ngày 31/3 lùi một tháng theo NGÀY cho ra 3/3 (Go chuẩn hóa "31/2"), và
// cắt tròn lại về 1/3 — tức ĐÚNG tháng đang mở. Khi ấy tháng 2 không
// được tính lại trong ba ngày cuối tháng 3, job vẫn xanh, và không có
// lỗi nào để lần ra.
func TestThePreviousMonthSurvivesAShortFebruary(t *testing.T) {
	windows := monthWindows(time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC))
	if len(windows) != 2 {
		t.Fatalf("cần 2 khoảng tháng, có %d", len(windows))
	}

	wantThis := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if got := windows[0].at; !got.Equal(wantThis) {
		t.Errorf("tháng đang mở = %s, cần %s", got, wantThis)
	}

	wantPrev := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if got := windows[1].at; !got.Equal(wantPrev) {
		t.Errorf("tháng trước = %s, cần %s — lùi tháng từ NGÀY 31 "+
			"trượt về chính tháng đang mở", got, wantPrev)
	}
}

// TestTheMetricJobsWriteEveryGranularity chạy CẢ HAI job lên database
// thật và đọc lại `metric_snapshot`.
//
// Bài trên kiểm danh sách khoảng; bài này kiểm rằng chạy job xong thì có
// HÀNG trong bảng, với ĐÚNG số tiền. Một khoảng tính sai vẫn ghi được
// hàng — chỉ là hàng mang giá trị 0 — nên phải so giá trị, không chỉ đếm
// hàng.
func TestTheMetricJobsWriteEveryGranularity(t *testing.T) {
	want := granularitiesInSchema(t)
	if len(want) == 0 {
		t.Fatal("không đọc được CHECK granularity — bài này đang KHÔNG kiểm gì")
	}

	db := testdb.Open(t)
	ctx := context.Background()

	for _, q := range []string{"DELETE FROM event_log", "DELETE FROM metric_snapshot"} {
		if _, err := db.Pool().Exec(ctx, q); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m, err := analytics.New(analytics.Config{Storage: "postgres", DB: db, Log: log})
	if err != nil {
		t.Fatalf("analytics.New: %v", err)
	}

	const amount = int64(49000)
	amt := amount
	err = m.TrackEvent(ctx, analytics.EventInput{
		Name:        analytics.EventOrderPlaced,
		Category:    "BUSINESS",
		SubjectType: "order",
		SubjectID:   "ord_granularity",
		EventID:     "evt_granularity",
		Amount:      &amt,
		OccurredAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ghi sự kiện: %v", err)
	}

	// `Track` NUỐT lỗi hạ tầng rồi trả nil — xem application/service.go.
	// Nên phải ĐẾM hàng, không được tin giá trị trả về: nếu sự kiện không
	// vào bảng thì mọi phép so bên dưới sẽ là 0 == 0.
	var events int
	err = db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE event_name = $1`,
		analytics.EventOrderPlaced).Scan(&events)
	if err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if events != 1 {
		t.Fatalf("event_log có %d hàng, cần 1 — phần còn lại của bài "+
			"sẽ xanh mà không kiểm gì", events)
	}

	if err := computeMetrics(m, log)(ctx); err != nil {
		t.Fatalf("computeMetrics: %v", err)
	}
	if err := computeMonthlyMetrics(m, log)(ctx); err != nil {
		t.Fatalf("computeMonthlyMetrics: %v", err)
	}

	for _, g := range want {
		var gmv, rows int64
		err := db.Pool().QueryRow(ctx, `
			SELECT count(*), coalesce(max(value), -1)
			  FROM metric_snapshot
			 WHERE metric_name = $1 AND granularity = $2 AND dimension = ''`,
			analytics.MetricGMV, g).Scan(&rows, &gmv)
		if err != nil {
			t.Fatalf("đọc gmv %s: %v", g, err)
		}
		if rows == 0 {
			t.Errorf("granularity %s: không hàng nào trong metric_snapshot", g)
			continue
		}
		if gmv != amount {
			t.Errorf("gmv %s = %d, cần %d — khoảng %s không phủ "+
				"thời điểm của sự kiện", g, gmv, amount, g)
		}
	}
}
