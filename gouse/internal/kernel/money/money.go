// Package money cung cấp kiểu Money — value object quan trọng nhất hệ thống.
//
// QUY TẮC TUYỆT ĐỐI: không bao giờ dùng float cho tiền.
//
//	0.1 + 0.2 = 0.30000000000000004
//
// Với hàng triệu giao dịch, sai số tích lũy thành tiền thật. Và nền tảng giữ
// tiền hộ seller/creator — độ lệch đối soát phải bằng 0.
//
// Xem docs/02-domain/value-objects.md mục 2.
package money

import (
	"errors"
	"fmt"
)

// Currency là mã tiền tệ ISO 4217.
type Currency string

const (
	VND Currency = "VND"
	USD Currency = "USD"
)

// exponent trả về số chữ số thập phân của đơn vị nhỏ nhất.
//
//	VND: 0 (đồng là đơn vị nhỏ nhất)
//	USD: 2 (cent)
func (c Currency) exponent() int {
	switch c {
	case VND:
		return 0
	case USD:
		return 2
	default:
		return 2
	}
}

func (c Currency) valid() bool {
	return len(c) == 3
}

var (
	ErrCurrencyMismatch = errors.New("money: khác đơn vị tiền tệ")
	ErrInvalidCurrency  = errors.New("money: đơn vị tiền tệ không hợp lệ")
	ErrNegativeRatio    = errors.New("money: tỷ lệ phân bổ không được âm")

	// ErrTranSo khi phép tính vượt giới hạn int64.
	//
	// Trả LỖI chứ không để tràn im lặng: tràn số trong phép chia tiền
	// không báo gì cả — nó cho ra con số sai mà TỔNG vẫn đúng, nên phép
	// kiểm hiển nhiên nhất vẫn xanh.
	ErrTranSo       = errors.New("money: phép tính vượt giới hạn số nguyên")
	ErrEmptyRatios  = errors.New("money: danh sách tỷ lệ rỗng")
	ErrZeroRatioSum = errors.New("money: tổng tỷ lệ bằng 0")
)

// Money là số tiền bất biến, lưu bằng số nguyên theo đơn vị nhỏ nhất.
//
// Giá trị zero (Money{}) là 0 với currency rỗng — chỉ dùng làm giá trị trung
// gian; mọi phép toán với currency rỗng đều trả lỗi.
type Money struct {
	amount   int64
	currency Currency
}

// New tạo Money từ số nguyên theo đơn vị nhỏ nhất.
//
//	New(299000, VND) = 299.000đ
//	New(29900, USD)  = $299.00
func New(amount int64, c Currency) (Money, error) {
	if !c.valid() {
		return Money{}, fmt.Errorf("%w: %q", ErrInvalidCurrency, c)
	}
	return Money{amount: amount, currency: c}, nil
}

// MustNew như New nhưng panic khi lỗi. Chỉ dùng cho hằng số và test.
func MustNew(amount int64, c Currency) Money {
	m, err := New(amount, c)
	if err != nil {
		panic(err)
	}
	return m
}

// Zero trả về 0 của một đơn vị tiền tệ.
func Zero(c Currency) Money { return Money{amount: 0, currency: c} }

func (m Money) Amount() int64      { return m.amount }
func (m Money) Currency() Currency { return m.currency }
func (m Money) IsZero() bool       { return m.amount == 0 }
func (m Money) IsNegative() bool   { return m.amount < 0 }
func (m Money) IsPositive() bool   { return m.amount > 0 }

// sameCurrency kiểm tra hai Money cùng đơn vị tiền tệ.
//
// Money zero (currency rỗng) được coi là tương thích với mọi currency để
// tổng hợp bằng vòng lặp từ giá trị zero hoạt động tự nhiên.
func (m Money) sameCurrency(other Money) error {
	if m.currency == "" || other.currency == "" {
		return nil
	}
	if m.currency != other.currency {
		return fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, other.currency)
	}
	return nil
}

// resolveCurrency trả về currency hiệu lực khi cộng/trừ hai giá trị.
func (m Money) resolveCurrency(other Money) Currency {
	if m.currency != "" {
		return m.currency
	}
	return other.currency
}

// Add cộng hai số tiền. Lỗi nếu khác đơn vị tiền tệ.
func (m Money) Add(other Money) (Money, error) {
	if err := m.sameCurrency(other); err != nil {
		return Money{}, err
	}
	return Money{amount: m.amount + other.amount, currency: m.resolveCurrency(other)}, nil
}

// Sub trừ hai số tiền. Kết quả có thể âm — hợp lệ với bút toán điều chỉnh
// và số dư seller âm khi hoàn hàng vượt doanh thu kỳ.
func (m Money) Sub(other Money) (Money, error) {
	if err := m.sameCurrency(other); err != nil {
		return Money{}, err
	}
	return Money{amount: m.amount - other.amount, currency: m.resolveCurrency(other)}, nil
}

// Neg trả về số đối — dùng cho bút toán đảo ngược.
func (m Money) Neg() Money {
	return Money{amount: -m.amount, currency: m.currency}
}

// MulQuantity nhân với số lượng nguyên. Dùng để tính line_total.
func (m Money) MulQuantity(qty int64) Money {
	return Money{amount: m.amount * qty, currency: m.currency}
}

// Equal so sánh bằng giá trị — Money là value object.
func (m Money) Equal(other Money) bool {
	return m.amount == other.amount && m.currency == other.currency
}

// Compare trả về -1, 0, 1. Lỗi nếu khác đơn vị tiền tệ.
func (m Money) Compare(other Money) (int, error) {
	if err := m.sameCurrency(other); err != nil {
		return 0, err
	}
	switch {
	case m.amount < other.amount:
		return -1, nil
	case m.amount > other.amount:
		return 1, nil
	default:
		return 0, nil
	}
}

// LessThan trả về true nếu m < other. Khác currency được coi là false.
func (m Money) LessThan(other Money) bool {
	c, err := m.Compare(other)
	return err == nil && c < 0
}

// String hiển thị dạng đọc được, chủ yếu cho log và thông báo lỗi.
// Định dạng hiển thị cho người dùng cuối là việc của tầng frontend.
func (m Money) String() string {
	if m.currency == "" {
		return fmt.Sprintf("%d", m.amount)
	}
	exp := m.currency.exponent()
	if exp == 0 {
		return fmt.Sprintf("%d %s", m.amount, m.currency)
	}
	div := int64(1)
	for i := 0; i < exp; i++ {
		div *= 10
	}
	neg := ""
	a := m.amount
	if a < 0 {
		neg = "-"
		a = -a
	}
	return fmt.Sprintf("%s%d.%0*d %s", neg, a/div, exp, a%div, m.currency)
}

// Sum cộng nhiều số tiền. Trả Zero của currency đầu tiên nếu danh sách rỗng.
func Sum(items ...Money) (Money, error) {
	var total Money
	for _, it := range items {
		var err error
		total, err = total.Add(it)
		if err != nil {
			return Money{}, err
		}
	}
	return total, nil
}
