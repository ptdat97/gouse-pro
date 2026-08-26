// Package application chứa các use case của module cart.
//
// Việc khó nhất ở đây KHÔNG phải thêm/xóa món — đó là mấy dòng code. Việc
// khó là ĐỒNG BỘ giỏ: hiển thị giỏ 10 món cần dữ liệu hiện tại từ ba
// module, và làm sai chỗ này thì mỗi lần khách mở giỏ là 30 lượt gọi.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/cart/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// EventPublisher phát domain event.
//
// Là PORT do tầng application định nghĩa nên nó không biết outbox hay
// database. Ngữ cảnh truyền vào PHẢI mang giao dịch của kho lưu trữ.
type EventPublisher interface {
	PublishItemAdded(ctx context.Context, e ItemAdded) error
}

// ItemAdded là sự thật "khách đã thêm một món vào giỏ".
//
// Đây là TÍN HIỆU NHU CẦU MẠNH: khách đã quyết định muốn món này, chỉ chưa
// trả tiền. Mạnh hơn lượt xem rất nhiều.
type ItemAdded struct {
	CartID   ids.ID
	OfferID  ids.ID
	SKUID    ids.ID
	SellerID ids.ID
	Quantity int

	// Nguồn giới thiệu: nội dung nào dẫn tới việc thêm giỏ.
	//
	// Cho phép trả lời "nội dung nào tạo NHU CẦU THẬT, không chỉ tạo lượt
	// xem" — câu hỏi mà dữ liệu lượt xem một mình không trả lời được.
	SourceContentID ids.ID
	SourceCreatorID ids.ID
}

// Service là tầng application của module cart.
type Service struct {
	carts  domain.Repository
	offers domain.OfferLookup
	clock  Clock
	events EventPublisher
}

type Deps struct {
	Carts domain.Repository

	// Offers có thể nil: khi đó giỏ vẫn hoạt động nhưng KHÔNG đồng bộ được
	// giá và tình trạng hàng. Chấp nhận được ở môi trường phát triển, không
	// chấp nhận được ở production — xem Module.New.
	Offers domain.OfferLookup
	Clock  Clock

	// Events có thể nil: khi đó giỏ vẫn hoạt động nhưng KHÔNG phát tín
	// hiệu nhu cầu.
	//
	// Ở production đây là mất mát KHÔNG BÙ ĐƯỢC: dữ liệu lịch sử không tạo
	// ngược được, nên mỗi ngày không ghi là một ngày mất vĩnh viễn.
	Events EventPublisher
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{carts: d.Carts, offers: d.Offers, clock: clock, events: d.Events}
}

func (s *Service) Now() time.Time { return s.clock.Now() }

// ---------------------------------------------------------------- Lấy giỏ

// GetOrCreateInput là dữ liệu tìm hoặc tạo giỏ.
type GetOrCreateInput struct {
	CustomerID ids.ID
	SessionID  string
	Currency   money.Currency
}

