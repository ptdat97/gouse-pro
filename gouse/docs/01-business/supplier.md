# Tác nhân: Nhà cung cấp (Supplier)

## 1. Phân loại

```text
Supplier              — Nhà cung cấp hàng thành phẩm
Manufacturer          — Nhà máy gia công theo thiết kế của nền tảng
Material Supplier     — Nhà cung cấp nguyên phụ liệu (vải, cúc, khóa, nhãn mác)
Production Partner    — Đối tác sản xuất chiến lược, tham gia từ khâu phát triển mẫu
```

### Bảng phân biệt

| | Supplier | Manufacturer | Material Supplier | Production Partner |
|---|---|---|---|---|
| Cung cấp cái gì | Hàng thành phẩm có sẵn | Gia công theo tech pack | Nguyên phụ liệu | Thành phẩm + phát triển mẫu |
| Nền tảng cung cấp thiết kế | Không | Có | Không | Cùng phát triển |
| Sở hữu trí tuệ thiết kế | Nhà cung cấp | Nền tảng | Không áp dụng | Thỏa thuận |
| Đơn vị đặt hàng | Purchase Order | Production Order | Purchase Order | Production Order |
| MOQ điển hình | Thấp | Cao | Rất cao | Trung bình |
| Lead time | Ngắn (2–4 tuần) | Dài (6–12 tuần) | Dài | Trung bình |
| Vai trò trong own brand | Bổ trợ | Cốt lõi | Cốt lõi | Cốt lõi |

**Phân biệt quan trọng nhất:** `Purchase Order` (mua hàng có sẵn) khác `Production Order` (đặt sản xuất theo thiết kế). Hai luồng nghiệp vụ khác nhau:

```text
Purchase Order:
  Đặt hàng → Nhà cung cấp giao → Nhập kho → Kiểm hàng → Bán

Production Order:
  Tech pack → Làm mẫu → Duyệt mẫu → Đặt nguyên liệu → Sản xuất
  → QC tại xưởng → Giao → Nhập kho → QC nhập kho → Bán
```

Luồng thứ hai dài hơn nhiều và có nhiều điểm có thể thất bại. Nếu mô hình hóa cả hai bằng một khái niệm "đơn đặt hàng nhà cung cấp" chung, sẽ mất khả năng quản lý các bước riêng của sản xuất. Xem [../07-workflows/supplier-production.md](../07-workflows/supplier-production.md).

---

## 2. Trách nhiệm

- Cung cấp hàng đúng thông số kỹ thuật đã thỏa thuận
- Giao đúng thời hạn cam kết
- Đảm bảo chất lượng theo tiêu chuẩn đã thống nhất (AQL)
- Cung cấp chứng từ: hóa đơn, chứng nhận xuất xứ, chứng nhận chất liệu
- Tuân thủ tiêu chuẩn lao động và môi trường (yêu cầu ngày càng quan trọng với thời trang)
- Bảo mật thiết kế của nền tảng (với manufacturer và production partner)

**Về tuân thủ tiêu chuẩn lao động:** đây không chỉ là vấn đề đạo đức — nhiều thị trường xuất khẩu yêu cầu truy xuất nguồn gốc chuỗi cung ứng. Hệ thống nên lưu được chứng nhận và ngày hết hạn của chúng, hỗ trợ báo cáo truy xuất khi cần.

---

## 3. Quyền hạn

Nhà cung cấp là tác nhân **bên ngoài**, tương tác chủ yếu qua quy trình nghiệp vụ chứ không phải giao diện tự phục vụ đầy đủ.

| Hành động | Điều kiện | Giai đoạn hỗ trợ |
|---|---|---|
| Nhận đơn mua/đơn sản xuất | Được kích hoạt | Phase 3 |
| Xác nhận đơn | Đơn ở trạng thái chờ xác nhận | Phase 3 |
| Cập nhật tiến độ sản xuất | Đơn đang sản xuất | Phase 3 |
| Tải lên chứng từ | Luôn | Phase 3 |
| Xem lịch sử đơn của mình | Luôn | Phase 3 |
| Xem dữ liệu bán hàng | **Không** | — |
| Xem thông tin khách hàng | **Không bao giờ** | — |

**Quyết định lộ trình:** ở MVP và Phase 2, tương tác với nhà cung cấp có thể xử lý ngoài hệ thống (email, bảng tính). Cổng thông tin nhà cung cấp (supplier portal) chỉ cần thiết ở Phase 3 khi số lượng đơn sản xuất đủ lớn.

Tuy nhiên, **mô hình dữ liệu** phải được thiết kế đúng ngay từ đầu để dữ liệu sản xuất nhập tay ở giai đoạn sớm vẫn dùng được về sau.

---

## 4. Quan hệ doanh thu

Nhà cung cấp là **chi phí**, không phải doanh thu. Nhưng cách quản lý chi phí này quyết định biên lợi nhuận của own brand.

```text
Giá vốn một SKU own brand:

  Chi phí nguyên liệu              45.000đ
  Chi phí gia công                 35.000đ
  Chi phí phụ liệu (nhãn, bao bì)   8.000đ
  Chi phí vận chuyển nhập           7.000đ
  Chi phí QC                        3.000đ
  Hao hụt sản xuất (2%)             2.000đ
  ─────────────────────────────────────────
  COGS                            100.000đ
```

**Yêu cầu kiến trúc quan trọng:** COGS phải gắn với **lô sản xuất cụ thể** (`Production Batch`), không phải với SKU nói chung.

