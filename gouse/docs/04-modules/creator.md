# Module: Creator

| | |
|---|---|
| **Bounded Context** | Growth |
| **Phân loại** | **Core** |
| **Giai đoạn** | Phase 2 |

---

## 1. Trách nhiệm

- Quản lý hồ sơ và danh tính creator
- Quy trình đăng ký và phê duyệt
- Liên kết kênh mạng xã hội
- Quản lý tài khoản nhận tiền
- Phân hạng creator

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Nội dung | `content` |
| Link affiliate, quy kết, hoa hồng | `affiliate` |
| Chiến dịch | `campaign` |
| Chi trả tiền | `payment` |

**Vì sao tách ba module `creator` / `content` / `affiliate`:**

```text
- Gộp cả ba tạo một module quá lớn
- Quan trọng hơn: `content` phải dùng được cho cả nội dung
  do nền tảng tự sản xuất (không có creator nào)
  → nếu content phụ thuộc creator, không làm được điều đó
```

---

## 3. Năm loại creator

```text
KOC · KOL · Influencer · Stylist · Content Partner
```

Khác biệt quan trọng nhất là **cơ chế trả tiền**:

| Loại | Mô hình trả tiền | Vai trò chiến lược |
|---|---|---|
| KOC | Hoa hồng thuần | Chuyển đổi cao |
| KOL | Phí cố định + hoa hồng | Nhận diện thương hiệu |
| Influencer | Phí cố định + hoa hồng | Phủ sóng |
| Stylist | Hoa hồng + phí tư vấn | **Giảm tỷ lệ hoàn hàng** |
| Content Partner | Hợp đồng | Uy tín thương hiệu |

**Hệ quả kiến trúc:** `Campaign` phải hỗ trợ cả ba cấu trúc chi phí (thuần hoa hồng, phí cố định, hỗn hợp). Không thiết kế chỉ với một trường `commission_rate`. Xem [campaign.md](campaign.md).

**Về Stylist:** vai trò đặc thù thời trang, tạo ra `Outfit`. Giá trị: tăng giá trị đơn hàng và giảm tỷ lệ hoàn hàng nhờ tư vấn size và phối đồ đúng.

---

## 4. Quyền hạn và ranh giới quyền riêng tư

| Hành động | Được phép |
|---|---|
| Xem hiệu suất nội dung của mình | Có |
| Xem số click, số đơn, doanh thu quy kết | Có (số liệu tổng hợp) |
| Xem hoa hồng của mình | Có |
| **Xem danh tính khách hàng** | **Không bao giờ** |

**Ràng buộc bắt buộc:** creator thấy **số liệu tổng hợp**, không thấy thông tin cá nhân của khách. Creator không phải bên xử lý dữ liệu cá nhân.

---

## 5. Chống gian lận

| Kiểu gian lận | Cách phát hiện |
|---|---|
| Click ảo | Phân tích IP, thiết bị, tần suất bất thường |
| Tự mua để lấy hoa hồng rồi hoàn | Đối chiếu định danh, tỷ lệ hoàn cao bất thường |
| Cookie stuffing | Tỷ lệ click/hiển thị bất thường |
| Nội dung sai lệch để bán được | Tỷ lệ hoàn hàng cao trên nội dung cụ thể |

**Nguyên tắc thiết kế:** phát hiện gian lận là **bước riêng, chạy bất đồng bộ**, không nằm trong đường đi chính của việc ghi nhận click — nếu không sẽ làm chậm trải nghiệm.

Dữ liệu cần cho việc phát hiện được ghi trong `affiliate.click`.

---

## 6. Dữ liệu sở hữu

```sql
creator
creator_channel          -- kênh mạng xã hội đã liên kết
creator_audience         -- số liệu người theo dõi
creator_bank_account
creator_tier             -- hạng creator
creator_specialty        -- phong cách, ngành hàng sở trường
```

---

## 7. Interface công khai

```go
type PublicAPI interface {
    GetCreator(ctx, creatorID string) (*CreatorView, error)
    GetCreatorsByIDs(ctx, ids []string) (map[string]CreatorView, error)
    IsCreatorActive(ctx, creatorID string) (bool, error)
    GetCreatorTier(ctx, creatorID string) (*TierView, error)

    ApplyAsCreator(ctx, req ApplicationRequest) (*CreatorView, error)
    ApproveCreator(ctx, creatorID string, approvedBy string) error
    SuspendCreator(ctx, creatorID string, reason string) error
}
```

---

## 8. Event

**Phát ra:** `creator.applied`, `creator.approved`, `creator.rejected`, `creator.suspended`, `creator.tier_changed`

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `affiliate.conversion_attributed` | affiliate | Cập nhật thống kê hiệu suất |
| `content.published` | content | Cập nhật thống kê nội dung |

---

## 9. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Creator ACTIVE phải có tài khoản nhận tiền đã xác minh |
| 2 | Creator không bao giờ thấy danh tính khách hàng |
| 3 | Không lưu số dư — gọi `payment.GetBalance()` |
| 4 | Phát hiện gian lận chạy bất đồng bộ |
| 5 | Một người có thể vừa là creator vừa là seller vừa là khách |

---

## 10. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **Phase 2** | Đăng ký, duyệt, hồ sơ, liên kết kênh |
| **Phase 3** | Phân hạng, chống gian lận, chỉ số hiệu suất |
| **Phase 4** | Creator marketplace (kết nối brand ↔ creator) |

---

## 11. Tài liệu liên quan

- [../01-business/creator.md](../01-business/creator.md) — tác nhân creator
- [affiliate.md](affiliate.md) — quy kết và hoa hồng
- [content.md](content.md) — nội dung
