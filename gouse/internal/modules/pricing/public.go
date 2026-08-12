// Package pricing là module quản lý giá: bảng giá của nền tảng, khung giá
// ràng buộc seller, và lịch sử giá.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module. Module khác CHỈ được import package
// này — quy tắc R1 của cmd/archcheck cưỡng chế điều đó.
//
// RANH GIỚI VỚI MARKETPLACE (điểm dễ nhầm nhất):
//
//	marketplace.Offer.price  — giá seller ĐẶT RA, là nguồn sự thật
//	pricing                  — giá của NỀN TẢNG cho own brand,
//	                           + khung giá ràng buộc seller
//
// pricing KHÔNG quyết định giá của seller. Nó chỉ trả lời "giá này có được
// chấp nhận không". Xem docs/04-modules/pricing.md mục 3.
package pricing

import (
	"context"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

// API là hợp đồng công khai của module pricing.
//
// Chữ ký khớp docs/04-modules/pricing.md mục 7.
type API interface {
	// GetPrice tra giá áp dụng cho một SKU trong một ngữ cảnh.
	//
	// CHỈ MỘT loại giá được áp dụng (quy tắc 2) — không cộng dồn. Giảm giá
	// thêm bằng mã khuyến mãi thuộc module promotion và áp dụng SAU.
	GetPrice(ctx context.Context, req PriceRequest) (*PriceResult, error)

	// GetPrices tra giá theo LÔ để tránh N+1.
	//
	// Hiển thị 50 sản phẩm cần 1 lời gọi, không phải 50. SKU không có giá
	// áp dụng được bị bỏ qua thay vì làm hỏng cả lời gọi.
	GetPrices(ctx context.Context, reqs []PriceRequest) (map[string]PriceResult, error)

	// GetPriceConstraint lấy khung giá ràng buộc seller của một SKU.
	GetPriceConstraint(ctx context.Context, skuID string) (*PriceConstraint, error)

	// ValidateSellerPrice kiểm tra giá seller có được chấp nhận không.
	//
	// Module marketplace gọi TRƯỚC khi cho seller lưu offer — đây là hàng
	// rào chống bán phá giá VÀ chống lỗi nhập liệu (gõ thiếu một số 0).
	ValidateSellerPrice(ctx context.Context, skuID string, amount int64, currency string) (PriceCheck, error)

	// GetPriceHistory lấy lịch sử giá trong một khoảng thời gian.
	GetPriceHistory(ctx context.Context, skuID string, period DateRange) ([]PricePoint, error)

	// GetLowestPriceLast30Days trả giá thấp nhất 30 ngày qua.
	//
	// Con số này BẮT BUỘC công bố ở một số thị trường khi quảng cáo giảm
	// giá. Trả về false nếu chưa có dữ liệu.
	GetLowestPriceLast30Days(ctx context.Context, skuID string) (Amount, bool, error)
}

// ---------------------------------------------------------------- DTO

// PriceRequest là ngữ cảnh tra giá.
type PriceRequest struct {
	SKUID string

	// CustomerTier để áp giá thành viên. Rỗng = khách vãng lai.
	CustomerTier string

	// CampaignID để áp giá chiến dịch. Rỗng = không đến từ chiến dịch nào.
	CampaignID string
}

// Amount là số tiền kèm đơn vị.
//
// Value là số nguyên theo đơn vị nhỏ nhất của tiền tệ (VND: đồng; USD: cent).
// KHÔNG dùng số thực cho tiền — sai số dấu phẩy động tích lũy thành lệch
// đối soát, và lệch đối soát là lỗi phải điều tra thủ công từng đơn.
type Amount struct {
	Value    int64
	Currency string
}

// PriceResult là kết quả tra giá.
type PriceResult struct {
	SKUID string

	// Amount là giá khách phải trả.
	Amount Amount

	// CompareAt là giá gạch ngang. Value = 0 nghĩa là không hiển thị.
	CompareAt Amount

	// PriceType cho biết loại giá đang áp dụng:
	// BASE, MEMBER, CLEARANCE, CAMPAIGN, FLASH.
	PriceType string

	// DiscountBasisPoints là mức giảm theo phần vạn (1000 = 10%).
	//
	// Dùng phần vạn thay vì phần trăm số thực để mọi trang hiển thị cùng
	// một con số.
	DiscountBasisPoints int64
}

// PriceConstraint là khung giá ràng buộc seller.
type PriceConstraint struct {
	SKUID string

	// MinPrice và MaxPrice có Value = 0 nghĩa là không giới hạn phía đó.
	MinPrice Amount
	MaxPrice Amount
}

// PriceCheck là kết quả kiểm tra giá seller.
//
// Chứa CẢ lý do lẫn khung giá, không chỉ true/false — seller cần biết
// "giá phải từ 80.000đ đến 200.000đ" để sửa.
type PriceCheck struct {
	Allowed bool

	// Code là mã máy đọc được: BELOW_MINIMUM, ABOVE_MAXIMUM,
	// CURRENCY_MISMATCH, PRICE_NOT_POSITIVE, SUSPICIOUS_PRICE.
	Code string

	Message string

	// NeedsReview đánh dấu giá cần người rà soát dù VẪN được chấp nhận.
	//
	// Giá lệch xa thị trường thường là lỗi nhập liệu, nhưng cũng có thể là
	// hàng thanh lý thật — chặn thẳng sẽ cản trở việc bán hàng hợp lệ.
	NeedsReview bool

	MinPrice Amount
	MaxPrice Amount
}

// PricePoint là một điểm trong lịch sử giá.
type PricePoint struct {
	SKUID     string
	PriceType string
	Amount    Amount
	CompareAt Amount

	// Reason là lý do thay đổi: INITIAL, MANUAL, CAMPAIGN, SEASON_END,
	// CLEARANCE, COST_CHANGE, COMPETITOR_MATCH.
	Reason string

	// RecordedAt là thời điểm ghi nhận, định dạng RFC3339.
	RecordedAt string
}

// DateRange là khoảng thời gian truy vấn lịch sử.
//
// Dùng chuỗi RFC3339 thay vì time.Time để hợp đồng công khai không phụ
// thuộc kiểu dữ liệu nội bộ. Rỗng nghĩa là không giới hạn phía đó.
type DateRange struct {
	From string
	To   string
}

// ---------------------------------------------------------------- Lỗi

// Các lỗi mà module khác có thể so sánh bằng errors.Is.
var (
	// ErrNotFound khi không tìm thấy tài nguyên.
	ErrNotFound = errNotFound{}
	// ErrInvalidID khi định danh sai định dạng.
	ErrInvalidID = errInvalidID{}
	// ErrNoPrice khi SKU chưa có mức giá nào áp dụng được.
	//
	// Khác ErrNotFound: SKU có thể có giá nhưng tất cả đã hết hạn hoặc
	// không khớp ngữ cảnh khách.
	ErrNoPrice = errNoPrice{}
)

type errNotFound struct{}

func (errNotFound) Error() string { return "pricing: không tìm thấy" }

type errInvalidID struct{}

func (errInvalidID) Error() string { return "pricing: định danh không hợp lệ" }

type errNoPrice struct{}

func (errNoPrice) Error() string { return "pricing: không có mức giá nào áp dụng được" }

// ---------------------------------------------------------------- Tiền tố ID

const (
	PriceIDPrefix      = string(ids.PrefixPrice)
	ConstraintIDPrefix = string(ids.PrefixPriceConstraint)
)
