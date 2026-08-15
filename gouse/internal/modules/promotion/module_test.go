package promotion_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/promotion"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newModule(t *testing.T, clock *fakeClock) (*promotion.Module, *pgxpool.Pool) {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE coupon_usage",
		"TRUNCATE coupon CASCADE",
		"TRUNCATE promotion CASCADE",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	m, err := promotion.New(promotion.Config{
		Storage: "postgres",
		DB:      db,
		Clock:   clock,
	})
	if err != nil {
		t.Fatalf("promotion.New: %v", err)
	}
	return m, db.Pool()
}

// basePromo là khuyến mãi giảm 10%, chạy trong một tháng quanh thời điểm
// của fakeClock.
func basePromo() promotion.CreatePromotionRequest {
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	return promotion.CreatePromotionRequest{
		Name:         "Giảm 10% tháng 3",
		Kind:         promotion.KindCoupon,
		DiscountType: promotion.DiscountPercentage,
		DiscountBPS:  1000, // 10%
		StartsAt:     start,
		EndsAt:       start.AddDate(0, 2, 0),
	}
}

// setup tạo khuyến mãi ĐANG CHẠY kèm một mã.
func setup(
	t *testing.T, m *promotion.Module, req promotion.CreatePromotionRequest, code string,
) (promotion.PromotionView, promotion.CouponView) {
	t.Helper()
	ctx := context.Background()

	p, err := m.CreatePromotion(ctx, req)
	if err != nil {
		t.Fatalf("CreatePromotion: %v", err)
	}
	if err := m.ActivatePromotion(ctx, p.ID); err != nil {
		t.Fatalf("ActivatePromotion: %v", err)
	}

	c, err := m.CreateCoupon(ctx, promotion.CreateCouponRequest{
		PromotionID: p.ID,
		Code:        code,
	})
	if err != nil {
		t.Fatalf("CreateCoupon: %v", err)
	}
	return p, c
}

func validate(
	m *promotion.Module, code string, total int64,
) (promotion.DiscountResult, error) {
	return m.ValidateCoupon(context.Background(), promotion.ValidateRequest{
		Code:       code,
		OrderTotal: total,
	})
}

// KHUYẾN MÃI MỚI TẠO KHÔNG ÁP ĐƯỢC NGAY.
//
// Một mã giảm 90% do gõ nhầm sẽ có hiệu lực tức thì nếu mặc định là
// ACTIVE — và không có bước nào để ai đó nhìn lại trước khi khách dùng.
func TestKhuyenMaiMoiTaoLaBanNhap(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	p, err := m.CreatePromotion(ctx, basePromo())
	if err != nil {
		t.Fatalf("CreatePromotion: %v", err)
	}
	if p.Status != promotion.StatusDraft {
		t.Fatalf("trạng thái = %q, mong DRAFT", p.Status)
	}

	if _, err := m.CreateCoupon(ctx, promotion.CreateCouponRequest{
		PromotionID: p.ID,
		Code:        "SALE10",
	}); err != nil {
		t.Fatalf("CreateCoupon: %v", err)
	}

	if _, err := validate(m, "SALE10", 500_000); !errors.Is(err, promotion.ErrNotActive) {
		t.Fatalf("áp mã của khuyến mãi DRAFT = %v, mong ErrNotActive", err)
	}

	if err := m.ActivatePromotion(ctx, p.ID); err != nil {
		t.Fatalf("ActivatePromotion: %v", err)
	}
	if _, err := validate(m, "SALE10", 500_000); err != nil {
		t.Fatalf("sau khi kích hoạt: %v", err)
	}
}

// MÃ CHUẨN HÓA VỀ CHỮ HOA.
//
// Khách gõ "sale10" và " SALE10 " phải ra CÙNG một mã — không thì họ nghĩ
// mã hỏng và gọi tổng đài.
func TestMaChuanHoa(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	_, c := setup(t, m, basePromo(), "  sale10  ")

	var stored string
	if err := pool.QueryRow(ctx,
		`SELECT code FROM coupon WHERE id = $1`, c.ID).Scan(&stored); err != nil {
		t.Fatalf("đọc mã: %v", err)
	}
	if stored != "SALE10" {
		t.Fatalf("mã lưu %q, mong \"SALE10\"", stored)
	}

	for _, typed := range []string{"sale10", "SALE10", " Sale10 "} {
		if _, err := validate(m, typed, 500_000); err != nil {
			t.Fatalf("gõ %q: %v", typed, err)
		}
	}
}

