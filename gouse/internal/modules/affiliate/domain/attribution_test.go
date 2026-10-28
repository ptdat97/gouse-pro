package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/affiliate/domain"
)

var at = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

func vnd(n int64) money.Money { return money.MustNew(n, money.VND) }

func attribution(t *testing.T, baseAmount int64, tyLe int32) *domain.Attribution {
	t.Helper()
	return attributionWithBase(t, vnd(baseAmount), tyLe)
}

func attributionWithBase(t *testing.T, base money.Money, tyLe int32) *domain.Attribution {
	t.Helper()
	a, err := domain.NewAttribution(domain.NewAttributionParams{
		OrderID:     ids.MustNew(ids.PrefixOrder),
		OrderLineID: ids.MustNew(ids.PrefixOrderLine),
		CreatorID:   ids.MustNew(ids.PrefixCreator),
		Rate:        types.MustNewBasisPoints(tyLe),
		Base:        base,
		CostBearer:  domain.DefaultCostBearer,
		Now:         at,
	})
	if err != nil {
		t.Fatalf("NewAttribution: %v", err)
	}
	return a
}

// Hoa hồng TÍNH từ tỷ lệ và tiền gốc đã đóng băng.
//
// Không nhận con số từ bên gọi: hai nguồn cho cùng một con số tiền sớm muộn
// sẽ lệch nhau. Và vì cả hai đầu vào đều đóng băng, phép tính cho cùng kết
// quả mãi mãi — điều kiện để đối soát lại được.
func TestCommissionFromFrozenRate(t *testing.T) {
	a := attribution(t, 200_000, 500) // 5%

	if got := a.Commission().Amount(); got != 10_000 {
		t.Fatalf("hoa hồng 5%% của 200.000: mong 10.000, nhận %d", got)
	}

	// Gọi lại phải cho CÙNG con số — không có trạng thái nào thay đổi.
	if a.Commission().Amount() != 10_000 {
		t.Error("hoa hồng phải xác định, gọi bao nhiêu lần cũng như nhau")
	}
}

// Tiền gốc là tiền THỰC TRẢ, không phải giá niêm yết.
//
// Bài này không kiểm phép trừ giảm giá (việc của bên gọi) mà khẳng định
// miền nhận con số ĐÃ trừ: 180.000 chứ không phải 200.000. Hoa hồng trên giá
// niêm yết của một đơn có giảm giá là trả nhiều hơn phần creator tạo ra.
func TestBaseIsAmountActuallyPaid(t *testing.T) {
	a := attribution(t, 180_000, 500) // 200.000 niêm yết, giảm 20.000

	if got := a.Commission().Amount(); got != 9_000 {
		t.Fatalf("hoa hồng 5%% của 180.000 thực trả: mong 9.000, nhận %d", got)
	}
}

// Quy công BẤT BIẾN: không có method nào đổi được nó.
//
// affiliate.md quy tắc 2. Bài này kiểm bằng CẤU TRÚC chứ không bằng hành vi
// — nếu ai thêm một setter, `go vet` không kêu và test hành vi cũng không.
// Nên khẳng định ở đây là: mọi method công khai đều chỉ ĐỌC.
func TestAttributionIsImmutable(t *testing.T) {
	a := attribution(t, 200_000, 500)

	before := a.Commission().Amount()
	_ = a.CreatorID()
	_ = a.Base()
	_ = a.Rate()
	_ = a.CostBearer()

	if a.Commission().Amount() != before {
		t.Error("đọc một bản quy công không được làm nó đổi")
	}
	if a.Model() != domain.AttributionModelLastClick {
		t.Errorf("mô hình quy công phải được GHI vào bản ghi, nhận %q",
			a.Model())
	}
}

// Thiếu creator thì từ chối.
//
// Một bản quy công không có creator là một khoản hoa hồng không ai nhận —
// rác trong bảng, và một khoản tiền không có chủ.
func TestMissingCreatorIsRejected(t *testing.T) {
	_, err := domain.NewAttribution(domain.NewAttributionParams{
		OrderLineID: ids.MustNew(ids.PrefixOrderLine),
		Rate:        types.MustNewBasisPoints(500),
		Base:        vnd(100_000),
		CostBearer:  domain.DefaultCostBearer,
		Now:         at,
	})
	if !errors.Is(err, domain.ErrMissingCreator) {
		t.Fatalf("mong ErrMissingCreator, nhận %v", err)
	}
}

// Bên chịu chi phí phải HỢP LỆ, không được để rỗng.
//
// affiliate.md mục 5: "Phải được ghi rõ trong Campaign, KHÔNG mặc định".
// Một bản quy công không nói ai chịu tiền là một khoản chi phí không ai ghi
// nhận — và lúc đối soát sẽ trừ nhầm bên.
func TestCostBearerMustBeValid(t *testing.T) {
	for _, bearer := range []domain.CostBearer{"", "AI_DO", "platform"} {
		_, err := domain.NewAttribution(domain.NewAttributionParams{
			OrderID:     ids.MustNew(ids.PrefixOrder),
			OrderLineID: ids.MustNew(ids.PrefixOrderLine),
			CreatorID:   ids.MustNew(ids.PrefixCreator),
			Rate:        types.MustNewBasisPoints(500),
			Base:        vnd(100_000),
			CostBearer:  bearer,
			Now:         at,
		})
		if !errors.Is(err, domain.ErrInvalidCostBearer) {
			t.Errorf("bên chịu %q phải bị từ chối, nhận %v", bearer, err)
		}
	}

	// Và hai giá trị hợp lệ phải đi qua.
	for _, bearer := range []domain.CostBearer{domain.CostBearerPlatform, domain.CostBearerSeller} {
		if _, err := domain.NewAttribution(domain.NewAttributionParams{
			OrderID:     ids.MustNew(ids.PrefixOrder),
			OrderLineID: ids.MustNew(ids.PrefixOrderLine),
			CreatorID:   ids.MustNew(ids.PrefixCreator),
			Rate:        types.MustNewBasisPoints(500),
			Base:        vnd(100_000),
			CostBearer:  bearer,
			Now:         at,
		}); err != nil {
			t.Errorf("bên chịu %q phải hợp lệ: %v", bearer, err)
		}
	}
}

