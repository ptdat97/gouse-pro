package postgres_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/infrastructure/postgres"
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

// TestLuuRoiDocLaiKhongMatTruongNao.
//
// # Lớp lỗi mà bài này canh
//
// Ba lần trong dự án này, cùng một hình dạng: **một danh sách trường viết
// tay bỏ sót một trường**, và trường đó bị xóa trắng hoặc không bao giờ
// được ghi.
//
//	withLineIDs quên ChoThanhToan      cờ khóa giao hàng bị xóa khi đọc
//	withLines   quên BenChiuGiamGia    bên chịu giảm giá bị xóa khi đọc
//	INSERT      quên shipping_method   rỗng trên CẢ 3.207 đơn thực hiện
//
// Ca thứ ba im lặng nhất: `Update` CÓ ghi cột đó, nên câu lệnh nhìn đúng;
// chỉ đường TẠO là thiếu. Và bút toán chi phí hãng vận chuyển đọc đúng
// trường ấy, nên nó không bao giờ được ghi — mà cũng không báo lỗi, vì
// "chưa khai giá" và "không biết phương thức" cùng dẫn tới giá 0.
//
// # Vì sao bài này không tự mắc lại lỗi đó
//
// Nó KHÔNG liệt kê trường bằng tay để so sánh. Nó dựng một bộ tham số với
// MỌI trường khác giá trị rỗng — và dùng reflect để BẮT BUỘC điều đó: thêm
// một trường mới mà quên điền thì bài dừng ngay, kèm tên trường.
// rongLucTao là những trường một đơn thực hiện VỪA SINH RA chưa thể có.
//
// Danh sách này là nghiệp vụ, không phải kỹ thuật: chưa xác nhận thì chưa
// có `ConfirmedAt`, chưa bàn giao thì chưa có mã vận đơn. Mọi trường KHÔNG
// nằm trong danh sách đều phải đi qua đường tạo mà không mất.
//
// Hướng của danh sách này là điều quan trọng: mặc định của một trường MỚI
// là "phải khứ hồi được". Quên xử lý nó thì bài test ĐỎ — không phải xanh.
var rongLucTao = map[string]bool{
	"Status":            true, // luôn về PENDING lúc tạo
	"CancelReason":      true,
	"FailureReason":     true,
	"StockLocationID":   true, // gán khi phân bổ kho
	"ShippingProvider":  true, // gán khi bàn giao
	"TrackingNumber":    true,
	"EstimatedDelivery": true,
	"CompletedAt":       true,
	"ConfirmedAt":       true,
	"PackedAt":          true,
	"ShippedAt":         true,
	"DeliveredAt":       true,
	"CancelledAt":       true,
	"Version":           true, // database đặt, không phải bộ nhớ
}

// TestDuongTaoKhongMatTruongNao canh câu INSERT.
//
// Đây là đường đã bỏ sót `shipping_method`: `Update` CÓ ghi cột đó nên câu
// lệnh nhìn đúng, chỉ đường TẠO là thiếu — và không test nào đi qua đường
// tạo rồi đọc lại để so.
func TestDuongTaoKhongMatTruongNao(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	store := postgres.NewFulfillmentStore(pool)

	goc := thamSoDayDu(t)
	fo := domain.RestoreFulfillmentOrder(goc)

	if err := store.SaveBatch(ctx, []*domain.FulfillmentOrder{fo}); err != nil {
		t.Fatalf("lưu đơn thực hiện: %v", err)
	}

	docLai, err := store.FindByID(ctx, fo.ID(), fo.SellerID())
	if err != nil {
		t.Fatalf("đọc lại: %v", err)
	}

	soSanh(t, goc, rutRa(docLai), rongLucTao)
}

