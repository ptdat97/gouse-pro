package customer

import (
	"context"
	"errors"
	"strings"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/customer/application"
	"github.com/fashion-commerce/platform/internal/modules/customer/domain"
	"github.com/fashion-commerce/platform/internal/modules/identity"
)

// ErrEmailUsedByGuest khi email đã có hồ sơ khách hàng từ trước.
//
// # Vì sao TỪ CHỐI thay vì gộp
//
// Hồ sơ cũ có thể là của một khách VÃNG LAI đã đặt hàng — kèm lịch sử mua,
// địa chỉ nhà, số điện thoại. Gắn nó vào tài khoản vừa đăng ký nghĩa là bất
// kỳ ai biết email người khác đều đọc được những thứ đó (customer.md mục 5).
//
// Gộp được CHỈ SAU KHI xác minh quyền sở hữu email. Luồng xác minh chưa
// dựng, nên đường đúng duy nhất lúc này là từ chối và nói rõ cách tra đơn
// cũ: bằng mã đơn + số điện thoại, không cần tài khoản.
var ErrEmailUsedByGuest = errors.New(
	"customer: email đã được dùng để đặt hàng trước đó")

// RegisterRequest là dữ liệu đăng ký tài khoản khách hàng.
type RegisterRequest struct {
	Email    string
	Password string

	Phone       string
	DisplayName string
}

// RegisterResult là kết quả đăng ký.
type RegisterResult struct {
	CustomerID string
	UserID     string
}

// RegisterShopper tạo TÀI KHOẢN ĐĂNG NHẬP và HỒ SƠ KHÁCH HÀNG.
//
// # Vì sao module customer làm việc này, không phải identity
//
// Đăng ký cho người mua sinh ra HAI thứ ở hai module: `user` (thông tin
// đăng nhập) và `customer` (hồ sơ mua hàng). identity nằm DƯỚI customer
// trong đồ thị phụ thuộc nên nó không gọi ngược lên được; customer gọi
// xuống identity thì hợp lệ.
//
// Đăng ký cho NHÂN VIÊN là luồng khác và không đi qua đây: họ không có hồ
// sơ khách hàng, và tài khoản do quản trị viên tạo.
//
// # Thứ tự có chủ ý: KIỂM TRA hồ sơ TRƯỚC khi tạo tài khoản
//
// Tạo user xong mới phát hiện email đã có hồ sơ sẽ để lại một tài khoản mồ
// côi không đăng nhập vào đâu được, và lần thử lại sau báo "email đã dùng"
// vì chính tài khoản mồ côi đó.
func (m *Module) RegisterShopper(
	ctx context.Context, req RegisterRequest,
) (RegisterResult, error) {
	var out RegisterResult

	if m.identity == nil {
		return out, errors.New(
			"customer: thiếu module identity — không tạo được tài khoản đăng nhập")
	}

	email := domain.NormalizeEmail(req.Email)
	if email == "" {
		return out, ErrInvalidInput
	}

	// Bước 1: hồ sơ khách hàng đã tồn tại chưa.
	//
	// PHÂN BIỆT hai trường hợp — chúng dẫn tới hai hành động khác hẳn của
	// người dùng, và trả chung một thông báo đẩy nhóm thứ hai vào đường
	// cụt (họ bấm "quên mật khẩu" cho một tài khoản không tồn tại):
	//
	//	hồ sơ CÓ user_id     → đã có tài khoản → đăng nhập
	//	hồ sơ KHÔNG có       → khách vãng lai  → tra đơn bằng mã + SĐT
	if existing, err := m.svc.GetByEmail(ctx, email); err == nil {
		if !existing.UserID().IsZero() {
			return out, identity.ErrDuplicateEmail
		}
		return out, ErrEmailUsedByGuest
	} else if !errors.Is(err, domain.ErrNotFound) {
		return out, err
	}

	// Bước 2: tài khoản đăng nhập.
	//
	// KHÔNG truyền Roles: đường đăng ký công khai để identity tự gán
	// CUSTOMER. Cho client chọn vai trò là để bất kỳ ai tự cấp mình ADMIN.
	user, err := m.identity.Register(ctx, identity.RegisterRequest{
		Email:       email,
		Password:    req.Password,
		Phone:       strings.TrimSpace(req.Phone),
		DisplayName: strings.TrimSpace(req.DisplayName),
	})
	if err != nil {
		return out, err
	}

	// Bước 3: hồ sơ khách hàng, gắn với tài khoản vừa tạo.
	c, err := m.svc.Create(ctx, application.CreateInput{
		Email:       email,
		Phone:       strings.TrimSpace(req.Phone),
		DisplayName: strings.TrimSpace(req.DisplayName),
		UserID:      ids.ID(user.ID),
	})
	if err != nil {
		// Tài khoản đã tạo nhưng hồ sơ thì không. Người dùng đăng nhập
		// được mà không mua được gì — và lần đăng ký lại báo "email đã
		// dùng" vì chính tài khoản này.
		//
		// KHÔNG tự dọn: xóa tài khoản là thao tác nguy hiểm hơn nhiều so
		// với việc để lại một bản ghi cần sửa tay. Ghi rõ trong lỗi để
		// người vận hành biết chính xác phải làm gì.
		return out, errors.Join(
			errors.New("customer: đã tạo tài khoản "+user.ID+
				" nhưng KHÔNG tạo được hồ sơ khách hàng — cần dọn thủ công"),
			err,
		)
	}

	return RegisterResult{CustomerID: c.ID().String(), UserID: user.ID}, nil
}
