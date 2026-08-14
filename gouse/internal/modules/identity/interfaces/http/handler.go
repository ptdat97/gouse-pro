// Package http là tầng interfaces của module identity: chuyển HTTP request
// thành lời gọi use case và chuyển kết quả thành JSON đúng đặc tả OpenAPI.
//
// Tầng này KHÔNG chứa quy tắc nghiệp vụ. Tên trường JSON lấy TỪ đặc tả
// api/paths/auth.yaml — đặc tả là nguồn sự thật, không phải struct Go.
//
// # Hai loại token nằm ở hai nơi khác nhau
//
//	Access token   → body JSON, client giữ TRONG BỘ NHỚ
//	Refresh token  → cookie HttpOnly, JavaScript KHÔNG đọc được
//
// Vì sao tách: localStorage đọc được bằng JavaScript, nên một lỗ hổng XSS
// duy nhất là mất tài khoản. Access token trong bộ nhớ mất khi tải lại
// trang — đó chính là lý do cần refresh token ở cookie.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
	"github.com/fashion-commerce/platform/internal/modules/identity/application"
	"github.com/fashion-commerce/platform/internal/modules/identity/domain"
	"github.com/fashion-commerce/platform/internal/platform/apierror"
	"github.com/fashion-commerce/platform/internal/platform/httpserver"
	"github.com/fashion-commerce/platform/internal/platform/logger"
	// privacy.HashIP chứ KHÔNG phải crypto.HashIP của chính module này:
	// R8 cấm interfaces import infrastructure. Hai hàm cho cùng kết quả —
	// crypto.HashIP chỉ là wrapper gọi thẳng privacy.HashIP.
	"github.com/fashion-commerce/platform/internal/platform/privacy"
	"github.com/fashion-commerce/platform/internal/platform/token"
)

// refreshCookieName là tên cookie chứa refresh token.
const refreshCookieName = "refresh_token"

// Handler phục vụ các endpoint xác thực.
type Handler struct {
	svc    *application.Service
	issuer *token.Issuer
	log    *slog.Logger

	// secureCookie bật cờ Secure trên cookie.
	//
	// Tắt được để phát triển trên http://localhost — trình duyệt không gửi
	// cookie Secure qua HTTP, nên bật cứng sẽ làm đăng nhập không chạy được
	// ở máy lập trình viên. Môi trường thật LUÔN bật.
	secureCookie bool
}

// NewHandler tạo handler.
func NewHandler(
	svc *application.Service, issuer *token.Issuer, secureCookie bool, log *slog.Logger,
) *Handler {
	return &Handler{svc: svc, issuer: issuer, secureCookie: secureCookie, log: log}
}

// Register gắn route vào mux.
//
// Đường dẫn khớp CHÍNH XÁC với api/openapi.yaml.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/auth/login", http.HandlerFunc(h.login))
	mux.Handle("POST /api/v1/auth/refresh", http.HandlerFunc(h.refresh))
	mux.Handle("POST /api/v1/auth/logout", http.HandlerFunc(h.logout))
}

// RegisterProtected gắn route CẦN xác thực.
//
// Tách khỏi Register vì các route này phải nằm sau middleware Auth. Gộp
// chung sẽ khiến người nối dây dễ quên, và quên ở đây nghĩa là endpoint trả
// dữ liệu tài khoản cho request chưa đăng nhập.
func (h *Handler) RegisterProtected(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/admin/me", http.HandlerFunc(h.adminMe))
}

// ---------------------------------------------------------------- Login

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResult struct {
	AccessToken string   `json:"access_token"`
	ExpiresIn   int      `json:"expires_in"`
	User        userInfo `json:"user"`
}

type userInfo struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name,omitempty"`
	Roles       []string `json:"roles"`
}

// login phục vụ POST /api/v1/auth/login (operationId: login).
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		h.fail(w, r, err)
		return
	}

	res, err := h.svc.Login(r.Context(), application.LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		UserAgent: r.UserAgent(),
		IPHash:    privacy.HashIP(clientIP(r)),
	})
	if err != nil {
		h.fail(w, r, translateAuthErr(err))
		return
	}

	h.writeAuthResult(w, r, res)
}

// ---------------------------------------------------------------- Refresh

