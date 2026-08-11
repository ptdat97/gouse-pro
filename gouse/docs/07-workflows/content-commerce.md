# Luồng: Thương mại qua nội dung

## 1. Tổng quan

```text
Creator tạo nội dung → Khách xem → Click → Mua → Quy kết → Hoa hồng
```

Đây là luồng tạo ra khác biệt của nền tảng so với ecommerce thông thường.

---

## 2. Sequence diagram — từ nội dung tới đơn hàng

```mermaid
sequenceDiagram
    autonumber
    actor CR as Creator
    actor KH as Khách hàng
    participant FE as Next.js
    participant Cnt as content
    participant Aff as affiliate
    participant Cart as cart
    participant Ord as order
    participant Pay as payment
    participant Bus as Event Bus

    CR->>Cnt: Tạo Outfit, gắn 4 sản phẩm
    Cnt->>Cnt: Kiểm duyệt (tự động + thủ công)
    Note over Cnt: is_sponsored tự động = true<br/>nếu thuộc campaign có trả phí
    Cnt->>Bus: content.published
    CR->>Aff: Tạo affiliate link
    Aff-->>CR: https://.../r/mA7xK2

    KH->>FE: Xem nội dung qua affiliate link
    FE->>Aff: RecordClick (bất đồng bộ)
    Note over Aff: Ghi Click: session, thiết bị,<br/>ip_hash, thời điểm
    Aff->>Bus: affiliate.click_recorded
    Bus->>Cnt: content.viewed → tín hiệu nhu cầu

    KH->>FE: Bấm sản phẩm trong outfit
    FE->>Cart: POST /cart/items<br/>source={content_id, creator_id}
    Note over Cart: Ghi nguồn NGAY ở bước thêm giỏ,<br/>không chờ tới lúc mua

    Note over KH: Khách rời đi, quay lại sau 3 ngày
    KH->>FE: Hoàn tất mua hàng
    FE->>Ord: PlaceOrder
    Ord->>Aff: ResolveAttribution(session, offer)
    Aff->>Aff: Tìm click trong CỬA SỔ QUY KẾT (7 ngày)
    Aff-->>Ord: creator_id + tỷ lệ hoa hồng
    Note over Ord: ĐÓNG BĂNG creator_id và<br/>tỷ lệ vào OrderLine

    Ord->>Bus: order.placed
    Bus->>Aff: tạo Attribution (BẤT BIẾN)
    Aff->>Bus: affiliate.conversion_attributed
    Bus->>Pay: ghi CREATOR_PAYABLE (Pending)
```

---

## 3. Ba quyết định quan trọng trong luồng

### 3.1 Ghi nguồn ở bước thêm giỏ (bước 12)

```text
Vì sao không chỉ ghi ở lúc mua?

    - Đo được tỷ lệ "thêm giỏ" của từng nội dung,
      không chỉ tỷ lệ mua
    - Phân tích nội dung nào tạo ý định nhưng không chốt được
    - Quy kết chính xác hơn khi khách mua sau vài ngày
```

### 3.2 Cửa sổ quy kết 7 ngày (bước 16)

```text
Khách hàng thời trang KHÔNG mua ngay.
Họ xem nội dung, cân nhắc, so sánh, mua sau vài ngày.

Cửa sổ quá ngắn → không phản ánh đúng đóng góp creator
Cửa sổ quá dài  → quy kết cho creator không thực sự tạo đơn
```

### 3.3 Lưu toàn bộ chuỗi click, không chỉ click cuối

```text
Mô hình quy kết ban đầu: LAST CLICK (đơn giản, dễ giải thích)

NHƯNG: lưu TẤT CẢ click, không chỉ click được quy kết.

Vì sao: nếu sau này muốn đổi sang mô hình chia tỷ lệ,
        phải có dữ liệu lịch sử để tính lại.
        Dữ liệu quá khứ KHÔNG TẠO NGƯỢC ĐƯỢC.
```

Đây là ứng dụng nguyên tắc P14: chính sách đơn giản trước, dữ liệu đầy đủ ngay từ đầu.

---

## 4. Nhiều creator cùng chuỗi

```mermaid
sequenceDiagram
    actor KH as Khách hàng
    participant Aff as affiliate

    Note over KH: Thứ Hai
    KH->>Aff: Click nội dung Creator A
    Aff->>Aff: Lưu Click #1 (creator A)

    Note over KH: Thứ Tư
    KH->>Aff: Click nội dung Creator B
    Aff->>Aff: Lưu Click #2 (creator B)

    Note over KH: Thứ Năm — mua hàng
    Aff->>Aff: ResolveAttribution
    Note over Aff: LAST CLICK → Creator B nhận 100%
    Note over Aff: NHƯNG cả hai click đều được lưu<br/>→ có thể tính lại theo mô hình khác sau này
```

---

## 5. Xử lý sản phẩm hết hàng trong nội dung

Nội dung sống lâu hơn sản phẩm. Video hay được xem nhiều tháng, khi sản phẩm gốc đã hết.

