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
	Modules  ModulesConfig
	Auth     AuthConfig
}

const (
	// minJWTSecretLen khớp yêu cầu của platform/token (RFC 7518 mục 3.2
	// cho HS256). Kiểm tra ở CẢ HAI nơi có chủ ý: config chặn sớm với thông
	// báo hướng dẫn được, token chặn lần cuối cho mọi đường khởi tạo khác.
	minJWTSecretLen = 32

	// devJWTSecret là khóa mặc định KHI PHÁT TRIỂN.
	//
	// Nằm trong mã nguồn nên ai đọc repo cũng ký được token ADMIN. Chỉ dùng
	// được vì production bắt buộc AUTH_JWT_SECRET và sẽ không khởi động nếu
	// thiếu — xem Load().
	devJWTSecret = "development-only-jwt-secret-do-not-use-in-production"
)

// AuthConfig là cấu hình xác thực.
type AuthConfig struct {
	// JWTSecret là khóa ký access token. Tối thiểu 32 ký tự.
	//
	// Ở production BẮT BUỘC đặt qua biến môi trường. Khi phát triển, giá
	// trị mặc định được dùng để chạy ngay không cần cấu hình — nhưng chính
	// vì nó nằm trong mã nguồn nên KHÔNG BAO GIỜ được dùng ngoài máy lập
	// trình viên: ai đọc repo cũng tự ký được token vai trò ADMIN.
	JWTSecret string

	// SecureCookie bật cờ Secure trên cookie refresh token.
	//
	// Bật ở mọi môi trường trừ development: trình duyệt không gửi cookie
	// Secure qua HTTP, nên bật nó ở localhost sẽ làm đăng nhập không chạy.
	SecureCookie bool
}

// ModulesConfig là cấu hình chung cho các module nghiệp vụ.
type ModulesConfig struct {
	// Storage chọn kho lưu trữ: "memory" hoặc "postgres".
	//
	// "memory" cho phép chạy và kiểm chứng mô hình domain khi chưa dựng
	// database. Dữ liệu MẤT khi tiến trình dừng, nên bị cấm ở production.
	Storage string
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

	storage := strings.ToLower(getEnvDefault("MODULES_STORAGE", defaultStorage(env)))
	switch storage {
	case "memory":
		// Kho in-memory mất TOÀN BỘ dữ liệu khi tiến trình khởi động lại.
		// Bật nhầm ở production nghĩa là mất đơn hàng — chặn từ lúc khởi
		// động thay vì phát hiện sau sự cố.
		if env.IsProduction() {
			collect(errors.New("MODULES_STORAGE=memory bị cấm ở production: dữ liệu sẽ mất khi khởi động lại"))
		}
	case "postgres":
	default:
		collect(fmt.Errorf("MODULES_STORAGE không hợp lệ: %q (phải là memory|postgres)", storage))
	}

	jwtSecret := os.Getenv("AUTH_JWT_SECRET")
	if jwtSecret == "" {
		// Ở production, thiếu khóa là lỗi khởi động. Sinh khóa ngẫu nhiên
		// thay thế nghe có vẻ tiện, nhưng khi chạy nhiều bản sao thì mỗi
		// bản ký bằng một khóa khác nhau: token do bản A cấp bị bản B từ
		// chối, và người dùng bị đăng xuất ngẫu nhiên.
		if env.IsProduction() {
			collect(errors.New(
				"AUTH_JWT_SECRET bắt buộc ở môi trường production"))
		} else {
			jwtSecret = devJWTSecret
		}
	}
	if jwtSecret != "" && len(jwtSecret) < minJWTSecretLen {
		collect(fmt.Errorf(
			"AUTH_JWT_SECRET phải dài tối thiểu %d ký tự, nhận %d — "+
				"khóa ngắn dò được bằng vét cạn, và khi đó bất kỳ ai cũng "+
				"tự phát hành được token vai trò ADMIN",
			minJWTSecretLen, len(jwtSecret)))
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
		Log:     LogConfig{Level: logLevel, Format: logFormat},
		Modules: ModulesConfig{Storage: storage},
		Auth: AuthConfig{
			JWTSecret: jwtSecret,
			// Bật ở mọi nơi TRỪ development — localhost chạy HTTP, mà trình
			// duyệt không gửi cookie Secure qua HTTP.
			SecureCookie: !env.IsDevelopment(),
		},
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

// defaultStorage: khi phát triển, chạy được ngay không cần dựng database;
// ngoài ra luôn mặc định PostgreSQL để không vô tình chạy bằng bộ nhớ.
func defaultStorage(env Environment) string {
	if env.IsDevelopment() {
		return "memory"
	}
	return "postgres"
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
