# Từ điển thuật ngữ nghiệp vụ (Business Glossary)

Đây là **ngôn ngữ chung** (ubiquitous language) của toàn hệ thống. Tên trong code, tên bảng dữ liệu, tên API và tên trong tài liệu phải khớp với từ điển này.

## Quy tắc sử dụng

1. Một khái niệm — một tên. Không dùng từ đồng nghĩa.
2. Nếu hai bounded context hiểu cùng một từ theo hai nghĩa khác nhau, **phải ghi rõ** trong mục "Từ đa nghĩa theo ngữ cảnh" ở cuối tài liệu này.
3. Thuật ngữ kỹ thuật giữ nguyên tiếng Anh trong code; tài liệu tiếng Việt giải thích nghĩa.
4. Thêm thuật ngữ mới phải cập nhật file này trong cùng pull request.

---

## A. Sản phẩm và danh mục

| Thuật ngữ | Nghĩa | Ghi chú phân biệt |
|---|---|---|
| **Product** | Khái niệm sản phẩm ở mức trình bày cho khách: "Áo sơ mi linen Oxford" | Không có giá, không có tồn kho |
| **Variant** | Một tổ hợp thuộc tính cụ thể: màu Trắng, size M | Vẫn chưa phải đơn vị bán |
| **SKU** | Đơn vị lưu kho nhỏ nhất, mã định danh vật lý duy nhất | Đơn vị đếm tồn kho |
| **Offer** | Lời chào bán một SKU bởi **một nhà bán cụ thể**, với giá và tồn kho cụ thể | Đơn vị mà khách thực sự mua |
| **Brand** | Thương hiệu. Có thể là own brand hoặc brand của seller | |
| **Collection** | Bộ sưu tập, thường gắn với mùa: "Thu Đông 2026" | Khái niệm hạng nhất trong thời trang |
| **Category** | Phân loại phân cấp: Nữ > Áo > Áo sơ mi | Dùng để duyệt tìm |
| **Attribute** | Thuộc tính sản phẩm: chất liệu, kiểu dáng, họa tiết | Dùng để lọc |
| **Size Chart** | Bảng quy đổi kích cỡ | Khác nhau theo brand và theo vùng |

**Quan hệ:**

```text
Brand → Collection → Product → Variant → SKU → Offer → Inventory
```

Đây là chuỗi quan trọng nhất trong hệ thống. Xem [../02-domain/entities.md](../02-domain/entities.md).

---

## B. Nhà bán và marketplace

| Thuật ngữ | Nghĩa |
|---|---|
| **Seller** | Nhà bán trên marketplace (cá nhân, doanh nghiệp hoặc thương hiệu) |
| **Seller Store** | Gian hàng — mặt tiền của seller trên nền tảng |
| **Own Brand** | Thương hiệu do nền tảng sở hữu. Trong hệ thống được mô hình hóa như một seller nội bộ đặc biệt |
| **Commission** | Hoa hồng nền tảng thu trên đơn hàng của seller |
| **Commission Rate** | Tỷ lệ hoa hồng, có thể khác nhau theo ngành hàng/seller/chiến dịch |
| **Settlement** | Đối soát — chốt số tiền phải trả cho seller trong một kỳ |
| **Payout** | Chi trả — chuyển tiền thật về tài khoản seller |
| **Seller Balance** | Số dư khả dụng của seller trên nền tảng |
| **Seller Performance** | Bộ chỉ số đánh giá seller: tỷ lệ hủy, thời gian xử lý, tỷ lệ hoàn |

---

## C. Creator và nội dung

| Thuật ngữ | Nghĩa |
|---|---|
| **Creator** | Người sáng tạo nội dung có khả năng dẫn dắt mua hàng |
| **KOC** | Key Opinion Consumer — người tiêu dùng có ảnh hưởng, quy mô nhỏ, độ tin cậy cao |
| **KOL** | Key Opinion Leader — người có ảnh hưởng quy mô lớn |
| **Content** | Nội dung: video, ảnh, lookbook, bài viết, outfit, livestream |
| **Product Tag** | Gắn sản phẩm vào nội dung, cho phép mua trực tiếp |
| **Outfit** | Bộ phối đồ gồm nhiều sản phẩm — đơn vị nội dung đặc thù thời trang |
| **Lookbook** | Tập hợp ảnh/outfit theo chủ đề hoặc bộ sưu tập |
| **Affiliate Link** | Đường dẫn có gắn mã định danh creator để quy kết |
| **Attribution** | Quy kết — xác định creator nào xứng đáng nhận công cho một đơn hàng |
| **Attribution Window** | Cửa sổ quy kết — khoảng thời gian sau khi click mà đơn hàng vẫn được tính cho creator |
| **Click** | Lượt nhấp vào affiliate link, có ghi nhận thời điểm |
| **Conversion** | Lượt click dẫn đến đơn hàng thành công |
| **Campaign** | Chiến dịch — gắn creator, sản phẩm, ngân sách và tỷ lệ hoa hồng trong một khoảng thời gian |
| **Live Commerce** | Bán hàng qua phát trực tiếp |

