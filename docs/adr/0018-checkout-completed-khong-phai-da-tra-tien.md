# ADR-0018: `checkout.completed` KHÔNG phải "đã trả tiền"

**Trạng thái:** Accepted (07/09) — chủ dự án chọn **A2 + B1 + dựng quy trình đảo bút toán**

**Đã triển khai đầy đủ** (07/09): A2, B1, quy trình đảo bút toán, và dọn
3110 bút toán ghi khống trên dữ liệu thật.

---

## Bối cảnh

Hệ thống có một event được dùng như thể nó trả lời hai câu hỏi khác nhau:

```text
checkout.completed  =  "khách đã bấm xong phiên thanh toán"
                    ≠  "tiền đã về"
```

Hai bên nhận đang treo trên event này, và cả hai chỉ đúng nếu nó mang
nghĩa thứ hai:

```text
payment.RevenueOnCheckoutCompleted   → ghi bút toán doanh thu + PLATFORM_CASH
fulfillment.SplitOnCheckoutCompleted → tạo đơn thực hiện cho nhà bán
```

### Đo trên dữ liệu thật (07/09/2026)

```text
bút toán ORDER_REVENUE theo trạng thái đơn:

  PENDING_PAYMENT     3110 bút toán   PLATFORM_CASH ghi NỢ  1.132.273.000 đ
  PARTIALLY_SHIPPED      4                                      1.456.000 đ
  SHIPPED                2                                        938.000 đ
  PAID                   1                                        390.000 đ
```

Sổ cái đang khẳng định nền tảng cầm **1,13 tỷ đồng tiền mặt** chưa hề tới.
Đúng MỘT đơn thật sự đã trả tiền.

Sổ cái là bất biến (ADR-0008), nên những dòng này không xóa được — sửa
đúng phải ghi bút toán ĐẢO.

```text
đơn hàng và đơn thực hiện:

  order              PENDING_PAYMENT  3166
  fulfillment_order  PENDING          3171
```

Gần một-đối-một: mỗi đơn chưa trả tiền đã có sẵn một đơn thực hiện nằm chờ
nhà bán xử lý.

### Đặc tả nói KHÁC, và nói từ trước

`docs/07-workflows/marketplace-order.md` mục 3 vẽ rõ mốc tách đơn:

```text
Bus->>Ful: order.paid
Note over Ful: Nhóm OrderLine theo (seller_id, nguồn hàng)
Ful->>Ful: Tạo FO-A / FO-B / FO-C
```

Event đó **tồn tại trong sổ đăng ký và chưa từng được dùng**:

```text
eventbus.TypeOrderPaid = "order.paid"   0 nơi phát · 0 nơi nghe
order.MarkPaid                          ghi trạng thái, KHÔNG phát event
```

Đây là lần thứ bảy của dạng lỗi ở mục 8 backlog — một thứ được khai trong
hợp đồng mà không đường mã nào đi qua.

### Vì sao mã nguồn chọn `checkout.completed`

Chú thích của `SplitOnCheckoutCompleted` nêu lý do thật và ĐÚNG:

> Payload của event này chứa đủ dữ liệu để tách: SKU, seller, số lượng và
> tiền của từng dòng. Nghe `order.placed` sẽ phải gọi ngược module order
> để lấy chi tiết — đúng thứ kiến trúc event sinh ra để tránh.

Lập luận này là về **dữ liệu tiện dùng**. Nhưng câu hỏi đặc tả trả lời là
một câu khác: **khi nào thì AN TOÀN để bảo nhà bán đi gói hàng.** Chọn
đúng câu thứ nhất rồi vô tình trả lời luôn câu thứ hai — đó là cả gốc rễ
của ADR này.

---

## Hệ quả đang có

**1. Nhà bán có thể giao hàng cho đơn chưa trả tiền.**

Không có chỗ nào trong `fulfillment` kiểm tra trạng thái thanh toán:
`Confirm`, `Pick`, `Pack`, `HandOver` đều đi thẳng. Một đơn CARD mà khách
bỏ giữa chừng vẫn đi hết tới lúc hàng rời kho.

