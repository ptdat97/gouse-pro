// Package application là các trường hợp sử dụng của module affiliate.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/affiliate/domain"
)

// DongDon là một dòng hàng của đơn, dưới dạng DỮ LIỆU THUẦN.
//
// # Vì sao không dùng kiểu của module order
//
// affiliate là EXTENSION và order là KERNEL (ADR-0021). Chiều extension →
// kernel là chiều được phép, nhưng nhận kiểu của order vào tận tầng
// application nghĩa là mỗi lần order đổi hình dạng view thì affiliate phải
// biên dịch lại. Dữ liệu thuần giữ cho việc dịch nằm gọn ở `module.go` —
// cùng cách module order nhận `FulfillmentProgressInput`.
type DongDon struct {
	OrderLineID string
	SellerID    string

	// CreatorID rỗng = dòng này khách tự tìm, KHÔNG quy công.
	CreatorID string

	// TienHang là đơn giá × số lượng, CHƯA trừ khuyến mãi.
	TienHang int64

	// KhuyenMai là các khoản giảm ĐÃ PHÂN BỔ cho dòng này, số ÂM.
	//
	// Chỉ khoản loại khuyến mãi: thuế và phí vận chuyển không phải doanh
	// thu hàng hóa — xem `domain.TienGocTinhHoaHong`.
	KhuyenMai []int64

	Currency string

	// DaHuy: dòng đã bị hủy hoặc đã hoàn hàng.
	//
	// KHÔNG quy công. Một đơn có thể được thanh toán rồi hủy từng phần
	// trước khi worker xử lý `order.paid`, và quy công cho một dòng đã hủy
	// là tạo ra một khoản phải đảo ngay lập tức.
	DaHuy bool
}

// DonPort đọc dòng hàng của một đơn.
//
// Đọc NGƯỢC thay vì nhận qua payload event, vì `order.paid` chỉ mang tổng
// tiền — không mang dòng hàng, và cũng không nên mang: theo ADR-0016, thêm
// trường vào một event đang chạy bắt MỌI bên nhận của nó khai lại
// `MaxEventVersion`, tức sửa hai module không liên quan tới hoa hồng.
type DonPort interface {
	// DongDonQuyCong trả các dòng của đơn kèm creator đã quy công.
	//
	// Trả cả dòng KHÔNG có creator: bên gọi cần biết đơn có dòng hay
	// không để phân biệt "đơn không có gì để quy công" với "không đọc
	// được đơn".
	DongDonQuyCong(ctx context.Context, orderID string) ([]DongDon, error)
}

// HanDoiTraPort cho biết một cặp (đơn, nhà bán) đã hết hạn đổi trả chưa.
//
// Chưa dùng ở phiên bản này: tín hiệu tới bằng EVENT
// `fulfillment_order.completed`, và event ấy chính là câu trả lời. Khai ra
// một cổng chưa ai gọi là đúng dạng lỗi `cmd/eventcheck` đang gác, nên nó
// KHÔNG được khai ở đây. Đoạn văn này ở lại để lần sau không ai khai nó.

type Deps struct {
	Repo domain.Repo
	Don  DonPort
	Log  *slog.Logger

	// Clock để test điều khiển thời gian. Nil = time.Now().UTC().
	Clock func() time.Time

	// TyLe là tỷ lệ hoa hồng creator. 0 = dùng domain.TyLeMacDinh.
	//
	// Bày ra để module `campaign` (Phase 2) nối vào, và để test khẳng định
	// tỷ lệ được ĐÓNG BĂNG chứ không đọc lại lúc tính.
	TyLe int
}

type Service struct {
	repo  domain.Repo
	don   DonPort
	log   *slog.Logger
	clock func() time.Time
	tyLe  types.BasisPoints
}

func NewService(d Deps) (*Service, error) {
	if d.Repo == nil {
		return nil, errors.New("affiliate: bắt buộc phải có kho lưu trữ")
	}
	if d.Don == nil {
		return nil, errors.New(
			"affiliate: bắt buộc phải có cổng đọc đơn — không có nó thì " +
				"`order.paid` tới mà không có dòng hàng nào để quy công")
	}

	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	clock := d.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}

	bp := d.TyLe
	if bp == 0 {
		bp = domain.TyLeMacDinh
	}
	tyLe, err := types.NewBasisPoints(int32(bp))
	if err != nil {
		return nil, fmt.Errorf("affiliate: tỷ lệ hoa hồng %d: %w", bp, err)
	}

	return &Service{repo: d.Repo, don: d.Don, log: log, clock: clock, tyLe: tyLe}, nil
}

// KetQuaQuyCong là số dòng đã xử lý của một lượt quy công.
type KetQuaQuyCong struct {
	DaQuyCong int
	DaBoQua   int
}

