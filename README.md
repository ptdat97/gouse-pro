# Gouse

Nền tảng thương mại thời trang: own brand + marketplace + creator commerce
+ chuỗi cung ứng.

```text
gouse-pro/
├── docs/        ← tài liệu: nghiệp vụ, domain, kiến trúc, ADR, roadmap
├── gouse/       ← Go backend · đặc tả OpenAPI
└── gouse-web/   ← Next.js monorepo · storefront, admin, seller
```

`docs/` nằm ở gốc chứ không nằm trong `gouse/`: phần lớn nội dung của nó —
nghiệp vụ, mô hình domain, quy tắc kiến trúc, ADR, roadmap — nói về CẢ HAI
phía, và `docs/08-frontend/` vốn đã mô tả `gouse-web`. Đặt trong backend
là nói sai phạm vi của chính nó.

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
| [docs/03-architecture/](docs/03-architecture/) | Quy tắc phụ thuộc giữa module, ranh giới tầng |
| [docs/adr/](docs/adr/) | Quyết định kiến trúc và **lý do** |
| [docs/10-roadmap/backlog.md](docs/10-roadmap/backlog.md) | Việc còn phải làm — backlog duy nhất |
| [gouse/api/openapi.yaml](gouse/api/openapi.yaml) | Hợp đồng API, nguồn sự thật |
