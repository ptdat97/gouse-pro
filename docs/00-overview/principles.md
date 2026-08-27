# Nguyên tắc kiến trúc

Đây là các nguyên tắc bắt buộc. Khi có tranh luận thiết kế, tài liệu này là trọng tài. Mọi ngoại lệ phải được ghi lại bằng một ADR.

---

## P1. Nghiệp vụ trước, kỹ thuật sau

**Nguyên tắc:** Không quyết định chi tiết kỹ thuật trước khi mô hình domain được hiểu rõ.

**Lý do:** Chọn sai công nghệ có thể sửa trong vài tuần. Chọn sai ranh giới domain phải viết lại trong nhiều tháng.

**Áp dụng:**
- Không thiết kế schema database trước khi xác định aggregate.
- Không thiết kế endpoint API trước khi xác định use case.
- Không chọn message broker trước khi biết event nào thật sự cần vượt tiến trình.

---

## P2. Modular Monolith trước, dịch vụ sau

**Nguyên tắc:** Bắt đầu bằng một tiến trình Go duy nhất, chia module rõ ràng. Không bắt đầu bằng microservices.

**Lý do:** Ở giai đoạn đầu, ranh giới domain còn thay đổi. Di chuyển ranh giới trong monolith là refactor; di chuyển ranh giới giữa các service là dự án di trú dữ liệu. Chi phí chênh nhau hàng chục lần.

**Áp dụng:** Xem [ADR-0001](../adr/0001-modular-monolith.md). Ranh giới module phải nghiêm ngặt **như thể** đã là service — để việc tách sau này chỉ là chuyện vận chuyển, không phải thiết kế lại.

---

## P3. API First

**Nguyên tắc:** Mọi năng lực nghiệp vụ được lộ ra qua API có hợp đồng rõ ràng, trước khi có giao diện dùng nó.

**Lý do:** Nền tảng phục vụ nhiều mặt tiền — storefront, seller center, creator center, admin, app di động, đối tác. Nếu API sinh ra sau giao diện, nó sẽ mang hình dạng của một màn hình cụ thể và không dùng lại được.

**Áp dụng:** Xem [ADR-0002](../adr/0002-api-first.md) và [06-api/api-guidelines.md](../06-api/api-guidelines.md).

---

## P4. Frontend không chứa logic nghiệp vụ

**Nguyên tắc:** Next.js chỉ làm trình bày, điều hướng, tổng hợp dữ liệu để hiển thị. Không tính giá, không quyết định trạng thái đơn hàng, không truy cập database.

**Lý do:** Logic nghiệp vụ trùng lặp ở hai nơi sẽ phân kỳ. Khi có app di động hoặc API đối tác, logic nằm ở frontend là logic không tồn tại.

**Ngoại lệ được phép:** kiểm tra hợp lệ ở phía client vì trải nghiệm người dùng — nhưng backend **luôn** kiểm tra lại. Hiển thị giá đã tính sẵn từ backend — không tự tính.

---

## P5. Module sở hữu dữ liệu của mình

**Nguyên tắc:** Mỗi bảng dữ liệu thuộc về đúng một module. Module khác không đọc/ghi trực tiếp bảng đó — phải đi qua interface công khai hoặc domain event.

**Lý do:** Đây là điều kiện tiên quyết để tách service sau này. Nếu năm module cùng `JOIN` vào bảng `orders`, không thể tách module đơn hàng ra được.

**Áp dụng:** Xem [03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md) và ma trận sở hữu dữ liệu tại [05-data/data-model.md](../05-data/data-model.md).

---

## P6. Không có phụ thuộc vòng

**Nguyên tắc:** Đồ thị phụ thuộc giữa các module phải là đồ thị có hướng không chu trình (DAG).

**Lý do:** Phụ thuộc vòng làm cho module không thể kiểm thử độc lập, không thể tách, và làm thay đổi lan truyền không kiểm soát.

**Áp dụng:** Khi A cần B và B cần A, giải bằng một trong ba cách:
1. Đảo chiều một hướng bằng **domain event** (B phát event, A lắng nghe).
2. Trích xuất phần chung thành module thứ ba ở tầng thấp hơn.
3. Xem lại ranh giới — có thể A và B thực ra là một module.

Kiểm tra tự động bằng lint phụ thuộc trong CI.

---

## P7. Sự kiện miền diễn tả việc đã xảy ra

**Nguyên tắc:** Domain event đặt tên ở **thì quá khứ** và mô tả **sự thật nghiệp vụ**, không phải mệnh lệnh kỹ thuật.

**Đúng:** `OrderPlaced`, `PaymentCaptured`, `QualityApproved`
**Sai:** `SendEmail`, `UpdateInventory`, `ProcessOrder`

