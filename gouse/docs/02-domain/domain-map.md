# Bản đồ miền (Domain Map)

## 1. Mục đích

Tài liệu này phân loại các miền nghiệp vụ thành ba nhóm — **Core**, **Supporting**, **Generic** — và giải thích vì sao mỗi miền thuộc nhóm đó.

Việc phân loại này **quyết định phân bổ nguồn lực**:

| Nhóm | Nghĩa | Cách đối xử |
|---|---|---|
| **Core Domain** | Nơi tạo ra lợi thế cạnh tranh | Đầu tư mạnh nhất, người giỏi nhất, mô hình domain đầy đủ, tự xây |
| **Supporting Domain** | Cần thiết cho nghiệp vụ nhưng không tạo khác biệt | Xây vừa đủ, mô hình đơn giản, có thể thuê ngoài một phần |
| **Generic Domain** | Ai cũng cần, không có gì đặc biệt | Mua/dùng dịch vụ có sẵn, không tự xây trừ khi bắt buộc |

**Sai lầm phổ biến:** đầu tư nhiều công sức vào generic domain (tự xây hệ thống gửi email, tự xây tìm kiếm từ đầu) và làm qua loa core domain. Kết quả: hệ thống chạy được nhưng không có gì khác biệt.

---

## 2. Bản đồ tổng thể

```text
┌─────────────────────────────────────────────────────────────────┐
│                        CORE DOMAIN                              │
│           (Lợi thế cạnh tranh — đầu tư mạnh nhất)               │
│                                                                 │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│   │  Marketplace │  │Creator/      │  │ Supply Chain │          │
│   │  & Offer     │  │Content       │  │ & Demand     │          │
│   │              │  │Commerce      │  │ Intelligence │          │
│   └──────────────┘  └──────────────┘  └──────────────┘          │
│                                                                 │
│   ┌──────────────┐  ┌──────────────┐                            │
│   │ Order &      │  │ Fashion      │                            │
│   │ Fulfillment  │  │ Product      │                            │
│   │ Orchestration│  │ Model        │                            │
│   └──────────────┘  └──────────────┘                            │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                     SUPPORTING DOMAIN                           │
│              (Cần thiết, không tạo khác biệt)                   │
│                                                                 │
│   Cart      Checkout    Pricing     Promotion    Inventory      │
│   Return    Warehouse   Procurement Quality      Loyalty        │
│   Campaign  Seller Mgmt Customer    Review                      │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                      GENERIC DOMAIN                             │
│                  (Dùng giải pháp có sẵn)                        │
│                                                                 │
│   Identity   Notification   Search Infra   File Storage         │
│   Payment    Shipping       Audit Log      Analytics Infra      │
│   Gateway    Integration                                        │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2.1. Ràng buộc bắt buộc: ai được quyết định hình dạng của domain

Phân loại ở trên không chỉ để đọc. Nó quyết định **ai được phép định hình
mô hình dữ liệu và ranh giới** của từng miền.

### Domain chiến lược — chúng ta kiểm soát, không phải OSS

Mười bốn miền dưới đây là lợi thế cạnh tranh. **Không OSS, framework hay
nhà cung cấp nào được quyết định hình dạng của chúng.**

```text
Marketplace Offer          Demand Signal
Seller                     Demand Planning
Creator                    Fashion Product Intelligence
Content Commerce           Supplier Network
Attribution                Procurement
                           Manufacturing
                           Quality
                           Replenishment
                           Own Brand Operations
```

**Lưu ý về phân loại hai tầng:** một số miền trong danh sách này (`Seller`,
`Procurement`, `Quality`) được xếp **Supporting** ở mục 2 theo tiêu chí
"mức đầu tư", nhưng vẫn là **chiến lược** theo tiêu chí "ai định hình mô
hình". Hai tiêu chí khác nhau:

```text
Core/Supporting/Generic  →  đầu tư BAO NHIÊU công sức
Chiến lược/Hạ tầng       →  AI được quyết định hình dạng
```

`Seller` là ví dụ rõ nhất: quản lý hồ sơ nhà bán là nghiệp vụ chuẩn
(Supporting, làm vừa đủ), nhưng ranh giới "seller không bao giờ thấy dữ
liệu seller khác" là ràng buộc do **chúng ta** đặt, không phải do một
framework marketplace nào đó đặt hộ.

**Được và không được:**

| Được | Không được |
|---|---|
| Đọc OSS để hiểu vấn đề | Dùng model dữ liệu của OSS làm model domain |
| Lấy thuật toán rời rạc rồi tự cài | Để framework quyết định aggregate/ranh giới |
| Học cách đặt tên, cách chia trạng thái | Ép nghiệp vụ theo hình dạng công cụ |

### Hạ tầng chung — nên tận dụng OSS

```text
Adapter thanh toán     Jobs / hàng đợi
Tìm kiếm               Caching
Media / xử lý ảnh      Observability
Lưu trữ file           Hạ tầng trang quản trị
Gửi email              Driver database, migration
```

Nhóm này dùng OSS thoải mái, nhưng **đặt sau interface do domain định
nghĩa** (`WRAP`) nếu sẽ đổi nhà cung cấp — nguyên tắc P13.

### Vì sao ràng buộc này đáng ghi thành quy tắc

Nhầm lẫn theo hai chiều đều tốn kém, nhưng không tốn như nhau:

```text
Nhầm chiến lược thành hạ tầng
    → dùng framework cho Offer/Attribution
    → sửa = VIẾT LẠI DOMAIN

