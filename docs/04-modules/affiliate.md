# Module: Affiliate

| | |
|---|---|
| **Bounded Context** | Growth |
| **Phân loại** | **Core** |
| **Giai đoạn** | Phase 2 |

---

## 1. Trách nhiệm

- Tạo và quản lý affiliate link
- Ghi nhận click với đầy đủ ngữ cảnh
- **Quy kết** (attribution) đơn hàng cho creator
- Tính hoa hồng creator
- Đảo ngược quy kết khi hoàn hàng

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Hồ sơ creator | `creator` |
| Nội dung | `content` |
| **Ghi sổ và chi trả tiền** | `payment` |
| Quyết định tỷ lệ hoa hồng chiến dịch | `campaign` |

---

## 3. Attribution — bài toán trung tâm

### 3.1 Luồng cơ bản

```text
Creator đăng nội dung, tạo affiliate link
    ↓
Khách click  →  ghi nhận Click (thời điểm, phiên, thiết bị)
    ↓
Khách mua trong CỬA SỔ QUY KẾT (ví dụ 7 ngày)
    ↓
Đơn giao thành công, hết hạn đổi trả
    ↓
Hoa hồng chuyển sang Available
    ↓
Đối soát và chi trả (qua payment)
```

### 3.2 Cửa sổ quy kết

```text
Khách hàng thời trang thường KHÔNG mua ngay.
Họ xem nội dung, cân nhắc, mua sau vài ngày.

Cửa sổ quá ngắn → không phản ánh đúng đóng góp của creator
Cửa sổ quá dài  → quy kết cho creator không thực sự tạo ra đơn

Khuyến nghị khởi điểm: 7 ngày
```

### 3.3 Nhiều creator cùng chuỗi — quyết định quan trọng

```text
Khách click nội dung Creator A (thứ Hai)
Khách click nội dung Creator B (thứ Tư)
Khách mua (thứ Năm)

Ai được tính?
```

**Quyết định:** bắt đầu bằng **last click** — Creator B nhận toàn bộ.

Lý do: đơn giản, creator dễ hiểu, ít tranh chấp về cách tính.

**Nhưng — yêu cầu bắt buộc về dữ liệu:**

```text
LƯU TOÀN BỘ CHUỖI CLICK, không chỉ click cuối.
```

Vì sao: nếu chỉ lưu click được quy kết, sau này muốn đổi sang mô hình chia tỷ lệ sẽ **không có dữ liệu lịch sử để tính lại**. Dữ liệu quá khứ không tạo ngược được.

Đây là ứng dụng trực tiếp của nguyên tắc P14: chính sách đơn giản trước, **dữ liệu đầy đủ ngay từ đầu**.

### 3.4 Click là MỘT LOẠI điểm chạm, không phải loại duy nhất

Mô hình dữ liệu hiện tại ghi `Click`. Về khái niệm, đó là một **Touchpoint**
— một lần khách tiếp xúc với nội dung của creator trước khi mua.

```text
Touchpoint (khái niệm)
    ├── CLICK        ← MVP/Phase 2: bấm vào link affiliate
    ├── VIEW         ← Phase 3: xem nội dung đủ lâu, không bấm
    ├── LIVE_JOIN    ← Phase 4: vào phiên live
    └── SAVE         ← Phase 3: lưu outfit, thêm wishlist từ nội dung
```

**MVP chỉ cài `CLICK`.** Không xây bảng `touchpoint` tổng quát ngay — đó là
trừu tượng hóa sớm (P15), và ba loại kia chưa có nguồn dữ liệu.

**Nhưng phải chuẩn bị sẵn hai thứ:**

```text
1. Bảng click có cột `touchpoint_type` với giá trị mặc định 'CLICK'
   → thêm loại mới không phải migration đau đớn

2. attribution.click_id được hiểu là "điểm chạm được quy kết",
   không phải "cú bấm chuột được quy kết"
   → đổi tên thành touchpoint_id khi có loại thứ hai
```

**Vì sao ghi khái niệm này ra dù chưa cài:** mô hình quy kết chỉ dựa trên
click sẽ bỏ sót phần lớn ảnh hưởng thật của creator. Người xem một video
rồi ba ngày sau tự tìm sản phẩm để mua là trường hợp **phổ biến hơn** người
bấm link mua ngay. Nếu cấu trúc dữ liệu chỉ biết đến click, sau này thêm
`VIEW` sẽ phải viết lại toàn bộ đường quy kết.

---

## 4. Đảo ngược khi hoàn hàng

```text
Đơn bị hoàn sau khi đã ghi nhận hoa hồng
    ↓
Phát affiliate.attribution_reversed
    ↓
payment ghi bút toán đảo ngược
    ↓
Nếu ĐÃ chi trả → khoản phải thu ngược, trừ vào kỳ sau
```

**Cơ chế phòng ngừa:** hoa hồng chỉ chuyển sang `Available` **sau khi hết hạn đổi trả** — giống với số dư seller. Nhờ đó phần lớn trường hợp hoàn hàng xảy ra trước khi tiền được chi.

---

## 5. Ai chịu chi phí hoa hồng creator

Phải được ghi rõ trong `Campaign`, không mặc định:

```text
Sản phẩm own brand           → nền tảng chịu
Chiến dịch do seller khởi xướng → seller chịu
Chiến dịch do nền tảng khởi xướng → nền tảng chịu (khuyến khích seller tham gia)
Thỏa thuận chia sẻ            → chia theo tỷ lệ
```

Thông tin này được đóng băng vào `Attribution` tại thời điểm quy kết.

---

## 6. Chống gian lận

```text
Click phải ghi đủ ngữ cảnh để phát hiện bất thường:

Click {
    affiliate_link_id
    creator_id
    session_id
    customer_id     (nullable)
    ip_hash         ← ĐÃ ẨN DANH HÓA
    device_fingerprint
    referrer
    user_agent_hash
    clicked_at
}
```

**Nguyên tắc:** phát hiện gian lận là **bước riêng, chạy bất đồng bộ**, không nằm trong đường đi chính của việc ghi click — nếu không sẽ làm chậm trải nghiệm khách.

**Lưu ý quyền riêng tư:** IP phải được băm/ẩn danh hóa. Lưu IP dạng gốc là dữ liệu cá nhân, cần cơ sở pháp lý.

---

## 7. Dữ liệu sở hữu

```sql
affiliate_link
click                   -- có thể rất lớn, cần chiến lược phân vùng
attribution             -- BẤT BIẾN sau khi tạo
commission_record
fraud_signal            -- tín hiệu nghi vấn gian lận
```

```sql
CREATE TABLE attribution (
    id                 UUID PRIMARY KEY,
    order_id           UUID NOT NULL,
    order_line_id      UUID NOT NULL,
    creator_id         UUID NOT NULL,
    click_id           UUID NOT NULL,
    campaign_id        UUID,
    attribution_model  TEXT NOT NULL,
    attribution_weight NUMERIC(5,4) NOT NULL,
    commission_rate    INT NOT NULL,      -- basis points, ĐÓNG BĂNG
    commission_base    BIGINT NOT NULL,   -- ĐÓNG BĂNG: giá niêm yết − adjustment
                                          -- do SELLER chịu; xem
                                          -- 01-business/monetization.md mục 3.3
    commission_amount  BIGINT NOT NULL,   -- ĐÓNG BĂNG
    cost_bearer        TEXT NOT NULL,
    status             TEXT NOT NULL,     -- PENDING | CONFIRMED | REVERSED
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_attribution_creator ON attribution (creator_id, created_at DESC);
CREATE INDEX idx_attribution_order ON attribution (order_id);
```

**Về bảng `click`:** đây là bảng ghi nhiều nhất hệ thống. Cần:
- Phân vùng theo thời gian
- Chính sách lưu trữ (ví dụ giữ chi tiết 90 ngày, sau đó chỉ giữ tổng hợp)
- Ghi bất đồng bộ để không làm chậm việc chuyển hướng khách

---

## 8. Interface công khai

```go
type PublicAPI interface {
    CreateAffiliateLink(ctx, req CreateLinkRequest) (*AffiliateLink, error)
    ResolveLink(ctx, shortCode string) (*LinkTarget, error)

    RecordClick(ctx, req RecordClickRequest) error   // bất đồng bộ

    // Quy kết — gọi khi đặt hàng
    ResolveAttribution(ctx, req AttributionRequest) ([]AttributionResult, error)

    GetCreatorEarnings(ctx, creatorID string, period DateRange) (*EarningsSummary, error)
    GetContentPerformance(ctx, contentID string) (*PerformanceView, error)
}
```

---

## 9. Event

**Phát ra:**

| Event | Bên nghe |
|---|---|
| `affiliate.click_recorded` | analytics, supply-chain |
| `affiliate.conversion_attributed` | **payment (ghi hoa hồng)**, creator, analytics |
| `affiliate.attribution_reversed` | **payment (đảo ngược)**, creator |
| `affiliate.fraud_suspected` | notification (cảnh báo admin) |

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `order.placed` | order | Xác nhận quy kết, tạo bản ghi hoa hồng |
| `order.cancelled` | order | Đảo ngược quy kết |
| `return.refunded` | return | Đảo ngược quy kết phần bị hoàn |
| `content.taken_down` | content | Vô hiệu hóa link liên quan |

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Lưu toàn bộ chuỗi click, không chỉ click được quy kết |
| 2 | `Attribution` bất biến sau khi tạo — sửa bằng bản ghi mới |
| 3 | Tỷ lệ hoa hồng đóng băng tại thời điểm quy kết |
| 4 | Hoa hồng chỉ Available sau khi hết hạn đổi trả |
| 5 | Hoàn hàng phải đảo ngược quy kết tương ứng |
| 6 | IP phải ẩn danh hóa |
| 7 | Phát hiện gian lận chạy bất đồng bộ |
| 8 | Ghi click không được làm chậm chuyển hướng khách |

---

## 11. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **Phase 2** | Link, click, quy kết last-click, hoa hồng cơ bản |
| **Phase 3** | Chống gian lận, báo cáo chi tiết cho creator |
| **Phase 4** | Mô hình quy kết đa điểm chạm, tối ưu phân bổ ngân sách |

---

## 12. Tài liệu liên quan

- [../01-business/creator.md](../01-business/creator.md) mục 4 — chính sách quy kết
- [creator.md](creator.md), [content.md](content.md), [campaign.md](campaign.md)
- [../07-workflows/creator-affiliate.md](../07-workflows/creator-affiliate.md)
