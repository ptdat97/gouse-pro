package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/catalog/domain"
)

func newCollection(t *testing.T, launch, end time.Time) *domain.Collection {
	t.Helper()
	c, err := domain.NewCollection(domain.NewCollectionParams{
		BrandID:         ids.MustNew(ids.PrefixBrand),
		Name:            "Thu Đông 2026",
		Slug:            "thu-dong-2026",
		Season:          "FW2026",
		LaunchDate:      launch,
		EndOfSeasonDate: end,
	})
	if err != nil {
		t.Fatalf("tạo bộ sưu tập lỗi: %v", err)
	}
	return c
}

func TestCollectionRequiresBrand(t *testing.T) {
	_, err := domain.NewCollection(domain.NewCollectionParams{
		Name: "Thu Đông", Slug: "thu-dong",
	})
	if err == nil {
		t.Fatal("bộ sưu tập không thuộc thương hiệu nào phải bị từ chối")
	}
}

func TestCollectionRejectsInvalidSeasonDates(t *testing.T) {
	base := time.Now().UTC()
	_, err := domain.NewCollection(domain.NewCollectionParams{
		BrandID: ids.MustNew(ids.PrefixBrand),
		Name:    "X", Slug: "x",
		LaunchDate:      base,
		EndOfSeasonDate: base.Add(-time.Hour),
	})
	if !errors.Is(err, domain.ErrInvalidSeasonDates) {
		t.Fatalf("ngày kết thúc trước ngày ra mắt phải lỗi, nhận %v", err)
	}
}

func TestCollectionAllowsUnsetDatesWhilePlanning(t *testing.T) {
	// Bộ sưu tập ở giai đoạn lên ý tưởng chưa chốt lịch — không nên
	// bắt buộc ngày ngay từ đầu.
	c, err := domain.NewCollection(domain.NewCollectionParams{
		BrandID: ids.MustNew(ids.PrefixBrand),
		Name:    "Ý tưởng mới", Slug: "y-tuong-moi",
	})
	if err != nil {
		t.Fatalf("chưa chốt lịch phải được phép: %v", err)
	}
	if c.Status() != domain.CollectionPlanning {
		t.Errorf("bộ sưu tập mới phải ở PLANNING, nhận %q", c.Status())
	}
}

// TestCollectionLifecycle kiểm chứng máy trạng thái.
func TestCollectionLifecycle(t *testing.T) {
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	c := newCollection(t, base, base.Add(90*24*time.Hour))

	// PLANNING không hiển thị cho khách — cơ chế "xuất bản có lịch"
	if c.IsVisibleToCustomer() {
		t.Error("bộ sưu tập PLANNING không được hiển thị cho khách")
	}

	if err := c.Launch(base); err != nil {
		t.Fatalf("ra mắt lỗi: %v", err)
	}
	if !c.IsVisibleToCustomer() {
		t.Error("bộ sưu tập ACTIVE phải hiển thị")
	}

	if err := c.MarkEnding(base.Add(60 * 24 * time.Hour)); err != nil {
		t.Fatalf("đánh dấu sắp hết mùa lỗi: %v", err)
	}
	if !c.IsVisibleToCustomer() {
		t.Error("bộ sưu tập ENDING vẫn phải hiển thị (đang xả hàng)")
	}

	if err := c.Archive(base.Add(95 * 24 * time.Hour)); err != nil {
		t.Fatalf("lưu trữ lỗi: %v", err)
	}
	if c.IsVisibleToCustomer() {
		t.Error("bộ sưu tập ARCHIVED không được hiển thị")
	}
}

func TestCollectionRejectsInvalidTransitions(t *testing.T) {
	base := time.Now().UTC()

	t.Run("không quay lại ACTIVE sau khi ARCHIVED", func(t *testing.T) {
		// Mở lại bộ sưu tập đã đóng làm sai lệch chỉ số sell-through
		// và báo cáo mùa vụ.
		c := newCollection(t, base, base.Add(90*24*time.Hour))
		_ = c.Launch(base)
		_ = c.Archive(base)

		if err := c.Launch(base); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("ARCHIVED → ACTIVE phải bị chặn, nhận %v", err)
		}
	})

	t.Run("không bỏ qua ACTIVE để tới ENDING", func(t *testing.T) {
		c := newCollection(t, base, base.Add(90*24*time.Hour))
		if err := c.MarkEnding(base); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("PLANNING → ENDING phải bị chặn, nhận %v", err)
		}
	})

	t.Run("không ra mắt hai lần", func(t *testing.T) {
		c := newCollection(t, base, base.Add(90*24*time.Hour))
		_ = c.Launch(base)
		if err := c.Launch(base); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("ACTIVE → ACTIVE phải bị chặn, nhận %v", err)
		}
	})
}

// TestCollectionScheduledPublishing kiểm chứng cơ chế công bố theo lịch —
// thay cho việc nhân đôi bảng như QOR Publish2.
func TestCollectionScheduledPublishing(t *testing.T) {
	launch := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := launch.Add(90 * 24 * time.Hour)
	c := newCollection(t, launch, end)

	if c.ShouldLaunch(launch.Add(-time.Hour)) {
		t.Error("chưa tới ngày ra mắt thì không được ra mắt")
	}
	if !c.ShouldLaunch(launch) {
		t.Error("đúng ngày ra mắt phải sẵn sàng")
	}
	if !c.ShouldLaunch(launch.Add(24 * time.Hour)) {
		t.Error("quá ngày ra mắt vẫn phải ra mắt (job có thể chạy trễ)")
	}

	_ = c.Launch(launch)

	if c.ShouldLaunch(launch.Add(time.Hour)) {
		t.Error("đã ra mắt rồi thì không ra mắt lại")
	}
	if !c.ShouldMarkEnding(end) {
		t.Error("tới ngày hết mùa phải chuyển sang ENDING")
	}
}

// TestCollectionWeeksRemaining kiểm chứng đầu vào cho quyết định bổ sung hàng.
//
// Nếu thời gian còn lại ít hơn lead time nhà cung cấp, KHÔNG nên đặt thêm —
// hàng về sẽ không kịp bán và phải xả giá.
func TestCollectionWeeksRemaining(t *testing.T) {
	launch := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := launch.Add(90 * 24 * time.Hour)
	c := newCollection(t, launch, end)

	cases := []struct {
		name string
		now  time.Time
		want int
	}{
		{"đầu mùa", launch, 12},
		{"còn 4 tuần", end.Add(-28 * 24 * time.Hour), 4},
		{"đúng ngày hết mùa", end, 0},
		{"đã qua mùa", end.Add(24 * time.Hour), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.WeeksRemaining(tc.now); got != tc.want {
				t.Errorf("WeeksRemaining = %d, mong %d", got, tc.want)
			}
		})
	}
}
