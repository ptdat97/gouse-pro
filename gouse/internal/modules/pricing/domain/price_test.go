package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
)

var testNow = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func newPrice(t *testing.T, mutate func(*domain.NewPriceParams)) *domain.Price {
	t.Helper()
	p := domain.NewPriceParams{
		SKUID:     ids.MustNew(ids.PrefixSKU),
		PriceType: domain.PriceTypeBase,
		Amount:    vnd(490000),
		Now:       testNow,
	}
	if mutate != nil {
		mutate(&p)
	}
	got, err := domain.NewPrice(p)
	if err != nil {
		t.Fatalf("NewPrice: %v", err)
	}
	return got
}

// Quy tắc 1: giá > 0. Giá 0 gần như luôn là lỗi nhập liệu hoặc lỗi chuyển
// đổi đơn vị tiền tệ, không phải "miễn phí".
func TestGiaPhaiLonHonKhong(t *testing.T) {
	for _, amount := range []int64{0, -1, -490000} {
		_, err := domain.NewPrice(domain.NewPriceParams{
			SKUID:  ids.MustNew(ids.PrefixSKU),
			Amount: vnd(amount),
			Now:    testNow,
		})
		if !errors.Is(err, domain.ErrNonPositivePrice) {
			t.Errorf("giá %d: lỗi = %v, mong ErrNonPositivePrice", amount, err)
		}
	}
}

func TestThieuSKUBiTuChoi(t *testing.T) {
	_, err := domain.NewPrice(domain.NewPriceParams{Amount: vnd(1000), Now: testNow})
	if !errors.Is(err, domain.ErrMissingSKU) {
		t.Errorf("lỗi = %v, mong ErrMissingSKU", err)
	}
}

// Giá gạch ngang thấp hơn giá bán sẽ hiển thị "giảm -20%" trên trang.
func TestGiaGachNgangPhaiCaoHonGiaBan(t *testing.T) {
	cases := []struct {
		ten       string
		amount    int64
		compareAt int64
		wantErr   error
	}{
		{"gạch ngang thấp hơn", 490000, 390000, domain.ErrCompareAtTooLow},
		{"gạch ngang bằng giá bán", 490000, 490000, domain.ErrCompareAtTooLow},
		{"gạch ngang cao hơn — hợp lệ", 390000, 490000, nil},
	}

	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			_, err := domain.NewPrice(domain.NewPriceParams{
				SKUID:     ids.MustNew(ids.PrefixSKU),
				Amount:    vnd(tc.amount),
				CompareAt: vnd(tc.compareAt),
				Now:       testNow,
			})
			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("lỗi = %v, mong không lỗi", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("lỗi = %v, mong %v", err, tc.wantErr)
			}
		})
	}
}

// Quy tắc 5: giá luôn kèm đơn vị tiền tệ. So sánh khác đơn vị cho kết quả
// sai một cách âm thầm.
func TestKhacDonViTienTeBiTuChoi(t *testing.T) {
	_, err := domain.NewPrice(domain.NewPriceParams{
		SKUID:     ids.MustNew(ids.PrefixSKU),
		Amount:    vnd(490000),
		CompareAt: money.MustNew(2000, money.USD),
		Now:       testNow,
	})
	if !errors.Is(err, domain.ErrCurrencyMismatch) {
		t.Errorf("lỗi = %v, mong ErrCurrencyMismatch", err)
	}
}

