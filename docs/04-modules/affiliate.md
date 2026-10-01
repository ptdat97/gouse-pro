# Module: Affiliate

| | |
|---|---|
| **Bounded Context** | Growth |
| **Phân loại** | **Core** |
| **Tầng (microkernel)** | **EXTENSION — đã dựng** |
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

> Khối trên là danh sách SỞ HỮU, và `cmd/archcheck` quy tắc R6 đọc chính nó
> để chặn module khác chạm vào năm bảng này. Nó khai cả bảng chưa dựng: sở
> hữu là lời tuyên bố về ranh giới, không phải ảnh chụp lược đồ.

### 7.1. ĐÃ DỰNG: hai bảng (migration 000058, 27/09/2026)

`attribution` và `commission_record`. Ba bảng còn lại chưa dựng:

| Bảng | Vì sao chưa |
|---|---|
| `affiliate_link` | cần HTTP cho creator tạo link, tức module `creator` (Phase 2). Và quy công KHÔNG cần link: chuỗi `source.creator_id` → cart → checkout → order_line đủ để quy công ngay hôm nay. Link là cách theo dõi click qua URL chuyển hướng — một tối ưu, không phải điều kiện. |
| `click` | cùng lý do: không có link thì không có click. |
| `fraud_signal` | chưa có phép phát hiện gian lận nào. Một bảng không ai ghi là dạng lỗi hay gặp nhất của dự án này. |

### 7.2. Lược đồ THẬT khác lược đồ dưới đây

Khối `CREATE TABLE` bên dưới là bản THIẾT KẾ. Bản đang chạy ở
`migrations/000058_affiliate.up.sql` khác nó ở sáu điểm, mỗi điểm có lý do
ghi tại chỗ trong migration:

| Thiết kế | Đang chạy | Vì sao |
|---|---|---|
| `id UUID` | `id TEXT` có tiền tố `atr_`/`crc_` | quy ước định danh của cả hệ thống (`internal/kernel/ids`) |
| `click_id NOT NULL` | không có | không có bảng `click` thì không có mã click. Bắt buộc một cột không nguồn nghĩa là chèn giá trị giả |
| `campaign_id` | không có | module `campaign` là Phase 2 |
| `attribution_weight NUMERIC` | không có | `LAST_CLICK` cho trọng số luôn bằng 1. Một cột luôn bằng 1 là chỗ để tin sai |
| `commission_amount` trên `attribution` | không có; TÍNH từ `commission_rate × commission_base` | hai nguồn cho cùng một con số tiền sớm muộn sẽ lệch. Cả hai đầu vào đều đóng băng nên phép tính cho cùng kết quả mãi mãi |
| `status` trên `attribution` | nằm ở `commission_record` | `attribution` BẤT BIẾN (quy tắc 2); một cột trạng thái trên bảng bất biến là tự phủ định |

Và MỘT cột thiết kế không có: `seller_id` trên `attribution`. Tín hiệu hết
hạn đổi trả là `fulfillment_order.completed`, mang `(order_id, seller_id)` —
không mang dòng hàng. Không có cột ấy, một đơn nhiều nhà bán sẽ mở khóa hoa
hồng của CẢ đơn khi nhà bán đầu tiên giao xong.

`content_id` ĐÃ ĐẦY từ 01/10/2026. Chuỗi nội dung đi hết sau migration
000060, thêm `order_line.attributed_content_id`:

```text
cart_item.source_content_id       migration 000009
checkout_line.source_content_id   migration 000057
order_line.attributed_content_id  migration 000060
attribution.content_id            ✓
```

Trước đó nó luôn rỗng vì `order_line` không có chỗ chở mã nội dung, nên câu
*"video nào bán được hàng"* không trả lời được (P3-84).

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

### 8.1. ĐANG CHẠY — cố ý nhỏ

```go
// internal/modules/affiliate/public.go
type API interface {
    SoDuCreator(ctx, creatorID string) (map[string]int64, error)
}
```

MỘT hàm, và nó chỉ ĐỌC. Việc ghi xảy ra qua event, nên không có đường nào để
một module khác tạo ra một khoản hoa hồng — quy công chỉ sinh ra từ một đơn
đã thanh toán.

Không module nào gọi mặt này. Nó tồn tại vì bài test tích hợp cần đọc số dư
để khẳng định TIỀN, và một mặt công khai có bên gọi thật là mặt duy nhất
chứng minh được là đúng.

`TheoCreator` (lịch sử hoa hồng của một creator) đã bị BỎ khỏi cổng kho lưu
trữ: nó cần một màn hình cho creator, mà module `creator` là Phase 2. Bày ra
một hàm chưa ai gọi là tạo trước thứ phải bảo trì.

### 8.2. Bản thiết kế — chưa cài

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

Bốn hàm đầu cần bảng `affiliate_link` và `click`. `ResolveAttribution` thì
KHÔNG còn cần: việc chọn creator đã xảy ra TRƯỚC khi tới module này —
`cart.Item.setSource` giữ nguồn của lần thêm gần nhất (cart.md mục 6), và
chuỗi giỏ→phiên→đơn chở đúng một creator. "Last click" ở đây là mô tả thứ đã
xảy ra, không phải phép tính module này chạy.

