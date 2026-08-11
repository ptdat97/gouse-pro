# Chỉ số đo lường (KPI)

## Nguyên tắc

1. Mỗi chỉ số phải có **chủ sở hữu** (module nào cung cấp dữ liệu).
2. Mỗi chỉ số phải **hành động được** — nếu số xấu, biết phải làm gì.
3. Chỉ số phải tính được **bằng truy vấn**, không bằng xử lý thủ công.
4. Nếu một chỉ số quan trọng không tính được, đó là **lỗ hổng kiến trúc**.

---

## 1. Chỉ số Bắc Đẩu (North Star)

```text
GMV có lợi nhuận (Profitable GMV)
= Tổng giá trị hàng hóa giao dịch
  − hàng bị hoàn
  − chi phí thu hút khách
  − chi phí vận hành đơn hàng
```

Vì sao không dùng GMV thuần: GMV có thể tăng bằng cách bán lỗ hoặc chi mạnh cho marketing. GMV có lợi nhuận buộc mọi quyết định phải cân bằng tăng trưởng và hiệu quả.

---

## 2. Chỉ số khách hàng

| Chỉ số | Công thức | Chủ sở hữu dữ liệu | Hành động khi xấu |
|---|---|---|---|
| GMV | Σ giá trị đơn hoàn tất | order | Xem phễu chuyển đổi |
| AOV | GMV / số đơn | order | Đẩy bán chéo, outfit |
| Số khách hoạt động | Khách có đơn trong kỳ | order, customer | Chiến dịch kích hoạt lại |
| Tỷ lệ mua lại | Khách mua ≥ 2 lần / tổng khách | order | Loyalty, chất lượng sản phẩm |
| CLV | Lợi nhuận gộp trung bình × vòng đời | order, payment | Cải thiện giữ chân |
| CAC | Chi phí marketing / khách mới | campaign, affiliate | Tối ưu kênh |
| Tỷ lệ CLV/CAC | CLV / CAC | tổng hợp | Cần > 3 để bền vững |
| Tỷ lệ chuyển đổi | Đơn / phiên truy cập | analytics | Xem lại UX, giá, tồn kho |
| Tỷ lệ bỏ giỏ | Giỏ bỏ / giỏ tạo | cart | Xem lại phí ship, checkout |

### Phễu chuyển đổi cần đo

```text
Lượt truy cập
    ↓  (tỷ lệ xem sản phẩm)
Xem sản phẩm
    ↓  (tỷ lệ thêm giỏ)
Thêm giỏ hàng
    ↓  (tỷ lệ vào checkout)
Bắt đầu checkout
    ↓  (tỷ lệ hoàn tất)
Đặt hàng
    ↓  (tỷ lệ thanh toán thành công)
Thanh toán thành công
    ↓  (tỷ lệ giao thành công)
Giao hàng thành công
    ↓  (1 − tỷ lệ hoàn)
Hoàn tất
```

Đo từng bước để biết chính xác chỗ rò rỉ. Đo tổng thể chỉ cho biết có vấn đề, không cho biết ở đâu.

---

## 3. Chỉ số đặc thù thời trang

Đây là các chỉ số mà ecommerce tổng quát không cần nhưng thời trang bắt buộc phải có.

| Chỉ số | Ý nghĩa | Ngưỡng tham khảo |
|---|---|---|
| **Tỷ lệ hoàn hàng** | Đơn bị hoàn / tổng đơn | < 20% |
| **Tỷ lệ hoàn do size** | Hoàn vì không vừa / tổng hoàn | Theo dõi để sửa bảng size |
| **Sell-through rate** | Đã bán / đã sản xuất theo mùa | > 80% cuối mùa |
| **Markdown rate** | Doanh thu giảm giá / tổng doanh thu | < 20% |
| **Full-price sell rate** | Bán giá gốc / tổng bán | > 70% |
| **Size availability** | SKU đủ size / tổng SKU | > 85% |
| **Collection performance** | Doanh thu thực / kế hoạch theo bộ sưu tập | > 90% |
| **Days of inventory** | Tồn kho / tốc độ bán trung bình | Theo mùa |

