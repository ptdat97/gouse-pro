package payment

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/application"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

var (
	// ErrPhuongThucKhongTraTruoc: COD hoặc rỗng — không có intent.
	//
	// Bên gọi coi đây là "không có gì để làm", KHÔNG phải lỗi: đó là câu
	// trả lời đúng cho một đơn COD (ADR-0017 phần 2).
	ErrPhuongThucKhongTraTruoc = domain.ErrPhuongThucKhongTraTruoc

	// ErrSoTienKhongKhop: số tiền webhook báo KHÁC số hệ thống chờ thu.
	//
	// Tầng HTTP PHẢI dịch thành 422 và không xử lý gì thêm.
	ErrSoTienKhongKhop = domain.ErrSoTienKhongKhop

	// ErrIntentKhongTim: đơn này không có ý định thanh toán nào.
	//
	// Với webhook, điều đó nghĩa là nhà cung cấp báo về một đơn hệ thống
	// KHÔNG chờ thu tiền — đơn COD, đơn không tồn tại, hoặc môi trường
	// test gọi nhầm vào production. Cả ba đều không được xử lý.
	ErrIntentKhongTim = domain.ErrIntentKhongTim
)

// TaoIntent tạo bản ghi số tiền CHỜ THU cho một đơn trả trước.
//
// IDEMPOTENT theo đơn. Xem ADR-0017.
func (m *Module) TaoIntent(
	ctx context.Context, req TaoIntentRequest,
) (*IntentView, error) {
	orderID, err := ids.Parse(req.OrderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	soTien, err := money.New(req.Amount, money.Currency(req.Currency))
	if err != nil {
		return nil, err
	}

	p, err := m.svc.TaoIntent(ctx, application.TaoIntentInput{
		OrderID: orderID, Amount: soTien, PhuongThuc: req.Method,
	})
	if err != nil {
		return nil, err
	}
	return toIntentView(p), nil
}

// DoiChieuVaThu là LỚP BẢO VỆ THỨ BA của webhook thanh toán.
//
// Trả ErrSoTienKhongKhop khi số tiền lệch — tầng HTTP dịch thành 422 và
// KHÔNG xử lý gì thêm.
func (m *Module) DoiChieuVaThu(
	ctx context.Context, orderID string, soTien int64, currency string,
	nhaCungCap, maNhaCungCap string,
) (*IntentView, bool, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, false, ErrInvalidID
	}
	m2, err := money.New(soTien, money.Currency(currency))
	if err != nil {
		return nil, false, err
	}

	res, err := m.svc.DoiChieuVaThu(ctx, id, m2, nhaCungCap, maNhaCungCap)
	if err != nil {
		return nil, false, err
	}
	return toIntentView(res.Intent), res.DaThuTruocDo, nil
}

// GhiThatBai ghi nhận nhà cung cấp báo thanh toán thất bại.
func (m *Module) GhiThatBai(
	ctx context.Context, orderID, lyDo string,
) (*IntentView, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	p, err := m.svc.GhiThatBai(ctx, id, lyDo)
	if err != nil {
		return nil, err
	}
	return toIntentView(p), nil
}

// LayIntentTheoDon đọc ý định thanh toán của một đơn.
func (m *Module) LayIntentTheoDon(
	ctx context.Context, orderID string,
) (*IntentView, error) {
	id, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return nil, ErrInvalidID
	}
	p, err := m.svc.LayIntentTheoDon(ctx, id)
	if err != nil {
		return nil, err
	}
	return toIntentView(p), nil
}

func toIntentView(p *domain.PaymentIntent) *IntentView {
	v := &IntentView{
		ID:       p.ID().String(),
		OrderID:  p.OrderID().String(),
		Amount:   p.Amount().Amount(),
		Currency: string(p.Amount().Currency()),
		Method:   p.PhuongThuc(),
		Status:   string(p.Status()),
	}
	if t := p.CapturedAt(); !t.IsZero() {
		v.CapturedAt = t.UTC().Format(time.RFC3339)
	}
	return v
}

// LaPhuongThucKhongTraTruoc cho biết lỗi này là "COD, không cần intent".
//
// Bên gọi dùng nó để phân biệt "không có gì để làm" với "hỏng thật".
func LaPhuongThucKhongTraTruoc(err error) bool {
	return errors.Is(err, domain.ErrPhuongThucKhongTraTruoc)
}

// LechDoiSoatView là một bất nhất giữa ý định thanh toán và đơn hàng.
type LechDoiSoatView struct {
	IntentID     string
	OrderID      string
	SoTien       int64
	Currency     string
	CapturedAt   string
	TrangThaiDon string
}

// DoiSoatDaThu tìm đơn ĐÃ THU TIỀN mà trạng thái đơn chưa theo kịp.
//
// `trangThaiDon` do BÊN GỌI cấp — payment không gọi ngược order được.
// Xem `application.DoiSoatDaThu`.
func (m *Module) DoiSoatDaThu(
	ctx context.Context, tuMoc time.Time, limit int,
	trangThaiDon func(ctx context.Context, orderID string) (string, error),
) ([]LechDoiSoatView, error) {
	lech, err := m.svc.DoiSoatDaThu(ctx, tuMoc, limit, trangThaiDon)
	if err != nil {
		return nil, err
	}
	out := make([]LechDoiSoatView, 0, len(lech))
	for _, l := range lech {
		out = append(out, LechDoiSoatView{
			IntentID: l.IntentID, OrderID: l.OrderID,
			SoTien: l.SoTien, Currency: l.Currency,
			CapturedAt:   l.CapturedAt.UTC().Format(time.RFC3339),
			TrangThaiDon: l.TrangThaiDon,
		})
	}
	return out, nil
}

// DemIntentChoThuQuaHan đếm intent còn chờ thu và cũ hơn `truoc`.
func (m *Module) DemIntentChoThuQuaHan(ctx context.Context, truoc time.Time) (int, error) {
	return m.svc.DemChoThuQuaHan(ctx, truoc)
}
