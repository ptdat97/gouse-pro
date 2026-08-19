package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

func newModule(t *testing.T) (*inventory.Module, *database.DB) {
	t.Helper()

	db := testdb.Open(t)

	// Dọn dữ liệu để mỗi test bắt đầu từ trạng thái sạch.
	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE inventory_movement CASCADE",
		"DELETE FROM reservation",
		"DELETE FROM inventory_item",
		"DELETE FROM stock_location",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	m, err := inventory.New(inventory.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("inventory.New: %v", err)
	}
	return m, db
}

func newLocation(t *testing.T, db *database.DB) string {
	t.Helper()
	locID := ids.MustNew(ids.PrefixStockLocation)
	_, err := db.Pool().Exec(context.Background(), `
		INSERT INTO stock_location (id, name, code, kind, created_at, updated_at)
		VALUES ($1, 'Kho chính', $2, 'PLATFORM', now(), now())`,
		locID.String(), "KHO-"+string(locID[len(locID)-6:]))
	if err != nil {
		t.Fatalf("tạo địa điểm: %v", err)
	}
	return locID.String()
}

// Module này CHỈ chạy với PostgreSQL — không có bản in-memory.
//
// Đây là quyết định có chủ đích: khóa lạc quan không kiểm chứng được bằng
// bộ nhớ, và một bản in-memory sẽ tạo cảm giác an toàn giả.
func TestChiHoTroPostgres(t *testing.T) {
	if _, err := inventory.New(inventory.Config{Storage: "memory"}); err == nil {
		t.Error("mong lỗi khi yêu cầu kho in-memory")
	}
	if _, err := inventory.New(inventory.Config{Storage: "postgres"}); err == nil {
		t.Error("mong lỗi khi thiếu kết nối database")
	}
}

// VÒNG ĐỜI ĐẦY ĐỦ: nhập → giữ → cam kết → xuất.
//
// Đây là đường đi của mọi đơn hàng thật.
func TestVongDoiDayDuTuNhapDenXuat(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	// 1. Nhập 10 sản phẩm.
	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: 10,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if item.Available != 10 || item.Total != 10 {
		t.Fatalf("sau nhập: available=%d total=%d, mong 10 và 10", item.Available, item.Total)
	}
	if !item.IsPlatformOwned {
		t.Error("không truyền OwnerID thì phải là hàng của nền tảng")
	}

	// 2. Khách vào checkout, giữ 3 sản phẩm.
	res, err := m.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 3,
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if res.Status != "ACTIVE" {
		t.Errorf("trạng thái = %q, mong ACTIVE", res.Status)
	}
	// Reservation PHẢI có thời hạn (quy tắc 5).
	if _, err := time.Parse(time.RFC3339, res.ExpiresAt); err != nil {
		t.Errorf("ExpiresAt %q không đúng RFC3339", res.ExpiresAt)
	}

	// Hàng đang giữ KHÔNG còn bán được cho khách khác.
	av, err := m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	if av[skuID] != 7 {
		t.Errorf("khả dụng = %d, mong 7 (10 − 3 đang giữ)", av[skuID])
	}

	// 3. Thanh toán thành công → cam kết.
	if err := m.Commit(ctx, res.ID); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// 4. Xuất hàng.
	if err := m.Ship(ctx, item.ID, 3, ""); err != nil {
		t.Fatalf("Ship: %v", err)
	}

	// Hàng đã xuất RỜI KHỎI tồn kho: tổng giảm còn 7.
	after, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: 1,
	})
	if err != nil {
		t.Fatalf("Receive lần 2: %v", err)
	}
	if after.Total != 8 {
		t.Errorf("tổng = %d, mong 8 (10 − 3 đã xuất + 1 nhập thêm)", after.Total)
	}
	if after.Committed != 0 {
		t.Errorf("committed = %d, mong 0 sau khi xuất", after.Committed)
	}
}

