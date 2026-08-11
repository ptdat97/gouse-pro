# Module: Identity

| | |
|---|---|
| **Bounded Context** | Platform |
| **Phân loại** | Generic |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý tài khoản đăng nhập (`User`)
- Xác thực: đăng nhập, đăng xuất, quản lý token
- Quản lý vai trò và quyền hệ thống
- Đăng nhập bằng mạng xã hội, xác thực hai lớp

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Hồ sơ khách hàng (địa chỉ, sở thích) | `customer` |
| Hồ sơ seller | `seller` |
| Hồ sơ creator | `creator` |
| **Phân quyền nghiệp vụ** ("user này xem được đơn này không") | Module sở hữu dữ liệu |

### Ranh giới quan trọng: xác thực vs phân quyền

```text
identity trả lời:     "Đây là user_id=123, có vai trò SELLER"
                      → hạ tầng, trung lập domain

module order trả lời: "user_id=123 có được xem order #1000 không?"
                      → quyết định nghiệp vụ
```

Nếu `identity` phải biết mọi quy tắc truy cập dữ liệu, nó sẽ phụ thuộc vào toàn hệ thống — vi phạm nguyên tắc P12.

---

## 3. Tách User khỏi hồ sơ nghiệp vụ

```text
User (identity)
  ├── Customer profile  (module customer)
  ├── Seller profile    (module seller)
  ├── Creator profile   (module creator)
  └── Staff profile     (module identity — nhân viên nội bộ)
```

**Vì sao tách:** một người có thể vừa là khách hàng, vừa là creator, vừa là seller. Nếu gộp `User` và `Customer` làm một, không mô hình hóa được.

Ví dụ thực tế phổ biến: một KOC vừa bán hàng trên marketplace, vừa làm nội dung affiliate, vừa mua sắm cho bản thân.

---

## 4. Mô hình vai trò và quyền

```text
User → có nhiều Role → mỗi Role có nhiều Permission

Vai trò hệ thống:
    CUSTOMER
    SELLER_OWNER          — chủ gian hàng
    SELLER_STAFF          — nhân viên gian hàng
    CREATOR
    ADMIN                 — quản trị nền tảng
    OPS_WAREHOUSE         — nhân viên kho
    OPS_MERCHANDISING     — nhân viên hàng hóa
    OPS_FINANCE           — nhân viên tài chính
    OPS_SUPPORT           — chăm sóc khách hàng
```

### Phạm vi quyền (scope)

Quyền không chỉ là "được làm gì" mà còn "trên phạm vi nào":

```text
Permission {
    action: "order.read"
    scope:  OWN | SELLER | ALL
}

Ví dụ:
    Khách hàng:      order.read scope=OWN     (chỉ đơn của mình)
    Seller:          order.read scope=SELLER  (chỉ đơn thuộc gian hàng mình)
    Nhân viên CSKH:  order.read scope=ALL
```

**Quan trọng:** phạm vi được `identity` cung cấp, nhưng việc **áp dụng** phạm vi vào truy vấn là trách nhiệm của module sở hữu dữ liệu.

---

## 5. Xác thực

### Token

```text
Access token   — thời hạn ngắn (ví dụ 15 phút)
Refresh token  — thời hạn dài (ví dụ 30 ngày), có thể thu hồi
```

Access token chứa: `user_id`, các vai trò, thời hạn. Không chứa dữ liệu nhạy cảm.

### Yêu cầu bảo mật

| Yêu cầu | Lý do |
|---|---|
| Mật khẩu băm bằng thuật toán chậm, có muối | Chống dò mật khẩu |
| Giới hạn số lần đăng nhập sai | Chống thử vét cạn |
| Refresh token thu hồi được | Xử lý khi tài khoản bị lộ |
| Ghi log mọi lần đăng nhập | Phát hiện bất thường |
| Xác thực hai lớp cho vai trò nhạy cảm | Bảo vệ tài khoản admin, tài chính |
| Phiên đăng nhập có thể xem và thu hồi | Người dùng tự kiểm soát |

**Bắt buộc xác thực hai lớp** cho: `ADMIN`, `OPS_FINANCE`, và `SELLER_OWNER` (vì liên quan tới tiền).

---

## 6. Dữ liệu sở hữu

```sql
"user"
user_credential         -- mật khẩu đã băm
user_social_account     -- liên kết mạng xã hội
role
permission
role_permission
user_role
session                 -- phiên đăng nhập
login_attempt           -- lịch sử đăng nhập
```

---

## 7. Interface công khai

```go
type PublicAPI interface {
    // Xác thực
    Authenticate(ctx, req AuthRequest) (*AuthResult, error)
    ValidateToken(ctx, token string) (*TokenClaims, error)
    RefreshToken(ctx, refreshToken string) (*AuthResult, error)
    RevokeSession(ctx, sessionID string) error

    // Truy vấn
    GetUser(ctx, userID string) (*UserView, error)
    GetUserRoles(ctx, userID string) ([]Role, error)
    HasPermission(ctx, userID string, action string) (bool, Scope, error)

    // Quản lý
    CreateUser(ctx, req CreateUserRequest) (*UserView, error)
    AssignRole(ctx, userID, roleID string) error
    RemoveRole(ctx, userID, roleID string) error
}
```

---

## 8. Event

### Phát ra

| Event | Khi nào |
|---|---|
| `user.registered` | Đăng ký tài khoản |
| `user.logged_in` | Đăng nhập (cho analytics, phát hiện bất thường) |
| `user.role_assigned` | Cấp vai trò |
| `user.suspended` | Khóa tài khoản |
| `user.password_changed` | Đổi mật khẩu |

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `seller.approved` | seller | Cấp vai trò `SELLER_OWNER` |
| `creator.approved` | creator | Cấp vai trò `CREATOR` |

Đây là **ngoại lệ có kiểm soát** của quy tắc "identity không phụ thuộc module nghiệp vụ": nó chỉ **nghe** event, không **gọi** module nào. Phụ thuộc một chiều, không tạo vòng.

---

## 9. Phụ thuộc

```text
Gọi đồng bộ:   (KHÔNG gọi module nghiệp vụ nào — bắt buộc)
Nghe event:    seller, creator (chỉ để cấp vai trò)
Được gọi bởi:  mọi module (qua middleware xác thực)
```

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | `identity` không gọi module nghiệp vụ nào |
| 2 | Tách `User` khỏi hồ sơ nghiệp vụ |
| 3 | Không lưu mật khẩu dạng gốc |
| 4 | Xác thực hai lớp bắt buộc cho vai trò nhạy cảm |
| 5 | Phân quyền nghiệp vụ thuộc module sở hữu dữ liệu |
| 6 | Mọi lần đăng nhập được ghi log |
| 7 | Token có thời hạn ngắn, refresh token thu hồi được |

---

## 11. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Đăng ký, đăng nhập, vai trò cơ bản, token |
| **Phase 2** | Đăng nhập mạng xã hội, xác thực hai lớp, quản lý phiên |
| **Phase 3** | Phân quyền chi tiết, tài khoản nhân viên cho seller |

---

## 12. Tài liệu liên quan

- [../09-operations/security.md](../09-operations/security.md) — bảo mật tổng thể
- [customer.md](customer.md) — hồ sơ khách hàng
- [../03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md) — vì sao identity phải độc lập
