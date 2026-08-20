package e2e_test

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// phatThoEvent đẩy một event có payload TỰ DỰNG vào outbox.
//
// Dùng map thay vì struct có chủ ý: bài test cần dựng được payload THIẾU
// trường và payload có trường LẠ — hai thứ struct Go không cho phép.
func phatThoEvent(
	t *testing.T, w *world, loai string, aggID ids.ID, payload map[string]any,
) {
	t.Helper()
	e, err := eventbus.NewEvent(loai, eventbus.AggregateFulfillment, aggID, payload)
	if err != nil {
		t.Fatalf("dựng event: %v", err)
	}
	outbox := eventbus.NewOutbox(w.db.Pool())
	if err := outbox.Publish(context.Background(), e); err != nil {
		t.Fatalf("phát event: %v", err)
	}
}

// TestBenNhanChiuDuocTruongLA — quy tắc TIẾN HÓA SCHEMA, có test.
//
// # Vì sao quy tắc này phải có test chứ không chỉ nằm trong tài liệu
//
// "Thêm trường tùy chọn là tương thích ngược" chỉ đúng nếu bên nhận THẬT
// SỰ bỏ qua trường nó không biết. Một dòng `DisallowUnknownFields` thêm
// vào vì lý do chính đáng (bắt lỗi chính tả trong payload) sẽ phá vỡ điều
// đó cho MỌI bên nhận, và lỗi chỉ lộ ra khi bên phát được nâng cấp trước
// — tức là ở production, giữa đêm.
//
// Ngày 19/08 đã có một sự cố cùng họ: worker CŨ tiêu thụ event MỚI và bỏ
// qua trường mới trong im lặng. Bài này khóa chiều ngược lại — bên nhận
// MỚI phải chịu được payload có trường nó chưa biết.
func TestBenNhanChiuDuocTruongLa(t *testing.T) {
	w := newWorld(t)
	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	foID := datVaGiao(t, w, shop, skuID, 20, 5)

	// Payload đúng, CỘNG các trường bên nhận chưa từng biết.
	phatThoEvent(t, w, eventbus.TypeFulfillmentCancelled, ids.ID(foID),
		map[string]any{
			"order_id":       ids.MustNew(ids.PrefixOrder).String(),
			"fulfillment_id": foID,
			"seller_id":      shop.String(),
			"release_stock":  true,
			"lines": []map[string]any{
				{"sku_id": skuID.String(), "quantity": 5,
					"truong_moi_toanh": "bên nhận cũ chưa biết"},
			},

			// Ba trường của một phiên bản tương lai.
			"ly_do_huy":         "khách đổi ý",
			"nguoi_huy":         "usr_abc",
			"chi_phi_phat_sinh": map[string]any{"amount": 15000, "currency": "VND"},
		})
	w.drain()

	avail, commit := w.stock(skuID, shop)
	if avail != 20 || commit != 0 {
		t.Errorf("tồn kho %d/%d, cần 20/0 — trường lạ đã làm bên nhận bỏ cuộc",
			avail, commit)
	}
}

// TestBenNhanChiuDuocThieuTruongTuyChon: trường KHÔNG bắt buộc vắng mặt
// thì bên nhận vẫn phải chạy.
//
// Đây là chiều quan trọng khi TRIỂN KHAI BÊN NHẬN TRƯỚC — thứ tự bắt buộc
// theo quy trình. Trong khoảng giữa, bên nhận mới đang chạy còn bên phát
// vẫn là bản cũ, nên nó nhận payload THIẾU những trường nó vừa học được.
func TestBenNhanChiuDuocThieuTruongTuyChon(t *testing.T) {
	w := newWorld(t)
	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	foID := datVaGiao(t, w, shop, skuID, 20, 5)

	// KHÔNG có `stock_location_id` và không có `fo_number`.
	phatThoEvent(t, w, eventbus.TypeFulfillmentCancelled, ids.ID(foID),
		map[string]any{
			"order_id":       ids.MustNew(ids.PrefixOrder).String(),
			"fulfillment_id": foID,
			"seller_id":      shop.String(),
			"release_stock":  true,
			"lines": []map[string]any{
				{"sku_id": skuID.String(), "quantity": 5},
			},
		})
	w.drain()

	avail, commit := w.stock(skuID, shop)
	if avail != 20 || commit != 0 {
		t.Errorf("tồn kho %d/%d, cần 20/0 — thiếu trường tùy chọn làm bên nhận hỏng",
			avail, commit)
	}
}

