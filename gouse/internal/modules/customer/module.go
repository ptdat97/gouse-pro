package customer

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/customer/application"
	"github.com/fashion-commerce/platform/internal/modules/customer/domain"
	customerpg "github.com/fashion-commerce/platform/internal/modules/customer/infrastructure/postgres"
	customerhttp "github.com/fashion-commerce/platform/internal/modules/customer/interfaces/http"
	"github.com/fashion-commerce/platform/internal/modules/identity"
	"github.com/fashion-commerce/platform/internal/platform/audit"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/privacy"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc *application.Service

	// identity tạo TÀI KHOẢN ĐĂNG NHẬP khi khách đăng ký.
	//
	// nil nghĩa là đường đăng ký không dùng được — mọi thứ còn lại vẫn
	// chạy. Đó là chủ ý: hồ sơ khách hàng tồn tại độc lập với việc có tài
	// khoản hay không (khách vãng lai cũng có hồ sơ).
	identity identity.API
}

var _ API = (*Module)(nil)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Ba ràng buộc quan trọng nhất đều nằm ở tầng database: email duy
	// nhất, ĐÚNG MỘT địa chỉ mặc định, và món yêu thích không trùng. Kiểm
	// tra trước khi ghi đều lọt khi hai request chạy song song.
	Storage string

	DB *database.DB

	Clock application.Clock

	// Identity tạo tài khoản đăng nhập ở đường ĐĂNG KÝ.
	//
	// Bỏ trống thì mọi thứ khác vẫn chạy, chỉ RegisterShopper trả lỗi —
	// hồ sơ khách hàng không phụ thuộc việc có tài khoản (khách vãng lai
	// cũng có hồ sơ).
	Identity identity.API

	// Audit là nhật ký thao tác, dùng cho endpoint quản trị xem hồ sơ khách.
	//
	// Bỏ trống thì mọi thứ khác vẫn chạy, chỉ `GetAsAdmin` từ chối — đọc
	// dữ liệu cá nhân mà không có đường ghi vết thì thà không đọc được.
	Audit *audit.Recorder
}

// New khởi tạo module customer.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"customer: chỉ hỗ trợ kho lưu trữ postgres — email duy nhất và " +
				"địa chỉ mặc định duy nhất cần chỉ mục UNIQUE ở tầng database")
	}
	if cfg.DB == nil {
		return nil, errors.New("customer: bắt buộc phải có kết nối database")
	}

	pool := cfg.DB.Pool()

	return &Module{identity: cfg.Identity, svc: application.NewService(application.Deps{
		Customers: customerpg.NewCustomerStore(pool),
		Addresses: customerpg.NewAddressStore(pool),
		Consents:  customerpg.NewConsentStore(pool),
		Wishlists: customerpg.NewWishlistStore(pool),
		Merges:    customerpg.NewMergeLogStore(pool),
		Clock:     cfg.Clock,
		Audit:     auditPort(cfg.Audit),
	})}, nil
}

