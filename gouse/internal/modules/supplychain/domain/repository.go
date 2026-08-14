package domain

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// SignalRepository là PORT cho kho tín hiệu nhu cầu.
//
// CHỈ CÓ GHI THÊM VÀ ĐỌC — không có Update, không có Delete.
//
// Sự thiếu vắng hai phương thức đó là điều quan trọng nhất của interface
// này. Tín hiệu là quan sát về một thời điểm đã qua; sửa nó nghĩa là sửa
// lịch sử, và toàn bộ giá trị của dữ liệu này nằm ở chỗ lịch sử đáng tin.
type SignalRepository interface {
	// Append ghi một tín hiệu.
	Append(ctx context.Context, s *Signal) error

	// AppendBatch ghi nhiều tín hiệu trong một lượt.
	//
	// Một đơn hàng ba dòng sinh ba tín hiệu. Ghi từng cái là ba lượt đi
	// database cho một sự kiện — với bảng ghi nhiều nhất hệ thống, đó là
	// khác biệt đáng kể.
	AppendBatch(ctx context.Context, signals []*Signal) error

	// CountByType đếm tín hiệu theo loại trong một khoảng thời gian.
	//
	// Dùng cho giám sát ở MVP: con số bằng 0 kéo dài nghĩa là đường ghi
	// tín hiệu đã hỏng, và mỗi ngày im lặng là một ngày dữ liệu mất vĩnh
	// viễn. Phase 2 sẽ có tổng hợp thật.
	CountByType(ctx context.Context, from, to time.Time) (map[SignalType]int, error)

	// CountForSKU đếm tín hiệu của một SKU theo loại.
	CountForSKU(ctx context.Context, skuID ids.ID, from, to time.Time) (map[SignalType]int, error)
}
