# Tác nhân: Creator

## 1. Phân loại

```text
KOC              — Key Opinion Consumer: người tiêu dùng có ảnh hưởng, quy mô nhỏ
KOL              — Key Opinion Leader: người ảnh hưởng quy mô lớn
Influencer       — Người có lượng theo dõi lớn, nội dung đa dạng
Stylist          — Chuyên gia phối đồ, tạo giá trị qua outfit và tư vấn
Content Partner  — Đối tác nội dung (tạp chí, kênh media, studio)
```

### Bảng phân biệt

| | KOC | KOL | Influencer | Stylist | Content Partner |
|---|---|---|---|---|---|
| Quy mô theo dõi | Nhỏ (1k–50k) | Lớn (100k+) | Lớn | Trung bình | Theo tổ chức |
| Độ tin cậy nhận thức | Rất cao | Trung bình | Trung bình | Cao (chuyên môn) | Cao |
| Chi phí hợp tác | Thấp / chỉ hoa hồng | Cao / có phí cố định | Cao | Trung bình | Hợp đồng |
| Mô hình trả tiền | Hoa hồng affiliate | Phí + hoa hồng | Phí + hoa hồng | Hoa hồng + phí tư vấn | Hợp đồng |
| Loại nội dung chính | Review thật, thử đồ | Video, campaign | Đa dạng | Outfit, lookbook | Bài viết, editorial |
| Tỷ lệ chuyển đổi | Cao | Thấp hơn | Thấp hơn | Cao | Trung bình |
| Vai trò chiến lược | Chuyển đổi | Nhận diện thương hiệu | Phủ sóng | Giảm tỷ lệ hoàn hàng | Uy tín thương hiệu |

**Nhận xét chiến lược:** KOC có tỷ lệ chuyển đổi cao nhất trên mỗi đồng chi phí, KOL tạo nhận diện. Nền tảng cần cả hai nhưng **cơ chế trả tiền khác nhau** — KOC chủ yếu hoa hồng thuần, KOL cần phí cố định vì họ không chấp nhận rủi ro doanh số.

**Hệ quả kiến trúc:** mô hình `Campaign` phải hỗ trợ đồng thời ba cấu trúc chi phí:

```text
1. Thuần hoa hồng       — trả theo doanh số phát sinh
2. Phí cố định          — trả một lần cho nội dung
3. Hỗn hợp             — phí cố định + hoa hồng
```

Không thiết kế `Campaign` chỉ với một trường `commission_rate`.

**Về Stylist:** đây là vai trò đặc thù thời trang. Stylist tạo ra `Outfit` — đơn vị nội dung ghép nhiều sản phẩm. Giá trị kinh doanh: tăng giá trị đơn hàng (khách mua cả bộ) và **giảm tỷ lệ hoàn hàng** (tư vấn size và phối đồ đúng). Đây là lý do `Outfit` là khái niệm hạng nhất, không phải "một bài viết có nhiều link".

---

## 2. Trách nhiệm

- Tạo nội dung trung thực về sản phẩm
- Gắn đúng sản phẩm vào nội dung
- Công bố rõ quan hệ tài trợ (yêu cầu pháp lý ở nhiều thị trường)
- Không đưa thông tin sai lệch về chất liệu, xuất xứ, công dụng
- Tuân thủ quy định nội dung của nền tảng

**Yêu cầu công bố tài trợ** có hệ quả kỹ thuật: nội dung thuộc chiến dịch có trả phí phải được **hệ thống tự động gắn nhãn**, không phụ thuộc vào việc creator có nhớ ghi hay không. Xem [content-commerce.md](content-commerce.md).

---

## 3. Quyền hạn

| Hành động | Điều kiện |
|---|---|
| Tạo hồ sơ creator | Đăng ký |
| Đăng nội dung | Đã được duyệt |
| Gắn sản phẩm vào nội dung | Sản phẩm đang bán, đã được duyệt |
| Tạo affiliate link | Đã được duyệt |
| Xem hiệu suất nội dung của mình | Luôn |
| Xem hoa hồng của mình | Luôn |
| Tham gia chiến dịch | Theo lời mời hoặc đăng ký |
| Xem thông tin khách hàng | **Không bao giờ** |

**Ràng buộc quan trọng:** creator thấy được **số liệu tổng hợp** (số click, số đơn, doanh thu quy kết), **không** thấy danh tính khách hàng. Đây là ranh giới quyền riêng tư không được vi phạm — creator không phải bên xử lý dữ liệu cá nhân của khách.

---

## 4. Quan hệ doanh thu

### Mô hình cơ bản

```text
Creator đăng nội dung, gắn sản phẩm
    ↓
Khách click affiliate link  →  ghi nhận Click (có thời điểm, có định danh phiên)
    ↓
Khách mua trong cửa sổ quy kết (attribution window)
    ↓
Đơn giao thành công, hết hạn đổi trả
    ↓
Hoa hồng creator được ghi nhận là Available
    ↓
Đối soát và chi trả
```

### Các quyết định chính sách cần được mô hình hóa

**a. Cửa sổ quy kết (attribution window)**

Bao lâu sau khi click thì đơn hàng vẫn tính cho creator? Ví dụ 7 ngày. Khách hàng thời trang thường không mua ngay — họ xem nội dung, cân nhắc, mua sau vài ngày. Cửa sổ quá ngắn sẽ không phản ánh đúng đóng góp thật của creator.

