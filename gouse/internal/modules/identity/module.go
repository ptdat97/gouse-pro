package identity

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/identity/application"
	"github.com/fashion-commerce/platform/internal/modules/identity/domain"
	"github.com/fashion-commerce/platform/internal/modules/identity/infrastructure/crypto"
	identitypg "github.com/fashion-commerce/platform/internal/modules/identity/infrastructure/postgres"
	identityhttp "github.com/fashion-commerce/platform/internal/modules/identity/interfaces/http"
	"github.com/fashion-commerce/platform/internal/platform/database"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/token"
)

// Module là cài đặt của API công khai.
type Module struct {
	svc    *application.Service
	issuer *token.Issuer

	// secureCookie bật cờ Secure trên cookie refresh token.
	secureCookie bool
}

var (
	_ API                      = (*Module)(nil)
	_ httpserver.TokenVerifier = (*Module)(nil)
)

// Config cấu hình module khi khởi tạo.
type Config struct {
	// Storage: module này CHỈ hỗ trợ "postgres".
	//
	// Email duy nhất và token duy nhất dựa vào chỉ mục UNIQUE. Kiểm tra
	// trước khi ghi vẫn lọt khi hai request đăng ký cùng lúc, và khi đó
	// hai tài khoản cùng email là bế tắc chỉ quản trị viên gỡ được.
	Storage string

	DB  *database.DB
	Log *slog.Logger

	// BcryptCost là chi phí băm; 0 dùng mặc định của thư viện.
	//
	// Test đặt giá trị thấp nhất (4) để không tốn hàng giây cho mỗi lần
	// băm. Môi trường thật KHÔNG được hạ giá trị này.
	BcryptCost int

	Clock application.Clock

	// Issuer phát hành và xác minh access token.
	//
	// BẮT BUỘC: không có nó thì đăng nhập thành công nhưng không cấp được
	// token, và mọi endpoint cần xác thực đều không dùng được.
	Issuer *token.Issuer

	// SecureCookie bật cờ Secure trên cookie refresh token.
	//
	// Tắt để phát triển trên http://localhost — trình duyệt không gửi cookie
	// Secure qua HTTP. Môi trường thật LUÔN bật.
	SecureCookie bool
}

// New khởi tạo module identity.
func New(cfg Config) (*Module, error) {
	if cfg.Storage != "" && cfg.Storage != "postgres" {
		return nil, errors.New(
			"identity: chỉ hỗ trợ kho lưu trữ postgres — email duy nhất và " +
				"token duy nhất cần chỉ mục UNIQUE ở tầng database")
	}
	if cfg.DB == nil {
		return nil, errors.New("identity: bắt buộc phải có kết nối database")
	}
	if cfg.Issuer == nil {
		return nil, errors.New(
			"identity: bắt buộc phải có Issuer — không có nó thì đăng nhập " +
				"thành công nhưng không cấp được access token")
	}

	pool := cfg.DB.Pool()

	return &Module{
		svc: application.NewService(application.Deps{
			Users:    identitypg.NewUserStore(pool),
			Sessions: identitypg.NewSessionStore(pool),
			Attempts: identitypg.NewLoginAttemptStore(pool),
			Hasher:   crypto.NewBcryptHasher(cfg.BcryptCost),
			Tokens:   crypto.NewTokenGenerator(),
			Clock:    cfg.Clock,
		}),
		issuer:       cfg.Issuer,
		secureCookie: cfg.SecureCookie,
	}, nil
}

// RegisterRoutes gắn các endpoint công khai của module vào mux.
//
// CHỈ các endpoint KHÔNG cần xác thực: login, refresh, logout. Endpoint cần
// xác thực đăng ký qua RegisterProtectedRoutes.
func (m *Module) RegisterRoutes(mux *http.ServeMux, log *slog.Logger) {
	identityhttp.NewHandler(m.svc, m.issuer, m.secureCookie, log).Register(mux)
}

// RegisterProtectedRoutes gắn các endpoint CẦN xác thực.
//
// Tách riêng vì mux nhận vào đây phải đã bọc middleware Auth. Gộp chung với
// RegisterRoutes sẽ khiến người nối dây dễ quên, và quên ở đây nghĩa là
// endpoint trả dữ liệu tài khoản cho request chưa đăng nhập.
func (m *Module) RegisterProtectedRoutes(mux *http.ServeMux, log *slog.Logger) {
	identityhttp.NewHandler(m.svc, m.issuer, m.secureCookie, log).RegisterProtected(mux)
}

