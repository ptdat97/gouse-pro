# ADR-0019 — Gợi ý size thuộc về đâu

**Trạng thái:** Đã chấp nhận · 16/09/2026

## Bối cảnh

`ProductDetail` trong đặc tả API khai trường `size_recommendation` kèm mô tả
tự nói ra giá trị của nó:

> Gợi ý size dựa trên lịch sử mua của khách. **Cơ chế giảm trực tiếp tỷ lệ
> hoàn hàng.**

Enum lý do là `PREVIOUS_PURCHASE | BODY_MEASUREMENTS | RETURN_HISTORY`.

Trường này chưa bao giờ được điền. `internal/modules/product/interfaces/http/dto.go`
ghi lý do: *"cần lịch sử mua hàng (Phase 2)"*.

**Từ 13/09 thì lý do đó không còn đúng.** Hai trong ba nguồn đã có dữ liệu:

```text
PREVIOUS_PURCHASE   dòng hàng của đơn đã mua  →  SKU  →  thuộc tính size
RETURN_HISTORY      mã lý do CHUẨN HÓA của yêu cầu trả hàng
                    (SIZE_TOO_SMALL · SIZE_TOO_LARGE)
BODY_MEASUREMENTS   CHƯA có — customer không lưu số đo
```

`RETURN_HISTORY` là nguồn mạnh nhất trong ba, và nó vừa được nối vào
`demand_signal` ở P3-42. Một khách trả hàng vì `SIZE_TOO_SMALL` là người nói
thẳng ra rằng size họ chọn sai, và sai theo hướng nào.

## Vấn đề thật: đặt nó ở đâu mà không tạo phụ thuộc vòng

Để gợi ý một size, cần ba mảnh dữ liệu thuộc BA module khác nhau:

```text
lịch sử mua       order      dòng hàng → SKU
lịch sử trả       returns    mã lý do theo dòng hàng
size của SKU      product    thuộc tính `size` của biến thể
size đang bán     product    các biến thể còn hàng của sản phẩm đang xem
```

Và bên gọi là `product` — trang chi tiết sản phẩm.

**Mọi phương án đặt logic vào một module rồi để `product` gọi nó đều tạo
vòng**, vì module đó cần `product` để dịch SKU sang size:

```text
product → recommendation → product        vòng
product → customer        → product        vòng
```

Đây là lý do việc này dừng lại chờ một quyết định thay vì được sửa lặng lẽ.

## Phương án

| | Phương án | Giá phải trả |
|---|---|---|
| **A** | Bên gọi truyền đủ dữ kiện: `product` đã biết thương hiệu, loại hàng và các size đang bán, nên nó truyền chúng vào. Module gợi ý chỉ cần LỊCH SỬ của khách | Cắt được vòng. Nhưng module gợi ý vẫn cần dịch SKU lịch sử → size |
| **B** ⭐ | Như A, **cộng** với việc size được ghi vào event lúc phát, nên module gợi ý không bao giờ phải hỏi `product` | Cắt vòng HOÀN TOÀN. Cần thêm trường vào hai loại event |
| **C** | Tầng HTTP của `product` tự điều phối, gọi cả `order` và `returns` | Logic nghiệp vụ nằm ở tầng interfaces — chỗ không ai kiểm thử được bằng test domain, và chỗ quy tắc R8 sinh ra để tránh |
| **D** | Không làm, gỡ `size_recommendation` khỏi đặc tả | Trung thực, nhưng bỏ đúng cơ chế giảm hoàn hàng mà đặc tả gọi tên |

**Đề xuất B.**

### Vì sao B chứ không phải A

A vẫn để module gợi ý phụ thuộc `product` để dịch SKU lịch sử sang size —
vòng chỉ mỏng đi chứ không mất. B đẩy việc dịch về phía **bên phát event**,
nơi dữ liệu đang có sẵn trong tay:

```text
checkout.completed   đã mang variant_description; thêm `size` là một trường
                     checkout ĐÃ tra được lúc dựng giỏ
returns.requested    thêm `size` cạnh `sku_id` và `reason_code`
```

Sau đó module gợi ý chỉ đọc event và giữ một **read model** của riêng nó.
Nó không gọi module nào lúc phục vụ request, nên nó cũng không làm chậm
trang chi tiết sản phẩm — điều mà mục 5 của
[docs/04-modules/recommendation.md](../04-modules/recommendation.md) đặt
thành yêu cầu bắt buộc (timeout ngắn, luôn có dự phòng).

### Lưu QUAN SÁT, không lưu kết luận

Read model lưu từng quan sát:

```text
(customer_id, brand_id, size, kết quả)     kết quả ∈ GIỮ | CHẬT | RỘNG
```

Không lưu "size gợi ý đã tính sẵn". Lý do giống hệt lý do sổ cái lưu bút
toán chứ không lưu số dư: quy tắc gợi ý SẼ đổi, và đổi quy tắc không được
làm mất dữ liệu đã quan sát. Tính lại từ quan sát thì được; dựng lại quan
sát từ một kết luận cũ thì không.

### Module nào giữ read model đó

**`recommendation`** — module đặc tả đã mô tả và chưa tồn tại.

KHÔNG đặt vào `customer`: hồ sơ khách là dữ liệu khách tự khai và tự sửa,
còn đây là dữ liệu SUY RA từ hành vi. Trộn hai loại vào một module sẽ làm
câu hỏi "khách có quyền xóa cái này không" không có câu trả lời rõ ràng.

Đây cũng chính là việc tầm nhìn yêu cầu: *"Định nghĩa interface
`Recommendation` ... từ sớm, dù cài đặt ban đầu chỉ là rule đơn giản."*

## Điều kiện để bắt đầu — và vì sao nêu ra

Module `recommendation` **chỉ nên dựng khi có bên gọi thật**. Rà soát ngày
13/09 cho thấy đặc tả API không khai `similar_products`, không có endpoint
xu hướng, không có `/products/{id}/similar`. Dựng cả module cho những thứ
đó bây giờ là tạo thêm một ca của dạng lỗi ở mục 8 backlog — lần này ở mức
MODULE.

`size_recommendation` thì KHÁC: nó là trường đã khai, ở một endpoint đang
chạy, có người dùng thật đọc. Nên phạm vi đợt đầu của module là **đúng một
phương thức**, và bốn phương thức còn lại của interface được khai nhưng trả
kết quả rỗng theo đúng cơ chế dự phòng bắt buộc — cho tới khi có bên gọi.

## Đánh đổi đã chấp nhận

**Gợi ý chỉ có với khách ĐÃ ĐĂNG NHẬP và đã từng mua.** Khách mới không có
gì để suy ra, và trường trả `null` — đúng như đặc tả khai kiểu nullable.

**Không suy ngược cho dữ liệu cũ.** Event mang `size` chỉ có từ ngày triển
khai; đơn và yêu cầu trả hàng trước đó không sinh quan sát. Có thể dựng một
lệnh đối soát đọc `order_line` và `return_line` để bù — nhưng đó là việc
riêng, và nó đọc bảng của module khác nên cần đặt ở tầng composition.

**Một size chỉ so được trong CÙNG thương hiệu.** Size M của hai thương hiệu
không bằng nhau, và đó là sự thật của ngành thời trang chứ không phải hạn
chế kỹ thuật. Gợi ý xuyên thương hiệu cần bảng quy đổi số đo — thuộc
`BODY_MEASUREMENTS`, chưa có.

## Liên quan

- [docs/04-modules/recommendation.md](../04-modules/recommendation.md)
- [ADR-0005](0005-module-boundaries.md) — ranh giới module
- `docs/00-overview/vision.md` — yêu cầu định nghĩa interface từ sớm
