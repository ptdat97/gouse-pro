# Tổng hợp kiến trúc sau nghiên cứu OSS

> **Sau khi nghiên cứu 12 dự án OSS và 2 mô hình kinh doanh, kiến trúc cuối cùng của chúng ta nên như thế nào?**

---

## 1. Kết luận ngắn

Nghiên cứu **xác nhận phần lớn** thiết kế đã có và **thay đổi ba điểm cụ thể**.

```text
Xác nhận:  mô hình Offer, tách Order/FulfillmentOrder, ledger bất biến,
           modular monolith, tiền số nguyên, khóa lạc quan,
           ghi demand_signal từ MVP

Thay đổi:  1. Link table cho quan hệ vượt module      (từ Medusa)
           2. Adjustment là thực thể hạng nhất        (từ Sylius)
           3. Định nghĩa cơ sở tính hoa hồng creator  (từ Sylius + TikTok)

Xác lập:   PostgreSQL + sqlc cho tầng dữ liệu         (ADR-0010)
```

Không có phát hiện nào buộc phải thiết kế lại kiến trúc. Điều này có ý nghĩa: nếu nghiên cứu OSS **trước khi** thiết kế, có nguy cơ bị OSS định đoạt domain. Làm ngược lại — thiết kế từ nghiệp vụ, rồi đối chiếu OSS — cho kết quả tốt hơn.

---

## 2. Kiến trúc mục tiêu

```text
                         Next.js
                    Tầng trình bày
              Storefront · Seller · Creator · Admin
                           │
                           ▼
                     OpenAPI / HTTP
                    (62 operation, đã có)
                           │
                           ▼
                 ┌────────────────────┐
                 │    Go Backend      │
                 │ Modular Monolith   │
                 │   archcheck × 7    │
                 └────────────────────┘
                           │
        ┌──────────────────┼───────────────────┐
        ▼                  ▼                   ▼
  Commerce Core      Growth Engine        Supply Chain
                                                │
  Product            Creator              Supplier
  Catalog            Content              Procurement
  Price              Affiliate            Manufacturing
  Inventory          Campaign             Production
  Cart               Attribution          Quality
  Checkout           Recommendation       Warehouse
  Order                                   Fulfillment
  Payment
  Marketplace
  Seller

Hạ tầng:
  PostgreSQL + sqlc · Object Storage · CDN
  Outbox (trong DB) · OpenTelemetry (Phase 2)
```

**Không có** trong kiến trúc: microservices, plugin system, GraphQL, gRPC, message broker riêng, workflow engine, rule engine tổng quát, ORM.

Mỗi thứ vắng mặt đều có lý do ghi trong [adoption-policy.md](adoption-policy.md) mục 4.

---

## 3. ADOPT — điều chúng ta lấy trực tiếp

### 3.1 Đã cài đặt

| Mẫu | Nguồn | Nơi cài |
|---|---|---|
| Chia tiền bảo toàn tổng | Flamingo `SplitInPayables` | `kernel/money/allocate.go` |
| Tiền số nguyên đơn vị nhỏ nhất | Digota | `kernel/money/money.go` |
| Interface công khai của module | Magento service contracts | `public.go` + archcheck R1 |
| Ports & Adapters | Flamingo | Toàn kiến trúc |

### 3.2 Sẽ cài đặt

| Mẫu | Nguồn | Giai đoạn |
|---|---|---|
| **Link table không khóa ngoại** | Medusa | Cùng module đầu tiên |
| **Adjustment là thực thể** | Sylius | MVP — cần cho hoàn tiền đúng |
| Nhiều Payment cho một Order | Sylius | Phase 2 |
| Repository in-memory cho test | Flamingo fake adapters | Module đầu tiên |
| Migration SQL có phiên bản | GoShop | Cùng tầng database |
| testcontainers | GoShop | Cùng tầng database |
| Điều kiện khuyến mãi là dữ liệu | Shopware | Phase 2 |

---

## 4. ADAPT — điều chúng ta thiết kế lại

Bảng này là phần quan trọng nhất: nó cho thấy chúng ta **không sao chép** mà **quyết định có ý thức**.

