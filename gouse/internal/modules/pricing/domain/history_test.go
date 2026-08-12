package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
)

func newPoint(t *testing.T, skuID ids.ID, amount int64, at time.Time) *domain.PricePoint {
	t.Helper()
	p, err := domain.NewPricePoint(domain.NewPricePointParams{
		SKUID:  skuID,
		Amount: vnd(amount),
		Reason: domain.ReasonManual,
		Now:    at,
	})
	if err != nil {
		t.Fatalf("NewPricePoint: %v", err)
	}
	return p
}

func TestGhiDiemLichSuGia(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)
	p := newPoint(t, skuID, 490000, testNow)

	if p.SKUID() != skuID {
		t.Errorf("skuID = %q, mong %q", p.SKUID(), skuID)
	}
	if !p.RecordedAt().Equal(testNow) {
		t.Errorf("recordedAt = %v, mong %v", p.RecordedAt(), testNow)
	}
	// Lý do là bắt buộc: rà soát thao túng giá cần biết VÌ SAO mỗi lần đổi.
	if p.Reason() == "" {
		t.Error("điểm lịch sử phải có lý do")
	}
}

func TestDiemLichSuTuChoiDuLieuSai(t *testing.T) {
	if _, err := domain.NewPricePoint(domain.NewPricePointParams{
		Amount: vnd(1000), Now: testNow,
	}); !errors.Is(err, domain.ErrMissingSKU) {
		t.Errorf("lỗi = %v, mong ErrMissingSKU", err)
	}

	if _, err := domain.NewPricePoint(domain.NewPricePointParams{
		SKUID: ids.MustNew(ids.PrefixSKU), Amount: vnd(0), Now: testNow,
	}); !errors.Is(err, domain.ErrNonPositivePrice) {
		t.Errorf("lỗi = %v, mong ErrNonPositivePrice", err)
	}
}

// Lý do mặc định là MANUAL, không được để rỗng — điểm lịch sử không có lý
// do thì vô dụng cho việc rà soát.
func TestLyDoMacDinh(t *testing.T) {
	p, err := domain.NewPricePoint(domain.NewPricePointParams{
		SKUID: ids.MustNew(ids.PrefixSKU), Amount: vnd(1000), Now: testNow,
	})
	if err != nil {
		t.Fatalf("NewPricePoint: %v", err)
	}
	if p.Reason() != domain.ReasonManual {
		t.Errorf("lý do = %q, mong MANUAL", p.Reason())
	}
}

// "Giá thấp nhất 30 ngày qua" là con số một số thị trường BẮT BUỘC công bố
// khi quảng cáo giảm giá. Không có lịch sử thì không trả lời được, và dữ
// liệu này KHÔNG tạo ngược được.
func TestTimGiaThapNhatTrongKhoang(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)

	points := []*domain.PricePoint{
		newPoint(t, skuID, 500000, testNow.AddDate(0, 0, -45)), // ngoài khoảng 30 ngày
		newPoint(t, skuID, 300000, testNow.AddDate(0, 0, -20)),
		newPoint(t, skuID, 450000, testNow.AddDate(0, 0, -10)),
		newPoint(t, skuID, 400000, testNow.AddDate(0, 0, -2)),
	}

	r := domain.DateRange{From: testNow.AddDate(0, 0, -30), To: testNow}
	got, ok := domain.LowestIn(points, r)
	if !ok {
		t.Fatal("không tìm được giá thấp nhất")
	}
	// 300000 là thấp nhất TRONG 30 ngày; 500000 nằm ngoài khoảng nên
	// không được tính vào.
	if got.Amount() != 300000 {
		t.Errorf("giá thấp nhất = %v, mong 300000", got)
	}

	// Khoảng không có điểm nào.
	trong := domain.DateRange{
		From: testNow.AddDate(0, 0, -100),
		To:   testNow.AddDate(0, 0, -90),
	}
	if _, ok := domain.LowestIn(points, trong); ok {
		t.Error("khoảng rỗng phải trả false")
	}

	// Danh sách rỗng và chứa nil không được panic.
	if _, ok := domain.LowestIn(nil, r); ok {
		t.Error("danh sách rỗng phải trả false")
	}
	if _, ok := domain.LowestIn([]*domain.PricePoint{nil}, r); ok {
		t.Error("danh sách toàn nil phải trả false")
	}
}

