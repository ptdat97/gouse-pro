# Luồng: Đăng bán sản phẩm

## 1. Hai đường đi khác nhau

```text
A. Seller bán sản phẩm ĐÃ CÓ trong danh mục
   → chỉ tạo Offer, không tạo Product

B. Seller tạo sản phẩm MỚI
   → tạo Product + Variant + SKU, rồi tạo Offer
```

Đường A được ưu tiên — nó chống trùng lặp danh mục.

---

## 2. Đường A: Tạo offer trên sản phẩm có sẵn

```mermaid
sequenceDiagram
    autonumber
    actor S as Seller
    participant API as Seller API
    participant Prd as product
    participant Cat as catalog
    participant Mkt as marketplace
    participant Inv as inventory
    participant Bus as Event Bus

    S->>API: Tìm sản phẩm theo mã/tên/thương hiệu
    API->>Prd: SearchProducts
    Prd-->>S: danh sách sản phẩm chuẩn

    S->>API: POST /seller/offers (sku_id, giá, tồn kho)
    API->>Mkt: CreateOffer

    Mkt->>Cat: IsBrandProtected(brand_id)
    alt Thương hiệu VERIFIED_ONLY
        Mkt->>Cat: HasValidAuthorization(brand, seller)
        alt Không có ủy quyền
            Mkt-->>S: 403 BRAND_PROTECTED
        end
    end

    Mkt->>Mkt: Kiểm tra: seller ACTIVE?<br/>đã có offer cho SKU này chưa?
    Mkt->>Mkt: Kiểm tra giá trong khung cho phép
    Mkt->>Mkt: Tạo Offer (status = ACTIVE)
    Mkt->>Bus: offer.created

    S->>API: PATCH /seller/inventory/{sku_id}
    API->>Inv: đặt số lượng khả dụng
    Inv->>Bus: inventory.received
```

---

## 3. Ba lớp kiểm soát khi tạo offer

### 3.1 Bảo vệ thương hiệu — chống hàng giả

```text
brand.protection_level:

OPEN           → seller nào cũng tạo offer được
VERIFIED_ONLY  → phải có brand_authorization còn hiệu lực
RESTRICTED     → chỉ seller được chỉ định
```

Đây là **quy tắc domain bắt buộc**, kiểm tra trong code, không phải quy trình thủ công bên ngoài.

Hàng giả là rủi ro sống còn của marketplace thời trang: kiện tụng, mất quyền phân phối thương hiệu chính hãng, mất niềm tin khách hàng.

### 3.2 Khung giá — chống hai rủi ro

```text
Giá dưới mức tối thiểu:
    → có thể là bán phá giá
    → hoặc lỗi nhập liệu (thiếu số 0: nhập 50.000 thay vì 500.000)

Giá trên mức tối đa:
    → thổi giá, lừa khách
```

Lỗi nhập liệu phổ biến hơn nhiều so với gian lận — khung giá bảo vệ chính seller.

### 3.3 Một seller một offer active cho một SKU

Thực thi bằng chỉ mục duy nhất có điều kiện ở tầng database:

```sql
CREATE UNIQUE INDEX idx_offer_unique_active
    ON offer (sku_id, seller_id) WHERE status = 'ACTIVE';
```

---

## 4. Đường B: Tạo sản phẩm mới

```mermaid
sequenceDiagram
    autonumber
    actor S as Seller
    participant API as Seller API
    participant Prd as product
    participant Cat as catalog
    actor Adm as Nhân viên duyệt
    participant Bus as Event Bus

    S->>API: POST /seller/products (thông tin đầy đủ)
    API->>Prd: CreateProduct

    Prd->>Prd: ĐỐI SÁNH TRÙNG LẶP
    alt Tìm thấy sản phẩm tương tự
        Prd-->>S: gợi ý sản phẩm có sẵn<br/>"Có phải bạn muốn bán sản phẩm này?"
        Note over S: Chọn dùng sản phẩm có sẵn<br/>→ chuyển sang đường A
    end

    Prd->>Prd: Kiểm tra tự động:<br/>đủ ảnh · đủ mô tả · có bảng size ·<br/>chất liệu · không từ ngữ cấm
    Prd->>Prd: status = PENDING_REVIEW

    alt Thương hiệu bảo vệ hoặc seller mới
        Prd->>Adm: chờ duyệt thủ công
        Adm->>Prd: ApproveProduct hoặc Reject(lý do)
    else Seller uy tín, thương hiệu OPEN
        Prd->>Prd: tự động duyệt
    end

    Prd->>Prd: status = ACTIVE
    Prd->>Bus: product.published
    Bus->>Prd: đưa vào chỉ mục tìm kiếm
```

---

## 5. Yêu cầu bắt buộc cho sản phẩm thời trang

```text
Bắt buộc — CheckReadyForReview cưỡng chế, gửi duyệt sẽ bị từ chối nếu thiếu:
    ✓ Ít nhất MỘT ảnh
    ✓ Mô tả
    ✓ Chất liệu (material_composition)
    ✓ Bảng size (size_chart_id) — chỉ với loại sản phẩm CẦN bảng size;
      túi và phụ kiện thì không
    ✓ Ít nhất một variant với ít nhất một SKU

Khuyến khích mạnh — KHÔNG chặn gửi duyệt:
    - Ba ảnh trở lên: mặt trước, mặt sau, chi tiết
    - Hướng dẫn bảo quản
    - Xuất xứ
    - Ảnh trên người mẫu có nêu số đo
    - Ảnh chi tiết chất liệu
```

