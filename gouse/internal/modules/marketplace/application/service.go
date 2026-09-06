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

	// SKUsOfProduct trả mọi SKU của một sản phẩm.
	//
	// Cần cho trang sản phẩm: khách xem một PRODUCT, nhưng offer gắn với
	// SKU (một tổ hợp màu/size cụ thể).
	SKUsOfProduct(ctx context.Context, productID ids.ID) ([]ids.ID, error)

	// SKUsOfProducts là bản THEO LÔ, cho trang danh sách.
	SKUsOfProducts(ctx context.Context, productIDs []ids.ID) (map[ids.ID][]ids.ID, error)
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
// OwnedOffer đọc offer và KIỂM TRA nó thuộc về seller đang gọi.
//
// # Vì sao quy tắc này nằm ở tầng application, không phải ở handler
//
// Nó được hỏi từ mọi đường GHI của seller: đổi giá, đưa lên bán, lưu trữ.
// Mỗi handler tự kiểm lại nghĩa là sớm muộn có một đường quên kiểm — và
// một đường quên là đủ để bất kỳ ai đổi giá offer của đối thủ về 1đ chỉ
// bằng cách đoán định danh.
//
// `GetOffer` KHÔNG lọc theo seller vì nó còn phục vụ trang sản phẩm công
// khai. Đó là lý do phải có hàm riêng này thay vì thêm điều kiện vào đó.
//
// Trả ErrNotFound cho CẢ offer không tồn tại lẫn offer của người khác:
// phân biệt hai trường hợp cho phép dò xem đối thủ đang bán những gì, mà
// định danh offer thì lộ ra ở trang công khai.
func (s *Service) OwnedOffer(
	ctx context.Context, offerID, sellerID ids.ID,
) (*domain.Offer, error) {
	o, err := s.offers.FindByID(ctx, offerID)
	if err != nil {
		return nil, err
	}
	if o.SellerID() != sellerID {
		return nil, domain.ErrNotFound
	}
	return o, nil
}

