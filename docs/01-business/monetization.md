# Nghiệp vụ: Mô hình kiếm tiền (Monetization)

Tài liệu này chi tiết hóa từng dòng phí và hoa hồng đã nêu tổng quan ở [../00-overview/business-model.md](../00-overview/business-model.md).

---

## 1. Bảng tổng hợp nguồn thu

| # | Nguồn thu | Bên trả | Cơ sở tính | Giai đoạn |
|---|---|---|---|---|
| 1 | Biên lợi nhuận own brand | Khách hàng | Giá bán − COGS | MVP |
| 2 | Hoa hồng marketplace | Seller | % giá trị đơn | MVP |
| 3 | Phí thanh toán | Seller | % giá trị giao dịch | Phase 2 |
| 4 | Phí fulfillment | Seller | Theo đơn / theo trọng lượng | Phase 2 |
| 5 | Phí lưu kho | Seller | Theo thể tích × thời gian | Phase 3 |
| 6 | Phí gian hàng | Seller | Cố định theo kỳ | Phase 2 |
| 7 | Retail media | Seller / Brand | CPC hoặc CPM | Phase 4 |
| 8 | Phí chiến dịch creator | Brand / Seller | Theo chiến dịch | Phase 4 |
| 9 | Dịch vụ chuỗi cung ứng | Seller | Theo thỏa thuận | Phase 3+ |

---

## 2. Hoa hồng marketplace

### 2.1 Cấu trúc phân tầng

Hoa hồng **không** nên là một con số duy nhất. Cấu trúc đề xuất:

```text
Tỷ lệ hoa hồng cơ sở theo ngành hàng:

  Áo, quần, váy          10%
  Đầm, áo khoác          12%
  Giày                    8%
  Túi xách               12%
  Phụ kiện nhỏ           15%
  Trang sức thời trang   15%
  Đồ thể thao             9%
```

**Nguyên tắc định giá:** ngành hàng có biên lợi nhuận cao chịu hoa hồng cao hơn. Phụ kiện có biên cao, giày có biên thấp và giá trị đơn lớn.

### 2.2 Điều chỉnh theo bối cảnh

```text
Tỷ lệ áp dụng = Tỷ lệ cơ sở
                × hệ số loại seller       (strategic partner có thể thấp hơn)
                × hệ số chương trình      (seller mới ưu đãi 3 tháng đầu)
                × hệ số hiệu suất         (seller xuất sắc được giảm)
                + phụ phí chiến dịch      (nếu tham gia chiến dịch đặc biệt)
```

### 2.3 Ràng buộc kiến trúc bắt buộc

**Tỷ lệ hoa hồng phải được đóng băng vào đơn hàng tại thời điểm đặt hàng.**

```text
OrderLine {
    offer_id
    seller_id
    unit_price          ← đóng băng
    quantity
    commission_rate     ← đóng băng
    commission_amount   ← đã tính, đóng băng
}
```

Vì sao bắt buộc (nguyên tắc P9):

| Nếu tính động khi đối soát | Nếu đóng băng |
|---|---|
| Đổi chính sách → số tiền đơn cũ thay đổi | Đơn cũ giữ nguyên |
| Chạy đối soát hai lần ra hai kết quả | Kết quả nhất quán |
| Không kiểm toán được | Kiểm toán được |
| Tranh chấp với seller không giải quyết được | Có bằng chứng rõ ràng |

Đây là một trong những lỗi nghiêm trọng và phổ biến nhất của hệ thống marketplace.

---

## 3. Hoa hồng creator

### 3.1 Cấu trúc

```text
Tỷ lệ hoa hồng creator = f(
    ngành hàng,
    loại creator,
    chiến dịch,
    độc quyền hay không
)

Khoảng tham khảo: 3% – 15% giá trị đơn quy kết
```

### 3.2 Ai chịu chi phí?

Đây là câu hỏi chính sách phải trả lời rõ ràng, và mô hình dữ liệu phải lưu được câu trả lời.

```text
Trường hợp A — Sản phẩm own brand:
    Nền tảng chịu toàn bộ.

Trường hợp B — Sản phẩm marketplace, chiến dịch do seller khởi xướng:
    Seller chịu. Trừ vào phần seller nhận.

Trường hợp C — Sản phẩm marketplace, chiến dịch do nền tảng khởi xướng:
    Nền tảng chịu, trừ vào hoa hồng nền tảng.
    Dùng để khuyến khích seller tham gia chiến dịch lớn.

Trường hợp D — Chia sẻ:
    Chia theo tỷ lệ thỏa thuận.
```

**Hệ quả kiến trúc:**

```text
Campaign {
    id
    commission_rate
    cost_bearer         ← PLATFORM | SELLER | SHARED
    platform_share      ← nếu SHARED
    seller_share        ← nếu SHARED
    fee_structure       ← COMMISSION_ONLY | FIXED_FEE | HYBRID
    fixed_fee_amount    ← nếu có
}
```

