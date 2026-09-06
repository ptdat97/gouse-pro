package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/payment/domain"
)

// ErrChuaNoiIntent: bản dựng này chưa nối kho ý định thanh toán.
//
// Báo lỗi rõ thay vì panic con trỏ nil: một bản dựng thiếu dây phải nói ra
// điều đó, không phải sập giữa một request thanh toán.
var ErrChuaNoiIntent = errors.New("payment: chưa nối kho ý định thanh toán")

// TaoIntentInput là dữ liệu tạo một ý định thanh toán.
type TaoIntentInput struct {
	OrderID    ids.ID
	Amount     money.Money
	PhuongThuc string
}

// TaoIntent tạo bản ghi số tiền CHỜ THU cho một đơn trả trước.
//
// IDEMPOTENT theo đơn: gọi lại trả về intent đã có thay vì tạo cái thứ
// hai. Hai intent cho một đơn nghĩa là thu tiền hai lần — và chỉ mục
// UNIQUE trên `order_id` là thứ cưỡng chế điều đó, không phải câu SELECT ở
// đây: hai request hoàn tất song song đều thấy "chưa có".
//
// Phương thức KHÔNG trả trước (COD, hoặc rỗng) trả ErrPhuongThucKhongTraTruoc
// — bên gọi bỏ qua chứ không coi là hỏng. Xem ADR-0017 phần 2.
func (s *Service) TaoIntent(
	ctx context.Context, in TaoIntentInput,
) (*domain.PaymentIntent, error) {
	if s.intents == nil {
		return nil, ErrChuaNoiIntent
	}

	p, err := domain.NewPaymentIntent(domain.NewPaymentIntentParams{
		OrderID:    in.OrderID,
		Amount:     in.Amount,
		PhuongThuc: in.PhuongThuc,
		Now:        s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}

	if err := s.intents.Create(ctx, p); err != nil {
		if errors.Is(err, domain.ErrIntentTrungDon) {
			// Đơn đã có intent: đọc lại cái cũ. Đây là đường đi BÌNH
			// THƯỜNG của một lần thử lại, không phải tình huống bất thường.
			return s.intents.FindByOrder(ctx, in.OrderID)
		}
		return nil, err
	}
	return p, nil
}

// KetQuaThu là kết quả xử lý một webhook thanh toán.
type KetQuaThu struct {
	Intent *domain.PaymentIntent

	// DaThuTruocDo = true khi intent đã CAPTURED từ một webhook trước.
	//
	// Bên gọi PHẢI kiểm cờ này trước khi làm tác dụng phụ: nhà cung cấp
	// gửi trùng là hành vi bình thường, và đánh dấu đơn đã trả tiền hai
	// lần thì lần thứ hai gặp máy trạng thái từ chối — một lỗi giả.
	DaThuTruocDo bool
}

// DoiChieuVaThu đối chiếu số tiền webhook báo rồi ghi nhận đã thu.
//
// ĐÂY LÀ LỚP BẢO VỆ THỨ BA của api/paths/webhooks.yaml. Hai lớp trước —
// chữ ký HMAC và idempotency theo event_id — nằm ở tầng HTTP; lớp này phải
// ở đây vì nó cần biết hệ thống CHỜ THU bao nhiêu.
//
// Trả domain.ErrSoTienKhongKhop khi lệch. Bên gọi PHẢI dịch nó thành 422
// và KHÔNG xử lý gì thêm — xem ADR-0017 phần 4.
func (s *Service) DoiChieuVaThu(
	ctx context.Context, orderID ids.ID, soTien money.Money,
	nhaCungCap, maNhaCungCap string,
) (*KetQuaThu, error) {
	if s.intents == nil {
		return nil, ErrChuaNoiIntent
	}

	p, err := s.intents.FindByOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}

	daThuTruoc := p.DaThu()

	if err := p.DoiChieuVaThu(soTien, nhaCungCap, maNhaCungCap, s.clock.Now()); err != nil {
		return nil, err
	}

	// Đã thu từ trước thì KHÔNG ghi lại: bản ghi không đổi gì, và ghi đè
	// sẽ đẩy `updated_at` tới trước mỗi lần nhà cung cấp gửi lại — làm
	// mọi báo cáo "thu tiền lúc nào" nói sai.
	if !daThuTruoc {
		if err := s.intents.Update(ctx, p); err != nil {
			return nil, fmt.Errorf("ghi nhận đã thu: %w", err)
		}
	}

	return &KetQuaThu{Intent: p, DaThuTruocDo: daThuTruoc}, nil
}