// Hủy checkout thì hàng phải QUAY LẠI kệ ngay.
func TestHuyCheckoutTraHangVeKe(t *testing.T) {
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

	res, err := m.Reserve(ctx, inventory.ReserveRequest{ItemID: item.ID, Quantity: 2})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	if err := m.ReleaseReservation(ctx, res.ID); err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}

	av, err := m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	if av[skuID] != 5 {
		t.Errorf("khả dụng = %d, mong 5 — hàng chưa quay lại kệ", av[skuID])
	}

	// Giải phóng hai lần phải bị chặn: nếu không, hàng bị cộng lên hai lần
	// và tồn kho ảo nhiều hơn thực tế.
	if err := m.ReleaseReservation(ctx, res.ID); err == nil {
		t.Error("giải phóng lần hai phải bị chặn")
	}
}

// Không đủ hàng phải trả ErrInsufficientStock — KHÔNG phải lỗi chung.
//
// Bên gọi cần phân biệt: hết hàng thì KHÔNG thử lại (hàng không tự xuất
// hiện), còn xung đột thì NÊN thử lại.
func TestHetHangTraLoiRieng(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: 2,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	_, err = m.Reserve(ctx, inventory.ReserveRequest{ItemID: item.ID, Quantity: 5})
	if !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Errorf("lỗi = %v, mong ErrInsufficientStock", err)
	}
}

// Kiểm tra cả giỏ: phải chỉ ra TỪNG món thiếu, không chỉ true/false.
func TestKiemTraGioHangChiRaTungMonThieu(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)

	duHang := ids.MustNew(ids.PrefixSKU).String()
	thieuHang := ids.MustNew(ids.PrefixSKU).String()

	if _, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: duHang, LocationID: locID, Quantity: 10,
	}); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if _, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: thieuHang, LocationID: locID, Quantity: 1,
	}); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	got, err := m.CheckAvailability(ctx, []inventory.AvailabilityRequest{
		{SKUID: duHang, Quantity: 2, LocationID: locID},
		{SKUID: thieuHang, Quantity: 5, LocationID: locID},
	})
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}

	if got.AllAvailable {
		t.Error("giỏ có món thiếu mà báo đủ hàng")
	}
	if len(got.Insufficient) != 1 {
		t.Fatalf("số món thiếu = %d, mong 1", len(got.Insufficient))
	}
	// "Chỉ còn 1 sản phẩm" hữu ích hơn nhiều so với "hết hàng".
	if got.Insufficient[0].Available != 1 || got.Insufficient[0].Requested != 5 {
		t.Errorf("chi tiết món thiếu sai: %+v", got.Insufficient[0])
	}
}

// QUY TẮC 3: hàng hoàn KHÔNG BAO GIỜ tự động vào Available.
func TestHangHoanPhaiQuaKiemDinh(t *testing.T) {
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

	// Khách trả 3 món về.
	if err := m.ReceiveReturn(ctx, item.ID, 3, ""); err != nil {
		t.Fatalf("ReceiveReturn: %v", err)
	}

	// Hàng hoàn KHÔNG được cộng vào khả dụng.
	av, err := m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	if av[skuID] != 5 {
		t.Errorf("khả dụng = %d, mong 5 — hàng hoàn đã lọt vào kệ mà chưa kiểm định", av[skuID])
	}

	// Kiểm định: 2 món đạt, 1 món hỏng.
	if err := m.ProcessReturnInspection(ctx, inventory.InspectionRequest{
		ItemID: item.ID, Quantity: 2, Passed: true,
	}); err != nil {
		t.Fatalf("ProcessReturnInspection (đạt): %v", err)
	}
	if err := m.ProcessReturnInspection(ctx, inventory.InspectionRequest{
		ItemID: item.ID, Quantity: 1, Passed: false,
		Reason: "Vết bẩn không xử lý được", PerformedBy: ids.MustNew(ids.PrefixUser).String(),
	}); err != nil {
		t.Fatalf("ProcessReturnInspection (hỏng): %v", err)
	}

	av, err = m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	// Chỉ 2 món đạt kiểm định mới vào kệ.
	if av[skuID] != 7 {
		t.Errorf("khả dụng = %d, mong 7 (5 + 2 đạt kiểm định)", av[skuID])
	}
}

