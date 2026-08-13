// Package application chứa các use case của module seller.
package application

import (
	"context"
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

// Service là tầng application của module seller.
type Service struct {
	sellers domain.Repository
	clock   Clock
}

type Deps struct {
	Sellers domain.Repository
	Clock   Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{sellers: d.Sellers, clock: clock}
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