// TestDuongCapNhatKhongMatTruongNao canh câu UPDATE.
//
// Sau khi cập nhật, MỌI trường phải khứ hồi được — không có trường nào
// "chưa tới lúc" nữa.
func TestDuongCapNhatKhongMatTruongNao(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	store := postgres.NewFulfillmentStore(pool)

	goc := thamSoDayDu(t)

	// Tạo trước với version 0 — đúng thứ database đặt cho dòng mới.
	tao := goc
	tao.Version = 0
	if err := store.SaveBatch(ctx, []*domain.FulfillmentOrder{
		domain.RestoreFulfillmentOrder(tao),
	}); err != nil {
		t.Fatalf("lưu đơn thực hiện: %v", err)
	}

	if err := store.Update(ctx, domain.RestoreFulfillmentOrder(tao)); err != nil {
		t.Fatalf("cập nhật: %v", err)
	}

	docLai, err := store.FindByID(ctx, tao.ID, tao.SellerID)
	if err != nil {
		t.Fatalf("đọc lại: %v", err)
	}

	// Update tăng version, nên so với 1 chứ không phải 0.
	mong := tao
	mong.Version = 1
	soSanh(t, mong, rutRa(docLai), nil)
}

// thamSoDayDu dựng tham số với MỌI trường khác rỗng.
//
// Reflect ở cuối là phần quan trọng nhất: nó biến "quên điền trường mới"
// từ một bài test xanh giả thành một lỗi có tên.
func thamSoDayDu(t *testing.T) domain.RestoreFOParams {
	t.Helper()

	tien, err := money.New(500_000, money.VND)
	if err != nil {
		t.Fatalf("tiền: %v", err)
	}
	hoaHong, err := money.New(50_000, money.VND)
	if err != nil {
		t.Fatalf("hoa hồng: %v", err)
	}
	donGia, err := money.New(250_000, money.VND)
	if err != nil {
		t.Fatalf("đơn giá: %v", err)
	}

	lineID := ids.MustNew(ids.PrefixOrderLine)
	p := domain.RestoreFOParams{
		ID:           ids.MustNew(ids.PrefixFulfillmentOrder),
		ChoThanhToan: true,
		OrderID:      ids.MustNew(ids.PrefixOrder),
		// FONumber DUY NHẤT mỗi lần gọi: cột này có ràng buộc UNIQUE và
		// cả gói test dùng chung một database.
		FONumber:         "FC-2026-09-" + string(ids.MustNew(ids.PrefixOrder))[4:14] + "-A",
		SellerID:         ids.MustNew(ids.PrefixSeller),
		LineIDs:          []ids.ID{lineID},
		Status:           domain.FOHandedOver,
		Subtotal:         tien,
		CommissionAmount: hoaHong,
		CancelReason:     "khách đổi ý",
		FailureReason:    "không liên lạc được",
		CustomerID:       ids.MustNew(ids.PrefixCustomer),
		NotifyEmail:      "khach@example.com",
		NotifyPhone:      "0900111222",
		ShippingAddress: domain.ShippingAddress{
			RecipientName: "Người Nhận", Phone: "0900111222",
			StreetAddress: "1 Đường Thử", Ward: "Phường 1",
			District: "Quận 1", Province: "TP.HCM", CountryCode: "VN",
		},
		StockLocationID:  ids.MustNew(ids.PrefixStockLocation),
		Type:             domain.TypeSeller,
		ShippingMethod:   "EXPRESS",
		ShippingProvider: "GHN",
		TrackingNumber:   "GHN-KH-0001",
		// Ngày giao dự kiến lưu ở cột kiểu DATE — dựng nó ĐÃ CẮT về ngày
		// theo giờ nghiệp vụ, đúng như domain đặt. Đặt kèm giờ phút thì
		// bài test đỏ vì một sai lệch không có thật.
		EstimatedDelivery: types.DauNgay(luc.Add(72 * time.Hour)),
		CompletedAt:       luc.Add(96 * time.Hour),
		ConfirmedAt:       luc.Add(time.Hour),
		PackedAt:          luc.Add(2 * time.Hour),
		ShippedAt:         luc.Add(3 * time.Hour),
		DeliveredAt:       luc.Add(48 * time.Hour),
		CancelledAt:       luc.Add(50 * time.Hour),
		CreatedAt:         luc,
		UpdatedAt:         luc.Add(4 * time.Hour),
		Version:           1,
		Lines: []domain.FOLine{{
			OrderLineID:        lineID,
			SKUID:              ids.MustNew(ids.PrefixSKU),
			ProductName:        "Áo thun basic",
			VariantDescription: "Đen / M",
			Quantity:           2,
			UnitPrice:          donGia,
			LineTotal:          tien,
		}},
	}

	// HÀNG RÀO CHO CHÍNH BÀI KIỂM.
	//
	// Trường nào còn ở giá trị rỗng thì bài này không kiểm được nó: lưu 0
	// rồi đọc lại 0 luôn khớp, kể cả khi cột bị bỏ quên hoàn toàn.
	v := reflect.ValueOf(p)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Fatalf("trường %q còn RỖNG trong bộ tham số của bài kiểm — "+
				"điền một giá trị khác rỗng, nếu không cột tương ứng có bị "+
				"bỏ quên khỏi câu lệnh SQL thì bài này vẫn xanh",
				v.Type().Field(i).Name)
		}
	}
	return p
}

