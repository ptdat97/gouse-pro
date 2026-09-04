// Package opsconfig cho phép sửa MỘT SỐ tham số vận hành lúc đang chạy,
// thay vì phải build lại và triển khai lại.
//
// # Vì sao không phải mọi hằng số đều vào đây
//
// Đây là quyết định quan trọng nhất của gói này, và nó là quyết định về
// AN TOÀN chứ không phải về tiện lợi.
//
// Có hai loại hằng số trong hệ thống, và chúng trông giống hệt nhau:
//
//	chính sách kinh doanh   "hạn giao là 48 giờ"  — người kinh doanh quyết
//	kiểm soát đúng đắn      "lý do tối thiểu 20 ký tự" — bảo vệ hệ thống
//
// Loại thứ hai KHÔNG được vào đây. Một kiểm soát tự nới lỏng được từ giao
// diện quản trị thì không còn là kiểm soát: người muốn lách nó chỉ cần đổi
// một con số, và thao tác đổi đó cũng do chính họ ký.
//
// Ví dụ cụ thể của thứ KHÔNG bao giờ nên đưa vào:
//
//	audit.minReasonLen     hạ xuống 1 là vô hiệu hóa toàn bộ nhật ký truy cập
//	token.minSecretLen     hạ xuống là mở cửa cho khóa yếu
//	eventbus.maxAttempts   hạ xuống là đẩy hàng đợi vào dead letter
//	identity.MaxFailedAttempts  nâng lên là mở cửa cho dò mật khẩu
//
// # Vì sao SỔ ĐĂNG KÝ ĐÓNG
//
// Chỉ khóa khai ở đây mới tồn tại. Gõ sai tên khóa thì lỗi ngay, không tạo
// ra một tham số ma mà không ai đọc. Và không có đường nào để thêm khóa
// mới từ giao diện — thêm khóa là việc của người viết mã, có review.
package opsconfig

import (
	"fmt"
	"sort"
	"time"
)

// Kieu là kiểu dữ liệu của một tham số.
type Kieu string

const (
	KieuThoiLuong Kieu = "duration" // time.Duration, nhập bằng giờ
	KieuTyLe      Kieu = "ratio"    // 0..1
	KieuSoNguyen  Kieu = "int"
)

// Khóa của các tham số. Chuỗi thuần, KHÔNG import module nào — quy tắc R3
// của archcheck cấm platform biết tới module nghiệp vụ.
const (
	KeySLAGiaoHang       = "fulfillment.shipping_sla_hours"
	KeyNguongHuyDon      = "fulfillment.max_cancellation_rate"
	KeyNguongGiaoDungHan = "fulfillment.min_on_time_rate"
	KeyMauToiThieu       = "fulfillment.min_sample_size"
)

// ThamSo mô tả một tham số vận hành.
type ThamSo struct {
	Khoa string
	Kieu Kieu

	// MacDinh là giá trị dùng khi chưa ai đặt, HOẶC khi không đọc được
	// giá trị đã đặt.
	//
	// Với thời lượng, đơn vị là GIỜ; với tỷ lệ là 0..1; với số nguyên là
	// chính nó.
	MacDinh float64

	// Min, Max là biên chấp nhận được.
	//
	// Bắt buộc, không phải tùy chọn: một tham số không có biên là một
	// tham số ai đó sẽ đặt bằng 0 và làm sập một thứ ở xa.
	Min, Max float64

	// MoTa và HeQua hiện trên giao diện quản trị.
	//
	// HeQua nói điều gì xảy ra khi đổi — người đổi con số thường không
	// phải người viết đoạn mã đọc nó.
	MoTa  string
	HeQua string
}

// soDang ký là danh sách ĐÓNG các tham số sửa được.
var soDangKy = map[string]ThamSo{
	KeySLAGiaoHang: {
		Khoa: KeySLAGiaoHang, Kieu: KieuThoiLuong,
		MacDinh: 48, Min: 1, Max: 720,
		MoTa: "Hạn nhà bán phải bàn giao cho đơn vị vận chuyển, tính từ " +
			"lúc đơn thực hiện được tạo.",
		HeQua: "Đổi con số này làm ĐỔI ĐIỂM hiệu suất của mọi nhà bán ở kỳ " +
			"đang xem — kể cả những đơn đã giao xong từ trước. Hạ xuống " +
			"khiến nhiều gian hàng đột ngột bị chấm là giao trễ.",
	},
	KeyNguongHuyDon: {
		Khoa: KeyNguongHuyDon, Kieu: KieuTyLe,
		MacDinh: 0.03, Min: 0, Max: 1,
		MoTa:  "Tỷ lệ hủy đơn TỐI ĐA còn được coi là đạt.",
		HeQua: "Ngưỡng chặt hơn làm nhiều gian hàng chuyển sang CẢNH BÁO.",
	},
	KeyNguongGiaoDungHan: {
		Khoa: KeyNguongGiaoDungHan, Kieu: KieuTyLe,
		MacDinh: 0.95, Min: 0, Max: 1,
		MoTa:  "Tỷ lệ giao đúng hạn TỐI THIỂU để được coi là đạt.",
		HeQua: "Ngưỡng cao hơn làm nhiều gian hàng chuyển sang CẢNH BÁO.",
	},
	KeyMauToiThieu: {
		Khoa: KeyMauToiThieu, Kieu: KieuSoNguyen,
		MacDinh: 10, Min: 1, Max: 10000,
		MoTa: "Số đơn tối thiểu trong kỳ thì mới chấm hiệu suất.",
		HeQua: "Hạ xuống quá thấp là chấm gian hàng mới mở bằng vài đơn — " +
			"một lần hủy thành tỷ lệ 33% và bị đánh giá NGHIÊM TRỌNG.",
	},
}

// Tham tra một tham số theo khóa.
func Tham(khoa string) (ThamSo, bool) {
	t, ok := soDangKy[khoa]
	return t, ok
}

// MoiThamSo trả toàn bộ sổ đăng ký, sắp theo khóa.
func MoiThamSo() []ThamSo {
	ra := make([]ThamSo, 0, len(soDangKy))
	for _, t := range soDangKy {
		ra = append(ra, t)
	}
	sort.Slice(ra, func(i, j int) bool { return ra[i].Khoa < ra[j].Khoa })
	return ra
}

// KiemGiaTri kiểm một giá trị có hợp lệ cho tham số này không.
//
// Kiểm ở đây chứ không ở tầng HTTP: mọi đường ghi đều phải đi qua, kể cả
// đường nạp dữ liệu hay một công cụ dòng lệnh sau này.
func (t ThamSo) KiemGiaTri(v float64) error {
	if v < t.Min || v > t.Max {
		return fmt.Errorf(
			"%w: %s phải trong khoảng [%g, %g], nhận %g",
			ErrNgoaiBien, t.Khoa, t.Min, t.Max, v)
	}
	if t.Kieu == KieuSoNguyen && v != float64(int64(v)) {
		return fmt.Errorf("%w: %s phải là số nguyên, nhận %g",
			ErrSaiKieu, t.Khoa, v)
	}
	return nil
}

// ThoiLuong đổi giá trị (giờ) thành time.Duration.
func (t ThamSo) ThoiLuong(v float64) time.Duration {
	return time.Duration(v * float64(time.Hour))
}
