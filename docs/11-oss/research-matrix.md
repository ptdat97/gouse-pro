# Ma trận nghiên cứu

So sánh 40 năng lực trên 10 dự án OSS, đối chiếu với yêu cầu nghiệp vụ của chúng ta.

## Ký hiệu

```text
●●●  Mạnh — mô hình đầy đủ, đáng học
●●   Có, ở mức cơ bản
●    Yếu hoặc chỉ là phần phụ
—    Không có
```

Cột **Quyết định** dùng ba giá trị theo [adoption-policy.md](adoption-policy.md):

```text
ADOPT   Lấy gần như nguyên vẹn
ADAPT   Lấy ý tưởng, thiết kế lại cho domain của chúng ta
BUILD   Tự xây — không OSS nào phù hợp, hoặc đây là lợi thế cạnh tranh
```

---

## 1. Danh mục sản phẩm

| Năng lực | Flamingo | QOR | Digota | GoShop | Medusa | Vendure | Saleor | Shopware | Sylius | Magento | Quyết định |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Product | ●●● | ●● | ●● | ●● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | **ADAPT** |
| Variant | ●●● | ● | ●● | ● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | **ADAPT** |
| SKU | ●● | ● | ●●● | ● | ●● | ●●● | ●●● | ●● | ●● | ●●● | **ADAPT** |
| Catalog / Category | ●●● | ●● | ● | ●● | ●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |
| Brand | ● | ● | — | ● | ●● | ●● | ●● | ●● | ●● | ●● | **BUILD** |
| Collection / mùa vụ | — | ● | — | — | ●● | ●● | ●● | ●● | ● | ●● | **BUILD** |
| Bảng size / số đo | — | — | — | — | — | — | — | — | — | ● | **BUILD** |

**Nhận xét:** không dự án nào mô hình hóa **bảng size có số đo thực tế** hay **bộ sưu tập gắn mùa vụ** — hai thứ quyết định tỷ lệ hoàn hàng và rủi ro tồn kho của thời trang. Đây là lý do `Collection` và `SizeChart` phải tự xây.

---

## 2. Giá và khuyến mãi

| Năng lực | Flamingo | QOR | Digota | GoShop | Medusa | Vendure | Saleor | Shopware | Sylius | Magento | Quyết định |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Money / đơn vị nhỏ nhất | ●●● | ● | ●●● | ● | ●● | ●● | ●● | ●● | ●● | ●● | **ADOPT** |
| Price value object | ●●● | ● | ●● | — | ●● | ●● | ●●● | ●● | ●● | ●● | **ADOPT** |
| Chia tiền không mất đồng | ●●● | — | — | — | ● | ● | ● | ● | ● | ● | **ADOPT** |
| Charge / phân loại khoản tiền | ●●● | — | — | — | ● | ● | ● | ●● | ●●● | ●● | **ADOPT** |
| Adjustment trên dòng hàng | ● | — | ●● | — | ●● | ●● | ●● | ●● | ●●● | ●●● | **ADOPT** |
| Thuế / VAT | ●●● | ● | ●● | — | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |
| Promotion / coupon | ● | ● | ●● | ● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |
| Rule engine cho khuyến mãi | — | — | — | — | ●● | ●● | ●● | ●●● | ●● | ●● | ADAPT |
| Giá theo kênh/thị trường | ● | — | — | — | ●●● | ●●● | ●●● | ●●● | ●● | ●● | ADAPT |

**Phát hiện quan trọng:** Flamingo `SplitInPayables` và Sylius `Adjustment` là hai mẫu đáng lấy nhất trong toàn bộ nghiên cứu. Xem [flamingo-commerce.md](flamingo-commerce.md) và [sylius.md](sylius.md).

---

## 3. Giỏ hàng, thanh toán, đơn hàng

| Năng lực | Flamingo | QOR | Digota | GoShop | Medusa | Vendure | Saleor | Shopware | Sylius | Magento | Quyết định |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Cart | ●●● | ● | ●● | ●● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |
| Cart nhiều điểm giao | ●●● | — | — | — | ●● | ● | ● | ● | ●● | ●● | **ADOPT** |
| Checkout | ●●● | ● | ● | ● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |
| Giữ tồn kho khi checkout | ● | — | ●●● | — | ●● | ●● | ●● | ●● | ●● | ●●● | **ADOPT** |
| Order | ●●● | ●● | ●●● | ●● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |
| Đơn nhiều nhà bán | ● | — | ●● | — | ●● | ●● | ● | ● | ● | ●● | **BUILD** |
| Giao/hủy/hoàn từng phần | ●● | — | ● | — | ●●● | ●●● | ●● | ●● | ●●● | ●●● | ADAPT |
| Payment | ●●● | ● | ●●● | ●● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |
| Nhiều giao dịch/đơn | ●●● | — | ●● | — | ●● | ●● | ●● | ●● | ●●● | ●● | **ADOPT** |
| Refund / adjustment | ●● | — | ●●● | — | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |

---

## 4. Tồn kho và giao hàng