// GhiThatBai ghi nhận nhà cung cấp báo thanh toán thất bại.
func (s *Service) GhiThatBai(
	ctx context.Context, orderID ids.ID, lyDo string,
) (*domain.PaymentIntent, error) {
	if s.intents == nil {
		return nil, ErrChuaNoiIntent
	}

	p, err := s.intents.FindByOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if err := p.GhiThatBai(lyDo, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.intents.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("ghi nhận thất bại: %w", err)
	}
	return p, nil
}

// LayIntentTheoDon đọc ý định thanh toán của một đơn.
func (s *Service) LayIntentTheoDon(
	ctx context.Context, orderID ids.ID,
) (*domain.PaymentIntent, error) {
	if s.intents == nil {
		return nil, ErrChuaNoiIntent
	}
	return s.intents.FindByOrder(ctx, orderID)
}

// LechDoiSoat là một bất nhất giữa ý định thanh toán và đơn hàng.
type LechDoiSoat struct {
	IntentID   string
	OrderID    string
	SoTien     int64
	Currency   string
	CapturedAt time.Time

	// TrangThaiDon là trạng thái đơn ĐỌC ĐƯỢC lúc đối soát.
	TrangThaiDon string
}

// TrangThaiDonFunc trả trạng thái hiện tại của một đơn.
//
// Truyền vào từ BÊN GỌI thay vì để payment tự hỏi order: chiều phụ thuộc
// đã là order → payment, và gọi ngược tạo phụ thuộc vòng. Tiến trình
// worker là nơi biết cả hai module.
type TrangThaiDonFunc func(ctx context.Context, orderID string) (string, error)

// DoiSoatDaThu tìm các đơn ĐÃ THU TIỀN mà trạng thái đơn chưa theo kịp.
//
// # Đây là đối soát NỘI BỘ, không phải đối chiếu với nhà cung cấp
//
// Yêu cầu 5 của `api/paths/webhooks.yaml` là đi hỏi PSP để phát hiện
// webhook BỊ MẤT. Việc đó cần adapter PSP thật, thứ chưa có (ADR-0017).
//
// Cái làm được ngay — và là lỗ hổng đã biết chứ không phải giả định — là
// đối soát hai nguồn NỘI BỘ với nhau. Handler webhook, khi thu tiền xong
// mà `MarkOrderPaid` hỏng, cố ý KHÔNG quay ngược intent: tiền về là sự
// thật đã xảy ra. Nó ghi log rồi đi tiếp, và để lại đúng trạng thái này.
//
// # Vì sao bất nhất ở đây là tín hiệu SẠCH
//
// Không có ca hợp lệ nào cho "intent CAPTURED mà đơn vẫn PENDING_PAYMENT":
// tiền đã vào tài khoản mà khách vẫn thấy đơn chờ thanh toán. Nên cảnh báo
// này không bao giờ kêu oan — khác hẳn "intent chờ thu quá hạn", thứ phần
// lớn chỉ là khách bỏ giữa chừng.
func (s *Service) DoiSoatDaThu(
	ctx context.Context, tuMoc time.Time, limit int, trangThai TrangThaiDonFunc,
) ([]LechDoiSoat, error) {
	if s.intents == nil {
		return nil, ErrChuaNoiIntent
	}

	daThu, err := s.intents.DaThuTuMoc(ctx, tuMoc, limit)
	if err != nil {
		return nil, err
	}

	var lech []LechDoiSoat
	for _, p := range daThu {
		tt, err := trangThai(ctx, p.OrderID().String())
		if err != nil {
			// Đơn tra không ra là bất nhất NẶNG HƠN, không phải lý do bỏ
			// qua: có tiền thu cho một mã đơn không đọc được.
			lech = append(lech, LechDoiSoat{
				IntentID: p.ID().String(), OrderID: p.OrderID().String(),
				SoTien: p.Amount().Amount(), Currency: string(p.Amount().Currency()),
				CapturedAt: p.CapturedAt(), TrangThaiDon: "KHÔNG ĐỌC ĐƯỢC: " + err.Error(),
			})
			continue
		}
		if tt != "PENDING_PAYMENT" {
			continue // đơn đã đi tiếp — đúng như mong đợi
		}
		lech = append(lech, LechDoiSoat{
			IntentID: p.ID().String(), OrderID: p.OrderID().String(),
			SoTien: p.Amount().Amount(), Currency: string(p.Amount().Currency()),
			CapturedAt: p.CapturedAt(), TrangThaiDon: tt,
		})
	}
	return lech, nil
}

// DemChoThuQuaHan đếm intent còn chờ thu và cũ hơn `truoc`.
func (s *Service) DemChoThuQuaHan(ctx context.Context, truoc time.Time) (int, error) {
	if s.intents == nil {
		return 0, ErrChuaNoiIntent
	}
	return s.intents.DemChoThuQuaHan(ctx, truoc)
}
