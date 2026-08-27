# Chính sách tiếp nhận OSS

## 1. Sáu quyết định

Mọi năng lực OSS hoặc nhà cung cấp được xét đều rơi vào một trong sáu nhóm:

```text
ADOPT      Dùng trực tiếp — mẫu đã kiểm chứng, phù hợp domain
ADAPT      Lấy pattern hoặc cách cài đặt, rồi điều chỉnh cho domain của ta
WRAP       Đặt OSS/nhà cung cấp phía SAU interface do domain của ta định nghĩa
BUILD      Tự xây — domain quan trọng, hoặc OSS không phù hợp
REFERENCE  Chỉ nghiên cứu để hiểu vấn đề — không dùng gì từ nó
REJECT     Cố ý không dùng — mâu thuẫn kiến trúc hoặc chi phí không tương xứng
```

### Câu hỏi quyết định thuộc nhóm nào

```text
Đây là HẠ TẦNG CHUNG hay DOMAIN CHIẾN LƯỢC?
    │
    ├── Hạ tầng chung (ai cũng cần, không tạo khác biệt)
    │       │
    │       ├── Dùng thẳng được?            → ADOPT
    │       ├── Sẽ đổi nhà cung cấp?        → WRAP
    │       └── Cần sửa cho hợp ràng buộc?  → ADAPT
    │
    └── Domain chiến lược (là lợi thế cạnh tranh)
            │
            ├── OSS có mẫu hay?             → ADAPT (lấy ý tưởng, tự cài)
            └── Không có gì phù hợp?        → BUILD
```

