package domain_test

import (
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
)

var testNow = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func newOffer(t *testing.T, price int64, handling int) *domain.Offer {
	t.Helper()
	o, err := domain.NewOffer(domain.NewOfferParams{
		SKUID:             ids.MustNew(ids.PrefixSKU),
		SellerID:          ids.MustNew(ids.PrefixSeller),
		Price:             vnd(price),
		HandlingTimeHours: handling,
		Now:               testNow,
	})
	if err != nil {
		t.Fatalf("NewOffer: %v", err)
	}
	if err := o.Activate(testNow); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	return o
}

func candidate(o *domain.Offer, active bool, perf int) domain.BuyBoxCandidate {
	return domain.BuyBoxCandidate{Offer: o, SellerActive: active, PerformanceScore: perf}
}

// RÀNG BUỘC BẮT BUỘC (mục 4): seller không hoạt động thì offer KHÔNG được
// thắng buy box, kể cả khi giá tốt nhất.
func TestSellerKhongHoatDongKhongThangBuyBox(t *testing.T) {
	reOnhat := newOffer(t, 100000, 24) // giá tốt nhất
	datHon := newOffer(t, 200000, 24)

	got := domain.SelectBuyBox([]domain.BuyBoxCandidate{
		candidate(reOnhat, false, 100), // seller bị đình chỉ
		candidate(datHon, true, 50),
	}, domain.DefaultWeights)

	if got.Winner == nil {
		t.Fatal("phải có offer thắng")
	}
	if got.Winner.ID() != datHon.ID() {
		t.Error("offer của seller bị đình chỉ đã thắng buy box dù giá tốt hơn")
	}
}

// Offer không bán được (hết hàng, đình chỉ) cũng không thắng.
func TestChiOfferBanDuocMoiThangBuyBox(t *testing.T) {
	hetHang := newOffer(t, 100000, 24)
	if err := hetHang.MarkOutOfStock(testNow); err != nil {
		t.Fatalf("MarkOutOfStock: %v", err)
	}
	conHang := newOffer(t, 300000, 24)

	got := domain.SelectBuyBox([]domain.BuyBoxCandidate{
		candidate(hetHang, true, 100),
		candidate(conHang, true, 50),
	}, domain.DefaultWeights)

	if got.Winner == nil || got.Winner.ID() != conHang.ID() {
		t.Error("offer hết hàng đã thắng buy box")
	}
}

// Không ứng viên nào hợp lệ thì trả nil, KHÔNG panic.
func TestKhongUngVienHopLeTraNil(t *testing.T) {
	dinhChi := newOffer(t, 100000, 24)

	for _, ten := range []string{"danh sách rỗng", "toàn seller đình chỉ", "toàn nil"} {
		var list []domain.BuyBoxCandidate
		switch ten {
		case "toàn seller đình chỉ":
			list = []domain.BuyBoxCandidate{candidate(dinhChi, false, 50)}
		case "toàn nil":
			list = []domain.BuyBoxCandidate{{Offer: nil, SellerActive: true}}
		}
		if got := domain.SelectBuyBox(list, domain.DefaultWeights); got.Winner != nil {
			t.Errorf("%s: mong nil, nhận %v", ten, got.Winner.ID())
		}
	}
}

// CẢNH BÁO VỀ CẠNH TRANH GIÁ (mục 4): nếu buy box CHỈ dựa vào giá thấp
// nhất, seller đua giảm giá tới mức không bền vững và cắt giảm chất lượng
// dịch vụ.
//
// Test này bảo vệ nguyên tắc đó: một offer đắt hơn nhưng giao nhanh hơn và
// hiệu suất tốt hơn PHẢI thắng được offer giá rẻ nhất.
func TestGiaRePhaiKhongDuDeThangBuyBox(t *testing.T) {
	// Rẻ hơn 10% nhưng giao chậm gấp 5 lần và hiệu suất kém.
	reNhungTe := newOffer(t, 90000, 120)
	datNhungTot := newOffer(t, 100000, 24)

	got := domain.SelectBuyBox([]domain.BuyBoxCandidate{
		candidate(reNhungTe, true, 10),
		candidate(datNhungTot, true, 100),
	}, domain.DefaultWeights)

	if got.Winner == nil {
		t.Fatal("phải có offer thắng")
	}
	if got.Winner.ID() != datNhungTot.ID() {
		t.Error("buy box chỉ nhìn giá — seller sẽ đua giảm giá và cắt chất lượng dịch vụ")
	}
}

