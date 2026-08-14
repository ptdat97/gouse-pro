// Package promotion quản lý khuyến mãi và mã giảm giá.
//
// ĐÂY LÀ ĐIỂM VÀO DUY NHẤT của module — quy tắc R1 của cmd/archcheck.
//
// # Vấn đề cốt lõi: ai chịu chi phí khuyến mãi
//
//	Khách dùng mã giảm 50.000đ cho đơn của Seller A. AI CHỊU 50.000đ?
//
//	PLATFORM  nền tảng chịu, trừ vào chi phí marketing
//	SELLER    seller chịu, trừ vào số tiền seller nhận
//	SHARED    chia theo tỷ lệ thỏa thuận            (Phase 2)
//
// Không trả lời được câu này thì không tính được seller thực nhận bao
// nhiêu, và đối soát cuối tháng sẽ lệch đúng bằng tổng tiền khuyến mãi.
// Vì vậy mọi kết quả tính giảm giá đều kèm CostAllocations.
//
// # Điều module này KHÔNG làm
//
// Nó KHÔNG biết giá gốc (thuộc `pricing`, `marketplace`), KHÔNG biết phí
// vận chuyển, và KHÔNG ghi sổ chi phí khuyến mãi (thuộc `payment`). Nó
// nhận vào một số tiền và trả về nên giảm bao nhiêu, ai chịu.
//
// # Phạm vi MVP
//
// Mã giảm giá cơ bản và miễn phí ship theo ngưỡng. Khuyến mãi tự động,
// combo/outfit, mua X tặng Y thuộc Phase 2 trở đi — xem
// docs/04-modules/promotion.md mục 11.
package promotion

import (
	"context"
	"errors"
	"time"
)

// API là hợp đồng công khai của module promotion.
type API interface {
	// ValidateCoupon kiểm tra mã và tính số tiền giảm.
	//
	// KHÔNG ghi nhận sử dụng — đây là đường ĐỌC, gọi mỗi lần khách gõ mã
	// vào giỏ hàng. Ghi nhận là việc của RecordUsage.
	//
	// Tách hai việc vì khách có thể gõ mã rồi bỏ giỏ hàng. Ghi nhận ngay
	// lúc kiểm tra sẽ làm mã hết lượt vì những người chưa mua gì.
	ValidateCoupon(ctx context.Context, req ValidateRequest) (DiscountResult, error)

	// AllocateDiscount phân bổ số tiền giảm xuống từng dòng hàng THEO TỶ LỆ.
	//
	// Kết quả PHẢI được đóng băng vào đơn hàng: khi khách trả lại một món,
	// số tiền hoàn là giá dòng TRỪ phần giảm đã phân bổ cho nó. Không lưu
	// lại thì nền tảng hoàn nhiều hơn đã thu.
	//
	// Tổng các phần luôn bằng ĐÚNG số tiền giảm — phần dư của phép chia
	// được rải chứ không biến mất.
	AllocateDiscount(ctx context.Context, req AllocateRequest) ([]DiscountLineView, error)

	// RecordUsage ghi nhận một lượt sử dụng mã.
	//
	// IDEMPOTENT theo (mã, đơn hàng): gọi lại KHÔNG trừ ngân sách lần nữa
	// và KHÔNG báo lỗi. Handler event xử lý lại cùng một event là chuyện
	// bình thường.
	RecordUsage(ctx context.Context, req RecordUsageRequest) error

	// ReleaseUsage giải phóng lượt sử dụng khi đơn bị hủy.
	//
	// IDEMPOTENT như RecordUsage. Trả về số lượt đã giải phóng.
	//
	// Không giải phóng thì mã hết lượt vì những đơn không bao giờ thành —
	// và chiến dịch kết thúc sớm hơn dự tính.
	ReleaseUsage(ctx context.Context, orderID string) (int, error)

	// ---------------------------------------------------------- Quản lý

	// CreatePromotion tạo chương trình khuyến mãi.
	//
	// Trạng thái ban đầu là DRAFT — phải gọi ActivatePromotion mới áp
	// được. Một mã giảm 90% do gõ nhầm sẽ có hiệu lực tức thì nếu mặc
	// định là ACTIVE.
	CreatePromotion(ctx context.Context, req CreatePromotionRequest) (PromotionView, error)

	GetPromotion(ctx context.Context, promotionID string) (PromotionView, error)

	ActivatePromotion(ctx context.Context, promotionID string) error
	PausePromotion(ctx context.Context, promotionID string) error

	// CreateCoupon phát hành một mã cho chương trình khuyến mãi.
	//
	// CustomerID khác rỗng tạo mã RIÊNG: người khác biết mã cũng không
	// dùng được. Dùng cho mã xin lỗi sau sự cố, hoặc tặng khách VIP.
	CreateCoupon(ctx context.Context, req CreateCouponRequest) (CouponView, error)

	// ExpireDuePromotions chuyển các khuyến mãi quá hạn sang EXPIRED.
	//
	// Gọi từ worker chạy định kỳ. Không có nó, khuyến mãi hết hạn vẫn mang
	// trạng thái ACTIVE và chỉ bị chặn bởi phép so sánh thời gian — một
	// lớp bảo vệ thay vì hai.
	ExpireDuePromotions(ctx context.Context) (int, error)
}

