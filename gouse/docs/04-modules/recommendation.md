# Module: Recommendation

| | |
|---|---|
| **Bounded Context** | Growth |
| **Phân loại** | Supporting (có thể thành Core) |
| **Giai đoạn** | Phase 2 |

---

## 1. Trách nhiệm

- Gợi ý sản phẩm, nội dung, outfit
- Xếp hạng kết quả khám phá
- Cá nhân hóa trải nghiệm

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Dữ liệu sản phẩm | `product`, `catalog` |
| Nội dung | `content` |
| Hạ tầng đánh chỉ mục tìm kiếm | Platform |
| Vị trí được tài trợ | `marketplace` (retail media) |

---

## 3. Nguyên tắc kiến trúc quan trọng nhất

> **Module thương mại KHÔNG BAO GIỜ phụ thuộc trực tiếp vào máy học.**

```text
        Recommendation Interface
                 ↓
    ┌────────────┼────────────┬─────────────┐
    ▼            ▼            ▼             ▼
Rule-based   Behavioral    ML model    Hybrid
(MVP)        (Phase 3)     (Phase 4)   (tương lai)
```

Module gọi (`product`, `content`, `cart`) chỉ biết interface:

```go
type RecommendationEngine interface {
    GetSimilarProducts(ctx, productID string, limit int) ([]ProductRef, error)
    GetPersonalizedProducts(ctx, customerID string, ctx RecContext, limit int) ([]ProductRef, error)
    GetCompleteTheLook(ctx, productID string, limit int) ([]ProductRef, error)
    GetTrendingProducts(ctx, categoryID string, limit int) ([]ProductRef, error)
    GetRelatedContent(ctx, productID string, limit int) ([]ContentRef, error)
}
```

**Vì sao bắt buộc:**

```text
Nếu module cart gọi thẳng hệ thống ML:
    - Không chạy được khi chưa có dữ liệu huấn luyện
    - ML lỗi → giỏ hàng lỗi
    - Đổi cách gợi ý = sửa module cart
    - Không kiểm thử được cart mà không có ML

Nếu qua interface:
    - MVP dùng quy tắc đơn giản, vẫn chạy
    - ML lỗi → trả về kết quả mặc định, giỏ hàng vẫn hoạt động
    - Nâng cấp = thay cài đặt, không sửa bên gọi
```

Đây là nguyên tắc P13 và P14.

---

## 4. Cài đặt theo giai đoạn

### MVP/Phase 2 — quy tắc đơn giản

```text
Sản phẩm tương tự:
    cùng danh mục + cùng khoảng giá + còn hàng
    sắp xếp theo doanh số gần đây

Hoàn thiện bộ (complete the look):
    lấy từ Outfit có chứa sản phẩm này
    → tận dụng dữ liệu do stylist tạo, không cần thuật toán

Xu hướng:
    doanh số 7 ngày gần nhất, có trọng số theo thời gian

Cá nhân hóa:
    dựa trên danh mục và thương hiệu khách đã mua/xem
```

**Nhận xét:** "Complete the look" dùng dữ liệu `Outfit` là ví dụ tốt — chất lượng cao mà không cần máy học, vì stylist đã phối đồ thủ công.

### Phase 3 — dựa trên hành vi

```text
Lọc cộng tác đơn giản:
    "khách mua sản phẩm này cũng mua..."
    "khách xem sản phẩm này cũng xem..."
```

### Phase 4 — mô hình học máy

```text
Nếu triển khai: chạy như dịch vụ riêng, gọi qua cùng interface
→ có thể dùng ngôn ngữ/hạ tầng khác
→ là ứng viên tách service hợp lệ (lý do: chuyên biệt công nghệ)
```

---

## 5. Cơ chế dự phòng — bắt buộc

```text
Mọi lời gọi gợi ý phải có phương án dự phòng:

    Engine trả về lỗi hoặc quá chậm
        → trả về kết quả mặc định (bán chạy trong danh mục)
        → KHÔNG để trang lỗi

    Timeout ngắn (ví dụ 200ms)
        → gợi ý không đáng để làm chậm trang
```

**Nguyên tắc:** gợi ý là tính năng **tăng cường**, không phải tính năng thiết yếu. Hỏng gợi ý không được làm hỏng việc bán hàng.

---

## 6. Đặc thù thời trang cần lưu ý

| Yếu tố | Ảnh hưởng tới gợi ý |
|---|---|
| Còn size của khách không | Gợi ý sản phẩm hết size khách mặc là vô ích |
| Mùa vụ | Không gợi ý áo khoác dạ giữa mùa hè |
| Phong cách | Khách mặc phong cách tối giản không muốn đồ họa tiết rực rỡ |
| Đã mua rồi | Không gợi ý lại đúng món vừa mua |

Yếu tố đầu tiên quan trọng nhất và thường bị bỏ qua: gợi ý phải **lọc theo size khả dụng** cho khách đã biết size.

---

## 7. Dữ liệu sở hữu

```sql
recommendation_rule
user_affinity           -- mức độ quan tâm của khách với danh mục/thương hiệu
product_similarity      -- bảng tính sẵn độ tương tự
recommendation_log      -- ghi nhận gợi ý đã hiển thị (để đo hiệu quả)
```

`recommendation_log` cần cho việc đo lường: gợi ý nào được click, dẫn tới mua hàng không. Không có nó thì không cải thiện được.

---

## 8. Interface công khai

Xem mục 3.

---

## 9. Event

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `order.placed` | order | Cập nhật độ tương tự, mức quan tâm |
| `cart.item_added` | cart | Tín hiệu quan tâm |
| `content.viewed` | content | Tín hiệu quan tâm |
| `product.published` | product | Đưa vào tập gợi ý |
| `inventory.depleted` | inventory | Loại khỏi gợi ý |

**Phát ra:** `recommendation.served` (cho analytics)

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Module thương mại không phụ thuộc trực tiếp ML |
| 2 | Luôn có phương án dự phòng |
| 3 | Timeout ngắn, không làm chậm trang |
| 4 | Chỉ gợi ý sản phẩm còn hàng |
| 5 | Ưu tiên sản phẩm còn size khách mặc |
| 6 | Không gợi ý sản phẩm khách vừa mua |
| 7 | Ghi log gợi ý để đo hiệu quả |

---

## 11. Tài liệu liên quan

- [content.md](content.md) — outfit làm nguồn "complete the look"
- [../01-business/content-commerce.md](../01-business/content-commerce.md) mục 7
- [../03-architecture/evolution-to-services.md](../03-architecture/evolution-to-services.md)
