package order_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

func vnd(n int64) order.Amount { return order.Amount{Value: n, Currency: "VND"} }

func newModule(t *testing.T) *order.Module {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("bỏ qua: cần DATABASE_URL để chạy test với PostgreSQL thật")
	}

	db, err := database.Open(context.Background(), database.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("mở database: %v", err)
	}
	t.Cleanup(db.Close)

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

	m, err := order.New(order.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("order.New: %v", err)
	}
	return m
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

// ĐẶT ĐƠN BA NGUỒN HÀNG — ví dụ ở mục 3.2 của ADR-0007.
//
//	Giỏ hàng:
//	├── Áo own brand   (kho nền tảng, Hà Nội)
//	├── Giày Seller A  (kho seller A, TP.HCM)
//	└── Túi Seller B   (kho seller B, Đà Nẵng)
//
// Ba món KHÔNG THỂ đóng chung một gói, nên phải thành BA đơn thực hiện.
func TestDatDonBaNguonHangTachThanhBaDonThucHien(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	ownBrand := ids.MustNew(ids.PrefixSeller).String()
	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	res, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		ShippingAddress: order.AddressInput{
			RecipientName: "Nguyễn Văn A",
			Phone:         "0900000000",
			StreetAddress: "12 Lý Thường Kiệt",
			Province:      "Hà Nội",
		},
		Currency: "VND",
		Lines: []order.PlaceOrderLineInput{
			line(ownBrand, 299000, 1, 0, "Áo sơ mi own brand"),
			line(sellerA, 890000, 1, 1000, "Giày Seller A"),
			line(sellerB, 450000, 2, 1200, "Túi Seller B"),
		},
		IdempotencyKey: "don-ba-nguon-1",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	if len(res.Fulfillments) != 3 {
		t.Fatalf("số đơn thực hiện = %d, mong 3", len(res.Fulfillments))
	}
	if res.Replayed {
		t.Error("lần đặt đầu tiên không phải phát lại")
	}

	// 299.000 + 890.000 + 900.000 = 2.089.000
	if res.Order.Total.Value != 2089000 {
		t.Errorf("tổng tiền = %d, mong 2089000", res.Order.Total.Value)
	}

	// Mã đơn thực hiện đánh theo thứ tự: -A, -B, -C.
	want := []string{"-A", "-B", "-C"}
	for i, fo := range res.Fulfillments {
		suffix := fo.FONumber[len(fo.FONumber)-2:]
		if suffix != want[i] {
			t.Errorf("mã đơn thực hiện %d = %q, hậu tố mong %q",
				i+1, fo.FONumber, want[i])
		}
	}

	// Đọc lại từ database: dữ liệu phải qua được vòng ghi–đọc nguyên vẹn.
	got, err := m.GetOrder(ctx, res.Order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if len(got.Lines) != 3 {
		t.Fatalf("số dòng hàng đọc lại = %d, mong 3", len(got.Lines))
	}
	if got.Total.Value != 2089000 {
		t.Errorf("tổng tiền đọc lại = %d, mong 2089000", got.Total.Value)
	}

	// Hoa hồng ĐÓNG BĂNG, tính một lần lúc đặt: 900.000 × 12% = 108.000.
	for _, l := range got.Lines {
		if l.SellerID != sellerB {
			continue
		}
		if l.CommissionAmount.Value != 108000 {
			t.Errorf("hoa hồng Seller B = %d, mong 108000", l.CommissionAmount.Value)
		}
		if l.SellerPayable.Value != 792000 {
			t.Errorf("phải trả Seller B = %d, mong 792000", l.SellerPayable.Value)
		}
	}
}

// RANH GIỚI BẢO MẬT (ADR-0007, lý do quyết định 3).
//
// Seller A KHÔNG được thấy phần của Seller B, kể cả khi biết chính xác
// định danh đơn thực hiện của B.
//
// Đây là kiểm chứng quan trọng nhất của module: nếu nó hỏng thì seller đọc
// được dữ liệu đối thủ, và không có cách nào phát hiện từ phía nạn nhân.
func TestSellerKhongThayDuocPhanCuaSellerKhac(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	res, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{
			line(sellerA, 890000, 1, 1000, "Giày Seller A"),
			line(sellerB, 450000, 1, 1200, "Túi Seller B"),
		},
		IdempotencyKey: "cach-ly-seller-1",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	// Tìm đơn thực hiện của B.
	var foB string
	for _, fo := range res.Fulfillments {
		if fo.SellerID == sellerB {
			foB = fo.ID
		}
	}
	if foB == "" {
		t.Fatal("không tìm thấy đơn thực hiện của Seller B")
	}

	// A biết chính xác id của B và cố đọc.
	_, err = m.GetSellerFulfillment(ctx, sellerA, foB)
	if !errors.Is(err, order.ErrNotFound) {
		t.Errorf("lỗi = %v, mong ErrNotFound — seller A đọc được đơn của B", err)
	}

	// A cũng không thao tác được lên đơn của B.
	if err := m.ConfirmFulfillment(ctx, sellerA, foB); !errors.Is(err, order.ErrNotFound) {
		t.Errorf("lỗi = %v, mong ErrNotFound — seller A xác nhận được đơn của B", err)
	}
	if err := m.CancelFulfillment(ctx, sellerA, foB, "phá hoại"); !errors.Is(err, order.ErrNotFound) {
		t.Errorf("lỗi = %v, mong ErrNotFound — seller A hủy được đơn của B", err)
	}

	// Danh sách việc của A chỉ có đúng một đơn — của chính A.
	list, err := m.ListSellerFulfillments(ctx, sellerA, nil, 50, 0)
	if err != nil {
		t.Fatalf("ListSellerFulfillments: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("số đơn của seller A = %d, mong 1", len(list))
	}
	if list[0].SellerID != sellerA {
		t.Errorf("đơn trong danh sách thuộc về %q, mong %q", list[0].SellerID, sellerA)
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
	if len(second.Fulfillments) != 1 {
		t.Errorf("số đơn thực hiện lần hai = %d, mong 1", len(second.Fulfillments))
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

// TRẠNG THÁI TỔNG HỢP SUY RA TỪ ĐƠN THỰC HIỆN (quy tắc 7).
//
// Hai seller, mỗi người đi một tốc độ. Trạng thái đơn phải phản ánh đúng
// tiến độ THẤP NHẤT có ý nghĩa với khách, và không được hiển thị lùi.
func TestTrangThaiDonSuyRaTuTienDoCuaSeller(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	res, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{
			line(sellerA, 890000, 1, 1000, "Giày Seller A"),
			line(sellerB, 450000, 1, 1200, "Túi Seller B"),
		},
		IdempotencyKey: "tien-do-hai-seller",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	foA, foB := res.Fulfillments[0].ID, res.Fulfillments[1].ID

	// A đi hết đường tới lúc xuất hàng.
	for _, step := range []func(context.Context, string, string) error{
		m.ConfirmFulfillment, m.PackFulfillment, m.ShipFulfillment,
	} {
		if err := step(ctx, sellerA, foA); err != nil {
			t.Fatalf("bước của seller A: %v", err)
		}
	}

	got, err := m.GetOrder(ctx, res.Order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if got.Status != order.StatusPartiallyShipped {
		t.Errorf("trạng thái = %q, mong PARTIALLY_SHIPPED", got.Status)
	}

	// B cũng xuất hàng: cả hai đã xuất.
	for _, step := range []func(context.Context, string, string) error{
		m.ConfirmFulfillment, m.PackFulfillment, m.ShipFulfillment,
	} {
		if err := step(ctx, sellerB, foB); err != nil {
			t.Fatalf("bước của seller B: %v", err)
		}
	}
	got, _ = m.GetOrder(ctx, res.Order.ID)
	if got.Status != order.StatusShipped {
		t.Errorf("trạng thái = %q, mong SHIPPED", got.Status)
	}

	// A giao xong trước. Đơn phải là PARTIALLY_DELIVERED, KHÔNG phải
	// SHIPPED — hiển thị lùi so với thực tế là lỗi khách nhìn thấy ngay.
	if err := m.DeliverFulfillment(ctx, sellerA, foA); err != nil {
		t.Fatalf("giao hàng seller A: %v", err)
	}
	got, _ = m.GetOrder(ctx, res.Order.ID)
	if got.Status != order.StatusPartiallyDelivered {
		t.Errorf("trạng thái = %q, mong PARTIALLY_DELIVERED", got.Status)
	}

	if err := m.DeliverFulfillment(ctx, sellerB, foB); err != nil {
		t.Fatalf("giao hàng seller B: %v", err)
	}
	got, _ = m.GetOrder(ctx, res.Order.ID)
	if got.Status != order.StatusDelivered {
		t.Errorf("trạng thái = %q, mong DELIVERED", got.Status)
	}
}

// HỦY TỪNG PHẦN: seller B hết hàng, phần của A vẫn giao.
//
// Hai việc phải đi cùng nhau: hủy đơn thực hiện VÀ hủy dòng hàng tương ứng.
// Chỉ hủy một bên thì khách vẫn bị tính tiền món không bao giờ được giao.
func TestSellerHetHangHuyPhanCuaMinhKhongAnhHuongSellerKhac(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()
	sellerB := ids.MustNew(ids.PrefixSeller).String()

	res, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID:  ids.MustNew(ids.PrefixCustomer).String(),
		Currency:    "VND",
		ShippingFee: vnd(0),
		Lines: []order.PlaceOrderLineInput{
			line(sellerA, 400000, 1, 1000, "Giày Seller A"),
			line(sellerB, 200000, 1, 1000, "Túi Seller B"),
		},
		IdempotencyKey: "seller-b-het-hang",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	var foB string
	for _, fo := range res.Fulfillments {
		if fo.SellerID == sellerB {
			foB = fo.ID
		}
	}

	if err := m.CancelFulfillment(ctx, sellerB, foB, "hết hàng tại kho"); err != nil {
		t.Fatalf("CancelFulfillment: %v", err)
	}

	got, err := m.GetOrder(ctx, res.Order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if got.Status != order.StatusPartiallyCancelled {
		t.Errorf("trạng thái = %q, mong PARTIALLY_CANCELLED", got.Status)
	}
	// Tổng tự động giảm còn phần của A.
	if got.Total.Value != 400000 {
		t.Errorf("tổng sau hủy = %d, mong 400000", got.Total.Value)
	}

	// Dòng đã hủy KHÔNG bị xóa (quy tắc 3): hóa đơn và đối soát vẫn giải
	// thích được vì sao khách từng thấy con số cũ.
	if len(got.Lines) != 2 {
		t.Errorf("số dòng hàng = %d, mong 2 — dòng đã hủy không được xóa", len(got.Lines))
	}

	// A vẫn làm việc bình thường.
	var foA string
	for _, fo := range res.Fulfillments {
		if fo.SellerID == sellerA {
			foA = fo.ID
		}
	}
	if err := m.ConfirmFulfillment(ctx, sellerA, foA); err != nil {
		t.Errorf("seller A phải xử lý được phần của mình: %v", err)
	}

	// Lý do hủy phải đọc lại được: khách cần lời giải thích.
	foView, err := m.GetSellerFulfillment(ctx, sellerB, foB)
	if err != nil {
		t.Fatalf("GetSellerFulfillment: %v", err)
	}
	if foView.CancelReason != "hết hàng tại kho" {
		t.Errorf("lý do hủy = %q, mong \"hết hàng tại kho\"", foView.CancelReason)
	}
}

// QUY TẮC 8: từ PACKED trở đi, hủy cần quy trình riêng.
//
// Đã tốn công đóng gói và vật tư, có thể đã bàn giao vận chuyển. Cho hủy
// như thao tác thường nghĩa là chi phí đó không ai chịu trách nhiệm.
func TestDaDongGoiThiKhongHuyDuocBangThaoTacThuong(t *testing.T) {
	m := newModule(t)
	ctx := context.Background()

	sellerA := ids.MustNew(ids.PrefixSeller).String()

	res, err := m.PlaceOrder(ctx, order.PlaceOrderRequest{
		CustomerID: ids.MustNew(ids.PrefixCustomer).String(),
		Currency:   "VND",
		Lines: []order.PlaceOrderLineInput{
			line(sellerA, 400000, 1, 1000, "Giày Seller A"),
		},
		IdempotencyKey: "da-dong-goi",
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	foA := res.Fulfillments[0].ID

	if err := m.ConfirmFulfillment(ctx, sellerA, foA); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if err := m.PackFulfillment(ctx, sellerA, foA); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	err = m.CancelFulfillment(ctx, sellerA, foA, "đổi ý")
	if !errors.Is(err, order.ErrInvalidStatus) {
		t.Errorf("lỗi = %v, mong ErrInvalidStatus — đã đóng gói mà vẫn hủy được", err)
	}

	// Khách cũng không hủy được cả đơn nữa (mục 6.1).
	err = m.CancelOrder(ctx, res.Order.ID, "khách đổi ý")
	if !errors.Is(err, order.ErrNotCancellable) {
		t.Errorf("lỗi = %v, mong ErrNotCancellable", err)
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
