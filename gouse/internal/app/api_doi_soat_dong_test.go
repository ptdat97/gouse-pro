package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// Một đợt đối soát phải nói được nó GỒM NHỮNG GÌ.
//
// # Vì sao bài này tồn tại
//
// Đặc tả `getMySettlement` viết: *"Seller phải xem được **từng dòng** cấu
// thành số tiền — đối soát không minh bạch là nguyên nhân tranh chấp lớn
// nhất giữa nền tảng và nhà bán."*
//
// `settlement_line` có từ migration 000041 và `doc()` nạp nó vào
// `Settlement.Lines()` mỗi lần đọc. Tới 23/09/2026 tầng DTO VỨT nó đi, nên API
// chỉ trả ba con số tổng. Lần thứ tư của cùng dạng lỗi trong khu này —
// `shipping_groups` (P3-50), `variants` (P3-64), trường sửa được của sản
// phẩm (P3-65).
//
// # Vì sao đi qua HTTP
//
// Lỗi nằm ĐÚNG ở tầng DTO. Gọi thẳng service thì `Dong()` luôn có dữ liệu
// và bài test xanh trong lúc API vẫn trả thiếu.
func TestSettlementDetailExplainsWhatTheDebtCovers(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	B, available := a.buildSellerWithAvailableFunds(t)
	if available <= 0 {
		t.Fatalf("không dựng được số dư rút được (%v)", available)
	}

	until := types.Now().Add(time.Minute)
	if _, err := a.mods.payment.NewSettlementForPeriod(
		ctx, until.Add(-7*24*time.Hour), until, 1000); err != nil {
		t.Fatalf("tạo đợt đối soát: %v", err)
	}

	ds := a.call(http.MethodGet, "/api/v1/seller/settlements", nil, bearer(B.token))
	if ds.code != http.StatusOK {
		t.Fatalf("danh sách đợt: HTTP %d — %s", ds.code, ds.raw)
	}
	listed, _ := ds.body["data"].([]any)
	if len(listed) == 0 {
		t.Fatal("không có đợt nào")
	}
	first, _ := listed[0].(map[string]any)
	id, _ := first["id"].(string)

	res := a.call(http.MethodGet, "/api/v1/seller/settlements/"+id, nil, bearer(B.token))
	if res.code != http.StatusOK {
		t.Fatalf("chi tiết đợt: HTTP %d — %s", res.code, res.raw)
	}
	d, _ := res.body["settlement"].(map[string]any)
	if d == nil {
		t.Fatalf("response không có `settlement`: %s", res.raw)
	}

	line, hasField := d["lines"].([]any)
	if !hasField {
		t.Fatalf("chi tiết đợt KHÔNG có `lines` — nhà bán chỉ thấy một con "+
			"số tổng và không đối chiếu được với sổ của họ: %s", res.raw)
	}
	if len(line) == 0 {
		t.Fatal("`lines` rỗng dù đợt có tiền")
	}

	var sumTotal float64
	for i, x := range line {
		l, _ := x.(map[string]any)
		am, _ := l["amount"].(map[string]any)
		v, _ := am["amount"].(float64)
		sumTotal += v

		// `reference_id` là thứ làm dòng này ĐỐI CHIẾU ĐƯỢC. Thiếu nó thì
		// dòng chỉ là một số tiền không gắn với đơn nào.
		if ref, _ := l["reference_id"].(string); ref == "" {
			t.Errorf("dòng %d không nói đến từ đơn nào", i)
		}
		if rt, _ := l["reference_type"].(string); rt != "FULFILLMENT_ORDER" {
			t.Errorf("dòng %d: reference_type = %q, mong FULFILLMENT_ORDER", i, rt)
		}
		if collected, _ := l["released_at"].(string); collected == "" {
			t.Errorf("dòng %d không nói chuyển sang rút được lúc nào", i)
		}
	}

	// BẤT BIẾN: tổng các dòng = `gross_amount`.
	//
	// Lệch nghĩa là màn hình cộng ra một số khác con số nền tảng nói — và
	// đó đúng là tình huống đẩy một nhà bán đi khiếu nại.
	gross, _ := d["gross_amount"].(map[string]any)
	g, _ := gross["amount"].(float64)
	if sumTotal != g {
		t.Errorf("tổng các dòng = %v, `gross_amount` = %v — nhà bán cộng "+
			"tay sẽ ra số khác số nền tảng nói", sumTotal, g)
	}
}
