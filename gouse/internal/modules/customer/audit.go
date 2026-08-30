package customer

import (
	"context"

	"github.com/fashion-commerce/platform/internal/modules/customer/application"
	"github.com/fashion-commerce/platform/internal/platform/audit"
)

// auditRecorder nối cổng ra của tầng application với nhật ký thao tác.
type auditRecorder struct {
	rec *audit.Recorder
}

var _ application.AuditRecorder = (*auditRecorder)(nil)

// NewAuditRecorder tạo bộ ghi vết kiểm toán cho module customer.
func NewAuditRecorder(rec *audit.Recorder) application.AuditRecorder {
	return &auditRecorder{rec: rec}
}

// RecordCustomerView ghi vết việc XEM hồ sơ khách hàng.
//
// Dùng `Write` (không phải WriteTx): đây là thao tác đọc, không có giao dịch
// nghiệp vụ nào để gắn vào — cùng ngoại lệ có chủ ý với `order.view`.
//
// Vẫn cưỡng chế lý do qua `ValidateReason`: hồ sơ khách chứa tên, email và
// số điện thoại, và "đang xử lý khiếu nại đơn X" là thứ phân biệt tra cứu
// chính đáng với tò mò. Một trường lý do trống làm toàn bộ nhật ký truy cập
// trở nên vô dụng — nó ghi lại rằng có người đã xem, nhưng không trả lời
// được câu hỏi duy nhất đáng hỏi khi điều tra: xem để làm gì.
func (a *auditRecorder) RecordCustomerView(
	ctx context.Context, in application.CustomerViewRecord,
) error {
	if err := audit.ValidateReason(in.Reason); err != nil {
		return err
	}

	return a.rec.Write(ctx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      in.ActorID,
		Action:       "customer.view",
		ResourceType: audit.ResourceCustomer,
		ResourceID:   in.CustomerID.String(),
		Reason:       in.Reason,
		RequestID:    in.RequestID,
	})
}