**Quy tắc không được vi phạm:** không có OSS hay framework nào được quyết
định hình dạng của một domain chiến lược. Xem [mục 1.1](#11-hạ-tầng-chung-vs-domain-chiến-lược).

### WRAP khác ADOPT ở chỗ nào

`ADOPT` là lấy một **mẫu thiết kế hoặc thuật toán** vào code của ta —
sau đó nó là code của ta. `WRAP` là để **một hệ thống bên ngoài tiếp tục
tồn tại** phía sau một interface do domain định nghĩa.

```text
ADOPT:  Money.Allocate()  ← thuật toán chia tiền, giờ là code của ta
WRAP:   payment.Gateway   ← interface của ta; Stripe/VNPay nằm phía sau
```

Dấu hiệu cần WRAP: **năng lực này sẽ đổi nhà cung cấp**. Cổng thanh toán,
hãng vận chuyển, dịch vụ gửi email, lưu trữ file, tìm kiếm — tất cả đều
đổi. Nguyên tắc P13 nói chính điều này.

### REFERENCE khác REJECT ở chỗ nào

`REJECT` là **kết luận về một lựa chọn**: đã xét, không dùng, có lý do.
`REFERENCE` là **nguồn để hiểu vấn đề**: đọc để biết ngành đã giải bài
toán này thế nào, không lấy gì cụ thể.

Phân biệt này quan trọng khi đọc lại sau một năm: `REJECT` nghĩa là "đừng
xét lại nếu bối cảnh chưa đổi"; `REFERENCE` nghĩa là "chỗ này có kiến thức,
quay lại đọc khi cần".

---

## 1.1. Hạ tầng chung vs domain chiến lược

Đây là phân biệt quan trọng nhất của toàn bộ tài liệu này.

### Domain chiến lược — PHẢI do chúng ta kiểm soát

Đây là những domain tạo nên lợi thế cạnh tranh. Chúng quyết định nền tảng
này khác gì một website thương mại điện tử thông thường.

```text
Marketplace Offer            Demand Signal
Seller                       Demand Planning
Creator                      Fashion Product Intelligence
Content Commerce             Supplier Network
Attribution                  Procurement
                             Manufacturing
                             Quality
                             Replenishment
                             Own Brand Operations
```

**Ràng buộc với các domain này:**

| Được | Không được |
|---|---|
| Đọc OSS để hiểu vấn đề (`REFERENCE`) | Dùng model dữ liệu của OSS làm model domain |
| Lấy thuật toán rời rạc (`ADAPT`) | Để framework quyết định aggregate hay ranh giới |
| Tự cài từ đầu (`BUILD`) | Ép nghiệp vụ theo hình dạng công cụ |

**Ví dụ đã áp dụng:** Vendure và Shopware mô hình hóa marketplace bằng
"channel per seller". Nếu theo, ta mất khả năng so sánh giá giữa các seller
trên cùng một SKU và không gộp được đơn nhiều seller — hai thứ cốt lõi của
mô hình kinh doanh. Vì vậy `Offer` được `BUILD`, còn cách làm của họ là
`REJECT`. Xem [vendure.md](vendure.md).

### Hạ tầng chung — nên tận dụng OSS

Đây là những thứ ai cũng cần và không tạo ra khác biệt cạnh tranh nào. Tự
xây là tiêu tiền vào chỗ không ai trả tiền cho.

```text
Adapter thanh toán       Jobs / hàng đợi
Tìm kiếm                 Caching
Media / xử lý ảnh        Observability
Lưu trữ file             Hạ tầng trang quản trị
Gửi email                Driver database, migration
```

**Ràng buộc với nhóm này:** dùng OSS thoải mái, nhưng **phía sau interface
do domain định nghĩa** (`WRAP`) nếu năng lực đó sẽ đổi nhà cung cấp.

### Vì sao phân biệt này đáng ghi thành quy tắc

Nhầm lẫn theo hai chiều đều tốn kém, và tốn theo hai cách khác nhau:

```text
Nhầm chiến lược thành hạ tầng
    → dùng framework cho Offer/Attribution
    → nghiệp vụ bị ép theo hình dạng công cụ
    → sửa = viết lại domain

Nhầm hạ tầng thành chiến lược
    → tự xây hệ thống gửi email, hàng đợi, tìm kiếm
    → tiêu thời gian vào chỗ không tạo khác biệt
    → sửa = vứt code đi, dùng OSS
```

Chiều thứ nhất tệ hơn nhiều: nó làm hỏng thứ tạo ra giá trị.

---

## 2. ADOPT — lấy gần như nguyên vẹn

### Tiêu chí

```text
✓ Giải đúng vấn đề chúng ta có
✓ Không ép domain của chúng ta theo hình dạng của OSS
✓ Là mẫu thiết kế hoặc thuật toán, không phải framework
✓ License cho phép (nếu sao chép code)
```

### Danh sách ADOPT

| Năng lực | Nguồn | Trạng thái |
|---|---|---|
| Thuật toán chia tiền bảo toàn tổng | Flamingo | **Đã cài** `Money.Allocate()` |
| Tiền bằng số nguyên đơn vị nhỏ nhất | Digota | **Đã cài** `Money` |
| Ports & Adapters | Flamingo | **Đã có** trong kiến trúc |
| Adapter giả cho test | Flamingo | Kế hoạch: repository in-memory |
| **Link table không khóa ngoại** | Medusa | **Bổ sung** — xem mục 5 |
| **Adjustment là thực thể** | Sylius | **Bổ sung** — xem mục 5 |
| **Nhiều state machine tách biệt** | Sylius | Xác nhận + bổ sung nhiều Payment |
| Tách tồn vật lý / khả bán | Magento MSI | Đã có, làm rõ thuật ngữ |
| Reservation không đụng tồn vật lý | Magento MSI | Đã có |
| Job dò sai lệch reservation | Magento MSI | Đã có, được xác nhận |
| Tách thuộc tính chung / theo ngữ cảnh | Saleor | Xác nhận mô hình Offer |
| Điều kiện khuyến mãi là dữ liệu có kiểu | Shopware | Kế hoạch Phase 2 |
| Migration SQL có phiên bản | GoShop | Kế hoạch |
| testcontainers cho integration test | GoShop | Kế hoạch |
| Service contracts / interface công khai | Magento, Flamingo | **Đã có** `public.go` |

---

## 3. ADAPT — lấy ý tưởng, thiết kế lại

### Tiêu chí

```text
✓ Vấn đề đúng, giải pháp không phù hợp ràng buộc của chúng ta
✓ Cần đơn giản hóa hoặc bổ sung cho domain thời trang/marketplace
```

### Danh sách ADAPT với lý do khác biệt

| Năng lực | Nguồn | Chúng ta làm khác thế nào và vì sao |
|---|---|---|
| Xử lý tranh chấp tồn kho | Digota | Khóa **lạc quan** thay vì bi quan — chịu được live commerce |
| Bù trừ khi lỗi | Medusa | Ưu tiên bù trừ **thụ động** (TTL) — không thể thất bại |
| Channel / ngữ cảnh bán | Vendure, Saleor | Ngữ cảnh = **nhà bán** (Offer), không phải kênh |
| State machine | Sylius, QOR | Phương thức domain thuần Go — domain layer phải sạch |
| Nháp/xuất bản có lịch | QOR | Trạng thái + thời điểm, **không nhân đôi bảng** |
| Xử lý ảnh | QOR | **Bất đồng bộ** trong worker, không chặn API |
| Điều kiện khuyến mãi | Shopware | Danh sách **phẳng nối AND**, không lồng nhau |
| Thuế | GoCommerce | Là `Adjustment`, một mức VAT ở MVP |
| Coupon | GoCommerce | Bổ sung `cost_bearer` cho marketplace |
| Metadata | Saleor | Phân biệt công khai/nội bộ, giữ quy tắc JSONB chặt |
| Khái niệm Stock (nhóm nguồn) | Magento | Hoãn tới Phase 2 khi có nhiều kho |

---

## 4. WRAP — đặt sau interface của chúng ta

### Tiêu chí

```text
✓ Là hạ tầng chung, không phải domain chiến lược
✓ Sẽ đổi nhà cung cấp (không phải "có thể", mà là "sẽ")
✓ Domain của ta định nghĩa interface, nhà cung cấp cài đặt
✓ Đổi nhà cung cấp = viết adapter mới, KHÔNG sửa module nghiệp vụ
```

### Danh sách WRAP

| Năng lực | Interface của ta | Phía sau là gì | Trạng thái |
|---|---|---|---|
| Cổng thanh toán | `payment.Gateway` | VNPay, Stripe… | MVP: một cổng |
| Vận chuyển | `fulfillment.Carrier` | GHN, GHTK… | MVP: một đối tác |
| Gửi email | `notification.Sender` | SES, Resend… | MVP |
| Lưu trữ file | `media.Storage` | S3, R2… | MVP |
| Tìm kiếm | `search.Index` | PostgreSQL FTS → Meilisearch | MVP: SQL; Phase 2: đổi cài đặt |
| Gợi ý sản phẩm | `recommendation.Engine` | Quy tắc → mô hình học máy | P14 |
| Hàng đợi / job | `platform/jobs` | Tiến trình worker → broker nếu cần | MVP |

**Kiểm chứng một WRAP có đúng không:** nếu đổi nhà cung cấp mà phải sửa bất
kỳ file nào trong `domain/` hoặc `application/`, thì abstraction đó sai —
nó đang rò rỉ khái niệm của nhà cung cấp ra ngoài.

**Cảnh báo về trừu tượng hóa sớm:** WRAP một năng lực **chưa dùng** là vi
phạm P15. Chỉ wrap khi đã có ít nhất một cài đặt thật đang chạy.

---

## 5. BUILD — tự xây

### Tiêu chí

```text
✓ Là DOMAIN CHIẾN LƯỢC — xem mục 1.1
✓ Hoặc: OSS có nhưng ép domain theo hình dạng của nó
✓ Hoặc: nhu cầu quá đặc thù, phần dùng được của OSS quá nhỏ
```

### Danh sách BUILD

| Năng lực | Vì sao tự xây | Trạng thái |
|---|---|---|
| **Mô hình `Offer`** | Không OSS nào tách Offer khỏi Product theo cách cần cho so sánh giá nhiều seller | **Đã cài** |
| **Tách `Order`/`FulfillmentOrder`** | Ranh giới bảo mật seller nằm trong cấu trúc dữ liệu — xem [ADR-0007](../adr/0007-marketplace-order-model.md) | **Đã cài** |
| **Sổ cái bút toán kép** | Giữ tiền hộ nhiều bên, cần đối soát được — xem [ADR-0008](../adr/0008-financial-ledger.md) | **Đã cài** |
| **Chống hàng giả (ủy quyền thương hiệu)** | Không OSS nào có; là điều kiện để thương hiệu chịu lên sàn | **Đã cài** |
| **Buy box có công thức công khai** | Mô hình hộp đen tạo tranh chấp không giải quyết được | **Đã cài** |
| **Đóng băng giá và tỷ lệ hoa hồng** | Nguyên tắc P9; đối soát phải tái dựng được | **Đã cài** |
| **Attribution creator** | Là động cơ tạo nhu cầu, không thể thuê ngoài | Phase 2 |
| **Demand signal → planning** | Dữ liệu lịch sử không tạo ngược được | Ghi từ MVP |
| **Chuỗi cung ứng own brand** | Là lợi thế cạnh tranh dài hạn (vision) | Phase 3 |
| `kernel/ids` (ULID có tiền tố) | Phần dùng được của thư viện quá nhỏ so với ràng buộc riêng | **Đã cài** |
| `kernel/money` | Tiền là thứ không được phép sai; muốn kiểm soát hoàn toàn | **Đã cài** |
| `cmd/archcheck` | Ranh giới module là quy ước riêng của dự án này | **Đã cài** |

---

## 6. REFERENCE — chỉ nghiên cứu

### Tiêu chí

```text
✓ Giải cùng lớp bài toán, giúp hiểu vấn đề
✗ Không lấy code, không lấy mẫu thiết kế cụ thể
✗ Có thể vì license, vì khác ngăn xếp công nghệ, hoặc vì quy mô khác hẳn
```

### Danh sách REFERENCE

| Nguồn | Học được gì | Vì sao không lấy gì cụ thể |
|---|---|---|
| [Magento](magento.md) | Multi-Source Inventory, service contract | OSL-3.0 — không sao chép code được |
| [Shein model](shein-model.md) | Chu kỳ thiết kế→sản xuất ngắn, dữ liệu dẫn dắt | Mô hình kinh doanh, không phải phần mềm |
| [Creator commerce](creator-commerce.md) | Mô hình attribution, cách chia hoa hồng | Nghiên cứu ngành, không phải một dự án |
| [Saleor](saleor.md) | Tách thuộc tính chung/theo ngữ cảnh | Python + GraphQL, khác ngăn xếp |
| [Sylius](sylius.md) | Nhiều state machine tách biệt | PHP; ý tưởng đã diễn đạt lại bằng Go thuần |

**Lưu ý pháp lý:** đọc code GPLv3/OSL-3.0 để hiểu ý tưởng là hợp pháp; sao
chép cấu trúc lớp thì không. Xem [mục 10](#10-quy-tắc-sao-chép-code).

---

## 7. REJECT — cố ý không làm

### Nguyên tắc

Nguyên tắc P15: **mỗi thứ đưa vào phải giải thích được vì sao cần cho *chính* nghiệp vụ này.**

Không đưa vào vì "phổ biến", "hiện đại", hay "có thể cần sau này".

### Danh sách REJECT

| Thứ bị từ chối | Nguồn | Lý do |
|---|---|---|
| **GORM** | GoShop, QOR | Model ORM trở thành model domain → vi phạm R2, khóa ngoại vượt module |
| **Plugin system** | Vendure, Shopware, Magento | Chi phí chỉ đáng khi bán framework; ta xây cho chính mình |
| **GraphQL** | Saleor | Phân quyền theo trường quá phức tạp cho ràng buộc seller-không-thấy-seller |
| **Admin sinh tự động** | QOR, Sylius | Vi phạm API First; màn hình của ta là hỗ trợ ra quyết định, không phải CRUD |
| **Rule engine tổng quát** | Shopware | Trừu tượng hóa sớm; khó gỡ lỗi khi không khớp |
| **Workflow engine** | Medusa | Chưa tương xứng chi phí; bù trừ thụ động đơn giản hơn |
| **Khóa phân tán** | Digota | Không cần ở monolith một database; tạo hàng đợi tuần tự |
| **Channel-per-seller làm marketplace** | Vendure, Shopware | Không so sánh giá được, không gộp đơn nhiều seller được |
| **Nhiều loại sản phẩm** | Magento | Một mô hình duy nhất đủ cho thời trang |
| **DAL / tầng truy vấn riêng** | Shopware | Truy vấn thương mại phức tạp cần SQL thật |
| **gRPC song song REST** | GoShop, Digota | Không client nào cần; gấp đôi công duy trì |
| **MongoDB** | Digota | Cần giao dịch ACID cho tài chính |
| **`big.Float` cho tiền** | Flamingo | Số nguyên đơn giản hơn khi lưu trữ, hiệu năng ổn định hơn |
| **Microservices từ đầu** | Nhiều dự án | Xem [ADR-0001](../adr/0001-modular-monolith.md) |
| **Message broker (Kafka/NATS)** | Nhiều dự án | Một tiến trình, một database — outbox trong PostgreSQL đủ. Thêm khi có nhu cầu đo được, không phải vì "event-driven" |

---

## 8. Ba thay đổi thiết kế từ nghiên cứu này

Đây là kết quả cụ thể nhất — nghiên cứu **không chỉ xác nhận** mà **thay đổi** thiết kế:

### 5.1 Link table (từ Medusa)

**Khoảng trống:** tài liệu nói "không khóa ngoại vượt module" nhưng không nói cách mô hình hóa quan hệ nhiều-nhiều vượt module.

**Bổ sung:** khái niệm link table với ba quy tắc — xem [medusa.md](medusa.md).

**Cập nhật:** [05-data/data-model.md](../05-data/data-model.md).

### 5.2 Adjustment (từ Sylius)

**Khoảng trống:** [07-workflows/return.md](../07-workflows/return.md) nói "giảm giá phải phân bổ xuống dòng hàng và lưu lại" nhưng không nói lưu dưới dạng gì.

**Bổ sung:** `Adjustment` là thực thể hạng nhất, có `cost_bearer`.

**Cập nhật:** [02-domain/entities.md](../02-domain/entities.md), [04-modules/order.md](../04-modules/order.md).

### 5.3 Cơ sở tính hoa hồng creator (từ creator commerce + Sylius)

**Khoảng trống:** hoa hồng là "% giá trị đơn" nhưng không định nghĩa "giá trị đơn" khi có nhiều loại giảm giá.

**Bổ sung:** cơ sở = giá niêm yết − adjustment do **seller** chịu; không trừ adjustment do nền tảng chịu.

**Cập nhật:** [01-business/monetization.md](../01-business/monetization.md), [04-modules/affiliate.md](../04-modules/affiliate.md).

---

## 9. Quy trình xét một thư viện mới

Áp dụng cho mọi phụ thuộc trong tương lai:

```text
Bước 1 — Rào cản pháp lý và bảo trì (xét TRƯỚC khi đọc code)
    License có cho phép dùng thương mại?
        Không / không có license → DỪNG
    Cập nhật gần đây không? (> 2 năm = rủi ro cao)
    Có cộng đồng không?

Bước 2 — Phù hợp kiến trúc
    Có ép domain theo hình dạng của nó không?
    Có kéo theo phụ thuộc nặng không?
    Có buộc domain layer phụ thuộc hạ tầng không?  ← chặn GORM

Bước 3 — Có thật sự cần không?
    Vấn đề cụ thể là gì?
    Tự viết mất bao lâu?
    Nếu thư viện ngừng bảo trì thì sao?

Bước 4 — Ghi vào dependency-registry.md
```

Bước 1 loại được go-saas/commerce trước khi tốn thời gian đọc code — xem [go-saas-commerce.md](go-saas-commerce.md).

---

## 10. Quy tắc sao chép code

```text
MIT / BSD-3 / Apache-2.0
    ✓ Được sao chép
    ✓ PHẢI giữ thông báo bản quyền gốc
    ✓ Ghi nguồn trong comment của file

GPLv3 / OSL-3.0 / AGPL
    ✗ KHÔNG sao chép code
    ✗ KHÔNG sao chép cấu trúc lớp trực tiếp
    ✓ ĐƯỢC đọc, hiểu ý tưởng, mô tả bằng lời, tự cài đặt từ đầu

Không có license
    ✗ CẤM mọi hình thức sử dụng
```

**Ranh giới pháp lý:** ý tưởng và kiến trúc không bị bảo hộ bản quyền; chỉ mã nguồn cụ thể mới bị. Học "reservation ngăn oversell" từ Magento là hợp pháp; sao chép lớp của họ thì không.

---

## 11. Tài liệu liên quan

- [research-matrix.md](research-matrix.md) — so sánh chi tiết
- [dependency-registry.md](dependency-registry.md) — danh sách thư viện
- [synthesis.md](synthesis.md) — kiến trúc tổng hợp
