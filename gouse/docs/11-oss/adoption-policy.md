# Chính sách tiếp nhận OSS

## 1. Bốn quyết định

Mọi năng lực OSS được xét đều rơi vào một trong bốn nhóm:

```text
ADOPT   Lấy gần như nguyên vẹn — mẫu đã kiểm chứng, phù hợp domain
ADAPT   Lấy ý tưởng, thiết kế lại cho domain của chúng ta
BUILD   Tự xây — OSS không có, hoặc đây là lợi thế cạnh tranh
REJECT  Cố ý không làm — mâu thuẫn với kiến trúc hoặc chi phí không tương xứng
```

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

## 4. REJECT — cố ý không làm

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

---

## 5. Ba thay đổi thiết kế từ nghiên cứu này

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

## 6. Quy trình xét một thư viện mới

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

## 7. Quy tắc sao chép code

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

## 8. Tài liệu liên quan

- [research-matrix.md](research-matrix.md) — so sánh chi tiết
- [dependency-registry.md](dependency-registry.md) — danh sách thư viện
- [synthesis.md](synthesis.md) — kiến trúc tổng hợp
