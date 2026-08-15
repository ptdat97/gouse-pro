package postgres_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory/domain"
	"github.com/fashion-commerce/platform/internal/modules/inventory/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

var testNow = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(testdb.DSN(t))
	if err != nil {
		t.Fatalf("DSN không hợp lệ: %v", err)
	}
	// Đủ kết nối cho test tranh chấp: ít hơn số goroutine thì chúng xếp
	// hàng chờ kết nối và không thật sự chạy song song.
	cfg.MaxConns = 30

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("kết nối database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	return pool
}

func cleanInventory(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	// inventory_movement có trigger chặn DELETE — dùng TRUNCATE, thao tác
	// DDL nên trigger DML không chặn được.
	for _, stmt := range []string{
		"TRUNCATE inventory_movement CASCADE",
		"DELETE FROM reservation",
		"DELETE FROM inventory_item",
		"DELETE FROM stock_location",
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu (%s): %v", stmt, err)
		}
	}
}

// seedItem tạo một địa điểm và một bản ghi tồn kho với số lượng cho trước.
func seedItem(t *testing.T, pool *pgxpool.Pool, available int) *domain.InventoryItem {
	t.Helper()
	ctx := context.Background()

	locID := ids.MustNew(ids.PrefixStockLocation)
	if _, err := pool.Exec(ctx, `
		INSERT INTO stock_location (id, name, code, kind, created_at, updated_at)
		VALUES ($1, 'Kho chính', $2, 'PLATFORM', $3, $3)`,
		locID.String(), "KHO-"+string(locID[len(locID)-6:]), testNow); err != nil {
		t.Fatalf("tạo địa điểm: %v", err)
	}

	item, err := domain.NewInventoryItem(domain.NewItemParams{
		SKUID:      ids.MustNew(ids.PrefixSKU),
		LocationID: locID,
		OwnerID:    domain.PlatformOwner,
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewInventoryItem: %v", err)
	}

	store := postgres.NewItemStore(pool)
	if err := store.Create(ctx, item); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if available > 0 {
		if err := item.Receive(available, testNow); err != nil {
			t.Fatalf("Receive: %v", err)
		}
		if err := store.ApplyChange(ctx, item, item.Version()); err != nil {
			t.Fatalf("ApplyChange: %v", err)
		}
	}

	got, err := store.FindByID(ctx, item.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	return got
}

// ĐÂY LÀ TEST QUAN TRỌNG NHẤT CỦA TOÀN BỘ MODULE.
//
// Kịch bản: còn ĐÚNG 1 sản phẩm, N khách bấm mua CÙNG LÚC.
//
// Nếu khóa lạc quan sai, kết quả là bán 2 sản phẩm khi chỉ có 1 — lỗi
// nghiêm trọng nhất của một sàn thương mại điện tử: khách trả tiền rồi
// mới biết không có hàng.
//
// Test này KHÔNG kiểm chứng được bằng kho in-memory: nó cần hai giao dịch
// database thật chạy song song trên cùng một dòng.
func TestKhongBanQuaSoLuongKhiNhieuKhachMuaCungLuc(t *testing.T) {
	pool := newPool(t)
	cleanInventory(t, pool)
	ctx := context.Background()

	const soKhach = 20
	item := seedItem(t, pool, 1) // CHỈ CÒN 1 SẢN PHẨM
	store := postgres.NewItemStore(pool)

	var (
		thanhCong atomic.Int64
		xungDot   atomic.Int64
		hetHang   atomic.Int64
		batDau    = make(chan struct{})
		wg        sync.WaitGroup
	)

	// Mọi khách ĐỌC TRƯỚC, ghi SAU — ép tất cả cùng nhìn thấy "còn 1".
	//
	// Không làm vậy thì goroutine chạy sau đọc được trạng thái đã cập nhật,
	// thấy hết hàng và dừng lại — khóa lạc quan không hề bị thử thách, và
	// test vẫn xanh kể cả khi khóa bị vô hiệu.
	snapshots := make([]*domain.InventoryItem, soKhach)
	for i := range snapshots {
		cur, err := store.FindByID(ctx, item.ID())
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		snapshots[i] = cur
	}

	for i := 0; i < soKhach; i++ {
		wg.Add(1)
		go func(cur *domain.InventoryItem) {
			defer wg.Done()
			<-batDau // chờ tất cả sẵn sàng rồi cùng lao vào

			if err := cur.Reserve(1, testNow); err != nil {
				// Hết hàng: KHÔNG thử lại (mục 5.4).
				if errors.Is(err, domain.ErrInsufficientStock) {
					hetHang.Add(1)
					return
				}
				t.Errorf("Reserve: %v", err)
				return
			}

			switch err := store.ApplyChange(ctx, cur, cur.Version()); {
			case err == nil:
				thanhCong.Add(1)
			case errors.Is(err, domain.ErrVersionConflict):
				xungDot.Add(1)
			default:
				t.Errorf("ApplyChange: %v", err)
			}
		}(snapshots[i])
	}

	close(batDau)
	wg.Wait()

	// CHỈ MỘT khách được giữ hàng.
	if got := thanhCong.Load(); got != 1 {
		t.Errorf("số khách giữ được hàng = %d, mong ĐÚNG 1 — đã bán quá số lượng!", got)
	}

	// Trạng thái cuối phải nhất quán: 0 khả dụng, 1 đang giữ.
	final, err := store.FindByID(ctx, item.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	q := final.Quantities()
	if q.Available() != 0 || q.Reserved() != 1 {
		t.Errorf("cuối cùng: available=%d reserved=%d, mong 0 và 1", q.Available(), q.Reserved())
	}
	// Bất biến cốt lõi: tổng không đổi.
	if q.Total() != 1 {
		t.Errorf("tổng = %d, mong 1 — bất biến bị phá", q.Total())
	}

	t.Logf("trong %d khách: %d thành công, %d xung đột phiên bản, %d thấy hết hàng",
		soKhach, thanhCong.Load(), xungDot.Load(), hetHang.Load())
}

// Nhiều hàng, nhiều khách: tổng số bán ra KHÔNG được vượt số lượng có.
//
// Khác test trên ở chỗ có nhiều người thắng — kiểm chứng rằng khóa lạc
// quan không chỉ đúng ở trường hợp biên "còn 1".
func TestTongBanRaKhongVuotTonKho(t *testing.T) {
	pool := newPool(t)
	cleanInventory(t, pool)
	ctx := context.Background()

	const (
		tonKho  = 10
		soKhach = 30
	)
	item := seedItem(t, pool, tonKho)
	store := postgres.NewItemStore(pool)

	var (
		daGiu  atomic.Int64
		batDau = make(chan struct{})
		wg     sync.WaitGroup
	)

	for i := 0; i < soKhach; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-batDau

			// Thử lại tối đa 3 lần khi xung đột phiên bản (mục 5.4).
			for lan := 0; lan < 3; lan++ {
				cur, err := store.FindByID(ctx, item.ID())
				if err != nil {
					t.Errorf("FindByID: %v", err)
					return
				}
				if err := cur.Reserve(1, testNow); err != nil {
					// Hết hàng — KHÔNG thử lại.
					return
				}
				err = store.ApplyChange(ctx, cur, cur.Version())
				if err == nil {
					daGiu.Add(1)
					return
				}
				if !errors.Is(err, domain.ErrVersionConflict) {
					t.Errorf("ApplyChange: %v", err)
					return
				}
				// Xung đột: thử lại.
			}
		}()
	}

	close(batDau)
	wg.Wait()

	got := daGiu.Load()
	if got > tonKho {
		t.Errorf("giữ được %d, vượt tồn kho %d — ĐÃ BÁN QUÁ SỐ LƯỢNG", got, tonKho)
	}

	final, err := store.FindByID(ctx, item.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	q := final.Quantities()
	if int64(q.Reserved()) != got {
		t.Errorf("reserved = %d, mong %d", q.Reserved(), got)
	}
	if q.Available()+q.Reserved() != tonKho {
		t.Errorf("available+reserved = %d, mong %d — bất biến bị phá",
			q.Available()+q.Reserved(), tonKho)
	}

	t.Logf("tồn kho %d, %d khách tranh mua, giữ được %d, còn lại %d",
		tonKho, soKhach, got, q.Available())
}

// Ràng buộc CHECK ở database là LỚP BẢO VỆ CUỐI CÙNG.
//
// Kể cả khi có lỗi logic ở tầng ứng dụng, database vẫn từ chối số âm.
// Chỉ báo "số SKU có tồn kho âm" phải LUÔN bằng 0 (mục 13).
func TestDatabaseTuChoiSoLuongAm(t *testing.T) {
	pool := newPool(t)
	cleanInventory(t, pool)
	ctx := context.Background()

	item := seedItem(t, pool, 5)

	// Ghi thẳng SQL, vòng qua mọi kiểm tra của domain — mô phỏng lỗi logic
	// hoặc ai đó sửa tay database.
	_, err := pool.Exec(ctx,
		`UPDATE inventory_item SET quantity_available = -1 WHERE id = $1`,
		item.ID().String())
	if err == nil {
		t.Error("database CHO PHÉP số lượng âm — ràng buộc CHECK không hoạt động")
	}
}

// Khóa định danh nghiệp vụ: một bản ghi cho mỗi (sku, địa điểm, chủ sở hữu).
//
// Hai bản ghi cho cùng tổ hợp làm tổng tồn kho tính ra sai mà không ai biết.
func TestKhongTrungKhoaDinhDanhNghiepVu(t *testing.T) {
	pool := newPool(t)
	cleanInventory(t, pool)
	ctx := context.Background()

	item := seedItem(t, pool, 0)
	store := postgres.NewItemStore(pool)

	trung, err := domain.NewInventoryItem(domain.NewItemParams{
		SKUID:      item.SKUID(),
		LocationID: item.LocationID(),
		OwnerID:    item.OwnerID(),
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewInventoryItem: %v", err)
	}

	if err := store.Create(ctx, trung); !errors.Is(err, domain.ErrDuplicateItem) {
		t.Errorf("lỗi = %v, mong ErrDuplicateItem", err)
	}
}

// Cùng SKU nhưng KHÁC chủ sở hữu phải là hai bản ghi riêng.
//
// Đây là điểm mấu chốt của mô hình "nền tảng giao hộ": hàng của seller
// nằm ở kho nền tảng vẫn thuộc sở hữu seller, không được gộp vào tài sản
// của nền tảng.
func TestCungSKUKhacChuSoHuuLaHaiBanGhiRieng(t *testing.T) {
	pool := newPool(t)
	cleanInventory(t, pool)
	ctx := context.Background()

	cuaNenTang := seedItem(t, pool, 10)
	store := postgres.NewItemStore(pool)

	sellerID := ids.MustNew(ids.PrefixSeller)
	cuaSeller, err := domain.NewInventoryItem(domain.NewItemParams{
		SKUID:      cuaNenTang.SKUID(),      // CÙNG SKU
		LocationID: cuaNenTang.LocationID(), // CÙNG kho
		OwnerID:    sellerID,                // KHÁC chủ sở hữu
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("NewInventoryItem: %v", err)
	}
	if err := store.Create(ctx, cuaSeller); err != nil {
		t.Fatalf("hàng seller gửi kho nền tảng phải tạo được bản ghi riêng: %v", err)
	}

	// Tra theo SKU phải ra CẢ HAI bản ghi.
	found, err := store.FindBySKUs(ctx, []ids.ID{cuaNenTang.SKUID()}, "")
	if err != nil {
		t.Fatalf("FindBySKUs: %v", err)
	}
	if len(found[cuaNenTang.SKUID()]) != 2 {
		t.Fatalf("số bản ghi = %d, mong 2", len(found[cuaNenTang.SKUID()]))
	}

	// Và phân biệt được chủ sở hữu.
	var coNenTang, coSeller bool
	for _, it := range found[cuaNenTang.SKUID()] {
		if it.IsPlatformOwned() {
			coNenTang = true
		} else if it.OwnerID() == sellerID {
			coSeller = true
		}
	}
	if !coNenTang || !coSeller {
		t.Error("không phân biệt được hàng nền tảng với hàng seller")
	}
}
