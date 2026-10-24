package recommendation

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/recommendation/domain"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// RecordSizeObservation dựng read model size từ hành vi thật của khách.
//
// # Hai nguồn, hai mức tin cậy
//
//	checkout.completed   khách MUA size X       → tín hiệu YẾU
//	returns.requested    khách TRẢ vì size sai  → tín hiệu MẠNH
//
// Khách giữ hàng có thể vì vừa, cũng có thể vì ngại trả. Khách trả hàng vì
// size thì chủ động bỏ công nói ra rằng size đó sai, và sai theo hướng nào.
// Quy tắc suy luận ưu tiên nguồn thứ hai — xem `domain.SuyLuanSize`.
type RecordSizeObservation struct {
	module *Module
	log    *slog.Logger
}

func NewSizeObservationHandler(m *Module, log *slog.Logger) *RecordSizeObservation {
	return &RecordSizeObservation{module: m, log: log}
}

var _ eventbus.Handler = (*RecordSizeObservation)(nil)

func (h *RecordSizeObservation) Name() string { return "recommendation.ghi_quan_sat_size" }

func (h *RecordSizeObservation) EventTypes() []string {
	return []string{
		eventbus.TypeCheckoutCompleted,
		eventbus.TypeReturnRequested,
	}
}

// MaxEventVersion: bên nhận này KHÔNG đọc trường nào mà các phiên bản sau
// thêm vào, nhưng ADR-0016 bắt MỌI bên nhận của một event nói rõ mình hiểu
// tới đâu — thiếu một khai báo là dispatcher HOÃN event cho TẤT CẢ.
func (h *RecordSizeObservation) MaxEventVersion(eventType string) int {
	if eventType == eventbus.TypeCheckoutCompleted {
		return 9
	}
	return eventbus.DefaultMaxEventVersion
}

type purchasePayload struct {
	OrderID      string `json:"order_id"`
	CustomerID   string `json:"customer_id"`
	Reservations []struct {
		SKUID string `json:"sku_id"`
	} `json:"reservations"`
}

type returnPayload struct {
	ReturnID   string `json:"return_id"`
	CustomerID string `json:"customer_id"`
	Lines      []struct {
		SKUID string `json:"sku_id"`
		LyDo  string `json:"reason_code"`
	} `json:"lines"`
}

func (h *RecordSizeObservation) Handle(ctx context.Context, e eventbus.Event) error {
	switch e.Type {
	case eventbus.TypeCheckoutCompleted:
		return h.handlePurchase(ctx, e)
	case eventbus.TypeReturnRequested:
		return h.handleReturn(ctx, e)
	}
	return nil
}

// handlePurchase ghi quan sát ĐÃ MUA cho từng dòng hàng.
//
// Khách VÃNG LAI bị bỏ qua: không có mã khách thì không gắn quan sát vào
// đâu được, và gợi ý size vốn chỉ phục vụ người đã đăng nhập.
func (h *RecordSizeObservation) handlePurchase(ctx context.Context, e eventbus.Event) error {
	var p purchasePayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event đặt hàng: %w", err)
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		return nil
	}

	for _, r := range p.Reservations {
		if err := h.module.recordObservation(ctx, domain.NewObservation{
			CustomerID: ids.ID(p.CustomerID),
			Outcome:    domain.OutcomePurchased,
			SourceType: "order",
			SourceID:   ids.ID(p.OrderID),
			ObservedAt: e.OccurredAt,
		}, r.SKUID); err != nil {
			return err
		}
	}
	return nil
}

// handleReturn ghi quan sát CHẬT hoặc RỘNG — chỉ với lý do về size.
//
// Trả hàng vì "khác mô tả" hay "hàng lỗi" KHÔNG nói gì về size, và ghi
// chúng thành quan sát size sẽ làm gợi ý dịch size vì một lý do chẳng liên
// quan.
func (h *RecordSizeObservation) handleReturn(ctx context.Context, e eventbus.Event) error {
	var p returnPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event trả hàng: %w", err)
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		return nil
	}

	for _, d := range p.Lines {
		var k domain.Outcome
		switch d.LyDo {
		case "SIZE_TOO_SMALL":
			k = domain.OutcomeTight
		case "SIZE_TOO_LARGE":
			k = domain.OutcomeLoose
		default:
			continue
		}

		if err := h.module.recordObservation(ctx, domain.NewObservation{
			CustomerID: ids.ID(p.CustomerID),
			Outcome:    k,
			SourceType: "return",
			SourceID:   ids.ID(p.ReturnID),
			ObservedAt: e.OccurredAt,
		}, d.SKUID); err != nil {
			return err
		}
	}
	return nil
}
