package domain

import (
	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// DiscountBearer là bên phải gánh chi phí của một khoản giảm giá.
//
// # Vì sao checkout cần khái niệm này
//
// Nó KHÔNG quyết định gì ở đây — quy tắc chia thuộc module promotion
// (`AllocateCost`, ba bên chịu, kèm bất biến "tổng luôn bằng đúng số tiền
// giảm"). Checkout chỉ NHẬN kết quả rồi đóng băng, để đơn hàng và sổ cái
// sau này trừ tiền đúng bên.
//
// Trước đây giá trị này bị gán cứng "PLATFORM" ở tầng adapter, nên một
// chương trình do nhà bán tự chạy vẫn được ghi là nền tảng gánh — sai theo
// hướng không ai khiếu nại, nên nó sống rất lâu.
type DiscountBearer string

const (
	// BearerPlatform — nền tảng gánh. Mặc định khi không khai.
	BearerPlatform DiscountBearer = "PLATFORM"

	// BearerSeller — nhà bán gánh, trừ vào tiền phải trả gian hàng đó.
	BearerSeller DiscountBearer = "SELLER"

	// BearerShared — chia theo tỷ lệ đã thỏa thuận.
	BearerShared DiscountBearer = "SHARED"
)

// OrDefault trả PLATFORM khi giá trị rỗng.
//
// Rỗng nghĩa là chưa ai khai, và mặc định phải là bên KHÔNG bị trừ tiền
// oan: đoán nhầm sang nhà bán là lấy tiền của người ngoài công ty.
func (b DiscountBearer) OrDefault() DiscountBearer {
	switch b {
	case BearerSeller, BearerShared:
		return b
	default:
		return BearerPlatform
	}
}

// DiscountCostAllocation là MỘT phần của khoản giảm và bên phải gánh nó.
//
// Chương trình do nền tảng hoặc nhà bán chịu có đúng một phần; chương
// trình CHIA ĐÔI có hai. Tổng các phần luôn bằng ĐÚNG số tiền giảm — bất
// biến này do `promotion.AllocateCost` giữ, và checkout chỉ đóng băng lại.
type DiscountCostAllocation struct {
	Bearer DiscountBearer

	// SellerID chỉ có nghĩa khi BenChiu là SELLER.
	SellerID ids.ID

	Amount money.Money
}

// TotalAllocated cộng các phần lại.
//
// Dùng để kiểm bất biến ở chỗ nhận: một bảng phân bổ không cộng đúng số
// tiền giảm là một khoản KHÔNG AI CHỊU, và nó phải bị chặn tại chỗ chứ
// không đi tiếp vào sổ cái.
func TotalAllocated(ds []DiscountCostAllocation, unit money.Currency) (money.Money, error) {
	tong := money.Zero(unit)
	for _, d := range ds {
		var err error
		if tong, err = tong.Add(d.Amount); err != nil {
			return money.Money{}, err
		}
	}
	return tong, nil
}
