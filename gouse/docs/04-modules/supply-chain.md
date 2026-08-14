# Module: Supply Chain

| | |
|---|---|
| **Bounded Context** | Supply Chain |
| **Phân loại** | **Core** |
| **Giai đoạn** | Phase 3 (nhưng **ghi tín hiệu nhu cầu từ MVP**) |

---

## 1. Trách nhiệm

- Thu thập và tổng hợp **tín hiệu nhu cầu**
- Dự báo nhu cầu
- Lập kế hoạch sản phẩm (sản xuất gì, bao nhiêu, khi nào)
- Quản lý phát triển sản phẩm own brand (concept → mẫu → duyệt)
- Đề xuất bổ sung hàng

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Đặt mua hàng | `procurement` |
| Đơn sản xuất, lô sản xuất | `manufacturing` |
| Kiểm định chất lượng | `quality` |
| Vận hành kho | `warehouse` |
| Số lượng tồn kho | `inventory` |
| Sản phẩm trong danh mục | `product`, `catalog` |

---

## 2.1. Ranh giới quan trọng nhất: own brand ≠ hàng seller

Chuỗi cung ứng ở tài liệu này **chỉ áp dụng cho hàng own brand**. Hàng của
seller đi một đường hoàn toàn khác.

```text
OWN BRAND                          MARKETPLACE SELLER
─────────────────────────          ──────────────────────────
Tín hiệu nhu cầu                   Seller tự quyết định nhập gì
    ↓                                  ↓
Dự báo · Kế hoạch sản phẩm         (nền tảng KHÔNG can thiệp)
    ↓                                  ↓
Thu mua / Sản xuất                 Seller tự nhập hàng
    ↓                                  ↓
Kiểm định chất lượng               Nền tảng chỉ QUAN SÁT tồn kho khai báo
    ↓                                  ↓
Kho nền tảng                       Kho của seller
    ↓                                  ↓
Nền tảng SỞ HỮU hàng               Seller SỞ HỮU hàng
```

### Hệ quả bắt buộc, không được nhầm

| | Own brand | Seller |
|---|---|---|
| Ai sở hữu hàng trong kho | Nền tảng | Seller |
| Ghi sổ khi bán | Doanh thu **toàn bộ** + giá vốn (COGS) | Chỉ **hoa hồng** |
| Hàng trong kho là tài sản của ai | Nền tảng | **KHÔNG** phải của nền tảng |
| Ai quyết định nhập bao nhiêu | Module này | Seller tự quyết |
| Chuỗi cung ứng có áp dụng không | **Có** | **Không** |

Điểm thứ hai và thứ ba là chỗ sai sẽ tạo ra báo cáo tài chính sai: gộp hàng
seller vào tài sản nền tảng làm phồng bảng cân đối kế toán bằng hàng không
thuộc về mình. Xem [ADR-0008](../adr/0008-financial-ledger.md).

### Vì sao KHÔNG gộp hai luồng thành một trừu tượng

Cám dỗ tự nhiên là dựng một khái niệm "nguồn cung" chung cho cả hai. Đừng.

```text
Hai luồng khác nhau ở:
    - Ai ra quyết định       (nền tảng vs seller)
    - Ai sở hữu tài sản      (khác nhau về kế toán)
    - Dữ liệu nào có sẵn     (own brand có lô sản xuất, seller không)
    - Chu kỳ thời gian       (sản xuất hàng tháng vs nhập hàng hàng tuần)
```

Trừu tượng hóa sớm ở đây sẽ tạo ra một mô hình mà cả hai luồng đều phải bẻ
cong để vừa — và chỗ bẻ cong đầu tiên sẽ là quyền sở hữu tài sản, tức là
phần kế toán. Nguyên tắc P15 và P16.

**Điểm giao duy nhất giữa hai luồng:** cả hai đều kết thúc ở `inventory`
với `owner_id` khác nhau. Đó là chỗ hợp nhất đúng — sau khi hàng đã nằm
trong kho, việc bán ra giống nhau.

---

## 3. Vì sao là Core Domain

Đây là năng lực khó sao chép nhất:

| Năng lực | Đối thủ sao chép trong |
|---|---|
| Giao diện đẹp | Vài tuần |
| Chính sách hoa hồng | Vài ngày |
| Mạng lưới creator | Vài tháng |
| **Chuyển nhu cầu thành hàng hóa đúng lượng, đúng lúc** | **Nhiều năm** |

**Lưu ý phân loại:** phần **thông minh** (tín hiệu, dự báo, kế hoạch) là Core. Phần **thực thi** (mua hàng, kiểm hàng, quản lý kệ) là Supporting và nằm ở module khác.

---

## 4. Tín hiệu nhu cầu — bắt đầu từ MVP