Với **COD** thì đây là hành vi ĐÚNG và bắt buộc: tiền về lúc giao. Sự khác
biệt COD ↔ trả trước chỉ mới diễn đạt được từ 06/09, khi P3-9 nối
`payment_method` vào đơn — trước đó quy tắc này không viết ra được.

**2. Doanh thu và TIỀN MẶT ghi sổ trước khi tiền về.**

ADR-0017 đã ghi nhận nửa doanh thu ("Xấu, và cần ghi rõ") nhưng chưa ai đo
quy mô. Nửa chưa ai nêu là `PLATFORM_CASH`: doanh thu ghi sớm là tranh
luận kế toán, còn nói mình đang cầm 1,13 tỷ tiền mặt không có là một câu
SAI SỰ THẬT về tài sản.

---

## Quyết định cần lấy

ADR-0017 đã nói việc này "là quyết định của chủ dự án". ADR này không tự
quyết; nó dọn sẵn lựa chọn kèm giá của từng phương án.

### Phần A — khi nào nhà bán được phép giao hàng

| | Phương án | Giá phải trả |
|---|---|---|
| **A1** | Tách đơn ở `order.paid` cho đơn trả trước, giữ `checkout.completed` cho COD | Đúng đặc tả. Nhưng hai mốc kích hoạt, và `order.paid` phải mang đủ payload — mất chính lợi ích mà mã nguồn đang có |
| **A2** ⭐ | Giữ nguyên mốc tách. Đơn thực hiện của đơn TRẢ TRƯỚC sinh ra ở trạng thái **chưa được phép hành động**; `order.paid` mở khóa | Giữ nguyên lập luận payload. Nhà bán vẫn thấy việc sắp tới. Cần một cờ mới trên FO và một bên nhận mới |
| **A3** | Không đổi gì | Hàng tiếp tục đi ra cho đơn chưa trả tiền |

**Đề xuất A2.** Nó tách hai câu hỏi vốn bị gộp: *"đã có đủ dữ liệu để lập
đơn việc chưa"* (đúng ở `checkout.completed`) và *"đã an toàn để hàng rời
kho chưa"* (chỉ đúng khi tiền về, hoặc khi là COD). Mỗi câu giữ mốc đúng
của nó thay vì ép cả hai dùng chung một mốc.

### Phần B — sổ cái nói gì khi chưa thu được tiền

| | Phương án | Giá phải trả |
|---|---|---|
| **B1** ⭐ | Ghi **khoản PHẢI THU** thay vì tiền mặt: `DEBIT ACCOUNTS_RECEIVABLE`. Khi thu được thì `DEBIT PLATFORM_CASH / CREDIT ACCOUNTS_RECEIVABLE` | Cần một loại tài khoản mới. KHÔNG động tới thời điểm ghi doanh thu, nên không phải bàn lại cả mô hình |
| **B2** | Dời hẳn ghi doanh thu sang lúc thu tiền | Đúng nhất về kế toán, nhưng động tới COD (tiền về lúc GIAO) và làm mọi báo cáo doanh thu đổi nghĩa |
| **B3** | Không đổi gì | Sổ cái tiếp tục khẳng định một khoản tiền mặt không tồn tại |

**Đề xuất B1.** Nó bỏ đúng câu nói sai — "đang cầm số tiền này" — mà không
mở lại tranh luận lớn hơn về thời điểm ghi doanh thu. B2 vẫn nên bàn, chỉ
là không cùng lúc.

### Cả hai phương án đề xuất đều cần `order.paid` được PHÁT

Đó cũng là việc nhỏ nhất và không gây tranh cãi trong ADR này: `MarkPaid`
hôm nay đổi trạng thái rồi im lặng. Nhưng phát một event chưa ai nghe là
dựng mã chết, nên nó chỉ nên làm CÙNG bên nhận đầu tiên, không làm trước.

---