---

## D. Đơn hàng và thanh toán

| Thuật ngữ | Nghĩa | Phân biệt quan trọng |
|---|---|---|
| **Cart** | Giỏ hàng — tập hợp offer khách định mua | Chưa giữ tồn kho |
| **Checkout** | Phiên thanh toán — quá trình chuyển giỏ hàng thành đơn | Có giữ tồn kho tạm thời |
| **Order** | Đơn hàng — **góc nhìn của khách**, một mã đơn duy nhất | Cái khách nhìn thấy |
| **Fulfillment Order** | Đơn thực hiện — **góc nhìn vận hành**, một nhà bán / một điểm xuất hàng | Cái vận hành thực thi |
| **Order Line** | Dòng hàng trong đơn: một offer, số lượng, giá tại thời điểm mua | Giá phải "đóng băng" |
| **Payment** | Khoản thanh toán của khách | |
| **Payment Intent** | Ý định thanh toán — bản ghi trước khi tiền thực sự về | |
| **Refund** | Hoàn tiền cho khách | |
| **Ledger** | Sổ cái — bản ghi tài chính bất biến, chỉ ghi thêm | Không sửa, không xóa |
| **Ledger Entry** | Bút toán — một dòng ghi trong sổ cái | |
| **Adjustment** | Bút toán điều chỉnh — cách duy nhất để "sửa" sổ cái | |

**Phân biệt Order và Fulfillment Order** là một trong những quyết định quan trọng nhất của kiến trúc này. Xem [../adr/0007-marketplace-order-model.md](../adr/0007-marketplace-order-model.md).

---

## E. Tồn kho và kho vận

| Thuật ngữ | Nghĩa |
|---|---|
| **Inventory** | Tồn kho — số lượng SKU tại một địa điểm |
| **Available** | Khả dụng — có thể bán |
| **Reserved** | Đã giữ — tạm giữ cho một checkout đang diễn ra, chưa chắc chắn |
| **Committed** | Đã cam kết — thuộc về một đơn hàng đã xác nhận |
| **In Transit** | Đang trung chuyển — giữa các kho hoặc từ nhà cung cấp |
| **Damaged** | Hư hỏng — không bán được |
| **Returned** | Đã hoàn về — chờ kiểm định |
| **Stock Location** | Điểm lưu kho — kho, cửa hàng, hoặc kho của seller |
| **Replenishment** | Bổ sung hàng — quyết định nhập/sản xuất thêm |
| **Safety Stock** | Tồn kho an toàn — mức đệm phòng biến động nhu cầu |
| **Lead Time** | Thời gian chờ — từ lúc đặt đến lúc hàng sẵn sàng bán |

Xem sơ đồ chuyển trạng thái đầy đủ tại [../04-modules/inventory.md](../04-modules/inventory.md).

---

## F. Chuỗi cung ứng và sản xuất

| Thuật ngữ | Nghĩa |
|---|---|
| **Supplier** | Nhà cung cấp — cung cấp hàng thành phẩm hoặc nguyên liệu |
| **Manufacturer** | Nhà máy — thực hiện sản xuất |
| **Material Supplier** | Nhà cung cấp nguyên phụ liệu (vải, cúc, khóa kéo) |
| **Purchase Order (PO)** | Đơn mua hàng gửi nhà cung cấp |
| **Production Order** | Đơn sản xuất gửi nhà máy |
| **Production Batch** | Lô sản xuất — đơn vị truy vết giá vốn và chất lượng |
| **Tech Pack** | Hồ sơ kỹ thuật sản phẩm gửi nhà máy: thông số, chất liệu, quy cách |
| **Sample** | Mẫu — bản thử trước khi sản xuất hàng loạt |
| **Quality Control (QC)** | Kiểm định chất lượng |
| **AQL** | Acceptable Quality Limit — ngưỡng lỗi chấp nhận được khi kiểm mẫu |
| **Demand Signal** | Tín hiệu nhu cầu — dữ liệu hành vi được tổng hợp thành chỉ báo nhu cầu |
| **Forecast** | Dự báo nhu cầu |
| **Product Planning** | Lập kế hoạch sản phẩm — quyết định sản xuất gì, bao nhiêu, khi nào |
| **MOQ** | Minimum Order Quantity — số lượng đặt hàng tối thiểu của nhà cung cấp |
| **COGS** | Cost of Goods Sold — giá vốn hàng bán |

