// Package returns là module trả hàng.
//
// Tài liệu kiến trúc gọi module này là `return`; ở mã nguồn nó là
// `returns` vì `return` là từ khóa của Go.
package returns

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/inventory"
	"github.com/fashion-commerce/platform/internal/modules/order"
	"github.com/fashion-commerce/platform/internal/modules/payment"
	"github.com/fashion-commerce/platform/internal/modules/returns/application"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
	returnspg "github.com/fashion-commerce/platform/internal/modules/returns/infrastructure/postgres"
	returnshttp "github.com/fashion-commerce/platform/internal/modules/returns/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/database"
)

// Module là cài đặt của API công khai.
type Module struct{ svc *application.Service }

// Config gom phụ thuộc.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres" — yêu cầu trả hàng là
	// chứng từ tài chính, không giữ trong bộ nhớ được.
	Storage string

	DB *database.DB

	Order     order.API
	Payment   payment.API
	Inventory inventory.API

	// Owner đổi mã nhà bán thành chủ sở hữu tồn kho. Nil thì coi hai thứ
	// là một, đúng với mọi nhà bán trừ own brand.
	Owner OwnerResolver

	Clock application.Clock
}

func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New("returns: chỉ hỗ trợ kho lưu trữ postgres")
	}
	if cfg.DB == nil {
		return nil, errors.New("returns: bắt buộc phải có kết nối database")
	}
	if cfg.Order == nil || cfg.Payment == nil || cfg.Inventory == nil {
		return nil, errors.New(
			"returns: bắt buộc phải có order, payment và inventory — " +
				"trả hàng là đảo ngược cả ba")
	}

	return &Module{svc: application.NewService(application.Deps{
		Repo:      returnspg.NewStore(cfg.DB.Pool()),
		Orders:    &orderAdapter{api: cfg.Order},
		Inventory: &inventoryAdapter{api: cfg.Inventory, owner: cfg.Owner},
		Payment:   &paymentAdapter{api: cfg.Payment},
		Clock:     cfg.Clock,
	})}, nil
}

// Service trả tầng application. CHỈ dùng trong test tích hợp.
func (m *Module) Service() *application.Service { return m.svc }

// RegisterCustomerRoutes gắn endpoint xin trả hàng của KHÁCH.
//
// Bên gọi PHẢI bọc ResolveShopper: handler lấy định danh khách từ context
// để chỉ cho trả hàng của chính họ.
func (m *Module) RegisterCustomerRoutes(
	mux *http.ServeMux, access returnshttp.OrderAccessPort, log *slog.Logger,
) {
	returnshttp.NewCustomerHandler(m.svc, access, log).Register(mux)
}

// RegisterSellerRoutes gắn endpoint duyệt trả hàng của NHÀ BÁN.
func (m *Module) RegisterSellerRoutes(mux *http.ServeMux, log *slog.Logger) {
	returnshttp.NewSellerHandler(m.svc, log).Register(mux)
}

// ---------------------------------------------------------------- Lỗi công khai

var (
	ErrNotFound      = errors.New("returns: không tìm thấy yêu cầu trả hàng")
	ErrInvalidID     = errors.New("returns: định danh không hợp lệ")
	ErrInvalidStatus = errors.New("returns: không thực hiện được với trạng thái hiện tại")
)

// Duyet là lối vào cho test tích hợp và công cụ vận hành.
func (m *Module) Duyet(ctx context.Context, returnID, sellerID string) error {
	id, err := ids.Parse(returnID, ids.PrefixReturnRequest)
	if err != nil {
		return ErrInvalidID
	}
	_, err = m.svc.Duyet(ctx, id, ids.ID(sellerID))
	return dichLoi(err)
}

// NhanHangVaHoanTien là lối vào cho test tích hợp và công cụ vận hành.
func (m *Module) NhanHangVaHoanTien(ctx context.Context, returnID, sellerID string) error {
	id, err := ids.Parse(returnID, ids.PrefixReturnRequest)
	if err != nil {
		return ErrInvalidID
	}
	_, err = m.svc.NhanHangVaHoanTien(ctx, id, ids.ID(sellerID))
	return dichLoi(err)
}

func dichLoi(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrInvalidStatus):
		return ErrInvalidStatus
	}
	return err
}
