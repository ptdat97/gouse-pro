# Admin API

Base path: `/api/v1/admin/`

---

## 1. Xác thực và phân quyền

```http
Authorization: Bearer <token>
```

Admin API dùng phân quyền theo vai trò chi tiết:

```text
ADMIN                 — toàn quyền
OPS_MERCHANDISING     — sản phẩm, danh mục, seller
OPS_WAREHOUSE         — kho, tồn kho, fulfillment
OPS_FINANCE           — sổ cái, đối soát, chi trả
OPS_SUPPORT           — đơn hàng, khách hàng (chỉ đọc + thao tác hỗ trợ)
OPS_CONTENT           — kiểm duyệt nội dung
```

### Hai yêu cầu bắt buộc

```text
1. Xác thực hai lớp bắt buộc cho ADMIN và OPS_FINANCE
2. Mọi truy cập dữ liệu cá nhân khách hàng đều ghi audit log
```

---

## 2. Thao tác nhạy cảm — bắt buộc có lý do

Các endpoint sau **bắt buộc** trường `reason`, được ghi vào `audit_log`:

```text
POST /api/v1/admin/ledger/adjustments
POST /api/v1/admin/inventory/adjustments
POST /api/v1/admin/sellers/{id}/suspend
POST /api/v1/admin/creators/{id}/suspend
POST /api/v1/admin/content/{id}/take-down
POST /api/v1/admin/orders/{id}/cancel
POST /api/v1/admin/refunds
```

**Kiểm tra:** lý do phải có độ dài tối thiểu; các giá trị như "test", "fix" bị từ chối. Lý do trống làm audit log vô giá trị.

---

## 3. Duyệt seller

### `POST /api/v1/admin/sellers/{id}/approve`

```json
{
  "seller_type": "BUSINESS",
  "commission_policy_id": "pol_01J9X",
  "reserve_rate": 1000,
  "reserve_hold_days": 30,
  "notes": "Đã xác minh giấy phép kinh doanh và tài khoản ngân hàng"
}
```

```json
{
  "seller": { "id": "sel_01J9X", "status": "APPROVED", "approved_at": "..." },
  "side_effects": [
    "Đã cấp vai trò SELLER_OWNER",
    "Đã tạo tài khoản tài chính",
    "Đã gửi email thông báo"
  ]
}
```

Trường `side_effects` giúp người vận hành hiểu điều gì đã xảy ra — hữu ích vì các tác động này diễn ra bất đồng bộ qua event.

### `POST /api/v1/admin/sellers/{id}/suspend`

```json
{
  "reason": "Tỷ lệ hủy đơn 8% vượt ngưỡng 3% trong 30 ngày",
  "reason_code": "PERFORMANCE_VIOLATION",
  "hold_payouts": true
}
```

```json
{
  "seller": { "id": "sel_01J9X", "status": "SUSPENDED" },
  "effects": {
    "offers_hidden": 142,
    "pending_fulfillment_orders": 8,
    "note": "Đơn đang xử lý KHÔNG bị hủy — seller phải hoàn tất hoặc chuyển admin xử lý"
  }
}
```

**Quy tắc quan trọng:** đình chỉ seller **không hủy** đơn hàng khách đã trả tiền. Response hiển thị rõ số đơn đang xử lý để người vận hành theo dõi.

---

## 4. Tài chính

### `POST /api/v1/admin/ledger/adjustments`

Đây là endpoint nhạy cảm nhất hệ thống.

```http
POST /api/v1/admin/ledger/adjustments
Idempotency-Key: 01J9X...
```

```json
{
  "reason": "Điều chỉnh hoa hồng đơn FC-2026-08-001234, ghi nhầm tỷ lệ 12% thay vì 10%",
  "reference_type": "ORDER",
  "reference_id": "ord_01J9X",
  "lines": [
    { "account_type": "PLATFORM_REVENUE", "direction": "DEBIT", "amount": 5980, "currency": "VND" },
    { "account_type": "SELLER_PAYABLE", "owner_id": "sel_01J9X", "direction": "CREDIT", "amount": 5980, "currency": "VND" }
  ]
}
```

**Response 422 (bút toán không cân):**

```json
{
  "error": {
    "code": "LEDGER_ENTRY_UNBALANCED",
    "message": "Tổng ghi nợ phải bằng tổng ghi có",
    "details": { "total_debit": 5980, "total_credit": 5000, "difference": 980 }
  }
}
```

**Nguyên tắc:** không có endpoint nào **sửa** bút toán cũ. Chỉ có endpoint tạo bút toán điều chỉnh mới. Xem [../adr/0008-financial-ledger.md](../adr/0008-financial-ledger.md).

### `POST /api/v1/admin/payouts`

```http
POST /api/v1/admin/payouts
Idempotency-Key: 01J9X...
```

```json
{ "settlement_ids": ["stl_01J9X", "stl_01J9Y"], "scheduled_date": "2026-08-13" }
```

```json
{
  "batch_id": "pbt_01J9X",
  "payouts": [
    { "id": "pay_01J9X", "seller_id": "sel_01J9X", "amount": { "amount": 9692500, "currency": "VND" }, "status": "PROCESSING" }
  ],
  "total_amount": { "amount": 24500000, "currency": "VND" },
  "warnings": [
    { "seller_id": "sel_01J9Z", "code": "NEGATIVE_BALANCE", "message": "Số dư âm −450.000đ, chuyển sang kỳ sau" }
  ]
}
```