| Vấn đề | OSS làm | Chúng ta làm | Vì sao khác |
|---|---|---|---|
| Tranh chấp tồn kho | Khóa bi quan phân tán (Digota) | Khóa lạc quan + `CHECK` | Live commerce: khóa bi quan tạo hàng đợi tuần tự |
| Bù trừ khi lỗi | Workflow engine với compensation (Medusa) | TTL tự hết hạn | Bù trừ thụ động **không thể thất bại** |
| Ngữ cảnh bán | Channel per seller (Vendure) | **Offer** per seller | Cần so sánh giá và gộp đơn nhiều seller |
| Trạng thái đơn | Một state machine | Order (suy ra) + FulfillmentOrder | Nhiều nguồn hàng, xử lý từng phần |
| Xuất bản có lịch | Nhân đôi bảng (QOR Publish2) | Trạng thái + thời điểm | Chỉ cần hoãn công bố, không sửa song song |
| Xử lý ảnh | Đồng bộ khi tải lên (QOR) | Bất đồng bộ trong worker | Không chặn request của khách |
| Điều kiện khuyến mãi | Rule engine lồng nhau (Shopware) | Danh sách phẳng AND | Tránh trừu tượng hóa sớm, phải giải thích được |
| Tính khả bán | Tổng hợp + chỉ mục (Magento MSI) | Cột riêng, cập nhật nguyên tử | An toàn hơn, không lệch chỉ mục |
| Tiền | `big.Float` (Flamingo) | `int64` | Lưu trữ đơn giản, hiệu năng ổn định |

---

## 5. REJECT — điều chúng ta cố ý không làm

Xem danh sách đầy đủ tại [adoption-policy.md](adoption-policy.md) mục 4. Ba từ chối quan trọng nhất:

### 5.1 ORM (GORM, ent)

```text
Vấn đề gốc: model ORM trở thành model domain
    → domain layer phụ thuộc thư viện database (vi phạm archcheck R2)
    → khóa ngoại ORM tạo quan hệ vượt ranh giới module (vi phạm ADR-0005)
    → không test được domain mà không có ORM
```

Đây không phải sở thích mà là **mâu thuẫn kiến trúc trực tiếp**. Xem [ADR-0010](../adr/0010-database-layer.md).

### 5.2 Plugin system

Mọi nền tảng lớn đều có. Chúng ta không, vì chi phí (duy trì điểm mở rộng ổn định, quản lý phiên bản API, xử lý xung đột, kiểm soát bảo mật code bên thứ ba) chỉ đáng khi **bán framework**. Chúng ta xây cho chính mình.

### 5.3 Channel-per-seller làm marketplace

Vendure, Shopware, Medusa đều hỗ trợ marketplace theo cách này. Nếu chọn nó, ba năng lực cốt lõi của chúng ta **không làm được**:

```text
✗ So sánh giá nhiều nhà bán trên cùng một trang
✗ Buy box
✗ Một giỏ hàng gộp own brand + nhiều seller
```

Đây là ví dụ rõ nhất cho nguyên tắc "OSS không được định đoạt domain".

---

## 6. Miền độc quyền — phải là của chúng ta

Bốn nhóm năng lực này **không được** trở thành phụ thuộc vào framework bên ngoài.

### 6.1 Mô hình Offer cho marketplace

```text
Product → Variant → SKU → Offer → Inventory
                            │
                    nhiều seller cùng bán một SKU
                    buy box · hoa hồng · đối soát
```

**Không OSS nào có.** Ma trận: 6/7 năng lực marketplace phải tự xây.

### 6.2 Quy kết creator

```text
Creator → Content → ProductTag → Click → Attribution → Commission
```

**Không OSS nào có.** Chỉ học được mô hình nghiệp vụ từ nền tảng đóng.

Điểm phải giữ: lưu **toàn bộ chuỗi click**, không chỉ click được quy kết — cho phép đổi mô hình quy kết sau này mà vẫn tính lại được dữ liệu quá khứ.

### 6.3 Tín hiệu nhu cầu → lập kế hoạch sản phẩm

```text
Hành vi khách → DemandSignal (gồm nhu cầu KHÔNG được đáp ứng)
             → Dự báo → Kế hoạch sản xuất theo SIZE
```

