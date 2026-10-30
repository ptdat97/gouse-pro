package database_test

import (
	"context"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/fashion-commerce/platform/internal/platform/database"
)

// openPoolWithTracer mở pool có bật đo thời gian truy vấn.
func openPoolWithTracer(t *testing.T) (*database.DB, *database.QueryTracer) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần TEST_DATABASE_URL để chạy test với PostgreSQL thật")
	}

	tr := database.NewQueryTracer("test")
	db, err := database.Open(context.Background(), database.Config{
		DSN: dsn, MaxConns: 2, MinConns: 0, Tracer: tr,
	})
	if err != nil {
		t.Fatalf("mở pool: %v", err)
	}
	t.Cleanup(db.Close)
	return db, tr
}

// countByOp đọc số lần quan sát của histogram theo nhãn `operation`.
func countByOp(t *testing.T, c prometheus.Collector) map[string]uint64 {
	t.Helper()

	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("đăng ký: %v", err)
	}
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("thu thập: %v", err)
	}

	result := map[string]uint64{}
	for _, f := range fams {
		if f.GetName() != "gouse_db_query_duration_seconds" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "operation" {
					result[l.GetValue()] = m.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return result
}

// TestTracerClassifiesByVerb: đọc và ghi phải vào hai nhãn KHÁC nhau.
//
// Đây là toàn bộ giá trị vận hành của chỉ số này. Câu hỏi thật khi hệ
// thống chậm là "đọc chậm hay ghi chậm": đọc chậm thường là thiếu chỉ mục,
// ghi chậm thường là tranh chấp khóa — hai hướng điều tra khác hẳn nhau.
// Gộp chung một nhãn thì con số không trả lời được câu nào.
func TestTracerClassifiesByVerb(t *testing.T) {
	db, tr := openPoolWithTracer(t)
	ctx := context.Background()

	if _, err := db.Pool().Exec(ctx,
		`CREATE TEMP TABLE thu_tracer (id int)`); err != nil {
		t.Fatalf("tạo bảng tạm: %v", err)
	}

	before := countByOp(t, tr)

	if _, err := db.Pool().Exec(ctx,
		`INSERT INTO thu_tracer (id) VALUES (1), (2)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var n int
	if err := db.Pool().QueryRow(ctx,
		`SELECT count(*) FROM thu_tracer`).Scan(&n); err != nil {
		t.Fatalf("select: %v", err)
	}
	if _, err := db.Pool().Exec(ctx,
		`UPDATE thu_tracer SET id = id + 1`); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := db.Pool().Exec(ctx,
		`DELETE FROM thu_tracer WHERE id > 100`); err != nil {
		t.Fatalf("delete: %v", err)
	}

	after := countByOp(t, tr)
	for _, op := range []string{
		database.OpSelect, database.OpInsert,
		database.OpUpdate, database.OpDelete,
	} {
		if after[op] <= before[op] {
			t.Errorf("thao tác %q không được đo (%d → %d) — có %v",
				op, before[op], after[op], after)
		}
	}
}

// TestTracerDoesNotBlowUpLabels là hàng rào quan trọng nhất của tệp này.
//
// Lấy câu SQL làm nhãn là cách phổ biến nhất khiến một hệ thống giám sát
// tự giết chính nó, và nó xảy ra âm thầm: mọi thứ chạy tốt cho tới khi
// Prometheus hết bộ nhớ.
//
// Bài này chạy 50 câu lệnh KHÁC NHAU về nội dung và đòi số chuỗi thời gian
// vẫn nằm trong tập động từ đóng.
func TestTracerDoesNotBlowUpLabels(t *testing.T) {
	db, tr := openPoolWithTracer(t)
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		var x int
		// Mỗi vòng một câu SQL khác nhau về mặt văn bản.
		if err := db.Pool().QueryRow(ctx,
			`SELECT `+itoa(i)+`::int`).Scan(&x); err != nil {
			t.Fatalf("truy vấn %d: %v", i, err)
		}
	}

	metrics := countByOp(t, tr)
	allowed := map[string]bool{
		database.OpSelect: true, database.OpInsert: true,
		database.OpUpdate: true, database.OpDelete: true,
		database.OpOther: true,
	}
	for labels := range metrics {
		if !allowed[labels] {
			t.Errorf("nhãn operation=%q nằm ngoài tập đóng — "+
				"số chuỗi thời gian sẽ tăng theo số câu SQL", labels)
		}
	}
	if len(metrics) > len(allowed) {
		t.Errorf("có %d nhãn khác nhau sau 50 câu lệnh, tối đa %d: %v",
			len(metrics), len(allowed), metrics)
	}
}

// TestTracerDoesNotCountNoRowsAsAnError.
//
// "Không tìm thấy" là câu trả lời HỢP LỆ và xảy ra liên tục ở đường tra
// cứu. Nếu nó bị đếm là lỗi thì chỉ số lỗi luôn khác 0, và một chỉ số luôn
// khác 0 là chỉ số không ai dùng để cảnh báo nữa.
//
// # Vì sao KHÔNG cần lọc trong mã, và làm sao biết
//
// pgx báo kết thúc truy vấn TRƯỚC khi `Row.Scan` chạy, nên ErrNoRows nổi
// lên ở Scan chứ không bao giờ tới tracer. Đo trực tiếp bằng một tracer
// thu thập `data.Err`: nó thấy đúng một giá trị, và giá trị đó là nil.
//
// Bài này vì thế ghim một tính chất của HỆ THỐNG, không phải một nhánh
// điều kiện trong mã của ta — nó vẫn đỏ nếu pgx đổi hành vi ở bản sau.
func TestTracerDoesNotCountNoRowsAsAnError(t *testing.T) {
	db, tr := openPoolWithTracer(t)
	ctx := context.Background()

	before := countErr(t, tr)

	var x int
	err := db.Pool().QueryRow(ctx, `SELECT 1 WHERE false`).Scan(&x)
	if err == nil {
		t.Fatal("truy vấn không trả ErrNoRows như mong đợi")
	}

	if after := countErr(t, tr); after != before {
		t.Errorf("truy vấn không có dòng nào bị đếm là lỗi (%v → %v) — "+
			"chỉ số lỗi sẽ luôn khác 0 và vì thế vô dụng cho cảnh báo",
			before, after)
	}
}

// TestTracerCountsRealErrors: lỗi cú pháp thì PHẢI được đếm.
func TestTracerCountsRealErrors(t *testing.T) {
	db, tr := openPoolWithTracer(t)
	ctx := context.Background()

	before := countErr(t, tr)

	if _, err := db.Pool().Exec(ctx, `SELECT * FROM bang_khong_ton_tai_zz`); err == nil {
		t.Fatal("truy vấn vào bảng không tồn tại lại thành công")
	}

	if after := countErr(t, tr); after <= before {
		t.Errorf("lỗi truy vấn không được đếm (%v → %v)", before, after)
	}
}

func countErr(t *testing.T, c prometheus.Collector) float64 {
	t.Helper()

	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("đăng ký: %v", err)
	}
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("thu thập: %v", err)
	}

	var total float64
	for _, f := range fams {
		if f.GetName() != "gouse_db_query_errors_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			total += m.GetCounter().GetValue()
		}
	}
	return total
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
