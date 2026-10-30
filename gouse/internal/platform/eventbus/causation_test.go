package eventbus_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// publishingHandler phát một event MỚI trong lúc xử lý event cũ.
//
// Đúng hình dạng của `fulfillment.cancel_on_order_cancelled` (hủy đơn thực
// hiện rồi phát `fulfillment.cancelled`) và của
// `order.recalculate_on_fulfillment_progress` (ghi tiền COD rồi phát
// `order.paid`).
type publishingHandler struct {
	outbox *eventbus.Outbox
	kind   string
	conID  ids.ID
}

func (h *publishingHandler) Name() string         { return "test.publishing_handler" }
func (h *publishingHandler) EventTypes() []string { return []string{"order.placed"} }

func (h *publishingHandler) Handle(ctx context.Context, _ eventbus.Event) error {
	tx, err := eventbus.MustTxFrom(ctx)
	if err != nil {
		return err
	}
	e, err := eventbus.NewEvent(h.kind, eventbus.AggregateOrder, h.conID,
		map[string]any{"from": "handler"})
	if err != nil {
		return err
	}
	return h.outbox.PublishTx(ctx, tx, e)
}

// TestAnEventPublishedFromAHandlerCarriesItsParent.
//
// # Vì sao `correlation_id` không đủ
//
// Correlation GOM: nó nói "những event này thuộc cùng một hành động của
// khách". Causation XẾP THỨ TỰ: nó nói "event nào sinh ra event này". Có
// cái đầu mà không có cái sau thì ta có một đống event cùng nhãn, không có
// cây — và câu hỏi lúc sự cố luôn là "cái gì gây ra cái này".
//
// Trước 30/10/2026 cột `causation_id` RỖNG trên 16.061 hàng: `WithTrace` là
// đường duy nhất điền nó và không bên phát nào gọi. Tìm ra bằng
// `cmd/deadcol` (P3-92), không bằng đọc mã — chín lần trước cũng vậy.
func TestAnEventPublishedFromAHandlerCarriesItsParent(t *testing.T) {
	bus, pool := newBus(t)
	ctx := context.Background()

	con := ids.MustNew(ids.PrefixOrder)
	bus.Subscribe(&publishingHandler{
		outbox: bus.Outbox(), kind: "order.paid", conID: con})

	cha := newEvent(t, "order.placed")
	if err := bus.Outbox().Publish(ctx, cha); err != nil {
		t.Fatalf("phát event cha: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	var causation string
	if err := pool.QueryRow(ctx,
		`SELECT coalesce(causation_id, '') FROM event_outbox
		  WHERE event_type = 'order.paid'`).Scan(&causation); err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}

	if causation != cha.ID.String() {
		t.Errorf("causation_id = %q, cần %q — event phát TRONG một bên nhận "+
			"phải nhớ event nào sinh ra nó, không thì chuỗi gom được mà "+
			"không xếp được thành cây", causation, cha.ID.String())
	}
}

// TestARootEventHasNoParent: và RỖNG là câu trả lời ĐÚNG ở đây.
//
// Bài này là nửa còn lại, và nó quan trọng ngang nửa trên. Cách sai hiển
// nhiên nhất khi điền một trường truy vết là điền nó Ở MỌI CHỖ — ví dụ rơi
// về mã request HTTP như `correlation_id` làm. Làm thế biến "không có cha"
// thành "cha là một thứ không phải event", và phép dựng cây sẽ đi tìm một
// mắt không tồn tại.
func TestARootEventHasNoParent(t *testing.T) {
	bus, pool := newBus(t)

	// Có mã request trong ngữ cảnh: đây đúng là chỗ một bản cài cẩu thả sẽ
	// lấy giá trị thay thế.
	ctx := logger.WithRequestID(context.Background(),
		"req_01J9XABC123DEF456GHJKMNPQR")

	e := newEvent(t, "checkout.completed")
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("phát event: %v", err)
	}

	var corr, causation string
	if err := pool.QueryRow(ctx,
		`SELECT coalesce(correlation_id, ''), coalesce(causation_id, '')
		   FROM event_outbox WHERE event_id = $1`,
		e.ID.String()).Scan(&corr, &causation); err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}

	if corr == "" {
		t.Error("correlation_id rỗng — bài này đang KHÔNG kiểm được gì: " +
			"nó cần một ngữ cảnh CÓ mã request để chứng minh causation " +
			"không lấy bừa giá trị ấy")
	}
	if causation != "" {
		t.Errorf("causation_id = %q, cần RỖNG — event phát ngoài mọi bên "+
			"nhận là GỐC cây; điền gì vào đó cũng là trỏ tới một event "+
			"không tồn tại", causation)
	}
}

// TestAPublisherSuppliedCausationIsKept: `WithTrace` vẫn thắng ngữ cảnh.
//
// Cùng thứ tự ưu tiên với correlation: bên phát biết rõ hơn thì nó quyết.
func TestAPublisherSuppliedCausationIsKept(t *testing.T) {
	bus, pool := newBus(t)
	ctx := context.Background()

	chaThat := ids.MustNew(ids.PrefixEvent).String()
	e := newEvent(t, "order.paid").WithTrace("ord_x", chaThat)
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("phát event: %v", err)
	}

	var causation string
	if err := pool.QueryRow(ctx,
		`SELECT coalesce(causation_id, '') FROM event_outbox WHERE event_id = $1`,
		e.ID.String()).Scan(&causation); err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}
	if causation != chaThat {
		t.Errorf("causation_id = %q, cần %q — giá trị bên phát tự đặt phải "+
			"thắng giá trị kế thừa từ ngữ cảnh", causation, chaThat)
	}
}
