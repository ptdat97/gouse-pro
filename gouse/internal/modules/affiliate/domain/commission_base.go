package domain

import "github.com/fashion-commerce/platform/internal/kernel/money"

// CommissionBase là số tiền mà tỷ lệ hoa hồng áp lên.
//
//	tiền hàng + các khoản KHUYẾN MÃI đã phân bổ cho dòng
//
// Khoản khuyến mãi là số ÂM, nên phép cộng ở đây là phép trừ.
//
// # Vì sao trừ khuyến mãi, và vì sao CHỈ khuyến mãi
//
// Hoa hồng tính trên giá niêm yết của một đơn có giảm giá là trả creator
// nhiều hơn phần doanh thu họ thực sự tạo ra: khách trả 180.000 mà nền tảng
// trả hoa hồng theo 200.000. Với mã giảm sâu, phần chênh ấy ăn hết lãi.
//
// Thuế và phí vận chuyển thì KHÔNG trừ và cũng không cộng — chúng không nằm
// trong phép tính này vì chúng không phải doanh thu hàng hóa. Thuế là tiền
// của nhà nước, phí vận chuyển là tiền của hãng vận chuyển; creator không
// tạo ra khoản nào trong hai khoản đó. Bên gọi lọc theo loại TRƯỚC khi gọi
// hàm này, nên hàm này chỉ nhận đúng các khoản khuyến mãi.
//
// # Vì sao KẸP về 0
//
// Một dòng được giảm 100% (hàng tặng, đền bù) cho tiền gốc âm nếu phân bổ
// lệch một đồng. Hoa hồng ÂM nghĩa là đòi tiền lại của creator qua một
// đường không ai thiết kế — kẹp về 0 là không trả gì, và đó là câu trả lời
// đúng cho một dòng khách không trả tiền.
func CommissionBase(lineAmount money.Money, discounts []money.Money) money.Money {
	base := lineAmount
	for _, km := range discounts {
		sum, err := base.Add(km)
		if err != nil {
			// Khác đơn vị tiền: BỎ QUA khoản ấy thay vì tính sai.
			//
			// Hệ thống một đơn vị tiền (VND) nên đây là nhánh không đến
			// được; nếu đến, thà tính hoa hồng trên tiền hàng chưa giảm —
			// một khoản hơi cao — còn hơn cộng một con số vô nghĩa.
			continue
		}
		base = sum
	}

	if base.IsNegative() {
		return money.Zero(lineAmount.Currency())
	}
	return base
}
