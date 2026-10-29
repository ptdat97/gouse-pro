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

// MaxEventVersion khai bên nhận này THEO KỊP phiên bản mới nhất của
// `checkout.completed` — con số nằm ngay dưới, không nhắc lại ở đây.
//
// Bên nhận này không đọc trường nào mà các phiên bản sau thêm vào. Nó vẫn
// phải khai vì ADR-0016 bắt MỌI bên nhận của một event nói rõ mình hiểu
// tới đâu: thiếu một khai báo là dispatcher HOÃN event cho TẤT CẢ. Đó là
// cái giá đã chấp nhận khi chọn cơ chế hoãn thay vì thả cho bên nhận đọc
// thiếu trường — nên mỗi lần nâng phiên bản, con số dưới đây phải đổi,
// còn đoạn văn này thì không.
func (h *RevenueOnCheckoutCompleted) MaxEventVersion(eventType string) int {
	if eventType == eventbus.TypeCheckoutCompleted {
		return 9
	}
	return eventbus.DefaultMaxEventVersion
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
	// ShippingFee có từ PHIÊN BẢN 3 của `checkout.completed`.
	//
	// Khoản phí khách trả cho việc giao hàng. Trước v3 nó không nằm ở đâu
	// trong sổ cái — xem domain.NewShippingRevenueEntry.
	ShippingFee int64 `json:"shipping_fee"`
	// DiscountAmount có từ PHIÊN BẢN 4 của `checkout.completed`.
	DiscountAmount int64 `json:"discount_amount"`
	// DiscountAllocations có từ PHIÊN BẢN 6: mỗi bên gánh bao nhiêu đồng.
	//
	// Thay `discount_seller_id` của v5, thứ chỉ nói được "ai chịu TRỌN" —
	// không diễn tả được chương trình CHIA ĐÔI.
	DiscountAllocations []struct {
		Bearer   string `json:"bearer"`
		SellerID string `json:"seller_id"`
		Amount   int64  `json:"amount"`
	} `json:"discount_allocations"`
}

// sellerShare là phần tiền của một nhà bán trong một đơn.
type sellerShare struct {
	sellerID   string
	tong       int64
	commission int64
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
	var sellerIDsInOrder []string
	shares := map[string]*sellerShare{}
	for _, r := range p.Reservations {
		g, co := shares[r.SellerID]
		if !co {
			g = &sellerShare{sellerID: r.SellerID}
			shares[r.SellerID] = g
			sellerIDsInOrder = append(sellerIDsInOrder, r.SellerID)
		}
		g.tong += r.LineTotal
		g.commission += r.CommissionAmount
	}

	for _, id := range sellerIDsInOrder {
		g := shares[id]
		if g.tong <= 0 {
			continue
		}

		internal, err := h.isInternal(ctx, g.sellerID)
		if err != nil {
			return err
		}

		req := OrderRevenueRequest{
			OrderID:     p.OrderID,
			GrossAmount: Amount{Value: g.tong, Currency: p.Currency},
			SellerID:    g.sellerID,
			CreatedBy:   "payment.revenue_on_checkout_completed",
		}

		if internal {
			// Own brand: tiền thuộc về nền tảng toàn phần, không có ai để
			// trả. SellerID vẫn giữ để truy được nguồn; NewOrderRevenueEntry
			// bỏ dòng "phải trả nhà bán" khi số tiền bằng 0.
			req.PlatformRevenue = Amount{Value: g.tong, Currency: p.Currency}
		} else {
			req.PlatformRevenue = Amount{Value: g.commission, Currency: p.Currency}
			req.SellerPayable = Amount{Value: g.tong - g.commission, Currency: p.Currency}
		}

		if err := h.module.RecordOrderRevenueInEventTx(ctx, req); err != nil {
			return fmt.Errorf("ghi doanh thu nhà bán %s: %w", g.sellerID, err)
		}
	}

	// PHÍ VẬN CHUYỂN — bút toán RIÊNG của cả đơn.
	//
	// Không gộp vào bút toán theo từng nhà bán ở trên: phí là khoản của
	// CẢ ĐƠN, và chia nó cho từng bên là một câu hỏi nghiệp vụ chưa có
	// câu trả lời. Miễn phí vận chuyển (PH-40) thì phí bằng 0 và không có
	// bút toán nào.
	if err := h.module.RecordShippingFeeInEventTx(
		ctx, p.OrderID, p.ShippingFee, p.Currency); err != nil {
		return fmt.Errorf("ghi phí vận chuyển: %w", err)
	}

	// KHOẢN GIẢM GIÁ — lệch theo hướng NGƯỢC với phí vận chuyển.
	//
	// Doanh thu ghi theo giá GỐC, khách trả phần đã trừ. Không ghi thì
	// khoản phải thu còn dư đúng bằng số đã giảm.
	if err := h.module.RecordDiscountInEventTx(
		ctx, p.OrderID, p.DiscountAmount, p.Currency,
		allocationsFromEvent(p)); err != nil {
		return fmt.Errorf("ghi khoản giảm giá: %w", err)
	}

	return nil
}

