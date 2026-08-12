package postgres

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// Mã lỗi PostgreSQL.
//
// Dùng mã thay vì so khớp chuỗi thông báo: thông báo đổi theo phiên bản và
// theo ngôn ngữ máy chủ, mã thì ổn định.
const (
	codeUniqueViolation     = "23505"
	codeCheckViolation      = "23514"
	codeForeignKeyViolation = "23503"
	codeRaiseException      = "P0001" // RAISE EXCEPTION trong plpgsql
)

// isUniqueViolation cho biết lỗi có phải vi phạm UNIQUE của một ràng buộc
// cụ thể không.
//
// Kiểm tra TÊN ràng buộc chứ không chỉ mã lỗi: một bảng có nhiều ràng buộc
// UNIQUE, và "slug đã tồn tại" cần thông báo khác với "mã SKU đã tồn tại".
// Chỉ xét mã sẽ trả nhầm lỗi và người dùng sửa nhầm chỗ.
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != codeUniqueViolation {
		return false
	}
	if constraint == "" {
		return true
	}
	return pgErr.ConstraintName == constraint ||
		strings.Contains(pgErr.ConstraintName, constraint)
}

// IsCheckViolation cho biết lỗi có phải vi phạm CHECK không.
//
// Vi phạm CHECK nghĩa là dữ liệu lọt qua được kiểm tra ở domain nhưng vẫn
// sai — hoặc domain thiếu kiểm tra, hoặc có đường ghi nào đó không đi qua
// domain. Cả hai đều là lỗi lập trình cần sửa, không phải lỗi người dùng.
func IsCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeCheckViolation
}

// IsImmutableViolation cho biết lỗi có phải do ghi vào bảng bất biến không.
//
// Trigger của bảng lịch sử dùng RAISE EXCEPTION, sinh mã P0001.
func IsImmutableViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeRaiseException
}
