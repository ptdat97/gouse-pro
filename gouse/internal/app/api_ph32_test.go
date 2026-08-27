package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/checkout"
	checkoutpg "github.com/fashion-commerce/platform/internal/modules/checkout/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// dongHoChanCheckout là đồng hồ thật, DỪNG ở lần gọi đầu tiên.
//
// `CompleteCheckout` gọi `s.clock.Now()` ĐÚNG sau khi đọc phiên và TRƯỚC
// mọi kiểm tra dùng thời gian:
//
//	c := s.checkouts.FindByID(...)   ← đọc
//	if c.Status() == Completed {...}
//	now := s.clock.Now()             ← chỗ này
//	if c.IsExpired(now) {...}        ← kiểm trên bản TRONG BỘ NHỚ
//
// Clock vốn đã là cổng tiêm sẵn có (`checkout.Config.Clock`), không phải
// thứ thêm vào để test dễ.
type dongHoChanCheckout struct {
	mu     sync.Mutex
	daChan bool
	toiRoi chan struct{}
	diTiep chan struct{}
}

func newDongHoChanCheckout() *dongHoChanCheckout {
	return &dongHoChanCheckout{
		toiRoi: make(chan struct{}),
		diTiep: make(chan struct{}),
	}
}

func (d *dongHoChanCheckout) Now() time.Time {
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

// TestPH32_PhienHetHanTrongLucDangHoanTat.
//
// # Bất biến
//
// Một phiên thanh toán ĐÃ HẾT HẠN không được sinh ra đơn hàng. Hàng của
// nó đã bị nhả về kho và có thể đã bán cho người khác.
//
// # Vì sao lớp kiểm hiện có không giữ nổi
//
// Cùng lớp lỗi với PH-31, cùng cơ chế: PostgreSQL chạy READ COMMITTED nên
// mỗi câu lệnh lấy một ảnh chụp mới, và `CompleteCheckout` kiểm
// `IsExpired()` trên bản phiên ĐÃ ĐỌC TRƯỚC ĐÓ.
//
//	T2: đọc phiên            → ACTIVE, chưa hết hạn
//	T1: job dọn hạn chạy     → đánh dấu EXPIRED + NHẢ toàn bộ reservation
//	T2: kiểm IsExpired() trên bản trong bộ nhớ → "chưa hết hạn" → đi tiếp
//	T2: tạo đơn hàng cho số hàng VỪA được trả lại kho
//
// Bài này KHÔNG trông chờ xác suất: nó dừng T2 đúng giữa khe bằng đồng hồ.
func TestPH32_PhienHetHanTrongLucDangHoanTat(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	dongHo := newDongHoChanCheckout()
	cham, err := checkout.New(checkout.Config{
		Storage:     "postgres",
		DB:          a.db,
		Cart:        a.mods.cart,
		Inventory:   a.mods.inventory,
		Marketplace: a.mods.marketplace,
		Order:       a.mods.order,
		Seller:      a.mods.seller,
		Events:      eventbus.NewOutbox(a.db.Pool()),
		Clock:       dongHo,
	})
	if err != nil {
		t.Fatalf("dựng checkout thứ hai: %v", err)
	}

	maPhien := a.dungPhienSanHoanTat("ph32@example.com", "0900555222")

	truocDon := a.demDong(`"order"`, "")

	// T2 — hoàn tất phiên, dừng ngay sau khi đọc phiên.
	loiT2 := make(chan error, 1)
	go func() {
		_, err := cham.CompleteCheckout(ctx, maPhien, "req_ph32_"+maPhien[4:20])
		loiT2 <- err
	}()

	select {
	case <-dongHo.toiRoi:
	case <-time.After(5 * time.Second):
		t.Fatal("T2 không tới được chỗ chặn — đường code đã đổi?")
	}

	// T1 — job dọn hạn: đẩy phiên quá hạn rồi dọn, đúng như worker làm.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE checkout SET expires_at = now() - interval '1 hour' WHERE id = $1`,
		maPhien); err != nil {
		t.Fatalf("đẩy hạn: %v", err)
	}
	if _, err := a.mods.checkout.ExpireStale(ctx, 10); err != nil {
		t.Fatalf("dọn phiên quá hạn: %v", err)
	}

	// Hàng đã được nhả: đó là lý do phiên này không được tạo đơn nữa.
	conGiu := a.demDong("reservation",
		"checkout_id = $1 AND status = 'ACTIVE'", maPhien)
	if conGiu != 0 {
		t.Fatalf("sau khi dọn còn %d reservation ACTIVE — job dọn không chạy", conGiu)
	}

	close(dongHo.diTiep)

	var errT2 error
	select {
	case errT2 = <-loiT2:
	case <-time.After(10 * time.Second):
		t.Fatal("T2 không kết thúc")
	}

	// ---------------------------------------------------------------

	sauDon := a.demDong(`"order"`, "")
	if sauDon != truocDon {
		t.Errorf("phiên ĐÃ HẾT HẠN vẫn sinh ra %d đơn hàng — "+
			"đơn được tạo cho số hàng vừa trả lại kho", sauDon-truocDon)
	}
	if errT2 == nil {
		t.Error("hoàn tất phiên đã hết hạn báo THÀNH CÔNG")
	} else {
		t.Logf("T2 trả: %v", errT2)
	}

	// Phiên phải GIỮ trạng thái EXPIRED, không bị ghi đè thành COMPLETED.
	var tt string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT status FROM checkout WHERE id = $1`, maPhien).Scan(&tt); err != nil {
		t.Fatalf("đọc trạng thái phiên: %v", err)
	}
	if tt == "COMPLETED" {
		t.Error("phiên hết hạn bị ghi đè thành COMPLETED")
	}
}

// TestPH32_GiuDeHoanTatChanJobDonHan — nửa còn lại của bất biến.
//
// Bài trên đo hướng nguy hiểm: job thắng thì KHÔNG được có đơn hàng. Bài
// này đo hướng ngược: phiên đã được giữ để hoàn tất thì job dọn hạn không
// được nhả hàng của nó.
//
// # Bài này chứng minh CÁI GÌ, và KHÔNG chứng minh cái gì
//
// Nó chứng minh phần ÂN HẠN: `GiuDeHoanTat` đẩy `expires_at` lên trước,
// nên `FindExpired` không nhặt phiên này nữa.
//
// Nó KHÔNG chứng minh nhánh `!duoc` trong `ExpireStale`. Nhánh ấy chỉ
// chạy khi job đã ĐỌC XONG danh sách quá hạn TRƯỚC lúc ân hạn được ghi —
// một cửa sổ hẹp hơn. Đã kiểm bằng cách phá: vô hiệu hoá nhánh đó thì bài
// này VẪN XANH.
//
// Không dựng lại được cửa sổ ấy một cách xác định với code hiện tại:
// `ExpireStale` gọi `clock.Now()` TRƯỚC `FindExpired`, nên cổng tiêm đồng
// hồ không chặn được vào đúng khe giữa hai bước. Nhánh `!duoc` vì thế là
// phòng thủ chiều sâu CHƯA có test — ghi ra đây thay vì để ngầm hiểu là
// đã được phủ.
func TestPH32_GiuDeHoanTatChanJobDonHan(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	maPhien := a.dungPhienSanHoanTat("ph32b@example.com", "0900555111")

	// Phiên sắp hết hạn — đúng lúc job sẽ nhặt tới.
	if _, err := a.db.Pool().Exec(ctx,
		`UPDATE checkout SET expires_at = now() + interval '1 second' WHERE id = $1`,
		maPhien); err != nil {
		t.Fatalf("đặt hạn: %v", err)
	}

	// Khách bấm "Đặt hàng": phiên được giữ lại thêm một quãng ân hạn.
	kho := checkoutpg.NewCheckoutStore(a.db.Pool())
	if err := kho.GiuDeHoanTat(
		ctx, ids.ID(maPhien), time.Now().UTC(), 30*time.Second); err != nil {
		t.Fatalf("giữ để hoàn tất: %v", err)
	}

	// Job chạy SAU khi hạn cũ đã qua.
	time.Sleep(1200 * time.Millisecond)
	if _, err := a.mods.checkout.ExpireStale(ctx, 50); err != nil {
		t.Fatalf("dọn phiên quá hạn: %v", err)
	}

	// Hàng phải CÒN NGUYÊN: đơn đang được tạo dựa vào nó.
	conGiu := a.demDong("reservation", "checkout_id = $1 AND status = 'ACTIVE'", maPhien)
	if conGiu == 0 {
		t.Error("job dọn hạn đã NHẢ hàng của một phiên đang được hoàn tất")
	}

	var tt string
	if err := a.db.Pool().QueryRow(ctx,
		`SELECT status FROM checkout WHERE id = $1`, maPhien).Scan(&tt); err != nil {
		t.Fatalf("đọc trạng thái: %v", err)
	}
	if tt == "EXPIRED" {
		t.Error("phiên đang được hoàn tất bị đánh dấu EXPIRED")
	}
}