Không thiết kế `Campaign` chỉ với một trường `commission_rate` — sẽ không mô hình hóa được KOL yêu cầu phí cố định.

### 3.3 Cơ sở tính hoa hồng — định nghĩa bắt buộc

Nói "hoa hồng 5% giá trị đơn" là **chưa đủ**. Khi một dòng hàng có nhiều khoản điều chỉnh với bên chịu chi phí khác nhau, "giá trị đơn" là số nào?

```text
Giá niêm yết                      300.000đ
  − Giảm giá seller (10%)         −30.000đ   cost_bearer = SELLER
  − Mã giảm giá nền tảng tài trợ  −20.000đ   cost_bearer = PLATFORM
  + Thuế VAT                      +25.000đ
  ─────────────────────────────────────────
  Khách trả                       275.000đ
```

Bốn cách hiểu cho bốn kết quả khác nhau — chênh tới 20%:

| Cơ sở | Số tiền | Hoa hồng 5% |
|---|---|---|
| Giá niêm yết | 300.000đ | 15.000đ |
| Sau giảm giá seller | 270.000đ | **13.500đ** |
| Sau mọi giảm giá | 250.000đ | 12.500đ |
| Số khách thực trả | 275.000đ | 13.750đ |

**Quyết định:**

```text
Cơ sở tính hoa hồng (nền tảng và creator)
  = giá niêm yết
  − các Adjustment có cost_bearer = SELLER
  KHÔNG trừ Adjustment có cost_bearer = PLATFORM
  KHÔNG tính thuế
```

Với ví dụ trên: cơ sở = **270.000đ**.

**Vì sao không trừ phần nền tảng tài trợ:**

Giảm giá do nền tảng chịu là **chi phí marketing của nền tảng** để thúc đẩy doanh số. Nếu trừ nó khỏi cơ sở tính, creator và seller bị **giảm thu nhập vì nền tảng chạy khuyến mãi** — điều này khiến họ ngại tham gia đúng lúc nền tảng cần họ nhất.

**Vì sao không tính thuế:** thuế là khoản thu hộ nhà nước, không phải doanh thu của bất kỳ bên nào.

**Hệ quả kiến trúc:** phải đóng băng cả **cơ sở tính**, không chỉ tỷ lệ:

```text
OrderLine {
    commission_rate     ← ĐÓNG BĂNG
    commission_base     ← ĐÓNG BĂNG (bổ sung)
    commission_amount   ← ĐÓNG BĂNG
}
```

Nếu chỉ lưu tỷ lệ, đối soát sau này phải tính lại cơ sở từ các adjustment — và kết quả có thể khác nếu quy tắc đã đổi. Xem [../11-oss/creator-commerce.md](../11-oss/creator-commerce.md) mục 3.

---

## 4. Ví dụ tính toán đầy đủ

Đơn hàng marketplace 300.000đ, có quy kết creator, seller dùng fulfillment nền tảng:

```text
Giá bán                                            300.000đ
    │
    ├─ Hoa hồng nền tảng (10%)                     −30.000đ  → Doanh thu NT
    ├─ Phí thanh toán (1.5%)                        −4.500đ  → Chi phí NT (trả PSP)
    ├─ Phí fulfillment (cố định)                   −15.000đ  → Doanh thu NT
    ├─ Hoa hồng creator (5%, seller chịu)          −15.000đ  → Phải trả creator
    │
    ▼
Ghi có seller                                      235.500đ

────────────────────────────────────────────────────────────
Doanh thu nền tảng ghi nhận:
    Hoa hồng                     30.000đ
    Phí fulfillment              15.000đ
    ─────────────────────────────────────
    Tổng doanh thu               45.000đ

Chi phí nền tảng:
    Phí PSP                       4.500đ
    Chi phí fulfillment thực     12.000đ
    ─────────────────────────────────────
    Tổng chi phí                 16.500đ

Lợi nhuận gộp nền tảng           28.500đ  (9,5% trên GMV)

Nghĩa vụ phải trả:
    Seller                      235.500đ
    Creator                      15.000đ
```

**Quan sát quan trọng:** GMV là 300.000đ nhưng doanh thu nền tảng chỉ 45.000đ. Báo cáo phải phân biệt rõ hai con số này. Nhầm lẫn GMV với doanh thu là sai lầm phổ biến khi trình bày số liệu marketplace.

---

## 5. So sánh với đơn own brand

Cùng giá bán 300.000đ:

```text
Giá bán                                            300.000đ  → Doanh thu NT
    │
    ├─ COGS (từ Production Batch)                −120.000đ  → Giá vốn
    ├─ Phí thanh toán                              −4.500đ
    ├─ Chi phí fulfillment                        −20.000đ
    ├─ Hoa hồng creator (5%, NT chịu)             −15.000đ
    │
    ▼
Lợi nhuận gộp                                     140.500đ  (46,8%)
```