// GIẢM THEO PHẦN TRĂM TÍNH BẰNG SỐ NGUYÊN, làm tròn XUỐNG.
//
// 10% của 999.999đ tính bằng float ra 99999.90000000001. Làm tròn LÊN sẽ
// khiến tổng giảm giá của nhiều dòng vượt quá mức đã hứa.
func TestGiamPhanTramLaSoNguyen(t *testing.T) {
	m, _ := newModule(t, newClock())

	setup(t, m, basePromo(), "SALE10")

	for _, tc := range []struct {
		total, want int64
	}{
		{1_000_000, 100_000},
		{999_999, 99_999}, // 99999.9 → cắt xuống
		{9, 0},            // 0.9 → cắt xuống 0
		{0, 0},
	} {
		got, err := validate(m, "SALE10", tc.total)
		if err != nil {
			t.Fatalf("đơn %d: %v", tc.total, err)
		}
		if got.Discount != tc.want {
			t.Fatalf("đơn %d giảm %d, mong %d", tc.total, got.Discount, tc.want)
		}
	}
}

// KHÔNG BAO GIỜ GIẢM NHIỀU HƠN GIÁ TRỊ ĐƠN.
//
// Đây là chặn trên quan trọng nhất: giảm nhiều hơn giá trị đơn nghĩa là
// nền tảng TRẢ TIỀN cho khách để họ mua hàng. Một lỗi cấu hình — mã giảm
// 500.000đ dùng cho đơn 200.000đ — là đủ.
func TestKhongGiamQuaGiaTriDon(t *testing.T) {
	m, _ := newModule(t, newClock())

	req := basePromo()
	req.DiscountType = promotion.DiscountFixed
	req.DiscountAmount = 500_000
	req.DiscountBPS = 0
	setup(t, m, req, "GIAM500K")

	got, err := validate(m, "GIAM500K", 200_000)
	if err != nil {
		t.Fatalf("ValidateCoupon: %v", err)
	}
	if got.Discount != 200_000 {
		t.Fatalf("giảm %d cho đơn 200.000đ, mong tối đa 200.000đ", got.Discount)
	}
	if got.Discount > 200_000 {
		t.Fatal("NỀN TẢNG TRẢ TIỀN CHO KHÁCH ĐỂ HỌ MUA HÀNG")
	}
}

// TRẦN GIẢM GIÁ: "giảm 50%, tối đa 100.000đ".
//
// Không có nó, một đơn 10 triệu được giảm 5 triệu.
func TestTranGiamGia(t *testing.T) {
	m, _ := newModule(t, newClock())

	req := basePromo()
	req.DiscountBPS = 5000 // 50%
	req.MaxDiscountAmount = 100_000
	setup(t, m, req, "SALE50")

	// Đơn nhỏ: chưa chạm trần.
	got, err := validate(m, "SALE50", 100_000)
	if err != nil {
		t.Fatalf("ValidateCoupon: %v", err)
	}
	if got.Discount != 50_000 {
		t.Fatalf("đơn 100.000đ giảm %d, mong 50.000đ", got.Discount)
	}

	// Đơn lớn: chạm trần.
	got, err = validate(m, "SALE50", 10_000_000)
	if err != nil {
		t.Fatalf("ValidateCoupon: %v", err)
	}
	if got.Discount != 100_000 {
		t.Fatalf("đơn 10 triệu giảm %d, mong trần 100.000đ", got.Discount)
	}
}

// MIỄN PHÍ SHIP THEO NGƯỠNG — tính năng MVP.
//
// Cờ FreeShipping tách khỏi Discount: phí vận chuyển do module khác tính,
// và promotion không biết nó là bao nhiêu.
func TestMienPhiShipTheoNguong(t *testing.T) {
	m, _ := newModule(t, newClock())

	req := basePromo()
	req.Name = "Miễn phí ship từ 500k"
	req.Kind = promotion.KindFreeShipping
	req.DiscountType = promotion.DiscountFreeShip
	req.DiscountBPS = 0
	req.MinOrderAmount = 500_000
	setup(t, m, req, "FREESHIP")

	// Chưa đạt ngưỡng.
	if _, err := validate(m, "FREESHIP", 499_999); !errors.Is(err, promotion.ErrBelowMinimum) {
		t.Fatalf("đơn 499.999đ = %v, mong ErrBelowMinimum", err)
	}

	// Đạt ngưỡng.
	got, err := validate(m, "FREESHIP", 500_000)
	if err != nil {
		t.Fatalf("đơn 500.000đ: %v", err)
	}
	if !got.FreeShipping {
		t.Fatal("không bật cờ FreeShipping")
	}
	if got.Discount != 0 {
		t.Fatalf("giảm %d cho hàng hóa, mong 0 — phí ship do module khác tính",
			got.Discount)
	}
}

