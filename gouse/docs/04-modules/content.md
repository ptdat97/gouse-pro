# Module: Content

| | |
|---|---|
| **Bounded Context** | Growth |
| **Phân loại** | **Core** |
| **Giai đoạn** | Phase 2 |

---

## 1. Trách nhiệm

- Quản lý nội dung: video, ảnh, lookbook, bài viết, outfit, livestream
- Gắn sản phẩm vào nội dung (product tagging)
- Quản lý `Outfit` — bộ phối đồ
- Kiểm duyệt nội dung
- Quản lý đánh giá sản phẩm của khách

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Hồ sơ creator | `creator` |
| Link affiliate, quy kết | `affiliate` |
| Chiến dịch | `campaign` |
| Xếp hạng, gợi ý nội dung | `recommendation` |
| Lưu trữ file media | Platform (object storage) |

**Ràng buộc thiết kế quan trọng:** `content` **không phụ thuộc** `creator`. Trường `creator_id` có thể null — nội dung do nền tảng tự sản xuất không có creator.

Nếu `content` bắt buộc phải có creator, nền tảng không đăng được nội dung của chính mình.

---

## 3. Outfit — đơn vị nội dung đặc thù thời trang

```text
Outfit "Đi làm mùa thu"
├── OutfitItem: Áo sơ mi linen  (MAIN)
├── OutfitItem: Quần âu          (MAIN)
├── OutfitItem: Giày loafer      (COMPLEMENT)
└── OutfitItem: Túi tote         (ACCESSORY)
```

### Vì sao là entity riêng, không phải "bài viết có nhiều link"

| Nếu là bài viết có link | Nếu Outfit là entity |
|---|---|
| Không tính được giá cả bộ | Hiển thị tổng giá, mua cả bộ một lần |
| Không gợi ý thay thế khi hết hàng | Có `substitute_product_ids` |
| Không đo được hiệu quả bộ | Đo tỷ lệ mua ≥ 2 món |
| Không tái sử dụng | Một outfit xuất hiện ở nhiều nơi |

**Giá trị kinh doanh:** outfit tăng giá trị đơn hàng trung bình — khách vào định mua một chiếc áo, mua cả bộ.

### Xử lý sản phẩm hết hàng trong outfit

Nội dung sống lâu hơn sản phẩm. Một video hay được xem nhiều tháng, khi sản phẩm gốc đã hết.

```text
Sản phẩm hết hàng tạm thời  → "Tạm hết hàng", cho phép nhận thông báo
Sản phẩm ngừng bán vĩnh viễn → hiển thị substitute_product_ids
```

**Không được để nội dung dẫn tới trang lỗi** — đó là lãng phí toàn bộ công sức tạo nội dung và tổn hại trải nghiệm.

---

## 4. Product Tagging

```text
ProductTag {
    content_id
    product_id
    offer_id        (nullable — có thể tag sản phẩm chung, không chỉ định seller)
    position_x      -- vị trí trên ảnh
    position_y
    timestamp_second (nullable) -- vị trí trong video
}
```

Yêu cầu:
- Một nội dung tham chiếu **nhiều** sản phẩm
- Một sản phẩm xuất hiện trong **nhiều** nội dung
- Tag có vị trí không gian (ảnh) hoặc thời gian (video)

---

## 5. Kiểm duyệt nội dung

```text
    DRAFT
      ↓
    PENDING_REVIEW
      │
      ├── Tự động: sản phẩm hợp lệ, từ ngữ cấm, bản quyền
      ├── Thủ công: creator mới hoặc nội dung bị gắn cờ
      │
      ├──→ REJECTED (có lý do cụ thể)
      ↓
    PUBLISHED
      │
      ├──→ TAKEN_DOWN (phát hiện vi phạm sau)
      └──→ ARCHIVED
```

### Công bố tài trợ — yêu cầu pháp lý

```text
Nội dung thuộc chiến dịch có trả phí
    → hệ thống TỰ ĐỘNG gắn nhãn "Được tài trợ"
    → KHÔNG phụ thuộc vào việc creator có nhớ ghi hay không
```

Đây là nghĩa vụ pháp lý của nền tảng ở nhiều thị trường, không phải tính năng tùy chọn.