// auditPort đổi bộ ghi nhật ký thành cổng của tầng application.
//
// Trả nil khi cfg.Audit nil, KHÔNG trả một adapter bọc nil: một adapter
// không rỗng bọc con trỏ nil sẽ qua được phép kiểm `s.audit == nil` ở
// `GetAsAdmin` rồi mới nổ lúc ghi — đúng lúc dữ liệu khách đã đọc xong.
func auditPort(rec *audit.Recorder) application.AuditRecorder {
	if rec == nil {
		return nil
	}
	return NewAuditRecorder(rec)
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// RegisterRoutes gắn các endpoint tài khoản khách hàng vào mux.
//
// Bên gọi PHẢI bọc httpserver.ResolveShopper: handler lấy định danh khách
// hàng từ context, và mọi endpoint ở đây yêu cầu đã đăng nhập.
func (m *Module) RegisterRoutes(mux *http.ServeMux, log *slog.Logger) {
	customerhttp.NewHandler(m.svc, log).Register(mux)
}

// RegisterPublicRoutes gắn đường ĐĂNG KÝ — endpoint công khai duy nhất của
// module này.
//
// Tách khỏi RegisterRoutes vì hai nhóm cần chuỗi middleware khác hẳn:
// nhóm kia yêu cầu đã đăng nhập, nhóm này thì không được yêu cầu (người
// đăng ký chưa có tài khoản) nhưng PHẢI có giới hạn tần suất.
// RegisterAdminRoutes gắn endpoint tra cứu hồ sơ khách của NHÂN VIÊN.
//
// Tách khỏi RegisterRoutes vì phạm vi khác hẳn: nhóm kia đọc hồ sơ của
// CHÍNH người đang đăng nhập (lấy từ ResolveShopper), nhóm này đọc hồ sơ
// của BẤT KỲ ai và vì thế phải ghi nhật ký mọi lần đọc.
func (m *Module) RegisterAdminRoutes(mux *http.ServeMux, log *slog.Logger) {
	customerhttp.NewAdminHandler(m.svc, log).Register(mux)
}

func (m *Module) RegisterPublicRoutes(mux *http.ServeMux, log *slog.Logger) {
	customerhttp.NewRegisterHandler(
		&registerAdapter{m: m}, log,
		identity.ErrDuplicateEmail, ErrEmailUsedByGuest, identity.ErrWeakPassword,
	).Register(mux)
}

// registerAdapter nối cổng của tầng HTTP với Module.
//
// Cần một adapter vì tầng interfaces KHÔNG được import gói cha (gói cha đã
// import nó — vòng lặp). Cổng dùng kiểu của riêng nó, và chỗ này dịch qua.
type registerAdapter struct{ m *Module }

var _ customerhttp.RegisterPort = (*registerAdapter)(nil)

func (a *registerAdapter) RegisterShopper(
	ctx context.Context, in customerhttp.RegisterInput,
) (customerhttp.RegisterOutput, error) {
	res, err := a.m.RegisterShopper(ctx, RegisterRequest{
		Email:       in.Email,
		Password:    in.Password,
		Phone:       in.Phone,
		DisplayName: in.DisplayName,
	})
	if err != nil {
		return customerhttp.RegisterOutput{}, err
	}
	return customerhttp.RegisterOutput{
		CustomerID: res.CustomerID,
		UserID:     res.UserID,
	}, nil
}

// ---------------------------------------------------------------- Hồ sơ

func (m *Module) Create(ctx context.Context, req CreateRequest) (CustomerView, error) {
	c, err := m.svc.Create(ctx, toCreateInput(req))
	if err != nil {
		return CustomerView{}, translateErr(err)
	}
	return toCustomerView(c), nil
}

func (m *Module) EnsureByEmail(ctx context.Context, req CreateRequest) (CustomerView, error) {
	c, err := m.svc.EnsureByEmail(ctx, toCreateInput(req))
	if err != nil {
		return CustomerView{}, translateErr(err)
	}
	return toCustomerView(c), nil
}

func (m *Module) GetCustomer(ctx context.Context, customerID string) (CustomerView, error) {
	c, err := m.svc.Get(ctx, ids.ID(customerID))
	if err != nil {
		return CustomerView{}, translateErr(err)
	}
	return toCustomerView(c), nil
}

func (m *Module) GetCustomerByUserID(ctx context.Context, userID string) (CustomerView, error) {
	c, err := m.svc.GetByUserID(ctx, ids.ID(userID))
	if err != nil {
		return CustomerView{}, translateErr(err)
	}
	return toCustomerView(c), nil
}

func (m *Module) GetCustomersByIDs(
	ctx context.Context, customerIDs []string,
) (map[string]CustomerView, error) {
	list := make([]ids.ID, 0, len(customerIDs))
	for _, id := range customerIDs {
		list = append(list, ids.ID(id))
	}

	found, err := m.svc.GetMany(ctx, list)
	if err != nil {
		return nil, translateErr(err)
	}

	out := make(map[string]CustomerView, len(found))
	for id, c := range found {
		out[id.String()] = toCustomerView(c)
	}
	return out, nil
}

func (m *Module) UpdateProfile(
	ctx context.Context, customerID, displayName, phone string,
) (CustomerView, error) {
	c, err := m.svc.UpdateProfile(ctx, ids.ID(customerID), displayName, phone)
	if err != nil {
		return CustomerView{}, translateErr(err)
	}
	return toCustomerView(c), nil
}

func (m *Module) LinkUser(ctx context.Context, customerID, userID string) error {
	return translateErr(m.svc.LinkUser(ctx, ids.ID(customerID), ids.ID(userID)))
}

func (m *Module) Anonymize(ctx context.Context, customerID string) error {
	return translateErr(m.svc.Anonymize(ctx, ids.ID(customerID)))
}

// ---------------------------------------------------------------- Địa chỉ

func (m *Module) AddAddress(ctx context.Context, req AddressRequest) (AddressView, error) {
	a, err := m.svc.AddAddress(ctx, toAddressParams(req))
	if err != nil {
		return AddressView{}, translateErr(err)
	}
	return toAddressView(a), nil
}

func (m *Module) GetAddresses(ctx context.Context, customerID string) ([]AddressView, error) {
	list, err := m.svc.ListAddresses(ctx, ids.ID(customerID))
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]AddressView, 0, len(list))
	for _, a := range list {
		out = append(out, toAddressView(a))
	}
	return out, nil
}

