package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

var (
	// ErrIntentSaiTrangThai: chuyển trạng thái không hợp lệ.
	ErrIntentSaiTrangThai = errors.New("payment: chuyển trạng thái intent không hợp lệ")

	// ErrSoTienKhongKhop: số tiền webhook báo KHÁC số tiền hệ thống chờ thu.
	//
	// Đây là lớp bảo vệ thứ ba của webhooks.yaml, và là lý do bảng
	// payment_intent tồn tại. Xem ADR-0017.
	ErrSoTienKhongKhop = errors.New("payment: số tiền không khớp ý định thanh toán")

	// ErrIntentKhongTim: không có intent nào cho đơn/mã này.
	ErrIntentKhongTim = errors.New("payment: không tìm thấy ý định thanh toán")

	// ErrPhuongThucKhongTraTruoc: COD (hoặc rỗng) không có intent.
	ErrPhuongThucKhongTraTruoc = errors.New("payment: phương thức này không trả trước")

	// ErrIntentTrungDon: đơn này đã có ý định thanh toán.
	//
	// Bên gọi nên coi là THÀNH CÔNG và đọc lại intent cũ: hai intent cho
	// một đơn nghĩa là thu tiền hai lần.
	ErrIntentTrungDon = errors.New("payment: đơn hàng này đã có ý định thanh toán")
)

// TrangThaiIntent là vòng đời của một ý định thanh toán.
//
//	REQUIRES_PAYMENT ──→ CAPTURED    webhook báo thành công, số tiền KHỚP
//	                 ──→ FAILED      webhook báo thất bại
//	                 ──→ CANCELLED   đơn bị hủy trước khi trả tiền
//
// CAPTURED là trạng thái CUỐI — tiền đã về. Không có đường đi ngược: hoàn
// tiền là một bản ghi KHÁC (`refund`), không phải một lần chuyển trạng
// thái lùi. Cùng nguyên tắc bất biến với sổ cái (ADR-0008).
type TrangThaiIntent string

const (
	IntentChoThanhToan TrangThaiIntent = "REQUIRES_PAYMENT"
	IntentDaThu        TrangThaiIntent = "CAPTURED"
	IntentThatBai      TrangThaiIntent = "FAILED"
	IntentDaHuy        TrangThaiIntent = "CANCELLED"
)

// PhuongThucTraTruoc cho biết phương thức này có cần intent không.
//
// COD KHÔNG: không có cuộc trao đổi nào với cổng thanh toán, nên không có
// gì để đối chiếu và không webhook nào sẽ tới. Tạo intent cho nó là dựng
// ra một hàng đợi rác làm mọi cảnh báo dựa trên tồn đọng thành vô dụng —
// và một cảnh báo luôn kêu thì không ai đọc. Xem ADR-0017 phần 2.
//
// Chuỗi RỖNG cũng không: đó là đơn tạo qua `placeOrder`, đường không nhận
// phương thức nào (P3-9).
func PhuongThucTraTruoc(phuongThuc string) bool {
	switch strings.TrimSpace(phuongThuc) {
	case "CARD", "BANK_TRANSFER", "E_WALLET":
		return true
	default:
		return false
	}
}

// PaymentIntent là SỐ TIỀN HỆ THỐNG CHỜ THU cho một đơn.
//
// Nó KHÔNG phải bút toán: sổ cái ghi tiền ĐÃ chuyển, còn bản ghi này ghi
// tiền CHƯA. Hai thứ khác nhau về bản chất, và trộn chúng lại sẽ đưa một
// con số chưa chắc chắn vào một cuốn sổ bất biến.
//
// Công dụng chính: là con số DUY NHẤT mà webhook thanh toán được đối chiếu
// vào. Chữ ký HMAC chứng minh thông điệp đến từ nhà cung cấp; nó không
// chứng minh con số bên trong đúng.
type PaymentIntent struct {
	id      ids.ID
	orderID ids.ID

	// amount ĐÓNG BĂNG từ tổng đơn lúc đặt — cùng con số khách đã thấy.
	amount money.Money

	phuongThuc string
	status     TrangThaiIntent

	// nhaCungCap và maNhaCungCap để trống cho tới khi có PSP thật.
	nhaCungCap   string
	maNhaCungCap string

	lyDoThatBai string

	capturedAt time.Time
	createdAt  time.Time
	updatedAt  time.Time
}

// NewPaymentIntentParams là dữ liệu tạo một ý định thanh toán.
type NewPaymentIntentParams struct {
	OrderID    ids.ID
	Amount     money.Money
	PhuongThuc string
	Now        time.Time
}

// NewPaymentIntent tạo ý định thanh toán ở trạng thái chờ.
func NewPaymentIntent(p NewPaymentIntentParams) (*PaymentIntent, error) {
	if p.OrderID.IsZero() {
		return nil, errors.New("payment: intent phải trỏ tới một đơn hàng")
	}
	if !PhuongThucTraTruoc(p.PhuongThuc) {
		return nil, ErrPhuongThucKhongTraTruoc
	}
	// Số tiền phải DƯƠNG. Một intent 0 đồng không có nghĩa, và nó sẽ khớp
	// với mọi webhook báo 0 — tức là vô hiệu hóa chính lớp bảo vệ này.
	if p.Amount.Amount() <= 0 {
		return nil, errors.New("payment: số tiền chờ thu phải lớn hơn 0")
	}

	id, err := ids.New(ids.PrefixPaymentIntent)
	if err != nil {
		return nil, err
	}

	return &PaymentIntent{
		id:         id,
		orderID:    p.OrderID,
		amount:     p.Amount,
		phuongThuc: strings.TrimSpace(p.PhuongThuc),
		status:     IntentChoThanhToan,
		createdAt:  p.Now,
		updatedAt:  p.Now,
	}, nil
}

