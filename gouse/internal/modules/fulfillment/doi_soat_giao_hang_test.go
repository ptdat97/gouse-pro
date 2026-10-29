package fulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment"
	fulfillmentpg "github.com/fashion-commerce/platform/internal/modules/fulfillment/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/opsconfig"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

// TestReconcileDeliveriesFindsLostContactParcels — bài chính của yêu cầu 5.
//
// Chạy trên DATABASE THẬT vì thứ đang kiểm là câu SQL: bộ lọc trạng thái
// và mốc `shipped_at` nằm trong đó, và một bản giả bằng Go sẽ kiểm chính
// vòng lặp mà production không chạy.
func TestReconcileDeliveriesFindsLostContactParcels(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	store := fulfillmentpg.NewFulfillmentStore(h.pool)

	sellerA := ids.MustNew(ids.PrefixSeller)
	fos := h.placeOrder(t, sellerA.String())
	fo := fos[0]

	h.handOver(t, sellerA.String(), fo.ID)

	// Đẩy mốc bàn giao lùi 10 ngày — quá ngưỡng mặc định 168 giờ.
	if _, err := h.pool.Exec(ctx,
		`UPDATE fulfillment_order SET shipped_at = now() - interval '10 days',
		        updated_at = now() - interval '10 days' WHERE id = $1`,
		fo.ID); err != nil {
		t.Fatalf("lùi mốc bàn giao: %v", err)
	}

	lostContact, err := store.ListInTransitBefore(ctx, time.Now().Add(-168*time.Hour), 100)
	if err != nil {
		t.Fatalf("ListInTransitBefore: %v", err)
	}
	if len(lostContact) != 1 {
		t.Fatalf("tìm được %d gói mất tin, mong 1", len(lostContact))
	}
	if got := lostContact[0].ID().String(); got != fo.ID {
		t.Errorf("gói = %s, mong %s", got, fo.ID)
	}
}

// TestAJustHandedOverParcelStaysOffTheList — ngưỡng phải thật sự lọc.
//
// Không có bài này thì một câu SQL quên mệnh đề `shipped_at < $1` vẫn
// xanh ở bài trên, và job sẽ gọi tên MỌI gói đang đi đường.
func TestAJustHandedOverParcelStaysOffTheList(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	store := fulfillmentpg.NewFulfillmentStore(h.pool)

	sellerA := ids.MustNew(ids.PrefixSeller)
	fos := h.placeOrder(t, sellerA.String())
	h.handOver(t, sellerA.String(), fos[0].ID)

	lostContact, err := store.ListInTransitBefore(ctx, time.Now().Add(-168*time.Hour), 100)
	if err != nil {
		t.Fatalf("ListInTransitBefore: %v", err)
	}
	if len(lostContact) != 0 {
		t.Errorf("gói VỪA bàn giao đã bị coi là mất tin (%d gói)", len(lostContact))
	}
}

// TestDaGiaoThiRaKhoiDanhSach — trên SQL, không chỉ trên domain.
//
// Bài domain đã khóa quy tắc; bài này khóa mệnh đề `status IN (...)`.
// Thiếu nó thì một gói đã giao xong vẫn nằm trong cảnh báo mãi mãi.
func TestOnceDeliveredItLeavesTheSQLList(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	store := fulfillmentpg.NewFulfillmentStore(h.pool)

	sellerA := ids.MustNew(ids.PrefixSeller)
	fos := h.placeOrder(t, sellerA.String())
	fo := fos[0]
	h.handOver(t, sellerA.String(), fo.ID)

	if _, err := h.pool.Exec(ctx,
		`UPDATE fulfillment_order SET shipped_at = now() - interval '10 days'
		  WHERE id = $1`, fo.ID); err != nil {
		t.Fatalf("lùi mốc: %v", err)
	}

	// Webhook về muộn: gói đã giao.
	if err := h.ful.MarkDelivered(ctx, sellerA.String(), fo.ID); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}

	lostContact, err := store.ListInTransitBefore(ctx, time.Now().Add(-168*time.Hour), 100)
	if err != nil {
		t.Fatalf("ListInTransitBefore: %v", err)
	}
	if len(lostContact) != 0 {
		t.Errorf("gói ĐÃ GIAO vẫn nằm trong danh sách mất tin (%d gói)", len(lostContact))
	}
}

// TestThieuCauHinhThiBAO LOI, không lặng lẽ trả rỗng.
//
// Một job đối chiếu không bao giờ tìm thấy gì trông hệt một hệ thống khỏe
// mạnh — và đó đúng là dạng hỏng mà công việc này sinh ra để bắt.
func TestMissingConfigReportsAnError(t *testing.T) {
	h := newHarness(t) // dựng KHÔNG có OpsConfig

	_, err := h.ful.ReconcileDeliveries(context.Background(), 100)
	if !errors.Is(err, fulfillment.ErrConfigNotWired) {
		t.Errorf("lỗi = %v, mong ErrConfigNotWired", err)
	}
}

