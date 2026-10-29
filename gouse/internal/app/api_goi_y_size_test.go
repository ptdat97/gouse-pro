package app

import (
	"context"
	"testing"

	"github.com/fashion-commerce/platform/internal/modules/recommendation"
)

// TestTheSizeSuggestionComesFromReturnHistory.
//
// # Vì sao bài này tồn tại
//
// `size_recommendation` là trường đã khai trong `ProductDetail` từ lâu, và
// đặc tả gọi nó là **"cơ chế giảm trực tiếp tỷ lệ hoàn hàng"**. `dto.go`
// ghi lý do chưa điền: *"cần lịch sử mua hàng (Phase 2)"*.
//
// Từ 13/09 lý do đó không còn đúng — lý do trả hàng đã được chuẩn hóa và
// đưa vào event. Bài này đi trọn đường mà một khách thật đi:
//
//	mua size M → trả vì SIZE_TOO_SMALL → xem lại sản phẩm → gợi ý L
//
// Và nó kiểm ở tầng module chứ không gọi thẳng quy tắc: chính chỗ nối giữa
// event và read model là chỗ dễ đứt nhất.
func TestTheSizeSuggestionComesFromReturnHistory(t *testing.T) {
	a := newAPITest(t)
	ctx := context.Background()

	if a.mods.recommendation == nil {
		t.Skip("module gợi ý chưa được nối")
	}

	custID := a.customerSample(t)
	if custID == "" {
		t.Skip("không có khách nào")
	}

	// Dựng quan sát như bên nhận event sẽ dựng.
	maTH := "brd_01J9XABC123DEF456GHJKMNPQR"
	record := func(size, outcome, sourceID string) {
		t.Helper()
		if _, err := a.db.Pool().Exec(ctx, `
			INSERT INTO size_observation (customer_id, brand_id, size, outcome,
			                           source_type, source_id, observed_at)
			VALUES ($1,$2,$3,$4,'test',$5, now())`,
			custID, maTH, size, outcome, sourceID); err != nil {
			t.Fatalf("ghi quan sát: %v", err)
		}
	}

	suggestion := func() *recommendation.RecommendSizeView {
		t.Helper()
		v, err := a.mods.recommendation.RecommendSize(ctx, recommendation.RecommendSizeRequest{
			CustomerID: custID, BrandID: maTH,
			SizeScale: []string{"S", "M", "L", "XL"},
		})
		if err != nil {
			t.Fatalf("gợi ý size: %v", err)
		}
		return v
	}

	// Chưa có gì: KHÔNG gợi ý. Đoán bừa còn tệ hơn im lặng.
	if v := suggestion(); v != nil {
		t.Fatalf("gợi ý %q khi chưa có quan sát nào", v.SuggestedSize)
	}

	// Mua size M → gợi ý M, căn cứ YẾU.
	record("M", "DA_MUA", "ord-1")
	v := suggestion()
	if v == nil || v.SuggestedSize != "M" || v.Reason != "PREVIOUS_PURCHASE" {
		t.Fatalf("sau khi mua M: %+v, cần M / PREVIOUS_PURCHASE", v)
	}

	// Trả vì CHẬT → gợi ý L, và căn cứ đổi sang lịch sử trả hàng.
	record("M", "CHAT", "ret-1")
	v = suggestion()
	if v == nil || v.SuggestedSize != "L" {
		t.Fatalf("sau khi trả vì chật: %+v, cần L — phàn nàn phải thắng "+
			"lần mua, nếu không khách lại chọn đúng size vừa trả", v)
	}
	if v.Reason != "RETURN_HISTORY" {
		t.Errorf("căn cứ %q, cần RETURN_HISTORY", v.Reason)
	}

	// Thương hiệu KHÁC không được ăn theo: size M của hai thương hiệu
	// không bằng nhau.
	other, err := a.mods.recommendation.RecommendSize(ctx, recommendation.RecommendSizeRequest{
		CustomerID: custID, BrandID: "brd_01J9XKHACKHACKHACKHACKHAC",
		SizeScale: []string{"S", "M", "L"},
	})
	if err != nil {
		t.Fatalf("gợi ý thương hiệu khác: %v", err)
	}
	if other != nil {
		t.Errorf("gợi ý %q cho thương hiệu chưa có quan sát nào — size M "+
			"của hai thương hiệu không bằng nhau", other.SuggestedSize)
	}
}
