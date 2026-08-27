# Gouse

Nền tảng thương mại thời trang: own brand + marketplace + creator commerce
+ chuỗi cung ứng.

```text
gouse-pro/
├── gouse/       ← Go backend · đặc tả OpenAPI · tài liệu kiến trúc
└── gouse-web/   ← Next.js monorepo · storefront, admin, seller
```

Trước đây là hai repo rời (`ptdat97/gouse`, `ptdat97/gouse-web`). Gộp lại
vì một lý do cụ thể: **OpenAPI là nguồn sự thật duy nhất**, và `gouse-web`
sinh kiểu TypeScript thẳng từ `gouse/api/openapi.yaml`. Hai repo rời nghĩa
là đặc tả và bên dùng nó lệch pha được — và đã lệch: `price_from` từng
được đặc tả khai là bắt buộc trong khi API không bao giờ trả về, còn
TypeScript thì che mất lỗi. Một repo thì một thay đổi đặc tả và bên dùng
nó đi chung một commit.

Lịch sử của cả hai repo được giữ nguyên: `git log gouse-web/...` và
`git blame` chạy liền mạch qua điểm gộp.

## Bắt đầu

```bash
cd gouse     && make run     # API ở cổng 8080
cd gouse-web && npm run dev  # 3000 admin · 3001 storefront · 3002 seller
```

Chi tiết ở [gouse/README.md](gouse/README.md) và
[gouse-web/README.md](gouse-web/README.md).

## Ngôn ngữ

Toàn bộ comment, tài liệu, commit message và chữ trên giao diện đều bằng
tiếng Việt. Định danh trong code theo quy ước của ngôn ngữ lập trình.

## Tài liệu

| Đường dẫn | Nội dung |
|---|---|
| [gouse/docs/03-architecture/](gouse/docs/03-architecture/) | Quy tắc phụ thuộc giữa module, ranh giới tầng |
| [gouse/docs/adr/](gouse/docs/adr/) | Quyết định kiến trúc và **lý do** |
| [gouse/docs/10-roadmap/backlog.md](gouse/docs/10-roadmap/backlog.md) | Việc còn phải làm — backlog duy nhất |
| [gouse/api/openapi.yaml](gouse/api/openapi.yaml) | Hợp đồng API, nguồn sự thật |
