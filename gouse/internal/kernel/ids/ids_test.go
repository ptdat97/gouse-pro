package ids_test

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// specPattern là pattern trong api/components/common.yaml#/schemas/Id.
// ID sinh ra PHẢI khớp — nếu không, response API sẽ vi phạm chính đặc tả
// mà chúng ta công bố.
var specPattern = regexp.MustCompile(`^[a-z]+_[0-9A-HJKMNP-TV-Z]{26}$`)

func TestGeneratedIDMatchesOpenAPISpec(t *testing.T) {
	prefixes := []ids.Prefix{
		ids.PrefixOrder, ids.PrefixOffer, ids.PrefixSKU,
		ids.PrefixSeller, ids.PrefixCreator, ids.PrefixLedgerEntry,
	}
	for _, p := range prefixes {
		for i := 0; i < 100; i++ {
			id := ids.MustNew(p)
			if !specPattern.MatchString(id.String()) {
				t.Fatalf("ID %q không khớp pattern trong đặc tả OpenAPI", id)
			}
		}
	}
}

func TestNoAmbiguousCharacters(t *testing.T) {
	// Crockford base32 bỏ I, L, O, U để tránh nhầm khi đọc qua điện thoại
	// hoặc gõ tay — quan trọng khi hỗ trợ khách hàng.
	for i := 0; i < 2000; i++ {
		id := ids.MustNew(ids.PrefixOrder)
		body := id.String()[4:]
		for _, c := range "ILOU" {
			for _, got := range body {
				if got == c {
					t.Fatalf("ID %q chứa ký tự dễ nhầm %q", id, c)
				}
			}
		}
	}
}

func TestUniqueness(t *testing.T) {
	const n = 50_000
	seen := make(map[ids.ID]struct{}, n)
	for i := 0; i < n; i++ {
		id := ids.MustNew(ids.PrefixOrder)
		if _, dup := seen[id]; dup {
			t.Fatalf("ID trùng sau %d lần sinh: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestMonotonicWithinSameMillisecond(t *testing.T) {
	// Tính đơn điệu quan trọng cho hiệu năng chỉ mục B-tree: chèn tuần tự
	// ít phân mảnh hơn chèn ngẫu nhiên.
	const n = 1000
	got := make([]string, n)
	for i := range got {
		got[i] = ids.MustNew(ids.PrefixOrder).String()
	}

	want := make([]string, n)
	copy(want, got)
	sort.Strings(want)

	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("ID không tăng dần tại vị trí %d: %s (mong %s)", i, got[i], want[i])
		}
	}
}

func TestConcurrentGenerationIsUnique(t *testing.T) {
	// Sinh ID từ nhiều goroutine — tình huống thực tế khi xử lý nhiều
	// request đồng thời.
	const goroutines = 50
	const perGoroutine = 500

	var mu sync.Mutex
	seen := make(map[ids.ID]struct{}, goroutines*perGoroutine)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]ids.ID, perGoroutine)
			for i := range local {
				local[i] = ids.MustNew(ids.PrefixOrder)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, dup := seen[id]; dup {
					t.Errorf("ID trùng khi chạy đồng thời: %s", id)
					return
				}
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*perGoroutine {
		t.Fatalf("mong %d ID duy nhất, nhận %d", goroutines*perGoroutine, len(seen))
	}
}

func TestParseValidatesPrefix(t *testing.T) {
	orderID := ids.MustNew(ids.PrefixOrder)

	// Đúng tiền tố
	if _, err := ids.Parse(orderID.String(), ids.PrefixOrder); err != nil {
		t.Fatalf("parse hợp lệ không được lỗi: %v", err)
	}

	// Sai tiền tố — bắt được lỗi truyền nhầm loại id, ví dụ truyền
	// offer_id vào chỗ mong order_id.
	if _, err := ids.Parse(orderID.String(), ids.PrefixOffer); !errors.Is(err, ids.ErrWrongPrefix) {
		t.Fatalf("sai tiền tố phải lỗi ErrWrongPrefix, nhận %v", err)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"thiếu gạch dưới", "ord01J9XABC123DEF456GHJKMNPQR"},
		{"phần ULID quá ngắn", "ord_01J9XABC"},
		{"phần ULID quá dài", "ord_01J9XABC123DEF456GHJKMNPQRSTVW"},
		{"chứa ký tự I bị cấm", "ord_01J9XABCI23DEF456GHJKMNPQ"},
		{"chuỗi rỗng", ""},
		{"chỉ có tiền tố", "ord_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ids.Parse(tc.in, ids.PrefixOrder); err == nil {
				t.Fatalf("chuỗi %q phải bị từ chối", tc.in)
			}
		})
	}
}

func TestPrefixExtraction(t *testing.T) {
	id := ids.MustNew(ids.PrefixSeller)
	if got := id.Prefix(); got != ids.PrefixSeller {
		t.Fatalf("mong tiền tố %q, nhận %q", ids.PrefixSeller, got)
	}
}

func TestNewRejectsEmptyPrefix(t *testing.T) {
	if _, err := ids.New(""); !errors.Is(err, ids.ErrEmptyPrefix) {
		t.Fatalf("tiền tố rỗng phải lỗi, nhận %v", err)
	}
}

func TestIsValid(t *testing.T) {
	if !ids.IsValid(ids.MustNew(ids.PrefixOrder).String()) {
		t.Error("ID vừa sinh phải hợp lệ")
	}
	if ids.IsValid("khong-phai-id") {
		t.Error("chuỗi bất kỳ không được coi là hợp lệ")
	}
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ids.MustNew(ids.PrefixOrder)
	}
}
