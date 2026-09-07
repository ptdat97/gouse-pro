# Các giai đoạn sau MVP — Phase 2, 3, 4

```text
FUTURE — KHÔNG TRIỂN KHAI TRONG GIAI ĐOẠN NÀY
```

> **Architecture Freeze (15/08/2026):** tài liệu này giữ nguyên làm **thiết
> kế tham chiếu**, nhưng không được triển khai cho tới khi Commerce Core
> chạy end-to-end.
>
> **Mười hai module dưới đây chưa tồn tại trong code** và không được tạo
> mới: `creator` · `content` · `affiliate` · `loyalty` · `recommendation`
> · `campaign` · `return` · `warehouse` · `quality` · `procurement` ·
> `manufacturing` · `supplier`. Hai mươi thao tác thuộc các giai đoạn này
> đã có trong OpenAPI vẫn ở mức `DESIGNED` — xem
> [../../gouse/api/README.md](../../gouse/api/README.md).
>
> `supplychain` LÀ ngoại lệ đã tồn tại: nó chỉ GHI `demand_signal`, không
> suy luận gì. Phần dự báo nhu cầu vẫn thuộc giai đoạn sau.
>
> **Trạng thái 20/08/2026: Commerce Core đã chạy end-to-end.** Nhưng điều
> kiện mở khóa các giai đoạn này KHÔNG phải "chạy được" mà là "chịu được
> điều kiện thật" — dự án đang ở phase **Production Hardening**, và trong
> phase đó danh sách trên bị KHÓA cứng. Lý do ở
> [backlog.md mục 6](backlog.md).
>
> Việc đang làm: [backlog.md mục 2](backlog.md).

---

## 1. Tổng quan

| Giai đoạn | Chủ đề | Module thêm mới |
|---|---|---|
| **Phase 2** | Creator Commerce và hoàn thiện vận hành | 7 |
| **Phase 3** | Chuỗi cung ứng | 5 |
| **Phase 4** | Nâng cấp chiều sâu | 0 |

### Vì sao Creator Commerce trước chuỗi cung ứng

```text
Creator commerce (Phase 2):
    - Tạo nhu cầu → tăng doanh số ngay
    - Chi phí thấp hơn quảng cáo
    - Sinh dữ liệu hành vi cho chuỗi cung ứng sau này

Chuỗi cung ứng (Phase 3):
    - Cần dữ liệu nhu cầu tích lũy (đã ghi từ MVP)
    - Chu kỳ dài, không tạo doanh thu ngay
    - Chỉ có giá trị khi đã có quy mô
```

### Ngoại lệ quan trọng — làm ngay từ MVP

Việc **ghi** tín hiệu nhu cầu phải làm từ MVP (P2-1 trong
[backlog.md](backlog.md)), dù toàn bộ phần dự báo thuộc Phase 3.

Dữ liệu này **không tạo ngược được**. Không ghi từ hôm nay thì Phase 3 khởi
động với lịch sử trống, và mọi tính toán bổ sung hàng đều vô nghĩa. Module
`supply-chain` hiện có CHỈ làm việc này — đó là toàn bộ phạm vi MVP của nó,
và không được mở rộng thêm.

### Nguyên tắc chung về hạ tầng

Áp dụng cho cả ba giai đoạn:

```text
Thêm KHI ĐO ĐƯỢC nhu cầu, không theo lịch.

Cache                     → khi có điểm nóng đo được, database tải cao
Chỉ mục tìm kiếm riêng    → khi tìm kiếm SQL chậm hoặc thiếu lọc phức tạp
Bản sao chỉ đọc           → khi truy vấn báo cáo ảnh hưởng giao dịch
Lưu trữ chuỗi thời gian   → khi ghi analytics ảnh hưởng database chính
Tách service              → CHỈ khi có lý do đo được, theo ADR-0009
```

---

## 2. Phase 2 — Creator Commerce và hoàn thiện vận hành

> **Mục tiêu:** kích hoạt **động cơ tạo nhu cầu** (creator/nội dung) và hoàn
> thiện các quy trình vận hành mà MVP xử lý thủ công.

Đây là giai đoạn nền tảng bắt đầu khác biệt so với một website thương mại
điện tử thông thường.