**Lý do:** Event là mệnh lệnh trá hình sẽ tạo ra ghép nối chặt — bên phát phải biết bên nhận làm gì. Event mô tả sự thật cho phép thêm bên nhận mới mà không sửa bên phát.

---

## P8. Bản ghi tài chính là bất biến

**Nguyên tắc:** Sổ cái chỉ ghi thêm (append-only). Sửa sai bằng bút toán điều chỉnh, không bằng `UPDATE`.

**Lý do:** Nền tảng giữ tiền hộ nhiều bên. Khả năng tái dựng lại số dư tại bất kỳ thời điểm nào trong quá khứ là yêu cầu kiểm toán, không phải tính năng.

**Áp dụng:** Xem [ADR-0008](../adr/0008-financial-ledger.md). Cấm tuyệt đối việc tính số dư bằng các phép cộng trừ rải rác trong nhiều module.

---

## P9. Giá và điều kiện được đóng băng tại thời điểm giao dịch

**Nguyên tắc:** Khi tạo đơn hàng, sao chép giá, tỷ lệ hoa hồng, thông tin sản phẩm vào đơn. Không tham chiếu động.

**Lý do:** Giá thay đổi hàng ngày. Nếu đơn hàng tham chiếu tới bảng giá hiện tại, hóa đơn tháng trước sẽ hiển thị sai khi giá đổi.

---

## P10. Idempotency là mặc định cho mọi thao tác thay đổi trạng thái

**Nguyên tắc:** Mọi API ghi và mọi bên xử lý event phải chịu được việc bị gọi lại nhiều lần với cùng đầu vào.

**Lý do:** Mạng không tin cậy. Client sẽ thử lại. Webhook từ cổng thanh toán sẽ gửi trùng. Nếu không idempotent, khách bị trừ tiền hai lần.

**Áp dụng:** Xem [05-data/idempotency.md](../05-data/idempotency.md).

---

## P11. Ranh giới nhất quán trùng với ranh giới aggregate

**Nguyên tắc:** Một giao dịch database chỉ được sửa **một** aggregate. Nhất quán giữa các aggregate là nhất quán cuối (eventual), đạt được qua event.

**Lý do:** Giao dịch trải rộng nhiều aggregate sẽ tạo tranh chấp khóa và ngăn việc tách service.

**Áp dụng:** Xem [05-data/consistency.md](../05-data/consistency.md).

---

## P12. Không tạo tầng dùng chung toàn cục

**Nguyên tắc:** Cấm các gói `common/`, `utils/`, `helpers/`, `services/` làm nơi chứa mọi thứ.

**Lý do:** Những gói này trở thành điểm phụ thuộc của toàn hệ thống và là nơi logic nghiệp vụ đi lạc vào. Chúng phá hủy tính module một cách âm thầm.

**Áp dụng:** Code dùng chung phải được phân loại rõ ràng thành một trong ba nhóm — hạ tầng trung lập với domain, năng lực kỹ thuật tổng quát, hoặc shared kernel. Xem [03-architecture/modular-monolith.md](../03-architecture/modular-monolith.md).

---

## P13. Năng lực có thể thay thế được thì phải nằm sau interface

**Nguyên tắc:** Tìm kiếm, gợi ý, thanh toán, vận chuyển, gửi thông báo, lưu trữ file — tất cả đều nằm sau interface do domain định nghĩa.

**Lý do:** Những năng lực này sẽ thay đổi nhà cung cấp. Nếu module đơn hàng gọi thẳng SDK của một cổng thanh toán cụ thể, đổi cổng thanh toán là sửa module đơn hàng.

**Áp dụng cụ thể với gợi ý sản phẩm:** module thương mại **không bao giờ** phụ thuộc trực tiếp vào máy học. Xem [04-modules/recommendation.md](../04-modules/recommendation.md).

---

## P14. Bắt đầu bằng quy tắc, để dành chỗ cho mô hình

**Nguyên tắc:** Với dự báo nhu cầu, gợi ý sản phẩm, chấm điểm seller — cài đặt đầu tiên là quy tắc tường minh, đơn giản, giải thích được. Interface được thiết kế sao cho sau này thay bằng mô hình học máy mà không đổi bên gọi.

**Lý do:** Chưa có dữ liệu thì không huấn luyện được mô hình. Kiến trúc phụ thuộc vào ML từ ngày đầu sẽ không chạy được ngày đầu.

---

## P15. Mỗi quyết định phải giải thích được vì sao cần cho **chính** nghiệp vụ này

**Nguyên tắc:** Không dùng một mẫu kiến trúc vì nó phổ biến. Mỗi quyết định phải trả lời được: "Đặc điểm nào của nền tảng thời trang này khiến ta cần nó?"