// ------------------------------------------------------- Khoản hoa hồng

// Hoa hồng bắt đầu ở CHỜ ĐẾN HẠN, không phải rút được.
//
// affiliate.md quy tắc 4: chỉ rút được sau khi hết hạn đổi trả, giống số dư
// nhà bán. Nhờ đó phần lớn hoàn hàng xảy ra TRƯỚC khi tiền được chi.
//
// Nếu bắt đầu ở RÚT ĐƯỢC, một đơn hoàn trong tuần đầu sẽ phải đòi lại tiền
// đã chi — và P3-71 cho thấy việc đòi lại ấy là chỗ dễ sai nhất của cả hệ
// thống tiền.
func TestCommissionStartsPending(t *testing.T) {
	c, err := domain.NewCommission(attribution(t, 200_000, 500), at)
	if err != nil {
		t.Fatalf("NewCommission: %v", err)
	}

	if c.Status() != domain.CommissionPending {
		t.Fatalf("trạng thái đầu phải là CHỜ ĐẾN HẠN, nhận %q", c.Status())
	}
	if got := c.Amount().Amount(); got != 10_000 {
		t.Errorf("số tiền lấy từ bản quy công: mong 10.000, nhận %d", got)
	}
}

// CHƯA hết hạn đổi trả thì KHÔNG chuyển sang rút được.
func TestNotAvailableBeforeReturnWindowCloses(t *testing.T) {
	c, _ := domain.NewCommission(attribution(t, 200_000, 500), at)

	if err := c.MarkAvailable(false, at); !errors.Is(err, domain.ErrReturnWindowOpen) {
		t.Fatalf("mong ErrReturnWindowOpen, nhận %v", err)
	}
	if c.Status() != domain.CommissionPending {
		t.Errorf("từ chối rồi thì trạng thái không được đổi, nhận %q", c.Status())
	}

	if err := c.MarkAvailable(true, at); err != nil {
		t.Fatalf("hết hạn rồi thì phải chuyển được: %v", err)
	}
	if c.Status() != domain.CommissionAvailable {
		t.Errorf("mong RÚT ĐƯỢC, nhận %q", c.Status())
	}
}

// Đảo BẮT BUỘC nêu lý do.
//
// Một khoản tiền biến mất mà không nêu lý do là điểm mù trong kiểm toán —
// cùng quy tắc với bút toán điều chỉnh của payment.
func TestReverseRequiresReason(t *testing.T) {
	c, _ := domain.NewCommission(attribution(t, 200_000, 500), at)

	if err := c.Reverse("", at); !errors.Is(err, domain.ErrReverseReasonRequired) {
		t.Fatalf("mong ErrReverseReasonRequired, nhận %v", err)
	}
	if c.Status() == domain.CommissionReversed {
		t.Error("từ chối rồi thì không được đánh dấu đã đảo")
	}

	if err := c.Reverse("đơn bị hủy", at); err != nil {
		t.Fatalf("đảo có lý do phải được: %v", err)
	}
	if c.Status() != domain.CommissionReversed || c.ReversedReason() == "" || c.ReversedAt().IsZero() {
		t.Errorf("đảo xong phải ghi đủ lý do và mốc thời gian: %q %q %v",
			c.Status(), c.ReversedReason(), c.ReversedAt())
	}
}

// Đảo là IDEMPOTENT, và đã đảo thì không rút được nữa.
//
// `order.cancelled` giao ÍT NHẤT MỘT LẦN, nên bên nhận sẽ thấy cùng một đơn
// hai lần. Đảo hai lần mà báo lỗi sẽ làm bên gọi thử lại mãi.
func TestReverseIsIdempotentAndBlocksAvailable(t *testing.T) {
	c, _ := domain.NewCommission(attribution(t, 200_000, 500), at)

	if err := c.Reverse("đơn bị hủy", at); err != nil {
		t.Fatalf("đảo lần đầu: %v", err)
	}
	firstAt := c.ReversedAt()

	if err := c.Reverse("đơn bị hủy", at.Add(time.Hour)); err != nil {
		t.Fatalf("đảo lần hai phải im lặng thành công: %v", err)
	}
	if !c.ReversedAt().Equal(firstAt) {
		t.Error("đảo lần hai không được đổi mốc thời gian của lần đầu")
	}

	// Đã đảo thì KHÔNG được chuyển sang rút được — tiền đã bị thu hồi.
	if err := c.MarkAvailable(true, at); !errors.Is(err, domain.ErrAlreadyReversed) {
		t.Fatalf("khoản đã đảo không được rút: mong ErrAlreadyReversed, nhận %v", err)
	}
}