---

## G. Khách hàng

| Thuật ngữ | Nghĩa |
|---|---|
| **Guest** | Khách vãng lai — chưa đăng ký, có thể mua hàng |
| **Registered Customer** | Khách đã đăng ký tài khoản |
| **Member** | Thành viên — đã tham gia chương trình khách hàng thân thiết |
| **VIP** | Hạng cao nhất — theo chi tiêu hoặc lời mời |
| **Wishlist** | Danh sách yêu thích |
| **Loyalty Point** | Điểm thưởng |
| **Tier** | Hạng thành viên |

---

## H. Thuật ngữ kiến trúc

| Thuật ngữ | Nghĩa |
|---|---|
| **Module** | Đơn vị tổ chức code trong monolith, có ranh giới rõ ràng |
| **Bounded Context** | Ranh giới trong đó một mô hình domain có nghĩa nhất quán |
| **Aggregate** | Cụm entity được đảm bảo nhất quán trong một giao dịch |
| **Aggregate Root** | Entity đại diện cho aggregate, là cửa ngõ truy cập duy nhất |
| **Entity** | Đối tượng có định danh, thay đổi theo thời gian |
| **Value Object** | Đối tượng không có định danh, bất biến, so sánh bằng giá trị |
| **Domain Event** | Sự kiện nghiệp vụ đã xảy ra, ở thì quá khứ |
| **Repository** | Cổng truy cập dữ liệu của một aggregate |
| **Use Case** | Một thao tác nghiệp vụ ở tầng application |
| **Anti-Corruption Layer (ACL)** | Lớp chuyển đổi bảo vệ domain khỏi mô hình của hệ thống ngoài |

---

## I. Từ đa nghĩa theo ngữ cảnh

Đây là các từ **cố ý mang nghĩa khác nhau** ở các bounded context khác nhau. Không được hợp nhất chúng.

| Từ | Trong context | Nghĩa |
|---|---|---|
| **Product** | Catalog | Bản ghi trình bày cho khách, có nội dung marketing |
| **Product** | Supply Chain | Đối tượng sản xuất, có tech pack, định mức nguyên liệu, giá vốn |
| **Customer** | Commerce | Người mua, có giỏ hàng và đơn hàng |
| **Customer** | Analytics | Hồ sơ hành vi, có thể ẩn danh, gộp nhiều thiết bị |
| **Order** | Commerce | Đơn hàng khách nhìn thấy |
| **Order** | Supply Chain | Đơn mua/đơn sản xuất gửi nhà cung cấp |
| **Balance** | Payment | Số dư khả dụng để chi trả |
| **Balance** | Ledger | Kết quả tổng hợp các bút toán tại một thời điểm |
| **Available** | Inventory | Số lượng có thể bán |
| **Available** | Seller | Trạng thái gian hàng đang hoạt động |

**Cách xử lý:** mỗi context dùng tên đầy đủ có tiền tố khi cần rõ ràng — `CatalogProduct` vs `SupplyProduct`, `PurchaseOrder` vs `CustomerOrder`. Ánh xạ giữa các context được mô tả trong [../02-domain/bounded-contexts.md](../02-domain/bounded-contexts.md).

---

## J. Thuật ngữ bị cấm

Các từ sau **không được dùng** vì mơ hồ hoặc gây nhầm lẫn:

| Từ bị cấm | Dùng thay bằng | Lý do |
|---|---|---|
| Item | `OrderLine`, `CartItem`, `SKU`, `Offer` | Quá mơ hồ |
| Vendor | `Seller` hoặc `Supplier` | Hai khái niệm hoàn toàn khác nhau |
| Stock | `Inventory` + trạng thái cụ thể | Không rõ trạng thái nào |
| User | `Customer`, `Seller`, `Creator`, `StaffUser` | Không rõ vai trò |
| Shop | `SellerStore` | Nhầm với storefront |
| Transaction | `LedgerEntry` hoặc `Payment` | Nhầm với database transaction |
| Status | Tên trạng thái cụ thể của aggregate | Quá chung chung |
