package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/logger"
)

// giaCoDinh là bảng giá hãng vận chuyển cho test.
type giaCoDinh int64

func (g giaCoDinh) GiaMotKien(string) int64 { return int64(g) }

// TestChiPhiHangVanChuyenVaoSoKhiBanGiao.
//
// # Bất biến
//
// Bàn giao kiện hàng cho hãng vận chuyển làm phát sinh NGHĨA VỤ TRẢ TIỀN,
// và nghĩa vụ đó phải nằm trong sổ cái ngay lúc đó.
//
// # Vì sao nó quan trọng
//
// Phí vận chuyển khách trả đã được ghi là doanh thu nền tảng từ ADR-0018.
// Thiếu vế chi phí thì doanh thu bị THỔI LÊN đúng bằng khoản chưa ghi, và
// lãi/lỗ mảng vận chuyển — con số duy nhất trả lời "thu phí ship như vậy
// là lãi hay lỗ" — không tồn tại.
func TestChiPhiHangVanChuyenVaoSoKhiBanGiao(t *testing.T) {
	a := newAPITest(t)

	const giaTraHang = 18_000

	maDon := a.datDonCOD(t, "chiphiship")
	a.phatEventKemChiPhi(t, giaTraHang)

	foID, sellerID := a.donThucHienCuaDon(t, maDon)
	tokNB := a.taoTokenNhaBan(t, sellerID)

	// TRƯỚC khi bàn giao: chưa có nghĩa vụ nào với hãng.
	//
	// Không có khẳng định này thì một bên nhận ghi chi phí ở MỌI bước tiến
	// độ vẫn qua bài test — khóa idempotency giữ cho tổng đúng, nên chỉ
	// THỜI ĐIỂM sai, và thời điểm sai nghĩa là sổ ghi một nghĩa vụ chưa
	// phát sinh.
	if chiPhi, noHang := a.soDuVanChuyen(t, foID); chiPhi != 0 || noHang != 0 {
		t.Errorf("chưa bàn giao mà đã ghi chi phí %d / nợ hãng %d", chiPhi, noHang)
	}

	res := a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/ship",
		map[string]any{"shipping_provider": "GHN", "tracking_number": "GHN-CP-1"},
		hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code != http.StatusOK {
		t.Fatalf("bàn giao: HTTP %d — %s", res.code, res.raw)
	}
	a.phatEventKemChiPhi(t, giaTraHang)

	chiPhi, noHang := a.soDuVanChuyen(t, foID)
	if chiPhi != giaTraHang {
		t.Errorf("SHIPPING_EXPENSE = %d, cần %d — bàn giao rồi mà chi phí "+
			"chưa vào sổ", chiPhi, giaTraHang)
	}
	if noHang != giaTraHang {
		t.Errorf("CARRIER_PAYABLE = %d, cần %d", noHang, giaTraHang)
	}

	// Phát lại event tiến độ KHÔNG được tính chi phí lần hai.
	a.phatLaiTienDo(t)
	a.phatEventKemChiPhi(t, giaTraHang)

	chiPhi2, _ := a.soDuVanChuyen(t, foID)
	if chiPhi2 != chiPhi {
		t.Errorf("chi phí %d → %d sau khi phát lại event — tính hai lần cho "+
			"một kiện hàng", chiPhi, chiPhi2)
	}
}

