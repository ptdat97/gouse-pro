package order_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

func vnd(n int64) order.Amount { return order.Amount{Value: n, Currency: "VND"} }

func newModule(t *testing.T) *order.Module {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE fulfillment_order_line CASCADE",
		"TRUNCATE fulfillment_order CASCADE",
		"TRUNCATE order_line_adjustment CASCADE",
		"TRUNCATE order_line CASCADE",
		`TRUNCATE "order" CASCADE`,
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	if _, err := db.Pool().Exec(ctx, "TRUNCATE audit_log"); err != nil {
		t.Fatalf("dọn nhật ký: %v", err)
	}

	m, err := order.New(order.Config{
		Storage: "postgres",
		DB:      db,
		Audit:   audit.NewRecorder(db.Pool()),
	})
	if err != nil {
		t.Fatalf("order.New: %v", err)
	}
	return m
}

// newModuleWithDB như newModule nhưng trả thêm kết nối để đọc nhật ký.
func newModuleWithDB(t *testing.T) (*order.Module, *database.DB) {
	t.Helper()
	m := newModule(t)
	// Kết nối THỨ HAI tới cùng database riêng của gói: newModule không trả
	// ra kết nối của nó, và mở thêm một pool rẻ hơn nhiều so với việc đổi
	// chữ ký của mọi test đang dùng newModule.
	return m, testdb.Open(t)
}

// line dựng một dòng hàng cho test.
func line(sellerID string, price int64, qty, rate int, name string) order.PlaceOrderLineInput {
	return order.PlaceOrderLineInput{
		OfferID:        ids.MustNew(ids.PrefixOffer).String(),
		SKUID:          ids.MustNew(ids.PrefixSKU).String(),
		SellerID:       sellerID,
		ProductName:    name,
		UnitPrice:      vnd(price),
		Quantity:       qty,
		CommissionRate: rate,
	}
}

// QUY TẮC 5: PlaceOrder phải IDEMPOTENT.
//
// Khách bấm "Đặt hàng" hai lần, hoặc client thử lại sau timeout — không
// được tạo hai đơn. Hai đơn nghĩa là khách bị trừ tiền hai lần.
func TestDatHangHaiLanChiTaoMotDon(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	req := order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 299000, 1, 1000, "Áo sơ mi"),
		},
		IdempotencyKey: "khach-bam-hai-lan",
	}

	first, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("lần đặt thứ nhất: %v", err)
	}
	second, err := m.PlaceOrder(ctx, req)
	if err != nil {
		t.Fatalf("lần đặt thứ hai: %v", err)
	}

	if first.Order.ID != second.Order.ID {
		t.Errorf("hai lần đặt ra hai đơn khác nhau: %s và %s",
			first.Order.ID, second.Order.ID)
	}
	if !second.Replayed {
		t.Error("lần thứ hai phải báo Replayed = true — nếu không, bên gọi " +
			"sẽ gửi email xác nhận lần thứ hai")
	}
}

// IDEMPOTENT DƯỚI TẢI SONG SONG.
//
// Kiểm tra khóa TRƯỚC khi ghi chỉ bắt được phần lớn trường hợp: hai request
// đến cùng lúc đều thấy "chưa có đơn" rồi cùng ghi. Ràng buộc UNIQUE ở
// database là thứ chặn thật.
//
// Test này chạy 10 request SONG SONG với cùng một khóa. Kết quả đúng là:
// đúng MỘT đơn trong database, và cả 10 lời gọi đều trả về đơn đó.
func TestDatHangSongSongVoiCungKhoaChiRaMotDon(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	req := order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 299000, 1, 1000, "Áo sơ mi"),
		},
		IdempotencyKey: "mot-khoa-muoi-request",
	}

	const n = 10
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		start = make(chan struct{})
		ids_  = map[string]int{}
		errs  []error
	)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Chờ hiệu lệnh chung để các goroutine thật sự chạy cùng lúc —
			// không có nó thì chúng chạy nối tiếp và test không kiểm chứng
			// được điều cần kiểm chứng.
			<-start

			res, err := m.PlaceOrder(ctx, req)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			ids_[res.Order.ID]++
		}()
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		t.Errorf("một request thất bại: %v", err)
	}
	if len(ids_) != 1 {
		t.Fatalf("số đơn khác nhau = %d, mong 1 — khóa idempotency không "+
			"chặn được request song song", len(ids_))
	}
	for id, count := range ids_ {
		if count != n {
			t.Errorf("đơn %s chỉ được trả về %d/%d lần", id, count, n)
		}
	}
}

