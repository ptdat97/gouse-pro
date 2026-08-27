# Nghiệp vụ: Thương mại qua nội dung (Content Commerce)

## 1. Vì sao nội dung là động cơ tạo nhu cầu

Trong thời trang, khách hàng thường **không biết mình muốn gì** cho đến khi nhìn thấy. Khác với việc mua một cục sạc — nơi khách biết chính xác nhu cầu và đi tìm — thời trang là mua theo cảm hứng.

| Cách tiếp cận | Cơ chế | Chi phí |
|---|---|---|
| Tìm kiếm | Khách đã biết mình muốn gì | Cao (đấu giá từ khóa) |
| Quảng cáo hiển thị | Chen ngang sự chú ý | Cao, hiệu quả giảm dần |
| **Nội dung/creator** | **Tạo ra ham muốn** | **Thấp hơn, tích lũy theo thời gian** |

Nội dung tạo ra ba giá trị:

1. **Tạo nhu cầu** — khách không tìm áo khoác, nhưng thấy outfit đẹp và muốn mua.
2. **Giảm tỷ lệ hoàn hàng** — thấy sản phẩm trên người thật, đúng dáng, đúng size.
3. **Tạo dữ liệu nhu cầu** — nội dung nào được tương tác nhiều là tín hiệu về xu hướng.

Giá trị thứ hai thường bị bỏ qua nhưng có tác động tài chính trực tiếp. Ảnh sản phẩm trên mẫu chuyên nghiệp không cho khách biết chiếc áo trông thế nào trên người có dáng giống mình.

---

## 2. Các loại nội dung

```text
Video       — video ngắn, review, thử đồ
Image       — ảnh đơn, ảnh bộ
Lookbook    — tập ảnh theo chủ đề hoặc bộ sưu tập
Article     — bài viết, hướng dẫn phối đồ, xu hướng
Outfit      — bộ phối đồ gồm nhiều sản phẩm
Live        — phát trực tiếp bán hàng
```

### Outfit — đơn vị nội dung đặc thù thời trang

Đây là loại nội dung quan trọng nhất và cần được mô hình hóa riêng.

```text
Outfit "Đi làm mùa thu"
├── Áo sơ mi linen trắng    (Product A, size M)
├── Quần âu ống suông        (Product B, size 28)
├── Giày loafer da           (Product C, size 39)
└── Túi tote canvas          (Product D)
```

**Vì sao Outfit không chỉ là "bài viết có nhiều link":**

| Nếu là bài viết có link | Nếu Outfit là entity |
|---|---|
| Không tính được giá trị cả bộ | Hiển thị tổng giá, cho phép mua cả bộ |
| Không gợi ý được thay thế | "Hết size? Đây là món tương tự" |
| Không đo được hiệu quả bộ | Đo tỷ lệ mua cả bộ vs mua lẻ |
| Không tái sử dụng được | Một outfit xuất hiện ở nhiều nơi |

**Giá trị kinh doanh:** outfit tăng giá trị đơn hàng trung bình đáng kể. Khách vào định mua một chiếc áo, mua cả bộ.

**Hệ quả kiến trúc:** `Outfit` là aggregate riêng trong module [content](../04-modules/content.md), có quan hệ nhiều-nhiều với `Product`, và có khả năng thay thế sản phẩm khi hết hàng.

---

## 3. Gắn sản phẩm vào nội dung (Product Tagging)

```text
Content
   │
   ├── ProductTag { product_id, vị trí trên ảnh/video, thời điểm trong video }
   ├── ProductTag { ... }
   └── ProductTag { ... }
```

### Yêu cầu

- Một nội dung tham chiếu **nhiều** sản phẩm.
- Một sản phẩm xuất hiện trong **nhiều** nội dung.
- Tag có thể gắn vị trí không gian (tọa độ trên ảnh) hoặc thời gian (giây thứ mấy trong video).
- Tag phải xử lý được trường hợp sản phẩm **hết hàng hoặc ngừng bán** sau khi nội dung đã đăng.

Yêu cầu cuối cùng quan trọng: nội dung sống lâu hơn sản phẩm. Một video hay có thể được xem trong nhiều tháng, khi sản phẩm gốc đã hết. Hệ thống phải:

