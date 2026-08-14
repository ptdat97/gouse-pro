package analytics_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/analytics"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func newModule(t *testing.T, clock *fakeClock) (*analytics.Module, *pgxpool.Pool) {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần DATABASE_URL để chạy test với PostgreSQL thật")
	}

	db, err := database.Open(context.Background(), database.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("mở database: %v", err)
	}
	t.Cleanup(db.Close)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE event_log",
		"TRUNCATE metric_snapshot",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	m, err := analytics.New(analytics.Config{
		Storage: "postgres",
		DB:      db,
		Clock:   clock,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("analytics.New: %v", err)
	}
	return m, db.Pool()
}

// day là ngày mà fakeClock đang ở.
var day = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

func amount(v int64) *int64 { return &v }

// order tạo sự kiện đặt hàng đã ghi nhận.
func order(sessionID, sellerID string, value int64, at time.Time) analytics.EventInput {
	return analytics.EventInput{
		Name:       analytics.EventOrderPlaced,
		Category:   analytics.CategoryBusiness,
		EventID:    ids.MustNew(ids.PrefixEvent).String(),
		SessionID:  sessionID,
		SellerID:   sellerID,
		Amount:     amount(value),
		OccurredAt: at,
	}
}

func behavior(name, sessionID, sellerID string, at time.Time) analytics.EventInput {
	return analytics.EventInput{
		Name:       name,
		Category:   analytics.CategoryBehavior,
		SessionID:  sessionID,
		SellerID:   sellerID,
		OccurredAt: at,
	}
}

// GHI SỰ KIỆN CƠ BẢN.
func TestGhiSuKien(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	if err := m.TrackEvent(ctx, behavior(
		analytics.EventProductView, "ses-1", "", day.Add(9*time.Hour))); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE event_name = $1`,
		analytics.EventProductView).Scan(&n); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if n != 1 {
		t.Fatalf("ghi %d sự kiện, mong 1", n)
	}
}

// GHI NHẬN KHÔNG BAO GIỜ CHẶN LUỒNG CHÍNH — quy tắc 3.
//
// Nếu analytics lỗi, việc BÁN HÀNG vẫn phải chạy bình thường. Mất một bản
// ghi phân tích là chuyện nhỏ; chặn một đơn hàng thì không.
//
// Test này dùng một module trỏ tới database ĐÃ ĐÓNG — mô phỏng sự cố hạ
// tầng tệ nhất.
func TestGhiNhanKhongChanLuongChinh(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần DATABASE_URL")
	}

	db, err := database.Open(context.Background(), database.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("mở database: %v", err)
	}

	m, err := analytics.New(analytics.Config{
		Storage: "postgres",
		DB:      db,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("analytics.New: %v", err)
	}

	// Database sập.
	db.Close()

	err = m.TrackEvent(context.Background(), behavior(
		analytics.EventProductView, "ses-1", "", day))
	if err != nil {
		t.Fatalf("TrackEvent trả lỗi khi database sập: %v\n"+
			"Analytics lỗi KHÔNG được phép chặn luồng bán hàng.", err)
	}
}

// DỮ LIỆU KHÔNG HỢP LỆ VẪN TRẢ LỖI.
//
// Đây là lỗi của BÊN GỌI, sửa được. Im lặng nuốt nó sẽ khiến sự kiện biến
// mất mà không ai biết vì sao — và người ta đi tìm lỗi ở database.
func TestDuLieuKhongHopLeTraLoi(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	if err := m.TrackEvent(ctx, analytics.EventInput{
		Name:       "   ",
		OccurredAt: day,
	}); !errors.Is(err, analytics.ErrInvalidInput) {
		t.Fatalf("tên rỗng = %v, mong ErrInvalidInput", err)
	}
}

// SỰ KIỆN NGHIỆP VỤ CHỐNG GHI TRÙNG THEO EventID.
//
// Handler xử lý lại cùng một event là chuyện bình thường (giao ít-nhất-
// một-lần). Mỗi lần xử lý lại là một đơn hàng nữa cộng vào GMV.
func TestSuKienNghiepVuChongGhiTrung(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	e := order("ses-1", "", 500_000, day.Add(9*time.Hour))

	for i := 0; i < 4; i++ {
		if err := m.TrackEvent(ctx, e); err != nil {
			t.Fatalf("TrackEvent lần %d: %v", i+1, err)
		}
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE event_id = $1`,
		e.EventID).Scan(&n); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if n != 1 {
		t.Fatalf("ghi %d bản cho một event_id, mong 1 — GMV bị cộng %d lần", n, n)
	}
}