// DoiChieuVaThu đối chiếu số tiền webhook báo rồi ghi nhận đã thu.
//
// ĐÂY LÀ LỚP BẢO VỆ THỨ BA của api/paths/webhooks.yaml, và toàn bộ lý do
// kiểu này tồn tại.
//
// So sánh TUYỆT ĐỐI, không có ngưỡng sai số và không làm tròn: tiền là số
// nguyên đơn vị nhỏ nhất (ADR-0008), nên "gần bằng" không phải khái niệm
// có thật — nó chỉ là chỗ cho một lỗi trốn qua. Đơn vị tiền tệ cũng phải
// khớp: 628000 VND và 628000 USD là hai con số cách nhau hai bậc độ lớn.
func (p *PaymentIntent) DoiChieuVaThu(
	soTien money.Money, nhaCungCap, maNhaCungCap string, now time.Time,
) error {
	if p.status == IntentDaThu {
		// ĐÃ thu rồi thì không phải lỗi: nhà cung cấp gửi trùng là hành vi
		// bình thường (yêu cầu 2 của webhooks.yaml). Nhưng vẫn phải đối
		// chiếu — một lần gửi lại với số tiền KHÁC là tín hiệu thật sự xấu.
		if !p.amount.Equal(soTien) {
			return ErrSoTienKhongKhop
		}
		return nil
	}
	if p.status != IntentChoThanhToan {
		return ErrIntentSaiTrangThai
	}
	if !p.amount.Equal(soTien) {
		return ErrSoTienKhongKhop
	}

	p.status = IntentDaThu
	p.nhaCungCap = strings.TrimSpace(nhaCungCap)
	p.maNhaCungCap = strings.TrimSpace(maNhaCungCap)
	p.capturedAt = now
	p.updatedAt = now
	return nil
}

// GhiThatBai ghi nhận nhà cung cấp báo thanh toán thất bại.
//
// KHÔNG đối chiếu số tiền ở đây: thất bại nghĩa là không có tiền nào
// chuyển, nên con số trong payload không có ý nghĩa gì để mà kiểm.
func (p *PaymentIntent) GhiThatBai(lyDo string, now time.Time) error {
	if p.status == IntentThatBai {
		return nil // gửi trùng
	}
	if p.status != IntentChoThanhToan {
		return ErrIntentSaiTrangThai
	}
	p.status = IntentThatBai
	p.lyDoThatBai = strings.TrimSpace(lyDo)
	p.updatedAt = now
	return nil
}

// Huy hủy ý định khi đơn bị hủy trước lúc trả tiền.
func (p *PaymentIntent) Huy(now time.Time) error {
	if p.status == IntentDaHuy {
		return nil
	}
	if p.status != IntentChoThanhToan {
		return ErrIntentSaiTrangThai
	}
	p.status = IntentDaHuy
	p.updatedAt = now
	return nil
}

func (p *PaymentIntent) ID() ids.ID              { return p.id }
func (p *PaymentIntent) OrderID() ids.ID         { return p.orderID }
func (p *PaymentIntent) Amount() money.Money     { return p.amount }
func (p *PaymentIntent) PhuongThuc() string      { return p.phuongThuc }
func (p *PaymentIntent) Status() TrangThaiIntent { return p.status }
func (p *PaymentIntent) NhaCungCap() string      { return p.nhaCungCap }
func (p *PaymentIntent) MaNhaCungCap() string    { return p.maNhaCungCap }
func (p *PaymentIntent) LyDoThatBai() string     { return p.lyDoThatBai }
func (p *PaymentIntent) CapturedAt() time.Time   { return p.capturedAt }
func (p *PaymentIntent) CreatedAt() time.Time    { return p.createdAt }
func (p *PaymentIntent) UpdatedAt() time.Time    { return p.updatedAt }
func (p *PaymentIntent) DaThu() bool             { return p.status == IntentDaThu }

// RestoreIntentParams dựng lại intent từ database.
type RestoreIntentParams struct {
	ID           ids.ID
	OrderID      ids.ID
	Amount       money.Money
	PhuongThuc   string
	Status       TrangThaiIntent
	NhaCungCap   string
	MaNhaCungCap string
	LyDoThatBai  string
	CapturedAt   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RestoreIntent dựng lại từ database, KHÔNG kiểm tra quy tắc nghiệp vụ.
func RestoreIntent(p RestoreIntentParams) *PaymentIntent {
	return &PaymentIntent{
		id:           p.ID,
		orderID:      p.OrderID,
		amount:       p.Amount,
		phuongThuc:   p.PhuongThuc,
		status:       p.Status,
		nhaCungCap:   p.NhaCungCap,
		maNhaCungCap: p.MaNhaCungCap,
		lyDoThatBai:  p.LyDoThatBai,
		capturedAt:   p.CapturedAt,
		createdAt:    p.CreatedAt,
		updatedAt:    p.UpdatedAt,
	}
}
