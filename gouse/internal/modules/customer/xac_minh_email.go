package customer

import (
	"context"
	"errors"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/customer/domain"
)

// XacMinhResult là kết quả xác minh email kèm việc gộp hồ sơ.
type XacMinhResult struct {
	UserID string
	Email  string

	// DaGopHoSo = true khi có một hồ sơ vãng lai vừa được gắn vào tài khoản.
	DaGopHoSo bool

	// SoDonDaGop là số ĐƠN vãng lai vừa được chuyển sang hồ sơ khách.
	//
	// Đây mới là "lịch sử mua hàng" theo nghĩa khách hiểu. Đo trên
	// database phát triển 07/09: 0 hồ sơ vãng lai nhưng 3150 đơn vãng lai
	// — nghĩa là gắn hồ sơ một mình KHÔNG mang lại đơn nào.
	SoDonDaGop int

	// CustomerID là hồ sơ nay thuộc về tài khoản. Rỗng khi không có gì để gộp.
	CustomerID string
}

// XacMinhEmailVaGop xác minh email RỒI gộp hồ sơ vãng lai vào tài khoản.
//
// # Vì sao hai việc này đi CÙNG NHAU, và ở module này
//
// Xác minh là việc của `identity` (nó giữ tài khoản). Gộp là việc của
// `customer` (nó giữ hồ sơ). Không module nào làm được cả hai, và
// `identity` KHÔNG biết `customer` tồn tại — chiều phụ thuộc là
// customer → identity.
//
// Nên chỗ duy nhất ghép được là đây.
//
// # Vì sao gộp CHỈ sau khi xác minh
//
// Hồ sơ vãng lai chứa lịch sử mua hàng và địa chỉ nhà. Gộp nó vào một tài
// khoản vừa đăng ký nghĩa là bất kỳ ai biết email người khác đều đọc được
// những thứ đó. Bấm được liên kết trong hộp thư là bằng chứng đọc được
// hộp thư đó — điều kiện tối thiểu, và là điều kiện chưa từng có trước
// P3-15.
//
// # Không có gì để gộp KHÔNG phải lỗi
//
// Người đăng ký bằng email chưa từng đặt hàng vẫn cần xác minh email cho
// những việc khác. Trả `DaGopHoSo: false` và đi tiếp.
func (m *Module) XacMinhEmailVaGop(
	ctx context.Context, token string,
) (*XacMinhResult, error) {
	if m.identity == nil {
		return nil, errors.New("customer: thiếu module identity")
	}

	res, err := m.identity.XacMinhEmail(ctx, token)
	if err != nil {
		return nil, err
	}

	out := &XacMinhResult{UserID: res.UserID, Email: res.Email}

	// Tìm hồ sơ theo EMAIL: nó vừa là hồ sơ của tài khoản (đường đăng ký
	// thường), vừa là hồ sơ vãng lai nếu có (đường `EnsureByEmail`).
	hoSo, err := m.svc.GetByEmail(ctx, domain.NormalizeEmail(res.Email))
	if errors.Is(err, domain.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.CustomerID = hoSo.ID().String()

	// Đã gắn vào ĐÚNG tài khoản này rồi: không phải lỗi.
	//
	// VẪN thử gắn đơn: lần trước có thể đã gắn hồ sơ xong mà gắn đơn hỏng,
	// và `GanDonVangLai` chỉ đụng đơn còn `customer_id` rỗng nên chạy lại
	// không gây hại.
	if hoSo.UserID().String() == res.UserID {
		out.SoDonDaGop = m.gopDonVangLai(ctx, res.Email, hoSo.ID().String())
		return out, nil
	}

	// Đã gắn vào tài khoản KHÁC: dừng lại.
	//
	// Không thể xảy ra qua đường đăng ký bình thường (email là duy nhất),
	// nhưng nếu xảy ra thì nó là dấu hiệu dữ liệu đã lệch, và gắn đè sẽ
	// chuyển lịch sử mua hàng của người này sang người khác.
	if !hoSo.UserID().IsZero() {
		return nil, ErrHoSoDaThuocTaiKhoanKhac
	}

	if err := m.svc.LinkUser(ctx, hoSo.ID(), ids.ID(res.UserID)); err != nil {
		return nil, err
	}
	out.DaGopHoSo = true
	out.SoDonDaGop = m.gopDonVangLai(ctx, res.Email, hoSo.ID().String())
	return out, nil
}

// gopDonVangLai chuyển chủ các đơn vãng lai, trả SỐ đơn đã gắn.
//
// Hỏng thì trả 0 chứ KHÔNG làm hỏng cả lần xác minh: email đã được xác
// minh và hồ sơ đã gắn — hai việc đó không được cuộn lại vì một lần gắn
// đơn thất bại. Khách bấm lại liên kết cũng không giúp gì (token đã dùng),
// nên đường sửa là chạy lại việc gắn, không phải xác minh lại.
func (m *Module) gopDonVangLai(ctx context.Context, email, customerID string) int {
	if m.orders == nil {
		return 0
	}
	n, err := m.orders.GanDonVangLaiChoKhach(ctx, email, customerID)
	if err != nil {
		return 0
	}
	return n
}

// GuiLienKetXacMinh phát token RỒI gửi thư chứa liên kết.
//
// # Vì sao token nguyên văn KHÔNG bao giờ ra khỏi hàm này
//
// Nó đi thẳng từ `PhatTokenXacMinh` vào thân thư và không được trả về, không
// được ghi log. Database chỉ giữ bản băm, nên một dòng log lộ ra là mất
// toàn bộ giá trị của việc băm.
//
// # Gửi thư hỏng KHÔNG hủy token
//
// Token đã ghi vào database rồi. Xóa nó đi vì thư gửi hỏng sẽ làm người
// dùng bấm "gửi lại" và nhận token thứ hai trong khi token thứ nhất có
// thể đã tới nơi — hai liên kết sống cùng lúc, đúng thứ `InvalidateForUser`
// tồn tại để tránh. Báo lỗi và để họ bấm gửi lại là đường sạch hơn.
func (m *Module) GuiLienKetXacMinh(ctx context.Context, userID string) error {
	if m.identity == nil {
		return errors.New("customer: thiếu module identity")
	}
	if m.notifier == nil {
		return ErrChuaNoiThongBao
	}

	token, email, err := m.identity.PhatTokenXacMinh(ctx, userID)
	if err != nil {
		return err
	}

	return m.notifier.GuiXacMinhEmail(ctx, userID, email, token)
}

// ErrChuaNoiThongBao: bản dựng này chưa nối module gửi thông báo.
//
// Báo lỗi rõ thay vì im lặng không gửi gì: một luồng xác minh mà thư không
// bao giờ tới là ngõ cụt giống hệt thứ P3-15 sinh ra để xóa bỏ.
var ErrChuaNoiThongBao = errors.New("customer: chưa nối module thông báo")

// ErrHoSoDaThuocTaiKhoanKhac: hồ sơ đã gắn vào một tài khoản khác.
//
// Gắn đè sẽ chuyển lịch sử mua hàng và địa chỉ nhà của người này sang
// người khác — im lặng.
var ErrHoSoDaThuocTaiKhoanKhac = errors.New(
	"customer: hồ sơ đã thuộc về một tài khoản khác")
