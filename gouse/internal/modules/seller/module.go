package seller

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/seller/application"
	"github.com/fashion-commerce/platform/internal/modules/seller/domain"
	sellerpg "github.com/fashion-commerce/platform/internal/modules/seller/infrastructure/postgres"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: hiện chỉ hỗ trợ "postgres".
	Storage string

	// DB là kết nối database. BẮT BUỘC.
	DB *database.DB

	Clock application.Clock
}

// New khởi tạo module seller.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New("seller: chỉ hỗ trợ kho lưu trữ postgres")
	}
	if cfg.DB == nil {
		return nil, errors.New("seller: bắt buộc phải có kết nối database")
	}

	return &Module{svc: application.NewService(application.Deps{
		Sellers: sellerpg.NewStore(cfg.DB.Pool()),
		Clock:   cfg.Clock,
	})}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- API

func (m *Module) GetSeller(ctx context.Context, sellerID string) (*SellerView, error) {
	id, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return nil, ErrInvalidID
	}
	sel, err := m.svc.GetSeller(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toView(sel)
	return &v, nil
}

func (m *Module) GetSellersByIDs(ctx context.Context, sellerIDs []string) (map[string]SellerView, error) {
	parsed := make([]ids.ID, 0, len(sellerIDs))
	for _, raw := range sellerIDs {
		id, err := ids.Parse(raw, ids.PrefixSeller)
		if err != nil {
			// Bỏ qua id sai định dạng thay vì làm hỏng cả lời gọi.
			continue
		}
		parsed = append(parsed, id)
	}

	found, err := m.svc.GetSellersByIDs(ctx, parsed)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]SellerView, len(found))
	for id, sel := range found {
		out[id.String()] = toView(sel)
	}
	return out, nil
}

func (m *Module) IsSellerActive(ctx context.Context, sellerID string) (bool, error) {
	id, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return false, ErrInvalidID
	}
	active, err := m.svc.IsActive(ctx, id)
	if err != nil {
		return false, translateErr(err)
	}
	return active, nil
}

func (m *Module) ApplyAsSeller(ctx context.Context, req ApplicationRequest) (*SellerView, error) {
	rate, err := types.NewBasisPoints(req.CommissionRateBP)
	if err != nil {
		return nil, err
	}

	sel, err := m.svc.Apply(ctx, application.ApplyInput{
		Name:           req.Name,
		Slug:           req.Slug,
		SellerType:     domain.SellerType(req.SellerType),
		LegalName:      req.LegalName,
		TaxCode:        req.TaxCode,
		Email:          req.Email,
		Phone:          req.Phone,
		CommissionRate: rate,
	})
	if err != nil {
		return nil, translateErr(err)
	}
	v := toView(sel)
	return &v, nil
}

func (m *Module) ApproveSeller(ctx context.Context, sellerID string, approvedBy string) error {
	id, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return ErrInvalidID
	}
	_, err = m.svc.Approve(ctx, id, approvedBy)
	return translateErr(err)
}

func (m *Module) SuspendSeller(ctx context.Context, sellerID string, reason string) error {
	id, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return ErrInvalidID
	}
	_, err = m.svc.Suspend(ctx, id, reason)
	return translateErr(err)
}

// ---------------------------------------------------------------- Chuyển đổi

func toView(s *domain.Seller) SellerView {
	return SellerView{
		ID:               s.ID().String(),
		Name:             s.Name(),
		Slug:             s.Slug(),
		SellerType:       string(s.Type()),
		Status:           string(s.Status()),
		CommissionRateBP: s.CommissionRate().Value(),
		IsActive:         s.IsActive(),
		IsInternal:       s.IsInternal(),
		OffersHidden:     s.OffersHidden(),
	}
}

func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrInvalidStatus),
		errors.Is(err, domain.ErrNoBankAccount):
		return ErrNotAllowed
	}
	return err
}