// LỚP THỨ HAI: sự kiện trùng phải được NHẬN RA, không phải nuốt lặng.
//
// Test trên (TestSuKienNghiepVuChongGhiTrung) chỉ đếm số hàng, nên chỉ
// mục UNIQUE một mình đã đủ làm nó xanh. Đã kiểm chứng ngược: bỏ việc
// dịch lỗi thành ErrDuplicateEvent, test đó VẪN xanh — vì Track rơi vào
// nhánh "sự cố hạ tầng" và cũng trả nil.
//
// Khác biệt nằm ở NHẬT KÝ: nhánh sự cố hạ tầng ghi CẢNH BÁO. Với một hệ
// thống xử lý lại event thường xuyên, mỗi lần xử lý lại sẽ sinh một cảnh
// báo giả — và cảnh báo giả nhiều tới mức không ai đọc nữa là cách chắc
// chắn nhất để bỏ lỡ cảnh báo thật.
//
// Test này bắt đúng ranh giới đó: ghi trùng KHÔNG được sinh cảnh báo nào.
func TestSuKienTrungKhongSinhCanhBao(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần DATABASE_URL")
	}

	db, err := database.Open(context.Background(), database.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("mở database: %v", err)
	}
	t.Cleanup(db.Close)

	ctx := context.Background()
	if _, err := db.Pool().Exec(ctx, "TRUNCATE event_log"); err != nil {
		t.Fatalf("dọn dữ liệu: %v", err)
	}

	var logs countingHandler
	m, err := analytics.New(analytics.Config{
		Storage: "postgres",
		DB:      db,
		Clock:   newClock(),
		Log:     slog.New(&logs),
	})
	if err != nil {
		t.Fatalf("analytics.New: %v", err)
	}

	e := order("ses-1", "", 500_000, day.Add(9*time.Hour))

	if err := m.TrackEvent(ctx, e); err != nil {
		t.Fatalf("TrackEvent lần 1: %v", err)
	}
	if got := logs.count(); got != 0 {
		t.Fatalf("lần ghi đầu sinh %d cảnh báo, mong 0", got)
	}

	// Ghi lại CÙNG event — phải im lặng.
	if err := m.TrackEvent(ctx, e); err != nil {
		t.Fatalf("TrackEvent lần 2: %v", err)
	}
	if got := logs.count(); got != 0 {
		t.Fatalf("ghi trùng sinh %d cảnh báo, mong 0 — xử lý lại event là "+
			"chuyện BÌNH THƯỜNG, không phải sự cố", got)
	}
}

// countingHandler đếm số bản ghi nhật ký từ mức WARN trở lên.
type countingHandler struct {
	mu sync.Mutex
	n  int
}

func (h *countingHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= slog.LevelWarn
}

func (h *countingHandler) Handle(_ context.Context, _ slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.n++
	return nil
}

func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }

func (h *countingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.n
}

// SỰ KIỆN HÀNH VI KHÔNG CHỐNG TRÙNG.
//
// Hai lần xem sản phẩm THẬT SỰ là hai lần xem. Chống trùng ở đây sẽ làm
// mất dữ liệu hành vi — thứ không tạo ngược được (quy tắc 6).
func TestSuKienHanhViKhongChongTrung(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	e := behavior(analytics.EventProductView, "ses-1", "", day.Add(9*time.Hour))

	for i := 0; i < 3; i++ {
		if err := m.TrackEvent(ctx, e); err != nil {
			t.Fatalf("TrackEvent lần %d: %v", i+1, err)
		}
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE event_name = $1`,
		analytics.EventProductView).Scan(&n); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if n != 3 {
		t.Fatalf("ghi %d sự kiện xem, mong 3 — dữ liệu hành vi bị mất", n)
	}
}

// ĐUA NHAU GHI CÙNG MỘT SỰ KIỆN NGHIỆP VỤ: chỉ MỘT bản được ghi.
//
// Kiểm tra "đã ghi chưa" ở tầng ứng dụng KHÔNG cứu được — mười worker
// cùng đọc thấy chưa có. Chỉ mục UNIQUE là thứ chặn.
func TestDuaNhauGhiSuKienNghiepVu(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	e := order("ses-1", "", 500_000, day.Add(9*time.Hour))

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = m.TrackEvent(ctx, e)
		}()
	}
	wg.Wait()

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE event_id = $1`,
		e.EventID).Scan(&rows); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if rows != 1 {
		t.Fatalf("%d request song song ghi %d bản, mong ĐÚNG 1", n, rows)
	}
}

