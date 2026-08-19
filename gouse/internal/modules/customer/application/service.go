// Package application chứa các use case của module customer.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/kernel/money"
	"github.com/fashion-commerce/platform/internal/modules/customer/domain"
)

// Clock cho phép test kiểm soát thời gian.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

var SystemClock Clock = systemClock{}

// Service là tầng application của module customer.
type Service struct {
	customers domain.CustomerRepository
	addresses domain.AddressRepository
	consents  domain.ConsentRepository
	wishlists domain.WishlistRepository
	merges    domain.MergeLogRepository
	clock     Clock
}

type Deps struct {
	Customers domain.CustomerRepository
	Addresses domain.AddressRepository
	Consents  domain.ConsentRepository
	Wishlists domain.WishlistRepository
	Merges    domain.MergeLogRepository
	Clock     Clock
}

func NewService(d Deps) *Service {
	clock := d.Clock
	if clock == nil {
		clock = SystemClock
	}
	return &Service{
		customers: d.Customers,
		addresses: d.Addresses,
		consents:  d.Consents,
		wishlists: d.Wishlists,
		merges:    d.Merges,
		clock:     clock,
	}
}

// ---------------------------------------------------------------- Hồ sơ

// CreateInput là dữ liệu tạo hồ sơ.
type CreateInput struct {
	Email       string
	Phone       string
	DisplayName string

	// UserID để trống với khách vãng lai.
	UserID ids.ID

	Currency string
}

