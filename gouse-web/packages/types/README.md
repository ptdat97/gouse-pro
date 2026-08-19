# @fc/types

Kiểu TypeScript **sinh tự động** từ `api/openapi.yaml`.

```bash
npm run types        # sinh lại
npm run types:check  # CI: fail nếu lệch bản đã commit
```

## Không sửa tay

`openapi.d.ts` là file sinh. Sửa tay sẽ bị ghi đè ở lần sinh tiếp theo, và
tệ hơn: nó làm frontend tin vào một hợp đồng mà backend không cam kết.

Muốn đổi kiểu → sửa `api/openapi.yaml`, rồi sinh lại.

## Vì sao commit file sinh

Để `types:check` trong CI phát hiện được lệch: backend đổi đặc tả mà quên
sinh lại thì frontend vẫn biên dịch theo hợp đồng cũ, và lỗi chỉ lộ ra ở
runtime — đúng thứ việc sinh kiểu sinh ra để ngăn.