// KHÁCH VÃNG LAI đặt được hàng (quy tắc 6), nhưng phải liên hệ được.
func TestKhachVangLaiDatDuocHangQuaDatabase(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	res, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		GuestEmail: "khach@example.com",
		GuestPhone: "0900000000",
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 299000, 1, 1000, "Áo sơ mi"),
		},
		IdempotencyKey: "khach-vang-lai-1",
	})
	if err != nil {
		t.Fatalf("khách vãng lai phải đặt được hàng: %v", err)
	}
	if res.Order.CustomerID != "" {
		t.Errorf("customerID = %q, mong rỗng", res.Order.CustomerID)
	}

	// Không có cả customerID lẫn email thì bị chặn — ràng buộc CHECK ở
	// database là hàng rào cuối, nhưng domain phải chặn trước.
	_, err = m.PlaceOrder(ctx, order.PlaceOrderRequest{
		Currency: "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 299000, 1, 1000, "Áo sơ mi"),
		},
		IdempotencyKey: "khach-vo-danh",
	})
	if !errors.Is(err, order.ErrInvalidInput) {
		t.Errorf("lỗi = %v, mong ErrInvalidInput", err)
	}
}

// KHOẢN ĐIỀU CHỈNH qua vòng ghi–đọc: hoàn tiền từng phần đọc TRỰC TIẾP.
//
// Đơn 3 món 500.000đ giảm 50.000đ, phân bổ về từng dòng. Khách trả món C
// (100.000đ) thì hoàn 100.000 − 10.000 = 90.000đ, không phải tính lại tỷ lệ.
func TestKhoanDieuChinhPhanBoVeTungDong(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()

	mkLine := func(price int64, name string, discount int64) order.PlaceOrderLineInput {
		l := line(sellerID, price, 1, 1000, name)
		l.Adjustments = []order.AdjustmentInput{{
			Type:       "PROMOTION",
			Label:      "Giảm giá THUDONG20",
			Amount:     vnd(-discount),
			SourceType: "promotion_rule",
			CostBearer: "PLATFORM",
		}}
		return l
	}

	res, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{
			mkLine(200000, "Món A", 20000),
			mkLine(200000, "Món B", 20000),
			mkLine(100000, "Món C", 10000),
		},
		IdempotencyKey: "phan-bo-giam-gia",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	got, err := m.GetOrder(ctx, res.Order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}

	for _, l := range got.Lines {
		if len(l.Adjustments) != 1 {
			t.Fatalf("dòng %q có %d khoản điều chỉnh, mong 1",
				l.ProductName, len(l.Adjustments))
		}
		a := l.Adjustments[0]
		if a.Label != "Giảm giá THUDONG20" {
			t.Errorf("nhãn = %q — khách phải đọc được vì sao bị trừ", a.Label)
		}
		if a.CostBearer != "PLATFORM" {
			t.Errorf("bên chịu chi phí = %q, mong PLATFORM", a.CostBearer)
		}

		// Món C: hoàn tiền = 100.000 − 10.000, ĐỌC TRỰC TIẾP.
		if l.ProductName == "Món C" && a.Amount.Value != -10000 {
			t.Errorf("khoản giảm của món C = %d, mong -10000", a.Amount.Value)
		}
	}
}