```text
Sản phẩm hết hàng tạm thời  → hiển thị "Tạm hết hàng", cho phép nhận thông báo
Sản phẩm ngừng bán vĩnh viễn → hiển thị sản phẩm tương tự
```

Không được để nội dung dẫn tới trang lỗi — đó là lãng phí toàn bộ công sức tạo nội dung.

---

## 4. Đường đi từ nội dung đến đơn hàng

```text
Creator tạo nội dung, gắn sản phẩm
        │
        ▼
Nội dung được duyệt và xuất bản
        │
        ▼
Nội dung được phân phối
   ├── Feed trên nền tảng
   ├── Trang sản phẩm (nội dung liên quan)
   ├── Trang bộ sưu tập
   └── Chia sẻ ra mạng xã hội (có affiliate link)
        │
        ▼
Khách xem nội dung  →  ghi nhận View
        │
        ▼
Khách click sản phẩm  →  ghi nhận Click + gắn attribution
        │
        ▼
Khách xem trang sản phẩm
        │
        ├──→ Rời đi (attribution vẫn còn hiệu lực trong cửa sổ quy kết)
        │
        ▼
Thêm giỏ hàng  →  tín hiệu nhu cầu mạnh
        │
        ▼
Đặt hàng  →  Conversion
        │
        ▼
Quy kết cho creator  →  ghi nhận hoa hồng
```

Chi tiết kỹ thuật: [../07-workflows/content-commerce.md](../07-workflows/content-commerce.md).

---

## 5. Live Commerce

Phát trực tiếp bán hàng là dạng nội dung có đặc thù kỹ thuật riêng.

### Khác biệt so với nội dung thường

| | Nội dung tĩnh | Live |
|---|---|---|
| Thời gian | Xem bất kỳ lúc nào | Đồng thời, thời gian thực |
| Tồn kho | Áp lực bình thường | **Đột biến cực mạnh** |
| Giá | Cố định | Có thể có giá riêng trong phiên |
| Tương tác | Bình luận | Bình luận, hỏi đáp trực tiếp |
| Quy kết | Theo click | Theo phiên live |

**Vấn đề kỹ thuật lớn nhất: đột biến tồn kho.**

Khi người dẫn nói "chỉ còn 50 chiếc, giá sốc trong 5 phút", có thể có hàng nghìn người bấm mua trong vài giây. Đây là kịch bản tranh chấp tồn kho khắc nghiệt nhất của toàn hệ thống.

Hệ quả kiến trúc:

```text
1. Cơ chế giữ tồn kho phải chịu được tranh chấp cao
   → không dùng khóa bi quan trên toàn bộ SKU

2. Phải có hàng đợi hoặc cơ chế hạn chế tốc độ
   → tránh làm sập hệ thống

3. Phải xử lý được oversell một cách có kiểm soát
   → thà từ chối rõ ràng còn hơn bán rồi hủy

4. Giá riêng trong phiên live phải có thời hạn rõ ràng
   → và được đóng băng vào đơn khi đặt
```

Xem [../04-modules/inventory.md](../04-modules/inventory.md) mục về tranh chấp đồng thời.

**Quyết định lộ trình:** live commerce thuộc Phase 3–4. Nhưng mô hình tồn kho phải được thiết kế chịu được tranh chấp cao **ngay từ MVP** — vì đổi cơ chế giữ tồn kho sau này rất rủi ro.

---

## 6. Kiểm duyệt nội dung

Nội dung do bên thứ ba tạo cần được kiểm soát.

```text
   Nháp (Draft)
      │
      ▼
   Chờ duyệt (Pending Review)
      │
      ├── Tự động: kiểm tra sản phẩm hợp lệ, từ ngữ cấm, bản quyền hình ảnh
      ├── Thủ công: với creator mới hoặc nội dung có gắn cờ
      │
      ├──→ Từ chối (Rejected) — có lý do rõ ràng
      │
      ▼
   Đã xuất bản (Published)
      │
      ├──→ Bị gỡ (Taken Down) — vi phạm phát hiện sau
      │
      └──→ Lưu trữ (Archived) — hết thời hạn hoặc creator gỡ
```

