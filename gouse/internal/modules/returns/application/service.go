// Package application là các use case của việc trả hàng.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/returns/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// SystemClock là đồng hồ thật.
var SystemClock Clock = systemClock{}

// DongDonHang là một dòng hàng trong đơn, dưới góc nhìn của việc trả hàng.
type DongDonHang struct {
	ID       ids.ID
	SKUID    ids.ID
	SellerID ids.ID
	Quantity int

	// LineTotal là giá NIÊM YẾT của dòng: đơn giá × số lượng.
	LineTotal money.Money

	// TongDieuChinh là tổng các khoản cộng/trừ đã phân bổ cho dòng này.
	// Âm với khoản giảm giá.
	TongDieuChinh money.Money

	// HoaHong là hoa hồng nền tảng của dòng, ĐÃ ĐÓNG BĂNG lúc đặt hàng.
	//
	// Cần để ĐẢO NGƯỢC đúng phần: hoàn tiền cho khách mà không đảo hoa
	// hồng nghĩa là nền tảng giữ lại hoa hồng của một đơn không thành.
	HoaHong money.Money
}

// DonHang là dữ liệu đơn hàng mà việc trả hàng cần.
type DonHang struct {
	ID ids.ID

	// DaGiao: chỉ đơn ĐÃ GIAO mới trả được.
	DaGiao bool

	// GiamGiaCapDon là số tiền giảm ghi ở CẤP ĐƠN.
	//
	// Nếu nó khác 0 mà không dòng nào mang khoản điều chỉnh tương ứng thì
	// phần giảm chưa được phân bổ — xem hàng rào ở TinhTienHoan.
	GiamGiaCapDon money.Money

	Dong []DongDonHang
}

// OrderPort là cổng đọc đơn hàng.
//
// PORT do returns định nghĩa: nó hỏi đúng thứ mình cần, không phải toàn bộ
// góc nhìn đơn hàng của module order.
type OrderPort interface {
	LayDonDeTraHang(ctx context.Context, orderID ids.ID) (DonHang, error)
}

// InventoryPort nhận hàng hoàn về kho.
type InventoryPort interface {
	// NhanHangHoan đưa hàng vào trạng thái Returned — KHÔNG phải Available.
	//
	// Bên cài đặt tự tra bản ghi tồn kho từ (SKU, nhà bán): việc trả hàng
	// không cần biết kho nào giữ món đó.
	NhanHangHoan(ctx context.Context, skuID, sellerID ids.ID, qty int, returnID ids.ID) error
}

// PaymentPort ghi bút toán hoàn tiền.
type PaymentPort interface {
	// GhiHoanTien đảo ngược phần tiền của một lần trả hàng.
	//
	// Ba con số, không phải một: tổng hoàn cho khách, phần thu lại từ nhà
	// bán, và phần thu lại từ doanh thu nền tảng. Chỉ ghi tổng thì sổ vẫn
	// cân nhưng số dư nhà bán sai — và số dư ấy là thứ đem đi chi trả.
	GhiHoanTien(ctx context.Context, in HoanTienInput) error
}

// HoanTienInput là dữ liệu đảo ngược tài chính của một lần trả hàng.
type HoanTienInput struct {
	OrderID  ids.ID
	ReturnID ids.ID
	SellerID ids.ID

	// TongHoan là số tiền trả lại khách, theo GIÁ THỰC TRẢ.
	TongHoan money.Money

	// DaoHoaHong là phần thu lại từ doanh thu nền tảng.
	DaoHoaHong money.Money

	// DaoNhaBan là phần thu lại từ số dư nhà bán = TongHoan − DaoHoaHong.
	DaoNhaBan money.Money
}

// Repository là PORT cho kho lưu trữ.
type Repository interface {
	Luu(ctx context.Context, y *domain.YeuCauTraHang) error
	TimTheoID(ctx context.Context, id ids.ID) (*domain.YeuCauTraHang, error)
	TimTheoDon(ctx context.Context, orderID ids.ID) ([]*domain.YeuCauTraHang, error)
	TimTheoNhaBan(ctx context.Context, sellerID ids.ID, status string, limit int) ([]*domain.YeuCauTraHang, error)

	// DongDaXinTra trả các order_line_id đã có yêu cầu trả CÒN HIỆU LỰC.
	//
	// Không có bước này, khách gửi hai yêu cầu cho cùng một món và được
	// hoàn tiền hai lần cho một lần mua.
	DongDaXinTra(ctx context.Context, orderID ids.ID) (map[ids.ID]int, error)
}

