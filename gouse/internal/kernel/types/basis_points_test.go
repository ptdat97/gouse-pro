package types_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/types"
)

func TestBasisPointsRange(t *testing.T) {
	// Giới hạn [0, 10000] là ràng buộc NGHIỆP VỤ: hoa hồng không thể
	// vượt giá trị đơn hàng.
	cases := []struct {
		v       int32
		wantErr bool
	}{
		{0, false},
		{1000, false},  // 10%
		{10000, false}, // 100%
		{10001, true},
		{-1, true},
	}
	for _, tc := range cases {
		_, err := types.NewBasisPoints(tc.v)
		if tc.wantErr && !errors.Is(err, types.ErrRateOutOfRange) {
			t.Errorf("NewBasisPoints(%d): mong lỗi, nhận %v", tc.v, err)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("NewBasisPoints(%d): không mong lỗi, nhận %v", tc.v, err)
		}
	}
}

func TestBasisPointsString(t *testing.T) {
	cases := []struct {
		v    int32
		want string
	}{
		{1000, "10.00%"},
		{150, "1.50%"},
		{10000, "100.00%"},
		{0, "0.00%"},
		{5, "0.05%"},
	}
	for _, tc := range cases {
		got := types.MustNewBasisPoints(tc.v).String()
		if got != tc.want {
			t.Errorf("BasisPoints(%d).String(): mong %q, nhận %q", tc.v, tc.want, got)
		}
	}
}

func TestQuantityRejectsNegative(t *testing.T) {
	// Tồn kho âm là một trong ba chỉ số phải LUÔN bằng 0.
	// Kiểu Quantity là lớp bảo vệ đầu tiên.
	if _, err := types.NewQuantity(-1); !errors.Is(err, types.ErrNegativeQty) {
		t.Fatalf("số lượng âm phải lỗi, nhận %v", err)
	}
}

func TestQuantitySubGuardsAgainstNegative(t *testing.T) {
	available := types.MustNewQuantity(5)
	requested := types.MustNewQuantity(10)

	// Trừ nhiều hơn số có → lỗi, không tạo ra số âm im lặng.
	if _, err := available.Sub(requested); !errors.Is(err, types.ErrNegativeQty) {
		t.Fatalf("trừ vượt phải lỗi, nhận %v", err)
	}

	// Trừ hợp lệ
	got, err := available.Sub(types.MustNewQuantity(3))
	if err != nil {
		t.Fatalf("trừ hợp lệ không được lỗi: %v", err)
	}
	if got.Value() != 2 {
		t.Fatalf("mong 2, nhận %d", got.Value())
	}
}

func TestQuantityGreaterThanOrEqual(t *testing.T) {
	// Dùng để kiểm tra đủ tồn kho trước khi giữ hàng.
	available := types.MustNewQuantity(12)

	if !available.GreaterThanOrEqual(types.MustNewQuantity(2)) {
		t.Error("12 >= 2 phải true")
	}
	if !available.GreaterThanOrEqual(types.MustNewQuantity(12)) {
		t.Error("12 >= 12 phải true")
	}
	if available.GreaterThanOrEqual(types.MustNewQuantity(13)) {
		t.Error("12 >= 13 phải false")
	}
}
