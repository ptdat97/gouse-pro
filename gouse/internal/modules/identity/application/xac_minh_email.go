package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/identity/domain"
)

// ErrChuaNoiXacMinhEmail: bản dựng này chưa nối kho token xác minh.
var ErrChuaNoiXacMinhEmail = errors.New("identity: chưa nối kho token xác minh email")

// PhatTokenXacMinh phát một liên kết xác minh email cho tài khoản.
//
// Trả về bản NGUYÊN VĂN của token — bên gọi đưa nó vào thư và KHÔNG ghi
// vào log. Database chỉ giữ bản băm.
//
// # Vô hiệu token cũ trước khi phát token mới
//
// Hai liên kết còn sống cùng lúc nghĩa là liên kết cũ trong hộp thư vẫn
// dùng được sau khi người dùng đã bấm "gửi lại" — và người ta bấm "gửi
// lại" chính vì nghi ngờ thư cũ.
func (s *Service) PhatTokenXacMinh(
	ctx context.Context, userID ids.ID,
) (nguyenVan string, email string, err error) {
	if s.emailTok == nil {
		return "", "", ErrChuaNoiXacMinhEmail
	}

	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if !u.EmailVerifiedAt().IsZero() {
		return "", "", ErrEmailDaXacMinh
	}

	now := s.clock.Now()
	if err := s.emailTok.InvalidateForUser(ctx, userID, now); err != nil {
		return "", "", err
	}

	plain, hashed, err := s.tokens.NewToken()
	if err != nil {
		return "", "", fmt.Errorf("sinh token xác minh: %w", err)
	}

	t, err := domain.NewEmailToken(domain.NewEmailTokenParams{
		UserID: userID, Hash: hashed, Email: u.Email(), Now: now,
	})
	if err != nil {
		return "", "", err
	}
	if err := s.emailTok.Create(ctx, t); err != nil {
		return "", "", err
	}
	return plain, u.Email(), nil
}

// ErrEmailDaXacMinh: email của tài khoản này đã được xác minh rồi.
var ErrEmailDaXacMinh = errors.New("identity: email đã được xác minh")

// KetQuaXacMinh là kết quả xác minh email.
type KetQuaXacMinh struct {
	UserID ids.ID
	Email  string
}

// XacMinhEmail đổi một token nguyên văn lấy việc đánh dấu email đã xác minh.
//
// # Ba lớp kiểm, và vì sao lớp cuối nằm ở SQL
//
//  1. băm khớp        → tra bằng băm, không bao giờ so bản nguyên văn
//  2. domain          → chưa dùng, chưa hết hạn, email chưa đổi
//  3. `used_at IS NULL` trong mệnh đề WHERE của lệnh cập nhật
//
// Lớp ba không thừa: lớp hai đọc rồi lớp ba ghi là HAI bước, và hai
// request tới cùng lúc đều qua được lớp hai. Ràng buộc ở lệnh ghi là thứ
// duy nhất chặn việc dùng một token hai lần.
func (s *Service) XacMinhEmail(
	ctx context.Context, nguyenVan string,
) (*KetQuaXacMinh, error) {
	if s.emailTok == nil {
		return nil, ErrChuaNoiXacMinhEmail
	}

	t, err := s.emailTok.FindByHash(ctx, s.tokens.HashToken(nguyenVan))
	if err != nil {
		return nil, err
	}

	u, err := s.users.FindByID(ctx, t.UserID())
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	if err := t.Dung(u.Email(), now); err != nil {
		return nil, err
	}
	if err := s.emailTok.MarkUsed(ctx, t); err != nil {
		return nil, err
	}

	u.VerifyEmail(now)
	if err := s.users.Update(ctx, u); err != nil {
		return nil, err
	}

	return &KetQuaXacMinh{UserID: u.ID(), Email: u.Email()}, nil
}
