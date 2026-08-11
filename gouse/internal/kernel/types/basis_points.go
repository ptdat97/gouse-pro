// Package types chứa các value object nguyên thủy dùng chung toàn hệ thống.
//
// Ràng buộc: package này chỉ phụ thuộc thư viện chuẩn. Nó nằm trong kernel,
// nghĩa là mọi module đều phụ thuộc vào nó — thay đổi ở đây ảnh hưởng toàn bộ.
package types

import (
	"errors"
	"fmt"
)

var (
	ErrRateOutOfRange = errors.New("types: tỷ lệ ngoài khoảng [0, 10000]")
	ErrNegativeQty    = errors.New("types: số lượng không được âm")
)

// BasisPoints biểu diễn phần trăm bằng phần vạn để tránh số thực.
//
//	1000  = 10,00%
//	10000 = 100%
//	150   = 1,50%
//
// Dùng cho: tỷ lệ hoa hồng, phí, giữ bảo đảm (reserve rate).
type BasisPoints struct {
	value int32
}

// NewBasisPoints tạo tỷ lệ, ràng buộc trong khoảng [0, 10000].
//
// Giới hạn trên 100% là ràng buộc nghiệp vụ: hoa hồng không thể vượt
// giá trị đơn hàng.
func NewBasisPoints(v int32) (BasisPoints, error) {
	if v < 0 || v > 10000 {
		return BasisPoints{}, fmt.Errorf("%w: %d", ErrRateOutOfRange, v)
	}
	return BasisPoints{value: v}, nil
}

// MustNewBasisPoints như NewBasisPoints nhưng panic khi lỗi.
// Chỉ dùng cho hằng số và test.
func MustNewBasisPoints(v int32) BasisPoints {
	bp, err := NewBasisPoints(v)
	if err != nil {
		panic(err)
	}
	return bp
}

func (b BasisPoints) Value() int32 { return b.value }
func (b BasisPoints) IsZero() bool { return b.value == 0 }

// Percent trả về giá trị phần trăm dạng chuỗi để hiển thị và ghi log.
func (b BasisPoints) String() string {
	return fmt.Sprintf("%d.%02d%%", b.value/100, b.value%100)
}

// Quantity là số lượng không âm.
//
// Kiểu riêng ngăn việc vô tình gán số lượng âm — lỗi này trong module
// inventory dẫn tới tồn kho âm, một trong ba chỉ số phải luôn bằng 0.
type Quantity struct {
	value int32
}

func NewQuantity(v int32) (Quantity, error) {
	if v < 0 {
		return Quantity{}, fmt.Errorf("%w: %d", ErrNegativeQty, v)
	}
	return Quantity{value: v}, nil
}

func MustNewQuantity(v int32) Quantity {
	q, err := NewQuantity(v)
	if err != nil {
		panic(err)
	}
	return q
}

func (q Quantity) Value() int32   { return q.value }
func (q Quantity) IsZero() bool   { return q.value == 0 }
func (q Quantity) String() string { return fmt.Sprintf("%d", q.value) }

// Add cộng số lượng.
func (q Quantity) Add(other Quantity) Quantity {
	return Quantity{value: q.value + other.value}
}

// Sub trừ số lượng, lỗi nếu kết quả âm.
func (q Quantity) Sub(other Quantity) (Quantity, error) {
	return NewQuantity(q.value - other.value)
}

// GreaterThanOrEqual dùng để kiểm tra đủ tồn kho trước khi giữ hàng.
func (q Quantity) GreaterThanOrEqual(other Quantity) bool {
	return q.value >= other.value
}