// QuyCongDon quy công mọi dòng có creator của một đơn.
//
// Gọi khi `order.paid` tới: quy công lúc TIỀN VỀ, không lúc đặt đơn. Đơn
// chưa trả tiền có thể không bao giờ trả, và một khoản hoa hồng cho doanh
// thu chưa có là một con số phải dọn về sau.
//
// IDEMPOTENT theo từng dòng: `order.paid` giao ít nhất một lần, nên lượt
// thứ hai thấy `ErrDaQuyCong` ở mọi dòng và trả về thành công.
func (s *Service) QuyCongDon(ctx context.Context, orderID string) (KetQuaQuyCong, error) {
	var kq KetQuaQuyCong

	oid, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return kq, fmt.Errorf("affiliate: mã đơn %q: %w", orderID, err)
	}

	dong, err := s.don.DongDonQuyCong(ctx, orderID)
	if err != nil {
		return kq, fmt.Errorf("affiliate: đọc dòng đơn %s: %w", orderID, err)
	}

	now := s.clock()
	for _, d := range dong {
		if d.CreatorID == "" || d.DaHuy {
			kq.DaBoQua++
			continue
		}

		if err := s.quyCongMotDong(ctx, oid, d, now); err != nil {
			if errors.Is(err, domain.ErrDaQuyCong) {
				kq.DaBoQua++
				continue
			}
			return kq, err
		}
		kq.DaQuyCong++
	}

	return kq, nil
}

func (s *Service) quyCongMotDong(
	ctx context.Context, orderID ids.ID, d DongDon, now time.Time,
) error {
	lineID, err := ids.Parse(d.OrderLineID, ids.PrefixOrderLine)
	if err != nil {
		return fmt.Errorf("affiliate: mã dòng đơn %q: %w", d.OrderLineID, err)
	}
	creatorID, err := ids.Parse(d.CreatorID, ids.PrefixCreator)
	if err != nil {
		return fmt.Errorf("affiliate: mã creator %q: %w", d.CreatorID, err)
	}
	sellerID, err := ids.Parse(d.SellerID, ids.PrefixSeller)
	if err != nil {
		return fmt.Errorf("affiliate: mã nhà bán %q: %w", d.SellerID, err)
	}

	// money.New kiểm và chuẩn hóa mã tiền, nên một chuỗi rỗng hay "vnd"
	// bị chặn ở đây thay vì thành một con số tiền không có đơn vị.
	cur := money.Currency(d.Currency)
	tienHang, err := money.New(d.TienHang, cur)
	if err != nil {
		return fmt.Errorf("affiliate: tiền hàng của dòng %s (%q): %w",
			d.OrderLineID, d.Currency, err)
	}

	km := make([]money.Money, 0, len(d.KhuyenMai))
	for _, v := range d.KhuyenMai {
		m, err := money.New(v, cur)
		if err != nil {
			return fmt.Errorf("affiliate: khoản khuyến mãi của dòng %s: %w",
				d.OrderLineID, err)
		}
		km = append(km, m)
	}

	a, err := domain.TaoAttribution(domain.TaoAttributionParams{
		OrderID:     orderID,
		OrderLineID: lineID,
		SellerID:    sellerID,
		CreatorID:   creatorID,
		Rate:        s.tyLe,
		Base:        domain.TienGocTinhHoaHong(tienHang, km),
		BenChiu:     domain.BenChiuMacDinh,
		Now:         now,
	})
	if err != nil {
		return err
	}

	c, err := domain.TaoHoaHong(a, now)
	if err != nil {
		return err
	}

	return s.repo.Luu(ctx, a, c)
}

// DaoTheoDon đảo mọi khoản hoa hồng của một đơn.
//
// Gọi khi `order.cancelled` tới. `lyDo` BẮT BUỘC và được ghi vào từng bản
// ghi: một khoản tiền biến mất mà không nêu lý do là điểm mù trong kiểm
// toán.
//
// IDEMPOTENT: `domain.Dao` im lặng thành công với khoản đã đảo.
func (s *Service) DaoTheoDon(ctx context.Context, orderID, lyDo string) (int, error) {
	if lyDo == "" {
		return 0, domain.ErrThieuLyDoDao
	}

	oid, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return 0, fmt.Errorf("affiliate: mã đơn %q: %w", orderID, err)
	}

	ghi, err := s.repo.TheoDonVaNhaBan(ctx, oid, "")
	if err != nil {
		return 0, err
	}

	return s.dao(ctx, ghi, lyDo)
}

