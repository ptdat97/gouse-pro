package app

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/modules/order"
)

// dongHoChanOrder dừng ở lần gọi đầu, đúng khe giữa đọc và ghi của
// CancelOwnOrder:
//
//	o := s.orders.FindByID(...)                ← đọc
//	o.CancelWithReason(reason, s.clock.Now())  ← chỗ này
//	s.orders.Update(ctx, o)                    ← ghi
type dongHoChanOrder struct {
	mu     sync.Mutex
	daChan bool
	toiRoi chan struct{}
	diTiep chan struct{}
}

func newDongHoChanOrder() *dongHoChanOrder {
	return &dongHoChanOrder{toiRoi: make(chan struct{}), diTiep: make(chan struct{})}
}

func (d *dongHoChanOrder) Now() time.Time {
	d.mu.Lock()
	lanDau := !d.daChan
	d.daChan = true
	d.mu.Unlock()
	if lanDau {
		close(d.toiRoi)
		<-d.diTiep
	}
	return time.Now().UTC()
}

// TestHuyDonKhongDeGhiDeTienDoGiaoHang.
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
func TestHuyDonKhongDeGhiDeTienDoGiaoHang(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	dongHo := newDongHoChanOrder()
	cham, err := order.New(order.Config{
		Storage: "postgres", DB: a.db, Clock: dongHo,
	})
	if err != nil {
		t.Fatalf("dựng order thứ hai: %v", err)
	}

	maPhien := a.dungPhienSanHoanTat("dua@example.com", "0900777222")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)

	// T2 — khách bấm Hủy, dừng ngay sau khi đọc đơn.
	loiT2 := make(chan error, 1)
	go func() {
		loiT2 <- cham.CancelOrder(ctx, maDon, "khách đổi ý")
	}()

	select {
	case <-dongHo.toiRoi:
	case <-time.After(5 * time.Second):
		t.Fatal("T2 không tới được chỗ chặn — đường code đã đổi?")
	}

	// T1 — nhà bán bàn giao: worker cập nhật tiến độ, đơn thành SHIPPED.
	if err := a.mods.order.ApplyFulfillmentProgress(ctx, maDon,
		[]order.FulfillmentProgressInput{{Shipped: true}}); err != nil {
		t.Fatalf("cập nhật tiến độ: %v", err)
	}
	if tt := a.trangThaiDon(t, maDon); tt != "SHIPPED" {
		t.Fatalf("sau bàn giao đơn ở %q, cần SHIPPED", tt)
	}

	close(dongHo.diTiep)

	var errT2 error
	select {
	case errT2 = <-loiT2:
	case <-time.After(10 * time.Second):
		t.Fatal("T2 không kết thúc")
	}

	// ---------------------------------------------------------------

	if tt := a.trangThaiDon(t, maDon); tt == "CANCELLED" {
		t.Error("đơn ĐÃ RỜI KHO bị ghi thành CANCELLED — " +
			"hàng đã đi mà hệ thống tin là đã hủy")
	}
	if errT2 == nil {
		t.Error("lệnh hủy báo THÀNH CÔNG trên một đơn đã bàn giao vận chuyển")
	} else {
		t.Logf("T2 trả: %v", errT2)
	}
}
