package postgres_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/order/domain"
	"github.com/fashion-commerce/platform/internal/modules/order/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

var luc = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), testdb.DSN(t))
	if err != nil {
		t.Fatalf("kết nối database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	return pool
}

// TestDonKhuHoiKhongMatTruong.
//
// # Lớp lỗi mà bài này canh, và vì sao kỷ luật con người không đủ
//
// `withLines` dựng lại đơn TỪ ĐẦU bằng một danh sách trường viết tay. Bỏ
// sót một trường ở đó làm nó biến mất im lặng sau MỖI lần đọc — trình biên
// dịch không nhắc, vì thiếu trường chỉ là giá trị rỗng hợp lệ.
//
// Đã xảy ra BA lần trong chính hàm đó:
//
//	Version            khóa lạc quan so với 0 → mọi lần chuyển trạng thái
//	                   TIẾP THEO thất bại
//	SourceCheckoutID   bất biến "một phiên một đơn" mất chỗ dựa
//	DeliveredAt        `HetHanTra` trả false khi mốc giao rỗng, nên HẠN
//	                   ĐỔI TRẢ không bao giờ hết và nền tảng hoàn tiền cho
//	                   đơn giao từ năm ngoái
//
// Lần thứ ba xảy ra DÙ chú thích ngay trên đó đã cảnh báo đúng điều này.
// Đó là bằng chứng đủ: chỗ này cần hàng rào, không cần thêm lời nhắc.
func TestDonKhuHoiKhongMatTruong(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	store := postgres.NewOrderStore(pool)

	goc := thamSoDayDu(t)
	don := domain.RestoreOrder(goc)

	if err := store.Save(ctx, don); err != nil {
		t.Fatalf("lưu đơn: %v", err)
	}

	docLai, err := store.FindByID(ctx, don.ID())
	if err != nil {
		t.Fatalf("đọc lại: %v", err)
	}

	soSanh(t, goc, rutRa(docLai), rongLucTao)
}

// TestDonKhuHoiQuaDuongCapNhat canh câu UPDATE và `withLines`.
//
// # Vì sao bài TẠO ở trên KHÔNG đủ, và tôi đã phát hiện bằng cách phá
//
// Bài tạo bỏ qua `DeliveredAt` và `CompletedAt` vì đơn vừa tạo chưa thể có
// chúng. Nhưng `DeliveredAt` CHÍNH LÀ trường mà `withLines` từng bỏ sót —
// nên danh sách bỏ qua làm hàng rào mù đúng chỗ nó sinh ra để canh.
//
// Phát hiện lúc phá: gỡ `DeliveredAt` khỏi `withLines` mà bài tạo vẫn
// XANH. Một bài kiểm xanh vì bỏ qua đúng thứ cần kiểm thì tệ hơn không có
// bài kiểm, vì nó tạo cảm giác an toàn.
//
// Bài này lấp chỗ đó: ghi rồi CẬP NHẬT với mọi trường đã điền, đọc lại, và
// so TẤT CẢ — không bỏ qua trường nào.
func TestDonKhuHoiQuaDuongCapNhat(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	store := postgres.NewOrderStore(pool)

	goc := thamSoDayDu(t)

	// Tạo với version 0 — đúng thứ database đặt cho dòng mới.
	tao := goc
	tao.Version = 0
	if err := store.Save(ctx, domain.RestoreOrder(tao)); err != nil {
		t.Fatalf("lưu đơn: %v", err)
	}

	if err := store.Update(ctx, domain.RestoreOrder(tao)); err != nil {
		t.Fatalf("cập nhật: %v", err)
	}

	docLai, err := store.FindByID(ctx, tao.ID)
	if err != nil {
		t.Fatalf("đọc lại: %v", err)
	}

	// Update tăng version, nên so với 1 chứ không phải 0.
	mong := tao
	mong.Version = 1
	soSanh(t, mong, rutRa(docLai), nil)
}

// rongLucTao là trường một đơn VỪA TẠO chưa thể có.
//
// Hướng của danh sách này quan trọng: mặc định của một trường MỚI là "phải
// khứ hồi được". Quên xử lý nó thì bài test ĐỎ — không phải xanh.
var rongLucTao = map[string]bool{
	"CompletedAt":        true, // đặt khi đơn chốt
	"DeliveredAt":        true, // đặt khi giao xong
	"CancellationReason": true,
	"Version":            true, // database đặt, không phải bộ nhớ
}

// thamSoDayDu dựng tham số với MỌI trường khác rỗng.
//
// Reflect ở cuối là phần quan trọng nhất: nó biến "quên điền trường mới"
// từ một bài test xanh giả thành một lỗi có tên.
func thamSoDayDu(t *testing.T) domain.RestoreOrderParams {
	t.Helper()

	tien := func(v int64) money.Money {
		t.Helper()
		m, err := money.New(v, money.VND)
		if err != nil {
			t.Fatalf("tiền: %v", err)
		}
		return m
	}

	dc := domain.Address{
		RecipientName: "Người Nhận", Phone: "0900111222",
		StreetAddress: "1 Đường Thử", Ward: "Phường 1",
		District: "Quận 1", Province: "TP.HCM", CountryCode: "VN",
	}

	line := domain.RestoreLine(domain.RestoreLineParams{
		ID: ids.MustNew(ids.PrefixOrderLine), OfferID: ids.MustNew(ids.PrefixOffer),
		SKUID: ids.MustNew(ids.PrefixSKU), SellerID: ids.MustNew(ids.PrefixSeller),
		ProductName: "Áo thun basic", VariantDescription: "Đen / M",
		UnitPrice: tien(250_000), Quantity: 2,
		Status: domain.LineActive, CreatedAt: luc, UpdatedAt: luc,
	})

	p := domain.RestoreOrderParams{
		ID:                 ids.MustNew(ids.PrefixOrder),
		OrderNumber:        "FC-2026-09-" + string(ids.MustNew(ids.PrefixOrder))[4:14],
		CustomerID:         ids.MustNew(ids.PrefixCustomer),
		GuestEmail:         "khach@example.com",
		GuestPhone:         "0900111222",
		ShippingAddress:    dc,
		BillingAddress:     dc,
		Currency:           money.VND,
		ShippingFee:        tien(30_000),
		DiscountAmount:     tien(10_000),
		TaxAmount:          tien(8_000),
		Status:             domain.StatusDelivered,
		Lines:              []*domain.Line{line},
		IdempotencyKey:     ids.MustNew(ids.PrefixRequest).String(),
		SourceCheckoutID:   ids.MustNew(ids.PrefixCheckout),
		PaymentMethod:      domain.PaymentMethodCOD,
		Version:            1,
		CancellationReason: "khách đổi ý",
		PlacedAt:           luc,
		CompletedAt:        luc.Add(96 * time.Hour),
		PaidAt:             luc.Add(48 * time.Hour),
		DeliveredAt:        luc.Add(48 * time.Hour),
		CreatedAt:          luc,
		UpdatedAt:          luc.Add(time.Hour),
	}

	// HÀNG RÀO CHO CHÍNH BÀI KIỂM.
	//
	// Trường nào còn rỗng thì bài này không kiểm được nó: lưu 0 rồi đọc
	// lại 0 luôn khớp, kể cả khi cột bị bỏ quên hoàn toàn.
	v := reflect.ValueOf(p)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Fatalf("trường %q còn RỖNG trong bộ tham số của bài kiểm — "+
				"điền một giá trị khác rỗng, nếu không cột tương ứng có bị "+
				"bỏ quên khỏi `withLines` hay khỏi SQL thì bài này vẫn xanh",
				v.Type().Field(i).Name)
		}
	}
	return p
}

