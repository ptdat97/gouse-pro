// Package application chứa các use case của module seller.
package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/seller/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// SystemClock là đồng hồ thật, dùng ở production.
var SystemClock Clock = systemClock{}

// AuditRecorder ghi vết kiểm toán cho thao tác nhạy cảm.
//
// Là PORT do tầng application định nghĩa nên nó không biết database hay
// bảng `audit_log`. Ngữ cảnh truyền vào PHẢI mang giao dịch của kho lưu
// trữ — xem domain.TxFunc.
type AuditRecorder interface {
	// RecordSuspension ghi việc đình chỉ nhà bán.
	//
	// Trả lỗi nếu lý do trống hoặc quá ngắn: đình chỉ một gian hàng là cắt
	// nguồn thu của người khác, và "vì sao" là thứ duy nhất trả lời được
	// khi họ khiếu nại sáu tháng sau.
	RecordSuspension(ctx context.Context, in SuspensionRecord) error

	// RecordApproval ghi việc duyệt hồ sơ nhà bán.
	//
	// KHÔNG bắt buộc lý do: duyệt là kết quả mong đợi của việc nộp hồ sơ,
	// không phải thao tác bất thường cần giải trình. Nhưng vẫn phải có vết
	// — duyệt nghĩa là mở quyền bán hàng và cam kết một tỷ lệ hoa hồng.
	RecordApproval(ctx context.Context, in ApprovalRecord) error
}

// ApprovalRecord là dữ liệu ghi vào nhật ký khi duyệt hồ sơ nhà bán.
type ApprovalRecord struct {
	SellerID ids.ID

	// ActorID là nhân viên duyệt. KHÔNG được rỗng.
	ActorID string

	// CommissionRateBP là tỷ lệ hoa hồng đã đặt, theo phần vạn.
	//
	// Ghi vào vết kiểm toán vì đây là cam kết tài chính: tranh chấp về hoa
	// hồng sáu tháng sau cần biết con số được đặt lúc nào và bởi ai.
	CommissionRateBP int32

	Notes     string
	RequestID string
}

// SuspensionRecord là dữ liệu ghi vào nhật ký khi đình chỉ nhà bán.
type SuspensionRecord struct {
	SellerID ids.ID

	// ActorID là nhân viên thực hiện. KHÔNG được rỗng.
	ActorID string

	Reason     string
	ReasonCode string

	// RequestID nối vết kiểm toán với chuỗi log của request.
	RequestID string
}

// Service là tầng application của module seller.
type Service struct {
	sellers domain.Repository
	audit   AuditRecorder
	clock   Clock
}

type Deps struct {
	Sellers domain.Repository
	Clock   Clock

	// Audit có thể nil: các use case không nhạy cảm vẫn chạy được. Chỉ
	// SuspendWithAudit bắt buộc có nó, và nó báo lỗi rõ nếu thiếu.
	Audit AuditRecorder
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{sellers: d.Sellers, audit: d.Audit, clock: clock}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Đăng ký

// ApplyInput là hồ sơ đăng ký nhà bán.
type ApplyInput struct {
	Name           string
	Slug           string
	SellerType     domain.SellerType
	LegalName      string
	TaxCode        string
	Email          string
	Phone          string
	CommissionRate types.BasisPoints
}

// Apply nộp hồ sơ đăng ký làm nhà bán.
func (s *Service) Apply(ctx context.Context, in ApplyInput) (*domain.Seller, error) {
	sel, err := domain.NewSeller(domain.NewSellerParams{
		Name:           in.Name,
		Slug:           in.Slug,
		SellerType:     in.SellerType,
		LegalName:      in.LegalName,
		TaxCode:        in.TaxCode,
		Email:          in.Email,
		Phone:          in.Phone,
		CommissionRate: in.CommissionRate,
		Now:            s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.sellers.Save(ctx, sel); err != nil {
		return nil, err
	}
	return sel, nil
}

// SubmitForReview nộp hồ sơ đi duyệt.
func (s *Service) SubmitForReview(ctx context.Context, id ids.ID) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		return sel.SubmitForReview(now)
	})
}

// Approve duyệt hồ sơ.
func (s *Service) Approve(ctx context.Context, id ids.ID, by string) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		return sel.Approve(by, now)
	})
}

// Reject từ chối hồ sơ kèm lý do.
func (s *Service) Reject(ctx context.Context, id ids.ID, reason string) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		return sel.Reject(reason, now)
	})
}

// Activate kích hoạt nhà bán.
//
// Yêu cầu tài khoản ngân hàng đã xác minh (quy tắc 1) — được cưỡng chế ở
// domain, không phải ở đây.
func (s *Service) Activate(ctx context.Context, id ids.ID) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		return sel.Activate(now)
	})
}