// TestThresholdsComeFromOperationalConfig — đổi tham số phải có tác dụng NGAY.
//
// Ngưỡng đọc mỗi lượt chạy chứ không chụp lúc khởi động: nếu chụp thì
// người vận hành đổi con số rồi ngồi chờ mà không hiểu vì sao không đổi
// gì, cho tới lần khởi động lại kế tiếp.
func TestThresholdsComeFromOperationalConfig(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	db := testdb.Open(t)
	cfg := opsconfig.NewStore(ctx, db.Pool())

	ful, err := fulfillment.New(fulfillment.Config{
		Storage: "postgres", DB: db, OpsConfig: cfg,
	})
	if err != nil {
		t.Fatalf("fulfillment.New: %v", err)
	}

	sellerA := ids.MustNew(ids.PrefixSeller)
	fos := h.placeOrder(t, sellerA.String())
	fo := fos[0]
	h.handOver(t, sellerA.String(), fo.ID)

	// Im lặng 48 giờ: dưới ngưỡng mặc định 168 giờ.
	if _, err := h.pool.Exec(ctx,
		`UPDATE fulfillment_order SET shipped_at = now() - interval '48 hours',
		        updated_at = now() - interval '48 hours' WHERE id = $1`,
		fo.ID); err != nil {
		t.Fatalf("lùi mốc: %v", err)
	}

	parcels, err := ful.ReconcileDeliveries(ctx, 100)
	if err != nil {
		t.Fatalf("ReconcileDeliveries: %v", err)
	}
	if len(parcels) != 0 {
		t.Fatalf("với ngưỡng mặc định 168h, gói im 48h không được tính (%d gói)", len(parcels))
	}

	// Hạ ngưỡng xuống 24 giờ — cùng gói đó nay là mất tin.
	if err := cfg.Set(ctx, opsconfig.SetInput{
		Key: opsconfig.KeyDeliverySilenceHours, Value: 24,
		UpdatedBy: "test", Reason: "hạ ngưỡng để kiểm chứng cấu hình có tác dụng ngay",
	}, nil); err != nil {
		t.Fatalf("đặt cấu hình: %v", err)
	}

	parcels, err = ful.ReconcileDeliveries(ctx, 100)
	if err != nil {
		t.Fatalf("ReconcileDeliveries sau khi hạ ngưỡng: %v", err)
	}
	if len(parcels) != 1 {
		t.Errorf("hạ ngưỡng xuống 24h mà vẫn tìm được %d gói, mong 1", len(parcels))
	}
}

// banGiao đưa một đơn thực hiện đi hết tới lúc bàn giao vận chuyển.
func (h *harness) handOver(t *testing.T, sellerID, foID string) {
	t.Helper()
	ctx := context.Background()
	for _, step := range []struct {
		ten string
		lam func() error
	}{
		{"xác nhận", func() error { return h.ful.ConfirmFulfillment(ctx, sellerID, foID) }},
		{"đóng gói", func() error { return h.ful.MarkPacked(ctx, sellerID, foID) }},
		{"bàn giao", func() error {
			return h.ful.HandOverToCarrier(ctx, fulfillment.HandOverRequest{
				SellerID: sellerID, FulfillmentID: foID,
				Provider: "GHN", TrackingNumber: "GHN-" + foID[len(foID)-6:],
			})
		}},
	} {
		if err := step.lam(); err != nil {
			t.Fatalf("%s: %v", step.ten, err)
		}
	}
}

// TestDoiSoatKhongDuocTuDANH DAU DA GIAO — quy tắc quan trọng nhất ở đây.
//
// # Vì sao bài này tồn tại
//
// Cám dỗ rất thật: mỗi gói kẹt là tiền nhà bán chưa được chi, và một dòng
// `fo.Deliver(now)` trong vòng lặp "gỡ kẹt" được ngay. Nhưng suy ra "chắc
// là giao rồi" từ việc IM LẶNG là bịa ra một sự kiện chưa từng xảy ra — và
// ở đây nó bịa ra tiền: DELIVERED mở đường cho `CompleteDelivered` chuyển
// số dư sang khả dụng, tức là chi tiền cho một lần giao hàng không ai xác
// nhận. Cùng nguyên tắc với ADR-0017 phần 4.
//
// Bài này được viết SAU khi một lần phá thật lọt qua toàn bộ các bài trên:
// thêm hai dòng `Deliver` + `Update` vào vòng lặp mà mọi test vẫn xanh.
func TestReconciliationMustNotMarkDeliveredByItself(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	db := testdb.Open(t)
	cfg := opsconfig.NewStore(ctx, db.Pool())
	ful, err := fulfillment.New(fulfillment.Config{
		Storage: "postgres", DB: db, OpsConfig: cfg,
	})
	if err != nil {
		t.Fatalf("fulfillment.New: %v", err)
	}

	sellerA := ids.MustNew(ids.PrefixSeller)
	fos := h.placeOrder(t, sellerA.String())
	fo := fos[0]
	h.handOver(t, sellerA.String(), fo.ID)

	if _, err := h.pool.Exec(ctx,
		`UPDATE fulfillment_order SET shipped_at = now() - interval '30 days',
		        updated_at = now() - interval '30 days' WHERE id = $1`,
		fo.ID); err != nil {
		t.Fatalf("lùi mốc: %v", err)
	}

	parcels, err := ful.ReconcileDeliveries(ctx, 100)
	if err != nil {
		t.Fatalf("ReconcileDeliveries: %v", err)
	}
	if len(parcels) != 1 {
		t.Fatalf("mong tìm được 1 gói mất tin, được %d", len(parcels))
	}

	// Trạng thái phải KHÔNG đổi — đọc thẳng từ database, không qua cache.
	var status string
	var deliveredAt *time.Time
	if err := h.pool.QueryRow(ctx,
		`SELECT status, delivered_at FROM fulfillment_order WHERE id = $1`,
		fo.ID).Scan(&status, &deliveredAt); err != nil {
		t.Fatalf("đọc lại trạng thái: %v", err)
	}

	if status != "HANDED_OVER" {
		t.Errorf("trạng thái = %q sau khi đối soát, mong HANDED_OVER "+
			"— job đối soát KHÔNG được đổi trạng thái", status)
	}
	if deliveredAt != nil {
		t.Errorf("delivered_at = %v — job đã bịa ra một lần giao hàng "+
			"không ai xác nhận, và nó mở đường chi tiền cho nhà bán", *deliveredAt)
	}
}
