# Nghiệp vụ: Marketplace

## 1. Vì sao marketplace tồn tại

Own brand không thể phủ hết nhu cầu. Marketplace giải quyết bài toán độ rộng danh mục mà không cần vốn tồn kho.

| Lý do | Giải thích |
|---|---|
| Mở rộng assortment nhanh | Thêm hàng nghìn SKU mà không cần sản xuất |
| Không tồn vốn | Seller giữ hàng, nền tảng không chịu rủi ro tồn kho |
| Thử nghiệm nhu cầu | Bán thử sản phẩm của seller trước khi own brand tự làm |
| Dữ liệu nhu cầu | Biết khách muốn gì mà own brand chưa có |
| Hiệu ứng mạng | Nhiều seller → nhiều lựa chọn → nhiều khách → nhiều seller |

Đánh đổi: biên lợi nhuận thấp hơn, ít kiểm soát chất lượng và trải nghiệm, rủi ro hàng giả.

**Vai trò chiến lược ít được nói tới:** marketplace là **phòng thí nghiệm nhu cầu** cho own brand. Nếu một kiểu áo của seller bán rất chạy, đó là bằng chứng thị trường đã được kiểm chứng để own brand làm phiên bản của mình với chất lượng và biên tốt hơn.

---

## 2. Mô hình Offer — quyết định kiến trúc trung tâm

### Vấn đề

Nhiều seller có thể bán **cùng một sản phẩm**. Ví dụ, ba seller cùng bán áo thun Uniqlo mã 450251:

```text
Product: Áo thun cotton Uniqlo U — mã 450251
├── Seller A → Offer A → 299.000đ, còn 12 cái, giao 2 ngày
├── Seller B → Offer B → 289.000đ, còn 3 cái,  giao 4 ngày
└── Seller C → Offer C → 310.000đ, còn 50 cái, giao 1 ngày
```

Khách hàng nhìn thấy **một trang sản phẩm**, với nhiều lựa chọn nhà bán.

### Quyết định

Tách `Offer` thành khái niệm riêng, nằm giữa `SKU` và `Inventory`:

```text
Product   — sản phẩm (thông tin chung, hình ảnh, mô tả)
   ↓
Variant   — tổ hợp thuộc tính (màu Trắng, size M)
   ↓
SKU       — đơn vị lưu kho định danh duy nhất
   ↓
Offer     — lời chào bán của MỘT seller cho SKU đó, có giá riêng
   ↓
Inventory — tồn kho của offer đó, tại điểm lưu kho của seller đó
```

**Điều gì thuộc về đâu:**

| Thuộc tính | Thuộc về | Lý do |
|---|---|---|
| Tên, mô tả, hình ảnh | Product | Chung cho mọi seller |
| Màu, size | Variant | Chung |
| Mã sản phẩm chuẩn | SKU | Chung — là cái để nhận diện "cùng một hàng" |
| **Giá** | **Offer** | Mỗi seller giá khác nhau |
| **Tồn kho** | **Offer** | Mỗi seller kho khác nhau |
| **Thời gian giao** | **Offer** | Phụ thuộc vị trí seller |
| **Chính sách đổi trả** | **Offer** | Có thể khác nhau |
| **Tình trạng hàng** | **Offer** | Mới / đã qua sử dụng |

### Vì sao không dùng cách đơn giản hơn?

**Phương án bị loại 1: mỗi seller tạo product riêng.**

Hậu quả: trang sản phẩm bị trùng lặp hàng loạt. Khách tìm "áo thun Uniqlo U" thấy 40 kết quả giống hệt nhau. Không so sánh được giá. Trải nghiệm tệ, SEO tệ.

**Phương án bị loại 2: gắn giá và tồn kho thẳng vào SKU, thêm cột `seller_id`.**

Hậu quả: SKU mất ý nghĩa là "định danh hàng hóa". Không trả lời được câu hỏi "có bao nhiêu nơi bán mã hàng này". Việc gom nhóm để so sánh trở thành truy vấn nhóm phức tạp thay vì quan hệ tự nhiên.

**Phương án bị loại 3: chỉ làm Offer khi nào thật sự có nhiều seller.**

Hậu quả: đây là cái bẫy phổ biến nhất. Đến lúc cần thì `price` và `stock` đã nằm rải rác trong hàng chục truy vấn, API, và màn hình. Việc tách ra trở thành dự án di trú lớn.

