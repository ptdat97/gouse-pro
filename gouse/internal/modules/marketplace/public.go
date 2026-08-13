// Package marketplace là module quản lý Offer — lời chào bán của một
// seller cho một SKU.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// Offer là ĐƠN VỊ KHÁCH THỰC SỰ MUA, không phải Product hay SKU: khách
// chọn mua từ một nhà bán cụ thể với giá cụ thể.
//
// PHÂN VAI VỀ HOA HỒNG — ba module, ba việc khác nhau:
//
//	marketplace → ĐỊNH NGHĨA quy tắc ("ngành áo, seller loại B, 10%")
//	order       → ĐÓNG BĂNG vào OrderLine tại thời điểm đặt hàng
//	payment     → GHI SỔ vào ledger
//
// Không có hai module cùng làm một việc.
package marketplace

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module marketplace.
//
// Chữ ký khớp docs/04-modules/marketplace.md mục 7.
type API interface {
	// ---- Offer ----

	GetOffer(ctx context.Context, offerID string) (*OfferView, error)
	GetOffersBySKU(ctx context.Context, skuID string) ([]OfferView, error)

	// GetOffersBySKUs nhận DANH SÁCH để tránh N+1.
	GetOffersBySKUs(ctx context.Context, skuIDs []string) (map[string][]OfferView, error)

	// GetOffersByIDs tra nhiều offer theo định danh, cũng để tránh N+1.
	//
	// Module cart giữ offer_id của từng món trong giỏ, nên hiển thị giỏ
	// 10 món cần đúng hàm này — một lượt gọi, không phải mười.
	//
	// Offer không tồn tại thì KHÔNG có mặt trong map trả về; bên gọi hiểu
	// đó là "đã bị gỡ".
	GetOffersByIDs(ctx context.Context, offerIDs []string) (map[string]OfferView, error)

	// ---- Buy box ----

	// GetBuyBoxOffer trả offer hiển thị mặc định cho một SKU.
	//
	// Chỉ chọn offer CÒN HÀNG, seller ACTIVE, offer ACTIVE. Trả nil nếu
	// không offer nào đủ điều kiện.
	GetBuyBoxOffer(ctx context.Context, skuID string) (*BuyBoxView, error)

	GetBuyBoxOffers(ctx context.Context, skuIDs []string) (map[string]BuyBoxView, error)

	// ---- Hoa hồng ----

	// GetCommissionRate chỉ trả TỶ LỆ, KHÔNG tính số tiền (quy tắc 8).
	//
	// Tính tiền là việc của order (đóng băng) và payment (ghi sổ). Nếu
	// module này tính luôn, sẽ có hai nơi cùng tính một con số.
	GetCommissionRate(ctx context.Context, sellerID string) (int32, error)

	// ---- Kiểm tra quyền ----

	// CanSellerCreateOffer kiểm tra seller có được tạo offer cho SKU không.
	//
	// HÀNG RÀO CHỐNG HÀNG GIẢ: rủi ro hàng giả là rủi ro SỐNG CÒN của
	// marketplace thời trang. Trả kèm lý do để giao diện hiển thị hành
	// động cụ thể ("Tải lên giấy ủy quyền").
	CanSellerCreateOffer(ctx context.Context, sellerID, skuID string) (allowed bool, reason string, err error)
}

// ---------------------------------------------------------------- DTO

// OfferView là dữ liệu offer cho module khác — CHỈ ĐỌC.
//
// KHÔNG chứa số lượng tồn kho: nguồn sự thật là InventoryItem, và hai nơi
// cùng lưu một sự thật thì sớm muộn chúng lệch nhau.
type OfferView struct {
	ID       string
	SKUID    string
	SellerID string

	// PriceAmount là số nguyên theo đơn vị nhỏ nhất của tiền tệ.
	PriceAmount   int64
	PriceCurrency string

	// CompareAtAmount = 0 nghĩa là không hiển thị giá gạch ngang.
	CompareAtAmount int64

	Condition         string
	HandlingTimeHours int
	MinOrderQuantity  int

	// MaxOrderQuantity = 0 nghĩa là không giới hạn.
	MaxOrderQuantity int

	Status string

	// IsSellable: khách đặt hàng được không.
	//
	// Chỉ ACTIVE mới bán được. OUT_OF_STOCK vẫn hiển thị (cho khách đăng
	// ký nhận thông báo) nhưng không đặt hàng được.
	IsSellable bool
}

// BuyBoxView là kết quả chọn buy box.
type BuyBoxView struct {
	Offer OfferView

	// Score là điểm của offer thắng, theo công thức CÔNG KHAI.
	//
	// Trả ra ngoài để seller hiểu vì sao mình không thắng và làm gì để cải
	// thiện. Mô hình hộp đen tạo tranh chấp không giải quyết được và cảm
	// giác bất công — dẫn tới seller rời nền tảng.
	Score int

	// OtherOffersCount là số offer khác cùng tranh.
	OtherOffersCount int
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound  = errNotFound{}
	ErrInvalidID = errInvalidID{}

	// ErrDuplicateActiveOffer khi seller đã có offer đang bán cho SKU này.
	ErrDuplicateActiveOffer = errDuplicate{}

	// ErrNotAuthorized khi seller không được bán thương hiệu của SKU này.
	ErrNotAuthorized = errNotAuthorized{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "marketplace: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "marketplace: định danh không hợp lệ" }

type errDuplicate struct{}

func (errDuplicate) Error() string {
	return "marketplace: nhà bán đã có offer đang bán cho SKU này"
}

type errNotAuthorized struct{}

func (errNotAuthorized) Error() string {
	return "marketplace: nhà bán không được phép bán thương hiệu này"
}

// ---------------------------------------------------------------- Tiền tố

const OfferIDPrefix = string(ids.PrefixOffer)