// refresh phục vụ POST /api/v1/auth/refresh (operationId: refreshSession).
//
// Đọc refresh token từ COOKIE, không từ body — token nằm trong body sẽ đi
// qua JavaScript, và khi đó cookie HttpOnly mất hết ý nghĩa.
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		h.fail(w, r, apierror.New(apierror.CodeUnauthorized,
			"Phiên đăng nhập không hợp lệ hoặc đã hết hạn"))
		return
	}

	res, err := h.svc.Refresh(r.Context(),
		cookie.Value, r.UserAgent(), privacy.HashIP(clientIP(r)))
	if err != nil {
		// Xóa cookie hỏng để trình duyệt không gửi lại mãi. Không xóa thì
		// client lặp vô hạn: gửi token chết → 401 → thử refresh → 401.
		h.clearRefreshCookie(w)
		h.fail(w, r, translateAuthErr(err))
		return
	}

	h.writeAuthResult(w, r, res)
}

// ---------------------------------------------------------------- Logout

// logout phục vụ POST /api/v1/auth/logout (operationId: logout).
//
// IDEMPOTENT: luôn trả 204, kể cả khi phiên không tồn tại hoặc đã thu hồi.
// Kết quả mong muốn — phiên không còn dùng được — đã đạt được, và báo lỗi ở
// đây chỉ tiết lộ token nào từng có thật.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookieName); err == nil && cookie.Value != "" {
		if err := h.svc.Logout(r.Context(), cookie.Value); err != nil {
			// Ghi log nhưng KHÔNG báo lỗi cho client.
			h.log.WarnContext(r.Context(), "đăng xuất thất bại", "error", err)
		}
	}

	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- Admin me

type adminMe struct {
	ID                string   `json:"id"`
	Email             string   `json:"email"`
	DisplayName       string   `json:"display_name,omitempty"`
	Roles             []string `json:"roles"`
	Scope             string   `json:"scope"`
	RequiresTwoFactor bool     `json:"requires_two_factor"`
}

// adminMe phục vụ GET /api/v1/admin/me (operationId: getAdminMe).
//
// Trả vai trò để giao diện dựng menu. Đây CHỈ là trải nghiệm — backend vẫn
// kiểm tra lại quyền ở từng endpoint, vì người dùng gọi API trực tiếp được.
func (h *Handler) adminMe(w http.ResponseWriter, r *http.Request) {
	ac, ok := httpserver.AuthContextFrom(r.Context())
	if !ok {
		// Chỉ xảy ra khi route bị nối thiếu middleware Auth.
		h.log.ErrorContext(r.Context(),
			"/api/v1/admin/me chạy không qua Auth — kiểm tra nối dây")
		h.fail(w, r, apierror.ErrUnauthorized)
		return
	}

	u, err := h.svc.GetUser(r.Context(), ids.ID(ac.UserID))
	if err != nil {
		h.fail(w, r, translateAuthErr(err))
		return
	}

	roles := roleStrings(u)

	h.ok(w, r, adminMe{
		ID:                u.ID().String(),
		Email:             u.Email(),
		DisplayName:       u.DisplayName(),
		Roles:             roles,
		Scope:             string(u.EffectiveScope()),
		RequiresTwoFactor: requiresTwoFactor(u),
	})
}

// ---------------------------------------------------------------- Hỗ trợ

// writeAuthResult phát hành access token và đặt cookie refresh token.
func (h *Handler) writeAuthResult(
	w http.ResponseWriter, r *http.Request, res *application.LoginResult,
) {
	roles := roleStrings(res.User)

	sellerIDs := make([]string, 0)
	for _, role := range []domain.Role{domain.RoleSellerOwner, domain.RoleSellerStaff} {
		for _, id := range res.User.ScopeIDsFor(role) {
			sellerIDs = append(sellerIDs, id.String())
		}
	}

	access, err := h.issuer.Issue(token.Claims{
		UserID:    res.User.ID().String(),
		Roles:     roles,
		Scope:     string(res.User.EffectiveScope()),
		SellerIDs: dedup(sellerIDs),
		SessionID: res.SessionID.String(),
	}, h.svc.Now())
	if err != nil {
		h.fail(w, r, apierror.Wrap(err, apierror.CodeInternalError,
			"Không phát hành được token"))
		return
	}

	h.setRefreshCookie(w, res.RefreshToken, res.ExpiresAt)

	h.ok(w, r, authResult{
		AccessToken: access,
		ExpiresIn:   int(h.issuer.TTL().Seconds()),
		User: userInfo{
			ID:          res.User.ID().String(),
			Email:       res.User.Email(),
			DisplayName: res.User.DisplayName(),
			Roles:       roles,
		},
	})
}

