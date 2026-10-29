package types_test

import (
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// TestAnEmptyStringMeansNoLimit.
//
// Chuỗi rỗng nghĩa là "không lọc", KHÔNG phải lỗi. `time.Parse` trần thì
// báo lỗi với chuỗi rỗng, nên mọi request không kèm `from` sẽ ăn 400 — một
// bộ lọc tùy chọn bỗng thành bắt buộc.
func TestAnEmptyStringMeansNoLimit(t *testing.T) {
	got, err := types.ParseDay("")
	if err != nil {
		t.Fatalf("chuỗi rỗng báo lỗi %v — bộ lọc tùy chọn thành bắt buộc", err)
	}
	if !got.IsZero() {
		t.Errorf("chuỗi rỗng trả %v, cần thời điểm zero", got)
	}
}

// TestMidnightFollowsBusinessTime: "20/08" bắt đầu lúc nửa đêm Ở VIỆT NAM.
func TestMidnightFollowsBusinessTime(t *testing.T) {
	got, err := types.ParseDay("2026-08-20")
	if err != nil {
		t.Fatalf("ParseDay: %v", err)
	}

	// Cùng thời điểm, viết theo UTC: 17:00 ngày 19/08.
	mong := time.Date(2026, 8, 19, 17, 0, 0, 0, time.UTC)
	if !got.Equal(mong) {
		t.Errorf("nửa đêm 20/08 = %s, cần %s — mốc ngày đang cắt theo UTC "+
			"thay vì giờ nghiệp vụ", got.UTC(), mong)
	}
}

// TestEndOfDayCoversTheWholeDay: khoảng [nửa đêm, EndOfDay] phải phủ đúng một ngày.
//
// Kiểm bằng hai mốc BIÊN thay vì so chuỗi: sát trong phải nằm trong, sát
// ngoài phải nằm ngoài. So chuỗi thì một cách cài sai lệch vài giờ vẫn có
// thể trùng khớp ở một vài trường hợp.
func TestEndOfDayCoversTheWholeDay(t *testing.T) {
	start, err := types.ParseDay("2026-08-20")
	if err != nil {
		t.Fatal(err)
	}
	end := types.EndOfDay(start)

	inside := time.Date(2026, 8, 20, 23, 59, 59, 0, types.BusinessTZ)
	outside := time.Date(2026, 8, 21, 0, 0, 0, 0, types.BusinessTZ)

	if inside.Before(start) || inside.After(end) {
		t.Errorf("23:59:59 ngày 20/08 nằm NGOÀI khoảng [%s, %s]", start, end)
	}
	if !outside.After(end) {
		t.Errorf("00:00 ngày 21/08 nằm TRONG khoảng đến hết 20/08 — khoảng " +
			"lấn sang ngày sau")
	}
}

// TestTheTimeZoneDoesNotShiftWithTheSeason.
//
// Việt Nam giữ nguyên +07:00 từ 1975 và không có giờ mùa hè. Bài này khóa
// giả định đó: nếu ai đó đổi sang một múi giờ CÓ giờ mùa hè, mốc ngày sẽ
// lệch một tiếng trong nửa năm — kiểu lỗi chỉ lộ ra theo mùa.
func TestTheTimeZoneDoesNotShiftWithTheSeason(t *testing.T) {
	for _, day := range []string{"2026-01-15", "2026-07-15"} {
		got, err := types.ParseDay(day)
		if err != nil {
			t.Fatal(err)
		}
		if _, drift := got.Zone(); drift != 7*60*60 {
			t.Errorf("%s: lệch múi giờ %d giây, cần %d", day, drift, 7*60*60)
		}
	}
}