// GIỚI HẠN LƯỢT TOÀN CỤC, và mã tự chuyển sang EXHAUSTED.
func TestGioiHanLuotToanCuc(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	req := basePromo()
	req.MaxUses = 2
	p, _ := setup(t, m, req, "SALE10")

	for i := 0; i < 2; i++ {
		if err := m.RecordUsage(ctx, promotion.RecordUsageRequest{
			Code:     "SALE10",
			OrderID:  ids.MustNew(ids.PrefixOrder).String(),
			Discount: 50_000,
		}); err != nil {
			t.Fatalf("RecordUsage lần %d: %v", i+1, err)
		}
	}

	got, err := m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}
	if got.Status != promotion.StatusExhausted {
		t.Fatalf("trạng thái = %q sau khi hết lượt, mong EXHAUSTED", got.Status)
	}

	if _, err := validate(m, "SALE10", 500_000); !errors.Is(err, promotion.ErrUsageLimitReached) {
		t.Fatalf("áp mã đã hết lượt = %v, mong ErrUsageLimitReached", err)
	}
}

// GIỚI HẠN LƯỢT MỖI KHÁCH.
//
// Thiếu nó thì một người dùng mã "giảm 100k cho khách mới" được vô số lần
// bằng cách tạo đơn liên tục.
func TestGioiHanLuotMoiKhach(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	req := basePromo()
	req.MaxUsesPerCustomer = 1
	setup(t, m, req, "SALE10")

	khachA := ids.MustNew(ids.PrefixCustomer).String()
	khachB := ids.MustNew(ids.PrefixCustomer).String()

	if err := m.RecordUsage(ctx, promotion.RecordUsageRequest{
		Code:       "SALE10",
		CustomerID: khachA,
		OrderID:    ids.MustNew(ids.PrefixOrder).String(),
		Discount:   50_000,
	}); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	// Khách A hết lượt.
	_, err := m.ValidateCoupon(ctx, promotion.ValidateRequest{
		Code:       "SALE10",
		CustomerID: khachA,
		OrderTotal: 500_000,
	})
	if !errors.Is(err, promotion.ErrCustomerLimitReached) {
		t.Fatalf("khách A dùng lần 2 = %v, mong ErrCustomerLimitReached", err)
	}

	// Khách B vẫn dùng được.
	if _, err := m.ValidateCoupon(ctx, promotion.ValidateRequest{
		Code:       "SALE10",
		CustomerID: khachB,
		OrderTotal: 500_000,
	}); err != nil {
		t.Fatalf("khách B: %v", err)
	}
}

// NGÂN SÁCH CHẶN CHI PHÍ, KHÁC với giới hạn lượt.
//
// Một mã giảm 10% không biết trước mỗi lượt tốn bao nhiêu, nên giới hạn
// theo lượt KHÔNG chặn được chi phí.
func TestNganSachChanChiPhi(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	req := basePromo()
	req.MaxBudget = 150_000
	p, _ := setup(t, m, req, "SALE10")

	// Tiêu 100.000đ.
	if err := m.RecordUsage(ctx, promotion.RecordUsageRequest{
		Code:     "SALE10",
		OrderID:  ids.MustNew(ids.PrefixOrder).String(),
		Discount: 100_000,
	}); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	// Còn 50.000đ: đơn lớn chỉ được giảm tối đa phần còn lại.
	got, err := validate(m, "SALE10", 10_000_000)
	if err != nil {
		t.Fatalf("ValidateCoupon: %v", err)
	}
	if got.Discount != 50_000 {
		t.Fatalf("giảm %d, mong 50.000đ (phần ngân sách còn lại)", got.Discount)
	}

	// Tiêu nốt.
	if err := m.RecordUsage(ctx, promotion.RecordUsageRequest{
		Code:     "SALE10",
		OrderID:  ids.MustNew(ids.PrefixOrder).String(),
		Discount: 50_000,
	}); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	view, err := m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}
	if view.Status != promotion.StatusExhausted {
		t.Fatalf("trạng thái = %q sau khi hết ngân sách, mong EXHAUSTED", view.Status)
	}

	if _, err := validate(m, "SALE10", 500_000); !errors.Is(err, promotion.ErrUsageLimitReached) {
		t.Fatalf("mã hết ngân sách = %v", err)
	}
}