### 4.1 Vì sao phải ghi từ MVP dù chưa dùng

```text
Dữ liệu lịch sử KHÔNG THỂ TẠO NGƯỢC.

Nếu đến Phase 3 mới bắt đầu ghi:
    → phải chờ thêm nhiều tháng mới đủ dữ liệu lập kế hoạch
    → mất cả năm trước khi bánh đà bắt đầu quay
```

Đây là một trong số ít trường hợp "làm sớm dù chưa cần" là quyết định đúng — vì dữ liệu có tính tích lũy.

### 4.2 Các loại tín hiệu

```text
VIEW                — lượt xem sản phẩm
SEARCH              — từ khóa tìm kiếm
SEARCH_NO_RESULT    — tìm không ra kết quả  ← QUAN TRỌNG
CLICK               — click từ nội dung
ADD_TO_CART         — thêm giỏ (tín hiệu mạnh)
WISHLIST            — lưu yêu thích (ý định rõ ràng)
ORDER               — đơn hàng thực tế
STOCKOUT            — hết hàng  ← QUAN TRỌNG
NOTIFY_REQUEST      — đăng ký nhận thông báo có hàng
RETURN              — hoàn hàng và lý do
```

### 4.3 Hai tín hiệu quan trọng nhất thường bị bỏ qua

`SEARCH_NO_RESULT` và `STOCKOUT` đo **nhu cầu không được đáp ứng** — thứ không xuất hiện trong dữ liệu bán hàng.

```text
Nếu chỉ nhìn doanh số:
    "Áo khoác dạ bán 200 chiếc"  →  kết luận: nhu cầu 200

Thực tế:
    - Hết hàng từ tuần thứ 3
    - 1.500 lượt tìm kiếm sau khi hết
    - 400 lượt đăng ký nhận thông báo
    →  nhu cầu thật gần 800
```

Lập kế hoạch chỉ dựa vào doanh số lịch sử sẽ **liên tục sản xuất thiếu** hàng bán chạy. Đây là sai lầm kinh điển.

---

## 5. Lập kế hoạch sản phẩm

### Đầu vào

```text
Dự báo nhu cầu theo SKU
Tồn kho hiện tại + hàng đang về
Lead time nhà cung cấp
MOQ nhà cung cấp
Ngân sách sản xuất
Lịch mùa vụ / bộ sưu tập
Biên lợi nhuận mục tiêu
```

### Đầu ra

```text
Kế hoạch sản xuất:
    - SKU nào
    - số lượng THEO TỪNG SIZE
    - nhà cung cấp
    - thời điểm đặt và thời điểm cần hàng
```

### Phân bổ size — đặc thù thời trang

```text
Sản xuất 500 áo KHÔNG phải 500 chiếc giống nhau:

    S:   15%  =  75
    M:   30%  = 150
    L:   30%  = 150
    XL:  20%  = 100
    XXL:  5%  =  25
```

Phân bổ sai gây thiệt hại kép: hết size M trong hai tuần (mất doanh số) **và** tồn XXL đến cuối mùa (hàng ế).

**Hệ quả kiến trúc:** kế hoạch phải ở mức **SKU** (bao gồm size), không phải mức Product.

### Mâu thuẫn cốt lõi cần hệ thống hiển thị

```text
Dự báo bán:        300 chiếc
MOQ nhà cung cấp:  500 chiếc
Lead time:         10 tuần
Mùa còn lại:       14 tuần

Phương án:
  A. Đặt 500  → dư 200, rủi ro tồn kho ~40% giá trị lô
  B. Đặt 0    → mất doanh số 300 chiếc
  C. Tìm nhà cung cấp MOQ thấp hơn, giá cao hơn
  D. Đặt 500 và chủ động lên kế hoạch xả 200 cuối mùa
```

**Yêu cầu:** hệ thống phải **hiển thị mâu thuẫn này ở bước lập kế hoạch**, kèm ước tính tài chính từng phương án. Không để phát hiện sau khi hàng đã về kho.

Đây là phần mềm **hỗ trợ ra quyết định**, không chỉ ghi chép quyết định.

---

## 6. Bổ sung hàng (Replenishment)

```text
Reorder point = (Tốc độ bán × Lead time) + Safety stock

Ví dụ: 50 chiếc/tuần, lead time 6 tuần, safety stock 100
       → Reorder point = 400

Khi tồn kho ≤ 400 → tạo đề xuất bổ sung
```

**Nguyên tắc:** hệ thống **đề xuất**, con người **quyết định**.

Tự động đặt hàng hoàn toàn là rủi ro lớn ở giai đoạn đầu — một lỗi tính toán có thể dẫn tới đơn sản xuất sai hàng trăm triệu đồng.

