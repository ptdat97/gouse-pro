package application

import (
	"time"

	"github.com/fashion-commerce/platform/internal/modules/pricing/infrastructure/inmemory"
)

// NewInMemoryService dựng Service chạy hoàn toàn trên bộ nhớ, dùng cho test.
//
// VÌ SAO Ở ĐÂY, KHÔNG Ở TẦNG TEST:
//
// Tầng interfaces KHÔNG được import infrastructure (quy tắc R8 của
// cmd/archcheck) — kể cả trong file test, vì công cụ quét cả file test và
// việc miễn trừ file test sẽ tạo lỗ hổng để lách quy tắc thật.
//
// Đặt hàm dựng ở đây giữ cho hướng phụ thuộc đúng chiều: application được
// phép biết infrastructure, còn interfaces chỉ biết application.
//
// clock có thể nil — khi đó dùng đồng hồ hệ thống.
func NewInMemoryService(clock Clock) *Service {
	return NewService(Deps{
		Prices:      inmemory.NewPriceStore(),
		Constraints: inmemory.NewConstraintStore(),
		History:     inmemory.NewHistoryStore(),
		Clock:       clock,
	})
}

// FixedClock là đồng hồ đứng yên, dùng cho test cần thời gian xác định.
//
// Test về giá flash và giá chiến dịch phải cho kết quả giống nhau dù chạy
// hôm nay hay sang năm.
type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T }
