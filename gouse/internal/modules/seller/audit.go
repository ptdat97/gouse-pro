package seller

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/modules/seller/application"
	sellerpg "github.com/fashion-commerce/platform/internal/modules/seller/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/audit"
)

// auditRecorder nối cổng ra của tầng application với nhật ký thao tác.
type auditRecorder struct {
	rec *audit.Recorder
}

var _ application.AuditRecorder = (*auditRecorder)(nil)

// NewAuditRecorder tạo bộ ghi vết kiểm toán cho module seller.
func NewAuditRecorder(rec *audit.Recorder) application.AuditRecorder {
	return &auditRecorder{rec: rec}
}

// RecordSuspension ghi vết đình chỉ BẰNG giao dịch của kho lưu trữ.
//
// Ngữ cảnh phải mang giao dịch mà `SaveWithAudit` đã mở. Thiếu nó thì trả
// lỗi chứ KHÔNG âm thầm ghi rời: ghi rời nghĩa là hai kết cục hỏng đều có
// thể xảy ra — seller bị đình chỉ mà không có vết, hoặc có vết cho việc
// chưa từng xảy ra. Cả hai đều phá giá trị của nhật ký.
func (a *auditRecorder) RecordSuspension(
	ctx context.Context, in application.SuspensionRecord,
) error {
	tx, ok := sellerpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"seller: ghi vết kiểm toán ngoài giao dịch của kho lưu trữ — " +
				"đình chỉ và vết kiểm toán phải cùng thành công hoặc cùng " +
				"thất bại")
	}

	// WriteSensitive kiểm tra lý do: tối thiểu 20 ký tự, chặn giá trị rác.
	// Đây là chốt chặn cuối phía server — giao diện cũng kiểm tra, nhưng đó
	// chỉ là trải nghiệm vì người dùng gọi API trực tiếp được.
	return a.rec.WriteSensitive(ctx, tx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      in.ActorID,
		Action:       "seller.suspend",
		ResourceType: audit.ResourceSeller,
		ResourceID:   in.SellerID.String(),
		Reason:       in.Reason,
		RequestID:    in.RequestID,
		Metadata: map[string]any{
			"reason_code": in.ReasonCode,
		},
	})
}

// RecordApproval ghi vết duyệt hồ sơ BẰNG giao dịch của kho lưu trữ.
//
// Dùng Write chứ không phải WriteSensitive: duyệt là kết quả mong đợi của
// việc nộp hồ sơ, không phải thao tác bất thường cần giải trình. Bắt buộc
// lý do ở đây chỉ khiến người duyệt gõ cho có.
//
// Nhưng vết vẫn BẮT BUỘC, và vẫn phải nằm trong giao dịch: duyệt là cam kết
// một tỷ lệ hoa hồng, và tranh chấp về con số đó cần biết ai đặt, lúc nào.
func (a *auditRecorder) RecordApproval(
	ctx context.Context, in application.ApprovalRecord,
) error {
	tx, ok := sellerpg.TxFrom(ctx)
	if !ok {
		return errors.New(
			"seller: ghi vết kiểm toán ngoài giao dịch của kho lưu trữ — " +
				"duyệt hồ sơ và vết kiểm toán phải cùng thành công hoặc " +
				"cùng thất bại")
	}

	return a.rec.WriteTx(ctx, tx, audit.Entry{
		ActorType:    audit.ActorUser,
		ActorID:      in.ActorID,
		Action:       "seller.approve",
		ResourceType: audit.ResourceSeller,
		ResourceID:   in.SellerID.String(),
		Reason:       in.Notes,
		RequestID:    in.RequestID,
		Metadata: map[string]any{
			"commission_rate_bp": in.CommissionRateBP,
		},
	})
}
