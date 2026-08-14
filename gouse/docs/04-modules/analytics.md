# Module: Analytics

| | |
|---|---|
| **Bounded Context** | Platform |
| **Phân loại** | Supporting |
| **Giai đoạn** | MVP (cơ bản) |

---

## 1. Trách nhiệm

- Thu thập và lưu trữ sự kiện hành vi
- Tính toán các chỉ số nghiệp vụ
- Cung cấp dữ liệu cho báo cáo và dashboard
- Cung cấp dữ liệu thô cho `supply-chain` (tín hiệu nhu cầu)

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Quyết định nghiệp vụ dựa trên số liệu | Module nghiệp vụ |
| Tổng hợp tín hiệu nhu cầu | `supply-chain` |
| Gợi ý sản phẩm | `recommendation` |
| Nguồn sự thật về giao dịch | Module sở hữu dữ liệu |

---

## 3. Ràng buộc kiến trúc

> **`analytics` chỉ NHẬN event, không GỌI module nghiệp vụ nào.**

Lý do giống `notification`: nếu analytics gọi mọi module để làm giàu dữ liệu, nó phụ thuộc toàn hệ thống.

**Hệ quả:** event payload phải chứa đủ thông tin để phân tích. Nếu thiếu, bổ sung vào event, không gọi ngược.

---

## 4. Nguyên tắc quan trọng: analytics không phải nguồn sự thật

```text
Câu hỏi "GMV tháng này bao nhiêu?"
    → analytics trả lời (số liệu tổng hợp, có thể trễ vài phút)

Câu hỏi "Seller A được trả bao nhiêu tiền?"
    → payment.GetBalance() trả lời (nguồn sự thật)
```

**Không bao giờ** dùng số liệu analytics để ra quyết định tài chính. Analytics là bản sao đọc, có thể trễ và có thể mất mát ở mức chấp nhận được.

---

## 5. Loại dữ liệu thu thập

```text
Sự kiện hành vi người dùng:
    page_view · product_view · search · add_to_cart
    checkout_start · purchase · content_view · click

Sự kiện nghiệp vụ (từ domain event):
    order.placed · fulfillment.delivered · return.refunded
    seller.approved · offer.created
```

Sự kiện hành vi có khối lượng rất lớn — cần chiến lược lưu trữ riêng.

---

## 6. Chiến lược lưu trữ

```text
MVP:
    Bảng event_log trong PostgreSQL, phân vùng theo ngày
    Bảng metric_snapshot cho chỉ số tính sẵn

Phase 2–3:
    Nếu tải ghi ảnh hưởng database chính
    → tách sang lưu trữ chuyên dụng cho dữ liệu chuỗi thời gian

Phase 4:
    Kho dữ liệu riêng nếu cần phân tích phức tạp
```

**Nguyên tắc:** không tách hạ tầng khi chưa đo được vấn đề. Đây là ứng viên tách service **nhóm 1** (dễ, rủi ro thấp) nhưng vẫn cần lý do bằng số liệu. Xem [../03-architecture/evolution-to-services.md](../03-architecture/evolution-to-services.md).

---

## 7. Chỉ số cần tính

Xem danh sách đầy đủ tại [../01-business/kpi.md](../01-business/kpi.md). Tóm tắt theo nhóm:

```text
Khách hàng:    GMV · AOV · tỷ lệ chuyển đổi · tỷ lệ mua lại · CLV · CAC
Thời trang:    tỷ lệ hoàn · sell-through · markdown rate · size availability
Marketplace:   take rate · seller hoạt động · assortment · tỷ lệ hủy
Creator:       GMV quy kết · tỷ lệ click→mua · chi phí mỗi đơn
Chuỗi cung ứng: forecast accuracy · vòng quay tồn kho · stockout rate
```

### Phễu chuyển đổi — đo từng bước

```text
Lượt truy cập → Xem sản phẩm → Thêm giỏ → Checkout
→ Đặt hàng → Thanh toán → Giao thành công → Hoàn tất
```

Đo tổng thể chỉ cho biết **có vấn đề**; đo từng bước cho biết **vấn đề ở đâu**.

---

## 8. Yêu cầu quyền riêng tư

| Yêu cầu | Cách thực hiện |
|---|---|
| Không lưu IP dạng gốc | Băm hoặc rút gọn |
| Tôn trọng tùy chọn theo dõi | Kiểm tra `customer_consent` loại `PERSONALIZATION` |
| Xóa dữ liệu khi khách yêu cầu | Ẩn danh hóa bản ghi hành vi |
| Không lưu dữ liệu nhạy cảm trong event | Không đưa số đo cơ thể, thông tin thanh toán vào analytics |

---

## 9. Dữ liệu sở hữu