// Cùng chất lượng thì giá THẤP hơn thắng — khách được lợi.
func TestCungChatLuongThiGiaThapThang(t *testing.T) {
	re := newOffer(t, 100000, 24)
	dat := newOffer(t, 200000, 24)

	got := domain.SelectBuyBox([]domain.BuyBoxCandidate{
		candidate(dat, true, 50),
		candidate(re, true, 50),
	}, domain.DefaultWeights)

	if got.Winner == nil || got.Winner.ID() != re.ID() {
		t.Error("cùng chất lượng nhưng giá cao hơn lại thắng")
	}
}

// Kết quả phải ỔN ĐỊNH: buy box nhảy qua lại giữa hai offer ngang điểm sẽ
// làm khách thấy giá đổi liên tục khi tải lại trang.
func TestKetQuaBuyBoxOnDinh(t *testing.T) {
	a := newOffer(t, 100000, 24)
	b := newOffer(t, 100000, 24)
	c := newOffer(t, 100000, 24)

	list := []domain.BuyBoxCandidate{
		candidate(a, true, 50), candidate(b, true, 50), candidate(c, true, 50),
	}

	first := domain.SelectBuyBox(list, domain.DefaultWeights)
	for i := 0; i < 50; i++ {
		got := domain.SelectBuyBox(list, domain.DefaultWeights)
		if got.Winner.ID() != first.Winner.ID() {
			t.Fatalf("kết quả đổi giữa các lần gọi: %s rồi %s",
				first.Winner.ID(), got.Winner.ID())
		}
	}
}

// Điểm được trả ra để seller hiểu vì sao mình không thắng.
//
// Mô hình hộp đen tạo tranh chấp không giải quyết được và cảm giác bất
// công — dẫn tới seller rời nền tảng.
func TestTraVeDiemVaSoOfferCanhTranh(t *testing.T) {
	a := newOffer(t, 100000, 24)
	b := newOffer(t, 150000, 48)
	c := newOffer(t, 200000, 72)

	got := domain.SelectBuyBox([]domain.BuyBoxCandidate{
		candidate(a, true, 80), candidate(b, true, 60), candidate(c, true, 40),
	}, domain.DefaultWeights)

	if got.Winner == nil {
		t.Fatal("phải có offer thắng")
	}
	if got.Score <= 0 || got.Score > 100 {
		t.Errorf("điểm = %d, mong trong khoảng (0, 100]", got.Score)
	}
	if got.OtherCount != 2 {
		t.Errorf("số offer cạnh tranh = %d, mong 2", got.OtherCount)
	}
}

// Chỉ có MỘT offer thì nó thắng với điểm tối đa — không bị phạt vì thiếu
// đối thủ để so sánh.
func TestMotOfferDuyNhatThangVoiDiemToiDa(t *testing.T) {
	only := newOffer(t, 500000, 72)

	got := domain.SelectBuyBox([]domain.BuyBoxCandidate{
		candidate(only, true, 100),
	}, domain.DefaultWeights)

	if got.Winner == nil || got.Winner.ID() != only.ID() {
		t.Fatal("offer duy nhất phải thắng")
	}
	if got.Score != 100 {
		t.Errorf("điểm = %d, mong 100 — không có đối thủ thì không bị phạt", got.Score)
	}
	if got.OtherCount != 0 {
		t.Errorf("số offer cạnh tranh = %d, mong 0", got.OtherCount)
	}
}
