package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/pricing/application"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func newService(t *testing.T) *application.Service {
	t.Helper()
	return application.NewInMemoryService(application.FixedClock{T: testNow})
}

func TestDatGiaVaTraGia(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	skuID := ids.MustNew(ids.PrefixSKU)

	if _, err := svc.SetPrice(ctx, application.SetPriceInput{
		SKUID:     skuID,
		PriceType: domain.PriceTypeBase,
		Amount:    vnd(490000),
		CompareAt: vnd(590000),
		Reason:    domain.ReasonInitial,
	}); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	got, err := svc.GetPrice(ctx, application.PriceQuery{SKUID: skuID})
	if err != nil {
		t.Fatalf("GetPrice: %v", err)
	}
	if got.Amount.Amount() != 490000 {
		t.Errorf("giá = %v, mong 490000", got.Amount)
	}
	if got.CompareAt.Amount() != 590000 {
		t.Errorf("giá gạch ngang = %v, mong 590000", got.CompareAt)
	}
	// 100000/590000 ≈ 16,94% → 1694 phần vạn.
	if got.DiscountBP != 1694 {
		t.Errorf("mức giảm = %d bp, mong 1694", got.DiscountBP)
	}
}

// SKU chưa có giá phải trả ErrNoPrice, không phải giá 0.
//
// Trả giá 0 sẽ hiển thị "miễn phí" trên trang và khách đặt được hàng
// không mất tiền.
func TestChuaCoGiaTraLoiChuKhongTraKhong(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	got, err := svc.GetPrice(ctx, application.PriceQuery{SKUID: ids.MustNew(ids.PrefixSKU)})
	if !errors.Is(err, application.ErrNoPrice) {
		t.Errorf("lỗi = %v, mong ErrNoPrice", err)
	}
	if !got.Amount.IsZero() {
		t.Errorf("giá = %v khi có lỗi", got.Amount)
	}
}

// Mọi giá đã hết hạn cũng phải trả ErrNoPrice — không được rơi về giá cũ.
func TestGiaHetHanHetThiTraLoi(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	skuID := ids.MustNew(ids.PrefixSKU)

	if _, err := svc.SetPrice(ctx, application.SetPriceInput{
		SKUID:     skuID,
		PriceType: domain.PriceTypeFlash,
		Amount:    vnd(290000),
		Period: domain.Period{
			From: testNow.Add(-2 * time.Hour),
			To:   testNow.Add(-time.Hour),
		},
	}); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	if _, err := svc.GetPrice(ctx, application.PriceQuery{SKUID: skuID}); !errors.Is(err, application.ErrNoPrice) {
		t.Errorf("lỗi = %v, mong ErrNoPrice", err)
	}
}

// Quy tắc 3: MỌI thay đổi giá ghi vào lịch sử. Ghi lịch sử phải nằm trong
// use case, không phải việc bên gọi phải nhớ làm.
func TestMoiThayDoiGiaDeuGhiLichSu(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	skuID := ids.MustNew(ids.PrefixSKU)

	amounts := []int64{500000, 450000, 400000}
	for _, amount := range amounts {
		if _, err := svc.SetPrice(ctx, application.SetPriceInput{
			SKUID:  skuID,
			Amount: vnd(amount),
			Reason: domain.ReasonManual,
		}); err != nil {
			t.Fatalf("SetPrice %d: %v", amount, err)
		}
	}

	points, err := svc.GetHistory(ctx, skuID, domain.DateRange{})
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(points) != len(amounts) {
		t.Fatalf("số điểm lịch sử = %d, mong %d", len(points), len(amounts))
	}
	// Mỗi điểm phải có lý do — thiếu lý do thì không rà soát được.
	for i, p := range points {
		if p.Reason() == "" {
			t.Errorf("điểm %d thiếu lý do", i)
		}
	}
}