---

## 9. Event

### 9.1. Đang chạy (01/10/2026)

**Lắng nghe — bốn bên nhận, đăng ký ở `cmd/worker`:**

| Event | Từ | Hành động |
|---|---|---|
| `order.paid` | order | Quy công mọi dòng có creator, tạo khoản hoa hồng `PENDING` |
| `order.cancelled` | order | Đảo mọi khoản hoa hồng của đơn, kèm lý do |
| `fulfillment.cancelled` v2 | fulfillment | Đảo hoa hồng của ĐÚNG những dòng nhà bán không giao nữa |
| `fulfillment_order.completed` | fulfillment | `PENDING` → `AVAILABLE` cho đúng `(đơn, nhà bán)` |

Bốn bên nhận, không ba: `order.cancelled` chỉ phát khi hủy CẢ đơn. Hủy TỪNG
PHẦN — một nhà bán không giao được — chỉ phát `fulfillment.cancelled`, và
trước 01/10/2026 khoản hoa hồng của dòng ấy đứng ở `PENDING` **mãi mãi**:
không đảo, và cũng không mở khóa, vì đơn thực hiện bị hủy không bao giờ phát
`fulfillment_order.completed`. Xem P3-85.

Đảo theo DÒNG, không theo đơn: nhà bán còn lại vẫn đang giao bình thường.

**Phát ra:** KHÔNG event nào.

Bảng cũ của tài liệu này khai bốn event `affiliate.*` và không cái nào tồn
tại. Chưa khai chúng vào `internal/platform/eventbus` là có chủ ý: một hằng
số event không ai phát là dạng lỗi đã xảy ra bốn lần ở dự án này (P3-76), và
`cmd/eventcheck` nay gác nó. Khai khi có bên nghe thật.

### 9.2. Vì sao `order.paid` chứ không `order.placed`

`order.placed` **không tồn tại**. Nó từng được khai ở `eventbus` và không
chỗ nào phát, không ai nghe, suốt nhiều tháng; xóa 25/09/2026 (P3-76). Tài
liệu này khai nó tới 27/09/2026 — một bên nhận viết theo bảng cũ sẽ là mã
không bao giờ chạy.

Và ngay cả khi nó tồn tại, nó vẫn sai mốc: quy công lúc ĐẶT đơn sinh ra một
khoản hoa hồng cho doanh thu chưa tồn tại. `order.paid` là mốc duy nhất nói
"tiền của đơn này đã về" (ADR-0018).

`return.refunded` cũng không tồn tại và chưa bao giờ tồn tại. Đảo theo hoàn
hàng đi qua `order.cancelled` khi cả đơn bị hủy; hoàn TỪNG PHẦN chưa có
đường tới đây — xem P3-85.

### 9.3. `fulfillment.cancelled` lên v2 vì module này

`order_line_id` thêm vào payload ngày 01/10/2026 — affiliate quy công theo
dòng đơn, và một đơn có thể có hai dòng CÙNG SKU với hai creator khác nhau,
nên `(sku_id, quantity)` của v1 không đủ để đảo đúng người.

Cái giá, đo thật: `inventory.release_on_fulfillment_cancelled` không dùng
trường mới — nó trả hàng về kho theo `(sku_id, quantity)` — nhưng vẫn phải
khai `MaxEventVersion` ≥ 2, nếu không dispatcher HOÃN event cho TẤT CẢ và
**hàng không được trả về kho** vì một trường module ấy không đọc (ADR-0016).

Đó là lý do mục 9.4 dưới đây không nhồi dữ liệu vào `order.paid`.

### 9.4. `order.paid` không mang dòng hàng

Payload của nó có `order_id`, `order_number`, `customer_id`,
`payment_method`, `amount`, `currency`, `paid_at` — không có dòng hàng, mà
hoa hồng tính theo TỪNG DÒNG.

Nên module này đọc dòng hàng qua **cổng công khai của order**
(`GetOrder` → `OrderView.Lines`), chiều extension → kernel mà ADR-0021 cho
phép. KHÔNG nhồi dòng hàng vào payload: theo ADR-0016, thêm trường vào một
event đang chạy bắt MỌI bên nhận của nó khai lại `MaxEventVersion` — tức sửa
payment và fulfillment cho một trường chỉ affiliate dùng.

### 9.5. Dự kiến (chưa cài)

| Event | Bên nghe | Cần gì trước |
|---|---|---|
| `affiliate.click_recorded` | analytics, supply-chain | bảng `click`, tức module `creator` |
| `affiliate.conversion_attributed` | payment, creator, analytics | một bên nghe thật |
| `affiliate.attribution_reversed` | payment, creator | một bên nghe thật |
| `affiliate.fraud_suspected` | notification | phép phát hiện gian lận |
| `content.taken_down` | (module `content`, Phase 2) | module `content` |

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