### Vì sao tỷ lệ hoàn hàng là chỉ số then chốt

Thời trang có tỷ lệ hoàn cao hơn hẳn các ngành khác vì khách không thử được trước khi mua. Mỗi lần hoàn hàng gây ra:

```text
Chi phí vận chuyển hai chiều
Chi phí kiểm định lại
Chi phí đóng gói lại
Rủi ro hàng không bán lại được
Doanh thu bị đảo ngược
Hoa hồng đã trả phải thu lại
```

**Phân tích lý do hoàn là bắt buộc**, không phải tùy chọn:

| Lý do hoàn | Hàm ý | Hành động |
|---|---|---|
| Không vừa size | Bảng size sai hoặc mô tả thiếu | Sửa bảng size, thêm ảnh số đo |
| Khác mô tả | Ảnh/mô tả gây hiểu nhầm | Sửa nội dung, cảnh cáo seller |
| Chất lượng kém | Lỗi sản xuất | Truy vết lô, làm việc với nhà cung cấp |
| Đổi ý | Bình thường | Không hành động |
| Giao sai hàng | Lỗi vận hành | Xem lại quy trình kho |

**Hệ quả kiến trúc:** `Return` phải có trường lý do **chuẩn hóa** (không phải văn bản tự do), và lý do này phải chảy ngược về module `catalog` (sửa mô tả), `quality` (truy vết lô), và `supply-chain` (điều chỉnh thiết kế).

---

## 4. Chỉ số marketplace

| Chỉ số | Ý nghĩa | Chủ sở hữu |
|---|---|---|
| Take rate | Doanh thu nền tảng / GMV | payment |
| Số seller hoạt động | Seller có đơn trong kỳ | seller, order |
| Assortment size | Số SKU đang bán | catalog, marketplace |
| Offer per product | Độ cạnh tranh mỗi sản phẩm | marketplace |
| Seller retention | Seller còn hoạt động sau N tháng | seller |
| Time to first sale | Từ lên sàn đến đơn đầu | seller, order |
| Tỷ lệ hủy do seller | Đơn seller hủy / tổng đơn seller | fulfillment |
| Tỷ lệ báo cáo hàng giả | Báo cáo / tổng offer | marketplace |
| Độ chính xác tồn kho seller | 1 − (hủy do hết hàng / tổng đơn) | inventory |

Hai chỉ số cuối là **chỉ số sức khỏe dài hạn**. Hàng giả phá hủy niềm tin thương hiệu; tồn kho ảo phá hủy niềm tin vào việc đặt hàng.

---

## 5. Chỉ số creator và nội dung

| Chỉ số | Ý nghĩa | Chủ sở hữu |
|---|---|---|
| Số creator hoạt động | Creator có nội dung/đơn trong kỳ | creator |
| Content-attributed GMV | GMV quy kết cho nội dung | affiliate |
| Tỷ lệ GMV từ nội dung | GMV nội dung / tổng GMV | affiliate, order |
| Content-to-click rate | Click / lượt xem nội dung | content |
| Click-to-purchase rate | Đơn / click | affiliate |
| Chi phí trên mỗi đơn từ creator | Hoa hồng / số đơn quy kết | affiliate |
| Outfit multi-item rate | Đơn mua ≥ 2 món trong outfit / đơn từ outfit | content, order |
| Return rate theo nội dung | Hoàn / đơn từ nội dung đó | content, return |
| Creator retention | Creator còn hoạt động sau N tháng | creator |

**Chỉ số so sánh quan trọng:**

```text
Chi phí trên mỗi đơn qua creator   vs   Chi phí trên mỗi đơn qua quảng cáo
```

Nếu kênh creator rẻ hơn, cần dồn nguồn lực vào đó. Đây là câu hỏi phân bổ ngân sách quan trọng nhất, và hệ thống phải trả lời được.

---

## 6. Chỉ số chuỗi cung ứng