### 2.1 Module thêm mới (7)

| Module | Lý do ở Phase 2 |
|---|---|
| `creator` | Danh tính creator |
| `content` | Nội dung, outfit, product tag |
| `affiliate` | Link, click, quy kết, hoa hồng |
| `campaign` | Chiến dịch với ba cấu trúc chi phí |
| `recommendation` | Gợi ý bằng quy tắc đơn giản |
| `return` | Quy trình hoàn hàng đầy đủ |
| `warehouse` | Vận hành kho, nhập hàng, kiểm kê |

### 2.2 Vì sao `return` ở Phase 2, không sớm hơn

```text
MVP xử lý thủ công được vì khối lượng nhỏ.

Phase 2 bắt buộc phải có vì:
    - Khối lượng đơn tăng, xử lý tay không kịp
    - Creator commerce làm tăng đơn từ khách mới → tỷ lệ hoàn cao hơn
    - Cần dữ liệu lý do hoàn chuẩn hóa để cải thiện sản phẩm
```

### 2.3 Phạm vi chi tiết

**Creator và nội dung**

```text
✓ Đăng ký creator, duyệt hồ sơ, xác minh kênh mạng xã hội
✓ Tạo nội dung: video, ảnh, lookbook, bài viết, OUTFIT
✓ Gắn sản phẩm vào nội dung (có vị trí trên ảnh/video)
✓ Kiểm duyệt nội dung (tự động + thủ công)
✓ Nhãn "Được tài trợ" TỰ ĐỘNG
✓ Feed khám phá
✓ Sản phẩm hết hàng trong nội dung → hiển thị thay thế
```

**Affiliate**

```text
✓ Tạo affiliate link
✓ Ghi click bất đồng bộ (không làm chậm chuyển hướng)
✓ LƯU TOÀN BỘ CHUỖI CLICK (không chỉ click cuối)
✓ Quy kết last-click, cửa sổ 7 ngày
✓ Hoa hồng creator, đóng băng tỷ lệ vào Attribution
✓ Đảo ngược khi hoàn hàng
✓ Đối soát và chi trả creator
```

**Điểm quan trọng:** dù dùng last-click, phải lưu đủ chuỗi click để sau này
đổi mô hình quy kết mà vẫn tính lại được dữ liệu quá khứ.

**Chiến dịch**

```text
✓ Ba cấu trúc chi phí: COMMISSION_ONLY, FIXED_FEE, HYBRID
✓ Xác định bên chịu chi phí (PLATFORM / SELLER / SHARED)
✓ Quản lý ngân sách, tự dừng khi hết
✓ Mời creator tham gia
```

**Trả hàng**

```text
✓ Yêu cầu trả hàng với LÝ DO CHUẨN HÓA
✓ Duyệt (tự động một số trường hợp)
✓ Nhận hàng, kiểm định
✓ Nhập lại kho theo kết quả kiểm định
✓ Hoàn tiền theo GIÁ THỰC TRẢ (sau phân bổ giảm giá)
✓ Đảo ngược ĐỦ chuỗi: hoa hồng NT, số dư seller, hoa hồng creator
✓ Ghi lịch sử size vào hồ sơ khách
```

**Kho**

```text
✓ Nhiều địa điểm lưu kho
✓ Quy trình nhập hàng
✓ Lấy hàng, đóng gói (quét mã xác nhận)
✓ Kiểm kê
✓ Khu vực riêng cho hàng hoàn
✓ Hàng ký gửi của seller (PLATFORM_SERVICE)
```

**Gợi ý**

```text
✓ Sản phẩm tương tự (cùng danh mục, khoảng giá, còn hàng)
✓ "Complete the look" — LẤY TỪ DỮ LIỆU OUTFIT
✓ Xu hướng (doanh số gần đây có trọng số thời gian)
✓ Cá nhân hóa cơ bản (danh mục, thương hiệu đã mua/xem)
✓ LỌC THEO SIZE khách mặc
```

**Lưu ý:** "Complete the look" dùng dữ liệu `Outfit` do stylist tạo — chất
lượng cao mà không cần thuật toán phức tạp. Đây là ví dụ tốt của nguyên tắc
P14.