Nhầm hạ tầng thành chiến lược
    → tự xây email, hàng đợi, tìm kiếm
    → sửa = vứt code đi, dùng OSS
```

Chiều thứ nhất tệ hơn nhiều: nó làm hỏng đúng thứ tạo ra giá trị.

Chi tiết cách phân loại và ví dụ đã áp dụng:
[../11-oss/adoption-policy.md](../11-oss/adoption-policy.md) mục 1.1.

---

## 3. Core Domain — chi tiết và lý do

### 3.1 Marketplace & Offer Model

**Vì sao là core:** khả năng để nhiều nhà bán cùng bán một sản phẩm, cạnh tranh về giá và dịch vụ, trong khi khách chỉ thấy một trang sản phẩm sạch sẽ — đây là năng lực trung tâm phân biệt nền tảng với cửa hàng online.

Bao gồm: mô hình Offer, buy box, kiểm soát chất lượng danh mục, hoa hồng, đối soát seller.

**Nếu làm sai:** trang sản phẩm trùng lặp, khách không so sánh được, seller không tin vào cơ chế phân phối đơn hàng.

### 3.2 Creator & Content Commerce

**Vì sao là core:** đây là động cơ tạo nhu cầu với chi phí thấp hơn quảng cáo. Cơ chế quy kết (attribution) chính xác và công bằng là điều kiện để creator tin tưởng và gắn bó.

Bao gồm: mô hình nội dung (đặc biệt là Outfit), gắn sản phẩm, attribution, hoa hồng creator, chiến dịch.

**Nếu làm sai:** creator không tin số liệu, rời nền tảng. Mất luôn kênh tạo nhu cầu rẻ nhất.

### 3.3 Supply Chain & Demand Intelligence

**Vì sao là core:** đây là lợi thế dài hạn khó sao chép nhất. Năng lực chuyển tín hiệu nhu cầu thành hàng hóa đúng số lượng, đúng thời điểm.

Bao gồm: tín hiệu nhu cầu, dự báo, lập kế hoạch sản phẩm, đơn sản xuất, lô sản xuất, bổ sung hàng.

**Nếu làm sai:** sản xuất thừa hàng ế và thiếu hàng bán chạy cùng lúc — tình trạng phổ biến nhất của thương hiệu thời trang.

**Lưu ý phân loại:** `Procurement`, `Quality`, `Warehouse` được xếp vào Supporting, dù chúng thuộc chuỗi cung ứng. Lý do: phần **thông minh** (biết sản xuất gì, bao nhiêu) là core; phần **thực thi** (gửi đơn mua, kiểm hàng, quản lý kệ) là nghiệp vụ chuẩn mà mọi công ty đều làm giống nhau.

### 3.4 Order & Fulfillment Orchestration

**Vì sao là core:** điều phối một đơn hàng có hàng từ own brand, nhiều seller, nhiều kho, với khả năng giao/hủy/hoàn từng phần — đây là bài toán khó và làm sai sẽ hỏng cả trải nghiệm khách lẫn niềm tin seller.

Bao gồm: tách Order/FulfillmentOrder, phân bổ nguồn hàng, xử lý từng phần.

**Nếu làm sai:** không xử lý được đơn nhiều nhà bán, phải hủy cả đơn khi một seller hết hàng.

### 3.5 Fashion Product Model

**Vì sao là core:** mô hình sản phẩm thời trang có yêu cầu riêng — bộ sưu tập, mùa vụ, phân bổ size, biến thể màu, bảng size theo thương hiệu, vòng đời từ concept tới bán.

**Nếu làm sai:** không quản lý được mùa vụ, không phân tích được theo size, không kết nối được với sản xuất.

**Đây là chỗ mà nền tảng ecommerce tổng quát thất bại** — mô hình sản phẩm của chúng quá đơn giản cho thời trang.

---

## 4. Supporting Domain — chi tiết và lý do

| Miền | Vì sao supporting | Ghi chú |
|---|---|---|
| **Cart** | Giỏ hàng là nghiệp vụ chuẩn, ai cũng làm giống nhau | Nhưng phải hỗ trợ nhiều seller |
| **Checkout** | Luồng chuẩn | Phức tạp ở chỗ chia theo seller |
| **Pricing** | Định giá cơ bản là chuẩn | Sẽ thành core nếu làm định giá động |
| **Promotion** | Khuyến mãi là nghiệp vụ chuẩn | Cần hỗ trợ ai chịu chi phí |
| **Inventory** | Quản lý tồn kho là chuẩn | Trạng thái và tranh chấp cần làm kỹ |
| **Return** | Quy trình chuẩn | Nhưng **rất quan trọng** với thời trang |
| **Warehouse** | Vận hành kho là chuẩn | Có thể dùng WMS bên ngoài |
| **Procurement** | Mua hàng là chuẩn | |
| **Quality** | Quy trình QC theo chuẩn ngành | AQL là chuẩn có sẵn |
| **Seller Management** | Quản lý hồ sơ, duyệt, hiệu suất | |
| **Customer** | Quản lý hồ sơ khách | Trừ dữ liệu size — phần đó gần core |
| **Loyalty** | Chương trình điểm thưởng chuẩn | |
| **Campaign** | Quản lý chiến dịch | |
| **Review** | Đánh giá sản phẩm | |

### Lưu ý về Return

Return được xếp Supporting nhưng **cần đầu tư nhiều hơn một supporting domain thông thường**. Lý do: tỷ lệ hoàn hàng thời trang rất cao, và dữ liệu lý do hoàn là đầu vào quan trọng cho thiết kế sản phẩm.

Đây là ví dụ cho thấy phân loại không phải nhị phân — nó là chỉ dẫn phân bổ nguồn lực, không phải luật cứng.

---

## 5. Generic Domain — chi tiết và lý do

| Miền | Vì sao generic | Chiến lược |
|---|---|---|
| **Identity** | Xác thực, phân quyền là bài toán đã giải | Dùng thư viện/dịch vụ chuẩn, tự quản lý phân quyền nghiệp vụ |
| **Payment Gateway** | Không tự xử lý thẻ | Tích hợp PSP. **Nhưng ledger là của mình** |
| **Notification** | Gửi email/SMS/push | Dùng dịch vụ |
| **Search Infrastructure** | Công cụ tìm kiếm | Dùng công cụ có sẵn. **Nhưng logic xếp hạng là của mình** |
| **File Storage** | Lưu ảnh, video | Dùng object storage |
| **Shipping Integration** | Kết nối đơn vị vận chuyển | Tích hợp API |
| **Audit Log** | Ghi nhật ký | Hạ tầng chuẩn |
| **Analytics Infrastructure** | Thu thập, lưu trữ sự kiện | Hạ tầng chuẩn |

### Ranh giới quan trọng: generic hạ tầng vs core logic

Đây là chỗ dễ nhầm:

```text
Search Infrastructure (generic)     ≠   Ranking & Discovery Logic (core-ish)
  - đánh chỉ mục                          - xếp hạng theo hiệu suất seller
  - truy vấn văn bản                      - ưu tiên hàng còn size
  - lọc, phân trang                       - trộn nội dung creator vào kết quả