// TestEventVersionDuocGhiVaDocLai: cột `event_version` phải đi được từ
// bên phát tới outbox.
//
// Cơ chế hiện mới có KHUNG — chưa bên nhận nào phân nhánh theo phiên bản,
// và đó là trạng thái chấp nhận được khi mọi event còn ở v1. Nhưng nếu
// bản thân con số không được ghi đúng thì ngày cần tới nó sẽ không có gì
// để dựa vào.
func TestEventVersionDuocGhiVaDocLai(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	datVaGiao(t, w, shop, skuID, 20, 2)

	rows, err := w.db.Pool().Query(ctx,
		`SELECT event_type, event_version FROM event_outbox ORDER BY id`)
	if err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}
	defer rows.Close()

	var soEvent int
	for rows.Next() {
		var loai string
		var phienBan int
		if err := rows.Scan(&loai, &phienBan); err != nil {
			t.Fatalf("đọc dòng: %v", err)
		}
		soEvent++
		if phienBan < 1 {
			t.Errorf("event %s có version = %d, phải >= 1", loai, phienBan)
		}
	}
	if soEvent == 0 {
		t.Fatal("không có event nào trong outbox — bài test không kiểm được gì")
	}
}

// TestMoiEventTrongChuoiDeuCoCorrelationID — PH-22.
//
// # Vì sao trường này phải có mặt ở MỌI mắt xích
//
// Một hành động của khách (bấm "Đặt hàng") sinh ra cả một CÂY event:
// checkout.completed → tách đơn thực hiện → chuyển tồn kho → ghi tín hiệu
// nhu cầu → gửi email. Chúng chạy ở tiến trình khác, vào thời điểm khác.
//
// Khi có sự cố, câu hỏi luôn là "đơn này đã đi qua những đâu". Trả lời
// được câu đó cần một mã chung xuyên suốt. Thiếu ở MỘT mắt xích là chuỗi
// đứt — và chuỗi đứt thì không lần được gì, kể cả những mắt còn nguyên.
//
// Trước 20/08: chỉ 2 chỗ phát có gọi `WithTrace`, phần còn lại luôn rỗng.
func TestMoiEventTrongChuoiDeuCoCorrelationID(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	shop := ids.MustNew(ids.PrefixSeller)
	skuID := ids.MustNew(ids.PrefixSKU)
	foID := datVaGiao(t, w, shop, skuID, 20, 3)

	// Thêm vài bước nữa để cây event rộng hơn.
	if err := w.ful.CancelFulfillment(
		ctx, shop.String(), foID, "khách đổi ý"); err != nil {
		t.Fatalf("hủy: %v", err)
	}
	w.drain()

	rows, err := w.db.Pool().Query(ctx,
		`SELECT event_type, coalesce(correlation_id, '') FROM event_outbox ORDER BY id`)
	if err != nil {
		t.Fatalf("đọc outbox: %v", err)
	}
	defer rows.Close()

	var tong, thieu int
	var viDu []string
	for rows.Next() {
		var loai, corr string
		if err := rows.Scan(&loai, &corr); err != nil {
			t.Fatalf("đọc dòng: %v", err)
		}
		tong++
		if corr == "" {
			thieu++
			viDu = append(viDu, loai)
		}
	}

	if tong == 0 {
		t.Fatal("không có event nào — bài test không kiểm được gì")
	}
	if thieu > 0 {
		t.Errorf("%d/%d event KHÔNG có correlation_id: %v", thieu, tong, viDu)
	}
}
