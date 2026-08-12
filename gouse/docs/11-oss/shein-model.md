# Mô hình SHEIN — chuỗi cung ứng theo nhu cầu

| | |
|---|---|
| Loại | Nghiên cứu mô hình kinh doanh (không phải OSS) |
| Nguồn | Tài liệu công khai, báo cáo ngành, phân tích chuỗi cung ứng |
| Vai trò | **Mô hình tham chiếu cho miền chuỗi cung ứng của chúng ta** |

---

## 1. Vì sao nghiên cứu SHEIN thay vì một dự án OSS

Không có dự án mã nguồn mở nào mô hình hóa chuỗi cung ứng thời trang theo nhu cầu. Kết quả ma trận nghiên cứu cho thấy rõ:

```text
Supplier          8/9 năng lực chuỗi cung ứng phải TỰ XÂY
Manufacturing     không OSS nào có
Production batch  không OSS nào có
Quality control   không OSS nào có
Demand planning   không OSS nào có
```

Năng lực này thuộc về các nền tảng đóng. Chúng ta chỉ học được **mô hình nghiệp vụ**, không học được cách cài đặt.

---

## 2. Cơ chế cốt lõi: sản xuất lô nhỏ để thử nhu cầu

### Cách SHEIN làm

Sản phẩm mới ra mắt với lô **rất nhỏ** — theo các phân tích công khai, khoảng 100–200 sản phẩm mỗi mẫu (một số nguồn nêu 50–100).

Đây là **phép thử thị trường chi phí thấp**: đo phản ứng thật của khách trước khi cam kết nguồn lực lớn.

Hành vi mua và tương tác với lô đầu được theo dõi **thời gian thực**. Dữ liệu này quyết định: mẫu nào tăng sản lượng, mẫu nào dừng.

### Vì sao điều này thay đổi bài toán

Ngành thời trang truyền thống:

```text
Dự báo trước mùa → đặt sản xuất lớn → bán → phát hiện sai lầm khi đã muộn
                                            → xả hàng, mất 50–70% giá trị
```

Mô hình lô nhỏ:

```text
Sản xuất lô nhỏ → đo phản ứng thật → tăng sản lượng thứ bán được
                                    → dừng thứ không bán được
```

**Rủi ro tồn kho chuyển từ "dự đoán đúng" sang "phản ứng nhanh".**

### Điều kiện kỹ thuật để làm được

Ba điều kiện, tất cả đều là yêu cầu kiến trúc:

```text
1. Đo được nhu cầu THẬT, kể cả nhu cầu KHÔNG ĐƯỢC ĐÁP ỨNG
   → cần ghi nhận: tìm kiếm không ra kết quả, hết hàng, đăng ký nhận tin

2. Chu kỳ từ tín hiệu tới quyết định sản xuất phải NGẮN
   → cần dữ liệu nhu cầu truy vấn được bởi module supply-chain,
     không nằm trong công cụ analytics bên thứ ba

3. Nhà cung cấp chấp nhận MOQ thấp
   → ràng buộc thương mại, nhưng hệ thống phải hiển thị được mâu thuẫn
     giữa MOQ và dự báo
```

Cả ba đã có trong thiết kế của chúng ta:

| Điều kiện | Nơi đã xử lý |
|---|---|
| Đo nhu cầu bị bỏ lỡ | [04-modules/supply-chain.md](../04-modules/supply-chain.md) mục 4.3 |
| Ghi tín hiệu từ MVP | [10-roadmap/mvp.md](../10-roadmap/mvp.md) mục 2 |
| Hiển thị mâu thuẫn MOQ | [07-workflows/replenishment.md](../07-workflows/replenishment.md) mục 5 |

**SHEIN xác nhận ba quyết định này là đúng và cần thiết**, không phải cẩn thận thừa.

---

## 3. Vòng phản hồi dữ liệu

### Cách SHEIN làm

Hệ thống thu thập tín hiệu nhu cầu từ tương tác trên ứng dụng và website, kết hợp với dữ liệu tồn kho, đưa vào hệ thống quản lý sản xuất kết nối với mạng lưới nhà cung cấp.

Quy mô: hàng nghìn nhà máy gia công, hơn 10.000 mẫu mới mỗi ngày, thời gian giao tính bằng ngày thay vì tuần.

### Vòng lặp

```text
Khách xem, tìm, thêm giỏ, mua, đánh giá, trả hàng
        ↓
Tín hiệu nhu cầu (bao gồm nhu cầu KHÔNG được đáp ứng)
        ↓
Quyết định: tăng sản lượng / giữ nguyên / dừng
        ↓
Đơn sản xuất tới nhà máy
        ↓
Hàng về kho → bán → dữ liệu mới
        ↺
```

Đây **chính là** bánh đà đã mô tả trong [00-overview/vision.md](../00-overview/vision.md) mục 4.

### Điểm mấu chốt thường bị bỏ qua

Vòng lặp này đứt ở đâu thì mô hình sụp ở đó. Điểm đứt phổ biến nhất:

```text
Dữ liệu hành vi nằm trong công cụ analytics bên thứ ba
    → module supply-chain KHÔNG truy vấn được
    → quyết định sản xuất quay lại dựa vào kinh nghiệm
    → mất toàn bộ lợi thế
```

Đây là lý do [02-domain/domain-map.md](../02-domain/domain-map.md) xếp `Supply Chain` vào **Core Domain**, và lý do `demand_signal` phải ghi từ MVP dù Phase 3 mới dùng.

---

## 4. Điều chúng ta lấy

### Adopt

**1. Tín hiệu nhu cầu bao gồm nhu cầu không được đáp ứng.**

Nếu chỉ nhìn doanh số, hệ thống sẽ **liên tục sản xuất thiếu** hàng bán chạy:

