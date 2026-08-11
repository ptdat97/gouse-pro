# Webhook

## 1. Hai chiều webhook

```text
INBOUND  — dịch vụ bên ngoài gọi VÀO hệ thống
           /api/v1/webhooks/...
           Nguồn: cổng thanh toán, đơn vị vận chuyển

OUTBOUND — hệ thống gọi RA dịch vụ bên ngoài
           Đích: đối tác tích hợp, seller có hệ thống riêng
           (Phase 3–4)
```

---

## 2. Webhook đến (Inbound)

### 2.1 Nguồn chính

```text
POST /api/v1/webhooks/payment/{provider}
POST /api/v1/webhooks/shipping/{provider}
```

### 2.2 Yêu cầu bắt buộc

```text
1. XÁC MINH CHỮ KÝ
   → chống giả mạo. Không có bước này, bất kỳ ai cũng
     có thể gửi "thanh toán thành công" giả

2. IDEMPOTENT
   → nhà cung cấp SẼ gửi trùng

3. TRẢ 200 NHANH
   → xử lý nặng chuyển sang bất đồng bộ
   → nhà cung cấp có timeout ngắn, chậm sẽ bị gửi lại

4. GHI LOG MỌI WEBHOOK
   → kể cả loại không xử lý — cần khi điều tra

5. KHÔNG TIN TUYỆT ĐỐI VÀO WEBHOOK
   → phải có đối chiếu định kỳ, vì webhook có thể mất
```

### 2.3 Luồng xử lý

```text
Nhận webhook
    ↓
1. Xác minh chữ ký
    ├── Sai → trả 401, ghi log cảnh báo
    ↓
2. Ghi vào webhook_log (thô, chưa xử lý)
    ↓
3. Kiểm tra provider_event_id đã xử lý chưa
    ├── Rồi → trả 200 ngay, KHÔNG xử lý lại
    ↓
4. Đưa vào hàng đợi xử lý
    ↓
5. Trả 200
    ↓
(bất đồng bộ)
6. Xử lý: cập nhật trạng thái thanh toán, phát event
7. Đánh dấu đã xử lý
```

**Điểm quan trọng ở bước 4–5:** trả 200 **trước khi** xử lý xong. Nếu xử lý mất 5 giây và nhà cung cấp timeout ở 3 giây, họ sẽ gửi lại — tạo ra xử lý trùng không cần thiết.

### 2.4 Ví dụ: webhook thanh toán

```http
POST /api/v1/webhooks/payment/provider-a
X-Signature: sha256=...
Content-Type: application/json

{
  "event_id": "evt_provider_123",
  "event_type": "payment.succeeded",
  "data": {
    "payment_intent_id": "pi_...",
    "amount": 628000,
    "currency": "VND",
    "metadata": { "checkout_id": "chk_01J9X" }
  }
}
```

```text
Xử lý:
1. Xác minh chữ ký với khóa bí mật của nhà cung cấp
2. Tìm payment_intent theo id
3. Đối chiếu SỐ TIỀN  ← quan trọng
4. Cập nhật trạng thái → CAPTURED
5. Phát event payment.captured
6. checkout và order xử lý tiếp
```

**Bước 3 bắt buộc:** đối chiếu số tiền webhook báo với số tiền hệ thống ghi nhận. Không khớp → không xử lý, cảnh báo ngay. Đây là lớp bảo vệ chống lỗi tích hợp và chống thao túng.

### 2.5 Đối chiếu định kỳ — không chỉ dựa vào webhook

```text
Webhook có thể mất do: lỗi mạng, hệ thống đang triển khai, lỗi nhà cung cấp.

Job định kỳ (ví dụ mỗi giờ):
    - Lấy danh sách giao dịch từ API nhà cung cấp
    - So với payment trong hệ thống
    - Phát hiện chênh lệch → cảnh báo
```

Không có bước này, một webhook mất nghĩa là khách đã trả tiền nhưng đơn hàng treo ở trạng thái chờ thanh toán.

---

## 3. Webhook đi (Outbound) — Phase 3–4

### 3.1 Đối tượng