```sql
event_log               -- sự kiện thô, phân vùng theo ngày
metric_snapshot         -- chỉ số tính sẵn
funnel_snapshot
cohort_data
```

---

## 10. Interface công khai

```go
type PublicAPI interface {
    TrackEvent(ctx, event AnalyticsEvent) error       // bất đồng bộ, không chặn

    GetMetric(ctx, req MetricRequest) (*MetricResult, error)
    GetFunnel(ctx, req FunnelRequest) (*FunnelResult, error)
    GetTimeSeries(ctx, req TimeSeriesRequest) (*TimeSeriesResult, error)
}
```

`TrackEvent` phải **không bao giờ chặn** luồng chính. Nếu analytics lỗi, việc bán hàng vẫn phải chạy bình thường.

---

## 11. Event

**Lắng nghe:** hầu hết event từ mọi module.

**Phát ra:** không phát event nghiệp vụ (chỉ là bên tiêu thụ).

---

## 12. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Không gọi module nghiệp vụ nào |
| 2 | Không phải nguồn sự thật cho quyết định tài chính |
| 3 | Ghi nhận không được chặn luồng chính |
| 4 | Không lưu dữ liệu cá nhân nhạy cảm |
| 5 | Tôn trọng tùy chọn theo dõi của khách |
| 6 | Ghi dữ liệu thô đầy đủ từ đầu — không tạo ngược được |

---

## 13. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Ghi sự kiện cơ bản, chỉ số cốt lõi (GMV, AOV, chuyển đổi) |
| **Phase 2** | Phễu chuyển đổi, chỉ số creator, dashboard seller |
| **Phase 3** | Chỉ số chuỗi cung ứng, phân tích cohort |
| **Phase 4** | Kho dữ liệu, phân tích nâng cao |

---

## 14. Trạng thái triển khai (MVP — 14/08/2026)

Mã nguồn: `internal/modules/analytics/`. Migration: `000019_analytics`.
Kiểm chứng: 24 test tích hợp trên PostgreSQL thật, đã kiểm chứng ngược.

**Đã có (đúng phạm vi MVP ở mục 13):** ghi sự kiện đơn lẻ và theo lô · lọc
dữ liệu nhạy cảm · chống ghi trùng sự kiện nghiệp vụ · năm chỉ số cốt lõi
(GMV, số đơn, AOV, tỷ lệ chuyển đổi, số phiên) · cắt lát theo seller ·
chuỗi thời gian · ẩn danh hóa.

**Đã nối vào luồng chạy thật:** `RecordEventsFromBus` nghe
`checkout.completed`, `cart.item_added`, `fulfillment.progress_changed`;
đăng ký ở `cmd/worker`. Job `tính chỉ số phân tích` chạy mỗi 5 phút.

**Chưa có (Phase 2 trở đi):** phễu chuyển đổi từng bước · chỉ số creator ·
dashboard seller · cohort · kho dữ liệu.

### Ba chỗ code KHÁC tài liệu — và vì sao code đúng

**1. Chưa có `funnel_snapshot`, `cohort_data`**

Mục 9 liệt kê bốn bảng; code có hai (`event_log`, `metric_snapshot`).

Phễu và cohort thuộc Phase 2–3 theo chính mục 13. Bảng rỗng chỉ tạo cảm
giác tính năng đã có. Dữ liệu THÔ để dựng chúng sau này ĐÃ được ghi đầy đủ
từ đầu (`session_id` nối các sự kiện của một lượt truy cập) — đó mới là
phần không tạo ngược được, đúng quy tắc 6.

**2. `event_log` chưa phân vùng theo ngày**

Mục 6 nói "phân vùng theo ngày". Code dùng bảng thường có chỉ mục theo
`occurred_at`.

Phân vùng có giá khi bảng đủ lớn để việc dọn dữ liệu cũ trở thành vấn đề —
`DROP PARTITION` tức thì so với `DELETE` quét hàng triệu hàng. Ở quy mô
MVP thì chưa, và chính mục 6 nói **không tách hạ tầng khi chưa đo được vấn
đề**. Chuyển sang bảng phân vùng sau này là một migration, không phải viết
lại module.

**3. Interface công khai khác mục 10**

`GetFunnel` chưa có (Phase 2). Thêm: `TrackBatch`, `ComputeMetrics`,
`CountEvents`, `AnonymizeCustomer`.

`TrackBatch` cần vì sự kiện hành vi có khối lượng rất lớn — ghi từng cái
là một lượt đi-về database cho mỗi lần khách di chuột.

`AnonymizeCustomer` là nghĩa vụ pháp lý (mục 8), không có trong bản thiết
kế nhưng bắt buộc phải có.

### Bên nhận event: một sự kiện cho MỖI gian hàng

