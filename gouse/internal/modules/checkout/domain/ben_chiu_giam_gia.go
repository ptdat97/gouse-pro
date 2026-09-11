package domain

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
