package application

import "github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"

// SLAHienTai trả thời hạn bàn giao đang áp dụng.
//
// Đọc từ cấu hình vận hành nếu có, ngược lại dùng ngưỡng đã biên dịch.
// CÙNG nguồn với phép chấm điểm hiệu suất — hai nơi đọc hai con số khác
// nhau nghĩa là màn hình nói "còn 3 giờ" trong khi báo cáo ghi trễ.
func (s *Service) SLAHienTai() domain.Nguong {
	if s.nguong != nil {
		return s.nguong.Nguong()
	}
	return domain.NguongMacDinh()
}
