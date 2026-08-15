// Package seller là module quản lý nhà bán: danh tính và chính sách.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// RANH GIỚI QUAN TRỌNG: module này KHÔNG sở hữu offer, đơn hàng, hay tiền.
// Muốn biết seller còn bao nhiêu tiền → gọi payment.GetBalance(). Nếu gộp
// cả số dư vào đây, module trở nên khổng lồ và không tách được.
package seller

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module seller.
//
// Chữ ký khớp docs/04-modules/seller.md mục 8.
type API interface {
	GetSeller(ctx context.Context, sellerID string) (*SellerView, error)

	// GetSellersByIDs nhận DANH SÁCH để tránh N+1.
	//
	// Trang danh sách offer cần tên seller của từng offer — 1 lời gọi,
	// không phải một lời gọi cho mỗi offer.
	GetSellersByIDs(ctx context.Context, sellerIDs []string) (map[string]SellerView, error)

	// IsSellerActive cho biết nhà bán có đang bán hàng được không.
	//
	// Module marketplace gọi TRƯỚC khi hiển thị offer: seller bị đình chỉ
	// thì mọi offer phải ẩn.
	IsSellerActive(ctx context.Context, sellerID string) (bool, error)

	// ApplyAsSeller nộp hồ sơ đăng ký.
	ApplyAsSeller(ctx context.Context, req ApplicationRequest) (*SellerView, error)

	// ApproveSeller duyệt hồ sơ VÀ ghi vết kiểm toán trong cùng giao dịch.
	//
	// Duyệt chuyển seller sang APPROVED, **chưa phải** ACTIVE — seller chỉ
	// bán được khi tài khoản ngân hàng đã xác minh.
	ApproveSeller(ctx context.Context, req ApproveRequest) (*ApproveResult, error)

	// SuspendSeller đình chỉ nhà bán VÀ ghi vết kiểm toán trong cùng giao
	// dịch.
	//
	// Việc này làm ẩn offer nhưng KHÔNG hủy đơn đang xử lý — đơn khách đã
	// trả tiền phải được hoàn tất hoặc hủy có kiểm soát kèm hoàn tiền.
	SuspendSeller(ctx context.Context, req SuspendRequest) (*SuspendResult, error)

	// ListSellers liệt kê nhà bán theo bộ lọc.
	//
	// Dùng cho hàng đợi duyệt hồ sơ ở giao diện quản trị: lọc theo trạng
	// thái PENDING để thấy hồ sơ đang chờ.
	ListSellers(ctx context.Context, f ListFilter) ([]SellerView, error)
}

// ListFilter là điều kiện lọc danh sách nhà bán.
type ListFilter struct {
	// Status lọc theo trạng thái. Rỗng = mọi trạng thái.
	Status string

	// SellerType lọc theo loại. Rỗng = mọi loại.
	SellerType string

	Limit  int
	Offset int
}

// ApproveRequest là yêu cầu duyệt hồ sơ nhà bán.
type ApproveRequest struct {
	SellerID string

	// ActorID là nhân viên duyệt. BẮT BUỘC.
	ActorID string

	// CommissionRateBP theo phần vạn (1000 = 10%). Bỏ qua với INTERNAL.
	CommissionRateBP int32

	Notes     string
	RequestID string
}

// ApproveResult là kết quả duyệt hồ sơ.
type ApproveResult struct {
	Seller SellerView

	// SideEffects liệt kê tác động để người vận hành hiểu điều gì đã xảy ra.
	//
	// Chỉ liệt kê việc ĐÃ xảy ra trong giao dịch này. Việc chạy bất đồng bộ
	// qua event không được kể ở đây như thể đã xong.
	SideEffects []string
}

// SuspendRequest là yêu cầu đình chỉ nhà bán.
type SuspendRequest struct {
	SellerID string

	// ActorID là nhân viên thực hiện. BẮT BUỘC — đình chỉ một gian hàng là
	// cắt nguồn thu của người khác, phải biết ai đã quyết định.
	ActorID string

	// Reason BẮT BUỘC, tối thiểu 20 ký tự, không nhận giá trị rác.
	Reason     string
	ReasonCode string

	RequestID string
}

// SuspendResult là kết quả đình chỉ.
type SuspendResult struct {
	Seller SellerView

	// Note giải thích tác động cho người vận hành.
	//
	// KHÔNG có `offers_hidden`: việc ẩn offer do marketplace làm khi nghe
	// event, tức là BẤT ĐỒNG BỘ. Trả về một con số tại thời điểm này là
	// bịa ra dữ liệu chưa tồn tại.
	Note string
}

// ---------------------------------------------------------------- DTO

// SellerView là dữ liệu nhà bán cho module khác — CHỈ ĐỌC.
//
// KHÔNG chứa số dư, tài khoản ngân hàng, hay giấy tờ pháp lý nhạy cảm.
type SellerView struct {
	ID   string
	Name string
	Slug string

	// SellerType: INTERNAL, INDIVIDUAL, BUSINESS, LOCAL_BRAND, STRATEGIC_PARTNER.
	SellerType string
	Status     string

	// CommissionRateBP là tỷ lệ hoa hồng theo phần vạn (1000 = 10%).
	//
	// Dùng phần vạn thay vì số thực: sai số dấu phẩy động tích lũy thành
	// lệch đối soát, và lệch đối soát phải điều tra thủ công từng đơn.
	CommissionRateBP int32

	// IsActive: đang bán hàng được không.
	IsActive bool

	// IsInternal: đây có phải own brand của nền tảng không.
	//
	// Quan trọng ở tầng ledger: đơn own brand ghi doanh thu toàn phần +
	// giá vốn; đơn marketplace ghi hoa hồng.
	IsInternal bool

	// OffersHidden: offer của nhà bán này có đang bị ẩn không.
	OffersHidden bool
}

// ApplicationRequest là hồ sơ đăng ký làm nhà bán.
type ApplicationRequest struct {
	Name       string
	Slug       string
	SellerType string
	LegalName  string
	TaxCode    string
	Email      string
	Phone      string

	// CommissionRateBP theo phần vạn. Bỏ qua với seller INTERNAL.
	CommissionRateBP int32
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound  = errNotFound{}
	ErrInvalidID = errInvalidID{}

	// ErrNotAllowed khi thao tác không hợp lệ với trạng thái hiện tại.
	ErrNotAllowed = errNotAllowed{}

	// ErrInvalidCommissionRate khi tỷ lệ hoa hồng ngoài khoảng [0, 10000].
	ErrInvalidCommissionRate = errInvalidCommissionRate{}
)

type errInvalidCommissionRate struct{}

func (errInvalidCommissionRate) Error() string {
	return "seller: tỷ lệ hoa hồng phải trong khoảng 0–10000 phần vạn"
}

type errNotFound struct{}

func (errNotFound) Error() string { return "seller: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "seller: định danh không hợp lệ" }

type errNotAllowed struct{}

func (errNotAllowed) Error() string {
	return "seller: thao tác không hợp lệ với trạng thái hiện tại"
}

// ---------------------------------------------------------------- Tiền tố

const SellerIDPrefix = string(ids.PrefixSeller)

// InternalSellerSlug là slug của seller nội bộ (own brand).
//
// Hằng công khai để module khác tra được own brand mà không cần đoán.
const InternalSellerSlug = "own-brand"