// Giá bị từ chối thì KHÔNG được để lại vết trong lịch sử — lịch sử phải
// phản ánh giá đã thực sự áp dụng.
func TestGiaKhongHopLeKhongGhiLichSu(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	skuID := ids.MustNew(ids.PrefixSKU)

	if _, err := svc.SetPrice(ctx, application.SetPriceInput{
		SKUID: skuID, Amount: vnd(0),
	}); !errors.Is(err, domain.ErrNonPositivePrice) {
		t.Fatalf("lỗi = %v, mong ErrNonPositivePrice", err)
	}

	points, err := svc.GetHistory(ctx, skuID, domain.DateRange{})
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("có %d điểm lịch sử dù giá bị từ chối", len(points))
	}
}

// "Giá thấp nhất 30 ngày qua" — con số bắt buộc công bố ở một số thị
// trường khi quảng cáo giảm giá.
func TestGiaThapNhat30Ngay(t *testing.T) {
	ctx := context.Background()
	skuID := ids.MustNew(ids.PrefixSKU)

	// Đồng hồ dịch chuyển được để tạo lịch sử trải theo thời gian — không
	// có nó thì mọi điểm lịch sử cùng một mốc và test vô nghĩa.
	clock := &movingClock{t: testNow.AddDate(0, 0, -40)}
	svc := application.NewInMemoryService(clock)

	steps := []struct {
		daysAgo int
		amount  int64
	}{
		{40, 300000}, // ngoài 30 ngày — KHÔNG được tính
		{25, 500000},
		{10, 420000},
		{2, 450000},
	}
	for _, st := range steps {
		clock.t = testNow.AddDate(0, 0, -st.daysAgo)
		if _, err := svc.SetPrice(ctx, application.SetPriceInput{
			SKUID: skuID, Amount: vnd(st.amount), Reason: domain.ReasonManual,
		}); err != nil {
			t.Fatalf("SetPrice: %v", err)
		}
	}

	clock.t = testNow
	lowest, ok, err := svc.LowestPriceLast30Days(ctx, skuID)
	if err != nil {
		t.Fatalf("LowestPriceLast30Days: %v", err)
	}
	if !ok {
		t.Fatal("không tìm được giá thấp nhất")
	}
	// 300000 nằm NGOÀI 30 ngày nên không tính; thấp nhất trong khoảng là 420000.
	if lowest.Amount() != 420000 {
		t.Errorf("giá thấp nhất 30 ngày = %v, mong 420000", lowest)
	}
}

type movingClock struct{ t time.Time }

func (c *movingClock) Now() time.Time { return c.t }

// Đổi giá rồi hỏi NGAY "giá thấp nhất 30 ngày" phải ra kết quả.
//
// Biên To của DateRange là biên MỞ, nên nếu khoảng truy vấn kết thúc đúng
// tại thời điểm hiện tại thì điểm lịch sử vừa ghi bị loại — và câu trả lời
// thành "chưa có dữ liệu". Với nghĩa vụ minh bạch giá, đó là câu trả lời
// không được phép sai.
//
// Test này từng thất bại chập chờn: nó chỉ lộ ra khi đồng hồ trả về đúng
// cùng một thời điểm cho cả lúc ghi lẫn lúc đọc.
func TestDoiGiaRoiHoiNgayVanRaKetQua(t *testing.T) {
	ctx := context.Background()
	skuID := ids.MustNew(ids.PrefixSKU)

	// Đồng hồ ĐỨNG YÊN: ghi và đọc ở cùng một thời điểm chính xác — đây là
	// trường hợp biên mà đồng hồ hệ thống chỉ thỉnh thoảng mới tạo ra.
	svc := application.NewInMemoryService(application.FixedClock{T: testNow})

	if _, err := svc.SetPrice(ctx, application.SetPriceInput{
		SKUID: skuID, Amount: vnd(299000), Reason: domain.ReasonSeasonEnd,
	}); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	lowest, ok, err := svc.LowestPriceLast30Days(ctx, skuID)
	if err != nil {
		t.Fatalf("LowestPriceLast30Days: %v", err)
	}
	if !ok {
		t.Fatal("giá vừa đặt bị loại khỏi khoảng 30 ngày — biên To đang là biên mở")
	}
	if lowest.Amount() != 299000 {
		t.Errorf("giá thấp nhất = %v, mong 299000", lowest)
	}
}