```text
Seller có hệ thống quản lý riêng
    → nhận thông báo đơn mới, không phải hỏi liên tục

Đối tác tích hợp
    → đồng bộ dữ liệu
```

### 3.2 Loại sự kiện gửi ra

```text
order.created            — đơn mới cho seller
order.cancelled
fulfillment.required     — cần chuẩn bị hàng
return.requested
settlement.ready         — đối soát sẵn sàng
payout.completed
inventory.low_stock      — cảnh báo cho seller
```

### 3.3 Yêu cầu

```text
1. KÝ payload
   → người nhận xác minh được nguồn gốc

2. THỬ LẠI với khoảng chờ tăng dần
   → ví dụ: 1 phút, 5 phút, 30 phút, 2 giờ, 6 giờ
   → sau N lần thất bại: đánh dấu endpoint có vấn đề, cảnh báo

3. TIMEOUT ngắn
   → không để endpoint chậm của đối tác làm nghẽn hệ thống

4. GIAO ÍT NHẤT MỘT LẦN
   → người nhận phải idempotent, có event_id để khử trùng

5. THỨ TỰ KHÔNG ĐẢM BẢO
   → payload có timestamp để người nhận tự xử lý
```

### 3.4 Định dạng

```json
{
  "event_id": "evt_01J9X",
  "event_type": "order.created",
  "occurred_at": "2026-08-10T14:25:11Z",
  "api_version": "v1",
  "data": {
    "fulfillment_order_id": "ful_01J9X",
    "fulfillment_number": "FC-2026-08-001234-A",
    "items": [ ... ]
  }
}
```

```http
X-Signature: sha256=...
X-Event-ID: evt_01J9X
X-Delivery-Attempt: 1
```

### 3.5 Quản lý endpoint của đối tác

```text
Seller/đối tác đăng ký:
    - URL nhận webhook
    - Loại sự kiện quan tâm
    - Khóa bí mật (để ký)

Hệ thống cung cấp:
    - Lịch sử gửi và trạng thái
    - Gửi lại thủ công
    - Công cụ kiểm tra endpoint
```

**Cơ chế bảo vệ:** nếu endpoint của một đối tác liên tục thất bại, tạm ngừng gửi và cảnh báo — không để hàng đợi webhook phình vô hạn vì một đối tác có vấn đề.

---

## 4. Dữ liệu

```sql
CREATE TABLE webhook_log (
    id                UUID PRIMARY KEY,
    direction         TEXT NOT NULL,       -- INBOUND | OUTBOUND
    provider          TEXT NOT NULL,
    provider_event_id TEXT,
    event_type        TEXT,
    payload           JSONB NOT NULL,
    signature_valid   BOOLEAN,
    processed         BOOLEAN NOT NULL DEFAULT false,
    processed_at      TIMESTAMPTZ,
    error             TEXT,
    received_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_webhook_provider_event
    ON webhook_log (provider, provider_event_id)
    WHERE provider_event_id IS NOT NULL;

CREATE TABLE webhook_endpoint (
    id             UUID PRIMARY KEY,
    owner_type     TEXT NOT NULL,       -- SELLER | PARTNER
    owner_id       UUID NOT NULL,
    url            TEXT NOT NULL,
    secret_hash    TEXT NOT NULL,
    event_types    TEXT[] NOT NULL,
    status         TEXT NOT NULL,       -- ACTIVE | PAUSED | FAILED
    failure_count  INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Chỉ mục duy nhất trên `(provider, provider_event_id)` là cơ chế idempotency ở tầng database.

---

## 5. Giám sát

| Chỉ báo | Ngưỡng cảnh báo |
|---|---|
| Webhook đến có chữ ký sai | > 0 (điều tra ngay) |
| Webhook đến chưa xử lý | > 10 |
| Chênh lệch khi đối chiếu với PSP | > 0 (nghiêm trọng) |
| Webhook đi thất bại liên tiếp | > 5 lần với một endpoint |
| Độ trễ xử lý webhook (p99) | > 30 giây |

---

## 6. Tài liệu liên quan

- [api-guidelines.md](api-guidelines.md)
- [../05-data/idempotency.md](../05-data/idempotency.md)
- [../04-modules/payment.md](../04-modules/payment.md) mục 10