// rutRa dựng lại bộ tham số từ thực thể đã đọc.
//
// Đây LÀ một danh sách viết tay, và đó là chỗ yếu duy nhất còn lại. Nhưng
// nó hỏng theo hướng AN TOÀN: quên một trường ở đây nghĩa là giá trị rút
// ra bằng rỗng, trong khi bản gốc khác rỗng — nên bài test ĐỎ và chỉ đúng
// tên trường, thay vì im lặng bỏ qua.
func rutRa(f *domain.FulfillmentOrder) domain.RestoreFOParams {
	return domain.RestoreFOParams{
		ID:                f.ID(),
		ChoThanhToan:      f.ChoThanhToan(),
		OrderID:           f.OrderID(),
		FONumber:          f.FONumber(),
		SellerID:          f.SellerID(),
		LineIDs:           f.LineIDs(),
		Lines:             f.Lines(),
		Status:            f.Status(),
		Subtotal:          f.Subtotal(),
		CommissionAmount:  f.CommissionAmount(),
		CancelReason:      f.CancelReason(),
		FailureReason:     f.FailureReason(),
		CustomerID:        f.CustomerID(),
		NotifyEmail:       f.NotifyEmail(),
		NotifyPhone:       f.NotifyPhone(),
		ShippingAddress:   f.ShippingAddress(),
		StockLocationID:   f.StockLocationID(),
		Type:              f.Type(),
		ShippingMethod:    f.ShippingMethod(),
		ShippingProvider:  f.ShippingProvider(),
		TrackingNumber:    f.TrackingNumber(),
		EstimatedDelivery: f.EstimatedDelivery(),
		CompletedAt:       f.CompletedAt(),
		ConfirmedAt:       f.ConfirmedAt(),
		PackedAt:          f.PackedAt(),
		ShippedAt:         f.ShippedAt(),
		DeliveredAt:       f.DeliveredAt(),
		CancelledAt:       f.CancelledAt(),
		CreatedAt:         f.CreatedAt(),
		UpdatedAt:         f.UpdatedAt(),
		Version:           f.Version(),
	}
}

// theoNgay là những trường lưu ở cột kiểu DATE.
//
// Chúng chỉ mang ngày lịch, nên so theo thời điểm là so sai đơn vị.
var theoNgay = map[string]bool{"EstimatedDelivery": true}

