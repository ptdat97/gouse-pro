// Package affiliate quy công đơn hàng cho creator và tính hoa hồng.
//
// # Module này là một EXTENSION
//
// Bỏ nó ra thì khách vẫn mua được hàng, tiền vẫn vào sổ, hàng vẫn rời kho —
// chỉ mất phần ghi công cho creator. Đó là tiêu chí phân loại của ADR-0021,
// và mọi quyết định thiết kế ở đây theo nó:
//
//	KHÔNG module nào import module này
//	KHÔNG bảng nào của module khác bị chạm (R6 cưỡng chế)
//	Dữ liệu tới bằng EVENT; dữ liệu đơn đọc qua CỔNG CÔNG KHAI của order
//
// Chiều extension → kernel là chiều được phép. Chiều ngược lại thì không:
// nếu module order phải biết về hoa hồng creator thì affiliate đã không còn
// là extension.
//
// # Mặt công khai này CỐ Ý nhỏ
//
// Chỉ có `New`, `Config`, hai bên nhận event, và `CreatorBalance`. Không có
// hàm nào cho module khác gọi, vì chưa module nào cần gọi. Bày ra một API
// chưa ai dùng là tạo trước thứ phải bảo trì — và dự án này đã gặp bốn hằng
// số event không ai phát (P3-76).
package affiliate

import (
	"context"

	"github.com/fashion-commerce/platform/internal/modules/affiliate/domain"
)

// TrangThai là trạng thái của một khoản hoa hồng, dưới dạng chuỗi.
const (
	StatusPending   = string(domain.CommissionPending)
	StatusAvailable = string(domain.CommissionAvailable)
	StatusReversed  = string(domain.CommissionReversed)
)

// API là mặt công khai của module affiliate.
//
// Hẹp có chủ ý: chỉ ĐỌC. Việc ghi xảy ra qua event, nên không có đường nào
// để một module khác tạo ra một khoản hoa hồng — quy công chỉ sinh ra từ một
// đơn đã thanh toán.
type API interface {
	// CreatorBalance là tổng tiền hoa hồng theo từng trạng thái.
	//
	// MỌI trạng thái có mặt kể cả khi bằng 0, nên bên đọc không phải phân
	// biệt "không có khóa" với "bằng không".
	CreatorBalance(ctx context.Context, creatorID string) (map[string]int64, error)
}
