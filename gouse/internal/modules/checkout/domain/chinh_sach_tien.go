package domain

import (
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
)

// ChinhSachTien là hai con số kinh doanh quyết định phí ship và thuế.
//
// Truyền VÀO domain chứ không đọc từ đâu: domain không biết `opsconfig`
// tồn tại, và một hàm nhận đủ đầu vào thì kiểm chứng được bằng bảng giá
// trị thay vì phải dựng cấu hình.
type ChinhSachTien struct {
	// ThueSuat theo phần vạn: 800 = 8%. Một tầng, áp cho mọi mặt hàng.
	ThueSuat types.BasisPoints

	// NguongMienPhiShip là TIỀN HÀNG tối thiểu để được miễn phí vận
	// chuyển. Ngưỡng 0 nghĩa là mọi đơn được miễn.
	NguongMienPhiShip money.Money
}

// ApDungPhiVaThue đặt phí vận chuyển (đã xét miễn phí) và thuế.
//
// # THỨ TỰ TÍNH, và vì sao nó phải nằm ở MỘT chỗ
//
//	tiền hàng = subtotal − giảm giá          khách thực trả cho HÀNG
//	phí ship  = 0 nếu tiền hàng ≥ ngưỡng, ngược lại = phí gốc
//	thuế      = thuế suất × (tiền hàng + phí ship)
//	tổng      = tiền hàng + phí ship + thuế
//
// Ba con số PHỤ THUỘC NHAU theo đúng thứ tự đó: giảm giá đổi thì ngưỡng
// miễn phí ship phải xét lại, và phí ship đổi thì thuế phải tính lại. Để
// mỗi đường ghi (áp mã, gỡ mã, chọn cách giao) tự cập nhật một mảnh là
// cách chắc chắn để ba con số lệch nhau — và lệch ở đây là khách bị thu
// sai tiền.
//
// Vì vậy chỉ có MỘT hàm đặt cả hai, và mọi đường ghi đều gọi lại nó.
//
// # Ngưỡng xét trên tiền hàng SAU GIẢM GIÁ
//
// Đó là số tiền khách thực trả cho hàng. Xét trên subtotal TRƯỚC giảm giá
// thì một mã giảm 200.000đ biến đơn 400.000đ thành đơn được miễn phí ship
// — nền tảng chịu cả hai khoản cho một đơn nhỏ hơn ngưỡng.
//
// # Thuế tính TRÊN CẢ phí vận chuyển
//
// Vận chuyển là một dịch vụ chịu thuế, không phải khoản thu hộ. Tính thuế
// chỉ trên tiền hàng sẽ ra số thuế thấp hơn thực tế phải nộp.
//
// # Làm tròn NỬA LÊN
//
// `money.RoundHalfUp` — chú thích của chính kernel ghi "dùng cho thuế theo
// quy định". Hoa hồng và phí thì làm tròn XUỐNG; hai quy tắc khác nhau cho
// hai loại số khác nhau, và trộn chúng làm đối soát ra hai kết quả.
func (c *Checkout) ApDungPhiVaThue(
	phiGoc money.Money, cs ChinhSachTien, now time.Time,
) error {
	if err := c.mutable(now); err != nil {
		return err
	}

	tienHang, err := c.Subtotal().Sub(c.discountAmount)
	if err != nil {
		return err
	}

	phi := phiGoc
	if phi.Currency() == "" {
		phi = money.Zero(c.currency)
	}
	if phi.Currency() != c.currency {
		return ErrKhacDonViTienTe
	}

	// Miễn phí ship khi tiền hàng ĐẠT ngưỡng.
	//
	// So sánh ">=" chứ không ">": ngưỡng 499.000đ nghĩa là đơn đúng
	// 499.000đ ĐƯỢC miễn. Khách nhìn con số quảng cáo và mua vừa đúng
	// bằng nó là hành vi phổ biến; để họ trượt vì một đồng là cách chắc
	// chắn để nhận một khiếu nại đúng.
	if !cs.NguongMienPhiShip.IsZero() || cs.NguongMienPhiShip.Currency() != "" {
		if !tienHang.LessThan(cs.NguongMienPhiShip) {
			phi = money.Zero(c.currency)
		}
	}

	chiuThue, err := tienHang.Add(phi)
	if err != nil {
		return err
	}

	c.shippingFee = phi
	c.taxAmount = chiuThue.ApplyRate(cs.ThueSuat, money.RoundHalfUp)
	c.touch(now)
	return nil
}