// Tra giá theo LÔ: hiển thị 50 sản phẩm là 1 lời gọi, không phải 50.
func TestTraGiaTheoLo(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	sku1 := ids.MustNew(ids.PrefixSKU)
	sku2 := ids.MustNew(ids.PrefixSKU)
	khongCoGia := ids.MustNew(ids.PrefixSKU)

	for _, tc := range []struct {
		skuID  ids.ID
		amount int64
	}{{sku1, 100000}, {sku2, 200000}} {
		if _, err := svc.SetPrice(ctx, application.SetPriceInput{
			SKUID: tc.skuID, Amount: vnd(tc.amount),
		}); err != nil {
			t.Fatalf("SetPrice: %v", err)
		}
	}

	got, err := svc.GetPrices(ctx, []application.PriceQuery{
		{SKUID: sku1}, {SKUID: sku2}, {SKUID: khongCoGia},
	})
	if err != nil {
		t.Fatalf("GetPrices: %v", err)
	}
	// SKU không có giá bị BỎ QUA, không làm hỏng cả lời gọi.
	if len(got) != 2 {
		t.Errorf("số kết quả = %d, mong 2", len(got))
	}
	if got[sku1].Amount.Amount() != 100000 {
		t.Errorf("giá sku1 = %v, mong 100000", got[sku1].Amount)
	}
	if _, có := got[khongCoGia]; có {
		t.Error("SKU không có giá không được xuất hiện trong kết quả")
	}
}

// Tra giá theo lô phải cho CÙNG kết quả với tra từng cái — nếu lệch, trang
// danh sách và trang chi tiết sẽ hiển thị giá khác nhau.
func TestTraGiaTheoLoKhopVoiTraTungCai(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	skuID := ids.MustNew(ids.PrefixSKU)
	campaignID := ids.MustNew(ids.PrefixCampaign)

	// Nhiều loại giá cùng lúc để việc chọn không tầm thường.
	for _, in := range []application.SetPriceInput{
		{SKUID: skuID, PriceType: domain.PriceTypeBase, Amount: vnd(500000)},
		{SKUID: skuID, PriceType: domain.PriceTypeMember, Amount: vnd(450000), CustomerTier: "GOLD"},
		{
			SKUID: skuID, PriceType: domain.PriceTypeCampaign, Amount: vnd(400000),
			CampaignID: campaignID,
			Period:     domain.Period{From: testNow.Add(-time.Hour), To: testNow.Add(time.Hour)},
		},
	} {
		if _, err := svc.SetPrice(ctx, in); err != nil {
			t.Fatalf("SetPrice: %v", err)
		}
	}

	queries := []application.PriceQuery{
		{SKUID: skuID},
		{SKUID: skuID, CustomerTier: "GOLD"},
		{SKUID: skuID, CustomerTier: "GOLD", CampaignID: campaignID},
	}

	for _, q := range queries {
		single, err := svc.GetPrice(ctx, q)
		if err != nil {
			t.Fatalf("GetPrice: %v", err)
		}
		batch, err := svc.GetPrices(ctx, []application.PriceQuery{q})
		if err != nil {
			t.Fatalf("GetPrices: %v", err)
		}
		if batch[q.SKUID].Amount.Amount() != single.Amount.Amount() {
			t.Errorf("hạng %q chiến dịch %q: lô = %v, đơn = %v",
				q.CustomerTier, q.CampaignID, batch[q.SKUID].Amount, single.Amount)
		}
		if batch[q.SKUID].PriceType != single.PriceType {
			t.Errorf("loại giá lệch: lô = %v, đơn = %v", batch[q.SKUID].PriceType, single.PriceType)
		}
	}
}

