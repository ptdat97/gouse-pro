# Module: Seller

| | |
|---|---|
| **Bounded Context** | Marketplace |
| **Phân loại** | Supporting |
| **Giai đoạn** | MVP |

---

## 1. Trách nhiệm

- Quản lý hồ sơ và danh tính nhà bán
- Quy trình đăng ký và phê duyệt
- Quản lý giấy tờ pháp lý và tài khoản ngân hàng
- Chính sách riêng của seller (hoa hồng, đổi trả, chu kỳ đối soát)
- Theo dõi và chấm điểm hiệu suất

## 2. KHÔNG thuộc trách nhiệm

| Việc | Thuộc module |
|---|---|
| Offer, giá bán | `marketplace` |
| Đơn hàng của seller | `fulfillment` |
| **Số dư và tiền** | `payment` |
| Tồn kho của seller | `inventory` |

**Ranh giới quan trọng:** module này sở hữu **danh tính và chính sách**. Nó **không** sở hữu offer, đơn hàng, hay tiền. Nếu gộp cả số dư vào đây, module trở nên khổng lồ và không tách được.

Muốn biết seller còn bao nhiêu tiền → gọi `payment.GetBalance()`. Không lưu trùng.

---

## 3. Own brand là seller nội bộ

Quyết định thiết kế quan trọng:

```text
Seller {
    id: "internal-own-brand"
    seller_type: INTERNAL
    commission_rate: 0
    inventory_owner: PLATFORM
    settlement_mode: INTERNAL_LEDGER
}
```

**Lợi ích:**

| | Nếu own brand là seller nội bộ | Nếu là đường đi riêng |
|---|---|---|
| Luồng đơn hàng | Một luồng duy nhất | Hai luồng, dễ phân kỳ |
| Giỏ lẫn own brand + seller | Tự nhiên | Cần logic đặc biệt |
| Báo cáo so sánh 1P/3P | Dùng chung cấu trúc | Gộp thủ công |
| Thêm own brand thứ hai | Không tốn công | Nhân bản logic |

Khác biệt bản chất duy nhất nằm ở **tầng ledger**: đơn own brand ghi doanh thu toàn phần + COGS; đơn marketplace ghi hoa hồng. Xem [payment.md](payment.md).

---

## 4. Bốn loại seller

```text
Individual · Business · Local Brand · Strategic Brand Partner
```

**Quyết định:** bốn loại này khác nhau ở **chính sách**, không ở **cấu trúc dữ liệu**. Cùng một aggregate `Seller`, phân biệt bằng `seller_type` và bản ghi chính sách gắn kèm.

Lý do: seller cá nhân có thể phát triển thành local brand. Nếu bốn loại là bốn bảng, nâng cấp là di trú dữ liệu; nếu một bảng, chỉ là đổi thuộc tính.

Chi tiết khác biệt: [../01-business/seller.md](../01-business/seller.md) mục 1.

---

## 5. Vòng đời và ràng buộc chuyển trạng thái

```text
Applied → Pending Review → Approved → Active
                 ↓                       ↕
             Rejected              Suspended / On Vacation
                                         ↓
                                    Terminated
```

### Ràng buộc bắt buộc

| Chuyển trạng thái | Hệ quả |
|---|---|
| → Suspended | Ẩn offer, **KHÔNG hủy đơn đang xử lý**, giữ payout |
| → On Vacation | Ẩn offer, seller vẫn phải hoàn tất đơn đang có |
| → Terminated | Ẩn offer, hoàn tất/hủy đơn có kiểm soát, đối soát lần cuối, chi trả số dư |

**Điểm dễ sai:** đình chỉ seller **không được** hủy đơn hàng khách đã trả tiền. Phải để seller hoàn tất, hoặc nền tảng hủy có kiểm soát và hoàn tiền khách.

---

## 6. Chấm điểm hiệu suất

| Chỉ số | Ngưỡng cảnh báo |
|---|---|
| Tỷ lệ hủy đơn do seller | > 3% |
| Thời gian xác nhận đơn | > 24 giờ |
| Thời gian bàn giao vận chuyển | > 48 giờ |
| Tỷ lệ hoàn do lỗi mô tả | > 5% |
| Điểm đánh giá trung bình | < 4.0/5 |
| Tỷ lệ khiếu nại | > 2% |
| Độ chính xác tồn kho | < 95% |

