package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/fashion-commerce/platform/internal/platform/database"
)

// moPool mở pool với trần kết nối đặt sẵn.
func moPool(t *testing.T, maxConns int32) *database.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần TEST_DATABASE_URL để chạy test với PostgreSQL thật")
	}

	db, err := database.Open(context.Background(), database.Config{
		DSN:      dsn,
		MaxConns: maxConns,
		MinConns: 0,
	})
	if err != nil {
		t.Fatalf("mở pool: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// doChiSo thu thập collector và trả về (tên+nhãn state) → giá trị.
func doChiSo(t *testing.T, c prometheus.Collector) map[string]float64 {
	t.Helper()

	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("đăng ký collector: %v", err)
	}
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("thu thập: %v", err)
	}

	ra := map[string]float64{}
	for _, f := range fams {
		for _, m := range f.GetMetric() {
			khoa := f.GetName()
			for _, l := range m.GetLabel() {
				if l.GetName() == "state" {
					khoa += "{" + l.GetValue() + "}"
				}
			}
			ra[khoa] = giaTri(m)
		}
	}
	return ra
}

func giaTri(m *dto.Metric) float64 {
	if g := m.GetGauge(); g != nil {
		return g.GetValue()
	}
	if c := m.GetCounter(); c != nil {
		return c.GetValue()
	}
	return 0
}

// TestPoolCollectorPhatDuChiSo — đường cơ bản.
func TestPoolCollectorPhatDuChiSo(t *testing.T) {
	db := moPool(t, 4)
	chiSo := doChiSo(t, database.NewPoolCollector(db, ""))

	for _, ten := range []string{
		"gouse_db_pool_connections{acquired}",
		"gouse_db_pool_connections{idle}",
		"gouse_db_pool_max_connections",
		"gouse_db_pool_empty_acquire_total",
		"gouse_db_pool_canceled_acquire_total",
		"gouse_db_pool_acquire_wait_seconds_total",
	} {
		if _, co := chiSo[ten]; !co {
			t.Errorf("thiếu chỉ số %s — có %v", ten, chiSo)
		}
	}

	if got := chiSo["gouse_db_pool_max_connections"]; got != 4 {
		t.Errorf("max_connections = %v, cần 4 — collector không đọc đúng "+
			"cấu hình thật của pool", got)
	}
}

// TestPoolCanKietDuocDem là lý do chỉ số này tồn tại.
//
// Khi pool cạn, request KHÔNG lỗi — nó chỉ đứng chờ. Độ trễ tăng vọt trong
// khi tỷ lệ lỗi vẫn đẹp, và người trực nhìn bảng theo dõi không hiểu vì
// sao trang chậm. Hai bộ đếm dưới đây là chỗ DUY NHẤT chuyện đó hiện ra.
//
// # Hai bộ đếm KHÁC nhau, và lần đầu tôi đã nhầm
//
// pgx tách rạch ròi:
//
//	EmptyAcquireCount    lượt phải chờ rồi CUỐI CÙNG LẤY ĐƯỢC
//	CanceledAcquireCount lượt chờ rồi BỎ CUỘC (context hết hạn)
//
// Một lượt hết hạn KHÔNG làm tăng EmptyAcquireCount. Bài này dựng riêng
// từng tình huống thay vì đòi cả hai cùng tăng trong một kịch bản.
func TestPoolCanKietDuocDem(t *testing.T) {
	const tran = 2
	db := moPool(t, tran)
	c := database.NewPoolCollector(db, "")

	ctx := context.Background()

	// Giữ TOÀN BỘ kết nối.
	giu := make([]*pgxpool.Conn, 0, tran)
	for i := 0; i < tran; i++ {
		conn, err := db.Pool().Acquire(ctx)
		if err != nil {
			t.Fatalf("lấy kết nối %d: %v", i, err)
		}
		giu = append(giu, conn)
	}
	nhaHet := func() {
		for _, conn := range giu {
			if conn != nil {
				conn.Release()
			}
		}
	}
	defer nhaHet()

	if got := doChiSo(t, c)["gouse_db_pool_connections{acquired}"]; got != tran {
		t.Fatalf("acquired = %v, cần %d — chưa dựng được tình huống cạn pool",
			got, tran)
	}

	// --- Tình huống 1: chờ rồi BỎ CUỘC ---
	boCuocTruoc := doChiSo(t, c)["gouse_db_pool_canceled_acquire_total"]

	choNgan, huy := context.WithTimeout(ctx, 150*time.Millisecond)
	if conn, err := db.Pool().Acquire(choNgan); err == nil {
		conn.Release()
		huy()
		t.Fatal("lấy được kết nối thứ 3 dù trần là 2")
	}
	huy()

	if sau := doChiSo(t, c)["gouse_db_pool_canceled_acquire_total"]; sau <= boCuocTruoc {
		t.Errorf("bỏ cuộc khi chờ mà canceled_acquire_total không tăng "+
			"(%v → %v) — khách bị từ chối vì hết kết nối sẽ không được đếm",
			boCuocTruoc, sau)
	}

	// --- Tình huống 2: chờ rồi LẤY ĐƯỢC ---
	choTruoc := doChiSo(t, c)["gouse_db_pool_empty_acquire_total"]

	xong := make(chan error, 1)
	go func() {
		conn, err := db.Pool().Acquire(ctx)
		if err == nil {
			conn.Release()
		}
		xong <- err
	}()

	// Nhả một kết nối để lượt đang chờ ở trên đi tiếp.
	time.Sleep(50 * time.Millisecond)
	giu[0].Release()
	giu[0] = nil

	select {
	case err := <-xong:
		if err != nil {
			t.Fatalf("lượt chờ không lấy được kết nối: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lượt chờ treo quá lâu")
	}

	if sau := doChiSo(t, c)["gouse_db_pool_empty_acquire_total"]; sau <= choTruoc {
		t.Errorf("phải chờ mới lấy được kết nối mà empty_acquire_total "+
			"không tăng (%v → %v) — tình trạng pool cạn sẽ không hiện ra "+
			"ở bất kỳ đâu, trong khi độ trễ đã tăng", choTruoc, sau)
	}
}

// TestPoolDaDongThiKhongPhatSo: phát 0 tệ hơn im lặng.
//
// Bảng theo dõi hiện "0 kết nối đang dùng, 0 lượt chờ" trông y hệt một hệ
// thống khỏe mạnh đang rảnh — trong khi thật ra nó đã chết.
func TestPoolDaDongThiKhongPhatSo(t *testing.T) {
	db := moPool(t, 2)
	c := database.NewPoolCollector(db, "")

	if len(doChiSo(t, c)) == 0 {
		t.Fatal("pool còn sống mà không phát chỉ số nào")
	}

	db.Close()
	if n := len(doChiSo(t, c)); n != 0 {
		t.Errorf("pool đã đóng nhưng vẫn phát %d chỉ số — "+
			"số 0 trông giống hệ thống rảnh, không giống hệ thống chết", n)
	}
}
