package money_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

func TestAddRejectsCurrencyMismatch(t *testing.T) {
	vnd := money.MustNew(100, money.VND)
	usd := money.MustNew(100, money.USD)

	if _, err := vnd.Add(usd); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("cộng khác tiền tệ phải lỗi, nhận: %v", err)
	}
}

func TestSubAllowsNegativeResult(t *testing.T) {
	// Số dư seller âm là trạng thái HỢP LỆ: hoàn hàng vượt doanh thu kỳ.
	// Khoản âm chuyển sang kỳ sau, không chuyển tiền âm.
	a := money.MustNew(1_000_000, money.VND)
	b := money.MustNew(2_000_000, money.VND)

	got, err := a.Sub(b)
	if err != nil {
		t.Fatalf("trừ hợp lệ không được lỗi: %v", err)
	}
	if got.Amount() != -1_000_000 {
		t.Fatalf("mong -1000000, nhận %d", got.Amount())
	}
	if !got.IsNegative() {
		t.Fatal("IsNegative phải true")
	}
}

func TestSumFromZeroValue(t *testing.T) {
	// Tổng hợp bằng vòng lặp từ Money{} phải hoạt động — đây là cách
	// tính subtotal của giỏ hàng và tổng bút toán.
	got, err := money.Sum(
		money.MustNew(299_000, money.VND),
		money.MustNew(650_000, money.VND),
		money.MustNew(301_000, money.VND),
	)
	if err != nil {
		t.Fatalf("Sum lỗi: %v", err)
	}
	if got.Amount() != 1_250_000 {
		t.Fatalf("mong 1250000, nhận %d", got.Amount())
	}
	if got.Currency() != money.VND {
		t.Fatalf("mong VND, nhận %s", got.Currency())
	}
}

func TestSumRejectsMixedCurrency(t *testing.T) {
	_, err := money.Sum(
		money.MustNew(100, money.VND),
		money.MustNew(100, money.USD),
	)
	if !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("mong ErrCurrencyMismatch, nhận %v", err)
	}
}

// TestAllocateNeverLosesMoney là test quan trọng nhất của package.
//
// Bất biến: tổng các phần được chia LUÔN bằng số tiền gốc.
// Vi phạm bất biến này làm sổ cái không cân (Σ DEBIT ≠ Σ CREDIT).
func TestAllocateNeverLosesMoney(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		ratios []int64
	}{
		{"chia 3 phần bằng nhau, có dư", 100_000, []int64{1, 1, 1}},
		{"chia 3 phần, dư 2", 100, []int64{1, 1, 1}},
		{"chia theo tỷ lệ lẻ", 1_250_000, []int64{299, 650, 301}},
		{"chia không dư", 90, []int64{1, 1, 1}},
		{"một phần nhận hết", 500, []int64{1}},
		{"tỷ lệ có số 0", 1000, []int64{0, 1, 1}},
		{"số âm (bút toán đảo ngược)", -100_000, []int64{1, 1, 1}},
		{"số tiền 0", 0, []int64{1, 2, 3}},
		{"tỷ lệ chênh lệch lớn", 1_000_001, []int64{1, 999999}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := money.MustNew(tc.amount, money.VND)
			parts, err := m.Allocate(tc.ratios)
			if err != nil {
				t.Fatalf("Allocate lỗi: %v", err)
			}
			if len(parts) != len(tc.ratios) {
				t.Fatalf("mong %d phần, nhận %d", len(tc.ratios), len(parts))
			}

			var sum int64
			for _, p := range parts {
				sum += p.Amount()
				if p.Currency() != money.VND {
					t.Fatalf("phần bị mất currency: %v", p)
				}
			}
			if sum != tc.amount {
				t.Fatalf("MẤT TIỀN: tổng phần = %d, gốc = %d (chênh %d)",
					sum, tc.amount, tc.amount-sum)
			}
		})
	}
}