// Giá flash quên tắt sẽ bán lỗ vô hạn — loại lỗi không ai phát hiện cho
// tới khi đối soát cuối tháng.
func TestGiaFlashVaChienDichPhaiCoThoiHan(t *testing.T) {
	for _, pt := range []domain.PriceType{domain.PriceTypeFlash, domain.PriceTypeCampaign} {
		t.Run(string(pt), func(t *testing.T) {
			if !pt.RequiresPeriod() {
				t.Fatalf("%s phải bắt buộc thời hạn", pt)
			}

			// Không có thời hạn kết thúc → bị chặn.
			_, err := domain.NewPrice(domain.NewPriceParams{
				SKUID: ids.MustNew(ids.PrefixSKU), PriceType: pt,
				Amount: vnd(290000), Now: testNow,
			})
			if err == nil {
				t.Error("mong lỗi khi giá flash/chiến dịch không có thời hạn")
			}

			// Có thời hạn → hợp lệ.
			if _, err := domain.NewPrice(domain.NewPriceParams{
				SKUID: ids.MustNew(ids.PrefixSKU), PriceType: pt,
				Amount: vnd(290000),
				Period: domain.Period{From: testNow, To: testNow.Add(4 * time.Hour)},
				Now:    testNow,
			}); err != nil {
				t.Errorf("có thời hạn vẫn lỗi: %v", err)
			}
		})
	}

	// Giá gốc thì được vô thời hạn.
	if domain.PriceTypeBase.RequiresPeriod() {
		t.Error("giá gốc không được bắt buộc thời hạn")
	}
}

func TestKhoangThoiGianKhongHopLe(t *testing.T) {
	_, err := domain.NewPrice(domain.NewPriceParams{
		SKUID:  ids.MustNew(ids.PrefixSKU),
		Amount: vnd(490000),
		Period: domain.Period{From: testNow.Add(time.Hour), To: testNow},
		Now:    testNow,
	})
	if !errors.Is(err, domain.ErrInvalidPeriod) {
		t.Errorf("lỗi = %v, mong ErrInvalidPeriod", err)
	}
}

// Biên: bao gồm From, KHÔNG bao gồm To. Hai mức giá liền kề không được
// cùng hiệu lực tại một thời điểm.
func TestBienKhoangThoiGian(t *testing.T) {
	p := domain.Period{From: testNow, To: testNow.Add(2 * time.Hour)}

	cases := []struct {
		ten  string
		t    time.Time
		mong bool
	}{
		{"trước From", testNow.Add(-time.Second), false},
		{"đúng From", testNow, true},
		{"giữa khoảng", testNow.Add(time.Hour), true},
		{"đúng To", testNow.Add(2 * time.Hour), false},
		{"sau To", testNow.Add(3 * time.Hour), false},
	}
	for _, tc := range cases {
		if got := p.Contains(tc.t); got != tc.mong {
			t.Errorf("%s: Contains = %v, mong %v", tc.ten, got, tc.mong)
		}
	}

	// Khoảng mở hai đầu chứa mọi thời điểm.
	var moHai domain.Period
	if !moHai.Contains(testNow) || !moHai.IsOpenEnded() {
		t.Error("khoảng rỗng phải chứa mọi thời điểm và là vô thời hạn")
	}
}

// Quy tắc 2: CHỈ MỘT giá được áp dụng. Thứ tự Flash > Campaign > Clearance
// > Member > Base.
func TestThuTuUuTienGia(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)
	campaignID := ids.MustNew(ids.PrefixCampaign)
	kỳ := domain.Period{From: testNow.Add(-time.Hour), To: testNow.Add(time.Hour)}

	base := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeBase, vnd(490000)
	})
	member := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeMember, vnd(450000)
		p.CustomerTier = "GOLD"
	})
	clearance := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeClearance, vnd(350000)
	})
	campaign := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeCampaign, vnd(390000)
		p.Period, p.CampaignID = kỳ, campaignID
	})
	flash := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeFlash, vnd(420000)
		p.Period, p.CampaignID = kỳ, campaignID
	})

	// Flash thắng dù giá CAO HƠN clearance — ưu tiên loại, không phải giá.
	got := domain.SelectBest(
		[]*domain.Price{base, member, clearance, campaign, flash},
		testNow, "GOLD", campaignID)
	if got == nil || got.Type() != domain.PriceTypeFlash {
		t.Fatalf("chọn được %v, mong FLASH", typeOf(got))
	}

	// Bỏ flash → campaign thắng.
	got = domain.SelectBest(
		[]*domain.Price{base, member, clearance, campaign},
		testNow, "GOLD", campaignID)
	if got == nil || got.Type() != domain.PriceTypeCampaign {
		t.Fatalf("chọn được %v, mong CAMPAIGN", typeOf(got))
	}

	// Bỏ campaign → clearance thắng.
	got = domain.SelectBest([]*domain.Price{base, member, clearance}, testNow, "GOLD", "")
	if got == nil || got.Type() != domain.PriceTypeClearance {
		t.Fatalf("chọn được %v, mong CLEARANCE", typeOf(got))
	}

	// Bỏ clearance → member thắng (đúng hạng).
	got = domain.SelectBest([]*domain.Price{base, member}, testNow, "GOLD", "")
	if got == nil || got.Type() != domain.PriceTypeMember {
		t.Fatalf("chọn được %v, mong MEMBER", typeOf(got))
	}

	// Chỉ còn base.
	got = domain.SelectBest([]*domain.Price{base}, testNow, "", "")
	if got == nil || got.Type() != domain.PriceTypeBase {
		t.Fatalf("chọn được %v, mong BASE", typeOf(got))
	}
}