// ---------------------------------------------------------------- DTO

// ValidateRequest là dữ liệu kiểm tra mã.
type ValidateRequest struct {
	// Code KHÔNG cần chuẩn hóa trước: module tự hạ chữ hoa và cắt khoảng
	// trắng. Khách gõ "sale10" và " SALE10 " phải ra cùng một mã.
	Code string

	// CustomerID để trống với khách vãng lai.
	//
	// Khách vãng lai vẫn dùng mã được, nhưng KHÔNG áp được giới hạn "mỗi
	// khách N lượt" — không có gì để đếm.
	CustomerID string

	// SellerID của đơn hàng, dùng cho mã riêng của gian hàng.
	SellerID string

	// OrderTotal là số nguyên đơn vị nhỏ nhất của tiền tệ.
	OrderTotal int64
	Currency   string
}

// AllocateRequest là dữ liệu phân bổ giảm giá xuống dòng hàng.
type AllocateRequest struct {
	Discount int64
	Currency string

	// Lines là các dòng hàng, theo đúng thứ tự.
	Lines []AllocateLine
}

// AllocateLine là một dòng hàng cần phân bổ.
type AllocateLine struct {
	// LineID là định danh do bên gọi cung cấp; module trả lại đúng chuỗi
	// này. Nó KHÔNG biết bảng order_line tồn tại.
	LineID string

	// Total là giá trị dòng hàng, dùng làm TỶ LỆ phân bổ.
	Total int64
}

// RecordUsageRequest là dữ liệu ghi nhận sử dụng.
type RecordUsageRequest struct {
	Code       string
	CustomerID string
	OrderID    string

	// Discount là số tiền đã giảm THẬT SỰ, lấy từ kết quả ValidateCoupon
	// lúc đặt hàng.
	//
	// KHÔNG tính lại ở đây: cấu hình khuyến mãi có thể đã đổi giữa lúc
	// khách đặt và lúc event được xử lý, và khi đó số ghi vào sổ sẽ khác
	// số khách đã trả (nguyên tắc P9 — đóng băng dữ liệu giao dịch).
	Discount int64
	Currency string
}

