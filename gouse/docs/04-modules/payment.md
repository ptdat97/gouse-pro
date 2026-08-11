# Module: Payment & Financial Ledger

| | |
|---|---|
| **Bounded Context** | Financial |
| **Phân loại** | **Core** (ledger) / Generic (cổng thanh toán) |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

Module này có **hai phần riêng biệt**, cần phân biệt rõ:

### Phần A — Thanh toán (Generic)

- Tạo ý định thanh toán, tích hợp cổng thanh toán
- Xử lý webhook từ nhà cung cấp dịch vụ thanh toán
- Thực hiện hoàn tiền qua cổng

### Phần B — Sổ cái tài chính (Core)

- **Nguồn sự thật duy nhất** về mọi dòng tiền
- Ghi bút toán bất biến
- Tính số dư của mọi bên (nền tảng, seller, creator, nhà cung cấp)
- Đối soát và chi trả

**Vì sao phân biệt:** phần A có thể thay nhà cung cấp bất kỳ lúc nào. Phần B là tài sản của nền tảng, không bao giờ thuê ngoài.

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Quyết định tỷ lệ hoa hồng | `marketplace` |
| Quyết định khi nào đơn hoàn tất | `order`, `fulfillment` |
| Quyết định chấp nhận trả hàng | `return` |
| Lưu thông tin thẻ của khách | **Không ai** — do PSP giữ |

**Ràng buộc tuân thủ quan trọng:** hệ thống **không bao giờ** lưu số thẻ đầy đủ, CVV, hay dữ liệu nhạy cảm của thẻ. Chỉ lưu mã token do PSP cấp và bốn số cuối để hiển thị.

---

## 3. Nguyên tắc sổ cái bất biến

Đây là nguyên tắc quan trọng nhất của module (nguyên tắc P8).

```text
Sổ cái CHỈ ĐƯỢC GHI THÊM (append-only).

KHÔNG BAO GIỜ:
    ✗ UPDATE một bút toán đã ghi
    ✗ DELETE một bút toán
    ✗ Sửa số tiền

Sửa sai bằng: ghi bút toán ĐIỀU CHỈNH mới.
```

### Vì sao

| Lý do | Giải thích |
|---|---|
| Kiểm toán | Phải tái dựng được số dư tại bất kỳ thời điểm quá khứ |
| Tranh chấp | Có bằng chứng không thể sửa khi seller khiếu nại |
| Phát hiện lỗi | Sổ cái bất biến làm lỗi lộ ra thay vì bị che giấu |
| Nghĩa vụ pháp lý | Chứng từ kế toán phải lưu giữ nguyên vẹn |

### Ví dụ: sửa sai đúng cách

```text
Phát hiện ghi nhầm hoa hồng 30.000đ, đúng phải là 25.000đ

SAI:
    UPDATE ledger_line SET amount = 25000 WHERE ...

ĐÚNG:
    Ghi bút toán mới, loại ADJUSTMENT:
      DEBIT   PLATFORM_REVENUE        5.000
      CREDIT  SELLER_PAYABLE          5.000
      description: "Điều chỉnh hoa hồng đơn #1000, ghi nhầm tỷ lệ"
      reference: ledger_entry cũ
```

Kết quả cuối cùng giống nhau, nhưng cách thứ hai giữ được lịch sử và giải thích được.

---

## 4. Mô hình sổ cái

### 4.1 Cấu trúc kép (double-entry)

```text
LedgerEntry  — một sự kiện tài chính
    │
    └── LedgerLine[]  — các bút toán ghi nợ/ghi có
                        Σ DEBIT phải = Σ CREDIT
```

### 4.2 Danh mục tài khoản

```text
PLATFORM_CASH              — tiền mặt nền tảng đang giữ
PLATFORM_REVENUE           — doanh thu nền tảng (hoa hồng, phí)
SELLER_PAYABLE             — phải trả seller
CREATOR_PAYABLE            — phải trả creator
CUSTOMER_REFUND_PAYABLE    — phải hoàn khách
SUPPLIER_PAYABLE           — phải trả nhà cung cấp
COGS                       — giá vốn hàng bán
FEE_EXPENSE                — chi phí (phí PSP, vận chuyển)
INVENTORY_ASSET            — giá trị hàng tồn kho (own brand)
```

### 4.3 Ví dụ bút toán — đơn marketplace

Đơn 300.000đ, hoa hồng 10%, hoa hồng creator 5%, phí PSP 1,5%:

```text
LedgerEntry: ORDER_REVENUE, reference=Order#1000

  DEBIT   PLATFORM_CASH                     300.000
  CREDIT  SELLER_PAYABLE (seller A)         250.500
  CREDIT  PLATFORM_REVENUE                   30.000
  CREDIT  CREATOR_PAYABLE (creator X)        15.000
  CREDIT  FEE_EXPENSE (phí PSP)               4.500
  ────────────────────────────────────────────────
  Σ DEBIT  = 300.000
  Σ CREDIT = 300.000  ✓
```

### 4.4 Ví dụ bút toán — đơn own brand

Đơn 300.000đ, COGS 120.000đ:

```text
LedgerEntry: ORDER_REVENUE, reference=Order#1001

  DEBIT   PLATFORM_CASH                     300.000
  CREDIT  PLATFORM_REVENUE                  300.000

LedgerEntry: COGS, reference=Order#1001
  DEBIT   COGS                              120.000
  CREDIT  INVENTORY_ASSET                   120.000
```

**Quan sát:** đơn own brand ghi **doanh thu toàn phần**, đơn marketplace chỉ ghi **hoa hồng** là doanh thu. Đây là phân biệt GMV/doanh thu ở tầng ghi sổ, như đã yêu cầu tại [../00-overview/business-model.md](../00-overview/business-model.md).

---

## 5. Số dư — tính chứ không lưu

```text
Balance(account) = Σ CREDIT − Σ DEBIT  (với tài khoản phải trả)
```

**Nguyên tắc:** số dư là **kết quả tính toán**, không phải trường được cập nhật.

### Vì sao không lưu số dư

```text
Nếu lưu số dư và cập nhật mỗi giao dịch:
    - Có thể lệch với tổng bút toán (do lỗi, do race condition)
    - Không biết lệch từ lúc nào
    - Trở thành điểm nghẽn ghi (mọi giao dịch cập nhật cùng một dòng)
```

### Tối ưu hiệu năng: bản chụp (snapshot)

Tính tổng hàng triệu bút toán mỗi lần truy vấn là chậm. Giải pháp:

```text
balance_snapshot {
    account_id
    as_of_ledger_entry_id     ← chốt tại bút toán nào
    balance
    computed_at
}

Truy vấn số dư hiện tại:
    = snapshot gần nhất + tổng các bút toán sau snapshot đó
```

**Điểm mấu chốt:** snapshot là **cache có thể tính lại**, không phải nguồn sự thật. Nếu snapshot sai, tính lại từ bút toán gốc.

Cần có công việc định kỳ kiểm tra: snapshot có khớp với tổng bút toán không. Lệch = cảnh báo nghiêm trọng.

---

## 6. Trạng thái số dư seller

```text
Pending    — đơn đã giao, chưa hết hạn đổi trả
    ↓
Available  — hết hạn đổi trả, sẵn sàng chi trả
    ↓
Processing — đang chuyển tiền
    ↓
Paid       — đã chuyển thành công

Trạng thái đặc biệt:
On Hold    — bị giữ (tranh chấp, vi phạm, yêu cầu pháp lý)
Negative   — số dư âm do hoàn hàng vượt doanh thu kỳ
```

### Xử lý số dư âm

```text
Tình huống: seller bị hoàn 2 triệu, chỉ bán được 1 triệu trong kỳ

Xử lý:
    1. KHÔNG chuyển tiền âm
    2. Ghi nhận khoản âm, chuyển sang kỳ sau
    3. Nếu âm kéo dài → yêu cầu nộp bù
    4. Nếu seller ngừng hoạt động với số dư âm → khoản phải thu khó đòi
```

**Cơ chế phòng ngừa:** giữ lại một tỷ lệ bảo đảm (reserve) với seller mới hoặc có tỷ lệ hoàn cao. Cấu hình trong `seller_policy`.

---

## 7. Đối soát và chi trả

```text
Chu kỳ đối soát (ví dụ hàng tuần)
    ↓
Tổng hợp mọi khoản trong kỳ cho từng seller:
    + doanh thu bán hàng (đơn đã COMPLETED)
    − hoa hồng
    − phí dịch vụ
    − hoàn tiền phát sinh
    ± điều chỉnh
    ↓
Tạo Settlement
    ↓
Seller xem, xác nhận (hoặc tự động sau thời hạn)
    ↓
Payout — gọi API ngân hàng
    ↓
Ghi bút toán:
    DEBIT   SELLER_PAYABLE
    CREDIT  PLATFORM_CASH
```

