package affiliate

import (
	"context"
	"fmt"

	"github.com/fashion-commerce/platform/internal/modules/affiliate/application"
	"github.com/fashion-commerce/platform/internal/modules/order"
)

// Adapter dưới đây nối cổng ra của tầng application với API công khai của
// module order.
//
// Nó nằm ở tầng này, KHÔNG phải trong application: tầng application chỉ biết
// interface do chính nó định nghĩa, nên nó kiểm chứng được bằng bản giả mà
// không cần dựng module order thật. Quy tắc R1 của archcheck cưỡng chế điều
// đó — chỉ file gốc module được import module khác, và chỉ qua public.go.

// OrderPort là phần API của module order mà module này dùng.
//
// Khai lại một interface HẸP thay vì nhận `order.API`: mặt công khai của
// order có hơn hai chục hàm, và nhận cả cụm nghĩa là một thay đổi ở bất kỳ
// hàm nào cũng làm affiliate không biên dịch được. Bản giả trong test cũng
// chỉ phải cài một hàm.
type OrderPort interface {
	GetOrder(ctx context.Context, orderID string) (*order.OrderView, error)
}

type orderAdapter struct{ api OrderPort }

var _ application.DonPort = (*orderAdapter)(nil)

// DongDonQuyCong đọc dòng hàng của đơn và dịch sang dữ liệu thuần.
//
// # Phép dịch quan trọng nhất ở đây: TÁCH khoản khuyến mãi
//
// `OrderLineView.LineTotal` là đơn giá × số lượng — CHƯA trừ giảm giá. Phần
// giảm nằm trong `Adjustments` như thực thể riêng, cùng chỗ với thuế và phí
// vận chuyển. Truyền thẳng `LineTotal` làm tiền gốc sẽ tính hoa hồng trên
// giá niêm yết của một đơn có giảm giá — trả creator nhiều hơn doanh thu họ
// tạo ra, và với mã giảm sâu thì ăn hết lãi.
//
// LỌC theo loại ở đây, không ở miền: `order.AdjustmentPromotion` là hằng số
// CỦA MODULE ORDER, và miền affiliate không được biết tên các loại khoản
// điều chỉnh của module khác.
func (a *orderAdapter) DongDonQuyCong(
	ctx context.Context, orderID string,
) ([]application.DongDon, error) {
	view, err := a.api.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, fmt.Errorf("affiliate: không đọc được đơn %s", orderID)
	}

	out := make([]application.DongDon, 0, len(view.Lines))
	for _, l := range view.Lines {
		d := application.DongDon{
			OrderLineID: l.ID,
			SellerID:    l.SellerID,
			CreatorID:   l.AttributedCreatorID,
			ContentID:   l.AttributedContentID,
			TienHang:    l.LineTotal.Value,
			Currency:    l.LineTotal.Currency,

			// Dòng đã hủy hoặc đã hoàn: KHÔNG quy công. `ACTIVE` là trạng
			// thái duy nhất có doanh thu — liệt kê theo hướng đóng, nên một
			// trạng thái mới của module order mặc định là không quy công
			// chứ không phải quy công nhầm.
			DaHuy: l.Status != order.LineStatusActive,
		}

		for _, adj := range l.Adjustments {
			if adj.Type != order.AdjustmentPromotion {
				continue
			}
			d.KhuyenMai = append(d.KhuyenMai, adj.Amount.Value)
		}

		out = append(out, d)
	}
	return out, nil
}
