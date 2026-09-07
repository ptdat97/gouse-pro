package domain

import "errors"

// ErrChoThanhToan khi nhà bán thao tác trên đơn chưa thu được tiền.
//
// Đây KHÔNG phải lỗi của nhà bán: họ làm đúng việc, chỉ là chưa tới lượt.
// Thông báo phải nói rõ điều đó, nếu không họ sẽ tưởng gian hàng bị lỗi.
var ErrChoThanhToan = errors.New(
	"fulfillment: đơn chưa thu được tiền — chưa được phép xử lý")

// PhuongThucTraTruoc cho biết phương thức này THU TIỀN TRƯỚC khi giao.
//
// # Vì sao danh sách nằm ở đây, không ở module payment
//
// Module này không hỏi payment được (ADR-0007: chiều đó đi bằng event), và
// điều nó cần biết không phải "cổng thanh toán nào" mà là một câu nghiệp
// vụ đơn giản: hàng có được rời kho trước khi tiền về không.
//
// Chuỗi RỖNG là đơn đi đường `placeOrder` — không qua phiên thanh toán,
// nên không có phương thức. Coi như KHÔNG trả trước: khóa một đơn mà không
// có đường nào mở khóa là làm hàng kẹt vĩnh viễn, tệ hơn hẳn rủi ro nó
// nhắm tới. Xem ADR-0018.
func PhuongThucTraTruoc(phuongThuc string) bool {
	switch phuongThuc {
	case "CARD", "BANK_TRANSFER", "E_WALLET":
		return true
	default:
		return false
	}
}

// ChoThanhToan cho biết đơn thực hiện có đang bị khóa chờ tiền không.
func (f *FulfillmentOrder) ChoThanhToan() bool { return f.choThanhToan }

// MoKhoaThanhToan mở khóa sau khi tiền đã về.
//
// IDEMPOTENT: gọi lại trên đơn đã mở khóa không phải lỗi. Event `order.paid`
// được phát lại là chuyện bình thường của giao hàng ít-nhất-một-lần, và
// biến một lần lặp thành lỗi sẽ làm event kẹt trong hàng đợi.
//
// KHÔNG có đường ngược lại: đã thu tiền thì không "chưa thu" lại được. Hoàn
// tiền là một bản ghi khác, cùng nguyên tắc bất biến với sổ cái.
func (f *FulfillmentOrder) MoKhoaThanhToan() {
	f.choThanhToan = false
}
