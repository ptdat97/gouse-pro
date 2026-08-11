# Mô hình kinh doanh

Tài liệu này mô tả nền tảng **kiếm tiền như thế nào** và **dòng tiền chảy ra sao**. Kiến trúc tài chính ([05-data](../05-data/), module [payment](../04-modules/payment.md)) tồn tại để phục vụ đúng những dòng tiền mô tả ở đây.

## 1. Các nguồn doanh thu

| # | Nguồn | Mô tả | Giai đoạn |
|---|---|---|---|
| 1 | Biên lợi nhuận own brand | Chênh lệch giá bán và giá vốn sản xuất | MVP |
| 2 | Hoa hồng marketplace | % trên giá trị đơn hàng của seller | MVP |
| 3 | Phí dịch vụ seller | Phí gian hàng, phí xử lý thanh toán, phí fulfillment | Phase 2 |
| 4 | Retail media | Seller trả tiền để được hiển thị ưu tiên | Phase 4 |
| 5 | Creator marketplace | Phí kết nối brand ↔ creator cho chiến dịch | Phase 4 |
| 6 | Dịch vụ chuỗi cung ứng | Cho seller dùng năng lực sản xuất/kho của nền tảng | Phase 3+ |

Nguồn 1 và 2 là cốt lõi. Các nguồn còn lại chỉ khả thi khi đã có quy mô.

## 2. Hai mô hình cung ứng song song

Nền tảng vận hành đồng thời hai mô hình. Kiến trúc phải xử lý cả hai bằng **cùng một mô hình đơn hàng**, khác nhau ở chỗ ai sở hữu hàng và ai được chia tiền.

### 2.1 Own brand (1P — first party)

```text
Nền tảng
 ↓
Nhà cung cấp / Nhà máy
 ↓
Sản xuất
 ↓
Kiểm định chất lượng (QC)
 ↓
Kho của nền tảng
 ↓
Khách hàng
```

- Nền tảng **sở hữu hàng tồn kho** → chịu rủi ro tồn kho.
- Nền tảng kiểm soát giá, chất lượng, thời điểm ra mắt.
- Doanh thu ghi nhận toàn bộ; giá vốn là chi phí sản xuất.
- Biên lợi nhuận cao nhưng cần vốn lưu động.

### 2.2 Marketplace (3P — third party)

```text
Seller
 ↓
Nền tảng (trung gian giao dịch)
 ↓
Khách hàng
```

- Seller sở hữu hàng tồn kho → nền tảng không chịu rủi ro tồn kho.
- Nền tảng thu **hoa hồng** trên mỗi đơn.
- Doanh thu của nền tảng chỉ là phần hoa hồng và phí, không phải toàn bộ GMV.
- Mở rộng assortment rất nhanh, biên thấp hơn.

**Hệ quả kế toán quan trọng:** GMV (tổng giá trị hàng hóa) ≠ doanh thu (revenue). Với đơn 1P, doanh thu = giá bán. Với đơn 3P, doanh thu = hoa hồng + phí. Ledger phải phân biệt được hai loại này ngay ở tầng ghi sổ, không phải bằng báo cáo xử lý sau. Xem [ADR-0008](../adr/0008-financial-ledger.md).

## 3. Dòng tiền một đơn hàng marketplace

Ví dụ cụ thể — khách mua một chiếc áo 300.000đ từ Seller A, được giới thiệu bởi Creator X:

```text
Khách trả:                                    300.000đ
    │
    ▼
Tài khoản thu hộ của nền tảng (chờ đối soát)
    │
    ├── Hoa hồng nền tảng (10%)               −30.000đ  → Doanh thu nền tảng
    ├── Hoa hồng creator (5%)                 −15.000đ  → Phải trả creator
    ├── Phí thanh toán PSP (1.5%)              −4.500đ  → Chi phí
    │
    ▼
Số dư phải trả Seller A                       250.500đ
    │
    ▼ (sau chu kỳ đối soát, ví dụ T+7 kể từ khi giao hàng thành công)
Payout về tài khoản ngân hàng Seller A
```

Quan sát quan trọng:

1. **Tiền không thuộc về nền tảng ngay khi khách trả.** Nó là khoản thu hộ. Nếu ghi nhận sai thời điểm, báo cáo tài chính sai.
2. **Hoa hồng creator được trích từ phần của ai?** Đây là quyết định chính sách, không phải kỹ thuật. Mô hình mặc định: trích từ hoa hồng nền tảng hoặc từ phần seller, tùy thỏa thuận chiến dịch — nên `Campaign` phải lưu rõ bên chịu chi phí.
3. **Payout xảy ra sau, theo lô.** Không phải mỗi đơn một lần chuyển tiền.
4. **Hoàn hàng làm đảo ngược toàn bộ chuỗi trên.** Nếu payout đã thực hiện, phát sinh khoản phải thu ngược từ seller (adjustment).