// soSanh đối chiếu TỪNG trường và gọi tên trường lệch.
//
// `reflect.DeepEqual` trên cả struct chỉ nói "khác nhau" — với ba mươi
// trường thì đó là thông điệp gần như vô dụng.
func soSanh(t *testing.T, goc, sau domain.RestoreFOParams, boQua map[string]bool) {
	t.Helper()

	a := reflect.ValueOf(goc)
	b := reflect.ValueOf(sau)
	for i := 0; i < a.NumField(); i++ {
		ten := a.Type().Field(i).Name
		if boQua[ten] {
			continue
		}
		x, y := a.Field(i).Interface(), b.Field(i).Interface()

		if tx, ok := x.(time.Time); ok {
			ty := y.(time.Time)

			// Cột kiểu DATE chỉ mang NGÀY LỊCH, không mang thời điểm. So
			// theo mốc tuyệt đối thì nửa đêm giờ Việt Nam và nửa đêm UTC
			// là hai thời điểm cách nhau 7 tiếng — khác nhau, dù cùng một
			// ngày và cùng một ý nghĩa.
			if theoNgay[ten] {
				if types.DauNgay(tx) != types.DauNgay(ty) {
					t.Errorf("trường %s lệch NGÀY sau khi lưu rồi đọc lại: "+
						"%s → %s", ten,
						tx.In(types.MuiGioNghiepVu).Format("2006-01-02"),
						ty.In(types.MuiGioNghiepVu).Format("2006-01-02"))
				}
				continue
			}

			// Còn lại so theo mốc tuyệt đối: database trả về múi giờ khác
			// nhưng cùng thời điểm, và DeepEqual coi đó là khác nhau.
			if !tx.Equal(ty) {
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

// TestNgayGiaoDuKienKhongLechMotNgay.
//
// # Vùng nguy hiểm
//
// Bàn giao lúc 21:00 giờ Việt Nam là 14:00 CÙNG NGÀY theo UTC — không sao.
// Nhưng 02:00 sáng giờ Việt Nam lại là 19:00 NGÀY HÔM TRƯỚC theo UTC. Cắt
// ngày theo UTC nên gần một phần ba số giờ trong ngày cho ra ngày lịch
// khác với ngày khách nhìn thấy trên lịch của họ.
//
// Cột lưu có kiểu DATE, nên sai một ngày ở đây là sai một ngày trong lời
// hứa giao hàng — và nó im lặng.
func TestNgayGiaoDuKienKhongLechMotNgay(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	store := postgres.NewFulfillmentStore(pool)

	for _, tc := range []struct {
		ten      string
		banGiao  time.Time
		mongNgay string
	}{
		{
			// 02:00 ngày 12/09 giờ VN = 19:00 ngày 11/09 UTC.
			ten:      "rạng sáng — lệch ngày so với UTC",
			banGiao:  time.Date(2026, 9, 11, 19, 0, 0, 0, time.UTC),
			mongNgay: "2026-09-15", // 12/09 + 3 ngày
		},
		{
			// 21:00 ngày 11/09 giờ VN = 14:00 cùng ngày UTC.
			ten:      "tối muộn — cùng ngày với UTC",
			banGiao:  time.Date(2026, 9, 11, 14, 0, 0, 0, time.UTC),
			mongNgay: "2026-09-14",
		},
	} {
		t.Run(tc.ten, func(t *testing.T) {
			goc := thamSoDayDu(t)
			goc.Version = 0
			goc.Status = domain.FOPacked
			// Cổng thanh toán của ADR-0018 chặn mọi chuyển trạng thái
			// khác HỦY khi đơn chưa thu được tiền — đúng như thiết kế.
			goc.ChoThanhToan = false
			goc.ShippingMethod = string(domain.GiaoTieuChuan)

			fo := domain.RestoreFulfillmentOrder(goc)
			if err := store.SaveBatch(ctx, []*domain.FulfillmentOrder{fo}); err != nil {
				t.Fatalf("lưu: %v", err)
			}
			if err := fo.HandOver("GHN", "GHN-"+string(fo.ID())[4:14], domain.BieuPhiMacDinh, tc.banGiao); err != nil {
				t.Fatalf("bàn giao: %v", err)
			}
			if err := store.Update(ctx, fo); err != nil {
				t.Fatalf("cập nhật: %v", err)
			}

			docLai, err := store.FindByID(ctx, fo.ID(), fo.SellerID())
			if err != nil {
				t.Fatalf("đọc lại: %v", err)
			}

			got := docLai.EstimatedDelivery().
				In(types.MuiGioNghiepVu).Format("2006-01-02")
			if got != tc.mongNgay {
				t.Errorf("ngày giao dự kiến = %s, cần %s — khách đọc ngày "+
					"theo lịch Việt Nam, không theo UTC", got, tc.mongNgay)
			}
		})
	}
}
