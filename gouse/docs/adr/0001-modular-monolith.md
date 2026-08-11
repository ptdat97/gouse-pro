# ADR-0001: Bắt đầu bằng Modular Monolith

**Trạng thái:** Accepted

---

## Context

Chúng ta xây dựng nền tảng thương mại thời trang gồm nhiều miền nghiệp vụ: thương mại, marketplace, creator commerce, chuỗi cung ứng, tài chính. Tổng cộng khoảng 27 module dự kiến.

Bối cảnh cụ thể:

```text
- Ranh giới domain CHƯA ỔN ĐỊNH — đây là hệ thống mới, mô hình
  kinh doanh sẽ tiến hóa qua 5 giai đoạn
- Đội ngũ ban đầu nhỏ
- Nhiều luồng nghiệp vụ cần nhất quán mạnh (tồn kho, tài chính)
- Cần ra thị trường nhanh để kiểm chứng mô hình
```

Câu hỏi: bắt đầu bằng microservices hay monolith?

---

## Decision

**Bắt đầu bằng Modular Monolith**: một tiến trình Go duy nhất, chia module với ranh giới **nghiêm ngặt như thể chúng đã là service riêng**.

Cụ thể:

```text
✓ Mỗi module có interface công khai (public.go) — điểm vào duy nhất
✓ Mỗi bảng thuộc đúng một module
✓ Không JOIN, không khóa ngoại vượt ranh giới module
✓ Giao tiếp qua interface đồng bộ hoặc domain event
✓ Event contract thiết kế như thể sẽ vượt tiến trình
✓ Kiểm tra ranh giới tự động trong CI — vi phạm làm CI thất bại
```

---

## Alternatives

### A. Microservices từ đầu — **bị loại**

```text
Ưu:
    + Ranh giới vật lý, không thể vi phạm
    + Mở rộng quy mô độc lập ngay
    + Đội độc lập

Nhược (quyết định):
    − Ranh giới domain chưa ổn định → di chuyển ranh giới
      giữa các service là DỰ ÁN DI TRÚ DỮ LIỆU, không phải refactor
    − Giao dịch phân tán cho luồng đặt hàng (chạm 5-6 module)
    − Chi phí vận hành cao: điều phối, giám sát, truy vết
    − Gỡ lỗi phức tạp
    − Đội nhỏ không đủ năng lực vận hành
```

**Lý do loại chính:** chi phí sửa ranh giới sai. Trong monolith là refactor vài ngày; giữa các service là dự án nhiều tuần với di trú dữ liệu và điều phối triển khai.

### B. Monolith thông thường (không module hóa) — **bị loại**

```text
Ưu:
    + Viết nhanh nhất
    + Không có ràng buộc

Nhược (quyết định):
    − Sau 1-2 năm trở thành khối rối không tách được
    − Thay đổi lan truyền không kiểm soát
    − Không kiểm thử độc lập được
    − Đóng cửa vĩnh viễn khả năng tách service
```

### C. Modular Monolith — **được chọn**

```text
Ưu:
    + Kỷ luật của microservices, chi phí vận hành của monolith
    + Giao dịch đơn giản trong module
    + Sửa ranh giới sai là refactor, không phải di trú
    + Gỡ lỗi dễ (một stack trace)
    + Triển khai đơn giản (một artifact)
    + Giữ mở khả năng tách service khi có lý do

Nhược:
    − Kỷ luật phụ thuộc vào công cụ và văn hóa đội
    − Không mở rộng quy mô từng module độc lập
    − Một lỗi nghiêm trọng có thể ảnh hưởng toàn hệ thống
```

---

## Consequences

### Tích cực

```text
✓ Ra thị trường nhanh hơn
✓ Chi phí hạ tầng thấp ở giai đoạn đầu
✓ Nhất quán mạnh dễ đạt được cho tồn kho và tài chính
✓ Ranh giới có thể điều chỉnh khi hiểu domain rõ hơn
✓ Tách service sau này chỉ là thay cài đặt interface
```

### Tiêu cực

```text
− Cần đầu tư vào công cụ kiểm tra ranh giới ngay từ đầu
− Cần kỷ luật liên tục — kiến trúc module không tự duy trì
− Rủi ro: nếu kỷ luật lỏng, sau 2 năm sẽ thành monolith rối
```

### Biện pháp giảm rủi ro

```text
1. Kiểm tra ranh giới TỰ ĐỘNG trong CI, vi phạm = CI thất bại
   (không phải cảnh báo — cảnh báo sẽ bị bỏ qua)

2. Rà soát đồ thị phụ thuộc mỗi quý

3. Theo dõi dấu hiệu xuống cấp:
   - Interface công khai > 30 phương thức → module quá lớn
   - Hai module luôn sửa cùng nhau → ranh giới sai
   - kernel/ phình to → đang thành bãi rác
```

---

## Trade-offs

| Chấp nhận | Để đổi lấy |
|---|---|
| Không mở rộng từng module độc lập | Đơn giản vận hành, tốc độ phát triển |
| Kỷ luật phụ thuộc vào công cụ | Linh hoạt sửa ranh giới |
| Một lỗi có thể ảnh hưởng rộng | Giao dịch đơn giản, gỡ lỗi dễ |
| Đầu tư công cụ kiểm tra ranh giới | Khả năng tách service sau này |

---

## Lưu ý quan trọng

> Modular Monolith **không phải giai đoạn tạm bợ** chờ ngày lên microservices.

Nó là kiến trúc hợp lệ và duy trì được lâu dài. Việc tách service là **giải pháp cho một vấn đề cụ thể**, không phải mục tiêu tự thân.

Xem [ADR-0009](0009-service-extraction.md) về điều kiện tách service.

---

## Tài liệu liên quan

- [../03-architecture/modular-monolith.md](../03-architecture/modular-monolith.md)
- [ADR-0005](0005-module-boundaries.md) — ranh giới module
- [ADR-0009](0009-service-extraction.md) — hoãn tách service