`checkout.completed` mang nhiều dòng hàng từ nhiều gian hàng. Handler tách
thành **một sự kiện `order.placed` cho mỗi gian hàng**, và gom các dòng
cùng gian hàng lại:

```text
Đơn: 3 dòng, seller A (2 dòng) + seller B (1 dòng)
  → 2 sự kiện, KHÔNG phải 1 và cũng KHÔNG phải 3

  1 sự kiện  → dashboard của A hiện cả doanh số của B
  3 sự kiện  → số đơn thổi lên gấp rưỡi, AOV thấp đi tương ứng
```

**Khóa chống trùng phải ghép id gian hàng:** chỉ mục là
`(event_id, event_name)`. Dùng chung `event_id` cho cả hai gian hàng thì
sự kiện thứ hai bị coi là bản trùng và **GMV của gian hàng thứ hai biến
mất**. Đã kiểm chứng ngược đúng kịch bản đó.

**Chỉ ghi mốc `DELIVERED`** trong chín trạng thái giao hàng. Các bước còn
lại là việc nội bộ của seller; ghi hết là nhồi nhật ký bằng dữ liệu không
ai hỏi tới, và khối lượng đó phải trả giá ở mọi truy vấn chỉ số sau này.

**Một cái bẫy về tên trường:** bên phát dùng `new_status`, không phải
`status`. Đọc sai tên thì JSON KHÔNG báo lỗi — nó trả chuỗi rỗng và mọi
mốc giao hàng lặng lẽ bị bỏ qua. Đã có test riêng chốt đúng chỗ này.

### Hai phát hiện từ kiểm chứng ngược

**Tỷ lệ chuyển đổi phải đếm theo PHIÊN, không theo sự kiện.** Test dựng
một phiên xem 20 sản phẩm cộng ba phiên xem một lần, một phiên mua. Đếm
đúng ra 25%; đếm theo sự kiện ra 4,3% — **sai gần sáu lần**, và người đọc
sẽ đi tìm một vấn đề không tồn tại.

**Chống ghi trùng có hai lớp, và test chỉ bắt được một.** Chỉ mục UNIQUE
chặn hàng thứ hai; việc dịch lỗi thành `ErrDuplicateEvent` quyết định bên
gọi thấy `nil` sạch hay một lỗi bị nuốt lặng. Bỏ lớp thứ hai, test đếm
hàng vẫn xanh — nhưng mỗi lần xử lý lại event sinh một CẢNH BÁO GIẢ. Cảnh
báo giả nhiều tới mức không ai đọc nữa là cách chắc chắn nhất để bỏ lỡ
cảnh báo thật. Đã thêm test cô lập lớp đó bằng cách đếm bản ghi nhật ký.

### Giới hạn đã biết, có chủ ý

| Giới hạn | Vì sao chấp nhận ở MVP |
|---|---|
| `ComputeMetrics` tính TỪNG seller một | Sàn có 10.000 seller nghĩa là 10.000 lần gọi. Cần gom nhóm bằng `GROUP BY` khi số seller đủ lớn — nhưng tối ưu trước khi đo là điều mục 6 cấm. |
| Chưa kiểm tra `customer_consent` loại PERSONALIZATION | Mục 8 yêu cầu tôn trọng tùy chọn theo dõi. Module `customer` đã có `HasConsent`, nhưng gọi nó ở đây vi phạm quy tắc 1 (không gọi module nghiệp vụ). Đúng cách là tầng interfaces kiểm tra trước khi gọi `TrackEvent`. |
| Job tính chỉ số chỉ tính TOÀN SÀN | Sàn 10.000 gian hàng nghĩa là 20.000 lượt tính mỗi 5 phút. Chỉ số theo gian hàng tính theo yêu cầu, hoặc bằng job thưa hơn khi có dashboard seller (Phase 2). |
| Mốc `DELIVERED` chưa cắt lát theo seller | Payload `fulfillment.progress_changed` chưa mang `seller_id`. GMV và số đơn KHÔNG bị ảnh hưởng — chúng đến từ `checkout.completed`, nơi `seller_id` đầy đủ. Cách sửa đúng là bổ sung trường vào payload, không phải gọi ngược module `fulfillment`. |
| Ghi sự kiện là ĐỒNG BỘ | Mục 10 nói "bất đồng bộ". Hiện tại ghi thẳng vào database nhưng KHÔNG BAO GIỜ trả lỗi hạ tầng ra ngoài — đã đạt mục tiêu thật (không chặn luồng chính) mà chưa cần hàng đợi. |

---

## 15. Tài liệu liên quan

- [../01-business/kpi.md](../01-business/kpi.md) — danh sách chỉ số đầy đủ
- [supply-chain.md](supply-chain.md) — tiêu thụ dữ liệu hành vi
- [../09-operations/observability.md](../09-operations/observability.md) — khác biệt với giám sát kỹ thuật
