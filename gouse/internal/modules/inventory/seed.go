package inventory

import (
	"context"
	"fmt"
)

// SeedInput là các SKU cần nhập kho.
//
// SKU phải CÓ THẬT từ module product — bịa định danh thì hàng tồn tại trong
// bảng nhưng không sản phẩm nào trỏ tới, và không ai mua được.
type SeedInput struct {
	SKUIDs []string

	// Quantity là số lượng nhập cho mỗi SKU.
	Quantity int
}

// SeedResult là kết quả nạp dữ liệu mẫu.
type SeedResult struct {
	// LocationID là kho vừa dùng — cần để gọi thử các endpoint tồn kho.
	LocationID string

	// StockedSKUIDs là các SKU đã có hàng.
	StockedSKUIDs []string
}

// demoLocationCode là mã kho mẫu.
//
// Cố định chứ không sinh ngẫu nhiên: nạp lại lần hai phải dùng ĐÚNG kho cũ
// (EnsureByCode idempotent theo mã), nếu không mỗi lần khởi động lại thêm
// một kho và hàng nằm rải rác không gom được.
const demoLocationCode = "HCM-01"

// SeedDemo nhập hàng mẫu vào kho nền tảng.
//
// CHỈ dùng cho môi trường phát triển.
//
// # Vì sao seed này cần tồn tại
//
// Không có tồn kho thì `startCheckout` luôn thất bại với "không đủ hàng" —
// giữ hàng là điều kiện bắt buộc để mở phiên thanh toán. Nghĩa là toàn bộ
// đường mua hàng không chạy thử được, dù mọi module đều đã sẵn sàng.
func SeedDemo(ctx context.Context, m *Module, in SeedInput) (SeedResult, error) {
	var out SeedResult
	if m == nil || len(in.SKUIDs) == 0 {
		return out, nil
	}

	qty := in.Quantity
	if qty <= 0 {
		qty = 100
	}

	locationID, err := m.EnsureLocation(ctx, "Kho TP.HCM", demoLocationCode, "PLATFORM")
	if err != nil {
		return out, fmt.Errorf("tạo kho mẫu: %w", err)
	}
	out.LocationID = locationID

	for _, skuID := range in.SKUIDs {
		if _, err := m.Receive(ctx, ReceiveRequest{
			SKUID:      skuID,
			LocationID: locationID,
			Quantity:   qty,
			// Nhập hàng phải ghi ai làm và theo chứng từ nào (quy tắc 7).
			// Dữ liệu mẫu không có chứng từ thật, nên ghi rõ đây là seed —
			// tốt hơn một mã bịa trông như thật.
			ReferenceID: "seed-demo",
			PerformedBy: "seed-demo",
		}); err != nil {
			return out, fmt.Errorf("nhập kho cho %s: %w", skuID, err)
		}
		out.StockedSKUIDs = append(out.StockedSKUIDs, skuID)
	}

	return out, nil
}
