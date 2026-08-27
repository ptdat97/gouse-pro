# Nghiệp vụ: Thương hiệu riêng (Own Brand)

## 1. Vì sao own brand tồn tại

Own brand là trụ cột đầu tiên và là nguồn lợi nhuận chính ở giai đoạn đầu.

| Lý do | Giải thích |
|---|---|
| Biên lợi nhuận cao | Không chia hoa hồng, kiểm soát giá vốn |
| Kiểm soát chất lượng | Quyết định chất liệu, quy cách, nhà máy |
| Kiểm soát trải nghiệm | Bao bì, thời gian giao, chính sách đổi trả |
| Khác biệt hóa | Sản phẩm không tìm được ở nơi khác |
| Dữ liệu đầy đủ | Biết chính xác giá vốn, tồn kho, hiệu quả từng SKU |

Đánh đổi: cần vốn lưu động, chịu rủi ro tồn kho, chu kỳ phát triển sản phẩm dài.

---

## 2. Vòng đời sản phẩm own brand

Đây là điểm khác biệt lớn nhất so với ecommerce thông thường. Sản phẩm own brand **không xuất hiện từ hư không** — nó có một vòng đời dài trước khi được bán.

```text
   Concept (Ý tưởng)
      │  ← đến từ tín hiệu nhu cầu, xu hướng, khoảng trống danh mục
      ▼
   Design (Thiết kế)
      │  ← phác thảo, chọn chất liệu, bảng màu
      ▼
   Tech Pack (Hồ sơ kỹ thuật)
      │  ← thông số, định mức, quy cách may
      ▼
   Costing (Tính giá vốn dự kiến)
      │  ← báo giá nhà cung cấp, tính biên lợi nhuận mục tiêu
      ▼
   Sampling (Làm mẫu)
      │  ← có thể lặp nhiều vòng
      ▼
   Sample Approval (Duyệt mẫu)
      │
      ├──→ Từ chối → quay lại Design hoặc Tech Pack
      │
      ▼
   Production Planning (Lập kế hoạch sản xuất)
      │  ← quyết định số lượng, phân bổ size, thời điểm
      ▼
   Production (Sản xuất)
      │
      ▼
   Quality Control (Kiểm định)
      │
      ├──→ Không đạt → làm lại / giảm giá / hủy
      │
      ▼
   Warehouse Receiving (Nhập kho)
      │
      ▼
   Catalog Publishing (Lên sàn)
      │
      ▼
   Selling (Bán)
      │
      ├──→ Replenishment (Bổ sung) → quay lại Production Planning
      │
      ▼
   End of Life (Kết thúc vòng đời)
      │  ← xả hàng, thanh lý, ngừng bán
```

**Thời gian điển hình:** từ concept đến lên sàn mất 3–6 tháng với sản phẩm mới, 4–8 tuần với sản phẩm tái sản xuất.

---

## 3. Vấn đề kiến trúc: `Product` xuất hiện ở đâu trong vòng đời này?

Đây là câu hỏi dễ trả lời sai.

**Cách sai:** tạo bản ghi `Product` trong catalog ngay từ bước Concept, rồi để nó ở trạng thái nháp suốt 6 tháng.

Vấn đề: module `catalog` sẽ chứa hàng nghìn bản ghi chưa bao giờ được bán, lẫn với sản phẩm thật. Mọi truy vấn catalog phải lọc trạng thái. Và tệ hơn — dữ liệu phát triển sản phẩm (tech pack, giá vốn, nhà cung cấp) sẽ bị nhét vào bảng sản phẩm của catalog, làm catalog phụ thuộc vào supply chain.

**Cách đúng:** hai khái niệm riêng, ở hai bounded context riêng.

```text
Supply Chain Context              Catalog Context
────────────────────              ───────────────
ProductDevelopment                CatalogProduct
  - concept                         - tên hiển thị
  - tech pack                       - mô tả marketing
  - bill of materials               - hình ảnh
  - costing                         - danh mục
  - sample                          - thuộc tính tìm kiếm
  - supplier                        - variant, SKU
  - production plan
        │
        │  khi mẫu được duyệt và
        │  quyết định sản xuất
        ▼
   Phát event ProductDevelopmentApproved
        │
        ▼
   Catalog tạo CatalogProduct (trạng thái nháp)
        │
        │  khi hàng đã nhập kho
        ▼
   Phát event InventoryReceived → CatalogProduct có thể publish
```

Đây là một ví dụ điển hình của việc **cùng một từ mang hai nghĩa ở hai context** — đã ghi trong [../00-overview/business-glossary.md](../00-overview/business-glossary.md) mục I.

Ánh xạ giữa hai context được thực hiện qua event và một khóa liên kết, không phải qua khóa ngoại trực tiếp. Chi tiết: [../02-domain/bounded-contexts.md](../02-domain/bounded-contexts.md).

---

## 4. Bộ sưu tập và tính mùa vụ

Thời trang bán theo mùa. Đây không phải chi tiết phụ — nó định hình toàn bộ quy trình.

```text
Collection (Bộ sưu tập)
  - tên: "Thu Đông 2026"
  - mùa: FW2026
  - ngày ra mắt dự kiến
  - ngày kết thúc bán dự kiến
  - chủ đề, bảng màu
  - danh sách sản phẩm
  - ngân sách sản xuất
```

**Hệ quả kinh doanh:** một bộ sưu tập bán không hết trước khi hết mùa sẽ mất 50–70% giá trị. Đây là rủi ro tài chính lớn nhất của own brand.

**Hệ quả kiến trúc:**