### Cần kiểm tra gì

| Loại | Kiểm tra |
|---|---|
| Sản phẩm | Sản phẩm có tồn tại, đang bán, không bị cấm |
| Thương hiệu | Không mạo danh, không nói sai về thương hiệu |
| Bản quyền | Hình ảnh, nhạc nền không vi phạm |
| Ngôn từ | Không vi phạm chuẩn cộng đồng |
| Công bố tài trợ | Nội dung có trả phí phải được gắn nhãn |

**Về công bố tài trợ:** hệ thống phải **tự động gắn nhãn** khi nội dung thuộc chiến dịch có trả phí. Không phụ thuộc vào việc creator có nhớ ghi hay không — đây là nghĩa vụ pháp lý của nền tảng ở nhiều thị trường.

---

## 7. Khám phá (Discovery)

Nội dung chỉ có giá trị nếu được nhìn thấy.

```text
Trending          — nội dung đang được tương tác nhiều
New Arrivals      — sản phẩm mới, nội dung mới
For You           — cá nhân hóa theo hành vi
Collections       — theo bộ sưu tập
Creator Content   — theo creator đang theo dõi
Lookbooks         — theo chủ đề
Campaigns         — nội dung chiến dịch đang chạy
```

**Nguyên tắc P13/P14 áp dụng:** khám phá là một **năng lực thay thế được**. Cài đặt đầu tiên là quy tắc đơn giản (mới nhất, nhiều tương tác nhất, cùng danh mục). Interface được thiết kế để sau này thay bằng hệ thống cá nhân hóa mà không sửa module gọi.

Xem [../04-modules/recommendation.md](../04-modules/recommendation.md).

---

## 8. Vòng phản hồi từ nội dung về sản phẩm

Đây là mắt xích của bánh đà mà nhiều nền tảng bỏ lỡ.

```text
Nội dung có tương tác cao
     │
     ├── Sản phẩm nào được tag nhiều nhất?
     ├── Outfit nào được lưu nhiều nhất?
     ├── Nội dung nào có tỷ lệ click → mua cao nhất?
     ├── Kiểu dáng/màu sắc nào được quan tâm nhưng chưa có hàng?
     │
     ▼
Tín hiệu nhu cầu (Demand Signal)
     │
     ▼
Kế hoạch sản phẩm own brand
```

**Ví dụ cụ thể:** một video thử áo khoác dạ oversize có 50.000 lượt xem và 3.000 lượt click, nhưng sản phẩm chỉ còn size S. Đó là tín hiệu rõ ràng: nhu cầu tồn tại, nguồn cung không đủ, cần sản xuất thêm với phân bổ size đúng.

**Hệ quả kiến trúc:** dữ liệu tương tác nội dung phải chảy vào module [supply-chain](../04-modules/supply-chain.md) dưới dạng tín hiệu nhu cầu, không chỉ nằm trong báo cáo marketing.

---

## 9. Chỉ số nội dung

| Chỉ số | Ý nghĩa |
|---|---|
| Content view | Lượt xem nội dung |
| Engagement rate | Tỷ lệ tương tác (thích, lưu, bình luận) |
| Content-to-click rate | Tỷ lệ xem → click sản phẩm |
| Click-to-purchase rate | Tỷ lệ click → mua |
| Content-attributed GMV | GMV quy kết cho nội dung |
| Outfit multi-item rate | Tỷ lệ mua từ 2 món trở lên trong outfit |
| Return rate by content | Tỷ lệ hoàn hàng theo nội dung — phát hiện mô tả sai lệch |

Chỉ số cuối cùng là công cụ kiểm soát chất lượng: nội dung có tỷ lệ hoàn hàng cao bất thường là dấu hiệu nội dung gây hiểu nhầm.

---

## 10. Tài liệu liên quan

- [creator.md](creator.md) — tác nhân creator
- [../04-modules/content.md](../04-modules/content.md) — đặc tả module
- [../04-modules/affiliate.md](../04-modules/affiliate.md) — quy kết và hoa hồng
- [../07-workflows/content-commerce.md](../07-workflows/content-commerce.md) — luồng chi tiết
