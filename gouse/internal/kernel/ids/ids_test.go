package ids_test

import (
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
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

// ------------------------------------------------ Tiền tố không được TRÙNG

// trungCoLyDo là những tiền tố trùng nhau ĐÃ TỒN TẠI, kèm lý do.
//
// Sổ này tồn tại để một lần trùng cũ không chặn hàng rào chống trùng MỚI.
// Nó CHỈ ĐƯỢC RÚT NGẮN.
var trungCoLyDo = map[string]string{
	"evt": "PrefixEmailToken và PrefixEvent cùng dùng `evt` từ đầu. Sửa " +
		"nghĩa là đổi định dạng mã của những hàng ĐÃ CÓ trong database, " +
		"tức một migration dữ liệu — xem P3-82.",
}

// Mỗi tiền tố phải là DUY NHẤT.
//
// # Vì sao quy tắc này quan trọng hơn vẻ ngoài của nó
//
// Chú thích của chính gói này nói: *"Tiền tố giúp gỡ lỗi nhanh — nhìn
// `off_` biết ngay là offer."* Hai khái niệm dùng một tiền tố phá đúng điều
// đó.
//
// Nặng hơn: `ids.Parse(x, PrefixA)` KHÔNG phân biệt được mã của A với mã
// của B khi hai tiền tố giống nhau. Một hàm nhận mã token xác minh email sẽ
// nhận luôn một mã event và coi là hợp lệ — phép kiểm kiểu ở biên biến
// thành không kiểm gì.
//
// Ngày 25/09/2026 đã có MỘT lần trùng, tìm ra khi thêm hai tiền tố cho
// module affiliate. Bài này chặn lần thứ hai.
func TestTienToKhongTrung(t *testing.T) {
	// Đọc chính file nguồn: một danh sách chép tay ở đây sẽ lệch với danh
	// sách thật, và khi ấy bài test gác một thứ không tồn tại.
	src, err := os.ReadFile("ids.go")
	if err != nil {
		t.Fatalf("đọc ids.go: %v", err)
	}

	re := regexp.MustCompile(`(Prefix\w+)\s+Prefix = "(\w+)"`)
	theoTienTo := map[string][]string{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		theoTienTo[m[2]] = append(theoTienTo[m[2]], m[1])
	}
	if len(theoTienTo) < 40 {
		t.Fatalf("chỉ đọc được %d tiền tố — đổi cách khai thì phải sửa cả "+
			"bài test này", len(theoTienTo))
	}

	for tienTo, tens := range theoTienTo {
		if len(tens) == 1 {
			continue
		}
		if _, coLyDo := trungCoLyDo[tienTo]; coLyDo {
			continue
		}
		sort.Strings(tens)
		t.Errorf("tiền tố %q bị %d hằng số cùng dùng: %s\n"+
			"Mã có tiền tố giống nhau thì `ids.Parse` không phân biệt được "+
			"chúng, và phép kiểm kiểu ở biên thành không kiểm gì.",
			tienTo, len(tens), strings.Join(tens, ", "))
	}

	// Sổ trỏ tới một tiền tố KHÔNG còn trùng cũng là một dòng nói dối.
	for tienTo := range trungCoLyDo {
		if len(theoTienTo[tienTo]) <= 1 {
			t.Errorf("`trungCoLyDo` còn dòng cho %q mà nó không còn trùng — "+
				"xóa đi, sổ này chỉ được rút ngắn", tienTo)
		}
	}
}