// Không kiểm tra hạng thành viên thì mọi khách đều nhận giá thành viên và
// chương trình thành viên mất hết ý nghĩa.
func TestGiaThanhVienChiApChoDungHang(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)
	base := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.Amount = skuID, vnd(490000)
	})
	member := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeMember, vnd(450000)
		p.CustomerTier = "GOLD"
	})
	all := []*domain.Price{base, member}

	// Khách vãng lai KHÔNG được giá thành viên.
	if got := domain.SelectBest(all, testNow, "", ""); got.Type() != domain.PriceTypeBase {
		t.Errorf("khách vãng lai nhận %v, mong BASE", got.Type())
	}
	// Hạng khác cũng không.
	if got := domain.SelectBest(all, testNow, "SILVER", ""); got.Type() != domain.PriceTypeBase {
		t.Errorf("hạng SILVER nhận %v, mong BASE", got.Type())
	}
	// Đúng hạng thì được.
	if got := domain.SelectBest(all, testNow, "GOLD", ""); got.Type() != domain.PriceTypeMember {
		t.Errorf("hạng GOLD nhận %v, mong MEMBER", got.Type())
	}
}

// Giá chiến dịch chỉ áp khi khách đến TỪ chiến dịch đó.
func TestGiaChienDichChiApDungChienDich(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)
	camA := ids.MustNew(ids.PrefixCampaign)
	camB := ids.MustNew(ids.PrefixCampaign)
	kỳ := domain.Period{From: testNow.Add(-time.Hour), To: testNow.Add(time.Hour)}

	base := newPrice(t, func(p *domain.NewPriceParams) { p.SKUID, p.Amount = skuID, vnd(490000) })
	campaign := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeCampaign, vnd(390000)
		p.Period, p.CampaignID = kỳ, camA
	})
	all := []*domain.Price{base, campaign}

	if got := domain.SelectBest(all, testNow, "", camA); got.Type() != domain.PriceTypeCampaign {
		t.Errorf("đúng chiến dịch nhận %v, mong CAMPAIGN", got.Type())
	}
	if got := domain.SelectBest(all, testNow, "", camB); got.Type() != domain.PriceTypeBase {
		t.Errorf("chiến dịch khác nhận %v, mong BASE", got.Type())
	}
	if got := domain.SelectBest(all, testNow, "", ""); got.Type() != domain.PriceTypeBase {
		t.Errorf("không có chiến dịch nhận %v, mong BASE", got.Type())
	}
}