Vì vậy: **làm Offer từ ngày đầu, kể cả khi chỉ có own brand bán.** Ban đầu mỗi SKU có đúng một offer. Chi phí thêm rất nhỏ, chi phí không làm thì rất lớn. Xem [ADR-0007](../adr/0007-marketplace-order-model.md).

---

## 3. Buy Box — chọn offer mặc định

Khi một sản phẩm có nhiều offer, trang sản phẩm phải chọn một offer hiển thị mặc định. Đây gọi là buy box.

### Tiêu chí đề xuất

```text
Điểm buy box = f(
    giá (đã gồm phí vận chuyển),
    thời gian giao dự kiến,
    hiệu suất seller,
    tỷ lệ hoàn hàng,
    độ tin cậy tồn kho,
    đánh giá của khách
)
```

**Nguyên tắc thiết kế (P14):** bắt đầu bằng công thức có trọng số **tường minh và công khai**. Seller phải hiểu được vì sao mình không thắng buy box và làm gì để cải thiện. Một mô hình hộp đen sẽ tạo ra tranh chấp không giải quyết được và cảm giác bất công.

**Cảnh báo về cạnh tranh giá:** nếu buy box chỉ dựa vào giá thấp nhất, seller sẽ đua giảm giá tới mức không bền vững, chất lượng dịch vụ giảm theo. Trọng số phải cân bằng giữa giá và chất lượng phục vụ.

---

## 4. Kiểm soát chất lượng danh mục

Đây là rủi ro sống còn của marketplace thời trang.

### 4.1 Hàng giả thương hiệu

Rủi ro lớn nhất. Hậu quả: kiện tụng, mất quyền phân phối thương hiệu chính hãng, mất niềm tin khách hàng.

Cơ chế kiểm soát:

```text
Thương hiệu được bảo vệ (brand gating)
  → Chỉ seller có giấy ủy quyền mới được tạo offer

Xác minh trước khi đăng bán
  → Sản phẩm thuộc thương hiệu nổi tiếng phải qua duyệt thủ công

Quy trình gỡ bỏ nhanh
  → Chủ thương hiệu báo cáo vi phạm, gỡ trong thời gian cam kết

Theo dõi tín hiệu
  → Giá thấp bất thường, tỷ lệ khiếu nại hàng giả cao
```

**Hệ quả kiến trúc:** `Brand` phải có thuộc tính `protection_level`, và `seller_document` phải lưu được giấy ủy quyền có ngày hết hạn. Việc tạo offer trên thương hiệu được bảo vệ là một kiểm tra bắt buộc ở tầng domain, không phải quy trình thủ công bên ngoài hệ thống.

### 4.2 Trùng lặp sản phẩm

Nhiều seller đăng cùng một hàng nhưng tạo product mới thay vì gắn offer vào product có sẵn.

Cơ chế:

```text
Đối sánh khi đăng bán       — gợi ý sản phẩm đã có dựa trên mã, tên, hình ảnh
Danh mục chuẩn hóa          — với thương hiệu lớn, nền tảng tạo sẵn product
Quy trình gộp               — admin gộp product trùng, chuyển offer sang product chuẩn
```

Quy trình gộp phải giữ được lịch sử: đơn hàng cũ vẫn phải trỏ đúng, đánh giá phải được chuyển theo.

### 4.3 Mô tả sai lệch

Đặc biệt phổ biến với thời trang: ảnh không đúng màu thật, mô tả chất liệu sai, bảng size không chuẩn.

Tín hiệu phát hiện: tỷ lệ hoàn hàng với lý do "không giống mô tả" cao bất thường ở một seller hoặc một offer cụ thể.

---

## 5. Cấu trúc hoa hồng

Hoa hồng không nên là một con số duy nhất cho toàn nền tảng.

```text
Tỷ lệ hoa hồng = f(
    ngành hàng,          — giày có biên khác áo
    loại seller,         — strategic partner có thể thương lượng
    chương trình,        — seller mới được ưu đãi giai đoạn đầu
    hiệu suất,           — thưởng seller phục vụ tốt
    chiến dịch           — chiến dịch riêng có tỷ lệ riêng
)
```