func TestAllocateSpecificDistribution(t *testing.T) {
	// Phần dư đi vào các phần đầu tiên — quy tắc xác định.
	m := money.MustNew(100_000, money.VND)
	parts, err := m.Allocate([]int64{1, 1, 1})
	if err != nil {
		t.Fatalf("Allocate lỗi: %v", err)
	}
	want := []int64{33_334, 33_333, 33_333}
	for i, w := range want {
		if parts[i].Amount() != w {
			t.Errorf("phần %d: mong %d, nhận %d", i, w, parts[i].Amount())
		}
	}
}

func TestAllocateRejectsInvalidInput(t *testing.T) {
	m := money.MustNew(1000, money.VND)

	if _, err := m.Allocate(nil); !errors.Is(err, money.ErrEmptyRatios) {
		t.Errorf("ratios rỗng phải lỗi, nhận %v", err)
	}
	if _, err := m.Allocate([]int64{1, -1}); !errors.Is(err, money.ErrNegativeRatio) {
		t.Errorf("ratio âm phải lỗi, nhận %v", err)
	}
	if _, err := m.Allocate([]int64{0, 0}); !errors.Is(err, money.ErrZeroRatioSum) {
		t.Errorf("tổng ratio = 0 phải lỗi, nhận %v", err)
	}
}

func TestAllocateEqual(t *testing.T) {
	m := money.MustNew(1_000_000, money.VND)
	parts, err := m.AllocateEqual(7)
	if err != nil {
		t.Fatalf("AllocateEqual lỗi: %v", err)
	}
	var sum int64
	for _, p := range parts {
		sum += p.Amount()
	}
	if sum != 1_000_000 {
		t.Fatalf("MẤT TIỀN: %d != 1000000", sum)
	}
}

// TestApplyRateCommissionScenario kiểm chứng ví dụ hoa hồng trong tài liệu.
// Xem docs/01-business/monetization.md mục 4.
func TestApplyRateCommissionScenario(t *testing.T) {
	orderTotal := money.MustNew(300_000, money.VND)

	platformCommission := orderTotal.ApplyRate(types.MustNewBasisPoints(1000), money.RoundDown)
	if platformCommission.Amount() != 30_000 {
		t.Errorf("hoa hồng nền tảng 10%%: mong 30000, nhận %d", platformCommission.Amount())
	}

	creatorCommission := orderTotal.ApplyRate(types.MustNewBasisPoints(500), money.RoundDown)
	if creatorCommission.Amount() != 15_000 {
		t.Errorf("hoa hồng creator 5%%: mong 15000, nhận %d", creatorCommission.Amount())
	}

	paymentFee := orderTotal.ApplyRate(types.MustNewBasisPoints(150), money.RoundDown)
	if paymentFee.Amount() != 4_500 {
		t.Errorf("phí PSP 1.5%%: mong 4500, nhận %d", paymentFee.Amount())
	}

	// Số dư seller = tổng − các khoản trừ
	deductions, err := money.Sum(platformCommission, creatorCommission, paymentFee)
	if err != nil {
		t.Fatalf("Sum lỗi: %v", err)
	}
	sellerPayable, err := orderTotal.Sub(deductions)
	if err != nil {
		t.Fatalf("Sub lỗi: %v", err)
	}
	if sellerPayable.Amount() != 250_500 {
		t.Errorf("số dư seller: mong 250500, nhận %d", sellerPayable.Amount())
	}

	// BẤT BIẾN LEDGER: Σ CREDIT phải = Σ DEBIT
	totalCredit, err := money.Sum(sellerPayable, platformCommission, creatorCommission, paymentFee)
	if err != nil {
		t.Fatalf("Sum lỗi: %v", err)
	}
	if !totalCredit.Equal(orderTotal) {
		t.Fatalf("BÚT TOÁN KHÔNG CÂN: credit %s != debit %s", totalCredit, orderTotal)
	}
}