// Kịch bản thao túng giá: tăng giá rồi giảm ngay trước khuyến mãi để
// quảng cáo "giảm 50%". Lịch sử phải làm lộ ra hành vi này.
func TestLichSuPhatHienThaoTungGia(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)

	points := []*domain.PricePoint{
		// Giá thật suốt gần một tháng.
		newPoint(t, skuID, 400000, testNow.AddDate(0, 0, -28)),
		// Đột ngột nâng lên trước đợt khuyến mãi.
		newPoint(t, skuID, 800000, testNow.AddDate(0, 0, -3)),
		// Rồi "giảm 50%" — thực chất chỉ về giá cũ.
		newPoint(t, skuID, 400000, testNow.AddDate(0, 0, -1)),
	}

	r := domain.DateRange{From: testNow.AddDate(0, 0, -30), To: testNow}
	lowest, ok := domain.LowestIn(points, r)
	if !ok {
		t.Fatal("không tìm được giá thấp nhất")
	}

	// Giá thấp nhất 30 ngày là 400.000đ — BẰNG giá "khuyến mãi".
	// Quảng cáo "giảm 50% còn 400.000đ" là sai sự thật, và con số này
	// chứng minh điều đó.
	if lowest.Amount() != 400000 {
		t.Errorf("giá thấp nhất 30 ngày = %v, mong 400000", lowest)
	}
}

// Duyệt map và sắp xếp không ổn định làm lịch sử hiển thị khác nhau mỗi
// lần tải trang.
func TestSapXepLichSuOnDinh(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)

	// Cố ý đưa vào theo thứ tự lộn xộn, có hai điểm CÙNG thời điểm.
	cungLuc := testNow.AddDate(0, 0, -5)
	points := []*domain.PricePoint{
		newPoint(t, skuID, 300000, testNow.AddDate(0, 0, -1)),
		newPoint(t, skuID, 500000, testNow.AddDate(0, 0, -10)),
		newPoint(t, skuID, 400000, cungLuc),
		newPoint(t, skuID, 450000, cungLuc),
	}

	first := domain.SortByTime(points)
	for i := 0; i < 50; i++ {
		got := domain.SortByTime(points)
		for j := range got {
			if got[j].ID() != first[j].ID() {
				t.Fatalf("thứ tự đổi giữa các lần gọi ở vị trí %d", j)
			}
		}
	}

	// Phải tăng dần theo thời gian.
	for i := 1; i < len(first); i++ {
		if first[i].RecordedAt().Before(first[i-1].RecordedAt()) {
			t.Errorf("vị trí %d sớm hơn vị trí trước", i)
		}
	}

	// KHÔNG được sửa lát cắt đầu vào — bên gọi thường đang dùng nó cho
	// việc khác.
	if points[0].Amount().Amount() != 300000 {
		t.Error("SortByTime đã sửa lát cắt đầu vào")
	}
}

func TestBienKhoangTruyVanLichSu(t *testing.T) {
	r := domain.DateRange{From: testNow, To: testNow.Add(2 * time.Hour)}

	cases := []struct {
		ten  string
		t    time.Time
		mong bool
	}{
		{"trước From", testNow.Add(-time.Second), false},
		{"đúng From", testNow, true},
		{"đúng To", testNow.Add(2 * time.Hour), false},
	}
	for _, tc := range cases {
		if got := r.Contains(tc.t); got != tc.mong {
			t.Errorf("%s: Contains = %v, mong %v", tc.ten, got, tc.mong)
		}
	}

	// Khoảng rỗng chứa mọi thời điểm.
	var moHai domain.DateRange
	if !moHai.Contains(testNow) {
		t.Error("khoảng rỗng phải chứa mọi thời điểm")
	}
}
