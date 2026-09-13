package supplychain_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/supplychain"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
	"github.com/fashion-commerce/platform/internal/platform/testdb"
)

func newModule(t *testing.T) (*supplychain.Module, *pgxpool.Pool) {
	t.Helper()

	db := testdb.Open(t)

	ctx := context.Background()
	for _, stmt := range []string{
		"TRUNCATE demand_signal",
		"TRUNCATE event_processed",
		"TRUNCATE event_outbox",
	} {
		if _, err := db.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("dọn dữ liệu: %v", err)
		}
	}

	m, err := supplychain.New(supplychain.Config{Storage: "postgres", DB: db})
	if err != nil {
		t.Fatalf("supplychain.New: %v", err)
	}
	return m, db.Pool()
}

// GHI TÍN HIỆU cơ bản, đọc lại đếm được.
func TestGhiVaDemTinHieu(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	skuID := ids.MustNew(ids.PrefixSKU).String()

	for i := 0; i < 3; i++ {
		if err := m.RecordSignal(ctx, supplychain.SignalRequest{
			Type:     supplychain.SignalAddToCart,
			SKUID:    skuID,
			Quantity: 1,
		}); err != nil {
			t.Fatalf("RecordSignal %d: %v", i, err)
		}
	}

	counts, err := m.CountSignals(ctx, "", "")
	if err != nil {
		t.Fatalf("CountSignals: %v", err)
	}
	if counts[supplychain.SignalAddToCart] != 3 {
		t.Errorf("số tín hiệu ADD_TO_CART = %d, mong 3",
			counts[supplychain.SignalAddToCart])
	}
}

// BA TÍN HIỆU GIÁ TRỊ NHẤT: nhu cầu KHÔNG được đáp ứng.
//
// Đây là lý do module tồn tại từ MVP. Chúng không xuất hiện trong dữ liệu
// bán hàng, nên nếu không ghi ngay thì mất vĩnh viễn:
//
//	Chỉ nhìn doanh số:  "Áo khoác bán 200" → nhu cầu là 200
//	Thực tế:            bán 200, hết hàng từ tuần 3,
//	                    1.500 lượt tìm sau khi hết,
//	                    400 lượt đăng ký báo hàng → gần 800
func TestGhiDuocNhuCauKhongDuocDapUng(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	skuID := ids.MustNew(ids.PrefixSKU).String()

	// Tìm kiếm KHÔNG có kết quả — không có SKU, đó chính là ý nghĩa.
	if err := m.RecordSignal(ctx, supplychain.SignalRequest{
		Type:       supplychain.SignalSearchNoResult,
		SearchTerm: "áo khoác dạ oversize",
	}); err != nil {
		t.Fatalf("ghi tìm kiếm không kết quả: %v", err)
	}

	// Hết hàng — quantity là số lượng KHÔNG đáp ứng được.
	if err := m.RecordSignal(ctx, supplychain.SignalRequest{
		Type:     supplychain.SignalStockout,
		SKUID:    skuID,
		Quantity: 12,
	}); err != nil {
		t.Fatalf("ghi hết hàng: %v", err)
	}

	// Đăng ký báo có hàng — khách chủ động để lại dấu vết.
	if err := m.RecordSignal(ctx, supplychain.SignalRequest{
		Type:  supplychain.SignalNotifyRequest,
		SKUID: skuID,
	}); err != nil {
		t.Fatalf("ghi đăng ký báo hàng: %v", err)
	}

	counts, err := m.CountSignals(ctx, "", "")
	if err != nil {
		t.Fatalf("CountSignals: %v", err)
	}

	for _, tc := range []struct {
		loai string
		mong int
	}{
		{supplychain.SignalSearchNoResult, 1},
		{supplychain.SignalStockout, 1},
		{supplychain.SignalNotifyRequest, 1},
	} {
		if counts[tc.loai] != tc.mong {
			t.Errorf("%s = %d, mong %d — đây là tín hiệu đo nhu cầu chưa "+
				"đáp ứng, mất nó là mất vĩnh viễn", tc.loai, counts[tc.loai], tc.mong)
		}
	}
}

// TÍN HIỆU PHẢI CHỈ VÀO MỘT THỨ GÌ ĐÓ.
//
// Không có sku, product, category lẫn từ khóa thì tín hiệu không nói lên
// điều gì và chỉ làm phồng bảng ghi nhiều nhất hệ thống.
func TestTinHieuPhaiCoDoiTuong(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	err := m.RecordSignal(ctx, supplychain.SignalRequest{
		Type:     supplychain.SignalView,
		Quantity: 1,
	})
	if !errors.Is(err, supplychain.ErrInvalidInput) {
		t.Errorf("lỗi = %v, mong ErrInvalidInput", err)
	}
}

