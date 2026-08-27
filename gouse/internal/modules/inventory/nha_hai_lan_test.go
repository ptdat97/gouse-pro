package inventory_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// dongHoChan là đồng hồ thật, nhưng DỪNG ở lần gọi đầu tiên.
//
// # Vì sao đây là điểm chèn hợp lệ, không phải mẹo test
//
// `releaseWith` gọi `s.clock.Now()` ĐÚNG giữa hai lần đọc:
//
//	reservation := r.Reservations.FindByID(...)   ← đọc 1
//	now := s.clock.Now()                          ← chỗ này
//	finish(reservation, now)
//	item := r.Items.FindByID(...)                 ← đọc 2
//
// Clock vốn đã là cổng tiêm sẵn có của module (`inventory.Config.Clock`),
// không phải thứ thêm vào để test dễ. Chặn ở đây cho ta điều khiển được
// đúng cái xen kẽ cần dựng lại, mà không sửa một dòng code production nào.
type dongHoChan struct {
	mu     sync.Mutex
	daChan bool
	toiRoi chan struct{} // đóng khi đã vào giữa hai lần đọc
	diTiep chan struct{} // bên test đóng để cho đi tiếp
}

func newDongHoChan() *dongHoChan {
	return &dongHoChan{
		toiRoi: make(chan struct{}),
		diTiep: make(chan struct{}),
	}
}

func (d *dongHoChan) Now() time.Time {
	d.mu.Lock()
	lanDau := !d.daChan
	d.daChan = true
	d.mu.Unlock()

	// CHỈ chặn lần đầu. `withRetry` chạy lại toàn bộ closure khi xung đột
	// phiên bản, và lần chạy lại phải đi thẳng — nếu không, bài test treo
	// thay vì đo được điều nó muốn đo.
	if lanDau {
		close(d.toiRoi)
		<-d.diTiep
	}
	return time.Now().UTC()
}

