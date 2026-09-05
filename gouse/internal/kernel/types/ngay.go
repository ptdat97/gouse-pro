package types

import "time"

// MuiGioNghiepVu là múi giờ mà nền tảng vận hành: Việt Nam, UTC+7.
//
// # Vì sao cần một múi giờ nghiệp vụ, trong khi mọi mốc đều lưu UTC
//
// Lưu trữ và hiển thị không phải chỗ có vấn đề: cột `TIMESTAMPTZ` lưu một
// thời điểm tuyệt đối, API trả ISO 8601 UTC, giao diện tự đổi sang giờ máy.
//
// Chỗ có vấn đề là bộ lọc theo NGÀY. "Ngày 20/08" không phải một thời
// điểm, nó là một KHOẢNG — và khoảng đó bắt đầu lúc nửa đêm ở đâu? Cắt
// theo UTC thì một thao tác lúc 03:00 sáng 20/08 giờ Việt Nam (tức 20:00
// UTC ngày 19/08) rơi ra ngoài bộ lọc ngày 20/08, dù giao diện hiển thị
// chính bản ghi đó là "20/08 03:00".
//
// Với nhật ký kiểm toán, thứ tồn tại để điều tra sự cố, mất bảy giờ đầu
// mỗi ngày là khiếm khuyết thật: "cho tôi xem mọi việc hôm đó" im lặng bỏ
// sót ca đêm.
//
// # Vì sao FixedZone chứ không LoadLocation
//
// `time.LoadLocation("Asia/Ho_Chi_Minh")` cần cơ sở dữ liệu múi giờ có sẵn
// trên máy chủ. Ảnh container tối giản thường KHÔNG có, và hàm đó trả lỗi —
// xử lý cẩu thả một chút là rơi về UTC, tức đúng cái lỗi này quay lại,
// nhưng chỉ ở môi trường triển khai chứ không ở máy lập trình viên.
//
// Việt Nam giữ nguyên +07:00 từ 1975 và không có giờ mùa hè, nên một múi
// giờ cố định là mô tả ĐÚNG, không phải xấp xỉ cho tiện.
var MuiGioNghiepVu = time.FixedZone("ICT", 7*60*60)

// PhanTichNgay đọc chuỗi YYYY-MM-DD thành nửa đêm theo giờ nghiệp vụ.
//
// Chuỗi rỗng trả về thời điểm zero, nghĩa là "không giới hạn".
func PhanTichNgay(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("2006-01-02", s, MuiGioNghiepVu)
}

// CuoiNgay trả thời điểm cuối cùng của ngày chứa `t`, theo giờ nghiệp vụ.
//
// "đến ngày 31/08" phải bao gồm CẢ NGÀY 31, không dừng lúc 00:00 sáng hôm
// đó — nếu không, người lọc theo tháng mất sạch bản ghi của ngày cuối
// tháng mà không có lỗi nào báo.
func CuoiNgay(t time.Time) time.Time {
	return t.Add(24*time.Hour - time.Nanosecond)
}
