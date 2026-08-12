package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/platform/config"
)

func TestLoadDefaults(t *testing.T) {
	// Không đặt biến môi trường nào → giá trị mặc định hợp lý cho development.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load với môi trường rỗng phải thành công: %v", err)
	}

	if cfg.Env != config.EnvDevelopment {
		t.Errorf("APP_ENV mặc định phải là development, nhận %q", cfg.Env)
	}
	if cfg.HTTP.Port != 8080 {
		t.Errorf("HTTP_PORT mặc định phải là 8080, nhận %d", cfg.HTTP.Port)
	}
	if cfg.HTTP.MaxRequestBytes != 1<<20 {
		t.Errorf("giới hạn kích thước request mặc định phải là 1MiB, nhận %d", cfg.HTTP.MaxRequestBytes)
	}
}

func TestDevelopmentDefaultsFavorReadability(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load lỗi: %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("development mặc định LOG_LEVEL=debug, nhận %q", cfg.Log.Level)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("development mặc định LOG_FORMAT=text (dễ đọc), nhận %q", cfg.Log.Format)
	}
}

func TestProductionDefaultsFavorQueryability(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://localhost/db")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load lỗi: %v", err)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("production mặc định LOG_LEVEL=info, nhận %q", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("production mặc định LOG_FORMAT=json (truy vấn được), nhận %q", cfg.Log.Format)
	}
}

// TestProductionRequiresDatabaseURL — thà KHÔNG KHỞI ĐỘNG còn hơn chạy rồi
// thất bại ở request đầu tiên của khách.
func TestProductionRequiresDatabaseURL(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("thiếu DATABASE_URL ở production phải là lỗi khởi động")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("thông báo lỗi phải nêu rõ biến thiếu, nhận: %v", err)
	}
}

func TestDevelopmentAllowsMissingDatabaseURL(t *testing.T) {
	// Cho phép chạy phần không cần database khi phát triển cục bộ.
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "")

	if _, err := config.Load(); err != nil {
		t.Fatalf("development không cần DATABASE_URL: %v", err)
	}
}

// TestReportsAllErrorsAtOnce — người vận hành sửa MỘT LẦN thay vì chạy lại
// nhiều lần để phát hiện từng lỗi.
func TestReportsAllErrorsAtOnce(t *testing.T) {
	t.Setenv("APP_ENV", "khong-hop-le")
	t.Setenv("HTTP_PORT", "khong-phai-so")
	t.Setenv("LOG_LEVEL", "sai-muc")

	_, err := config.Load()
	if err == nil {
		t.Fatal("cấu hình sai phải trả lỗi")
	}

	msg := err.Error()
	for _, want := range []string{"APP_ENV", "HTTP_PORT", "LOG_LEVEL"} {
		if !strings.Contains(msg, want) {
			t.Errorf("thông báo lỗi thiếu %q — người vận hành phải chạy lại nhiều lần:\n%v",
				want, err)
		}
	}
}

func TestParsesDurations(t *testing.T) {
	t.Setenv("HTTP_READ_TIMEOUT", "5s")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "30s")
	t.Setenv("DB_CONN_MAX_LIFETIME", "1h")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load lỗi: %v", err)
	}
	if cfg.HTTP.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout: mong 5s, nhận %v", cfg.HTTP.ReadTimeout)
	}
	if cfg.HTTP.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout: mong 30s, nhận %v", cfg.HTTP.ShutdownTimeout)
	}
	if cfg.Database.ConnMaxLifetime != time.Hour {
		t.Errorf("ConnMaxLifetime: mong 1h, nhận %v", cfg.Database.ConnMaxLifetime)
	}
}

func TestRejectsInvalidDuration(t *testing.T) {
	t.Setenv("HTTP_READ_TIMEOUT", "5 giây")

	_, err := config.Load()
	if err == nil {
		t.Fatal("khoảng thời gian sai định dạng phải trả lỗi")
	}
	// Thông báo phải có ví dụ để người vận hành sửa được ngay.
	if !strings.Contains(err.Error(), "30s") {
		t.Errorf("thông báo lỗi nên kèm ví dụ định dạng đúng, nhận: %v", err)
	}
}

func TestEnvironmentHelpers(t *testing.T) {
	if !config.EnvProduction.IsProduction() {
		t.Error("EnvProduction.IsProduction() phải true")
	}
	if config.EnvDevelopment.IsProduction() {
		t.Error("EnvDevelopment.IsProduction() phải false")
	}
	if !config.EnvDevelopment.IsDevelopment() {
		t.Error("EnvDevelopment.IsDevelopment() phải true")
	}
}