// LỌC DỮ LIỆU NHẠY CẢM — quy tắc 4.
//
// Bên gọi chuyển tiếp dữ liệu do người dùng gửi lên. Một trường tên
// "password" lọt vào properties sẽ nằm trong database phân tích — nơi
// nhiều người đọc được và giữ rất lâu.
func TestLocDuLieuNhayCam(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	if err := m.TrackEvent(ctx, analytics.EventInput{
		Name:       analytics.EventProductView,
		Category:   analytics.CategoryBehavior,
		OccurredAt: day,
		Properties: map[string]any{
			"page":          "/ao-khoac",
			"password":      "bi-mat-cua-toi",
			"card_number":   "4111111111111111",
			"cvv":           "123",
			"auth_token":    "abc123",
			"waist":         68,
			"bust":          88,
			"body_height":   170,
			"national_id":   "012345678",
			"product_price": 500000,
		},
	}); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	var props map[string]any
	if err := pool.QueryRow(ctx,
		`SELECT properties FROM event_log LIMIT 1`).Scan(&props); err != nil {
		t.Fatalf("đọc thuộc tính: %v", err)
	}

	for _, banned := range []string{
		"password", "card_number", "cvv", "auth_token",
		"waist", "bust", "body_height", "national_id",
	} {
		if _, ok := props[banned]; ok {
			t.Fatalf("trường NHẠY CẢM %q lọt vào database phân tích", banned)
		}
	}

	// Trường vô hại phải còn.
	for _, kept := range []string{"page", "product_price"} {
		if _, ok := props[kept]; !ok {
			t.Fatalf("trường vô hại %q bị lọc mất", kept)
		}
	}
}

// KHÔNG LƯU IP NGUYÊN VĂN — yêu cầu quyền riêng tư, mục 8.
func TestKhongLuuIPNguyenVan(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	e := behavior(analytics.EventProductView, "ses-1", "", day)
	e.IP = "203.0.113.7"
	if err := m.TrackEvent(ctx, e); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	var ipHash string
	if err := pool.QueryRow(ctx,
		`SELECT ip_hash FROM event_log LIMIT 1`).Scan(&ipHash); err != nil {
		t.Fatalf("đọc sự kiện: %v", err)
	}
	if ipHash == "203.0.113.7" {
		t.Fatal("lưu IP NGUYÊN VĂN")
	}
	if len(ipHash) != 64 {
		t.Fatalf("ip_hash dài %d, mong 64 (SHA-256 hex)", len(ipHash))
	}
}

