package customer

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/customer/application"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// eventPublisher nối cổng ra của tầng application với outbox.
type eventPublisher struct {
	outbox *eventbus.Outbox
}

var _ application.EventPublisher = (*eventPublisher)(nil)

// NewEventPublisher tạo bộ phát event nối với outbox.
func NewEventPublisher(outbox *eventbus.Outbox) application.EventPublisher {
	return &eventPublisher{outbox: outbox}
}

// PublishThemYeuThich ghi event vào outbox BẰNG giao dịch của kho lưu trữ.
func (p *eventPublisher) PublishThemYeuThich(
	ctx context.Context, in application.ThemYeuThich,
) error {
	tx, err := eventbus.MustTxFrom(ctx)
	if err != nil {
		return errors.New(
			"customer: phát event yêu thích ngoài giao dịch của kho lưu trữ")
	}

	e, err := eventbus.NewEvent(
		eventbus.TypeWishlistItemAdded,
		eventbus.AggregateCustomer,
		in.CustomerID,
		struct {
			CustomerID string `json:"customer_id"`
			ProductID  string `json:"product_id"`

			// VariantID có thể RỖNG: khách thích cả sản phẩm chứ không
			// riêng một size. Bên nhận phải chịu được cả hai.
			VariantID string `json:"variant_id"`

			// MuonBaoKhiCoHang là lời hứa "có hàng là tôi mua".
			//
			// Trường quý nhất của payload: nó biến một lượt quan tâm thành
			// một cam kết, và đó là tín hiệu nhu cầu rõ ràng nhất khách
			// chủ động để lại.
			MuonBaoKhiCoHang bool `json:"notify_when_available"`

			ThemLuc string `json:"added_at"`
		}{
			CustomerID:       in.CustomerID.String(),
			ProductID:        in.ProductID.String(),
			VariantID:        in.VariantID.String(),
			MuonBaoKhiCoHang: in.MuonBaoKhiCoHang,
			ThemLuc:          in.ThemLuc.UTC().Format(time.RFC3339),
		})
	if err != nil {
		return err
	}

	// Gốc chuỗi là mã KHÁCH: chưa có đơn nào ở bước này, và câu hỏi khi
	// tra cứu là "khách này đã quan tâm những gì".
	e = e.WithTrace(in.CustomerID.String(), "")

	return p.outbox.PublishTx(ctx, tx, e)
}
