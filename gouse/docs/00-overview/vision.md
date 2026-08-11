# Tầm nhìn

## Chúng ta đang xây dựng cái gì

> Một **nền tảng thương mại thời trang** sở hữu thương hiệu riêng, dùng marketplace (nhà bán và thương hiệu bên thứ ba) để mở rộng độ phủ hàng hóa, dùng creator/nội dung làm động cơ tạo nhu cầu, và dùng năng lực điều phối chuỗi cung ứng làm lợi thế cạnh tranh dài hạn.

Điểm cần nhấn mạnh: **đây không phải một hệ thống ecommerce tổng quát.** Nếu kiến trúc chỉ giải quyết "bán hàng online", nó sẽ không đỡ được ba thứ khác biệt của mô hình này: multi-seller offer, quy kết doanh thu cho creator, và vòng lặp từ dữ liệu nhu cầu ngược về sản xuất.

## Bốn trụ cột

```text
1. Own Brand          — thương hiệu tự sản xuất, biên lợi nhuận cao, kiểm soát chất lượng
2. Marketplace        — mở rộng assortment nhanh, không tồn vốn hàng
3. Creator Commerce   — tạo nhu cầu, giảm chi phí thu hút khách
4. Supply Chain       — biến dữ liệu nhu cầu thành hàng hóa đúng lúc, đúng số lượng
```

Ba trụ cột đầu tạo doanh thu. Trụ cột thứ tư tạo **lợi thế không sao chép được**: đối thủ có thể copy giao diện, copy chính sách hoa hồng, nhưng không copy được năng lực chuyển tín hiệu nhu cầu thành đơn sản xuất trong vài tuần.

## Lộ trình tiến hóa mô hình kinh doanh

```text
Own Brand
    ↓
Own Brand + Marketplace
    ↓
Marketplace + Creator Commerce
    ↓
Marketplace + Supply Network
    ↓
Demand-driven Fashion Supply Chain
```

Kiến trúc phải chịu được cả 5 giai đoạn mà **không cần thiết kế lại domain model**. Đây là ràng buộc thiết kế quan trọng nhất của toàn bộ tài liệu này.

Cụ thể, nghĩa là ngay từ giai đoạn 1 ta đã phải:

- Mô hình hóa `Offer` tách khỏi `Product`, kể cả khi mới chỉ có own brand bán (xem [ADR-0007](../adr/0007-marketplace-order-model.md)).
- Tách `Order` (góc nhìn khách) khỏi `Fulfillment Order` (góc nhìn vận hành), kể cả khi chưa có multi-seller.
- Ghi sổ tài chính bằng ledger bất biến, kể cả khi chưa phải chia hoa hồng cho ai (xem [ADR-0008](../adr/0008-financial-ledger.md)).
- Định nghĩa interface `Recommendation` và `DemandSignal` từ sớm, dù cài đặt ban đầu chỉ là rule đơn giản.

Chi phí của việc này ở giai đoạn 1 là nhỏ. Chi phí của việc không làm nó, ở giai đoạn 3, là viết lại module đơn hàng và tài chính.

## Bánh đà kinh doanh (business flywheel)

Đây là vòng lặp mà kiến trúc bắt buộc phải bảo toàn:

```text
Customer
 ↓
Discovery
 ↓
Content / Creator
 ↓
Purchase
 ↓
Behavior Data
 ↓
Demand Signal
 ↓
Product Planning
 ↓
Own Brand / Supplier
 ↓
Production
 ↓
Inventory
 ↓
Sales
 ↓
More Data
 ↺
```

Mỗi vòng quay làm nền tảng hiểu nhu cầu tốt hơn, dẫn tới hàng hóa đúng hơn, dẫn tới bán tốt hơn, dẫn tới nhiều dữ liệu hơn.

**Hệ quả kiến trúc:** không được để đứt đoạn ở khâu `Behavior Data → Demand Signal`. Đây là chỗ đa số nền tảng ecommerce đứt — dữ liệu hành vi nằm trong công cụ analytics của bên thứ ba, không quay ngược được vào quy trình lập kế hoạch sản phẩm. Chúng ta phải giữ dữ liệu hành vi ở dạng có thể truy vấn được bởi domain `Supply Chain`.

## Ba hiệu ứng mạng độc lập

```text
Customer ↔ Content / Creator      — càng nhiều khách, càng hút creator; càng nhiều creator, càng hút khách
Customer ↔ Marketplace / Seller   — càng nhiều khách, càng hút seller; càng nhiều seller, assortment càng rộng
Demand   ↔ Supplier / Manufacturing — càng nhiều đơn, càng có lực đàm phán và dữ liệu để đặt sản xuất
```

Ba hiệu ứng này **độc lập** — nghĩa là hệ thống phải vận hành được khi một trong ba còn yếu. Không được thiết kế sao cho marketplace chỉ chạy được khi đã có creator, hay own brand chỉ chạy được khi đã có marketplace.

Đây là lý do các bounded context được tách theo trục này (xem [02-domain/bounded-contexts.md](../02-domain/bounded-contexts.md)).

## Mục tiêu dài hạn

> **Xây dựng một Fashion Commerce Platform, không chỉ là một website thương mại điện tử.**

Tiêu chí phân biệt cụ thể:

| Ecommerce website | Fashion Commerce Platform |
|---|---|
| Bán hàng của mình | Điều phối nhiều nguồn cung |
| Marketing đẩy traffic | Nội dung/creator tạo nhu cầu |
| Nhập hàng theo kinh nghiệm | Sản xuất theo tín hiệu nhu cầu |
| Một luồng đơn hàng | Đơn hàng đa nhà bán, đa điểm xuất |
| Sổ sách kế toán cuối kỳ | Ledger giao dịch, đối soát liên tục |
| Sản phẩm là bản ghi tĩnh | Sản phẩm có vòng đời từ concept → sản xuất → bán |

## Cái gì KHÔNG nằm trong tầm nhìn

Ghi rõ để tránh mở rộng phạm vi ngoài kiểm soát:

- Không xây dựng hệ thống ERP tổng quát cho bên thứ ba dùng.
- Không xây dựng nền tảng logistics/vận chuyển riêng — tích hợp với đơn vị vận chuyển.
- Không xây dựng cổng thanh toán riêng — tích hợp PSP.
- Không xây dựng công cụ thiết kế thời trang (CAD) — chỉ quản lý artifact thiết kế.
- Không làm marketplace đa ngành hàng. Thời trang và phụ kiện thời trang là phạm vi.

## Tài liệu liên quan

- [business-model.md](business-model.md) — cách nền tảng kiếm tiền
- [principles.md](principles.md) — nguyên tắc kiến trúc rút ra từ tầm nhìn này
- [../03-architecture/architecture.md](../03-architecture/architecture.md) — hiện thực hóa kỹ thuật