// GHI LÔ: một sự kiện sai KHÔNG làm mất cả lô.
func TestGhiLoBoQuaSuKienSai(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	events := []analytics.EventInput{
		behavior(analytics.EventProductView, "ses-1", "", day),
		{Name: "", OccurredAt: day}, // sai: tên rỗng
		behavior(analytics.EventAddToCart, "ses-1", "", day),
		behavior(analytics.EventCheckoutStart, "ses-1", "", day),
	}

	n, err := m.TrackBatch(ctx, events)
	if err != nil {
		t.Fatalf("TrackBatch: %v", err)
	}
	if n != 3 {
		t.Fatalf("ghi %d sự kiện, mong 3 (bỏ qua 1 sự kiện sai)", n)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_log`).Scan(&rows); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if rows != 3 {
		t.Fatalf("database có %d hàng, mong 3", rows)
	}
}

// TÍNH GMV, SỐ ĐƠN VÀ AOV.
//
// GMV KHÔNG PHẢI DOANH THU của nền tảng — nền tảng chỉ nhận hoa hồng.
func TestTinhGMVvaAOV(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	for _, v := range []int64{300_000, 500_000, 700_000} {
		if err := m.TrackEvent(ctx, order("ses-x", "", v, day.Add(9*time.Hour))); err != nil {
			t.Fatalf("TrackEvent: %v", err)
		}
	}

	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	for _, tc := range []struct {
		name string
		want int64
	}{
		{analytics.MetricGMV, 1_500_000},
		{analytics.MetricOrderCount, 3},
		{analytics.MetricAOV, 500_000},
	} {
		got, err := m.GetMetric(ctx, analytics.MetricRequest{
			Name:        tc.name,
			PeriodStart: day,
			Granularity: analytics.GranularityDay,
		})
		if err != nil {
			t.Fatalf("GetMetric(%s): %v", tc.name, err)
		}
		if got.Value != tc.want {
			t.Fatalf("%s = %d, mong %d", tc.name, got.Value, tc.want)
		}
	}
}

// TỶ LỆ CHUYỂN ĐỔI ĐẾM THEO PHIÊN, KHÔNG THEO SỰ KIỆN.
//
// Một người xem 20 sản phẩm là 20 sự kiện nhưng MỘT phiên. Dùng số sự
// kiện làm mẫu số sẽ ra tỷ lệ thấp hơn thực tế nhiều lần, và người đọc sẽ
// đi tìm một vấn đề không tồn tại.
func TestTyLeChuyenDoiDemTheoPhien(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	// 4 phiên xem sản phẩm; phiên đầu xem 20 lần.
	for i := 0; i < 20; i++ {
		if err := m.TrackEvent(ctx, behavior(
			analytics.EventProductView, "ses-1", "", day.Add(9*time.Hour))); err != nil {
			t.Fatalf("TrackEvent: %v", err)
		}
	}
	for _, s := range []string{"ses-2", "ses-3", "ses-4"} {
		if err := m.TrackEvent(ctx, behavior(
			analytics.EventProductView, s, "", day.Add(9*time.Hour))); err != nil {
			t.Fatalf("TrackEvent: %v", err)
		}
	}

	// 1 trong 4 phiên mua hàng.
	if err := m.TrackEvent(ctx, behavior(
		analytics.EventPurchase, "ses-1", "", day.Add(10*time.Hour))); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	got, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricConversionRate,
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}

	// 1 phiên mua / 4 phiên xem = 25% = 2500 điểm cơ bản.
	//
	// Nếu đếm theo SỰ KIỆN sẽ ra 1/23 ≈ 4,3% — sai gần sáu lần.
	if got.Value != 2500 {
		t.Fatalf("tỷ lệ chuyển đổi = %d điểm cơ bản, mong 2500 (25%%). "+
			"Đếm theo sự kiện thay vì phiên sẽ ra khoảng 434.", got.Value)
	}
	if got.SampleSize != 4 {
		t.Fatalf("cỡ mẫu = %d, mong 4 phiên", got.SampleSize)
	}
}

// CỠ MẪU PHẢI ĐI KÈM CHỈ SỐ.
//
// Tỷ lệ chuyển đổi 50% từ 2 lượt truy cập không nói lên điều gì, còn từ
// 20.000 lượt thì có. Thiếu cỡ mẫu, người đọc không phân biệt được.
func TestCoMauDiKemChiSo(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	if err := m.TrackEvent(ctx, behavior(
		analytics.EventProductView, "ses-1", "", day)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}
	if err := m.TrackEvent(ctx, behavior(
		analytics.EventProductView, "ses-2", "", day)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}
	if err := m.TrackEvent(ctx, behavior(
		analytics.EventPurchase, "ses-1", "", day)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	got, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricConversionRate,
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if got.Value != 5000 {
		t.Fatalf("tỷ lệ = %d, mong 5000 (50%%)", got.Value)
	}
	if got.SampleSize != 2 {
		t.Fatalf("cỡ mẫu = %d, mong 2 — không có nó thì 50%% trông như "+
			"một con số đáng tin", got.SampleSize)
	}
}

// CHỈ SỐ CẮT LÁT THEO SELLER, và seller CHỈ thấy số của mình.
func TestChiSoTheoSeller(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	if err := m.TrackEvent(ctx, order("ses-1", sellerA, 300_000, day)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}
	if err := m.TrackEvent(ctx, order("ses-2", sellerB, 900_000, day)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	for _, sellerID := range []string{sellerA, sellerB, ""} {
		if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
			PeriodStart: day,
			Granularity: analytics.GranularityDay,
			SellerID:    sellerID,
		}); err != nil {
			t.Fatalf("ComputeMetrics(%s): %v", sellerID, err)
		}
	}

	for _, tc := range []struct {
		sellerID string
		want     int64
	}{
		{sellerA, 300_000},
		{sellerB, 900_000},
		{"", 1_200_000}, // toàn sàn
	} {
		got, err := m.GetMetric(ctx, analytics.MetricRequest{
			Name:        analytics.MetricGMV,
			PeriodStart: day,
			Granularity: analytics.GranularityDay,
			SellerID:    tc.sellerID,
		})
		if err != nil {
			t.Fatalf("GetMetric(%s): %v", tc.sellerID, err)
		}
		if got.Value != tc.want {
			t.Fatalf("GMV của %q = %d, mong %d", tc.sellerID, got.Value, tc.want)
		}
	}

	// Seller A KHÔNG được thấy doanh số của seller B.
	gotA, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricGMV,
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
		SellerID:    sellerA,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if gotA.Value == 900_000 || gotA.Value == 1_200_000 {
		t.Fatal("seller A thấy doanh số của seller khác")
	}
}

// TÍNH LẠI GHI ĐÈ, KHÔNG THÊM HÀNG MỚI.
//
// Hai giá trị GMV cho cùng một ngày là hai câu trả lời cho cùng một câu
// hỏi, và không có cách nào biết cái nào đúng.
func TestTinhLaiGhiDe(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	if err := m.TrackEvent(ctx, order("ses-1", "", 500_000, day)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	compute := func() {
		if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
			PeriodStart: day,
			Granularity: analytics.GranularityDay,
		}); err != nil {
			t.Fatalf("ComputeMetrics: %v", err)
		}
	}
	compute()

	// Thêm một đơn rồi tính lại.
	if err := m.TrackEvent(ctx, order("ses-2", "", 300_000, day)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}
	compute()
	compute()

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM metric_snapshot WHERE metric_name = $1`,
		analytics.MetricGMV).Scan(&rows); err != nil {
		t.Fatalf("đếm chỉ số: %v", err)
	}
	if rows != 1 {
		t.Fatalf("có %d hàng GMV cho một ngày, mong 1", rows)
	}

	got, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricGMV,
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if got.Value != 800_000 {
		t.Fatalf("GMV = %d sau khi tính lại, mong 800.000đ", got.Value)
	}
}