```text
Chỉ nhìn doanh số:  "bán 200 chiếc" → nhu cầu 200
Thực tế:            hết hàng từ tuần 3
                    1.500 lượt tìm kiếm sau khi hết
                    400 lượt đăng ký nhận thông báo
                    → nhu cầu thật gần 800
```

Đã có trong thiết kế. SHEIN xác nhận đây là yếu tố quyết định.

**2. Lô sản xuất là đơn vị truy vết.**

Sản xuất lô nhỏ chỉ có nghĩa nếu **đo được kết quả từng lô**: lô nào bán chạy, lô nào chất lượng kém, lô nào giá vốn cao.

Đã có: `ProductionBatch` với `unit_cost` riêng theo lô.

**3. Phân bổ theo size là quyết định hạng nhất.**

Lô 500 chiếc không phải 500 chiếc giống nhau. Phân bổ sai gây thiệt hại kép: hết size bán chạy **và** tồn size ế.

Đã có: kế hoạch sản xuất ở mức SKU (bao gồm size).

### Adapt

**Tốc độ phù hợp với quy mô của chúng ta.**

SHEIN có hàng nghìn nhà máy và chu kỳ tính bằng ngày. Chúng ta bắt đầu với vài nhà cung cấp và chu kỳ tính bằng tuần.

Điều lấy được **không phải tốc độ** mà là **cấu trúc quyết định**:

```text
✓ Đo nhu cầu → quyết định sản xuất  (cấu trúc)
✗ Chu kỳ 3 ngày                      (tốc độ, phụ thuộc quy mô)
```

Kiến trúc phải cho phép rút ngắn chu kỳ khi quy mô tăng, nhưng không giả định tốc độ đó ngay từ đầu.

### Reject

**Không sao chép quy mô hay tần suất ra mắt sản phẩm.**

10.000 mẫu mới mỗi ngày đòi hỏi năng lực vận hành mà chúng ta không có và không cần. Sao chép con số này sẽ dẫn tới thiết kế quá phức tạp cho nhu cầu thật.

Nguyên tắc P15 áp dụng: mỗi quyết định phải giải thích được vì sao cần cho **chính** nghiệp vụ của chúng ta.

---

## 5. Rủi ro của mô hình cần lưu ý

Nghiên cứu này không nên bỏ qua mặt trái. Mô hình lô nhỏ tốc độ cao bị phê phán về:

```text
Điều kiện lao động trong chuỗi cung ứng
Tác động môi trường của thời trang nhanh
Vấn đề sở hữu trí tuệ với thiết kế
```

### Hệ quả kiến trúc

Đây không chỉ là vấn đề đạo đức — nó có hệ quả kỹ thuật trực tiếp:

```text
Nhiều thị trường yêu cầu TRUY XUẤT NGUỒN GỐC chuỗi cung ứng
    → cần lưu chứng nhận nhà cung cấp và HẠN của chúng
    → cần truy vết từ sản phẩm → lô → nhà máy → nguyên liệu
```

Đã có trong thiết kế:

```text
supplier_certification với valid_until  → cảnh báo trước khi hết hạn
ProductionBatch.material_batch_refs     → truy vết nguyên liệu
```

Đây là ví dụ tốt cho thấy yêu cầu tuân thủ và yêu cầu vận hành trùng nhau: cùng dữ liệu phục vụ cả việc thu hồi hàng lỗi lẫn báo cáo truy xuất nguồn gốc.

---

## 6. Tổng kết

| Hạng mục | Quyết định |
|---|---|
| Sản xuất lô nhỏ thử nhu cầu | **ADOPT** — cấu trúc quyết định |
| Tín hiệu nhu cầu gồm nhu cầu bị bỏ lỡ | **ADOPT** — đã có, được xác nhận |
| Lô sản xuất là đơn vị truy vết | **ADOPT** — đã có |
| Phân bổ theo size ở mức SKU | **ADOPT** — đã có |
| Vòng phản hồi dữ liệu khép kín | **ADOPT** — bánh đà, đã có |
| Chu kỳ tính bằng ngày | **ADAPT** — cấu trúc trước, tốc độ sau |
| Quy mô hàng nghìn nhà máy | **REJECT** — không phù hợp giai đoạn |
| Truy xuất nguồn gốc | **ADOPT** — vừa tuân thủ vừa vận hành |

**Nhận xét cuối:** SHEIN chủ yếu **xác nhận** thiết kế chuỗi cung ứng đã có. Giá trị lớn nhất của nghiên cứu này là bằng chứng rằng ba quyết định gây tranh cãi nhất — ghi `demand_signal` từ MVP, đo nhu cầu bị bỏ lỡ, giá vốn theo lô — là **điều kiện cần** của mô hình, không phải cẩn thận thừa.

---

## 7. Tài liệu liên quan

- [../01-business/supply-chain.md](../01-business/supply-chain.md)
- [../04-modules/supply-chain.md](../04-modules/supply-chain.md)
- [../07-workflows/replenishment.md](../07-workflows/replenishment.md)
- [../00-overview/vision.md](../00-overview/vision.md) mục 4

## 8. Nguồn

- [Shein supply-chain-as-a-service (Sourcing Journal)](https://sourcingjournal.com/topics/logistics/shein-supply-chain-as-a-service-small-batch-on-demand-manufacturing-brands-designers-ipo-fast-fashion-production-500924/)
- [Chinese digital platforms reshaping garment manufacturing (ODI)](https://odi.org/en/insights/the-chinese-digital-platforms-reshaping-garment-manufacturing/)
- [How Shein Built a Real-Time Supply Chain (Logistics Navigators)](https://www.logisticsnavigators.com/businessbreakdowns/sheins-supply-chain-how-real-time-logistics-beat-traditional-fast-fashion)