// Suspend đình chỉ nhà bán.
//
// LƯU Ý: việc này làm ẩn offer nhưng KHÔNG hủy đơn đang xử lý. Module
// marketplace nghe event và ẩn offer; module order KHÔNG được đụng tới đơn
// khách đã trả tiền.
func (s *Service) Suspend(ctx context.Context, id ids.ID, reason string) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		return sel.Suspend(reason, now)
	})
}

// ApproveInput là dữ liệu duyệt hồ sơ nhà bán từ giao diện quản trị.
type ApproveInput struct {
	SellerID ids.ID
	ActorID  string

	// CommissionRateBP theo phần vạn. Bắt buộc với seller không phải
	// INTERNAL — quên đặt nghĩa là bán hộ miễn phí.
	CommissionRateBP types.BasisPoints

	Notes     string
	RequestID string
}

// ApproveWithAudit duyệt hồ sơ VÀ ghi vết kiểm toán trong CÙNG giao dịch.
//
// Duyệt chuyển seller sang APPROVED, chưa phải ACTIVE — seller chỉ bán được
// khi tài khoản ngân hàng đã xác minh (quy tắc 1, cưỡng chế ở database).
// Tách hai bước là có chủ ý: người duyệt hồ sơ pháp lý và người xác minh
// tài khoản ngân hàng thường không phải một người.
func (s *Service) ApproveWithAudit(
	ctx context.Context, in ApproveInput,
) (*domain.Seller, error) {
	if s.audit == nil {
		return nil, errors.New(
			"seller: thiếu AuditRecorder — duyệt hồ sơ là cam kết tài chính, " +
				"không được chạy khi chưa có đường ghi vết kiểm toán")
	}
	if strings.TrimSpace(in.ActorID) == "" {
		return nil, errors.New("seller: thiếu định danh người duyệt")
	}

	sel, err := s.sellers.FindByID(ctx, in.SellerID)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	if err := sel.Approve(in.ActorID, now); err != nil {
		return nil, err
	}

	// Seller INTERNAL luôn có hoa hồng 0 — own brand không tự trả hoa hồng
	// cho chính mình, và database có ràng buộc CHECK cho việc này.
	//
	// Thứ tự đặt hoa hồng so với Approve không ảnh hưởng tính đúng đắn: mọi
	// đường lỗi đều return trước khi lưu, nên `sel` bị bỏ đi nguyên vẹn.
	// Kiểm chứng bằng cách đảo thứ tự — test vẫn xanh.
	if !sel.IsInternal() {
		if err := sel.SetCommissionRate(in.CommissionRateBP, now); err != nil {
			return nil, err
		}
	}

	err = s.sellers.SaveWithAudit(ctx, sel, func(txCtx context.Context) error {
		return s.audit.RecordApproval(txCtx, ApprovalRecord{
			SellerID:         in.SellerID,
			ActorID:          in.ActorID,
			CommissionRateBP: sel.CommissionRate().Value(),
			Notes:            in.Notes,
			RequestID:        in.RequestID,
		})
	})
	if err != nil {
		return nil, err
	}

	return sel, nil
}

// ApprovalSideEffects mô tả điều đã xảy ra, và điều CHƯA xảy ra.
//
// Nêu rõ bước còn thiếu quan trọng hơn liệt kê việc đã xong: người duyệt
// thường tưởng duyệt xong là seller bán được ngay, rồi không hiểu vì sao
// gian hàng vẫn im lìm.
//
// Đặt ở application vì nó suy ra từ TRẠNG THÁI DOMAIN, không phải cách hiển
// thị. Để ở hai nơi thì hai bản sẽ lệch nhau ngay lần đổi quy tắc đầu tiên.
func ApprovalSideEffects(sel *domain.Seller) []string {
	out := []string{"Hồ sơ chuyển sang trạng thái APPROVED"}

	if sel.IsInternal() {
		return out
	}

	out = append(out, "Đã đặt tỷ lệ hoa hồng")
	if !sel.BankAccountVerified() {
		out = append(out,
			"CHƯA kích hoạt: cần xác minh tài khoản ngân hàng trước khi "+
				"seller bán được hàng")
	}
	return out
}

// SuspendInput là dữ liệu đình chỉ nhà bán từ giao diện quản trị.
type SuspendInput struct {
	SellerID   ids.ID
	ActorID    string
	Reason     string
	ReasonCode string
	RequestID  string
}