// Service là tầng application của module returns.
type Service struct {
	repo      Repository
	orders    OrderPort
	inventory InventoryPort
	payment   PaymentPort
	clock     Clock
}

type Deps struct {
	Repo      Repository
	Orders    OrderPort
	Inventory InventoryPort
	Payment   PaymentPort
	Clock     Clock
}

func NewService(d Deps) *Service {
	c := d.Clock
	if c == nil {
		c = SystemClock
	}
	return &Service{repo: d.Repo, orders: d.Orders,
		inventory: d.Inventory, payment: d.Payment, clock: c}
}

// TinhTienHoan tính số tiền hoàn cho một dòng, theo GIÁ THỰC TRẢ.
//
// # Hàng rào chống hoàn thừa
//
// docs/07-workflows/return.md mục 5 gọi đây là điểm dễ sai nhất của cả
// luồng: hoàn theo giá niêm yết thay vì giá thực trả làm nền tảng trả ra
// nhiều hơn đã thu vào.
//
// Ví dụ của tài liệu: đơn 500.000đ giảm 50.000đ. Món A niêm yết 200.000đ
// nhưng khách chỉ trả 180.000đ. Hoàn 200.000đ là mất 20.000đ mỗi lần.
//
// Hệ thống HIỆN TẠI có `checkout.ApplyCoupon` đặt giảm giá ở cấp đơn,
// nhưng KHÔNG gì phân bổ nó xuống dòng — `promotion.AllocateDiscount` tồn
// tại và không ai gọi. Nên hàm này TỪ CHỐI khi thấy giảm giá cấp đơn mà
// dòng không mang khoản điều chỉnh nào.
//
// Từ chối và bắt xử lý tay còn hơn âm thầm trả ra số tiền sai.
func TinhTienHoan(don DonHang, dong DongDonHang, soLuongTra int) (money.Money, error) {
	if soLuongTra <= 0 || soLuongTra > dong.Quantity {
		return money.Money{}, domain.ErrQuantityExceeded
	}

	if don.GiamGiaCapDon.IsPositive() && !coDieuChinh(don) {
		return money.Money{}, domain.ErrGiamGiaChuaPhanBo
	}

	// Giá thực trả của cả dòng = niêm yết + điều chỉnh (điều chỉnh âm khi
	// là khoản giảm).
	thucTra, err := dong.LineTotal.Add(dong.TongDieuChinh)
	if err != nil {
		return money.Money{}, err
	}
	if thucTra.Amount() < 0 {
		return money.Money{}, fmt.Errorf(
			"returns: giá thực trả của dòng %s là số âm", dong.ID)
	}

	if soLuongTra == dong.Quantity {
		return thucTra, nil
	}

	// Trả một phần: chia theo tỷ lệ, LÀM TRÒN XUỐNG.
	//
	// Làm tròn xuống là chủ ý — làm tròn lên nghĩa là trả nhiều hơn đã
	// thu, và phần chênh nhân với số lần trả hàng của cả nền tảng.
	moiCai := thucTra.Amount() / int64(dong.Quantity)
	return money.New(moiCai*int64(soLuongTra), thucTra.Currency())
}

// coDieuChinh cho biết CÓ ÍT NHẤT một dòng mang khoản điều chỉnh.
func coDieuChinh(don DonHang) bool {
	for _, d := range don.Dong {
		if !d.TongDieuChinh.IsZero() {
			return true
		}
	}
	return false
}

// XinTraInput là yêu cầu trả hàng của khách.
type XinTraInput struct {
	OrderID    ids.ID
	CustomerID ids.ID
	LyDo       domain.LyDo
	GhiChu     string

	// Dong là các dòng xin trả: mã dòng hàng → số lượng.
	Dong map[ids.ID]int
}