// LOẠI TÍN HIỆU KHÔNG HỢP LỆ bị chặn.
func TestLoaiTinHieuKhongHopLeBiChan(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	err := m.RecordSignal(ctx, supplychain.SignalRequest{
		Type:  "KHONG_TON_TAI",
		SKUID: ids.MustNew(ids.PrefixSKU).String(),
	})
	if !errors.Is(err, supplychain.ErrInvalidInput) {
		t.Errorf("lỗi = %v, mong ErrInvalidInput", err)
	}
}

// GHI THEO LÔ: một đơn ba dòng sinh ba tín hiệu trong một lượt.
func TestGhiTheoLo(t *testing.T) {
	m, _ := newModule(t)
	ctx := context.Background()

	reqs := make([]supplychain.SignalRequest, 0, 3)
	for i := 0; i < 3; i++ {
		reqs = append(reqs, supplychain.SignalRequest{
			Type:     supplychain.SignalOrder,
			SKUID:    ids.MustNew(ids.PrefixSKU).String(),
			Quantity: i + 1,
		})
	}

	if err := m.RecordSignals(ctx, reqs); err != nil {
		t.Fatalf("RecordSignals: %v", err)
	}

	counts, _ := m.CountSignals(ctx, "", "")
	if counts[supplychain.SignalOrder] != 3 {
		t.Errorf("số tín hiệu ORDER = %d, mong 3", counts[supplychain.SignalOrder])
	}
}