// TestChuaKhaiGiaHangThiKhongGhiButToanNao.
//
// Hệ thống KHÔNG biết giá thỏa thuận với hãng: biểu phí của fulfillment là
// phí KHÁCH TRẢ. Chưa khai thì không ghi — đặt một con số mặc định sẽ làm
// lãi vận chuyển trông như một sự thật đã đo, trong khi không ai đo cả.
func TestChuaKhaiGiaHangThiKhongGhiButToanNao(t *testing.T) {
	a := newAPITest(t)

	maDon := a.datDonCOD(t, "chuakhaigia")
	a.phatEventKemChiPhi(t, 0) // 0 = chưa khai

	foID, sellerID := a.donThucHienCuaDon(t, maDon)
	tokNB := a.taoTokenNhaBan(t, sellerID)

	res := a.call(http.MethodPost,
		"/api/v1/seller/fulfillment-orders/"+foID+"/ship",
		map[string]any{"shipping_provider": "GHN", "tracking_number": "GHN-CP-2"},
		hopNhat(khoaIdem(), map[string]string{"Authorization": "Bearer " + tokNB}))
	if res.code != http.StatusOK {
		t.Fatalf("bàn giao: HTTP %d — %s", res.code, res.raw)
	}
	a.phatEventKemChiPhi(t, 0)

	chiPhi, noHang := a.soDuVanChuyen(t, foID)
	if chiPhi != 0 || noHang != 0 {
		t.Errorf("chưa khai giá mà vẫn ghi sổ: chi phí %d, nợ hãng %d — "+
			"con số này từ đâu ra?", chiPhi, noHang)
	}
}

// phatEventKemChiPhi phát event kèm bên nhận ghi chi phí vận chuyển.
func (a *apiTest) phatEventKemChiPhi(t *testing.T, gia int64) {
	t.Helper()
	log := logger.New("error", "text")
	bus := eventbus.NewDispatcher(a.db.Pool(), log)
	a.dangKyBenNhan(bus, log)
	bus.Subscribe(payment.NewChiPhiVanChuyenHandler(
		a.mods.payment, giaCoDinh(gia), log))

	if _, err := bus.DispatchBatch(context.Background(), 200); err != nil {
		t.Fatalf("phát event: %v", err)
	}
}

// soDuVanChuyen trả số dư SHIPPING_EXPENSE và CARRIER_PAYABLE của MỘT kiện.
func (a *apiTest) soDuVanChuyen(t *testing.T, foID string) (chiPhi, noHang int64) {
	t.Helper()
	if err := a.db.Pool().QueryRow(context.Background(), `
		SELECT COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'SHIPPING_EXPENSE'), 0),
		       COALESCE(SUM(l.amount) FILTER (
		           WHERE l.account_type = 'CARRIER_PAYABLE'), 0)
		  FROM ledger_line  l
		  JOIN ledger_entry e ON e.id = l.entry_id
		 WHERE e.reference_id = $1`, foID).Scan(&chiPhi, &noHang); err != nil {
		t.Fatalf("đọc số dư vận chuyển: %v", err)
	}
	return chiPhi, noHang
}

// phatLaiTienDo đưa event tiến độ trở lại hàng đợi chưa xử lý.
func (a *apiTest) phatLaiTienDo(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.db.Pool().Exec(ctx, `
		UPDATE event_outbox SET published_at = NULL, attempts = 0
		 WHERE event_type = 'fulfillment.progress_changed'`); err != nil {
		t.Fatalf("đưa event trở lại hàng đợi: %v", err)
	}
	if _, err := a.db.Pool().Exec(ctx, `
		DELETE FROM event_processed
		 WHERE event_id IN (SELECT event_id FROM event_outbox
		                     WHERE event_type = 'fulfillment.progress_changed')`,
	); err != nil {
		t.Fatalf("xóa dấu đã xử lý: %v", err)
	}
}

// datDonCOD đặt một đơn COD, không áp mã giảm giá.
func (a *apiTest) datDonCOD(t *testing.T, nhan string) string {
	t.Helper()

	maPhien := a.dungPhienSanHoanTat(emailMoi(nhan), "0900333222")
	res := a.call(http.MethodPost, "/api/v1/checkout/"+maPhien+"/complete",
		map[string]any{"payment_method": "COD"}, khoaIdem())
	if res.code != http.StatusOK && res.code != http.StatusCreated {
		t.Fatalf("hoàn tất phiên: HTTP %d — %s", res.code, res.raw)
	}
	don, _ := res.body["order"].(map[string]any)
	maDon, _ := don["id"].(string)
	if maDon == "" {
		t.Fatalf("không lấy được mã đơn: %s", res.raw)
	}
	return maDon
}