func (s *Service) UpdatePrice(
	ctx context.Context, offerID ids.ID, price, compareAt money.Money, changedBy ids.ID,
) (*domain.Offer, error) {
	// KHÔNG kiểm quyền sở hữu ở đây: `changedBy` là "AI SỬA" cho vết
	// kiểm toán, và quản trị viên cũng sửa giá được. Gộp nó làm khóa phân
	// quyền là lẫn hai khái niệm khác nhau.
	//
	// Đường của seller đi qua `OwnedOffer` trước — xem tầng interfaces.
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
		// Chỉ đụng tới offer đang bán; nháp, đã đình chỉ hay đã lưu trữ
		// thì bỏ qua.
		if o.Status() != domain.StatusActive {
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
	return s.buyBoxes(ctx, skuIDs, newSellerActiveCache(s.seller))
}

// buyBoxes nhận SẴN bộ nhớ đệm trạng thái nhà bán.
//
// `ListProductOffers` cần đúng những trạng thái nhà bán mà buy box vừa
// tra. Để mỗi bên tự tra là hỏi database hai lần cùng một câu trong cùng
// một request — và đây là đường nóng nhất của cửa hàng.
func (s *Service) buyBoxes(
	ctx context.Context, skuIDs []ids.ID, sellerActive *sellerActiveCache,
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

	for _, skuID := range skuIDs {
		offers := offersBySKU[skuID]
		if len(offers) == 0 {
			continue
		}

		// Hết hàng thì KHÔNG offer nào thắng buy box, kể cả offer ACTIVE:
		// hiển thị "mua ngay" rồi báo hết hàng ở bước thanh toán là trải
		// nghiệm tệ nhất.
		//
		// Điều kiện này nay đi vào ứng viên chứ không lọc trước vòng lặp:
		// `SelectBuyBox` phải thấy ĐỦ ba đầu vào để dùng chung
		// `CanCustomerBuy` với hai nơi còn lại.
		coHang := s.inventory == nil || available[skuID] > 0

		candidates := make([]domain.BuyBoxCandidate, 0, len(offers))
		for _, o := range offers {
			active, activeErr := sellerActive.get(ctx, o.SellerID())
			if activeErr != nil {
				return nil, activeErr
			}

			candidates = append(candidates, domain.BuyBoxCandidate{
				Offer:        o,
				SellerActive: active,
				InStock:      coHang,
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

// sellerActiveCache tra trạng thái nhà bán MỘT LẦN cho mỗi nhà bán.
//
// Một sản phẩm 12 tổ hợp màu/size của cùng một nhà bán là 12 lượt hỏi cho
// một câu trả lời. Bộ nhớ đệm sống trong đúng một lời gọi — trạng thái nhà
// bán đổi giữa hai request thì request sau thấy giá trị mới.
//
// `seller` nil (test, hoặc bản dựng chưa nối module) thì coi như đang hoạt
// động: đó là hành vi đã có từ trước, giữ nguyên.
type sellerActiveCache struct {
	port  SellerPort
	known map[ids.ID]bool
}

func newSellerActiveCache(port SellerPort) *sellerActiveCache {
	return &sellerActiveCache{port: port, known: map[ids.ID]bool{}}
}

func (c *sellerActiveCache) get(ctx context.Context, sellerID ids.ID) (bool, error) {
	if active, ok := c.known[sellerID]; ok {
		return active, nil
	}
	active := true
	if c.port != nil {
		var err error
		active, err = c.port.IsActive(ctx, sellerID)
		if err != nil {
			return false, fmt.Errorf("kiểm tra nhà bán: %w", err)
		}
	}
	c.known[sellerID] = active
	return active, nil
}

// ---------------------------------------------------------------- Đọc

func (s *Service) GetOffer(ctx context.Context, id ids.ID) (*domain.Offer, error) {
	return s.offers.FindByID(ctx, id)
}

func (s *Service) GetOffersBySKU(ctx context.Context, skuID ids.ID) ([]*domain.Offer, error) {
	return s.offers.FindBySKU(ctx, skuID)
}

// ProductOffer là một offer kèm hai câu trả lời mà bản thân offer không
// tự trả lời được.
type ProductOffer struct {
	Offer    *domain.Offer
	IsBuyBox bool

	// IsSellable: khách BẤM MUA ĐƯỢC không.
	//
	// KHÁC `Offer.IsSellable()`, và khác một cách quan trọng: hàm kia chỉ
	// nhìn trạng thái offer, không biết gì về tồn kho — offer hết hàng vẫn
	// ở trạng thái ACTIVE, có chủ ý (P3-23: tồn kho là sự thật của
	// inventory, offer không chép lại).
	//
	// Cờ này là quy tắc ĐẦY ĐỦ — `domain.CanCustomerBuy`: offer đang bán
	// VÀ còn hàng VÀ nhà bán đang hoạt động. ĐÚNG hàm mà buy box dùng để
	// loại ứng viên, nên hai chỗ trong cùng một response không thể trả lời
	// khác nhau nữa. Cả hai vế đều đã lệch thật một lần: hết hàng (P3-23)
	// và nhà bán bị đình chỉ.
	IsSellable bool
}

// ListProductOffers trả các offer của một sản phẩm, đánh dấu offer thắng
// buy box.
//
// Khách xem một PRODUCT, nhưng offer gắn với SKU — nên hàm này gom SKU của
// sản phẩm rồi lấy offer của từng SKU.
//
// `skuID` khác rỗng thì chỉ lấy offer của đúng SKU đó (khách đã chọn một
// tổ hợp màu/size).
func (s *Service) ListProductOffers(
	ctx context.Context, productID, skuID ids.ID,
) ([]ProductOffer, error) {
	skuIDs := []ids.ID{skuID}
	if skuID.IsZero() {
		var err error
		skuIDs, err = s.product.SKUsOfProduct(ctx, productID)
		if err != nil {
			return nil, err
		}
	}
	if len(skuIDs) == 0 {
		return nil, nil
	}

	bySKU, err := s.offers.FindBySKUs(ctx, skuIDs)
	if err != nil {
		return nil, err
	}

	// Trạng thái nhà bán: đầu vào thứ ba của `CanCustomerBuy`, và cũng là
	// thứ buy box cần. Dựng bộ nhớ đệm ở đây rồi ĐƯA CHO buy box dùng
	// chung — mỗi bên tự tra là hỏi database hai lần cùng một câu.
	//
	// Trước đây chỉ buy box hỏi tới nó, nên đình chỉ một nhà bán làm nhãn
	// "Đề xuất" biến mất mà nút "Thêm vào giỏ" vẫn sáng.
	sellerActive := newSellerActiveCache(s.seller)

	// Buy box tính RIÊNG cho từng SKU — mỗi tổ hợp màu/size có người thắng
	// của nó — nhưng tra MỘT LẦN cho cả danh sách.
	//
	// Trước đây chỗ này gọi GetBuyBox bên trong vòng lặp SKU, và mỗi lần
	// gọi lại tra tồn kho cùng trạng thái nhà bán từ đầu: một sản phẩm 12
	// tổ hợp màu/size là 12 lượt đi-về cho dữ liệu lấy được bằng một lượt.
	// Bản theo lô đã có sẵn, chỉ là chưa dùng.
	boxes, err := s.buyBoxes(ctx, skuIDs, sellerActive)
	if err != nil {
		return nil, err
	}

	// Tồn kho theo lô, cùng nguồn với buy box.
	//
	// Bắt buộc phải hỏi ở đây chứ không suy từ `boxes`: SKU hết hàng thì
	// KHÔNG có bản ghi buy box nào, mà "không có người thắng" còn xảy ra
	// vì lý do khác (mọi nhà bán bị đình chỉ). Suy ngược từ chỗ vắng mặt
	// là đoán.
	available := map[ids.ID]int{}
	if s.inventory != nil {
		available, err = s.inventory.AvailableForSKUs(ctx, skuIDs)
		if err != nil {
			return nil, fmt.Errorf("tra tồn kho: %w", err)
		}
	}

	var out []ProductOffer
	for _, sku := range skuIDs {
		offers := bySKU[sku]
		if len(offers) == 0 {
			continue
		}

		box := boxes[sku]
		coHang := s.inventory == nil || available[sku] > 0

		for _, o := range offers {
			// KHÔNG lộ offer đã lưu trữ hay bị đình chỉ: khách thấy một
			// mức giá không đặt được là trải nghiệm tệ hơn không thấy gì.
			//
			// Offer HẾT HÀNG thì vẫn hiện — khác hẳn hai trường hợp trên.
			// Khách cần biết nhà bán này CÓ bán món đó để quyết định chờ
			// hay mua của người khác.
			if !o.IsVisibleToCustomer() {
				continue
			}
			active, activeErr := sellerActive.get(ctx, o.SellerID())
			if activeErr != nil {
				return nil, activeErr
			}
			out = append(out, ProductOffer{
				Offer:      o,
				IsBuyBox:   box.Winner != nil && box.Winner.ID() == o.ID(),
				IsSellable: domain.CanCustomerBuy(o, coHang, active),
			})
		}
	}

	return out, nil
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

// SellerOffer là offer nhìn từ phía NHÀ BÁN, kèm câu trả lời mà bản thân
// offer không tự trả lời được.
type SellerOffer struct {
	Offer *domain.Offer

	// IsSellable: KHÁCH mua được offer này không.
	//
	// Cùng câu hỏi và cùng quy tắc (`domain.CanCustomerBuy`) với
	// `ProductOffer.IsSellable` của trang sản phẩm — cố ý. Nhà bán hỏi
	// "hàng của tôi bán được không" thì câu trả lời phải là câu KHÁCH
	// nhận được, không phải một phiên bản dễ dãi hơn.
	//
	// Trước đây tầng HTTP tự suy `o.Status() == StatusActive`, tức bỏ qua
	// cả tồn kho lẫn trạng thái nhà bán — trong khi đặc tả của chính
	// trường đó dặn "đừng suy ra từ status". Hệ quả: Seller Center chưa
	// bao giờ có tín hiệu "hết hàng" hay "tài khoản đang bị đình chỉ".
	IsSellable bool
}

// SellableNow trả lời "khách mua được offer này không" cho MỘT offer.
//
// Dùng ở đường trả về sau khi tạo hoặc sửa offer, nơi chỉ có một bản ghi.
// Danh sách thì dùng `ListSellerOffers` — nó tra theo lô.
func (s *Service) SellableNow(ctx context.Context, o *domain.Offer) (bool, error) {
	if o == nil {
		return false, nil
	}

	coHang := true
	if s.inventory != nil {
		available, err := s.inventory.AvailableForSKUs(ctx, []ids.ID{o.SKUID()})
		if err != nil {
			return false, fmt.Errorf("tra tồn kho: %w", err)
		}
		coHang = available[o.SKUID()] > 0
	}

	active := true
	if s.seller != nil {
		var err error
		active, err = s.seller.IsActive(ctx, o.SellerID())
		if err != nil {
			return false, fmt.Errorf("kiểm tra nhà bán: %w", err)
		}
	}

	return domain.CanCustomerBuy(o, coHang, active), nil
}

// ListSellerOffers trả offer của MỘT nhà bán kèm cờ bán được.
//
// Tra theo LÔ: một lượt cho tồn kho của mọi SKU, một lượt cho trạng thái
// nhà bán — không phải mỗi offer một lượt.
func (s *Service) ListSellerOffers(
	ctx context.Context, sellerID ids.ID, limit, offset int,
) ([]SellerOffer, error) {
	offers, err := s.GetOffersBySeller(ctx, sellerID, limit, offset)
	if err != nil {
		return nil, err
	}
	if len(offers) == 0 {
		return nil, nil
	}

	// Tồn kho theo SKU, cùng nguồn và cùng độ chi tiết với trang sản phẩm.
	//
	// `AvailableForSKUs` trả tổng theo SKU, không tách theo chủ sở hữu —
	// giữ nguyên như vậy là CÓ CHỦ Ý: tách ở đây thì nhà bán lại thấy một
	// con số khác với con số quyết định nút mua của khách, tức là dựng lại
	// đúng chỗ lệch vừa xóa.
	skuIDs := make([]ids.ID, 0, len(offers))
	seen := make(map[ids.ID]bool, len(offers))
	for _, o := range offers {
		if !seen[o.SKUID()] {
			seen[o.SKUID()] = true
			skuIDs = append(skuIDs, o.SKUID())
		}
	}

	available := map[ids.ID]int{}
	if s.inventory != nil {
		available, err = s.inventory.AvailableForSKUs(ctx, skuIDs)
		if err != nil {
			return nil, fmt.Errorf("tra tồn kho: %w", err)
		}
	}

	active := true
	if s.seller != nil {
		active, err = s.seller.IsActive(ctx, sellerID)
		if err != nil {
			return nil, fmt.Errorf("kiểm tra nhà bán: %w", err)
		}
	}

	out := make([]SellerOffer, 0, len(offers))
	for _, o := range offers {
		coHang := s.inventory == nil || available[o.SKUID()] > 0
		out = append(out, SellerOffer{
			Offer:      o,
			IsSellable: domain.CanCustomerBuy(o, coHang, active),
		})
	}
	return out, nil
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

// PriceRange là khoảng giá MUA ĐƯỢC của một sản phẩm.
type PriceRange struct {
	// From là giá THẤP NHẤT trong các offer đang thắng buy box.
	//
	// Lấy từ buy box chứ không phải từ mọi offer: buy box đã loại offer
	// hết hàng và nhà bán bị đình chỉ, nên con số này là giá khách THẬT SỰ
	// mua được. Lấy min trên mọi offer sẽ quảng cáo một mức giá không đặt
	// được — đúng thứ đặc tả gọi là hứa suông.
	From money.Money

	// CompareAt là giá gạch ngang, chỉ có khi offer rẻ nhất đang giảm giá.
	CompareAt money.Money
}

// GetPriceRanges trả khoảng giá của NHIỀU sản phẩm trong một lượt.
//
// # Vì sao đây là việc của marketplace chứ không phải của product
//
// Giá thuộc về OFFER, và offer là khái niệm của module này. Module product
// nằm cùng tầng nên không gọi được marketplace, và nhồi giá vào
// `ProductSummary` sẽ bắt mọi lời gọi danh mục kéo theo truy vấn giá kể cả
// khi không hiển thị.
//
// Trang tự ghép hai nguồn — cùng mẫu với việc tra tên nhà bán.
//
// Sản phẩm không có offer nào bán được thì VẮNG MẶT trong kết quả, không
// phải trả giá 0: giá 0 hiển thị ra là "miễn phí".
func (s *Service) GetPriceRanges(
	ctx context.Context, productIDs []ids.ID,
) (map[ids.ID]PriceRange, error) {
	out := make(map[ids.ID]PriceRange, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}

	skusByProduct, err := s.product.SKUsOfProducts(ctx, productIDs)
	if err != nil {
		return nil, err
	}

	// Gom MỌI sku của MỌI sản phẩm rồi hỏi buy box một lượt.
	var allSKUs []ids.ID
	for _, skus := range skusByProduct {
		allSKUs = append(allSKUs, skus...)
	}
	if len(allSKUs) == 0 {
		return out, nil
	}

	boxes, err := s.GetBuyBoxes(ctx, allSKUs)
	if err != nil {
		return nil, err
	}

	for productID, skus := range skusByProduct {
		var reHon *domain.Offer
		for _, sku := range skus {
			box, ok := boxes[sku]
			if !ok || box.Winner == nil {
				continue
			}
			if reHon == nil || box.Winner.Price().LessThan(reHon.Price()) {
				reHon = box.Winner
			}
		}
		if reHon == nil {
			continue
		}
		pr := PriceRange{From: reHon.Price()}
		if reHon.HasDiscount() {
			pr.CompareAt = reHon.CompareAt()
		}
		out[productID] = pr
	}
	return out, nil
}
