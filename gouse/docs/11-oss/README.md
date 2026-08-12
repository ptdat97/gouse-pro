# Nghiên cứu mã nguồn mở

## 1. Mục đích

> **Xây dựng Fashion Commerce Core của chúng ta, đồng thời thu nhặt có chọn lọc những ý tưởng và cách cài đặt đã được kiểm chứng từ OSS để tăng tốc phát triển.**

Mục tiêu **không phải** chọn một framework thương mại điện tử có sẵn rồi xây trên đó.

```text
OSS cho chúng ta TỐC ĐỘ.
Kiến trúc của chúng ta cho chúng ta KIỂM SOÁT.
Miền nghiệp vụ của chúng ta cho chúng ta LỢI THẾ CẠNH TRANH.
```

---

## 2. Nguyên tắc bất di bất dịch

> **Miền nghiệp vụ của chúng ta là nguồn sự thật. OSS không được định đoạt nó.**

Kiến trúc cuối cùng **không được** trở thành:

```text
Flamingo + QOR + Digota + Medusa + Shopware
```

Nó phải là:

```text
FASHION COMMERCE PLATFORM CỦA CHÚNG TA
        │
        ├── Mô hình domain của chúng ta
        ├── Quy tắc nghiệp vụ của chúng ta
        ├── Ranh giới module của chúng ta
        └── Các mẫu OSS được CHỌN LỌC
```

---

## 3. Cách đọc tài liệu này

| Bạn muốn | Đọc |
|---|---|
| Kết luận cuối cùng, quyết định kiến trúc | [synthesis.md](synthesis.md) |
| So sánh năng lực giữa các dự án | [research-matrix.md](research-matrix.md) |
| Quy tắc quyết định adopt/adapt/reject | [adoption-policy.md](adoption-policy.md) |
| Thư viện nào được dùng, license gì | [dependency-registry.md](dependency-registry.md) |
| Chi tiết một dự án cụ thể | các file bên dưới |

---

## 4. Danh sách dự án đã nghiên cứu

### Hệ sinh thái Go

| Dự án | Sao | Cập nhật cuối | License | Vai trò với chúng ta |
|---|---|---|---|---|
| [Flamingo Commerce](flamingo-commerce.md) | 591 | 2026-08-11 | MIT | **Tham chiếu kiến trúc Go chính** |
| [QOR](qor.md) | 5.343 | 2026-07-27 | MIT | Tham chiếu quản trị, workflow, media |
| [Digota](digota.md) | 524 | **2021-02-14** | MIT | Tham chiếu tiền tệ, khóa phân tán |
| [GoShop](goshop.md) | 395 | 2026-05-22 | MIT | Tham chiếu cấu trúc ứng dụng Go |
| [GoCommerce](gocommerce.md) | 1.617 | 2025-07-11 | MIT | Tham chiếu API headless, thuế |
| [go-saas/commerce](go-saas-commerce.md) | **5** | **2023-09-03** | **Không có** | Tham chiếu thứ yếu, gần như bỏ |

### Nền tảng thương mại lớn (non-Go)

| Dự án | Sao | Ngôn ngữ | License | Vai trò với chúng ta |
|---|---|---|---|---|
| [Medusa](medusa.md) | 35.728 | TypeScript | MIT + **Enterprise riêng** | Module links, workflows |
| [Vendure](vendure.md) | 8.319 | TypeScript | **GPLv3** / thương mại | Channel, plugin, event |
| [Saleor](saleor.md) | 23.218 | Python | BSD-3 | Channel listing, API-first |
| [Shopware](shopware.md) | 3.402 | PHP | MIT | Rule system, DAL, sales channel |
| [Sylius](sylius.md) | 8.513 | PHP | MIT | **State machine, Adjustment** |
| [Magento](magento.md) | 12.166 | PHP | **OSL-3.0** | **MSI reservation**, catalog phức tạp |

Số liệu lấy từ GitHub API ngày **12/08/2026**.

### Mô hình kinh doanh

| Tài liệu | Nội dung |
|---|---|
| [shein-model.md](shein-model.md) | Chuỗi cung ứng theo nhu cầu, lô nhỏ, vòng phản hồi dữ liệu |
| [creator-commerce.md](creator-commerce.md) | TikTok Shop, quy kết creator, hoa hồng |

