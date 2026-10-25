package returns_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/returns/application"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
)

// fixedWindow là cổng hạn đổi trả cho test.
type fixedWindow time.Duration

func (h fixedWindow) ReturnWindow() time.Duration { return time.Duration(h) }

// TestQuaHanThiKHONGTraDuoc — chỗ mất tiền.
//
// # Vì sao nó từng hở
//
// `Request` chỉ kiểm `ord.Delivered`, và `Delivered` là true cho CẢ đơn
// COMPLETED — tức đơn đã hết hạn đổi trả và số dư nhà bán đã chuyển sang
// KHẢ DỤNG. Cho trả sau mốc đó nghĩa là nền tảng hoàn tiền cho khách
// trong khi nhà bán đã rút được, đúng thứ mà chú thích của
// `Order.Complete` gọi là "rất khó thu hồi".
//
// Trước hôm nay `Order` thậm chí KHÔNG mang mốc giao, nên không có gì
// để đếm hạn.
func TestCannotReturnAfterWindowCloses(t *testing.T) {
	const window = 7 * 24 * time.Hour
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

	svc := application.NewService(application.Deps{
		Window: fixedWindow(window),
		Clock:  clock{t: now},
	})

	// Giao cách đây 8 ngày — quá hạn 7 ngày.
	ord := application.Order{
		ID:          ids.MustNew(ids.PrefixOrder),
		Delivered:   true,
		DeliveredAt: now.Add(-8 * 24 * time.Hour),
	}
	if !svc.ReturnWindowClosed(ord, now) {
		t.Error("đơn giao 8 ngày trước, hạn 7 ngày — phải HẾT hạn")
	}

	// Giao cách đây 6 ngày — còn hạn.
	ord.DeliveredAt = now.Add(-6 * 24 * time.Hour)
	if svc.ReturnWindowClosed(ord, now) {
		t.Error("đơn giao 6 ngày trước, hạn 7 ngày — phải CÒN hạn")
	}
}

// TestThieuDuLieuThiKHONGChan — phía an toàn.
//
// Đơn cũ tạo trước migration 000049 không có mốc giao, và bản dựng chưa
// nối cấu hình không có hạn. Cả hai đều KHÔNG chặn: chặn một yêu cầu hợp
// lệ vì thiếu dữ liệu lịch sử là từ chối quyền của khách do lỗi hệ thống.
func TestMissingDataDoesNotBlock(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	rat := now.Add(-365 * 24 * time.Hour)

	// Không có cổng hạn.
	svc := application.NewService(application.Deps{Clock: clock{t: now}})
	if svc.ReturnWindowClosed(application.Order{Delivered: true, DeliveredAt: rat}, now) {
		t.Error("chưa nối cấu hình hạn mà đã chặn")
	}

	// Có cổng nhưng đơn không có mốc giao.
	svc2 := application.NewService(application.Deps{
		Window: fixedWindow(7 * 24 * time.Hour), Clock: clock{t: now},
	})
	if svc2.ReturnWindowClosed(application.Order{Delivered: true}, now) {
		t.Error("đơn cũ không có mốc giao mà đã chặn")
	}
}

type clock struct{ t time.Time }

func (d clock) Now() time.Time { return d.t }

// expiredOrder là cổng đọc đơn trả về một đơn ĐÃ QUÁ HẠN.
type expiredOrder struct{ deliveredAt time.Time }

func (d expiredOrder) GetOrderForReturn(
	_ context.Context, orderID ids.ID,
) (application.Order, error) {
	return application.Order{
		ID: orderID, Delivered: true, DeliveredAt: d.deliveredAt,
		Lines: []application.OrderLine{{
			ID:       ids.MustNew(ids.PrefixOrderLine),
			SKUID:    ids.MustNew(ids.PrefixSKU),
			SellerID: ids.MustNew(ids.PrefixSeller),
			Quantity: 1,
		}},
	}, nil
}

// TestXinTraQuaHanBiTUCHOI — quy tắc phải được ÁP DỤNG, không chỉ tồn tại.
//
// # Vì sao bài này được viết SAU
//
// Hai bài trên kiểm `ReturnWindowClosed` trực tiếp. Một lần phá thật — bỏ hẳn lời
// gọi `ReturnWindowClosed` khỏi `Request` — lọt qua cả hai: quy tắc vẫn đúng, chỉ
// là không ai gọi nó ở chỗ cần. Đúng dạng lỗi mà mục 8 backlog liệt kê,
// lần này trong chính bài test của tôi.
func TestLateRequestIsRejected(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

	svc := application.NewService(application.Deps{
		Orders: expiredOrder{deliveredAt: now.Add(-8 * 24 * time.Hour)},
		Window: fixedWindow(7 * 24 * time.Hour),
		Clock:  clock{t: now},
	})

	_, err := svc.Request(context.Background(), application.RequestInput{
		OrderID:    ids.MustNew(ids.PrefixOrder),
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		Lines: []application.RequestedLine{{
			OrderLineID: ids.MustNew(ids.PrefixOrderLine),
			Quantity:    1,
			LyDo:        "CHANGED_MIND",
		}},
	})
	if !errors.Is(err, domain.ErrReturnWindowClosed) {
		t.Errorf("lỗi = %v, mong ErrReturnWindowClosed — đơn quá hạn vẫn xin trả được "+
			"nghĩa là nền tảng hoàn tiền sau khi nhà bán đã rút", err)
	}
}