// DaoTheoDongDon đảo hoa hồng của những DÒNG ĐƠN cụ thể.
//
// # Vì sao cần đường này bên cạnh `DaoTheoDon`
//
// Hủy TỪNG PHẦN — một nhà bán trong đơn nhiều nhà bán không giao được —
// KHÔNG phát `order.cancelled`. Nó phát `fulfillment.cancelled`, và đơn chỉ
// chuyển sang `PARTIALLY_CANCELLED`.
//
// Trước 01/10/2026 không đường nào tới đây, nên khoản hoa hồng của dòng bị
// hủy đứng ở CHỜ ĐẾN HẠN **mãi mãi**: không đảo, vì không có
// `order.cancelled`; không mở khóa, vì đơn thực hiện bị hủy không bao giờ
// phát `fulfillment_order.completed`. Một dòng rác trong bảng tiền, và nó
// im lặng. Xem P3-85.
//
// Dùng `DaoTheoDon` cho việc này là SAI: nó đảo theo `order_id`, tức đảo
// luôn hoa hồng của nhà bán vẫn đang giao bình thường.
//
// Danh sách RỖNG trả 0 và không lỗi: đơn thực hiện cũ có `order_line_id`
// rỗng (điền từ migration 000024), và việc ấy không phải lỗi cần thử lại.
func (s *Service) DaoTheoDongDon(
	ctx context.Context, orderLineIDs []string, lyDo string,
) (int, error) {
	if lyDo == "" {
		return 0, domain.ErrThieuLyDoDao
	}
	if len(orderLineIDs) == 0 {
		return 0, nil
	}

	ma := make([]ids.ID, 0, len(orderLineIDs))
	for _, raw := range orderLineIDs {
		if raw == "" {
			continue
		}
		id, err := ids.Parse(raw, ids.PrefixOrderLine)
		if err != nil {
			return 0, fmt.Errorf("affiliate: mã dòng đơn %q: %w", raw, err)
		}
		ma = append(ma, id)
	}
	if len(ma) == 0 {
		return 0, nil
	}

	ghi, err := s.repo.TheoDongDon(ctx, ma)
	if err != nil {
		return 0, err
	}

	return s.dao(ctx, ghi, lyDo)
}

// dao đảo một tập khoản hoa hồng và trả số khoản THẬT SỰ đổi trạng thái.
//
// Dùng chung bởi `DaoTheoDon` và `DaoTheoDongDon`: hai đường vào khác nhau
// nhưng cùng một phép, và hai bản cài của một phép đảo tiền sớm muộn sẽ lệch.
func (s *Service) dao(
	ctx context.Context, ghi []*domain.CommissionRecord, lyDo string,
) (int, error) {
	now := s.clock()
	var daDao int
	for _, c := range ghi {
		truoc := c.Status()
		if err := c.Dao(lyDo, now); err != nil {
			return daDao, err
		}
		if c.Status() == truoc {
			continue // đã đảo từ lượt trước
		}
		if err := s.repo.CapNhat(ctx, c); err != nil {
			return daDao, err
		}
		daDao++
	}
	return daDao, nil
}

// MoKhoaTheoDon chuyển hoa hồng của một (đơn, nhà bán) sang RÚT ĐƯỢC.
//
// Gọi khi `fulfillment_order.completed` tới — event đó CHÍNH LÀ tín hiệu
// "đã qua hạn đổi trả", nên affiliate không tự đoán hạn ấy. Hạn đổi trả là
// cấu hình nghiệp vụ mà module fulfillment sở hữu, và đoán lại ở đây là cài
// cùng một quy tắc lần thứ hai.
//
// `sellerID` rỗng nghĩa là mọi nhà bán của đơn. Một đơn nhiều nhà bán hết
// hạn theo TỪNG nhà bán, nên đường thường dùng là truyền đúng nhà bán.
//
// Khoản đã ĐẢO thì bỏ qua: hàng đã hoàn, tiền không mở khóa lại.
func (s *Service) MoKhoaTheoDon(
	ctx context.Context, orderID, sellerID string,
) (int, error) {
	oid, err := ids.Parse(orderID, ids.PrefixOrder)
	if err != nil {
		return 0, fmt.Errorf("affiliate: mã đơn %q: %w", orderID, err)
	}

	var sid ids.ID
	if sellerID != "" {
		sid, err = ids.Parse(sellerID, ids.PrefixSeller)
		if err != nil {
			return 0, fmt.Errorf("affiliate: mã nhà bán %q: %w", sellerID, err)
		}
	}

	ghi, err := s.repo.TheoDonVaNhaBan(ctx, oid, sid)
	if err != nil {
		return 0, err
	}

	now := s.clock()
	var daMo int
	for _, c := range ghi {
		if c.Status() != domain.HoaHongChoDenHan {
			continue
		}

		// `true` vì event ĐÃ là bằng chứng hết hạn đổi trả. Tham số vẫn
		// tồn tại để miền không phải tin bên gọi một cách ngầm ẩn.
		if err := c.ChuyenSangRutDuoc(true, now); err != nil {
			if errors.Is(err, domain.ErrDaDao) {
				continue
			}
			return daMo, err
		}
		if err := s.repo.CapNhat(ctx, c); err != nil {
			return daMo, err
		}
		daMo++
	}

	return daMo, nil
}

// SoDuCreator là tổng tiền theo từng trạng thái của một creator.
func (s *Service) SoDuCreator(
	ctx context.Context, creatorID string,
) (map[domain.TrangThaiHoaHong]int64, error) {
	cid, err := ids.Parse(creatorID, ids.PrefixCreator)
	if err != nil {
		return nil, fmt.Errorf("affiliate: mã creator %q: %w", creatorID, err)
	}
	return s.repo.TongTheoCreator(ctx, cid)
}
