package testdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTheHighestMigrationIsReadFromDisk.
//
// Hàng rào `checkTemplateMigrated` trả nil khi KHÔNG đọc được số trên đĩa —
// có chủ ý, để nó không tự tin sai ở một cây nguồn khác. Nghĩa là một lỗi
// trong `highestMigration` sẽ làm hàng rào IM LẶNG tắt, không đỏ.
//
// Bài này đứng ở chỗ đó: số đọc được phải lớn hơn 0 và thư mục phải tồn tại.
func TestTheHighestMigrationIsReadFromDisk(t *testing.T) {
	highest, dir, err := highestMigration()
	if err != nil {
		t.Fatalf("đọc migrations: %v (thư mục %s)", err, dir)
	}
	if highest <= 0 {
		t.Fatalf("số migration cao nhất = %d ở %s — hàng rào khuôn đang TẮT",
			highest, dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("thư mục migration %s không đọc được: %v", dir, err)
	}
}

// TestATemplateBehindOnDiskIsRejected: khuôn tụt hậu phải ĐỎ, kèm lệnh sửa.
//
// Dựng một database có đúng bảng `schema_migrations` ở phiên bản 1 rồi đưa
// cho hàng rào. Không dùng khuôn thật: hạ phiên bản khuôn thật sẽ làm mọi
// gói test khác đang chạy song song đỏ theo.
func TestATemplateBehindOnDiskIsRejected(t *testing.T) {
	fake := fakeTemplate(t, 1, false)

	err := checkTemplateMigrated(fake)
	if !errors.Is(err, ErrTemplateBehind) {
		t.Fatalf("lỗi = %v, cần ErrTemplateBehind — khuôn ở phiên bản 1 mà "+
			"trên đĩa có hàng chục migration", err)
	}
	// Thông điệp phải nói LÀM GÌ, không chỉ nói sai cái gì: đây là lý do
	// hàng rào tồn tại — bản không có nó đỏ ở một bài test khuyến mãi.
	if !strings.Contains(err.Error(), "migrate -path") {
		t.Errorf("thông điệp không nêu lệnh phải chạy:\n%v", err)
	}
}

// TestADirtyTemplateIsRejected: migration chạy dở cũng phải ĐỎ.
//
// Khuôn dirty nguy hiểm hơn khuôn tụt hậu: `version` đã là số mới nên phép
// so sánh phiên bản cho qua, trong khi lược đồ nằm nửa đường.
func TestADirtyTemplateIsRejected(t *testing.T) {
	onDisk, _, err := highestMigration()
	if err != nil {
		t.Skipf("không đọc được migrations: %v", err)
	}
	fake := fakeTemplate(t, onDisk, true)

	err = checkTemplateMigrated(fake)
	if !errors.Is(err, ErrTemplateBehind) {
		t.Fatalf("lỗi = %v, cần ErrTemplateBehind — khuôn dirty ở ĐÚNG "+
			"phiên bản mới nhất vẫn là khuôn không dùng được", err)
	}
	if !strings.Contains(err.Error(), "force") {
		t.Errorf("thông điệp không nêu `force`:\n%v", err)
	}
}

// TestATemplateAtHeadPasses: và nó phải cho qua khi đúng.
//
// Một hàng rào luôn đỏ cũng vô dụng như một hàng rào luôn xanh.
func TestATemplateAtHeadPasses(t *testing.T) {
	onDisk, _, err := highestMigration()
	if err != nil {
		t.Skipf("không đọc được migrations: %v", err)
	}
	fake := fakeTemplate(t, onDisk, false)

	if err := checkTemplateMigrated(fake); err != nil {
		t.Fatalf("khuôn ở đúng phiên bản %d mà bị từ chối: %v", onDisk, err)
	}
}

// giaKhuon dựng một database tạm chỉ có bảng `schema_migrations`.
func fakeTemplate(t *testing.T, version int64, dirty bool) string {
	t.Helper()

	template := strings.TrimSpace(os.Getenv(EnvVar))
	if template == "" {
		t.Skipf("bỏ qua: cần %s", EnvVar)
	}

	name := fmt.Sprintf("gouse_khuon_thu_%d_%v", version, dirty)
	admin, err := withDB(template, "postgres")
	if err != nil {
		t.Fatalf("dsn quản trị: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, admin)
	if err != nil {
		t.Fatalf("kết nối quản trị: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`"`); err != nil {
		t.Fatalf("xóa database thử: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatalf("tạo database thử: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		p, err := pgxpool.New(c, admin)
		if err != nil {
			return
		}
		defer p.Close()
		_, _ = p.Exec(c, `DROP DATABASE IF EXISTS "`+name+`"`)
	})

	dsn, err := withDB(template, name)
	if err != nil {
		t.Fatalf("dsn database thử: %v", err)
	}
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("kết nối database thử: %v", err)
	}
	defer p.Close()
	if _, err := p.Exec(ctx,
		`CREATE TABLE schema_migrations (version BIGINT NOT NULL, dirty BOOLEAN NOT NULL)`); err != nil {
		t.Fatalf("tạo schema_migrations: %v", err)
	}
	if _, err := p.Exec(ctx,
		`INSERT INTO schema_migrations (version, dirty) VALUES ($1, $2)`, version, dirty); err != nil {
		t.Fatalf("ghi phiên bản: %v", err)
	}
	return dsn
}
