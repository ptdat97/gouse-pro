# ADR-0008: Sổ cái tài chính bất biến

**Trạng thái:** Accepted

---

## Context

Nền tảng **giữ tiền hộ nhiều bên**:

```text
Khách trả 300.000đ
    ├── 250.500đ → phải trả Seller A
    ├──  30.000đ → doanh thu nền tảng
    ├──  15.000đ → phải trả Creator X
    └──   4.500đ → chi phí cổng thanh toán
```

Đặc điểm nghiệp vụ tạo ra yêu cầu nghiêm ngặt:

```text
- Tiền KHÔNG thuộc về nền tảng khi khách trả — là khoản thu hộ
- Chi trả theo chu kỳ, sau khi hết hạn đổi trả
- Hoàn hàng làm đảo ngược toàn bộ chuỗi
- Seller và creator sẽ khiếu nại về số tiền
- GMV ≠ doanh thu (đơn 1P ghi toàn phần, đơn 3P chỉ ghi hoa hồng)
- Nghĩa vụ lưu trữ chứng từ kế toán
```

Câu hỏi: mô hình hóa tiền như thế nào?

---

## Decision

### Quyết định 1: Sổ cái chỉ ghi thêm (append-only)

```text
KHÔNG BAO GIỜ:
    ✗ UPDATE một bút toán đã ghi
    ✗ DELETE một bút toán
    ✗ Sửa số tiền

Sửa sai bằng: ghi bút toán ĐIỀU CHỈNH mới.
```

### Quyết định 2: Bút toán kép (double-entry)

```text
LedgerEntry — một sự kiện tài chính
    └── LedgerLine[] — các dòng ghi nợ/ghi có
                        Σ DEBIT phải = Σ CREDIT
```

### Quyết định 3: Số dư là kết quả TÍNH TOÁN

```text
Balance(account) = Σ CREDIT − Σ DEBIT

KHÔNG lưu số dư như một trường được cập nhật.
```

### Quyết định 4: `LedgerEntry` là aggregate, không phải `Account`

---

## Alternatives

### A. Bảng giao dịch đơn giản, cập nhật số dư — **bị loại**

```text
transaction { id, seller_id, amount, type }
seller_balance { seller_id, balance }  ← cập nhật mỗi giao dịch

Ưu:
    + Đơn giản, dễ hiểu
    + Truy vấn số dư nhanh

Nhược (quyết định):
    − Số dư có thể LỆCH với tổng giao dịch (lỗi, tranh chấp đồng thời)
    − Không biết lệch từ lúc nào
    − Điểm nghẽn ghi: mọi giao dịch cập nhật CÙNG một dòng
    − Không tái dựng được số dư tại thời điểm quá khứ
    − Không phát hiện được sai sót
```

**Lý do loại chính:** không có cách nào biết số dư đúng hay sai. Với hệ thống giữ tiền hộ, đây là rủi ro không chấp nhận được.

### B. Cho phép sửa bút toán khi sai — **bị loại**

```text
Ưu:
    + Trực giác hơn: sai thì sửa

Nhược (quyết định):
    − Mất lịch sử — không biết trước đó ghi gì
    − Không kiểm toán được
    − Tranh chấp với seller không có bằng chứng
    − Che giấu lỗi thay vì làm lộ ra
```

**Ví dụ so sánh:**

```text
Phát hiện ghi nhầm hoa hồng 30.000đ, đúng phải 25.000đ

❌ SAI:  UPDATE ledger_line SET amount = 25000 WHERE ...
         → không ai biết đã từng ghi 30.000đ

✅ ĐÚNG: Bút toán mới loại ADJUSTMENT:
           DEBIT   PLATFORM_REVENUE   5.000
           CREDIT  SELLER_PAYABLE     5.000
           description: "Điều chỉnh hoa hồng đơn #1000, ghi nhầm tỷ lệ"
         → kết quả giống nhau, nhưng GIỮ ĐƯỢC LỊCH SỬ
```

### C. `Account` là aggregate chứa các bút toán — **bị loại**

```text
Ưu:
    + Trực giác: tài khoản chứa giao dịch của nó

Nhược (quyết định):
    − Account của nền tảng có HÀNG TRIỆU bút toán
      → aggregate vô hạn, không tải được
    − Mọi giao dịch chạm vào Account nền tảng
      → điểm nghẽn ghi nghiêm trọng nhất hệ thống
    − Số dư lưu trong Account có thể lệch với tổng bút toán
```

**Cách đã chọn:**

```text
LedgerEntry là aggregate — bất biến, độc lập, ghi song song được
Account chỉ là DANH MỤC (ai sở hữu tài khoản nào)
Balance là KẾT QUẢ TÍNH TOÁN
```

### D. Event Sourcing đầy đủ — **bị loại (nhưng áp dụng tinh thần)**

```text
Nhược: độ phức tạp cao cho toàn hệ thống, truy vấn khó

Áp dụng có chọn lọc: mô hình append-only chỉ dùng ở nơi
có lý do rõ ràng — ledger, inventory_movement, attribution,
point_transaction
```

---

## Consequences