### 2.4 Nâng cấp module có sẵn

| Module | Bổ sung ở Phase 2 |
|---|---|
| `customer` | Dữ liệu size, gợi ý size, gộp danh tính guest |
| `inventory` | Nhiều địa điểm, chuyển kho, xử lý hàng hoàn |
| `fulfillment` | Nhiều kho, phân bổ nguồn hàng, nhiều đối tác vận chuyển, xử lý giao thất bại |
| `payment` | Đối soát tự động, payout tự động, hoàn tiền |
| `seller` | Chấm điểm hiệu suất, chính sách riêng |
| `marketplace` | Buy box đầy đủ, kiểm soát thương hiệu bảo vệ |
| `promotion` | Khuyến mãi tự động, phân bổ chi phí, khuyến mãi của seller |
| `product` | Chống trùng lặp, quy trình gộp sản phẩm |
| `catalog` | Bộ sưu tập, ủy quyền thương hiệu |
| `analytics` | Phễu chuyển đổi, chỉ số creator, dashboard seller |
| `notification` | Nhiều kênh (SMS, push), nhắc giỏ bỏ quên |

### 2.5 Vòng lặp bánh đà bắt đầu quay

Đây là điểm quan trọng nhất của Phase 2:

```text
Creator tạo nội dung
    ↓
Khách xem, click, thêm giỏ, mua
    ↓
Dữ liệu hành vi được ghi:
    - content.viewed
    - affiliate.click_recorded
    - cart.item_added (có source_content_id)
    - order.placed
    - inventory.depleted
    - return.inspected (kèm lý do)
    ↓
demand_signal tích lũy
    ↓
(Phase 3 sẽ dùng để lập kế hoạch sản xuất)
```

Cuối Phase 2, nền tảng có **dữ liệu nhu cầu thật** để bước vào Phase 3.

### 2.6 Tiêu chí hoàn thành

**Chức năng**

```text
✓ Creator đăng nội dung, gắn sản phẩm, tạo affiliate link
✓ Khách mua qua nội dung → quy kết đúng creator
✓ Hoa hồng creator tính đúng, đảo ngược đúng khi hoàn hàng
✓ Khách yêu cầu trả hàng qua hệ thống, không cần liên hệ thủ công
✓ Hàng hoàn qua kiểm định trước khi nhập lại kho
✓ Đối soát và chi trả seller/creator tự động theo chu kỳ
```

**Chất lượng**

```text
✓ Độ trễ chuyển hướng affiliate link < 50ms
✓ Tỷ lệ quy kết bị đảo ngược trong ngưỡng
✓ Chuỗi đảo ngược tài chính khi hoàn hàng ĐỦ 100%
✓ Không có nội dung dẫn tới trang lỗi khi sản phẩm hết
```

**Dữ liệu**

```text
✓ demand_signal đã tích lũy đủ để phân tích (tối thiểu 6 tháng)
✓ Lý do hoàn hàng chuẩn hóa, phân tích được
✓ Toàn bộ chuỗi click được lưu (không chỉ click quy kết)
```

### 2.7 Rủi ro chính

| Rủi ro | Giảm thiểu |
|---|---|
| Creator không tin số liệu quy kết | Minh bạch từng dòng, giải thích rõ mô hình last-click |
| Gian lận click | Ghi đủ ngữ cảnh, phát hiện bất đồng bộ |
| Chuỗi đảo ngược hoàn hàng thiếu bước | Kiểm tra tự động: mọi `return.refunded` phải sinh đủ N bút toán |
| Nội dung mô tả sai lệch để bán hàng | Theo dõi tỷ lệ hoàn theo nội dung |
| Bảng `click` phình quá nhanh | Phân vùng theo ngày, chính sách lưu trữ 90 ngày |

---

## 3. Phase 3 — Chuỗi cung ứng

> **Mục tiêu:** kích hoạt **lợi thế cạnh tranh dài hạn** — biến dữ liệu nhu
> cầu tích lũy từ MVP và Phase 2 thành năng lực sản xuất đúng thứ, đúng
> lượng, đúng lúc.

Đây là giai đoạn nền tảng trở thành thứ không sao chép được.

### 3.1 Module thêm mới (5)

