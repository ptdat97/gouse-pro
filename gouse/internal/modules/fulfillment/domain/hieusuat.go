package domain

import "time"

// SLAGiaoHang là thời hạn nhà bán phải bàn giao cho đơn vị vận chuyển,
// tính từ lúc đơn thực hiện được tạo.
//
// # Vì sao là một HẰNG SỐ CÔNG KHAI, không phải cột trong bảng
//
// Đặc tả yêu cầu "chỉ số, ngưỡng, và tác động đều công khai và tường
// minh", với lý do: mô hình chấm điểm hộp đen tạo tranh chấp không giải
// quyết được. Một thời hạn khác nhau theo từng đơn mà không ai nhìn thấy
// chính là hộp đen.
//
// Con số này đi thẳng vào response, nên nhà bán luôn đọc được thước đo
// đang dùng để chấm mình.
const SLAGiaoHang = 48 * time.Hour

// Ngưỡng đạt của từng chỉ số.
//
// Lấy từ ví dụ trong đặc tả (api/paths/seller.yaml#/performance) chứ không
// tự đặt: đó là con số đã được thống nhất khi viết hợp đồng API.
const (
	NguongHuyDon      = 0.03 // tỷ lệ hủy TỐI ĐA
	NguongGiaoDungHan = 0.95 // tỷ lệ giao đúng hạn TỐI THIỂU
)

// Nguong là bộ ngưỡng đang áp dụng.
//
// Gom thành MỘT kiểu thay vì ba tham số rời: ba con số float truyền liền
// nhau là chỗ dễ đảo thứ tự nhất, và đảo `HuyDon` với `GiaoDungHan` cho ra
// kết luận ngược hẳn mà trình biên dịch không nói gì.
type Nguong struct {
	SLAGiaoHang time.Duration
	HuyDon      float64
	GiaoDungHan float64
	MauToiThieu int
}

// NguongMacDinh là bộ ngưỡng dùng khi chưa ai đặt.
//
// Vẫn nằm ở đây, không chuyển hết sang cấu hình: hệ thống phải chạy đúng
// khi database không đọc được, và "chạy đúng" nghĩa là dùng đúng con số đã
// thống nhất khi viết hợp đồng API.
func NguongMacDinh() Nguong {
	return Nguong{
		SLAGiaoHang: SLAGiaoHang,
		HuyDon:      NguongHuyDon,
		GiaoDungHan: NguongGiaoDungHan,
		MauToiThieu: MauToiThieu,
	}
}

// TrangThaiChiSo là kết luận về một chỉ số.
type TrangThaiChiSo string

const (
	ChiSoTot         TrangThaiChiSo = "GOOD"
	ChiSoCanhBao     TrangThaiChiSo = "WARNING"
	ChiSoNghiemTrong TrangThaiChiSo = "CRITICAL"
)

// ChiSoHieuSuat là một phép đo kèm ngưỡng và kết luận.
type ChiSoHieuSuat struct {
	Ten       string
	GiaTri    float64
	Nguong    float64
	TrangThai TrangThaiChiSo
}

// SoLieuHieuSuat là số đếm thô lấy từ kho lưu trữ.
//
// Tách khỏi phần tính toán để việc phân loại ngưỡng kiểm được mà không cần
// database — đó là chỗ dễ sai nhất và cũng là chỗ rẻ nhất để kiểm.
type SoLieuHieuSuat struct {
	// TongDon là số đơn thực hiện tạo ra trong kỳ.
	TongDon int

	// DonHuy là số đơn đã hủy trong kỳ.
	DonHuy int

	// DonDaGiao là số đơn đã bàn giao cho vận chuyển (có shipped_at).
	DonDaGiao int

	// DonGiaoDungHan là số đơn bàn giao trong hạn SLA.
	DonGiaoDungHan int
}