// QUY TẮC 7: điều chỉnh thủ công PHẢI có lý do và người thực hiện.
//
// Điều chỉnh không lý do là điểm mù trong kiểm toán — không phân biệt được
// sai sót kiểm kê với thất thoát.
func TestDieuChinhThuCongBatBuocCoLyDo(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: 10,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	nguoiKiemKe := ids.MustNew(ids.PrefixUser).String()

	// Thiếu lý do → bị chặn.
	if err := m.Adjust(ctx, inventory.AdjustRequest{
		ItemID: item.ID, Delta: -2, PerformedBy: nguoiKiemKe,
	}); err == nil {
		t.Error("điều chỉnh không lý do phải bị chặn")
	}

	// Thiếu người thực hiện → bị chặn.
	if err := m.Adjust(ctx, inventory.AdjustRequest{
		ItemID: item.ID, Delta: -2, Reason: "Kiểm kê tháng 8",
	}); err == nil {
		t.Error("điều chỉnh không ghi người thực hiện phải bị chặn")
	}

	// Đủ thông tin → chấp nhận.
	if err := m.Adjust(ctx, inventory.AdjustRequest{
		ItemID: item.ID, Delta: -2,
		Reason: "Kiểm kê tháng 8: thiếu 2 sản phẩm", PerformedBy: nguoiKiemKe,
	}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}

	av, err := m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	if av[skuID] != 8 {
		t.Errorf("khả dụng = %d, mong 8", av[skuID])
	}
}

// QUY TẮC 4: mọi biến động phải ghi vào nhật ký.
//
// Nhật ký cho phép tái dựng trạng thái và điều tra sai lệch kiểm kê.
func TestMoiBienDongDeuGhiNhatKy(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: 10,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	res, err := m.Reserve(ctx, inventory.ReserveRequest{ItemID: item.ID, Quantity: 3})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := m.Commit(ctx, res.ID); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	movements, err := m.Service().GetMovements(ctx, ids.ID(item.ID), 100)
	if err != nil {
		t.Fatalf("GetMovements: %v", err)
	}
	// Ba biến động: RECEIVE, RESERVE, COMMIT.
	if len(movements) != 3 {
		t.Fatalf("số dòng nhật ký = %d, mong 3", len(movements))
	}

	// Nhật ký phải ghi số lượng SAU biến động, để đối chiếu được.
	for _, mv := range movements {
		if mv.QuantityAfter() < 0 {
			t.Errorf("quantityAfter âm: %d", mv.QuantityAfter())
		}
		if mv.Quantity() <= 0 {
			t.Errorf("quantity phải dương (hướng nằm ở loại biến động): %d", mv.Quantity())
		}
	}
}

// Nhật ký biến động BẤT BIẾN — database từ chối sửa/xóa.
func TestNhatKyBienDongBatBien(t *testing.T) {
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

	movements, err := m.Service().GetMovements(ctx, ids.ID(item.ID), 10)
	if err != nil || len(movements) == 0 {
		t.Fatalf("GetMovements: %v (số dòng %d)", err, len(movements))
	}
	mvID := movements[0].ID().String()

	if _, err := db.Pool().Exec(ctx,
		`UPDATE inventory_movement SET quantity = 999 WHERE id = $1`, mvID); err == nil {
		t.Error("UPDATE nhật ký thành công — nhật ký KHÔNG bất biến")
	}
	if _, err := db.Pool().Exec(ctx,
		`DELETE FROM inventory_movement WHERE id = $1`, mvID); err == nil {
		t.Error("DELETE nhật ký thành công — nhật ký KHÔNG bất biến")
	}
}

