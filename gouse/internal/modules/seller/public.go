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

	ApproveSeller(ctx context.Context, sellerID string, approvedBy string) error

	// SuspendSeller đình chỉ nhà bán.
	//
	// Việc này làm ẩn offer nhưng KHÔNG hủy đơn đang xử lý — đơn khách đã
	// trả tiền phải được hoàn tất hoặc hủy có kiểm soát kèm hoàn tiền.
	SuspendSeller(ctx context.Context, sellerID string, reason string) error
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
)

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
