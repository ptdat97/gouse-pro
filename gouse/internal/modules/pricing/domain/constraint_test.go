package domain_test

import (
	"errors"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/pricing/domain"
)

func newConstraint(t *testing.T, min, max int64) *domain.PriceConstraint {
	t.Helper()
	p := domain.NewConstraintParams{SKUID: ids.MustNew(ids.PrefixSKU), Now: testNow}
	if min > 0 {
		p.MinPrice = vnd(min)
	}
	if max > 0 {
		p.MaxPrice = vnd(max)
	}
	c, err := domain.NewPriceConstraint(p)
	if err != nil {
		t.Fatalf("NewPriceConstraint: %v", err)
	}
	return c
}

// Khung giá không có giới hạn nào tạo cảm giác an toàn giả rằng giá đang
// được kiểm soát.
func TestKhungGiaPhaiCoItNhatMotGioiHan(t *testing.T) {
	_, err := domain.NewPriceConstraint(domain.NewConstraintParams{
		SKUID: ids.MustNew(ids.PrefixSKU), Now: testNow,
	})
	if !errors.Is(err, domain.ErrEmptyConstraint) {
		t.Errorf("lỗi = %v, mong ErrEmptyConstraint", err)
	}
}

// Min > Max thì KHÔNG giá nào hợp lệ — seller không đăng bán được và sẽ
// không hiểu vì sao.
func TestMinKhongDuocCaoHonMax(t *testing.T) {
	_, err := domain.NewPriceConstraint(domain.NewConstraintParams{
		SKUID:    ids.MustNew(ids.PrefixSKU),
		MinPrice: vnd(500000),
		MaxPrice: vnd(100000),
		Now:      testNow,
	})
	if !errors.Is(err, domain.ErrMinAboveMax) {
		t.Errorf("lỗi = %v, mong ErrMinAboveMax", err)
	}
}

func TestKhungGiaKhacDonViTienTe(t *testing.T) {
	_, err := domain.NewPriceConstraint(domain.NewConstraintParams{
		SKUID:    ids.MustNew(ids.PrefixSKU),
		MinPrice: vnd(100000),
		MaxPrice: money.MustNew(50000, money.USD),
		Now:      testNow,
	})
	if !errors.Is(err, domain.ErrCurrencyMismatch) {
		t.Errorf("lỗi = %v, mong ErrCurrencyMismatch", err)
	}
}

// Quy tắc 4: giá seller phải trong khung ràng buộc.
//
// Giá tối thiểu chống CẢ bán phá giá LẪN lỗi nhập liệu — seller gõ thiếu
// một số 0 sẽ bán 10.000đ thay vì 100.000đ.
func TestKiemTraGiaSellerTheoKhung(t *testing.T) {
	c := newConstraint(t, 100000, 500000)

	cases := []struct {
		ten      string
		gia      int64
		mongOK   bool
		mongCode domain.ViolationCode
	}{
		{"đúng khung", 300000, true, domain.ViolationNone},
		{"đúng bằng min", 100000, true, domain.ViolationNone},
		{"đúng bằng max", 500000, true, domain.ViolationNone},
		{"thấp hơn min — gõ thiếu số 0", 10000, false, domain.ViolationBelowMin},
		{"cao hơn max — thổi giá", 900000, false, domain.ViolationAboveMax},
		{"giá 0", 0, false, domain.ViolationNotPositive},
		{"giá âm", -1000, false, domain.ViolationNotPositive},
	}

	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			got := c.Check(vnd(tc.gia))
			if got.Allowed != tc.mongOK {
				t.Errorf("Allowed = %v, mong %v (%s)", got.Allowed, tc.mongOK, got.Message)
			}
			if got.Code != tc.mongCode {
				t.Errorf("Code = %q, mong %q", got.Code, tc.mongCode)
			}
			// Kết quả phải kèm khung giá để giao diện hiển thị "giá phải từ X đến Y".
			if got.Min.IsZero() || got.Max.IsZero() {
				t.Error("kết quả phải kèm khung giá để seller biết sửa thế nào")
			}
			// Bị từ chối thì phải có lý do đọc được.
			if !got.Allowed && got.Message == "" {
				t.Error("từ chối mà không nêu lý do")
			}
		})
	}
}

