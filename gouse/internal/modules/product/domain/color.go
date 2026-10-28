package domain

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

// ColorFamily là nhóm màu dùng để LỌC.
//
// # Vì sao lọc theo nhóm chứ không theo tên màu
//
// Khách lọc theo "màu xanh", không lọc theo "Xanh navy đậm". Một sàn thời
// trang có hàng trăm tên màu do người bán tự đặt — "Trắng ngà", "Trắng
// kem", "Off-white" — và bộ lọc liệt kê từng cái là bộ lọc không ai dùng.
//
// Xem docs/02-domain/value-objects.md mục 3.2.
type ColorFamily string

const (
	ColorWhite  ColorFamily = "WHITE"
	ColorBlack  ColorFamily = "BLACK"
	ColorGrey   ColorFamily = "GREY"
	ColorRed    ColorFamily = "RED"
	ColorPink   ColorFamily = "PINK"
	ColorOrange ColorFamily = "ORANGE"
	ColorYellow ColorFamily = "YELLOW"
	ColorGreen  ColorFamily = "GREEN"
	ColorBlue   ColorFamily = "BLUE"
	ColorPurple ColorFamily = "PURPLE"
	ColorBrown  ColorFamily = "BROWN"
	ColorBeige  ColorFamily = "BEIGE"
	ColorSilver ColorFamily = "SILVER"
	ColorOther  ColorFamily = "OTHER"
)

// AttrColorFamily là khóa lưu NHÓM màu trong `attributes` của biến thể.
//
// Suy ra tự động từ AttrColor lúc tạo biến thể — người bán không phải khai
// thêm, và một trường phải khai thêm là một trường sẽ bị bỏ trống.
const AttrColorFamily = "color_family"

// familyKeywords ánh xạ từ khóa trong TÊN MÀU sang nhóm.
var familyKeywords = map[string]ColorFamily{
	"trắng": ColorWhite, "trang": ColorWhite, "white": ColorWhite, "ngà": ColorWhite,
	"đen": ColorBlack, "den": ColorBlack, "black": ColorBlack,
	"xám": ColorGrey, "xam": ColorGrey, "grey": ColorGrey, "gray": ColorGrey, "ghi": ColorGrey,
	"đỏ": ColorRed, "do": ColorRed, "red": ColorRed, "đô": ColorRed, "burgundy": ColorRed,
	"hồng": ColorPink, "hong": ColorPink, "pink": ColorPink,
	"cam": ColorOrange, "orange": ColorOrange,
	"vàng": ColorYellow, "vang": ColorYellow, "yellow": ColorYellow, "gold": ColorYellow,
	"xanh lá": ColorGreen, "xanh la": ColorGreen, "green": ColorGreen,
	"rêu": ColorGreen, "reu": ColorGreen, "olive": ColorGreen,
	"xanh": ColorBlue, "blue": ColorBlue, "navy": ColorBlue, "denim": ColorBlue,
	"tím": ColorPurple, "tim": ColorPurple, "purple": ColorPurple, "violet": ColorPurple,
	"nâu": ColorBrown, "nau": ColorBrown, "brown": ColorBrown,
	"be": ColorBeige, "beige": ColorBeige, "kem": ColorBeige, "cream": ColorBeige,
	"bạc": ColorSilver, "bac": ColorSilver, "silver": ColorSilver,
}

// keywordsByLength là các từ khóa xếp theo ĐỘ DÀI GIẢM DẦN.
//
// # Vì sao thứ tự là bắt buộc, không phải tối ưu
//
// Phép so khớp là "tên màu có CHỨA từ khóa không", và các từ khóa lồng
// nhau: "denim" chứa "den", "xanh lá" chứa "xanh", "beige" chứa "be".
// Duyệt map Go theo thứ tự NGẪU NHIÊN, nên cùng một tên màu có thể ra hai
// nhóm khác nhau ở hai lần chạy.
//
// Đã xảy ra thật: "Denim wash" ra BLACK vì "den" khớp trước "denim". Bài
// test bắt được ngay, nhưng trên production nó sẽ là "bộ lọc màu xanh
// thỉnh thoảng thiếu hàng" — thứ không ai truy ra nguyên nhân.
//
// Từ DÀI HƠN luôn cụ thể hơn, nên xét trước là đúng.
var keywordsByLength = sortByLength(familyKeywords)