// CreatePromotionRequest là dữ liệu tạo khuyến mãi.
type CreatePromotionRequest struct {
	Name        string
	Description string

	// Kind: COUPON, FREE_SHIPPING. Các loại khác chưa cài đặt ở MVP.
	Kind string

	// DiscountType: PERCENTAGE, FIXED, FREE_SHIP.
	DiscountType string

	// DiscountBPS là điểm cơ bản: 1000 = 10%. Dùng với PERCENTAGE.
	DiscountBPS int32

	// DiscountAmount dùng với FIXED.
	DiscountAmount int64

	// MaxDiscountAmount là CHẶN TRÊN cho giảm theo phần trăm.
	//
	// "Giảm 50%, tối đa 100.000đ". 0 = không giới hạn — nhưng với
	// PERCENTAGE thì bỏ trống nghĩa là một đơn 10 triệu được giảm 5 triệu.
	MaxDiscountAmount int64

	// MinOrderAmount là ngưỡng giá trị đơn tối thiểu.
	//
	// Đây là điều kiện của MIỄN PHÍ SHIP THEO NGƯỠNG.
	MinOrderAmount int64

	// CostBearer: PLATFORM, SELLER, SHARED. Bỏ trống = PLATFORM.
	CostBearer string

	// Tỷ lệ chia khi CostBearer = SHARED. Phải cộng đúng 10000.
	PlatformShareBPS int32
	SellerShareBPS   int32

	// SellerID khác rỗng nghĩa là khuyến mãi CHỈ áp cho gian hàng đó.
	SellerID string

	// MaxUses là số lượt tối đa TOÀN CỤC. 0 = không giới hạn.
	MaxUses int

	// MaxUsesPerCustomer là số lượt tối đa MỖI KHÁCH. 0 = không giới hạn.
	//
	// Thiếu nó thì một người dùng mã "giảm 100k cho khách mới" được vô số
	// lần bằng cách tạo đơn liên tục.
	MaxUsesPerCustomer int

	// MaxBudget là tổng số tiền tối đa được giảm. 0 = không giới hạn.
	//
	// Khác MaxUses: một mã giảm 10% không biết trước mỗi lượt tốn bao
	// nhiêu, nên giới hạn theo lượt KHÔNG chặn được chi phí.
	MaxBudget int64

	StartsAt time.Time
	EndsAt   time.Time

	Currency string
}

// CreateCouponRequest là dữ liệu phát hành mã.
type CreateCouponRequest struct {
	PromotionID string
	Code        string

	// CustomerID khác rỗng tạo mã RIÊNG cho một khách.
	CustomerID string
}

// DiscountResult là kết quả tính giảm giá.
type DiscountResult struct {
	CouponID    string
	PromotionID string

	// Discount là số tiền giảm cho hàng hóa.
	//
	// KHÔNG bao gồm phí vận chuyển — xem FreeShipping.
	Discount int64
	Currency string

	// FreeShipping cho biết mã này miễn phí vận chuyển.
	//
	// TÁCH KHỎI Discount có chủ ý: phí vận chuyển do module khác tính, và
	// promotion không biết nó là bao nhiêu. Bên gọi tự trừ.
	FreeShipping bool

	// CostAllocations là ai chịu bao nhiêu.
	//
	// PHẢI được đóng băng vào đơn hàng. Tỷ lệ chia có thể đổi khi thỏa
	// thuận với seller thay đổi; nếu đối soát đọc tỷ lệ HIỆN TẠI thay vì
	// tỷ lệ lúc bán, số tiền seller đã nhận tháng trước sẽ tính ra khác.
	CostAllocations []CostAllocationView
}

// CostAllocationView là phần chi phí một bên phải chịu.
type CostAllocationView struct {
	// Bearer: PLATFORM hoặc SELLER.
	Bearer string

	// SellerID chỉ có nghĩa khi Bearer là SELLER.
	SellerID string

	Amount int64
}

// DiscountLineView là phần giảm giá phân bổ cho MỘT dòng hàng.
type DiscountLineView struct {
	LineID   string
	Discount int64
}

