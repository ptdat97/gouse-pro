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

// OwnerResolver đổi định danh NHÀ BÁN lấy định danh CHỦ SỞ HỮU tồn kho.
//
// Cổng do BÊN GỌI khai: module seller đứng TRÊN inventory trong đồ thị
// phụ thuộc nên hỏi ngược lên là vi phạm ranh giới. cmd/worker nối hai
// đầu, cùng mẫu với TokenVerifier và CustomerResolver.
type OwnerResolver interface {
	InventoryOwnerID(ctx context.Context, sellerID string) (string, error)
}

// ReleaseOnFulfillmentCancelled trả hàng về kho khi đơn thực hiện bị hủy.
//
// # Vì sao việc này KHÔNG được để thiếu
//
// Đường vào kho đã có (Reserved → Committed khi đặt hàng). Không có đường
// ra thì mỗi đơn bị hủy ăn mất một phần kho VĨNH VIỄN: hàng có thật trên
// kệ nhưng hệ thống mãi coi là đã cam kết cho một đơn không còn tồn tại.
//
// Kiểm chứng bằng đơn thật ngày 20/08 trước khi có handler này: đặt 5
// món rồi hủy, tồn kho đứng nguyên 15 khả dụng / 5 cam kết. Không lỗi,
// không log — chỉ phát hiện được khi kiểm kê tay thấy số thực nhiều hơn
// số hệ thống.
//
// # Hai điều kiện, không phải một
//
//  1. `release_stock` — hàng còn trong kho hay đang trên đường trả về.
//     Hủy sau khi giao thất bại thì hàng chưa cầm trong tay, và bán một
//     món chưa về là để lỗi hiện ra ở khách THỨ HAI.
//  2. Chủ sở hữu suy ra từ nhà bán qua OwnerForSeller (ADR-0012), không
//     dùng thẳng seller_id — own brand là seller nội bộ nhưng hàng của
//     nó thuộc nền tảng.
type ReleaseOnFulfillmentCancelled struct {
	module *Module
	owner  OwnerResolver
	log    *slog.Logger
}

// NewReleaseHandler tạo bên nhận trả hàng về kho khi hủy đơn thực hiện.
//
// owner có thể nil: khi đó chủ sở hữu được coi là CHÍNH nhà bán, đúng với
// mọi nhà bán ngoài. Thà trả được hàng cho nhà bán ngoài còn hơn không ai
// được trả.
func NewReleaseHandler(
	m *Module, owner OwnerResolver, log *slog.Logger,
) *ReleaseOnFulfillmentCancelled {
	return &ReleaseOnFulfillmentCancelled{module: m, owner: owner, log: log}
}

var _ eventbus.Handler = (*ReleaseOnFulfillmentCancelled)(nil)

func (h *ReleaseOnFulfillmentCancelled) Name() string {
	return "inventory.release_on_fulfillment_cancelled"
}

func (h *ReleaseOnFulfillmentCancelled) EventTypes() []string {
	return []string{eventbus.TypeFulfillmentCancelled}
}

// fulfillmentCancelledPayload chỉ khai những trường handler này dùng.
type fulfillmentCancelledPayload struct {
	OrderID         string `json:"order_id"`
	FulfillmentID   string `json:"fulfillment_id"`
	SellerID        string `json:"seller_id"`
	StockLocationID string `json:"stock_location_id"`
	ReleaseStock    bool   `json:"release_stock"`

	Lines []struct {
		SKUID    string `json:"sku_id"`
		Quantity int    `json:"quantity"`
	} `json:"lines"`
}

func (h *ReleaseOnFulfillmentCancelled) Handle(
	ctx context.Context, e eventbus.Event,
) error {
	var p fulfillmentCancelledPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("giải mã payload fulfillment.cancelled: %w", err)
	}

	// Hàng đã rời kho: KHÔNG trả về khả dụng ở đây. Nó quay lại qua quy
	// trình hàng trả, có bước kiểm tra chất lượng.
	if !p.ReleaseStock {
		h.log.InfoContext(ctx, "hủy đơn thực hiện nhưng hàng đã rời kho — không trả về",
			"fulfillment_id", p.FulfillmentID, "order_id", p.OrderID)
		return nil
	}
	if len(p.Lines) == 0 {
		return nil
	}

	ownerID := p.SellerID
	if h.owner != nil {
		resolved, err := h.owner.InventoryOwnerID(ctx, p.SellerID)
		if err != nil {
			return fmt.Errorf("tra chủ sở hữu tồn kho của %s: %w", p.SellerID, err)
		}
		ownerID = resolved
	}

	for _, l := range p.Lines {
		err := h.module.ReleaseCommittedInEventTx(ctx, ReleaseCommittedRequest{
			SKUID:       l.SKUID,
			OwnerID:     ownerID,
			LocationID:  p.StockLocationID,
			Quantity:    l.Quantity,
			Reason:      "Hủy đơn thực hiện " + p.FulfillmentID,
			ReferenceID: p.FulfillmentID,
			PerformedBy: p.SellerID,
		})

		// Không tìm thấy bản ghi tồn kho thì BỎ QUA món đó, không làm
		// hỏng cả event. Trả lỗi ở đây khiến dispatcher thử lại mãi, và
		// lần nào cũng hỏng như nhau — trong khi các món còn lại thì
		// đáng ra trả về được.
		if errors.Is(err, ErrNotFound) {
			h.log.WarnContext(ctx, "không có bản ghi tồn kho để trả hàng về",
				"sku_id", l.SKUID, "owner_id", ownerID,
				"fulfillment_id", p.FulfillmentID)
			continue
		}
		if err != nil {
			return fmt.Errorf("trả hàng về kho cho %s: %w", l.SKUID, err)
		}
	}
	return nil
}
