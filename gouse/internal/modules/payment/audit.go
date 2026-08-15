package payment

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	paymentpg "github.com/fashion-commerce/platform/internal/modules/payment/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/audit"
)

// auditRecorder nối cổng ra của tầng application với nhật ký thao tác.
type auditRecorder struct {
	rec *audit.Recorder
}

var _ application.AuditRecorder = (*auditRecorder)(nil)

// NewAuditRecorder tạo bộ ghi vết kiểm toán cho module payment.
func NewAuditRecorder(rec *audit.Recorder) application.AuditRecorder {
	return &auditRecorder{rec: rec}
}

// RecordLedgerAdjustment ghi vết điều chỉnh sổ cái BẰNG giao dịch của kho
// lưu trữ.
//
// Ngữ cảnh phải mang giao dịch mà `AppendWithAudit` đã mở. Thiếu nó thì trả
// lỗi chứ KHÔNG âm thầm ghi rời.
//
// Với sổ cái, hậu quả của việc ghi rời nặng hơn mọi module khác: bút toán
// đã vào sổ mà vết kiểm toán hỏng nghĩa là có một khoản tiền xuất hiện
// trong sổ sách mà không ai truy được ai đã tạo ra nó.
func (a *auditRecorder) RecordLedgerAdjustment(
	ctx context.Context, in application.AdjustmentRecord,
) error {
	tx, ok := paymentpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"payment: ghi vết kiểm toán ngoài giao dịch của kho lưu trữ — " +
				"bút toán điều chỉnh và vết kiểm toán phải cùng thành công " +
				"hoặc cùng thất bại")
	}

	// WriteSensitive kiểm tra lý do: tối thiểu 20 ký tự, chặn giá trị rác.
	return a.rec.WriteSensitive(ctx, tx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      in.ActorID,
		Action:       "ledger.adjust",
		ResourceType: audit.ResourceLedger,
		ResourceID:   in.EntryID.String(),
		Reason:       in.Reason,
		RequestID:    in.RequestID,
		Metadata: map[string]any{
			"reference_type": in.ReferenceType,
			"reference_id":   in.ReferenceID.String(),
			"total_amount":   in.TotalAmount,
			"currency":       in.Currency,
		},
	})
}
