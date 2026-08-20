package seller

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/seller/application"
	"github.com/fashion-commerce/platform/internal/modules/seller/domain"
	sellerpg "github.com/fashion-commerce/platform/internal/modules/seller/infrastructure/postgres"
	sellerhttp "github.com/fashion-commerce/platform/internal/modules/seller/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/audit"
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

	// Audit là nơi ghi nhật ký thao tác nhạy cảm.
	//
	// Thiếu nó thì module vẫn khởi tạo được (các use case khác không cần),
	// nhưng SuspendSeller sẽ trả lỗi thay vì âm thầm bỏ qua việc ghi vết.
	Audit *audit.Recorder
}

// New khởi tạo module seller.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New("seller: chỉ hỗ trợ kho lưu trữ postgres")
	}
	if cfg.DB == nil {
		return nil, errors.New("seller: bắt buộc phải có kết nối database")
	}

	deps := application.Deps{
		Sellers: sellerpg.NewStore(cfg.DB.Pool()),
		Clock:   cfg.Clock,
	}
	if cfg.Audit != nil {
		deps.Audit = NewAuditRecorder(cfg.Audit)
	}

	return &Module{svc: application.NewService(deps)}, nil
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

func (m *Module) ApproveSeller(
	ctx context.Context, req ApproveRequest,
) (*ApproveResult, error) {
	id, err := ids.Parse(req.SellerID, ids.PrefixSeller)
	if err != nil {
		return nil, ErrInvalidID
	}

	// Kiểm tra khoảng [0, 10000] ngay tại biên module: hoa hồng 150% lọt
	// vào là mọi đơn của seller đó tính sai cho tới khi có người để ý.
	rate, err := types.NewBasisPoints(req.CommissionRateBP)
	if err != nil {
		return nil, ErrInvalidCommissionRate
	}

	sel, err := m.svc.ApproveWithAudit(ctx, application.ApproveInput{
		SellerID:         id,
		ActorID:          req.ActorID,
		CommissionRateBP: rate,
		Notes:            req.Notes,
		RequestID:        req.RequestID,
	})
	if err != nil {
		return nil, translateErr(err)
	}

	return &ApproveResult{
		Seller:      toView(sel),
		SideEffects: application.ApprovalSideEffects(sel),
	}, nil
}

// RegisterAdminRoutes gắn các endpoint quản trị vào mux.
//
// Tên có tiền tố "Admin" vì mux truyền vào PHẢI đã bọc Auth và
// RequireRole("ADMIN", "OPS_MERCHANDISING"). Gắn nhầm vào mux công khai
// nghĩa là bất kỳ ai cũng đình chỉ được nhà bán.
func (m *Module) RegisterAdminRoutes(mux *http.ServeMux, log *slog.Logger) {
	sellerhttp.NewHandler(m.svc, log).Register(mux)
}

// RegisterPublicRoutes gắn endpoint tra hồ sơ nhà bán cho KHÁCH.
//
// Mux truyền vào KHÔNG cần Auth: khách chưa đăng nhập vẫn phải xem được
// mình đang mua của ai.
//
// Handler công khai trả một tập trường HẸP HƠN hẳn bản quản trị — không
// có tên pháp lý, mã số thuế, liên hệ hay tỷ lệ hoa hồng. Xem
// interfaces/http/public.go.
func (m *Module) RegisterPublicRoutes(mux *http.ServeMux, log *slog.Logger) {
	sellerhttp.NewPublicHandler(m.svc, log).Register(mux)
}

// suspensionNote là ghi chú tác động trả cho người vận hành.
//
// Quy tắc quan trọng nhất của thao tác này, nên nó đi kèm mọi response chứ
// không nằm trong tài liệu để người ta tự tìm.
const suspensionNote = "Đơn đang xử lý KHÔNG bị hủy — seller phải hoàn tất " +
	"hoặc chuyển admin xử lý"

func (m *Module) SuspendSeller(
	ctx context.Context, req SuspendRequest,
) (*SuspendResult, error) {
	id, err := ids.Parse(req.SellerID, ids.PrefixSeller)
	if err != nil {
		return nil, ErrInvalidID
	}

	sel, err := m.svc.SuspendWithAudit(ctx, application.SuspendInput{
		SellerID:   id,
		ActorID:    req.ActorID,
		Reason:     req.Reason,
		ReasonCode: req.ReasonCode,
		RequestID:  req.RequestID,
	})
	if err != nil {
		return nil, translateErr(err)
	}

	return &SuspendResult{Seller: toView(sel), Note: suspensionNote}, nil
}

func (m *Module) ListSellers(ctx context.Context, f ListFilter) ([]SellerView, error) {
	list, err := m.svc.ListSellers(ctx, domain.Filter{
		Status: domain.Status(f.Status),
		Type:   domain.SellerType(f.SellerType),
		Limit:  f.Limit,
		Offset: f.Offset,
	})
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]SellerView, 0, len(list))
	for _, sel := range list {
		out = append(out, toView(sel))
	}
	return out, nil
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
