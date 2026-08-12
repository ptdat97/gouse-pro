# Creator commerce — nghiên cứu mô hình

| | |
|---|---|
| Loại | Nghiên cứu mô hình kinh doanh (không phải OSS) |
| Nguồn tham chiếu | TikTok Shop, mô hình affiliate của các nền tảng xã hội |
| Vai trò | **Mô hình tham chiếu cho miền creator của chúng ta** |

---

## 1. Khoảng trống hoàn toàn trong OSS

Ma trận nghiên cứu cho kết quả rõ ràng nhất ở nhóm này:

```text
Creator                 không dự án OSS nào có
Affiliate / attribution không dự án OSS nào có (Magento có ở mức rất sơ khai)
Product tag trong nội dung  không có
Outfit / phối đồ        không có
```

Creator commerce là năng lực của nền tảng đóng. Không có mã nguồn để học — chỉ có **mô hình nghiệp vụ** từ tài liệu công khai và chính sách của họ.

---

## 2. Mô hình quy kết của TikTok Shop

### Cách nền tảng làm

**Mô hình quy kết:** last-click. Hoa hồng thuộc về creator có link được click **ngay trước** lúc mua, không phải creator giới thiệu đầu tiên.

**Cửa sổ quy kết:** 7 ngày cho click, 1 ngày cho lượt xem. Click được ưu tiên hơn lượt xem.

**Tỷ lệ hoa hồng:** do người bán tự đặt, không phải nền tảng. Biên độ cho phép rất rộng (khoảng 1%–80%), thực tế tập trung ở 5%–50% tùy ngành hàng.

**Cơ sở tính hoa hồng:** giá sau khi trừ giảm giá của người bán, nhưng **bao gồm** phần trợ giá do nền tảng tài trợ.

### So sánh với thiết kế của chúng ta

| Hạng mục | TikTok Shop | Chúng ta | Đánh giá |
|---|---|---|---|
| Mô hình quy kết | Last-click | Last-click | **Khớp** |
| Cửa sổ click | 7 ngày | 7 ngày | **Khớp** |
| Cửa sổ xem | 1 ngày | Chưa có | Cân nhắc Phase 3 |
| Tỷ lệ do ai đặt | Người bán | Chiến dịch (`Campaign`) | Tương đương |
| Cơ sở tính | Sau giảm giá seller | Chưa nói rõ | **Cần làm rõ** |

Thiết kế của chúng ta khớp ở hai điểm quan trọng nhất. Nhưng nghiên cứu này phát hiện **một khoảng trống**.

---

## 3. Phát hiện: cơ sở tính hoa hồng chưa được định nghĩa

### Vấn đề

Tài liệu của chúng ta nói hoa hồng creator là "% giá trị đơn quy kết" nhưng **không định nghĩa "giá trị đơn" là gì** khi có nhiều loại giảm giá.

Với mô hình `Adjustment` mới lấy từ Sylius, một dòng hàng có thể có nhiều khoản:

```text
Giá niêm yết                    300.000đ
  − Giảm giá seller (10%)       −30.000đ   cost_bearer = SELLER
  − Mã giảm giá nền tảng        −20.000đ   cost_bearer = PLATFORM
  + Thuế VAT                    +25.000đ
  ─────────────────────────────────────
  Khách trả                     275.000đ
```

Hoa hồng creator 5% tính trên số nào? Bốn phương án cho ra bốn kết quả khác nhau:

```text
Trên giá niêm yết       300.000 × 5% = 15.000đ
Trên sau giảm seller    270.000 × 5% = 13.500đ
Trên sau mọi giảm giá   250.000 × 5% = 12.500đ
Trên số khách trả       275.000 × 5% = 13.750đ
```

Chênh lệch tới 20%. Nếu không định nghĩa rõ, đối soát sẽ tranh chấp.

### Quyết định

Theo mô hình TikTok Shop và logic kinh tế:

```text
Cơ sở tính hoa hồng creator
  = giá niêm yết
  − các Adjustment có cost_bearer = SELLER
  (KHÔNG trừ Adjustment có cost_bearer = PLATFORM)
  (KHÔNG tính thuế)
```

**Lý do:** giảm giá do nền tảng tài trợ là chi phí marketing của nền tảng để thúc đẩy doanh số. Trừ nó khỏi cơ sở tính hoa hồng nghĩa là **creator bị phạt vì nền tảng chạy khuyến mãi** — điều này làm creator ngại tham gia đúng lúc nền tảng cần họ nhất.

