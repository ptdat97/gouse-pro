package inventory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// CommitOnCheckoutCompleted chuyển Reserved → Committed khi đơn đã tạo xong.
//
// VÌ SAO BƯỚC NÀY QUAN TRỌNG: sau khi khách đặt hàng, số lượng phải rời
// khỏi trạng thái "đang giữ tạm" sang "đã cam kết cho đơn". Không chuyển
// thì hai chuyện xảy ra:
//
//  1. Báo cáo tồn kho sai — hàng đã bán vẫn hiện là "đang giữ"
//  2. Tiến trình dọn reservation quá hạn có thể NHẢ hàng của một đơn
//     đã thanh toán, và bán nó cho người khác
//
// Chuyện thứ hai là lý do việc này không thể để "làm sau": nó biến một đơn
// đã thu tiền thành đơn không có hàng.
//
// # Vì sao nghe checkout.completed chứ không phải order.placed
//
// Cần `reservation_id` để commit, mà `order` không giữ nó — reservation là
// dữ liệu VẬN HÀNH, không thuộc hợp đồng với khách. Chỉ `checkout` biết cả
// hai đầu: mã đơn vừa tạo và các mã giữ hàng đã dùng.
//
// Nhồi `reservation_id` vào Order chỉ để event này dùng được sẽ làm bẩn
// hợp đồng với khách bằng chi tiết vận hành.
type CommitOnCheckoutCompleted struct {
	module *Module
	log    *slog.Logger
}

// NewCommitHandler tạo bên nhận chuyển Reserved → Committed.
func NewCommitHandler(m *Module, log *slog.Logger) *CommitOnCheckoutCompleted {
	return &CommitOnCheckoutCompleted{module: m, log: log}
}

var _ eventbus.Handler = (*CommitOnCheckoutCompleted)(nil)

func (h *CommitOnCheckoutCompleted) Name() string {
	return "inventory.commit_on_checkout_completed"
}

func (h *CommitOnCheckoutCompleted) EventTypes() []string {
	return []string{eventbus.TypeCheckoutCompleted}
}

// checkoutCompletedPayload là phần dữ liệu bên nhận này cần.
//
// Chỉ khai báo những trường dùng tới: thêm trường mới vào event không được
// phá bên nhận cũ, và cách chắc chắn nhất là không đọc thứ mình không cần.
type checkoutCompletedPayload struct {
	OrderID      string `json:"order_id"`
	Reservations []struct {
		ReservationID string `json:"reservation_id"`
		Quantity      int    `json:"quantity"`
	} `json:"reservations"`
}

// Handle chuyển mọi reservation của phiên sang Committed.
//
// Chạy trong GIAO DỊCH của dispatcher (xem CommitInEventTx): việc commit và
// việc đánh dấu event đã xử lý cùng thành công hoặc cùng thất bại.
func (h *CommitOnCheckoutCompleted) Handle(ctx context.Context, e eventbus.Event) error {
	var p checkoutCompletedPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}

	for _, r := range p.Reservations {
		err := h.module.CommitInEventTx(ctx, r.ReservationID)

		switch {
		case err == nil:
			continue

		case errors.Is(err, domain.ErrReservationNotActive):
			// Reservation đã rời trạng thái ACTIVE — thường là đã commit rồi.
			//
			// KHÔNG phải lỗi: đây là kết quả MONG MUỐN khi event được phát
			// lại. Trả lỗi ở đây sẽ khiến event bị thử lại mãi rồi rơi vào
			// dead letter dù mọi thứ đều đúng.
			//
			// Đây là lớp idempotency THỨ HAI, độc lập với bảng
			// event_processed: kể cả khi cơ chế kia hỏng, trạng thái của
			// chính reservation vẫn chặn việc commit hai lần.
			h.log.Debug("reservation đã được xử lý trước đó",
				"reservation_id", r.ReservationID,
				"order_id", p.OrderID)
			continue

		case errors.Is(err, domain.ErrReservationExpired),
			errors.Is(err, ErrNotFound):
			// Reservation đã hết hạn hoặc không còn: bị dọn trước khi event
			// tới. Hàng đã nhả và có thể đã bán cho khách khác.
			//
			// Đây là SỰ CỐ THẬT cần người xem, nhưng thử lại không cứu được
			// gì — reservation không tự quay lại. Ghi cảnh báo rồi đi tiếp,
			// thay vì để event kẹt trong hàng đợi và chặn mọi event sau nó.
			h.log.Error("RESERVATION KHÔNG CÒN cho đơn đã đặt",
				"reservation_id", r.ReservationID,
				"order_id", p.OrderID,
				"lỗi", err,
				"hệ_quả", "hàng có thể đã bị nhả và bán cho khách khác")
			continue

		default:
			// Lỗi tạm thời (tranh chấp phiên bản, mất kết nối): trả về để
			// dispatcher thử lại toàn bộ event.
			return fmt.Errorf("commit reservation %s: %w", r.ReservationID, err)
		}
	}

	return nil
}
