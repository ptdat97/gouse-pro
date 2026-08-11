# Tác nhân: Nhà bán (Seller)

## 1. Phân loại

```text
Individual Seller        — Cá nhân kinh doanh nhỏ
Business Seller          — Doanh nghiệp có đăng ký kinh doanh
Local Brand              — Thương hiệu nội địa có bản sắc riêng
Strategic Brand Partner  — Đối tác thương hiệu chiến lược, có thỏa thuận riêng
```

### Bảng phân biệt

| | Individual | Business | Local Brand | Strategic Partner |
|---|---|---|---|---|
| Giấy phép kinh doanh | Không bắt buộc | Bắt buộc | Bắt buộc | Bắt buộc |
| Hợp đồng | Điều khoản chuẩn | Điều khoản chuẩn | Điều khoản chuẩn | Hợp đồng riêng |
| Tỷ lệ hoa hồng | Chuẩn theo ngành hàng | Chuẩn | Thương lượng | Thương lượng |
| Số SKU tối đa | Có giới hạn | Cao hơn | Không giới hạn | Không giới hạn |
| Trang thương hiệu riêng | Không | Không | Có | Có |
| Tự quản lý bộ sưu tập | Không | Hạn chế | Có | Có |
| Dùng fulfillment nền tảng | Tùy chọn | Tùy chọn | Tùy chọn | Thường có |
| Chu kỳ đối soát | Chuẩn | Chuẩn | Chuẩn | Có thể riêng |
| Quản lý tài khoản riêng | Không | Không | Có | Có |

**Hệ quả kiến trúc:** bốn loại này khác nhau ở **chính sách**, không phải ở **cấu trúc dữ liệu**. Cả bốn dùng chung aggregate `Seller`, phân biệt bằng thuộc tính `seller_type` và bằng bản ghi chính sách gắn kèm. Không tạo bốn bảng riêng.

Lý do: một seller cá nhân có thể phát triển thành local brand. Nếu bốn loại là bốn bảng, việc nâng cấp là di trú dữ liệu; nếu là một bảng, chỉ là đổi thuộc tính và chính sách.

---

## 2. Own Brand được mô hình hóa thế nào?

Đây là một quyết định thiết kế quan trọng.

**Quyết định:** Own brand được mô hình hóa như một **seller nội bộ đặc biệt** (`seller_type = internal`).

**Lý do:**

| Nếu own brand là seller nội bộ | Nếu own brand là đường đi riêng |
|---|---|
| Một luồng đơn hàng duy nhất | Hai luồng song song, dễ phân kỳ |
| Giỏ hàng lẫn own brand + seller xử lý tự nhiên | Cần logic đặc biệt để trộn |
| Báo cáo so sánh 1P/3P dễ dàng | Phải gộp thủ công từ hai nguồn |
| Thêm own brand thứ hai không tốn công | Phải nhân bản logic |

**Điểm khác biệt được xử lý bằng chính sách, không bằng cấu trúc:**

```text
Own brand seller:
  - commission_rate = 0        (không tự thu hoa hồng của mình)
  - settlement = nội bộ         (không payout ra ngoài, chỉ ghi sổ nội bộ)
  - inventory owner = platform  (nền tảng sở hữu hàng)
  - có liên kết tới Supply Chain (production batch, COGS)
```

Sự khác biệt bản chất duy nhất: đơn own brand ghi nhận **doanh thu toàn phần và COGS**, đơn marketplace ghi nhận **hoa hồng**. Khác biệt này nằm ở tầng ledger, không ở tầng đơn hàng. Xem [../04-modules/payment.md](../04-modules/payment.md).

---

## 3. Trách nhiệm

Seller chịu trách nhiệm:

- Cung cấp thông tin pháp lý và tài khoản nhận tiền chính xác
- Đăng và duy trì thông tin sản phẩm đúng sự thật
- Đảm bảo hàng có sẵn đúng như tồn kho đã khai báo
- Xử lý đơn trong thời hạn cam kết
- Đóng gói đúng quy cách
- Xử lý yêu cầu đổi trả theo chính sách nền tảng
- Đảm bảo hàng chính hãng, không vi phạm sở hữu trí tuệ

**Trách nhiệm cuối cùng đặc biệt quan trọng với thời trang** — hàng giả thương hiệu là rủi ro pháp lý và uy tín lớn nhất của marketplace thời trang. Hệ thống phải hỗ trợ quy trình xác minh thương hiệu và gỡ bỏ nhanh. Xem [marketplace.md](marketplace.md).

---

## 4. Quyền hạn

| Hành động | Điều kiện |
|---|---|
| Tạo sản phẩm | Seller đã được duyệt |
| Tạo offer trên sản phẩm có sẵn | Seller đã được duyệt, sản phẩm được phép bán chung |
| Sửa giá offer của mình | Luôn được (trong khung giá cho phép) |
| Cập nhật tồn kho của mình | Luôn được |
| Xem đơn hàng | **Chỉ fulfillment order thuộc về mình** |
| Xem thông tin khách | **Chỉ thông tin cần cho giao hàng, chỉ trên đơn của mình** |
| Xem báo cáo tài chính | Chỉ số liệu của mình |
| Tạo khuyến mãi | Chỉ trên offer của mình |
| Tham gia chiến dịch nền tảng | Theo lời mời hoặc đăng ký |

**Nguyên tắc cách ly quan trọng nhất:** seller **không bao giờ** thấy được dữ liệu của seller khác, kể cả gián tiếp qua báo cáo tổng hợp có thể suy ngược. Đây là ràng buộc bảo mật cấp cao nhất của marketplace — vi phạm sẽ mất niềm tin của toàn bộ nhà bán.