**Ví dụ áp dụng:**

| Quyết định | Lý do gắn với nghiệp vụ |
|---|---|
| Tách `Offer` khỏi `Product` | Nhiều seller bán cùng một sản phẩm với giá khác nhau |
| Tách `Fulfillment Order` | Một đơn có hàng từ nhiều nguồn, giao từ nhiều kho |
| `Collection` là hạng nhất | Thời trang bán theo mùa, vòng đời sản phẩm ngắn |
| Truy vết lô sản xuất | Cùng SKU khác lô có giá vốn và chất lượng khác nhau |
| Attribution có cửa sổ thời gian | Khách xem nội dung creator hôm nay, mua sau ba ngày |
| Ledger bất biến | Giữ tiền hộ seller và creator, phải đối soát được |

**Chống chỉ định:** không đưa Kubernetes, service mesh, CQRS toàn hệ thống, hay event sourcing vào tài liệu chỉ vì chúng "hiện đại". Nếu không giải thích được bằng bảng như trên, không đưa vào.

---

## P16. Thà rõ ràng còn hơn ngắn gọn

**Nguyên tắc:** Ưu tiên code và tài liệu tường minh, dễ đọc, hơn là trừu tượng hóa thông minh.

**Lý do:** Nền tảng này sẽ có nhiều người và nhiều đội cùng làm trong nhiều năm. Trừu tượng hóa sớm dựa trên hai ba trường hợp sử dụng thường sai và rất khó gỡ.

**Quy tắc thực dụng:** chấp nhận lặp lại đến lần thứ ba mới trừu tượng hóa.

---

## P17. Code là trọng tài cuối cùng, không phải tài liệu

**Nguyên tắc:** Tài liệu mô tả kiến trúc **dự định** và các ranh giới. Code, test và hành vi thật trên production là **thẩm quyền cuối cùng**. Khi triển khai cho thấy một giả định trong tài liệu là sai, hãy **cập nhật ADR và tài liệu**, đừng ép code chạy theo tài liệu.

**Lý do:** Tài liệu được viết trước khi biết sự thật. Code là nơi sự thật xuất hiện. Một tài liệu nói "dùng công cụ X" trong khi mười module đã chạy tốt với công cụ Y không làm cho Y sai — nó làm cho tài liệu cũ.

Ép code theo tài liệu trong tình huống đó là viết lại thứ đang chạy được để phục vụ một câu văn. Đó là chi phí thật đổi lấy sự nhất quán hình thức.

**Quy trình khi phát hiện lệch:**

```text
Code khác tài liệu
    ↓
Code sai?          → sửa code, giữ tài liệu
Tài liệu sai?      → sửa tài liệu; nếu là quyết định kiến trúc thì
                     ghi ADR mới hoặc cập nhật ADR cũ kèm LÝ DO THẬT
Cả hai đều đúng?   → tài liệu thiếu ngữ cảnh; bổ sung điều kiện áp dụng
```

**Điều kiện bắt buộc:** không im lặng sửa tài liệu cho khớp code. Phải ghi lại **vì sao** giả định ban đầu sai — đó là thứ có giá trị cho người đọc sau, còn bản thân sự nhất quán thì không.

**Ví dụ đã xảy ra trong dự án này:** [ADR-0010](../adr/0010-database-layer.md) chọn `sqlc`; mười module triển khai bằng `pgx` viết tay. ADR đã được cập nhật với lý do thật thay vì viết lại mười module.

---

## Bảng tra cứu nhanh: nguyên tắc → tài liệu chi tiết

| Nguyên tắc | Chi tiết tại |
|---|---|
| P2 Modular monolith | [ADR-0001](../adr/0001-modular-monolith.md), [03-architecture/modular-monolith.md](../03-architecture/modular-monolith.md) |
| P3 API First | [ADR-0002](../adr/0002-api-first.md), [06-api/api-guidelines.md](../06-api/api-guidelines.md) |
| P5, P6 Ranh giới module | [03-architecture/dependency-rules.md](../03-architecture/dependency-rules.md) |
| P7 Domain event | [02-domain/domain-events.md](../02-domain/domain-events.md), [ADR-0006](../adr/0006-internal-events.md) |
| P8 Ledger | [ADR-0008](../adr/0008-financial-ledger.md) |
| P10 Idempotency | [05-data/idempotency.md](../05-data/idempotency.md) |
| P11 Nhất quán | [05-data/consistency.md](../05-data/consistency.md) |
| P13, P14 Interface thay thế được | [04-modules/recommendation.md](../04-modules/recommendation.md) |
| P17 Code là trọng tài | [ADR-0010](../adr/0010-database-layer.md) — ví dụ đã xảy ra thật |
