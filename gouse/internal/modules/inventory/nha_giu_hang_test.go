package inventory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
)

// TestNhaGiuHangHaiLanKhongSinhHangTuKhongKhi — lỗi tồn kho nghiêm trọng.
//
// # Triệu chứng thật, tìm được nhờ chỉ số mới (26/08)
//
// Job "dọn giữ hàng quá hạn" thất bại MỌI lượt suốt nhiều giờ với
// "inventory: không đủ hàng: reserved có 0, cần 1". Trước khi có
// `gouse_worker_job_last_success_timestamp_seconds`, nhịp tim toàn cục vẫn
// tươi rói vì các job khác vẫn chạy — không gì nổi lên.
//
// Nhật ký biến động cho thấy nguyên nhân:
//
//	18:09:17  RESERVE  1  → còn 76
//	18:19:28  RELEASE  1  → còn 77
//	18:19:28  RELEASE  1  → còn 78   ← nhả LẦN HAI, cách 1 mili giây
//
// 78 CAO HƠN số trước khi giữ. Đó là hàng sinh ra từ không khí: hệ thống
// tin mình có nhiều hàng hơn thực tế, và sẽ bán phần chênh đó cho ai đó.
//
// # Nguyên nhân gốc
//
// `releaseWith` đọc reservation, kiểm trạng thái TRONG BỘ NHỚ, rồi ghi.
// Khóa lạc quan bảo vệ bản ghi TỒN KHO, không bảo vệ RESERVATION — và
// `ReservationStore.Save` là một upsert KHÔNG ĐIỀU KIỆN.
//
// Hai lượt nhả song song vì thế cùng đọc thấy ACTIVE, cùng đi qua kiểm
// tra domain, và cùng ghi. Kiểm tra ở tầng ứng dụng không thay được ràng
// buộc ở tầng dữ liệu.
//
// Hai đường nhả CÓ THẬT chạy song song trong hệ thống: `releaseAll` khi
// phiên thanh toán thất bại, và job dọn giữ hàng quá hạn.
func TestNhaGiuHangHaiLanKhongSinhHangTuKhongKhi(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU)

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID.String(), LocationID: locID, Quantity: 10,
		PerformedBy: "test",
	})
	if err != nil {
		t.Fatalf("nhập kho: %v", err)
	}

	res, err := m.Reserve(ctx, inventory.ReserveRequest{
		ItemID:   item.ID,
		Quantity: 3,
		TTL:      15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("giữ hàng: %v", err)
	}

	truoc := doc(t, m, skuID)
	if truoc.Available != 7 || truoc.Reserved != 3 {
		t.Fatalf("dựng sai: %d khả dụng / %d giữ chỗ, cần 7/3",
			truoc.Available, truoc.Reserved)
	}

	// Tám lượt nhả SONG SONG cho CÙNG một reservation.
	const songSong = 8
	var start, done sync.WaitGroup
	start.Add(1)

	var mu sync.Mutex
	var thanhCong int

	for i := 0; i < songSong; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			if err := m.ReleaseReservation(ctx, res.ID); err == nil {
				mu.Lock()
				thanhCong++
				mu.Unlock()
			}
		}()
	}
	start.Done()
	done.Wait()

	if thanhCong != 1 {
		t.Errorf("%d lượt nhả cùng thành công, cần đúng 1", thanhCong)
	}

	// Con số phải TRỞ VỀ ĐÚNG trạng thái trước khi giữ, không hơn.
	sau := doc(t, m, skuID)
	if sau.Available != 10 {
		t.Errorf("còn %d khả dụng, cần 10 — nhả nhiều lần đã SINH RA hàng",
			sau.Available)
	}
	if sau.Reserved != 0 {
		t.Errorf("còn %d giữ chỗ, cần 0", sau.Reserved)
	}

	// Tổng số lượng vật lý không được đổi vì nhả hàng.
	if sau.Total != truoc.Total {
		t.Errorf("tổng số lượng đổi %d → %d — nhả hàng không được tạo hay hủy hàng",
			truoc.Total, sau.Total)
	}
}

func doc(t *testing.T, m *inventory.Module, skuID ids.ID) inventory.ItemView {
	t.Helper()
	found, err := m.GetItemsBySKUs(context.Background(),
		[]string{skuID.String()}, "")
	if err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	items := found[skuID.String()]
	if len(items) != 1 {
		t.Fatalf("có %d bản ghi tồn kho, cần 1", len(items))
	}
	return items[0]
}
