package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/platform/opsconfig"
)

// TestThuDuLieuHanhViDongPhieu.
//
// # Vì sao bài này tồn tại
//
// Đo trước khi có tuyến: **0 sự kiện xem sản phẩm / tìm kiếm / xem trang**
// từng được ghi. Hệ quả trên chỉ số thật: `conversion_rate` = 0 với cỡ mẫu
// 0, và `session_count` = 0 — trong khi `gmv`, `order_count`, `aov` tính
// đúng. Nền tảng trả lời được "bán bao nhiêu" nhưng mù với "bao nhiêu
// người xem mà không mua".
//
// docs/00-overview/vision.md gọi khâu `Behavior Data → Demand Signal` là
// chỗ đa số nền tảng thương mại điện tử đứt, và nói rõ dữ liệu này KHÔNG
// tạo ngược được.
func TestThuDuLieuHanhViDongPhieu(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	res := a.call(http.MethodPost, "/api/v1/events", map[string]any{
		"session_id": "ses-hanhvi-1",
		"events": []any{
			map[string]any{
				"name": "product_view", "subject_type": "product",
				"subject_id": "prd_01J9XABC123DEF456GHJKMNPQR",
			},
			map[string]any{
				"name":       "search",
				"properties": map[string]any{"query": "áo khoác dạ"},
			},
		},
	}, nil)
	if res.code != http.StatusOK {
		t.Fatalf("ghi sự kiện hành vi: HTTP %d — %s", res.code, res.raw)
	}
	if n, _ := res.body["accepted"].(float64); n != 2 {
		t.Fatalf("ghi được %v sự kiện, cần 2", n)
	}

	var soXem, soTim int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE event_name = 'product_view'),
		       count(*) FILTER (WHERE event_name = 'search')
		  FROM event_log WHERE session_id = $1`, "ses-hanhvi-1").
		Scan(&soXem, &soTim); err != nil {
		t.Fatalf("đọc event_log: %v", err)
	}
	if soXem != 1 || soTim != 1 {
		t.Errorf("ghi được %d lượt xem và %d lượt tìm, cần 1/1 — khúc đầu "+
			"của phễu vẫn trống", soXem, soTim)
	}

	// KHÔNG thu IP và user-agent: thu dữ liệu không dùng tới là tự tạo
	// nghĩa vụ bảo vệ mà không đổi lại được gì.
	var ipHash, ua string
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT ip_hash, user_agent FROM event_log
		 WHERE session_id = $1 LIMIT 1`, "ses-hanhvi-1").
		Scan(&ipHash, &ua); err != nil {
		t.Fatalf("đọc trường riêng tư: %v", err)
	}
	if ipHash != "" || ua != "" {
		t.Errorf("đường thu hành vi đã ghi ip_hash=%q user_agent=%q — "+
			"cả hai phải RỖNG", ipHash, ua)
	}
}

// TestChiNhanSuKienTrongDanhSachDong.
//
// Đây là endpoint CÔNG KHAI, không xác thực. Danh sách mở nghĩa là bất kỳ
// ai cũng ghi được sự kiện tên tùy ý vào bảng mà mọi chỉ số kinh doanh đọc
// từ đó — `purchase` giả sẽ thổi phồng tỷ lệ chuyển đổi, và không có cách
// nào phân biệt sau này.
func TestChiNhanSuKienTrongDanhSachDong(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	res := a.call(http.MethodPost, "/api/v1/events", map[string]any{
		"session_id": "ses-gia-mao",
		"events": []any{
			map[string]any{"name": "purchase", "subject_id": "prd_gia"},
			map[string]any{"name": "order.placed"},
			map[string]any{
				"name": "product_view", "subject_type": "product",
				"subject_id": "prd_01J9XABC123DEF456GHJKMNPQR",
			},
		},
	}, nil)
	if res.code != http.StatusOK {
		t.Fatalf("HTTP %d — %s", res.code, res.raw)
	}

	// Sự kiện hợp lệ đi cùng phải được ghi: một tên lạ không được làm mất
	// cả lô, nếu không client cũ sẽ làm thủng dữ liệu của client mới.
	if n, _ := res.body["accepted"].(float64); n != 1 {
		t.Errorf("ghi được %v sự kiện, cần ĐÚNG 1 (chỉ product_view)", n)
	}

	var gia int
	if err := a.db.Pool().QueryRow(ctx, `
		SELECT count(*) FROM event_log
		 WHERE session_id = $1 AND event_name IN ('purchase', 'order.placed')`,
		"ses-gia-mao").Scan(&gia); err != nil {
		t.Fatalf("đọc event_log: %v", err)
	}
	if gia != 0 {
		t.Errorf("%d sự kiện NGHIỆP VỤ giả lọt vào qua đường công khai — "+
			"mọi chỉ số kinh doanh đọc từ bảng này", gia)
	}
}

