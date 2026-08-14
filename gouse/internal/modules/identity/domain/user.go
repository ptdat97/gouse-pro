// Package domain chứa mô hình nghiệp vụ của module identity.
//
// RANH GIỚI QUAN TRỌNG NHẤT (identity.md mục 2):
//
//	identity trả lời:  "Đây là user_id=123, có vai trò SELLER"
//	                   → hạ tầng, TRUNG LẬP với domain
//
//	module order:      "user_id=123 có được xem order #1000 không?"
//	                   → quyết định NGHIỆP VỤ
//
// Nếu identity phải biết mọi quy tắc truy cập dữ liệu, nó sẽ phụ thuộc
// toàn hệ thống. Vì vậy nó chỉ cung cấp DANH TÍNH và PHẠM VI; việc áp
// dụng phạm vi vào truy vấn là trách nhiệm của module sở hữu dữ liệu.
//
// # Tách User khỏi hồ sơ nghiệp vụ
//
//	User (identity)
//	  ├── Customer profile  (module customer)
//	  ├── Seller profile    (module seller)
//	  └── Creator profile   (module creator)
//
// Một người có thể vừa là khách, vừa là creator, vừa là seller — ví dụ
// một KOC bán hàng trên sàn, làm nội dung affiliate, và mua sắm cho bản
// thân. Gộp User với Customer thì không mô hình hóa được.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/fashion-commerce/platform/internal/kernel/ids"
)

var (
	ErrNotFound         = errors.New("identity: không tìm thấy")
	ErrInvalidEmail     = errors.New("identity: email không hợp lệ")
	ErrWeakPassword     = errors.New("identity: mật khẩu quá ngắn")
	ErrDuplicateEmail   = errors.New("identity: email đã được dùng")
	ErrInvalidLogin     = errors.New("identity: email hoặc mật khẩu không đúng")
	ErrAccountLocked    = errors.New("identity: tài khoản tạm khóa do đăng nhập sai nhiều lần")
	ErrAccountSuspended = errors.New("identity: tài khoản đã bị khóa")
	ErrSessionInvalid   = errors.New("identity: phiên đăng nhập không hợp lệ hoặc đã hết hạn")
)

// MinPasswordLength là độ dài tối thiểu.
//
// Tám ký tự là mức tối thiểu tuyệt đối, không phải mức khuyến nghị. Độ dài
// quan trọng hơn độ phức tạp: bắt buộc ký tự đặc biệt khiến người dùng
// chọn "Password1!" — dễ đoán hơn một cụm từ dài.
const MinPasswordLength = 8

// Status là trạng thái tài khoản.
type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusSuspended Status = "SUSPENDED"
	StatusDeleted   Status = "DELETED"
)

// CanLogin cho biết trạng thái này còn đăng nhập được không.
func (s Status) CanLogin() bool { return s == StatusActive }

// Role là vai trò hệ thống.
type Role string

const (
	RoleCustomer         Role = "CUSTOMER"
	RoleSellerOwner      Role = "SELLER_OWNER"
	RoleSellerStaff      Role = "SELLER_STAFF"
	RoleCreator          Role = "CREATOR"
	RoleAdmin            Role = "ADMIN"
	RoleOpsWarehouse     Role = "OPS_WAREHOUSE"
	RoleOpsMerchandising Role = "OPS_MERCHANDISING"
	RoleOpsFinance       Role = "OPS_FINANCE"
	RoleOpsSupport       Role = "OPS_SUPPORT"
)

// RequiresTwoFactor cho biết vai trò này BẮT BUỘC xác thực hai lớp.
//
// Ba vai trò dưới đây đều chạm tới TIỀN: admin có toàn quyền, ops_finance
// duyệt chi trả, seller_owner nhận tiền bán hàng. Tài khoản của họ bị lộ
// là mất tiền thật, không chỉ mất dữ liệu.
//
// MVP chưa cài xác thực hai lớp, nhưng hàm này tồn tại để nơi cần biết đã
// có sẵn câu trả lời — thêm 2FA sau không phải đi tìm chỗ nào cần bật.
func (r Role) RequiresTwoFactor() bool {
	switch r {
	case RoleAdmin, RoleOpsFinance, RoleSellerOwner:
		return true
	}
	return false
}