**Hệ quả kiến trúc quan trọng:** tỷ lệ hoa hồng áp dụng cho một đơn hàng phải được **đóng băng vào đơn tại thời điểm đặt hàng** (nguyên tắc P9). Không tra cứu động khi đối soát.

Lý do: nếu tháng sau đổi chính sách hoa hồng, các đơn tháng trước không được phép thay đổi số tiền phải trả. Nếu tính động, đối soát sẽ ra kết quả khác nhau mỗi lần chạy — không kiểm toán được.

Xem [monetization.md](monetization.md).

---

## 6. Đối soát và chi trả

```text
Đơn giao thành công
    │
    ▼
Bắt đầu đếm thời hạn đổi trả (ví dụ 7 ngày)
    │
    ▼
Hết hạn đổi trả, không có yêu cầu trả hàng
    │
    ▼
Số dư seller: Pending → Available
    │
    ▼
Đến kỳ đối soát (ví dụ mỗi thứ Ba)
    │
    ▼
Tạo Settlement: tổng hợp mọi khoản trong kỳ
    │  + doanh thu bán hàng
    │  − hoa hồng
    │  − phí dịch vụ
    │  − hoàn tiền phát sinh
    │  ± điều chỉnh
    ▼
Seller xác nhận (hoặc tự động sau thời hạn)
    │
    ▼
Payout — chuyển tiền thật
    │
    ▼
Ghi bút toán vào ledger
```

**Điểm dễ sai:** đối soát phải xử lý được trường hợp **số dư âm** — seller bị hoàn hàng nhiều hơn doanh số trong kỳ. Khi đó khoản âm chuyển sang kỳ sau, không phải chuyển tiền âm.

Xem [../04-modules/payment.md](../04-modules/payment.md) và [../07-workflows/marketplace-order.md](../07-workflows/marketplace-order.md).

---

## 7. Ranh giới giữa nền tảng và seller

Bảng phân định trách nhiệm — cần rõ ràng để tránh tranh chấp:

| Việc | Nền tảng | Seller |
|---|---|---|
| Thu tiền khách | Có | Không |
| Xác định giá bán | Không (trừ khung giá) | Có |
| Đảm bảo hàng có sẵn | Không | Có |
| Đóng gói | Nếu dùng fulfillment nền tảng | Ngược lại |
| Vận chuyển | Điều phối đối tác | Bàn giao hàng |
| Hỗ trợ khách hàng cấp 1 | Có | Không |
| Xử lý khiếu nại sản phẩm | Trung gian | Chịu trách nhiệm |
| Quyết định chấp nhận hoàn hàng | Theo chính sách nền tảng | Thực hiện |
| Chịu chi phí hoàn hàng | Tùy lý do hoàn | Tùy lý do hoàn |

Dòng cuối cùng cần chính sách rõ:

```text
Lý do hoàn: hàng lỗi, sai mô tả, giao sai   → Seller chịu chi phí
Lý do hoàn: khách đổi ý, không vừa size      → Theo chính sách (thường khách hoặc nền tảng)
```

Phân loại lý do hoàn hàng vì vậy không phải dữ liệu thống kê — nó **quyết định dòng tiền**. Xem [../04-modules/return.md](../04-modules/return.md).

---

## 8. Chỉ số marketplace

| Chỉ số | Ý nghĩa |
|---|---|
| GMV | Tổng giá trị hàng hóa giao dịch |
| Take rate | Tỷ lệ doanh thu nền tảng / GMV |
| Active seller count | Số seller có đơn trong kỳ |
| Assortment size | Số SKU đang bán |
| Offer per product | Mức độ cạnh tranh trên mỗi sản phẩm |
| Seller retention | Tỷ lệ seller tiếp tục bán |
| Time to first sale | Thời gian từ khi seller lên sàn tới đơn đầu tiên |
| Counterfeit report rate | Tỷ lệ báo cáo hàng giả |

Chỉ số cuối phải luôn ở mức rất thấp — đây là chỉ số sức khỏe dài hạn quan trọng nhất.

---

## 9. Tài liệu liên quan

- [seller.md](seller.md) — tác nhân nhà bán
- [monetization.md](monetization.md) — chi tiết hoa hồng và phí
- [../04-modules/marketplace.md](../04-modules/marketplace.md) — đặc tả module
- [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md) — quyết định về Offer