---

## 7. Phát triển sản phẩm own brand

```text
ProductDevelopment {
    concept_name
    collection_id
    target_cost, target_retail_price, target_margin
    demand_signal_ref       ← tín hiệu nào dẫn tới ý tưởng này
    status  CONCEPT → DESIGN → TECH_PACK → COSTING → SAMPLING
            → SAMPLE_APPROVED → PLANNING → IN_PRODUCTION → LAUNCHED
    catalog_product_id      ← liên kết sau khi tạo trong Catalog
}
```

**Quan trọng:** đây là entity của **Supply Chain context**, không phải Catalog. Khi mẫu được duyệt, phát event để Catalog tạo `Product` qua Anti-Corruption Layer.

Xem [../02-domain/bounded-contexts.md](../02-domain/bounded-contexts.md) mục 5 và [product.md](product.md) mục 5.

---

## 8. Dữ liệu sở hữu

```sql
demand_signal            -- tín hiệu thô (BẤT BIẾN, khối lượng lớn)
demand_aggregate         -- tổng hợp theo SKU/tuần
forecast
product_development
tech_pack
sample
production_plan
plan_line                -- phân bổ theo size
replenishment_suggestion
```

`demand_signal` có khối lượng rất lớn — cần phân vùng theo thời gian và chính sách lưu trữ (giữ chi tiết N tháng, sau đó chỉ giữ tổng hợp).

---

## 9. Interface công khai

```go
type PublicAPI interface {
    RecordDemandSignal(ctx, signal DemandSignalInput) error   // bất đồng bộ

    GetDemandIndicator(ctx, skuID string, period DateRange) (*DemandIndicator, error)
    GetForecast(ctx, skuID string, horizon int) (*Forecast, error)

    GetReplenishmentSuggestions(ctx, filter Filter) ([]Suggestion, error)
    ApproveReplenishment(ctx, suggestionID string) (*ProductionPlanRef, error)

    GetProductDevelopment(ctx, devID string) (*DevelopmentView, error)
    ApproveSample(ctx, sampleID string, decision Decision) error
}
```

---

## 10. Event

**Phát ra:**

| Event | Bên nghe |
|---|---|
| `demand_signal.recorded` | (nội bộ tổng hợp) |
| `product_development.approved` | **product (tạo CatalogProduct qua ACL)** |
| `production_plan.created` | procurement, manufacturing |
| `replenishment.suggested` | notification |
| `forecast.updated` | (nội bộ) |

**Lắng nghe:**

| Event | Từ | Ghi tín hiệu loại |
|---|---|---|
| `inventory.depleted` | inventory | `STOCKOUT` |
| `cart.item_added` | cart | `ADD_TO_CART` |
| `order.placed` | order | `ORDER` |
| `wishlist.item_added` | customer | `WISHLIST` |
| `content.viewed` | content | `VIEW` |
| `return.inspected` | return | `RETURN` (kèm lý do) |
| `affiliate.click_recorded` | affiliate | `CLICK` |

Bảng này là **cơ chế hoàn thành bánh đà** — dữ liệu hành vi chảy ngược vào lập kế hoạch sản xuất.

---

## 11. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Ghi tín hiệu nhu cầu từ MVP, dù chưa dùng |
| 2 | Ghi tín hiệu bất đồng bộ, không làm chậm luồng chính |
| 3 | Kế hoạch ở mức SKU (bao gồm size) |
| 4 | Hệ thống đề xuất, người quyết định |
| 5 | Hiển thị mâu thuẫn MOQ/dự báo ở bước lập kế hoạch |
| 6 | `ProductDevelopment` thuộc Supply Chain, không thuộc Catalog |
| 7 | Không chuyển sản xuất khi chưa duyệt mẫu |
| 8 | Dự báo dùng quy tắc trước, để dành interface cho mô hình |

---

## 12. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | **Chỉ ghi nhận `demand_signal`** — chưa có giao diện, chưa dùng |
| **Phase 2** | Tổng hợp tín hiệu, báo cáo nhu cầu cơ bản |
| **Phase 3** | Dự báo, lập kế hoạch, phát triển sản phẩm, bổ sung hàng |
| **Phase 4** | Dự báo nâng cao, tối ưu phân bổ size, demand intelligence |

---

## 13. Tài liệu liên quan

- [../01-business/supply-chain.md](../01-business/supply-chain.md) — nghiệp vụ đầy đủ
- [../01-business/own-brand.md](../01-business/own-brand.md) — vòng đời sản phẩm own brand
- [procurement.md](procurement.md), [manufacturing.md](manufacturing.md)
- [../07-workflows/replenishment.md](../07-workflows/replenishment.md)