// Scope là phạm vi của một vai trò.
//
// Quyền không chỉ là "được làm gì" mà còn "TRÊN PHẠM VI NÀO":
//
//	Khách hàng:     order.read scope=OWN     (chỉ đơn của mình)
//	Seller:         order.read scope=SELLER  (chỉ đơn thuộc gian hàng)
//	Nhân viên CSKH: order.read scope=ALL
type Scope string

const (
	ScopeOwn    Scope = "OWN"
	ScopeSeller Scope = "SELLER"
	ScopeAll    Scope = "ALL"
)

// ScopeOf trả phạm vi mặc định của một vai trò.
//
// LƯU Ý: hàm này trả về phạm vi, KHÔNG áp dụng nó. Module sở hữu dữ liệu
// nhận phạm vi rồi tự đưa vào truy vấn của mình — identity không biết bảng
// nào tồn tại ở module nào.
func ScopeOf(r Role) Scope {
	switch r {
	case RoleAdmin, RoleOpsSupport, RoleOpsFinance,
		RoleOpsWarehouse, RoleOpsMerchandising:
		return ScopeAll
	case RoleSellerOwner, RoleSellerStaff:
		return ScopeSeller
	default:
		return ScopeOwn
	}
}

// RoleGrant là một vai trò đã cấp, kèm phạm vi cụ thể.
type RoleGrant struct {
	Role Role

	// ScopeID là thực thể mà vai trò gắn vào: seller_id với SELLER_*.
	//
	// Rỗng với vai trò không gắn thực thể nào (CUSTOMER, ADMIN).
	ScopeID ids.ID

	GrantedAt time.Time
}

// User là tài khoản đăng nhập.
//
// KHÔNG chứa hồ sơ nghiệp vụ: địa chỉ, ngày sinh, số đo cơ thể thuộc
// module customer. Ở đây chỉ có những gì cần để xác thực và phân vai.
type User struct {
	id ids.ID

	// email là định danh đăng nhập, LUÔN chữ thường.
	//
	// Khách gõ "Khach@Example.com" và "khach@example.com" phải vào cùng
	// một tài khoản — không thì họ tạo hai tài khoản rồi không hiểu vì
	// sao mất đơn hàng cũ.
	email string
	phone string

	displayName string
	status      Status

	emailVerifiedAt time.Time

	roles []RoleGrant

	createdAt time.Time
	updatedAt time.Time
}

type NewUserParams struct {
	Email       string
	Phone       string
	DisplayName string
	Now         time.Time
}

