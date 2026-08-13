// Package application chứa các use case của module marketplace.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/kernel/types"
	"github.com/fashion-commerce/platform/internal/modules/marketplace/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// CatalogPort là những gì marketplace CẦN từ catalog.
//
// Định nghĩa ở phía BÊN GỌI, chỉ khai báo đúng năng lực thực dùng — không
// phụ thuộc toàn bộ API của catalog.
type CatalogPort interface {
	// CanSellerSellBrand kiểm tra seller có được bán thương hiệu không.
	//
	// Đây là HÀNG RÀO CHỐNG HÀNG GIẢ. Quy tắc nằm ở catalog (nơi sở hữu dữ
	// liệu ủy quyền); marketplace chỉ hỏi và tuân theo.
	CanSellerSellBrand(ctx context.Context, brandID, sellerID ids.ID) (allowed bool, reason string, err error)
}

// ProductPort là những gì marketplace CẦN từ product.
type ProductPort interface {
	// BrandOfSKU tra thương hiệu của một SKU.
	//
	// Cần để biết kiểm tra ủy quyền với thương hiệu nào — offer gắn với
	// SKU, còn mức bảo vệ gắn với thương hiệu.
	BrandOfSKU(ctx context.Context, skuID ids.ID) (brandID ids.ID, found bool, err error)

	// IsSKUSellable cho biết mặt hàng còn được kinh doanh không.
	IsSKUSellable(ctx context.Context, skuID ids.ID) (bool, error)
}

// SellerPort là những gì marketplace CẦN từ seller.
type SellerPort interface {
	// IsActive cho biết nhà bán có đang hoạt động không.
	//
	// Seller bị đình chỉ thì offer không được thắng buy box, kể cả khi
	// giá tốt nhất (quy tắc 6).
	IsActive(ctx context.Context, sellerID ids.ID) (bool, error)

	// CommissionRate trả tỷ lệ hoa hồng của seller.
	CommissionRate(ctx context.Context, sellerID ids.ID) (types.BasisPoints, error)
}

// InventoryPort là những gì marketplace CẦN từ inventory.
type InventoryPort interface {
	// AvailableForSKUs trả số lượng khả dụng của nhiều SKU.
	//
	// Offer KHÔNG lưu số lượng: nguồn sự thật là InventoryItem. Buy box
	// chỉ chọn offer còn hàng (quy tắc 6).
	AvailableForSKUs(ctx context.Context, skuIDs []ids.ID) (map[ids.ID]int, error)
}

// NotAuthorizedError khi seller không được phép bán thương hiệu.
//
// Kiểu riêng vì nó mang theo LÝ DO — giao diện cần lý do để hiển thị hành
// động cụ thể ("Tải lên giấy ủy quyền") thay vì thông báo chung chung.
type NotAuthorizedError struct {
	SKUID    ids.ID
	SellerID ids.ID
	Reason   string
}

func (e *NotAuthorizedError) Error() string {
	return fmt.Sprintf("marketplace: seller %s không được bán SKU %s: %s",
		e.SellerID, e.SKUID, e.Reason)
}

// ErrNotAuthorized là mẫu để so sánh bằng errors.Is.
var ErrNotAuthorized = &NotAuthorizedError{}

func (e *NotAuthorizedError) Is(target error) bool {
	var t *NotAuthorizedError
	return errors.As(target, &t)
}

var (
	ErrSKUNotFound    = errors.New("marketplace: SKU không tồn tại")
	ErrSKUNotSellable = errors.New("marketplace: SKU không còn được kinh doanh")
	ErrSellerInactive = errors.New("marketplace: nhà bán không ở trạng thái hoạt động")
)

// Service là tầng application của module marketplace.
type Service struct {
	offers    domain.OfferRepository
	history   domain.PriceHistoryRepository
	catalog   CatalogPort
	product   ProductPort
	seller    SellerPort
	inventory InventoryPort
	weights   domain.BuyBoxWeights
	clock     Clock
}