1. `Collection` là aggregate hạng nhất trong catalog, không phải một cái tag.
2. Kế hoạch sản xuất gắn với bộ sưu tập, có mốc thời gian bắt buộc.
3. Hệ thống phải cảnh báo khi tiến độ sản xuất đe dọa ngày ra mắt.
4. Cần theo dõi tỷ lệ bán hết (sell-through rate) theo bộ sưu tập, không chỉ theo SKU.

### Chỉ số sell-through

```text
Sell-through rate = Số lượng đã bán / Số lượng đã sản xuất

Tuần 4:  mục tiêu > 30%   — nếu thấp hơn, cân nhắc khuyến mãi sớm
Tuần 8:  mục tiêu > 60%
Tuần 12: mục tiêu > 80%   — nếu thấp hơn, chuẩn bị xả hàng
```

Chỉ số này phải tính được **theo thời gian thực** để can thiệp kịp, không phải báo cáo cuối mùa khi đã quá muộn.

---

## 5. Phân bổ size — vấn đề đặc thù thời trang

Sản xuất 500 chiếc áo không phải là sản xuất 500 chiếc giống nhau. Phải quyết định:

```text
S:  15%   =  75 chiếc
M:  30%   = 150 chiếc
L:  30%   = 150 chiếc
XL: 20%   = 100 chiếc
XXL: 5%   =  25 chiếc
```

Phân bổ sai dẫn tới tình huống rất tốn kém: hết size M trong hai tuần, còn tồn XXL đến cuối mùa. Doanh số mất đi ở size bán chạy **và** hàng tồn ở size ế.

**Hệ quả kiến trúc:**

- Kế hoạch sản xuất phải ở mức SKU (bao gồm size), không phải mức Product.
- Dữ liệu bán hàng lịch sử theo size là đầu vào bắt buộc của planning.
- Tồn kho theo size phải được theo dõi riêng và ảnh hưởng tới quyết định bổ sung.
- Tỷ lệ hoàn hàng theo size là tín hiệu về vấn đề form dáng, cần đưa vào vòng phản hồi thiết kế.

Điểm cuối cùng là ví dụ đẹp của bánh đà: nếu size M bị hoàn nhiều với lý do "chật", đó là dữ liệu để sửa bảng size ở lô sau.

---

## 6. Kết nối với tín hiệu nhu cầu

Đây là nơi own brand tạo ra lợi thế cạnh tranh thật sự.

```text
Dữ liệu hành vi trên nền tảng
  - sản phẩm được tìm nhiều nhưng không có hàng
  - sản phẩm marketplace bán chạy
  - nội dung creator có tương tác cao
  - wishlist tập trung vào kiểu dáng nào
  - size nào hay hết hàng
        │
        ▼
   Demand Signal (Tín hiệu nhu cầu)
        │
        ▼
   Product Planning (Lập kế hoạch)
        │
        ▼
   Own brand sản xuất đúng thứ thị trường đang muốn
```

**Ví dụ cụ thể:** nếu 2000 khách tìm "áo khoác dạ oversize" trong một tháng và marketplace chỉ có ba offer với giá cao, đó là tín hiệu rõ ràng để own brand làm sản phẩm này.

Đây là điều mà một thương hiệu thời trang truyền thống không làm được — họ không có dữ liệu nhu cầu ở quy mô này. Và là điều mà một marketplace thuần túy không tận dụng được — họ không có năng lực sản xuất.

**Hệ quả kiến trúc:** dữ liệu hành vi phải quay ngược được vào domain supply chain. Nếu dữ liệu này nằm trong một công cụ analytics bên thứ ba mà supply chain không truy vấn được, bánh đà bị đứt. Xem [supply-chain.md](supply-chain.md) và [../04-modules/supply-chain.md](../04-modules/supply-chain.md).

---

## 7. Cách own brand xuất hiện trong hệ thống

Nhắc lại quyết định từ [seller.md](seller.md): own brand là một **seller nội bộ**.

```text
Seller {
  id: "internal-own-brand"
  seller_type: INTERNAL
  commission_rate: 0
  inventory_owner: PLATFORM
  settlement_mode: INTERNAL_LEDGER
}
```

Nhờ đó:

- Giỏ hàng chứa cả own brand và marketplace hoạt động tự nhiên.
- Một `FulfillmentOrder` của own brand đi qua kho nền tảng, của seller đi qua seller.
- Báo cáo so sánh hiệu quả 1P và 3P dùng chung cấu trúc dữ liệu.

Khác biệt duy nhất nằm ở tầng ledger: đơn own brand ghi doanh thu toàn phần + COGS; đơn marketplace ghi hoa hồng.

---

## 8. Luồng nghiệp vụ chính

| Luồng | Tài liệu |
|---|---|
| Tạo sản phẩm own brand | [../07-workflows/own-brand-product.md](../07-workflows/own-brand-product.md) |
| Đặt sản xuất | [../07-workflows/supplier-production.md](../07-workflows/supplier-production.md) |
| Bổ sung hàng | [../07-workflows/replenishment.md](../07-workflows/replenishment.md) |

---

## 9. Chỉ số đo lường

| Chỉ số | Ý nghĩa | Ngưỡng tham khảo |
|---|---|---|
| Gross margin | Biên lợi nhuận gộp | > 55% |
| Sell-through rate | Tỷ lệ bán hết theo mùa | > 80% cuối mùa |
| Inventory turnover | Vòng quay tồn kho | > 4 lần/năm |
| Concept-to-shelf time | Thời gian từ ý tưởng đến lên sàn | < 120 ngày |
| Sample approval cycles | Số vòng làm mẫu | < 3 |
| Markdown rate | Tỷ lệ hàng phải giảm giá | < 20% |
| Stockout rate | Tỷ lệ hết hàng ở SKU bán chạy | < 5% |

Xem thêm [kpi.md](kpi.md).
