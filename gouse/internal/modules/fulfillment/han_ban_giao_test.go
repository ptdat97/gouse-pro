package fulfillment_test

import (
	"context"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	fulfillmentpg "github.com/fashion-commerce/platform/internal/modules/fulfillment/infrastructure/postgres"
)

// TestHanTrenManHinhKHOP voi phep cham diem trong SQL.
//
// # Vì sao đây là bài quan trọng nhất của P3-17
//
// "Đơn này có đúng hạn không" nay có HAI cài đặt:
//
//	Go   `FulfillmentOrder.BanGiaoDungHan` — con số nhà bán thấy trên màn hình
//	SQL  `shipped_at <= created_at + $4::interval` — con số vào điểm hiệu suất
//
// Hai cài đặt cho cùng một câu hỏi là cách chắc chắn để chúng lệch nhau,
// và lệch ở đây nghĩa là nhà bán thấy "còn 3 giờ" trong khi báo cáo đã ghi
// họ trễ. Tranh chấp kiểu đó không giải quyết được bằng dữ liệu, vì cả hai
// bên đều đọc đúng thứ hệ thống nói với họ.
//
// Bài này bắt cả hai trả lời trên CÙNG dữ liệu và so kết quả.
func TestHanTrenManHinhKhopVoiPhepChamDiem(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sla := 48 * time.Hour

	sellerA := ids.MustNew(ids.PrefixSeller)

	fos := h.placeOrder(t, sellerA.String())
	if len(fos) != 1 {
		t.Fatalf("số đơn thực hiện = %d, cần 1", len(fos))
	}
	foID := fos[0].ID

	// Đưa đơn tới bàn giao, rồi ĐẶT LẠI mốc thời gian trong database để
	// dựng đúng ca cần thử: tạo lúc T, bàn giao lúc T + 48h + 1s.
	//
	// Sửa thẳng database chứ không chờ 48 giờ; đây là bài về CÔNG THỨC,
	// không phải về đồng hồ.
	for _, b := range []struct {
		ten string
		lam func() error
	}{
		{"xác nhận", func() error { return h.ful.ConfirmFulfillment(ctx, sellerA.String(), foID) }},
		{"nhặt hàng", func() error { return h.ful.MarkPicking(ctx, sellerA.String(), foID) }},
		{"đóng gói", func() error { return h.ful.MarkPacked(ctx, sellerA.String(), foID) }},
	} {
		if err := b.lam(); err != nil {
			t.Fatalf("%s: %v", b.ten, err)
		}
	}

	for _, tt := range []struct {
		ten         string
		treBaoNhieu time.Duration
		mongDungHan bool
	}{
		{"bàn giao ĐÚNG mốc hạn", 0, true},
		{"trễ một giây", time.Second, false},
		{"sớm một giờ", -time.Hour, true},
	} {
		t.Run(tt.ten, func(t *testing.T) {
			// Đặt created_at và shipped_at về đúng mốc cần thử.
			//
			// CẮT XUỐNG GIÂY, và đó không phải tiểu tiết: hạn hiển thị đi
			// qua RFC3339 nên mất phần dưới giây, còn truy vấn chấm điểm
			// so ở độ chính xác micro-giây của PostgreSQL. Bản đầu của bài
			// này dùng `time.Now()` nguyên vẹn và đỏ ở ca ĐÚNG MỐC HẠN —
			// màn hình nói trễ, điểm hiệu suất nói đúng hạn, lệch 953 mili-giây.
			//
			// Chênh lệch đó có thật nhưng dưới một giây, và một hạn hiển
			// thị cho người đọc thì tính bằng giây là đúng. Xem ghi chú ở
			// `HanBanGiao`.
			tao := time.Now().UTC().Truncate(time.Second).Add(-72 * time.Hour)
			giao := tao.Add(sla + tt.treBaoNhieu)
			if _, err := h.pool.Exec(ctx, `
				UPDATE fulfillment_order
				   SET created_at = $2, shipped_at = $3, status = 'HANDED_OVER'
				 WHERE id = $1`, foID, tao, giao); err != nil {
				t.Fatalf("đặt mốc thời gian: %v", err)
			}

			// Câu trả lời của SQL — thứ đi vào điểm hiệu suất.
			store := fulfillmentpg.NewFulfillmentStore(h.pool)
			so, err := store.DemHieuSuat(ctx, sellerA,
				tao.Add(-time.Hour), time.Now().UTC(), sla)
			if err != nil {
				t.Fatalf("DemHieuSuat: %v", err)
			}
			sqlDungHan := so.DonGiaoDungHan == 1

			// Câu trả lời của Go — thứ nhà bán thấy trên màn hình.
			fo, err := h.ful.GetSellerFulfillment(ctx, sellerA.String(), foID)
			if err != nil {
				t.Fatalf("GetSellerFulfillment: %v", err)
			}
			hanTrenManHinh, err := time.Parse(time.RFC3339, fo.SLADeadline)
			if err != nil {
				t.Fatalf("đọc hạn trên màn hình %q: %v", fo.SLADeadline, err)
			}
			goDungHan := !giao.After(hanTrenManHinh)

			if goDungHan != sqlDungHan {
				t.Errorf("MÀN HÌNH nói đúng hạn=%v, ĐIỂM HIỆU SUẤT nói %v — "+
					"hai câu trả lời cho cùng một câu hỏi. Hạn hiển thị %v, "+
					"bàn giao %v", goDungHan, sqlDungHan, hanTrenManHinh, giao)
			}
			if goDungHan != tt.mongDungHan {
				t.Errorf("đúng hạn = %v, mong %v", goDungHan, tt.mongDungHan)
			}
		})
	}
}