// ĐÓNG BĂNG qua vòng GHI–ĐỌC DATABASE.
//
// Test ở tầng domain đã chứng minh dữ liệu không đổi trong bộ nhớ. Test này
// chứng minh nó cũng không đổi khi đi qua PostgreSQL: nếu store JOIN sang
// bảng product hay pricing để lấy tên và giá, đối soát tháng trước sẽ ra số
// khác sau khi seller đổi giá.
func TestDongBangSongSotQuaVongGhiDoc(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	sellerID := ids.MustNew(ids.PrefixSeller).String()

	res, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{{
			OfferID:            ids.MustNew(ids.PrefixOffer).String(),
			SKUID:              ids.MustNew(ids.PrefixSKU).String(),
			SellerID:           sellerID,
			ProductName:        "Áo sơ mi linen Oxford",
			VariantDescription: "Trắng / M",
			UnitPrice:          vnd(299000),
			Quantity:           1,
			CommissionRate:     1000,
		}},
		IdempotencyKey: "dong-bang-qua-db",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	got, err := m.GetOrder(ctx, res.Order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}

	l := got.Lines[0]
	for _, tc := range []struct {
		field string
		got   any
		want  any
	}{
		{"tên sản phẩm", l.ProductName, "Áo sơ mi linen Oxford"},
		{"mô tả biến thể", l.VariantDescription, "Trắng / M"},
		{"đơn giá", l.UnitPrice.Value, int64(299000)},
		{"tỷ lệ hoa hồng", l.CommissionRate, 1000},
		{"tiền hoa hồng", l.CommissionAmount.Value, int64(29900)},
		{"phải trả seller", l.SellerPayable.Value, int64(269100)},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, mong %v", tc.field, tc.got, tc.want)
		}
	}
}

// ---------------------------------------------------------------- Quản trị

// placeForAdmin dựng một đơn để test các endpoint quản trị.
func placeForAdmin(t *testing.T, m *order.Module, key string) order.OrderView {
	t.Helper()
	res, err := m.PlaceOrder(context.Background(), order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ids.MustNew(ids.PrefixSeller).String(), 299000, 1, 1000, "Áo sơ mi"),
		},
		ShippingAddress: order.AddressInput{
			RecipientName: "Nguyễn Văn A",
			Phone:         "0901234567",
			StreetAddress: "12 Nguyễn Huệ",
			Province:      "TP.HCM",
		},
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	return res.Order
}

const adminActor = "usr_01J9XABC123DEF456GHJKMNPQR"

// XEM chi tiết đơn phải ghi vết — đây là dữ liệu cá nhân khách hàng.
//
// admin-api.md mục 6: đọc trộm dữ liệu khách không để lại dấu vết nào khác,
// nên chính việc ĐỌC phải sinh bản ghi.
func TestXemChiTietDonGhiVetTruyCap(t *testing.T) {
	m, db := newModuleWithDB(t)
	ctx := context.Background()
	v := placeForAdmin(t, m, "xem-chi-tiet")

	const reason = "Xử lý khiếu nại giao hàng chậm, khách gọi hotline sáng nay"
	got, err := m.ViewOrderAsAdmin(ctx, order.ViewOrderRequest{
		OrderID: v.ID, ActorID: adminActor, Reason: reason,
	})
	if err != nil {
		t.Fatalf("ViewOrderAsAdmin: %v", err)
	}
	if got.OrderNumber != v.OrderNumber {
		t.Errorf("trả nhầm đơn: %q", got.OrderNumber)
	}

	rec := audit.NewRecorder(db.Pool())
	entries, _, err := rec.Query(ctx, audit.Filter{Action: "order.view"})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("mỗi lần XEM phải sinh đúng 1 vết, nhận %d", len(entries))
	}
	if entries[0].ActorID != adminActor {
		t.Error("vết phải ghi ai đã xem")
	}
	if entries[0].Reason != reason {
		t.Errorf("lý do bị đổi: %q", entries[0].Reason)
	}
}