Lý do: hai lô cùng SKU sản xuất cách nhau ba tháng có giá vốn khác nhau (giá vải đổi, tỷ giá đổi, số lượng đặt khác nhau). Nếu chỉ lưu một giá vốn duy nhất cho SKU, mọi tính toán biên lợi nhuận đều sai.

Hệ quả: khi bán một sản phẩm, hệ thống phải biết được đơn vị hàng đó thuộc lô nào (hoặc dùng phương pháp tính giá vốn nhất quán như FIFO). Xem [../04-modules/manufacturing.md](../04-modules/manufacturing.md) và [../04-modules/warehouse.md](../04-modules/warehouse.md).

### Điều khoản thanh toán

```text
Đặt cọc         — thường 30% khi đặt sản xuất
Thanh toán giữa — có thể có khi hoàn thành nguyên liệu
Thanh toán cuối — sau khi giao hàng và QC đạt
```

Các khoản này phải được ghi vào ledger như mọi giao dịch tài chính khác. Xem [../adr/0008-financial-ledger.md](../adr/0008-financial-ledger.md).

---

## 5. Vòng đời

```text
   Tiềm năng (Prospect)
        │ (đánh giá năng lực, nhà xưởng, mẫu thử)
        ▼
   Đang đánh giá (Under Evaluation)
        │
        ├──→ Loại (Rejected)
        │
        ▼
   Đã phê duyệt (Approved)
        │
        ▼
   Đang hợp tác (Active) ◄────────┐
        │                          │
        ├──→ Theo dõi (Watch List) ┘  (chất lượng hoặc tiến độ có vấn đề)
        │
        ├──→ Tạm dừng (Suspended)
        │
        └──→ Chấm dứt (Terminated)
```

### Đánh giá năng lực nhà cung cấp

Trước khi phê duyệt, cần ghi nhận:

```text
Năng lực sản xuất (số lượng/tháng)
Chủng loại sản phẩm làm được
MOQ
Lead time điển hình
Tiêu chuẩn chất lượng đạt được
Chứng nhận (ISO, tiêu chuẩn lao động, môi trường)
Kết quả kiểm tra nhà xưởng
Đánh giá mẫu thử
```

### Chỉ số hiệu suất nhà cung cấp

| Chỉ số | Ý nghĩa |
|---|---|
| On-time delivery rate | Tỷ lệ giao đúng hạn |
| Quality pass rate | Tỷ lệ lô đạt QC lần đầu |
| Defect rate | Tỷ lệ sản phẩm lỗi |
| Cost variance | Chênh lệch giá thực tế so với báo giá |
| Sample approval cycles | Số vòng làm mẫu trung bình trước khi duyệt |

Chỉ số cuối cùng đặc biệt quan trọng với thời trang: nhà cung cấp cần 5 vòng làm mẫu sẽ làm chậm toàn bộ lịch ra mắt bộ sưu tập.

---

## 6. Rủi ro chuỗi cung ứng cần mô hình hóa

| Rủi ro | Hệ quả | Cách hệ thống hỗ trợ |
|---|---|---|
| Nhà cung cấp giao trễ | Lỡ mùa bán, hàng mất giá trị | Theo dõi tiến độ theo mốc, cảnh báo sớm |
| Chất lượng không đạt | Phải làm lại, trễ thêm | Bản ghi QC, lịch sử lỗi theo nhà cung cấp |
| Phụ thuộc một nhà cung cấp | Rủi ro tập trung | Báo cáo tỷ trọng theo nhà cung cấp |
| Giá nguyên liệu biến động | Biên lợi nhuận giảm | COGS theo lô, theo dõi chênh lệch |
| MOQ cao hơn nhu cầu thật | Tồn kho dư | Kết nối MOQ vào quyết định planning |

Rủi ro cuối cùng là mâu thuẫn cốt lõi của own brand thời trang: nhà máy đòi đặt tối thiểu 500 chiếc, dự báo chỉ bán được 300. Hệ thống phải hiển thị rõ mâu thuẫn này ở bước lập kế hoạch, không để phát hiện sau khi hàng đã về kho.

---

## 7. Luồng nghiệp vụ chính

| Luồng | Tài liệu |
|---|---|
| Đặt sản xuất và theo dõi | [../07-workflows/supplier-production.md](../07-workflows/supplier-production.md) |
| Bổ sung hàng | [../07-workflows/replenishment.md](../07-workflows/replenishment.md) |
| Tạo sản phẩm own brand | [../07-workflows/own-brand-product.md](../07-workflows/own-brand-product.md) |

---

## 8. Dữ liệu sở hữu

Module [supply-chain](../04-modules/supply-chain.md) và các module con sở hữu:

```text
supplier                    — hồ sơ nhà cung cấp
supplier_capability         — năng lực sản xuất
supplier_certification      — chứng nhận và hạn dùng
supplier_performance        — chỉ số hiệu suất
supplier_contact            — đầu mối liên hệ
```

Module [procurement](../04-modules/procurement.md):

```text
purchase_order              — đơn mua hàng
purchase_order_line         — dòng hàng
supplier_quotation          — báo giá
```

Module [manufacturing](../04-modules/manufacturing.md):

```text
production_order            — đơn sản xuất
production_batch            — lô sản xuất
tech_pack                   — hồ sơ kỹ thuật
sample                      — mẫu và kết quả duyệt mẫu
bill_of_materials           — định mức nguyên phụ liệu
```