// CẮT TRÒN THỜI GIAN NHẤT QUÁN.
//
// Chỉ số của "ngày 15/3" phải luôn nằm ở cùng một hàng. Nếu chỗ ghi cắt
// tròn khác chỗ đọc, dashboard sẽ hiện hai hàng cho cùng một ngày.
func TestCatTronThoiGianNhatQuan(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	if err := m.TrackEvent(ctx, order("ses-1", "", 500_000, day.Add(14*time.Hour))); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	// Tính với một thời điểm GIỮA ngày.
	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day.Add(17*time.Hour + 42*time.Minute),
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	// Đọc với một thời điểm KHÁC trong cùng ngày.
	got, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricGMV,
		PeriodStart: day.Add(3 * time.Hour),
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if got.Value != 500_000 {
		t.Fatalf("GMV = %d, mong 500.000đ — cắt tròn không nhất quán", got.Value)
	}

	var stored time.Time
	if err := pool.QueryRow(ctx,
		`SELECT period_start FROM metric_snapshot WHERE metric_name = $1`,
		analytics.MetricGMV).Scan(&stored); err != nil {
		t.Fatalf("đọc chỉ số: %v", err)
	}
	if !stored.UTC().Equal(day) {
		t.Fatalf("period_start = %v, mong %v (đầu ngày UTC)", stored.UTC(), day)
	}
}

