package domain

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// TxFunc chạy bên trong giao dịch mà kho lưu trữ đã mở.
//
// Ngữ cảnh truyền vào MANG giao dịch đó. Đây là cách tầng application phát
// domain event mà không cần biết database — nó chỉ nhận một ngữ cảnh và
// đưa cho bộ phát event.
//
// VÌ SAO CẦN: ghi trạng thái phiên và ghi event vào outbox phải cùng thành
// công hoặc cùng thất bại. Phiên chuyển COMPLETED mà event không được ghi
// nghĩa là tồn kho không bao giờ chuyển sang Committed — hàng đã bán vẫn
// nằm ở trạng thái "đang giữ" cho tới khi có người phát hiện thủ công.
type TxFunc func(ctx context.Context) error

// Repository là PORT cho kho lưu trữ phiên thanh toán.
type Repository interface {
	Save(ctx context.Context, c *Checkout) error

	// SaveWithEvents ghi phiên VÀ chạy fn trong CÙNG một giao dịch.
	//
	// fn thất bại → toàn bộ hoàn tác, kể cả thay đổi phiên.
	SaveWithEvents(ctx context.Context, c *Checkout, fn TxFunc) error

	FindByID(ctx context.Context, id ids.ID) (*Checkout, error)

	// FindByCompletionKey tra phiên theo khóa hoàn tất.
	//
	// QUY TẮC 5: hoàn tất phải idempotent. Khách bấm "Thanh toán" hai lần
	// thì lần thứ hai phải trả đơn CŨ — hai đơn cho một lần mua nghĩa là
	// khách bị trừ tiền hai lần.
	FindByCompletionKey(ctx context.Context, key string) (*Checkout, error)

	// FindActiveByCart tìm phiên còn hiệu lực của một giỏ.
	//
	// Khách bấm "Thanh toán" rồi quay lại giỏ rồi bấm lần nữa: phải nhận
	// lại ĐÚNG phiên cũ, không mở phiên thứ hai. Phiên thứ hai sẽ giữ hàng
	// lần thứ hai cho cùng một giỏ — khóa gấp đôi số hàng thật cần.
	FindActiveByCart(ctx context.Context, cartID ids.ID) (*Checkout, error)

	// FindExpired lấy các phiên đã quá hạn mà chưa được dọn.
	//
	// Đây là đầu vào của tiến trình nền. Mỗi phiên trong danh sách này
	// đang KHÓA HÀNG mà không ai dùng tới.
	FindExpired(ctx context.Context, now time.Time, limit int) ([]*Checkout, error)

	// GiuDeHoanTat giành quyền hoàn tất một phiên, bằng MỘT câu lệnh
	// nguyên tử.
	//
	// # Vì sao cần
	//
	// `CompleteCheckout` kiểm hạn trên bản phiên ĐÃ ĐỌC TRƯỚC ĐÓ. Job dọn
	// hạn chen vào giữa lúc đọc và lúc tạo đơn sẽ nhả toàn bộ hàng, và
	// phiên vẫn tạo đơn cho số hàng vừa trả lại kho (PH-32).
	//
	// Hàm này đẩy `expires_at` lên trước một quãng ân hạn NẾU phiên còn
	// hiệu lực. Job dọn hạn từ đó không nhặt phiên này nữa, nên nó không
	// nhả hàng trong lúc đơn đang được tạo.
	//
	// Trả ErrExpired hoặc ErrInvalidStatus khi không giành được — bên gọi
	// phải DỪNG, không được tạo đơn.
	//
	// Ân hạn tự hết: tiến trình chết giữa chừng thì phiên hết hạn bình
	// thường sau đó. Không có trạng thái nào kẹt lại để ai đó phải dọn.
	GiuDeHoanTat(ctx context.Context, id ids.ID, now time.Time, anHan time.Duration) error

	// GiuDeDonHan giành quyền DỌN một phiên quá hạn, bằng MỘT câu lệnh
	// nguyên tử.
	//
	// Trả về false khi phiên vừa được bên khác giành — đang hoàn tất, hoặc
	// đã kết thúc. Bên gọi PHẢI bỏ qua phiên đó và KHÔNG nhả hàng của nó.
	//
	// Hai hàm giành quyền này loại trừ nhau: chúng cùng sửa một dòng bằng
	// một câu lệnh, nên PostgreSQL xếp chúng nối đuôi, và điều kiện WHERE
	// của bên thua không còn đúng nữa.
	GiuDeDonHan(ctx context.Context, id ids.ID, now time.Time) (bool, error)

	// CountExpiredPending đếm số phiên quá hạn chưa dọn.
	//
	// Chỉ báo giám sát: con số này tăng dần nghĩa là tiến trình dọn đã
	// ngừng chạy, và hàng đang bị khóa mà không ai biết.
	CountExpiredPending(ctx context.Context, now time.Time) (int, error)
}