**Bản trước của mục này đòi ≥3 ảnh, hướng dẫn bảo quản và xuất xứ ở nhóm
BẮT BUỘC. Mã chưa bao giờ cưỡng chế ba thứ đó** — `CheckReadyForReview`
kiểm một ảnh và không nhắc tới hai trường kia. Nới tài liệu theo mã
(23/09/2026), không siết mã theo tài liệu.

Lý do: con số 3 chưa ai đo xem có đúng không, và một tài liệu nói "bắt
buộc" về thứ không ai cưỡng chế sẽ dẫn người đọc sau viết mã theo nó rồi
phát hiện hệ thống nói khác. Khi có số liệu hoàn hàng thật, siết lại là một
quyết định có bằng chứng — xem mục "Điểm cần giám sát".

**Vì sao những thứ này bắt buộc:** chúng ảnh hưởng **trực tiếp** tới tỷ lệ hoàn hàng — vấn đề kinh tế lớn nhất của thương mại thời trang.

```text
Thiếu bảng size    → khách chọn sai size → hoàn hàng
Thiếu chất liệu    → khách mua nhầm → hoàn hàng
Ảnh không đủ       → khác kỳ vọng → hoàn hàng
```

---

## 6. Chống trùng lặp danh mục

```text
Vấn đề: nhiều seller đăng cùng một hàng nhưng tạo product riêng
    → khách tìm "áo thun Uniqlo U" thấy 40 kết quả giống hệt
    → không so sánh được giá
    → trải nghiệm tệ, SEO tệ

Ba cơ chế:
    1. Đối sánh khi đăng bán (mã sản phẩm, tên, thương hiệu)
    2. Danh mục chuẩn hóa — nền tảng tạo sẵn product cho thương hiệu lớn
    3. Quy trình gộp — admin gộp product trùng
```

### Quy trình gộp

```mermaid
sequenceDiagram
    actor Adm as Nhân viên
    participant Prd as product
    participant Mkt as marketplace
    participant Bus as Event Bus

    Adm->>Prd: MergeProducts(nguồn, đích)
    Prd->>Mkt: chuyển offer sang product đích
    Prd->>Prd: product nguồn → MERGED (KHÔNG XÓA)
    Prd->>Prd: ghi product_merge_log
    Prd->>Bus: product.merged
    Note over Prd: Đơn hàng cũ vẫn trỏ đúng<br/>Đánh giá được chuyển theo<br/>URL cũ chuyển hướng
```

**Không xóa product bị gộp** — đơn hàng cũ tham chiếu tới nó, và URL cũ cần chuyển hướng để giữ SEO.

---

## 7. Sản phẩm own brand — đường đi thứ ba

Sản phẩm own brand **không** đi qua hai đường trên. Nó đến từ Supply Chain qua Anti-Corruption Layer:

```mermaid
sequenceDiagram
    participant SC as supply-chain
    participant Bus as Event Bus
    participant Prd as product
    participant Wh as warehouse
    participant Inv as inventory

    SC->>SC: ProductDevelopment: mẫu được duyệt
    SC->>Bus: product_development.approved

    Bus->>Prd: CreateProductFromDevelopment (ACL)
    Note over Prd: ACL chỉ lấy: tên, loại, thuộc tính, SKU<br/>BỎ QUA: tech pack, giá vốn, nhà cung cấp
    Prd->>Prd: tạo Product (status = DRAFT)

    Note over SC: ...sản xuất, QC, vận chuyển...

    Wh->>Bus: warehouse.goods_received
    Bus->>Inv: tăng tồn kho
    Bus->>Prd: có thể publish
    Prd->>Prd: status = ACTIVE
```

**Vì sao cần ACL:** Catalog **không được biết** về tech pack, giá vốn, nhà cung cấp. Nếu những khái niệm này rò rỉ sang, mô hình catalog bị ô nhiễm bởi khái niệm sản xuất.

Xem [../02-domain/bounded-contexts.md](../02-domain/bounded-contexts.md) mục 5.

---

## 8. Điểm cần giám sát

| Chỉ báo | Ngưỡng |
|---|---|
| Thời gian duyệt sản phẩm | < 24 giờ |
| Tỷ lệ sản phẩm bị từ chối | Theo dõi theo seller |
| Tỷ lệ trùng lặp phát hiện được | Theo dõi xu hướng |
| Offer bị chặn do thương hiệu bảo vệ | Theo dõi (dấu hiệu hàng giả) |
| Sản phẩm thiếu bảng size | 0 với loại CẦN bảng size (bắt buộc) |
| Tỷ lệ hoàn hàng theo SỐ ẢNH của sản phẩm | chưa đo — đây là số liệu quyết định có siết ≥3 ảnh hay không |

---

## 9. Tài liệu liên quan

- [../04-modules/product.md](../04-modules/product.md), [../04-modules/marketplace.md](../04-modules/marketplace.md)
- [../01-business/marketplace.md](../01-business/marketplace.md) mục 4
- [own-brand-product.md](own-brand-product.md)