// Cơ chế hết hạn phải ĐÁNG TIN CẬY: nếu ngừng chạy, hàng bị khóa dần và
// cuối cùng không bán được gì (mục 6.3).
func TestDonReservationQuaHan(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()
	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, Quantity: 10,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	// TTL rất ngắn để test không phải chờ.
	res, err := m.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 4, TTL: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	_ = res

	time.Sleep(10 * time.Millisecond)

	// Chỉ báo giám sát phải thấy bản ghi quá hạn.
	pending, err := m.Service().CountExpiredPending(ctx)
	if err != nil {
		t.Fatalf("CountExpiredPending: %v", err)
	}
	if pending != 1 {
		t.Errorf("số quá hạn chưa dọn = %d, mong 1", pending)
	}

	daDon, err := m.Service().ExpireReservations(ctx, 100)
	if err != nil {
		t.Fatalf("ExpireReservations: %v", err)
	}
	if daDon != 1 {
		t.Errorf("đã dọn = %d, mong 1", daDon)
	}

	// Hàng phải quay lại kệ.
	av, err := m.GetAvailability(ctx, []string{skuID}, locID)
	if err != nil {
		t.Fatalf("GetAvailability: %v", err)
	}
	if av[skuID] != 10 {
		t.Errorf("khả dụng = %d, mong 10 — hàng quá hạn chưa quay lại kệ", av[skuID])
	}

	// Dọn xong thì không còn tồn đọng.
	pending, err = m.Service().CountExpiredPending(ctx)
	if err != nil {
		t.Fatalf("CountExpiredPending: %v", err)
	}
	if pending != 0 {
		t.Errorf("còn %d bản ghi quá hạn sau khi dọn", pending)
	}
}