**b. Quy kết khi có nhiều creator**

Nếu khách click nội dung của Creator A hôm thứ Hai và Creator B hôm thứ Tư rồi mua thứ Năm — ai được tính?

Các mô hình:

```text
Last click        — B nhận toàn bộ. Đơn giản, dễ giải thích, dễ tranh chấp
First click       — A nhận toàn bộ
Chia tỷ lệ        — chia theo trọng số. Công bằng hơn, phức tạp hơn
```

**Khuyến nghị:** bắt đầu bằng **last click** vì đơn giản và creator dễ hiểu. Nhưng **thiết kế mô hình dữ liệu ghi lại toàn bộ chuỗi click**, không chỉ click cuối. Nếu chỉ lưu click cuối, sau này muốn đổi mô hình quy kết sẽ không có dữ liệu lịch sử để tính lại.

Đây là ứng dụng trực tiếp của nguyên tắc P14: chính sách đơn giản trước, dữ liệu đầy đủ ngay từ đầu.

**c. Hoa hồng bị đảo ngược khi hoàn hàng**

Nếu đơn bị hoàn sau khi đã ghi nhận hoa hồng, phải có bút toán đảo ngược. Nếu đã chi trả, phát sinh khoản phải thu ngược hoặc trừ vào kỳ sau.

Đây là lý do hoa hồng chỉ chuyển sang `Available` **sau khi hết hạn đổi trả**, giống với seller.

**d. Bên nào chịu chi phí hoa hồng creator?**

Với sản phẩm own brand: nền tảng chịu.
Với sản phẩm marketplace: tùy thỏa thuận — có thể trừ vào phần seller, hoặc nền tảng trừ vào hoa hồng của mình để khuyến khích.

`Campaign` phải lưu rõ trường này. Không mặc định.

---

## 5. Vòng đời

```text
   Đăng ký (Applied)
        │
        ▼
   Chờ duyệt (Pending Review)  — kiểm tra hồ sơ, kênh mạng xã hội, chất lượng nội dung
        │
        ├──→ Từ chối (Rejected)
        │
        ▼
   Đã duyệt (Approved)
        │
        ▼
   Đang hoạt động (Active) ◄─────┐
        │                         │
        ├──→ Tạm ngưng (Suspended)┘  (vi phạm nội dung, gian lận click)
        │
        └──→ Chấm dứt (Terminated)
```

### Rủi ro gian lận cần chống

| Kiểu gian lận | Mô tả | Cách phát hiện |
|---|---|---|
| Click ảo | Tự click link của mình nhiều lần | Phân tích IP, thiết bị, tần suất |
| Tự mua | Creator tự mua để lấy hoa hồng rồi hoàn hàng | Đối chiếu định danh, tỷ lệ hoàn cao bất thường |
| Cookie stuffing | Gắn link ẩn để chiếm quy kết | Kiểm tra tỷ lệ click/hiển thị bất thường |
| Nội dung sai lệch | Mô tả sai sản phẩm để bán được | Tỷ lệ hoàn hàng cao trên nội dung cụ thể |

**Hệ quả kiến trúc:** `Click` phải ghi đủ ngữ cảnh (thời điểm, IP đã ẩn danh hóa, dấu vân tay thiết bị, nguồn giới thiệu) để phát hiện bất thường sau này. Việc phát hiện gian lận là **bước riêng**, chạy bất đồng bộ, không nằm trong đường đi chính của việc ghi nhận click — nếu không sẽ làm chậm trải nghiệm.

---

## 6. Luồng nghiệp vụ chính

| Luồng | Tài liệu |
|---|---|
| Đăng ký creator và tạo affiliate | [../07-workflows/creator-affiliate.md](../07-workflows/creator-affiliate.md) |
| Nội dung dẫn tới mua hàng | [../07-workflows/content-commerce.md](../07-workflows/content-commerce.md) |

---

## 7. Dữ liệu sở hữu

Module [creator](../04-modules/creator.md) sở hữu:

```text
creator                — hồ sơ creator
creator_channel        — kênh mạng xã hội đã liên kết
creator_audience       — số liệu người theo dõi (tự khai hoặc đồng bộ)
creator_bank_account   — tài khoản nhận tiền
creator_tier           — hạng creator (nếu có chương trình phân hạng)
```

Module [affiliate](../04-modules/affiliate.md) sở hữu:

```text
affiliate_link         — link có gắn mã creator
click                  — lượt click, có ngữ cảnh
attribution            — bản ghi quy kết đơn hàng cho creator
commission_record      — hoa hồng phát sinh
```

Module [content](../04-modules/content.md) sở hữu:

```text
content                — nội dung
content_product_tag    — sản phẩm gắn trong nội dung
outfit                 — bộ phối đồ
```

**Ranh giới cần lưu ý:** ba module tách biệt — `creator` (danh tính), `affiliate` (quy kết và tiền), `content` (nội dung). Gộp cả ba sẽ tạo một module quá lớn; và quan trọng hơn, `content` cần được dùng bởi cả nội dung do nền tảng tự sản xuất (không có creator nào), nên nó không được phụ thuộc vào `creator`.
