package inmemory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
	"github.com/fashion-commerce/platform/internal/modules/pricing/infrastructure/inmemory"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func newPrice(t *testing.T, skuID ids.ID, amount int64, pt domain.PriceType) *domain.Price {
	t.Helper()
	p := domain.NewPriceParams{
		SKUID: skuID, PriceType: pt, Amount: vnd(amount), Now: testNow,
	}
	if pt.RequiresPeriod() {
		p.Period = domain.Period{From: testNow, To: testNow.Add(4 * time.Hour)}
	}
	got, err := domain.NewPrice(p)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	return got
}

func TestLuuVaDocLaiGia(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewPriceStore()
	skuID := ids.MustNew(ids.PrefixSKU)
	p := newPrice(t, skuID, 490000, domain.PriceTypeBase)

	if err := s.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.FindByID(ctx, p.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Amount().Amount() != 490000 {
		t.Errorf("giá = %v, mong 490000", got.Amount())
	}
	// Đơn vị tiền tệ phải giữ nguyên qua vòng lưu/đọc.
	if got.Amount().Currency() != money.VND {
		t.Errorf("đơn vị = %v, mong VND", got.Amount().Currency())
	}
}

func TestKhongTimThayTraErrNotFound(t *testing.T) {
	ctx := context.Background()

	if _, err := inmemory.NewPriceStore().FindByID(ctx, ids.MustNew(ids.PrefixPrice)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("giá: lỗi = %v, mong ErrNotFound", err)
	}
	if _, err := inmemory.NewConstraintStore().FindBySKU(ctx, ids.MustNew(ids.PrefixSKU)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("khung giá: lỗi = %v, mong ErrNotFound", err)
	}
}

// Kho phải hành xử GIỐNG database: sửa aggregate sau khi Save không được
// làm đổi dữ liệu đã lưu.
func TestSuaSauKhiLuuKhongAnhHuongKho(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewPriceStore()
	skuID := ids.MustNew(ids.PrefixSKU)
	p := newPrice(t, skuID, 490000, domain.PriceTypeBase)

	if err := s.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p.Deactivate(testNow)

	got, err := s.FindByID(ctx, p.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !got.IsActive() {
		t.Error("sửa sau khi Save đã ảnh hưởng dữ liệu trong kho")
	}
}

// Kho trả MỌI mức giá, kể cả đã ngừng — việc chọn mức nào là quyết định
// nghiệp vụ của domain.SelectBest, không phải của kho.
func TestKhoTraCaGiaDaNgungApDung(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewPriceStore()
	skuID := ids.MustNew(ids.PrefixSKU)

	base := newPrice(t, skuID, 490000, domain.PriceTypeBase)
	ngung := newPrice(t, skuID, 250000, domain.PriceTypeClearance)
	ngung.Deactivate(testNow)

	for _, p := range []*domain.Price{base, ngung} {
		if err := s.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	got, err := s.FindBySKU(ctx, skuID)
	if err != nil {
		t.Fatalf("FindBySKU: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("số mức giá = %d, mong 2 (kho không được tự lọc)", len(got))
	}

	// Và domain mới là nơi quyết định.
	best := domain.SelectBest(got, testNow, "", "")
	if best == nil || best.Type() != domain.PriceTypeBase {
		t.Error("SelectBest phải bỏ qua giá đã ngừng áp dụng")
	}
}

// Lưu lại cùng một mức giá không được nhân đôi bản ghi.
func TestLuuLaiKhongNhanDoi(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewPriceStore()
	skuID := ids.MustNew(ids.PrefixSKU)
	p := newPrice(t, skuID, 490000, domain.PriceTypeBase)

	for i := 0; i < 3; i++ {
		if err := s.Save(ctx, p); err != nil {
			t.Fatalf("Save lần %d: %v", i, err)
		}
	}

	got, err := s.FindBySKU(ctx, skuID)
	if err != nil {
		t.Fatalf("FindBySKU: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("số mức giá = %d, mong 1", len(got))
	}
}

// Lấy giá theo lô: hiển thị 50 sản phẩm phải là 1 truy vấn, không phải 50.
func TestLayGiaTheoLo(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewPriceStore()

	sku1 := ids.MustNew(ids.PrefixSKU)
	sku2 := ids.MustNew(ids.PrefixSKU)
	khongCo := ids.MustNew(ids.PrefixSKU)

	if err := s.Save(ctx, newPrice(t, sku1, 100000, domain.PriceTypeBase)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, newPrice(t, sku2, 200000, domain.PriceTypeBase)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.FindBySKUs(ctx, []ids.ID{sku1, sku2, khongCo})
	if err != nil {
		t.Fatalf("FindBySKUs: %v", err)
	}
	// SKU không có giá bị bỏ qua, không làm hỏng cả lời gọi.
	if len(got) != 2 {
		t.Errorf("số kết quả = %d, mong 2", len(got))
	}
	if len(got[sku1]) != 1 || got[sku1][0].Amount().Amount() != 100000 {
		t.Errorf("giá sku1 sai: %v", got[sku1])
	}
}

func TestThuTuGiaOnDinh(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewPriceStore()
	skuID := ids.MustNew(ids.PrefixSKU)

	for _, amount := range []int64{100000, 200000, 300000, 400000, 500000} {
		if err := s.Save(ctx, newPrice(t, skuID, amount, domain.PriceTypeBase)); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	first, err := s.FindBySKU(ctx, skuID)
	if err != nil {
		t.Fatalf("FindBySKU: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, err := s.FindBySKU(ctx, skuID)
		if err != nil {
			t.Fatalf("FindBySKU: %v", err)
		}
		for j := range got {
			if got[j].ID() != first[j].ID() {
				t.Fatalf("thứ tự đổi giữa các lần gọi ở vị trí %d", j)
			}
		}
	}
}

// Bỏ sót ngưỡng cảnh báo khi lưu thì khi đọc lên nó thành 0, và việc phát
// hiện giá bất thường im lặng ngừng hoạt động — loại lỗi không có thông
// báo nào cả.
func TestKhungGiaGiuNguongCanhBaoQuaVongLuuDoc(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewConstraintStore()
	skuID := ids.MustNew(ids.PrefixSKU)

	c, err := domain.NewPriceConstraint(domain.NewConstraintParams{
		SKUID:          skuID,
		MinPrice:       vnd(10000),
		MaxPrice:       vnd(1000000),
		ReferencePrice: vnd(500000),
		Now:            testNow,
	})
	if err != nil {
		t.Fatalf("NewPriceConstraint: %v", err)
	}
	if err := s.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.FindBySKU(ctx, skuID)
	if err != nil {
		t.Fatalf("FindBySKU: %v", err)
	}

	if got.SuspiciousBelowBP() != c.SuspiciousBelowBP() {
		t.Errorf("ngưỡng = %d, mong %d", got.SuspiciousBelowBP(), c.SuspiciousBelowBP())
	}
	// Kiểm chứng bằng HÀNH VI, không chỉ bằng giá trị trường: giá thấp bất
	// thường vẫn phải bị đánh dấu sau khi đọc lại từ kho.
	if res := got.Check(vnd(100000)); !res.NeedsReview {
		t.Error("sau khi đọc lại từ kho, cảnh báo giá bất thường không còn hoạt động")
	}
}

func TestMoiSKUChiMotKhungGia(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewConstraintStore()
	skuID := ids.MustNew(ids.PrefixSKU)

	for _, max := range []int64{500000, 800000} {
		c, err := domain.NewPriceConstraint(domain.NewConstraintParams{
			SKUID: skuID, MinPrice: vnd(100000), MaxPrice: vnd(max), Now: testNow,
		})
		if err != nil {
			t.Fatalf("NewPriceConstraint: %v", err)
		}
		if err := s.Save(ctx, c); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	// Lần lưu sau ghi đè lần trước — không có chuyện hai khung giá mâu thuẫn.
	got, err := s.FindBySKU(ctx, skuID)
	if err != nil {
		t.Fatalf("FindBySKU: %v", err)
	}
	if got.MaxPrice().Amount() != 800000 {
		t.Errorf("max = %v, mong 800000", got.MaxPrice())
	}
}

// Lịch sử CHỈ GHI THÊM — không có phương thức sửa hay xóa.
func TestLichSuChiGhiThem(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewHistoryStore()
	skuID := ids.MustNew(ids.PrefixSKU)

	amounts := []int64{500000, 450000, 400000}
	for i, amount := range amounts {
		p, err := domain.NewPricePoint(domain.NewPricePointParams{
			SKUID:  skuID,
			Amount: vnd(amount),
			Reason: domain.ReasonManual,
			Now:    testNow.AddDate(0, 0, -10+i),
		})
		if err != nil {
			t.Fatalf("NewPricePoint: %v", err)
		}
		if err := s.Append(ctx, p); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := s.FindBySKU(ctx, skuID, domain.DateRange{})
	if err != nil {
		t.Fatalf("FindBySKU: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("số điểm = %d, mong 3", len(got))
	}

	// Trả theo thứ tự thời gian tăng dần.
	for i := 1; i < len(got); i++ {
		if got[i].RecordedAt().Before(got[i-1].RecordedAt()) {
			t.Errorf("vị trí %d sớm hơn vị trí trước — lịch sử không theo thứ tự", i)
		}
	}
}

func TestLichSuLocTheoKhoangThoiGian(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewHistoryStore()
	skuID := ids.MustNew(ids.PrefixSKU)

	for i, ngay := range []int{-45, -20, -5} {
		p, err := domain.NewPricePoint(domain.NewPricePointParams{
			SKUID:  skuID,
			Amount: vnd(int64(100000 * (i + 1))),
			Now:    testNow.AddDate(0, 0, ngay),
		})
		if err != nil {
			t.Fatalf("NewPricePoint: %v", err)
		}
		if err := s.Append(ctx, p); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	// Chỉ lấy 30 ngày gần nhất — bỏ điểm cách đây 45 ngày.
	got, err := s.FindBySKU(ctx, skuID, domain.DateRange{
		From: testNow.AddDate(0, 0, -30), To: testNow,
	})
	if err != nil {
		t.Fatalf("FindBySKU: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("số điểm trong 30 ngày = %d, mong 2", len(got))
	}
}

func TestLichSuSKUKhacKhongLanNhau(t *testing.T) {
	ctx := context.Background()
	s := inmemory.NewHistoryStore()
	sku1 := ids.MustNew(ids.PrefixSKU)
	sku2 := ids.MustNew(ids.PrefixSKU)

	for _, skuID := range []ids.ID{sku1, sku2} {
		p, err := domain.NewPricePoint(domain.NewPricePointParams{
			SKUID: skuID, Amount: vnd(100000), Now: testNow,
		})
		if err != nil {
			t.Fatalf("NewPricePoint: %v", err)
		}
		if err := s.Append(ctx, p); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := s.FindBySKU(ctx, sku1, domain.DateRange{})
	if err != nil {
		t.Fatalf("FindBySKU: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("số điểm = %d, mong 1", len(got))
	}
	if got[0].SKUID() != sku1 {
		t.Error("lịch sử của SKU khác lẫn vào")
	}
}
