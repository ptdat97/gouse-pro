package domain

import "time"

// DangTrenDuong cho biết gói hàng đang nằm trong tay đơn vị vận chuyển.
//
// Hai trạng thái này là nơi DUY NHẤT hệ thống phụ thuộc vào tin từ bên
// ngoài để đi tiếp: mọi trạng thái trước đó do nhà bán tự bấm, mọi trạng
// thái sau đó là kết cục. Không có webhook thì gói hàng đứng ở đây.
//
// DELIVERY_FAILED KHÔNG tính là đang trên đường: đó là tin ĐÃ VỀ, chỉ là
// tin xấu. Đơn cần người xử lý, nhưng không phải vì mất liên lạc.
func (f *FulfillmentOrder) DangTrenDuong() bool {
	return f.status == FOHandedOver || f.status == FOInTransit
}

// BatTinGiaoHang cho biết gói hàng đã bàn giao quá lâu mà KHÔNG có tin gì.
//
// # Đây là yêu cầu 5 của api/paths/webhooks.yaml
//
// "KHÔNG TIN TUYỆT ĐỐI — phải có đối chiếu định kỳ, vì webhook có thể
// mất." Đặc tả webhook vận chuyển còn nói rõ hơn: hệ thống dùng "hai cơ
// chế song song — webhook (thời gian thực) và hỏi định kỳ".
//
// Hôm nay chỉ có cơ chế thứ nhất, nên một webhook mất là gói hàng đứng
// vĩnh viễn ở HANDED_OVER. Và nó không đứng một mình: tiền của nhà bán
// chỉ chuyển sang khả dụng khi đơn thực hiện đi hết DELIVERED → COMPLETED
// (xem `Service.CompleteDelivered`), nên mỗi gói kẹt ở đây là một khoản
// phải trả bị giữ lại mà không ai biết vì sao.
//
// # `nguong` là ngưỡng NGHI NGỜ, không phải hạn giao hàng
//
// Nó không nói "gói này giao trễ" — chuyện đó là việc của đơn vị vận
// chuyển và đã có `HanBanGiao` cho phần trách nhiệm của nhà bán. Nó nói
// "đã lâu bất thường mà không có cập nhật NÀO, hãy đi hỏi". Vì vậy ngưỡng
// phải rộng hơn thời gian giao hàng thông thường, không bằng.
//
// Trả false khi chưa bàn giao: `shippedAt` rỗng nghĩa là gói chưa rời tay
// nhà bán, và việc đó do `TreHan` trông.
func (f *FulfillmentOrder) BatTinGiaoHang(nguong time.Duration, now time.Time) bool {
	if !f.DangTrenDuong() || f.shippedAt.IsZero() {
		return false
	}
	return now.After(f.shippedAt.Add(nguong))
}

// ImLangBaoLau trả về thời gian kể từ lần cuối có tin về gói hàng.
//
// Dùng để xếp thứ tự xử lý và để ghi vào cảnh báo — người trực cần biết
// "8 ngày" chứ không chỉ "quá ngưỡng".
//
// Mốc là `updatedAt` chứ không phải `shippedAt`: một gói đã đi qua
// IN_TRANSIT hôm qua thì có tin mới hơn lúc bàn giao, và tính từ lúc bàn
// giao sẽ làm nó trông đáng ngờ hơn thực tế.
func (f *FulfillmentOrder) ImLangBaoLau(now time.Time) time.Duration {
	moc := f.updatedAt
	if moc.IsZero() {
		moc = f.shippedAt
	}
	if moc.IsZero() {
		return 0
	}
	return now.Sub(moc)
}
