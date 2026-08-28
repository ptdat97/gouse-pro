package domain

import (
	"sort"
	"strings"
)

// NhomMau là nhóm màu dùng để LỌC.
//
// # Vì sao lọc theo nhóm chứ không theo tên màu
//
// Khách lọc theo "màu xanh", không lọc theo "Xanh navy đậm". Một sàn thời
// trang có hàng trăm tên màu do người bán tự đặt — "Trắng ngà", "Trắng
// kem", "Off-white" — và bộ lọc liệt kê từng cái là bộ lọc không ai dùng.
//
// Xem docs/02-domain/value-objects.md mục 3.2.
type NhomMau string

const (
	MauTrang  NhomMau = "WHITE"
	MauDen    NhomMau = "BLACK"
	MauXam    NhomMau = "GREY"
	MauDo     NhomMau = "RED"
	MauHong   NhomMau = "PINK"
	MauCam    NhomMau = "ORANGE"
	MauVang   NhomMau = "YELLOW"
	MauXanhLa NhomMau = "GREEN"
	MauXanh   NhomMau = "BLUE"
	MauTim    NhomMau = "PURPLE"
	MauNau    NhomMau = "BROWN"
	MauBe     NhomMau = "BEIGE"
	MauBac    NhomMau = "SILVER"
	MauKhac   NhomMau = "OTHER"
)

// AttrColorFamily là khóa lưu NHÓM màu trong `attributes` của biến thể.
//
// Suy ra tự động từ AttrColor lúc tạo biến thể — người bán không phải khai
// thêm, và một trường phải khai thêm là một trường sẽ bị bỏ trống.
const AttrColorFamily = "color_family"

// tuKhoaNhomMau ánh xạ từ khóa trong TÊN MÀU sang nhóm.
var tuKhoaNhomMau = map[string]NhomMau{
	"trắng": MauTrang, "trang": MauTrang, "white": MauTrang, "ngà": MauTrang,
	"đen": MauDen, "den": MauDen, "black": MauDen,
	"xám": MauXam, "xam": MauXam, "grey": MauXam, "gray": MauXam, "ghi": MauXam,
	"đỏ": MauDo, "do": MauDo, "red": MauDo, "đô": MauDo, "burgundy": MauDo,
	"hồng": MauHong, "hong": MauHong, "pink": MauHong,
	"cam": MauCam, "orange": MauCam,
	"vàng": MauVang, "vang": MauVang, "yellow": MauVang, "gold": MauVang,
	"xanh lá": MauXanhLa, "xanh la": MauXanhLa, "green": MauXanhLa,
	"rêu": MauXanhLa, "reu": MauXanhLa, "olive": MauXanhLa,
	"xanh": MauXanh, "blue": MauXanh, "navy": MauXanh, "denim": MauXanh,
	"tím": MauTim, "tim": MauTim, "purple": MauTim, "violet": MauTim,
	"nâu": MauNau, "nau": MauNau, "brown": MauNau,
	"be": MauBe, "beige": MauBe, "kem": MauBe, "cream": MauBe,
	"bạc": MauBac, "bac": MauBac, "silver": MauBac,
}

// tuKhoaTheoDoDai là các từ khóa xếp theo ĐỘ DÀI GIẢM DẦN.
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
var tuKhoaTheoDoDai = xepTheoDoDai(tuKhoaNhomMau)

func xepTheoDoDai(m map[string]NhomMau) []string {
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

// SuyRaNhomMau đoán nhóm màu từ TÊN màu.
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
func SuyRaNhomMau(tenMau string) NhomMau {
	s := strings.ToLower(strings.TrimSpace(tenMau))
	if s == "" {
		return MauKhac
	}
	for _, tu := range tuKhoaTheoDoDai {
		if strings.Contains(s, tu) {
			return tuKhoaNhomMau[tu]
		}
	}
	return MauKhac
}
