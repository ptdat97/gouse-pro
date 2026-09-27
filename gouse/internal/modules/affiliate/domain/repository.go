package domain

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// Repo là kho lưu trữ của module affiliate.
//
// # Vì sao LƯU CẢ HAI trong một lời gọi
//
// Một bản quy công không kèm khoản hoa hồng là một lời hứa không có số
// tiền; một khoản hoa hồng không kèm bản quy công là một số tiền không
// giải thích được. `Luu` nhận cả hai và cài đặt phải ghi chúng trong CÙNG
// một giao dịch — không bày ra hai hàm để bên gọi tự nhớ phải gọi đủ.
type Repo interface {
	// Luu ghi bản quy công và khoản hoa hồng của nó.
	//
	// Trả ErrDaQuyCong khi dòng đơn đã có bản quy công. Đó KHÔNG phải lỗi
	// hệ thống: `order.paid` giao ít nhất một lần. Phát hiện ở tầng
	// DATABASE (UNIQUE) chứ không phải bằng một lượt đọc trước, vì hai
	// lượt worker chồng nhau đều thấy "chưa quy công".
	Luu(ctx context.Context, a *Attribution, c *CommissionRecord) error

	// TheoDonVaNhaBan trả các khoản hoa hồng của một đơn.
	//
	// `sellerID` rỗng nghĩa là MỌI nhà bán — dùng khi đảo cả đơn. Khác
	// rỗng thì chỉ dòng của nhà bán ấy, đúng hình dạng khóa mà
	// `fulfillment_order.completed` mang tới.
	TheoDonVaNhaBan(
		ctx context.Context, orderID, sellerID ids.ID,
	) ([]*CommissionRecord, error)

	// KHÔNG có `TheoCreator` (lịch sử hoa hồng của một creator).
	//
	// Nó cần một màn hình cho creator, mà module `creator` là Phase 2.
	// Khai một hàm chưa ai gọi là đúng dạng lỗi `cmd/eventcheck` đang gác ở
	// phía event, và dự án này đã gặp nó bốn lần ở hằng số event. Thêm khi
	// có bên gọi thật.

	// CapNhat ghi lại trạng thái đã đổi của một khoản hoa hồng.
	CapNhat(ctx context.Context, c *CommissionRecord) error

	// TongTheoCreator là tổng tiền theo từng trạng thái của một creator.
	//
	// Tính bằng SQL chứ không tải hết bản ghi rồi cộng ở Go: một creator
	// bán tốt sẽ có hàng nghìn dòng, và số dư là thứ hiện trên mọi lần mở
	// trang.
	TongTheoCreator(
		ctx context.Context, creatorID ids.ID,
	) (map[TrangThaiHoaHong]int64, error)
}