### Tích cực

```text
✓ Tái dựng số dư tại BẤT KỲ thời điểm quá khứ
✓ Kiểm toán được — có bằng chứng không thể sửa
✓ Tranh chấp với seller giải quyết được
✓ Sai sót LỘ RA thay vì bị che giấu
✓ Ghi song song được — không có điểm nghẽn
✓ Phân biệt GMV/doanh thu ngay ở tầng ghi sổ
```

### Tiêu cực

```text
− Tính số dư chậm hơn (phải tổng hợp nhiều bút toán)
− Bảng ledger lớn nhanh
− Sửa sai phức tạp hơn (phải hiểu bút toán kép)
− Cần đào tạo đội về khái niệm kế toán
```

### Giải pháp cho vấn đề hiệu năng

```text
balance_snapshot {
    account_id
    as_of_ledger_entry_id     ← chốt tại bút toán nào
    balance
    computed_at
}

Số dư hiện tại = snapshot gần nhất + tổng bút toán sau đó
```

**Điểm mấu chốt:** snapshot là **cache có thể tính lại**, không phải nguồn sự thật. Job hàng ngày kiểm tra snapshot có khớp với tổng bút toán không — lệch = cảnh báo nghiêm trọng.

---

## Thực thi ở tầng database

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

-- Lớp bảo vệ cuối cùng
CREATE RULE ledger_entry_no_update AS ON UPDATE TO ledger_entry DO INSTEAD NOTHING;
CREATE RULE ledger_entry_no_delete AS ON DELETE TO ledger_entry DO INSTEAD NOTHING;
```

Kể cả khi có lỗi code hoặc thao tác thủ công nhầm, database vẫn từ chối.

---

## Ví dụ bút toán

**Đơn marketplace 300.000đ:**

```text
LedgerEntry: ORDER_REVENUE, ref=Order#1000

  DEBIT   PLATFORM_CASH                     300.000
  CREDIT  SELLER_PAYABLE (seller A)         250.500
  CREDIT  PLATFORM_REVENUE                   30.000
  CREDIT  CREATOR_PAYABLE (creator X)        15.000
  CREDIT  FEE_EXPENSE                         4.500
  ────────────────────────────────────────────────
  Σ DEBIT = Σ CREDIT = 300.000  ✓
```

**Đơn own brand 300.000đ:**

```text
LedgerEntry: ORDER_REVENUE
  DEBIT   PLATFORM_CASH      300.000
  CREDIT  PLATFORM_REVENUE   300.000

LedgerEntry: COGS
  DEBIT   COGS               120.000
  CREDIT  INVENTORY_ASSET    120.000
```

**Quan sát:** đơn own brand ghi doanh thu toàn phần; đơn marketplace chỉ hoa hồng là doanh thu. Phân biệt GMV/doanh thu nằm ở **tầng ghi sổ**, không phải xử lý sau bằng báo cáo.

---

## Yêu cầu kèm theo

### 1. Mọi thao tác tài chính phải idempotent

```text
Chuyển tiền hai lần do lỗi thử lại là loại lỗi tốn kém
và khó thu hồi nhất.

→ ledger_entry.idempotency_key UNIQUE
→ Payout: kiểm tra trạng thái TRƯỚC khi gọi API ngân hàng
```

### 2. Kiểm tra tính toàn vẹn hàng ngày

```text
Ba chỉ số PHẢI LUÔN BẰNG 0:
    1. Số bút toán không cân bằng
    2. Độ lệch giữa snapshot và tổng bút toán
    3. Độ lệch khi đối chiếu với PSP/ngân hàng

Bất kỳ giá trị khác = SỰ CỐ NGHIÊM TRỌNG, không phải sai số.
```

### 3. Đóng băng tỷ lệ hoa hồng vào đơn hàng

```text
Nếu tính động khi đối soát:
    → đổi chính sách hoa hồng làm số tiền đơn CŨ thay đổi
    → chạy đối soát hai lần ra hai kết quả
    → không kiểm toán được

→ commission_rate và commission_amount ĐÓNG BĂNG trong OrderLine
```

### 4. Money là số nguyên, không phải số thực

```text
0.1 + 0.2 = 0.30000000000000004

Với hàng triệu giao dịch, sai số tích lũy thành tiền thật.
Và độ lệch đối soát phải bằng 0.

→ Money { amount int64, currency Currency }
→ Chia tiền dùng Allocate() để không mất đồng nào
```

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Tính số dư chậm hơn | Không bao giờ sai, luôn kiểm chứng được |
| Bảng ledger lớn | Lịch sử đầy đủ, kiểm toán được |
| Sửa sai phức tạp hơn | Giữ lịch sử, sai sót lộ ra |
| Cần đào tạo về kế toán | Mô hình chuẩn ngành, người kế toán hiểu ngay |

---

## Tài liệu liên quan

- [../04-modules/payment.md](../04-modules/payment.md)
- [../01-business/monetization.md](../01-business/monetization.md)
- [../02-domain/value-objects.md](../02-domain/value-objects.md) mục 2
- [../05-data/idempotency.md](../05-data/idempotency.md)