func (m *Module) GetDefaultAddress(ctx context.Context, customerID string) (AddressView, error) {
	a, err := m.svc.GetDefaultAddress(ctx, ids.ID(customerID))
	if err != nil {
		return AddressView{}, translateErr(err)
	}
	return toAddressView(a), nil
}

func (m *Module) UpdateAddress(
	ctx context.Context, addressID string, req AddressRequest,
) (AddressView, error) {
	a, err := m.svc.UpdateAddress(
		ctx, ids.ID(req.CustomerID), ids.ID(addressID), toAddressParams(req))
	if err != nil {
		return AddressView{}, translateErr(err)
	}
	return toAddressView(a), nil
}

func (m *Module) SetDefaultAddress(ctx context.Context, customerID, addressID string) error {
	return translateErr(m.svc.SetDefaultAddress(ctx, ids.ID(customerID), ids.ID(addressID)))
}

func (m *Module) DeleteAddress(ctx context.Context, customerID, addressID string) error {
	return translateErr(m.svc.DeleteAddress(ctx, ids.ID(customerID), ids.ID(addressID)))
}

// ---------------------------------------------------------------- Đồng ý

func (m *Module) RecordConsent(ctx context.Context, req ConsentRequest) error {
	_, err := m.svc.RecordConsent(ctx, domain.NewConsentParams{
		CustomerID: ids.ID(req.CustomerID),
		Type:       domain.ConsentType(req.Type),
		Granted:    req.Granted,
		Source:     req.Source,

		PolicyVersion: req.PolicyVersion,

		// Băm IP NGAY tại biên module: bên trong không bao giờ thấy địa
		// chỉ nguyên văn, nên không có chỗ nào lỡ tay ghi nó ra nhật ký.
		IPHash:    privacy.HashIP(req.IP),
		UserAgent: req.UserAgent,
	})
	return translateErr(err)
}

func (m *Module) HasConsent(ctx context.Context, customerID, consentType string) (bool, error) {
	ok, err := m.svc.HasConsent(ctx, ids.ID(customerID), domain.ConsentType(consentType))
	if err != nil {
		return false, translateErr(err)
	}
	return ok, nil
}