// Cùng loại giá thì chọn giá THẤP HƠN — nếu cấu hình trùng lặp do lỗi vận
// hành, khách được lợi chứ không bị thiệt.
func TestCungLoaiThiChonGiaThapHon(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)
	cao := newPrice(t, func(p *domain.NewPriceParams) { p.SKUID, p.Amount = skuID, vnd(490000) })
	thap := newPrice(t, func(p *domain.NewPriceParams) { p.SKUID, p.Amount = skuID, vnd(390000) })

	// Thử cả hai thứ tự để chắc chắn không phụ thuộc thứ tự đầu vào.
	for _, list := range [][]*domain.Price{{cao, thap}, {thap, cao}} {
		got := domain.SelectBest(list, testNow, "", "")
		if got == nil || got.Amount().Amount() != 390000 {
			t.Errorf("chọn giá %v, mong 390000", got.Amount())
		}
	}
}

func TestGiaHetHanKhongDuocApDung(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)
	base := newPrice(t, func(p *domain.NewPriceParams) { p.SKUID, p.Amount = skuID, vnd(490000) })
	flashHetHan := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeFlash, vnd(290000)
		p.Period = domain.Period{From: testNow.Add(-2 * time.Hour), To: testNow.Add(-time.Hour)}
	})

	got := domain.SelectBest([]*domain.Price{base, flashHetHan}, testNow, "", "")
	if got == nil || got.Type() != domain.PriceTypeBase {
		t.Errorf("chọn %v, mong BASE — giá flash đã hết hạn", typeOf(got))
	}
}

func TestGiaNgungApDungKhongDuocChon(t *testing.T) {
	skuID := ids.MustNew(ids.PrefixSKU)
	base := newPrice(t, func(p *domain.NewPriceParams) { p.SKUID, p.Amount = skuID, vnd(490000) })
	clearance := newPrice(t, func(p *domain.NewPriceParams) {
		p.SKUID, p.PriceType, p.Amount = skuID, domain.PriceTypeClearance, vnd(250000)
	})

	clearance.Deactivate(testNow)
	if clearance.IsActive() {
		t.Fatal("Deactivate không có tác dụng")
	}

	got := domain.SelectBest([]*domain.Price{base, clearance}, testNow, "", "")
	if got == nil || got.Type() != domain.PriceTypeBase {
		t.Errorf("chọn %v, mong BASE", typeOf(got))
	}
}

func TestKhongCoGiaNaoApDungTraNil(t *testing.T) {
	if got := domain.SelectBest(nil, testNow, "", ""); got != nil {
		t.Errorf("danh sách rỗng trả %v, mong nil", got)
	}
	// Lát cắt chứa nil không được panic.
	if got := domain.SelectBest([]*domain.Price{nil, nil}, testNow, "", ""); got != nil {
		t.Errorf("toàn nil trả %v, mong nil", got)
	}
}

// Dùng phần vạn thay vì phần trăm số thực: 1/3 không biểu diễn chính xác
// bằng float và làm tròn khiến các trang hiển thị mức giảm khác nhau.
func TestTinhMucGiamTheoPhanVan(t *testing.T) {
	cases := []struct {
		amount, compareAt int64
		mongBP            int64
	}{
		{400000, 500000, 2000}, // giảm 20%
		{250000, 500000, 5000}, // giảm 50%
		{333333, 500000, 3333}, // 33,33% — chỗ số thực sẽ sai
		{490000, 0, 0},         // không có giá gạch ngang
	}

	for _, tc := range cases {
		p := newPrice(t, func(np *domain.NewPriceParams) {
			np.Amount = vnd(tc.amount)
			if tc.compareAt > 0 {
				np.CompareAt = vnd(tc.compareAt)
			}
		})
		if got := p.DiscountBasisPoints(); got != tc.mongBP {
			t.Errorf("giá %d/%d: mức giảm = %d bp, mong %d", tc.amount, tc.compareAt, got, tc.mongBP)
		}
		if tc.compareAt > 0 && !p.HasDiscount() {
			t.Errorf("giá %d/%d: HasDiscount = false", tc.amount, tc.compareAt)
		}
	}
}

func typeOf(p *domain.Price) any {
	if p == nil {
		return nil
	}
	return p.Type()
}