// XinTra tạo một yêu cầu trả hàng.
func (s *Service) XinTra(
	ctx context.Context, in XinTraInput,
) (*domain.YeuCauTraHang, error) {
	if len(in.Dong) == 0 {
		return nil, domain.ErrNoLines
	}

	don, err := s.orders.LayDonDeTraHang(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}
	if !don.DaGiao {
		return nil, fmt.Errorf("%w: đơn chưa giao xong", domain.ErrInvalidStatus)
	}

	daXin, err := s.repo.DongDaXinTra(ctx, in.OrderID)
	if err != nil {
		return nil, err
	}

	theoID := map[ids.ID]DongDonHang{}
	for _, d := range don.Dong {
		theoID[d.ID] = d
	}

	var dongTra []domain.Dong
	var sellerID ids.ID
	for lineID, qty := range in.Dong {
		d, co := theoID[lineID]
		if !co {
			return nil, fmt.Errorf("%w: dòng %s không thuộc đơn này",
				domain.ErrNotFound, lineID)
		}
		if daXin[lineID]+qty > d.Quantity {
			return nil, domain.ErrDuplicateLine
		}

		tien, err := TinhTienHoan(don, d, qty)
		if err != nil {
			return nil, err
		}

		// MỘT yêu cầu chỉ thuộc MỘT nhà bán: người duyệt là nhà bán, và
		// một yêu cầu trải hai gian hàng thì không ai duyệt được trọn vẹn.
		if sellerID.IsZero() {
			sellerID = d.SellerID
		} else if sellerID != d.SellerID {
			return nil, fmt.Errorf(
				"returns: một yêu cầu chỉ được chứa hàng của MỘT nhà bán")
		}

		dongTra = append(dongTra, domain.Dong{
			ID:          ids.MustNew(ids.PrefixReturnLine),
			OrderLineID: lineID,
			SKUID:       d.SKUID,
			Quantity:    qty,
			TienHoan:    tien,
		})
	}

	y, err := domain.Tao(domain.TaoParams{
		OrderID: in.OrderID, SellerID: sellerID, CustomerID: in.CustomerID,
		LyDo: in.LyDo, GhiChu: in.GhiChu, Dong: dongTra, Now: s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.Luu(ctx, y); err != nil {
		return nil, err
	}
	return y, nil
}

// Duyet nhà bán chấp nhận yêu cầu.
func (s *Service) Duyet(ctx context.Context, id, sellerID ids.ID) (*domain.YeuCauTraHang, error) {
	return s.doi(ctx, id, sellerID, func(y *domain.YeuCauTraHang, now time.Time) error {
		return y.Duyet(now)
	})
}

// TuChoi nhà bán từ chối yêu cầu, kèm lý do.
func (s *Service) TuChoi(
	ctx context.Context, id, sellerID ids.ID, lyDo string,
) (*domain.YeuCauTraHang, error) {
	return s.doi(ctx, id, sellerID, func(y *domain.YeuCauTraHang, now time.Time) error {
		return y.TuChoi(lyDo, now)
	})
}

// NhanHangVaHoanTien ghi nhận hàng về kho rồi hoàn tiền.
//
// # Thứ tự quan trọng
//
// Nhận hàng TRƯỚC, hoàn tiền SAU. Hoàn tiền trước rồi nhận hàng thất bại
// nghĩa là đã trả tiền cho món hàng chưa bao giờ về.
//
// Hàng vào trạng thái Returned, KHÔNG phải Available. Quy tắc bắt buộc của
// docs/07-workflows/return.md mục 4: hàng hoàn tự động thành Available
// nghĩa là bán lại hàng hỏng cho khách khác.
func (s *Service) NhanHangVaHoanTien(
	ctx context.Context, id, sellerID ids.ID,
) (*domain.YeuCauTraHang, error) {
	y, err := s.doi(ctx, id, sellerID, func(y *domain.YeuCauTraHang, now time.Time) error {
		return y.DaNhanHang(now)
	})
	if err != nil {
		return nil, err
	}

	for _, d := range y.Dong() {
		if err := s.inventory.NhanHangHoan(ctx, d.SKUID, y.SellerID(), d.Quantity, y.ID()); err != nil {
			return nil, fmt.Errorf("returns: nhập hàng hoàn về kho: %w", err)
		}
	}

	daoHoaHong, err := s.tinhDaoHoaHong(ctx, y)
	if err != nil {
		return nil, err
	}
	daoNhaBan, err := y.TienHoan().Sub(daoHoaHong)
	if err != nil {
		return nil, err
	}

	if err := s.payment.GhiHoanTien(ctx, HoanTienInput{
		OrderID: y.OrderID(), ReturnID: y.ID(), SellerID: y.SellerID(),
		TongHoan: y.TienHoan(), DaoHoaHong: daoHoaHong, DaoNhaBan: daoNhaBan,
	}); err != nil {
		return nil, fmt.Errorf("returns: ghi sổ hoàn tiền: %w", err)
	}

	now := s.clock.Now()
	if err := y.DaHoanTien(now); err != nil {
		return nil, err
	}
	if err := s.repo.Luu(ctx, y); err != nil {
		return nil, err
	}
	return y, nil
}

// tinhDaoHoaHong tính phần hoa hồng phải thu lại.
//
// Theo TỶ LỆ phần đã hoàn trên tổng dòng: trả một nửa số lượng thì đảo một
// nửa hoa hồng. Làm tròn XUỐNG, cùng lý do với tiền hoàn — làm tròn lên ở
// đây nghĩa là thu lại của nhà bán nhiều hơn phần họ thực sự mất.
func (s *Service) tinhDaoHoaHong(
	ctx context.Context, y *domain.YeuCauTraHang,
) (money.Money, error) {
	don, err := s.orders.LayDonDeTraHang(ctx, y.OrderID())
	if err != nil {
		return money.Money{}, err
	}

	theoID := map[ids.ID]DongDonHang{}
	for _, d := range don.Dong {
		theoID[d.ID] = d
	}

	tong := money.Money{}
	for _, d := range y.Dong() {
		goc, co := theoID[d.OrderLineID]
		if !co || goc.Quantity == 0 || goc.HoaHong.IsZero() {
			continue
		}
		phan := goc.HoaHong.Amount() * int64(d.Quantity) / int64(goc.Quantity)
		m, err := money.New(phan, goc.HoaHong.Currency())
		if err != nil {
			return money.Money{}, err
		}
		if tong.IsZero() {
			tong = m
			continue
		}
		if tong, err = tong.Add(m); err != nil {
			return money.Money{}, err
		}
	}
	if tong.IsZero() {
		return money.New(0, y.TienHoan().Currency())
	}
	return tong, nil
}

// doi đọc–sửa–ghi một yêu cầu, kèm kiểm CHỦ SỞ HỮU.
func (s *Service) doi(
	ctx context.Context, id, sellerID ids.ID,
	buoc func(*domain.YeuCauTraHang, time.Time) error,
) (*domain.YeuCauTraHang, error) {
	y, err := s.repo.TimTheoID(ctx, id)
	if err != nil {
		return nil, err
	}
	// KHÔNG phân biệt "không tồn tại" với "của nhà bán khác": phân biệt
	// hai trường hợp cho phép dò mã yêu cầu của đối thủ.
	if !sellerID.IsZero() && y.SellerID() != sellerID {
		return nil, domain.ErrNotFound
	}
	if err := buoc(y, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.repo.Luu(ctx, y); err != nil {
		return nil, err
	}
	return y, nil
}

// DanhSachTheoDon trả các yêu cầu trả hàng của một đơn.
func (s *Service) DanhSachTheoDon(ctx context.Context, orderID ids.ID) ([]*domain.YeuCauTraHang, error) {
	return s.repo.TimTheoDon(ctx, orderID)
}

// DanhSachTheoNhaBan trả các yêu cầu của một gian hàng.
func (s *Service) DanhSachTheoNhaBan(
	ctx context.Context, sellerID ids.ID, status string, limit int,
) ([]*domain.YeuCauTraHang, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.TimTheoNhaBan(ctx, sellerID, status, limit)
}
