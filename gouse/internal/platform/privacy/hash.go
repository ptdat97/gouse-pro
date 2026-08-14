// Package privacy chứa tiện ích bảo vệ dữ liệu cá nhân.
//
// # Vì sao ở platform chứ không ở kernel
//
// Băm địa chỉ IP là HẠ TẦNG CHUNG, không phải khái niệm nghiệp vụ. Không
// có quy tắc kinh doanh nào của nền tảng thời trang nói về SHA-256.
// `kernel` giữ những khái niệm domain dùng chung (tiền, id, tỷ lệ) và phải
// tối thiểu tuyệt đối vì cả hệ thống phụ thuộc vào nó.
//
// # Vì sao không để mỗi module tự viết
//
// `identity` và `customer` đều cần đúng việc này. Hai bản sao chép sẽ trôi
// xa nhau, và khi đó cùng một địa chỉ IP băm ra hai giá trị khác nhau —
// đủ để phá mọi truy vấn "nhiều lần thử từ cùng một nguồn".
package privacy

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashIP băm địa chỉ IP để lưu nhật ký.
//
// # Vì sao không lưu IP nguyên văn
//
// Địa chỉ IP là DỮ LIỆU CÁ NHÂN ở nhiều thị trường: nó định vị được người
// dùng và cần cơ sở pháp lý để lưu. Băm giữ được thứ ta thật sự cần —
// "hai lần thử này có cùng nguồn không" — mà không giữ chính địa chỉ đó.
//
// # Vì sao SHA-256 chứ không bcrypt
//
// Đây KHÔNG phải mật khẩu. Không gian địa chỉ IPv4 chỉ có 2^32 giá trị,
// nên kẻ có bảng băm vẫn dò ngược được — băm chậm cũng không đổi điều đó.
// Mục tiêu ở đây là không lưu dữ liệu cá nhân dưới dạng đọc thẳng được,
// không phải chống dò ngược.
//
// Cần chống dò ngược thật thì phải thêm muối bí mật của hệ thống (HMAC),
// và khi đó mất luôn khả năng so hai bản ghi từ hai module khác nhau.
//
// Chuỗi rỗng trả về chuỗi rỗng: "không biết IP" phải phân biệt được với
// "IP có giá trị nào đó", chứ không thành một giá trị băm trông như thật.
func HashIP(ip string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:])
}