func TestApplyRateRounding(t *testing.T) {
	// 333 * 10% = 33.3 → RoundDown = 33, RoundHalfUp = 33
	// 335 * 10% = 33.5 → RoundDown = 33, RoundHalfUp = 34
	cases := []struct {
		amount     int64
		bp         int32
		wantDown   int64
		wantHalfUp int64
	}{
		{333, 1000, 33, 33},
		{335, 1000, 33, 34},
		{300_000, 1000, 30_000, 30_000},
		{100, 1, 0, 0}, // 0.01% của 100 = 0.01
	}
	for _, tc := range cases {
		m := money.MustNew(tc.amount, money.VND)
		rate := types.MustNewBasisPoints(tc.bp)

		if got := m.ApplyRate(rate, money.RoundDown); got.Amount() != tc.wantDown {
			t.Errorf("RoundDown(%d, %d bp): mong %d, nhận %d",
				tc.amount, tc.bp, tc.wantDown, got.Amount())
		}
		if got := m.ApplyRate(rate, money.RoundHalfUp); got.Amount() != tc.wantHalfUp {
			t.Errorf("RoundHalfUp(%d, %d bp): mong %d, nhận %d",
				tc.amount, tc.bp, tc.wantHalfUp, got.Amount())
		}
	}
}

func TestApplyRateNegativeAmount(t *testing.T) {
	// Bút toán đảo ngược khi hoàn hàng.
	m := money.MustNew(-300_000, money.VND)
	got := m.ApplyRate(types.MustNewBasisPoints(1000), money.RoundDown)
	if got.Amount() != -30_000 {
		t.Fatalf("mong -30000, nhận %d", got.Amount())
	}
}

func TestNegForReversalEntry(t *testing.T) {
	commission := money.MustNew(30_000, money.VND)
	reversal := commission.Neg()

	if reversal.Amount() != -30_000 {
		t.Fatalf("mong -30000, nhận %d", reversal.Amount())
	}
	// Cộng lại phải về 0 — bút toán gốc + bút toán đảo = 0.
	sum, err := commission.Add(reversal)
	if err != nil {
		t.Fatalf("Add lỗi: %v", err)
	}
	if !sum.IsZero() {
		t.Fatalf("bút toán đảo phải triệt tiêu, nhận %s", sum)
	}
}

func TestMulQuantity(t *testing.T) {
	unitPrice := money.MustNew(299_000, money.VND)
	lineTotal := unitPrice.MulQuantity(2)
	if lineTotal.Amount() != 598_000 {
		t.Fatalf("mong 598000, nhận %d", lineTotal.Amount())
	}
}

func TestNewRejectsInvalidCurrency(t *testing.T) {
	if _, err := money.New(100, money.Currency("VNDD")); !errors.Is(err, money.ErrInvalidCurrency) {
		t.Errorf("currency 4 ký tự phải lỗi, nhận %v", err)
	}
	if _, err := money.New(100, money.Currency("")); !errors.Is(err, money.ErrInvalidCurrency) {
		t.Errorf("currency rỗng phải lỗi, nhận %v", err)
	}
}

func TestString(t *testing.T) {
	cases := []struct {
		m    money.Money
		want string
	}{
		{money.MustNew(299_000, money.VND), "299000 VND"},
		{money.MustNew(29_900, money.USD), "299.00 USD"},
		{money.MustNew(-4_500, money.USD), "-45.00 USD"},
		{money.MustNew(5, money.USD), "0.05 USD"},
	}
	for _, tc := range cases {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("String(): mong %q, nhận %q", tc.want, got)
		}
	}
}

func TestCompare(t *testing.T) {
	a := money.MustNew(100, money.VND)
	b := money.MustNew(200, money.VND)

	if c, _ := a.Compare(b); c != -1 {
		t.Errorf("100 vs 200: mong -1, nhận %d", c)
	}
	if c, _ := b.Compare(a); c != 1 {
		t.Errorf("200 vs 100: mong 1, nhận %d", c)
	}
	if c, _ := a.Compare(a); c != 0 {
		t.Errorf("100 vs 100: mong 0, nhận %d", c)
	}
	if _, err := a.Compare(money.MustNew(100, money.USD)); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Error("so sánh khác currency phải lỗi")
	}
}