func TestKiemTraGiaKhacDonViTienTe(t *testing.T) {
	c := newConstraint(t, 100000, 500000)

	got := c.Check(money.MustNew(30000, money.USD))
	if got.Allowed {
		t.Error("giá khác đơn vị tiền tệ phải bị từ chối")
	}
	if got.Code != domain.ViolationWrongCurrency {
		t.Errorf("Code = %q, mong CURRENCY_MISMATCH", got.Code)
	}
}

// Chỉ có một phía giới hạn thì phía kia không chặn.
func TestKhungGiaMotPhia(t *testing.T) {
	chiMin := newConstraint(t, 100000, 0)
	if got := chiMin.Check(vnd(99999999)); !got.Allowed {
		t.Errorf("chỉ có min: giá cao bị chặn (%s)", got.Message)
	}
	if got := chiMin.Check(vnd(50000)); got.Allowed {
		t.Error("chỉ có min: giá thấp phải bị chặn")
	}

	chiMax := newConstraint(t, 0, 500000)
	if got := chiMax.Check(vnd(1)); !got.Allowed {
		t.Errorf("chỉ có max: giá thấp bị chặn (%s)", got.Message)
	}
	if got := chiMax.Check(vnd(600000)); got.Allowed {
		t.Error("chỉ có max: giá cao phải bị chặn")
	}
}

// Giá lệch xa thị trường là CẢNH BÁO, không phải từ chối — chặn thẳng sẽ
// cản trở hàng thanh lý thật.
func TestGiaBatThuongDuocChapNhanNhungDanhDau(t *testing.T) {
	c, err := domain.NewPriceConstraint(domain.NewConstraintParams{
		SKUID:          ids.MustNew(ids.PrefixSKU),
		MinPrice:       vnd(10000),
		MaxPrice:       vnd(1000000),
		ReferencePrice: vnd(500000),
		Now:            testNow,
	})
	if err != nil {
		t.Fatalf("NewPriceConstraint: %v", err)
	}

	// Thấp hơn 50% giá tham chiếu (< 250.000đ) → cảnh báo.
	got := c.Check(vnd(100000))
	if !got.Allowed {
		t.Error("giá bất thường phải được CHẤP NHẬN, không bị chặn")
	}
	if !got.NeedsReview {
		t.Error("giá bất thường phải được đánh dấu cần rà soát")
	}
	if got.Code != domain.ViolationSuspicious {
		t.Errorf("Code = %q, mong SUSPICIOUS_PRICE", got.Code)
	}

	// Trong ngưỡng bình thường → không cảnh báo.
	got = c.Check(vnd(400000))
	if !got.Allowed || got.NeedsReview {
		t.Errorf("giá bình thường bị đánh dấu: %+v", got)
	}

	// Đúng ngưỡng (50%) không bị coi là bất thường.
	if got := c.Check(vnd(250000)); got.NeedsReview {
		t.Error("giá đúng ngưỡng không được coi là bất thường")
	}
}

func TestKhongCoGiaThamChieuThiKhongCanhBao(t *testing.T) {
	c := newConstraint(t, 1000, 1000000)
	if got := c.Check(vnd(2000)); got.NeedsReview {
		t.Error("không có giá tham chiếu thì không được cảnh báo")
	}
}

func TestCapNhatKhungGia(t *testing.T) {
	c := newConstraint(t, 100000, 500000)

	if err := c.Update(vnd(200000), vnd(600000), testNow); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if c.MinPrice().Amount() != 200000 || c.MaxPrice().Amount() != 600000 {
		t.Errorf("khung giá = %v–%v, mong 200000–600000", c.MinPrice(), c.MaxPrice())
	}

	// Cập nhật sai phải bị chặn VÀ không làm hỏng giá trị cũ.
	if err := c.Update(vnd(900000), vnd(100000), testNow); !errors.Is(err, domain.ErrMinAboveMax) {
		t.Errorf("lỗi = %v, mong ErrMinAboveMax", err)
	}
	if c.MinPrice().Amount() != 200000 {
		t.Errorf("cập nhật thất bại đã làm đổi khung giá: %v", c.MinPrice())
	}

	if err := c.Update(money.Money{}, money.Money{}, testNow); !errors.Is(err, domain.ErrEmptyConstraint) {
		t.Errorf("lỗi = %v, mong ErrEmptyConstraint", err)
	}
}

func TestKhungGiaCoTienToDung(t *testing.T) {
	c := newConstraint(t, 100000, 500000)
	if c.ID().Prefix() != ids.PrefixPriceConstraint {
		t.Errorf("tiền tố = %q, mong %q", c.ID().Prefix(), ids.PrefixPriceConstraint)
	}
}