// Không có lý do thì KHÔNG được đọc dữ liệu khách.
//
// Lý do trống biến nhật ký truy cập thành danh sách vô nghĩa — không phân
// biệt được tra cứu chính đáng với tò mò.
func TestXemChiTietDonThieuLyDoBiTuChoi(t *testing.T) {
	m, db := newModuleWithDB(t)
	ctx := context.Background()
	v := placeForAdmin(t, m, "thieu-ly-do")

	for _, reason := range []string{"", "xem thử", "testtesttesttesttest"} {
		if _, err := m.ViewOrderAsAdmin(ctx, order.ViewOrderRequest{
			OrderID: v.ID, ActorID: adminActor, Reason: reason,
		}); err == nil {
			t.Errorf("lý do %q phải bị từ chối", reason)
		}
	}

	// Và không có vết nào được ghi cho các lần bị từ chối.
	rec := audit.NewRecorder(db.Pool())
	entries, _, err := rec.Query(ctx, audit.Filter{Action: "order.view"})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("truy cập bị từ chối thì KHÔNG được có vết, nhận %d", len(entries))
	}
}

// Hủy đơn ghi vết trong CÙNG giao dịch với việc đổi trạng thái.
func TestHuyDonQuanTriGhiVet(t *testing.T) {
	m, db := newModuleWithDB(t)
	ctx := context.Background()
	v := placeForAdmin(t, m, "huy-don")

	const reason = "Khách yêu cầu hủy vì đặt nhầm size, hàng chưa xuất kho"
	got, err := m.CancelOrderAsAdmin(ctx, order.CancelOrderRequest{
		OrderID: v.ID, ActorID: adminActor, Reason: reason,
	})
	if err != nil {
		t.Fatalf("CancelOrderAsAdmin: %v", err)
	}
	if got.Status != "CANCELLED" {
		t.Errorf("trạng thái = %q, mong CANCELLED", got.Status)
	}

	rec := audit.NewRecorder(db.Pool())
	entries, _, err := rec.Query(ctx, audit.Filter{Action: "order.cancel"})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("mong 1 vết hủy đơn, nhận %d", len(entries))
	}
	if entries[0].Metadata["order_number"] != v.OrderNumber {
		t.Errorf("vết phải ghi mã đơn — khách đọc mã này khi khiếu nại: %v",
			entries[0].Metadata)
	}
}