**Nguyên tắc P14:** chấm điểm bằng **quy tắc tường minh**, công khai với seller. Mô hình hộp đen tạo tranh chấp không giải quyết được.

Hiệu suất ảnh hưởng: thứ hạng buy box, khả năng tham gia chiến dịch, tỷ lệ hoa hồng ưu đãi, trạng thái tài khoản.

---

## 7. Dữ liệu sở hữu

```sql
seller
seller_store
seller_document          -- giấy phép, ủy quyền thương hiệu
seller_bank_account
seller_policy            -- hoa hồng, đổi trả, chu kỳ đối soát, reserve
seller_performance       -- chỉ số theo kỳ
seller_staff             -- nhân viên của seller
```

`seller_policy` có `reserve_rate` và `reserve_hold_days` — cơ chế giữ lại một phần doanh thu với seller mới hoặc có tỷ lệ hoàn cao. Xem [../01-business/monetization.md](../01-business/monetization.md) mục 6.2.

---

## 8. Interface công khai

```go
type PublicAPI interface {
    GetSeller(ctx, sellerID string) (*SellerView, error)
    GetSellersByIDs(ctx, ids []string) (map[string]SellerView, error)
    GetSellerStore(ctx, sellerID string) (*StoreView, error)

    IsSellerActive(ctx, sellerID string) (bool, error)
    GetSellerPolicy(ctx, sellerID string) (*PolicyView, error)
    GetSellerPerformance(ctx, sellerID string) (*PerformanceView, error)

    ApplyAsSeller(ctx, req ApplicationRequest) (*SellerView, error)
    ApproveSeller(ctx, sellerID string, approvedBy string) error
    SuspendSeller(ctx, sellerID string, reason string) error
}
```

---

## 9. Event

**Phát ra:** `seller.applied`, `seller.approved`, `seller.rejected`, `seller.suspended`, `seller.reactivated`, `seller.terminated`, `seller.performance_updated`

**Lắng nghe:**

| Event | Từ | Hành động |
|---|---|---|
| `fulfillment_order.shipped` | fulfillment | Cập nhật chỉ số thời gian xử lý |
| `fulfillment_order.cancelled` | fulfillment | Cập nhật tỷ lệ hủy |
| `return.inspected` | return | Cập nhật tỷ lệ hoàn theo lý do |
| `order.placed` | order | Cập nhật thống kê doanh số |

---

## 10. Quy tắc nghiệp vụ

| # | Quy tắc |
|---|---|
| 1 | Seller ACTIVE phải có tài khoản ngân hàng đã xác minh |
| 2 | Đình chỉ không hủy đơn đang xử lý |
| 3 | Chấm điểm bằng quy tắc công khai |
| 4 | Không lưu số dư — gọi `payment.GetBalance()` |
| 5 | Own brand là seller `INTERNAL` |
| 6 | Chấm dứt phải đối soát và chi trả số dư còn lại |
| 7 | Seller không bao giờ thấy dữ liệu seller khác |

Quy tắc 7 là ràng buộc bảo mật cấp cao nhất của marketplace — vi phạm sẽ mất niềm tin toàn bộ nhà bán.

---

## 11. Giai đoạn triển khai

| Giai đoạn | Phạm vi |
|---|---|
| **MVP** | Đăng ký, duyệt thủ công, hồ sơ cơ bản, own brand nội bộ |
| **Phase 2** | Chấm điểm hiệu suất, chính sách riêng, tài khoản nhân viên |
| **Phase 3** | Duyệt tự động một phần, phân hạng seller, chương trình ưu đãi |

---

## 12. Tài liệu liên quan

- [../01-business/seller.md](../01-business/seller.md) — tác nhân nhà bán
- [marketplace.md](marketplace.md) — offer và hoa hồng
- [../07-workflows/seller-onboarding.md](../07-workflows/seller-onboarding.md)