| Module | Vai trò |
|---|---|
| `supply-chain` | Tín hiệu nhu cầu, dự báo, lập kế hoạch, phát triển sản phẩm |
| `procurement` | Nhà cung cấp, đơn mua hàng |
| `manufacturing` | Đơn sản xuất, lô sản xuất, giá vốn |
| `quality` | Kiểm định chất lượng |
| `loyalty` | Điểm thưởng, hạng thành viên |

### 3.2 Điều kiện tiên quyết

Phase 3 chỉ khả thi nếu Phase 1–2 đã làm đúng:

```text
✓ demand_signal đã tích lũy tối thiểu 12 tháng
  → cần dữ liệu cùng kỳ năm trước để so sánh mùa vụ

✓ Dữ liệu bán hàng theo SIZE đầy đủ
  → phân bổ size là quyết định tốn tiền nhất

✓ Lý do hoàn hàng chuẩn hóa
  → biết size nào bị trả vì chật/rộng để sửa thiết kế

✓ Inventory có trạng thái đầy đủ và chính xác
  → nếu tồn kho sai, mọi tính toán bổ sung đều sai
```

**Nếu chưa đủ dữ liệu:** làm phần thực thi (procurement, manufacturing,
quality) trước, phần thông minh (dự báo, lập kế hoạch) sau.

### 3.3 Phạm vi chi tiết

**Tín hiệu nhu cầu và dự báo**

```text
✓ Tổng hợp demand_signal theo SKU / theo tuần
✓ Chuẩn hóa (loại ảnh hưởng khuyến mãi, mùa vụ)
✓ Dự báo bằng QUY TẮC và trung bình có trọng số
✓ Đo độ chính xác dự báo

KHÔNG dùng học máy ở giai đoạn này (nguyên tắc P14)
→ interface DemandSignalProvider thiết kế sẵn để thay sau
```

**Phát triển sản phẩm own brand**

```text
✓ ProductDevelopment: concept → design → tech pack → costing
                      → sampling → duyệt mẫu
✓ Tech pack với thông số, số đo theo size, định mức nguyên liệu
✓ Quản lý vòng làm mẫu (theo dõi sample_approval_cycles)
✓ Kiểm soát giá vốn mục tiêu ở bước costing
✓ Anti-Corruption Layer → tạo Product trong Catalog khi duyệt mẫu
```

**Lập kế hoạch sản xuất**

```text
✓ Kế hoạch ở mức SKU (bao gồm PHÂN BỔ SIZE)
✓ Đưa MOQ và lead time vào quyết định
✓ HIỂN THỊ MÂU THUẪN MOQ vs dự báo, kèm ước tính tài chính
✓ Ràng buộc mùa vụ (không sản xuất khi hàng về sẽ trễ mùa)
```

**Thu mua và sản xuất**

```text
✓ Hồ sơ nhà cung cấp, năng lực, chứng nhận (theo dõi hạn)
✓ Purchase Order (mua hàng có sẵn)
✓ Production Order (đặt sản xuất theo tech pack)
✓ ProductionBatch với GIÁ VỐN THEO LÔ
✓ Truy vết nguyên liệu (cho kịch bản thu hồi)
✓ Theo dõi tiến độ theo mốc, cảnh báo khi đe dọa ngày ra mắt
✓ Chấm điểm hiệu suất nhà cung cấp
```

**Kiểm định chất lượng**

```text
✓ Năm điểm kiểm: duyệt mẫu, inline, final, nhập kho, hàng hoàn
✓ Kiểm mẫu theo AQL
✓ Phân loại lỗi chuẩn hóa (CRITICAL/MAJOR/MINOR)
✓ Ảnh chứng minh BẮT BUỘC với mọi lỗi
✓ Xử lý lô không đạt: làm lại / giảm giá / trả hàng
```

**Bổ sung hàng**

```text
✓ Tính điểm đặt hàng lại (reorder point)
✓ Đề xuất bổ sung theo SIZE
✓ Hiển thị tín hiệu nhu cầu BỊ BỎ LỠ (stockout, tìm không ra kết quả)
✓ Ràng buộc mùa vụ
✓ HỆ THỐNG ĐỀ XUẤT, CON NGƯỜI QUYẾT ĐỊNH
```