**Không OSS nào có.** SHEIN xác nhận đây là năng lực quyết định.

Điểm phải giữ: ghi `demand_signal` **từ MVP** dù Phase 3 mới dùng — dữ liệu lịch sử không tạo ngược được.

### 6.4 Vận hành own brand

```text
Concept → Design → TechPack → Costing → Sample → Duyệt
       → ProductionOrder → ProductionBatch (giá vốn theo lô)
       → QC → Warehouse → Inventory
```

**Không OSS nào có.** Ma trận: 8/9 năng lực chuỗi cung ứng phải tự xây.

Điểm phải giữ: giá vốn gắn với **lô**, không gắn với SKU — nếu không, mọi tính toán biên lợi nhuận đều sai.

---

## 7. Ba thay đổi cụ thể và tài liệu cần cập nhật

### 7.1 Link table (Medusa)

**Vấn đề:** tài liệu nói "không khóa ngoại vượt module" nhưng không nói cách mô hình hóa quan hệ nhiều-nhiều vượt module.

**Giải pháp:** link table với ba quy tắc:

```text
1. Chỉ chứa hai định danh + metadata của chính quan hệ
2. KHÔNG có ràng buộc khóa ngoại vượt module
3. Thuộc về module SỞ HỮU Ý NGHĨA của quan hệ
```

Bốn quan hệ áp dụng: `product_tag`, `outfit_item`, `campaign_participant`, `brand_authorization`.

→ Cập nhật [05-data/data-model.md](../05-data/data-model.md)

### 7.2 Adjustment (Sylius)

**Vấn đề:** [07-workflows/return.md](../07-workflows/return.md) yêu cầu "giảm giá phân bổ xuống dòng hàng và lưu lại" nhưng không nói dưới dạng gì.

**Giải pháp:**

```text
OrderLineAdjustment {
    type         PROMOTION | TAX | SHIPPING | COMMISSION | FEE
    label        nhãn hiển thị
    amount       Money (âm = giảm)
    source_type  nguồn gốc
    source_id
    cost_bearer  PLATFORM | SELLER | SHARED   ← bổ sung của chúng ta
}
```

→ Cập nhật [02-domain/entities.md](../02-domain/entities.md), [04-modules/order.md](../04-modules/order.md)

### 7.3 Cơ sở tính hoa hồng creator

**Vấn đề:** "% giá trị đơn" không xác định khi có nhiều loại giảm giá với bên chịu chi phí khác nhau.

**Giải pháp:**

```text
Cơ sở = giá niêm yết
      − Adjustment có cost_bearer = SELLER
      (KHÔNG trừ Adjustment do nền tảng chịu)
      (KHÔNG tính thuế)
```

**Lý do:** giảm giá do nền tảng tài trợ là chi phí marketing của nền tảng. Trừ nó khỏi cơ sở tính nghĩa là creator bị phạt vì nền tảng chạy khuyến mãi.

→ Cập nhật [01-business/monetization.md](../01-business/monetization.md), [04-modules/affiliate.md](../04-modules/affiliate.md)

---

## 8. Rà soát nhất quán sau nghiên cứu

Kiểm tra theo mục 32 của nhiệm vụ.

### Domain

```text
✓ Trách nhiệm rõ ràng — ma trận trách nhiệm module không đổi
✓ Không trùng lặp thực thể — Adjustment là thực thể MỚI, không trùng
✓ Bounded context đúng — nghiên cứu không phát hiện ranh giới sai
✓ Aggregate hợp lý — Adjustment thuộc aggregate Order (cùng giao dịch)
```

### Kiến trúc

```text
✓ Không phụ thuộc vòng — archcheck R5 kiểm tra tự động
✓ Modular monolith thật sự module — archcheck 7 quy tắc, CI chặn merge
✓ Logic nghiệp vụ độc lập HTTP — archcheck R2, domain chỉ dùng kernel
✓ Logic nghiệp vụ độc lập Next.js — ADR-0004, frontend không có DSN
```

### Commerce

