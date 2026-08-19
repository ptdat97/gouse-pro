package fulfillment

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/application"
	"github.com/fashion-commerce/platform/internal/modules/fulfillment/domain"
	fulfillmentpg "github.com/fashion-commerce/platform/internal/modules/fulfillment/infrastructure/postgres"
	fulfillmenthttp "github.com/fashion-commerce/platform/internal/modules/fulfillment/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/eventbus"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Ranh giới bảo mật của seller được cưỡng chế bằng mệnh đề WHERE trong
	// SQL. Một bản in-memory sẽ lọc bằng vòng lặp Go — và vòng lặp thì có
	// thể quên, trong khi câu SQL đã viết sẵn thì không.
	Storage string

	DB    *database.DB
	Clock application.Clock

	// Events phát domain event. Nil nghĩa là KHÔNG phát.
	//
	// Ở production đây là thiếu sót nghiêm trọng: không có event thì trạng
	// thái tổng hợp của đơn hàng không bao giờ được cập nhật, và khách
	// thấy đơn mãi ở trạng thái "đang xử lý" dù hàng đã giao.
	Events *eventbus.Outbox
}

// New khởi tạo module fulfillment.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"fulfillment: chỉ hỗ trợ kho lưu trữ postgres — ranh giới bảo mật " +
				"của seller nằm trong mệnh đề WHERE của câu SQL")
	}
	if cfg.DB == nil {
		return nil, errors.New("fulfillment: bắt buộc phải có kết nối database")
	}

	deps := application.Deps{
		Repo:  fulfillmentpg.NewFulfillmentStore(cfg.DB.Pool()),
		Clock: cfg.Clock,
	}
	if cfg.Events != nil {
		deps.Events = &eventPublisher{outbox: cfg.Events}
	}

	return &Module{svc: application.NewService(deps)}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- API

func (m *Module) GetOrderFulfillments(
	ctx context.Context, orderID string,
) ([]FulfillmentView, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	fos, err := m.svc.ListByOrder(ctx, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return toViews(fos), nil
}

func (m *Module) ListSellerFulfillments(
	ctx context.Context, sellerID string, statuses []string, limit, offset int,
) ([]FulfillmentView, error) {
	id, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return nil, ErrInvalidID
	}

	filter := make([]domain.FOStatus, 0, len(statuses))
	for _, st := range statuses {
		filter = append(filter, domain.FOStatus(st))
	}

	fos, err := m.svc.ListSellerWork(ctx, id, filter, limit, offset)
	if err != nil {
		return nil, translateErr(err)
	}
	return toViews(fos), nil
}

func (m *Module) GetSellerFulfillment(
	ctx context.Context, sellerID, fulfillmentID string,
) (*FulfillmentView, error) {
	sid, fid, err := parseSellerAndFO(sellerID, fulfillmentID)
	if err != nil {
		return nil, err
	}
	fo, err := m.svc.GetSellerFulfillment(ctx, sid, fid)
	if err != nil {
		return nil, translateErr(err)
	}
	v := toView(fo)
	return &v, nil
}

func (m *Module) AllocateInventory(
	ctx context.Context, sellerID, fulfillmentID, locationID string,
) error {
	sid, fid, err := parseSellerAndFO(sellerID, fulfillmentID)
	if err != nil {
		return err
	}

	var locID ids.ID
	if locationID != "" {
		id, err := ids.Parse(locationID, ids.PrefixStockLocation)
		if err != nil {
			return ErrInvalidID
		}
		locID = id
	}

	return translateErr(m.svc.Allocate(ctx, sid, fid, locID))
}

func (m *Module) ConfirmFulfillment(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.step(ctx, sellerID, fulfillmentID, m.svc.Confirm)
}

func (m *Module) MarkPicking(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.step(ctx, sellerID, fulfillmentID, m.svc.Pick)
}

func (m *Module) MarkPacked(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.step(ctx, sellerID, fulfillmentID, m.svc.Pack)
}

func (m *Module) HandOverToCarrier(ctx context.Context, req HandOverRequest) error {
	sid, fid, err := parseSellerAndFO(req.SellerID, req.FulfillmentID)
	if err != nil {
		return err
	}
	return translateErr(m.svc.HandOver(ctx, sid, fid,
		strings.TrimSpace(req.Provider), strings.TrimSpace(req.TrackingNumber)))
}

func (m *Module) MarkInTransit(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.step(ctx, sellerID, fulfillmentID, m.svc.MarkInTransit)
}

func (m *Module) MarkDeliveryFailed(
	ctx context.Context, sellerID, fulfillmentID, reason string,
) error {
	sid, fid, err := parseSellerAndFO(sellerID, fulfillmentID)
	if err != nil {
		return err
	}
	return translateErr(m.svc.MarkDeliveryFailed(ctx, sid, fid, strings.TrimSpace(reason)))
}

func (m *Module) MarkDelivered(ctx context.Context, sellerID, fulfillmentID string) error {
	return m.step(ctx, sellerID, fulfillmentID, m.svc.Deliver)
}