// Quy tắc 4: giá seller phải trong khung ràng buộc.
func TestKiemTraGiaSeller(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	skuID := ids.MustNew(ids.PrefixSKU)

	if _, err := svc.SetConstraint(ctx, application.SetConstraintInput{
		SKUID:    skuID,
		MinPrice: vnd(100000),
		MaxPrice: vnd(500000),
	}); err != nil {
		t.Fatalf("SetConstraint: %v", err)
	}

	cases := []struct {
		ten    string
		gia    int64
		mongOK bool
	}{
		{"trong khung", 300000, true},
		{"gõ thiếu số 0", 30000, false},
		{"thổi giá", 900000, false},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			got, err := svc.ValidateSellerPrice(ctx, skuID, vnd(tc.gia))
			if err != nil {
				t.Fatalf("ValidateSellerPrice: %v", err)
			}
			if got.Allowed != tc.mongOK {
				t.Errorf("Allowed = %v, mong %v (%s)", got.Allowed, tc.mongOK, got.Message)
			}
		})
	}
}

// SKU chưa có khung giá thì chấp nhận mọi giá DƯƠNG.
//
// Chặn hết khi chưa cấu hình sẽ khiến không seller nào đăng bán được cho
// tới khi nền tảng cấu hình xong từng SKU — không khả thi.
func TestChuaCoKhungGiaThiChapNhanGiaDuong(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	skuID := ids.MustNew(ids.PrefixSKU)

	got, err := svc.ValidateSellerPrice(ctx, skuID, vnd(123456))
	if err != nil {
		t.Fatalf("ValidateSellerPrice: %v", err)
	}
	if !got.Allowed {
		t.Errorf("SKU chưa có khung giá phải chấp nhận giá dương (%s)", got.Message)
	}

	// Nhưng giá 0 hoặc âm thì vẫn phải chặn.
	for _, gia := range []int64{0, -1000} {
		got, err := svc.ValidateSellerPrice(ctx, skuID, vnd(gia))
		if err != nil {
			t.Fatalf("ValidateSellerPrice: %v", err)
		}
		if got.Allowed {
			t.Errorf("giá %d phải bị chặn dù chưa có khung giá", gia)
		}
		if got.Code != domain.ViolationNotPositive {
			t.Errorf("giá %d: Code = %q, mong PRICE_NOT_POSITIVE", gia, got.Code)
		}
	}
}

func TestNgungApDungGia(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	skuID := ids.MustNew(ids.PrefixSKU)

	base, err := svc.SetPrice(ctx, application.SetPriceInput{
		SKUID: skuID, PriceType: domain.PriceTypeBase, Amount: vnd(500000),
	})
	if err != nil {
		t.Fatalf("SetPrice: %v", err)
	}
	clearance, err := svc.SetPrice(ctx, application.SetPriceInput{
		SKUID: skuID, PriceType: domain.PriceTypeClearance, Amount: vnd(250000),
	})
	if err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	// Đang áp giá xả hàng.
	got, err := svc.GetPrice(ctx, application.PriceQuery{SKUID: skuID})
	if err != nil {
		t.Fatalf("GetPrice: %v", err)
	}
	if got.PriceType != domain.PriceTypeClearance {
		t.Fatalf("loại giá = %v, mong CLEARANCE", got.PriceType)
	}

	// Ngừng giá xả hàng → quay về giá gốc.
	if _, err := svc.DeactivatePrice(ctx, clearance.ID()); err != nil {
		t.Fatalf("DeactivatePrice: %v", err)
	}
	got, err = svc.GetPrice(ctx, application.PriceQuery{SKUID: skuID})
	if err != nil {
		t.Fatalf("GetPrice: %v", err)
	}
	if got.PriceType != domain.PriceTypeBase || got.Amount.Amount() != 500000 {
		t.Errorf("sau khi ngừng giá xả: %v %v, mong BASE 500000", got.PriceType, got.Amount)
	}
	_ = base
}