| | Marketplace | Own Brand |
|---|---|---|
| Doanh thu ghi nhận | 45.000đ | 300.000đ |
| Lợi nhuận gộp | 28.500đ | 140.500đ |
| Biên trên GMV | 9,5% | 46,8% |
| Vốn cần | 0 | ~120.000đ/đơn vị, ứng trước nhiều tháng |
| Rủi ro tồn kho | Không | Có |

**Kết luận chiến lược:** own brand có lợi nhuận tuyệt đối cao hơn nhiều nhưng cần vốn và chịu rủi ro. Marketplace mở rộng nhanh với vốn thấp. Kết hợp cả hai là cách cân bằng — và là lý do kiến trúc phải xử lý tốt cả hai.

---

## 6. Số dư và chu kỳ chi trả

### 6.1 Các trạng thái số dư seller

```text
Pending    — đơn đã giao, đang trong thời hạn đổi trả
             (chưa được rút, vì có thể phát sinh hoàn hàng)
    │
    ▼ (hết hạn đổi trả)
Available  — sẵn sàng chi trả
    │
    ▼ (đến kỳ đối soát)
Processing — đang xử lý chuyển tiền
    │
    ▼
Paid       — đã chuyển thành công

Trạng thái đặc biệt:
On Hold    — bị giữ do tranh chấp, vi phạm, hoặc yêu cầu pháp lý
Negative   — số dư âm do hoàn hàng vượt doanh thu kỳ
```

### 6.2 Xử lý số dư âm

Tình huống thực tế: seller bán 5 triệu tháng trước, đã nhận tiền. Tháng này bị hoàn 2 triệu nhưng chỉ bán được 1 triệu.

```text
Số dư kỳ này = 1.000.000 − 2.000.000 = −1.000.000đ

Xử lý:
  1. Không chuyển tiền âm
  2. Chuyển khoản âm sang kỳ sau
  3. Nếu âm kéo dài → yêu cầu seller nộp bù hoặc giữ hàng ký gửi
  4. Nếu seller ngừng hoạt động với số dư âm → khoản phải thu khó đòi
```

**Cơ chế phòng ngừa:** giữ lại một tỷ lệ bảo đảm (reserve) với seller mới hoặc seller có tỷ lệ hoàn hàng cao. Ví dụ giữ 10% doanh thu trong 30 ngày.

Đây là quyết định kinh doanh, nhưng mô hình dữ liệu phải hỗ trợ: `seller_policy` cần có trường `reserve_rate` và `reserve_hold_days`.

---

## 7. Retail Media (Phase 4)

Seller trả tiền để được hiển thị ưu tiên.

```text
Vị trí bán được:
  - Kết quả tìm kiếm được tài trợ
  - Đề xuất trên trang sản phẩm
  - Banner trang chủ
  - Vị trí trong feed nội dung
  - Chiến dịch email

Mô hình tính tiền:
  CPC (Cost Per Click)          — phổ biến nhất
  CPM (Cost Per Mille)          — cho mục tiêu nhận diện
  CPA (Cost Per Acquisition)    — chia sẻ rủi ro
```

**Cảnh báo chiến lược:** retail media có biên lợi nhuận rất cao nhưng làm giảm chất lượng kết quả tìm kiếm nếu lạm dụng. Nguyên tắc: nội dung được tài trợ phải **được đánh dấu rõ ràng** và bị giới hạn tỷ lệ trên mỗi trang.

Nếu khách không tin kết quả tìm kiếm, họ sẽ rời nền tảng — và mất luôn nguồn thu retail media.

---

## 8. Nguyên tắc kiến trúc tài chính

Tổng hợp các ràng buộc bắt buộc rút ra từ tài liệu này:

| # | Ràng buộc | Lý do |
|---|---|---|
| 1 | Ledger bất biến, chỉ ghi thêm | Kiểm toán, tái dựng số dư quá khứ |
| 2 | Sửa sai bằng bút toán điều chỉnh | Không mất dấu vết |
| 3 | Đóng băng tỷ lệ hoa hồng vào đơn | Đối soát nhất quán |
| 4 | Phân biệt GMV và doanh thu ở tầng ghi sổ | Báo cáo đúng |
| 5 | Không tính số dư bằng phép cộng rải rác | Tránh sai lệch |
| 6 | Mọi khoản tiền có trạng thái rõ ràng | Biết tiền của ai, khi nào được rút |
| 7 | Hoàn hàng phải đảo ngược đủ chuỗi | Không để tiền treo |
| 8 | Mọi giao dịch tài chính là idempotent | Tránh trả tiền hai lần |

Chi tiết cài đặt: [../04-modules/payment.md](../04-modules/payment.md) và [../adr/0008-financial-ledger.md](../adr/0008-financial-ledger.md).

---

## 9. Tài liệu liên quan

- [../00-overview/business-model.md](../00-overview/business-model.md) — tổng quan mô hình kinh doanh
- [kpi.md](kpi.md) — chỉ số đo lường
- [marketplace.md](marketplace.md) — nghiệp vụ marketplace
- [../04-modules/payment.md](../04-modules/payment.md) — module thanh toán và ledger
