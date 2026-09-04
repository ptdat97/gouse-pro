package money

import (
	"fmt"
	"math"
	"math/bits"

	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// Allocate chia số tiền theo tỷ lệ mà KHÔNG MẤT ĐỒNG NÀO.
//
// Vì sao cần hàm riêng thay vì chia rồi làm tròn:
//
//	Chia 100.000đ cho 3 phần bằng nhau:
//	  Cách sai: 100000/3 = 33333.33 → làm tròn 33333 mỗi phần
//	            Tổng = 99999đ → MẤT 1đ
//	  Allocate: 33334 + 33333 + 33333 = 100000đ ✓
//
// Với hàng triệu giao dịch chia tiền cho seller và creator, mất từng đồng
// sẽ làm sổ cái không cân và vi phạm bất biến Σ DEBIT = Σ CREDIT.
//
// Phần dư được phân bổ cho các phần đầu tiên theo thứ tự — quy tắc xác định,
// cho kết quả giống nhau mỗi lần chạy.
func (m Money) Allocate(ratios []int64) ([]Money, error) {
	if len(ratios) == 0 {
		return nil, ErrEmptyRatios
	}

	var totalRatio int64
	for _, r := range ratios {
		if r < 0 {
			return nil, fmt.Errorf("%w: %d", ErrNegativeRatio, r)
		}
		// Tổng tỷ lệ cũng tràn được: mười dòng, mỗi dòng 2 tỷ là đủ.
		//
		// Kiểm TRƯỚC khi cộng, vì sau khi tràn thì `totalRatio` âm và mọi
		// phép sau đó vô nghĩa.
		if totalRatio > math.MaxInt64-r {
			return nil, fmt.Errorf("%w: tổng tỷ lệ vượt giới hạn", ErrTranSo)
		}
		totalRatio += r
	}
	if totalRatio == 0 {
		return nil, ErrZeroRatioSum
	}

	// Số âm: phân bổ trên giá trị tuyệt đối rồi đảo dấu, để phần dư
	// được phân bổ nhất quán bất kể dấu.
	negative := m.amount < 0
	amount := m.amount
	if negative {
		amount = -amount
	}

	results := make([]Money, len(ratios))
	var allocated int64
	for i, r := range ratios {
		// Nhân 128 BIT rồi mới chia.
		//
		// `amount * r` bằng int64 tràn ở mức hoàn toàn có thật: chia 20 tỷ
		// theo tỷ lệ 19 tỷ : 1 tỷ cho tích 3,8e20, vượt trần int64
		// (9,22e18). Kết quả khi ấy KHÔNG báo lỗi gì — nó trả ra
		// 9,78 tỷ / 10,22 tỷ thay vì 19 tỷ / 1 tỷ.
		//
		// Tổng vẫn bằng số gốc, nên phép kiểm hiển nhiên nhất — "tổng các
		// phần bằng tổng ban đầu" — VẪN XANH. Đó là kiểu hỏng tệ nhất với
		// tiền: sai âm thầm, và chỉ lộ ra khi ai đó đối chiếu từng dòng.
		//
		// Ở một tỷ lệ khác, tràn làm `share` ÂM, `allocated` âm khổng lồ,
		// và vòng rải phần dư bên dưới chạy tới ~9e18 lần — tiến trình
		// treo cứng.
		//
		// `bits.Mul64` cho tích 128 bit, `bits.Div64` chia nó về 64 bit.
		// Thương chắc chắn vừa vì `r <= totalRatio`, nên `share <= amount`.
		hi, lo := bits.Mul64(uint64(amount), uint64(r))
		share := int64(chia128(hi, lo, uint64(totalRatio)))
		results[i] = Money{amount: share, currency: m.currency}
		allocated += share
	}

	// Phân bổ phần dư: mỗi phần nhận thêm 1 đơn vị cho tới khi hết dư.
	//
	// Phần dư của phép chia sàn luôn NHỎ HƠN số phần, nên vòng này chạy
	// tối đa n-1 lần. Nó chỉ chạy vô tận khi `allocated` sai — tức khi
	// phép nhân ở trên đã tràn.
	remainder := amount - allocated
	for i := int64(0); i < remainder; i++ {
		idx := int(i) % len(results)
		results[idx].amount++
	}

	if negative {
		for i := range results {
			results[i].amount = -results[i].amount
		}
	}

	return results, nil
}

// AllocateEqual chia đều thành n phần, không mất đồng nào.
func (m Money) AllocateEqual(n int) ([]Money, error) {
	if n <= 0 {
		return nil, ErrEmptyRatios
	}
	ratios := make([]int64, n)
	for i := range ratios {
		ratios[i] = 1
	}
	return m.Allocate(ratios)
}

// Rounding xác định cách làm tròn khi áp dụng tỷ lệ phần trăm.
type Rounding int

const (
	// RoundDown làm tròn xuống — mặc định cho hoa hồng và phí,
	// có lợi cho bên trả tiền.
	RoundDown Rounding = iota
	// RoundHalfUp làm tròn nửa lên — dùng cho thuế theo quy định.
	RoundHalfUp
)

// ApplyRate áp dụng tỷ lệ phần trăm (basis points) lên số tiền.
//
// Dùng cho: hoa hồng nền tảng, hoa hồng creator, phí thanh toán, thuế.
//
//	MustNew(300000, VND).ApplyRate(types.MustNewBasisPoints(1000), RoundDown)
//	  → 30.000đ (10% của 300.000đ)
//
// Quy tắc làm tròn phải nhất quán toàn hệ thống — nếu mỗi nơi tự quyết định,
// đối soát sẽ ra kết quả khác nhau.
func (m Money) ApplyRate(rate types.BasisPoints, mode Rounding) Money {
	const scale = 10000 // basis points: 10000 = 100%

	negative := m.amount < 0
	amount := m.amount
	if negative {
		amount = -amount
	}

	product := amount * int64(rate.Value())
	result := product / scale

	if mode == RoundHalfUp {
		if rem := product % scale; rem*2 >= scale {
			result++
		}
	}

	if negative {
		result = -result
	}
	return Money{amount: result, currency: m.currency}
}

// chia128 chia số 128 bit (hi:lo) cho y, trả thương 64 bit.
//
// `bits.Div64` PANIC khi thương không vừa 64 bit. Ở đây điều đó không xảy
// ra vì `r <= totalRatio` nên thương `<= amount`, nhưng kiểm vẫn rẻ hơn
// một lần panic trên đường tính tiền.
func chia128(hi, lo, y uint64) uint64 {
	if y == 0 || hi >= y {
		// Không thể tới đây với dữ liệu hợp lệ. Trả 0 thay vì panic: một
		// phần bằng 0 làm phần dư gánh lại và tổng vẫn đúng, còn panic
		// thì giết cả request.
		return 0
	}
	q, _ := bits.Div64(hi, lo, y)
	return q
}