// GetOrCreateCart tìm giỏ đang dùng, tạo mới nếu chưa có.
//
// QUY TẮC 5: mỗi khách chỉ có MỘT giỏ ACTIVE. Chỉ mục UNIQUE có điều kiện
// ở database là thứ cưỡng chế điều đó — hai tab cùng mở giỏ sẽ có một tab
// thua, và nhánh ErrDuplicateOwner bên dưới xử lý bằng cách đọc lại giỏ đã
// có thay vì báo lỗi cho khách.
func (s *Service) GetOrCreateCart(
	ctx context.Context, in GetOrCreateInput,
) (*domain.Cart, error) {
	if existing, err := s.findActive(ctx, in); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	c, err := domain.NewCart(domain.NewCartParams{
		CustomerID: in.CustomerID,
		SessionID:  in.SessionID,
		Currency:   in.Currency,
		Now:        s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}

	if err := s.carts.Save(ctx, c); err != nil {
		// Hai request song song cùng tạo giỏ; database chặn cái thứ hai.
		// Đọc lại giỏ đã có là hành vi ĐÚNG: khách chỉ muốn có một giỏ để
		// dùng, không quan tâm request nào tạo ra nó.
		if errors.Is(err, domain.ErrDuplicateOwner) {
			return s.findActive(ctx, in)
		}
		return nil, err
	}
	return c, nil
}

func (s *Service) findActive(
	ctx context.Context, in GetOrCreateInput,
) (*domain.Cart, error) {
	if !in.CustomerID.IsZero() {
		return s.carts.FindActiveByCustomer(ctx, in.CustomerID)
	}
	if in.SessionID != "" {
		return s.carts.FindActiveBySession(ctx, in.SessionID)
	}
	return nil, domain.ErrNoOwner
}

// FindActiveCart tìm giỏ đang dùng — KHÔNG tạo nếu chưa có.
//
// Khác GetOrCreateCart ở đúng một điểm, và điểm đó là toàn bộ lý do nó
// tồn tại: đường ĐỌC không được tạo dữ liệu. Khách chưa có giỏ thì câu
// trả lời đúng là "giỏ rỗng", không phải "vừa tạo cho bạn một giỏ".
//
// Trả domain.ErrNotFound khi chưa có; bên gọi dựng giỏ rỗng để hiển thị.
func (s *Service) FindActiveCart(
	ctx context.Context, in GetOrCreateInput,
) (*domain.Cart, error) {
	return s.findActive(ctx, in)
}

// GetCart đọc giỏ theo định danh, ĐÃ ĐỒNG BỘ với dữ liệu hiện tại.
func (s *Service) GetCart(ctx context.Context, id ids.ID) (*domain.Cart, error) {
	c, err := s.carts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.Sync(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ---------------------------------------------------------------- Đồng bộ

// Sync làm mới giá và tình trạng của mọi món trong giỏ.
//
// MỘT LƯỢT GỌI cho toàn bộ giỏ, không phải một lượt cho mỗi món (cart.md
// mục 11): giỏ 10 món mà gọi trong vòng lặp thì mỗi lần khách mở giỏ là
// hàng chục lượt gọi qua ba module.
//
// Sau khi đồng bộ, giỏ được GHI LẠI nếu có gì đổi: nếu không, khách phải
// đợi lần thao tác tiếp theo mới thấy giá mới được lưu, và hai thiết bị
// của cùng một khách sẽ hiện hai con số khác nhau.
func (s *Service) Sync(ctx context.Context, c *domain.Cart) error {
	if err := s.SyncView(ctx, c); err != nil {
		return err
	}
	if s.offers == nil || c.ItemCount() == 0 {
		return nil
	}
	return s.carts.Save(ctx, c)
}

// SyncView đồng bộ giá và tình trạng hàng NGAY TRONG BỘ NHỚ, không ghi.
//
// # Vì sao tách khỏi Sync
//
// Đường ĐỌC giỏ (`GET /api/v1/cart`) không được ghi. Trước 26/08 nó ghi,
// và hệ quả là hai lượt ĐỌC song song tranh chấp nhau — thứ không ai chờ
// đợi ở một endpoint đọc. Triệu chứng: ngay sau khi đặt hàng, giao diện
// làm mới giỏ và nhận 500 khoảng một nửa số lần (PH-29).
//
// # Vì sao KHÔNG ghi mà vẫn đúng
//
// Giỏ hàng KHÔNG hứa gì với khách: giá và tình trạng hàng ở giỏ là thông
// tin tham khảo, cam kết chỉ có ở checkout. Con số đã đồng bộ chỉ cần
// đúng TẠI THỜI ĐIỂM hiển thị — lưu nó xuống không làm nó đúng lâu hơn,
// vì lần đọc sau lại đồng bộ lại từ đầu.
//
// Ghi ở đường đọc còn có hại: nó biến mỗi lần khách mở giỏ thành một lần
// ghi database, và ở trang có nhiều tab thì thành nhiều lần ghi tranh
// nhau cho cùng một dữ liệu.
func (s *Service) SyncView(ctx context.Context, c *domain.Cart) error {
	if s.offers == nil || c.ItemCount() == 0 {
		return nil
	}

	items := c.Items()
	offerIDs := make([]ids.ID, 0, len(items))
	for _, it := range items {
		offerIDs = append(offerIDs, it.OfferID())
	}

	data, err := s.offers.LookupOffers(ctx, offerIDs)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	for _, it := range items {
		d, ok := data[it.OfferID()]
		if !ok {
			// Không có trong kết quả tra cứu nghĩa là offer ĐÃ BỊ GỠ.
			// Món vẫn ở lại giỏ với dấu UNAVAILABLE — quy tắc 6.
			d = domain.SyncData{OfferExists: false}
		}
		it.Sync(d, now)
	}
	return nil
}

// ---------------------------------------------------------------- Sửa giỏ

// AddItemInput là dữ liệu thêm một món vào giỏ.
type AddItemInput struct {
	CartID   ids.ID
	OfferID  ids.ID
	Quantity int

	SourceContentID ids.ID
	SourceCreatorID ids.ID
}

// AddItem thêm một món vào giỏ.
//
// Giá và giới hạn số lượng được TRA NGAY từ marketplace, không nhận từ bên
// gọi: khách gửi lên giá thì khách đặt được giá của chính mình.
//
// Đây là khác biệt quan trọng với order.PlaceOrder, nơi giá do checkout
// truyền xuống. Lý do: ở đó giá đã được chốt qua một bước khách nhìn thấy
// và đồng ý; ở đây chưa có bước nào như vậy.
func (s *Service) AddItem(ctx context.Context, in AddItemInput) (*domain.Cart, error) {
	c, err := s.carts.FindByID(ctx, in.CartID)
	if err != nil {
		return nil, err
	}

	if s.offers == nil {
		return nil, errors.New("cart: không tra được dữ liệu offer")
	}
	data, err := s.offers.LookupOffers(ctx, []ids.ID{in.OfferID})
	if err != nil {
		return nil, err
	}
	d, ok := data[in.OfferID]
	if !ok || !d.OfferExists {
		return nil, domain.ErrNotFound
	}

	if _, err := c.AddItem(domain.NewItemParams{
		OfferID:            in.OfferID,
		SKUID:              d.SKUID,
		SellerID:           d.SellerID,
		ProductName:        d.ProductName,
		VariantDescription: d.VariantDescription,
		ImageURL:           d.ImageURL,
		SellerName:         d.SellerName,
		UnitPrice:          d.UnitPrice,
		Quantity:           in.Quantity,
		MinOrderQuantity:   d.MinOrderQuantity,
		MaxOrderQuantity:   d.MaxOrderQuantity,
		SourceContentID:    in.SourceContentID,
		SourceCreatorID:    in.SourceCreatorID,
		Now:                s.clock.Now(),
	}); err != nil {
		return nil, err
	}

	// Gia hạn giỏ: khách còn tương tác thì giỏ còn sống.
	c.Touch(s.clock.Now(), domain.DefaultTTL)

	// Ghi giỏ VÀ phát tín hiệu nhu cầu trong CÙNG một giao dịch.
	//
	// Món vào giỏ mà tín hiệu không được ghi nghĩa là mất một quan sát về
	// nhu cầu — và dữ liệu lịch sử không tạo ngược được.
	if err := s.carts.SaveWithEvents(ctx, c, func(txCtx context.Context) error {
		if s.events == nil {
			return nil
		}
		return s.events.PublishItemAdded(txCtx, ItemAdded{
			CartID:          c.ID(),
			OfferID:         in.OfferID,
			SKUID:           d.SKUID,
			SellerID:        d.SellerID,
			Quantity:        in.Quantity,
			SourceContentID: in.SourceContentID,
			SourceCreatorID: in.SourceCreatorID,
		})
	}); err != nil {
		return nil, err
	}
	return c, s.Sync(ctx, c)
}

// UpdateQuantity đổi số lượng một món.
func (s *Service) UpdateQuantity(
	ctx context.Context, cartID, itemID ids.ID, quantity int,
) (*domain.Cart, error) {
	c, err := s.carts.FindByID(ctx, cartID)
	if err != nil {
		return nil, err
	}
	if err := c.UpdateQuantity(itemID, quantity, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.carts.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, s.Sync(ctx, c)
}

// RemoveItem xóa một món khỏi giỏ.
func (s *Service) RemoveItem(
	ctx context.Context, cartID, itemID ids.ID,
) (*domain.Cart, error) {
	c, err := s.carts.FindByID(ctx, cartID)
	if err != nil {
		return nil, err
	}
	if err := c.RemoveItem(itemID, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.carts.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ClearCart xóa toàn bộ món trong giỏ.
func (s *Service) ClearCart(ctx context.Context, cartID ids.ID) error {
	c, err := s.carts.FindByID(ctx, cartID)
	if err != nil {
		return err
	}
	if err := c.Clear(s.clock.Now()); err != nil {
		return err
	}
	return s.carts.Save(ctx, c)
}

// ---------------------------------------------------------------- Gộp giỏ

// MergeResult là kết quả gộp giỏ khi khách đăng nhập.
type MergeResult struct {
	Cart *domain.Cart

	// Warnings PHẢI được hiển thị cho khách.
	//
	// Im lặng bỏ qua nghĩa là khách đăng nhập xong thấy giỏ ít hàng hơn
	// lúc chưa đăng nhập mà không hiểu vì sao — trải nghiệm tệ nhất của
	// luồng này.
	Warnings []domain.MergeWarning
}

// MergeOnLogin gộp giỏ theo phiên vào giỏ của tài khoản.
//
// Ba trường hợp, xử lý khác nhau:
//
//  1. Tài khoản CHƯA có giỏ → chỉ ĐỔI CHỦ giỏ phiên, không gộp gì
//  2. Tài khoản ĐÃ có giỏ   → gộp, món trùng cộng dồn
//  3. Không có giỏ phiên    → trả giỏ tài khoản như thường
//
// Trường hợp 1 quan trọng hơn vẻ ngoài của nó: đó là đường đi phổ biến
// nhất (khách mới thêm hàng rồi mới đăng ký), và đổi chủ giữ nguyên mọi
// nguồn giới thiệu — thứ sẽ mất nếu tạo giỏ mới rồi chép món sang.
func (s *Service) MergeOnLogin(
	ctx context.Context, customerID ids.ID, sessionID string,
) (*MergeResult, error) {
	now := s.clock.Now()

	sessionCart, err := s.carts.FindActiveBySession(ctx, sessionID)
	if errors.Is(err, domain.ErrNotFound) {
		// Trường hợp 3: không có giỏ phiên.
		c, err := s.GetOrCreateCart(ctx, GetOrCreateInput{CustomerID: customerID})
		if err != nil {
			return nil, err
		}
		return &MergeResult{Cart: c}, s.Sync(ctx, c)
	}
	if err != nil {
		return nil, err
	}

	accountCart, err := s.carts.FindActiveByCustomer(ctx, customerID)
	if errors.Is(err, domain.ErrNotFound) {
		// Trường hợp 1: đổi chủ, giữ nguyên mọi thứ kể cả nguồn giới thiệu.
		if err := sessionCart.AssignToCustomer(customerID, now); err != nil {
			return nil, err
		}
		if err := s.carts.Save(ctx, sessionCart); err != nil {
			return nil, err
		}
		return &MergeResult{Cart: sessionCart}, s.Sync(ctx, sessionCart)
	}
	if err != nil {
		return nil, err
	}

	// Trường hợp 2: gộp thật.
	warnings, err := accountCart.MergeFrom(sessionCart, now)
	if err != nil {
		return nil, err
	}

	// Ghi giỏ nguồn TRƯỚC: nó vừa được đánh dấu MERGED, và chỉ mục UNIQUE
	// chỉ cho phép một giỏ ACTIVE mỗi phiên. Ghi giỏ đích trước rồi lỗi ở
	// giỏ nguồn sẽ để lại hai giỏ ACTIVE cho cùng một người.
	if err := s.carts.Save(ctx, sessionCart); err != nil {
		return nil, err
	}
	if err := s.carts.Save(ctx, accountCart); err != nil {
		return nil, err
	}

	if err := s.Sync(ctx, accountCart); err != nil {
		return nil, err
	}
	return &MergeResult{Cart: accountCart, Warnings: warnings}, nil
}

// ---------------------------------------------------------------- Chuyển đổi

// MarkConverted đánh dấu giỏ đã thành đơn hàng.
//
// Gọi bởi checkout sau khi đặt hàng thành công. Giỏ KHÔNG bị xóa: nó cho
// biết nội dung nào dẫn tới việc mua thật — dữ liệu đầu vào của bánh đà
// creator commerce.
func (s *Service) MarkConverted(ctx context.Context, cartID ids.ID) error {
	c, err := s.carts.FindByID(ctx, cartID)
	if err != nil {
		return err
	}
	if err := c.MarkConverted(s.clock.Now()); err != nil {
		return err
	}
	return s.carts.Save(ctx, c)
}