// setRefreshCookie đặt refresh token vào cookie HttpOnly.
//
// Bốn thuộc tính, mỗi cái chặn một đường tấn công:
//
//	HttpOnly          JavaScript không đọc được → XSS không lấy được token
//	Secure            chỉ gửi qua HTTPS → không lộ khi nghe lén mạng
//	SameSite=Strict   không gửi kèm request từ site khác → chặn CSRF
//	Path=/api/v1/auth chỉ gửi tới endpoint xác thực → không đi kèm mọi request
func (h *Handler) setRefreshCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     "/api/v1/auth",
		Expires:  expires,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearRefreshCookie xóa cookie refresh token.
//
// Thuộc tính phải khớp CHÍNH XÁC với lúc đặt (nhất là Path) — lệch một
// thuộc tính là trình duyệt giữ nguyên cookie cũ và người dùng không đăng
// xuất được.
func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) ok(w http.ResponseWriter, r *http.Request, body any) {
	if err := apierror.WriteJSON(w, http.StatusOK, body); err != nil {
		h.log.ErrorContext(r.Context(), "không ghi được response",
			"error", err, "path", r.URL.Path)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierror.Write(w, r, err, logger.RequestIDFromContext(r.Context()), h.log)
}

// decodeJSON đọc body JSON.
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	// Trường lạ là dấu hiệu client hiểu sai hợp đồng — báo sớm tốt hơn là
	// âm thầm bỏ qua rồi để họ tự hỏi vì sao dữ liệu không được lưu.
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return apierror.New(apierror.CodeValidationFailed,
			"Dữ liệu gửi lên không hợp lệ")
	}
	return nil
}

// translateAuthErr chuyển lỗi domain thành lỗi API.
//
// # Mọi lý do đăng nhập thất bại đều trả CÙNG MỘT lỗi
//
// Email không tồn tại, sai mật khẩu, tài khoản bị treo — tất cả thành 401
// với cùng thông báo. Phân biệt chúng cho client là để lộ email nào có tài
// khoản thật, biến đường đăng nhập thành công cụ dò danh sách người dùng.
//
// NGOẠI LỆ có chủ ý: ErrAccountLocked. Người dùng thật cần biết vì sao mật
// khẩu đúng mà vẫn không vào được — không thì họ thử lại liên tục và tự kéo
// dài thời gian khóa của mình.
func translateAuthErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrAccountLocked):
		return apierror.New(apierror.CodeUnauthorized,
			"Tài khoản tạm khóa do đăng nhập sai nhiều lần, vui lòng thử lại sau")

	case errors.Is(err, domain.ErrInvalidLogin),
		errors.Is(err, domain.ErrAccountSuspended),
		errors.Is(err, domain.ErrSessionInvalid),
		errors.Is(err, domain.ErrNotFound):
		return apierror.New(apierror.CodeUnauthorized,
			"Email hoặc mật khẩu không đúng")

	default:
		return apierror.From(err)
	}
}

// roleStrings lấy danh sách vai trò dạng chuỗi.
func roleStrings(u *domain.User) []string {
	grants := u.Roles()
	roles := make([]string, 0, len(grants))
	for _, g := range grants {
		roles = append(roles, string(g.Role))
	}
	return roles
}

// requiresTwoFactor cho biết tài khoản có bắt buộc xác thực hai lớp không.
//
// Luồng 2FA CHƯA triển khai. Trường này báo cho giao diện biết tài khoản nào
// sẽ cần, để không phát hành Admin UI khi chưa có — xem
// docs/06-api/admin-api.md mục 1.
func requiresTwoFactor(u *domain.User) bool {
	for _, g := range u.Roles() {
		if g.Role.RequiresTwoFactor() {
			return true
		}
	}
	return false
}

// clientIP lấy địa chỉ IP của client.
//
// Ưu tiên X-Forwarded-For vì hệ thống chạy sau bộ cân bằng tải; giá trị đầu
// tiên là IP gốc, phần sau là chuỗi proxy.
//
// Header này client tự đặt được, nên nó KHÔNG dùng cho phân quyền — chỉ để
// phát hiện "nhiều lần thử từ cùng một nguồn". Giá trị được BĂM trước khi
// lưu: IP nguyên văn là dữ liệu cá nhân, cần cơ sở pháp lý để giữ.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return strings.TrimSpace(rip)
	}
	return r.RemoteAddr
}

// dedup loại bỏ giá trị trùng, giữ nguyên thứ tự.
func dedup(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
