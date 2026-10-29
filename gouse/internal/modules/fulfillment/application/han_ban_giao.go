package application

import "github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"

// CurrentSLA trả thời hạn bàn giao đang áp dụng.
//
// Đọc từ cấu hình vận hành nếu có, ngược lại dùng ngưỡng đã biên dịch.
// CÙNG nguồn với phép chấm điểm hiệu suất — hai nơi đọc hai con số khác
// nhau nghĩa là màn hình nói "còn 3 giờ" trong khi báo cáo ghi trễ.
func (s *Service) CurrentSLA() domain.Thresholds {
	if s.thresholds != nil {
		return s.thresholds.Thresholds()
	}
	return domain.DefaultThresholds()
}