func sortByLength(m map[string]ColorFamily) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		// So thêm theo bảng chữ cái để thứ tự XÁC ĐỊNH với từ cùng độ dài.
		return out[i] < out[j]
	})
	return out
}

// InferColorFamily đoán nhóm màu từ TÊN màu.
//
// # Giới hạn, nói trước
//
// Đây là phép đoán theo TỪ KHÓA trong tên, không phải phân loại theo mã
// màu. Tên do người bán tự đặt, nên có tên không khớp từ nào — chúng vào
// nhóm OTHER và vẫn tìm được bằng cách duyệt danh mục, chỉ là không lọc
// theo màu được.
//
// Cách đúng hơn là lưu `hex_code` rồi tính khoảng cách màu, nhưng nó cần
// người bán nhập mã màu — thứ hôm nay chưa có ở đâu. Khi có, hàm này là
// chỗ duy nhất phải đổi.
func InferColorFamily(colorName string) ColorFamily {
	s := strings.ToLower(strings.TrimSpace(colorName))
	if s == "" {
		return ColorOther
	}
	for _, kw := range keywordsByLength {
		if strings.Contains(s, kw) {
			return familyKeywords[kw]
		}
	}
	return ColorOther
}

// AttrColorHex là MÃ MÀU của biến thể, dạng "#RRGGBB".
//
// # Vì sao nó phải do NGƯỜI BÁN NHẬP
//
// Khác `color_family`: nhóm màu suy được từ TÊN màu (`InferColorFamily`), còn
// mã màu thì không. "Xanh navy" ra được nhóm BLUE, nhưng ra #1B2A49 hay
// #0A1F44 thì chỉ người cầm sản phẩm mới biết.
//
// Đặc tả khai từ đầu và nêu đúng lý do cần: "khách chọn theo màu nhìn
// thấy, không theo tên" (`common.yaml#/schemas/Color`). Bộ chọn màu bằng
// CHỮ bắt khách dịch "Xanh rêu" thành hình ảnh trong đầu, và đoán sai thì
// hàng bị trả về.
//
// TÙY CHỌN, có chủ ý: bắt buộc nó sẽ chặn mọi sản phẩm hiện có và mọi nhà
// bán chưa kịp lấy mã màu. Thiếu hex thì giao diện hiện ô chữ như cũ.
const AttrColorHex = "color_hex"

// ErrInvalidColorHex khi mã màu sai định dạng.
var ErrInvalidColorHex = errors.New(
	"product: mã màu phải có dạng #RRGGBB")

// colorHexPattern khớp ĐÚNG sáu chữ số hex sau dấu thăng.
//
// KHÔNG chấp nhận dạng rút gọn ba ký tự (#FFF): nó hợp lệ trong CSS nhưng
// tạo ra hai cách viết cho cùng một màu, và phép so sánh chuỗi ở mọi nơi
// khác sẽ coi chúng là hai màu khác nhau.
var colorHexPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// NormalizeColorHex kiểm và chuẩn hóa mã màu về CHỮ HOA.
//
// Chuỗi rỗng hợp lệ và trả về rỗng: mã màu là tùy chọn.
//
// Chuẩn hóa chữ hoa vì "#ff0000" và "#FF0000" là một màu, và để hai cách
// viết cùng tồn tại nghĩa là lọc theo mã màu sẽ bỏ sót một nửa.
func NormalizeColorHex(hex string) (string, error) {
	hex = strings.TrimSpace(hex)
	if hex == "" {
		return "", nil
	}
	if !colorHexPattern.MatchString(hex) {
		return "", ErrInvalidColorHex
	}
	return strings.ToUpper(hex), nil
}