### 3.4 Nguyên tắc thiết kế quan trọng nhất

> **Hệ thống đề xuất, con người quyết định.**

```text
KHÔNG tự động đặt hàng.

Lý do: một lỗi tính toán có thể dẫn tới đơn sản xuất sai
       hàng trăm triệu đồng.

Rủi ro quá lớn so với lợi ích tiết kiệm vài phút thao tác.
```

Hệ thống hiển thị:

```text
✓ Tín hiệu nhu cầu đầy đủ (kể cả nhu cầu bị bỏ lỡ)
✓ Mâu thuẫn giữa ràng buộc (MOQ, lead time, mùa vụ) và dự báo
✓ Ước tính tài chính của TỪNG phương án
```

Đây là **phần mềm hỗ trợ ra quyết định**, không phải phần mềm ghi chép
quyết định.

### 3.5 Bánh đà hoàn chỉnh

Cuối Phase 3, vòng lặp khép kín:

```text
Customer → Discovery → Content/Creator → Purchase
    → Behavior Data → Demand Signal → Product Planning
    → Own Brand/Supplier → Production → Inventory → Sales
    → More Data ↺
```

**Ví dụ vòng lặp thực tế**

```text
1. Video creator về áo khoác dạ oversize: 50.000 lượt xem
2. 3.000 click, 850 thêm wishlist
3. Sản phẩm chỉ còn size S → inventory.depleted
4. 240 lượt tìm kiếm không ra kết quả
5. demand_signal tổng hợp: nhu cầu ~800, cung 200
6. Đề xuất: sản xuất 800, phân bổ nhiều M và L
7. Sản xuất, QC, nhập kho
8. Bán → dữ liệu mới → vòng lặp tiếp
```

Trước Phase 3, bước 5–7 không tồn tại — bánh đà bị đứt.

### 3.6 Nâng cấp module có sẵn

| Module | Bổ sung |
|---|---|
| `inventory` | Truy vết theo lô sản xuất, tích hợp sâu warehouse |
| `payment` | Thanh toán nhà cung cấp, ghi COGS theo lô, đa tiền tệ |
| `product` | Nhận sản phẩm từ Supply Chain qua ACL |
| `catalog` | Quản lý mùa vụ, cảnh báo tiến độ bộ sưu tập |
| `pricing` | Quy tắc giảm giá theo mùa, cảnh báo sell-through |
| `warehouse` | Nhiều kho, chuyển kho, tối ưu đường lấy hàng |
| `fulfillment` | Dịch vụ fulfillment cho seller |
| `return` | Đổi hàng (không chỉ trả), phân tích nguyên nhân |
| `seller` | Duyệt tự động một phần, phân hạng |

### 3.7 Tiêu chí hoàn thành

**Chức năng**

```text
✓ Tạo được sản phẩm own brand từ concept tới lên sàn qua hệ thống
✓ Đặt sản xuất, theo dõi tiến độ, kiểm định, nhập kho
✓ Giá vốn tính theo LÔ, không phải theo SKU
✓ Đề xuất bổ sung hiển thị mâu thuẫn và ước tính tài chính
✓ Truy vết được: đơn hàng → lô sản xuất → nguyên liệu
```

**Chất lượng**

```text
✓ Forecast accuracy > 70%
✓ Quality pass rate > 95%
✓ On-time delivery (nhà cung cấp) > 90%
✓ Stockout rate ở SKU bán chạy < 5%
✓ Concept-to-shelf time < 120 ngày
```

**Kiến trúc**

```text
✓ ACL giữa Supply Chain và Catalog hoạt động đúng
  → Catalog KHÔNG biết về tech pack, giá vốn, nhà cung cấp
✓ Mọi lô sản xuất có ProductionBatch với unit_cost
✓ Lô chưa qua QC không vào tồn kho bán được
```

### 3.8 Rủi ro chính