// Reservation hết hạn thì KHÔNG cam kết được: hàng có thể đã được giải
// phóng và bán cho khách khác.
func TestKhongCamKetDuocReservationDaHetHan(t *testing.T) {
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

	res, err := m.Reserve(ctx, inventory.ReserveRequest{
		ItemID: item.ID, Quantity: 2, TTL: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if err := m.Commit(ctx, res.ID); err == nil {
		t.Error("cam kết reservation đã hết hạn phải bị chặn")
	}
}

func TestIDSaiDinhDangTraErrInvalidID(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	if _, err := m.Reserve(ctx, inventory.ReserveRequest{
		ItemID: "khong-phai-id", Quantity: 1,
	}); !errors.Is(err, inventory.ErrInvalidID) {
		t.Errorf("lỗi = %v, mong ErrInvalidID", err)
	}
	if err := m.Commit(ctx, "sku_01KZV27T7DZ04AMAPE1A2W60EY"); !errors.Is(err, inventory.ErrInvalidID) {
		t.Errorf("lỗi = %v, mong ErrInvalidID", err)
	}
}

// ---------------------------------------------------- Kiểm kê tồn kho

// KIỂM KÊ đặt số lượng về con số TUYỆT ĐỐI, không phải cộng thêm.
//
// Đó là cách người kiểm kê nghĩ: "đếm được 40 cái", không phải "thêm 15
// cái". Bắt họ tự tính chênh lệch là mời gọi lỗi số học vào một con số
// quyết định bán được bao nhiêu hàng.
func TestKiemKeDatSoLuongTuyetDoi(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()
	seller := ids.MustNew(ids.PrefixSeller).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, OwnerID: seller,
		Quantity: 25, ReferenceID: "nhap-dau", PerformedBy: seller,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	// Đếm được 40 → phải thành 40, không phải 25+40.
	if err := m.SetAvailable(ctx, item.ID, 40,
		"Kiểm kê thực tế cuối tháng", seller); err != nil {
		t.Fatalf("SetAvailable: %v", err)
	}

	var available int
	if err := db.Pool().QueryRow(ctx,
		`SELECT quantity_available FROM inventory_item WHERE id = $1`,
		item.ID).Scan(&available); err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	if available != 40 {
		t.Errorf("khả dụng = %d, mong 40 — con số kiểm kê là TUYỆT ĐỐI", available)
	}

	// Đếm được ÍT hơn cũng phải đúng: mất mát hàng là chuyện có thật.
	if err := m.SetAvailable(ctx, item.ID, 12,
		"Kiểm kê phát hiện thiếu hàng", seller); err != nil {
		t.Fatalf("SetAvailable giảm: %v", err)
	}
	if err := db.Pool().QueryRow(ctx,
		`SELECT quantity_available FROM inventory_item WHERE id = $1`,
		item.ID).Scan(&available); err != nil {
		t.Fatalf("đọc tồn kho: %v", err)
	}
	if available != 12 {
		t.Errorf("khả dụng = %d, mong 12", available)
	}
}

// KIỂM KÊ PHẢI GHI NHẬT KÝ, kèm lý do.
//
// Quy tắc 7 của inventory.md: tồn kho lệch mà không có lý do thì không ai
// đối soát được, và MẤT MÁT trông giống hệt sai sót nhập liệu.
func TestKiemKeGhiNhatKyKemLyDo(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()
	seller := ids.MustNew(ids.PrefixSeller).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, OwnerID: seller,
		Quantity: 25, ReferenceID: "nhap-dau", PerformedBy: seller,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	const reason = "Kiểm kê thực tế cuối tháng"
	if err := m.SetAvailable(ctx, item.ID, 40, reason, seller); err != nil {
		t.Fatalf("SetAvailable: %v", err)
	}

	var (
		mType, gotReason string
		qty              int
	)
	if err := db.Pool().QueryRow(ctx, `
		SELECT movement_type, quantity, reason FROM inventory_movement
		 WHERE inventory_item_id = $1 AND movement_type = 'ADJUST'
		 ORDER BY occurred_at DESC LIMIT 1`, item.ID,
	).Scan(&mType, &qty, &gotReason); err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}

	// Nhật ký lưu CHÊNH LỆCH (25 → 40 là 15), vì đó là thứ đã biến động.
	if qty != 15 {
		t.Errorf("số lượng biến động = %d, mong 15", qty)
	}
	if gotReason != reason {
		t.Errorf("lý do = %q, mong %q", gotReason, reason)
	}
}

// KIỂM KÊ ĐÚNG BẰNG số hiện tại thì KHÔNG ghi nhật ký.
//
// Một dòng "điều chỉnh 0 đơn vị" làm loãng nhật ký mà không nói lên điều
// gì — và nhật ký tồn kho là thứ người ta đọc khi đi tìm hàng thất lạc.
func TestKiemKeKhongDoiThiKhongGhiNhatKy(t *testing.T) {
	m, db := newModule(t)
	ctx := context.Background()

	locID := newLocation(t, db)
	skuID := ids.MustNew(ids.PrefixSKU).String()
	seller := ids.MustNew(ids.PrefixSeller).String()

	item, err := m.Receive(ctx, inventory.ReceiveRequest{
		SKUID: skuID, LocationID: locID, OwnerID: seller,
		Quantity: 25, ReferenceID: "nhap-dau", PerformedBy: seller,
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if err := m.SetAvailable(ctx, item.ID, 25, "Kiểm kê khớp sổ", seller); err != nil {
		t.Fatalf("SetAvailable: %v", err)
	}

	var adjusts int
	if err := db.Pool().QueryRow(ctx, `
		SELECT count(*) FROM inventory_movement
		 WHERE inventory_item_id = $1 AND movement_type = 'ADJUST'`, item.ID,
	).Scan(&adjusts); err != nil {
		t.Fatalf("đếm nhật ký: %v", err)
	}
	if adjusts != 0 {
		t.Errorf("có %d dòng điều chỉnh, mong 0 — kiểm kê khớp sổ không phải "+
			"một biến động", adjusts)
	}
}
