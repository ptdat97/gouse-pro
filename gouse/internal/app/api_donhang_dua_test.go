package app

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/order"
)

// blockingOrderClock dừng ở lần gọi đầu, đúng khe giữa đọc và ghi của
// CancelOwnOrder:
//
//	o := s.orders.FindByID(...)                ← đọc
//	o.CancelWithReason(reason, s.clock.Now())  ← chỗ này
//	s.orders.Update(ctx, o)                    ← ghi
type blockingOrderClock struct {
	mu      sync.Mutex
	blocked bool
	arrived chan struct{}
	proceed chan struct{}
}

func newBlockingOrderClock() *blockingOrderClock {
	return &blockingOrderClock{arrived: make(chan struct{}), proceed: make(chan struct{})}
}

func (d *blockingOrderClock) Now() time.Time {
	d.mu.Lock()
	firstTime := !d.blocked
	d.blocked = true
	d.mu.Unlock()
	if firstTime {
		close(d.arrived)
		<-d.proceed
	}
	return types.Now()
}

// TestCancellingAnOrderDoesNotOverwriteDeliveryProgress.
//
// # Bất biến
//
// Một đơn ĐÃ RỜI KHO không được ghi thành "đã hủy". Hàng đã đi; ghi hủy
// nghĩa là hoàn tiền cho một lô hàng khách vẫn nhận được.
//
// # Vì sao ca này xảy ra được
//
// Tìm ra khi quét có hệ thống hình dạng `FindByID` + `Update` — cùng hình
// dạng đã gây ra PH-31 và PH-32. Bảng `order` KHÔNG có cột `version`, và
// hai đường sau cùng sửa nó, ở hai tiến trình khác nhau:
//
//	khách bấm Hủy           → API      → CancelOwnOrder
//	nhà bán bàn giao        → worker   → ApplyFulfillmentProgress
//
// Cả hai đọc-sửa-ghi ở hai giao dịch rời. Ai ghi sau thắng, và bản ghi
// của người kia biến mất — không lỗi, không cảnh báo.
//
// Bài này dừng lượt hủy đúng giữa khe bằng đồng hồ, không trông chờ xác suất.
func TestCancellingAnOrderDoesNotOverwriteDeliveryProgress(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	clock := newBlockingOrderClock()
	graded, err := order.New(order.Config{
		Storage: "postgres", DB: a.db, Clock: clock,
	})
	if err != nil {
		t.Fatalf("dựng order thứ hai: %v", err)
	}

	checkoutID := a.buildCheckoutReadyToComplete("dua@example.com", "0900777222")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+checkoutID+"/complete",
		map[string]any{"payment_method": "COD"}, idemKey())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	ord, _ := res.body["order"].(map[string]any)
	orderID, _ := ord["id"].(string)

	// T2 — khách bấm Hủy, dừng ngay sau khi đọc đơn.
	errT2Ch := make(chan error, 1)
	go func() {
		errT2Ch <- graded.CancelOrder(ctx, orderID, "khách đổi ý")
	}()

	select {
	case <-clock.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("T2 không tới được chỗ chặn — đường code đã đổi?")
	}

	// T1 — nhà bán bàn giao: worker cập nhật tiến độ, đơn thành SHIPPED.
	if err := a.mods.order.ApplyFulfillmentProgress(ctx, orderID,
		[]order.FulfillmentProgressInput{{Shipped: true}}); err != nil {
		t.Fatalf("cập nhật tiến độ: %v", err)
	}
	if tt := a.orderStatus(t, orderID); tt != "SHIPPED" {
		t.Fatalf("sau bàn giao đơn ở %q, cần SHIPPED", tt)
	}

	close(clock.proceed)

	var errT2 error
	select {
	case errT2 = <-errT2Ch:
	case <-time.After(10 * time.Second):
		t.Fatal("T2 không kết thúc")
	}

	// ---------------------------------------------------------------

	if tt := a.orderStatus(t, orderID); tt == "CANCELLED" {
		t.Error("đơn ĐÃ RỜI KHO bị ghi thành CANCELLED — " +
			"hàng đã đi mà hệ thống tin là đã hủy")
	}
	if errT2 == nil {
		t.Error("lệnh hủy báo THÀNH CÔNG trên một đơn đã bàn giao vận chuyển")
	} else {
		t.Logf("T2 trả: %v", errT2)
	}
}