| Rủi ro | Giảm thiểu |
|---|---|
| Dữ liệu nhu cầu chưa đủ để dự báo | Làm phần thực thi trước, phần thông minh sau |
| Dự báo sai → sản xuất thừa/thiếu | Con người quyết định, hiển thị rõ độ không chắc chắn |
| ACL bị rò rỉ khái niệm sản xuất sang Catalog | Kiểm tra ranh giới trong CI |
| Giá vốn không gắn lô → tính biên sai | Bắt buộc `production_batch_id` khi nhập kho own brand |
| Quy trình phức tạp, người dùng không theo | Đào tạo, giao diện hỗ trợ ra quyết định rõ ràng |

---

## 4. Phase 4 — nâng cấp chiều sâu

Phase 4 **không thêm module mới**:

```text
recommendation  → cá nhân hóa nâng cao, có thể dùng ML
                  (tách service — lý do: chuyên biệt công nghệ)

supply-chain    → dự báo nâng cao, tối ưu phân bổ size

marketplace     → retail media (vị trí được tài trợ)

content         → live commerce

campaign        → creator marketplace (kết nối brand ↔ creator)

analytics       → kho dữ liệu, phân tích chuyên sâu
```

---

## 5. Tài liệu liên quan

- [mvp.md](mvp.md) — phạm vi MVP (đã đóng băng)
- [backlog.md](backlog.md) — việc đang làm
- [scale.md](scale.md) — chiến lược mở rộng, điểm nghẽn dự kiến
- [../01-business/content-commerce.md](../01-business/content-commerce.md)
- [../01-business/supply-chain.md](../01-business/supply-chain.md)
- [../07-workflows/creator-affiliate.md](../07-workflows/creator-affiliate.md)
- [../07-workflows/replenishment.md](../07-workflows/replenishment.md)
- [../07-workflows/own-brand-product.md](../07-workflows/own-brand-product.md)

---

## 6. Rà soát CHIỀU RỘNG sản phẩm (06/09/2026)

**Trạng thái: KHẢO SÁT, chưa lên lịch, chưa cam kết.** Không mục nào ở đây
được phép mở ra khi phase hiện tại chưa đóng — xem "Miền bị KHÓA" trong
[backlog.md](backlog.md) mục 6, và quy tắc nhận việc ở mục 7.

Mục này tồn tại để trả lời một câu hỏi khác câu "làm gì tiếp": **thứ gì
đang VẮNG MẶT mà không ai ghi là vắng mặt?** Một khoảng trống có tên thì
được cân nhắc; một khoảng trống vô danh thì bị phát hiện bởi khách hàng.

Mười bốn miền được soi, xếp theo mức độ ĐÁNG LO chứ không theo thứ tự đề
bài.

### 6.1 Có chỗ giữ nhưng RỖNG — nợ đã nằm sẵn trong code

Nguy hiểm hơn hẳn ba nhóm sau: code trông như đã có, nên không ai đi tìm.

| Miền | Thực tế |
|---|---|
| **Thuế** | ✅ ĐÃ SỬA 06–07/09 (PH-40) — một tầng, mặc định 8%, sửa được lúc chạy, giá niêm yết ĐÃ GỒM VAT. Trước đó `SetTax` chỉ được gọi từ một bài test với giá trị 0. |
| **Phí vận chuyển** | ✅ ĐÃ SỬA 06/09 (P3-8) — theo từng nguồn hàng, kèm ngưỡng miễn phí 499.000đ. Vẫn chưa theo khoảng cách/khối lượng vì thiếu dữ liệu. |

Cùng một hình dạng với `payment_method` trước P3-9: một trường đi qua mọi
tầng mà không ai điền. Khác biệt là thuế có hệ quả PHÁP LÝ, không chỉ vận
hành.

### 6.2 Có ở mức MVP — đủ để bán hàng, chưa đủ để cạnh tranh

Không phải khoảng trống. Ghi ra để lần sau không ai "phát hiện" lại.

| Miền | Đang có | Chưa có |
|---|---|---|
| **Khuyến mãi** | `promotion` + `coupon`; PERCENTAGE · FIXED · FREE_SHIP | Mua X tặng Y, combo, khuyến mãi theo bậc, xếp chồng nhiều mã |
| **Giá** | `pricing`: mức giá, KHUNG GIÁ (chống phá giá), lịch sử | Giá theo nhóm khách, giá theo số lượng, giá theo thời điểm |
| **Tìm kiếm** | `operationId: search`, ghi nhận truy vấn không ra kết quả | Gợi ý gõ, sửa lỗi chính tả, xếp hạng theo hành vi, lọc mặt |
| **CMS / nội dung** | Đã hoạch định: module `content` ở Phase 2 | — |