```mermaid
sequenceDiagram
    participant Inv as inventory
    participant Bus as Event Bus
    participant Cnt as content
    actor KH as Khách hàng
    participant FE as Next.js

    Inv->>Bus: inventory.depleted (sku_X)
    Bus->>Cnt: đánh dấu product tag hết hàng

    KH->>FE: Xem outfit cũ
    FE->>Cnt: GET /content/{id}
    Cnt-->>FE: sản phẩm 2: available=false<br/>+ substitutes: [prd_Y, prd_Z]
    Note over FE: Hiển thị "Tạm hết hàng"<br/>+ nút nhận thông báo<br/>+ gợi ý sản phẩm tương tự
```

**Nguyên tắc:** không được để nội dung dẫn tới trang lỗi. Đó là lãng phí toàn bộ công sức tạo nội dung và tổn hại trải nghiệm.

Đồng thời, sự kiện này là **tín hiệu nhu cầu** — khách muốn mua nhưng không có hàng.

---

## 6. Đảo ngược khi hoàn hàng

```mermaid
sequenceDiagram
    actor KH as Khách hàng
    participant Ret as return
    participant Bus as Event Bus
    participant Aff as affiliate
    participant Pay as payment

    KH->>Ret: Trả hàng (SIZE_TOO_SMALL)
    Ret->>Ret: Duyệt, nhận hàng, kiểm định
    Ret->>Bus: return.refunded

    Bus->>Aff: đảo ngược quy kết
    Aff->>Aff: Attribution → REVERSED
    Note over Aff: KHÔNG xóa bản ghi cũ —<br/>tạo bản ghi đảo ngược
    Aff->>Bus: affiliate.attribution_reversed

    Bus->>Pay: ghi bút toán đảo ngược
    Note over Pay: Nếu ĐÃ chi trả creator →<br/>khoản phải thu ngược, trừ kỳ sau
```

**Cơ chế bảo vệ:** hoa hồng creator chỉ chuyển sang `Available` **sau khi hết hạn đổi trả** — giống số dư seller. Nhờ đó phần lớn trường hợp hoàn hàng xảy ra trước khi tiền được chi.

---

## 7. Vòng phản hồi về chuỗi cung ứng

Đây là mắt xích hoàn thành bánh đà.

```mermaid
flowchart TD
    A[Nội dung có tương tác cao] --> B[Sản phẩm nào được tag nhiều?<br/>Outfit nào được lưu nhiều?<br/>Nội dung nào chuyển đổi cao?]
    B --> C[content.viewed<br/>cart.item_added<br/>inventory.depleted<br/>wishlist.item_added]
    C --> D[supply-chain:<br/>DemandSignal]
    D --> E[Tổng hợp theo SKU/tuần]
    E --> F[Dự báo nhu cầu]
    F --> G[Kế hoạch sản xuất own brand]
    G --> H[Sản phẩm mới đúng nhu cầu]
    H --> A
```

**Ví dụ cụ thể:**

```text
Video thử áo khoác dạ oversize:
    50.000 lượt xem
    3.000 lượt click
    850 lượt thêm wishlist
    Sản phẩm chỉ còn size S

Tín hiệu rõ ràng:
    → nhu cầu tồn tại và lớn
    → nguồn cung không đủ
    → cần sản xuất thêm, phân bổ size đúng (nhiều M, L)
```

Nếu dữ liệu này nằm trong công cụ analytics bên thứ ba mà supply chain không truy vấn được, **bánh đà bị đứt**.

---

## 8. Chống gian lận

```text
Kiểu gian lận:
    Click ảo          → phân tích IP, thiết bị, tần suất
    Tự mua rồi hoàn   → đối chiếu định danh, tỷ lệ hoàn bất thường
    Cookie stuffing   → tỷ lệ click/hiển thị bất thường
    Nội dung sai lệch → tỷ lệ hoàn cao trên nội dung cụ thể
```

**Nguyên tắc:** phát hiện gian lận chạy **bất đồng bộ**, không nằm trong đường đi chính của việc ghi click — nếu không sẽ làm chậm trải nghiệm khách.

---

## 9. Điểm cần giám sát

| Chỉ báo | Ý nghĩa |
|---|---|
| Content-to-click rate | Nội dung có hấp dẫn không |
| Click-to-purchase rate | Nội dung có dẫn tới mua không |
| Return rate theo nội dung | Nội dung có gây hiểu nhầm không |
| Tỷ lệ quy kết bị đảo ngược | Chất lượng đơn từ creator |
| Độ trễ ghi click | < 50ms (không làm chậm chuyển hướng) |
| Tín hiệu gian lận | Theo dõi danh sách |

---

## 10. Tài liệu liên quan

- [creator-affiliate.md](creator-affiliate.md) — chi tiết quy kết
- [../01-business/content-commerce.md](../01-business/content-commerce.md)
- [../04-modules/content.md](../04-modules/content.md), [../04-modules/affiliate.md](../04-modules/affiliate.md)