```text
✓ Một sản phẩm nhiều offer     — mô hình Offer (ADR-0007)
✓ Một đơn nhiều seller         — FulfillmentOrder theo seller
✓ Giao từng phần               — mỗi FO có vòng đời riêng
✓ Hoàn tiền từng phần          — Adjustment cho phép tính đúng giá thực trả
✓ Trả hàng từng phần           — ReturnLine theo OrderLine
```

Riêng "hoàn tiền từng phần" **được cải thiện** nhờ Adjustment — trước đó tài liệu nêu yêu cầu nhưng thiếu cơ chế.

### Marketplace

```text
✓ Sở hữu seller rõ ràng   — seller thấy FulfillmentOrder, không thấy Order
✓ Đối soát kiểm toán được — ledger bất biến, Adjustment truy vết nguồn gốc
✓ Hoa hồng độc lập tính toán đơn — marketplace định nghĩa quy tắc,
                                    order đóng băng, payment ghi sổ
```

### Creator

```text
✓ Nội dung tham chiếu sản phẩm — ProductTag (link table)
✓ Quy kết theo dõi được        — Attribution bất biến, lưu đủ chuỗi click
✓ Hoa hồng tính và kiểm toán được — cơ sở tính ĐÃ ĐƯỢC ĐỊNH NGHĨA (mới)
```

### Chuỗi cung ứng

```text
✓ Own brand có nhà cung cấp    — ProductDevelopment → Supplier
✓ Truy vết theo lô             — ProductionBatch với unit_cost riêng
✓ QC ảnh hưởng tồn kho         — quality.approved → warehouse → inventory
✓ Tín hiệu nhu cầu điều khiển bổ sung — demand_signal → replenishment
```

### Tài chính

```text
✓ Tiền an toàn         — int64 + currency, Allocate không mất đồng
✓ Ledger kiểm toán được — bất biến, RULE chặn UPDATE/DELETE
✓ Hoàn tiền truy vết được — Adjustment + bút toán đảo ngược
```

### API

```text
✓ Độc lập Next.js      — 62 operation, sinh kiểu TypeScript thành công
✓ App di động dùng được — cùng REST API
✓ Đối tác dùng được    — /api/v1/partner/ (Phase 4) + webhook
```

### Mở rộng tương lai

```text
✓ Tách Search          — nhóm 1, tầng nền đồ thị phụ thuộc
✓ Tách Recommendation  — nhóm 2, đã có interface
✓ Tách Analytics       — nhóm 1, chỉ nhận event
✓ Tách Media           — nhóm 1, xử lý bất đồng bộ
✓ Tách Supply Chain    — nhóm 3, khi có đội riêng
```

Link table (từ Medusa) **cải thiện** khả năng tách: quan hệ vượt module giờ có mô hình rõ ràng thay vì khóa ngoại ngầm.

---

## 9. Nguyên tắc cuối

Hệ thống cuối cùng **không được là**:

```text
Một bản Go của một framework thương mại điện tử có sẵn.
```

Nó phải là:

```text
Fashion Commerce Platform độc quyền
  xây bằng Go
  tăng tốc nhờ OSS đã kiểm chứng
  nền tảng modular monolith
  kiến trúc API-first
  Next.js làm tầng trình bày
  và các miền độc quyền: Marketplace + Creator + Demand + Supply Chain
```

Bằng chứng cụ thể cho thấy chúng ta không bị OSS định đoạt:

```text
24/72 năng lực phải TỰ XÂY vì OSS không có
Từ chối channel-per-seller dù 3 nền tảng lớn dùng nó
Từ chối ORM dù đó là mặc định của hệ sinh thái Go
Từ chối plugin system dù mọi nền tảng lớn đều có
Chọn khóa lạc quan thay vì khóa phân tán như Digota
```

---

## 10. Tài liệu liên quan

- [README.md](README.md) — điều hướng nghiên cứu
- [research-matrix.md](research-matrix.md) — so sánh 40 năng lực
- [adoption-policy.md](adoption-policy.md) — quy tắc quyết định
- [dependency-registry.md](dependency-registry.md) — license và thư viện
- [../adr/0010-database-layer.md](../adr/0010-database-layer.md) — quyết định sqlc
- [../10-roadmap/deliverables.md](../10-roadmap/deliverables.md) — tổng hợp bàn giao