// Hủy đơn thất bại KHÔNG để lại trạng thái nửa vời.
func TestHuyDonLyDoRacKhongDoiTrangThai(t *testing.T) {
	m, db := newModuleWithDB(t)
	ctx := context.Background()
	v := placeForAdmin(t, m, "huy-ly-do-rac")

	if _, err := m.CancelOrderAsAdmin(ctx, order.CancelOrderRequest{
		OrderID: v.ID, ActorID: adminActor, Reason: "fixfixfixfixfixfixfixfix",
	}); err == nil {
		t.Fatal("lý do rác phải bị từ chối")
	}

	got, err := m.GetOrder(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if got.Status == "CANCELLED" {
		t.Error("ghi vết thất bại thì việc hủy PHẢI bị hủy theo")
	}

	rec := audit.NewRecorder(db.Pool())
	entries, _, err := rec.Query(ctx, audit.Filter{Action: "order.cancel"})
	if err != nil {
		t.Fatalf("đọc nhật ký: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("giao dịch hủy thì KHÔNG được có vết, nhận %d", len(entries))
	}
}

// Danh sách quản trị lọc được theo mã đơn và trạng thái.
func TestDanhSachDonQuanTriLocDuoc(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	a := placeForAdmin(t, m, "loc-a")
	placeForAdmin(t, m, "loc-b")

	all, err := m.ListOrders(ctx, order.ListFilter{})
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("không lọc phải trả 2 đơn, nhận %d", len(all))
	}

	// Tổng tiền được TÍNH TỪ dòng hàng — quên nạp dòng thì danh sách hiển
	// thị toàn 0đ và nhân viên hỗ trợ không dùng được. Lỗi này đã xảy ra
	// thật, phát hiện lúc chạy server.
	for _, o := range all {
		if o.LineCount == 0 {
			t.Errorf("đơn %s: chưa nạp dòng hàng", o.OrderNumber)
		}
		if o.Total.Value == 0 {
			t.Errorf("đơn %s: tổng tiền = 0, dòng hàng chưa được nạp",
				o.OrderNumber)
		}
	}

	// Tra theo mã đơn — đường tra cứu chính của nhân viên hỗ trợ, vì khách
	// đọc mã này qua điện thoại.
	one, err := m.ListOrders(ctx, order.ListFilter{OrderNumber: a.OrderNumber})
	if err != nil {
		t.Fatalf("ListOrders theo mã: %v", err)
	}
	if len(one) != 1 || one[0].ID != a.ID {
		t.Errorf("lọc theo mã đơn sai: %+v", one)
	}

	none, err := m.ListOrders(ctx, order.ListFilter{Status: "KHONG_TON_TAI"})
	if err != nil {
		t.Fatalf("ListOrders theo trạng thái: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("trạng thái không tồn tại phải trả rỗng, nhận %d", len(none))
	}
}

// LÝ DO HỦY của khách được LƯU XUỐNG DATABASE.
//
// Đặc tả bắt khách chọn một trong năm lý do. Kiểm tra rồi vứt đi thì việc
// bắt họ chọn là vô nghĩa — và ba vấn đề khác nhau ("giao quá chậm",
// "tìm được giá tốt hơn", "đặt nhầm") trông giống hệt nhau: "đơn bị hủy".
//
// Test TRUY VẤN THẲNG bảng `order`. Đọc qua module thì không phân biệt được
// "đã ghi xuống" với "còn trong bộ nhớ từ lúc hủy" — và các báo cáo về lý
// do hủy đọc thẳng bảng này.
func TestLyDoHuyCuaKhachLuuXuongDatabase(t *testing.T) {
	m, db := newModuleWithDB(t)
	ctx := context.Background()
	v := placeForAdmin(t, m, "khach-tu-huy")

	if err := m.CancelOrder(ctx, v.ID, "DELIVERY_TOO_SLOW"); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}

	var stored string
	if err := db.Pool().QueryRow(ctx,
		`SELECT cancellation_reason FROM "order" WHERE id = $1`, v.ID,
	).Scan(&stored); err != nil {
		t.Fatalf("đọc lý do hủy: %v", err)
	}
	if stored != "DELIVERY_TOO_SLOW" {
		t.Errorf("lý do hủy trong database = %q, mong DELIVERY_TOO_SLOW", stored)
	}
}

// HỦY BỞI QUẢN TRỊ VIÊN không ghi lý do vào đơn.
//
// Lý do của nhân viên là VĂN BẢN TỰ DO đã được kiểm tra chất lượng và ghi
// vào nhật ký thao tác (ADR-0011). Chép nó vào cột dữ liệu nghiệp vụ sẽ
// làm hỏng mọi báo cáo tổng hợp theo năm lý do đóng của khách.
func TestHuyBoiQuanTriKhongGhiLyDoVaoDon(t *testing.T) {
	m, db := newModuleWithDB(t)
	ctx := context.Background()
	v := placeForAdmin(t, m, "quan-tri-huy")

	if _, err := m.CancelOrderAsAdmin(ctx, order.CancelOrderRequest{
		OrderID: v.ID, ActorID: adminActor,
		Reason: "Khách yêu cầu hủy vì đặt nhầm size, hàng chưa xuất kho",
	}); err != nil {
		t.Fatalf("CancelOrderAsAdmin: %v", err)
	}

	var stored string
	if err := db.Pool().QueryRow(ctx,
		`SELECT cancellation_reason FROM "order" WHERE id = $1`, v.ID,
	).Scan(&stored); err != nil {
		t.Fatalf("đọc lý do hủy: %v", err)
	}
	if stored != "" {
		t.Errorf("lý do hủy trong đơn = %q, mong rỗng — lý do của nhân viên "+
			"thuộc về nhật ký thao tác", stored)
	}
}