// Create tạo hồ sơ khách hàng.
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Customer, error) {
	c, err := domain.NewCustomer(domain.NewCustomerParams{
		Email:       in.Email,
		Phone:       in.Phone,
		DisplayName: in.DisplayName,
		UserID:      in.UserID,
		Currency:    in.Currency,
		Now:         s.clock.Now(),
	})
	if err != nil {
		return nil, err
	}

	if err := s.customers.Save(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// EnsureByEmail trả hồ sơ có sẵn hoặc tạo mới.
//
// # Vì sao cần hàm này
//
// Khách vãng lai thanh toán bằng email đã từng đặt hàng phải vào ĐÚNG hồ
// sơ cũ, không tạo hồ sơ thứ hai. Nếu không, lịch sử mua hàng của họ bị
// chia ra và không bao giờ gộp lại được.
//
// CHẠY ĐUA: hai request cùng email cùng lúc sẽ cùng thấy "chưa có" rồi
// cùng ghi. Ràng buộc UNIQUE chặn request thứ hai, và ở đó ta đọc lại hồ
// sơ vừa được request đầu tạo thay vì báo lỗi ra ngoài.
func (s *Service) EnsureByEmail(ctx context.Context, in CreateInput) (*domain.Customer, error) {
	email := domain.NormalizeEmail(in.Email)

	existing, err := s.customers.FindByEmail(ctx, email)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	c, err := s.Create(ctx, in)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, domain.ErrDuplicateEmail) {
		return nil, err
	}

	// Request khác thắng cuộc đua. Hồ sơ của họ cũng là hồ sơ đúng.
	return s.customers.FindByEmail(ctx, email)
}

func (s *Service) Get(ctx context.Context, id ids.ID) (*domain.Customer, error) {
	return s.customers.FindByID(ctx, id)
}

func (s *Service) GetByUserID(ctx context.Context, userID ids.ID) (*domain.Customer, error) {
	return s.customers.FindByUserID(ctx, userID)
}

func (s *Service) GetByEmail(ctx context.Context, email string) (*domain.Customer, error) {
	return s.customers.FindByEmail(ctx, domain.NormalizeEmail(email))
}

func (s *Service) GetMany(
	ctx context.Context, list []ids.ID,
) (map[ids.ID]*domain.Customer, error) {
	return s.customers.FindManyByIDs(ctx, list)
}

// UpdateProfile sửa tên hiển thị và số điện thoại.
func (s *Service) UpdateProfile(
	ctx context.Context, id ids.ID, displayName, phone string,
) (*domain.Customer, error) {
	c, err := s.customers.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := c.UpdateProfile(displayName, phone, s.clock.Now()); err != nil {
		return nil, err
	}
	if err := s.customers.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// RecordOrder cập nhật thống kê sau khi một đơn hoàn tất.
//
// Gọi từ handler nghe event `order.completed`. Chống xử lý trùng là việc
// của bảng event_processed, không phải của hàm này.
func (s *Service) RecordOrder(
	ctx context.Context, id ids.ID, amount money.Money,
) error {
	c, err := s.customers.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if err := c.RecordOrder(amount, s.clock.Now()); err != nil {
		return err
	}
	return s.customers.Update(ctx, c)
}

// Anonymize gỡ dữ liệu định danh theo yêu cầu xóa tài khoản.
//
// GIỮ LẠI order_count và total_spent: chúng là dữ liệu giao dịch, cần cho
// đối soát với seller. Xem domain.Customer.Anonymize.
//
// KHÔNG xóa địa chỉ ở đây — xem ghi chú trong hàm.
func (s *Service) Anonymize(ctx context.Context, id ids.ID) error {
	c, err := s.customers.FindByID(ctx, id)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	c.Anonymize(now)
	if err := s.customers.Update(ctx, c); err != nil {
		return err
	}

	// Sổ địa chỉ chứa tên, số điện thoại và địa chỉ nhà — đều là dữ liệu
	// định danh, nên phải xóa cùng.
	//
	// Đơn hàng KHÔNG bị ảnh hưởng: chúng giữ bản SAO ĐÓNG BĂNG của địa chỉ
	// lúc giao hàng (nguyên tắc P9), không tham chiếu tới bảng này.
	list, err := s.addresses.ListByCustomer(ctx, id)
	if err != nil {
		return err
	}
	for _, a := range list {
		a.Delete(now)
		if err := s.addresses.Update(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- Địa chỉ

// AddAddress thêm địa chỉ vào sổ.
//
// Địa chỉ ĐẦU TIÊN luôn thành mặc định, kể cả khi bên gọi không yêu cầu.
// Không có mặc định thì trang thanh toán không biết điền gì, và khách phải
// chọn lại địa chỉ ở mỗi đơn.
func (s *Service) AddAddress(
	ctx context.Context, p domain.NewAddressParams,
) (*domain.Address, error) {
	if _, err := s.customers.FindByID(ctx, p.CustomerID); err != nil {
		return nil, err
	}

	existing, err := s.addresses.ListByCustomer(ctx, p.CustomerID)
	if err != nil {
		return nil, err
	}

	p.Now = s.clock.Now()
	makeDefault := p.IsDefault || len(existing) == 0

	// Tạo với is_default=false rồi đặt mặc định qua SetDefault.
	//
	// Ghi thẳng is_default=true sẽ đụng chỉ mục UNIQUE khi khách đã có một
	// địa chỉ mặc định khác — và SetDefault là chỗ DUY NHẤT gỡ cờ cũ và
	// đặt cờ mới trong cùng một giao dịch.
	p.IsDefault = false

	a, err := domain.NewAddress(p)
	if err != nil {
		return nil, err
	}
	if err := s.addresses.Save(ctx, a); err != nil {
		return nil, err
	}

	if makeDefault {
		if err := s.addresses.SetDefault(ctx, p.CustomerID, a.ID(), p.Now); err != nil {
			return nil, err
		}
		a.SetDefault(true, p.Now)
	}
	return a, nil
}

func (s *Service) ListAddresses(
	ctx context.Context, customerID ids.ID,
) ([]*domain.Address, error) {
	return s.addresses.ListByCustomer(ctx, customerID)
}

func (s *Service) GetDefaultAddress(
	ctx context.Context, customerID ids.ID,
) (*domain.Address, error) {
	return s.addresses.FindDefault(ctx, customerID)
}

// UpdateAddress sửa một địa chỉ.
//
// KIỂM TRA QUYỀN SỞ HỮU ở tầng domain (BelongsTo) BÊN CẠNH điều kiện
// customer_id trong SQL. Hai lớp độc lập vì một lớp có thể bị sửa hỏng mà
// không ai nhận ra: biết id địa chỉ của người khác là đọc và sửa được tên,
// số điện thoại và địa chỉ nhà của họ.
func (s *Service) UpdateAddress(
	ctx context.Context, customerID, addressID ids.ID, p domain.NewAddressParams,
) (*domain.Address, error) {
	a, err := s.addresses.FindByID(ctx, addressID)
	if err != nil {
		return nil, err
	}
	if !a.BelongsTo(customerID) || a.IsDeleted() {
		return nil, domain.ErrAddressNotFound
	}

	p.Now = s.clock.Now()
	if err := a.Update(p); err != nil {
		return nil, err
	}
	if err := s.addresses.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// SetDefaultAddress đặt một địa chỉ làm mặc định.
func (s *Service) SetDefaultAddress(ctx context.Context, customerID, addressID ids.ID) error {
	a, err := s.addresses.FindByID(ctx, addressID)
	if err != nil {
		return err
	}
	if !a.BelongsTo(customerID) || a.IsDeleted() {
		return domain.ErrAddressNotFound
	}
	return s.addresses.SetDefault(ctx, customerID, addressID, s.clock.Now())
}

// DeleteAddress xóa MỀM một địa chỉ.
//
// Xóa địa chỉ mặc định KHÔNG tự chọn địa chỉ khác thay thế: chọn hộ là
// đoán, và đoán sai nghĩa là hàng đi tới địa chỉ khách không muốn. Trang
// thanh toán sẽ hỏi lại.
func (s *Service) DeleteAddress(ctx context.Context, customerID, addressID ids.ID) error {
	a, err := s.addresses.FindByID(ctx, addressID)
	if err != nil {
		return err
	}
	if !a.BelongsTo(customerID) {
		return domain.ErrAddressNotFound
	}

	a.Delete(s.clock.Now())
	return s.addresses.Update(ctx, a)
}

// ---------------------------------------------------------------- Đồng ý

// RecordConsent ghi nhận một lần đồng ý hoặc rút lại.
func (s *Service) RecordConsent(
	ctx context.Context, p domain.NewConsentParams,
) (*domain.Consent, error) {
	if _, err := s.customers.FindByID(ctx, p.CustomerID); err != nil {
		return nil, err
	}

	p.Now = s.clock.Now()
	c, err := domain.NewConsent(p)
	if err != nil {
		return nil, err
	}
	if err := s.consents.Record(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// HasConsent cho biết khách CÓ ĐANG đồng ý loại này không.
//
// # Vắng mặt nghĩa là KHÔNG ĐỒNG Ý
//
// Không tìm thấy bản ghi nào nghĩa là khách CHƯA đồng ý, không phải "chưa
// từ chối". Suy diễn ngược lại là gửi thư quảng cáo cho người chưa bao giờ
// bấm đồng ý — vi phạm pháp luật ở nhiều thị trường.
func (s *Service) HasConsent(
	ctx context.Context, customerID ids.ID, t domain.ConsentType,
) (bool, error) {
	if !domain.ValidConsentType(t) {
		return false, domain.ErrInvalidConsent
	}

	current, err := s.consents.Current(ctx, customerID)
	if err != nil {
		return false, err
	}

	c, ok := current[t]
	if !ok {
		return false, nil
	}
	return c.Granted(), nil
}

func (s *Service) ConsentHistory(
	ctx context.Context, customerID ids.ID,
) ([]*domain.Consent, error) {
	return s.consents.History(ctx, customerID)
}

// ---------------------------------------------------------------- Wishlist

// wishlistFor trả danh sách mặc định, tạo mới nếu chưa có.
func (s *Service) wishlistFor(
	ctx context.Context, customerID ids.ID,
) (*domain.Wishlist, error) {
	w, err := s.wishlists.FindDefault(ctx, customerID)
	if err == nil {
		return w, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	if _, err := s.customers.FindByID(ctx, customerID); err != nil {
		return nil, err
	}

	fresh := domain.NewWishlist(customerID, s.clock.Now())
	if err := s.wishlists.Save(ctx, fresh); err != nil {
		// Hai request cùng thêm món đầu tiên sẽ cùng tạo danh sách; chỉ
		// mục UNIQUE chặn request thứ hai. Đọc lại danh sách của request
		// đầu thay vì báo lỗi.
		if w, findErr := s.wishlists.FindDefault(ctx, customerID); findErr == nil {
			return w, nil
		}
		return nil, err
	}
	return fresh, nil
}

// AddToWishlist thêm một món.
//
// Trả về true nếu món thật sự được thêm mới. Thêm lại món đã có KHÔNG báo
// lỗi: khách bấm tim hai lần là chuyện thường.
func (s *Service) AddToWishlist(
	ctx context.Context, customerID, productID, variantID ids.ID,
	note string, notifyWhenAvailable bool,
) (bool, error) {
	w, err := s.wishlistFor(ctx, customerID)
	if err != nil {
		return false, err
	}

	return s.wishlists.AddItem(ctx, w.ID(), domain.WishlistItem{
		ProductID:           productID,
		VariantID:           variantID,
		Note:                note,
		NotifyWhenAvailable: notifyWhenAvailable,
		AddedAt:             s.clock.Now(),
	})
}

// RemoveFromWishlist bỏ một món.
func (s *Service) RemoveFromWishlist(
	ctx context.Context, customerID, productID, variantID ids.ID,
) (bool, error) {
	w, err := s.wishlists.FindDefault(ctx, customerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Chưa có danh sách nghĩa là món đó chắc chắn không nằm trong
			// đó. Kết quả mong muốn đã đạt.
			return false, nil
		}
		return false, err
	}

	return s.wishlists.RemoveItem(ctx, w.ID(), productID, variantID)
}

// GetWishlist trả danh sách yêu thích.
//
// Khách chưa có danh sách sẽ nhận danh sách RỖNG chứ không phải lỗi: chưa
// thích gì là trạng thái bình thường, không phải sự cố.
func (s *Service) GetWishlist(
	ctx context.Context, customerID ids.ID,
) (*domain.Wishlist, error) {
	w, err := s.wishlists.FindDefault(ctx, customerID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.NewWishlist(customerID, s.clock.Now()), nil
	}
	if err != nil {
		return nil, err
	}
	return w, nil
}

// CountWishlistForProduct đếm số khách đã thích một sản phẩm.
//
// TÍN HIỆU NHU CẦU: nhiều người thích mà chưa mua thường là dấu hiệu giá
// cao hoặc hết size.
func (s *Service) CountWishlistForProduct(ctx context.Context, productID ids.ID) (int, error) {
	return s.wishlists.CountByProduct(ctx, productID)
}

// ---------------------------------------------------------------- Gộp

// MergeInput là dữ liệu gộp danh tính.
type MergeInput struct {
	// SourceCustomerID là hồ sơ khách vãng lai BỊ GỘP VÀO hồ sơ đích.
	SourceCustomerID ids.ID

	// TargetCustomerID là hồ sơ giữ lại.
	TargetCustomerID ids.ID

	// EmailVerified là XÁC NHẬN đã kiểm chứng quyền sở hữu email.
	//
	// Đây KHÔNG phải cờ tiện lợi. Xem ghi chú ở Merge.
	EmailVerified bool
}

// Merge gộp hồ sơ khách vãng lai vào hồ sơ đã đăng ký.
//
// # Xác minh email là YÊU CẦU BẢO MẬT, không phải tùy chọn
//
// Không xác minh thì bất kỳ ai đăng ký bằng email người khác đều đọc được
// toàn bộ lịch sử mua hàng của họ — kể cả địa chỉ nhà và số điện thoại.
//
// Vì vậy hàm này TỪ CHỐI khi EmailVerified=false, và bên gọi phải tự chứng
// minh đã gửi thư xác minh chứ không được truyền true cho tiện.
//
// # Phạm vi ở MVP
//
// Hàm này CHỈ ghi nhật ký gộp và ẩn danh hồ sơ nguồn. Việc chuyển đơn hàng
// từ hồ sơ nguồn sang hồ sơ đích thuộc module `order` — customer không sửa
// bảng của module khác (quy tắc R2). Module order sẽ nghe event
// `customer.identities_merged`.
func (s *Service) Merge(ctx context.Context, in MergeInput) error {
	if !in.EmailVerified {
		return domain.ErrMergeNotVerified
	}
	if in.SourceCustomerID == in.TargetCustomerID {
		return domain.ErrMergeSameCustomer
	}

	source, err := s.customers.FindByID(ctx, in.SourceCustomerID)
	if err != nil {
		return err
	}
	target, err := s.customers.FindByID(ctx, in.TargetCustomerID)
	if err != nil {
		return err
	}

	now := s.clock.Now()

	if err := s.merges.Record(ctx, domain.MergeRecord{
		SourceCustomerID: source.ID(),
		TargetCustomerID: target.ID(),
		Reason:           "email_verified",
		MergedAt:         now,
	}); err != nil {
		return err
	}

	// Ẩn danh hồ sơ nguồn thay vì xóa: đơn hàng cũ vẫn trỏ tới
	// customer_id của nó cho tới khi module order chuyển xong, và xóa hàng
	// sẽ làm những đơn đó mồ côi.
	source.Anonymize(now)
	return s.customers.Update(ctx, source)
}

// LinkUser gắn hồ sơ khách vãng lai với tài khoản vừa đăng ký.
//
// Đây là đường đi THƯỜNG GẶP hơn Merge: khách vãng lai đăng ký bằng chính
// email họ đã dùng để đặt hàng. Không cần gộp gì cả — chỉ điền user_id vào
// hồ sơ có sẵn, và toàn bộ lịch sử mua hàng tự thuộc về tài khoản mới.
func (s *Service) LinkUser(ctx context.Context, customerID, userID ids.ID) error {
	c, err := s.customers.FindByID(ctx, customerID)
	if err != nil {
		return err
	}
	if err := c.LinkUser(userID, s.clock.Now()); err != nil {
		return err
	}
	return s.customers.Update(ctx, c)
}