### Yêu cầu idempotency cho payout

```text
Payout là thao tác NGUY HIỂM NHẤT hệ thống — chuyển tiền thật.

Bắt buộc:
    - Mỗi payout có idempotency key duy nhất
    - Kiểm tra trạng thái trước khi gọi lại API ngân hàng
    - Ghi nhận mọi phản hồi từ ngân hàng
    - Có quy trình đối chiếu với sao kê ngân hàng
```

Chuyển tiền hai lần do lỗi thử lại là loại lỗi tốn kém và khó thu hồi nhất.

---

## 8. Dữ liệu sở hữu

```sql
payment                 -- thanh toán của khách
payment_intent          -- ý định thanh toán
account                 -- danh mục tài khoản
ledger_entry            -- bút toán (BẤT BIẾN)
ledger_line             -- dòng bút toán (BẤT BIẾN)
balance_snapshot        -- cache số dư
settlement              -- đối soát
settlement_line
payout                  -- chi trả
refund                  -- hoàn tiền
```

### Bảng `ledger_entry` và `ledger_line`

```sql
CREATE TABLE ledger_entry (
    id               UUID PRIMARY KEY,
    entry_type       TEXT NOT NULL,
    reference_type   TEXT NOT NULL,
    reference_id     UUID NOT NULL,
    description      TEXT NOT NULL,
    idempotency_key  TEXT NOT NULL UNIQUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by       TEXT NOT NULL
    -- KHÔNG có updated_at — bản ghi bất biến
);

CREATE TABLE ledger_line (
    id               UUID PRIMARY KEY,
    ledger_entry_id  UUID NOT NULL REFERENCES ledger_entry(id),
    account_id       UUID NOT NULL REFERENCES account(id),
    direction        TEXT NOT NULL CHECK (direction IN ('DEBIT','CREDIT')),
    amount           BIGINT NOT NULL CHECK (amount > 0),
    currency         CHAR(3) NOT NULL
);

CREATE INDEX idx_ledger_line_account ON ledger_line (account_id, ledger_entry_id);
CREATE INDEX idx_ledger_entry_reference ON ledger_entry (reference_type, reference_id);
```

**Bảo vệ ở tầng database:**

```sql
-- Chặn UPDATE và DELETE trên bảng sổ cái
CREATE RULE ledger_entry_no_update AS ON UPDATE TO ledger_entry DO INSTEAD NOTHING;
CREATE RULE ledger_entry_no_delete AS ON DELETE TO ledger_entry DO INSTEAD NOTHING;
```

Đây là lớp bảo vệ cuối cùng — kể cả khi có lỗi code hoặc thao tác thủ công nhầm, database vẫn từ chối.

---

## 9. Interface công khai

```go
type PublicAPI interface {
    // Thanh toán
    CreatePaymentIntent(ctx, req PaymentIntentRequest) (*PaymentIntent, error)
    GetPaymentStatus(ctx, paymentID string) (*PaymentStatus, error)

    // Sổ cái — ĐIỂM VÀO DUY NHẤT
    RecordLedgerEntry(ctx, req LedgerEntryRequest) (*LedgerEntryResult, error)

    // Số dư — CÁCH DUY NHẤT để biết số dư
    GetBalance(ctx, accountType string, ownerID string) (*Balance, error)
    GetBalanceBreakdown(ctx, ownerID string) (*BalanceBreakdown, error)

    // Hoàn tiền
    IssueRefund(ctx, req RefundRequest) (*Refund, error)

    // Đối soát
    CreateSettlement(ctx, req SettlementRequest) (*Settlement, error)
    ExecutePayout(ctx, settlementID string, idempotencyKey string) (*Payout, error)
}
```

**Ràng buộc tuyệt đối:** `GetBalance` là cách duy nhất biết số dư. Không module nào được tự cộng trừ bản ghi để tính.

---

## 10. Cổng thanh toán — nằm sau interface

Nguyên tắc P13:

```go
// domain định nghĩa
type PaymentGateway interface {
    CreateIntent(ctx, req IntentRequest) (*IntentResult, error)
    Capture(ctx, intentID string) (*CaptureResult, error)
    Refund(ctx, req RefundRequest) (*RefundResult, error)
    VerifyWebhook(payload []byte, signature string) (*WebhookEvent, error)
}

// infrastructure cài đặt cho từng nhà cung cấp
```