func (m *Module) CancelFulfillment(
	ctx context.Context, sellerID, fulfillmentID, reason string,
) error {
	sid, fid, err := parseSellerAndFO(sellerID, fulfillmentID)
	if err != nil {
		return err
	}
	return translateErr(m.svc.Cancel(ctx, sid, fid, strings.TrimSpace(reason)))
}

func (m *Module) CompleteDelivered(ctx context.Context, limit int) (int, error) {
	n, err := m.svc.CompleteDelivered(ctx, limit)
	return n, translateErr(err)
}

func (m *Module) step(
	ctx context.Context, sellerID, fulfillmentID string,
	fn func(context.Context, ids.ID, ids.ID) error,
) error {
	sid, fid, err := parseSellerAndFO(sellerID, fulfillmentID)
	if err != nil {
		return err
	}
	return translateErr(fn(ctx, sid, fid))
}

func parseSellerAndFO(sellerID, fulfillmentID string) (ids.ID, ids.ID, error) {
	sid, err := ids.Parse(sellerID, ids.PrefixSeller)
	if err != nil {
		return "", "", ErrInvalidID
	}
	fid, err := ids.Parse(fulfillmentID, ids.PrefixFulfillmentOrder)
	if err != nil {
		return "", "", ErrInvalidID
	}
	return sid, fid, nil
}

// ---------------------------------------------------------------- Chuyển đổi

func toAmount(m money.Money) Amount {
	return Amount{Value: m.Amount(), Currency: string(m.Currency())}
}

func toViews(fos []*domain.FulfillmentOrder) []FulfillmentView {
	out := make([]FulfillmentView, 0, len(fos))
	for _, fo := range fos {
		out = append(out, toView(fo))
	}
	return out
}

func toView(fo *domain.FulfillmentOrder) FulfillmentView {
	lineIDs := fo.LineIDs()
	strIDs := make([]string, 0, len(lineIDs))
	for _, id := range lineIDs {
		strIDs = append(strIDs, id.String())
	}

	return FulfillmentView{
		ID:                fo.ID().String(),
		OrderID:           fo.OrderID().String(),
		FONumber:          fo.FONumber(),
		SellerID:          fo.SellerID().String(),
		Status:            string(fo.Status()),
		Type:              string(fo.Type()),
		LineIDs:           strIDs,
		Subtotal:          toAmount(fo.Subtotal()),
		CommissionAmount:  toAmount(fo.CommissionAmount()),
		SellerPayable:     toAmount(fo.SellerPayable()),
		StockLocationID:   fo.StockLocationID().String(),
		ShippingMethod:    fo.ShippingMethod(),
		ShippingProvider:  fo.ShippingProvider(),
		TrackingNumber:    fo.TrackingNumber(),
		EstimatedDelivery: formatTime(fo.EstimatedDelivery()),
		CancelReason:      fo.CancelReason(),
		FailureReason:     fo.FailureReason(),
		ConfirmedAt:       formatTime(fo.ConfirmedAt()),
		PackedAt:          formatTime(fo.PackedAt()),
		ShippedAt:         formatTime(fo.ShippedAt()),
		DeliveredAt:       formatTime(fo.DeliveredAt()),
		CompletedAt:       formatTime(fo.CompletedAt()),
		CancelledAt:       formatTime(fo.CancelledAt()),
		CreatedAt:         formatTime(fo.CreatedAt()),
	}
}

// formatTime trả chuỗi rỗng cho mốc thời gian chưa xảy ra.
//
// Trả "0001-01-01T00:00:00Z" sẽ khiến giao diện hiện một ngày vô nghĩa.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, application.ErrForbidden):
		return ErrForbidden
	case errors.Is(err, domain.ErrInvalidStatus):
		return ErrInvalidStatus
	case errors.Is(err, domain.ErrNoLines):
		return ErrInvalidInput
	}
	return err
}

// RegisterCustomerRoutes gắn endpoint lô giao cho KHÁCH.
//
// `access` trả lời câu "người này có được xem đơn đó không" — quy tắc đó
// thuộc module `order`, và module này HỎI thay vì cài lại. Hai bản cài đặt
// của một quy tắc bảo mật sẽ lệch nhau, và một bản lỏng là đủ để lộ lịch
// sử mua hàng.
//
// Bên gọi PHẢI bọc httpserver.ResolveShopper.
func (m *Module) RegisterCustomerRoutes(
	mux *http.ServeMux, access fulfillmenthttp.OrderAccessPort, log *slog.Logger,
) {
	fulfillmenthttp.NewCustomerHandler(m.svc, access, log).Register(mux)
}

// RegisterSellerRoutes gắn các endpoint đơn thực hiện của NHÀ BÁN.
//
// Bên gọi PHẢI bọc httpserver.Auth và RequireRole("SELLER_OWNER",
// "SELLER_STAFF"): định danh nhà bán lấy từ token, và handler từ chối khi
// token không gắn với nhà bán nào.
func (m *Module) RegisterSellerRoutes(mux *http.ServeMux, log *slog.Logger) {
	fulfillmenthttp.NewSellerHandler(m.svc, log).Register(mux)
}
