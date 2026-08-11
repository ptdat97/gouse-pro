# Module: Pricing

| | |
|---|---|
| **Bounded Context** | Commerce |
| **Phân loại** | Supporting (có thể thành Core nếu làm định giá động) |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý bảng giá và quy tắc giá
- Tính giá hiển thị cho một offer trong một ngữ cảnh
- Lưu lịch sử giá
- Quản lý giá theo bộ sưu tập, theo mùa

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Giá của offer marketplace | `marketplace` (seller tự đặt) |
| Mã giảm giá, khuyến mãi | `promotion` |
| Tính tiền đơn hàng | `checkout` |
| Giá vốn (COGS) | `manufacturing` |

---

## 3. Ranh giới với Marketplace

Đây là điểm dễ nhầm.

```text
marketplace.Offer.price     — giá seller ĐẶT RA (nguồn sự thật)
pricing                     — quy tắc và bảng giá của NỀN TẢNG (own brand)
                              + khung giá ràng buộc seller
```

**Với own brand:** `pricing` quyết định giá bán.

**Với seller marketplace:** seller tự đặt giá trong `Offer`. `pricing` chỉ cung cấp **khung giá ràng buộc**:

```text
Khung giá:
    - Giá tối thiểu (chống bán phá giá, chống lỗi nhập liệu)
    - Giá tối đa (chống thổi giá)
    - Cảnh báo giá bất thường so với thị trường
```

---

## 4. Các loại giá

```text
Base price           — giá gốc
Compare-at price     — giá gạch ngang để hiển thị mức giảm
Member price         — giá riêng cho thành viên
Campaign price       — giá trong chiến dịch, có thời hạn
Flash price          — giá trong phiên live, thời hạn rất ngắn
Clearance price      — giá xả hàng cuối mùa
```

### Thứ tự ưu tiên

```text
Flash > Campaign > Clearance > Member > Base
```

Chỉ **một** giá được áp dụng — không cộng dồn nhiều loại giá. Giảm giá thêm (mã giảm giá) thuộc `promotion` và áp dụng **sau** khi đã chọn giá.

---

## 5. Định giá theo mùa — đặc thù thời trang

```text
Bộ sưu tập ra mắt
    ↓  Giá gốc, mục tiêu bán giá đầy đủ
Tuần 1–4     — 100% giá
    ↓
Tuần 5–8     — nếu sell-through < mục tiêu → giảm 10–20%
    ↓
Tuần 9–12    — giảm 30–40%
    ↓
Cuối mùa     — xả hàng 50–70%
```

**Vì sao cần mô hình hóa:** hàng thời trang hết mùa mất giá rất nhanh. Giảm giá đúng lúc bán được nhiều hơn giữ giá rồi xả sâu vào phút chót.

Hệ thống cần **cảnh báo** khi sell-through thấp hơn mục tiêu để can thiệp kịp, không phải phát hiện khi đã cuối mùa.

**Nguyên tắc P14:** MVP dùng quy tắc theo lịch. Định giá động theo nhu cầu là Phase 4.

---

## 6. Dữ liệu sở hữu

```sql
price_list              -- bảng giá
price_rule              -- quy tắc giá
price_history           -- lịch sử thay đổi
price_constraint        -- khung giá ràng buộc seller
```

`price_history` cần cho: phát hiện thao túng giá (tăng rồi giảm giả vờ khuyến mãi), phân tích độ co giãn của cầu, và nghĩa vụ minh bạch giá ở một số thị trường.

---

## 7. Interface công khai

```go
type PublicAPI interface {
    GetPrice(ctx, req PriceRequest) (*PriceResult, error)
    GetPrices(ctx, reqs []PriceRequest) (map[string]PriceResult, error)

    GetPriceConstraint(ctx, skuID string) (*PriceConstraint, error)
    ValidateSellerPrice(ctx, skuID string, price Money) (bool, string, error)

    GetPriceHistory(ctx, skuID string, period DateRange) ([]PricePoint, error)
}

type PriceRequest struct {
    SKUID       string
    OfferID     string
    CustomerTier string   // để áp giá thành viên
    CampaignID  string
}
```

---

## 8. Event

**Phát ra:** `price.changed`, `price.markdown_suggested`

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `collection.ending` | catalog | Kích hoạt quy tắc giảm giá cuối mùa |
| `inventory.low_stock` | inventory | Cân nhắc ngừng khuyến mãi khi sắp hết |

---

## 9. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Giá > 0 |
| 2 | Chỉ một loại giá được áp dụng, không cộng dồn |
| 3 | Mọi thay đổi giá ghi vào lịch sử |
| 4 | Giá seller phải trong khung ràng buộc |
| 5 | Giá luôn kèm đơn vị tiền tệ |
| 6 | Giá được đóng băng khi vào checkout |

---

## 10. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Giá cơ bản, giá gạch ngang, khung giá |
| **Phase 2** | Giá theo chiến dịch, giá thành viên |
| **Phase 3** | Quy tắc giảm giá theo mùa, cảnh báo sell-through |
| **Phase 4** | Định giá động theo nhu cầu |

---

## 11. Tài liệu liên quan

- [marketplace.md](marketplace.md) — giá của seller
- [promotion.md](promotion.md) — khuyến mãi
- [../01-business/own-brand.md](../01-business/own-brand.md) — quản lý mùa vụ