type Deps struct {
	Offers    domain.OfferRepository
	History   domain.PriceHistoryRepository
	Catalog   CatalogPort
	Product   ProductPort
	Seller    SellerPort
	Inventory InventoryPort
	Weights   domain.BuyBoxWeights
	Clock     Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	w := d.Weights
	if w.Price == 0 && w.Handling == 0 && w.Performance == 0 {
		w = domain.DefaultWeights
	}
	return &Service{
		offers:    d.Offers,
		history:   d.History,
		catalog:   d.Catalog,
		product:   d.Product,
		seller:    d.Seller,
		inventory: d.Inventory,
		weights:   w,
		clock:     clock,
	}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Tạo offer

// CreateOfferInput là dữ liệu tạo offer.
type CreateOfferInput struct {
	SKUID             ids.ID
	SellerID          ids.ID
	Price             money.Money
	CompareAt         money.Money
	Condition         domain.Condition
	HandlingTimeHours int
	MinOrderQuantity  int
	MaxOrderQuantity  int

	// Activate = true thì đưa lên bán ngay sau khi tạo.
	Activate bool
}

// CreateOffer tạo offer mới sau khi kiểm tra MỌI điều kiện.
//
// Thứ tự kiểm tra có chủ đích — kiểm tra rẻ và chặn nhiều nhất trước:
//  1. SKU tồn tại và còn kinh doanh
//  2. Seller đang hoạt động
//  3. HÀNG RÀO CHỐNG HÀNG GIẢ: seller có được bán thương hiệu này không
//
// Bước 3 là quan trọng nhất. Rủi ro hàng giả là rủi ro SỐNG CÒN của
// marketplace thời trang (mục 5 của đặc tả).
func (s *Service) CreateOffer(ctx context.Context, in CreateOfferInput) (*domain.Offer, error) {
	if err := s.CheckCanCreateOffer(ctx, in.SellerID, in.SKUID); err != nil {
		return nil, err
	}

	// Báo lỗi rõ ràng TRƯỚC khi database từ chối vì trùng. Ràng buộc
	// UNIQUE vẫn là chốt chặn thật cho trường hợp hai request đồng thời.
	if existing, err := s.offers.FindActiveForSellerSKU(ctx, in.SellerID, in.SKUID); err == nil && existing != nil {
		return nil, domain.ErrDuplicateActiveOffer
	} else if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	now := s.clock.Now()
	o, err := domain.NewOffer(domain.NewOfferParams{
		SKUID:             in.SKUID,
		SellerID:          in.SellerID,
		Price:             in.Price,
		CompareAt:         in.CompareAt,
		Condition:         in.Condition,
		HandlingTimeHours: in.HandlingTimeHours,
		MinOrderQuantity:  in.MinOrderQuantity,
		MaxOrderQuantity:  in.MaxOrderQuantity,
		Now:               now,
	})
	if err != nil {
		return nil, err
	}

	if in.Activate {
		if err := o.Activate(now); err != nil {
			return nil, err
		}
	}

	if err := s.offers.Save(ctx, o); err != nil {
		return nil, err
	}

	// Quy tắc 5: lưu lịch sử MỌI lần đặt/đổi giá. Ghi ở đây chứ không để
	// bên gọi tự nhớ — nếu để bên gọi, sẽ có chỗ quên.
	if err := s.recordPrice(ctx, o, in.SellerID, now); err != nil {
		return nil, err
	}
	return o, nil
}

// CheckCanCreateOffer kiểm tra seller có được tạo offer cho SKU này không.
//
// Tách riêng để Seller Center hỏi TRƯỚC khi người dùng nhập cả biểu mẫu —
// báo lỗi sau khi họ điền xong là trải nghiệm tệ.
func (s *Service) CheckCanCreateOffer(ctx context.Context, sellerID, skuID ids.ID) error {
	sellable, err := s.product.IsSKUSellable(ctx, skuID)
	if err != nil {
		return fmt.Errorf("kiểm tra SKU: %w", err)
	}
	if !sellable {
		return ErrSKUNotSellable
	}

	active, err := s.seller.IsActive(ctx, sellerID)
	if err != nil {
		return fmt.Errorf("kiểm tra nhà bán: %w", err)
	}
	if !active {
		return ErrSellerInactive
	}

	// HÀNG RÀO CHỐNG HÀNG GIẢ.
	brandID, found, err := s.product.BrandOfSKU(ctx, skuID)
	if err != nil {
		return fmt.Errorf("tra thương hiệu của SKU: %w", err)
	}
	if !found {
		return ErrSKUNotFound
	}

	allowed, reason, err := s.catalog.CanSellerSellBrand(ctx, brandID, sellerID)
	if err != nil {
		return fmt.Errorf("kiểm tra quyền bán thương hiệu: %w", err)
	}
	if !allowed {
		return &NotAuthorizedError{SKUID: skuID, SellerID: sellerID, Reason: reason}
	}
	return nil
}

// ---------------------------------------------------------------- Sửa offer

// UpdatePrice đổi giá offer và GHI LỊCH SỬ.
//
// Quy tắc 5: lưu lịch sử mọi lần đổi giá. Cần cho việc phát hiện thao túng
// giá (tăng rồi giảm để giả vờ khuyến mãi).
func (s *Service) UpdatePrice(
	ctx context.Context, offerID ids.ID, price, compareAt money.Money, changedBy ids.ID,
) (*domain.Offer, error) {
	o, err := s.offers.FindByID(ctx, offerID)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	if err := o.ChangePrice(price, compareAt, now); err != nil {
		return nil, err
	}
	if err := s.offers.Save(ctx, o); err != nil {
		return nil, err
	}
	if err := s.recordPrice(ctx, o, changedBy, now); err != nil {
		return nil, err
	}
	return o, nil
}

// ActivateOffer đưa offer lên bán.
//
// Kiểm tra LẠI quyền bán: giữa lúc tạo nháp và lúc đưa lên bán có thể đã
// nhiều ngày, và giấy ủy quyền có thể đã hết hạn.
func (s *Service) ActivateOffer(ctx context.Context, offerID ids.ID) (*domain.Offer, error) {
	o, err := s.offers.FindByID(ctx, offerID)
	if err != nil {
		return nil, err
	}
	if err := s.CheckCanCreateOffer(ctx, o.SellerID(), o.SKUID()); err != nil {
		return nil, err
	}
	return s.change(ctx, offerID, func(o *domain.Offer, now time.Time) error {
		return o.Activate(now)
	})
}

// SuspendOffer đình chỉ offer.
func (s *Service) SuspendOffer(ctx context.Context, offerID ids.ID) (*domain.Offer, error) {
	return s.change(ctx, offerID, func(o *domain.Offer, now time.Time) error {
		return o.Suspend(now)
	})
}

// ArchiveOffer ngừng bán vĩnh viễn.
func (s *Service) ArchiveOffer(ctx context.Context, offerID ids.ID) (*domain.Offer, error) {
	return s.change(ctx, offerID, func(o *domain.Offer, now time.Time) error {
		return o.Archive(now)
	})
}

// SuspendOffersOfSeller ẩn TOÀN BỘ offer của một seller.
//
// Quy tắc 4: seller bị đình chỉ → mọi offer ẩn.
//
// LƯU Ý: việc này KHÔNG hủy đơn đang xử lý. Đơn khách đã trả tiền phải
// được hoàn tất hoặc hủy có kiểm soát kèm hoàn tiền — đó là việc của
// module order, không phải ở đây.
func (s *Service) SuspendOffersOfSeller(ctx context.Context, sellerID ids.ID) (int, error) {
	offers, err := s.offers.FindBySeller(ctx, sellerID, 1000, 0)
	if err != nil {
		return 0, err
	}

	now := s.clock.Now()
	count := 0
	for _, o := range offers {
		// Chỉ đụng tới offer đang bán hoặc hết hàng; đã lưu trữ thì bỏ qua.
		if o.Status() != domain.StatusActive && o.Status() != domain.StatusOutOfStock {
			continue
		}
		if err := o.Suspend(now); err != nil {
			continue
		}
		if err := s.offers.Save(ctx, o); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// MarkOutOfStock đánh dấu offer hết hàng.
//
// Do event `inventory.depleted` kích hoạt — số lượng tồn kho là sự thật
// của module inventory, không phải của marketplace.
func (s *Service) MarkOutOfStock(ctx context.Context, offerID ids.ID) (*domain.Offer, error) {
	return s.change(ctx, offerID, func(o *domain.Offer, now time.Time) error {
		return o.MarkOutOfStock(now)
	})
}

// MarkBackInStock đưa offer trở lại bán sau khi có hàng.
func (s *Service) MarkBackInStock(ctx context.Context, offerID ids.ID) (*domain.Offer, error) {
	return s.change(ctx, offerID, func(o *domain.Offer, now time.Time) error {
		return o.MarkBackInStock(now)
	})
}

// ---------------------------------------------------------------- Buy box

// GetBuyBox chọn offer hiển thị mặc định cho một SKU.
//
// RÀNG BUỘC BẮT BUỘC (quy tắc 6): chỉ chọn offer còn hàng, seller active,
// offer active. Việc lọc và tính điểm do domain.SelectBuyBox quyết định.
func (s *Service) GetBuyBox(ctx context.Context, skuID ids.ID) (domain.BuyBoxResult, error) {
	all, err := s.GetBuyBoxes(ctx, []ids.ID{skuID})
	if err != nil {
		return domain.BuyBoxResult{}, err
	}
	return all[skuID], nil
}

// GetBuyBoxes chọn buy box cho NHIỀU SKU trong một lần gọi.
//
// Trang danh sách 50 sản phẩm cần giá buy box của từng cái — phải là vài
// truy vấn, không phải 50 lần lặp.
func (s *Service) GetBuyBoxes(
	ctx context.Context, skuIDs []ids.ID,
) (map[ids.ID]domain.BuyBoxResult, error) {
	out := make(map[ids.ID]domain.BuyBoxResult, len(skuIDs))
	if len(skuIDs) == 0 {
		return out, nil
	}

	offersBySKU, err := s.offers.FindBySKUs(ctx, skuIDs)
	if err != nil {
		return nil, err
	}

	// Tồn kho theo lô: offer KHÔNG lưu số lượng, nguồn sự thật là inventory.
	available := map[ids.ID]int{}
	if s.inventory != nil {
		available, err = s.inventory.AvailableForSKUs(ctx, skuIDs)
		if err != nil {
			return nil, fmt.Errorf("tra tồn kho: %w", err)
		}
	}

	// Trạng thái seller: hỏi MỘT LẦN cho mỗi seller, không hỏi lại cho
	// từng offer của cùng seller.
	sellerActive := map[ids.ID]bool{}

	for _, skuID := range skuIDs {
		offers := offersBySKU[skuID]
		if len(offers) == 0 {
			continue
		}

		// Hết hàng thì KHÔNG offer nào thắng buy box, kể cả offer ACTIVE:
		// hiển thị "mua ngay" rồi báo hết hàng ở bước thanh toán là trải
		// nghiệm tệ nhất.
		if s.inventory != nil && available[skuID] <= 0 {
			continue
		}

		candidates := make([]domain.BuyBoxCandidate, 0, len(offers))
		for _, o := range offers {
			sellerID := o.SellerID()
			active, known := sellerActive[sellerID]
			if !known {
				if s.seller != nil {
					active, err = s.seller.IsActive(ctx, sellerID)
					if err != nil {
						return nil, fmt.Errorf("kiểm tra nhà bán: %w", err)
					}
				} else {
					active = true
				}
				sellerActive[sellerID] = active
			}

			candidates = append(candidates, domain.BuyBoxCandidate{
				Offer:        o,
				SellerActive: active,
				// Chưa có module chấm điểm hiệu suất (Phase 2).
				PerformanceScore: domain.DefaultPerformanceScore,
			})
		}

		if res := domain.SelectBuyBox(candidates, s.weights); res.Winner != nil {
			out[skuID] = res
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- Đọc

func (s *Service) GetOffer(ctx context.Context, id ids.ID) (*domain.Offer, error) {
	return s.offers.FindByID(ctx, id)
}

func (s *Service) GetOffersBySKU(ctx context.Context, skuID ids.ID) ([]*domain.Offer, error) {
	return s.offers.FindBySKU(ctx, skuID)
}

func (s *Service) GetOffersBySKUs(
	ctx context.Context, skuIDs []ids.ID,
) (map[ids.ID][]*domain.Offer, error) {
	return s.offers.FindBySKUs(ctx, skuIDs)
}

// GetOffersByIDs lấy nhiều offer theo định danh.
func (s *Service) GetOffersByIDs(
	ctx context.Context, offerIDs []ids.ID,
) (map[ids.ID]*domain.Offer, error) {
	return s.offers.FindByIDs(ctx, offerIDs)
}

// GetOffersBySeller lấy offer của MỘT seller.
//
// BẢO MẬT: sellerID bắt buộc. Thiếu thì trả lỗi thay vì trả offer của mọi
// seller — một lỗi lập trình ở tầng gọi sẽ thành rò rỉ dữ liệu toàn sàn.
func (s *Service) GetOffersBySeller(
	ctx context.Context, sellerID ids.ID, limit, offset int,
) ([]*domain.Offer, error) {
	if sellerID.IsZero() {
		return nil, errors.New("marketplace: bắt buộc phải có định danh nhà bán")
	}
	return s.offers.FindBySeller(ctx, sellerID, limit, offset)
}

// GetCommissionRate trả TỶ LỆ hoa hồng, KHÔNG tính số tiền.
//
// Quy tắc 8 (mục 11). Phân vai rõ ràng (mục 2):
//
//	marketplace → ĐỊNH NGHĨA quy tắc
//	order       → ĐÓNG BĂNG vào OrderLine tại thời điểm đặt hàng
//	payment     → GHI SỔ vào ledger
//
// Nếu module này tính luôn số tiền, nó lấn sang việc của order và payment,
// và sẽ có hai nơi cùng tính một con số.
func (s *Service) GetCommissionRate(
	ctx context.Context, sellerID ids.ID,
) (types.BasisPoints, error) {
	return s.seller.CommissionRate(ctx, sellerID)
}

func (s *Service) GetPriceHistory(
	ctx context.Context, offerID ids.ID, limit int,
) ([]*domain.PricePoint, error) {
	return s.history.FindByOffer(ctx, offerID, limit)
}

// ---------------------------------------------------------------- Nội bộ

func (s *Service) change(
	ctx context.Context, offerID ids.ID, apply func(*domain.Offer, time.Time) error,
) (*domain.Offer, error) {
	o, err := s.offers.FindByID(ctx, offerID)
	if err != nil {
		return nil, err
	}
	if err := apply(o, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.offers.Save(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

func (s *Service) recordPrice(
	ctx context.Context, o *domain.Offer, changedBy ids.ID, now time.Time,
) error {
	point, err := domain.NewPricePoint(o, changedBy, now)
	if err != nil {
		return err
	}
	return s.history.Append(ctx, point)
}
