package domain

// ExportedFOSuffix mở foSuffix cho test ngoài gói.
//
// Hàm này không thuộc hợp đồng của domain — nó là chi tiết của việc tách
// đơn. Nhưng quy tắc nó cài (không bao giờ sinh ký tự ngoài A–Z) đáng
// được khóa trực tiếp thay vì phải dựng 27 nhà bán để chạm tới.
func ExportedFOSuffix(i int) string { return foSuffix(i) }
