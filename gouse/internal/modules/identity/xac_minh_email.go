package identity

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/identity/application"
)

// PhatTokenXacMinh phát liên kết xác minh email cho một tài khoản.
//
// Trả bản NGUYÊN VĂN của token. Bên gọi đưa nó vào thư và KHÔNG ghi vào
// log — database chỉ giữ bản băm, nên một dòng log lộ ra là mất toàn bộ
// giá trị của việc băm.
func (m *Module) PhatTokenXacMinh(
	ctx context.Context, userID string,
) (token string, email string, err error) {
	id, err := ids.Parse(userID, ids.PrefixUser)
	if err != nil {
		return "", "", ErrInvalidInput
	}
	return m.svc.PhatTokenXacMinh(ctx, id)
}

// XacMinhEmail đổi token nguyên văn lấy việc đánh dấu email đã xác minh.
func (m *Module) XacMinhEmail(
	ctx context.Context, token string,
) (*XacMinhResult, error) {
	res, err := m.svc.XacMinhEmail(ctx, token)
	if err != nil {
		return nil, err
	}
	return &XacMinhResult{UserID: res.UserID.String(), Email: res.Email}, nil
}

// ErrEmailDaXacMinhRoi: email của tài khoản này đã được xác minh.
var ErrEmailDaXacMinhRoi = application.ErrEmailDaXacMinh