// SỰ KIỆN KHÔNG CÓ TIỀN KHÔNG CỘNG VÀO GMV.
//
// "Đơn hàng 0đ" và "sự kiện xem sản phẩm" là hai chuyện khác nhau. Cộng
// nhầm loại thứ hai làm AOV thấp giả tạo.
func TestSuKienKhongCoTienKhongVaoGMV(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	if err := m.TrackEvent(ctx, order("ses-1", "", 600_000, day)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	// Sự kiện CÙNG TÊN nhưng KHÔNG có tiền — lỗi của bên gọi.
	noAmount := order("ses-2", "", 0, day)
	noAmount.Amount = nil
	if err := m.TrackEvent(ctx, noAmount); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	aov, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricAOV,
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if aov.Value != 600_000 {
		t.Fatalf("AOV = %d, mong 600.000đ — sự kiện không có tiền bị tính "+
			"vào mẫu số", aov.Value)
	}
}

// CHỈ SỐ CHƯA TÍNH TRẢ VỀ 0, KHÔNG PHẢI LỖI.
//
// Dashboard mở vào một ngày chưa có worker chạy là chuyện bình thường.
func TestChiSoChuaTinhTraVe0(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	got, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricGMV,
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if got.Value != 0 {
		t.Fatalf("chỉ số chưa tính = %d, mong 0", got.Value)
	}
}

// CHIA CHO 0 KHÔNG LÀM SẬP.
//
// "Chưa có đơn nào" là trạng thái BÌNH THƯỜNG của một ngày mới bắt đầu.
func TestChiaCho0KhongLamSap(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics trên ngày trống: %v", err)
	}

	for _, name := range []string{
		analytics.MetricAOV, analytics.MetricConversionRate,
	} {
		got, err := m.GetMetric(ctx, analytics.MetricRequest{
			Name:        name,
			PeriodStart: day,
			Granularity: analytics.GranularityDay,
		})
		if err != nil {
			t.Fatalf("GetMetric(%s): %v", name, err)
		}
		if got.Value != 0 {
			t.Fatalf("%s = %d trên ngày trống, mong 0", name, got.Value)
		}
	}
}

// CHUỖI THỜI GIAN theo ngày.
func TestChuoiThoiGian(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	for i, v := range []int64{100_000, 200_000, 300_000} {
		d := day.AddDate(0, 0, i)
		if err := m.TrackEvent(ctx, order("ses-x", "", v, d.Add(9*time.Hour))); err != nil {
			t.Fatalf("TrackEvent: %v", err)
		}
		if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
			PeriodStart: d,
			Granularity: analytics.GranularityDay,
		}); err != nil {
			t.Fatalf("ComputeMetrics: %v", err)
		}
	}

	series, err := m.GetTimeSeries(ctx, analytics.TimeSeriesRequest{
		Name:        analytics.MetricGMV,
		Granularity: analytics.GranularityDay,
		From:        day,
		To:          day.AddDate(0, 0, 3),
	})
	if err != nil {
		t.Fatalf("GetTimeSeries: %v", err)
	}
	if len(series) != 3 {
		t.Fatalf("chuỗi có %d điểm, mong 3", len(series))
	}
	for i, want := range []int64{100_000, 200_000, 300_000} {
		if series[i].Value != want {
			t.Fatalf("điểm %d = %d, mong %d", i, series[i].Value, want)
		}
	}
}

// KHOẢNG THỜI GIAN NỬA MỞ: [From, To).
//
// Hai khoảng liền nhau không được đếm trùng mốc chung — nếu trùng, tổng
// của các khoảng sẽ lớn hơn tổng thật.
func TestKhoangThoiGianNuaMo(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	// Một đơn ĐÚNG lúc nửa đêm ngày hôm sau.
	nextDay := day.AddDate(0, 0, 1)
	if err := m.TrackEvent(ctx, order("ses-1", "", 500_000, nextDay)); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	// Tính cho ngày ĐẦU: không được tính đơn của ngày sau.
	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}

	got, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricGMV,
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if got.Value != 0 {
		t.Fatalf("GMV ngày 15/3 = %d, mong 0 — đơn lúc 00:00 ngày 16/3 "+
			"bị tính vào ngày 15", got.Value)
	}

	// Ngày sau phải có đơn đó.
	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: nextDay,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	got, err = m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricGMV,
		PeriodStart: nextDay,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if got.Value != 500_000 {
		t.Fatalf("GMV ngày 16/3 = %d, mong 500.000đ", got.Value)
	}
}

