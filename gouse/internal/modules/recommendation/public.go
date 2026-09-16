// Package recommendation gợi ý cho khách, tách khỏi mọi module thương mại.
//
// # Nguyên tắc quan trọng nhất (đặc tả mục 3)
//
// Module thương mại KHÔNG BAO GIỜ phụ thuộc trực tiếp vào cách gợi ý được
// tính. Bên gọi chỉ biết interface:
//
//	rule-based (hôm nay) → theo hành vi (Phase 3) → học máy (Phase 4)
//
// Đổi cài đặt không sửa bên gọi. Và gợi ý hỏng KHÔNG được làm hỏng việc
// bán hàng: mọi phương thức ở đây trả kết quả RỖNG thay vì lỗi khi không
// đủ căn cứ.
//
// # Vì sao chỉ MỘT phương thức hoạt động
//
// `size_recommendation` là trường đã khai trong `ProductDetail`, ở endpoint
// đang chạy, có người đọc thật. Bốn phương thức còn lại của đặc tả không có
// bên gọi nào — dựng chúng bây giờ là tạo thêm một ca "khai mà không ai
// gọi", dạng lỗi hay gặp nhất của dự án này.
//
// ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
package recommendation

import "context"

// API là hợp đồng công khai của module.
type API interface {
	// GoiYSize gợi ý size cho khách đang xem một sản phẩm.
	//
	// Trả nil khi không đủ căn cứ — KHÔNG đoán. Một gợi ý sai làm khách
	// chọn nhầm rồi đổ lỗi cho nền tảng, tức là làm TĂNG đúng tỷ lệ hoàn
	// hàng mà nó sinh ra để giảm.
	GoiYSize(ctx context.Context, req GoiYSizeRequest) (*GoiYSizeView, error)
}

// GoiYSizeRequest là dữ kiện bên gọi đã biết.
//
// # Vì sao bên gọi truyền `CacSize` vào
//
// "Lên một size" chỉ có nghĩa khi biết thang size của sản phẩm đang xem, và
// thang ấy khác nhau theo loại hàng: S/M/L, 38/39/40, hay Free.
//
// Module `product` là nơi biết thang đó. Truyền vào thay vì để module này
// hỏi ngược cũng chính là thứ cắt được phụ thuộc vòng — xem ADR-0019.
type GoiYSizeRequest struct {
	CustomerID string
	BrandID    string

	// CacSize theo ĐÚNG THỨ TỰ hiển thị, từ nhỏ tới lớn.
	//
	// KHÔNG được sắp theo bảng chữ cái: "S, M, L" sắp chữ cái thành
	// "L, M, S" và mọi phép "lên một size" đảo chiều.
	CacSize []string
}

// GoiYSizeView khớp đúng lược đồ `size_recommendation` của đặc tả API.
type GoiYSizeView struct {
	SuggestedSize string

	// Reason: PREVIOUS_PURCHASE hoặc RETURN_HISTORY.
	//
	// BODY_MEASUREMENTS có trong đặc tả nhưng chưa dùng được: khách chưa
	// lưu số đo ở đâu cả.
	Reason string
}
