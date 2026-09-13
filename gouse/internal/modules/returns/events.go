package returns

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/returns/application"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// eventPublisher nối cổng ra của tầng application với outbox.
//
// Nằm ở tầng này chứ không phải trong application: tầng application chỉ
// biết interface do chính nó định nghĩa, nên nó kiểm chứng được bằng bản
// giả mà không cần database. Đây cũng là điều quy tắc R1 của archcheck
// cưỡng chế.
type eventPublisher struct {
	outbox *eventbus.Outbox
}

var _ application.EventPublisher = (*eventPublisher)(nil)

// NewEventPublisher tạo bộ phát event nối với outbox.
func NewEventPublisher(outbox *eventbus.Outbox) application.EventPublisher {
	return &eventPublisher{outbox: outbox}
}

// PublishTraHangDuocXin ghi event vào outbox BẰNG giao dịch của kho lưu trữ.
//
// Ngữ cảnh phải mang giao dịch mà `LuuKemEvent` đã mở. Thiếu nó thì trả lỗi
// chứ KHÔNG âm thầm mở giao dịch riêng: ghi rời nghĩa là có thể có tín hiệu
// cho một yêu cầu đã bị cuộn ngược, hoặc có yêu cầu mà lý do hoàn không
// vào được dữ liệu chất lượng.
func (p *eventPublisher) PublishTraHangDuocXin(
	ctx context.Context, in application.TraHangDuocXin,
) error {
	tx, err := eventbus.MustTxFrom(ctx)
	if err != nil {
		return errors.New(
			"returns: phát event trả hàng ngoài giao dịch của kho lưu trữ")
	}

	type dongPayload struct {
		SKUID    string `json:"sku_id"`
		Quantity int    `json:"quantity"`

		// LyDo là mã lý do CHUẨN HÓA, ví dụ SIZE_TOO_SMALL.
		//
		// Chuẩn hóa là điều làm nó dùng được: "áo bé quá" và "chật" viết
		// tự do không gom được thành một con số, còn mã thì gom được — và
		// chỉ khi gom được thì nó mới nói lên bảng size có sai hay không.
		LyDo string `json:"reason_code"`
	}

	dong := make([]dongPayload, 0, len(in.Dong))
	for _, d := range in.Dong {
		dong = append(dong, dongPayload{
			SKUID:    d.SKUID.String(),
			Quantity: d.Quantity,
			LyDo:     d.LyDo,
		})
	}

	e, err := eventbus.NewEvent(
		eventbus.TypeReturnRequested,
		eventbus.AggregateReturn,
		in.ReturnID,
		struct {
			ReturnID   string        `json:"return_id"`
			OrderID    string        `json:"order_id"`
			CustomerID string        `json:"customer_id"`
			Lines      []dongPayload `json:"lines"`
			XinLuc     string        `json:"requested_at"`
		}{
			ReturnID:   in.ReturnID.String(),
			OrderID:    in.OrderID.String(),
			CustomerID: in.CustomerID.String(),
			Lines:      dong,
			XinLuc:     in.XinLuc.UTC().Format(time.RFC3339),
		})
	if err != nil {
		return err
	}

	// Gốc chuỗi là mã ĐƠN: việc trả hàng thuộc về một đơn cụ thể, và câu
	// hỏi khi tra cứu luôn là "đơn này đã xảy ra những gì".
	e = e.WithTrace(in.OrderID.String(), "")

	return p.outbox.PublishTx(ctx, tx, e)
}