// isInternal hỏi bên gọi xem nhà bán có phải own brand không.
//
// Thiếu port thì coi MỌI nhà bán là bên ngoài. Đó là hướng an toàn: ghi
// thiếu doanh thu nền tảng còn sửa được bằng bút toán điều chỉnh, còn ghi
// thừa tiền phải trả cho một nhà bán không tồn tại thì có thể đã chuyển
// tiền đi trước khi ai kịp nhận ra.
func (h *RevenueOnCheckoutCompleted) isInternal(ctx context.Context, sellerID string) (bool, error) {
	if h.seller == nil || sellerID == "" {
		return false, nil
	}
	return h.seller.IsInternal(ctx, sellerID)
}

// ReleaseBalanceOnReturnWindowPassed nghe fulfillment_order.completed.
//
// # Vì sao chuyển số dư phải là một BÚT TOÁN
//
// Số dư là KẾT QUẢ TÍNH từ sổ cái (ADR-0008 quyết định 3), không phải một
// cột được cập nhật. Nên "chuyển trạng thái tiền" là ghi thêm một bút
// toán, không phải sửa một con số — sửa thẳng số dư là phá bỏ chính thứ
// khiến sổ đối chiếu được.
type ReleaseBalanceOnReturnWindowPassed struct {
	module *Module
	log    *slog.Logger
}

func NewSellerReleaseHandler(m *Module, log *slog.Logger) *ReleaseBalanceOnReturnWindowPassed {
	return &ReleaseBalanceOnReturnWindowPassed{module: m, log: log}
}

var _ eventbus.Handler = (*ReleaseBalanceOnReturnWindowPassed)(nil)

func (h *ReleaseBalanceOnReturnWindowPassed) Name() string {
	return "payment.seller_release_on_fulfillment_completed"
}

func (h *ReleaseBalanceOnReturnWindowPassed) EventTypes() []string {
	return []string{eventbus.TypeFulfillmentCompleted}
}

type sellerReleasePayload struct {
	FulfillmentID string `json:"fulfillment_id"`
	SellerID      string `json:"seller_id"`
	SellerPayable int64  `json:"seller_payable"`
	Currency      string `json:"currency"`
}

func (h *ReleaseBalanceOnReturnWindowPassed) Handle(ctx context.Context, e eventbus.Event) error {
	var p sellerReleasePayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}

	// Đơn own brand không có nhà bán ngoài để trả tiền: tiền thuộc nền
	// tảng toàn phần và không đi đâu cả.
	if p.SellerID == "" || p.SellerPayable <= 0 {
		return nil
	}

	return h.module.ReleaseToAvailableInEventTx(ctx, SellerReleaseRequest{
		FulfillmentID: p.FulfillmentID,
		SellerID:      p.SellerID,
		Amount:        Amount{Value: p.SellerPayable, Currency: p.Currency},
		CreatedBy:     "payment.seller_release_on_fulfillment_completed",
	})
}

// ---------------------------------------------------- Thu được tiền

// RecordPaymentOnOrderPaid ghi bút toán chuyển KHOẢN PHẢI THU thành TIỀN MẶT.
//
// # Vì sao cần bên nhận này
//
// Bút toán doanh thu ghi lúc `checkout.completed` — lúc khách bấm xong
// phiên thanh toán, KHÔNG phải lúc tiền về. Từ ADR-0018 phần B1 nó ghi NỢ
// `ACCOUNTS_RECEIVABLE` thay vì `PLATFORM_CASH`, và đây là bên nhận đóng
// vòng: khi `order.paid` tới thì khoản phải thu đó thành tiền mặt.
//
// Không có nó, mọi đơn nằm mãi ở dạng "khách nợ" và số dư tiền mặt của
// nền tảng luôn bằng 0 — sai theo hướng ngược lại với lỗi cũ, nhưng vẫn
// là sai.
type RecordPaymentOnOrderPaid struct {
	module *Module
	log    *slog.Logger
}

func NewPaymentHandler(m *Module, log *slog.Logger) *RecordPaymentOnOrderPaid {
	return &RecordPaymentOnOrderPaid{module: m, log: log}
}

var _ eventbus.Handler = (*RecordPaymentOnOrderPaid)(nil)

func (h *RecordPaymentOnOrderPaid) Name() string {
	return "payment.record_payment_on_order_paid"
}

func (h *RecordPaymentOnOrderPaid) EventTypes() []string {
	return []string{eventbus.TypeOrderPaid}
}