// TinhChiSo đổi số đếm thô thành danh sách chỉ số kèm kết luận.
//
// # Vì sao KHÔNG trả chỉ số khi mẫu quá nhỏ
//
// Một nhà bán có 3 đơn, hủy 1, ra tỷ lệ hủy 33% và bị chấm CRITICAL. Con
// số đó không nói lên điều gì về chất lượng — nó nói về việc mẫu quá nhỏ.
// Chấm một gian hàng mới mở là NGHIÊM TRỌNG vì họ hủy đúng một đơn là kiểu
// bất công mà đặc tả sinh ra để tránh.
//
// # Vì sao NHẬN ngưỡng thay vì đọc hằng số
//
// Ba con số này là CHÍNH SÁCH KINH DOANH, sửa được từ giao diện quản trị
// (xem platform/opsconfig). Domain không đọc cấu hình — nó nhận vào và
// tính, nên vẫn kiểm được mà không cần database, và vẫn đúng với bất kỳ
// bộ ngưỡng nào.
func TinhChiSo(s SoLieuHieuSuat, ng Nguong) []ChiSoHieuSuat {
	var ra []ChiSoHieuSuat

	if s.TongDon >= ng.MauToiThieu {
		tyLeHuy := float64(s.DonHuy) / float64(s.TongDon)
		ra = append(ra, ChiSoHieuSuat{
			Ten:    "cancellation_rate",
			GiaTri: tyLeHuy,
			Nguong: ng.HuyDon,
			// Vượt ngưỡng là NGHIÊM TRỌNG, không phải cảnh báo: mỗi đơn
			// hủy là một khách đã trả tiền rồi không nhận được hàng.
			TrangThai: xepHangNguocNguong(tyLeHuy, ng.HuyDon),
		})
	}

	if s.DonDaGiao >= ng.MauToiThieu {
		tyLeDungHan := float64(s.DonGiaoDungHan) / float64(s.DonDaGiao)
		ra = append(ra, ChiSoHieuSuat{
			Ten:       "on_time_shipping_rate",
			GiaTri:    tyLeDungHan,
			Nguong:    ng.GiaoDungHan,
			TrangThai: xepHangTheoNguong(tyLeDungHan, ng.GiaoDungHan),
		})
	}

	return ra
}

// KyMacDinh là kỳ dùng khi người gọi không nêu.
//
// Khai ở domain để test và handler cùng đọc một nguồn: chép tay chuỗi
// "LAST_30_DAYS" ở hai nơi thì một ngày nào đó chúng lệch nhau.
const KyMacDinh = "LAST_30_DAYS"

// MauToiThieu là số đơn tối thiểu để một chỉ số có nghĩa.
const MauToiThieu = 10

// xepHangTheoNguong dùng cho chỉ số CÀNG CAO CÀNG TỐT.
//
// Vùng cảnh báo là 90% ngưỡng: rơi xuống dưới ngưỡng một chút là dấu hiệu
// cần chú ý, rơi sâu là vấn đề thật. Không có vùng đệm thì chỉ số nhảy
// giữa TỐT và NGHIÊM TRỌNG chỉ vì một đơn.
func xepHangTheoNguong(giaTri, nguong float64) TrangThaiChiSo {
	switch {
	case giaTri >= nguong:
		return ChiSoTot
	case giaTri >= nguong*0.9:
		return ChiSoCanhBao
	default:
		return ChiSoNghiemTrong
	}
}

// xepHangNguocNguong dùng cho chỉ số CÀNG THẤP CÀNG TỐT.
func xepHangNguocNguong(giaTri, nguong float64) TrangThaiChiSo {
	switch {
	case giaTri <= nguong:
		return ChiSoTot
	case giaTri <= nguong*1.5:
		return ChiSoCanhBao
	default:
		return ChiSoNghiemTrong
	}
}

// ChuaDo là một chỉ số đặc tả có khai nhưng hệ thống CHƯA đo được.
//
// # Vì sao trả ra thay vì im lặng bỏ qua
//
// Đặc tả yêu cầu minh bạch để tránh tranh chấp. Trả bốn chỉ số và im lặng
// về ba chỉ số còn lại tạo ra đúng thứ hộp đen ấy, chỉ khác là ở phía
// người viết API: nhà bán không có cách nào biết mình đang được chấm bằng
// những gì.
//
// Nói thẳng "chưa đo, vì lý do này" trung thực hơn hẳn một con số bịa.
type ChuaDo struct {
	Ten  string
	LyDo string
}

// ChiSoChuaDo liệt kê những gì đặc tả khai mà chưa có dữ liệu để tính.
func ChiSoChuaDo() []ChuaDo {
	return []ChuaDo{
		{
			Ten: "return_rate_description",
			LyDo: "cần dữ liệu của module returns; đó là module ở tầng khác " +
				"nên phải qua một read model chung, chưa dựng",
		},
		{
			Ten:  "average_rating",
			LyDo: "hệ thống chưa có đánh giá của khách — không có bảng nào lưu",
		},
		{
			Ten: "inventory_accuracy",
			LyDo: "chưa thống nhất công thức: nhật ký kho có ghi lượt điều " +
				"chỉnh nhưng chưa định nghĩa thế nào là một lần lệch",
		},
		{
			Ten: "buy_box_win_rate",
			LyDo: "buy box tính tại thời điểm hỏi và không được lưu lại, " +
				"nên không có lịch sử để tính tỷ lệ",
		},
	}
}