// KHUYẾN MÃI HẾT HẠN KHÔNG ÁP ĐƯỢC.
//
// Thời điểm kết thúc là HẾT HẠN, không phải còn dùng được. Lệch một giây
// là mã sống thêm một giây sau khi chiến dịch đã đóng.
func TestKhuyenMaiHetHan(t *testing.T) {
	clock := newClock()
	m, _ := newModule(t, clock)
	ctx := context.Background()

	p, _ := setup(t, m, basePromo(), "SALE10")

	if _, err := validate(m, "SALE10", 500_000); err != nil {
		t.Fatalf("còn hạn: %v", err)
	}

	// Nhảy tới ĐÚNG thời điểm kết thúc.
	view, err := m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}
	clock.advance(view.EndsAt.Sub(clock.Now()))

	if _, err := validate(m, "SALE10", 500_000); !errors.Is(err, promotion.ErrExpired) {
		t.Fatalf("đúng thời điểm kết thúc = %v, mong ErrExpired", err)
	}
}

// KHUYẾN MÃI CHƯA BẮT ĐẦU trả lỗi RIÊNG.
//
// Báo "hết hạn" cho mã chưa bắt đầu khiến khách bỏ đi thay vì quay lại.
func TestKhuyenMaiChuaBatDau(t *testing.T) {
	m, _ := newModule(t, newClock())

	req := basePromo()
	req.StartsAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	req.EndsAt = req.StartsAt.AddDate(0, 1, 0)
	setup(t, m, req, "SALE10")

	if _, err := validate(m, "SALE10", 500_000); !errors.Is(err, promotion.ErrNotStarted) {
		t.Fatalf("chưa tới ngày = %v, mong ErrNotStarted", err)
	}
}

// WORKER HẾT HẠN chuyển trạng thái, tạo LỚP BẢO VỆ THỨ HAI.
//
// Không có nó, khuyến mãi hết hạn vẫn mang trạng thái ACTIVE và chỉ bị
// chặn bởi phép so sánh thời gian.
func TestWorkerHetHanDoiTrangThai(t *testing.T) {
	clock := newClock()
	m, _ := newModule(t, clock)
	ctx := context.Background()

	p, _ := setup(t, m, basePromo(), "SALE10")

	clock.advance(90 * 24 * time.Hour)

	n, err := m.ExpireDuePromotions(ctx)
	if err != nil {
		t.Fatalf("ExpireDuePromotions: %v", err)
	}
	if n != 1 {
		t.Fatalf("đổi %d khuyến mãi, mong 1", n)
	}

	view, err := m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}
	if view.Status != promotion.StatusExpired {
		t.Fatalf("trạng thái = %q, mong EXPIRED", view.Status)
	}

	// Gọi lại KHÔNG đổi gì nữa — idempotent.
	n, err = m.ExpireDuePromotions(ctx)
	if err != nil {
		t.Fatalf("ExpireDuePromotions lần 2: %v", err)
	}
	if n != 0 {
		t.Fatalf("lần 2 đổi %d khuyến mãi, mong 0", n)
	}
}

// GHI NHẬN SỬ DỤNG LÀ IDEMPOTENT.
//
// Handler event xử lý lại cùng một event là chuyện bình thường. Ghi hai
// lần nghĩa là ngân sách bị trừ hai lần cho MỘT đơn.
func TestGhiNhanSuDungIdempotent(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	p, _ := setup(t, m, basePromo(), "SALE10")
	orderID := ids.MustNew(ids.PrefixOrder).String()

	for i := 0; i < 4; i++ {
		if err := m.RecordUsage(ctx, promotion.RecordUsageRequest{
			Code:     "SALE10",
			OrderID:  orderID,
			Discount: 50_000,
		}); err != nil {
			t.Fatalf("RecordUsage lần %d: %v", i+1, err)
		}
	}

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM coupon_usage WHERE order_id = $1`,
		orderID).Scan(&rows); err != nil {
		t.Fatalf("đếm lượt sử dụng: %v", err)
	}
	if rows != 1 {
		t.Fatalf("ghi %d lượt cho một đơn, mong 1", rows)
	}

	view, err := m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}
	if view.UsedCount != 1 {
		t.Fatalf("bộ đếm = %d, mong 1", view.UsedCount)
	}
	if view.UsedBudget != 50_000 {
		t.Fatalf("ngân sách đã tiêu = %d, mong 50.000đ", view.UsedBudget)
	}
}

// ĐUA NHAU GHI NHẬN CÙNG MỘT ĐƠN: ngân sách chỉ trừ MỘT lần.
//
// Kiểm tra "đã ghi chưa" ở tầng ứng dụng KHÔNG cứu được — mười request
// cùng đọc thấy chưa có. Chỉ ràng buộc UNIQUE (coupon_id, order_id) chặn.
func TestDuaNhauGhiNhanCungDon(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	p, _ := setup(t, m, basePromo(), "SALE10")
	orderID := ids.MustNew(ids.PrefixOrder).String()

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = m.RecordUsage(ctx, promotion.RecordUsageRequest{
				Code:     "SALE10",
				OrderID:  orderID,
				Discount: 50_000,
			})
		}()
	}
	wg.Wait()

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM coupon_usage WHERE order_id = $1`,
		orderID).Scan(&rows); err != nil {
		t.Fatalf("đếm lượt sử dụng: %v", err)
	}
	if rows != 1 {
		t.Fatalf("%d request song song ghi %d lượt, mong ĐÚNG 1", n, rows)
	}

	view, err := m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}
	if view.UsedBudget != 50_000 {
		t.Fatalf("ngân sách đã tiêu = %d, mong 50.000đ — bị trừ nhiều lần",
			view.UsedBudget)
	}
}

