package payment

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// SellerKind cho biết một nhà bán có phải OWN BRAND của nền tảng không.
//
// # Vì sao payment cần biết
//
// Với đơn marketplace, doanh thu nền tảng là HOA HỒNG; phần còn lại là
// tiền phải trả nhà bán. Với đơn own brand, doanh thu nền tảng là TOÀN BỘ
// và không có ai để trả — hàng là tài sản của chính nền tảng.
//
// Không suy ra được từ hoa hồng: own brand có hoa hồng bằng 0, và nếu lấy
// hoa hồng làm doanh thu thì mọi đơn own brand ghi sổ doanh thu bằng 0
// trong khi tiền vẫn vào két. Đó là phân biệt GMV với doanh thu mà
// docs/07-workflows/marketplace-order.md mục 4 nói tới.
//
// PORT do payment định nghĩa, bên gọi cài đặt: payment ở tầng giao dịch,
// seller ở tầng nghiệp vụ bên dưới — phụ thuộc đi xuống là hợp lệ, nhưng
// đi qua port thì payment không phải import seller.
type SellerKind interface {
	IsInternal(ctx context.Context, sellerID string) (bool, error)
}

// RevenueOnCheckoutCompleted ghi bút toán doanh thu khi phiên hoàn tất.
//
// # Một bút toán cho MỖI nhà bán, không phải mỗi đơn
//
// Đơn trộn hàng của ba nhà bán sinh ra ba bút toán, mỗi cái ghi phần tiền
// của một bên — đúng như docs/07-workflows/marketplace-order.md mục 4 mô
// tả. Gộp thành một bút toán sẽ mất thông tin "phải trả ai bao nhiêu",
// và đó chính là thứ quyết toán cần.
type RevenueOnCheckoutCompleted struct {
	module *Module
	seller SellerKind
	log    *slog.Logger
}

func NewRevenueHandler(m *Module, sk SellerKind, log *slog.Logger) *RevenueOnCheckoutCompleted {
	return &RevenueOnCheckoutCompleted{module: m, seller: sk, log: log}
}

var _ eventbus.Handler = (*RevenueOnCheckoutCompleted)(nil)

func (h *RevenueOnCheckoutCompleted) Name() string {
	return "payment.revenue_on_checkout_completed"
}

func (h *RevenueOnCheckoutCompleted) EventTypes() []string {
	return []string{eventbus.TypeCheckoutCompleted}
}

// revenuePayload là phần dữ liệu bên nhận này cần.
//
// Chỉ khai những trường dùng tới: thêm trường mới vào event không được phá
// bên nhận cũ, và cách chắc chắn nhất là không đọc thứ mình không cần.
type revenuePayload struct {
	OrderID      string `json:"order_id"`
	Currency     string `json:"currency"`
	Reservations []struct {
		SellerID         string `json:"seller_id"`
		LineTotal        int64  `json:"line_total"`
		CommissionAmount int64  `json:"commission_amount"`
	} `json:"reservations"`
}

// phanCuaNhaBan là phần tiền của một nhà bán trong một đơn.
type phanCuaNhaBan struct {
	sellerID string
	tong     int64
	hoaHong  int64
}