// VerifyAccessToken cài đặt httpserver.TokenVerifier.
//
// Đây là cầu nối giữ được quy tắc R3: platform khai báo interface nó cần,
// module này cài đặt. Chiều phụ thuộc là identity → platform, nên platform
// không bao giờ phải biết module nghiệp vụ nào tồn tại.
//
// # Vì sao KHÔNG tra database ở đây
//
// Access token tự chứa và có thời hạn 15 phút. Tra database mỗi request để
// kiểm tra phiên còn sống sẽ thêm một truy vấn vào MỌI lời gọi API — cái
// giá phải trả ở đường đi nóng nhất của hệ thống.
//
// Đánh đổi: một tài khoản bị treo vẫn dùng được tối đa 15 phút. Đó là lý do
// TTL ngắn, và là quyết định đã ghi ở domain/session.go.
func (m *Module) VerifyAccessToken(
	_ context.Context, accessToken string,
) (httpserver.AuthContext, error) {
	claims, err := m.issuer.Verify(accessToken)
	if err != nil {
		return httpserver.AuthContext{}, err
	}

	return httpserver.AuthContext{
		UserID:    claims.UserID,
		Roles:     claims.Roles,
		Scope:     claims.Scope,
		SellerIDs: claims.SellerIDs,
		SessionID: claims.SessionID,
	}, nil
}

// Service trả về tầng application cho tầng interfaces của CHÍNH module này.
func (m *Module) Service() *application.Service { return m.svc }

// ---------------------------------------------------------------- API

func (m *Module) Register(ctx context.Context, req RegisterRequest) (UserView, error) {
	roles := make([]domain.RoleGrant, 0, len(req.Roles))
	for _, r := range req.Roles {
		role, err := parseRole(r.Role)
		if err != nil {
			return UserView{}, err
		}
		roles = append(roles, domain.RoleGrant{
			Role:    role,
			ScopeID: ids.ID(r.ScopeID),
		})
	}

	u, err := m.svc.Register(ctx, application.RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		Phone:       req.Phone,
		DisplayName: req.DisplayName,
		Roles:       roles,
	})
	if err != nil {
		return UserView{}, translateErr(err)
	}
	return toUserView(u), nil
}

func (m *Module) Login(ctx context.Context, req LoginRequest) (AuthResult, error) {
	res, err := m.svc.Login(ctx, application.LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		UserAgent: req.UserAgent,
		// Băm IP NGAY tại biên module: bên trong không bao giờ thấy địa
		// chỉ nguyên văn, nên không có chỗ nào lỡ tay ghi nó ra nhật ký.
		IPHash: crypto.HashIP(req.IP),
	})
	if err != nil {
		return AuthResult{}, translateErr(err)
	}
	return toAuthResult(res), nil
}

func (m *Module) Refresh(ctx context.Context, req RefreshRequest) (AuthResult, error) {
	res, err := m.svc.Refresh(ctx, req.RefreshToken, req.UserAgent, crypto.HashIP(req.IP))
	if err != nil {
		return AuthResult{}, translateErr(err)
	}
	return toAuthResult(res), nil
}

func (m *Module) Logout(ctx context.Context, refreshToken string) error {
	return translateErr(m.svc.Logout(ctx, refreshToken))
}

func (m *Module) Authenticate(ctx context.Context, refreshToken string) (AuthContext, error) {
	u, sess, err := m.svc.Authenticate(ctx, refreshToken)
	if err != nil {
		return AuthContext{}, translateErr(err)
	}

	grants := u.Roles()
	roles := make([]string, 0, len(grants))
	for _, g := range grants {
		roles = append(roles, string(g.Role))
	}

	// Gom seller_id của CẢ HAI vai trò seller: chủ gian hàng và nhân viên
	// đều phải thấy dữ liệu gian hàng đó. Bỏ sót một vai trò là chặn nhầm
	// người có quyền.
	var sellerIDs []string
	for _, r := range []domain.Role{domain.RoleSellerOwner, domain.RoleSellerStaff} {
		for _, id := range u.ScopeIDsFor(r) {
			sellerIDs = append(sellerIDs, id.String())
		}
	}

	return AuthContext{
		UserID:    u.ID().String(),
		Roles:     roles,
		Scope:     string(u.EffectiveScope()),
		SellerIDs: dedup(sellerIDs),
		SessionID: sess.ID().String(),
	}, nil
}