// TestGioiHanSuKienTheoPhien.
//
// Một client hỏng gửi vòng lặp hàng nghìn sự kiện một giây là tình huống
// thường gặp hơn kẻ cố tình phá, và nó làm hỏng dữ liệu y như vậy.
func TestGioiHanSuKienTheoPhien(t *testing.T) {
	a := newAPITest(t)
	tok := a.taoTaiKhoanVaiTro(t, identity.RoleAdmin)

	// Hạ ngưỡng xuống 3 để bài chạy nhanh — và đó chính là điểm: ngưỡng
	// là tham số vận hành, chỉnh được khi thấy lưu lượng thật.
	if res := a.datCauHinh(t, tok, opsconfig.KeyTranSuKienMotPhien, 3,
		"Ha nguong de kiem chung gioi han theo phien co tac dung"); res.code != http.StatusOK {
		t.Fatalf("đặt ngưỡng: HTTP %d — %s", res.code, res.raw)
	}

	lo := func(n int) reply {
		ev := make([]any, 0, n)
		for i := 0; i < n; i++ {
			ev = append(ev, map[string]any{
				"name": "product_view", "subject_type": "product",
				"subject_id": "prd_01J9XABC123DEF456GHJKMNPQR",
			})
		}
		return a.call(http.MethodPost, "/api/v1/events", map[string]any{
			"session_id": "ses-qua-nhanh", "events": ev,
		}, nil)
	}

	if res := lo(3); res.code != http.StatusOK {
		t.Fatalf("lô đầu trong ngưỡng bị từ chối: HTTP %d — %s", res.code, res.raw)
	}
	res := lo(1)
	if res.code != http.StatusTooManyRequests {
		t.Errorf("vượt ngưỡng mà trả HTTP %d, cần 429 — giới hạn theo "+
			"phiên không có tác dụng", res.code)
	}

	// Phiên KHÁC không bị vạ lây.
	khac := a.call(http.MethodPost, "/api/v1/events", map[string]any{
		"session_id": "ses-khac",
		"events": []any{map[string]any{
			"name": "product_view", "subject_type": "product",
			"subject_id": "prd_01J9XABC123DEF456GHJKMNPQR",
		}},
	}, nil)
	if khac.code != http.StatusOK {
		t.Errorf("phiên khác bị chặn lây: HTTP %d — giới hạn phải theo "+
			"TỪNG phiên", khac.code)
	}
}

// TestLoQuaLonBiTuChoi chặn lô khổng lồ làm nghẽn một giao dịch database.
func TestLoQuaLonBiTuChoi(t *testing.T) {
	a := newAPITest(t)

	ev := make([]any, 0, 51)
	for i := 0; i < 51; i++ {
		ev = append(ev, map[string]any{"name": "page_view"})
	}
	res := a.call(http.MethodPost, "/api/v1/events", map[string]any{
		"session_id": "ses-lo-lon", "events": ev,
	}, nil)
	if res.code != http.StatusBadRequest && res.code != http.StatusUnprocessableEntity {
		t.Errorf("lô 51 sự kiện trả HTTP %d, cần bị từ chối", res.code)
	}
}