Với ví dụ trên: cơ sở = 270.000đ, hoa hồng = 13.500đ.

### Hệ quả

```text
→ Cập nhật docs/01-business/monetization.md: định nghĩa cơ sở tính
→ Cập nhật docs/04-modules/affiliate.md: đóng băng commission_base
   vào Attribution, không chỉ commission_rate
```

Việc đóng băng cả **cơ sở tính** (không chỉ tỷ lệ) là cần thiết — nếu chỉ lưu tỷ lệ, đối soát sau này phải tính lại cơ sở từ các adjustment, và kết quả có thể khác nếu quy tắc đổi.

---

## 4. Vấn đề "assisted conversion"

### Bối cảnh

Last-click có điểm yếu ai cũng biết: creator giới thiệu sản phẩm đầu tiên (tạo ra nhận biết) không nhận được gì nếu khách mua sau khi click link của creator khác.

Các nền tảng đang bổ sung công cụ hiển thị **đóng góp gián tiếp** — những đơn hàng mà một creator có ảnh hưởng nhưng không nhận được quy kết cuối.

### Điều này xác nhận quyết định của chúng ta

[04-modules/affiliate.md](../04-modules/affiliate.md) mục 3.3 đã quyết định:

```text
Dùng last-click (đơn giản, dễ giải thích)
NHƯNG lưu TOÀN BỘ chuỗi click, không chỉ click được quy kết
```

Lý do đã ghi: "nếu chỉ lưu click cuối, sau này muốn đổi mô hình quy kết sẽ không có dữ liệu lịch sử để tính lại".

Nghiên cứu này xác nhận đó là quyết định đúng — các nền tảng lớn đang đi chính xác theo hướng đó, và họ **có dữ liệu** để làm vì đã lưu đủ chuỗi.

### Adopt

Giữ nguyên. Bổ sung một tính năng có giá trị mà không đổi mô hình quy kết:

```text
Báo cáo cho creator: "Nội dung của bạn có N lượt click dẫn tới đơn hàng
được quy kết cho người khác trong 7 ngày"
```

Việc này minh bạch với creator mà không cần đổi cách chia tiền — có thể làm ở Phase 3.

---

## 5. Ba loại người tạo nội dung và cấu trúc trả tiền

### Quan sát từ thị trường

Các nền tảng phân biệt rõ:

```text
KOC (người tiêu dùng có ảnh hưởng)
    quy mô nhỏ, độ tin cậy cao, tỷ lệ chuyển đổi cao
    → chấp nhận hoa hồng thuần, chịu rủi ro doanh số

KOL / Influencer (người ảnh hưởng lớn)
    quy mô lớn, tạo nhận diện
    → yêu cầu PHÍ CỐ ĐỊNH, không chấp nhận rủi ro doanh số

Content Partner (tổ chức truyền thông)
    → hợp đồng sản xuất nội dung
```

### Hệ quả kiến trúc

Đây là lý do `Campaign` phải hỗ trợ **ba** cấu trúc chi phí, không chỉ một trường `commission_rate`:

```text
COMMISSION_ONLY   chỉ hoa hồng          → KOC
FIXED_FEE         phí cố định           → Content Partner
HYBRID            phí + hoa hồng        → KOL
```

Đã có trong [04-modules/campaign.md](../04-modules/campaign.md) mục 3. Nghiên cứu xác nhận cả ba đều cần thiết, không phải trừu tượng hóa sớm.

---

## 6. Đặc thù thời trang mà nền tảng chung không có

Đây là chỗ chúng ta **đi xa hơn** mô hình tham chiếu.

### Outfit — đơn vị nội dung riêng của thời trang

TikTok Shop và các nền tảng chung mô hình hóa nội dung là "video có gắn sản phẩm". Với thời trang, điều đó chưa đủ:

```text
Outfit "Đi làm mùa thu"
├── Áo sơ mi linen   (MAIN)
├── Quần âu          (MAIN)
├── Giày loafer      (COMPLEMENT)
└── Túi tote         (ACCESSORY)

Tổng: 749.000đ  →  "Mua cả bộ"
```

Giá trị kinh doanh: tăng giá trị đơn hàng trung bình (khách vào định mua một món, mua cả bộ).

### Stylist — vai trò creator đặc thù

