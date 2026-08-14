package domain

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// TxFunc chạy bên trong giao dịch mà kho lưu trữ đã mở.
//
// Ngữ cảnh truyền vào MANG giao dịch đó, nên tầng application phát được
// domain event mà không cần biết database.
type TxFunc func(ctx context.Context) error

// Repository là PORT cho kho lưu trữ giỏ hàng.
//
// Save ghi cả giỏ và các món trong MỘT giao dịch: giỏ có món mà món không
// ghi được là giỏ rỗng trong mắt khách, và họ sẽ thêm lại rồi thấy trùng.
type Repository interface {
	Save(ctx context.Context, c *Cart) error

	// SaveWithEvents ghi giỏ VÀ chạy fn trong CÙNG một giao dịch.
	//
	// fn là nơi tầng application phát domain event. Ghi giỏ thành công mà
	// event thất bại nghĩa là tín hiệu nhu cầu bị mất — và dữ liệu lịch sử
	// không tạo ngược được.
	SaveWithEvents(ctx context.Context, c *Cart, fn TxFunc) error

	FindByID(ctx context.Context, id ids.ID) (*Cart, error)

	// FindActiveByCustomer tìm giỏ ĐANG DÙNG của một khách.
	//
	// QUY TẮC 5: mỗi khách chỉ có một giỏ ACTIVE. Giỏ đã chuyển đổi hoặc
	// bỏ quên vẫn ở lại như dữ liệu phân tích, nên hàm này lọc theo trạng
	// thái chứ không phải trả giỏ mới nhất.
	FindActiveByCustomer(ctx context.Context, customerID ids.ID) (*Cart, error)

	FindActiveBySession(ctx context.Context, sessionID string) (*Cart, error)

	// Delete xóa hẳn một giỏ.
	//
	// CHỈ dùng cho giỏ theo phiên đã hết hạn mà chưa có món nào — dữ liệu
	// không có giá trị phân tích. Giỏ đã chuyển đổi KHÔNG BAO GIỜ xóa: nó
	// cho biết nội dung nào dẫn tới việc mua thật.
	Delete(ctx context.Context, id ids.ID) error
}

// OfferLookup là PORT để lấy dữ liệu hiện tại của offer.
//
// Đây là cách module cart hỏi marketplace/inventory mà KHÔNG phụ thuộc
// trực tiếp vào chúng ở tầng domain: tầng application cài đặt port này
// bằng cách gọi API công khai của các module đó.
//
// PHƯƠNG THỨC THEO LÔ, không phải theo từng cái: hiển thị giỏ 10 món cần
// dữ liệu từ nhiều module, và gọi trong vòng lặp sẽ thành 10 lượt gọi mỗi
// lần khách mở giỏ.
type OfferLookup interface {
	// LookupOffers tra dữ liệu hiện tại của nhiều offer cùng lúc.
	//
	// Offer không tìm thấy thì KHÔNG có mặt trong map trả về — bên gọi
	// hiểu đó là "đã bị gỡ" và đánh dấu UNAVAILABLE.
	LookupOffers(ctx context.Context, offerIDs []ids.ID) (map[ids.ID]SyncData, error)
}