// THÊM GIỎ QUA EVENT sinh tín hiệu ADD_TO_CART.
//
// Đây là mắt xích hoàn thành bánh đà: hành vi của khách chảy ngược vào
// việc lập kế hoạch sản xuất, mà module cart không cần biết supply-chain
// tồn tại.
func TestEventThemGioSinhTinHieu(t *testing.T) {
	m, pool := newModule(t)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(supplychain.NewSignalHandler(m))

	skuID := ids.MustNew(ids.PrefixSKU)
	contentID := ids.MustNew(ids.PrefixContent)

	e, err := eventbus.NewEvent(
		eventbus.TypeCartItemAdded, eventbus.AggregateCart,
		ids.MustNew(ids.PrefixCart),
		map[string]any{
			"sku_id":            skuID.String(),
			"offer_id":          ids.MustNew(ids.PrefixOffer).String(),
			"quantity":          2,
			"source_content_id": contentID.String(),
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	counts, _ := m.CountSignals(ctx, "", "")
	if counts[supplychain.SignalAddToCart] != 1 {
		t.Fatalf("số tín hiệu ADD_TO_CART = %d, mong 1",
			counts[supplychain.SignalAddToCart])
	}

	// Nguồn giới thiệu phải được giữ: nó trả lời "nội dung nào tạo NHU CẦU
	// THẬT", không chỉ tạo lượt xem.
	var meta string
	if err := pool.QueryRow(ctx,
		`SELECT metadata->>'source_content_id' FROM demand_signal LIMIT 1`).
		Scan(&meta); err != nil {
		t.Fatalf("đọc metadata: %v", err)
	}
	if meta != contentID.String() {
		t.Errorf("nguồn nội dung = %q, mong %q — mất nó là mất khả năng đo "+
			"nội dung nào tạo nhu cầu thật", meta, contentID)
	}
}

// ĐẶT HÀNG QUA EVENT sinh tín hiệu ORDER cho từng dòng.
func TestEventDatHangSinhTinHieu(t *testing.T) {
	m, pool := newModule(t)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(supplychain.NewSignalHandler(m))

	e, err := eventbus.NewEvent(
		eventbus.TypeCheckoutCompleted, eventbus.AggregateCheckout,
		ids.MustNew(ids.PrefixCheckout),
		map[string]any{
			"order_id": ids.MustNew(ids.PrefixOrder).String(),
			"reservations": []map[string]any{
				{"sku_id": ids.MustNew(ids.PrefixSKU).String(), "quantity": 2},
				{"sku_id": ids.MustNew(ids.PrefixSKU).String(), "quantity": 1},
			},
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	counts, _ := m.CountSignals(ctx, "", "")
	if counts[supplychain.SignalOrder] != 2 {
		t.Errorf("số tín hiệu ORDER = %d, mong 2 (một cho mỗi dòng hàng)",
			counts[supplychain.SignalOrder])
	}
}

// PHÁT LẠI EVENT không ghi tín hiệu hai lần.
//
// Mô hình at-least-once nghĩa là event SẼ được phát lại. Ghi hai lần nghĩa
// là số liệu nhu cầu bị thổi phồng — và kế hoạch sản xuất theo đó sẽ thừa.
func TestPhatLaiKhongGhiTinHieuHaiLan(t *testing.T) {
	m, pool := newModule(t)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(supplychain.NewSignalHandler(m))

	e, err := eventbus.NewEvent(
		eventbus.TypeCartItemAdded, eventbus.AggregateCart,
		ids.MustNew(ids.PrefixCart),
		map[string]any{
			"sku_id":   ids.MustNew(ids.PrefixSKU).String(),
			"quantity": 1,
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("lượt 1: %v", err)
	}

	// Ép phát lại: mô phỏng worker chết trước khi kịp đánh dấu.
	if _, err := pool.Exec(ctx,
		`UPDATE event_outbox SET published_at = NULL`); err != nil {
		t.Fatalf("ép phát lại: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("lượt 2: %v", err)
	}

	counts, _ := m.CountSignals(ctx, "", "")
	if counts[supplychain.SignalAddToCart] != 1 {
		t.Errorf("số tín hiệu = %d, mong ĐÚNG 1 — ghi hai lần làm số liệu "+
			"nhu cầu bị thổi phồng và kế hoạch sản xuất bị thừa",
			counts[supplychain.SignalAddToCart])
	}
}

// THỜI ĐIỂM NGHIỆP VỤ khác thời điểm ghi.
//
// Tín hiệu đi qua outbox nên có độ trễ. Tổng hợp theo tuần mà dùng thời
// điểm ghi sẽ đẩy nhầm tín hiệu cuối tuần sang tuần sau.
func TestGiuThoiDiemNghiepVuKhongPhaiThoiDiemGhi(t *testing.T) {
	m, pool := newModule(t)
	ctx := context.Background()

	// Tín hiệu xảy ra 3 ngày trước.
	occurred := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Second)

	if err := m.RecordSignal(ctx, supplychain.SignalRequest{
		Type:       supplychain.SignalView,
		SKUID:      ids.MustNew(ids.PrefixSKU).String(),
		OccurredAt: occurred.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("RecordSignal: %v", err)
	}

	var stored time.Time
	if err := pool.QueryRow(ctx,
		`SELECT occurred_at FROM demand_signal LIMIT 1`).Scan(&stored); err != nil {
		t.Fatalf("đọc occurred_at: %v", err)
	}

	if !stored.UTC().Equal(occurred) {
		t.Errorf("thời điểm lưu = %v, mong %v — dùng thời điểm ghi sẽ đẩy "+
			"nhầm tín hiệu sang kỳ sau khi tổng hợp", stored.UTC(), occurred)
	}

	// Và lọc theo khoảng thời gian phải tìm được nó.
	from := occurred.Add(-time.Hour).Format(time.RFC3339)
	to := occurred.Add(time.Hour).Format(time.RFC3339)
	counts, err := m.CountSignals(ctx, from, to)
	if err != nil {
		t.Fatalf("CountSignals: %v", err)
	}
	if counts[supplychain.SignalView] != 1 {
		t.Errorf("lọc theo kỳ ra %d tín hiệu, mong 1", counts[supplychain.SignalView])
	}
}

// TestEventHetHangSinhTinHieuStockout.
//
// # Vì sao tín hiệu này quý hơn vẻ ngoài của nó
//
// Module này khai BA loại tín hiệu là lý do nó tồn tại từ MVP:
// SEARCH_NO_RESULT, STOCKOUT và NOTIFY_REQUEST — chúng đo nhu cầu KHÔNG
// được đáp ứng, thứ không bao giờ xuất hiện trong dữ liệu bán hàng.
//
// Trước bài này, chỉ loại ĐẦU có bên phát. Hai loại còn lại được khai
// trong domain, có chú thích giải thích vì sao chúng quan trọng, và không
// dòng mã nào tạo ra chúng.
//
//	Chỉ nhìn doanh số:  "bán 200 chiếc" → nhu cầu là 200
//	Thực tế:            bán 200, HẾT HÀNG từ tuần 3
//
// Kế hoạch sản xuất thiếu tín hiệu này sẽ liên tục làm thiếu đúng những
// mặt hàng bán chạy nhất.
func TestEventHetHangSinhTinHieuStockout(t *testing.T) {
	m, pool := newModule(t)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(supplychain.NewSignalHandler(m))

	skuID := ids.MustNew(ids.PrefixSKU)
	itemID := ids.MustNew(ids.PrefixInventoryItem)

	e, err := eventbus.NewEvent(
		eventbus.TypeInventoryDepleted, eventbus.AggregateItem, itemID,
		map[string]any{
			"inventory_item_id":  itemID.String(),
			"sku_id":             skuID.String(),
			"stock_location_id":  ids.MustNew(ids.PrefixStockLocation).String(),
			"inventory_owner_id": "own_platform",
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 100); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	counts, err := m.CountSignals(ctx, "", "")
	if err != nil {
		t.Fatalf("CountSignals: %v", err)
	}
	if counts[supplychain.SignalStockout] != 1 {
		t.Fatalf("số tín hiệu STOCKOUT = %d, mong 1 — nhu cầu bị bỏ lỡ "+
			"không được ghi, và dữ liệu này không tạo ngược được",
			counts[supplychain.SignalStockout])
	}

	// Tín hiệu phải trỏ đúng SKU: một tín hiệu không biết nói về mặt hàng
	// nào thì không dùng để lập kế hoạch được.
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM demand_signal
		 WHERE signal_type = 'STOCKOUT' AND sku_id = $1`,
		skuID.String()).Scan(&n); err != nil {
		t.Fatalf("đọc tín hiệu: %v", err)
	}
	if n != 1 {
		t.Errorf("tín hiệu STOCKOUT gắn với SKU: %d, cần 1", n)
	}
}

// TestEventTraHangSinhTinHieuKemLyDo.
//
// # Vì sao trả hàng là tín hiệu NHU CẦU, không chỉ là chi phí
//
// Với thời trang, LÝ DO hoàn là đầu vào để sửa bảng size và mô tả sản
// phẩm: "size nhỏ" lặp lại trên một mã nghĩa là bảng size của mã đó sai,
// và sửa bảng size rẻ hơn nhiều so với chịu tỷ lệ hoàn cao mãi.
//
// Con số ấy chỉ gom được khi lý do được CHUẨN HÓA — "áo bé quá" viết tự do
// thì không cộng được với "chật". Nên bài này kiểm cả việc mã lý do TỚI
// ĐƯỢC metadata của tín hiệu, không chỉ việc tín hiệu tồn tại.
func TestEventTraHangSinhTinHieuKemLyDo(t *testing.T) {
	m, pool := newModule(t)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(supplychain.NewSignalHandler(m))

	skuNho := ids.MustNew(ids.PrefixSKU).String()
	skuKhac := ids.MustNew(ids.PrefixSKU).String()

	e, err := eventbus.NewEvent(
		eventbus.TypeReturnRequested, eventbus.AggregateReturn,
		ids.MustNew(ids.PrefixReturnRequest),
		map[string]any{
			"order_id": ids.MustNew(ids.PrefixOrder).String(),
			"lines": []map[string]any{
				{"sku_id": skuNho, "quantity": 1, "reason_code": "SIZE_TOO_SMALL"},
				{"sku_id": skuKhac, "quantity": 2, "reason_code": "NOT_AS_DESCRIBED"},
			},
		})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := bus.Outbox().Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := bus.DispatchBatch(ctx, 10); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	// MỘT tín hiệu MỖI DÒNG: một yêu cầu có thể trả ba món với ba lý do
	// khác nhau, và gộp lại sẽ buộc phải chọn một lý do đại diện.
	var soTinHieu int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM demand_signal WHERE signal_type = 'RETURN'`).
		Scan(&soTinHieu); err != nil {
		t.Fatalf("đếm tín hiệu: %v", err)
	}
	if soTinHieu != 2 {
		t.Fatalf("ghi được %d tín hiệu RETURN, cần 2 (một mỗi dòng)", soTinHieu)
	}

	// LÝ DO phải tới được metadata — đó là phần đáng giữ nhất.
	var lyDo string
	if err := pool.QueryRow(ctx, `
		SELECT metadata->>'reason_code' FROM demand_signal
		 WHERE signal_type = 'RETURN' AND sku_id = $1`, skuNho).
		Scan(&lyDo); err != nil {
		t.Fatalf("đọc lý do: %v", err)
	}
	if lyDo != "SIZE_TOO_SMALL" {
		t.Errorf("mã lý do = %q, cần SIZE_TOO_SMALL — không có nó thì tín "+
			"hiệu chỉ nói CÓ hàng bị trả, không nói VÌ SAO, và dữ liệu "+
			"chất lượng của thời trang mất hết", lyDo)
	}

	// Số lượng phải giữ nguyên: trả 2 món khác trả 1 món.
	var sl int
	if err := pool.QueryRow(ctx, `
		SELECT quantity FROM demand_signal
		 WHERE signal_type = 'RETURN' AND sku_id = $1`, skuKhac).
		Scan(&sl); err != nil {
		t.Fatalf("đọc số lượng: %v", err)
	}
	if sl != 2 {
		t.Errorf("số lượng = %d, cần 2", sl)
	}
}

// TestEventYeuThichSinhHaiTinHieu.
//
// # HAI tín hiệu từ MỘT hành động, và vì sao không gộp
//
// Thêm vào yêu thích là "tôi muốn món này" — ý định mua rõ ràng, chỉ chưa
// đúng thời điểm. Bật thêm "báo khi có hàng" là việc KHÁC: khách nói "có
// hàng là tôi mua", cam kết mạnh hơn hẳn.
//
// Gộp thành một loại sẽ không phân biệt được hai mức cam kết, mà chênh
// lệch giữa chúng chính là thứ quyết định nên sản xuất bao nhiêu.
// `NOTIFY_REQUEST` là một trong BA tín hiệu module này tồn tại để thu, và
// trước hôm nay không bên phát nào tồn tại.
func TestEventYeuThichSinhHaiTinHieu(t *testing.T) {
	m, pool := newModule(t)
	ctx := context.Background()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewDispatcher(pool, log)
	bus.Subscribe(supplychain.NewSignalHandler(m))

	spXinBao := ids.MustNew(ids.PrefixProduct).String()
	spChiThich := ids.MustNew(ids.PrefixProduct).String()

	phat := func(productID string, muonBao bool) {
		t.Helper()
		e, err := eventbus.NewEvent(
			eventbus.TypeWishlistItemAdded, eventbus.AggregateCustomer,
			ids.MustNew(ids.PrefixCustomer),
			map[string]any{
				"customer_id":           ids.MustNew(ids.PrefixCustomer).String(),
				"product_id":            productID,
				"variant_id":            "",
				"notify_when_available": muonBao,
			})
		if err != nil {
			t.Fatalf("NewEvent: %v", err)
		}
		if err := bus.Outbox().Publish(ctx, e); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	phat(spXinBao, true)
	phat(spChiThich, false)
	if _, err := bus.DispatchBatch(ctx, 10); err != nil {
		t.Fatalf("DispatchBatch: %v", err)
	}

	dem := func(loai, productID string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM demand_signal
			 WHERE signal_type = $1 AND product_id = $2`, loai, productID).
			Scan(&n); err != nil {
			t.Fatalf("đếm tín hiệu %s: %v", loai, err)
		}
		return n
	}

	// Món xin báo: CẢ HAI tín hiệu.
	if got := dem("WISHLIST", spXinBao); got != 1 {
		t.Errorf("món xin báo có %d tín hiệu WISHLIST, cần 1", got)
	}
	if got := dem("NOTIFY_REQUEST", spXinBao); got != 1 {
		t.Errorf("món xin báo có %d tín hiệu NOTIFY_REQUEST, cần 1 — đây "+
			"là một trong ba tín hiệu quý nhất, và lời hứa 'có hàng là tôi "+
			"mua' bị mất", got)
	}

	// Món chỉ thích: CHỈ MỘT.
	if got := dem("WISHLIST", spChiThich); got != 1 {
		t.Errorf("món chỉ thích có %d tín hiệu WISHLIST, cần 1", got)
	}
	if got := dem("NOTIFY_REQUEST", spChiThich); got != 0 {
		t.Errorf("món KHÔNG xin báo mà có %d tín hiệu NOTIFY_REQUEST — "+
			"gộp hai mức cam kết lại thì không còn phân biệt được", got)
	}

	// Ghi theo SẢN PHẨM, không đoán SKU: khách thường thích cả sản phẩm
	// rồi mới chọn size, và điền một SKU đoán làm mọi phép tổng hợp theo
	// size sai.
	var sku string
	if err := pool.QueryRow(ctx, `
		SELECT sku_id FROM demand_signal
		 WHERE signal_type = 'WISHLIST' AND product_id = $1`, spXinBao).
		Scan(&sku); err != nil {
		t.Fatalf("đọc sku_id: %v", err)
	}
	if sku != "" {
		t.Errorf("sku_id = %q, cần RỖNG khi khách chưa chọn size", sku)
	}
}