// SuspendWithAudit đình chỉ nhà bán VÀ ghi vết kiểm toán trong CÙNG một
// giao dịch.
//
// Đây là đường mà giao diện quản trị phải dùng, không phải Suspend. Suspend
// dành cho thao tác nội bộ của hệ thống (nơi actor là chính hệ thống);
// người thật đình chỉ một gian hàng thì luôn phải để lại vết.
//
// Ghi chú về tác động: đình chỉ làm ẩn offer nhưng KHÔNG hủy đơn khách đã
// trả tiền. Việc ẩn offer do marketplace làm khi nghe event, tức là BẤT
// ĐỒNG BỘ — vì vậy hàm này không trả về số offer đã ẩn: con số đó chưa tồn
// tại tại thời điểm trả lời.
func (s *Service) SuspendWithAudit(
	ctx context.Context, in SuspendInput,
) (*domain.Seller, error) {
	if s.audit == nil {
		return nil, errors.New(
			"seller: thiếu AuditRecorder — thao tác nhạy cảm không được " +
				"chạy khi chưa có đường ghi vết kiểm toán")
	}
	if strings.TrimSpace(in.ActorID) == "" {
		return nil, errors.New("seller: thiếu định danh người thực hiện")
	}

	sel, err := s.sellers.FindByID(ctx, in.SellerID)
	if err != nil {
		return nil, err
	}
	if err := sel.Suspend(in.Reason, s.clock.Now()); err != nil {
		return nil, err
	}

	err = s.sellers.SaveWithAudit(ctx, sel, func(txCtx context.Context) error {
		return s.audit.RecordSuspension(txCtx, SuspensionRecord{
			SellerID:   in.SellerID,
			ActorID:    in.ActorID,
			Reason:     in.Reason,
			ReasonCode: in.ReasonCode,
			RequestID:  in.RequestID,
		})
	})
	if err != nil {
		return nil, err
	}

	return sel, nil
}

// Reactivate khôi phục nhà bán.
func (s *Service) Reactivate(ctx context.Context, id ids.ID) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		return sel.Reactivate(now)
	})
}

// GoOnVacation chuyển sang chế độ nghỉ bán.
func (s *Service) GoOnVacation(ctx context.Context, id ids.ID) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		return sel.GoOnVacation(now)
	})
}

// Terminate chấm dứt hợp tác.
func (s *Service) Terminate(ctx context.Context, id ids.ID, reason string) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		return sel.Terminate(reason, now)
	})
}

// VerifyBankAccount đánh dấu tài khoản ngân hàng đã xác minh.
func (s *Service) VerifyBankAccount(ctx context.Context, id ids.ID) (*domain.Seller, error) {
	return s.change(ctx, id, func(sel *domain.Seller, now time.Time) error {
		sel.VerifyBankAccount(now)
		return nil
	})
}

// ---------------------------------------------------------------- Đọc

func (s *Service) GetSeller(ctx context.Context, id ids.ID) (*domain.Seller, error) {
	return s.sellers.FindByID(ctx, id)
}

func (s *Service) GetSellerBySlug(ctx context.Context, slug string) (*domain.Seller, error) {
	return s.sellers.FindBySlug(ctx, slug)
}

func (s *Service) GetSellersByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*domain.Seller, error) {
	return s.sellers.FindByIDs(ctx, list)
}

func (s *Service) ListSellers(ctx context.Context, f domain.Filter) ([]*domain.Seller, error) {
	return s.sellers.List(ctx, f)
}

// IsActive cho biết nhà bán có đang bán hàng được không.
//
// Module marketplace gọi TRƯỚC khi hiển thị offer: seller bị đình chỉ thì
// mọi offer phải ẩn (quy tắc 4 của marketplace).
func (s *Service) IsActive(ctx context.Context, id ids.ID) (bool, error) {
	sel, err := s.sellers.FindByID(ctx, id)
	if err != nil {
		return false, err
	}
	return sel.IsActive(), nil
}

// EnsureInternalSeller tạo seller nội bộ (own brand) nếu chưa có.
//
// Own brand là một seller INTERNAL, KHÔNG phải đường đi riêng (mục 3 của
// đặc tả). Nhờ vậy đơn hàng lẫn own brand và hàng seller đi CHUNG một luồng.
//
// Idempotent: gọi nhiều lần trả về cùng một seller.
func (s *Service) EnsureInternalSeller(ctx context.Context, name, slug string) (*domain.Seller, error) {
	existing, err := s.sellers.FindBySlug(ctx, slug)
	if err == nil {
		return existing, nil
	}

	sel, err := domain.NewSeller(domain.NewSellerParams{
		Name:       name,
		Slug:       slug,
		SellerType: domain.SellerInternal,
		Now:        s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}

	// Đưa thẳng tới ACTIVE: own brand không cần duyệt và không cần tài
	// khoản ngân hàng (nhận tiền qua sổ cái nội bộ).
	now := s.clock.Now()
	for _, step := range []func() error{
		func() error { return sel.SubmitForReview(now) },
		func() error { return sel.Approve("system", now) },
		func() error { return sel.Activate(now) },
	} {
		if err := step(); err != nil {
			return nil, err
		}
	}

	if err := s.sellers.Save(ctx, sel); err != nil {
		return nil, err
	}
	return sel, nil
}

// change đọc, biến đổi, rồi lưu lại.
func (s *Service) change(
	ctx context.Context, id ids.ID, apply func(*domain.Seller, time.Time) error,
) (*domain.Seller, error) {
	sel, err := s.sellers.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := apply(sel, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.sellers.Save(ctx, sel); err != nil {
		return nil, err
	}
	return sel, nil
}
