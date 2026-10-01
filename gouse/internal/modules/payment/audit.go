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

// RecordSettlementSigned ghi vết việc KÝ một bước của đợt đối soát.
//
// Cùng ràng buộc giao dịch với `RecordLedgerAdjustment`, và cùng lý do: một
// đợt chuyển sang `PAID` mà vết kiểm toán hỏng nghĩa là có một khoản tiền
// RỜI nền tảng mà không ai truy được ai đã ký.
//
// `Action` phân biệt hai bước (`settlement.confirm` / `settlement.mark_paid`)
// chứ không gộp thành một: báo cáo kiểm toán cần tách "ai chốt con số" khỏi
// "ai quyết định chuyển tiền".
func (a *auditRecorder) RecordSettlementSigned(
	ctx context.Context, in application.SettlementSignRecord,
) error {
	tx, ok := paymentpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"payment: ghi vết ký đợt đối soát ngoài giao dịch của kho lưu " +
				"trữ — trạng thái đợt và vết kiểm toán phải cùng thành công " +
				"hoặc cùng thất bại")
	}

	// WriteSensitive kiểm lý do: tối thiểu 20 ký tự, chặn giá trị rác.
	return a.rec.WriteSensitive(ctx, tx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      in.ActorID,
		Action:       "settlement." + in.Buoc,
		ResourceType: audit.ResourceSettlement,
		ResourceID:   in.SettlementID.String(),
		Reason:       in.Reason,
		RequestID:    in.RequestID,
		Metadata: map[string]any{
			// KHÔNG đặt tên nhà bán hay số tài khoản vào đây: audit log giữ
			// nhiều năm, và dữ liệu cá nhân lọt vào metadata sẽ sống lâu
			// hơn mọi chính sách xóa dữ liệu.
			"seller_id":  in.SellerID.String(),
			"net_amount": in.NetAmount,
			"currency":   in.Currency,
		},
	})
}
