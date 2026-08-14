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