## Việc phải làm với dữ liệu đang có

Dù chọn phương án nào, 3110 bút toán kia đã nằm trong sổ bất biến. Chúng
cần một quyết định riêng:

```text
ghi bút toán ĐẢO cho các đơn không bao giờ trả tiền   ← đúng nguyên tắc
để nguyên và ghi chú                                   ← nhanh, nhưng mọi
                                                          báo cáo tài sản
                                                          vẫn sai
```

Phần lớn 3110 đơn đó là dữ liệu phát triển, nên với môi trường này việc
dọn có thể đơn giản. Nhưng quy trình đảo bút toán vẫn phải tồn tại TRƯỚC
khi hệ thống chạy thật — vì lúc đó thì không dọn được nữa.

---

## Bổ sung 11/09/2026 — ai ghi nhận thu tiền COD

ADR này khẳng định "với COD thì tiền về lúc giao" nhưng **không chỉ định ai
ghi nhận**. Không đường nào ở production làm việc đó: `MarkOrderPaid` chỉ
được gọi từ webhook cổng thanh toán, thứ COD không có.

Đo trên sổ cái thật: **0 bút toán THU TIỀN trong toàn hệ thống**. Khoản
phải thu của mọi đơn COD sẽ tồn vĩnh viễn, và nhà bán không được quyết
toán vì quyết toán đọc từ sổ cái.

### Trạng thái và việc trả tiền là HAI TRỤC, không phải một

Cột `status` mang tiến độ GIAO HÀNG. Với đơn trả trước, tiền về ở đầu chuỗi
nên một cột diễn đạt được cả hai. Với COD thì tiền về lúc GIAO — khi đơn đã
mang `DELIVERED`. Ghi nhận bằng cách đặt `status = PAID` sẽ đẩy đơn **lùi**
và xóa mất sự thật "đã giao xong".

Đã thêm cột `paid_at` (migration 000050). `status` giữ nguyên nghĩa là trục
HÀNG; `paid_at` là trục TIỀN. `MarkPaid` của đường trả trước cũng đặt nó,
nên một câu hỏi "đã thu tiền chưa" có MỘT câu trả lời cho cả hai đường.

Mốc thời gian chứ không phải cờ: đối soát cần biết tiền về LÚC NÀO. Chênh
lệch giữa ngày giao và ngày hãng chuyển tiền COD về là một khoản phải thu
có tuổi, và tuổi của nó là thứ người vận hành đi đòi.

### Ghi nhận khi giao TRỌN đơn, không phải kiện đầu tiên

Đơn tách cho hai nhà bán đi thành hai kiện, và khách trả tiền cho từng
người giao. Ghi nhận ở kiện đầu là khẳng định đã thu đủ tiền cả đơn trong
khi kiện thứ hai còn trên đường — đúng thứ ADR này vừa dọn 1,13 tỷ đồng.

Chờ giao trọn là ghi **trễ** chứ không ghi **sai**, và đó là hướng hỏng
chấp nhận được: khoản phải thu tồn lâu hơn thực tế thì có người đi đòi,
còn tiền mặt ghi khống thì không ai đi tìm.

### Chỗ ghi nhận

`order.ApplyFulfillmentProgress` — chỗ DUY NHẤT biết đơn vừa giao xong.
Module này đã nghe `fulfillment.progress_changed` và tự tính trạng thái
tổng hợp; hỏi ngược fulfillment sẽ tạo phụ thuộc vòng (ADR-0007).

---

## Liên quan

- [ADR-0008](0008-financial-ledger.md) — sổ cái bất biến, danh mục tài khoản
- [ADR-0017](0017-payment-intent.md) — đã nêu nửa doanh thu; ADR này đo nó
  và tìm thêm nửa tiền mặt lẫn nửa giao hàng
- `docs/07-workflows/marketplace-order.md` mục 3 — nơi `order.paid` được
  khai làm mốc tách đơn
- `docs/10-roadmap/backlog.md` mục 8 — dạng lỗi "khai mà không ai dùng"
