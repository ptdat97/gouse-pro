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

// hanCoDinh là cổng hạn đổi trả cho test.
type hanCoDinh time.Duration

func (h hanCoDinh) HanDoiTra() time.Duration { return time.Duration(h) }

// TestQuaHanThiKHONGTraDuoc — chỗ mất tiền.
//
// # Vì sao nó từng hở
//
// `XinTra` chỉ kiểm `don.DaGiao`, và `DaGiao` là true cho CẢ đơn
// COMPLETED — tức đơn đã hết hạn đổi trả và số dư nhà bán đã chuyển sang
// KHẢ DỤNG. Cho trả sau mốc đó nghĩa là nền tảng hoàn tiền cho khách
// trong khi nhà bán đã rút được, đúng thứ mà chú thích của
// `Order.Complete` gọi là "rất khó thu hồi".
//
// Trước hôm nay `DonHang` thậm chí KHÔNG mang mốc giao, nên không có gì
// để đếm hạn.
func TestQuaHanThiKhongTraDuoc(t *testing.T) {
	const han = 7 * 24 * time.Hour
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

	svc := application.NewService(application.Deps{
		Han:   hanCoDinh(han),
		Clock: dongHo{t: now},
	})

	// Giao cách đây 8 ngày — quá hạn 7 ngày.
	don := application.DonHang{
		ID:      ids.MustNew(ids.PrefixOrder),
		DaGiao:  true,
		GiaoLuc: now.Add(-8 * 24 * time.Hour),
	}
	if !svc.HetHanTra(don, now) {
		t.Error("đơn giao 8 ngày trước, hạn 7 ngày — phải HẾT hạn")
	}

	// Giao cách đây 6 ngày — còn hạn.
	don.GiaoLuc = now.Add(-6 * 24 * time.Hour)
	if svc.HetHanTra(don, now) {
		t.Error("đơn giao 6 ngày trước, hạn 7 ngày — phải CÒN hạn")
	}
}

// TestThieuDuLieuThiKHONGChan — phía an toàn.
//
// Đơn cũ tạo trước migration 000049 không có mốc giao, và bản dựng chưa
// nối cấu hình không có hạn. Cả hai đều KHÔNG chặn: chặn một yêu cầu hợp
// lệ vì thiếu dữ liệu lịch sử là từ chối quyền của khách do lỗi hệ thống.
func TestThieuDuLieuThiKhongChan(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	rat := now.Add(-365 * 24 * time.Hour)

	// Không có cổng hạn.
	svc := application.NewService(application.Deps{Clock: dongHo{t: now}})
	if svc.HetHanTra(application.DonHang{DaGiao: true, GiaoLuc: rat}, now) {
		t.Error("chưa nối cấu hình hạn mà đã chặn")
	}

	// Có cổng nhưng đơn không có mốc giao.
	svc2 := application.NewService(application.Deps{
		Han: hanCoDinh(7 * 24 * time.Hour), Clock: dongHo{t: now},
	})
	if svc2.HetHanTra(application.DonHang{DaGiao: true}, now) {
		t.Error("đơn cũ không có mốc giao mà đã chặn")
	}
}

type dongHo struct{ t time.Time }

func (d dongHo) Now() time.Time { return d.t }

// donGiaHetHan là cổng đọc đơn trả về một đơn ĐÃ QUÁ HẠN.
type donGiaHetHan struct{ giaoLuc time.Time }

func (d donGiaHetHan) LayDonDeTraHang(
	_ context.Context, orderID ids.ID,
) (application.DonHang, error) {
	return application.DonHang{
		ID: orderID, DaGiao: true, GiaoLuc: d.giaoLuc,
		Dong: []application.DongDonHang{{
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
// Hai bài trên kiểm `HetHanTra` trực tiếp. Một lần phá thật — bỏ hẳn lời
// gọi `HetHanTra` khỏi `XinTra` — lọt qua cả hai: quy tắc vẫn đúng, chỉ
// là không ai gọi nó ở chỗ cần. Đúng dạng lỗi mà mục 8 backlog liệt kê,
// lần này trong chính bài test của tôi.
func TestXinTraQuaHanBiTuChoi(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

	svc := application.NewService(application.Deps{
		Orders: donGiaHetHan{giaoLuc: now.Add(-8 * 24 * time.Hour)},
		Han:    hanCoDinh(7 * 24 * time.Hour),
		Clock:  dongHo{t: now},
	})

	_, err := svc.XinTra(context.Background(), application.XinTraInput{
		OrderID:    ids.MustNew(ids.PrefixOrder),
		CustomerID: ids.MustNew(ids.PrefixCustomer),
		Dong: []application.DongXinTra{{
			OrderLineID: ids.MustNew(ids.PrefixOrderLine),
			Quantity:    1,
			LyDo:        "CHANGED_MIND",
		}},
	})
	if !errors.Is(err, domain.ErrHetHanTra) {
		t.Errorf("lỗi = %v, mong ErrHetHanTra — đơn quá hạn vẫn xin trả được "+
			"nghĩa là nền tảng hoàn tiền sau khi nhà bán đã rút", err)
	}
}