Ngoài KOC/KOL, thời trang có **stylist**: chuyên gia phối đồ. Giá trị của họ khác biệt:

```text
KOL:      tạo nhận diện, tiếp cận rộng
Stylist:  GIẢM TỶ LỆ HOÀN HÀNG
          → tư vấn size, phối đồ đúng dáng người
```

Vì tỷ lệ hoàn hàng là vấn đề kinh tế lớn nhất của thương mại thời trang, stylist có thể tạo giá trị lớn hơn KOL dù lượng theo dõi nhỏ hơn.

Đã có trong [01-business/creator.md](../01-business/creator.md) mục 1.

### Tỷ lệ hoàn hàng theo nội dung

Chỉ số này **không có** ở nền tảng chung nhưng thiết yếu với chúng ta:

```text
Nội dung A: 68 đơn, tỷ lệ hoàn 9%   ✓
Nội dung B: 41 đơn, tỷ lệ hoàn 28%  ⚠ nội dung gây hiểu nhầm
```

Chỉ số hai chiều: giúp creator cải thiện, giúp nền tảng phát hiện nội dung mô tả sai lệch.

Đã có trong [06-api/creator-api.md](../06-api/creator-api.md).

---

## 7. Chống gian lận

### Các kiểu gian lận đã biết

```text
Click ảo            tự click link của mình nhiều lần
Tự mua rồi hoàn     lấy hoa hồng rồi trả hàng
Cookie stuffing     gắn link ẩn chiếm quy kết
Nội dung sai lệch   mô tả sai để bán được
```

### Nguyên tắc kiến trúc

Phát hiện gian lận phải chạy **bất đồng bộ**, không nằm trong đường đi chính:

```text
Khách click link → CHUYỂN HƯỚNG NGAY (< 50ms)
                 → ghi Click song song
                 → phân tích gian lận sau, theo lô
```

Nếu kiểm tra gian lận đồng bộ, mỗi click chậm thêm hàng trăm mili-giây — làm hỏng chính trải nghiệm mà creator commerce cần.

Đã có trong [07-workflows/creator-affiliate.md](../07-workflows/creator-affiliate.md) mục 3.

---

## 8. Tổng kết

| Hạng mục | Quyết định |
|---|---|
| Last-click + cửa sổ 7 ngày | **ADOPT** — khớp mô hình tham chiếu |
| Lưu toàn bộ chuỗi click | **ADOPT** — đã có, được xác nhận |
| **Cơ sở tính hoa hồng** | **ADOPT** — phát hiện khoảng trống, cần bổ sung |
| Ba cấu trúc chi phí chiến dịch | **ADOPT** — đã có, được xác nhận |
| Cửa sổ quy kết cho lượt xem | **ADAPT** — cân nhắc Phase 3 |
| Báo cáo đóng góp gián tiếp | **ADAPT** — Phase 3, không đổi cách chia tiền |
| Outfit là thực thể riêng | **BUILD** — đặc thù thời trang, đi xa hơn tham chiếu |
| Vai trò Stylist | **BUILD** — đặc thù thời trang |
| Tỷ lệ hoàn theo nội dung | **BUILD** — đặc thù thời trang |

**Nhận xét cuối:** Nghiên cứu này xác nhận phần lớn thiết kế và phát hiện **một khoảng trống thật**: cơ sở tính hoa hồng chưa được định nghĩa khi có nhiều loại giảm giá với bên chịu chi phí khác nhau.

Khoảng trống này chỉ lộ ra **sau khi** lấy mô hình `Adjustment` từ Sylius — hai nghiên cứu độc lập kết hợp lại mới phát hiện được.

---

## 9. Tài liệu liên quan

- [../01-business/creator.md](../01-business/creator.md)
- [../04-modules/affiliate.md](../04-modules/affiliate.md)
- [../04-modules/campaign.md](../04-modules/campaign.md)
- [sylius.md](sylius.md) — mô hình Adjustment liên quan

## 10. Nguồn

- [TikTok Shop Affiliate Commission 2026](https://www.hamstergarage.com/article/tiktok-shop-affiliate-commission-rates-fees-payouts)
- [Solving TikTok Shop's Last-Click Problem](https://socialcommerceclub.com/blogs/tiktok-shop/solving-tiktok-shops-last-click-problem-for-affiliate-attribution)
- [TikTok Attribution: Windows, Models](https://tikadtools.com/blog/tiktok-attribution/)
