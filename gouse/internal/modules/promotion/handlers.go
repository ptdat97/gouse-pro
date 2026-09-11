package promotion

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// Bên nhận event của module promotion.
//
// # Vì sao chúng xuất hiện muộn
//
// `RecordUsage` và `ReleaseUsage` đã có đủ ba tầng từ lâu — kèm cả phần
// khó nhất: ghi lượt trước rồi cộng dồn NGUYÊN TỬ, idempotent theo đơn,
// và chú thích giải thích vì sao thứ tự đó quan trọng. Nhưng KHÔNG ai gọi
// chúng từ ngoài module.
//
// Hệ quả: mọi giới hạn của mã giảm giá đều vô hiệu.
//
//	CountByCustomer  luôn trả 0   → "mỗi khách N lượt" vô nghĩa
//	used_count       không tăng   → `max_uses` không bao giờ chạm
//	used_budget      không tăng   → ngân sách khuyến mãi không bao giờ cạn
//
// Đo trên dữ liệu thật ngày 11/09: 1 đơn dùng mã, 0 lượt được ghi, bộ đếm
// của chương trình vẫn bằng 0.

// GhiLuotDungKhiHoanTat ghi nhận một lượt dùng mã khi phiên thanh toán xong.
type GhiLuotDungKhiHoanTat struct {
	module *Module
	log    *slog.Logger
}

func NewGhiLuotDungHandler(m *Module, log *slog.Logger) *GhiLuotDungKhiHoanTat {
	return &GhiLuotDungKhiHoanTat{module: m, log: log}
}

var _ eventbus.Handler = (*GhiLuotDungKhiHoanTat)(nil)

func (h *GhiLuotDungKhiHoanTat) Name() string {
	return "promotion.ghi_luot_dung_khi_hoan_tat"
}

func (h *GhiLuotDungKhiHoanTat) EventTypes() []string {
	return []string{eventbus.TypeCheckoutCompleted}
}

// MaxEventVersion: cần phiên bản 7 để có `coupon_code`.
func (h *GhiLuotDungKhiHoanTat) MaxEventVersion(eventType string) int {
	if eventType == eventbus.TypeCheckoutCompleted {
		return 8
	}
	return eventbus.DefaultMaxEventVersion
}

type luotDungPayload struct {
	OrderID        string `json:"order_id"`
	CustomerID     string `json:"customer_id"`
	CouponCode     string `json:"coupon_code"`
	DiscountAmount int64  `json:"discount_amount"`
	Currency       string `json:"currency"`
}

// Handle ghi lượt dùng. Đơn KHÔNG dùng mã thì bỏ qua.
//
// IDEMPOTENT theo đơn: `RecordUsage` trả nil khi lượt đã ghi, nên event
// phát lại không đếm hai lần.
func (h *GhiLuotDungKhiHoanTat) Handle(ctx context.Context, e eventbus.Event) error {
	var p luotDungPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}

	code := strings.TrimSpace(p.CouponCode)
	if code == "" {
		// Đường đi phổ biến nhất: đơn không dùng mã.
		return nil
	}

	if err := h.module.RecordUsage(ctx, RecordUsageRequest{
		Code:       code,
		CustomerID: p.CustomerID,
		OrderID:    p.OrderID,
		Discount:   p.DiscountAmount,
		Currency:   p.Currency,
	}); err != nil {
		return fmt.Errorf("ghi nhận lượt dùng mã %q: %w", code, err)
	}
	return nil
}

// GiaiPhongLuotKhiHuyDon trả lại lượt dùng khi đơn bị hủy.
//
// # Vì sao cần
//
// Không trả lại thì khách dùng mã, hủy đơn, và MẤT lượt — với mã giới hạn
// một lượt mỗi khách thì họ mất luôn quyền dùng. Và ngân sách khuyến mãi
// bị trừ cho một đơn không còn tồn tại, nên chương trình cạn sớm hơn thực
// tế.
type GiaiPhongLuotKhiHuyDon struct {
	module *Module
	log    *slog.Logger
}

func NewGiaiPhongLuotHandler(m *Module, log *slog.Logger) *GiaiPhongLuotKhiHuyDon {
	return &GiaiPhongLuotKhiHuyDon{module: m, log: log}
}

var _ eventbus.Handler = (*GiaiPhongLuotKhiHuyDon)(nil)

func (h *GiaiPhongLuotKhiHuyDon) Name() string {
	return "promotion.giai_phong_luot_khi_huy_don"
}

func (h *GiaiPhongLuotKhiHuyDon) EventTypes() []string {
	return []string{eventbus.TypeOrderCancelled}
}

type huyDonLuotPayload struct {
	OrderID string `json:"order_id"`
}

// Handle giải phóng lượt của đơn bị hủy.
//
// IDEMPOTENT: đơn không có lượt nào thì trả 0 và không phải lỗi.
func (h *GiaiPhongLuotKhiHuyDon) Handle(ctx context.Context, e eventbus.Event) error {
	var p huyDonLuotPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event: %w", err)
	}

	n, err := h.module.ReleaseUsage(ctx, p.OrderID)
	if err != nil {
		return fmt.Errorf("giải phóng lượt dùng mã: %w", err)
	}
	if n > 0 {
		h.log.InfoContext(ctx, "đã giải phóng lượt dùng mã của đơn bị hủy",
			"order_id", p.OrderID, "so_luot", n)
	}
	return nil
}
