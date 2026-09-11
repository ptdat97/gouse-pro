package domain

import (
	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// BenChiuGiamGia là bên phải gánh chi phí của một khoản giảm giá.
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
type BenChiuGiamGia string

const (
	// BenChiuNenTang — nền tảng gánh. Mặc định khi không khai.
	BenChiuNenTang BenChiuGiamGia = "PLATFORM"

	// BenChiuNhaBan — nhà bán gánh, trừ vào tiền phải trả gian hàng đó.
	BenChiuNhaBan BenChiuGiamGia = "SELLER"

	// BenChiuChiaDoi — chia theo tỷ lệ đã thỏa thuận.
	BenChiuChiaDoi BenChiuGiamGia = "SHARED"
)

// HoacMacDinh trả PLATFORM khi giá trị rỗng.
//
// Rỗng nghĩa là chưa ai khai, và mặc định phải là bên KHÔNG bị trừ tiền
// oan: đoán nhầm sang nhà bán là lấy tiền của người ngoài công ty.
func (b BenChiuGiamGia) HoacMacDinh() BenChiuGiamGia {
	switch b {
	case BenChiuNhaBan, BenChiuChiaDoi:
		return b
	default:
		return BenChiuNenTang
	}
}

// PhanBoChiPhiGiam là MỘT phần của khoản giảm và bên phải gánh nó.
//
// Chương trình do nền tảng hoặc nhà bán chịu có đúng một phần; chương
// trình CHIA ĐÔI có hai. Tổng các phần luôn bằng ĐÚNG số tiền giảm — bất
// biến này do `promotion.AllocateCost` giữ, và checkout chỉ đóng băng lại.
type PhanBoChiPhiGiam struct {
	BenChiu BenChiuGiamGia

	// SellerID chỉ có nghĩa khi BenChiu là SELLER.
	SellerID ids.ID

	SoTien money.Money
}

// TongPhanBo cộng các phần lại.
//
// Dùng để kiểm bất biến ở chỗ nhận: một bảng phân bổ không cộng đúng số
// tiền giảm là một khoản KHÔNG AI CHỊU, và nó phải bị chặn tại chỗ chứ
// không đi tiếp vào sổ cái.
func TongPhanBo(ds []PhanBoChiPhiGiam, donVi money.Currency) (money.Money, error) {
	tong := money.Zero(donVi)
	for _, d := range ds {
		var err error
		if tong, err = tong.Add(d.SoTien); err != nil {
			return money.Money{}, err
		}
	}
	return tong, nil
}