Payment Gateway (generic)           ≠   Financial Ledger (core)
  - xử lý thẻ                             - ghi sổ hoa hồng
  - chống gian lận thanh toán             - đối soát seller/creator
                                          - tính số dư nhiều bên

Notification Delivery (generic)     ≠   Notification Policy (supporting)
  - gửi email                             - gửi cái gì, cho ai, khi nào
  - gửi SMS                               - tần suất, kênh ưu tiên
```

**Nguyên tắc:** mua/dùng phần hạ tầng, tự xây phần quyết định nghiệp vụ. Phần quyết định luôn nằm sau interface do domain định nghĩa (nguyên tắc P13).

---

## 6. Phân loại có thể thay đổi theo thời gian

| Miền | Hiện tại | Có thể trở thành | Điều kiện |
|---|---|---|---|
| Pricing | Supporting | Core | Khi làm định giá động theo nhu cầu |
| Recommendation | Supporting | Core | Khi cá nhân hóa trở thành khác biệt chính |
| Loyalty | Supporting | Core | Khi chương trình thành viên là lý do khách ở lại |
| Warehouse | Supporting | Core | Khi tốc độ giao hàng thành lợi thế cạnh tranh |

**Nguyên tắc:** rà soát phân loại này mỗi 6–12 tháng. Chiến lược kinh doanh thay đổi thì phân bổ nguồn lực kỹ thuật phải thay đổi theo.

---

## 7. Danh sách miền đầy đủ và phân loại

| Miền | Phân loại | Module tương ứng |
|---|---|---|
| Customer | Supporting | [customer](../04-modules/customer.md) |
| Discovery | Supporting → Core | [recommendation](../04-modules/recommendation.md) |
| Content | **Core** | [content](../04-modules/content.md) |
| Creator | **Core** | [creator](../04-modules/creator.md) |
| Catalog | **Core** | [catalog](../04-modules/catalog.md) |
| Product | **Core** | [product](../04-modules/product.md) |
| Brand | **Core** | [catalog](../04-modules/catalog.md) |
| Marketplace | **Core** | [marketplace](../04-modules/marketplace.md) |
| Seller | Supporting | [seller](../04-modules/seller.md) |
| Pricing | Supporting | [pricing](../04-modules/pricing.md) |
| Promotion | Supporting | [promotion](../04-modules/promotion.md) |
| Cart | Supporting | [cart](../04-modules/cart.md) |
| Checkout | Supporting | [checkout](../04-modules/checkout.md) |
| Order | **Core** | [order](../04-modules/order.md) |
| Payment | **Core** (ledger) / Generic (gateway) | [payment](../04-modules/payment.md) |
| Inventory | Supporting | [inventory](../04-modules/inventory.md) |
| Fulfillment | **Core** | [fulfillment](../04-modules/fulfillment.md) |
| Return | Supporting (quan trọng) | [return](../04-modules/return.md) |
| Supply Chain | **Core** | [supply-chain](../04-modules/supply-chain.md) |
| Supplier | Supporting | [supply-chain](../04-modules/supply-chain.md) |
| Procurement | Supporting | [procurement](../04-modules/procurement.md) |
| Manufacturing | **Core** | [manufacturing](../04-modules/manufacturing.md) |
| Quality | Supporting | [quality](../04-modules/quality.md) |
| Warehouse | Supporting | [warehouse](../04-modules/warehouse.md) |
| Production Planning | **Core** | [supply-chain](../04-modules/supply-chain.md) |
| Recommendation | Supporting | [recommendation](../04-modules/recommendation.md) |
| Affiliate | **Core** | [affiliate](../04-modules/affiliate.md) |
| Campaign | Supporting | [campaign](../04-modules/campaign.md) |
| Loyalty | Supporting | [loyalty](../04-modules/loyalty.md) |
| Analytics | Supporting | [analytics](../04-modules/analytics.md) |
| Identity | Generic | [identity](../04-modules/identity.md) |
| Notification | Generic | [notification](../04-modules/notification.md) |

---

## 8. Hệ quả cho lộ trình

Phân loại này giải thích thứ tự trong [../10-roadmap/mvp.md](../10-roadmap/mvp.md):

```text
MVP phải có:
  - Fashion Product Model (core)     — nền tảng của mọi thứ
  - Marketplace & Offer (core)       — mô hình phải đúng từ đầu
  - Order & Fulfillment (core)       — mô hình phải đúng từ đầu
  - Các supporting tối thiểu để chạy được

MVP có thể làm đơn giản:
  - Recommendation (quy tắc đơn giản)
  - Promotion (chỉ mã giảm giá cơ bản)
  - Loyalty (chưa cần)

MVP dùng dịch vụ ngoài:
  - Payment gateway
  - Notification
  - File storage
```

**Nguyên tắc quyết định:** core domain phải có **mô hình đúng** ngay từ MVP, dù tính năng còn ít. Supporting domain có thể làm đơn giản rồi mở rộng. Generic domain không tự xây.

---

## 9. Tài liệu liên quan

- [bounded-contexts.md](bounded-contexts.md) — ranh giới ngữ cảnh
- [../00-overview/principles.md](../00-overview/principles.md) — nguyên tắc P15 về giải thích quyết định
- [../10-roadmap/mvp.md](../10-roadmap/mvp.md) — phạm vi MVP
