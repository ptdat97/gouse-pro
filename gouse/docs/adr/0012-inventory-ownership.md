# ADR-0012: Chủ sở hữu tồn kho SUY RA từ nhà bán, không bằng nhà bán

**Trạng thái:** Accepted
**Ngày:** 20/08/2026

---

## Context

Một SKU có thể có tồn kho của NHIỀU chủ sở hữu cùng lúc. Đó không phải
trường hợp biên — đó là lý do cái chợ tồn tại: nhiều nhà bán cùng chào một
mã hàng, mỗi người giữ hàng của mình.

Khóa nghiệp vụ của `InventoryItem` đã tách ba chiều từ đầu:

```text
(sku_id, stock_location_id, inventory_owner_id)
     ↑            ↑                  ↑
   hàng gì      ở đâu             CỦA AI
```

Tách `stock_location_id` khỏi `inventory_owner_id` là để mô hình hóa hàng
KÝ GỬI: hàng của nhà bán nằm trong kho nền tảng vẫn là tài sản của nhà bán
(fulfillment.md mục 2.3).

Nhưng còn một câu hỏi tài liệu chưa nói thành lời: **đổi `seller_id` lấy
`inventory_owner_id` như thế nào?**

Câu trả lời "chúng bằng nhau" là sai, và sai ở một chỗ rất dễ vấp. Own
brand của nền tảng là một **seller nội bộ** (own-brand.md mục 7,
seller.md mục 3) — nó có bản ghi seller thật, có offer, đi chung một luồng
đơn hàng với nhà bán ngoài, để giỏ lẫn 1P và 3P hoạt động tự nhiên. Nghĩa
là `seller_id` của nó là một ULID thật. Nhưng hàng thì **không phải của
nó** — hàng là tài sản NỀN TẢNG, và bản ghi seller nội bộ khai đúng điều
đó bằng `inventory_owner: PLATFORM`.

### Lỗi đã xảy ra (P3-18)

Ba module cùng vấp, mỗi module một cách, và cả ba đều IM LẶNG:

```text
checkout      giữ hàng lấy bản ghi ĐẦU TIÊN còn đủ hàng, bất kể của ai
marketplace   nhập kho lúc tạo offer dùng thẳng seller_id làm chủ sở hữu
inventory     kiểm kê tìm bản ghi theo seller_id
```

Kiểm chứng bằng đơn thật trước khi sửa: đặt 1 món qua offer của nhà bán,
kho NỀN TẢNG 100 → 99 (giữ 1), còn 40 cái của chính nhà bán không nhúc
nhích. Với hai nhà bán ngoài thì hậu quả nặng hơn: bán cho B trừ kho của
A — A hụt hàng vì những đơn chưa bao giờ nhận, B giao hàng từ số tồn hệ
thống tưởng vẫn nguyên. Hai sổ sách cùng sai và cùng không báo.

Hai đường còn lại tạo ra bản ghi ở nhầm chủ: tồn tại trong database mà
không đường nào đọc tới.

---

## Decision

**Chủ sở hữu tồn kho là thuộc tính SUY RA từ nhà bán**, và quy tắc suy ra
nằm ở MỘT chỗ duy nhất:

```go
// internal/modules/inventory/public.go
func OwnerForSeller(sellerID string, isInternal bool) string {
    if isInternal {
        return PlatformOwnerID   // "own_platform"
    }
    return sellerID
}
```

Đặt ở module `inventory` vì hằng số `own_platform` thuộc về nó. Để module
`seller` tự sinh chuỗi đó nghĩa là cùng một giá trị định nghĩa ở hai nơi,
và hai nơi sẽ lệch. Bên gọi ghép hai mảnh: `IsInternal` từ `seller`, định
danh từ `inventory`.

### Giữ hàng: chủ sở hữu là điều kiện LOẠI TRỪ

Không phải thứ tự ưu tiên. Bản ghi tồn kho của chủ khác **không phải lựa
chọn kém hơn — nó không phải lựa chọn.** Nhà bán hết hàng thì đơn THẤT
BẠI; không có đường lùi sang kho người khác.

"Mượn tạm cho đơn chạy được" chính là lỗi vừa sửa, chỉ khác là có chủ đích
— và nó vẫn trừ hàng của người không bán món đó.

### Module đứng dưới thì dùng cổng do bên gọi khai

`inventory` nằm DƯỚI `seller` trong đồ thị phụ thuộc nên không hỏi ngược
lên được. Đường kiểm kê dùng cổng `OwnerResolver` do chính nó khai báo,
`cmd/api` nối — cùng mẫu với `TokenVerifier` và `CustomerResolver`.

---

## Consequences

**Được:**

- Một nguồn sự thật cho một quy tắc đang có ba nơi cần tới.
- Hàng ký gửi vẫn đúng: lọc theo CHỦ SỞ HỮU, không theo KHO.
- Bất biến phát biểu được thành câu, nên test được:
  `offer → seller → inventory owner → reservation → fulfillment`
  không được đi chệch sang chủ khác ở bất kỳ mắt xích nào.

**Mất:**

- Mỗi đường ghi/đọc tồn kho theo nhà bán phải tra thêm hồ sơ seller.
  Chấp nhận được: kết quả tra được dùng lại theo seller trong một lời gọi,
  và giỏ 10 món của cùng nhà bán chỉ tốn 1 lượt.
- Thêm một cổng nữa ở `cmd/api`.

**Rủi ro còn lại:** quy tắc đúng không tự lan sang đường mới. Mọi đường
mới chạm tồn kho theo nhà bán PHẢI đi qua `OwnerForSeller` — đó là điều
cần kiểm khi review, vì archcheck không bắt được loại vi phạm này.

---

## Kiểm chứng

Ba test hồi quy, mỗi test được xác nhận ĐỎ khi bỏ đúng phần nó canh:

```text
TestGiuHangDungChuSoHuu             bỏ lọc chủ → "hàng của A bị trừ cho đơn của B: 10 → 7"
TestKhongMuonKhoNguoiKhacKhiHetHang bỏ lọc chủ → phiên mở được dù chỉ còn 1/5
TestOwnBrandLayHangCuaNenTang       bỏ quy tắc INTERNAL → own brand không mua được hàng nền tảng
```

Test thứ ba XANH cả khi có lỗi gốc, vì nó canh việc khác — phải phá đúng
cái nó canh mới thấy nó đỏ.

Và một test xuyên chuỗi: `internal/e2e/TestDonNhieuNhaBanDiHetChuoi` dựng
đơn hai nhà bán cùng bán một SKU, đi qua giữ hàng → đặt đơn → event → tách
đơn thực hiện. Loại lỗi này KHÔNG thể thấy từ test của một module: mỗi
module dựng bản giả cho hàng xóm, và bản giả cư xử theo giả định của người
viết test.

---

## Tài liệu liên quan

- [../04-modules/inventory.md](../04-modules/inventory.md) mục 3.1 — khóa ba chiều
- [../01-business/own-brand.md](../01-business/own-brand.md) mục 7 — own brand là seller nội bộ
- [0007-marketplace-order-model.md](0007-marketplace-order-model.md) — Offer và tách Order/FulfillmentOrder
