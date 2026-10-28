package inventory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
)

// TestDoubleReleaseCreatesNoStockFromNothing — lỗi tồn kho nghiêm trọng.
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
// `ReservationStore.Save` từng là upsert KHÔNG ĐIỀU KIỆN.
//
// Chi tiết quyết định nằm ở con số 77 rồi 78: lượt nhả thứ hai ĐỌC ĐƯỢC
// kết quả của lượt thứ nhất. Nó không phải hai bên cùng đọc một giá trị
// cũ — nếu vậy khóa lạc quan trên `inventory_item` đã chặn. Nó là hai câu
// SELECT của CÙNG một giao dịch nhìn thấy hai thời điểm khác nhau, đúng
// theo cách READ COMMITTED làm việc.
//
// Cơ chế đầy đủ và cách dựng lại nó một cách XÁC ĐỊNH nằm ở
// nha_hai_lan_test.go. Bài đó chặn đúng khe giữa hai lần đọc bằng đồng
// hồ tiêm vào, nên không phụ thuộc may rủi.
//
// Hai đường nhả CÓ THẬT chạy song song trong hệ thống: `releaseAll` khi
// phiên thanh toán thất bại, và job dọn giữ hàng quá hạn.
func TestDoubleReleaseCreatesNoStockFromNothing(t *testing.T) {
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

	before := readItem(t, m, skuID)
	if before.Available != 7 || before.Reserved != 3 {
		t.Fatalf("dựng sai: %d khả dụng / %d giữ chỗ, cần 7/3",
			before.Available, before.Reserved)
	}

	// Tám lượt nhả SONG SONG cho CÙNG một reservation.
	const songSong = 8
	var start, done sync.WaitGroup
	start.Add(1)

	var mu sync.Mutex
	var succeeded int

	for i := 0; i < songSong; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			if err := m.ReleaseReservation(ctx, res.ID); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	start.Done()
	done.Wait()

	if succeeded != 1 {
		t.Errorf("%d lượt nhả cùng thành công, cần đúng 1", succeeded)
	}

	// Con số phải TRỞ VỀ ĐÚNG trạng thái trước khi giữ, không hơn.
	after := readItem(t, m, skuID)
	if after.Available != 10 {
		t.Errorf("còn %d khả dụng, cần 10 — nhả nhiều lần đã SINH RA hàng",
			after.Available)
	}
	if after.Reserved != 0 {
		t.Errorf("còn %d giữ chỗ, cần 0", after.Reserved)
	}

	// Tổng số lượng vật lý không được đổi vì nhả hàng.
	if after.Total != before.Total {
		t.Errorf("tổng số lượng đổi %d → %d — nhả hàng không được tạo hay hủy hàng",
			before.Total, after.Total)
	}
}

func readItem(t *testing.T, m *inventory.Module, skuID ids.ID) inventory.ItemView {
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
