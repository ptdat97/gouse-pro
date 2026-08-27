# ADR-0002: API First

**Trạng thái:** Accepted

---

## Context

Nền tảng phục vụ **sáu loại mặt tiền** khác nhau:

```text
Storefront       — khách hàng mua sắm
Seller Center    — nhà bán
Creator Center   — creator
Admin            — nhân viên vận hành
Mobile App       — tương lai
Partner API      — tương lai
```

Nếu API được sinh ra **sau** giao diện, nó sẽ mang hình dạng của một màn hình cụ thể và không dùng lại được cho mặt tiền khác.

Ngoài ra, backend và frontend cần làm **song song** để không chờ nhau.

---

## Decision

**Hợp đồng API được thiết kế và thống nhất TRƯỚC khi viết code cài đặt hoặc giao diện.**

```text
1. Xác định use case nghiệp vụ
2. Thiết kế hợp đồng API
3. Viết đặc tả OpenAPI (/api/openapi.yaml)
4. Rà soát cùng backend + frontend + người hiểu nghiệp vụ
5. Sinh mã và server giả lập
6. Cài đặt SONG SONG
7. Kiểm thử hợp đồng trong CI
```

**OpenAPI là nguồn sự thật duy nhất.** Cập nhật đặc tả **cùng pull request** với thay đổi code; CI so sánh và thất bại nếu lệch.

---

## Alternatives

### A. Code First — sinh đặc tả từ code — **bị loại**

```text
Ưu:
    + Đặc tả luôn khớp code
    + Ít công viết đặc tả

Nhược (quyết định):
    − Frontend phải CHỜ backend xong
    − API mang hình dạng của cài đặt, không phải của nghiệp vụ
    − Không rà soát được hợp đồng trước khi tốn công cài đặt
    − Thay đổi phá vỡ chỉ phát hiện khi frontend gặp lỗi
```

### B. Giao diện trước, API theo sau — **bị loại**

```text
Ưu:
    + Thiết kế trải nghiệm được ưu tiên
    + Nhanh cho mặt tiền đầu tiên

Nhược (quyết định):
    − API "lấy dữ liệu trang chủ" chỉ dùng được cho trang chủ web
    − App di động cần trang chủ khác → viết API mới
    − Đối tác muốn lấy danh mục → không có API phù hợp
    − Cùng logic nghiệp vụ viết lại nhiều lần
```

### C. GraphQL — **bị loại (cho giai đoạn này)**

```text
Ưu:
    + Client lấy đúng dữ liệu cần
    + Một endpoint

Nhược (quyết định):
    − Phân quyền phức tạp hơn nhiều (phải kiểm tra ở từng trường)
    − Cache khó (không dùng được cache HTTP thông thường)
    − Rủi ro truy vấn tốn kém từ client
    − Đội chưa có kinh nghiệm vận hành
    − REST đủ cho nhu cầu hiện tại
```

**Lưu ý:** không loại vĩnh viễn. Nếu sau này có nhiều mặt tiền với nhu cầu dữ liệu rất khác nhau, có thể cân nhắc lại.

---

## Consequences

### Tích cực

```text
✓ Backend và frontend làm song song (dùng server giả lập)
✓ API dùng lại được cho mọi mặt tiền
✓ Hợp đồng được rà soát trước khi tốn công cài đặt
✓ Sinh tự động: kiểu TypeScript, client SDK, tài liệu
✓ Thay đổi phá vỡ phát hiện ngay trong CI
✓ Đối tác tích hợp được mà không cần code riêng
```

### Tiêu cực

```text
− Thêm bước thiết kế trước khi code
− Cần duy trì đặc tả OpenAPI đồng bộ với code
− Đôi khi thiết kế API trước dẫn tới hợp đồng chưa tối ưu
  (phát hiện vấn đề khi cài đặt)
```

### Ngoại lệ có kiểm soát: endpoint tổng hợp (BFF)

Nguyên tắc "API phản ánh khả năng nghiệp vụ" có chi phí thật: trang sản phẩm cần 5 lệnh gọi.

```text
Cho phép endpoint tổng hợp KHI:
    ✓ Có vấn đề hiệu năng ĐO ĐƯỢC (không phải phỏng đoán)
    ✓ Đặt trong nhóm riêng /api/v1/storefront/...
    ✓ CHỈ tổng hợp, KHÔNG có logic nghiệp vụ mới
    ✓ API thành phần vẫn tồn tại và dùng độc lập được
```

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Thêm bước thiết kế hợp đồng | Làm song song, API dùng lại được |
| Duy trì đặc tả OpenAPI | Sinh mã tự động, phát hiện lệch sớm |
| REST thay vì GraphQL | Phân quyền và cache đơn giản |
| Nhiều lệnh gọi hơn cho một trang | Hợp đồng rõ ràng, dùng lại được (bù bằng BFF khi cần) |

---

## Hệ quả bắt buộc: Frontend không truy cập database

Quyết định này kéo theo ràng buộc: **Next.js không bao giờ truy cập database**, dù về mặt kỹ thuật server component có thể làm được.

```text
Lý do:
    - Logic nghiệp vụ sẽ dần rò rỉ vào frontend
    - App di động không chạy được code Next.js
    - Không qua phân quyền, giới hạn tốc độ, ghi log của backend
    - Truy vấn không xuất hiện trong giám sát backend
```

---

## Tài liệu liên quan

- [../03-architecture/api-first.md](../03-architecture/api-first.md)
- [../06-api/api-guidelines.md](../06-api/api-guidelines.md)
- [ADR-0004](0004-nextjs-frontend.md) — vai trò của frontend
