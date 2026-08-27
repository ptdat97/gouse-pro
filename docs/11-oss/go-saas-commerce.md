# go-saas/commerce

| | |
|---|---|
| Repository | `github.com/go-saas/commerce` |
| License | **KHÔNG CÓ** |
| Sao / Fork | **5 / 2** |
| Cập nhật cuối | **2023-09-03 — ngừng gần 3 năm** |
| Vai trò | **Gần như không dùng được** |

---

## 1. Kết luận thẳng: dự án này không đủ tiêu chuẩn tham chiếu

Nhiệm vụ nghiên cứu liệt kê go-saas/commerce là "tham chiếu thứ yếu". Sau khi kiểm tra dữ liệu thật, tôi đánh giá nó **thấp hơn cả mức đó**.

Ba vấn đề chặn:

### 1.1 Không có license

```text
Không có file LICENSE = tác giả giữ TOÀN QUYỀN theo mặc định.

Hệ quả pháp lý:
  ✗ KHÔNG được sao chép code
  ✗ KHÔNG được dùng làm phụ thuộc
  ✗ KHÔNG được phân phối lại dưới bất kỳ hình thức nào
```

Đây không phải vấn đề kỹ thuật mà là ràng buộc pháp lý tuyệt đối. Một dự án MIT có 50 sao vẫn dùng được; một dự án không license có 5.000 sao cũng không.

### 1.2 Không có cộng đồng

5 sao, 2 fork. So sánh trong nhóm nghiên cứu:

```text
Medusa      35.728
Saleor      23.218
GoCommerce   1.617
Flamingo       591
Digota         524
go-saas          5   ← chưa được kiểm chứng bởi ai
```

Số sao không đo chất lượng code, nhưng ở mức 5 sao thì không có bằng chứng nào cho thấy dự án từng được dùng trong production.

### 1.3 Ngừng phát triển

Đẩy code cuối tháng 9/2023. Gần ba năm không cập nhật.

---

## 2. Điều duy nhất đáng ghi nhận

Dự án có tổ chức module theo miền (cart, order, payment, product, user) và dùng OpenAPI. Nhưng:

```text
Cả hai ý tưởng này đều đã có ở nguồn TỐT HƠN:
  Tổ chức module → Flamingo, GoShop
  OpenAPI-first   → Saleor, Medusa, GoCommerce
```

Không có ý tưởng nào **chỉ** go-saas mới có.

---

## 3. Quyết định

```text
✗ REJECT hoàn toàn

Không sao chép code (rào cản pháp lý)
Không dùng làm phụ thuộc (rào cản pháp lý + ngừng bảo trì)
Không dùng làm tham chiếu kiến trúc (có nguồn tốt hơn)
```

---

## 4. Bài học rút ra cho quy trình đánh giá OSS

Trường hợp này minh họa vì sao [dependency-registry.md](dependency-registry.md) bắt buộc ghi bốn thông tin trước khi xét bất kỳ dự án nào:

```text
1. License          → không có license = dừng ngay
2. Ngày cập nhật    → ngừng > 2 năm = rủi ro cao
3. Quy mô cộng đồng → tín hiệu về việc đã được kiểm chứng chưa
4. Số phụ thuộc     → phụ thuộc càng nhiều, rủi ro càng cao
```

Nếu bỏ qua bước này và đi thẳng vào đọc code, có thể tốn nhiều giờ cho một dự án mà **về mặt pháp lý không dùng được**.

Đây cũng là lý do tôi kiểm tra GitHub API cho **toàn bộ 12 dự án** trước khi đọc bất kỳ dòng code nào.

---

## 5. Tài liệu liên quan

- [dependency-registry.md](dependency-registry.md) — quy trình đánh giá
- [README.md](README.md) mục 5 — bảng license
