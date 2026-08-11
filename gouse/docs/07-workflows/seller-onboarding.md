# Luồng: Đăng ký và phê duyệt nhà bán

## 1. Sequence diagram

```mermaid
sequenceDiagram
    autonumber
    actor S as Người bán
    participant API as API
    participant Sel as seller
    participant Cat as catalog
    actor Adm as Nhân viên duyệt
    participant Idt as identity
    participant Pay as payment
    participant Bus as Event Bus

    S->>API: POST /sellers (hồ sơ, giấy tờ, tài khoản NH)
    API->>Sel: ApplyAsSeller
    Sel->>Sel: Kiểm tra: mã số thuế hợp lệ,<br/>giấy tờ đầy đủ
    Sel->>Sel: status = PENDING_REVIEW
    Sel->>Bus: seller.applied
    Bus->>Adm: thông báo hồ sơ mới

    Adm->>Sel: Xác minh giấy phép kinh doanh
    Adm->>Sel: Xác minh tài khoản ngân hàng
    Note over Adm: Xác minh tài khoản NH là BẮT BUỘC —<br/>sai tài khoản = chuyển tiền nhầm người

    alt Hồ sơ không đạt
        Adm->>Sel: RejectSeller(lý do cụ thể)
        Sel->>Bus: seller.rejected
        Bus->>S: email nêu rõ thiếu gì, cách bổ sung
    else Hồ sơ đạt
        Adm->>Sel: ApproveSeller(chính sách hoa hồng, reserve)
        Sel->>Sel: status = APPROVED → ACTIVE
        Sel->>Bus: seller.approved

        Bus->>Idt: cấp vai trò SELLER_OWNER
        Bus->>Pay: tạo tài khoản SELLER_PAYABLE
        Bus->>S: email chào mừng + hướng dẫn
    end

    opt Bán thương hiệu được bảo vệ
        S->>Cat: Tải lên giấy ủy quyền thương hiệu
        Adm->>Cat: Duyệt brand_authorization<br/>(có valid_until)
    end
```

---

## 2. Các bước xác minh bắt buộc

| Bước | Vì sao bắt buộc |
|---|---|
| Giấy phép kinh doanh | Trách nhiệm pháp lý, chống seller ảo |
| Mã số thuế | Nghĩa vụ thuế, hóa đơn |
| **Tài khoản ngân hàng** | **Sai tài khoản = chuyển tiền nhầm người, rất khó thu hồi** |
| Thông tin liên hệ | Xử lý sự cố, khiếu nại |
| Giấy ủy quyền thương hiệu | Chống hàng giả |

**Về xác minh tài khoản ngân hàng:** phải xác minh tên chủ tài khoản khớp với tên doanh nghiệp/cá nhân đăng ký. Đây là bước chống gian lận quan trọng — kẻ gian có thể đăng ký seller với giấy tờ giả nhưng tài khoản ngân hàng của mình.

---

## 3. Chính sách gán khi phê duyệt

```text
ApproveSeller(seller_id, {
    seller_type:          BUSINESS,
    commission_policy_id: pol_01J9X,     -- tỷ lệ theo ngành hàng
    reserve_rate:         1000,          -- giữ 10% trong 30 ngày
    reserve_hold_days:    30,
    settlement_cycle:     WEEKLY,
    return_policy_days:   7
})
```

**Về `reserve_rate`:** giữ lại một tỷ lệ doanh thu với seller mới. Đây là cơ chế bảo vệ khi seller có tỷ lệ hoàn hàng cao hoặc bỏ trốn với số dư âm.

Tỷ lệ này giảm dần khi seller chứng minh được độ tin cậy.

---

## 4. Đình chỉ và chấm dứt

```mermaid
stateDiagram-v2
    [*] --> Applied
    Applied --> PendingReview
    PendingReview --> Rejected: hồ sơ không đạt
    PendingReview --> Approved
    Rejected --> PendingReview: nộp lại
    Approved --> Active
    Active --> Suspended: vi phạm / hiệu suất kém
    Active --> OnVacation: seller chủ động
    Suspended --> Active: khắc phục xong
    OnVacation --> Active
    Suspended --> Terminated
    Active --> Terminated
    Terminated --> [*]
```

### Ràng buộc bắt buộc khi đình chỉ

```mermaid
sequenceDiagram
    actor Adm as Nhân viên
    participant Sel as seller
    participant Bus as Event Bus
    participant Mkt as marketplace
    participant Pay as payment
    participant Ful as fulfillment

    Adm->>Sel: SuspendSeller(reason, hold_payouts=true)
    Sel->>Bus: seller.suspended

    Bus->>Mkt: ẩn TOÀN BỘ offer
    Bus->>Pay: giữ payout

    Note over Ful: ĐƠN ĐANG XỬ LÝ KHÔNG BỊ HỦY
    Ful-->>Adm: báo cáo 8 đơn đang xử lý
    Note over Adm: Phải theo dõi: seller hoàn tất<br/>hoặc admin xử lý có kiểm soát
```

**Quy tắc quan trọng:** đình chỉ seller **không được hủy** đơn hàng khách đã trả tiền.

```text
Nếu hủy hết đơn khi đình chỉ:
    → khách vô tội bị hủy đơn
    → phải hoàn tiền hàng loạt
    → tổn hại uy tín nền tảng

Cách đúng:
    → ẩn offer (không nhận đơn mới)
    → để seller hoàn tất đơn đang có
    → nếu seller không hợp tác: admin hủy có kiểm soát, hoàn tiền khách
```

### Chấm dứt

```text
1. Ẩn toàn bộ offer
2. Hoàn tất hoặc hủy có kiểm soát các đơn đang có
3. Chờ hết thời hạn đổi trả của đơn cuối cùng
4. Đối soát lần cuối
5. Chi trả số dư còn lại (hoặc thu hồi nếu âm)
6. Lưu trữ hồ sơ theo quy định
```

---

## 5. Own brand — seller nội bộ

```text
Own brand KHÔNG đi qua luồng đăng ký này.

Được tạo trực tiếp:
    Seller {
        seller_type: INTERNAL
        commission_rate: 0
        inventory_owner: PLATFORM
        settlement_mode: INTERNAL_LEDGER
    }
```

Nhờ mô hình hóa own brand như một seller, mọi luồng đơn hàng dùng chung cấu trúc. Xem [../04-modules/seller.md](../04-modules/seller.md) mục 3.

---

## 6. Điểm cần giám sát

| Chỉ báo | Ngưỡng |
|---|---|
| Thời gian duyệt hồ sơ | < 3 ngày làm việc |
| Tỷ lệ từ chối | Theo dõi xu hướng |
| Time to first sale | Từ duyệt tới đơn đầu tiên |
| Seller có ủy quyền hết hạn | Cảnh báo trước 30 ngày |
| Seller bị đình chỉ có đơn treo | Cảnh báo ngay |

---

## 7. Tài liệu liên quan

- [../04-modules/seller.md](../04-modules/seller.md)
- [../01-business/seller.md](../01-business/seller.md)
- [../06-api/seller-api.md](../06-api/seller-api.md)
- [product-publishing.md](product-publishing.md) — bước tiếp theo
