package pricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/pricing"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

func newModule(t *testing.T) *pricing.Module {
	t.Helper()
	m, err := pricing.New(pricing.Config{Storage: "memory"})
	if err != nil {
		t.Fatalf("pricing.New: %v", err)
	}
	return m
}

func TestKhoLuuTruKhongHopLeBiTuChoi(t *testing.T) {
	if _, err := pricing.New(pricing.Config{Storage: "khong-co-that"}); err == nil {
		t.Error("mong lỗi với kho lưu trữ không hợp lệ")
	}
	// postgres chưa cài đặt — phải báo rõ chứ không im lặng dùng in-memory.
	if _, err := pricing.New(pricing.Config{Storage: "postgres"}); err == nil {
		t.Error("mong lỗi khi postgres chưa được cài đặt")
	}
}

// Dữ liệu mẫu phải chạy được qua API CÔNG KHAI, không chỉ qua tầng nội bộ.
func TestNapDuLieuMauVaTraGiaQuaAPICongKhai(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)

	sku1 := ids.MustNew(ids.PrefixSKU).String()
	sku2 := ids.MustNew(ids.PrefixSKU).String()

	seeded, err := pricing.SeedDemo(ctx, m, pricing.SeedInput{SKUIDs: []string{sku1, sku2}})
	if err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	got, err := m.GetPrice(ctx, pricing.PriceRequest{SKUID: seeded.PricedSKUID})
	if err != nil {
		t.Fatalf("GetPrice: %v", err)
	}
	if got.Amount.Value != 490000 || got.Amount.Currency != "VND" {
		t.Errorf("giá = %d %s, mong 490000 VND", got.Amount.Value, got.Amount.Currency)
	}
	if got.PriceType != "BASE" {
		t.Errorf("loại giá = %q, mong BASE", got.PriceType)
	}
	// 100000/590000 → 1694 phần vạn.
	if got.DiscountBasisPoints != 1694 {
		t.Errorf("mức giảm = %d bp, mong 1694", got.DiscountBasisPoints)
	}
}

// Quy tắc 2 qua API công khai: giá xả hàng thắng giá gốc.
func TestGiaXaHangThangGiaGocQuaAPICongKhai(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)

	sku1 := ids.MustNew(ids.PrefixSKU).String()
	sku2 := ids.MustNew(ids.PrefixSKU).String()
	seeded, err := pricing.SeedDemo(ctx, m, pricing.SeedInput{SKUIDs: []string{sku1, sku2}})
	if err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	got, err := m.GetPrice(ctx, pricing.PriceRequest{SKUID: seeded.ClearanceSKUID})
	if err != nil {
		t.Fatalf("GetPrice: %v", err)
	}
	if got.PriceType != "CLEARANCE" {
		t.Errorf("loại giá = %q, mong CLEARANCE", got.PriceType)
	}
	if got.Amount.Value != 299000 {
		t.Errorf("giá = %d, mong 299000", got.Amount.Value)
	}
}

func TestIDSaiDinhDangTraErrInvalidID(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)

	for _, id := range []string{"khong-phai-id", "prd_01J9XABC123DEF456GHJKMNPQR", ""} {
		if _, err := m.GetPrice(ctx, pricing.PriceRequest{SKUID: id}); !errors.Is(err, pricing.ErrInvalidID) {
			t.Errorf("GetPrice(%q): lỗi = %v, mong ErrInvalidID", id, err)
		}
		if _, err := m.GetPriceConstraint(ctx, id); !errors.Is(err, pricing.ErrInvalidID) {
			t.Errorf("GetPriceConstraint(%q): lỗi = %v, mong ErrInvalidID", id, err)
		}
	}
}

func TestSKUChuaCoGiaTraErrNoPrice(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)

	_, err := m.GetPrice(ctx, pricing.PriceRequest{SKUID: ids.MustNew(ids.PrefixSKU).String()})
	if !errors.Is(err, pricing.ErrNoPrice) {
		t.Errorf("lỗi = %v, mong ErrNoPrice", err)
	}
}

// Đơn vị tiền tệ không hợp lệ KHÔNG được coi là giá hợp lệ — nếu bỏ qua,
// offer sẽ lưu với giá không kiểm chứng được.
func TestDonViTienTeKhongHopLeBiChan(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	got, err := m.ValidateSellerPrice(ctx, skuID, 100000, "KHONG-CO-THAT")
	if err != nil {
		t.Fatalf("ValidateSellerPrice: %v", err)
	}
	if got.Allowed {
		t.Error("đơn vị tiền tệ không hợp lệ phải bị chặn")
	}
	if got.Code != "CURRENCY_MISMATCH" {
		t.Errorf("Code = %q, mong CURRENCY_MISMATCH", got.Code)
	}
}