```text
Content {
    campaign_id     (nullable)
    is_sponsored    -- tự động = true nếu campaign có trả phí
}
```

---

## 6. Dữ liệu sở hữu

```sql
content
content_media
product_tag
outfit
outfit_item
review                  -- đánh giá sản phẩm của khách
review_media
content_moderation_log
```

**Về `review`:** đánh giá sản phẩm được đặt ở module này vì nó cũng là nội dung do người dùng tạo, cần kiểm duyệt, và có thể chứa ảnh. Gộp vào đây tránh tạo module riêng cho một khái niệm nhỏ.

**Quy tắc đánh giá:** chỉ khách **đã mua và nhận hàng** mới được đánh giá sản phẩm đó. Kiểm tra qua `order.GetOrderLineForReturn()` hoặc interface tương đương.

---

## 7. Interface công khai

```go
type PublicAPI interface {
    GetContent(ctx, contentID string) (*ContentView, error)
    GetContentByProduct(ctx, productID string, page Pagination) (*ContentList, error)
    GetContentByCreator(ctx, creatorID string, page Pagination) (*ContentList, error)

    GetOutfit(ctx, outfitID string) (*OutfitView, error)
    GetOutfitsByProduct(ctx, productID string) ([]OutfitView, error)

    GetProductTags(ctx, contentID string) ([]ProductTagView, error)

    GetReviews(ctx, productID string, page Pagination) (*ReviewList, error)
    GetReviewSummary(ctx, productIDs []string) (map[string]ReviewSummary, error)

    PublishContent(ctx, req PublishRequest) (*ContentView, error)
    TakeDownContent(ctx, contentID string, reason string) error
}
```

---

## 8. Event

**Phát ra:**

| Event | Bên nghe |
|---|---|
| `content.published` | recommendation, notification, analytics |
| `content.taken_down` | recommendation, affiliate |
| `content.viewed` | analytics, **supply-chain (tín hiệu nhu cầu)** |
| `outfit.created` | recommendation |
| `review.published` | product (cập nhật điểm đánh giá), seller |

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `product.unpublished` | product | Đánh dấu tag không còn hiệu lực, hiển thị thay thế |
| `inventory.depleted` | inventory | Hiển thị "tạm hết hàng" trên tag |
| `campaign.started` | campaign | Gắn nhãn tài trợ |
| `return.inspected` | return | Theo dõi tỷ lệ hoàn theo nội dung |

Event cuối là cơ chế kiểm soát chất lượng: nội dung có tỷ lệ hoàn hàng cao bất thường là dấu hiệu mô tả gây hiểu nhầm.

---

## 9. Phụ thuộc

```text
Gọi đồng bộ:   product  (thông tin sản phẩm để hiển thị tag)
               catalog  (thương hiệu, bộ sưu tập)
               creator  (thông tin creator — nullable)
               campaign (kiểm tra chiến dịch)
Nghe event:    product, inventory, campaign, return
Được gọi bởi:  affiliate, recommendation
```

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | `content` không bắt buộc có creator |
| 2 | Nội dung tài trợ tự động gắn nhãn |
| 3 | Outfit phải có ít nhất 2 sản phẩm |
| 4 | Nội dung không được dẫn tới trang lỗi khi sản phẩm hết |
| 5 | Chỉ khách đã mua mới được đánh giá |
| 6 | Nội dung PUBLISHED phải có ít nhất một media |
| 7 | Mọi quyết định kiểm duyệt phải ghi lý do |

---

## 11. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **Phase 2** | Nội dung cơ bản, product tag, outfit, đánh giá, kiểm duyệt |
| **Phase 3** | Lookbook, nội dung theo bộ sưu tập, gợi ý thay thế |
| **Phase 4** | Live commerce, nội dung tương tác |

---

## 12. Tài liệu liên quan

- [../01-business/content-commerce.md](../01-business/content-commerce.md) — nghiệp vụ
- [creator.md](creator.md) — người tạo nội dung
- [affiliate.md](affiliate.md) — quy kết doanh thu
- [../07-workflows/content-commerce.md](../07-workflows/content-commerce.md)