| Năng lực | Flamingo | QOR | Digota | GoShop | Medusa | Vendure | Saleor | Shopware | Sylius | Magento | Quyết định |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Inventory cơ bản | ● | ● | ●●● | ● | ●●● | ●●● | ●●● | ●●● | ●● | ●●● | ADAPT |
| Nhiều kho / nguồn hàng | — | — | ● | — | ●●● | ●●● | ●● | ●● | ● | ●●● | **ADOPT** |
| Reservation chống oversell | — | — | ●● | — | ●● | ●● | ●● | ●● | ● | ●●● | **ADOPT** |
| Tách tồn vật lý / khả bán | — | — | ● | — | ●● | ●● | ●● | ●● | ● | ●●● | **ADOPT** |
| Khóa đồng thời | — | — | ●●● | — | ● | ● | ● | ● | ● | ●● | **ADAPT** |
| Fulfillment | ●● | — | ● | — | ●●● | ●●● | ●● | ●● | ●●● | ●●● | ADAPT |
| Nhiều lô giao / đơn | ●● | — | — | — | ●●● | ●● | ● | ● | ●●● | ●● | **ADOPT** |
| Return / RMA | ● | — | ● | — | ●●● | ●●● | ●● | ●● | ●● | ●●● | ADAPT |
| Lý do hoàn chuẩn hóa | — | — | — | — | ● | ● | ● | ● | ● | ● | **BUILD** |

**Phát hiện:** Magento MSI là mô hình tồn kho mạnh nhất trong nhóm — tách **tồn vật lý** khỏi **số lượng khả bán**, dùng reservation. Nhưng license OSL-3.0 nghĩa là **chỉ học ý tưởng, không sao chép code**.

Digota có khóa phân tán (Zookeeper/Redis) — nhưng dự án ngừng cập nhật từ 2021 và chúng ta không cần khóa phân tán ở monolith.

---

## 5. Marketplace

| Năng lực | Flamingo | QOR | Digota | GoShop | Medusa | Vendure | Saleor | Shopware | Sylius | Magento | Quyết định |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Seller / Vendor | — | ● | ●● | — | ●● | ●●● | ● | ● | ● | ●● | **BUILD** |
| Seller Store | — | ● | ● | — | ● | ●● | ● | ●● | ● | ● | **BUILD** |
| **Offer (nhiều bán/1 SKU)** | — | — | — | — | ● | ●● | ● | — | — | ●● | **BUILD** |
| Commission | — | — | ● | — | ● | ● | — | — | — | ● | **BUILD** |
| Settlement / payout | — | — | — | — | — | ● | — | — | — | ● | **BUILD** |
| Seller performance | — | — | — | — | — | — | — | — | — | ● | **BUILD** |
| Đa kênh / đa gian hàng | ● | — | — | — | ●●● | ●●● | ●●● | ●●● | ●● | ●● | **ADAPT** |

**Đây là khoảng trống lớn nhất của toàn bộ hệ sinh thái OSS.**

Không dự án nào mô hình hóa marketplace thật sự: nhiều nhà bán cạnh tranh trên **cùng một SKU**, có hoa hồng, đối soát, và chấm điểm hiệu suất. Vendure và Medusa hỗ trợ marketplace qua **Channel/Sales Channel** — nhưng đó là mô hình "mỗi seller một gian hàng riêng", không phải "nhiều seller cùng bán một mã hàng".

Phân tích chi tiết sự khác biệt: [vendure.md](vendure.md) mục "Channel vs Offer".

---

## 6. Creator commerce và nội dung

| Năng lực | Flamingo | QOR | Digota | GoShop | Medusa | Vendure | Saleor | Shopware | Sylius | Magento | Quyết định |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Creator | — | — | — | — | — | — | — | — | — | — | **BUILD** |
| Affiliate / attribution | — | — | — | — | — | — | — | — | — | ● | **BUILD** |
| Content (video/lookbook) | — | ●● | — | — | — | — | — | ●● | — | ●● | **BUILD** |
| Product tag trong nội dung | — | — | — | — | — | — | — | — | — | — | **BUILD** |
| Outfit / phối đồ | — | — | — | — | — | — | — | — | — | — | **BUILD** |
| Campaign | — | ● | — | — | ● | ● | ● | ●● | ● | ●● | ADAPT |
| Recommendation | — | — | — | — | ● | ● | ● | ●● | ● | ●● | ADAPT |

**Toàn bộ nhóm này gần như trống trong OSS thương mại.**

Creator commerce là năng lực của các nền tảng đóng (TikTok Shop, SHEIN, Instagram Shopping). Không có OSS nào để học cách cài đặt — chỉ có thể học **mô hình nghiệp vụ** từ tài liệu công khai của họ.

Xem [creator-commerce.md](creator-commerce.md).

---

## 7. Chuỗi cung ứng