| Chỉ số | Ý nghĩa | Mục tiêu |
|---|---|---|
| Forecast accuracy | 1 − |thực tế − dự báo| / thực tế | > 70% |
| On-time delivery (nhà cung cấp) | Lô giao đúng hạn / tổng lô | > 90% |
| Quality pass rate | Lô đạt QC lần đầu | > 95% |
| Inventory turnover | COGS / tồn kho trung bình | > 4 lần/năm |
| Stockout rate | SKU hết hàng / SKU đang bán | < 5% |
| Excess inventory rate | Tồn quá hạn mùa / tổng tồn | < 15% |
| Concept-to-shelf time | Ý tưởng → lên sàn | < 120 ngày |
| Cash-to-cash cycle | Ngày tồn kho + ngày phải thu − ngày phải trả | Càng ngắn càng tốt |

### Stockout rate — chỉ số bị đánh giá thấp

Hết hàng gây thiệt hại kép: mất doanh số **và** mất khách (khách đi mua nơi khác, có thể không quay lại).

Nhưng stockout không xuất hiện trong dữ liệu bán hàng — nó là **doanh số không xảy ra**. Vì vậy phải đo bằng cách khác:

```text
Sự kiện cần ghi nhận:
  - SKU chuyển sang trạng thái hết hàng (thời điểm)
  - Lượt xem sản phẩm khi đang hết hàng
  - Lượt đăng ký nhận thông báo có hàng
  - Tìm kiếm không ra kết quả
```

Đây là đầu vào của tín hiệu nhu cầu. Xem [supply-chain.md](supply-chain.md) mục 4.2.

---

## 7. Chỉ số vận hành và kỹ thuật

| Chỉ số | Ngưỡng đề xuất |
|---|---|
| Thời gian phản hồi API (p95) | < 300ms |
| Thời gian phản hồi API (p99) | < 1s |
| Tỷ lệ lỗi API | < 0,1% |
| Thời gian tải trang sản phẩm | < 2s |
| Uptime | > 99,9% |
| Tỷ lệ thanh toán thất bại do lỗi hệ thống | < 0,1% |
| Độ trễ đồng bộ tồn kho | < 5s |
| Độ lệch đối soát tài chính | 0 |

**Chỉ số cuối cùng phải luôn bằng không.** Bất kỳ độ lệch nào trong đối soát tài chính đều là lỗi nghiêm trọng cần điều tra ngay, không phải sai số chấp nhận được.

Xem [../09-operations/observability.md](../09-operations/observability.md).

---

## 8. Ma trận chỉ số theo giai đoạn

| Giai đoạn | Chỉ số trọng tâm |
|---|---|
| MVP | Tỷ lệ chuyển đổi, AOV, tỷ lệ hoàn, số seller hoạt động |
| Phase 2 | Tỷ lệ mua lại, GMV từ nội dung, hiệu suất seller |
| Phase 3 | Forecast accuracy, sell-through, inventory turnover |
| Phase 4 | CLV/CAC, take rate, doanh thu retail media |

Không đo tất cả từ đầu. Mỗi giai đoạn có câu hỏi kinh doanh riêng.

---

## 9. Yêu cầu kiến trúc cho việc đo lường

Để các chỉ số trên tính được, kiến trúc phải đảm bảo:

| Yêu cầu | Vì sao |
|---|---|
| Mọi sự kiện nghiệp vụ được ghi nhận với dấu thời gian | Tính chỉ số theo thời gian |
| Lý do hoàn hàng được chuẩn hóa | Phân tích nguyên nhân |
| Attribution được lưu đầy đủ chuỗi | Tính lại theo mô hình khác |
| COGS gắn với lô sản xuất | Tính biên lợi nhuận thật |
| Sự kiện hết hàng được ghi nhận | Đo nhu cầu bị bỏ lỡ |
| Ledger bất biến | Đối soát chính xác |
| Dữ liệu hành vi truy vấn được bởi supply chain | Bánh đà không đứt |

**Nguyên tắc:** ghi nhận dữ liệu thô đầy đủ ngay từ đầu, kể cả khi chưa dùng tới. Dữ liệu lịch sử không thể tạo ngược.

---

## 10. Tài liệu liên quan

- [monetization.md](monetization.md) — mô hình doanh thu
- [../04-modules/analytics.md](../04-modules/analytics.md) — module phân tích
- [../09-operations/observability.md](../09-operations/observability.md) — quan sát hệ thống