// TestNhaGiuHangHaiLan_TaiHienXacDinh dựng lại ĐÚNG cơ chế của PH-31.
//
// # Vì sao ba lần thử tái hiện trước đều xanh
//
// Chúng bắn nhiều lượt nhả cùng lúc và trông đợi một cuộc đua kinh điển:
// hai bên cùng đọc một giá trị rồi cùng ghi đè. Nhưng nhật ký production
// nói KHÁC:
//
//	RESERVE  → còn 76
//	RELEASE  → còn 77
//	RELEASE  → còn 78     ← đọc được KẾT QUẢ của lượt trước
//
// Lượt nhả thứ hai thấy 77, tức nó đọc bảng tồn kho SAU khi lượt thứ nhất
// đã commit. Đó không phải hai bên cùng đọc một giá trị cũ — đó là hai
// bên đọc ở HAI THỜI ĐIỂM KHÁC NHAU, và chính vì thế khóa lạc quan trên
// `inventory_item` không bắt được: phiên bản mà lượt hai đọc là phiên bản
// mới nhất, nên câu UPDATE của nó hợp lệ.
//
// # Cơ chế thật
//
// PostgreSQL chạy READ COMMITTED: MỖI CÂU LỆNH trong một giao dịch lấy
// một ảnh chụp MỚI. Một giao dịch đọc hai bảng bằng hai câu lệnh có thể
// thấy hai thời điểm khác nhau của thế giới.
//
//	T2: BEGIN
//	T2: SELECT reservation   → ACTIVE          (T1 chưa commit)
//	T1: COMMIT                                 (item 76→77, reservation EXPIRED)
//	T2: kiểm IsFinal() trên bản trong bộ nhớ   → còn ACTIVE → cho qua
//	T2: SELECT item          → 77, phiên bản mới nhất  (thấy việc của T1)
//	T2: UPDATE item WHERE version = <mới nhất> → 1 dòng → 78
//	T2: COMMIT
//
// Cửa sổ chỉ rộng bằng khoảng giữa hai câu SELECT của T2. Bắn tám lượt
// song song hầu như luôn cho mọi lượt đọc reservation gần như đồng thời,
// rồi chúng chặn nhau ở khóa lạc quan của item và thử lại — nên xác suất
// rơi trúng cửa sổ này là rất thấp. Nó xảy ra một lần trong nhiều giờ
// chạy thật.
//
// Bài test này KHÔNG trông chờ vào xác suất: nó dừng T2 đúng giữa hai lần
// đọc bằng đồng hồ, chạy trọn T1, rồi thả T2 ra.
func TestNhaGiuHangHaiLan_TaiHienXacDinh(t *testing.T) {
	mNhanh, db := newModule(t)
	ctx := context.Background()

	dongHo := newDongHoChan()
	mCham, err := inventory.New(inventory.Config{
		Storage: "postgres", DB: db, Clock: dongHo,
	})
	if err != nil {
		t.Fatalf("dựng module thứ hai: %v", err)
	}

	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU)

	item, err := mNhanh.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID.String(), LocationID: locID, Quantity: 10,
		PerformedBy: "test",
	})
	if err != nil {
		t.Fatalf("nhập kho: %v", err)
	}

	// Reservation NẠN NHÂN — cái sẽ bị nhả hai lần.
	res, err := mNhanh.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 1, TTL: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("giữ hàng: %v", err)
	}

	// Reservation NGƯỜI KHÁC, còn sống suốt bài test.
	//
	// Thiếu nó thì bài test xanh vì lý do SAI: nhả xong lượt đầu là
	// `reserved` về 0, và bất biến của chính bản ghi tồn kho chặn lượt
	// thứ hai ("không đủ hàng"). Trong production, SKU đang bán luôn có
	// nhiều lượt giữ cùng lúc, nên `reserved` vẫn dương và lượt nhả thứ
	// hai đi lọt. Đó là lý do lỗi này chỉ xuất hiện trên hệ thống thật.
	if _, err := mNhanh.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 5, TTL: 15 * time.Minute,
	}); err != nil {
		t.Fatalf("giữ hàng của người khác: %v", err)
	}

	truoc := doTonKho(t, db, item.ID)
	if truoc.khaDung != 4 || truoc.dangGiu != 6 {
		t.Fatalf("trạng thái ban đầu sai: khả dụng %d, đang giữ %d",
			truoc.khaDung, truoc.dangGiu)
	}

	// T2 — dừng lại ngay sau khi đọc reservation.
	loiT2 := make(chan error, 1)
	go func() {
		loiT2 <- mCham.ReleaseReservation(ctx, res.ID)
	}()

	select {
	case <-dongHo.toiRoi:
	case <-time.After(5 * time.Second):
		t.Fatal("T2 không tới được chỗ chặn — đường code đã đổi?")
	}

	// T1 — chạy trọn vẹn và commit trong lúc T2 đang dừng.
	if err := mNhanh.ReleaseReservation(ctx, res.ID); err != nil {
		t.Fatalf("T1 nhả hàng: %v", err)
	}

	giua := doTonKho(t, db, item.ID)
	if giua.khaDung != 5 || giua.dangGiu != 5 {
		t.Fatalf("sau T1 phải là 5/5, đang là %d/%d",
			giua.khaDung, giua.dangGiu)
	}

	// Thả T2. Bản reservation trong bộ nhớ của nó vẫn ghi ACTIVE.
	close(dongHo.diTiep)

	var errT2 error
	select {
	case errT2 = <-loiT2:
	case <-time.After(10 * time.Second):
		t.Fatal("T2 không kết thúc")
	}

	// ---------------------------------------------------------------

	sau := doTonKho(t, db, item.ID)

	// Bất biến: hàng khả dụng = tổng nhập − tổng đang giữ CÒN SỐNG.
	// Reservation của "người khác" vẫn giữ 5, nên khả dụng phải đúng 5.
	if sau.khaDung != 5 {
		t.Errorf("HÀNG SINH TỪ KHÔNG KHÍ: khả dụng %d, cần 5 — "+
			"lượt giữ của người khác vẫn ôm 5 trong tổng 10", sau.khaDung)
	}
	if sau.dangGiu != 5 {
		t.Errorf("đang giữ %d, cần 5 — con số này phải khớp lượt giữ còn sống",
			sau.dangGiu)
	}

	if n := demNhaCua(t, db, res.ID); n != 1 {
		t.Errorf("reservation có %d biến động RELEASE, cần đúng 1", n)
	}

	// T2 phải BÁO LỖI, không được im lặng coi như thành công: bên gọi cần
	// biết việc nhả này đã có người khác làm.
	if errT2 == nil {
		t.Error("T2 báo thành công cho một lần nhả KHÔNG xảy ra")
	} else if !errors.Is(errT2, inventory.ErrConflict) &&
		!errors.Is(errT2, inventory.ErrNotFound) {
		t.Logf("T2 trả: %v", errT2)
	}
}

type tonKho struct{ khaDung, dangGiu int }