**Domain không biết tên nhà cung cấp nào.** Đổi hoặc thêm nhà cung cấp = thêm adapter, không sửa domain.

### Xử lý webhook

```text
Webhook từ PSP là nguồn sự thật về việc tiền đã về hay chưa.

Yêu cầu bắt buộc:
    1. Xác minh chữ ký — chống giả mạo
    2. Idempotent — PSP sẽ gửi trùng
    3. Trả 200 nhanh, xử lý bất đồng bộ
    4. Ghi log mọi webhook nhận được, kể cả loại không xử lý
    5. Có cơ chế đối chiếu định kỳ với PSP (webhook có thể mất)
```

Điểm 5 quan trọng: không được chỉ dựa vào webhook. Phải có công việc định kỳ đối chiếu giao dịch với PSP để phát hiện chênh lệch.

---

## 11. Event

### Phát ra

| Event | Khi nào |
|---|---|
| `payment.intent_created` | Tạo ý định thanh toán |
| `payment.captured` | Thu tiền thành công |
| `payment.failed` | Thất bại |
| `ledger.entry_recorded` | Ghi bút toán |
| `settlement.created` | Tạo đối soát |
| `payout.executed` | Chuyển tiền thành công |
| `payout.failed` | Chuyển tiền lỗi |
| `refund.issued` | Hoàn tiền |

### Lắng nghe

| Event | Từ | Hành động |
|---|---|---|
| `order.placed` | order | Ghi bút toán doanh thu, hoa hồng |
| `order.cancelled` | order | Ghi bút toán đảo ngược |
| `fulfillment_order.completed` | fulfillment | Chuyển số dư Pending → Available |
| `return.refunded` | return | Ghi bút toán hoàn tiền |
| `affiliate.conversion_attributed` | affiliate | Ghi hoa hồng creator |
| `affiliate.attribution_reversed` | affiliate | Đảo ngược hoa hồng creator |
| `warehouse.goods_received` | warehouse | Ghi tăng giá trị tồn kho |

---

## 12. Quy tắc nghiệp vụ quan trọng

| # | Quy tắc | Lý do |
|---|---|---|
| 1 | Sổ cái bất biến, chỉ ghi thêm | Kiểm toán |
| 2 | Σ DEBIT = Σ CREDIT trong mỗi bút toán | Cân đối kế toán |
| 3 | Số dư là kết quả tính, không phải trường lưu | Nguồn sự thật duy nhất |
| 4 | Mọi thao tác tài chính phải idempotent | Chống trả tiền hai lần |
| 5 | Payout chỉ sau khi hết hạn đổi trả | Bảo vệ khỏi hoàn hàng |
| 6 | Không lưu dữ liệu thẻ nhạy cảm | Tuân thủ |
| 7 | Sửa sai bằng bút toán điều chỉnh | Giữ lịch sử |
| 8 | Mọi bút toán có tham chiếu tới nghiệp vụ gốc | Truy vết |
| 9 | Đối chiếu định kỳ với PSP và ngân hàng | Phát hiện chênh lệch |

---

## 13. Giám sát cần có

| Chỉ báo | Ngưỡng |
|---|---|
| **Độ lệch đối soát** | **Phải bằng 0** |
| Bút toán không cân bằng | Phải bằng 0 |
| Snapshot lệch với bút toán | Phải bằng 0 |
| Payout thất bại | Cảnh báo ngay |
| Webhook chưa xử lý | > 10 |
| Chênh lệch với sao kê PSP | Cảnh báo ngay |
| Số dư seller âm | Theo dõi danh sách |

Ba chỉ báo đầu phải luôn bằng 0. Bất kỳ giá trị khác là sự cố nghiêm trọng cần điều tra ngay, không phải sai số chấp nhận được.

---

## 14. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Thanh toán một cổng, ledger đầy đủ, đối soát thủ công |
| **Phase 2** | Đối soát tự động, payout tự động, hoàn tiền |
| **Phase 3** | Nhiều cổng thanh toán, thanh toán nhà cung cấp, đa tiền tệ |
| **Phase 4** | Báo cáo tài chính nâng cao, dự báo dòng tiền |

---

## 15. Tài liệu liên quan

- [../adr/0008-financial-ledger.md](../adr/0008-financial-ledger.md) — quyết định về ledger bất biến
- [../01-business/monetization.md](../01-business/monetization.md) — mô hình doanh thu
- [../05-data/idempotency.md](../05-data/idempotency.md) — chống xử lý trùng
- [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md) — luồng đối soát