func (m *Module) GetConsentHistory(
	ctx context.Context, customerID string,
) ([]ConsentView, error) {
	list, err := m.svc.ConsentHistory(ctx, ids.ID(customerID))
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]ConsentView, 0, len(list))
	for _, c := range list {
		out = append(out, ConsentView{
			Type:          string(c.Type()),
			Granted:       c.Granted(),
			Source:        c.Source(),
			PolicyVersion: c.PolicyVersion(),
			RecordedAt:    c.RecordedAt(),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------- Wishlist

func (m *Module) AddToWishlist(ctx context.Context, req WishlistRequest) (bool, error) {
	added, err := m.svc.AddToWishlist(ctx, ids.ID(req.CustomerID),
		ids.ID(req.ProductID), ids.ID(req.VariantID), req.Note,
		req.NotifyWhenAvailable)
	if err != nil {
		return false, translateErr(err)
	}
	return added, nil
}

func (m *Module) RemoveFromWishlist(ctx context.Context, req WishlistRequest) (bool, error) {
	removed, err := m.svc.RemoveFromWishlist(ctx, ids.ID(req.CustomerID),
		ids.ID(req.ProductID), ids.ID(req.VariantID))
	if err != nil {
		return false, translateErr(err)
	}
	return removed, nil
}

func (m *Module) GetWishlist(ctx context.Context, customerID string) (WishlistView, error) {
	w, err := m.svc.GetWishlist(ctx, ids.ID(customerID))
	if err != nil {
		return WishlistView{}, translateErr(err)
	}

	items := w.Items()
	out := WishlistView{
		ID:    w.ID().String(),
		Name:  w.Name(),
		Items: make([]WishlistItemView, 0, len(items)),
	}
	for _, item := range items {
		out.Items = append(out.Items, WishlistItemView{
			ProductID:           item.ProductID.String(),
			VariantID:           item.VariantID.String(),
			Note:                item.Note,
			NotifyWhenAvailable: item.NotifyWhenAvailable,
			AddedAt:             item.AddedAt,
		})
	}
	return out, nil
}

func (m *Module) CountWishlistForProduct(ctx context.Context, productID string) (int, error) {
	n, err := m.svc.CountWishlistForProduct(ctx, ids.ID(productID))
	if err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

// ---------------------------------------------------------------- Gộp

func (m *Module) MergeGuestIdentity(ctx context.Context, req MergeRequest) error {
	return translateErr(m.svc.Merge(ctx, application.MergeInput{
		SourceCustomerID: ids.ID(req.SourceCustomerID),
		TargetCustomerID: ids.ID(req.TargetCustomerID),
		EmailVerified:    req.EmailVerified,
	}))
}

// ---------------------------------------------------------------- Chuyển đổi

func toCreateInput(req CreateRequest) application.CreateInput {
	return application.CreateInput{
		Email:       req.Email,
		Phone:       req.Phone,
		DisplayName: req.DisplayName,
		UserID:      ids.ID(req.UserID),
		Currency:    req.Currency,
	}
}

func toAddressParams(req AddressRequest) domain.NewAddressParams {
	return domain.NewAddressParams{
		CustomerID:     ids.ID(req.CustomerID),
		RecipientName:  req.RecipientName,
		RecipientPhone: req.RecipientPhone,
		Line1:          req.Line1,
		Line2:          req.Line2,
		Ward:           req.Ward,
		District:       req.District,
		Province:       req.Province,
		Postcode:       req.Postcode,
		Country:        req.Country,
		Note:           req.Note,
		IsDefault:      req.IsDefault,
	}
}

func toCustomerView(c *domain.Customer) CustomerView {
	return CustomerView{
		ID:          c.ID().String(),
		UserID:      c.UserID().String(),
		Email:       c.Email(),
		Phone:       c.Phone(),
		DisplayName: c.DisplayName(),
		Status:      string(c.Status()),
		OrderCount:  c.OrderCount(),
		TotalSpent:  c.TotalSpent().Amount(),
		Currency:    string(c.TotalSpent().Currency()),
		CreatedAt:   c.CreatedAt(),
		UpdatedAt:   c.UpdatedAt(),
	}
}

func toAddressView(a *domain.Address) AddressView {
	return AddressView{
		ID:             a.ID().String(),
		CustomerID:     a.CustomerID().String(),
		RecipientName:  a.RecipientName(),
		RecipientPhone: a.RecipientPhone(),
		Line1:          a.Line1(),
		Line2:          a.Line2(),
		Ward:           a.Ward(),
		District:       a.District(),
		Province:       a.Province(),
		Postcode:       a.Postcode(),
		Country:        a.Country(),
		Note:           a.Note(),
		IsDefault:      a.IsDefault(),
		CreatedAt:      a.CreatedAt(),
		UpdatedAt:      a.UpdatedAt(),
	}
}

// translateErr đổi lỗi domain thành lỗi CÔNG KHAI.
//
// Bên ngoài so lỗi bằng errors.Is với các biến khai báo ở public.go; nếu
// module rò rỉ lỗi domain, bên gọi sẽ phải import package domain và ranh
// giới module mất ý nghĩa.
func translateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domain.ErrAddressNotFound):
		return ErrAddressNotFound
	case errors.Is(err, domain.ErrDuplicateEmail):
		return ErrDuplicateEmail
	case errors.Is(err, domain.ErrVersionConflict):
		return ErrVersionConflict
	case errors.Is(err, domain.ErrAnonymized):
		return ErrAnonymized
	case errors.Is(err, domain.ErrMergeNotVerified):
		return ErrMergeNotVerified
	case errors.Is(err, domain.ErrInvalidEmail),
		errors.Is(err, domain.ErrInvalidAddress),
		errors.Is(err, domain.ErrInvalidConsent),
		errors.Is(err, domain.ErrMergeSameCustomer):
		return ErrInvalidInput
	default:
		return err
	}
}
