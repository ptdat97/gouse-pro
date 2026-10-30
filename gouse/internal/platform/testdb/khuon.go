package testdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Vì sao có hàng rào này: một lần thêm migration mà quên migrate KHUÔN
//
// `testdb` sao database của mỗi gói từ khuôn `gouse_test`. Khuôn KHÔNG tự
// migrate — nó được dựng một lần bằng `make test-db`. Thêm migration mà
// quên bước ấy thì mọi database test tụt lại đúng số phiên bản ấy.
//
// Cách nó hỏng không phải "thiếu cột". Ngày 29/10/2026, thêm 000063 đổi tên
// `fulfillment_order.cho_thanh_toan` thành `awaiting_payment` rồi sửa mã Go
// theo. Khuôn vẫn ở 62, nên câu INSERT của `fulfillment` lỗi — nhưng nó
// chạy trong một HANDLER, và handler lỗi thì `drain()` ghi vào
// `event_outbox.last_error` rồi đi tiếp. Bài test duy nhất đỏ là một bài
// KHUYẾN MÃI, với câu:
//
//	--- FAIL: TestASellerBornePromotionRecordsTheRightBearer
//	    sổ cái trừ nhà bán 0 đ, cần 49000
//
// Không một chữ nào nói về migration. Phải `git stash` rồi chạy lại ở HEAD
// mới biết là do mình, chứ không phải một bài test chập chờn.
//
// Hàng rào này đọc số migration cao nhất trên ĐĨA và so với
// `schema_migrations` của khuôn. Nó chạy MỘT LẦN cho mỗi gói test, cùng chỗ
// với phép sao khuôn — tức nơi sự thật nằm.
//
// # Vì sao KHÔNG đặt ở `make check`
//
// Ai gõ `go test ./...` thẳng vẫn phải được bảo vệ. Cùng lý do với hàng rào
// "không chạy test trên database máy chủ" ở `DSN`: một quy ước trong
// Makefile chỉ bảo vệ người đi qua Makefile.

// reTenMigration khớp tiền tố số của tên tệp migration: `000063_....up.sql`.
var reMigrationFile = regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)

// ErrTemplateBehind khi khuôn tụt lại so với migration trên đĩa.
var ErrTemplateBehind = errors.New("testdb: database khuôn tụt hậu")

// checkTemplateMigrated so phiên bản khuôn với số migration cao nhất.
//
// Trả nil khi KHÔNG xác định được số trên đĩa: gói này có thể được gọi từ
// một cây nguồn khác, và một hàng rào tự tin sai còn tệ hơn không có.
func checkTemplateMigrated(template string) error {
	onDisk, dir, err := highestMigration()
	if err != nil || onDisk == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, template)
	if err != nil {
		return nil // lỗi kết nối để bên gọi báo, với thông điệp của nó
	}
	defer pool.Close()

	var inDB int64
	var dirty bool
	err = pool.QueryRow(ctx,
		`SELECT version, dirty FROM schema_migrations`).Scan(&inDB, &dirty)
	if err != nil {
		return fmt.Errorf("%w: không đọc được `schema_migrations` của khuôn "+
			"(%v).\nChạy: make test-db", ErrTemplateBehind, err)
	}

	if dirty {
		return fmt.Errorf("%w: khuôn đang DIRTY ở phiên bản %d — một "+
			"migration chạy dở.\nChạy: migrate -path %s -database "+
			"\"$TEST_DATABASE_URL\" force %d && make test-db",
			ErrTemplateBehind, inDB, dir, inDB-1)
	}

	if inDB < onDisk {
		return fmt.Errorf("%w: khuôn ở phiên bản %d, trên đĩa có %d.\n"+
			"Mọi database test sao từ khuôn nên chúng cũng tụt lại — và "+
			"cách hỏng thường KHÔNG nói về migration: một handler lỗi im "+
			"lặng làm một bài test không liên quan đỏ.\n"+
			"Chạy: migrate -path %s -database \"$TEST_DATABASE_URL\" up",
			ErrTemplateBehind, inDB, onDisk, dir)
	}
	return nil
}

// highestMigration đọc số migration cao nhất trong `migrations/`.
//
// Tìm thư mục từ đường dẫn CỦA CHÍNH TỆP NÀY, không từ thư mục làm việc:
// `go test` chạy với thư mục làm việc là gói đang test, nên một đường dẫn
// tương đối sẽ trỏ sai ở mọi gói trừ một.
func highestMigration() (cao int64, dir string, err error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return 0, "", errors.New("không xác định được đường dẫn gói testdb")
	}
	// <root>/internal/platform/testdb/khuon.go → <root>
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(self))))
	dir = filepath.Join(root, "migrations")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, dir, err
	}
	var nums []int64
	for _, e := range entries {
		m := reMigrationFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return 0, dir, nil
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	return nums[len(nums)-1], dir, nil
}