| Năng lực | Flamingo | QOR | Digota | GoShop | Medusa | Vendure | Saleor | Shopware | Sylius | Magento | Quyết định |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Supplier | — | ● | — | — | — | — | — | ● | — | ● | **BUILD** |
| Procurement / PO | — | ● | — | — | — | — | — | ● | — | ● | **BUILD** |
| Manufacturing | — | — | — | — | — | — | — | — | — | — | **BUILD** |
| Production batch | — | — | — | — | — | — | — | — | — | — | **BUILD** |
| Quality control | — | — | — | — | — | — | — | — | — | — | **BUILD** |
| Warehouse | — | — | — | — | ●● | ●● | ● | ●● | — | ●● | ADAPT |
| Demand planning | — | — | — | — | — | — | — | — | — | — | **BUILD** |
| Replenishment | — | — | — | — | — | — | — | — | — | ● | **BUILD** |
| Giá vốn theo lô | — | — | — | — | — | — | — | — | — | ● | **BUILD** |

**Khoảng trống lớn thứ hai.** OSS thương mại điện tử dừng ở "nhập hàng vào kho" — không có gì về sản xuất, kiểm định, hay lập kế hoạch theo nhu cầu.

Đây chính là lý do chuỗi cung ứng được phân loại **Core Domain** trong [02-domain/domain-map.md](../02-domain/domain-map.md): nó là lợi thế không sao chép được.

---

## 8. Kiến trúc và hạ tầng

| Năng lực | Flamingo | QOR | Digota | GoShop | Medusa | Vendure | Saleor | Shopware | Sylius | Magento | Quyết định |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Ports & Adapters | ●●● | ● | ●● | ●● | ●● | ●● | ●● | ●● | ●●● | ●● | **ADOPT** |
| Ranh giới module cưỡng chế | ●● | — | ● | ● | ●●● | ●● | ● | ●● | ●● | ●● | **ADOPT** |
| Event nội bộ | ●● | ● | ● | — | ●●● | ●●● | ●● | ●●● | ●●● | ●●● | ADAPT |
| Workflow / bù trừ | — | ●● | — | — | ●●● | ● | ● | ●● | ●● | ● | **ADAPT** |
| State machine | — | ●●● | — | — | ●● | ●● | ● | ●● | ●●● | ●● | **ADOPT** |
| API-first / OpenAPI | ●● | ● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ●● | ●● | ADAPT |
| Admin | ● | ●●● | — | ● | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● | ADAPT |
| Media library | — | ●●● | — | — | ●● | ●●● | ●●● | ●●● | ●● | ●●● | **ADAPT** |
| Publish / draft + lịch | — | ●●● | — | — | ● | ●● | ●●● | ●●● | ● | ●●● | **ADAPT** |
| Search | ●●● | ● | — | ● | ●● | ●●● | ●●● | ●●● | ●● | ●●● | ADAPT |
| Background job | — | ●● | — | — | ●●● | ●●● | ●● | ●●● | ●● | ●● | ADAPT |
| Testing (fake adapter) | ●●● | ● | ●● | ●●● | ●● | ●●● | ●●● | ●● | ●●● | ●● | **ADOPT** |
| Observability | ●● | — | ● | ● | ●● | ●● | ●●● | ●● | ● | ●● | ADAPT |
| Extensibility / plugin | ●●● | ●● | ● | — | ●●● | ●●● | ●● | ●●● | ●●● | ●●● | **REJECT** |

**Về extensibility:** mọi nền tảng lớn đều có hệ thống plugin. Chúng ta **cố ý không làm** — plugin system là chi phí kiến trúc rất lớn và chỉ có giá trị khi cần hệ sinh thái bên thứ ba. Chúng ta xây nền tảng cho **chính mình**, không bán framework.

---

## 9. Tổng hợp quyết định

| Quyết định | Số năng lực | Ý nghĩa |
|---|---|---|
| **ADOPT** | 16 | Mẫu đã kiểm chứng, lấy gần như nguyên vẹn |
| **ADAPT** | 31 | Lấy ý tưởng, thiết kế lại cho domain thời trang/marketplace |
| **BUILD** | 24 | Tự xây — OSS không có, hoặc đây là lợi thế cạnh tranh |
| **REJECT** | 1 | Cố ý không làm (plugin system) |
| **Tổng** | **72** | |

### Phân bố BUILD theo nhóm

```text
Marketplace         6/7  năng lực phải tự xây
Creator commerce    5/7  năng lực phải tự xây
Chuỗi cung ứng      8/9  năng lực phải tự xây
Thời trang đặc thù  3/3  (bảng size, bộ sưu tập, lý do hoàn)
```

**Kết luận:** OSS giúp được nhiều ở **Commerce Core** (giá, giỏ, đơn, thanh toán, tồn kho) nhưng gần như không giúp gì ở ba miền tạo lợi thế cạnh tranh của chúng ta.

Điều này **xác nhận** phân loại core/supporting/generic đã có trong [02-domain/domain-map.md](../02-domain/domain-map.md).

---

## 10. Tài liệu liên quan

- [adoption-policy.md](adoption-policy.md) — quy tắc quyết định
- [synthesis.md](synthesis.md) — kiến trúc tổng hợp cuối cùng
- [dependency-registry.md](dependency-registry.md) — license và thư viện