type paymentPayload struct {
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// Handle ghi bút toán trong GIAO DỊCH của dispatcher.
//
// Số tiền lấy từ payload chứ không tính lại: nó là con số `payment_intent`
// đã đối chiếu tuyệt đối với nhà cung cấp (ADR-0017 phần 4). Tính lại ở
// đây là mở đường cho hai con số khác nhau cùng nói về một lần thu tiền.
func (h *RecordPaymentOnOrderPaid) Handle(ctx context.Context, e eventbus.Event) error {
	var p paymentPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}
	if p.Amount <= 0 {
		// Đơn 0 đồng không có gì để thu. Không phải lỗi — trả nil để event
		// không kẹt trong hàng đợi.
		h.log.WarnContext(ctx, "order.paid với số tiền không dương",
			"order_id", p.OrderID, "so_tien", p.Amount)
		return nil
	}
	return h.module.RecordPaymentInEventTx(ctx, p.OrderID, p.Amount, p.Currency)
}

// allocationsFromEvent đổi bảng phân bổ trong payload sang đầu vào của module.
func allocationsFromEvent(p revenuePayload) []AllocateCostInput {
	out := make([]AllocateCostInput, 0, len(p.DiscountAllocations))
	for _, d := range p.DiscountAllocations {
		out = append(out, AllocateCostInput{
			Bearer: d.Bearer, SellerID: d.SellerID, Amount: d.Amount,
		})
	}
	return out
}

// ---------------------------------------------------- Chi phí vận chuyển

// RecordCarrierCostOnHandover ghi nghĩa vụ trả hãng khi kiện hàng được bàn giao.
//
// # Vì sao bên nhận này tồn tại
//
// Từ ADR-0018, phí vận chuyển khách trả được ghi là doanh thu nền tảng.
// Nhưng khoản nền tảng TRẢ hãng chưa có bút toán nào, nên doanh thu bị
// thổi lên đúng bằng chi phí chưa ghi, và lãi/lỗ mảng vận chuyển — con số
// duy nhất trả lời "thu phí ship như vậy là lãi hay lỗ" — không tồn tại.
//
// # Giá lấy từ CẤU HÌNH VẬN HÀNH, và có thể CHƯA KHAI
//
// Hệ thống không biết giá thỏa thuận với hãng: `bieuPhi` của fulfillment
// là phí KHÁCH TRẢ, không phải giá nền tảng trả hãng. Con số đó là dữ liệu
// kinh doanh và phải do người vận hành nhập
// (`fulfillment.carrier_cost_standard`).
//
// Chưa khai thì KHÔNG ghi bút toán nào — và `cmd/doisoatso` báo số kiện
// đang thiếu. Đặt một con số mặc định sẽ làm lãi vận chuyển trông như một
// sự thật đã đo, trong khi không ai đo cả.
type RecordCarrierCostOnHandover struct {
	module *Module
	price  CarrierPricePort
	log    *slog.Logger
}

// CarrierPricePort tra giá nền tảng trả hãng cho MỘT kiện.
//
// Trả 0 nghĩa là CHƯA KHAI — bên gọi không ghi bút toán.
type CarrierPricePort interface {
	PricePerParcel(payMethod string) int64
}

func NewCarrierCostHandler(
	m *Module, price CarrierPricePort, log *slog.Logger,
) *RecordCarrierCostOnHandover {
	return &RecordCarrierCostOnHandover{module: m, price: price, log: log}
}

var _ eventbus.Handler = (*RecordCarrierCostOnHandover)(nil)

func (h *RecordCarrierCostOnHandover) Name() string {
	return "payment.record_carrier_cost_on_handover"
}

func (h *RecordCarrierCostOnHandover) EventTypes() []string {
	return []string{eventbus.TypeFulfillmentProgress}
}

// MaxEventVersion: cần phiên bản có `shipping_method`.
func (h *RecordCarrierCostOnHandover) MaxEventVersion(eventType string) int {
	if eventType == eventbus.TypeFulfillmentProgress {
		return 2
	}
	return eventbus.DefaultMaxEventVersion
}

type carrierCostPayload struct {
	FulfillmentID  string `json:"fulfillment_id"`
	NewStatus      string `json:"new_status"`
	ShippingMethod string `json:"shipping_method"`
}

// Handle ghi bút toán chi phí khi trạng thái chuyển sang ĐÃ BÀN GIAO.
//
// CHỈ ở bước bàn giao: nghĩa vụ với hãng phát sinh khi hàng rời kho, và
// các bước sau (đang giao, đã giao) không tạo thêm nghĩa vụ nào. Ghi ở mọi
// bước sẽ tính chi phí nhiều lần cho một kiện hàng.
func (h *RecordCarrierCostOnHandover) Handle(ctx context.Context, e eventbus.Event) error {
	var p carrierCostPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}
	if p.NewStatus != "HANDED_OVER" {
		return nil
	}

	price := h.price.PricePerParcel(p.ShippingMethod)
	if price <= 0 {
		// CHƯA KHAI giá hãng. Không ghi, và không coi là lỗi — nhưng cũng
		// KHÔNG im lặng: `cmd/doisoatso` đếm số kiện thiếu bút toán này.
		return nil
	}

	return h.module.RecordShippingCostInEventTx(ctx, p.FulfillmentID, price, "VND")
}