Cụ thể: seller thấy `FulfillmentOrder` của mình, **không** thấy `Order` đầy đủ (vì đơn có thể chứa hàng của seller khác). Đây là lý do kỹ thuật thứ hai cho việc tách Order/Fulfillment Order.

---

## 5. Quan hệ doanh thu

```text
Seller bán hàng 300.000đ
    │
    ├── Hoa hồng nền tảng (theo ngành hàng, ví dụ 10%)    −30.000đ
    ├── Phí thanh toán (nếu áp dụng)                       −4.500đ
    ├── Phí fulfillment (nếu dùng kho nền tảng)           −15.000đ
    ├── Hoa hồng creator (nếu đơn đến từ affiliate)       −15.000đ
    │
    ▼
Ghi có vào số dư seller                                  235.500đ
    │
    ▼ (theo chu kỳ đối soát, sau khi hết hạn đổi trả)
Payout về tài khoản ngân hàng
```

**Quyết định chính sách quan trọng:** payout chỉ thực hiện **sau khi hết thời hạn đổi trả**. Nếu trả tiền ngay khi giao hàng, khi khách hoàn hàng nền tảng phải đòi lại tiền từ seller — rất khó thu hồi.

Điều này tạo ra khái niệm số dư nhiều trạng thái:

```text
Pending    — đơn đã giao, chưa hết hạn đổi trả
Available  — đã hết hạn đổi trả, sẵn sàng chi trả
Paid       — đã chuyển tiền
On Hold    — bị giữ do tranh chấp hoặc vi phạm
```

Xem [monetization.md](monetization.md) và [../04-modules/payment.md](../04-modules/payment.md).

---

## 6. Vòng đời

```text
    Đăng ký (Applied)
        │
        ▼
   Chờ duyệt (Pending Review)
        │
        ├──→ Bị từ chối (Rejected) ──→ có thể nộp lại
        │
        ▼
   Đã duyệt (Approved)
        │
        ▼
   Đang hoạt động (Active) ◄──────┐
        │                          │
        ├──→ Tạm ngưng (Suspended) ┘  (vi phạm, hiệu suất kém — có thể khôi phục)
        │
        ├──→ Tạm nghỉ (On Vacation)   (seller chủ động, offer ẩn tạm thời)
        │
        └──→ Chấm dứt (Terminated)     (vĩnh viễn)
```

### Ràng buộc khi thay đổi trạng thái

| Chuyển trạng thái | Hệ quả bắt buộc |
|---|---|
| → Suspended | Ẩn toàn bộ offer, **không** hủy đơn đang xử lý, giữ payout |
| → On Vacation | Ẩn offer, seller vẫn phải hoàn tất đơn đang có |
| → Terminated | Ẩn offer, hoàn tất hoặc hủy đơn đang có, đối soát lần cuối, chi trả số dư còn lại |

**Điểm dễ sai:** đình chỉ seller **không** được hủy đơn hàng khách đã trả tiền. Phải để seller hoàn tất, hoặc nền tảng hủy và hoàn tiền có kiểm soát. Xem [../04-modules/seller.md](../04-modules/seller.md).

---

## 7. Đánh giá hiệu suất (Seller Performance)

Chỉ số theo dõi:

| Chỉ số | Ngưỡng cảnh báo (đề xuất) |
|---|---|
| Tỷ lệ hủy đơn do seller | > 3% |
| Thời gian xác nhận đơn | > 24 giờ |
| Thời gian bàn giao vận chuyển | > 48 giờ |
| Tỷ lệ hoàn hàng do lỗi mô tả | > 5% |
| Điểm đánh giá trung bình | < 4.0/5 |
| Tỷ lệ khiếu nại | > 2% |

Hiệu suất ảnh hưởng tới: thứ hạng hiển thị, khả năng tham gia chiến dịch, tỷ lệ hoa hồng (nếu có chính sách thưởng), và trạng thái tài khoản.

**Nguyên tắc thiết kế:** chấm điểm bằng **quy tắc tường minh** trước, để dành interface cho mô hình phức tạp hơn sau (nguyên tắc P14). Seller phải hiểu được vì sao mình bị điểm thấp — một mô hình hộp đen sẽ tạo tranh chấp không giải quyết được.

---

## 8. Luồng nghiệp vụ chính

| Luồng | Tài liệu |
|---|---|
| Đăng ký và duyệt seller | [../07-workflows/seller-onboarding.md](../07-workflows/seller-onboarding.md) |
| Đăng bán sản phẩm | [../07-workflows/product-publishing.md](../07-workflows/product-publishing.md) |
| Xử lý đơn marketplace | [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md) |
| Đối soát và chi trả | [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md) |

---

## 9. Dữ liệu sở hữu

Module [seller](../04-modules/seller.md) sở hữu:

```text
seller                   — hồ sơ nhà bán
seller_store             — thông tin gian hàng
seller_document          — giấy tờ pháp lý, giấy ủy quyền thương hiệu
seller_bank_account      — tài khoản nhận tiền
seller_policy            — chính sách riêng: hoa hồng, đổi trả, chu kỳ đối soát
seller_performance       — chỉ số hiệu suất theo kỳ
```

Dữ liệu liên quan thuộc module khác:

```text
offer                    — thuộc module marketplace
fulfillment_order        — thuộc module fulfillment
seller_balance           — thuộc module payment
settlement               — thuộc module payment
```

**Ranh giới cần lưu ý:** module `seller` sở hữu **danh tính và chính sách** của nhà bán. Nó **không** sở hữu offer, đơn hàng hay tiền. Đây là ứng dụng nguyên tắc P5 — nếu module seller cũng giữ luôn số dư, nó sẽ trở thành module khổng lồ không tách được.
