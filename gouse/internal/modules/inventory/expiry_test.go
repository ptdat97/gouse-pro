package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
)

// Kịch bản mà cmd/worker phải xử lý được, kiểm chứng ĐẦU-CUỐI.
//
// VÌ SAO QUAN TRỌNG (docs/04-modules/inventory.md mục 6.3):
//
// Khách vào checkout thì hàng bị giữ. Nếu họ bỏ ngang và không có gì giải
// phóng, hàng nằm khóa vĩnh viễn. Tích lũy dần, cuối cùng không bán được
// gì — mà tồn kho trên báo cáo vẫn đầy.
//
// Đây là loại sự cố KHÔNG CÓ THÔNG BÁO LỖI nào cả: hệ thống vẫn chạy, chỉ
// là doanh số tụt dần. Test này bảo vệ đúng cơ chế đó.
func TestNhieuKhachBoCheckoutThiHangQuayLaiKe(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	const tonKho = 10
	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: tonKho,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	// Ba khách vào checkout rồi bỏ ngang.
	for i := 0; i < 3; i++ {
		if _, err := m.Reserve(ctx, inventory.ReserveRequest{
			ItemID: item.ID, Quantity: 2, TTL: time.Millisecond,
		}); err != nil {
			t.Fatalf("Reserve %d: %v", i, err)
		}
	}
	time.Sleep(20 * time.Millisecond)

	// Hàng đang bị khóa: chỉ còn 4/10 bán được.
	av, err := m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	if av[skuID] != 4 {
		t.Fatalf("khả dụng = %d, mong 4 (10 − 3×2 đang giữ)", av[skuID])
	}

	// Chỉ báo giám sát phải thấy tồn đọng.
	pending, err := m.Service().CountExpiredPending(ctx)
	if err != nil {
		t.Fatalf("CountExpiredPending: %v", err)
	}
	if pending != 3 {
		t.Fatalf("quá hạn chưa dọn = %d, mong 3", pending)
	}

	// Đây là việc cmd/worker gọi định kỳ 30 giây một lần.
	daDon, err := m.Service().ExpireReservations(ctx, 200)
	if err != nil {
		t.Fatalf("ExpireReservations: %v", err)
	}
	if daDon != 3 {
		t.Errorf("đã dọn = %d, mong 3", daDon)
	}

	// TOÀN BỘ hàng phải quay lại kệ.
	av, err = m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	if av[skuID] != tonKho {
		t.Errorf("khả dụng = %d, mong %d — hàng chưa quay lại kệ đầy đủ", av[skuID], tonKho)
	}

	// Và không còn tồn đọng.
	pending, err = m.Service().CountExpiredPending(ctx)
	if err != nil {
		t.Fatalf("CountExpiredPending: %v", err)
	}
	if pending != 0 {
		t.Errorf("còn %d bản ghi quá hạn sau khi dọn", pending)
	}
}

// Dọn hai lần liên tiếp phải AN TOÀN: lượt hai không tìm thấy gì để dọn.
//
// Quan trọng vì worker có thể chạy nhiều bản song song (nhiều pod), và hai
// bản có thể cùng quét một khoảng thời gian. Nếu dọn hai lần cộng hàng lên
// hai lần, tồn kho ảo sẽ nhiều hơn thực tế.
func TestDonHaiLanKhongCongHangLenHaiLan(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: 5,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if _, err := m.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 3, TTL: time.Millisecond,
	}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	if _, err := m.Service().ExpireReservations(ctx, 100); err != nil {
		t.Fatalf("ExpireReservations lần 1: %v", err)
	}
	daDon, err := m.Service().ExpireReservations(ctx, 100)
	if err != nil {
		t.Fatalf("ExpireReservations lần 2: %v", err)
	}
	if daDon != 0 {
		t.Errorf("lượt hai dọn được %d, mong 0", daDon)
	}

	av, err := m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	// Đúng 5, KHÔNG phải 8 — hàng không được cộng lên hai lần.
	if av[skuID] != 5 {
		t.Errorf("khả dụng = %d, mong 5 — hàng bị cộng lên nhiều lần", av[skuID])
	}
}

// Reservation CÒN HẠN không được đụng tới.
//
// Dọn nhầm hàng đang giữ hợp lệ sẽ khiến khách đang thanh toán mất hàng.
func TestKhongDonReservationConHan(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: 5,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	// TTL dài — khách vẫn đang thanh toán.
	res, err := m.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 2, TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	daDon, err := m.Service().ExpireReservations(ctx, 100)
	if err != nil {
		t.Fatalf("ExpireReservations: %v", err)
	}
	if daDon != 0 {
		t.Errorf("dọn nhầm %d reservation còn hạn", daDon)
	}

	// Hàng vẫn được giữ cho khách.
	av, err := m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	if av[skuID] != 3 {
		t.Errorf("khả dụng = %d, mong 3 — hàng đang giữ bị giải phóng nhầm", av[skuID])
	}

	// Và khách vẫn cam kết được.
	if err := m.Commit(ctx, res.ID); err != nil {
		t.Errorf("khách đang thanh toán không cam kết được: %v", err)
	}
}