// ẨN DANH KHÁCH: gỡ định danh nhưng GIỮ số liệu tổng hợp.
//
// Xóa hàng sẽ làm GMV của các tháng trước thay đổi — một chuyện không giải
// thích được với ai.
func TestAnDanhKhachGiuSoLieu(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	customerID := ids.MustNew(ids.PrefixCustomer).String()

	e := order("ses-1", "", 500_000, day)
	e.CustomerID = customerID
	e.IP = "203.0.113.7"
	e.UserAgent = "Mozilla/5.0"
	if err := m.TrackEvent(ctx, e); err != nil {
		t.Fatalf("TrackEvent: %v", err)
	}

	n, err := m.AnonymizeCustomer(ctx, customerID)
	if err != nil {
		t.Fatalf("AnonymizeCustomer: %v", err)
	}
	if n != 1 {
		t.Fatalf("ẩn danh %d bản ghi, mong 1", n)
	}

	// Hàng vẫn còn, nhưng KHÔNG còn định danh.
	var (
		gotCustomer, gotSession, gotIP, gotUA string
		gotAmount                             *int64
	)
	if err := pool.QueryRow(ctx, `
		SELECT customer_id, session_id, ip_hash, user_agent, amount
		  FROM event_log LIMIT 1`).
		Scan(&gotCustomer, &gotSession, &gotIP, &gotUA, &gotAmount); err != nil {
		t.Fatalf("đọc sự kiện: %v", err)
	}

	for name, got := range map[string]string{
		"customer_id": gotCustomer,
		"session_id":  gotSession,
		"ip_hash":     gotIP,
		"user_agent":  gotUA,
	} {
		if got != "" {
			t.Fatalf("%s còn giá trị %q sau khi ẩn danh", name, got)
		}
	}

	// SỐ TIỀN PHẢI CÒN: nó là số liệu tổng hợp, không phải định danh.
	if gotAmount == nil || *gotAmount != 500_000 {
		t.Fatalf("số tiền = %v sau khi ẩn danh, mong 500.000đ — GMV các "+
			"tháng trước sẽ thay đổi", gotAmount)
	}

	// GMV tính lại vẫn đúng.
	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	}); err != nil {
		t.Fatalf("ComputeMetrics: %v", err)
	}
	got, err := m.GetMetric(ctx, analytics.MetricRequest{
		Name:        analytics.MetricGMV,
		PeriodStart: day,
		Granularity: analytics.GranularityDay,
	})
	if err != nil {
		t.Fatalf("GetMetric: %v", err)
	}
	if got.Value != 500_000 {
		t.Fatalf("GMV = %d sau khi ẩn danh, mong 500.000đ", got.Value)
	}
}

// ẨN DANH VỚI customerID RỖNG KHÔNG XÓA GÌ.
//
// Chuỗi rỗng khớp với MỌI sự kiện của khách chưa đăng nhập. Một lời gọi
// sai tham số sẽ mất dữ liệu KHÔNG TẠO NGƯỢC ĐƯỢC.
func TestAnDanhIDRongKhongXoaGi(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	// Ba sự kiện của khách CHƯA đăng nhập.
	for _, s := range []string{"ses-1", "ses-2", "ses-3"} {
		if err := m.TrackEvent(ctx, behavior(
			analytics.EventProductView, s, "", day)); err != nil {
			t.Fatalf("TrackEvent: %v", err)
		}
	}

	n, err := m.AnonymizeCustomer(ctx, "")
	if err != nil {
		t.Fatalf("AnonymizeCustomer: %v", err)
	}
	if n != 0 {
		t.Fatalf("ẩn danh %d bản ghi với id rỗng, mong 0", n)
	}

	var kept int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_log WHERE session_id <> ''`).Scan(&kept); err != nil {
		t.Fatalf("đếm sự kiện: %v", err)
	}
	if kept != 3 {
		t.Fatalf("còn %d sự kiện có session_id, mong 3 — dữ liệu bị xóa nhầm", kept)
	}
}

// ĐỘ MỊN KHÔNG HỢP LỆ BỊ TỪ CHỐI.
func TestDoMinKhongHopLe(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	if err := m.ComputeMetrics(ctx, analytics.ComputeRequest{
		PeriodStart: day,
		Granularity: "WEEK",
	}); !errors.Is(err, analytics.ErrInvalidInput) {
		t.Fatalf("độ mịn lạ = %v, mong ErrInvalidInput", err)
	}

	if _, err := m.GetTimeSeries(ctx, analytics.TimeSeriesRequest{
		Name:        analytics.MetricGMV,
		Granularity: analytics.GranularityDay,
		From:        day.AddDate(0, 0, 3),
		To:          day, // ngược
	}); !errors.Is(err, analytics.ErrInvalidInput) {
		t.Fatalf("khoảng thời gian ngược = %v, mong ErrInvalidInput", err)
	}
}
