package domain

import (
	"context"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
)

// OfferRepository là PORT cho offer.
type OfferRepository interface {
	Save(ctx context.Context, o *Offer) error

	FindByID(ctx context.Context, id ids.ID) (*Offer, error)

	// FindBySKU lấy MỌI offer của một SKU, kể cả đã ngừng bán.
	//
	// Việc lọc ứng viên buy box là quyết định NGHIỆP VỤ (SelectBuyBox),
	// không phải của kho lưu trữ — nếu kho tự lọc, quy tắc sẽ nằm rải rác
	// ở cả SQL lẫn domain.
	FindBySKU(ctx context.Context, skuID ids.ID) ([]*Offer, error)

	// FindBySKUs nhận DANH SÁCH để tránh N+1.
	//
	// Trang danh sách 50 sản phẩm cần biết giá buy box của từng cái —
	// phải là 1 truy vấn, không phải 50.
	FindBySKUs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID][]*Offer, error)

	// FindByIDs lấy nhiều offer theo định danh, tránh N+1.
	//
	// Module cart giữ offer_id của từng món, nên hiển thị giỏ 10 món cần
	// đúng hàm này — một truy vấn, không phải mười.
	FindByIDs(ctx context.Context, offerIDs []ids.ID) (map[ids.ID]*Offer, error)

	// FindBySeller lấy offer của MỘT seller.
	//
	// BẢO MẬT: seller không bao giờ được thấy offer của seller khác (quy
	// tắc 7 của module seller). Lọc phải nằm trong truy vấn.
	FindBySeller(ctx context.Context, sellerID ids.ID, limit, offset int) ([]*Offer, error)

	// FindActiveForSellerSKU tìm offer ACTIVE của một seller cho một SKU.
	//
	// Dùng để báo lỗi rõ ràng trước khi database từ chối vì trùng.
	FindActiveForSellerSKU(ctx context.Context, sellerID, skuID ids.ID) (*Offer, error)
}

// PriceHistoryRepository là PORT cho lịch sử giá offer.
//
// CHỈ CÓ GHI THÊM VÀ ĐỌC. Ở tầng database còn có trigger chặn UPDATE/DELETE
// — lịch sử sửa được thì không phát hiện được thao túng giá.
type PriceHistoryRepository interface {
	Append(ctx context.Context, p *PricePoint) error
	FindByOffer(ctx context.Context, offerID ids.ID, limit int) ([]*PricePoint, error)
}

// PricePoint là một điểm trong lịch sử giá offer — BẤT BIẾN.
type PricePoint struct {
	ID         ids.ID
	OfferID    ids.ID
	SKUID      ids.ID
	SellerID   ids.ID
	Price      money.Money
	CompareAt  money.Money
	ChangedBy  ids.ID
	RecordedAt time.Time
}

// NewPricePoint ghi một điểm lịch sử giá.
func NewPricePoint(o *Offer, changedBy ids.ID, now time.Time) (*PricePoint, error) {
	id, err := ids.New(ids.PrefixOffer)
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return &PricePoint{
		ID:         id,
		OfferID:    o.ID(),
		SKUID:      o.SKUID(),
		SellerID:   o.SellerID(),
		Price:      o.Price(),
		CompareAt:  o.CompareAt(),
		ChangedBy:  changedBy,
		RecordedAt: now,
	}, nil
}
