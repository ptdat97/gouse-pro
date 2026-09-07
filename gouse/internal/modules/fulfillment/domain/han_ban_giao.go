package domain

import "time"

// HanBanGiao là thời điểm nhà bán PHẢI bàn giao cho đơn vị vận chuyển.
//
// # Vì sao TÍNH RA chứ không lưu thành cột
//
// Chú thích của `SLAGiaoHang` đã quyết điều này: đặc tả yêu cầu "chỉ số,
// ngưỡng, và tác động đều công khai và tường minh", vì mô hình chấm điểm
// hộp đen tạo tranh chấp không giải quyết được. Một thời hạn riêng cho
// từng đơn, lưu ở đâu đó mà không ai nhìn thấy, chính là hộp đen.
//
// Hệ quả phải biết: đổi `fulfillment.shipping_sla_hours` làm DỜI hạn của
// mọi đơn đang chạy, không chỉ đơn mới. Đó là hành vi đã được ghi trong
// `HeQua` của tham số đó ("đổi con số này làm ĐỔI ĐIỂM hiệu suất của mọi
// nhà bán ở kỳ đang xem — kể cả những đơn đã giao xong từ trước").
//
// # Cùng MỘT công thức với phép chấm điểm
//
// Truy vấn chấm hiệu suất đếm đơn đúng hạn bằng
// `shipped_at <= created_at + sla`. Hàm này phải cho ra ĐÚNG mốc đó, nếu
// không nhà bán sẽ thấy "còn 3 giờ" trên màn hình trong khi báo cáo đã ghi
// họ trễ — hai câu trả lời cho cùng một câu hỏi.
// # Chênh lệch DƯỚI MỘT GIÂY, đã biết
//
// Hạn đi ra ngoài qua RFC3339 nên mất phần dưới giây, còn truy vấn chấm
// điểm so ở độ chính xác micro-giây của PostgreSQL. Một lần bàn giao rơi
// vào ĐÚNG giây của hạn có thể được màn hình và báo cáo đánh giá khác
// nhau.
//
// Không sửa, có chủ ý: một hạn hiển thị cho người đọc thì tính bằng giây
// là đúng, và cắt cả hai bên xuống giây sẽ làm phép chấm điểm bớt chính
// xác để chiều một trường hợp không ai gặp — nhà bán phải bàn giao trong
// đúng giây thứ 172.800 thì mới chạm tới nó.
func (f *FulfillmentOrder) HanBanGiao(sla time.Duration) time.Time {
	return f.createdAt.Add(sla)
}

// BanGiaoDungHan cho biết đơn ĐÃ bàn giao có kịp hạn không.
//
// Chỉ có nghĩa khi đã bàn giao: chưa bàn giao thì câu hỏi đúng là "còn bao
// lâu", không phải "có kịp không". Trả false khi chưa bàn giao — và bên
// gọi phải kiểm `ShippedAt` trước, đúng như truy vấn chấm điểm làm
// (`shipped_at IS NOT NULL AND ...`).
func (f *FulfillmentOrder) BanGiaoDungHan(sla time.Duration) bool {
	if f.shippedAt.IsZero() {
		return false
	}
	return !f.shippedAt.After(f.HanBanGiao(sla))
}

// TreHan cho biết đơn CHƯA bàn giao đã quá hạn chưa.
//
// Khác `BanGiaoDungHan`: hàm kia nhìn lại quá khứ, hàm này nhìn hiện tại.
// Đơn đã bàn giao KHÔNG còn trễ được nữa, dù bàn giao muộn — việc đó đã
// tính vào điểm hiệu suất rồi, và hiện lại nhãn "trễ" trên một đơn đang đi
// đường chỉ làm nhà bán tưởng còn việc phải làm.
func (f *FulfillmentOrder) TreHan(sla time.Duration, now time.Time) bool {
	if !f.shippedAt.IsZero() || f.status == FOCancelled {
		return false
	}
	return now.After(f.HanBanGiao(sla))
}
