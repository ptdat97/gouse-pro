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

// GhiQuanSatSize dựng read model size từ hành vi thật của khách.
//
// # Hai nguồn, hai mức tin cậy
//
//	checkout.completed   khách MUA size X       → tín hiệu YẾU
//	returns.requested    khách TRẢ vì size sai  → tín hiệu MẠNH
//
// Khách giữ hàng có thể vì vừa, cũng có thể vì ngại trả. Khách trả hàng vì
// size thì chủ động bỏ công nói ra rằng size đó sai, và sai theo hướng nào.
// Quy tắc suy luận ưu tiên nguồn thứ hai — xem `domain.SuyLuanSize`.
type GhiQuanSatSize struct {
	module *Module
	log    *slog.Logger
}

func NewQuanSatSizeHandler(m *Module, log *slog.Logger) *GhiQuanSatSize {
	return &GhiQuanSatSize{module: m, log: log}
}

var _ eventbus.Handler = (*GhiQuanSatSize)(nil)

func (h *GhiQuanSatSize) Name() string { return "recommendation.ghi_quan_sat_size" }

func (h *GhiQuanSatSize) EventTypes() []string {
	return []string{
		eventbus.TypeCheckoutCompleted,
		eventbus.TypeReturnRequested,
	}
}

// MaxEventVersion: bên nhận này KHÔNG đọc trường nào mà các phiên bản sau
// thêm vào, nhưng ADR-0016 bắt MỌI bên nhận của một event nói rõ mình hiểu
// tới đâu — thiếu một khai báo là dispatcher HOÃN event cho TẤT CẢ.
func (h *GhiQuanSatSize) MaxEventVersion(eventType string) int {
	if eventType == eventbus.TypeCheckoutCompleted {
		return 9
	}
	return eventbus.DefaultMaxEventVersion
}

type muaPayload struct {
	OrderID      string `json:"order_id"`
	CustomerID   string `json:"customer_id"`
	Reservations []struct {
		SKUID string `json:"sku_id"`
	} `json:"reservations"`
}

type traPayload struct {
	ReturnID   string `json:"return_id"`
	CustomerID string `json:"customer_id"`
	Lines      []struct {
		SKUID string `json:"sku_id"`
		LyDo  string `json:"reason_code"`
	} `json:"lines"`
}

func (h *GhiQuanSatSize) Handle(ctx context.Context, e eventbus.Event) error {
	switch e.Type {
	case eventbus.TypeCheckoutCompleted:
		return h.handleMua(ctx, e)
	case eventbus.TypeReturnRequested:
		return h.handleTra(ctx, e)
	}
	return nil
}

// handleMua ghi quan sát ĐÃ MUA cho từng dòng hàng.
//
// Khách VÃNG LAI bị bỏ qua: không có mã khách thì không gắn quan sát vào
// đâu được, và gợi ý size vốn chỉ phục vụ người đã đăng nhập.
func (h *GhiQuanSatSize) handleMua(ctx context.Context, e eventbus.Event) error {
	var p muaPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event đặt hàng: %w", err)
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		return nil
	}

	for _, r := range p.Reservations {
		if err := h.module.ghiQuanSat(ctx, domain.QuanSatMoi{
			CustomerID: ids.ID(p.CustomerID),
			KetQua:     domain.KetQuaDaMua,
			NguonLoai:  "order",
			NguonID:    ids.ID(p.OrderID),
			QuanSatLuc: e.OccurredAt,
		}, r.SKUID); err != nil {
			return err
		}
	}
	return nil
}

// handleTra ghi quan sát CHẬT hoặc RỘNG — chỉ với lý do về size.
//
// Trả hàng vì "khác mô tả" hay "hàng lỗi" KHÔNG nói gì về size, và ghi
// chúng thành quan sát size sẽ làm gợi ý dịch size vì một lý do chẳng liên
// quan.
func (h *GhiQuanSatSize) handleTra(ctx context.Context, e eventbus.Event) error {
	var p traPayload
	if err := e.Unmarshal(&p); err != nil {
		return fmt.Errorf("đọc dữ liệu event trả hàng: %w", err)
	}
	if strings.TrimSpace(p.CustomerID) == "" {
		return nil
	}

	for _, d := range p.Lines {
		var k domain.KetQua
		switch d.LyDo {
		case "SIZE_TOO_SMALL":
			k = domain.KetQuaChat
		case "SIZE_TOO_LARGE":
			k = domain.KetQuaRong
		default:
			continue
		}

		if err := h.module.ghiQuanSat(ctx, domain.QuanSatMoi{
			CustomerID: ids.ID(p.CustomerID),
			KetQua:     k,
			NguonLoai:  "return",
			NguonID:    ids.ID(p.ReturnID),
			QuanSatLuc: e.OccurredAt,
		}, d.SKUID); err != nil {
			return err
		}
	}
	return nil
}
