package cart

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/modules/cart/application"
	cartpg "github.com/fashion-commerce/platform/internal/modules/cart/infrastructure/postgres"
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

// PublishItemAdded ghi event vào outbox BẰNG giao dịch của kho lưu trữ.
//
// Ngữ cảnh phải mang giao dịch mà `SaveWithEvents` đã mở. Thiếu nó thì trả
// lỗi chứ KHÔNG âm thầm mở giao dịch riêng — ghi rời nghĩa là món có thể
// vào giỏ mà tín hiệu nhu cầu không tồn tại, và dữ liệu lịch sử không tạo
// ngược được.
func (p *eventPublisher) PublishItemAdded(
	ctx context.Context, in application.ItemAdded,
) error {
	tx, ok := cartpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"cart: phát event ngoài giao dịch của kho lưu trữ — event và " +
				"thay đổi giỏ phải cùng thành công hoặc cùng thất bại")
	}

	// Payload chứa ĐỦ để bên nhận xử lý mà không phải gọi ngược lại cart.
	//
	// supply-chain cần sku_id và quantity để ghi tín hiệu; nguồn giới thiệu
	// để trả lời "nội dung nào tạo nhu cầu thật".
	e, err := eventbus.NewEvent(
		eventbus.TypeCartItemAdded,
		eventbus.AggregateCart,
		in.CartID,
		struct {
			CartID          string `json:"cart_id"`
			OfferID         string `json:"offer_id"`
			SKUID           string `json:"sku_id"`
			SellerID        string `json:"seller_id"`
			Quantity        int    `json:"quantity"`
			SourceContentID string `json:"source_content_id"`
			SourceCreatorID string `json:"source_creator_id"`
		}{
			CartID:          in.CartID.String(),
			OfferID:         in.OfferID.String(),
			SKUID:           in.SKUID.String(),
			SellerID:        in.SellerID.String(),
			Quantity:        in.Quantity,
			SourceContentID: in.SourceContentID.String(),
			SourceCreatorID: in.SourceCreatorID.String(),
		})
	if err != nil {
		return err
	}

	return p.outbox.PublishTx(ctx, tx, e)
}