// PromotionView là chương trình khuyến mãi trả ra ngoài.
type PromotionView struct {
	ID          string
	Name        string
	Description string

	Kind         string
	DiscountType string

	DiscountBPS       int32
	DiscountAmount    int64
	MaxDiscountAmount int64
	MinOrderAmount    int64

	CostBearer       string
	PlatformShareBPS int32
	SellerShareBPS   int32
	SellerID         string

	MaxUses            int
	MaxUsesPerCustomer int
	UsedCount          int

	MaxBudget  int64
	UsedBudget int64

	// Status: DRAFT, ACTIVE, PAUSED, EXHAUSTED, EXPIRED.
	Status string

	StartsAt time.Time
	EndsAt   time.Time

	Currency string
}

// CouponView là mã giảm giá trả ra ngoài.
type CouponView struct {
	ID          string
	PromotionID string
	Code        string

	// CustomerID khác rỗng nghĩa là mã riêng của một khách.
	CustomerID string

	UsedCount int
	Active    bool
}

// ---------------------------------------------------------------- Lỗi

var (
	ErrNotFound = errors.New("promotion: không tìm thấy khuyến mãi")

	ErrCouponNotFound = errors.New("promotion: mã giảm giá không tồn tại")

	// ErrCouponInactive là mã đã bị tắt.
	ErrCouponInactive = errors.New("promotion: mã giảm giá đã bị vô hiệu")

	ErrNotStarted = errors.New("promotion: khuyến mãi chưa bắt đầu")

	ErrExpired = errors.New("promotion: khuyến mãi đã hết hạn")

	ErrNotActive = errors.New("promotion: khuyến mãi không còn hiệu lực")

	// ErrBelowMinimum là đơn hàng chưa đạt giá trị tối thiểu.
	//
	// Bên gọi NÊN hiển thị còn thiếu bao nhiêu: "mua thêm 50.000đ để được
	// miễn phí vận chuyển" là câu làm tăng giá trị đơn hàng.
	ErrBelowMinimum = errors.New("promotion: đơn hàng chưa đạt giá trị tối thiểu")

	ErrUsageLimitReached = errors.New("promotion: mã giảm giá đã hết lượt sử dụng")

	ErrCustomerLimitReached = errors.New("promotion: bạn đã dùng hết lượt của mã này")

	ErrBudgetExhausted = errors.New("promotion: ngân sách khuyến mãi đã hết")

	// ErrWrongCustomer là mã riêng của khách khác.
	ErrWrongCustomer = errors.New("promotion: mã giảm giá không dành cho bạn")

	ErrWrongSeller = errors.New("promotion: mã giảm giá không áp dụng cho gian hàng này")

	ErrInvalidInput = errors.New("promotion: dữ liệu không hợp lệ")

	// ErrVersionConflict là khuyến mãi đã bị thao tác khác sửa.
	//
	// Với khuyến mãi đang chạy quảng cáo, xung đột là chuyện THƯỜNG XUYÊN
	// chứ không hiếm. Bên gọi nên ĐỌC LẠI rồi thử lại.
	ErrVersionConflict = errors.New("promotion: dữ liệu đã bị thay đổi bởi thao tác khác")
)

// ---------------------------------------------------------------- Hằng

// Loại khuyến mãi. MVP chỉ cài COUPON và FREE_SHIPPING.
const (
	KindCoupon       = "COUPON"
	KindFreeShipping = "FREE_SHIPPING"
)

// Cách tính giảm.
const (
	DiscountPercentage = "PERCENTAGE"
	DiscountFixed      = "FIXED"
	DiscountFreeShip   = "FREE_SHIP"
)

// Bên chịu chi phí.
const (
	BearerPlatform = "PLATFORM"
	BearerSeller   = "SELLER"
	BearerShared   = "SHARED"
)

// Trạng thái khuyến mãi.
const (
	StatusDraft     = "DRAFT"
	StatusActive    = "ACTIVE"
	StatusPaused    = "PAUSED"
	StatusExhausted = "EXHAUSTED"
	StatusExpired   = "EXPIRED"
)