// rutRa dựng lại bộ tham số từ đơn đã đọc.
//
// Đây LÀ một danh sách viết tay, và là chỗ yếu duy nhất còn lại. Nhưng nó
// hỏng theo hướng AN TOÀN: quên một trường ở đây nghĩa là giá trị rút ra
// bằng rỗng trong khi bản gốc khác rỗng — bài test ĐỎ và gọi đúng tên
// trường, thay vì im lặng bỏ qua.
func rutRa(o *domain.Order) domain.RestoreOrderParams {
	return domain.RestoreOrderParams{
		ID:                 o.ID(),
		OrderNumber:        o.OrderNumber(),
		CustomerID:         o.CustomerID(),
		GuestEmail:         o.GuestEmail(),
		GuestPhone:         o.GuestPhone(),
		ShippingAddress:    o.ShippingAddress(),
		BillingAddress:     o.BillingAddress(),
		Currency:           o.Currency(),
		ShippingFee:        o.ShippingFee(),
		DiscountAmount:     o.DiscountAmount(),
		TaxAmount:          o.TaxAmount(),
		Status:             o.Status(),
		Lines:              o.Lines(),
		IdempotencyKey:     o.IdempotencyKey(),
		SourceCheckoutID:   o.SourceCheckoutID(),
		PaymentMethod:      o.PaymentMethod(),
		Version:            o.Version(),
		CancellationReason: o.CancellationReason(),
		PlacedAt:           o.PlacedAt(),
		CompletedAt:        o.CompletedAt(),
		PaidAt:             o.PaidAt(),
		DeliveredAt:        o.DeliveredAt(),
		CreatedAt:          o.CreatedAt(),
		UpdatedAt:          o.UpdatedAt(),
	}
}

// soSanh đối chiếu TỪNG trường và gọi tên trường lệch.
//
// `reflect.DeepEqual` trên cả struct chỉ nói "khác nhau" — với hơn hai
// mươi trường thì đó là thông điệp gần như vô dụng.
func soSanh(t *testing.T, goc, sau domain.RestoreOrderParams, boQua map[string]bool) {
	t.Helper()

	a := reflect.ValueOf(goc)
	b := reflect.ValueOf(sau)
	for i := 0; i < a.NumField(); i++ {
		ten := a.Type().Field(i).Name
		if boQua[ten] {
			continue
		}
		// Dòng hàng so riêng: chúng là con trỏ, và DeepEqual trên con trỏ
		// so địa chỉ chứ không so nội dung.
		if ten == "Lines" {
			if len(sau.Lines) != len(goc.Lines) {
				t.Errorf("số dòng hàng = %d, cần %d",
					len(sau.Lines), len(goc.Lines))
			}
			continue
		}
		x, y := a.Field(i).Interface(), b.Field(i).Interface()

		if tx, ok := x.(time.Time); ok {
			if ty := y.(time.Time); !tx.Equal(ty) {
				t.Errorf("trường %s lệch sau khi lưu rồi đọc lại: %v → %v",
					ten, tx, ty)
			}
			continue
		}
		if !reflect.DeepEqual(x, y) {
			t.Errorf("trường %s lệch sau khi lưu rồi đọc lại: %#v → %#v",
				ten, x, y)
		}
	}
}