### 6.3 Có VAI TRÒ nhưng chưa có CÔNG CỤ

Quyền đã cấp, màn hình thì chưa có. Người được cấp quyền hôm nay không làm
được việc mà tên vai trò hứa.

| Miền | Thực tế |
|---|---|
| **Merchandising** | `OPS_MERCHANDISING` là vai trò có thật trong ma trận quyền. Không có màn hình sắp xếp danh mục, ghim sản phẩm, dựng bộ sưu tập theo mùa. |
| **Công cụ vận hành marketplace** | Có: duyệt/đình chỉ nhà bán, điều chỉnh sổ cái, điều chỉnh tồn kho, xem đơn. Chưa có: bảng điều khiển sức khỏe nhà bán, hàng đợi tranh chấp, công cụ hàng loạt, giám sát chất lượng nhà bán. |
| **Chiều rộng Admin** | 16/80 thao tác là admin. Đủ cho bảy luồng MVP; chưa đủ cho một đội vận hành làm việc cả ngày trong đó. |

### 6.4 KHÔNG tồn tại ở bất kỳ đâu — kể cả trong tài liệu

Năm miền dưới đây có **0 lần nhắc** trong toàn bộ `docs/`. Đó là phát hiện
chính của đợt rà: chúng chưa từng bị bác bỏ, chúng chưa từng được nghĩ tới.

| Miền | Vì sao nó KHÔNG phải "thêm một module" |
|---|---|
| **Đăng ký định kỳ** | Đổi mô hình đơn hàng: đơn lặp lại, thanh toán định kỳ, tạm dừng/tiếp tục. Đụng order, payment, fulfillment cùng lúc. |
| **B2B** | Đổi mô hình KHÁCH: tổ chức nhiều người dùng, hạn mức công nợ, báo giá, duyệt mua. `customer` hiện là một cá nhân. |
| **Đa vùng** | `money` đã hỗ trợ VND + USD kèm đơn vị nhỏ nhất đúng. Trên đó thì KHÔNG có gì: không mô hình vùng, không giá/thuế/phí ship theo vùng, không kho theo vùng. |
| **Đa ngôn ngữ** | Toàn bộ chữ trên giao diện là tiếng Việt cứng, có chủ ý (README). Đổi sang đa ngôn ngữ đụng cả `product` (tên, mô tả theo ngôn ngữ) chứ không chỉ tầng trình bày. |
| **Tích hợp ngoài** | Không có khái niệm nào cho ERP, PIM, phần mềm kế toán, hay sàn thứ ba. Cả `payment` lẫn `fulfillment` cũng CHƯA có adapter nhà cung cấp thật (ADR-0017). |

### 6.5 Đề xuất — và lý do KHÔNG đề xuất nhiều hơn

```text
ĐÃ LÀM 06/09                          P3-8 phí vận chuyển theo nguồn
                                      PH-40 thuế một tầng 8%

SAU khi đóng phase, trước Phase 2     adapter nhà cung cấp thật
                                      (payment + shipping) — mọi tích hợp
                                      khác đều đứng sau nó

Phase 2+ (đã có chỗ)                  content · merchandising · công cụ
                                      vận hành marketplace

CHƯA nên đưa vào kế hoạch nào         subscriptions · B2B · đa vùng ·
                                      đa ngôn ngữ
```

Bốn miền cuối bị để ngoài kế hoạch **có chủ ý**, không phải vì quên. Mỗi
cái đổi một mô hình LÕI — đơn hàng, khách hàng, hoặc danh mục — và dự án
chưa có tín hiệu thị trường nào nói cái nào cần trước. Đưa chúng vào
roadmap lúc này là cam kết một kiến trúc cho một nhu cầu chưa tồn tại, mà
quy tắc nhận việc ở backlog mục 7 đã nói thẳng: lý do "sau này có thể cần"
thì không làm.

Cái đúng phải làm bây giờ là **ghi tên chúng ra**, và đó là mục đích của
mục này.