---

## 5. Cảnh báo về license — đọc trước khi sao chép code

Đây là ràng buộc **pháp lý**, không phải sở thích kỹ thuật.

| License | Dự án | Được sao chép code vào sản phẩm độc quyền? |
|---|---|---|
| MIT | Flamingo, QOR, Digota, GoShop, GoCommerce, Shopware, Sylius | **Có** — giữ thông báo bản quyền |
| BSD-3 | Saleor | **Có** — giữ thông báo bản quyền |
| **GPLv3** | **Vendure** | **KHÔNG** — copyleft, lây nhiễm toàn bộ sản phẩm |
| **OSL-3.0** | **Magento** | **KHÔNG** — copyleft mạnh, kích hoạt cả khi chỉ dùng qua mạng |
| MIT + Enterprise | **Medusa** | **Có phần** — phần Enterprise theo giấy phép riêng |
| **Không có** | **go-saas/commerce** | **KHÔNG** — không license = giữ toàn quyền, mặc định cấm |

### Hệ quả thực tế

```text
Vendure và Magento: CHỈ ĐỌC ĐỂ HỌC Ý TƯỞNG.
    → không sao chép code
    → không sao chép cấu trúc file
    → ghi lại ý tưởng bằng lời, rồi tự cài đặt từ đầu

Ý tưởng và kiến trúc KHÔNG bị bảo hộ bản quyền — chỉ mã nguồn cụ thể mới bị.
Học "reservation ngăn oversell" từ Magento là hợp pháp;
sao chép lớp `ReservationBuilder` của họ thì không.
```

Chi tiết: [dependency-registry.md](dependency-registry.md).

---

## 6. Ba phát hiện làm thay đổi thiết kế của chúng ta

Nghiên cứu này **không chỉ xác nhận** thiết kế cũ — nó thay đổi ba điểm:

### 6.1 Bảng liên kết tường minh (từ Medusa)

Tài liệu cũ của chúng ta nói "không khóa ngoại vượt ranh giới module" nhưng **không nói cách liên kết dữ liệu**. Medusa giải bằng **link table** — bảng trung gian chỉ chứa hai định danh, không có ràng buộc khóa ngoại.

→ Cập nhật [05-data/data-model.md](../05-data/data-model.md).

### 6.2 Adjustment là khái niệm hạng nhất (từ Sylius)

Tài liệu cũ để giảm giá, thuế, phí ship nằm rải rác thành các trường trên `Order`. Sylius mô hình hóa chúng thành **Adjustment** gắn vào từng dòng hàng, có loại và nguồn gốc.

→ Điều này giải quyết trực tiếp bài toán "hoàn tiền theo giá thực trả" mà tôi đã nêu ở [07-workflows/return.md](../07-workflows/return.md).

### 6.3 Tách state machine theo mối quan tâm (từ Sylius)

Tài liệu cũ có **một** trạng thái cho `Order`. Sylius tách thành **ba** state machine độc lập: checkout, payment, shipping.

→ Phù hợp hơn với mô hình `Order`/`FulfillmentOrder` của chúng ta.

Chi tiết cả ba: [synthesis.md](synthesis.md).

---

## 7. Điều chúng ta KHÔNG lấy từ OSS

Bốn năng lực này là **tài sản trí tuệ cốt lõi** — không dự án OSS nào có, và không được để chúng trở thành phụ thuộc vào framework bên ngoài:

```text
1. Mô hình Offer cho marketplace thời trang
2. Quy kết creator (attribution)
3. Tín hiệu nhu cầu → lập kế hoạch sản phẩm
4. Vận hành own brand: concept → mẫu → sản xuất → QC → lô
```

Xem [synthesis.md](synthesis.md) mục "Miền độc quyền".

---

## 8. Tài liệu liên quan

- [../00-overview/principles.md](../00-overview/principles.md) — P15: mỗi quyết định phải giải thích được vì sao cần cho **chính** nghiệp vụ này
- [../10-roadmap/deliverables.md](../10-roadmap/deliverables.md) — tổng hợp bàn giao
- [../adr/](../adr/) — các quyết định kiến trúc
