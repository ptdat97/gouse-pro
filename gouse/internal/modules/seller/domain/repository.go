package domain

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// TxFunc chạy bên trong giao dịch mà kho lưu trữ đã mở.
//
// Ngữ cảnh truyền vào MANG giao dịch đó, nên tầng application ghi được vết
// kiểm toán mà không cần biết database.
type TxFunc func(ctx context.Context) error

// Repository là PORT cho nhà bán.
type Repository interface {
	Save(ctx context.Context, s *Seller) error

	// SaveWithAudit ghi nhà bán VÀ chạy fn trong CÙNG một giao dịch.
	//
	// fn là nơi tầng application ghi vết kiểm toán. Đổi trạng thái thành
	// công mà vết kiểm toán thất bại nghĩa là một seller bị đình chỉ mà
	// không ai biết ai đã làm việc đó — và đó chính là câu hỏi duy nhất
	// người ta cần trả lời khi có tranh chấp.
	//
	// Chiều ngược lại còn tệ hơn: vết kiểm toán ghi thành công nhưng giao
	// dịch nghiệp vụ rollback tạo ra bằng chứng cho việc CHƯA TỪNG xảy ra.
	SaveWithAudit(ctx context.Context, s *Seller, fn TxFunc) error

	FindByID(ctx context.Context, id ids.ID) (*Seller, error)
	FindBySlug(ctx context.Context, slug string) (*Seller, error)

	// FindByIDs nhận DANH SÁCH để tránh N+1.
	//
	// Trang danh sách offer cần tên seller của từng offer — phải là 1 truy
	// vấn, không phải một truy vấn cho mỗi offer.
	FindByIDs(ctx context.Context, list []ids.ID) (map[ids.ID]*Seller, error)

	List(ctx context.Context, f Filter) ([]*Seller, error)
}

// Filter là điều kiện lọc danh sách nhà bán.
type Filter struct {
	Status Status
	Type   SellerType
	Limit  int
	Offset int
}
