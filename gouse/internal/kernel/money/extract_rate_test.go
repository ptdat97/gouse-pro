package money_test

import (
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// TestExtractRateTachDungPhanThue — giá đã gồm VAT thì thuế nằm TRONG.
func TestExtractRateTachDungPhanThue(t *testing.T) {
	for _, tt := range []struct {
		ten    string
		tong   int64
		suatBP int32
		mong   int64
	}{
		{"420.000 gồm 8%", 420_000, 800, 31_111},
		{"108.000 gồm 8%", 108_000, 800, 8_000},
		{"110.000 gồm 10%", 110_000, 1000, 10_000},
		{"thuế suất 0", 500_000, 0, 0},
		{"số 0", 0, 800, 0},
	} {
		t.Run(tt.ten, func(t *testing.T) {
			got := money.MustNew(tt.tong, money.VND).
				ExtractRate(types.MustNewBasisPoints(tt.suatBP), money.RoundHalfUp)
			if got.Amount() != tt.mong {
				t.Errorf("thuế = %d, mong %d", got.Amount(), tt.mong)
			}
		})
	}
}

// TestExtractRateDoiChieuNGUOC — phần chưa thuế cộng phần thuế phải ra
// ĐÚNG số tiền ban đầu.
//
// Đây là bất biến phân biệt công thức đúng với công thức sai: dùng nhầm
// `m × r / 10000` cho 33.600, và 420.000 − 33.600 = 386.400, mà 386.400
// cộng 8% ra 417.312 ≠ 420.000.
func TestExtractRateDoiChieuNguoc(t *testing.T) {
	suat := types.MustNewBasisPoints(800)

	for _, tong := range []int64{1, 999, 420_000, 1_234_567, 999_999_999} {
		m := money.MustNew(tong, money.VND)
		thue := m.ExtractRate(suat, money.RoundHalfUp)
		chuaThue, err := m.Sub(thue)
		if err != nil {
			t.Fatalf("Sub: %v", err)
		}

		// chưa thuế + 8% của chưa thuế ≈ tổng (lệch tối đa 1đ do làm tròn)
		congLai, err := chuaThue.Add(chuaThue.ApplyRate(suat, money.RoundHalfUp))
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		lech := congLai.Amount() - tong
		if lech < -1 || lech > 1 {
			t.Errorf("tổng %d: chưa thuế %d + thuế = %d, lệch %d — công "+
				"thức tách sai", tong, chuaThue.Amount(), congLai.Amount(), lech)
		}
	}
}

// TestExtractRateKhongTranSoVoiSoTienLON.
//
// Tích `m × r` tràn int64 khi số tiền lớn, và tràn ở đây làm số thuế SAI
// mà tổng vẫn khớp — kiểu hỏng không có gì báo. Cùng lớp lỗi với phép
// chia tiền đã sửa trước đây.
func TestExtractRateKhongTranSoVoiSoTienLon(t *testing.T) {
	// 9.000.000.000.000.000 × 800 = 7,2e18 — vẫn trong int64 nhưng sát
	// trần; nhân 128 bit thì không có vấn đề gì.
	const lon = 9_000_000_000_000_000
	thue := money.MustNew(lon, money.VND).
		ExtractRate(types.MustNewBasisPoints(800), money.RoundHalfUp)

	if thue.Amount() <= 0 || thue.Amount() >= lon {
		t.Fatalf("thuế = %d, phải nằm trong (0, %d)", thue.Amount(), lon)
	}
	// 8/108 của số đó ≈ 666.666.666.666.666,7
	if got := thue.Amount(); got != 666_666_666_666_667 {
		t.Errorf("thuế = %d, mong 666666666666667", got)
	}
}
