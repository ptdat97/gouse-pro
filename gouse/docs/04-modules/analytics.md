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

## 14. Tài liệu liên quan

- [../01-business/kpi.md](../01-business/kpi.md) — danh sách chỉ số đầy đủ
- [supply-chain.md](supply-chain.md) — tiêu thụ dữ liệu hành vi
- [../09-operations/observability.md](../09-operations/observability.md) — khác biệt với giám sát kỹ thuật