// Khung giá qua API công khai — hàng rào chống lỗi nhập liệu của seller.
func TestKiemTraGiaSellerQuaAPICongKhai(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)

	sku1 := ids.MustNew(ids.PrefixSKU).String()
	seeded, err := pricing.SeedDemo(ctx, m, pricing.SeedInput{SKUIDs: []string{sku1}})
	if err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	cases := []struct {
		ten    string
		gia    int64
		mongOK bool
	}{
		{"trong khung", 450000, true},
		{"gõ thiếu số 0", 49000, false},
		{"thổi giá", 2000000, false},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			got, err := m.ValidateSellerPrice(ctx, seeded.PricedSKUID, tc.gia, "VND")
			if err != nil {
				t.Fatalf("ValidateSellerPrice: %v", err)
			}
			if got.Allowed != tc.mongOK {
				t.Errorf("Allowed = %v, mong %v (%s)", got.Allowed, tc.mongOK, got.Message)
			}
			// Kết quả phải kèm khung giá để seller biết sửa thế nào.
			if !tc.mongOK && got.MinPrice.Value == 0 && got.MaxPrice.Value == 0 {
				t.Error("từ chối mà không cho biết khung giá")
			}
		})
	}
}

// Lịch sử phải đọc được qua API công khai, kèm lý do — thiếu lý do thì
// không rà soát thao túng giá được.
func TestLichSuGiaQuaAPICongKhai(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)

	sku1 := ids.MustNew(ids.PrefixSKU).String()
	sku2 := ids.MustNew(ids.PrefixSKU).String()
	seeded, err := pricing.SeedDemo(ctx, m, pricing.SeedInput{SKUIDs: []string{sku1, sku2}})
	if err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	// SKU thứ hai có hai lần đặt giá: giá gốc rồi giá xả hàng.
	points, err := m.GetPriceHistory(ctx, seeded.ClearanceSKUID, pricing.DateRange{})
	if err != nil {
		t.Fatalf("GetPriceHistory: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("số điểm lịch sử = %d, mong 2", len(points))
	}
	for i, p := range points {
		if p.Reason == "" {
			t.Errorf("điểm %d thiếu lý do", i)
		}
		// Thời điểm phải đúng RFC3339 để client parse được.
		if _, err := time.Parse(time.RFC3339, p.RecordedAt); err != nil {
			t.Errorf("điểm %d: RecordedAt %q không đúng RFC3339", i, p.RecordedAt)
		}
	}
}

func TestKhoangThoiGianSaiDinhDangBiTuChoi(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	if _, err := m.GetPriceHistory(ctx, skuID, pricing.DateRange{From: "12/08/2026"}); err == nil {
		t.Error("mong lỗi với thời điểm không đúng RFC3339")
	}
}

// Tra giá theo lô: bỏ qua id sai định dạng thay vì làm hỏng cả lời gọi.
func TestTraGiaTheoLoBoQuaIDSai(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)

	sku1 := ids.MustNew(ids.PrefixSKU).String()
	seeded, err := pricing.SeedDemo(ctx, m, pricing.SeedInput{SKUIDs: []string{sku1}})
	if err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	got, err := m.GetPrices(ctx, []pricing.PriceRequest{
		{SKUID: seeded.PricedSKUID},
		{SKUID: "id-sai-dinh-dang"},
		{SKUID: ids.MustNew(ids.PrefixSKU).String()}, // hợp lệ nhưng chưa có giá
	})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("số kết quả = %d, mong 1", len(got))
	}
	if got[seeded.PricedSKUID].Amount.Value != 490000 {
		t.Errorf("giá = %d, mong 490000", got[seeded.PricedSKUID].Amount.Value)
	}
}

// "Giá thấp nhất 30 ngày qua" qua API công khai.
func TestGiaThapNhat30NgayQuaAPICongKhai(t *testing.T) {
	ctx := context.Background()
	m := newModule(t)

	sku1 := ids.MustNew(ids.PrefixSKU).String()
	sku2 := ids.MustNew(ids.PrefixSKU).String()
	seeded, err := pricing.SeedDemo(ctx, m, pricing.SeedInput{SKUIDs: []string{sku1, sku2}})
	if err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	lowest, ok, err := m.GetLowestPriceLast30Days(ctx, seeded.ClearanceSKUID)
	if err != nil {
		t.Fatalf("GetLowestPriceLast30Days: %v", err)
	}
	if !ok {
		t.Fatal("mong có dữ liệu")
	}
	// Hai mức giá: 520000 và 299000 → thấp nhất là 299000.
	if lowest.Value != 299000 {
		t.Errorf("giá thấp nhất = %d, mong 299000", lowest.Value)
	}

	// SKU chưa có lịch sử trả false, KHÔNG phải lỗi — "chưa có dữ liệu"
	// là trạng thái hợp lệ, không phải sự cố.
	_, ok, err = m.GetLowestPriceLast30Days(ctx, ids.MustNew(ids.PrefixSKU).String())
	if err != nil {
		t.Fatalf("GetLowestPriceLast30Days: %v", err)
	}
	if ok {
		t.Error("SKU chưa có lịch sử phải trả false")
	}
}