**Đây là thao tác nguy hiểm nhất** — chuyển tiền thật ra ngoài hệ thống. Yêu cầu:

```text
1. Idempotency-Key bắt buộc
2. Kiểm tra trạng thái trước khi gọi API ngân hàng
3. Xác thực hai lớp
4. Ghi audit log đầy đủ
5. Cảnh báo rõ các trường hợp bất thường (số dư âm)
```

---

## 5. Chuỗi cung ứng

### `POST /api/v1/admin/production-orders`

```json
{
  "product_development_id": "pdv_01J9X",
  "supplier_id": "sup_01J9X",
  "tech_pack_id": "tpk_01J9X",
  "lines": [
    { "sku_id": "sku_01J9X", "size": "S",  "quantity": 75 },
    { "sku_id": "sku_01J9Y", "size": "M",  "quantity": 150 },
    { "sku_id": "sku_01J9Z", "size": "L",  "quantity": 150 },
    { "sku_id": "sku_01JA0", "size": "XL", "quantity": 100 },
    { "sku_id": "sku_01JA1", "size": "XXL","quantity": 25 }
  ],
  "unit_cost_agreed": { "amount": 100000, "currency": "VND" },
  "expected_delivery_date": "2026-10-15"
}
```

**Đơn sản xuất phân bổ theo size** — đây là yêu cầu đặc thù thời trang. Phân bổ sai gây thiệt hại kép: hết size bán chạy và tồn size ế.

**Response 422 (chưa duyệt mẫu):**

```json
{
  "error": {
    "code": "SAMPLE_NOT_APPROVED",
    "message": "Không thể tạo đơn sản xuất khi mẫu chưa được duyệt",
    "details": { "product_development_id": "pdv_01J9X", "current_status": "SAMPLING", "sample_round": 2 }
  }
}
```

### `GET /api/v1/admin/replenishment-suggestions`

```json
{
  "data": [
    {
      "id": "rps_01J9X",
      "sku_id": "sku_01J9X",
      "sku_code": "SM-LIN-OXF-WHT-M",
      "product_name": "Áo sơ mi linen Oxford — Trắng / M",
      "current_stock": 42,
      "reorder_point": 400,
      "sales_velocity_per_week": 50,
      "lead_time_weeks": 6,
      "suggested_quantity": 500,
      "demand_signals": {
        "stockout_events_30d": 3,
        "search_no_result_30d": 240,
        "notify_requests": 85,
        "wishlist_adds_30d": 190
      },
      "constraints": {
        "supplier_moq": 500,
        "forecast_demand": 300,
        "conflict": true,
        "conflict_note": "MOQ 500 vượt dự báo 300. Rủi ro tồn 200 đơn vị (~20 triệu đồng)."
      },
      "options": [
        { "action": "ORDER_MOQ", "quantity": 500, "estimated_excess_risk": { "amount": 20000000, "currency": "VND" } },
        { "action": "SKIP", "estimated_lost_sales": { "amount": 89700000, "currency": "VND" } },
        { "action": "ALTERNATE_SUPPLIER", "supplier_id": "sup_01J9Y", "moq": 200, "unit_cost_delta": 15000 }
      ]
    }
  ]
}
```

**Đây là ví dụ rõ nhất về "phần mềm hỗ trợ ra quyết định":**

```text
Hệ thống KHÔNG tự đặt hàng.
Hệ thống hiển thị:
    - Tín hiệu nhu cầu (bao gồm nhu cầu BỊ BỎ LỠ: stockout, tìm không ra)
    - Mâu thuẫn MOQ vs dự báo
    - Ước tính tài chính của từng phương án

Con người quyết định.
```

Trường `demand_signals.search_no_result_30d` và `stockout_events_30d` là dữ liệu mà nền tảng chỉ có được nếu **ghi tín hiệu nhu cầu từ MVP** — lý do tại [../04-modules/supply-chain.md](../04-modules/supply-chain.md) mục 4.1.

---

## 6. Truy cập dữ liệu khách hàng

### `GET /api/v1/admin/customers/{id}`

Mọi lần gọi endpoint này **ghi audit log**:

```json
{
  "actor_id": "usr_staff_01J9X",
  "action": "customer.view",
  "resource_type": "CUSTOMER",
  "resource_id": "cus_01J9X",
  "reason": "Xử lý khiếu nại đơn FC-2026-08-001234",
  "occurred_at": "..."
}
```

**Cảnh báo tự động:** nhân viên truy cập nhiều hồ sơ khách trong thời gian ngắn mà không liên quan tới ticket hỗ trợ nào → cảnh báo cho quản lý.

---

## 7. Audit log

### `GET /api/v1/admin/audit-log`

```http
GET /api/v1/admin/audit-log?resource_type=LEDGER&action=ledger.adjust&from=2026-08-01&to=2026-08-31
```

Chỉ vai trò `ADMIN` truy cập được. Audit log **không thể sửa hoặc xóa** qua bất kỳ endpoint nào.

---

## 8. Tài liệu liên quan

- [api-domains.md](api-domains.md) — phân quyền
- [../08-frontend/admin.md](../08-frontend/admin.md)
- [../05-data/audit.md](../05-data/audit.md)
- [../09-operations/security.md](../09-operations/security.md)
