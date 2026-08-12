// Package config đọc cấu hình từ biến môi trường.
//
// Nguyên tắc:
//   - Cấu hình qua biến môi trường
//   - Bí mật (thông tin kết nối DB, khóa API) KHÔNG có giá trị mặc định —
//     thiếu là lỗi khởi động, không phải chạy với giá trị yếu
//   - Kiểm tra toàn bộ khi khởi động, không phải lúc dùng
//
// Xem docs/09-operations/deployment.md mục 6.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment là môi trường chạy.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

func (e Environment) IsProduction() bool  { return e == EnvProduction }
func (e Environment) IsDevelopment() bool { return e == EnvDevelopment }

// Config là toàn bộ cấu hình ứng dụng.
type Config struct {
	Env      Environment
	HTTP     HTTPConfig
	Log      LogConfig
	Database DatabaseConfig
}

type HTTPConfig struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	// MaxRequestBytes chống tấn công bằng payload lớn.
	MaxRequestBytes int64
}

type LogConfig struct {
	Level  string // debug | info | warn | error
	Format string // json | text
}

type DatabaseConfig struct {
	DSN string
	// MaxOpenConns đặt theo số lõi CPU của DATABASE, không theo số request.
	// Quá nhiều kết nối làm database CHẬM ĐI, không nhanh hơn.
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Load đọc và kiểm tra cấu hình.
//
// Trả về TẤT CẢ lỗi cùng lúc thay vì dừng ở lỗi đầu tiên — người vận hành
// sửa một lần thay vì chạy lại nhiều lần để phát hiện từng lỗi.
func Load() (*Config, error) {
	var errs []error
	collect := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	env := Environment(getEnvDefault("APP_ENV", string(EnvDevelopment)))
	switch env {
	case EnvDevelopment, EnvStaging, EnvProduction:
	default:
		collect(fmt.Errorf("APP_ENV không hợp lệ: %q (phải là development|staging|production)", env))
	}

	port, err := getIntDefault("HTTP_PORT", 8080)
	collect(err)

	readTimeout, err := getDurationDefault("HTTP_READ_TIMEOUT", 10*time.Second)
	collect(err)
	writeTimeout, err := getDurationDefault("HTTP_WRITE_TIMEOUT", 30*time.Second)
	collect(err)
	idleTimeout, err := getDurationDefault("HTTP_IDLE_TIMEOUT", 120*time.Second)
	collect(err)
	shutdownTimeout, err := getDurationDefault("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second)
	collect(err)
	maxBytes, err := getInt64Default("HTTP_MAX_REQUEST_BYTES", 1<<20) // 1 MiB
	collect(err)

	logLevel := strings.ToLower(getEnvDefault("LOG_LEVEL", defaultLogLevel(env)))
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		collect(fmt.Errorf("LOG_LEVEL không hợp lệ: %q", logLevel))
	}

	logFormat := strings.ToLower(getEnvDefault("LOG_FORMAT", defaultLogFormat(env)))
	switch logFormat {
	case "json", "text":
	default:
		collect(fmt.Errorf("LOG_FORMAT không hợp lệ: %q (phải là json|text)", logFormat))
	}

	maxOpen, err := getIntDefault("DB_MAX_OPEN_CONNS", 25)
	collect(err)
	maxIdle, err := getIntDefault("DB_MAX_IDLE_CONNS", 5)
	collect(err)
	connLifetime, err := getDurationDefault("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	collect(err)
	connIdleTime, err := getDurationDefault("DB_CONN_MAX_IDLE_TIME", 5*time.Minute)
	collect(err)

	dsn := os.Getenv("DATABASE_URL")
	// Ở production, thiếu DSN là lỗi khởi động — thà không khởi động còn hơn
	// chạy rồi thất bại ở request đầu tiên của khách.
	if dsn == "" && env.IsProduction() {
		collect(errors.New("DATABASE_URL bắt buộc ở môi trường production"))
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("cấu hình không hợp lệ:\n%w", errors.Join(errs...))
	}

	return &Config{
		Env: env,
		HTTP: HTTPConfig{
			Port:            port,
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			IdleTimeout:     idleTimeout,
			ShutdownTimeout: shutdownTimeout,
			MaxRequestBytes: maxBytes,
		},
		Log: LogConfig{Level: logLevel, Format: logFormat},
		Database: DatabaseConfig{
			DSN:             dsn,
			MaxOpenConns:    maxOpen,
			MaxIdleConns:    maxIdle,
			ConnMaxLifetime: connLifetime,
			ConnMaxIdleTime: connIdleTime,
		},
	}, nil
}

// defaultLogLevel: development ưu tiên chi tiết, production ưu tiên gọn.
func defaultLogLevel(env Environment) string {
	if env.IsDevelopment() {
		return "debug"
	}
	return "info"
}

// defaultLogFormat: text dễ đọc khi phát triển, JSON để truy vấn khi vận hành.
func defaultLogFormat(env Environment) string {
	if env.IsDevelopment() {
		return "text"
	}
	return "json"
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getIntDefault(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s không phải số nguyên: %q", key, v)
	}
	return n, nil
}

func getInt64Default(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s không phải số nguyên: %q", key, v)
	}
	return n, nil
}

func getDurationDefault(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s không phải khoảng thời gian hợp lệ: %q (ví dụ: 30s, 5m)", key, v)
	}
	return d, nil
}
