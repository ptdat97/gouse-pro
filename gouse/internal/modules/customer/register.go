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

	// CanXacMinhEmail = true khi email này ĐÃ có hồ sơ vãng lai.
	//
	// Tài khoản đã tạo nhưng hồ sơ CHƯA gắn vào: lịch sử mua hàng và địa
	// chỉ nhà chỉ được gộp sau khi bấm liên kết xác minh trong hộp thư.
	// Giao diện dùng cờ này để nói cho khách biết còn một bước nữa.
	CanXacMinhEmail bool
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
	var hoSoVangLai ids.ID
	if existing, err := m.svc.GetByEmail(ctx, email); err == nil {
		if !existing.UserID().IsZero() {
			return out, identity.ErrDuplicateEmail
		}
		// Hồ sơ VÃNG LAI có sẵn: cho đăng ký, nhưng CHƯA gắn (P3-15).
		//
		// Trước 07/09 đường này TỪ CHỐI hẳn, và từ chối là đúng khi chưa
		// có cách chứng minh quyền sở hữu email — hồ sơ vãng lai chứa lịch
		// sử mua hàng và địa chỉ nhà. Nhưng nó để khách ở ngõ cụt: không
		// tạo được tài khoản bằng CHÍNH email của mình.
		//
		// Nay: tạo tài khoản, chưa gắn hồ sơ. Gộp xảy ra sau khi bấm liên
		// kết xác minh trong hộp thư — thứ chứng minh họ đọc được email đó.
		hoSoVangLai = existing.ID()
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

	// Bước 2b: email này có ĐƠN vãng lai đang chờ không?
	//
	// Hỏi `order` chứ không tra hồ sơ khách. Khách vãng lai KHÔNG có hồ sơ
	// — đo trên database phát triển 07/09: 0 hồ sơ vãng lai nhưng 3150 đơn
	// vãng lai. Kiểm bằng hồ sơ là kiểm một thứ gần như không bao giờ tồn
	// tại, và khi đó cờ "cần xác minh" không bao giờ bật.
	coDonVangLai := false
	if m.orders != nil {
		if n, errDem := m.orders.DemDonVangLai(ctx, email); errDem == nil && n > 0 {
			coDonVangLai = true
		}
	}

	// Bước 3a: đã có hồ sơ vãng lai thì KHÔNG tạo hồ sơ mới.
	//
	// Email là DUY NHẤT trên `customer`, nên hai hồ sơ cùng email là điều
	// không thể. Hồ sơ vãng lai chính là hồ sơ của người này — chỉ là chưa
	// được phép gắn vào tài khoản.
	//
	// Tài khoản chưa có hồ sơ vẫn mua hàng được: `CustomerIDForUser` trả
	// rỗng và họ được coi như khách vãng lai, nên đơn mới vẫn rơi đúng vào
	// hồ sơ đó qua `EnsureByEmail`. Thứ họ CHƯA thấy là lịch sử cũ — đúng
	// điều đang chờ xác minh.
	if !hoSoVangLai.IsZero() {
		return RegisterResult{
			CustomerID:      hoSoVangLai.String(),
			UserID:          user.ID,
			CanXacMinhEmail: true,
		}, nil
	}

	// Bước 3b: hồ sơ khách hàng, gắn với tài khoản vừa tạo.
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

	return RegisterResult{
		CustomerID: c.ID().String(),
		UserID:     user.ID,
		// Hồ sơ MỚI nhưng email đã có đơn vãng lai: lịch sử đó chỉ về với
		// họ sau khi xác minh.
		CanXacMinhEmail: coDonVangLai,
	}, nil
}