func (h *RevenueOnCheckoutCompleted) Handle(ctx context.Context, e eventbus.Event) error {
	var p revenuePayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}
	if p.OrderID == "" || len(p.Reservations) == 0 {
		return nil
	}

	// Gom theo nhà bán, GIỮ THỨ TỰ xuất hiện: bút toán sinh ra theo thứ
	// tự xác định thì hai lần chạy lại cho cùng kết quả, và log đọc được.
	var thuTu []string
	phan := map[string]*phanCuaNhaBan{}
	for _, r := range p.Reservations {
		g, co := phan[r.SellerID]
		if !co {
			g = &phanCuaNhaBan{sellerID: r.SellerID}
			phan[r.SellerID] = g
			thuTu = append(thuTu, r.SellerID)
		}
		g.tong += r.LineTotal
		g.hoaHong += r.CommissionAmount
	}

	for _, id := range thuTu {
		g := phan[id]
		if g.tong <= 0 {
			continue
		}

		noiBo, err := h.laNoiBo(ctx, g.sellerID)
		if err != nil {
			return err
		}

		req := OrderRevenueRequest{
			OrderID:     p.OrderID,
			GrossAmount: Amount{Value: g.tong, Currency: p.Currency},
			SellerID:    g.sellerID,
			CreatedBy:   "payment.revenue_on_checkout_completed",
		}

		if noiBo {
			// Own brand: tiền thuộc về nền tảng toàn phần, không có ai để
			// trả. SellerID vẫn giữ để truy được nguồn; NewOrderRevenueEntry
			// bỏ dòng "phải trả nhà bán" khi số tiền bằng 0.
			req.PlatformRevenue = Amount{Value: g.tong, Currency: p.Currency}
		} else {
			req.PlatformRevenue = Amount{Value: g.hoaHong, Currency: p.Currency}
			req.SellerPayable = Amount{Value: g.tong - g.hoaHong, Currency: p.Currency}
		}

		if err := h.module.RecordOrderRevenueInEventTx(ctx, req); err != nil {
			return fmt.Errorf("ghi doanh thu nhà bán %s: %w", g.sellerID, err)
		}
	}

	return nil
}

// laNoiBo hỏi bên gọi xem nhà bán có phải own brand không.
//
// Thiếu port thì coi MỌI nhà bán là bên ngoài. Đó là hướng an toàn: ghi
// thiếu doanh thu nền tảng còn sửa được bằng bút toán điều chỉnh, còn ghi
// thừa tiền phải trả cho một nhà bán không tồn tại thì có thể đã chuyển
// tiền đi trước khi ai kịp nhận ra.
func (h *RevenueOnCheckoutCompleted) laNoiBo(ctx context.Context, sellerID string) (bool, error) {
	if h.seller == nil || sellerID == "" {
		return false, nil
	}
	return h.seller.IsInternal(ctx, sellerID)
}

// ChuyenSoDuKhiHetHanDoiTra nghe fulfillment_order.completed.
//
// # Vì sao chuyển số dư phải là một BÚT TOÁN
//
// Số dư là KẾT QUẢ TÍNH từ sổ cái (ADR-0008 quyết định 3), không phải một
// cột được cập nhật. Nên "chuyển trạng thái tiền" là ghi thêm một bút
// toán, không phải sửa một con số — sửa thẳng số dư là phá bỏ chính thứ
// khiến sổ đối chiếu được.
type ChuyenSoDuKhiHetHanDoiTra struct {
	module *Module
	log    *slog.Logger
}

func NewSellerReleaseHandler(m *Module, log *slog.Logger) *ChuyenSoDuKhiHetHanDoiTra {
	return &ChuyenSoDuKhiHetHanDoiTra{module: m, log: log}
}

var _ eventbus.Handler = (*ChuyenSoDuKhiHetHanDoiTra)(nil)

func (h *ChuyenSoDuKhiHetHanDoiTra) Name() string {
	return "payment.seller_release_on_fulfillment_completed"
}

func (h *ChuyenSoDuKhiHetHanDoiTra) EventTypes() []string {
	return []string{eventbus.TypeFulfillmentCompleted}
}

type sellerReleasePayload struct {
	FulfillmentID string `json:"fulfillment_id"`
	SellerID      string `json:"seller_id"`
	SellerPayable int64  `json:"seller_payable"`
	Currency      string `json:"currency"`
}

func (h *ChuyenSoDuKhiHetHanDoiTra) Handle(ctx context.Context, e eventbus.Event) error {
	var p sellerReleasePayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}

	// Đơn own brand không có nhà bán ngoài để trả tiền: tiền thuộc nền
	// tảng toàn phần và không đi đâu cả.
	if p.SellerID == "" || p.SellerPayable <= 0 {
		return nil
	}

	return h.module.ChuyenSangRutDuocInEventTx(ctx, SellerReleaseRequest{
		FulfillmentID: p.FulfillmentID,
		SellerID:      p.SellerID,
		Amount:        Amount{Value: p.SellerPayable, Currency: p.Currency},
		CreatedBy:     "payment.seller_release_on_fulfillment_completed",
	})
}