// NewUser tạo tài khoản mới ở trạng thái hoạt động.
func NewUser(p NewUserParams) (*User, error) {
	email := NormalizeEmail(p.Email)
	if !validEmail(email) {
		return nil, ErrInvalidEmail
	}

	id, err := ids.New(ids.PrefixUser)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &User{
		id:          id,
		email:       email,
		phone:       strings.TrimSpace(p.Phone),
		displayName: strings.TrimSpace(p.DisplayName),
		status:      StatusActive,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// RestoreUserParams dựng lại từ kho lưu trữ.
type RestoreUserParams struct {
	ID              ids.ID
	Email           string
	Phone           string
	DisplayName     string
	Status          Status
	EmailVerifiedAt time.Time
	Roles           []RoleGrant
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RestoreUser dựng lại mà không kiểm tra. CHỈ dùng ở infrastructure.
func RestoreUser(p RestoreUserParams) *User {
	return &User{
		id:              p.ID,
		email:           p.Email,
		phone:           p.Phone,
		displayName:     p.DisplayName,
		status:          p.Status,
		emailVerifiedAt: p.EmailVerifiedAt,
		roles:           p.Roles,
		createdAt:       p.CreatedAt,
		updatedAt:       p.UpdatedAt,
	}
}

func (u *User) ID() ids.ID                 { return u.id }
func (u *User) Email() string              { return u.email }
func (u *User) Phone() string              { return u.phone }
func (u *User) DisplayName() string        { return u.displayName }
func (u *User) Status() Status             { return u.status }
func (u *User) EmailVerifiedAt() time.Time { return u.emailVerifiedAt }
func (u *User) CreatedAt() time.Time       { return u.createdAt }
func (u *User) UpdatedAt() time.Time       { return u.updatedAt }

// Roles trả bản sao danh sách vai trò.
func (u *User) Roles() []RoleGrant {
	return append([]RoleGrant(nil), u.roles...)
}

// HasRole cho biết người dùng có vai trò này không.
func (u *User) HasRole(r Role) bool {
	for _, g := range u.roles {
		if g.Role == r {
			return true
		}
	}
	return false
}

// ScopeIDsFor trả các thực thể mà người dùng có vai trò này.
//
// Ví dụ: một người là SELLER_STAFF của hai gian hàng thì trả về hai
// seller_id. Module sở hữu dữ liệu dùng danh sách đó để lọc truy vấn.
func (u *User) ScopeIDsFor(r Role) []ids.ID {
	var out []ids.ID
	for _, g := range u.roles {
		if g.Role == r && !g.ScopeID.IsZero() {
			out = append(out, g.ScopeID)
		}
	}
	return out
}

// EffectiveScope trả phạm vi RỘNG NHẤT trong các vai trò của người dùng.
//
// Một người vừa là CUSTOMER vừa là OPS_SUPPORT thì phạm vi là ALL — vai
// trò rộng hơn thắng. Trả về phạm vi hẹp hơn sẽ chặn nhân viên hỗ trợ làm
// việc của họ.
func (u *User) EffectiveScope() Scope {
	scope := ScopeOwn
	for _, g := range u.roles {
		switch ScopeOf(g.Role) {
		case ScopeAll:
			return ScopeAll
		case ScopeSeller:
			scope = ScopeSeller
		}
	}
	return scope
}

// CanLogin cho biết tài khoản này còn đăng nhập được không.
func (u *User) CanLogin() bool { return u.status.CanLogin() }

// GrantRole cấp một vai trò.
//
// Idempotent: cấp lại vai trò đã có không tạo bản ghi thứ hai.
func (u *User) GrantRole(r Role, scopeID ids.ID, now time.Time) {
	for _, g := range u.roles {
		if g.Role == r && g.ScopeID == scopeID {
			return
		}
	}
	u.roles = append(u.roles, RoleGrant{Role: r, ScopeID: scopeID, GrantedAt: now})
	u.touch(now)
}

// RevokeRole thu hồi một vai trò.
func (u *User) RevokeRole(r Role, scopeID ids.ID, now time.Time) {
	for i, g := range u.roles {
		if g.Role == r && g.ScopeID == scopeID {
			u.roles = append(u.roles[:i], u.roles[i+1:]...)
			u.touch(now)
			return
		}
	}
}

// Suspend khóa tài khoản.
//
// KHÔNG xóa: tài khoản có đơn hàng, bút toán và nhật ký trỏ tới. Xóa cứng
// sẽ để lại dữ liệu mồ côi ở khắp hệ thống.
func (u *User) Suspend(now time.Time) {
	u.status = StatusSuspended
	u.touch(now)
}

// Reactivate mở khóa tài khoản.
func (u *User) Reactivate(now time.Time) {
	u.status = StatusActive
	u.touch(now)
}

// VerifyEmail đánh dấu email đã xác minh.
func (u *User) VerifyEmail(now time.Time) {
	u.emailVerifiedAt = now
	u.touch(now)
}

func (u *User) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	u.updatedAt = now
}

// ---------------------------------------------------------------- Tiện ích

// NormalizeEmail chuẩn hóa email về dạng so sánh được.
//
// Chữ thường và bỏ khoảng trắng. Không xử lý dấu chấm trong phần tên
// (Gmail coi "a.b@gmail.com" và "ab@gmail.com" là một, nhà cung cấp khác
// thì không) — đoán sai quy tắc của nhà cung cấp sẽ gộp nhầm hai người
// khác nhau thành một tài khoản.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// validEmail kiểm tra dạng cơ bản.
//
// CỐ Ý ĐƠN GIẢN: biểu thức chính quy đầy đủ theo RFC 5322 dài hàng trăm ký
// tự và vẫn chấp nhận những địa chỉ không tồn tại. Cách kiểm tra thật duy
// nhất là gửi email xác minh — thứ hệ thống này đã có.
func validEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	domain := s[at+1:]
	return strings.Contains(domain, ".") &&
		!strings.HasPrefix(domain, ".") &&
		!strings.HasSuffix(domain, ".") &&
		!strings.Contains(s, " ")
}
