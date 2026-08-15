package order

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/modules/order/application"
	orderpg "github.com/fashion-commerce/platform/internal/modules/order/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/audit"
)

// auditRecorder nối cổng ra của tầng application với nhật ký thao tác.
type auditRecorder struct {
	rec *audit.Recorder
}

var _ application.AuditRecorder = (*auditRecorder)(nil)

// NewAuditRecorder tạo bộ ghi vết kiểm toán cho module order.
func NewAuditRecorder(rec *audit.Recorder) application.AuditRecorder {
	return &auditRecorder{rec: rec}
}

// RecordOrderView ghi vết việc XEM chi tiết đơn.
//
// Dùng `Write` (không phải WriteTx): đây là thao tác đọc, không có giao dịch
// nghiệp vụ nào để gắn vào. Đó là ngoại lệ có chủ ý so với các use case ghi.
//
// Vẫn dùng WriteSensitive để cưỡng chế lý do: "đang xử lý khiếu nại đơn X"
// phân biệt tra cứu chính đáng với tò mò, và một trường lý do trống làm
// toàn bộ nhật ký truy cập trở nên vô dụng.
func (a *auditRecorder) RecordOrderView(
	ctx context.Context, in application.OrderViewRecord,
) error {
	if err := audit.ValidateReason(in.Reason); err != nil {
		return err
	}

	return a.rec.Write(ctx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      in.ActorID,
		Action:       "order.view",
		ResourceType: audit.ResourceOrder,
		ResourceID:   in.OrderID.String(),
		Reason:       in.Reason,
		RequestID:    in.RequestID,
	})
}

// RecordOrderCancellation ghi vết hủy đơn BẰNG giao dịch của kho lưu trữ.
//
// Ngữ cảnh phải mang giao dịch mà `UpdateWithAudit` đã mở. Thiếu nó thì trả
// lỗi: đơn của khách đã trả tiền bị hủy mà không có vết là thứ không giải
// thích được khi khách khiếu nại.
func (a *auditRecorder) RecordOrderCancellation(
	ctx context.Context, in application.OrderCancelRecord,
) error {
	tx, ok := orderpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"order: ghi vết kiểm toán ngoài giao dịch của kho lưu trữ — " +
				"hủy đơn và vết kiểm toán phải cùng thành công hoặc cùng " +
				"thất bại")
	}

	return a.rec.WriteSensitive(ctx, tx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      in.ActorID,
		Action:       "order.cancel",
		ResourceType: audit.ResourceOrder,
		ResourceID:   in.OrderID.String(),
		Reason:       in.Reason,
		RequestID:    in.RequestID,
		Metadata: map[string]any{
			"order_number": in.OrderNumber,
		},
	})
}