// KHÓA LẠC QUAN CHẶN MẤT CẬP NHẬT BỘ ĐẾM.
//
// Một mã đang chạy quảng cáo có hàng trăm người cùng áp trong một giây, và
// MỖI lượt đều tăng used_count. Không có khóa lạc quan thì bộ đếm thấp hơn
// số lượt thật — mã giới hạn 100 lượt sẽ được dùng vài trăm lần.
func TestKhoaLacQuanChanMatBoDem(t *testing.T) {
	m, pool := newModule(t, newClock())
	ctx := context.Background()

	p, _ := setup(t, m, basePromo(), "SALE10")

	const n = 12
	var (
		wg sync.WaitGroup
		mu sync.Mutex
		ok int
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			err := m.RecordUsage(ctx, promotion.RecordUsageRequest{
				Code:     "SALE10",
				OrderID:  ids.MustNew(ids.PrefixOrder).String(),
				Discount: 10_000,
			})
			if err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Bộ đếm PHẢI bằng đúng số lượt đã ghi vào bảng — bảng là nguồn sự
	// thật, bộ đếm chỉ là bản tóm tắt.
	var usages int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM coupon_usage WHERE promotion_id = $1`,
		p.ID).Scan(&usages); err != nil {
		t.Fatalf("đếm lượt sử dụng: %v", err)
	}

	view, err := m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}

	if view.UsedCount != usages {
		t.Fatalf("bộ đếm = %d nhưng bảng có %d lượt — MẤT CẬP NHẬT",
			view.UsedCount, usages)
	}
	if int64(usages)*10_000 != view.UsedBudget {
		t.Fatalf("ngân sách = %d nhưng %d lượt × 10.000đ = %d",
			view.UsedBudget, usages, int64(usages)*10_000)
	}
}

// HỦY ĐƠN GIẢI PHÓNG LƯỢT, và mã sống lại từ EXHAUSTED.
//
// Không giải phóng thì mã hết lượt vì những đơn không bao giờ thành — và
// chiến dịch kết thúc sớm hơn dự tính.
func TestHuyDonGiaiPhongLuot(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	req := basePromo()
	req.MaxUses = 1
	p, _ := setup(t, m, req, "SALE10")

	orderID := ids.MustNew(ids.PrefixOrder).String()
	if err := m.RecordUsage(ctx, promotion.RecordUsageRequest{
		Code:     "SALE10",
		OrderID:  orderID,
		Discount: 50_000,
	}); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	view, _ := m.GetPromotion(ctx, p.ID)
	if view.Status != promotion.StatusExhausted {
		t.Fatalf("trạng thái = %q, mong EXHAUSTED", view.Status)
	}

	n, err := m.ReleaseUsage(ctx, orderID)
	if err != nil {
		t.Fatalf("ReleaseUsage: %v", err)
	}
	if n != 1 {
		t.Fatalf("giải phóng %d lượt, mong 1", n)
	}

	view, err = m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}
	if view.Status != promotion.StatusActive {
		t.Fatalf("trạng thái = %q sau khi giải phóng, mong ACTIVE", view.Status)
	}
	if view.UsedCount != 0 || view.UsedBudget != 0 {
		t.Fatalf("bộ đếm = %d, ngân sách = %d, mong 0 và 0",
			view.UsedCount, view.UsedBudget)
	}

	// Mã dùng lại được.
	if _, err := validate(m, "SALE10", 500_000); err != nil {
		t.Fatalf("sau khi giải phóng: %v", err)
	}
}

// GIẢI PHÓNG LÀ IDEMPOTENT: gọi lại KHÔNG trừ ngân sách lần nữa.
func TestGiaiPhongIdempotent(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	p, _ := setup(t, m, basePromo(), "SALE10")

	orderID := ids.MustNew(ids.PrefixOrder).String()
	if err := m.RecordUsage(ctx, promotion.RecordUsageRequest{
		Code:     "SALE10",
		OrderID:  orderID,
		Discount: 50_000,
	}); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	for i := 0; i < 3; i++ {
		n, err := m.ReleaseUsage(ctx, orderID)
		if err != nil {
			t.Fatalf("ReleaseUsage lần %d: %v", i+1, err)
		}
		want := 0
		if i == 0 {
			want = 1
		}
		if n != want {
			t.Fatalf("lần %d giải phóng %d lượt, mong %d", i+1, n, want)
		}
	}

	view, err := m.GetPromotion(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPromotion: %v", err)
	}
	if view.UsedBudget != 0 {
		t.Fatalf("ngân sách = %d sau ba lần giải phóng, mong 0 — bị trừ nhiều lần",
			view.UsedBudget)
	}

	// Đơn chưa từng dùng mã: giải phóng vẫn thành công, không đổi gì.
	n, err := m.ReleaseUsage(ctx, ids.MustNew(ids.PrefixOrder).String())
	if err != nil {
		t.Fatalf("giải phóng đơn lạ: %v", err)
	}
	if n != 0 {
		t.Fatalf("giải phóng %d lượt cho đơn chưa dùng mã", n)
	}
}

// PHÂN BỔ GIẢM GIÁ XUỐNG DÒNG HÀNG, KHÔNG MẤT ĐỒNG NÀO.
//
// Khách trả một món → hoàn giá dòng TRỪ phần giảm đã phân bổ. Không lưu
// lại thì nền tảng hoàn nhiều hơn đã thu.
func TestPhanBoGiamGiaXuongDongHang(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	got, err := m.AllocateDiscount(ctx, promotion.AllocateRequest{
		Discount: 50_000,
		Lines: []promotion.AllocateLine{
			{LineID: "A", Total: 200_000},
			{LineID: "B", Total: 200_000},
			{LineID: "C", Total: 100_000},
		},
	})
	if err != nil {
		t.Fatalf("AllocateDiscount: %v", err)
	}

	want := map[string]int64{"A": 20_000, "B": 20_000, "C": 10_000}
	var sum int64
	for _, line := range got {
		if line.Discount != want[line.LineID] {
			t.Fatalf("dòng %s giảm %d, mong %d",
				line.LineID, line.Discount, want[line.LineID])
		}
		sum += line.Discount
	}
	if sum != 50_000 {
		t.Fatalf("tổng phân bổ = %d, mong ĐÚNG 50.000đ", sum)
	}
}

// PHÂN BỔ CHIA KHÔNG HẾT KHÔNG ĐƯỢC LÀM MẤT TIỀN.
//
// 100đ chia cho 3 dòng bằng nhau = 33,33đ mỗi dòng. Cắt xuống thành 33 là
// mất 1đ; với hàng triệu đơn, chỗ mất đó thành tiền thật và không có bút
// toán nào giải thích.
func TestPhanBoChiaKhongHetKhongMatTien(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	got, err := m.AllocateDiscount(ctx, promotion.AllocateRequest{
		Discount: 100,
		Lines: []promotion.AllocateLine{
			{LineID: "A", Total: 1000},
			{LineID: "B", Total: 1000},
			{LineID: "C", Total: 1000},
		},
	})
	if err != nil {
		t.Fatalf("AllocateDiscount: %v", err)
	}

	var sum int64
	for _, line := range got {
		sum += line.Discount
	}
	if sum != 100 {
		t.Fatalf("tổng phân bổ = %d, MẤT %dđ", sum, 100-sum)
	}
}

// AI CHỊU CHI PHÍ — vấn đề cốt lõi của module.
//
// Không trả lời được thì không tính được seller thực nhận bao nhiêu, và
// đối soát cuối tháng sẽ lệch đúng bằng tổng tiền khuyến mãi.
func TestAiChiuChiPhiKhuyenMai(t *testing.T) {
	m, _ := newModule(t, newClock())
	sellerID := ids.MustNew(ids.PrefixSeller).String()

	t.Run("nền tảng chịu", func(t *testing.T) {
		m, _ := newModule(t, newClock())
		req := basePromo()
		req.CostBearer = promotion.BearerPlatform
		setup(t, m, req, "PLATFORM10")

		got, err := validate(m, "PLATFORM10", 1_000_000)
		if err != nil {
			t.Fatalf("ValidateCoupon: %v", err)
		}
		if len(got.CostAllocations) != 1 {
			t.Fatalf("có %d phân bổ, mong 1", len(got.CostAllocations))
		}
		a := got.CostAllocations[0]
		if a.Bearer != promotion.BearerPlatform || a.Amount != 100_000 {
			t.Fatalf("phân bổ = %+v, mong PLATFORM chịu 100.000đ", a)
		}
	})

	t.Run("seller chịu", func(t *testing.T) {
		m, _ := newModule(t, newClock())
		req := basePromo()
		req.CostBearer = promotion.BearerSeller
		req.SellerID = sellerID
		setup(t, m, req, "SELLER10")

		got, err := m.ValidateCoupon(context.Background(), promotion.ValidateRequest{
			Code:       "SELLER10",
			SellerID:   sellerID,
			OrderTotal: 1_000_000,
		})
		if err != nil {
			t.Fatalf("ValidateCoupon: %v", err)
		}
		a := got.CostAllocations[0]
		if a.Bearer != promotion.BearerSeller {
			t.Fatalf("bên chịu = %q, mong SELLER", a.Bearer)
		}
		if a.SellerID != sellerID {
			t.Fatalf("seller = %q, mong %q", a.SellerID, sellerID)
		}
		if a.Amount != 100_000 {
			t.Fatalf("số tiền = %d, mong 100.000đ", a.Amount)
		}
	})

	t.Run("chia đôi", func(t *testing.T) {
		m, _ := newModule(t, newClock())
		req := basePromo()
		req.CostBearer = promotion.BearerShared
		req.PlatformShareBPS = 6000 // 60%
		req.SellerShareBPS = 4000   // 40%
		req.SellerID = sellerID
		setup(t, m, req, "SHARED10")

		got, err := m.ValidateCoupon(context.Background(), promotion.ValidateRequest{
			Code:       "SHARED10",
			SellerID:   sellerID,
			OrderTotal: 999_999,
		})
		if err != nil {
			t.Fatalf("ValidateCoupon: %v", err)
		}
		if len(got.CostAllocations) != 2 {
			t.Fatalf("có %d phân bổ, mong 2", len(got.CostAllocations))
		}

		var sum int64
		for _, a := range got.CostAllocations {
			sum += a.Amount
		}
		// TỔNG PHẢI BẰNG ĐÚNG SỐ TIỀN GIẢM: lệch một đồng là một khoản
		// KHÔNG AI CHỊU, xuất hiện ở mọi đơn dùng mã.
		if sum != got.Discount {
			t.Fatalf("tổng phân bổ = %d nhưng giảm %d — %dđ KHÔNG AI CHỊU",
				sum, got.Discount, got.Discount-sum)
		}
		// Đơn 999.999đ giảm 10% = 99.999đ — số LẺ, chia 60/40 KHÔNG hết.
		//
		// Dùng số lẻ có chủ ý: với 100.000đ tròn thì phép chia nào cũng
		// đúng, và test sẽ không phát hiện được cách chia làm mất tiền.
		// Đã kiểm chứng ngược bằng cách thay Allocate bằng hai phép nhân
		// chia riêng — mất đúng 1đ.
		if got.CostAllocations[0].Amount != 60_000 {
			t.Fatalf("nền tảng chịu %d, mong 60.000đ (60%% của 99.999đ, "+
				"phần dư rải cho bên đầu)", got.CostAllocations[0].Amount)
		}
		if got.CostAllocations[1].Amount != 39_999 {
			t.Fatalf("seller chịu %d, mong 39.999đ", got.CostAllocations[1].Amount)
		}
	})

	_ = m
}

// TỶ LỆ CHIA PHẢI CỘNG ĐÚNG 100%.
//
// Lệch một điểm cơ bản là một khoản tiền KHÔNG AI CHỊU.
func TestTyLeChiaPhaiCong100(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	req := basePromo()
	req.CostBearer = promotion.BearerShared
	req.PlatformShareBPS = 6000
	req.SellerShareBPS = 3999 // thiếu 1 điểm cơ bản
	req.SellerID = ids.MustNew(ids.PrefixSeller).String()

	if _, err := m.CreatePromotion(ctx, req); !errors.Is(err, promotion.ErrInvalidInput) {
		t.Fatalf("tỷ lệ cộng 9999 = %v, mong ErrInvalidInput", err)
	}
}

// MÃ RIÊNG CHỈ DÀNH CHO MỘT KHÁCH.
func TestMaRiengChiDanhChoMotKhach(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	p, err := m.CreatePromotion(ctx, basePromo())
	if err != nil {
		t.Fatalf("CreatePromotion: %v", err)
	}
	if err := m.ActivatePromotion(ctx, p.ID); err != nil {
		t.Fatalf("ActivatePromotion: %v", err)
	}

	chuNhan := ids.MustNew(ids.PrefixCustomer).String()
	nguoiKhac := ids.MustNew(ids.PrefixCustomer).String()

	if _, err := m.CreateCoupon(ctx, promotion.CreateCouponRequest{
		PromotionID: p.ID,
		Code:        "XINLOI50",
		CustomerID:  chuNhan,
	}); err != nil {
		t.Fatalf("CreateCoupon: %v", err)
	}

	if _, err := m.ValidateCoupon(ctx, promotion.ValidateRequest{
		Code:       "XINLOI50",
		CustomerID: chuNhan,
		OrderTotal: 500_000,
	}); err != nil {
		t.Fatalf("chủ nhân dùng mã: %v", err)
	}

	_, err = m.ValidateCoupon(ctx, promotion.ValidateRequest{
		Code:       "XINLOI50",
		CustomerID: nguoiKhac,
		OrderTotal: 500_000,
	})
	if !errors.Is(err, promotion.ErrWrongCustomer) {
		t.Fatalf("người khác dùng = %v, mong ErrWrongCustomer", err)
	}
}

// MÃ CỦA GIAN HÀNG KHÔNG ÁP CHO GIAN HÀNG KHÁC.
func TestMaCuaGianHangKhongApChoGianHangKhac(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	req := basePromo()
	req.SellerID = sellerA
	req.CostBearer = promotion.BearerSeller
	setup(t, m, req, "SHOPA10")

	if _, err := m.ValidateCoupon(ctx, promotion.ValidateRequest{
		Code:       "SHOPA10",
		SellerID:   sellerA,
		OrderTotal: 500_000,
	}); err != nil {
		t.Fatalf("đơn của seller A: %v", err)
	}

	_, err := m.ValidateCoupon(ctx, promotion.ValidateRequest{
		Code:       "SHOPA10",
		SellerID:   sellerB,
		OrderTotal: 500_000,
	})
	if !errors.Is(err, promotion.ErrWrongSeller) {
		t.Fatalf("đơn của seller B = %v, mong ErrWrongSeller", err)
	}
}

// TỪ CHỐI KHUYẾN MÃI KHÔNG CÓ NGHĨA.
//
// "Giảm 0đ" qua mọi kiểm tra, khách áp được, và không giảm gì cả — rồi bộ
// phận hỗ trợ nhận khiếu nại "mã của tôi không hoạt động".
func TestTuChoiKhuyenMaiKhongCoNghia(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	for name, mutate := range map[string]func(*promotion.CreatePromotionRequest){
		"phần trăm bằng 0": func(r *promotion.CreatePromotionRequest) {
			r.DiscountBPS = 0
		},
		"số tiền bằng 0": func(r *promotion.CreatePromotionRequest) {
			r.DiscountType = promotion.DiscountFixed
			r.DiscountBPS = 0
			r.DiscountAmount = 0
		},
		"tên rỗng": func(r *promotion.CreatePromotionRequest) {
			r.Name = "   "
		},
		"kết thúc trước khi bắt đầu": func(r *promotion.CreatePromotionRequest) {
			r.EndsAt = r.StartsAt.Add(-time.Hour)
		},
	} {
		req := basePromo()
		mutate(&req)
		if _, err := m.CreatePromotion(ctx, req); !errors.Is(err, promotion.ErrInvalidInput) {
			t.Fatalf("%s = %v, mong ErrInvalidInput", name, err)
		}
	}
}

// TẠM DỪNG KHUYẾN MÃI CHẶN ÁP MÃ NGAY.
func TestTamDungChanApMa(t *testing.T) {
	m, _ := newModule(t, newClock())
	ctx := context.Background()

	p, _ := setup(t, m, basePromo(), "SALE10")

	if err := m.PausePromotion(ctx, p.ID); err != nil {
		t.Fatalf("PausePromotion: %v", err)
	}
	if _, err := validate(m, "SALE10", 500_000); !errors.Is(err, promotion.ErrNotActive) {
		t.Fatalf("khuyến mãi tạm dừng = %v, mong ErrNotActive", err)
	}

	if err := m.ActivatePromotion(ctx, p.ID); err != nil {
		t.Fatalf("ActivatePromotion: %v", err)
	}
	if _, err := validate(m, "SALE10", 500_000); err != nil {
		t.Fatalf("sau khi bật lại: %v", err)
	}
}

// MÃ KHÔNG TỒN TẠI trả lỗi riêng, không phải lỗi chung.
func TestMaKhongTonTai(t *testing.T) {
	m, _ := newModule(t, newClock())

	if _, err := validate(m, "KHONGTONTAI", 500_000); !errors.Is(err, promotion.ErrCouponNotFound) {
		t.Fatalf("mã lạ = %v, mong ErrCouponNotFound", err)
	}
}