func doTonKho(t *testing.T, db *database.DB, itemID string) tonKho {
	t.Helper()
	var tk tonKho
	err := db.Pool().QueryRow(context.Background(),
		`SELECT quantity_available, quantity_reserved
		   FROM inventory_item WHERE id = $1`, itemID).
		Scan(&tk.khaDung, &tk.dangGiu)
	if err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	return tk
}

func demNhaCua(t *testing.T, db *database.DB, reservationID string) int {
	t.Helper()
	var n int
	err := db.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM inventory_movement
		  WHERE reference_id = $1 AND movement_type = 'RELEASE'`,
		reservationID).Scan(&n)
	if err != nil {
		t.Fatalf("đếm biến động: %v", err)
	}
	return n
}

// TestChotVaHetHanChongNhau_TaiHienXacDinh — cùng lớp lỗi với PH-31.
//
// `commitWith` có ĐÚNG hình dạng của `releaseWith`: đọc reservation, kiểm
// trạng thái trong bộ nhớ, đọc tồn kho, ghi cả hai. Nên nó dính đúng cái
// khe hở READ COMMITTED ấy, chỉ khác hậu quả.
//
// Ở đây job dọn hạn thắng cuộc đua, nhưng lượt chốt đơn vẫn giữ bản
// reservation ACTIVE trong bộ nhớ và đi tiếp. Nếu lọt, hệ thống chốt hàng
// cho một đơn trong khi số giữ chỗ ấy VỪA bị trả về kho — hàng được bán
// hai lần.
//
// Bài này tồn tại vì sửa xong một chỗ mà không kiểm chỗ có cùng hình dạng
// thì mới chỉ sửa được triệu chứng.
func TestChotVaHetHanChongNhau_TaiHienXacDinh(t *testing.T) {
	mNhanh, db := newModule(t)
	ctx := context.Background()

	dongHo := newDongHoChan()
	mCham, err := inventory.New(inventory.Config{
		Storage: "postgres", DB: db, Clock: dongHo,
	})
	if err != nil {
		t.Fatalf("dựng module thứ hai: %v", err)
	}

	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU)

	item, err := mNhanh.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID.String(), LocationID: locID, Quantity: 10,
		PerformedBy: "test",
	})
	if err != nil {
		t.Fatalf("nhập kho: %v", err)
	}

	res, err := mNhanh.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 1, TTL: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("giữ hàng: %v", err)
	}
	if _, err := mNhanh.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 5, TTL: 15 * time.Minute,
	}); err != nil {
		t.Fatalf("giữ hàng của người khác: %v", err)
	}

	// T2 — chốt đơn, dừng ngay sau khi đọc reservation.
	loiT2 := make(chan error, 1)
	go func() {
		loiT2 <- mCham.Commit(ctx, res.ID)
	}()

	select {
	case <-dongHo.toiRoi:
	case <-time.After(5 * time.Second):
		t.Fatal("T2 không tới được chỗ chặn — đường code đã đổi?")
	}

	// T1 — job dọn hạn nhả chính reservation đó, và commit.
	if err := mNhanh.ReleaseReservation(ctx, res.ID); err != nil {
		t.Fatalf("T1 nhả hàng: %v", err)
	}

	close(dongHo.diTiep)

	var errT2 error
	select {
	case errT2 = <-loiT2:
	case <-time.After(10 * time.Second):
		t.Fatal("T2 không kết thúc")
	}

	sau := doTonKho(t, db, item.ID)

	// Reservation của "người khác" giữ 5. Không gì được chốt.
	if sau.khaDung != 5 || sau.dangGiu != 5 {
		t.Errorf("tồn kho là %d/%d, cần 5/5", sau.khaDung, sau.dangGiu)
	}
	if n := demChotCua(t, db, res.ID); n != 0 {
		t.Errorf("có %d biến động COMMIT cho reservation ĐÃ NHẢ, cần 0 — "+
			"hàng vừa trả về kho lại bị bán", n)
	}
	if errT2 == nil {
		t.Error("T2 báo chốt đơn THÀNH CÔNG trên hàng đã bị nhả")
	} else {
		t.Logf("T2 trả: %v", errT2)
	}
}

func demChotCua(t *testing.T, db *database.DB, reservationID string) int {
	t.Helper()
	var n int
	err := db.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM inventory_movement
		  WHERE reference_id = $1 AND movement_type = 'COMMIT'`,
		reservationID).Scan(&n)
	if err != nil {
		t.Fatalf("đếm biến động: %v", err)
	}
	return n
}