Chi tiết mô hình: [../04-modules/payment.md](../04-modules/payment.md), luồng: [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md).

## 4. Dòng tiền một đơn hàng own brand

```text
Khách trả:                                    300.000đ  → Doanh thu nền tảng
    │
    ├── Giá vốn hàng bán (COGS)              −120.000đ  → Chi phí (từ Production Batch)
    ├── Phí thanh toán PSP                     −4.500đ
    ├── Chi phí fulfillment                   −20.000đ
    ├── Hoa hồng creator (nếu có)             −15.000đ
    │
    ▼
Lợi nhuận gộp                                 140.500đ
```

Điểm cần lưu ý: **COGS đến từ đâu?** Từ `Production Batch` — lô sản xuất cụ thể mà SKU đó thuộc về. Hai lô sản xuất cùng một SKU có thể có giá vốn khác nhau. Nếu không truy vết được SKU → lô sản xuất, không tính được biên lợi nhuận thật.

Đây là lý do module [manufacturing](../04-modules/manufacturing.md) và [warehouse](../04-modules/warehouse.md) phải giữ liên kết lô, không chỉ đếm số lượng.

## 5. Kinh tế đơn vị (unit economics) cần đo được

Kiến trúc phải cho phép trả lời các câu hỏi sau **bằng truy vấn, không bằng file Excel thủ công**:

| Câu hỏi | Dữ liệu cần | Module sở hữu |
|---|---|---|
| Biên lợi nhuận thật của SKU này? | Giá bán, COGS theo lô, chi phí fulfillment | order, manufacturing, fulfillment |
| Creator nào tạo ra doanh thu thật (sau hoàn hàng)? | Attribution, đơn hàng, hoàn hàng | affiliate, order, return |
| Seller nào có tỷ lệ hoàn hàng bất thường? | Đơn hàng, hoàn hàng theo seller | seller, return |
| Sản phẩm nào nên tái sản xuất? | Tín hiệu nhu cầu, tồn kho, lead time | supply-chain, inventory |
| Chi phí thu hút khách qua kênh creator vs quảng cáo? | Attribution, chi phí chiến dịch | affiliate, campaign |

Nếu một câu hỏi trong bảng trên không trả lời được, đó là **lỗ hổng kiến trúc**, không phải thiếu tính năng báo cáo.

## 6. Mô hình chi phí

```text
Chi phí biến đổi theo đơn:
  - Phí cổng thanh toán
  - Phí vận chuyển
  - Chi phí đóng gói
  - Hoa hồng creator
  - Chi phí xử lý hoàn hàng

Chi phí biến đổi theo hàng:
  - Giá vốn sản xuất (own brand)
  - Chi phí lưu kho
  - Chi phí hàng tồn lỗi thời (đặc biệt nghiêm trọng với thời trang theo mùa)

Chi phí cố định:
  - Hạ tầng công nghệ
  - Nhân sự vận hành
  - Marketing thương hiệu
```

**Rủi ro đặc thù ngành thời trang:** hàng tồn lỗi mốt mất giá rất nhanh. Một bộ sưu tập hết mùa có thể mất 50–70% giá trị. Đây chính là lý do trụ cột chuỗi cung ứng quan trọng: sản xuất đúng lượng quan trọng hơn sản xuất rẻ.

## 7. Ràng buộc kiến trúc rút ra từ mô hình kinh doanh

| Đặc điểm kinh doanh | Ràng buộc kiến trúc bắt buộc |
|---|---|
| Nhiều nguồn cung cho cùng sản phẩm | Tách `Offer` khỏi `Product` |
| Đơn có hàng từ nhiều nhà bán | Tách `Order` khỏi `Fulfillment Order` |
| Chia tiền cho nhiều bên | Ledger bất biến, không tính số dư bằng phép cộng rải rác |
| Doanh thu creator cần quy kết | Mô hình attribution có thời hạn, ghi nhận bất biến |
| Sản xuất theo nhu cầu | Dữ liệu hành vi phải quay ngược được vào planning |
| Hàng theo mùa, vòng đời ngắn | `Collection` và mùa vụ là khái niệm hạng nhất trong catalog |
| GMV ≠ doanh thu | Phân biệt 1P/3P ngay ở tầng ghi sổ |

## Tài liệu liên quan

- [../01-business/monetization.md](../01-business/monetization.md) — chi tiết từng dòng phí và hoa hồng
- [../01-business/kpi.md](../01-business/kpi.md) — chỉ số đo lường
- [../adr/0008-financial-ledger.md](../adr/0008-financial-ledger.md) — quyết định về ledger
