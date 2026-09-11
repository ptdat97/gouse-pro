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
	"github.com/fashion-commerce/platform/internal/platform/opsconfig"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service

	// nguongBatTin đọc ngưỡng "im lặng bao lâu thì coi là mất tin".
	//
	// Giữ ở Module chứ không nhét vào `domain.Nguong`: bộ ngưỡng đó là
	// của phép CHẤM ĐIỂM nhà bán, còn con số này là của phép đối chiếu
	// vận hành. Trộn hai thứ lại sẽ có một trường mà phép chấm điểm không
	// bao giờ dùng tới.
	nguongBatTin func() time.Duration

	// bieuPhiPort cấp phí vận chuyển cho đường BÁO GIÁ của tầng module.
	//
	// Tầng application có bản của riêng nó cho đường bàn giao; đây là cùng
	// một cổng, giữ ở hai nơi vì `EstimateShipping` không đi qua service.
	bieuPhiPort application.BieuPhiPort
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

	// OpsConfig cấp ngưỡng chấm hiệu suất sửa được lúc chạy.
	//
	// Nil thì dùng ngưỡng mặc định đã biên dịch — module vẫn chạy đúng khi
	// chưa nối dây phần cấu hình, và khi database không đọc được.
	OpsConfig *opsconfig.Store
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
	if cfg.OpsConfig != nil {
		deps.BieuPhi = &bieuPhiAdapter{cfg: cfg.OpsConfig}
		deps.Nguong = &nguongAdapter{cfg: cfg.OpsConfig}
		// Hạn đổi trả: CÙNG tham số mà `returns` dùng để từ chối yêu cầu
		// quá hạn. Một nguồn cho hai module.
		deps.Han = &hanDoiTraAdapter{cfg: cfg.OpsConfig}
	}

	return &Module{
		svc:          application.NewService(deps),
		nguongBatTin: nguongBatTinTu(cfg.OpsConfig),
		bieuPhiPort:  deps.BieuPhi,
	}, nil
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
	return m.ganHanBanGiao(toViews(fos), fos), nil
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
	v := m.ganHanBanGiao([]FulfillmentView{toView(fo)},
		[]*domain.FulfillmentOrder{fo})[0]
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

// MoKhoaTheoDon mở khóa giao hàng cho mọi đơn thực hiện của một đơn hàng.
//
// Gọi bởi bên nhận event `order.paid` — xem ADR-0018 phần A2. Trả về SỐ
// đơn thực hiện đã đổi (0 nghĩa là không có gì bị khóa, ví dụ đơn COD).
func (m *Module) MoKhoaTheoDon(ctx context.Context, orderID string) (int, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return 0, ErrInvalidID
	}
	n, err := m.svc.MoKhoaTheoDon(ctx, id)
	return n, translateErr(err)
}

// HuyTheoDon hủy mọi đơn thực hiện của một đơn hàng đã bị hủy.
//
// Gọi bởi bên nhận `order.cancelled` — xem `application.HuyTheoDon` về
// việc vì sao nó không kiểm chủ sở hữu và vì sao không được nối ra HTTP.
func (m *Module) HuyTheoDon(ctx context.Context, orderID, lyDo string) (int, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return 0, ErrInvalidID
	}
	n, err := m.svc.HuyTheoDon(ctx, id, lyDo)
	return n, translateErr(err)
}

