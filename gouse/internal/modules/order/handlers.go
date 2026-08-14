package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// RecalculateOnFulfillmentProgress tính lại trạng thái tổng hợp của đơn.
//
// QUY TẮC 7: trạng thái đơn được SUY RA từ tiến độ các nguồn hàng, không
// tự đặt. Đặt tay ở hai chỗ sẽ dẫn tới đơn báo "đã giao" khi một gói còn
// nằm ở kho.
//
// # Vì sao đi bằng event chứ không gọi trực tiếp
//
// Module fulfillment trỏ tới order qua `order_id`. Nếu order gọi ngược lại
// fulfillment để hỏi tiến độ, hai module phụ thuộc lẫn nhau — và không
// tách được service nào ra khỏi cái nào.
//
// Event đi MỘT CHIỀU: fulfillment phát, order nghe. Module fulfillment
// không biết order tồn tại.
type RecalculateOnFulfillmentProgress struct {
	module *Module
	log    *slog.Logger
}

// NewProgressHandler tạo bên nhận tính lại trạng thái tổng hợp.
func NewProgressHandler(m *Module, log *slog.Logger) *RecalculateOnFulfillmentProgress {
	return &RecalculateOnFulfillmentProgress{module: m, log: log}
}

var _ eventbus.Handler = (*RecalculateOnFulfillmentProgress)(nil)

func (h *RecalculateOnFulfillmentProgress) Name() string {
	return "order.recalculate_on_fulfillment_progress"
}

func (h *RecalculateOnFulfillmentProgress) EventTypes() []string {
	return []string{eventbus.TypeFulfillmentProgress}
}

// progressPayload là phần dữ liệu bên nhận này cần.
type progressPayload struct {
	OrderID  string `json:"order_id"`
	Progress []struct {
		Cancelled bool `json:"cancelled"`
		Delivered bool `json:"delivered"`
		Shipped   bool `json:"shipped"`
	} `json:"progress"`
}

// Handle tính lại trạng thái tổng hợp từ tiến độ mọi nguồn hàng.
//
// Event mang tiến độ của TẤT CẢ nguồn hàng, không chỉ nguồn vừa đổi — nhờ
// vậy hàm này tính được ngay mà không phải gọi ngược fulfillment.
func (h *RecalculateOnFulfillmentProgress) Handle(
	ctx context.Context, e eventbus.Event,
) error {
	var p progressPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}

	if len(p.Progress) == 0 {
		// Không có tiến độ nào để tính: không phải lỗi.
		return nil
	}

	progress := make([]FulfillmentProgressInput, 0, len(p.Progress))
	for _, item := range p.Progress {
		progress = append(progress, FulfillmentProgressInput{
			Cancelled: item.Cancelled,
			Delivered: item.Delivered,
			Shipped:   item.Shipped,
		})
	}

	err := h.module.ApplyFulfillmentProgress(ctx, p.OrderID, progress)

	switch {
	case err == nil:
		return nil

	case errors.Is(err, ErrNotFound):
		// Đơn không tồn tại: dữ liệu đã phân kỳ giữa hai module. Thử lại
		// không cứu được gì — đơn không tự xuất hiện.
		//
		// Ghi cảnh báo rồi đi tiếp, thay vì để event kẹt và chặn mọi event
		// sau nó trong hàng đợi.
		h.log.Error("KHÔNG TÌM THẤY đơn hàng khi cập nhật tiến độ",
			"order_id", p.OrderID,
			"hệ_quả", "trạng thái đơn sẽ không phản ánh tiến độ giao hàng")
		return nil

	case errors.Is(err, ErrInvalidStatus):
		// Đơn đã ở trạng thái cuối (COMPLETED/CANCELLED) nên không tính
		// lại. Đây là kết quả mong muốn, không phải lỗi.
		return nil

	default:
		// Lỗi tạm thời: trả về để dispatcher thử lại.
		return fmt.Errorf("cập nhật trạng thái đơn %s: %w", p.OrderID, err)
	}
}