func (m *Module) GetUser(ctx context.Context, userID string) (UserView, error) {
	u, err := m.svc.GetUser(ctx, ids.ID(userID))
	if err != nil {
		return UserView{}, translateErr(err)
	}
	return toUserView(u), nil
}

func (m *Module) ListSessions(ctx context.Context, userID string) ([]SessionView, error) {
	sessions, err := m.svc.ListSessions(ctx, ids.ID(userID))
	if err != nil {
		return nil, translateErr(err)
	}

	out := make([]SessionView, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, SessionView{
			ID:         s.ID().String(),
			UserAgent:  s.UserAgent(),
			ExpiresAt:  s.ExpiresAt(),
			CreatedAt:  s.CreatedAt(),
			LastUsedAt: s.LastUsedAt(),
		})
	}
	return out, nil
}

func (m *Module) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	return translateErr(m.svc.ChangePassword(ctx, ids.ID(userID), oldPassword, newPassword))
}

func (m *Module) GrantRole(ctx context.Context, userID, role, scopeID string) error {
	r, err := parseRole(role)
	if err != nil {
		return err
	}
	return translateErr(m.svc.GrantRole(ctx, ids.ID(userID), r, ids.ID(scopeID)))
}

func (m *Module) RevokeRole(ctx context.Context, userID, role, scopeID string) error {
	r, err := parseRole(role)
	if err != nil {
		return err
	}
	return translateErr(m.svc.RevokeRole(ctx, ids.ID(userID), r, ids.ID(scopeID)))
}

func (m *Module) Suspend(ctx context.Context, userID string) error {
	return translateErr(m.svc.Suspend(ctx, ids.ID(userID)))
}

// ---------------------------------------------------------------- Chuyển đổi

func toUserView(u *domain.User) UserView {
	grants := u.Roles()
	roles := make([]RoleGrantView, 0, len(grants))
	for _, g := range grants {
		roles = append(roles, RoleGrantView{
			Role:      string(g.Role),
			ScopeID:   g.ScopeID.String(),
			GrantedAt: g.GrantedAt,
		})
	}

	return UserView{
		ID:            u.ID().String(),
		Email:         u.Email(),
		Phone:         u.Phone(),
		DisplayName:   u.DisplayName(),
		Status:        string(u.Status()),
		Roles:         roles,
		EmailVerified: !u.EmailVerifiedAt().IsZero(),
		CreatedAt:     u.CreatedAt(),
	}
}

func toAuthResult(r *application.LoginResult) AuthResult {
	return AuthResult{
		User:           toUserView(r.User),
		RefreshToken:   r.RefreshToken,
		SessionID:      r.SessionID.String(),
		ExpiresAt:      r.ExpiresAt,
		AccessTokenTTL: domain.AccessTokenTTL,
	}
}

// parseRole đổi chuỗi thành vai trò, TỪ CHỐI chuỗi lạ.
//
// Không dùng domain.Role(s) trực tiếp: Go cho phép ép kiểu bất kỳ chuỗi
// nào, nên "SUPER_ADMIN" sẽ được ghi thẳng vào database. Ràng buộc CHECK
// sẽ chặn, nhưng lỗi lúc đó là lỗi database khó đọc thay vì lỗi đầu vào rõ
// ràng.
func parseRole(s string) (domain.Role, error) {
	switch domain.Role(s) {
	case domain.RoleCustomer, domain.RoleSellerOwner, domain.RoleSellerStaff,
		domain.RoleCreator, domain.RoleAdmin, domain.RoleOpsWarehouse,
		domain.RoleOpsMerchandising, domain.RoleOpsFinance, domain.RoleOpsSupport:
		return domain.Role(s), nil
	}
	return "", ErrInvalidInput
}

func dedup(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
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
	case errors.Is(err, domain.ErrDuplicateEmail):
		return ErrDuplicateEmail
	case errors.Is(err, domain.ErrWeakPassword):
		return ErrWeakPassword
	case errors.Is(err, domain.ErrInvalidEmail):
		return ErrInvalidInput
	case errors.Is(err, domain.ErrInvalidLogin):
		return ErrInvalidLogin
	case errors.Is(err, domain.ErrAccountLocked):
		return ErrAccountLocked
	case errors.Is(err, domain.ErrAccountSuspended):
		return ErrAccountSuspended
	case errors.Is(err, domain.ErrSessionInvalid):
		return ErrSessionInvalid
	default:
		return err
	}
}