// DoiSoatGiaoHang trả danh sách gói hàng MẤT TIN từ đơn vị vận chuyển.
//
// Yêu cầu 5 của `api/paths/webhooks.yaml` — xem
// `application.DoiSoatGiaoHang` về việc vì sao nó chỉ đọc.
//
// Ngưỡng đọc từ cấu hình vận hành tại MỖI lần chạy, không chụp lại lúc
// khởi động: đổi tham số phải có tác dụng ở vòng chạy kế tiếp, không phải
// sau lần khởi động lại kế tiếp.
func (m *Module) DoiSoatGiaoHang(ctx context.Context, limit int) ([]GoiBatTinView, error) {
	if m.nguongBatTin == nil {
		return nil, ErrChuaNoiCauHinh
	}
	goi, err := m.svc.DoiSoatGiaoHang(ctx, m.nguongBatTin(), limit)
	if err != nil {
		return nil, translateErr(err)
	}
	out := make([]GoiBatTinView, 0, len(goi))
	for _, g := range goi {
		out = append(out, GoiBatTinView{
			FulfillmentID: g.FulfillmentID.String(),
			FONumber:      g.FONumber,
			OrderID:       g.OrderID.String(),
			SellerID:      g.SellerID.String(),
			NhaVanChuyen:  g.NhaVanChuyen,
			MaVanDon:      g.MaVanDon,
			TrangThai:     g.TrangThai,
			ShippedAt:     g.ShippedAt,
			ImLang:        g.ImLang,
		})
	}
	return out, nil
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

// ganHanBanGiao điền hạn bàn giao và cờ trễ hạn vào các view.
//
// Tách khỏi `toView` vì nó cần SLA đang áp dụng — thứ chỉ tầng application
// đọc được. `toView` là hàm thuần, và giữ nó thuần thì mọi bên gọi khác
// không phải kéo theo một phụ thuộc cấu hình.
func (m *Module) ganHanBanGiao(views []FulfillmentView, fos []*domain.FulfillmentOrder) []FulfillmentView {
	sla := m.svc.SLAHienTai().SLAGiaoHang
	now := m.svc.Now()
	for i := range views {
		views[i].SLADeadline = formatTime(fos[i].HanBanGiao(sla))
		views[i].SLABreached = fos[i].TreHan(sla, now)
	}
	return views
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
// RegisterWebhookRoutes gắn endpoint nhận webhook của hãng vận chuyển.
//
// KHÔNG bọc Auth: bên gọi là hệ thống của hãng vận chuyển, xác thực bằng
// CHỮ KÝ HMAC trên thân request chứ không bằng token.
//
// KHÔNG bọc RequireIdempotencyKey: nhà cung cấp không gửi header đó, và
// idempotency ở đây dựa vào `event_id` của chính họ.
func (m *Module) RegisterWebhookRoutes(
	mux *http.ServeMux,
	nhatKy fulfillmenthttp.GhiSuKien,
	biMat fulfillmenthttp.BiMatNhaCungCap,
	log *slog.Logger,
) {
	fulfillmenthttp.NewWebhookHandler(m.svc, nhatKy, biMat, log).Register(mux)
}

func (m *Module) RegisterSellerRoutes(mux *http.ServeMux, log *slog.Logger) {
	fulfillmenthttp.NewSellerHandler(m.svc, log).Register(mux)
}

// nguongAdapter dịch tham số vận hành sang từ vựng của fulfillment.
//
// Cổng do bên gọi khai, nên tầng application của fulfillment không biết
// tới `opsconfig` — nó chỉ biết mình cần một bộ ngưỡng.
type nguongAdapter struct{ cfg *opsconfig.Store }

var _ application.NguongPort = (*nguongAdapter)(nil)

func nguongBatTinTu(cfg *opsconfig.Store) func() time.Duration {
	if cfg == nil {
		return nil
	}
	return func() time.Duration {
		return cfg.DocThoiLuong(opsconfig.KeyNguongBatTinGiaoHang)
	}
}

func (a *nguongAdapter) Nguong() domain.Nguong {
	return domain.Nguong{
		SLAGiaoHang: a.cfg.DocThoiLuong(opsconfig.KeySLAGiaoHang),
		HuyDon:      a.cfg.Doc(opsconfig.KeyNguongHuyDon),
		GiaoDungHan: a.cfg.Doc(opsconfig.KeyNguongGiaoDungHan),
		MauToiThieu: a.cfg.DocSoNguyen(opsconfig.KeyMauToiThieu),
	}
}

// hanDoiTraAdapter đọc hạn đổi trả từ cấu hình vận hành.
//
// Đọc MỖI lần chứ không chụp lúc khởi động: đổi tham số phải có tác dụng
// ở lượt chạy kế tiếp của job hoàn tất đơn.
type hanDoiTraAdapter struct{ cfg *opsconfig.Store }

var _ application.HanDoiTraPort = (*hanDoiTraAdapter)(nil)

func (a *hanDoiTraAdapter) HanDoiTra() time.Duration {
	return a.cfg.DocThoiLuong(opsconfig.KeyHanDoiTra)
}

// bieuPhiAdapter đọc biểu phí vận chuyển từ cấu hình vận hành.
//
// ĐỌC MỖI LẦN: phí là giá hiện trên màn hình thanh toán, và số ngày là lời
// hứa giao hàng. Chụp lúc khởi động nghĩa là đổi giá phải chờ triển khai.
type bieuPhiAdapter struct{ cfg *opsconfig.Store }

var _ application.BieuPhiPort = (*bieuPhiAdapter)(nil)

func (a *bieuPhiAdapter) BieuPhi() domain.BieuPhiGiao {
	return domain.BieuPhiGiao{
		domain.GiaoTieuChuan: {
			PhiMotNguon:  int64(a.cfg.DocSoNguyen(opsconfig.KeyPhiGiaoTieuChuan)),
			SoNgayDuKien: a.cfg.DocSoNguyen(opsconfig.KeyNgayGiaoTieuChuan),
		},
		domain.GiaoNhanh: {
			PhiMotNguon:  int64(a.cfg.DocSoNguyen(opsconfig.KeyPhiGiaoNhanh)),
			SoNgayDuKien: a.cfg.DocSoNguyen(opsconfig.KeyNgayGiaoNhanh),
		},
	}
}

// bieuPhi trả biểu phí đang hiệu lực cho tầng module.
func (m *Module) bieuPhi() domain.BieuPhiGiao {
	if m.bieuPhiPort == nil {
		return domain.BieuPhiMacDinh
	}
	return m.bieuPhiPort.BieuPhi()
}
