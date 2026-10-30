package main

import "regexp"

// Ngữ âm tiếng Việt không dấu, viết thành ba tập ĐÓNG.
//
// # Vì sao HÌNH DẠNG chứ không phải DANH SÁCH TỪ
//
// Hai công cụ trước lọc theo danh sách từ Việt, và danh sách ấy đã vá năm
// lần vẫn sót: `Ghi`, `GiaTri`, `nguong`, `chung`, `ban`. Một danh sách từ
// là tập MỞ — mỗi lượt đổi tên lại gặp từ chưa ai nghĩ tới.
//
// Phụ âm đầu, nguyên âm, phụ âm cuối thì là tập ĐÓNG: tiếng Việt không
// sinh thêm âm tiết mới. Nên phép đo theo hình dạng không bao giờ sót vì
// "chưa nghĩ ra từ ấy" — nó chỉ sót vì âm tiết TRÙNG một từ tiếng Anh,
// và đó là thứ `allowed` nói tới, có liệt kê ra để đọc được.
const (
	onset = "ngh|ng|nh|ch|gh|gi|kh|ph|qu|th|tr|b|c|d|g|h|k|l|m|n|p|r|s|t|v|x"

	nucleus = "ieu|uoi|uou|uya|yeu|oai|oay|uay|oeo|uyu|" +
		"ai|ao|au|ay|eo|eu|ia|ie|iu|oa|oe|oi|oo|ua|ue|ui|uo|uu|uy|ya|ye|yu|" +
		"a|e|i|o|u|y"

	coda = "ngh|ng|nh|ch|c|m|n|p|t"
)

var reSyllable = regexp.MustCompile(
	`^(?:` + onset + `)?(?:` + nucleus + `)(?:` + coda + `)?$`)

// splitCamel tách một định danh thành các mảnh theo kiểu lạc đà.
//
// Viết tay chứ không dùng regexp: RE2 của Go KHÔNG có lookahead phủ
// định, và `[A-Z]+(?![a-z])` — nhánh giữ nguyên cụm viết hoa hết — cần
// đúng thứ ấy. `regexp.MustCompile` panic ngay lúc init, nên cái bẫy này
// ồn ào; nhưng viết lại bằng `strings.Split` theo hoa/thường thì ÂM
// THẦM: `JSON` thành `j s o n`, bốn âm tiết giả, và phép đo ồn tới mức
// không ai đọc nữa.
//
//	GetHTTPResponse  →  Get  HTTP  Response
//	jsonName         →  json  Name
//	chan429          →  chan  429
func splitCamel(s string) []string {
	var out []string
	start := 0
	for i := 1; i <= len(s); i++ {
		if i == len(s) {
			break
		}
		prev, cur := s[i-1], s[i]
		var boundary bool
		switch {
		case isUpper(cur) && (isLower(prev) || isDigit(prev)):
			boundary = true
		case isUpper(cur) && isUpper(prev) && i+1 < len(s) && isLower(s[i+1]):
			// Cuối một cụm viết hoa: `HTTPResponse` cắt trước `R`.
			boundary = true
		case isDigit(cur) != isDigit(prev):
			boundary = true
		}
		if boundary {
			out = append(out, s[start:i])
			start = i
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// vietnameseSyllables trả các âm tiết đáng ngờ trong một định danh.
func vietnameseSyllables(ident string) []string {
	var out []string
	for _, piece := range splitCamel(ident) {
		s := lower(piece)
		if len(s) < 2 || allowed[s] || !reSyllable.MatchString(s) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// lower hạ chữ mà KHÔNG gọi strings.ToLower trên chuỗi có dấu.
//
// Định danh Go hợp lệ có thể chứa chữ Unicode, và ToLower trên tiếng Việt
// CÓ DẤU trả về chữ có dấu — thứ không bao giờ khớp `reSyllable`. Hạ tay
// theo ASCII nên âm tiết có dấu rơi ra ngoài, và đó là đúng: định danh có
// dấu thì vi phạm hiển nhiên, đã có phép đo khác bắt.
func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
